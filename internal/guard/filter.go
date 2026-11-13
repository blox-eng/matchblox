// Package guard stands between an agent and its terminal. Cursor's CLI
// clears the screen and prints its whole chat again each time the
// terminal's width changes (#399); the guard shows the end of that replay
// and not the replay.
package guard

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// ClearCodes are what Cursor writes before it prints its chat again: cursor
// home, clear the screen, clear the scrollback (its alt-screen component,
// read in 2026.10.19). A clear without the last part is not a replay.
const ClearCodes = "\x1b[H\x1b[2J\x1b[3J"

// pendWait is how long output that could be the start of ClearCodes waits
// for its next byte before it passes as it is.
const pendWait = 20 * time.Millisecond

// Filter writes an agent's output to out: unchanged, but for a replay.
type Filter struct {
	mu      sync.Mutex
	out     io.Writer
	now     func() time.Time
	pend    []byte    // output held back: it could be the start of ClearCodes
	pendAt  time.Time // when pend last grew
	holding bool
}

// NewFilter is a Filter that writes to out and reads the time from now.
func NewFilter(out io.Writer, now func() time.Time) *Filter {
	return &Filter{out: out, now: now}
}

// Holding tells if the agent replays now.
func (f *Filter) Holding() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holding
}

// Write takes the agent's next output.
func (f *Filter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data := append(f.pend, p...)
	f.pend = nil
	if i := bytes.Index(data, []byte(ClearCodes)); i >= 0 {
		f.holding = true
		_, err := f.out.Write(data)
		return len(p), err
	}
	keep := codesPrefix(data)
	if keep > 0 {
		f.pend = append([]byte(nil), data[len(data)-keep:]...)
		f.pendAt = f.now()
	}
	_, err := f.out.Write(data[:len(data)-keep])
	return len(p), err
}

// Tick lets time pass: output that waited for the rest of ClearCodes, and
// got none, passes.
func (f *Filter) Tick() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pend) == 0 || f.now().Sub(f.pendAt) < pendWait {
		return nil
	}
	pend := f.pend
	f.pend = nil
	_, err := f.out.Write(pend)
	return err
}

// codesPrefix is how many bytes at the end of data are a start of
// ClearCodes, and not all of it.
func codesPrefix(data []byte) int {
	for n := min(len(ClearCodes)-1, len(data)); n > 0; n-- {
		if bytes.HasSuffix(data, []byte(ClearCodes[:n])) {
			return n
		}
	}
	return 0
}
