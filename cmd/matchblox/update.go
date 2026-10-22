package main

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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// releaseRepo is where the releases, their checksums and their build
// attestations are (www/install.sh, .github/workflows/publish.yml).
const releaseRepo = "blox-eng/matchblox"

// releaseTag is a release tag, as install.sh and publish.yml accept it.
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// updater replaces this binary with the latest release, the way
// www/install.sh installs one: the checksum, the provenance when gh can
// check it, and the version the new binary names.
type updater struct {
	base       string // https://github.com
	client     *http.Client
	self       string // this binary, its symlinks resolved
	current    string // its version
	goos, arch string
	lookPath   func(string) (string, error)
	run        func(argv []string) (string, error) // combined output
	out        io.Writer
}

func liveUpdater(out io.Writer) (*updater, error) {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		return nil, fmt.Errorf("could not find this binary: %w", err)
	}
	return &updater{
		base:     "https://github.com",
		client:   &http.Client{Timeout: 5 * time.Minute},
		self:     self,
		current:  version,
		goos:     runtime.GOOS,
		arch:     runtime.GOARCH,
		lookPath: exec.LookPath,
		run: func(argv []string) (string, error) {
			out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput() //nolint:gosec // argv built by updater
			return string(out), err
		},
		out: out,
	}, nil
}

func updateCmd(out io.Writer) error {
	u, err := liveUpdater(out)
	if err != nil {
		return err
	}
	return u.update()
}

func (u *updater) say(format string, a ...any) { _, _ = fmt.Fprintf(u.out, format+"\n", a...) }

func (u *updater) update() error {
	if u.deferred() {
		return nil
	}
	if !releaseTag.MatchString(u.current) {
		return fmt.Errorf("this matchblox (%s) is not a release build, so it does not update itself. Install a release: curl -fsSL https://matchblox.sh | sh", u.current)
	}
	if (u.goos != "linux" && u.goos != "darwin") || (u.arch != "amd64" && u.arch != "arm64") {
		return fmt.Errorf("matchblox is published for Linux and macOS on amd64 and arm64, not %s/%s", u.goos, u.arch)
	}
	latest, err := u.latest()
	if err != nil {
		return err
	}
	switch c := compareTags(u.current, latest); {
	case u.current == latest:
		u.say("matchblox %s is the latest release.", u.current)
		return nil
	case c > 0:
		u.say("matchblox %s is newer than the latest release, %s. Nothing to do.", u.current, latest)
		return nil
	}
	u.say("matchblox %s → %s (%s/%s)", u.current, latest, u.goos, u.arch)
	if err := u.install(latest); err != nil {
		return err
	}
	u.say("  updated %s", u.self)
	u.say("Open matchblox: the console replaces the running service with %s.", latest)
	return nil
}

// deferred tells how to update a binary that a package manager installed,
// and reports whether it did: replacing it would break that manager's own
// record of it.
func (u *updater) deferred() bool {
	if strings.Contains(u.self, "/Cellar/matchblox/") {
		u.say("Homebrew installed matchblox. Update it with Homebrew:\n  brew upgrade matchblox")
		return true
	}
	if _, err := u.lookPath("pacman"); err == nil {
		if pkg, err := u.run([]string{"pacman", "-Qqo", u.self}); err == nil {
			pkg = strings.TrimSpace(pkg)
			u.say("pacman installed matchblox (%s). Update it the way you installed it, for example:\n  yay -Syu %s", pkg, pkg)
			return true
		}
	}
	return false
}

// latest is the tag of the latest release. github.com/<repo>/releases/latest
// redirects to it, or to the release list when there is none.
func (u *updater) latest() (string, error) {
	c := *u.client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(u.base + "/" + releaseRepo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("could not reach GitHub to find the latest release: %w", err)
	}
	_ = resp.Body.Close()
	loc, err := resp.Location()
	if err != nil {
		return "", fmt.Errorf("could not find the latest release: GitHub answered %s", resp.Status)
	}
	tag, ok := strings.CutPrefix(loc.String(), u.base+"/"+releaseRepo+"/releases/tag/")
	if !ok {
		return "", fmt.Errorf("could not find the latest release: GitHub sent %s", loc)
	}
	if !releaseTag.MatchString(tag) {
		return "", fmt.Errorf("the latest release %q is not a release tag", tag)
	}
	return tag, nil
}

