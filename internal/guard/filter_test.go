package guard

import (
	"bytes"
	"fmt"
	"io"
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
		c.t = c.t.Add(printWait) // the longest a hold waits for a print to begin
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

// burst is n lines of a chat, as Cursor prints them, each named by word.
func burst(word string, n int) []byte {
	var b bytes.Buffer
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s %d| %s\r\n", word, i, strings.Repeat("chat ", 12))
	}
	return b.Bytes()
}

// tickFor lets d pass in steps of the guard's own tick.
func tickFor(t *testing.T, f *Filter, c *fakeClock, d time.Duration) {
	t.Helper()
	for end := c.t.Add(d); c.t.Before(end); {
		c.t = c.t.Add(25 * time.Millisecond)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestFilterNarrowerAsCursorDoes: what Cursor 2026.10.19 printed when its
// pane went from 216 to 180 columns, measured on a real chat (#402): the
// whole chat with no clear before it, 0.25 s later the clear alone, 0.5 s
// after it the whole chat again. The pane gets the notice and the end of the
// second print only.
func TestFilterNarrowerAsCursorDoes(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, burst("first", 1620), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 500*time.Millisecond)
	writeIn(t, f, append(burst("second", 1626), "→ Add a follow-up"...), 8190)
	tickFor(t, f, c, time.Second)
	got := out.String()
	if f.Holding() {
		t.Fatal("1 s after the second print, the hold goes on")
	}
	if strings.Contains(got, "first ") {
		t.Fatalf("the pane got the first print, which has no clear before it: %d lines of it", strings.Count(got, "first "))
	}
	if strings.Count(got, Notice) != 1 || strings.Contains(got, "second 1|") || !strings.HasSuffix(got, "second 1626| "+strings.Repeat("chat ", 12)+"\r\n→ Add a follow-up") {
		t.Fatalf("the pane got %d notices, the second print's start: %v, its end: %q; want one notice and only the end", strings.Count(got, Notice), strings.Contains(got, "second 1|"), got[max(len(got)-60, 0):])
	}
	if n := strings.Count(got, "\n"); n != tailLines {
		t.Fatalf("the pane got %d lines, want the last %d", n, tailLines)
	}
}

// TestFilterWiderAsCursorDoes: what Cursor printed when its pane went from
// 180 to 216 columns (#402): the clear, 0.26 s of nothing, then the whole
// chat. The pause is no end of the replay.
func TestFilterWiderAsCursorDoes(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 360*time.Millisecond) // past quietFor, and still no print
	writeIn(t, f, burst("chat", 1621), 8190)
	tickFor(t, f, c, time.Second)
	got := out.String()
	if strings.Contains(got, "chat 1|") || !strings.Contains(got, "chat 1621|") || strings.Count(got, "\n") != tailLines {
		t.Fatalf("after the pause the pane got the chat's start: %v, its end: %v, %d lines; want only its last %d", strings.Contains(got, "chat 1|"), strings.Contains(got, "chat 1621|"), strings.Count(got, "\n"), tailLines)
	}
}

// TestFilterSmallRedrawAfterResize: a width change after which the agent
// redraws only a few lines passes those bytes unchanged, within one window.
func TestFilterSmallRedrawAfterResize(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	in := []byte("\x1b[2A\x1b[2K→ ask\r\n\x1b[2KAuto · 79%")
	writeIn(t, f, in, 7)
	tickFor(t, f, c, quietFor+50*time.Millisecond)
	if !bytes.Equal(out.Bytes(), in) || f.Holding() {
		t.Fatalf("a small redraw after a resize came out as %q (holding %v), want %q", out.Bytes(), f.Holding(), in)
	}
}

// TestFilterTokensBeforeReplayPrint: a few bytes between Cursor's clear and
// its print, such as streamed words, do not end the hold before the print.
func TestFilterTokensBeforeReplayPrint(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, burst("first", 1620), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	for range 20 {
		writeIn(t, f, []byte("tok "), 4)
		tickFor(t, f, c, 25*time.Millisecond)
	}
	writeIn(t, f, burst("second", 1620), 8190)
	tickFor(t, f, c, time.Second)
	got := out.String()
	if strings.Contains(got, "second 1|") || strings.Count(got, "\n") != tailLines {
		t.Fatalf("with words between the clear and the print, the pane got the print's start: %v, %d lines; want only its last %d", strings.Contains(got, "second 1|"), strings.Count(got, "\n"), tailLines)
	}
}

