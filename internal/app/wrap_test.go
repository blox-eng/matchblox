package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// A word of two-column runes in a line too narrow for one of them: wrap
// ends, and every rune is on a line.
func TestWrapEndsOnWideRunesInANarrowLine(t *testing.T) {
	for _, w := range []int{4, 5, 6, 9} {
		done := make(chan []string, 1)
		go func() { done <- wrap("日本語日本語", w, plainText) }()
		select {
		case lines := <-done:
			if got := strings.ReplaceAll(strings.Join(lines, ""), " ", ""); got != "日本語日本語" {
				t.Errorf("w %d: the lines hold %q, want every rune of 日本語日本語", w, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("w %d: wrap did not end", w)
		}
	}
}

// A word breaks by its columns, not its bytes: one of three-byte runes that
// the line holds stays whole.
func TestWrapMeasuresAWordInColumns(t *testing.T) {
	word := strings.Repeat("…", 10)
	lines := wrap(word, 20, plainText)
	if len(lines) != 1 || lines[0] != " "+word {
		t.Fatalf("lines = %q, want the word whole on one line", lines)
	}
	for _, l := range wrap(strings.Repeat("日", 30), 20, plainText) {
		if ansi.StringWidth(l) > 20 {
			t.Errorf("line %q is %d columns, over 20", l, ansi.StringWidth(l))
		}
	}
}
