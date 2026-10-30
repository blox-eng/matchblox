package sample

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// runs is a fake command runner: it keeps each argv and answers the
// display-message with the pane's history and height.
type runs struct {
	argv          [][]string
	history, rows int
}

func (r *runs) run(name string, args ...string) ([]byte, error) {
	r.argv = append(r.argv, append([]string{name}, args...))
	if slices.Contains(args, "display-message") {
		return fmt.Appendf(nil, "%d %d\n", r.history, r.rows), nil
	}
	return []byte("history\n"), nil
}

func TestCapturePaneUpTakesTheLinesUpFromTheBottom(t *testing.T) {
	r := &runs{history: 500, rows: 20}
	text, up, ok := capturePaneUp(r.run, "%3@matchblox", 30)
	if !ok || up != 30 || text != "history\n" {
		t.Fatalf("text %q up %d ok %v", text, up, ok)
	}
	want := [][]string{
		{"tmux", "-u", "-L", "matchblox", "display-message", "-p", "-t", "%3", "#{history_size} #{pane_height}"},
		{"tmux", "-u", "-L", "matchblox", "capture-pane", "-p", "-e", "-S", "-30", "-E", "-11", "-t", "%3"},
	}
	if !reflect.DeepEqual(r.argv, want) {
		t.Fatalf("argv %q, want %q", r.argv, want)
	}
}

func TestCapturePaneUpStopsAtTheTopOfTheHistory(t *testing.T) {
	r := &runs{history: 10, rows: 20}
	_, up, ok := capturePaneUp(r.run, "%3", 30)
	if !ok || up != 10 {
		t.Fatalf("up %d ok %v, want 10", up, ok)
	}
	if got := r.argv[1]; !reflect.DeepEqual(got, []string{"tmux", "-u", "capture-pane", "-p", "-e", "-S", "-10", "-E", "9", "-t", "%3"}) {
		t.Fatalf("argv %q", got)
	}
}

func TestCapturePaneUpOfAGonePane(t *testing.T) {
	fail := func(string, ...string) ([]byte, error) { return nil, errors.New("no pane") }
	if _, _, ok := capturePaneUp(fail, "%3", 30); ok {
		t.Fatal("a gone pane captured")
	}
}
