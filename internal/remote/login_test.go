package remote

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fixture is a recorded ssh -v stderr and its exit code (exits.txt).
func fixture(t *testing.T, name string) (string, int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "login", name))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join("testdata", "login", "exits.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for sc := bufio.NewScanner(f); sc.Scan(); {
		if n, code, ok := strings.Cut(sc.Text(), " "); ok && n == name {
			exit, err := strconv.Atoi(code)
			if err != nil {
				t.Fatal(err)
			}
			return string(b), exit
		}
	}
	t.Fatalf("%s has no exit code in exits.txt", name)
	return "", 0
}

func TestClassifyLogin(t *testing.T) {
	for _, c := range []struct {
		file      string
		want      Login
		detail    string
		tailscale bool
	}{
		{"tailscale-ok.stderr", LoginOK, "", true},
		{"tailscale-check.stderr", LoginTailscaleCheck, "https://login.tailscale.com/a/0123456789abcd", true},
		{"tailscale-denied.stderr", LoginTailscaleDenied, `tailnet policy does not permit you to SSH as user "nosuchuser"`, true},
		{"openssh-password.stderr", LoginPassword, "", false},
		{"openssh-hostkey-unknown.stderr", LoginHostKeyUnknown, "", false},
		{"openssh-hostkey-changed.stderr", LoginHostKeyChanged, "", false},
		{"openssh-refused.stderr", LoginUnreachable, "ssh: connect to host 127.0.0.1 port 1: Connection refused", false},
	} {
		stderr, exit := fixture(t, c.file)
		got, detail := ClassifyLogin(stderr, exit)
		if got != c.want || detail != c.detail {
			t.Errorf("%s: %v %q, want %v %q", c.file, got, detail, c.want, c.detail)
		}
		if Tailscale(stderr) != c.tailscale {
			t.Errorf("%s: Tailscale = %v", c.file, !c.tailscale)
		}
	}
}

// TestClassifyLoginShowsOnlyATailscaleURL: the URL comes from the server,
// so only a login.tailscale.com link of plain characters is shown.
func TestClassifyLoginShowsOnlyATailscaleURL(t *testing.T) {
	for _, url := range []string{
		"https://evil.example.com/a/0123",
		"https://login.tailscale.com.evil.example/a/0123",
		"https://login.tailscale.com/a/01\x1b]8;;x",
		"https://login.tailscale.com/a/0123?x=1",
		"http://login.tailscale.com/a/0123",
	} {
		stderr := "# Tailscale SSH requires an additional check.\n# To authenticate, visit: " + url + "\n"
		if got, detail := ClassifyLogin(stderr, 255); got != LoginTailscaleCheck || detail != "" {
			t.Errorf("%q: %v %q", url, got, detail)
		}
	}
}

// TestUnreachableSaysTimedOut: a check that ssh did not end by itself
// (exit -1), or that said nothing, timed out.
func TestUnreachableSaysTimedOut(t *testing.T) {
	for _, c := range []struct {
		stderr string
		exit   int
	}{
		{"debug1: Connecting to ws-1 port 22.\n", -1},
		{"", 255},
		{"debug1: Connecting to ws-1 port 22.\n", 255},
	} {
		if got, detail := ClassifyLogin(c.stderr, c.exit); got != LoginUnreachable || detail != "timed out" {
			t.Errorf("%q %d: %v %q", c.stderr, c.exit, got, detail)
		}
	}
}

// TestClassifyLoginCleansTheReason: Tailscale's reason and ssh's last line
// come from the network: no control characters reach the screen.
func TestClassifyLoginCleansTheReason(t *testing.T) {
	_, detail := ClassifyLogin("tailscale: no\x1b[2J \x07way\n", 255)
	if detail != "no[2J way" {
		t.Errorf("detail %q", detail)
	}
	_, detail = ClassifyLogin("debug1: x\nssh: bad\x1b]0;t\x07 host\n", 255)
	if detail != "ssh: bad]0;t host" {
		t.Errorf("detail %q", detail)
	}
}

func TestMasterOpts(t *testing.T) {
	want := []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m"}
	if got := MasterOpts("/c/ssh"); !slices.Equal(got, want) {
		t.Errorf("MasterOpts = %q", got)
	}
	if got := MasterOpts(""); got != nil {
		t.Errorf("no directory, no master: %q", got)
	}
}

