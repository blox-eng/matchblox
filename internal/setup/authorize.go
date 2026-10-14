package setup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/remote"
)

// exePath is a binary path the gate line can hold: sshd runs it through the
// login shell inside single quotes, within the option's double quotes.
var exePath = regexp.MustCompile(`^/[A-Za-z0-9._+@/ -]+$`)

func checkExe(exe string) error {
	if exePath.MatchString(exe) {
		return nil
	}
	return errors.New("the matchblox path has a character the gate line cannot hold (only letters, digits, space and ._+@/- ): " + exe +
		"; install matchblox in a plain directory, such as /usr/local/bin")
}

// Authorize lets the matchblox key of a console start only `matchblox gate`
// on this host: one line in ~/.ssh/authorized_keys, after a backup. A key
// that is there already gets its line again, with the path of this binary.
// `restrict` turns off forwarding of ports, the agent and X11; `pty` allows
// the attach to a pane.
func Authorize(home, exe, pubKey string) (backup string, err error) {
	if !remote.ValidPubKey(pubKey) {
		return "", errors.New("not one ed25519 public key")
	}
	if err := checkExe(exe); err != nil {
		return "", err
	}
	return putKeyLine(home, strings.Fields(pubKey)[1], `restrict,pty,command="'`+exe+`' gate" `+pubKey, true)
}

// putKeyLine adds line to ~/.ssh/authorized_keys in place of the lines of
// the same key. Every other line keeps its bytes.
func putKeyLine(home, body, line string, keepBackup bool) (backup string, err error) {
	return rewriteKeys(home, keepBackup, func(old []byte) ([]byte, error) { return withLine(old, body, line), nil }, nil)
}

func withLine(old []byte, body, line string) []byte {
	var lines []string
	for l := range strings.SplitSeq(strings.TrimSuffix(string(old), "\n"), "\n") {
		if l != "" && !hasKey(l, body) {
			lines = append(lines, l)
		}
	}
	return []byte(strings.Join(append(lines, line), "\n") + "\n")
}

// hasKey is whether a line holds the key body as one of its fields; sshd
// takes a space or a tab between them.
func hasKey(line, body string) bool { return slices.Contains(strings.Fields(line), body) }

// beforeReplace runs between the change and the second read; tests write
// the file there.
var beforeReplace = func() {}

// rewriteKeys writes change(old) to ~/.ssh/authorized_keys under the lock,
// after a backup when keepBackup. Other programs (ssh-copy-id, an editor) do
// not take the lock, so the file is read again just before the rename, and
// the change starts again when the file moved. A link stays a link. written,
// when not nil, runs after the write and still under the lock.
func rewriteKeys(home string, keepBackup bool, change func(old []byte) ([]byte, error), written func()) (backup string, err error) {
	err = withKeysLock(home, func() error {
		if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
			return err
		}
		for range 3 {
			target, old, mode, err := read(authorizedKeys(home))
			if err != nil {
				return err
			}
			next, err := change(old)
			if err != nil || bytes.Equal(next, old) {
				return err
			}
			beforeReplace()
			_, now, _, err := read(authorizedKeys(home))
			if err != nil {
				return err
			}
			if !bytes.Equal(now, old) {
				continue
			}
			if keepBackup && old != nil {
				if backup, err = keep(target, old, mode); err != nil {
					return err
				}
			}
			if err := replace(target, next, mode); err != nil {
				return err
			}
			if written != nil {
				written()
			}
			return nil
		}
		return errors.New("authorized_keys changed three times while matchblox wrote it; run the command again")
	})
	return backup, err
}
