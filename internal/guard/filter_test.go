package guard

import (
	"bytes"
	"math/rand"
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
