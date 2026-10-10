package release

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakeCurl = `#!/bin/sh
out= w= url=
while [ $# -gt 0 ]; do
  case $1 in
    -o) out=$2; shift 2 ;;
    -w) w=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
[ -z "${FAKE_OFFLINE:-}" ] || exit 6
echo "$url" >>"$FIX/curl.log"
case $url in
  */releases/latest)
    if [ -f "$FIX/latest" ]; then printf '%s' "https://github.com/blox-eng/matchblox/releases/tag/$(cat "$FIX/latest")"
    else printf '%s' "https://github.com/blox-eng/matchblox/releases"; fi ;;
  */releases/download/*)
    f="$FIX/$(echo "$url" | sed 's|.*/releases/download/||' | tr / _)"
    [ -f "$f" ] || exit 22
    cp "$f" "$out" ;;
  *) exit 22 ;;
esac
`

const fakeGh = `#!/bin/sh
echo "gh $*" >>"$FIX/gh.log"
case "$1 $2" in
  "auth status") exit "${FAKE_GH_AUTH:-0}" ;;
  "attestation verify") exit "${FAKE_GH_VERIFY:-0}" ;;
esac
exit 1
`

const fakeGo = `#!/bin/sh
echo "go $*" >>"$FIX/go.log"
case $1 in
  version) echo "go version go1.26.0 linux/amd64" ;;
  install) printf '#!/bin/sh\necho dev\n' >"$GOBIN/matchblox"; chmod 0755 "$GOBIN/matchblox" ;;
esac
`

const fakeUname = `#!/bin/sh
case $1 in -s) echo Linux ;; -m) echo x86_64 ;; esac
`

// world is a machine for install.sh: a fake GitHub (curl), gh, go and
// uname, and only the real tools the script uses.
type world struct {
	t                *testing.T
	fix, bin, target string
	env              []string
}

func newWorld(t *testing.T, fakes ...string) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{t: t, fix: filepath.Join(dir, "fix"), bin: filepath.Join(dir, "bin"), target: filepath.Join(dir, "home", "bin")}
	for _, d := range []string{w.fix, w.bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scripts := map[string]string{"curl": fakeCurl, "gh": fakeGh, "go": fakeGo, "uname": fakeUname}
	for _, name := range append([]string{"curl", "uname"}, fakes...) {
		w.write(filepath.Join(w.bin, name), scripts[name], 0o755)
	}
	for _, tool := range []string{"sh", "sed", "cut", "head", "mkdir", "mktemp", "rm", "install", "cp", "chmod", "cat", "tr", "sha256sum", "shasum", "grep", "perl"} {
		if p, err := exec.LookPath(tool); err == nil {
			_ = os.Symlink(p, filepath.Join(w.bin, tool))
		}
	}
	w.env = []string{"PATH=" + w.bin, "HOME=" + filepath.Join(dir, "home"), "FIX=" + w.fix,
		"MATCHBLOX_BIN_DIR=" + w.target, "MATCHBLOX_NO_START=1"}
	return w
}

func (w *world) write(p, s string, mode os.FileMode) {
	w.t.Helper()
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		w.t.Fatal(err)
	}
}

// release publishes tag with a linux-amd64 binary that says says.
func (w *world) release(tag, says string, latest bool) {
	w.t.Helper()
	bin := "#!/bin/sh\n[ \"$1\" = version ] && echo " + says + "\n"
	sum := sha256.Sum256([]byte(bin))
	w.write(filepath.Join(w.fix, tag+"_matchblox-linux-amd64"), bin, 0o644)
	w.write(filepath.Join(w.fix, tag+"_matchblox-linux-amd64.sha256"), hex.EncodeToString(sum[:])+"  matchblox-linux-amd64\n", 0o644)
	if latest {
		w.write(filepath.Join(w.fix, "latest"), tag, 0o644)
	}
}

