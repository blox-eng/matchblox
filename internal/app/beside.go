package app

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

// Beside mode: the console's tmux window holds two panes, the list and the
// agent beside it. The console draws the list only; tmux draws the agent.

// besideMsg is the agent pane the split opened, or why it did not.
type besideMsg struct {
	pane string
	err  error
}

// besideSizeMsg is the window's width, as the window changed.
type besideSizeMsg struct{ w int }

var paneID = regexp.MustCompile(`^%\d+$`)

// hexOf is c as a tmux colour.
func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// besideCmd splits the window once, at the start.
func (m Model) besideCmd() tea.Cmd {
	run := m.tmuxRun()
	if !m.opt.Beside || run == nil {
		return nil
	}
	console, accent := m.opt.Pane, hexOf(m.st.theme.Accent)
	return func() tea.Msg {
		argv := remote.SplitArgv(console, listWide, windowWidth(run, console))
		out, err := run(argv[1:]...)
		if err != nil {
			return besideMsg{err: err}
		}
		pane := strings.TrimSpace(out)
		if !paneID.MatchString(pane) {
			return besideMsg{err: fmt.Errorf("tmux answered %q", pane)}
		}
		if _, err := run(remote.BesideArgv(console, pane, accent)[1:]...); err != nil {
			return besideMsg{err: err}
		}
		return besideMsg{pane: pane}
	}
}

// windowWidth is the console window's width; 0 when tmux does not say.
func windowWidth(run func(args ...string) (string, error), pane string) int {
	out, err := run("display", "-p", "-t", pane, "#{window_width}")
	if err != nil {
		return 0
	}
	w, _ := strconv.Atoi(strings.TrimSpace(out))
	return w
}

func (m Model) windowWidth() int {
	if run := m.tmuxRun(); run != nil {
		return windowWidth(run, m.opt.Pane)
	}
	return 0
}

// sizeCmd follows the window: one pane zoomed where the agent would be too
// narrow beside the list, else both, the list at its width.
func (m Model) sizeCmd(paneW int) tea.Cmd {
	run := m.tmuxRun()
	if m.beside == "" || run == nil {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		w := windowWidth(run, console)
		if w == 0 {
			w = paneW
		}
		if m.onePaneAt(w) {
			_, _ = run(remote.ZoomArgv(console, true)[1:]...)
			return besideSizeMsg{w}
		}
		lw := listMedium
		if layout(w, m.shapeRows()) == Wide {
			lw = listWide
		}
		// A resize ends a zoom, so it runs only where the window is not one.
		_, _ = run(remote.ZoomArgv(console, false)[1:]...)
		_, _ = run(remote.ListWidthArgv(console, lw)[1:]...)
		return besideSizeMsg{w}
	}
}

// onePaneAt: a window of w columns shows one pane at a time.
func (m Model) onePaneAt(w int) bool {
	return w-listMedium-1 < chatMin || m.onPocket() && (m.upright() || layout(w, m.shapeRows()) == Narrow)
}
