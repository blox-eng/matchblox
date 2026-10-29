package app

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/hosts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// staleAfter is how long a lost host of two or more keeps its rows as
// they were before they say stale (R5): a short reconnect shows nothing.
const staleAfter = 30 * time.Second

// staleMsg is the tick at which a lost host turns stale: it draws again.
type staleMsg struct{}

// staleTick ticks when a host whose last state came at at turns stale.
// One host says stale at once, as today: no tick.
func (m Model) staleTick(at time.Time) tea.Cmd {
	d := at.Add(staleAfter).Sub(m.now())
	if m.hostCount() == 1 || d <= 0 {
		return nil
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return staleMsg{} })
}

// stale: what shows of host is its last state, and the host is gone. One
// host: lost, with a state. Two or more: lost, and staleAfter or more
// since its last state (R5).
func (m Model) stale(host string) bool {
	lost, have, at := m.lost, m.have, m.at
	if host != m.opt.Host {
		i := m.peerAt(host)
		if i < 0 {
			return false
		}
		l := m.peers[i]
		lost, have, at = l.lost, l.have, l.at
	}
	if m.hostCount() == 1 {
		return lost && have
	}
	return lost && have && m.now().Sub(at) >= staleAfter
}

// stateAt is when host's last state came.
func (m Model) stateAt(host string) time.Time {
	if host == m.opt.Host {
		return m.at
	}
	if i := m.peerAt(host); i >= 0 {
		return m.peers[i].at
	}
	return time.Time{}
}

// nameOf is the word for host on screen. The home host goes by the name
// in its hello, this machine before it; s.Host stays the routing key.
func (m Model) nameOf(host string) string {
	if host == m.opt.Host {
		return cmp.Or(m.opt.Host, m.host.Host, hosts.ThisMachine)
	}
	return hostName(host)
}

// hosts are the console's hosts in Hosts order: home first.
func (m Model) hosts() []string {
	out := []string{m.opt.Host}
	for _, l := range m.peers {
		out = append(out, l.host)
	}
	return out
}

// staleHosts are the hosts that are stale, by name, in Hosts order.
func (m Model) staleHosts() []string {
	if m.hostCount() == 1 {
		return nil
	}
	var out []string
	for _, h := range m.hosts() {
		if m.stale(h) {
			out = append(out, m.nameOf(h))
		}
	}
	return out
}

// lostLine is the key line while a host of two or more is stale: which
// are lost and when the next try is. A blocked host has its own row.
func (m Model) lostLine() string {
	if m.hostCount() == 1 {
		return ""
	}
	var names []string
	var next time.Duration
	for _, h := range m.hosts() {
		blocked, redialing, backoff := m.blocked, m.redialing, m.backoff
		if i := m.peerAt(h); h != m.opt.Host && i >= 0 {
			blocked, redialing, backoff = m.peers[i].blocked, m.peers[i].redialing, m.peers[i].backoff
		}
		if blocked != nil || !m.stale(h) {
			continue
		}
		names = append(names, m.nameOf(h))
		if redialing && (next == 0 || backoff < next) {
			next = backoff
		}
	}
	var line string
	switch len(names) {
	case 0:
		return ""
	case 1:
		line = names[0] + ": connection lost."
	default:
		line = fmt.Sprintf("%d hosts lost: %s.", len(names), strings.Join(names, ", "))
	}
	if next > 0 {
		line += " Tries again in " + next.String() + "."
	}
	return line
}

// blockedHost is a host of two or more the console cannot reach, or one
// that runs another version: a row at the end of Sessions, with today's
// words and fix.
type blockedHost struct {
	host    string
	err     error
	other   bool // the host runs another version, version
	version int
}

// fixable: ⏎ fixes it from here. A newer host has no fix here: the
// console is the one to update.
func (b blockedHost) fixable() bool { return !b.other || b.version < proto.Version }

// blockOf is why host cannot be reached, and whether it runs an older
// version, which matchblox connect updates.
func (m Model) blockOf(host string) (blocked error, older bool) {
	if host == m.opt.Host {
		return m.blocked, m.mismatch && m.host.Version < proto.Version
	}
	if i := m.peerAt(host); i >= 0 {
		l := m.peers[i]
		return l.blocked, l.mismatch && l.hello.Version < proto.Version
	}
	return nil, false
}

// blockedHosts are the blocked rows, in Hosts order. One host is the
// whole screen instead, as today.
func (m Model) blockedHosts() []blockedHost {
	if m.hostCount() == 1 {
		return nil
	}
	var out []blockedHost
	if m.mismatch {
		out = append(out, blockedHost{host: m.opt.Host, other: true, version: m.host.Version})
	} else if m.blocked != nil {
		out = append(out, blockedHost{host: m.opt.Host, err: m.blocked})
	}
	for _, l := range m.peers {
		switch {
		case l.mismatch:
			out = append(out, blockedHost{host: l.host, other: true, version: l.hello.Version})
		case l.blocked != nil:
			out = append(out, blockedHost{host: l.host, err: l.blocked})
		}
	}
	return out
}

