package login

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/remote"
)

func TestInstallArgvRefusesBadLine(t *testing.T) {
	for _, l := range []string{"ssh-ed25519 AAAA; rm -rf ~", "", line("ssh-ed25519", "") + "\nssh-rsa AAAA", line("ssh-dss", "")} {
		if argv, err := InstallArgv("ws-1", l, nil); err == nil {
			t.Errorf("%q: %q", l, argv)
		}
	}
	if argv, err := InstallArgv("-oProxyCommand=id", line("ssh-ed25519", ""), nil); err == nil {
		t.Errorf("a host that is an option: %q", argv)
	}
}

// TestInstallArgv: the script is fixed text and the key is its $1, quoted
// once; the master's options go in front, and no BatchMode, so the host
// asks its password through askpass.
func TestInstallArgv(t *testing.T) {
	key := line("ssh-ed25519", "me@laptop")
	master := remote.MasterOpts("/m/ssh")
	argv, err := InstallArgv("ws-1", key, master)
	if err != nil {
		t.Fatal(err)
	}
	head := append(append(append([]string{"ssh"}, master...), remote.NoForward()...), "-o", "ConnectTimeout=10", "--", "ws-1")
	if !slices.Equal(argv[:len(argv)-1], head) {
		t.Fatalf("argv %q", argv)
	}
	const script = `umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; [ -s ~/.ssh/authorized_keys ] && [ "$(tail -c1 ~/.ssh/authorized_keys)" != "" ] && echo >> ~/.ssh/authorized_keys; k=$(printf '%s\n' "$1" | cut -d' ' -f2); grep -qF -- " $k" ~/.ssh/authorized_keys || printf '%s\n' "$1" >> ~/.ssh/authorized_keys`
	words, err := remote.Split(argv[len(argv)-1])
	if err != nil || !slices.Equal(words, []string{"/bin/sh", "-c", script, "sh", key}) {
		t.Fatalf("remote line %q: %q %v", argv[len(argv)-1], words, err)
	}
	if slices.Contains(argv, "BatchMode=yes") {
		t.Fatal("BatchMode: the host could not ask the password")
	}
}

