package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/release"
)

// releaseRepo is where the releases, their checksums and their build
// attestations are (www/install.sh, .github/workflows/publish.yml).
const releaseRepo = "blox-eng/matchblox"

// Updater replaces this binary with the latest release, the way
// www/install.sh installs one: the checksum, the provenance when gh can
// check it, and the version the new binary names.
type Updater struct {
	Base       string // https://github.com
	Client     *http.Client
	Self       string // this binary, its symlinks resolved
	Current    string // its version
	GOOS, Arch string
	LookPath   func(string) (string, error)
	Run        func(argv []string) (string, error) // combined output
	Out        io.Writer
}

// Proof is what Install checked before it replaced the binary.
type Proof struct {
	Checksum   bool
	Provenance bool // false when gh could not check it
}

// New is an Updater for this binary, at version current.
func New(current string, out io.Writer) (*Updater, error) {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		return nil, fmt.Errorf("could not find this binary: %w", err)
	}
	return &Updater{
		Base:     "https://github.com",
		Client:   &http.Client{Timeout: 5 * time.Minute},
		Self:     self,
		Current:  current,
		GOOS:     runtime.GOOS,
		Arch:     runtime.GOARCH,
		LookPath: exec.LookPath,
		Run: func(argv []string) (string, error) {
			out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput() //nolint:gosec // argv built by updater
			return string(out), err
		},
		Out: out,
	}, nil
}

func (u *Updater) say(format string, a ...any) { _, _ = fmt.Fprintf(u.Out, format+"\n", a...) }

// Update is `matchblox update`: it says what it does and what it finds.
func (u *Updater) Update() error {
	if cmd := u.Managed(); cmd != "" {
		u.say("Homebrew installed matchblox. Update it with Homebrew:\n  %s", cmd)
		return nil
	}
	if !release.IsRelease(u.Current) {
		return fmt.Errorf("this matchblox (%s) is not a release build, so it does not update itself. Install a release: curl -fsSL https://matchblox.sh | sh", u.Current)
	}
	if (u.GOOS != "linux" && u.GOOS != "darwin") || (u.Arch != "amd64" && u.Arch != "arm64") {
		return fmt.Errorf("matchblox is published for Linux and macOS on amd64 and arm64, not %s/%s", u.GOOS, u.Arch)
	}
	latest, err := u.Latest()
	if err != nil {
		return err
	}
	switch c := release.Compare(u.Current, latest); {
	case u.Current == latest:
		u.say("matchblox %s is the latest release.", u.Current)
		return nil
	case c > 0:
		u.say("matchblox %s is newer than the latest release, %s. Nothing to do.", u.Current, latest)
		return nil
	}
	u.say("matchblox %s → %s (%s/%s)", u.Current, latest, u.GOOS, u.Arch)
	if _, err := u.Install(latest); err != nil {
		return err
	}
	u.say("  updated %s", u.Self)
	u.say("Open matchblox: the console replaces the running service with %s.", latest)
	return nil
}

// Managed is the command that updates a binary a package manager installed,
// or "" when none did: replacing it would break that manager's own record
// of it.
func (u *Updater) Managed() string {
	if strings.Contains(u.Self, "/Cellar/matchblox/") {
		return "brew upgrade matchblox"
	}
	return ""
}

// Latest is the tag of the latest release. github.com/<repo>/releases/latest
// redirects to it, or to the release list when there is none.
func (u *Updater) Latest() (string, error) {
	c := *u.Client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(u.Base + "/" + releaseRepo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("could not reach GitHub to find the latest release: %w", err)
	}
	_ = resp.Body.Close()
	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("could not find the latest release: GitHub answered %s", resp.Status)
	}
	tag, ok := strings.CutPrefix(loc.String(), u.Base+"/"+releaseRepo+"/releases/tag/")
	if !ok {
		return "", fmt.Errorf("could not find the latest release: GitHub sent %s", loc)
	}
	if !release.IsRelease(tag) {
		return "", fmt.Errorf("the latest release %q is not a release tag", tag)
	}
	return tag, nil
}