// install downloads the release beside this binary, checks it, and only
// then moves it over this binary. Nothing changes when a check fails.
func (u *updater) install(tag string) (err error) {
	asset := "matchblox-" + u.goos + "-" + u.arch
	dl := u.base + "/" + releaseRepo + "/releases/download/" + tag + "/" + asset
	dir := filepath.Dir(u.self)
	f, err := os.CreateTemp(dir, ".matchblox-update-*")
	if err != nil {
		return fmt.Errorf("could not write to %s: %w. Run it as the user that owns %s, or with sudo", dir, err, u.self)
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
		return fmt.Errorf("could not download the checksum for %s: %w. Nothing was changed", asset, err)
	}
	want, _, _ := strings.Cut(strings.TrimSpace(string(sum)), " ")
	if want == "" {
		return errors.New("the published checksum is empty. Nothing was changed")
	}
	h := sha256.New()
	if _, err = u.fetch(dl, io.MultiWriter(f, h)); err != nil {
		return fmt.Errorf("could not download %s %s: %w. Nothing was changed", asset, tag, err)
	}
	if err = f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s\n  published %s\n  download  %s\nNothing was changed", asset, want, got)
	}
	u.say("  checksum  ok")

	if err = u.provenance(tmp, asset); err != nil {
		return err
	}

	if err = os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // an executable
		return err
	}
	says, _ := u.run([]string{tmp, "version"})
	if says = strings.TrimSpace(says); says != tag {
		return fmt.Errorf("the %s binary says %q, not %s. Nothing was changed. Please report this: https://github.com/%s/issues", tag, says, tag, releaseRepo)
	}
	if err = os.Rename(tmp, u.self); err != nil {
		return fmt.Errorf("could not replace %s: %w", u.self, err)
	}
	return nil
}

// provenance checks the build attestation: the checksum comes from the same
// release as the binary, so only the attestation proves that CI built it.
// Only a logged-in gh can check it; without one, it says so and goes on,
// as install.sh does.
func (u *updater) provenance(path, asset string) error {
	switch {
	case !u.has("gh"):
		u.say("  provenance not checked (install the 'gh' CLI to verify the build attestation)")
	case !u.ok("gh", "attestation", "verify", "--help"):
		u.say("  provenance not checked (gh is too old to verify attestations: update gh, then run this again)")
	case !u.ok("gh", "auth", "status"):
		u.say("  provenance not checked (gh is not logged in: gh auth login, then run this again)")
	default:
		if log, err := u.run([]string{"gh", "attestation", "verify", path, "--repo", releaseRepo,
			"--signer-workflow", releaseRepo + "/.github/workflows/publish.yml"}); err != nil {
			return fmt.Errorf("provenance verification FAILED for %s.\n%s\nThe checksum matched, but the build attestation did not verify against %s.\nNothing was changed. Please report this: https://github.com/%s/security",
				asset, strings.TrimSpace(log), releaseRepo, releaseRepo)
		}
		u.say("  provenance ok (built by CI in %s)", releaseRepo)
	}
	return nil
}

func (u *updater) has(name string) bool { _, err := u.lookPath(name); return err == nil }

func (u *updater) ok(argv ...string) bool { _, err := u.run(argv); return err == nil }

// fetch GETs url into w, or returns the body when w is nil (a small file).
func (u *updater) fetch(url string, w io.Writer) ([]byte, error) {
	resp, err := u.client.Get(url)
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

// compareTags orders two release tags by their numbers: -1, 0 or 1. A
// pre-release of a version comes before the version.
func compareTags(a, b string) int {
	na, pa := tagParts(a)
	nb, pb := tagParts(b)
	for i := range na {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	return strings.Compare(pa, pb)
}

func tagParts(tag string) (n [3]int, pre string) {
	core, pre, _ := strings.Cut(strings.TrimPrefix(tag, "v"), "-")
	for i, s := range strings.SplitN(core, ".", 3) {
		n[i], _ = strconv.Atoi(s)
	}
	return n, pre
}
