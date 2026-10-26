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
	head := append(append([]string{"ssh"}, master...), "-o", "ConnectTimeout=10", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "--", "ws-1")
	if !slices.Equal(argv[:len(argv)-1], head) {
		t.Fatalf("argv %q", argv)
	}
	const script = `umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; grep -qxF "$1" ~/.ssh/authorized_keys || printf '%s\n' "$1" >> ~/.ssh/authorized_keys`
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
// a line already there stays as it is.
func TestInstallScriptTwice(t *testing.T) {
	home := t.TempDir()
	key := line("ssh-ed25519", "me@laptop")
	argv, err := InstallArgv("ws-1", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		c := exec.Command("/bin/sh", "-c", argv[len(argv)-1]) //nolint:gosec // test
		c.Env = append(os.Environ(), "HOME="+home)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	b, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != key+"\n" {
		t.Fatalf("authorized_keys %q", b)
	}
	mode(t, filepath.Join(home, ".ssh"), 0o700)
	mode(t, filepath.Join(home, ".ssh", "authorized_keys"), 0o600)
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
