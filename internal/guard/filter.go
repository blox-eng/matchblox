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

// Notice is the one line the pane shows while the agent replays.
const Notice = "Cursor redraws for the new width…"

const (
	// tailLines and tailBytes bound what the pane gets of a replay: its
	// last lines.
	tailLines = 1000
	tailBytes = 4 << 20
	// A replay is a burst. It has ended when an endWindow brings under
	// endBytes: Cursor's own drawing and a streamed answer are far slower.
	endWindow = 250 * time.Millisecond
	endBytes  = 16 << 10
	// holdCap ends a hold that never slows: the pane then shows what
	// comes, as without a guard.
	holdCap = 60 * time.Second
)

// Filter writes an agent's output to out: unchanged, but for a replay.
type Filter struct {
	mu      sync.Mutex
	out     io.Writer
	now     func() time.Time
	pend    []byte    // output held back: it could be the start of ClearCodes
	pendAt  time.Time // when pend last grew
	holding bool
	tail    []byte    // the end of the replay so far
	heldAt  time.Time // when the hold began
	winAt   time.Time // when the window that counts winBytes began
	winSize int       // bytes of the replay in that window
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
	return len(p), f.write(p)
}

func (f *Filter) write(p []byte) error {
	if f.holding {
		f.hold(p)
		return nil
	}
	data := append(f.pend, p...)
	f.pend = nil
	if i := bytes.Index(data, []byte(ClearCodes)); i >= 0 {
		if _, err := f.out.Write(append(data[:i:i], ClearCodes+Notice...)); err != nil {
			return err
		}
		f.holding, f.tail, f.winSize = true, nil, 0
		f.heldAt = f.now()
		f.winAt = f.heldAt
		f.hold(data[i+len(ClearCodes):])
		return nil
	}
	keep := codesPrefix(data)
	if keep > 0 {
		f.pend = append([]byte(nil), data[len(data)-keep:]...)
		f.pendAt = f.now()
	}
	_, err := f.out.Write(data[:len(data)-keep])
	return err
}

// hold keeps the end of a replay: its last tailLines lines, from its last
// clear on.
func (f *Filter) hold(p []byte) {
	f.winSize += len(p)
	f.tail = append(f.tail, p...)
	if i := bytes.LastIndex(f.tail, []byte(ClearCodes)); i >= 0 {
		f.tail = f.tail[i+len(ClearCodes):]
	}
	from := 0
	if over := len(f.tail) - tailBytes; over > 0 {
		from = over // then on to the next whole line
		if i := bytes.IndexByte(f.tail[over:], '\n'); i >= 0 {
			from = over + i + 1
		}
	}
	lines := 0
	for i := len(f.tail) - 1; i >= from; i-- {
		if f.tail[i] != '\n' {
			continue
		}
		if lines++; lines > tailLines {
			from = i + 1
			break
		}
	}
	if from > 0 {
		f.tail = append(f.tail[:0], f.tail[from:]...)
	}
}

// Tick lets time pass. Output that waited for the rest of ClearCodes, and
// got none, passes. A replay that has ended, or runs over holdCap, gives
// the pane its end.
func (f *Filter) Tick() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.holding {
		ended := false
		if now.Sub(f.winAt) >= endWindow {
			ended = f.winSize < endBytes
			f.winAt, f.winSize = now, 0
		}
		if !ended && now.Sub(f.heldAt) < holdCap {
			return nil
		}
		tail := f.tail
		f.holding, f.tail = false, nil
		_, err := f.out.Write(append([]byte(ClearCodes+"\x1b[0m"), tail...))
		return err
	}
	if len(f.pend) == 0 || now.Sub(f.pendAt) < pendWait {
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