// TestFilterQuietHoldKeepsOrder: a hold that a width change starts, and that
// turns out to hold no replay, gives the pane its bytes in their order: a
// cursor shown or hidden, or a switch of screen, comes where the agent put it.
func TestFilterQuietHoldKeepsOrder(t *testing.T) {
	for _, in := range []string{
		"abc\x1b[?25ldef\x1b[?25hghi",
		"abc\x1b[?25ldef\x1b[?1049hpicker",
		"abc\x1b[>1udef\x1b[<ughi\x1b[=1;1ujkl\x1b[>4;1mmno", // the kitty keyboard protocol, modifyOtherKeys
	} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		f.Resized()
		writeIn(t, f, []byte(in), 3)
		tickFor(t, f, c, time.Second)
		if out.String() != in {
			t.Fatalf("a quiet hold of %q gave the pane %q", in, out.String())
		}
	}
}

// TestFilterQuietHoldModesReachAReplay: a mode set in the print before the
// clear still reaches the terminal, though that print never shows.
func TestFilterQuietHoldModesReachAReplay(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, append([]byte("\x1b[?2004h"), burst("first", 1200)...), 4096)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, append([]byte(ClearCodes), burst("second", 1200)...), 4096)
	tickFor(t, f, c, time.Second)
	got := out.String()
	if strings.Count(got, "\x1b[?2004h") != 1 || strings.Contains(got, "first ") {
		t.Fatalf("the mode set before the clear reached the pane %d times, the first print shows: %v; want once and no", strings.Count(got, "\x1b[?2004h"), strings.Contains(got, "first "))
	}
}

// TestFilterSettlesForCursorsPause: Cursor's clear can come more than
// quietFor after its first print; the hold a width change starts waits
// settleFor, so that print still never shows.
func TestFilterSettlesForCursorsPause(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, burst("first", 1620), 8190)
	tickFor(t, f, c, 400*time.Millisecond)
	writeIn(t, f, append([]byte(ClearCodes), burst("second", 1620)...), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "first ") {
		t.Fatalf("with 400 ms between the print and the clear, %d lines of the first print reached the pane", strings.Count(got, "first "))
	}
}

// TestFilterQuietHoldKeepsEveryByte: a width change after which the agent
// prints a long text with no clear, so no replay, gives the pane every byte
// of it: nothing is cut from the scrollback.
func TestFilterQuietHoldKeepsEveryByte(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	in := burst("out", 1500)
	writeIn(t, f, in, 4096)
	tickFor(t, f, c, time.Second)
	if !bytes.Equal(out.Bytes(), in) {
		t.Fatalf("1,500 lines with no clear after a resize gave the pane %d lines, the first: %v; want all of them, as they came", strings.Count(out.String(), "\n"), strings.Contains(out.String(), "out 1|"))
	}
}

// TestFilterCodesAfterClearAreNoPrint: what Cursor sends right after its
// clear, a mode set or a style, is not its print beginning: the pause after
// it still holds (measured on a wider pane, #402).
func TestFilterCodesAfterClearAreNoPrint(t *testing.T) {
	for _, after := range []string{"\x1b[?25l", "\x1b[0m", "\x1b[?2026h\x1b[1;1H"} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		f.Resized()
		writeIn(t, f, []byte(ClearCodes+after), 64)
		tickFor(t, f, c, 360*time.Millisecond)
		writeIn(t, f, burst("chat", 1621), 8190)
		tickFor(t, f, c, time.Second)
		if got := out.String(); strings.Contains(got, "chat 1|") || strings.Count(got, "\n") != tailLines {
			t.Fatalf("with %q after the clear, the pane got the chat's start: %v, %d lines; want only the last %d", after, strings.Contains(got, "chat 1|"), strings.Count(got, "\n"), tailLines)
		}
	}
}