func (w *world) run(env ...string) (string, error) {
	w.t.Helper()
	cmd := exec.Command(filepath.Join(w.bin, "sh"), filepath.Join("..", "..", "www", "install.sh"))
	cmd.Env = append(append([]string{}, w.env...), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (w *world) installed() string {
	b, err := os.ReadFile(filepath.Join(w.target, "matchblox"))
	if err != nil {
		return ""
	}
	return string(b)
}

func (w *world) log(name string) string {
	b, _ := os.ReadFile(filepath.Join(w.fix, name+".log"))
	return string(b)
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
	}
}

func TestInstallTheLatestReleaseVerified(t *testing.T) {
	w := newWorld(t, "gh")
	w.release("v0.1.0", "v0.1.0", true)
	out, err := w.run()
	if err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	mustContain(t, out, "matchblox v0.1.0 (linux/amd64)", "checksum  ok", "provenance ok", "installed "+filepath.Join(w.target, "matchblox"))
	if !strings.Contains(w.installed(), "echo v0.1.0") {
		t.Fatalf("installed %q", w.installed())
	}
	mustContain(t, w.log("gh"), "attestation verify", "--signer-workflow blox-eng/matchblox/.github/workflows/publish.yml")
}

func TestInstallTheVersionAsked(t *testing.T) {
	w := newWorld(t)
	w.release("v0.1.0", "v0.1.0", false)
	w.release("v0.2.0", "v0.2.0", true)
	out, err := w.run("MATCHBLOX_VERSION=v0.1.0")
	if err != nil || !strings.Contains(w.installed(), "echo v0.1.0") {
		t.Fatalf("%v, installed %q:\n%s", err, w.installed(), out)
	}
	if strings.Contains(w.log("curl"), "/releases/latest") {
		t.Fatalf("asked for the latest with a version set:\n%s", w.log("curl"))
	}
}

func TestInstallRefusesAVersionThatIsNoTag(t *testing.T) {
	w := newWorld(t)
	out, err := w.run("MATCHBLOX_VERSION=../../evil")
	if err == nil || w.installed() != "" {
		t.Fatalf("installed %q:\n%s", w.installed(), out)
	}
	mustContain(t, out, "not a release tag")
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	w := newWorld(t)
	w.release("v0.1.0", "v0.1.0", true)
	w.write(filepath.Join(w.fix, "v0.1.0_matchblox-linux-amd64"), "#!/bin/sh\necho evil\n", 0o644)
	out, err := w.run()
	if err == nil || w.installed() != "" {
		t.Fatalf("installed %q:\n%s", w.installed(), out)
	}
	mustContain(t, out, "checksum mismatch", "Nothing was installed")
}

func TestInstallRefusesABinaryThatNamesAnotherVersion(t *testing.T) {
	w := newWorld(t)
	w.release("v0.1.0", "dev", true)
	out, err := w.run()
	if err == nil || w.installed() != "" {
		t.Fatalf("installed %q:\n%s", w.installed(), out)
	}
	mustContain(t, out, `says "dev", not v0.1.0`, "Nothing was installed")
}

func TestInstallRefusesAFailedProvenance(t *testing.T) {
	w := newWorld(t, "gh")
	w.release("v0.1.0", "v0.1.0", true)
	out, err := w.run("FAKE_GH_VERIFY=1")
	if err == nil || w.installed() != "" {
		t.Fatalf("installed %q:\n%s", w.installed(), out)
	}
	mustContain(t, out, "provenance verification FAILED", "Nothing was installed")
}

func TestInstallSaysWhenGhCannotCheck(t *testing.T) {
	for _, c := range []struct {
		name  string
		fakes []string
		env   []string
		want  string
	}{
		{"no gh", nil, nil, "install the 'gh' CLI"},
		{"gh logged out", []string{"gh"}, []string{"FAKE_GH_AUTH=1"}, "gh auth login"},
	} {
		w := newWorld(t, c.fakes...)
		w.release("v0.1.0", "v0.1.0", true)
		out, err := w.run(c.env...)
		if err != nil || w.installed() == "" {
			t.Fatalf("%s: %v:\n%s", c.name, err, out)
		}
		mustContain(t, out, "provenance not checked", c.want)
		if strings.Contains(w.log("gh"), "attestation") {
			t.Fatalf("%s: verified with gh that cannot: %s", c.name, w.log("gh"))
		}
	}
}

func TestInstallBuildsFromSourceBeforeTheFirstRelease(t *testing.T) {
	w := newWorld(t, "go")
	out, err := w.run()
	if err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	mustContain(t, out, "no release yet", "building from source with go1.26.0")
	mustContain(t, w.log("go"), "install github.com/blox-eng/matchblox/cmd/matchblox@latest")
	if w.installed() == "" {
		t.Fatal("nothing installed")
	}
}

func TestInstallWithoutAReleaseOrGoSaysWhatToDo(t *testing.T) {
	w := newWorld(t)
	out, err := w.run()
	if err == nil {
		t.Fatalf("no error:\n%s", out)
	}
	mustContain(t, out, "no release yet", "Go 1.26", "https://go.dev/dl/")
}

func TestInstallOfflineInstallsNothing(t *testing.T) {
	w := newWorld(t, "go")
	out, err := w.run("FAKE_OFFLINE=1")
	if err == nil || w.installed() != "" {
		t.Fatalf("installed %q:\n%s", w.installed(), out)
	}
	mustContain(t, out, "could not reach github.com")
	if w.log("go") != "" {
		t.Fatalf("built from source while offline: %s", w.log("go"))
	}
}
