package app

import (
	"cmp"
	"encoding/json"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// The review has two halves. The review pane (reviewpane.go) runs in the
// agent seat as `matchblox frame --kind=review`: it reads the diff, holds
// the comments and takes the keys. At ⏎ it sends the one message itself,
// through the paste: act on its own conn to the service, to the pane it
// opened on. No comment, no file text and no paste goes into a tmux option:
// any process of the same user can read one. The pane tells the list only
// that the review is over ("leave") or was sent ("sent" and a count), through
// the window option @matchblox_review. The list owns the restore of the
// agent view, the mode and the keys.

// reviewSpec is what the list tells the review pane about the session it
// reviews: its name for the head, the host and pane the send goes to, the
// folder git runs in, and the branch it started from. host is "" for this
// machine.
type reviewSpec struct {
	name, host, pane, dir, base string
}

// reviewOn is the review the list opened in the agent seat. host and pane
// are what the review opened on: a word for any other host or pane is
// dropped.
type reviewOn struct{ host, pane, name string }

// reviewAct is the one word the review pane hands to the list: "leave", or
// "sent" with the count of comments it sent. It never holds review text.
type reviewAct struct {
	Act  string `json:"act"`
	Host string `json:"host,omitempty"`
	Pane string `json:"pane,omitempty"`
	N    int    `json:"n,omitempty"`
}

// reviewMsg is the end of taking the agent seat for a review.
type reviewMsg struct{ err error }

// reviewArgv is the agent seat's command: the same frame subcommand as the
// header and the bar.
func reviewArgv(self string, spec reviewSpec) []string {
	argv := append(frameArgv(self, "review"), "--session", spec.name, "--pane", spec.pane, "--dir", spec.dir)
	if spec.host != "" {
		argv = append(argv, "--host", spec.host)
	}
	if spec.base != "" {
		argv = append(argv, "--base", spec.base)
	}
	return argv
}

// askReview is D: the review of the chosen session takes the agent seat.
func (m Model) askReview() (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	run := m.tmuxRun()
	switch {
	case !ok && m.chosenSide().waits():
		return m, m.notice(m.noRowWhy())
	case !ok:
		return m, m.notice("no session to review · " + m.keys.main.New.Help().Key + " starts one")
	case m.chain != nil:
		return m, m.notice("review did not open: a chain view holds the agent pane · close the chain first")
	case s.Pane == "":
		return m, m.notice(noPane(s.Name))
	case m.beside == "" || run == nil:
		return m, m.notice("review did not open: there is no agent pane here. " + m.keys.main.Back.Help().Key + " back to the list")
	}
	host := cmp.Or(s.Host, m.opt.Host)
	if host != m.opt.Host && m.watchConn(host) == nil {
		return m, m.notice("review did not open: " + m.nameOf(host) + " is not reached. " + m.keys.main.Back.Help().Key + " back to the list")
	}
	if host != m.opt.Host {
		// The review sends through this machine's service only. A review the
		// owner cannot send is not opened: the words come before any comment.
		return m, m.notice("review did not open: " + m.nameOf(host) + " is another machine, and a review is sent from this machine only. " + m.keys.side.Enter.Help().Key + " goes into the session")
	}
	if r := m.reviewing; r != nil && r.host == host && r.pane == s.Pane {
		return m, m.reviewFocus() // already open: the keys go back to it
	}
	spec := reviewSpec{name: m.cardName(s), pane: s.Pane, dir: s.Cwd, base: m.startBranch(s)}
	if host != m.opt.Host {
		spec.host = host
	}
	m.reviewing = &reviewOn{host: host, pane: s.Pane, name: spec.name}
	argv, agent, console := reviewArgv(m.opt.Self, spec), m.beside, m.opt.Pane
	return m, func() tea.Msg {
		if _, err := run(append([]string{"respawn-pane", "-k", "-t", agent, "--"}, argv...)...); err != nil {
			return reviewMsg{err}
		}
		writeMode(run, console, "REVIEW")
		_, err := run("select-pane", "-Z", "-t", agent)
		return reviewMsg{err}
	}
}

// startBranch is the branch s started from, as its host's service
// recorded it: this machine's, else the peer's.
func (m Model) startBranch(s sample.Session) string {
	if s.Host != "" && s.Host != m.opt.Host {
		if i := m.peerAt(s.Host); i >= 0 {
			return m.peers[i].st.StartBranch[s.Name]
		}
		return ""
	}
	return m.homeStart[s.Name]
}

// reviewFocus gives the keys back to the review pane.
func (m Model) reviewFocus() tea.Cmd {
	run, agent := m.tmuxRun(), m.beside
	return func() tea.Msg {
		_, _ = run("select-pane", "-Z", "-t", agent)
		return nil
	}
}

// reviewOpened: the seat runs the review, or says why it does not.
func (m Model) reviewOpened(msg reviewMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil {
		m.besideShows = paneKey{} // the seat no longer shows an agent
		return m, nil
	}
	if m.reviewing == nil {
		return m, nil
	}
	m.reviewing = nil
	said := m.sayError("review did not open: " + msg.err.Error() + ". " + m.keys.main.Back.Help().Key + " back to the list")
	return m, said
}

// takeReview is the list's look at the word the review pane wrote. It
// clears the option at once.
func (m Model) takeReview(raw string) (Model, tea.Cmd) {
	clear := m.clearOption(sample.ReviewOption)
	var a reviewAct
	r := m.reviewing
	if r == nil || json.Unmarshal([]byte(raw), &a) != nil {
		return m, clear
	}
	switch a.Act {
	case "leave":
		next, end := m.endReview()
		return next, tea.Batch(clear, end)
	case "sent":
		// The review sent through its own conn. The list gives the keys, says
		// so, and ends the review: for the review it opened, never another.
		if cmp.Or(a.Host, m.opt.Host) != r.host || a.Pane != r.pane || a.N < 1 {
			return m, clear
		}
		var keys tea.Cmd
		for _, s := range m.all.sessions {
			if cmp.Or(s.Host, m.opt.Host) == r.host && s.Pane == r.pane {
				keys = m.giveKeys(s) // a send counts as giving the keys: Task 3 leaves DONE on it
			}
		}
		said := m.say("sent a review of " + plural(a.N, "comment") + " to " + r.name + " · now in WORKING")
		next, end := m.endReview()
		return next, tea.Batch(clear, keys, end, said)
	}
	return m, clear
}

// endReview gives the agent seat its agent view back, the list its mode and
// its keys.
func (m Model) endReview() (Model, tea.Cmd) {
	r := m.reviewing
	if r == nil {
		return m, nil
	}
	m.reviewing, m.besideShows = nil, paneKey{}
	var show tea.Cmd
	if next, cmd := m.viewsOn(&action{label: "show " + r.name, beside: true, preview: true, pane: r.pane, host: r.host}, r.name, []string{r.pane}); next != nil {
		if nm, ok := next.(Model); ok {
			m = nm
		}
		show = cmd
	}
	run, console := m.tmuxRun(), m.opt.Pane
	back := func() tea.Msg {
		if run == nil {
			return nil
		}
		writeMode(run, console, "LIST")
		_, _ = run("select-pane", "-Z", "-t", console)
		return nil
	}
	return m, tea.Batch(show, back)
}

// clearOption empties a window option of the console.
func (m Model) clearOption(name string) tea.Cmd {
	run, console := m.tmuxRun(), m.opt.Pane
	if run == nil {
		return nil
	}
	return func() tea.Msg {
		_, _ = run("set-option", "-wu", "-t", console, name)
		return nil
	}
}
