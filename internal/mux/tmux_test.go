package mux

import (
	"errors"
	"slices"
	"testing"
	"time"

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

func TestTmuxLoadPutsTheTextOnStdinAndPasteIsBracketed(t *testing.T) {
	var ran [][]string
	var in string
	m := Tmux{
		Run:   func(argv []string) (string, error) { ran = append(ran, argv); return "", nil },
		RunIn: func(argv []string, stdin string) error { ran = append(ran, argv); in = stdin; return nil },
	}
	if err := m.Load("%4@matchblox", "matchblox-paste-1", "a\nb;"); err != nil {
		t.Fatal(err)
	}
	if err := m.Drop("%1", "matchblox-paste-1"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"tmux", "-L", "matchblox", "load-buffer", "-b", "matchblox-paste-1", "-"},
		{"tmux", "delete-buffer", "-b", "matchblox-paste-1"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal) || in != "a\nb;" {
		t.Fatalf("ran %q with %q, want %q", ran, in, want)
	}
	if got, want := m.Paste("%1", "matchblox-paste-1"), []string{"tmux", "paste-buffer", "-p", "-d", "-b", "matchblox-paste-1", "-t", "%1"}; !slices.Equal(got, want) {
		t.Fatalf("paste %q, want %q", got, want)
	}
}

func TestTmuxTypedAtIsTheLatestKeyOfAClientOnThePane(t *testing.T) {
	var ran []string
	m := Tmux{Run: func(argv []string) (string, error) {
		ran = argv
		return "1700000000 %1\n1700000100 %1\n1700000500 %2\ncan't find session: gone\n", nil
	}}
	if got := m.TypedAt("%1"); !got.Equal(time.Unix(1700000100, 0)) {
		t.Fatalf("TypedAt = %v", got)
	}
	if want := []string{"tmux", "list-clients", "-F", "#{client_activity} #{pane_id}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	if got := m.TypedAt("%3"); !got.IsZero() {
		t.Fatalf("no client on %%3: %v", got)
	}
	m.TypedAt("%4@matchblox")
	if want := []string{"tmux", "-L", "matchblox", "list-clients", "-F", "#{client_activity} #{pane_id}"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %q, want %q", ran, want)
	}
	m.Run = func([]string) (string, error) { return "", errors.New("no server") }
	if got := m.TypedAt("%1"); !got.IsZero() {
		t.Fatalf("no server: %v", got)
	}
}