func TestCheckArgv(t *testing.T) {
	want := []string{"ssh", "-v", "-o", "BatchMode=yes",
		"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m",
		"-o", "ConnectTimeout=10", "--", "ws-1", "true"}
	if got := CheckArgv("ws-1", "/c/ssh"); !slices.Equal(got, want) {
		t.Errorf("CheckArgv =\n %q\nwant\n %q", got, want)
	}
}

// TestMasterArgv: the master's check starts no connection; its open has
// no BatchMode, so ssh asks what it needs (through askpass), and runs
// nothing but true on the host.
func TestMasterArgv(t *testing.T) {
	master := []string{"-o", "ControlMaster=auto", "-o", "ControlPath=/c/ssh/%C", "-o", "ControlPersist=10m"}
	if got, want := MasterUpArgv("ws-1", "/c/ssh"), append(append([]string{"ssh"}, master...), "-O", "check", "--", "ws-1"); !slices.Equal(got, want) {
		t.Errorf("MasterUpArgv =\n %q\nwant\n %q", got, want)
	}
	want := append(append([]string{"ssh"}, master...), "-o", "ConnectTimeout=10", "--", "ws-1", "true")
	got := OpenArgv("ws-1", "/c/ssh")
	if !slices.Equal(got, want) {
		t.Errorf("OpenArgv =\n %q\nwant\n %q", got, want)
	}
	if slices.Contains(got, "BatchMode=yes") {
		t.Error("the open must be able to ask")
	}
}

// shortTemp is a temporary directory with a short path: a socket path
// under it must fit sun_path.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestMasterDirMode(t *testing.T) {
	base := filepath.Join(shortTemp(t), "m")
	dir, err := EnsureMasterDir(base)
	if err != nil || dir != filepath.Join(base, "ssh") {
		t.Fatalf("EnsureMasterDir = %q, %v", dir, err)
	}
	for _, d := range []string{base, dir} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: mode %v, %v", d, fi.Mode().Perm(), err)
		}
	}
	if _, err := EnsureMasterDir(base); err != nil {
		t.Fatalf("a second call: %v", err)
	}
	for _, d := range []string{dir, base} {
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := EnsureMasterDir(base); err == nil {
			t.Fatalf("a 0755 %s was taken", d)
		}
		if err := os.Chmod(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMasterDirSymlinkRefused: another user can make /tmp/matchblox-<uid>
// a link to a directory of theirs first.
func TestMasterDirSymlinkRefused(t *testing.T) {
	tmp := shortTemp(t)
	if err := os.Mkdir(filepath.Join(tmp, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tmp, "real"), filepath.Join(tmp, "m")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureMasterDir(filepath.Join(tmp, "m")); err == nil {
		t.Fatal("a link was taken")
	}
}

// TestMasterPathFits: ssh listens on ControlPath plus 17 bytes (".", 16
// random characters), and a path too long for sun_path ends ssh with 255.
// Such a directory gives no master.
func TestMasterPathFits(t *testing.T) {
	base := "/" + strings.Repeat("b", 69) // 70 bytes; the directory is base/ssh
	dir := base + "/ssh"
	if masterFits(dir, "darwin") || masterFits(dir, "linux") {
		t.Fatalf("%d bytes fit", len(dir))
	}
	if !masterFits("/tmp/matchblox-501/ssh", "darwin") || !masterFits("/tmp/matchblox-1000/ssh", "linux") {
		t.Fatal("the default directory does not fit")
	}
	if !masterFits(strings.Repeat("d", 49), "linux") || masterFits(strings.Repeat("d", 46), "darwin") {
		t.Fatal("the limits are 108 on linux and 104 on darwin, with the NUL")
	}
}

func TestTmuxTooOld(t *testing.T) {
	for v, old := range map[string]bool{
		"3.1c": true, "2.9a": true, "3.2": false, "3.2a": false, "3.4": false, "next-3.6": false,
		"": false, "master": false,
	} {
		if TmuxTooOld(v, MinHostTmux) != old {
			t.Errorf("TmuxTooOld(%q) = %v", v, !old)
		}
	}
}

// TestLineFoldsAJumpWithAMaster: the master options go first in the jump's
// ssh, and the line still folds them.
func TestLineFoldsAJumpWithAMaster(t *testing.T) {
	j := Jump{Host: "ws-1", Pane: "%1", Prefix: "C-b", ID: "0a1b2c3d"}
	got := Line(JumpSSH("ws-1", j, MasterOpts("/c/ssh")))
	if want := "ssh … ws-1 " + Quote(j.Remote()); got != want {
		t.Errorf("Line =\n %s\nwant\n %s", got, want)
	}
}
