package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// keyTmux is the list's tmux for the keys sheet: it keeps the agent pane's
// capture and the mode the list publishes.
type keyTmux struct {
	agent string
	mode  string
	sent  []string
	calls [][]string
}

func (k *keyTmux) run(args ...string) (string, error) {
	k.calls = append(k.calls, append([]string(nil), args...))
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "capture-pane":
		return k.agent, nil
	case "send-keys":
		k.sent = append(k.sent, strings.Join(args, " "))
		k.agent += strings.Join(args, " ")
		return "", nil
	case "set-option":
		for i, a := range args {
			if a == sample.ModeOption && i+1 < len(args) {
				k.mode = args[i+1]
			}
		}
	case "show-option":
		if slices.Contains(args, sample.ModeOption) {
			return k.mode, nil
		}
	}
	return "", nil
}

func (k *keyTmux) split() bool {
	for _, c := range k.calls {
		if slices.Contains(c, "split-window") || slices.Contains(c, "split-pane") {
			return true
		}
	}
	return false
}

// keysDesk is the list pane of a four-pane desk, keys in the list, with an
// agent pane whose capture the test can read.
func keysDesk(t *testing.T) (Model, *keyTmux) {
	t.Helper()
	tm := &keyTmux{agent: "agent ready\n", mode: "LIST"}
	m := boardShot(t, boardFixture(t), 40, 48, func(m *Model) {
		m.deskFrame = true
		m.beside = "%9"
		m.opt.Beside = true
		m.opt.Pane = "%1"
		m.opt.Tmux = tm.run
		m.barMode = "LIST"
	})
	return m, tm
}

func markedLine(m Model) string {
	for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
		if strings.Contains(l, "▸") {
			return l
		}
	}
	return ""
}

func TestQuestionOpensOverTheList(t *testing.T) {
	m, tm := keysDesk(t)
	before, err := tm.run("capture-pane", "-p", "-t", "%9")
	if err != nil {
		t.Fatal(err)
	}
	m = pressOn(m, "?")
	after, _ := tm.run("capture-pane", "-p", "-t", "%9")
	if after != before || after != "agent ready\n" {
		t.Fatalf("agent capture changed %q -> %q", before, after)
	}
	if len(tm.sent) != 0 {
		t.Fatalf("? sent keys into a pane: %q", tm.sent)
	}
	if m.keySheet == nil {
		t.Fatal("? did not open the keys sheet")
	}
	body := ansi.Strip(m.render())
	last := -1
	for _, want := range []string{"anywhere", "the list", "a review", "the shell", "⚙", "ctrl+q", "t  shell"} {
		i := strings.Index(body, want)
		if i < 0 {
			t.Fatalf("list missing %q:\n%s", want, body)
		}
		if i < last && want != "ctrl+q" && want != "t  shell" {
			t.Fatalf("%q is out of group order in\n%s", want, body)
		}
		if want != "ctrl+q" && want != "t  shell" {
			last = i
		}
	}
	if tm.mode != "KEYS" || m.barMode != "KEYS" {
		t.Fatalf("mode %q, option %q, want KEYS", m.barMode, tm.mode)
	}
	bar := New(Options{NoMotion: true, Conn: newFake(), Pane: "%1", Tmux: tm.run, Now: func() time.Time { return demoAt }})
	f := newFrame("bar", bar)
	f.width = 80
	shown, _ := f.Update(frameTick{})
	raw := shown.(frameModel).View().Content
	if want := bar.modeWord("KEYS"); !strings.Contains(raw, want) {
		t.Fatalf("bar %q, want the accent word %q", ansi.Strip(raw), want)
	}
	first := markedLine(m)
	m = pressOn(m, "down")
	if next := markedLine(m); next == "" || next == first {
		t.Fatalf("down stayed on %q", first)
	}
	m = pressOn(m, "ctrl+q")
	if m.keySheet != nil || strings.Contains(ansi.Strip(m.render()), "anywhere") {
		t.Fatal("ctrl+q left the keys sheet open")
	}
	if m.barMode != "LIST" || tm.mode != "LIST" {
		t.Fatalf("after close, mode %q option %q, want LIST", m.barMode, tm.mode)
	}
	if end, _ := tm.run("capture-pane", "-p", "-t", "%9"); end != before {
		t.Fatalf("agent capture after the sheet: %q", end)
	}
}

func TestQuestionShellRunsT(t *testing.T) {
	m, tm := keysDesk(t)
	byT := pressOn(m, "t")
	if byT.shellAsks != 1 || byT.keySheet != nil {
		t.Fatalf("t: shell asks %d, sheet open %v", byT.shellAsks, byT.keySheet != nil)
	}
	if tm.split() {
		t.Fatalf("t split a pane: %q", tm.calls)
	}

	m, tm = keysDesk(t)
	m = pressOn(m, "?", "s", "h", "e", "l", "l")
	if line := markedLine(m); !strings.Contains(line, "t  shell") {
		t.Fatalf("shell chose %q", line)
	}
	m = pressOn(m, "enter")
	if m.shellAsks != 1 || m.keySheet != nil {
		t.Fatalf("? shell enter: shell asks %d, sheet open %v", m.shellAsks, m.keySheet != nil)
	}
	if byT.shellAsks != m.shellAsks {
		t.Fatalf("t asks %d, sheet asks %d", byT.shellAsks, m.shellAsks)
	}
	if tm.split() {
		t.Fatalf("shell split a pane: %q", tm.calls)
	}
}

func TestNoKeyDoesX(t *testing.T) {
	m, _ := keysDesk(t)
	m = pressOn(m, "?", "x", "y", "z")
	const want = `No key does "xyz". esc clears it.`
	if got := ansi.Strip(m.render()); !strings.Contains(got, want) {
		t.Fatalf("list %q, want %q", got, want)
	}
	m = pressOn(m, "esc")
	got := ansi.Strip(m.render())
	if strings.Contains(got, want) || strings.Contains(got, "xyz") {
		t.Fatalf("esc did not clear the filter: %q", got)
	}
	if m.keySheet == nil || !strings.Contains(got, "anywhere") {
		t.Fatal("esc closed the sheet instead of clearing the filter")
	}
}

// TestEnterOnALaterRowSaysWhere: ⏎ on a row that works in the review, the
// shell or ⚙ keeps the sheet and says where the key works (review of
// 2026-11-27, Minor 2). Another key clears the word.
func TestEnterOnALaterRowSaysWhere(t *testing.T) {
	for _, tc := range []struct{ filter, want string }{
		{"send", "works in a review"},
		{"flip", "works in the shell"},
		{"open", "works in ⚙"},
	} {
		m, _ := keysDesk(t)
		m = pressOn(m, "?")
		for _, r := range tc.filter {
			m = pressOn(m, string(r))
		}
		// "flip" and "open" also name rows of other groups: go down to the later one.
		for i := 0; i < 10 && m.keyMatches()[m.keySheet.at].run != runLater; i++ {
			m = pressOn(m, "down")
		}
		m = pressOn(m, "enter")
		if m.keySheet == nil {
			t.Fatalf("%s: ⏎ closed the sheet with no word", tc.filter)
		}
		if body := ansi.Strip(m.render()); !strings.Contains(body, tc.want) {
			t.Fatalf("%s: no %q in\n%s", tc.filter, tc.want, body)
		}
		m = pressOn(m, "down")
		if body := ansi.Strip(m.render()); strings.Contains(body, tc.want) {
			t.Fatalf("%s: the word stays after another key", tc.filter)
		}
	}
}
