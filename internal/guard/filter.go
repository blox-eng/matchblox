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

// Notice is the one line the pane shows while the agent replays.
const Notice = "Cursor redraws for the new width…"

// SessionEnv names the session a guard runs for, for the words that say
// how to start it again.
const SessionEnv = "MATCHBLOX_SESSION"

// LinkName is the name the guard runs under: a link to matchblox. tmux
// names a pane's command after its program, and a pane named matchblox is a
// console to the way back.
const LinkName = "matchblox-guard"

const (
	// tailLines and tailBytes bound what the pane gets of a replay: its
	// last lines.
	tailLines = 1000
	tailBytes = 4 << 20
	// armFor is how long after the agent's start, or a change of width, its
	// clear starts a hold. A clear at another time is the agent's own (a
	// new chat), with no replay behind it.
	armFor = 3 * time.Second
	// quietFor without output ends a hold: the replay is over. A rate tells
	// nothing: a renderer at work on a long chat can be slow.
	quietFor = 300 * time.Millisecond
	// holdCap ends a hold that never goes quiet: the pane then shows what
	// comes, as without a guard.
	holdCap = 60 * time.Second
	// pendWait is how long output that could be the start of ClearCodes
	// waits for its next byte before it passes as it is.
	pendWait = 20 * time.Millisecond
	// seqMax is the longest escape sequence the hold reads as one: a longer
	// one is the chat's (an image), not a query.
	seqMax = 4 << 10
)

// Filter writes an agent's output to out: unchanged, but for a replay.
type Filter struct {
	mu      sync.Mutex
	out     io.Writer
	now     func() time.Time
	pend    []byte    // output held back: it could be the start of ClearCodes
	pendAt  time.Time // when pend last grew
	armed   time.Time // until when a clear starts a hold
	holding bool
	tail    []byte    // the end of the replay so far
	seq     []byte    // an escape sequence of the replay, not yet whole
	heldAt  time.Time // when the hold began
	lastAt  time.Time // when the replay last printed
}

// NewFilter is a Filter that writes to out and reads the time from now.
func NewFilter(out io.Writer, now func() time.Time) *Filter {
	return &Filter{out: out, now: now}
}

// Arm tells the filter that the agent starts, or that its terminal's width
// changed: a clear in the next armFor is a replay.
func (f *Filter) Arm() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed = f.now().Add(armFor)
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
		return f.hold(p)
	}
	data := append(f.pend, p...)
	f.pend = nil
	now := f.now()
	if i := bytes.Index(data, []byte(ClearCodes)); i >= 0 && !now.After(f.armed) {
		if _, err := f.out.Write(append(data[:i:i], ClearCodes+Notice...)); err != nil {
			return err
		}
		f.holding, f.tail, f.seq = true, nil, nil
		f.heldAt = now
		return f.hold(data[i+len(ClearCodes):])
	}
	keep := codesPrefix(data)
	if keep > 0 {
		f.pend = append([]byte(nil), data[len(data)-keep:]...)
		f.pendAt = now
	}
	_, err := f.out.Write(data[:len(data)-keep])
	return err
}

// hold takes more of a replay. What the agent asks of the terminal or sets
// in it passes at once; the rest is kept, its last tailLines lines, from
// its last clear on.
func (f *Filter) hold(p []byte) error {
	f.lastAt = f.now()
	var pass []byte
	for _, b := range p {
		if len(f.seq) == 0 && b != 0x1b {
			f.tail = append(f.tail, b)
			continue
		}
		f.seq = append(f.seq, b)
		whole, known := sequence(f.seq)
		if !whole && known && len(f.seq) < seqMax {
			continue
		}
		if whole && toTerminal(f.seq) {
			pass = append(pass, f.seq...)
		} else {
			f.tail = append(f.tail, f.seq...)
		}
		f.seq = f.seq[:0]
	}
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
	if len(pass) == 0 {
		return nil
	}
	_, err := f.out.Write(pass)
	return err
}

// sequence reads seq, which starts with ESC, as an escape sequence: whole
// when its last byte ends it, and known while it can still be one. A
// sequence that is not known is plain bytes.
func sequence(seq []byte) (whole, known bool) {
	if len(seq) < 2 {
		return false, true
	}
	last := seq[len(seq)-1]
	switch seq[1] {
	case '[': // CSI: parameters and intermediates, then one final byte
		if len(seq) == 2 {
			return false, true
		}
		if last >= 0x40 && last <= 0x7e {
			return true, true
		}
		return false, last >= 0x20 && last <= 0x3f
	case ']', 'P', '_', '^', 'X': // a string: to BEL (OSC) or to ESC \
		if len(seq) == 2 {
			return false, true
		}
		if last == 0x07 && seq[1] == ']' {
			return true, true
		}
		return last == '\\' && seq[len(seq)-2] == 0x1b && len(seq) > 3, true
	}
	return true, true // ESC and one byte
}

// toTerminal tells if a whole escape sequence asks the terminal something
// or sets a mode in it, and is no part of what the chat looks like: a
// private CSI (modes, the keyboard protocol), a status, attributes or
// window report, an OSC that asks, a DCS that asks.
func toTerminal(seq []byte) bool {
	if len(seq) < 3 {
		return false // ESC and one byte: a cursor save, a reverse index
	}
	body, final := seq[2:len(seq)-1], seq[len(seq)-1]
	switch seq[1] {
	case '[':
		if len(body) > 0 && bytes.IndexByte([]byte("?><="), body[0]) >= 0 {
			return true
		}
		return final == 'n' || final == 'c' || final == 't'
	case ']':
		return bytes.Contains(seq, []byte(";?"))
	case 'P':
		return bytes.HasPrefix(body, []byte("+q")) || bytes.HasPrefix(body, []byte("$q"))
	}
	return false
}

// Tick lets time pass. Output that waited for the rest of ClearCodes, and
// got none, passes. A replay that went quiet, or runs over holdCap, gives
// the pane its end.
func (f *Filter) Tick() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.holding {
		if now.Sub(f.lastAt) < quietFor && now.Sub(f.heldAt) < holdCap {
			return nil
		}
		return f.release()
	}
	if len(f.pend) == 0 || now.Sub(f.pendAt) < pendWait {
		return nil
	}
	pend := f.pend
	f.pend = nil
	_, err := f.out.Write(pend)
	return err
}

// release ends a hold: the pane gets the clear and the end of the replay.
func (f *Filter) release() error {
	end := append(append([]byte(ClearCodes+"\x1b[0m"), f.tail...), f.seq...)
	f.holding, f.tail, f.seq = false, nil, nil
	_, err := f.out.Write(end)
	return err
}

// Flush writes what the filter still holds, at the agent's end: the end of
// a replay, or output that waited for the rest of ClearCodes.
func (f *Filter) Flush() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holding {
		return f.release()
	}
	pend := f.pend
	f.pend = nil
	if len(pend) == 0 {
		return nil
	}
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
