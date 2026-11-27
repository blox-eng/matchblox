// Package muxtest is a fake multiplexer for tests.
package muxtest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/mux"
)

// Fake is a multiplexer in memory: Screen is each pane's text, Mode the
// panes in copy mode, NoBracket the panes that take no bracketed paste,
// Typed the last key of a person in each pane, Panes what List returns,
// Client and Created what LastClient returns, Self what Caller returns.
// Its steps are argv words that name no program.
type Fake struct {
	mu         sync.Mutex
	Screen     map[string]string
	Mode       map[string]bool
	NoBracket  map[string]bool
	Typed      map[string]time.Time
	Panes      []mux.Pane
	Client     string            // LastClient's tty
	Created    time.Time         // LastClient's creation time
	Self       string            // Caller's pane
	Buffers    []mux.Buffer      // ListBuffers' list; nil: no server
	BufferText map[string]string // ShowBuffer's text of each buffer
	Sized      []mux.SizedWindow // SizedWindows' list; nil: no server
	marked     map[string]bool
	marks      int
	splits     int
	loads      []Load
	drops      []string
	notices    []string
	restores   []mux.SizedWindow
}

// Load is one text Load put into a buffer.
type Load struct{ Pane, Name, Text string }

var _ mux.Mux = (*Fake)(nil)

func (*Fake) Type(pane, text string) []string { return []string{"fake", "type", pane, text} }

func (*Fake) Keys(pane string, keys ...string) []string {
	return slices.Concat([]string{"fake", "keys", pane}, keys)
}

func (f *Fake) Load(pane, name, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads = append(f.loads, Load{pane, name, text})
	return nil
}

func (f *Fake) NoBracketedPaste(pane string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.NoBracket[pane]
}

func (*Fake) Paste(pane, name string) []string { return []string{"fake", "paste", pane, name} }

func (f *Fake) Drop(_, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.drops = append(f.drops, name)
	return nil
}

// Loads are the texts Load put into buffers, in order.
func (f *Fake) Loads() []Load {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.loads)
}

// Dropped are the buffers Drop deleted, in order.
func (f *Fake) Dropped() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.drops)
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

func (f *Fake) TypedAt(pane string) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Typed[pane]
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

func (*Fake) Start(s mux.Start) [][]string {
	step := []string{"fake", "start", s.Name, s.Dir}
	if s.Detached {
		step = append(step, "-d")
	}
	if s.EmptyExit {
		step = append(step, "exit-empty")
	}
	step = append(step, s.Env...)
	return [][]string{append(step, s.Argv...)}
}

func (*Fake) Rename(pane, name string) [][]string {
	return [][]string{{"fake", "rename", pane, name}}
}

func (*Fake) RenameSession(pane, name string) []string {
	return []string{"fake", "rename-session", pane, name}
}

func (*Fake) NewWindow(name, dir string, argv ...string) []string {
	return slices.Concat([]string{"fake", "new-window", name, dir}, argv)
}

func (*Fake) Kill(pane string, pid int) [][]string {
	steps := [][]string{{"fake", "kill-pane", pane}}
	if pid > 0 {
		steps = append(steps, []string{"fake", "kill", "-TERM", strconv.Itoa(pid)})
	}
	return steps
}

func (*Fake) KillSession(name string) []string {
	return []string{"fake", "kill-session", name}
}

func (*Fake) HasSession(name string) []string {
	return []string{"fake", "has-session", name}
}

func (*Fake) Respawn(pane, dir string, env []string, argv []string) [][]string {
	return [][]string{slices.Concat([]string{"fake", "respawn-pane", pane, dir}, env, argv)}
}

// Split opens no pane: it returns the next fake id, %f1, %f2, and so on.
func (f *Fake) Split(_ string, _ []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.splits++
	return "%f" + strconv.Itoa(f.splits), nil
}

// Beside is Split.
func (f *Fake) Beside(target string, _ int, argv []string) (string, error) {
	return f.Split(target, argv)
}

// SplitEdge is Split.
func (f *Fake) SplitEdge(target string, argv []string, _ string, _ int) (string, error) {
	return f.Split(target, argv)
}

func (*Fake) SwitchClient(pane string) []string { return []string{"fake", "switch-client", pane} }

func (*Fake) ViewCmd() string { return "fake-view" }

func (*Fake) Focus(pane string) [][]string {
	return [][]string{{"fake", "select-window", pane}, {"fake", "select-pane", pane}}
}

// LastClient is Client, with Created as its creation time. No Client is
// "" and the zero time, as no session is.
func (f *Fake) LastClient(string) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Client, f.Created, nil
}

func (*Fake) Attach(session string) []string { return []string{"fake", "attach", session} }

// Caller is Self.
func (f *Fake) Caller() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Self
}

func (f *Fake) List() ([]mux.Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.Panes), nil
}

func (f *Fake) ListStrict() ([]mux.Pane, error) { return f.List() }

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

// ListBuffers is Buffers. With none it is an error, as a missing server is.
func (f *Fake) ListBuffers(context.Context) ([]mux.Buffer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Buffers == nil {
		return nil, errors.New("muxtest: no server")
	}
	return slices.Clone(f.Buffers), nil
}

// ShowBuffer is BufferText of name; a name with none is an error.
func (f *Fake) ShowBuffer(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	text, ok := f.BufferText[name]
	if !ok {
		return "", errors.New("muxtest: no buffer " + name)
	}
	return text, nil
}

// Notice notes text.
func (f *Fake) Notice(_ context.Context, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notices = append(f.notices, text)
}

// Notices are the texts Notice showed, in order.
func (f *Fake) Notices() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.notices)
}

// SizedWindows is Sized. With none it is an error, as a missing server is.
func (f *Fake) SizedWindows() ([]mux.SizedWindow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Sized == nil {
		return nil, errors.New("muxtest: no server")
	}
	return slices.Clone(f.Sized), nil
}

// RestoreWindow notes the window and its mark.
func (f *Fake) RestoreWindow(window, mark string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restores = append(f.restores, mux.SizedWindow{Window: window, Mark: mark})
	return nil
}

// Restored are the windows RestoreWindow put back, in order.
func (f *Fake) Restored() []mux.SizedWindow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.restores)
}
