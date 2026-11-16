package app

import (
	"cmp"
	"errors"
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
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
		if pane := keptPane(run, console); pane != "" {
			if _, err := run(remote.BesideArgv(console, pane, accent)[1:]...); err != nil {
				return besideMsg{err: err}
			}
			return besideMsg{pane: pane}
		}
		pane, err := splitBeside(run, console, accent, listWide, windowWidth(run, console))
		return besideMsg{pane: pane, err: err}
	}
}

// splitBeside opens the agent pane beside the console's, listW columns
// left to the list, and names it in the window.
func splitBeside(run func(args ...string) (string, error), console, accent string, listW, windowW int) (string, error) {
	out, err := run(remote.SplitArgv(console, listW, windowW)[1:]...)
	if err != nil {
		return "", err
	}
	pane := strings.TrimSpace(out)
	if !paneID.MatchString(pane) {
		return "", fmt.Errorf("tmux answered %q", pane)
	}
	if _, err := run(remote.BesideArgv(console, pane, accent)[1:]...); err != nil {
		return "", err
	}
	return pane, nil
}

// keptPane is the agent pane an earlier console left in this window, when it
// still exists; a console that restarts keeps it. "" when there is none.
func keptPane(run func(args ...string) (string, error), console string) string {
	out, err := run("show-option", "-wqv", "-t", console, sample.BesideOption)
	pane := strings.TrimSpace(out)
	if err != nil || !livePane(run, pane) {
		return ""
	}
	return pane
}

// livePane: tmux still has pane.
func livePane(run func(args ...string) (string, error), pane string) bool {
	if !paneID.MatchString(pane) {
		return false
	}
	got, err := run("display", "-p", "-t", pane, "#{pane_id}")
	return err == nil && strings.TrimSpace(got) == pane
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

// sizeCmd follows the window: the pane in use zoomed where the agent would
// be too narrow beside the list, else both, the list at its width.
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
			_, _ = run(remote.ZoomArgv(inUse(run, console), true)[1:]...)
			return besideSizeMsg{w}
		}
		// A resize ends a zoom, so it runs only where the window is not one.
		_, _ = run(remote.ZoomArgv(console, false)[1:]...)
		_, _ = run(remote.ListWidthArgv(console, m.listWidth(w))[1:]...)
		return besideSizeMsg{w}
	}
}

// inUse is the pane of the console's window that has the keys; the
// console when tmux does not say.
func inUse(run func(args ...string) (string, error), console string) string {
	out, err := run("list-panes", "-t", console, "-F", "#{?pane_active,#{pane_id},}")
	if pane := strings.TrimSpace(out); err == nil && paneID.MatchString(pane) {
		return pane
	}
	return console
}

// listWidth is the list pane's width in a window of w columns.
func (m Model) listWidth(w int) int {
	if layout(w, m.shapeRows()) == Wide {
		return listWide
	}
	return listMedium
}

// onePaneAt: a window of w columns shows one pane at a time.
func (m Model) onePaneAt(w int) bool {
	return w-listMedium-1 < chatMin || m.onPocket() && (m.upright() || layout(w, m.shapeRows()) == Narrow)
}

// besideMode: the agent has a pane beside the list, or the console makes
// one at its start.
func (m Model) besideMode() bool {
	return m.beside != "" || m.opt.Beside && m.tmuxRun() != nil
}

// shownMsg is the end of a choice: the agent pane it used, and why the
// view did not start.
type shownMsg struct {
	agent      string // the agent pane, made again where it was gone
	host, pane string // the session shown
	name       string
	err        error
}

// showChosen shows the session the list chooses in the agent pane.
func (m Model) showChosen() (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	switch {
	case !ok:
		return m, m.notice("no session to show · " + m.keys.main.New.Help().Key + " starts one")
	case isShipCard(s):
		m.flash = m.lineOff(s)
		return m, nil
	case s.Pane == "":
		m.flash = noPane(s.Name)
		return m, nil
	}
	return m.showBeside(s.Host, s.Pane)
}

// showBeside is goIntoAsking for the agent pane: on a host the stoker
// drives the take-over question comes first, and a peer's consent box.
func (m Model) showBeside(host, pane string) (tea.Model, tea.Cmd) {
	if h := cmp.Or(host, m.opt.Host); m.takeOverOn(h) {
		m.held, m.heldOn, m.flash = func(m Model) (tea.Model, tea.Cmd) { return m.besideOn(host, pane) }, h, ""
		return m, nil
	}
	return m.besideOn(host, pane)
}

