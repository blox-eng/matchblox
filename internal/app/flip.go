package app

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
)

// flipAPC takes a flip or a walk the tmux binding hands the console from a
// marked pane ("<tty> <pane>"), or the phone's button sends at the console
// (""), which is ⌥s or ⌥n here. ok is false for any other APC.
func (m Model) flipAPC(apc string) (tea.Model, tea.Cmd, bool) {
	kind, arg, ok := keycmd.ParseAPC(apc)
	if !ok {
		return m, nil, false
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
		m.flash = m.keys.room.Flip.Help().Key + ": no session in " + pane
		return m
	}
	k := m.roomKeyOf(s)
	from := map[string]string{}
	if m.room != nil && m.room.k == k {
		from = m.room.from
	}
	m = m.openRoom(k)
	m.room.from = from
	m.room.from[tty] = pane
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
		m.flash = m.keys.room.Walk.Help().Key + ": no session in " + pane
		return m, nil
	}
	seen, _ := m.members(m.roomKeyOf(s))
	i := slices.IndexFunc(seen, func(x sample.Session) bool { return x.Pane == pane })
	if i >= 0 && i+1 < len(seen) && seen[i+1].Host == m.opt.Host {
		m.askedBy = tty
		return m.goInto(m.opt.Host, seen[i+1].Pane)
	}
	return m.flipFrom(tty, pane), nil
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
