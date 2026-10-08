package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/blox-eng/matchblox/internal/remote"
)

// Authorize lets the matchblox key of a console start only `matchblox gate`
// on this host: one line in ~/.ssh/authorized_keys, after a backup. A key
// that is there already gets its line again, with the path of this binary.
// `restrict` turns off forwarding of ports, the agent and X11; `pty` allows
// the attach to a pane.
func Authorize(home, exe, pubKey string) (backup string, err error) {
	if !remote.ValidPubKey(pubKey) {
		return "", errors.New("not one ed25519 public key")
	}
	if !filepath.IsAbs(exe) || strings.ContainsAny(exe, " \t\"'\\") {
		return "", errors.New("the matchblox path must be absolute, with no space or quote: " + exe)
	}
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "authorized_keys")
	target, old, mode, err := read(path)
	if err != nil {
		return "", err
	}
	body := strings.Fields(pubKey)[1]
	var lines []string
	for line := range strings.SplitSeq(strings.TrimSuffix(string(old), "\n"), "\n") {
		if line != "" && !strings.Contains(line, " "+body) {
			lines = append(lines, line)
		}
	}
	lines = append(lines, `restrict,pty,command="`+exe+` gate" `+pubKey)
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
