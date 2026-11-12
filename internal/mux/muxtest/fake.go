// Package muxtest is a fake multiplexer for tests.
package muxtest

import (
	"maps"
	"slices"
	"sync"

	"github.com/blox-eng/matchblox/internal/mux"
)

// Fake is a multiplexer in memory: Screen is each pane's text, Mode the
// panes in copy mode. Its steps are argv words that name no program.
type Fake struct {
	mu     sync.Mutex
	Screen map[string]string
	Mode   map[string]bool
	marked map[string]bool
	marks  int
}

var _ mux.Mux = (*Fake)(nil)

func (*Fake) Type(pane, text string) []string { return []string{"fake", "type", pane, text} }

func (*Fake) Keys(pane string, keys ...string) []string {
	return slices.Concat([]string{"fake", "keys", pane}, keys)
}

func (f *Fake) Screens(ps []string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, p := range ps {
		if s, ok := f.Screen[p]; ok {
			out[p] = s
		}
	}
	return out
}

func (f *Fake) InMode(pane string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Mode[pane], nil
}

// SetScreen sets pane's text.
func (f *Fake) SetScreen(pane, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Screen == nil {
		f.Screen = map[string]string{}
	}
	f.Screen[pane] = text
}

func (f *Fake) Mark(pane string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.marked == nil {
		f.marked = map[string]bool{}
	}
	f.marks++
	if on {
		f.marked[pane] = true
	} else {
		delete(f.marked, pane)
	}
	return nil
}

// Marked are the panes marked now.
func (f *Fake) Marked() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.marked)
}

// MarkCalls is how many times Mark ran.
func (f *Fake) MarkCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.marks
}
