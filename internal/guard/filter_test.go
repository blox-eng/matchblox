package guard

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

// writeIn writes p to f in chunks of n bytes.
func writeIn(t *testing.T, f *Filter, p []byte, n int) {
	t.Helper()
	for len(p) > 0 {
		k := min(n, len(p))
		if _, err := f.Write(p[:k]); err != nil {
			t.Fatal(err)
		}
		p = p[k:]
	}
}

// TestFilterHalfCodes: a part of the clear codes is an agent's own output
// and passes as it is; the whole codes start a hold however they are cut.
func TestFilterHalfCodes(t *testing.T) {
	var out bytes.Buffer
	f := NewFilter(&out, newClock().now)
	half := []byte("\x1b[H\x1b[2Jtext after a plain clear\r\n")
	writeIn(t, f, half, 1)
	if !bytes.Equal(out.Bytes(), half) {
		t.Fatalf("a clear with no scrollback clear came out as %q, want %q", out.Bytes(), half)
	}
	if f.Holding() {
		t.Fatal("a clear with no scrollback clear started a hold")
	}
	out.Reset()
	c := newClock()
	f = NewFilter(&out, c.now)
	writeIn(t, f, []byte("ab\x1b[H"), 4)
	if got := out.String(); got != "ab" {
		t.Fatalf("before the wait, %q came out, want %q: the rest could start the codes", got, "ab")
	}
	c.t = c.t.Add(time.Second)
	f.Tick()
	if got := out.String(); got != "ab\x1b[H" {
		t.Fatalf("after the wait, %q came out, want the cursor-home too", got)
	}
	for _, cut := range [][]string{{"\x1b[H\x1b[2J\x1b[3J"}, {"\x1b", "[H\x1b[2", "J\x1b[3J"}, {"\x1b[H", "\x1b[2J", "\x1b[3J"}} {
		out.Reset()
		f := NewFilter(&out, newClock().now)
		f.Arm()
		for _, part := range cut {
			if _, err := f.Write([]byte(part)); err != nil {
				t.Fatal(err)
			}
		}
		if !f.Holding() {
			t.Fatalf("the clear codes written as %q started no hold", cut)
		}
	}
}

// TestFilterPassesUnchanged: with no clear codes in it, every byte comes
// out as it went in, however the writes are cut.
func TestFilterPassesUnchanged(t *testing.T) {
	in := make([]byte, 1<<20)
	rng := rand.New(rand.NewSource(1))
	rng.Read(in)
	in = bytes.ReplaceAll(in, []byte(ClearCodes), []byte("........."))
	for _, n := range []int{1, 7, 4096} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		writeIn(t, f, in, n)
		c.t = c.t.Add(time.Second)
		f.Tick() // a last byte that could start the codes waits for the next one, not for ever
		if !bytes.Equal(out.Bytes(), in) {
			t.Fatalf("in chunks of %d: %d bytes came out, %d went in, or they differ", n, out.Len(), len(in))
		}
		if f.Holding() {
			t.Fatalf("in chunks of %d: a hold with no clear codes", n)
		}
	}
}

// replay is what Cursor prints after a width change: the clear codes,
// every line of the chat, then its input box.
func replay(lines int) []byte {
	var b bytes.Buffer
	b.WriteString(ClearCodes)
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}
	b.WriteString(box)
	return b.Bytes()
}

const box = "\x1b[2m╭──────╮\x1b[0m\r\n│ → ask │\r\n╰──────╯"

