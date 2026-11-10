package panes

import (
	"reflect"
	"testing"
)

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