// Install downloads the release beside this binary, checks it, and only
// then moves it over this binary. Nothing changes when a check fails.
func (u *Updater) Install(tag string) (proof Proof, err error) {
	asset := "matchblox-" + u.GOOS + "-" + u.Arch
	dl := u.Base + "/" + releaseRepo + "/releases/download/" + tag + "/" + asset
	dir := filepath.Dir(u.Self)
	f, err := os.CreateTemp(dir, ".matchblox-update-*")
	if err != nil {
		return Proof{}, fmt.Errorf("could not write to %s: %w. Run it as the user that owns %s, or with sudo", dir, err, u.Self)
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	sum, err := u.fetch(dl+".sha256", nil)
	if err != nil {
		return Proof{}, fmt.Errorf("could not download the checksum for %s: %w. Nothing was changed", asset, err)
	}
	want, _, _ := strings.Cut(strings.TrimSpace(string(sum)), " ")
	if want == "" {
		return Proof{}, errors.New("the published checksum is empty. Nothing was changed")
	}
	h := sha256.New()
	if _, err = u.fetch(dl, io.MultiWriter(f, h)); err != nil {
		return Proof{}, fmt.Errorf("could not download %s %s: %w. Nothing was changed", asset, tag, err)
	}
	if err = f.Close(); err != nil {
		return Proof{}, err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return Proof{}, fmt.Errorf("checksum mismatch for %s\n  published %s\n  download  %s\nNothing was changed", asset, want, got)
	}
	u.say("  checksum  ok")
	proof.Checksum = true

	if proof.Provenance, err = u.provenance(tmp, asset); err != nil {
		return Proof{}, err
	}

	if err = os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // an executable
		return Proof{}, err
	}
	says, _ := u.Run([]string{tmp, "version"})
	if says = strings.TrimSpace(says); says != tag {
		return Proof{}, fmt.Errorf("the %s binary says %q, not %s. Nothing was changed. Please report this: https://github.com/%s/issues", tag, says, tag, releaseRepo)
	}
	if err = os.Rename(tmp, u.Self); err != nil {
		return Proof{}, fmt.Errorf("could not replace %s: %w", u.Self, err)
	}
	return proof, nil
}

// provenance checks the build attestation: the checksum comes from the same
// release as the binary, so only the attestation proves that CI built it.
// Only a logged-in gh can check it; without one, it says so and goes on,
// as install.sh does.
func (u *Updater) provenance(path, asset string) (bool, error) {
	switch {
	case !u.has("gh"):
		u.say("  provenance not checked (install the 'gh' CLI to verify the build attestation)")
	case !u.ok("gh", "attestation", "verify", "--help"):
		u.say("  provenance not checked (gh is too old to verify attestations: update gh, then run this again)")
	case !u.ok("gh", "auth", "status"):
		u.say("  provenance not checked (gh is not logged in: gh auth login, then run this again)")
	default:
		if log, err := u.Run([]string{"gh", "attestation", "verify", path, "--repo", releaseRepo,
			"--signer-workflow", releaseRepo + "/.github/workflows/publish.yml"}); err != nil {
			return false, fmt.Errorf("provenance verification FAILED for %s.\n%s\nThe checksum matched, but the build attestation did not verify against %s.\nNothing was changed. Please report this: https://github.com/%s/security",
				asset, strings.TrimSpace(log), releaseRepo, releaseRepo)
		}
		u.say("  provenance ok (built by CI in %s)", releaseRepo)
		return true, nil
	}
	return false, nil
}

func (u *Updater) has(name string) bool { _, err := u.LookPath(name); return err == nil }

func (u *Updater) ok(argv ...string) bool { _, err := u.Run(argv); return err == nil }

// fetch GETs url into w, or returns the body when w is nil (a small file).
func (u *Updater) fetch(url string, w io.Writer) ([]byte, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	if w == nil {
		return io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	}
	_, err = io.Copy(w, resp.Body)
	return nil, err
}