// TestFilterKeepsTail: of a replay, the pane gets one line while it runs,
// then the end of the chat and the input box, and never its start.
func TestFilterKeepsTail(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	in := replay(50000)
	writeIn(t, f, in[:len(ClearCodes)], 3)
	if got, want := out.String(), ClearCodes+Notice; got != want {
		t.Fatalf("at the start of a replay the pane got %q, want %q", got, want)
	}
	writeIn(t, f, in[len(ClearCodes):], 4096)
	if !f.Holding() || out.Len() != len(ClearCodes+Notice) {
		t.Fatalf("during the replay: holding=%v, %d bytes out; want a hold and no more than the notice", f.Holding(), out.Len())
	}
	out.Reset()
	c.t = c.t.Add(quietFor)
	if err := f.Tick(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if f.Holding() {
		t.Fatal("the replay ended and the hold did not")
	}
	if !strings.HasPrefix(got, ClearCodes+"\x1b[0m") {
		t.Fatalf("the end of the replay starts with %q, want the clear codes and a style reset", got[:min(len(got), 20)])
	}
	if !strings.HasSuffix(got, "line 50000\r\n"+box) {
		t.Fatalf("the end of the replay ends with %q, want the last line and the input box", got[max(len(got)-60, 0):])
	}
	if n := strings.Count(got, "\n"); n != tailLines {
		t.Fatalf("the pane got %d lines of the chat, want %d", n, tailLines)
	}
	for _, old := range []string{"line 1\r\n", "line 40000\r\n"} {
		if strings.Contains(got, old) {
			t.Fatalf("the pane got %q: the start of the chat scrolled by", old)
		}
	}
	out.Reset()
	writeIn(t, f, []byte("typed"), 5)
	if out.String() != "typed" {
		t.Fatalf("after the replay %q came out, want what went in", out.String())
	}
}

// TestFilterOwnClear: Cursor also clears for its own reasons (a /clear, a
// new chat). No width changed, so nothing is held: every byte passes as it
// comes, and no notice shows.
func TestFilterOwnClear(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	c.t = c.t.Add(armFor + time.Millisecond) // the start is long past
	in := replay(40)
	writeIn(t, f, in, 7)
	if f.Holding() || !bytes.Equal(out.Bytes(), in) {
		t.Fatalf("a clear with no width change before it: holding=%v, %d bytes out of %d; want all of them, unchanged", f.Holding(), out.Len(), len(in))
	}
}

// TestFilterSlowReplay: a replay is held however slowly Cursor prints it,
// as long as it keeps printing. 60 kB/s for 20 s is a renderer at work on a
// long chat.
func TestFilterSlowReplay(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, []byte(ClearCodes), 9)
	line := 0
	for at := time.Duration(0); at < 20*time.Second; at += 25 * time.Millisecond {
		var b bytes.Buffer
		for b.Len() < 1500 {
			line++
			fmt.Fprintf(&b, "line %d| %s\r\n", line, strings.Repeat("chat ", 12))
		}
		writeIn(t, f, b.Bytes(), b.Len())
		c.t = c.t.Add(25 * time.Millisecond)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
		if !f.Holding() {
			t.Fatalf("the hold ended %v into a replay that still prints", at)
		}
	}
	c.t = c.t.Add(quietFor)
	if err := f.Tick(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if f.Holding() || strings.Contains(got, "line 1|") || !strings.Contains(got, fmt.Sprintf("line %d|", line)) {
		t.Fatalf("after a slow replay of %d lines: holding=%v, first line shown=%v, last line shown=%v", line, f.Holding(), strings.Contains(got, "line 1|"), strings.Contains(got, fmt.Sprintf("line %d|", line)))
	}
}

// TestFilterHoldPassesModesAndQueries: what Cursor asks of the terminal or
// sets in it during a replay is not text of the chat. It passes at once, in
// order and once: a query held back gets its answer late, and a mode set
// that is cut off with the start of the chat is never set.
func TestFilterHoldPassesModesAndQueries(t *testing.T) {
	asked := []string{
		"\x1b[?2004h",       // bracketed paste on
		"\x1b[>1u",          // the kitty keyboard protocol, pushed
		"\x1b[?1004h",       // focus events on
		"\x1b]11;?\x07",     // the background colour, asked
		"\x1b[6n",           // the cursor's position, asked
		"\x1b[c",            // the terminal's attributes, asked
		"\x1bP+q544e\x1b\\", // a terminfo name, asked
	}
	for _, chunk := range []int{1, 5, 4096} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		f.Arm()
		in := []byte(ClearCodes + "\x1b[1mbold\x1b[0m " + strings.Join(asked, "text between ") + "\r\n")
		for i := 1; i <= 1500; i++ {
			in = fmt.Appendf(in, "\x1b[32mline %d|\x1b[0m\r\n", i)
		}
		writeIn(t, f, in, chunk)
		if got, want := out.String(), ClearCodes+Notice+strings.Join(asked, ""); got != want {
			t.Fatalf("in chunks of %d, during the replay the pane got %q, want the notice and then each query and mode set at once: %q", chunk, got, want)
		}
		c.t = c.t.Add(quietFor)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		for _, seq := range asked {
			if n := strings.Count(got, seq); n != 1 {
				t.Errorf("in chunks of %d, %q reached the terminal %d times, want once", chunk, seq, n)
			}
		}
		if !strings.HasSuffix(got, "\x1b[32mline 1500|\x1b[0m\r\n") || strings.Contains(got, "line 1|") {
			t.Errorf("in chunks of %d, the end of the replay is wrong: ends %q", chunk, got[max(len(got)-40, 0):])
		}
	}
}

// TestFilterCap: a replay that never slows does not hold the pane for ever.
func TestFilterCap(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, []byte(ClearCodes), 9)
	chunk := bytes.Repeat([]byte("0123456789abcde\n"), 4096) // 64 KiB each 100 ms
	for at := time.Duration(0); at < holdCap; at += 100 * time.Millisecond {
		if !f.Holding() {
			t.Fatalf("the hold ended at %v, with a replay in full flow", at)
		}
		writeIn(t, f, chunk, len(chunk))
		c.t = c.t.Add(100 * time.Millisecond)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if f.Holding() {
		t.Fatalf("after %v the hold goes on", holdCap)
	}
	if n := strings.Count(out.String(), "\n"); n != tailLines {
		t.Fatalf("at the cap the pane got %d lines, want the last %d", n, tailLines)
	}
	before := out.Len()
	writeIn(t, f, chunk, len(chunk))
	if out.Len() != before+len(chunk) {
		t.Fatal("after the cap, the rest of the replay does not pass as it comes")
	}
}

