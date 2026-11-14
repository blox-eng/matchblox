// Package guard stands between an agent and its terminal. Cursor's CLI
// clears the screen and prints its whole chat again each time the
// terminal's width changes (#399); the guard shows the end of that replay
// and not the replay.
package guard

import (
	"bytes"
	"io"
	"slices"
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
	// quietFor without output ends a hold: the replay is over. So does a
	// quietFor that brings under replayLines new lines: a replay prints
	// hundreds, and what follows it is the agent's own slow drawing, a
	// spinner or an answer as it comes. A rate in bytes tells nothing: a
	// renderer at work on a long chat can be slow.
	quietFor    = 300 * time.Millisecond
	replayLines = 10
	// After a change of width, Cursor first prints its whole chat with no
	// clear before it, then clears 0.25 s later and prints it again 0.5 s
	// after that (measured on 2026.10.19, #402). A hold that a width change
	// starts waits settleFor of quiet after a print, for the clear to come;
	// a hold waits up to printWait after a clear, for its print to begin.
	settleFor = 600 * time.Millisecond
	printWait = 2 * time.Second
	// quietCap bounds a hold that a width change starts. It keeps every
	// byte, as it may hold no replay; past the cap it passes them.
	quietCap = 16 << 20
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
	mu       sync.Mutex
	out      io.Writer
	now      func() time.Time
	pend     []byte    // output held back: it could be the start of ClearCodes
	pendAt   time.Time // when pend last grew
	armed    time.Time // until when a clear starts a hold
	holding  bool      // a replay: the pane shows the notice
	quiet    bool      // a hold that a width change starts, with no clear yet: the pane shows nothing new
	clearAt  time.Time // when the replay's last clear came
	tail     []byte    // the end of the replay so far
	seq      []byte    // an escape sequence of the replay, not yet whole
	heldAt   time.Time // when the hold began
	lastAt   time.Time // when the replay last printed
	winAt    time.Time // when the window that counts winLines began
	winLines int       // lines of the replay in that window
	winText  int       // bytes of text, not of escape sequences, in that window
	printed  int       // lines since the replay's last clear
	text     int       // bytes of text, not of escape sequences, since then
	modes    []mode    // the modes set in a quiet hold, where they are in its tail
}

// mode is a mode the agent set in a quiet hold, and where in the hold's
// tail it is: a replay drops what came before its clear, but not the modes.
type mode struct {
	at  int
	seq []byte
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

// Resized tells the filter that the terminal's width changed. It arms, and
// holds what the agent prints next: Cursor prints its whole chat once
// before its clear. A clear makes the hold a replay; with none, what was
// held passes as it came.
func (f *Filter) Resized() {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	f.armed = now.Add(armFor)
	if f.holding || f.quiet {
		return
	}
	f.quiet, f.tail, f.seq, f.modes, f.winLines, f.winText, f.text = true, nil, nil, nil, 0, 0, 0
	f.heldAt, f.winAt, f.lastAt = now, now, now
	pend := f.pend
	f.pend = nil
	_ = f.hold(pend) // the start of a clear, if any: it comes whole with the next write
}

// Holding tells if the filter holds the agent's output now.
func (f *Filter) Holding() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holding || f.quiet
}

// Write takes the agent's next output.
func (f *Filter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(p), f.write(p)
}