// TestInstallScriptTwice runs the remote line here, as a host's shell
// would: the key goes in once, ~/.ssh is 0700 and authorized_keys 0600, and
// the lines already there stay as they are, a last line without its
// newline too.
func TestInstallScriptTwice(t *testing.T) {
	key := line("ssh-ed25519", "me@laptop")
	old := line("ssh-ed25519", "me@old")
	for _, c := range []struct {
		name, seed, want string
	}{
		{"no file", "", key + "\n"},
		{"an empty file", "\x00", key + "\n"},
		{"a last newline", old + "\n", old + "\n" + key + "\n"},
		{"no last newline", old, old + "\n" + key + "\n"},
		{"the key without its newline", key, key + "\n"},
		// The same key put there before with another comment, or options:
		// the key blob says it is there.
		{"the key with another comment", strings.Replace(key, "me@laptop", "me@desk", 1) + "\n", strings.Replace(key, "me@laptop", "me@desk", 1) + "\n"},
		{"the key with options", `from="10.0.0.0/8" ` + key + "\n", `from="10.0.0.0/8" ` + key + "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			ak := filepath.Join(home, ".ssh", "authorized_keys")
			if c.seed != "" {
				if err := os.MkdirAll(filepath.Dir(ak), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(ak, []byte(strings.TrimPrefix(c.seed, "\x00")), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runInstall(t, home, key)
			runInstall(t, home, key)
			b, err := os.ReadFile(ak)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != c.want {
				t.Fatalf("authorized_keys %q\nwant %q", b, c.want)
			}
			mode(t, filepath.Dir(ak), 0o700)
			mode(t, ak, 0o600)
		})
	}
}

// runInstall runs InstallArgv's remote line with HOME at home.
func runInstall(t *testing.T, home, key string) {
	t.Helper()
	argv, err := InstallArgv("ws-1", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := exec.Command("/bin/sh", "-c", argv[len(argv)-1]) //nolint:gosec // test
	c.Env = append(os.Environ(), "HOME="+home)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
}

func mode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != want {
		t.Errorf("%s mode %o, want %o", path, fi.Mode().Perm(), want)
	}
}

func TestNewKeyAndAddArgv(t *testing.T) {
	if got, want := NewKeyArgv("/h/.ssh/id_ed25519"), []string{"ssh-keygen", "-q", "-t", "ed25519", "-C", "matchblox-login", "-f", "/h/.ssh/id_ed25519"}; !slices.Equal(got, want) {
		t.Errorf("NewKeyArgv = %q", got)
	}
	if got := AddArgv("/k", "linux"); !slices.Equal(got, []string{"ssh-add", "/k"}) {
		t.Errorf("AddArgv linux = %q", got)
	}
	if got := AddArgv("/k", "darwin"); !slices.Equal(got, []string{"ssh-add", "--apple-use-keychain", "/k"}) {
		t.Errorf("AddArgv darwin = %q", got)
	}
}

// TestNewKeyHasPassphrase makes a key as the console does: the passphrase
// comes only through askpass, twice, and the key has it.
func TestNewKeyHasPassphrase(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen here")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "askpass")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho \"$1\" >> "+filepath.Join(dir, "asked")+"\necho p4ss\n"), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	key := filepath.Join(dir, "id_ed25519")
	argv := NewKeyArgv(key)
	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // test
	c.Env = append(os.Environ(), "SSH_ASKPASS="+helper, "SSH_ASKPASS_REQUIRE=force")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if HasNoPassphrase(key) {
		t.Fatal("the new key has no passphrase")
	}
	if err := exec.Command("ssh-keygen", "-y", "-P", "p4ss", "-f", key).Run(); err != nil { //nolint:gosec // test
		t.Fatalf("p4ss does not open the key: %v", err)
	}
	asked, _ := os.ReadFile(filepath.Join(dir, "asked"))
	if n := strings.Count(string(asked), "\n"); n != 2 {
		t.Fatalf("askpass asked %d times: %q", n, asked)
	}
}

// TestLockedNeedsTheWrongPassphraseWords: a key counts as locked only when
// ssh-keygen says the empty passphrase is wrong; any other end is not.
func TestLockedNeedsTheWrongPassphraseWords(t *testing.T) {
	dir := t.TempDir()
	open, locked, junk := filepath.Join(dir, "open"), filepath.Join(dir, "locked"), filepath.Join(dir, "junk")
	keygen(t, open, "")
	keygen(t, locked, "p4ss")
	if err := os.WriteFile(junk, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Locked(locked) {
		t.Error("a key with a passphrase is not locked?")
	}
	for _, p := range []string{open, junk, filepath.Join(dir, "missing")} {
		if Locked(p) {
			t.Errorf("%s is locked?", filepath.Base(p))
		}
	}
}

// TestRemoveMadeTakesOnlyItsOwnFiles: a file that is not the one ssh-keygen
// made (replaced after) stays; the ones it made go.
func TestRemoveMadeTakesOnlyItsOwnFiles(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id_ed25519")
	keygen(t, key, "")
	made, err := StatMade(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(key + ".pub"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key+".pub", []byte("someone else's\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := made.Remove(); err == nil {
		t.Error("a file it did not make: no error")
	}
	if _, err := os.Stat(key); err == nil {
		t.Error("the key it made is still there")
	}
	if b, _ := os.ReadFile(key + ".pub"); string(b) != "someone else's\n" {
		t.Errorf("the other .pub: %q", b)
	}

	keygen(t, key+"2", "")
	made, err = StatMade(key + "2")
	if err != nil {
		t.Fatal(err)
	}
	if err := made.Remove(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{key + "2", key + "2.pub"} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s is still there", p)
		}
	}
}