// TestFilterQuietHoldOnePass: a long chat after a resize costs the guard one
// pass over its bytes, not a pass per write: the pane does not freeze. Six
// times the bytes take about six times as long, where a pass per write takes
// thirty-six; a slow machine, or the race detector, changes neither ratio.
func TestFilterQuietHoldOnePass(t *testing.T) {
	line := []byte("line | chat chat chat chat chat chat chat chat chat chat chat chat\r\n")
	took := func(mib int) time.Duration {
		best := time.Duration(1 << 62)
		for range 2 { // the faster of two runs: a pause of the machine is no cost of the guard
			f := NewFilter(io.Discard, newClock().now)
			f.Resized()
			in := bytes.Repeat(line, (mib<<20)/len(line))
			began := time.Now()
			writeIn(t, f, in, 4096)
			best = min(best, time.Since(began))
		}
		return best
	}
	small, big := took(4), took(24)
	if ratio := float64(big) / float64(small); ratio > 18 {
		t.Fatalf("4 MiB in a quiet hold took %v, 24 MiB %v: %.0f times as long for six times the bytes; each write scans all the hold holds", small, big, ratio)
	}
}

// TestFilterQuietPassesQueries: what asks the terminal for an answer passes
// at once in a quiet hold too, also the questions that look like modes.
func TestFilterQuietPassesQueries(t *testing.T) {
	for _, q := range []string{"\x1b[?u", "\x1b[?2026$p", "\x1b[>q", "\x1b[>0q", "\x1b[6n", "\x1b]11;?\x07", "\x1b[?4m"} {
		var out bytes.Buffer
		f := NewFilter(&out, newClock().now)
		f.Resized()
		writeIn(t, f, []byte("chat\r\n"+q+"more"), 64)
		if out.String() != q {
			t.Errorf("in a quiet hold, the query %q gave the pane %q at once, want the query alone", q, out.String())
		}
	}
}

// TestFilterShortChatSpinnerEnds: a short chat, then a spinner: the hold
// ends soon after the chat's lines, not after printWait.
func TestFilterShortChatSpinnerEnds(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 260*time.Millisecond)
	writeIn(t, f, []byte("hi\r\nthere\r\n"), 64)
	for at := time.Duration(0); at < 2*time.Second; at += 100 * time.Millisecond {
		writeIn(t, f, []byte("\r\x1b[2K⠋"), 64)
		tickFor(t, f, c, 100*time.Millisecond)
		if !f.Holding() {
			if at > 600*time.Millisecond {
				t.Fatalf("the hold ended %v after a short chat's lines, want within 600 ms", at)
			}
			return
		}
	}
	t.Fatal("2 s after a short chat's lines, under a spinner, the pane still shows only the notice")
}

// TestFilterResizeDuringReplayHoldsNextPrint: a second width change while
// the first one's replay is held (a pane edge dragged on) holds the print
// Cursor makes for it, though no clear comes before that print.
func TestFilterResizeDuringReplayHoldsNextPrint(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, burst("a1", 1600), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 500*time.Millisecond)
	writeIn(t, f, burst("a2", 1600), 8190)
	f.Resized()
	tickFor(t, f, c, 400*time.Millisecond)
	writeIn(t, f, burst("b1", 1600), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 500*time.Millisecond)
	writeIn(t, f, burst("b2", 1600), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "b1 ") || !strings.Contains(got, "b2 1600|") {
		t.Fatalf("after a resize during a replay the pane got the next print with no clear: %d lines of it; the last print's end shown: %v", strings.Count(got, "b1 "), strings.Contains(got, "b2 1600|"))
	}
}

// TestFilterResizeDuringQuietHoldRestartsIt: two width changes 550 ms apart
// with Cursor's print for each: the second one's print still never shows.
func TestFilterResizeDuringQuietHoldRestartsIt(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	writeIn(t, f, burst("a", 50), 8190)
	tickFor(t, f, c, 550*time.Millisecond)
	f.Resized()
	writeIn(t, f, burst("first", 1600), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 500*time.Millisecond)
	writeIn(t, f, burst("second", 1600), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "first ") {
		t.Fatalf("%d lines of the second width change's first print reached the pane", strings.Count(got, "first "))
	}
}

