package app

import (
	"cmp"
	"errors"
	"fmt"
	"image/color"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// besideSizeMsg is the window's size, as the window changed.
type besideSizeMsg struct{ w, h int }

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
		w, _ := windowSize(run, console)
		pane, err := splitBeside(run, console, accent, listWide, w)
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

// windowSize is the console window's width and height; 0 for what tmux
// does not say.
func windowSize(run func(args ...string) (string, error), pane string) (w, h int) {
	out, err := run("display", "-p", "-t", pane, "#{window_width} #{window_height}")
	if err != nil {
		return 0, 0
	}
	f := strings.Fields(out)
	if len(f) > 0 {
		w, _ = strconv.Atoi(f[0])
	}
	if len(f) > 1 {
		h, _ = strconv.Atoi(f[1])
	}
	return w, h
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
		w, h := windowSize(run, console)
		if w == 0 {
			w = paneW
		}
		if m.boxZoom || m.lendsWindow() && !m.onePaneAt(w, h) { // the box or the animation keeps the window
			return besideSizeMsg{w, h}
		}
		if m.onePaneAt(w, h) {
			_, _ = run(remote.ZoomArgv(inUse(run, console), true)[1:]...)
			return besideSizeMsg{w, h}
		}
		// A resize ends a zoom, so it runs only where the window is not one.
		_, _ = run(remote.ZoomArgv(console, false)[1:]...)
		_, _ = run(remote.ListWidthArgv(console, m.listWidth(w, h))[1:]...)
		return besideSizeMsg{w, h}
	}
}

// boxOpen: a box asks over the screen, centred: a consent, the launch
// sheet, a form or the ssh prompt. The row menu is not one: it stays by
// its row.
func (m Model) boxOpen() bool {
	return m.consenting() || m.sheet != nil || m.form != nil || m.asking()
}

// lendsWindow: the list's pane takes the whole window, so that what it
// shows centres in the window, not in the sidebar: a box, the start
// screen, or the pairing page's knock and opening.
func (m Model) lendsWindow() bool {
	return m.boxOpen() || m.splashing() || m.pair.owns()
}

// boxBackMsg is the agent pane's screen as the box opened.
type boxBackMsg []string

// boxZoomed lends the window to a box, or to an animation, beside the
// agent: the list's pane zooms, and the box centres over the whole window,
// not in the list's columns. The agent's screen shows dim beside the list
// behind a box. As the box closes, the window goes back to both panes.
func (m Model) boxZoomed() (Model, tea.Cmd) {
	run := m.tmuxRun()
	if m.beside == "" || run == nil || m.lendsWindow() == m.boxZoom {
		return m, nil
	}
	if !m.boxZoom && m.winW > 0 && m.onePaneAt(m.winW, m.winH) { // one pane has the window already
		return m, nil
	}
	m.boxZoom = !m.boxZoom
	if !m.boxZoom {
		m.boxBack = nil
		return m, m.sizeCmd(m.width)
	}
	console, agent := m.opt.Pane, m.beside
	return m, func() tea.Msg {
		out, _ := run(remote.CaptureArgv(agent)[1:]...)
		_, _ = run(remote.ZoomArgv(console, true)[1:]...)
		return boxBackMsg(strings.Split(out, "\n"))
	}
}

