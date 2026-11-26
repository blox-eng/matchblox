//go:build unix

package remote

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// besideOf opens the agent pane beside console, as the console does, and
// gives its id.
func besideOf(t *testing.T, sock, console string, listW, windowW int) string {
	t.Helper()
	id, err := mux.Tmux{Server: sock}.Beside(console, BesidePercent(listW, windowW), BesideShell())
	if err != nil {
		t.Fatal(err)
	}
	_, bare := mux.SplitPane(id)
	return bare
}

type paneBox struct {
	id          string
	left, width int
}

func panesOf(t *testing.T, sock, target string) []paneBox {
	t.Helper()
	var out []paneBox
	for _, l := range strings.Split(mustTmux(t, sock, "list-panes", "-t", target, "-F", "#{pane_id} #{pane_left} #{pane_width}"), "\n") {
		f := strings.Fields(l)
		left, _ := strconv.Atoi(f[1])
		width, _ := strconv.Atoi(f[2])
		out = append(out, paneBox{f[0], left, width})
	}
	return out
}

func TestAChainViewFillsTheRightColumnOnly(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			sock := tmuxtest.Server(t)
			mustTmux(t, sock, "-f", "/dev/null", "new-session", "-d", "-s", "console", "-x", "160", "-y", "50", "sleep", "600")
			console := mustTmux(t, sock, "display", "-p", "-t", "=console:", "#{pane_id}")
			beside := besideOf(t, sock, console, 40, 160)
			listW := panesOf(t, sock, console)[0].width

			views := make([][]string, n)
			for i := range views {
				views[i] = []string{"cat"}
			}
			out := mustTmux(t, sock, ChainArgv(beside, views)[1:]...)
			if got := len(strings.Split(out, "\n")); got != n-1 {
				t.Fatalf("%d split lines: %q", got, out)
			}
			panes := panesOf(t, sock, console)
			if len(panes) != n+1 {
				t.Fatalf("%d panes: %v", len(panes), panes)
			}
			var right []string
			for _, p := range panes {
				switch {
				case p.id == console:
					if p.left != 0 || p.width != listW {
						t.Errorf("the console moved: %+v, was %d wide", p, listW)
					}
				case p.left <= listW:
					t.Errorf("%s is not right of the list: %+v", p.id, p)
				default:
					right = append(right, p.id)
				}
			}
			chain := strings.Fields(mustTmux(t, sock, "show-option", "-wqv", "-t", console, sample.ChainOption))
			if chain[0] != beside || !sameSet(chain, right) {
				t.Fatalf("%s = %q, panes right %q", sample.ChainOption, chain, right)
			}
			// Closed on its last member: that pane alone fills the right side.
			keep := chain[len(chain)-1]
			for _, id := range chain[:len(chain)-1] {
				mustTmux(t, sock, "kill-pane", "-t", id)
			}
			if got := panesOf(t, sock, console); len(got) != 2 || got[0].width != listW || got[1].id != keep || got[1].left != listW+1 {
				t.Fatalf("after the close: %+v", got)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// TestTheZoomReadsTheConsolesWindow: the session shows another window, zoomed;
// the console's window, not zoomed, still zooms.
func TestTheZoomReadsTheConsolesWindow(t *testing.T) {
	sock := tmuxtest.Server(t)
	mustTmux(t, sock, "-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", "120", "-y", "30", "sleep", "600")
	console := mustTmux(t, sock, "display", "-p", "-t", "=matchblox:", "#{pane_id}")
	besideOf(t, sock, console, 40, 120)
	other := mustTmux(t, sock, "new-window", "-t", "=matchblox:", "-P", "-F", "#{pane_id}", "sleep", "600")
	mustTmux(t, sock, "split-window", "-t", other, "sleep", "600")
	mustTmux(t, sock, "resize-pane", "-Z", "-t", other)
	mustTmux(t, sock, ZoomArgv(console, true)[1:]...)
	if z := mustTmux(t, sock, "display", "-p", "-t", console, "#{window_zoomed_flag}"); z != "1" {
		t.Fatalf("the console's window is not zoomed: %s", z)
	}
	mustTmux(t, sock, ZoomArgv(console, false)[1:]...)
	if z := mustTmux(t, sock, "display", "-p", "-t", console, "#{window_zoomed_flag}"); z != "0" {
		t.Fatalf("the console's window is still zoomed: %s", z)
	}
	if z := mustTmux(t, sock, "display", "-p", "-t", other, "#{window_zoomed_flag}"); z != "1" {
		t.Fatalf("the other window lost its zoom: %s", z)
	}
}