// shows: the console has rows to draw: a state from a host, or a blocked
// host's row. One host: its state, as today.
func (m Model) shows() bool {
	return m.have || m.hostCount() > 1 &&
		(slices.ContainsFunc(m.peers, func(l link) bool { return l.have }) || len(m.blockedHosts()) > 0)
}

// selectedBlocked is the blocked host's row under the selection: after
// the ghost rows.
func (m Model) selectedBlocked() (blockedHost, bool) {
	if m.tab != tabSessions {
		return blockedHost{}, false
	}
	bs := m.blockedHosts()
	i := m.sessIndex() - len(m.doors) - len(m.snap.Sessions) - len(m.ghosts())
	if i < 0 || i >= len(bs) {
		return blockedHost{}, false
	}
	return bs[i], true
}

// blockedWords are today's words of why a host cannot be reached, and
// its fix, for its row.
func (m Model) blockedWords(b blockedHost) (why, fix string) {
	if b.other {
		why = fmt.Sprintf("is version %d, the console is version %d", b.version, proto.Version)
		if b.version > proto.Version {
			return why, "update the console: " + installCmd
		}
		return why, "connect"
	}
	switch {
	case errors.Is(b.err, remote.ErrNotInstalled):
		return "matchblox is not installed", "connect"
	case errors.Is(b.err, remote.ErrHostKeyUnknown):
		return "host key is not known yet", "connect"
	case errors.Is(b.err, remote.ErrHostKeyChanged):
		return "host key changed: this can be an attack", "try again"
	}
	return "is not connected", "connect"
}

// blockedRow is a blocked host's row: the host, why, and ⏎ with its fix.
func (m Model) blockedRow(b blockedHost, sel bool, w int) string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	why, fix := m.blockedWords(b)
	line := mark + st.ink(sel).Render(m.nameOf(b.host)) + "  " + st.warn.Render(why) + "  "
	if !b.fixable() {
		return fit(line+st.muted.Render(fix), w)
	}
	return fit(line+keyWord(m.keys.main.Open, st.accent, st.muted, " "+fix), w)
}

// blockedRows are the blocked hosts' rows, after the ghost rows.
func (m Model) blockedRows(b *body, at, sel, w int) {
	for i, bh := range m.blockedHosts() {
		b.addRow(at+i, i == sel, w, m.st, m.blockedRow(bh, i == sel, w))
	}
}

// unblockHost is ⏎ on a blocked host's row: today's fix for that host.
func (m Model) unblockHost(b blockedHost) (tea.Model, tea.Cmd) {
	if b.host == m.opt.Host {
		return m.unblock()
	}
	i := m.peerAt(b.host)
	if errors.Is(m.peers[i].blocked, remote.ErrHostKeyChanged) {
		return m.peerTryAgain(i)
	}
	if m.pending = m.connectOn(b.host); m.pending == nil {
		_, m.flash = m.blockedWords(b)
	}
	return m, nil
}

// peerTryAgain dials peer i again, after the builder fixed what its row
// showed.
func (m Model) peerTryAgain(i int) (tea.Model, tea.Cmd) {
	m.peers = slices.Clone(m.peers)
	p := &m.peers[i]
	p.blocked, p.mismatch = nil, false
	m.flash = "trying " + p.host + " again…"
	if p.redial == nil {
		return m, nil
	}
	p.lost, p.redialing = true, true
	host, gen := p.host, m.opt.Gen
	return m, func() tea.Msg { return peerRedialMsg{host: host, gen: gen} }
}

// rowTag is the muted words after a session's name, in room cells: its
// host when the console has two or more, then its account. Short of
// room the account goes first; taggedName then cuts the host (R11).
func (m Model) rowTag(s sample.Session, room int) string {
	account := m.accountTag(s)
	if m.hostCount() == 1 {
		return account
	}
	host := m.nameOf(s.Host)
	if account == "" {
		return host
	}
	if full := host + " · " + account; lipgloss.Width(full) <= room {
		return full
	}
	return host
}

// hostNote is host's live note on Hosts: its sessions, and those that
// wait. "" before its first state.
func (m Model) hostNote(host string) string {
	have := m.have
	if host != m.opt.Host {
		i := m.peerAt(host)
		if i < 0 {
			return ""
		}
		have = m.peers[i].have
	}
	if !have {
		return ""
	}
	n, waits := 0, 0
	for _, s := range m.all.sessions {
		if s.Host != host {
			continue
		}
		n++
		if m.groupOf(s) == groupWaits {
			waits++
		}
	}
	note := fmt.Sprintf("%d sessions", n)
	if waits > 0 {
		note += fmt.Sprintf(" · %d waits", waits)
	}
	return note
}
