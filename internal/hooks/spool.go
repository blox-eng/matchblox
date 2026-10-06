package hooks

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// Append adds one event to the spool. One write for each line, so hooks
// that run at the same time do not mix their lines.
func Append(spool string, ev Event) error {
	if err := os.MkdirAll(filepath.Dir(spool), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(spool, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

// Replay gives each spooled event to fn, oldest first, and empties the
// spool. It moves the file away before it reads, so a hook that appends
// meanwhile starts a new spool and loses nothing. A broken line is skipped.
func Replay(spool string, fn func(Event)) error {
	taken := spool + "." + strconv.Itoa(os.Getpid()) + ".replay"
	if err := os.Rename(spool, taken); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	f, err := os.Open(taken)
	if err != nil {
		return err
	}
	defer os.Remove(taken)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 2<<20)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.SessionID != "" {
			fn(ev)
		}
	}
	return sc.Err()
}
