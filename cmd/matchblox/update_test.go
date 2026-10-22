package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releaseWorld is a GitHub with one latest release, a gh, and this binary
// in a directory of its own.
type releaseWorld struct {
	t       *testing.T
	latest  string // "" is a repository with no release
	bin     []byte // matchblox-linux-amd64 of the latest release
	sum     string // its published .sha256, the true one when ""
	hits    []string
	gh      string // "" no gh, "old", "out" (not logged in), "ok", "fail"
	calls   []string
	out     bytes.Buffer
	updater *updater
}

func newReleaseWorld(t *testing.T, current, latest string) *releaseWorld {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the release binary is a shell script here")
	}
	w := &releaseWorld{t: t, latest: latest, gh: "ok", bin: []byte("#!/bin/sh\necho " + latest + "\n")}
	srv := httptest.NewServer(http.HandlerFunc(w.serve))
	t.Cleanup(srv.Close)
	self := filepath.Join(t.TempDir(), "matchblox")
	if err := os.WriteFile(self, []byte("old"), 0o755); err != nil { //nolint:gosec // an executable
		t.Fatal(err)
	}
	w.updater = &updater{
		base: srv.URL, client: srv.Client(), self: self, current: current, goos: "linux", arch: "amd64",
		lookPath: w.lookPath, run: w.run, out: &w.out,
	}
	return w
}

func (w *releaseWorld) serve(rw http.ResponseWriter, r *http.Request) {
	w.hits = append(w.hits, r.URL.Path)
	base := "http://" + r.Host + "/" + releaseRepo + "/releases"
	switch p := strings.TrimPrefix(r.URL.Path, "/"+releaseRepo+"/releases/"); {
	case p == "latest" && w.latest == "":
		http.Redirect(rw, r, base, http.StatusFound)
	case p == "latest":
		http.Redirect(rw, r, base+"/tag/"+w.latest, http.StatusFound)
	case p == "download/"+w.latest+"/matchblox-linux-amd64":
		_, _ = rw.Write(w.bin)
	case p == "download/"+w.latest+"/matchblox-linux-amd64.sha256":
		sum := w.sum
		if sum == "" {
			h := sha256.Sum256(w.bin)
			sum = hex.EncodeToString(h[:])
		}
		_, _ = rw.Write([]byte(sum + "  matchblox-linux-amd64\n"))
	default:
		http.NotFound(rw, r)
	}
}

func (w *releaseWorld) lookPath(name string) (string, error) {
	if name == "gh" && w.gh != "" {
		return "/usr/bin/" + name, nil
	}
	return "", exec.ErrNotFound
}

func (w *releaseWorld) run(argv []string) (string, error) {
	line := strings.Join(argv, " ")
	w.calls = append(w.calls, line)
	fail := errors.New("exit status 1")
	switch {
	case line == "gh attestation verify --help":
		if w.gh == "old" {
			return `unknown command "attestation" for "gh"`, fail
		}
		return "", nil
	case line == "gh auth status":
		if w.gh == "out" {
			return "You are not logged into any GitHub hosts", fail
		}
		return "", nil
	case argv[0] == "gh":
		if w.gh == "fail" {
			return "sigstore: no matching attestation", fail
		}
		return "", nil
	}
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput() //nolint:gosec // the downloaded test binary
	return string(out), err
}

// installed is what the binary's path holds now, and it checks that no
// download is left beside it.
func (w *releaseWorld) installed() string {
	w.t.Helper()
	b, err := os.ReadFile(w.updater.self)
	if err != nil {
		w.t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(w.updater.self), ".matchblox-update-*"))
	if len(left) > 0 {
		w.t.Errorf("a download is left behind: %v", left)
	}
	return string(b)
}

func TestUpdateInstallsTheLatestReleaseVerified(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	if err := w.updater.update(); err != nil {
		t.Fatalf("%v\n%s", err, w.out.String())
	}
	if got := w.installed(); got != string(w.bin) {
		t.Fatalf("installed %q, want the v0.2.0 binary", got)
	}
	mustContain(t, w.out.String(), "matchblox v0.1.0 → v0.2.0 (linux/amd64)", "checksum  ok", "provenance ok", "updated "+w.updater.self)
	mustContain(t, strings.Join(w.calls, "\n"), "--repo blox-eng/matchblox --signer-workflow blox-eng/matchblox/.github/workflows/publish.yml")
	if st, _ := os.Stat(w.updater.self); st.Mode().Perm() != 0o755 {
		t.Errorf("mode %v, want 0755", st.Mode().Perm())
	}
}

