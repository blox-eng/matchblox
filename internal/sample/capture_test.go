package sample

import (
	"slices"
	"testing"
)

// TestCaptureScreenOnlyReads: explain captures a pane to show it, so the
// capture runs only the tmux commands that read, never one that types.
func TestCaptureScreenOnlyReads(t *testing.T) {
	var argv [][]string
	run := func(name string, args ...string) ([]byte, error) {
		argv = append(argv, append([]string{name}, args...))
		return []byte("0\nDo you want to proceed?\n"), nil
	}
	if _, _, ok := captureScreen(run, "%3@matchblox"); !ok {
		t.Fatal("the capture failed")
	}
	reads := []string{"display-message", "capture-pane"}
	for _, a := range argv {
		var cmds []string
		i := 1
		for i < len(a) && (a[i] == "-u" || a[i] == "-L") {
			if a[i] == "-L" {
				i++ // the server's name
			}
			i++
		}
		cmds = append(cmds, a[i])
		for j := i; j < len(a)-1; j++ {
			if a[j] == ";" {
				cmds = append(cmds, a[j+1])
			}
		}
		for _, c := range cmds {
			if !slices.Contains(reads, c) {
				t.Errorf("the capture runs the tmux command %q: %q", c, a)
			}
		}
	}
	if len(argv) == 0 {
		t.Fatal("the capture ran nothing")
	}
}