// TestFilterQuietHoldFastStreamEnds: a resize while the agent streams fast
// output with no clear, such as a long log: the pane is not frozen; within
// printWait it gets every byte, in order.
func TestFilterQuietHoldFastStreamEnds(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	var in []byte
	for i := range 40 {
		b := burst(fmt.Sprintf("log%d", i), 20)
		in = append(in, b...)
		writeIn(t, f, b, 4096)
		tickFor(t, f, c, 100*time.Millisecond)
	}
	if f.Holding() || !bytes.Equal(out.Bytes(), in) {
		t.Fatalf("4 s of a fast stream after a resize with no clear: holding %v, the pane got %d of %d bytes", f.Holding(), out.Len(), len(in))
	}
}

// TestFilterQuietHoldFooterInNewWindow: the last few lines of Cursor's first
// print (its input box) can land in a window of their own; that is no
// trickle, and the print still never shows.
func TestFilterQuietHoldFooterInNewWindow(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	tickFor(t, f, c, 25*time.Millisecond)
	writeIn(t, f, burst("first", 1600), 8190)
	tickFor(t, f, c, 350*time.Millisecond)
	writeIn(t, f, []byte("╭── ask ──╮\r\n│ → \r\n╰─────────╯\r\n"), 64)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, append([]byte(ClearCodes), burst("second", 1600)...), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "first ") {
		t.Fatalf("with the footer in a window of its own, %d lines of the first print reached the pane", strings.Count(got, "first "))
	}
}

// TestFilterQuietCapPassesAll: past quietCap with no clear, a quiet hold
// gives the pane every byte as it came, with no Tick.
func TestFilterQuietCapPassesAll(t *testing.T) {
	var out bytes.Buffer
	f := NewFilter(&out, newClock().now)
	f.Resized()
	in := burst("out", quietCap/70+1000)
	writeIn(t, f, in, 65536)
	if !bytes.Equal(out.Bytes(), in) || f.Holding() {
		t.Fatalf("past quietCap: the pane got %d of %d bytes, holding %v", out.Len(), len(in), f.Holding())
	}
}

// TestFilterQuietHoldModeAfterClear: modes set before and after the clear,
// in the one write that turns a quiet hold into a replay, each reach the
// terminal once; the print before the clear never shows.
func TestFilterQuietHoldModeAfterClear(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	in := append([]byte("\x1b[?2004h"), burst("first", 50)...)
	in = append(append(in, ClearCodes...), "\x1b[?1004h"...)
	in = append(in, burst("second", 1200)...)
	writeIn(t, f, in, len(in))
	tickFor(t, f, c, 3*time.Second)
	got := out.String()
	if strings.Count(got, "\x1b[?2004h") != 1 || strings.Count(got, "\x1b[?1004h") != 1 || strings.Contains(got, "first ") {
		t.Fatalf("modes before and after the clear reached the pane %d and %d times, the first print shows: %v", strings.Count(got, "\x1b[?2004h"), strings.Count(got, "\x1b[?1004h"), strings.Contains(got, "first "))
	}
}