// besideOn runs the view of host's pane in the agent pane: this machine's
// at once, a peer's after its consent box.
func (m Model) besideOn(host, pane string) (tea.Model, tea.Cmd) {
	host = cmp.Or(host, m.opt.Host)
	name := m.besideName(host, pane)
	a := &action{label: "show " + name, beside: true, pane: pane, host: host}
	if host != "" {
		_, ssh, b := m.peerJump(remote.Jump{Host: host, Pane: pane}, a)
		if ssh != nil {
			b.steps, b.say, b.nav, b.remote = [][]string{remote.PeerViewArgv(name, ssh)}, shellLine(ssh), true, true
		}
		return m.runInto(b)
	}
	if m.opt.Socket == "" {
		m.flash = name + " is not shown: the console does not know its tmux socket ($TMUX names none); " + m.keys.main.Jump.Help().Key + " opens it"
		return m, nil
	}
	j := remote.Jump{Pane: pane, Prefix: m.prefix(), ID: m.jumpID(), Outer: true}
	if !remote.ViewAllowed(j, name, m.opt.Socket) {
		m.flash = "refused: " + name + " is not shown: its pane, its name or the tmux socket " + m.opt.Socket + " is not one a view takes"
		return m, nil
	}
	a.steps = [][]string{remote.ViewArgv(j, name, m.opt.Socket)}
	return m.runInto(a)
}

// besideName is the name the list shows for host's pane; the pane id when the
// list has none, or a name a view cannot print.
func (m Model) besideName(host, pane string) string {
	for _, s := range m.all.sessions {
		if name := m.cardName(s); cmp.Or(s.Host, m.opt.Host) == host && s.Pane == pane && remote.ViewNameAllowed(name) {
			return name
		}
	}
	return pane
}

// startShow runs a choice in the agent pane, one at a time: a choice made
// while one runs waits, and a later one takes its place.
func (m Model) startShow(a action) (Model, tea.Cmd) {
	if m.showing {
		m.showNext = &a
		return m, nil
	}
	cmd := m.besideRun(a)
	m.showing = cmd != nil
	return m, cmd
}

// errOwnWindow: the chosen pane is in the console's window, and a view of
// it would show the view itself.
var errOwnWindow = errors.New("in the console's own window")

// sameWindow: tmux has pane in the console's window.
func sameWindow(run func(args ...string) (string, error), console, pane string) bool {
	here, err := run("display", "-p", "-t", console, "#{window_id}")
	if err != nil {
		return false
	}
	there, err := run("display", "-p", "-t", pane, "#{window_id}")
	return err == nil && strings.TrimSpace(there) == strings.TrimSpace(here)
}

// besideRun respawns the agent pane with a's view off the update loop, and
// gives it the keys. Where the agent pane is gone it is made again first.
// Where one pane shows, the agent's is zoomed.
func (m Model) besideRun(a action) tea.Cmd {
	run := m.tmuxRun()
	if run == nil || len(a.steps) != 1 {
		return nil
	}
	console, accent, agent, winW, name := m.opt.Pane, hexOf(m.st.theme.Accent), m.beside, m.winW, m.besideName(a.host, a.pane)
	return func() tea.Msg {
		msg := shownMsg{agent: agent, host: a.host, pane: a.pane, name: name}
		if !a.remote && sameWindow(run, console, a.pane) {
			msg.err = errOwnWindow
			return msg
		}
		w := windowWidth(run, console)
		if w == 0 {
			w = winW
		}
		if !livePane(run, agent) {
			pane, err := splitBeside(run, console, accent, m.listWidth(w), w)
			if err != nil {
				msg.err = fmt.Errorf("the agent pane did not open: %w", err)
				return msg
			}
			msg.agent = pane
		}
		if _, err := run(append([]string{"respawn-pane", "-k", "-t", msg.agent, "--"}, a.steps[0]...)...); err != nil {
			msg.err = err
			return msg
		}
		// -Z keeps a zoom: a zoomed list hands it to the agent.
		if _, err := run("select-pane", "-Z", "-t", msg.agent); err != nil {
			msg.err = err
			return msg
		}
		if m.onePaneAt(w) {
			_, _ = run(remote.ZoomArgv(msg.agent, true)[1:]...)
		}
		return msg
	}
}

// besideShown takes the end of a choice: the agent pane stays current, and a
// view that did not start says why.
func (m Model) besideShown(msg shownMsg) (tea.Model, tea.Cmd) {
	if msg.agent != "" {
		m.beside = msg.agent
	}
	m.showing = false
	if next := m.showNext; next != nil { // this choice is not the latest: it says nothing
		m.showNext = nil
		return m.startShow(*next)
	}
	switch {
	case errors.Is(msg.err, errOwnWindow):
		m.flash = msg.name + " is in the console's own window: " + m.keys.main.Jump.Help().Key + " opens it"
		return m, nil
	case msg.err != nil:
		m.flash = msg.name + " is not shown: " + msg.err.Error()
		return m, nil
	}
	return m, m.tookOver(msg.host, msg.pane)
}
