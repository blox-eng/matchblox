package panes

import (
	"reflect"
	"testing"
)

func TestWindowOfReadsTheWindowOfAPane(t *testing.T) {
	got := WindowOf("%4@matchblox-test")
	want := []string{"tmux", "-L", "matchblox-test", "display-message", "-p", "-t", "%4", "#{window_id}\t#{window_panes}\t#{window_width}x#{window_height}\t#{@matchblox-sized}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestWindowSizeOfReadsOnlyTheWindowsOwnValue(t *testing.T) {
	got := WindowSizeOf("", "@5")
	want := []string{"tmux", "show-options", "-w", "-v", "-t", "@5", "window-size"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

// The first size marks the window with what it had; a later size keeps
// the first mark.
func TestSizeWindowMarksManualResizesAndReadsThePane(t *testing.T) {
	got := SizeWindow("%4", "@5", "- 120x40", 60, 20)
	want := []string{"tmux",
		"set-option", "-w", "-t", "@5", "@matchblox-sized", "- 120x40", ";",
		"set-option", "-w", "-t", "@5", "window-size", "manual", ";",
		"resize-window", "-t", "@5", "-x", "60", "-y", "20", ";",
		"display-message", "-p", "-t", "%4", "#{pane_width} #{pane_height}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	got = SizeWindow("%4@matchblox-test", "@5", "", 61, 21)
	want = []string{"tmux", "-L", "matchblox-test",
		"set-option", "-w", "-t", "@5", "window-size", "manual", ";",
		"resize-window", "-t", "@5", "-x", "61", "-y", "21", ";",
		"display-message", "-p", "-t", "%4", "#{pane_width} #{pane_height}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("again: got %q", got)
	}
}

func TestSizeWindowStaysInTmuxBounds(t *testing.T) {
	got := SizeWindow("%4", "@5", "", 3, 1)
	if got[12] != "20" || got[14] != "5" {
		t.Fatalf("small: got %q", got)
	}
	got = SizeWindow("%4", "@5", "", 99999, 99999)
	if got[12] != "10000" || got[14] != "10000" {
		t.Fatalf("large: got %q", got)
	}
}

func TestMarkOf(t *testing.T) {
	for _, c := range []struct{ prev, size, want string }{
		{"", "120x40", "- 120x40"},
		{"latest\n", "120x40", "latest 120x40"},
		{"manual", "80x24", "manual 80x24"},
	} {
		if got := MarkOf(c.prev, c.size); got != c.want {
			t.Errorf("MarkOf(%q, %q) = %q, want %q", c.prev, c.size, got, c.want)
		}
	}
}

func TestRestoreWindow(t *testing.T) {
	unset := []string{"tmux", "set-option", "-w", "-u", "-t", "@5", "window-size", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}
	for _, c := range []struct {
		server, mark string
		want         []string
	}{
		{"", "- 120x40", unset},
		{"", "nonsense 1x1", unset}, // a value tmux would refuse: tmux sizes the window by its clients again
		{"", "latest 120x40", []string{"tmux", "set-option", "-w", "-t", "@5", "window-size", "latest", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}},
		{"", "manual 120x40", []string{"tmux", "set-option", "-w", "-t", "@5", "window-size", "manual", ";", "resize-window", "-t", "@5", "-x", "120", "-y", "40", ";", "set-option", "-w", "-u", "-t", "@5", "@matchblox-sized"}},
		{"matchblox-test", "- 120x40", append([]string{"tmux", "-L", "matchblox-test"}, unset[1:]...)},
	} {
		if got := RestoreWindow(c.server, "@5", c.mark); !reflect.DeepEqual(got, c.want) {
			t.Errorf("RestoreWindow(%q, %q) = %q, want %q", c.server, c.mark, got, c.want)
		}
	}
}

func TestSizedWindowsListsEveryWindowWithItsMark(t *testing.T) {
	got := SizedWindows("matchblox-test")
	want := []string{"tmux", "-L", "matchblox-test", "list-windows", "-a", "-F", "#{window_id}\t#{@matchblox-sized}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