// TestFilterCells: each state of the filter at each event that the other
// tests do not reach.
func TestFilterCells(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(f *Filter, c *fakeClock)
		want string
		hold bool
	}{
		{"quiet, then the agent ends", func(f *Filter, c *fakeClock) {
			f.Resized()
			writeIn(t, f, []byte("abc\x1b[?25lx"), 3)
			_ = f.Flush()
		}, "abc\x1b[?25lx", false},
		{"replay, then the agent ends", func(f *Filter, c *fakeClock) {
			f.Arm()
			writeIn(t, f, []byte(ClearCodes+"a\r\nb"), 64)
			_ = f.Flush()
		}, ClearCodes + Notice + ClearCodes + "\x1b[0ma\r\nb", false},
		{"passing, then the agent ends on a start of the clear", func(f *Filter, c *fakeClock) {
			writeIn(t, f, []byte("ab\x1b[H"), 64)
			_ = f.Flush()
		}, "ab\x1b[H", false},
		{"passing, a switch of screen", func(f *Filter, c *fakeClock) {
			f.Arm()
			writeIn(t, f, []byte("x\x1b[?1049hview"), 64)
		}, "x\x1b[?1049hview", false},
		{"quiet, armed again", func(f *Filter, c *fakeClock) {
			f.Resized()
			writeIn(t, f, []byte("text"), 64)
			f.Arm()
		}, "", true},
		{"replay, armed again", func(f *Filter, c *fakeClock) {
			f.Arm()
			writeIn(t, f, []byte(ClearCodes+"text"), 64)
			f.Arm()
		}, ClearCodes + Notice, true},
		{"replay, a second clear", func(f *Filter, c *fakeClock) {
			f.Arm()
			writeIn(t, f, append(append([]byte(ClearCodes), burst("one", 2000)...), append([]byte(ClearCodes), burst("two", 20)...)...), 8190)
			tickFor(t, f, c, time.Second)
		}, ClearCodes + Notice + ClearCodes + "\x1b[0m" + string(burst("two", 20)), false},
	} {
		var out bytes.Buffer
		c := newClock()
		f := NewFilter(&out, c.now)
		tc.run(f, c)
		if out.String() != tc.want || f.Holding() != tc.hold {
			t.Errorf("%s: the pane got %q, holding %v; want %q, holding %v", tc.name, out.String()[:min(out.Len(), 120)], f.Holding(), tc.want[:min(len(tc.want), 120)], tc.hold)
		}
	}
}

// TestFilterQuietHoldSlowFirstPrint: on a long chat or a slow machine,
// Cursor's first print after a resize can take over 2 s; it still never
// shows.
func TestFilterQuietHoldSlowFirstPrint(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Resized()
	for i := range 100 { // 1,600 lines over 2.5 s
		writeIn(t, f, burst(fmt.Sprintf("first%d", i), 16), 8190)
		tickFor(t, f, c, 25*time.Millisecond)
	}
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, []byte(ClearCodes), 11)
	tickFor(t, f, c, 500*time.Millisecond)
	writeIn(t, f, burst("second", 1600), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "first") {
		t.Fatalf("a first print over 2.5 s: %d of its lines reached the pane", strings.Count(got, "first"))
	}
}

// TestFilterResizeLateInReplayHoldsNextPrint: a resize near the end of a
// long replay: the quiet hold that the replay's end starts waits for the
// next print, and does not end at once.
func TestFilterResizeLateInReplayHoldsNextPrint(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, []byte(ClearCodes), 11)
	c.t = c.t.Add(10 * time.Millisecond) // the resize comes after the clear
	f.Resized()
	for i := range 76 { // the replay goes on for 1.9 s
		writeIn(t, f, burst(fmt.Sprintf("chat%d", i), 21), 8190)
		tickFor(t, f, c, 25*time.Millisecond)
	}
	tickFor(t, f, c, 400*time.Millisecond)
	writeIn(t, f, burst("next", 1600), 8190)
	tickFor(t, f, c, 250*time.Millisecond)
	writeIn(t, f, append([]byte(ClearCodes), burst("last", 1600)...), 8190)
	tickFor(t, f, c, time.Second)
	if got := out.String(); strings.Contains(got, "next ") {
		t.Fatalf("after a resize late in a replay, %d lines of the next print, with no clear before it, reached the pane", strings.Count(got, "next "))
	}
}

// TestFilterFlushAfterResizeInReplayHoldsNothing: at the agent's end, a
// resize during the last replay leaves no hold behind: what comes after the
// flush still reaches the pane.
func TestFilterFlushAfterResizeInReplayHoldsNothing(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	f.Arm()
	writeIn(t, f, append([]byte(ClearCodes), burst("chat", 20)...), 8190)
	c.t = c.t.Add(10 * time.Millisecond) // the resize comes after the clear
	f.Resized()
	_ = f.Flush()
	writeIn(t, f, []byte("late bytes"), 64)
	if f.Holding() || !strings.HasSuffix(out.String(), "late bytes") {
		t.Fatalf("after the flush: holding %v, the late bytes reached the pane: %v", f.Holding(), strings.HasSuffix(out.String(), "late bytes"))
	}
}
