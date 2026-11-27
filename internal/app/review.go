package app

import (
	"cmp"
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The review has two halves. The review pane (reviewpane.go) runs in the
// agent seat as `matchblox frame --kind=review`: it reads the diff, holds
// the comments and takes the keys. It never sends to a session. At ⏎ or esc
// it writes one act to the window option @matchblox_review, and the list
// takes it from its quarter-second look: the list owns the paste, the
// restore of the agent view and the keys. A send the list refuses comes
// back to the pane through @matchblox_review_reply.

// reviewSpec is what the list tells the review pane about the session it
// reviews: its name for the head, the host and pane the send goes to, the
// folder git runs in, and the branch it started from. host is "" for this
// machine.
type reviewSpec struct {
	name, host, pane, dir, base string
}

// reviewOn is the review the list opened in the agent seat. host and pane
// are what the review opened on: the paste goes there, and an act for any
// other host or pane is dropped. actID is the paste in flight.
type reviewOn struct {
	host, pane, name string
	actID            string
	n                int
}

// reviewAct is the one act the review pane hands to the list.
type reviewAct struct {
	Act  string `json:"act"` // "send" or "leave"
	Host string `json:"host,omitempty"`
	Pane string `json:"pane,omitempty"`
	Name string `json:"name,omitempty"`
	N    int    `json:"n,omitempty"`
	Text string `json:"text,omitempty"`
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

// takeReview is the list's look at the act the review pane wrote. It
// clears the option at once: the act holds the review's text.
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
	case "send":
		// The target is what the review opened on, never the list's choice
		// at this moment.
		if cmp.Or(a.Host, m.opt.Host) != r.host || a.Pane != r.pane || a.N < 1 || strings.TrimSpace(a.Text) == "" {
			return m, clear
		}
		if m.watchConn(r.host) == nil {
			return m, tea.Batch(clear, m.replyReview(m.nameOf(r.host)+" is not reached"))
		}
		id, send := m.sendToID(r.host, proto.Act{RecID: proto.PastePrefix + r.pane, Text: a.Text, Confirm: "y"})
		if id == "" {
			return m, tea.Batch(clear, m.replyReview(m.nameOf(r.host)+" is not reached"))
		}
		nr := *r
		nr.actID, nr.n = id, a.N
		m.reviewing = &nr
		return m, tea.Batch(clear, send)
	}
	return m, clear
}

// reviewResult is the service's answer to the review's paste. A refusal
// goes back to the review pane, which keeps its comments. An answer that
// is not a refusal counts as giving the keys, and the review ends.
func (m Model) reviewResult(r proto.Result) (Model, tea.Cmd, bool) {
	rv := m.reviewing
	if rv == nil || rv.actID == "" || r.ActID != rv.actID {
		return m, nil, false
	}
	if why := cmp.Or(r.Err, last(r.Skipped)); why != "" {
		nr := *rv
		nr.actID = ""
		m.reviewing = &nr
		return m, m.replyReview(why), true
	}
	var keys tea.Cmd
	for _, s := range m.all.sessions {
		if cmp.Or(s.Host, m.opt.Host) == rv.host && s.Pane == rv.pane {
			keys = m.giveKeys(s) // a send counts as giving the keys: Task 3 leaves DONE on it
		}
	}
	said := m.say("sent a review of " + plural(rv.n, "comment") + " to " + rv.name + " · now in WORKING")
	next, end := m.endReview()
	return next, tea.Batch(keys, end, said), true
}

func last(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
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

// replyReview tells the review pane that a send did not go, and why.
func (m Model) replyReview(why string) tea.Cmd {
	run, console := m.tmuxRun(), m.opt.Pane
	if run == nil {
		return nil
	}
	return func() tea.Msg {
		_, _ = run("set-option", "-w", "-t", console, sample.ReviewReplyOption, why)
		return nil
	}
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
