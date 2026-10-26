package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
)

// TestSSHKeychainDoorAppendsOnce: the door shows the lines as a diff, keeps
// the file's bytes, appends them once, and is done after.
func TestSSHKeychainDoorAppendsOnce(t *testing.T) {
	home := t.TempDir()
	conf := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(conf), 0o700); err != nil {
		t.Fatal(err)
	}
	const old = "Host *\n  ServerAliveInterval 30" // no newline at its end
	if err := os.WriteFile(conf, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	d := SSHKeychainDoor(home, "me@ws-1")
	if d.ID != doors.SSHKeychain || d.Path != conf || d.Done || d.Problem != "" {
		t.Fatalf("door %+v", d)
	}
	for _, want := range []string{"+Host ws-1", "+  AddKeysToAgent yes", "+  UseKeychain yes"} {
		if !strings.Contains(d.Preview, want) {
			t.Fatalf("preview lacks %q:\n%s", want, d.Preview)
		}
	}
	backup, err := AddSSHKeychain(home, "me@ws-1", d.Sum)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	if want := old + "\n\nHost ws-1\n  AddKeysToAgent yes\n  UseKeychain yes\n"; string(b) != want {
		t.Fatalf("config =\n%q\nwant\n%q", b, want)
	}
	if bk, _ := os.ReadFile(backup); string(bk) != old {
		t.Fatalf("backup %q", bk)
	}
	if fi, _ := os.Stat(conf); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
	if d := SSHKeychainDoor(home, "ws-1"); !d.Done {
		t.Fatalf("not done: %+v", d)
	}
	if _, err := AddSSHKeychain(home, "ws-1", Sum(b)); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(conf); string(again) != string(b) {
		t.Fatalf("a second run appended again:\n%s", again)
	}
	if _, err := AddSSHKeychain(home, "ws-2", d.Sum); !errors.Is(err, ErrChanged) {
		t.Fatalf("a file that changed after the preview: %v", err)
	}
}

// TestSSHKeychainDoorNewFile: no ~/.ssh/config: the door makes it, 0600.
func TestSSHKeychainDoorNewFile(t *testing.T) {
	home := t.TempDir()
	d := SSHKeychainDoor(home, "ws-1")
	if _, err := AddSSHKeychain(home, "ws-1", d.Sum); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(home, ".ssh", "config")
	b, _ := os.ReadFile(conf)
	if string(b) != "Host ws-1\n  AddKeysToAgent yes\n  UseKeychain yes\n" {
		t.Fatalf("config %q", b)
	}
	if fi, _ := os.Stat(conf); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}

// TestSSHKeychainDoorRefusesABadHost: the host goes in a config line.
func TestSSHKeychainDoorRefusesABadHost(t *testing.T) {
	home := t.TempDir()
	for _, h := range []string{"ws-1\n  ProxyCommand id", "-oX", "", "a b"} {
		if d := SSHKeychainDoor(home, h); d.Problem == "" {
			t.Errorf("%q: %+v", h, d)
		}
		if _, err := AddSSHKeychain(home, h, Sum(nil)); err == nil {
			t.Errorf("%q written", h)
		}
	}
}
