package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeTmux answers the console's tmux calls: set is whether set-option
// works, before the pane's own allow-passthrough ("" unset).
type fakeTmux struct {
	set    bool
	before string
	calls  [][]string
}

func (f *fakeTmux) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "show-options":
		return []byte(f.before + "\n"), nil
	case "set-option":
		if !f.set {
			return nil, errors.New("invalid option: allow-passthrough")
		}
		return nil, nil
	case "display":
		return []byte("main\t@1\n"), nil
	}
	return []byte("/dev/ttys002\t4242\txterm-kitty\tmain\t@1\n"), nil
}

var inTmux = func(k string) string {
	return map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TMUX_PANE": "%7"}[k]
}

// TestOldTmuxKeepsTheMarks: a tmux that cannot pass images through
// (3.2 and older) gets the marks, not blank cells, though kitty shows it.
func TestOldTmuxKeepsTheMarks(t *testing.T) {
	f := &fakeTmux{}
	ask, restore := consoleGraphicsWith(inTmux, f.run)
	defer restore()
	if can, _, _ := ask(); can {
		t.Fatal("logos drawn where tmux passes no image")
	}
}

// TestConsoleRestoresPassthroughOnExit: the console's pane gets back its
// own allow-passthrough when the console ends, unset or as it was:
// passthrough left open lets later output in that pane reach the terminal.
func TestConsoleRestoresPassthroughOnExit(t *testing.T) {
	for before, want := range map[string][]string{
		"":   {"set-option", "-p", "-u", "-t", "%7", "allow-passthrough"},
		"on": {"set-option", "-p", "-t", "%7", "allow-passthrough", "on"},
	} {
		f := &fakeTmux{set: true, before: before}
		ask, restore := consoleGraphicsWith(inTmux, f.run)
		if can, _, _ := ask(); !can {
			t.Fatalf("before %q: kitty on a passing tmux cannot draw", before)
		}
		restore()
		if last := f.calls[len(f.calls)-1]; !slices.Equal(last, want) {
			t.Errorf("before %q: last call %q, want %q", before, strings.Join(last, " "), strings.Join(want, " "))
		}
	}
}
