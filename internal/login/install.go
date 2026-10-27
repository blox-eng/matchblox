package login

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/remote"
)

// installScript appends the key ($1) to the host's authorized_keys once
// (the key's blob says it is there, whatever its comment or options),
// and makes ~/.ssh 0700 and the file 0600 when it makes them. A last line
// without its newline gets one first, else the key would join it. It is
// fixed text: the key is only ever its argument.
const installScript = `umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; [ -s ~/.ssh/authorized_keys ] && [ "$(tail -c1 ~/.ssh/authorized_keys)" != "" ] && echo >> ~/.ssh/authorized_keys; k=$(printf '%s\n' "$1" | cut -d' ' -f2); grep -qF -- " $k" ~/.ssh/authorized_keys || printf '%s\n' "$1" >> ~/.ssh/authorized_keys`

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
	argv := append(append(append([]string{"ssh"}, master...), remote.NoForward()...), "-o", "ConnectTimeout=10", "--", host)
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

// wrongPassphrase is what ssh-keygen -y says when the passphrase it got
// does not open the key (OpenSSH 9.6, 2026-10-26).
const wrongPassphrase = "incorrect passphrase supplied to decrypt private key"

// Locked: the key file at path is locked by a passphrase. Only ssh-keygen
// refusing the empty passphrase counts; an open key, a file that is not a
// key, a missing one or a time-out is not locked.
func Locked(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-y", "-P", "", "-f", path).CombinedOutput() //nolint:gosec // a path of the builder's ~/.ssh
	return err != nil && ctx.Err() == nil && strings.Contains(string(out), wrongPassphrase)
}

// Made is a new key's two files as ssh-keygen made them.
type Made struct {
	paths []string
	files []os.FileInfo
}

// StatMade notes the private and .pub files at path right after
// ssh-keygen made them.
func StatMade(path string) (Made, error) {
	var m Made
	for _, p := range []string{path, path + ".pub"} {
		fi, err := os.Lstat(p)
		if err != nil {
			return Made{}, err
		}
		m.paths, m.files = append(m.paths, p), append(m.files, fi)
	}
	return m, nil
}

// Remove removes the files of m that are still the ones ssh-keygen made:
// the same file and modification time. Any other file stays, and is the
// error.
func (m Made) Remove() error {
	var errs []error
	for i, p := range m.paths {
		fi, err := os.Lstat(p)
		if err != nil || !os.SameFile(fi, m.files[i]) || !fi.ModTime().Equal(m.files[i].ModTime()) {
			errs = append(errs, fmt.Errorf("%s is not the file ssh-keygen made: it stays", p))
			continue
		}
		if err := os.Remove(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
