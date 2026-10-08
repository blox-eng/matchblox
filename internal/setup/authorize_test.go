package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeFakeFakeFakeFakeFakeFakeFakeFakeFakeFake matchblox"

func TestAuthorizeAddsTheGateLine(t *testing.T) {
	home := t.TempDir()
	own := "ssh-ed25519 AAAAOwnKey me@laptop\n"
	writeFile(t, filepath.Join(home, ".ssh", "authorized_keys"), own)
	backup, err := Authorize(home, "/opt/mb/matchblox", pub)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("no backup of authorized_keys")
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	want := `restrict,pty,command="'/opt/mb/matchblox' gate" ` + pub + "\n"
	if string(b) != own+want {
		t.Fatalf("authorized_keys:\n%s\nwant:\n%s", b, own+want)
	}
	fi, _ := os.Stat(filepath.Join(home, ".ssh", "authorized_keys"))
	di, _ := os.Stat(filepath.Join(home, ".ssh"))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatalf("modes %v %v, want 0600 and 0700", fi.Mode().Perm(), di.Mode().Perm())
	}
}

func TestAuthorizeTwiceKeepsOneLineAndMovesThePath(t *testing.T) {
	home := t.TempDir()
	if _, err := Authorize(home, "/old/matchblox", pub); err != nil {
		t.Fatal(err)
	}
	if _, err := Authorize(home, "/new/matchblox", pub); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if strings.Count(string(b), "AAAAC3Nza") != 1 || !strings.Contains(string(b), `command="'/new/matchblox' gate"`) {
		t.Fatalf("authorized_keys:\n%s", b)
	}
}

func TestAuthorizeMakesTheDirectoryPrivate(t *testing.T) {
	home := t.TempDir()
	if _, err := Authorize(home, "/opt/mb/matchblox", pub); err != nil {
		t.Fatal(err)
	}
	di, _ := os.Stat(filepath.Join(home, ".ssh"))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("~/.ssh mode %v", di.Mode().Perm())
	}
}

func TestAuthorizeRefusesWhatIsNotOneKey(t *testing.T) {
	home := t.TempDir()
	for _, bad := range []string{"ssh-rsa AAAA x", pub + "\nssh-ed25519 BBBB evil", `ssh-ed25519 AAAA" ,command="sh`} {
		if _, err := Authorize(home, "/opt/mb/matchblox", bad); err == nil {
			t.Errorf("Authorize(%q) wrote it", bad)
		}
	}
	for _, exe := range []string{"relative/matchblox", `/opt/mb"x/matchblox`, "/opt/mb'x/matchblox", "/opt/$HOME/matchblox",
		"/opt/`id`/matchblox", "/opt/a;b/matchblox", "/opt/a\nb/matchblox", `/opt/a\b/matchblox`} {
		if _, err := Authorize(home, exe, pub); err == nil {
			t.Errorf("Authorize with exe %q wrote it", exe)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A home with a space (/Users/John Smith on macOS) is common: the path is
// quoted for the shell sshd runs the command with.
func TestAuthorizeQuotesAPathWithASpace(t *testing.T) {
	home := t.TempDir()
	if _, err := Authorize(home, "/Users/John Smith/.local/bin/matchblox", pub); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if want := `command="'/Users/John Smith/.local/bin/matchblox' gate"`; !strings.Contains(string(b), want) {
		t.Fatalf("authorized_keys:\n%s\nwant %s", b, want)
	}
}
