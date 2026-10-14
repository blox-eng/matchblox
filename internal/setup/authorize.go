package setup

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blox-eng/matchblox/internal/remote"
)

// exePath is a binary path the gate line can hold: sshd runs it through the
// login shell inside single quotes, within the option's double quotes.
var exePath = regexp.MustCompile(`^/[A-Za-z0-9._+@/ -]+$`)

// Authorize lets the matchblox key of a console start only `matchblox gate`
// on this host: one line in ~/.ssh/authorized_keys, after a backup. A key
// that is there already gets its line again, with the path of this binary.
// `restrict` turns off forwarding of ports, the agent and X11; `pty` allows
// the attach to a pane.
func Authorize(home, exe, pubKey string) (backup string, err error) {
	if !remote.ValidPubKey(pubKey) {
		return "", errors.New("not one ed25519 public key")
	}
	if !exePath.MatchString(exe) {
		return "", errors.New("the matchblox path has a character the gate line cannot hold (only letters, digits, space and ._+@/- ): " + exe +
			"; install matchblox in a plain directory, such as /usr/local/bin")
	}
	return putKeyLine(home, strings.Fields(pubKey)[1], `restrict,pty,command="'`+exe+`' gate" `+pubKey)
}

// putKeyLine adds line to ~/.ssh/authorized_keys, after a backup, in place
// of the lines of the same key. Every other line keeps its bytes.
func putKeyLine(home, body, line string) (backup string, err error) {
	err = withKeysLock(home, func() error {
		backup, err = putKeyLineLocked(home, body, line)
		return err
	})
	return backup, err
}

func putKeyLineLocked(home, body, line string) (backup string, err error) {
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "authorized_keys")
	target, old, mode, err := read(path)
	if err != nil {
		return "", err
	}
	var lines []string
	for l := range strings.SplitSeq(strings.TrimSuffix(string(old), "\n"), "\n") {
		if l != "" && !strings.Contains(l, " "+body) {
			lines = append(lines, l)
		}
	}
	lines = append(lines, line)
	next := strings.Join(lines, "\n") + "\n"
	if next == string(old) {
		return "", nil
	}
	if old != nil {
		if backup, err = keep(target, old, mode); err != nil {
			return "", err
		}
	}
	return backup, replace(target, []byte(next), mode)
}
