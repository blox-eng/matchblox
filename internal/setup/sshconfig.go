package setup

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/remote"
)

// sshConfig is the builder's own ssh config.
func sshConfig(home string) string { return filepath.Join(home, ".ssh", "config") }

// keychainBlock is the lines that keep a key's passphrase in the macOS
// Keychain for host, and give the key to the agent at its first use. It is
// "" for a name ssh would not read as one host.
func keychainBlock(host string) string {
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		host = host[i+1:] // ssh matches Host against the name without its user
	}
	if host == "" || !remote.ValidHost(host) {
		return ""
	}
	return "Host " + host + "\n  AddKeysToAgent yes\n  UseKeychain yes\n"
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
	return edit(sshConfig(home), seen, func(old []byte) ([]byte, error) { return withBlock(old, block), nil })
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
