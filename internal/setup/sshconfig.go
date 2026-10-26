package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// sshConfig is the builder's own ssh config.
func sshConfig(home string) string { return filepath.Join(home, ".ssh", "config") }

// keychainBlock is the lines that keep a key's passphrase in the macOS
// Keychain for host, and give the key to the agent at its first use. It is
// "" for a name ssh would not read as one host. UseKeychain is Apple's own:
// any other OpenSSH (Homebrew's) stops at it in any Host block, so a Match
// all before it tells every ssh to skip it.
func keychainBlock(host string) string {
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		host = host[i+1:] // ssh matches Host against the name without its user
	}
	if host == "" || !remote.ValidHost(host) {
		return ""
	}
	return "Match all\n  IgnoreUnknown UseKeychain\n\nHost " + host + "\n  AddKeysToAgent yes\n  UseKeychain yes\n"
}

// SSHKeychainDoor adds keychainBlock to ~/.ssh/config, for macOS after the
// console put a key on host (design §3.3).
func SSHKeychainDoor(home, host string) doors.Door {
	path := sshConfig(home)
	d := doors.Door{ID: doors.SSHKeychain, Title: "Keep the key's passphrase in the Keychain", Path: path,
		Why: "ssh to " + host + " takes the key's passphrase from the macOS Keychain, and gives the key to your agent."}
	block := keychainBlock(host)
	if block == "" {
		d.Problem = "not a host name: " + host
		return d
	}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	next := withBlock(old, block)
	if string(next) == string(old) {
		d.Done = true
		return d
	}
	d.Sum, d.Preview = Sum(old), diff(string(old), string(next))
	return d
}

// AddSSHKeychain appends keychainBlock to ~/.ssh/config once, when the
// file is still what the person saw.
func AddSSHKeychain(home, host, seen string) (backup string, err error) {
	block := keychainBlock(host)
	if block == "" {
		return "", errors.New("not a host name: " + host)
	}
	return edit(sshConfig(home), seen, func(old []byte) ([]byte, error) {
		next := withBlock(old, block)
		return next, sshReads(next, host)
	})
}

// sshReads checks that this machine's ssh reads config: a config it stops
// at stops every ssh of the builder, git's too. No ssh, no check.
func sshReads(config []byte, host string) error {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil
	}
	f, err := os.CreateTemp("", "matchblox-ssh-config-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(config)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	out, err := exec.Command("ssh", "-G", "-F", f.Name(), "--", host).CombinedOutput() //nolint:gosec // a host keychainBlock took
	if err != nil {
		return fmt.Errorf("ssh does not read ~/.ssh/config with the lines: %s", strings.TrimSpace(lastLine(string(out))))
	}
	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// withBlock is old with block at its end, after a blank line; old as it is
// when it has the block.
func withBlock(old []byte, block string) []byte {
	s := string(old)
	if strings.Contains(s, block) {
		return old
	}
	if s != "" {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n"
	}
	return []byte(s + block)
}