// TestFilterShortReplay: a replay shorter than the tail comes out whole,
// its first blank line too.
func TestFilterShortReplay(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, []byte(ClearCodes+"\nfirst\r\nlast"), 64)
	c.t = c.t.Add(quietFor)
	if err := f.Tick(); err != nil {
		t.Fatal(err)
	}
	if want := ClearCodes + Notice + ClearCodes + "\x1b[0m\nfirst\r\nlast"; out.String() != want {
		t.Fatalf("a short replay came out as %q, want %q", out.String(), want)
	}
}

// TestFilterHoldShortEscapes: a two-byte escape in a replay (save and
// restore the cursor, reverse index, a lone ESC at its end) is part of the
// chat's drawing. It is kept in place, and the guard does not break on it.
func TestFilterHoldShortEscapes(t *testing.T) {
	for _, chunk := range []int{1, 64} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		f.Arm()
		const body = "\x1b7saved\x1b8 \x1bMup\r\nlast\x1b"
		writeIn(t, f, []byte(ClearCodes+body), chunk)
		c.t = c.t.Add(quietFor)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
		if want := ClearCodes + Notice + ClearCodes + "\x1b[0m" + body; out.String() != want {
			t.Fatalf("in chunks of %d the pane got %q, want %q", chunk, out.String(), want)
		}
	}
}

// TestFilterHoldLosesNoByte: whatever a replay holds, escape sequences cut
// and malformed in every way, the guard does not break, and each byte
// reaches the pane once: at once when the terminal must have it, else with
// the end of the replay.
func TestFilterHoldLosesNoByte(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	alphabet := []byte("\x1b\x1b\x1b[]P_^X?><=;0123456789 $+qhlunctm\\\x07ab\r")
	for round := range 2000 {
		in := make([]byte, 1+rng.Intn(300))
		for i := range in {
			in[i] = alphabet[rng.Intn(len(alphabet))]
		}
		if bytes.Contains(in, []byte(ClearCodes)) {
			continue
		}
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		f.Arm()
		writeIn(t, f, []byte(ClearCodes), 9)
		writeIn(t, f, in, 1+rng.Intn(16))
		c.t = c.t.Add(quietFor)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
		got := bytes.Replace(out.Bytes(), []byte(ClearCodes+Notice), nil, 1)
		got = bytes.Replace(got, []byte(ClearCodes+"\x1b[0m"), nil, 1)
		a, b := slices.Clone(in), slices.Clone(got)
		slices.Sort(a)
		slices.Sort(b)
		if !bytes.Equal(a, b) {
			t.Fatalf("round %d: %q went in, %q came out: a byte was lost or made", round, in, got)
		}
	}
}

// TestFilterHoldEndsUnderSlowStream: Cursor may stream an answer when the
// width changes. Its replay is a burst of hundreds of lines; the stream
// after it is a spinner and a few words a second. The hold ends with the
// burst, not with the stream.
func TestFilterHoldEndsUnderSlowStream(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, replay(5000), 4096)
	for at := time.Duration(0); at < 5*time.Second; at += 100 * time.Millisecond {
		c.t = c.t.Add(100 * time.Millisecond)
		writeIn(t, f, []byte("\r\x1b[2K⠋ Generating…"), 64)
		if at%time.Second == 0 {
			writeIn(t, f, []byte(" a few words of the answer\r\n"), 64)
		}
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
		if !f.Holding() {
			if at > time.Second {
				t.Fatalf("the hold ended %v after the replay's burst, want within 1 s", at)
			}
			return
		}
	}
	t.Fatal("5 s after the replay's burst, under a slow stream, the pane still shows only the notice")
}

// TestFilterAltScreenEndsHold: when the agent opens a full-screen view
// during a replay, the chat belongs on the screen below it. The pane gets
// the end of the chat first, then the switch.
func TestFilterAltScreenEndsHold(t *testing.T) {
	for _, mode := range []string{"\x1b[?1049h", "\x1b[?1047h", "\x1b[?47h"} {
		var out bytes.Buffer
		f := NewFilter(&out, newClock().now)
		f.Arm()
		writeIn(t, f, []byte(ClearCodes+"chat line\r\n"+mode+"full screen"), 3)
		if want := ClearCodes + Notice + ClearCodes + "\x1b[0mchat line\r\n" + mode + "full screen"; out.String() != want || f.Holding() {
			t.Fatalf("with %q the pane got %q (holding %v), want the chat, then the switch, then its view: %q", mode, out.String(), f.Holding(), want)
		}
	}
}

// TestFilterHyperlinkStaysInPlace: a link whose address holds ";?" is text
// of the chat, not a question to the terminal: it stays where it is.
func TestFilterHyperlinkStaysInPlace(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	const body = "before\r\n\x1b]8;;https://x.dev/a;?b=1\x1b\\link\x1b]8;;\x1b\\"
	writeIn(t, f, []byte(ClearCodes+body), 5)
	c.t = c.t.Add(quietFor)
	if err := f.Tick(); err != nil {
		t.Fatal(err)
	}
	if want := ClearCodes + Notice + ClearCodes + "\x1b[0m" + body; out.String() != want {
		t.Fatalf("the pane got %q, want the link in its place: %q", out.String(), want)
	}
}
