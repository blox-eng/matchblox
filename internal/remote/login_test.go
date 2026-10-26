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

func TestMasterDirMode(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "matchblox")
	dir, err := EnsureMasterDir(cache)
	if err != nil || dir != filepath.Join(cache, "ssh") {
		t.Fatalf("EnsureMasterDir = %q, %v", dir, err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, %v", fi.Mode().Perm(), err)
	}
	if _, err := EnsureMasterDir(cache); err != nil {
		t.Fatalf("a second call: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureMasterDir(cache); err == nil {
		t.Fatal("a 0755 directory was taken")
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