func TestUpdateFromAPreReleaseOfTheLatest(t *testing.T) {
	w := newReleaseWorld(t, "v0.2.0-rc.1", "v0.2.0")
	if err := w.updater.update(); err != nil {
		t.Fatalf("%v\n%s", err, w.out.String())
	}
	if got := w.installed(); got != string(w.bin) {
		t.Fatalf("installed %q, want the v0.2.0 binary", got)
	}
}

func TestUpdateOnTheLatestDoesNothing(t *testing.T) {
	w := newReleaseWorld(t, "v0.2.0", "v0.2.0")
	if err := w.updater.update(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, w.out.String(), "matchblox v0.2.0 is the latest release.")
	if got := w.installed(); got != "old" || len(w.hits) != 1 {
		t.Fatalf("installed %q after %v", got, w.hits)
	}
}

func TestUpdateNewerThanTheLatestDoesNothing(t *testing.T) {
	w := newReleaseWorld(t, "v0.10.0", "v0.9.0")
	if err := w.updater.update(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, w.out.String(), "v0.10.0 is newer than the latest release, v0.9.0")
	if got := w.installed(); got != "old" {
		t.Fatalf("installed %q", got)
	}
}

func TestUpdateRefusesABadChecksum(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	w.sum = strings.Repeat("0", 64)
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") || !strings.Contains(err.Error(), "Nothing was changed") {
		t.Fatalf("err = %v", err)
	}
	if got := w.installed(); got != "old" {
		t.Fatalf("installed %q", got)
	}
	if calls := strings.Join(w.calls, "\n"); strings.Contains(calls, "gh attestation verify "+filepath.Dir(w.updater.self)) {
		t.Errorf("verified the provenance of a bad download: %s", calls)
	}
}

func TestUpdateRefusesAFailedProvenance(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	w.gh = "fail"
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "provenance verification FAILED") || !strings.Contains(err.Error(), "no matching attestation") {
		t.Fatalf("err = %v", err)
	}
	if got := w.installed(); got != "old" {
		t.Fatalf("installed %q", got)
	}
}

func TestUpdateSaysWhenGhCannotCheck(t *testing.T) {
	for gh, want := range map[string]string{
		"":    "install the 'gh' CLI",
		"old": "gh is too old",
		"out": "gh is not logged in",
	} {
		w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
		w.gh = gh
		if err := w.updater.update(); err != nil {
			t.Fatalf("gh %q: %v", gh, err)
		}
		mustContain(t, w.out.String(), "provenance not checked ("+want, "checksum  ok")
		if got := w.installed(); got != string(w.bin) {
			t.Fatalf("gh %q: installed %q", gh, got)
		}
	}
}

func TestUpdateRefusesABinaryThatNamesAnotherVersion(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	w.bin = []byte("#!/bin/sh\necho v0.1.9\n")
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), `says "v0.1.9", not v0.2.0`) {
		t.Fatalf("err = %v", err)
	}
	if got := w.installed(); got != "old" {
		t.Fatalf("installed %q", got)
	}
}

func TestUpdateDefersToHomebrew(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	cellar := filepath.Join(t.TempDir(), "Cellar", "matchblox", "0.1.0", "bin")
	if err := os.MkdirAll(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	w.updater.self = filepath.Join(cellar, "matchblox")
	if err := w.updater.update(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, w.out.String(), "Homebrew installed matchblox", "brew upgrade matchblox")
	if len(w.hits) != 0 {
		t.Fatalf("reached GitHub: %v", w.hits)
	}
}

func TestUpdateRefusesABuildFromSource(t *testing.T) {
	w := newReleaseWorld(t, "dev", "v0.2.0")
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "not a release build") || !strings.Contains(err.Error(), "matchblox.sh") {
		t.Fatalf("err = %v", err)
	}
	if len(w.hits) != 0 {
		t.Fatalf("reached GitHub: %v", w.hits)
	}
}

func TestUpdateWithoutARelease(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "")
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "could not find the latest release") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateRefusesAPlatformWithNoRelease(t *testing.T) {
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	w.updater.goos = "windows"
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "not windows/amd64") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateInADirectoryItCannotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	w := newReleaseWorld(t, "v0.1.0", "v0.2.0")
	dir := filepath.Dir(w.updater.self)
	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // read-only on purpose
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // back, for TempDir's cleanup
	err := w.updater.update()
	if err == nil || !strings.Contains(err.Error(), "could not write to "+dir) {
		t.Fatalf("err = %v", err)
	}
	if got := w.installed(); got != "old" {
		t.Fatalf("installed %q", got)
	}
}

func TestCompareTags(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.2.0-rc.1", "v0.2.0", -1},
		{"v0.2.0", "v0.2.0-rc.1", 1},
		{"v0.2.1-0.20261022-abc", "v0.2.0", 1},
	} {
		if got := compareTags(c.a, c.b); got != c.want {
			t.Errorf("compareTags(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