// agentBack is the agent's screen behind a box, w by h from the body's
// first row, dim.
func (m Model) agentBack(w, h int) []string {
	out := make([]string, h)
	for i := range out {
		l := ""
		if r := i + 3; r < len(m.boxBack) { // the body starts under the header, the tabs and a hairline
			l = ansi.Truncate(ansi.Strip(m.boxBack[r]), w, "")
		}
		out[i] = m.st.faint.Render(l + strings.Repeat(" ", w-lipgloss.Width(l)))
	}
	return out
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

// listWidth is the list pane's width in a window of w by h.
func (m Model) listWidth(w, h int) int {
	if layout(w, m.windowRows(h)) == Wide {
		return listWide
	}
	return listMedium
}

// onePaneAt: a window of w by h shows one pane at a time. The window's shape
// decides, never the list pane's: beside the list that pane is a narrow
// part of the window, upright on any screen, and a zoom would change it.
func (m Model) onePaneAt(w, h int) bool {
	rows := m.windowRows(h)
	return w-listMedium-1 < chatMin || m.onPocket() && (portrait(w, rows) || layout(w, rows) == Narrow)
}

// windowRows is h, or the list pane's rows where tmux did not say: the two
// are as tall, side by side.
func (m Model) windowRows(h int) int {
	if h > 0 {
		return h
	}
	return m.shapeRows()
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
	chain      *chainView // the chain view it opened
	chainOff   bool       // it closed the window's chain view
	skipped    []string   // the members a chain view left out: in the console's own window
	preview    bool       // a look: the keys stayed with the list
	focused    bool       // ⏎ on the session shown: no view started
	ended      bool       // focused, but the view had ended: it shows again
}

// showChosen shows the session the list chooses in the agent pane.
func (m Model) showChosen() (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	switch {
	case ok && m.chainPlace(s) >= 0:
		return m, m.selectPane(m.chain.panes[m.chainPlace(s)])
	case !ok:
		return m, m.notice("no session to show · " + m.keys.main.New.Help().Key + " starts one")
	case s.Pane == "":
		said := m.sayError(noPane(s.Name))
		return m, said
	case m.besideShows == m.shownKey(s.Host, s.Pane) && !m.showing:
		name := m.besideName(m.besideShows.host, s.Pane)
		return m.startShow(action{label: "show " + name, beside: true, focus: true, pane: s.Pane, host: m.besideShows.host})
	}
	return m.showBeside(s.Host, s.Pane)
}

// shownKey is the key m.besideShows has for host's pane.
func (m Model) shownKey(host, pane string) paneKey {
	return paneKey{cmp.Or(host, m.opt.Host), pane}
}

// previewRest is how long the list rests on a row before the agent pane
// shows it: a run of ↑↓ shows only the row it stops on.
var previewRest = 150 * time.Millisecond

// previewMsg is the end of a rest after the list's move seq.
type previewMsg struct{ seq int }

// previewSoon shows the chosen session once the list rests.
func (m *Model) previewSoon() tea.Cmd {
	if !m.besideMode() {
		return nil
	}
	m.previewSeq++
	seq := m.previewSeq
	return tea.Tick(previewRest, func(time.Time) tea.Msg { return previewMsg{seq} })
}

// look shows the session the list rests on in the agent pane, and the
// keys stay with the list. A look never asks: a host whose stoker asks to
// take over, and a peer the builder did not yet let a view run on, show it
// on ⏎ only. One pane and a chain view show no look either.
func (m Model) look(msg previewMsg) (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	host := cmp.Or(s.Host, m.opt.Host)
	if msg.seq != m.previewSeq || !ok || s.Pane == "" || m.chain != nil || !m.besideMode() ||
		m.onePaneAt(m.winW, m.winH) || m.besideShows == m.shownKey(host, s.Pane) || m.takeOverOn(host) ||
		host != "" && !m.viewed[host] {
		return m, nil
	}
	name := m.besideName(host, s.Pane)
	return m.viewsOn(&action{label: "show " + name, beside: true, preview: true, pane: s.Pane, host: host}, name, []string{s.Pane})
}

// viewing: pane runs a view still, its tmux client or its ssh; an ended
// view waits in its shell for a key.
func viewing(run func(args ...string) (string, error), pane string) bool {
	out, err := run("display", "-p", "-t", pane, "#{pane_current_command}")
	cmd := strings.TrimSpace(out)
	return err == nil && (cmd == "tmux" || cmd == "ssh")
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
	return m.viewsOn(&action{label: "show " + name, beside: true, pane: pane, host: host}, name, []string{pane})
}

// viewsOn runs a with a view of each of a.host's panes, what naming them
// in a refusal: this machine's at once, a peer's after one consent box for
// all of them.
func (m Model) viewsOn(a *action, what string, panes []string) (tea.Model, tea.Cmd) {
	host := a.host
	if host != "" {
		b, steps, says := a, [][]string(nil), []string(nil)
		for _, pane := range panes {
			_, ssh, next := m.peerJump(remote.Jump{Host: host, Pane: pane, View: true}, a)
			if ssh == nil && a.preview {
				return m, nil
			}
			if ssh == nil {
				return m.runInto(next)
			}
			b = next
			steps, says = append(steps, remote.PeerViewArgv(m.besideName(host, pane), ssh)), append(says, shellLine(ssh))
		}
		b.steps, b.say, b.nav, b.remote = steps, strings.Join(says, "\n"), true, true
		if a.preview {
			return m.startShow(*b)
		}
		return m.runInto(b)
	}
	if m.opt.Socket == "" {
		said := m.sayError(what + " is not shown: the console does not know its tmux socket ($TMUX names none); " + m.keys.main.Jump.Help().Key + " opens it")
		return m, said
	}
	for _, pane := range panes {
		name := m.besideName(host, pane)
		j := remote.Jump{Pane: pane, Prefix: m.prefix(), ID: m.jumpID(), Outer: true}
		if !remote.ViewAllowed(j, name, m.opt.Socket) {
			said := m.sayError("refused: " + name + " is not shown: its pane, its name or the tmux socket " + m.opt.Socket + " is not one a view takes")
			return m, said
		}
		a.steps = append(a.steps, remote.ViewArgv(j, name, m.opt.Socket))
	}
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
// Where one pane shows, the agent's is zoomed. A chain view it replaces
// closes first; a chain's views split the agent pane.
func (m Model) besideRun(a action) tea.Cmd {
	run := m.tmuxRun()
	if a.focus && run != nil {
		agent := m.beside
		return func() tea.Msg {
			msg := shownMsg{agent: agent, host: a.host, pane: a.pane, name: m.besideName(a.host, a.pane), focused: true}
			if !viewing(run, agent) {
				msg.ended = true
				return msg
			}
			if _, err := run("select-pane", "-Z", "-t", agent); err != nil {
				msg.err = err
			}
			return msg
		}
	}
	if run == nil || !a.closing && (len(a.steps) == 0 || len(a.steps) > 1 && len(a.chain) != len(a.steps)) {
		return nil
	}
	console, accent, agent, winW, winH, name := m.opt.Pane, hexOf(m.st.theme.Accent), m.beside, m.winW, m.winH, m.besideName(a.host, a.pane)
	names := make([]string, len(a.chain))
	for i, k := range a.chain {
		names[i] = m.besideName(a.host, k.pane)
	}
	return func() tea.Msg {
		msg := shownMsg{agent: agent, host: a.host, pane: a.pane, name: name, preview: a.preview}
		if a.closing {
			endChain(run, console, cmp.Or(a.keep, agent))
			msg.chainOff = true
			if a.keep != "" && a.keep != agent {
				if _, err := run(remote.BesideArgv(console, a.keep, accent)[1:]...); err != nil {
					msg.err = err
					return msg
				}
				msg.agent = a.keep
			}
			return msg
		}
		steps, on := a.steps, a.chain
		if !a.remote && len(on) > 0 {
			steps, on = nil, nil
			for i, k := range a.chain {
				if sameWindow(run, console, k.pane) {
					msg.skipped = append(msg.skipped, names[i])
					continue
				}
				steps, on = append(steps, a.steps[i]), append(on, k)
			}
			if len(on) == 0 {
				msg.err = errOwnWindow
				return msg
			}
		} else if !a.remote && sameWindow(run, console, a.pane) {
			msg.err = errOwnWindow
			return msg
		}
		w, h := windowSize(run, console)
		if w == 0 {
			w, h = winW, winH
		}
		if !livePane(run, agent) {
			pane, err := splitBeside(run, console, accent, m.listWidth(w, h), w)
			if err != nil {
				msg.err = fmt.Errorf("the agent pane did not open: %w", err)
				return msg
			}
			msg.agent = pane
		}
		msg.chainOff = endChain(run, console, msg.agent)
		if len(on) > 0 {
			chain, err := showChain(run, msg.agent, steps, on)
			if err != nil {
				msg.chainOff = endChain(run, console, msg.agent) || msg.chainOff
				msg.err = err
				return msg
			}
			msg.chain = chain
		} else if _, err := run(append([]string{"respawn-pane", "-k", "-t", msg.agent, "--"}, steps[0]...)...); err != nil {
			msg.err = err
			return msg
		}
		if a.preview {
			return msg
		}
		// -Z keeps a zoom: a zoomed list hands it to the agent.
		if _, err := run("select-pane", "-Z", "-t", msg.agent); err != nil {
			msg.err = err
			return msg
		}
		if m.onePaneAt(w, h) {
			_, _ = run(remote.ZoomArgv(msg.agent, true)[1:]...)
		}
		return msg
	}
}

// showChain runs the chain's views in agent and the panes split from it,
// one member of on each.
func showChain(run func(args ...string) (string, error), agent string, views [][]string, on []paneKey) (*chainView, error) {
	out, err := run(remote.ChainArgv(agent, views)[1:]...)
	if err != nil {
		return nil, err
	}
	c := &chainView{on: on, panes: make([]string, len(on))}
	c.panes[0] = agent
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		i, pane, _ := strings.Cut(l, " ")
		if n, err := strconv.Atoi(i); err == nil && n > 0 && n < len(on) && paneID.MatchString(pane) {
			c.panes[n] = pane
		}
	}
	if slices.Contains(c.panes, "") {
		return nil, fmt.Errorf("tmux answered %q", out)
	}
	return c, nil
}

// endChain kills the panes of the window's chain view but keep, and
// forgets the view. It tells whether there was one.
func endChain(run func(args ...string) (string, error), console, keep string) bool {
	out, err := run("show-option", "-wqv", "-t", console, sample.ChainOption)
	panes := strings.Fields(out)
	if err != nil || len(panes) == 0 {
		return false
	}
	for _, p := range panes {
		if p != keep && paneID.MatchString(p) {
			_, _ = run("kill-pane", "-t", p)
		}
	}
	_, _ = run("set-option", "-wu", "-t", console, sample.ChainOption)
	return true
}

// besideShown takes the end of a choice: the agent pane stays current, and a
// view that did not start says why.
func (m Model) besideShown(msg shownMsg) (tea.Model, tea.Cmd) {
	if msg.agent != "" {
		m.beside = msg.agent
	}
	if msg.chainOff {
		m.chain = nil
	}
	if msg.chain != nil {
		m.chain = msg.chain
	}
	m.showing = false
	switch {
	case msg.ended:
		m.besideShows = paneKey{}
	case msg.focused:
	case msg.err != nil || msg.chainOff && msg.chain == nil:
		m.besideShows = paneKey{}
	case msg.chain != nil:
		m.besideShows = paneKey{}
	default:
		m.besideShows = m.shownKey(msg.host, msg.pane)
	}
	if next := m.showNext; next != nil { // this choice is not the latest: it says nothing
		m.showNext = nil
		return m.startShow(*next)
	}
	if msg.ended {
		return m.showBeside(msg.host, msg.pane)
	}
	switch {
	case errors.Is(msg.err, errOwnWindow):
		said := m.say(msg.name + " is in the console's own window: " + m.keys.main.Jump.Help().Key + " opens it")
		return m, said
	case msg.err != nil:
		said := m.sayError(msg.name + " is not shown: " + msg.err.Error())
		return m, said
	case len(msg.skipped) > 0:
		m.sayError("not shown, in the console's own window: " + strings.Join(msg.skipped, ", "))
	}
	if msg.preview {
		return m, nil
	}
	if msg.chain == nil {
		return m, m.tookOver(msg.host, msg.pane)
	}
	var took []tea.Cmd
	for _, k := range msg.chain.on {
		took = append(took, m.tookOver(msg.host, k.pane))
	}
	return m, tea.Batch(took...)
}

// chainMost is the most members a chain view shows: 4 leave each agent
// 60 × 15 or more on a 16-inch laptop.
const chainMost = 4

// chainView is a chain's members on screen beside the list: on[i] shows in
// panes[i], and panes[0] is the agent pane.
type chainView struct {
	on    []paneKey
	panes []string
}

// chainPlace is the place of s in the chain view; -1 where it is not on
// screen.
func (m Model) chainPlace(s sample.Session) int {
	if m.chain == nil || !m.besideMode() {
		return -1
	}
	return slices.Index(m.chain.on, paneOf(s))
}

// chainToggle is g beside the list: it opens the chosen row's chain view,
// or closes the one open.
func (m Model) chainToggle(s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	if m.chain != nil {
		return m.closeChainBeside()
	}
	k, in := m.chainKeyOf(s)
	switch {
	case !ok:
		return m, m.notice(m.noRowWhy())
	case !in || k.isOne():
		return m, m.notice(s.Name + " is in no chain")
	}
	return m.chainBeside(k)
}

// chainBeside shows up to chainMost members of chain k that run in tmux,
// in the list's order, one pane each beside the list.
func (m Model) chainBeside(k chainKey) (tea.Model, tea.Cmd) {
	at := map[paneKey]sample.Session{}
	for _, s := range m.sideSessions() {
		at[paneOf(s)] = s
	}
	var on []paneKey
	for _, p := range m.sideOrder() {
		s, ok := at[p]
		if in, member := m.chainKeyOf(s); ok && member && in == k && s.Pane != "" && len(on) < chainMost {
			on = append(on, p)
		}
	}
	name := m.chainLabel(k)
	if len(on) == 0 {
		said := m.sayError(name + " is not shown: no member runs in tmux")
		return m, said
	}
	if h := cmp.Or(k.host, m.opt.Host); m.takeOverOn(h) {
		m.held, m.heldOn, m.flash = func(m Model) (tea.Model, tea.Cmd) { return m.chainOn(k.host, name, on) }, h, ""
		return m, nil
	}
	return m.chainOn(k.host, name, on)
}

// chainOn runs the chain view of host's members on.
func (m Model) chainOn(host, name string, on []paneKey) (tea.Model, tea.Cmd) {
	host = cmp.Or(host, m.opt.Host)
	panes := make([]string, len(on))
	for i, k := range on {
		panes[i] = k.pane
	}
	return m.viewsOn(&action{label: "show " + name, beside: true, pane: panes[0], host: host, chain: on}, name, panes)
}

// closeChainBeside closes the chain view: one agent pane shows again, the
// chosen row's. A member on screen keeps its pane; another row shows as ⏎
// shows it.
func (m Model) closeChainBeside() (tea.Model, tea.Cmd) {
	a := action{label: "close the chain view", beside: true, closing: true}
	if s, ok := m.sideChosen(); ok {
		switch i := m.chainPlace(s); {
		case i > 0:
			a.keep = m.chain.panes[i]
		case i < 0 && s.Pane != "":
			return m.showChosen()
		}
	}
	return m.startShow(a)
}

// selectPane gives pane the keys.
func (m Model) selectPane(pane string) tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	return func() tea.Msg {
		_, _ = run("select-pane", "-Z", "-t", pane)
		return nil
	}
}

// endBeside ends the agent pane and a chain view's with the console, so the
// next matchblox attaches to the list, not to an agent with none. tmux's
// hook (panes.BesideHookArgv) does the same when the console crashes.
func endBeside(o Options) {
	run := Model{opt: o}.tmuxRun()
	if !o.Beside || run == nil {
		return
	}
	agent := keptPane(run, o.Pane)
	endChain(run, o.Pane, agent)
	if agent != "" {
		_, _ = run("kill-pane", "-t", agent)
	}
	for _, opt := range []string{sample.BesideOption, sample.ConsoleOption} {
		_, _ = run("set-option", "-wu", "-t", o.Pane, opt)
	}
}
