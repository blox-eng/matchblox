package app

import (
	"cmp"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The desk shows the session the list chooses in its agent region: the
// view's nested tmux client, run in a pty whose screen matchblox draws
// (agentterm.go). The console's tmux window holds the console alone.

// shownMsg is a choice checked off the update loop: the view to run for
// the session, or why it does not run.
type shownMsg struct {
	host, pane, name string
	argv             []string // the view's argv (remote.ViewArgv, remote.PeerViewArgv)
	err              error
	preview          bool // a look: the keys stay with the list
}

var paneID = regexp.MustCompile(`^%\d+$`)

// migratedMsg: this start took down the split of an older desk.
type migratedMsg struct{}

// migrateSplit takes down what an older desk left on the console's tmux
// (design review D9): the viewer pane beside this console and a chain
// view's panes, the window's options that named them, the global hooks
// that ended them, and tmux's ctrl+q, which would never let the key reach
// matchblox. A pane, a hook or a ctrl+q of the owner's own stays, and the
// agent's own session is never touched. It says so once, only where it
// took something down.
func (m Model) migrateSplit() tea.Cmd {
	run := m.tmuxRun()
	if !m.opt.Beside || run == nil {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		moved := false
		if out, err := run("show-option", "-wqv", "-t", console, sample.BesideOption); err == nil && strings.TrimSpace(out) != "" {
			moved = true
			panes := strings.Fields(out)
			if chain, err := run("show-option", "-wqv", "-t", console, sample.ChainOption); err == nil {
				panes = append(panes, strings.Fields(chain)...)
			}
			for _, p := range panes {
				if p != console && paneID.MatchString(p) {
					_, _ = run("kill-pane", "-t", p)
				}
			}
			for _, opt := range []string{sample.BesideOption, sample.ChainOption, sample.ConsoleOption} {
				_, _ = run("set-option", "-wu", "-t", console, opt)
			}
		}
		for _, hook := range []string{"pane-exited", "pane-died"} { // the split's own end, set globally
			at := hook + "[" + strconv.Itoa(keycmd.MarkSlot) + "]"
			if out, err := run("show-hooks", "-g", at); err == nil && strings.Contains(out, sample.BesideOption) {
				_, _ = run("set-hook", "-gu", at)
			}
		}
		if keys, err := run("list-keys", "-T", "root", "C-q"); err == nil && strings.Contains(keys, sample.BesideOption) {
			moved = true
			_, _ = run("unbind-key", "-n", "C-q")
		}
		if !moved {
			return nil
		}
		return migratedMsg{}
	}
}

// besideMode: the desk shows the chosen agent's own screen.
func (m Model) besideMode() bool {
	return m.opt.Beside && m.tmuxRun() != nil
}

// placeAgent puts the agent region where the desk's layout has it, beside
// the list under the head row's rule, and fits the agent's screen to it.
func (m *Model) placeAgent() {
	if !m.desk() {
		m.over = "" // off the desk no sheet draws
		return
	}
	box := m.sideBoxAt(max(m.width, minWidth))
	m.agent.x, m.agent.y = box.chatX, 2
	if w, h := box.chatW, m.bodyRows(); w > 0 && (w != m.agent.w || h != m.agent.h) {
		m.agent.resize(w, h)
	}
}

// showChosen shows the session the list chooses in the agent region, and
// gives it the keys.
func (m Model) showChosen() (tea.Model, tea.Cmd) {
	seen := m.grantKeys()
	next, cmd := m.showChosenPane()
	return next, tea.Batch(seen, cmd)
}

func (m Model) showChosenPane() (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	switch {
	case !ok && m.chosenSide().waits():
		return m, m.notice(m.noRowWhy())
	case !ok:
		return m, m.notice("no session to show · " + m.keys.main.New.Help().Key + " starts one")
	case s.Pane == "":
		said := m.sayError(noPane(s.Name))
		return m, said
	case m.besideShows == m.shownKey(s.Host, s.Pane) && !m.showing && m.agent.live():
		m.agent.keys = true
		return m, nil
	}
	return m.showBeside(s.Host, s.Pane)
}

// shownKey is the key m.besideShows has for host's pane.
func (m Model) shownKey(host, pane string) paneKey {
	return paneKey{cmp.Or(host, m.opt.Host), pane}
}

// previewRest is how long the list rests on a row before the agent region
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

// look shows the session the list rests on in the agent region, and the
// keys stay with the list. A look never asks: a host whose stoker asks to
// take over, and a peer the builder did not yet let a view run on, show it
// on ⏎ only.
func (m Model) look(msg previewMsg) (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	host := cmp.Or(s.Host, m.opt.Host)
	if msg.seq != m.previewSeq || !ok || s.Pane == "" || !m.besideMode() ||
		m.besideShows == m.shownKey(host, s.Pane) || m.takeOverOn(host) || host != "" && !m.viewed[host] {
		return m, nil
	}
	name := m.besideName(host, s.Pane)
	return m.viewsOn(&action{label: "show " + name, beside: true, preview: true, pane: s.Pane, host: host}, name, []string{s.Pane})
}

// showBeside is goIntoAsking for the agent region: on a host the stoker
// drives the take-over question comes first, and a peer's consent box.
func (m Model) showBeside(host, pane string) (tea.Model, tea.Cmd) {
	if h := cmp.Or(host, m.opt.Host); m.takeOverOn(h) {
		m.held, m.heldOn, m.flash = func(m Model) (tea.Model, tea.Cmd) { return m.besideOn(host, pane) }, h, ""
		return m, nil
	}
	return m.besideOn(host, pane)
}

// besideOn runs the view of host's pane in the agent region: this
// machine's at once, a peer's after its consent box.
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
		b, steps := a, [][]string(nil)
		for _, pane := range panes {
			_, ssh, next := m.peerJump(remote.Jump{Host: host, Pane: pane, View: true}, a)
			if ssh == nil && a.preview {
				return m, nil
			}
			if ssh == nil {
				return m.runInto(next)
			}
			b = next
			steps = append(steps, remote.PeerViewArgv(m.besideName(host, pane), ssh))
		}
		// In words: a view's ssh line holds every key it binds on the host.
		b.steps, b.say, b.nav, b.remote = steps, "Show "+what+" from "+host+": ssh "+host+" shows it beside the list", true, true
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

// startShow runs a choice in the agent region, one at a time: a choice
// made while one is checked waits, and a later one takes its place.
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

// besideRun checks a's view off the update loop: a pane of this machine in
// the console's own window would show the console inside itself.
func (m Model) besideRun(a action) tea.Cmd {
	if len(a.steps) == 0 {
		return nil
	}
	run, console := m.tmuxRun(), m.opt.Pane
	msg := shownMsg{host: a.host, pane: a.pane, name: m.besideName(a.host, a.pane), argv: a.steps[0], preview: a.preview}
	return func() tea.Msg {
		if !a.remote && run != nil && sameWindow(run, console, a.pane) {
			msg.err = errOwnWindow
		}
		return msg
	}
}

// besideShown takes the end of a choice: the agent region opens the view,
// and the keys go to it but on a look; a view that cannot run says why.
func (m Model) besideShown(msg shownMsg) (tea.Model, tea.Cmd) {
	m.showing = false
	if next := m.showNext; next != nil { // this choice is not the latest: it says nothing
		m.showNext = nil
		return m.startShow(*next)
	}
	switch {
	case errors.Is(msg.err, errOwnWindow):
		said := m.say(msg.name + " is in the console's own window: " + m.keys.main.Jump.Help().Key + " opens it")
		return m, said
	case msg.err != nil:
		said := m.sayError(msg.name + " is not shown: " + msg.err.Error())
		return m, said
	}
	box := m.sideBoxAt(max(m.width, minWidth))
	open := m.agent.open(msg.name, msg.argv, box.chatW, m.bodyRows())
	m.placeAgent()
	m.besideShows = m.shownKey(msg.host, msg.pane)
	if msg.preview {
		return m, open
	}
	m.agent.keys = true // they reach the agent once its first frame shows (design review D6)
	return m, tea.Batch(open, m.tookOver(msg.host, msg.pane))
}