func (f *Filter) write(p []byte) error {
	if f.holding || f.quiet {
		return f.hold(p)
	}
	data := append(f.pend, p...)
	f.pend = nil
	now := f.now()
	if i := bytes.Index(data, []byte(ClearCodes)); i >= 0 && !now.After(f.armed) {
		if _, err := f.out.Write(append(data[:i:i], ClearCodes+Notice...)); err != nil {
			return err
		}
		f.holding, f.tail, f.seq, f.winLines, f.winText, f.printed, f.text = true, nil, nil, 0, 0, 0, 0
		f.heldAt, f.winAt, f.clearAt = now, now, now
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
	for i, b := range p {
		if len(f.seq) == 0 && b != 0x1b {
			if b == '\n' {
				f.winLines++
				f.printed++
			}
			f.winText++
			f.text++
			f.tail = append(f.tail, b)
			continue
		}
		f.seq = append(f.seq, b)
		whole, known := sequence(f.seq)
		if !whole && known && len(f.seq) < seqMax {
			continue
		}
		if whole && otherScreen(f.seq) {
			// The chat belongs on the screen the agent now leaves: the pane
			// gets its end first, then the switch and what follows.
			rest := append(slices.Clone(f.seq), p[i+1:]...)
			f.seq = nil
			if _, err := f.out.Write(pass); err != nil {
				return err
			}
			if err := f.release(); err != nil {
				return err
			}
			return f.write(rest)
		}
		switch {
		case whole && toTerminal(f.seq) && f.quiet && !query(f.seq):
			// In its place: the hold may hold no replay, and then it comes
			// out as it went in.
			f.modes = append(f.modes, mode{len(f.tail), slices.Clone(f.seq)})
			f.tail = append(f.tail, f.seq...)
		case whole && toTerminal(f.seq):
			pass = append(pass, f.seq...)
		default:
			f.tail = append(f.tail, f.seq...)
		}
		f.seq = f.seq[:0]
	}
	if i := bytes.LastIndex(f.tail, []byte(ClearCodes)); i >= 0 {
		from := i + len(ClearCodes)
		kept := f.tail[from:]
		if f.quiet {
			// The hold a width change started is a replay. What came before
			// the clear, the clear erases, but not the modes set in it; those
			// set after it pass at once, as in any replay.
			head := []byte(ClearCodes + Notice)
			kept = nil
			at := from
			for _, m := range f.modes {
				if m.at >= from {
					kept = append(kept, f.tail[at:m.at]...)
					at = m.at + len(m.seq)
				}
				head = append(head, m.seq...)
			}
			kept = append(kept, f.tail[at:]...)
			f.quiet, f.holding, f.modes = false, true, nil
			pass = append(head, pass...)
		}
		f.tail = kept
		f.clearAt, f.printed, f.text = f.now(), bytes.Count(f.tail, []byte("\n")), plainLen(f.tail)
	}
	if f.quiet {
		if len(f.tail) > quietCap {
			if _, err := f.out.Write(pass); err != nil {
				return err
			}
			return f.release()
		}
		if len(pass) == 0 {
			return nil
		}
		_, err := f.out.Write(pass)
		return err
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
	case ']': // the question mark is the last parameter: a link's address may hold one
		return bytes.HasSuffix(seq, []byte(";?\x07")) || bytes.HasSuffix(seq, []byte(";?\x1b\\"))
	case 'P':
		return bytes.HasPrefix(body, []byte("+q")) || bytes.HasPrefix(body, []byte("$q"))
	}
	return false
}

// plainLen is how many bytes of b are text, outside escape sequences.
func plainLen(b []byte) int {
	n := 0
	for i := 0; i < len(b); {
		if b[i] != 0x1b {
			n++
			i++
			continue
		}
		j := i + 2
		for ; j <= len(b); j++ {
			if whole, known := sequence(b[i:j]); whole || !known {
				break
			}
		}
		i = j
	}
	return n
}

// query tells if a sequence that toTerminal passes asks the terminal for
// an answer: the agent waits for it, so it passes at once.
func query(seq []byte) bool {
	final := seq[len(seq)-1]
	return seq[1] == ']' || seq[1] == 'P' || (seq[1] == '[' && (final == 'n' || final == 'c' || final == 't'))
}

// otherScreen tells if a whole escape sequence switches to or from the
// alternate screen.
func otherScreen(seq []byte) bool {
	for _, mode := range []string{"\x1b[?1049", "\x1b[?1047", "\x1b[?47"} {
		if string(seq) == mode+"h" || string(seq) == mode+"l" {
			return true
		}
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
	if f.holding || f.quiet {
		slow := false
		if now.Sub(f.winAt) >= quietFor {
			slow = f.winText > 0 && f.winLines < replayLines // a stream, not a print
			f.winAt, f.winLines, f.winText = now, 0, 0
		}
		// After a clear, until its print has begun: words that come first
		// are no end of the replay.
		before := f.holding && f.printed < replayLines && now.Sub(f.clearAt) < printWait
		wait := quietFor
		if f.quiet {
			wait = settleFor
		}
		switch {
		case now.Sub(f.heldAt) >= holdCap:
		case before && f.text == 0 && len(f.seq) == 0: // no text yet: codes are no print
			return nil
		case before && now.Sub(f.lastAt) < quietFor:
			return nil
		case !before && !slow && now.Sub(f.lastAt) < wait:
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

// release ends a hold: the pane gets the clear and the end of the replay,
// or, from a hold with no clear in it, what was held, as it came.
func (f *Filter) release() error {
	end := append(slices.Clone(f.tail), f.seq...)
	if f.holding {
		end = append([]byte(ClearCodes+"\x1b[0m"), end...)
	}
	f.holding, f.quiet, f.tail, f.seq, f.modes = false, false, nil, nil, nil
	_, err := f.out.Write(end)
	return err
}

// Flush writes what the filter still holds, at the agent's end: the end of
// a replay, or output that waited for the rest of ClearCodes.
func (f *Filter) Flush() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holding || f.quiet {
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
