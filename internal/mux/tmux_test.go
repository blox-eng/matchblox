package mux

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/panes"
)

func TestTmuxTypeEscapesAFinalSemicolon(t *testing.T) {
	if got, want := (Tmux{}).Type("%1", "a;"), panes.Literal("%1", "a;"); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTmuxKeysPressKeyNames(t *testing.T) {
	if got, want := (Tmux{}).Keys("%1", "C-u", "Enter"), panes.Keys("%1", "C-u", "Enter"); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTmuxInModeReadsDisplayMessage(t *testing.T) {
	var ran []string
	m := Tmux{Run: func(argv []string) (string, error) { ran = argv; return "1\n", nil }}
	in, err := m.InMode("%1")
	if err != nil || !in {
		t.Fatalf("in = %v, %v", in, err)
	}
	if want := []string{"tmux", "display-message", "-p", "-t", "%1", "#{pane_in_mode}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	m.Run = func([]string) (string, error) { return "0\n", nil }
	if in, err := m.InMode("%1"); err != nil || in {
		t.Fatalf("not in a mode: %v, %v", in, err)
	}
}

func TestTmuxScreensUsesItsCapture(t *testing.T) {
	m := Tmux{Capture: func(ps []string) map[string]string { return map[string]string{ps[0]: "screen"} }}
	if got := m.Screens([]string{"%1"}); got["%1"] != "screen" {
		t.Fatalf("got %q", got)
	}
}

func TestTmuxMarkSetsAndTakesOffThePaneOption(t *testing.T) {
	var ran [][]string
	m := Tmux{Run: func(argv []string) (string, error) { ran = append(ran, argv); return "", nil }}
	if err := m.Mark("%1", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Mark("%4@matchblox", false); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"tmux", "set-option", "-p", "-t", "%1", "@matchblox_match", "1"},
		{"tmux", "-L", "matchblox", "set-option", "-p", "-u", "-t", "%4", "@matchblox_match"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
}
