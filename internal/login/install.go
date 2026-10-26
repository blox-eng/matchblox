package login

import (
	"errors"

	"github.com/blox-eng/matchblox/internal/remote"
)

// installScript appends the key ($1) to the host's authorized_keys once,
// and makes ~/.ssh 0700 and the file 0600 when it makes them. It is fixed
// text: the key is only ever its argument.
const installScript = `umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; grep -qxF "$1" ~/.ssh/authorized_keys || printf '%s\n' "$1" >> ~/.ssh/authorized_keys`

// InstallArgv puts the key line on the host with the builder's own login,
// through the master options (the login it opens is the master the jumps
// use). It has no BatchMode: the host's password comes through askpass,
// whose environment goes on the command, never here.
func InstallArgv(host, line string, master []string) ([]string, error) {
	if !remote.ValidHost(host) {
		return nil, errors.New("not a host name")
	}
	if !remote.ValidUserKey(line) {
		return nil, errors.New("not a public key line")
	}
	argv := append(append([]string{"ssh"}, master...), "-o", "ConnectTimeout=10", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "--", host)
	return append(argv, remote.Quote([]string{"/bin/sh", "-c", installScript, "sh", line})), nil
}

// NewKeyArgv makes an ed25519 key at path. Without -N, ssh-keygen asks the
// passphrase twice through askpass; the console refuses an empty one.
func NewKeyArgv(path string) []string {
	return []string{"ssh-keygen", "-q", "-t", "ed25519", "-C", "matchblox-login", "-f", path}
}

// AddArgv puts the key in the builder's agent; on macOS the Keychain keeps
// its passphrase.
func AddArgv(path, goos string) []string {
	if goos == "darwin" {
		return []string{"ssh-add", "--apple-use-keychain", path}
	}
	return []string{"ssh-add", path}
}
