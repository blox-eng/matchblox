package guard

import (
	"bytes"
	"fmt"
	"math/rand"
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
	for range 2 { // one window that saw the burst, one that saw nothing
		c.t = c.t.Add(endWindow)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
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

// TestFilterTrickle: Cursor also clears for its own reasons (a new chat, a
// /clear), with little after it. That hold ends within one window, and
// what follows passes as it comes.
func TestFilterTrickle(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
	writeIn(t, f, []byte(ClearCodes), 9)
	word := bytes.Repeat([]byte("w"), 200)
	for at := 100 * time.Millisecond; at <= 300*time.Millisecond; at += 100 * time.Millisecond {
		c.t = c.t.Add(100 * time.Millisecond)
		writeIn(t, f, word, 200)
		if err := f.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if f.Holding() {
		t.Fatal("300 ms after a clear with 600 bytes behind it, the hold goes on")
	}
	if want := ClearCodes + Notice + ClearCodes + "\x1b[0m" + strings.Repeat("w", 600); out.String() != want {
		t.Fatalf("after a small clear the pane got %d bytes, want %d: the clear and all 600 bytes", out.Len(), len(want))
	}
	before := out.Len()
	writeIn(t, f, word, 200)
	if out.Len() != before+200 {
		t.Fatalf("after the hold, 200 bytes in gave %d out at once", out.Len()-before)
	}
}

// TestFilterCap: a replay that never slows does not hold the pane for ever.
func TestFilterCap(t *testing.T) {
	var out bytes.Buffer
	c := newClock()
	f := NewFilter(&out, c.now)
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
	writeIn(t, f, []byte(ClearCodes+"\nfirst\r\nlast"), 64)
	c.t = c.t.Add(endWindow)
	if err := f.Tick(); err != nil {
		t.Fatal(err)
	}
	if want := ClearCodes + Notice + ClearCodes + "\x1b[0m\nfirst\r\nlast"; out.String() != want {
		t.Fatalf("a short replay came out as %q, want %q", out.String(), want)
	}
}
