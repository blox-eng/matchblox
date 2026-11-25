package app

import (
	"slices"

	"github.com/blox-eng/matchblox/internal/sample"
)

// space is a repo with a live session: its name, and its sessions that
// need the owner, counted as needCount counts.
type space struct {
	name string
	need int
}

// spaces are the repos the sessions of the hosts in view work in, in the
// order the list meets them. With one repo there is none: the list is that
// repo.
func (m Model) spaces() []space {
	var out []space
	for _, s := range m.sideSessions() {
		if !m.inView(s.Host) {
			continue
		}
		name := m.repoOf(s)
		i := slices.IndexFunc(out, func(sp space) bool { return sp.name == name })
		if i < 0 {
			i, out = len(out), append(out, space{name: name})
		}
		if m.counts(s) && m.sideStateOf(s) == sideNeed {
			out[i].need++
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// inSpace: s works in the open space, or no space is open.
func (m Model) inSpace(s sample.Session) bool { return m.space == "" || m.repoOf(s) == m.space }

// turnSpace opens the space by places after the open one, through all.
func (m *Model) turnSpace(by int) string {
	stops := []string{""}
	for _, sp := range m.spaces() {
		stops = append(stops, sp.name)
	}
	if len(stops) == 1 {
		return "every session works in one repo"
	}
	at := max(slices.Index(stops, m.space), 0) // a space whose sessions ended turns from all
	m.space, m.peekAt = stops[(at+by+len(stops))%len(stops)], m.now()
	return ""
}

// spaceEmpty says which filters hide every session, and the way out.
func (m Model) spaceEmpty() string {
	where := m.space
	if m.boardHost != "" {
		where += " on " + m.boardHost
	}
	return "Nothing in " + where + ". " + m.keys.side.Leave.Help().Key + " shows all."
}
