package app

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
)

// flipAPC takes a flip, a walk or a jump the tmux binding hands the
// console from a marked pane ("<tty> <pane>", a jump's after its digit), or
// the phone's button sends at the console (""), which is ⌥s or ⌥n here. ok
// is false for any other APC.
func (m Model) flipAPC(apc string) (tea.Model, tea.Cmd, bool) {
	kind, arg, ok := keycmd.ParseAPC(apc)
	if !ok {
		return m, nil, false
	}
	if kind == "jump" {
		digit, from, _ := strings.Cut(arg, " ")
		tty, pane, _ := strings.Cut(from, " ")
		next, cmd := m.jumpFrom(int(digit[0]-'0'), tty, pane)
		return next, cmd, true
	}
	tty, pane, _ := strings.Cut(arg, " ")
	switch {
	case kind == "flip" && pane == "":
		next, cmd := m.flipHere()
		return next, cmd, true
	case kind == "flip":
		return m.flipFrom(tty, pane), nil, true
	case kind == "next" && pane == "":
		next, cmd := m.walkHere()
		return next, cmd, true
	case kind == "next":
		next, cmd := m.walkFrom(tty, pane)
		return next, cmd, true
	}
	return m, nil, true // open: the island's tap, which the phone does not send yet
}

// homeSession is the session of this machine in pane.
func (m Model) homeSession(pane string) (sample.Session, bool) {
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Host == m.opt.Host && s.Pane == pane })
	if i < 0 {
		return sample.Session{}, false
	}
	return m.all.sessions[i], true
}

// flipFrom opens the room of the session in pane, and keeps pane as where
// the client tty came from. A second client in the same room keeps its own.
func (m Model) flipFrom(tty, pane string) Model {
	s, ok := m.homeSession(pane)
	if !ok {
		m.sayError(m.keys.room.Flip.Help().Key + ": no session in " + pane)
		return m
	}
	k := m.roomKeyOf(s)
	if m.room == nil || m.room.k != k {
		m = m.openRoom(k)
	}
	// The room open already keeps its draft and the order on its way; the
	// cursor lands as in a new one: on the need, else on the talk line,
	// typing.
	r := *m.room
	r.from = maps.Clone(r.from)
	r.from[tty] = pane
	m.room = &r
	at := m.roomLanding(k) // on m itself: it chooses the need's card, and may unfold the plan
	r = *m.room
	r.at = at
	r.typing = at >= 0 && m.roomRows(k)[at].kind == roomTalk
	m.room = &r
	return m
}

// flipHere is ⌥s at the console: in a room it goes back to the session the
// client came from, or with none starts typing; elsewhere it opens the
// selected session's room.
func (m Model) flipHere() (tea.Model, tea.Cmd) {
	if m.room == nil {
		if s, ok := m.selectedSession(); ok && m.tab == tabSessions {
			return m.openRoom(m.roomKeyOf(s)), nil
		}
		return m, nil
	}
	r := *m.room
	m.room = &r
	tty, pane := m.cameFrom()
	if pane == "" {
		r.typing = true
		return m, nil
	}
	if _, ok := m.homeSession(pane); !ok {
		delete(r.from, tty)
		return m, m.say("That session ended.")
	}
	m.askedBy = tty
	return m.goInto(m.opt.Host, pane)
}

// cameFrom is the client that pressed and the pane it came from: the
// marked client's, else the one client the room knows.
func (m Model) cameFrom() (tty, pane string) {
	if p, ok := m.room.from[m.askedBy]; ok {
		return m.askedBy, p
	}
	if m.askedBy == "" && len(m.room.from) == 1 {
		for t, p := range m.room.from {
			return t, p
		}
	}
	return "", ""
}

// walkFrom is ⌥n in the match in pane: the chain's next match, and after
// the last its room, where the stoker is.
func (m Model) walkFrom(tty, pane string) (tea.Model, tea.Cmd) {
	s, ok := m.homeSession(pane)
	if !ok {
		said := m.sayError(m.keys.room.Walk.Help().Key + ": no session in " + pane)
		return m, said
	}
	seen, _ := m.members(m.roomKeyOf(s))
	i := slices.IndexFunc(seen, func(x sample.Session) bool { return x.Pane == pane })
	if i >= 0 && i+1 < len(seen) && seen[i+1].Host == m.opt.Host {
		m.askedBy = tty
		return m.goInto(m.opt.Host, seen[i+1].Pane)
	}
	return m.flipFrom(tty, pane), nil
}

// jumpFrom is ⌥ and digit n in the match in pane: the client on tty goes
// to session n's pane on this host. The phone has no ⌥, and tmux joined its
// Esc and digit: they go back to the pane as typed. A phone in an agent's
// pane no longer views the console, so its pairing tells it, read off the
// update loop; a pairing note that outlived its phone types a desk's ⌥n,
// and never moves a phone.
func (m Model) jumpFrom(n int, tty, pane string) (tea.Model, tea.Cmd) {
	run := m.opt.Run
	typeBack := []string{"tmux", "send-keys", "-t", pane, "Escape", strconv.Itoa(n)}
	if m.pocketClient(tty) {
		return m, func() tea.Msg { _ = run(typeBack); return nil }
	}
	if m.opt.Pair == nil || m.opt.Pair.DeviceTTYs == nil {
		return m.jumpFromDesk(jumpMsg{n, tty, pane})
	}
	phones := m.opt.Pair.DeviceTTYs
	return m, func() tea.Msg {
		if _, phone := phones()[tty]; phone {
			_ = run(typeBack)
			return nil
		}
		return jumpMsg{n, tty, pane}
	}
}

// jumpMsg is a jump from a pane whose client is no phone.
type jumpMsg struct {
	n         int
	tty, pane string
}

func (m Model) jumpFromDesk(j jumpMsg) (tea.Model, tea.Cmd) {
	run, n, tty := m.opt.Run, j.n, j.tty
	tmux := func(argv ...string) tea.Cmd {
		return func() tea.Msg { _ = run(append([]string{"tmux"}, argv...)); return nil }
	}
	s, no := m.numberedOn(n, m.opt.Host)
	if no != "" {
		return m, tmux("display-message", "-c", tty, no)
	}
	m.askedBy = tty
	a := m.jumpOn(m.opt.Host, s.Pane)
	a.inPane = true
	return m.runInto(m.forHost(m.opt.Host, a))
}

// walkHere is ⌥n at the console: in a room, its first match.
func (m Model) walkHere() (tea.Model, tea.Cmd) {
	if m.room == nil {
		return m, nil
	}
	seen, _ := m.members(m.room.k)
	if len(seen) == 0 || seen[0].Host != m.opt.Host {
		return m, nil
	}
	return m.goInto(m.opt.Host, seen[0].Pane)
}
