package app

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// showHostMsg asks the shell to show host in all hosts again: Show writes
// the hidden file, as Machine › Hosts does.
type showHostMsg struct{ host string }

// hostHidden: the builder hid host, so all hosts leaves it out. The home
// host is never hidden.
func (m Model) hostHidden(host string) bool {
	return host != m.opt.Host && slices.Contains(m.hiddenHosts, hostName(host))
}

// inView: host is the one s shows, or all hosts show and host is not
// hidden.
func (m Model) inView(host string) bool {
	if m.boardHost != "" {
		return m.nameOf(host) == m.boardHost
	}
	return !m.hostHidden(host)
}

// viewHosts are the hosts of the view s shows, in Hosts order: the one
// host, or every host but the hidden.
func (m Model) viewHosts() []string {
	return slices.DeleteFunc(m.hosts(), func(h string) bool { return !m.inView(h) })
}

// shownHosts are the hosts all hosts holds: every host but the hidden.
func (m Model) shownHosts() []string {
	return slices.DeleteFunc(m.hosts(), m.hostHidden)
}

// shownHost is the host s shows, by its routing key. false: all hosts.
func (m Model) shownHost() (string, bool) {
	hs := m.hosts()
	if i := slices.IndexFunc(hs, func(h string) bool { return m.boardHost != "" && m.nameOf(h) == m.boardHost }); i >= 0 {
		return hs[i], true
	}
	return "", false
}

// hostStops are the views s steps through, by name: every host, home first
// and the others in name order, hidden ones too, then all hosts (""). With
// one host shown, all hosts is that host: it has no stop of its own.
func (m Model) hostStops() []string {
	var names []string
	for _, h := range m.hosts()[1:] {
		names = append(names, m.nameOf(h))
	}
	slices.Sort(names)
	stops := append([]string{m.nameOf(m.opt.Host)}, names...)
	if shown := m.shownHosts(); len(shown) == 1 {
		stops = slices.DeleteFunc(stops, func(n string) bool { return n == m.nameOf(shown[0]) })
	}
	return append(stops, "")
}

// turnHost is s: the next view of hostStops. With one host linked there is
// no other view, and the words say so.
func (m *Model) turnHost() string {
	stops := m.hostStops()
	switch {
	case len(stops) == 1 && len(m.hiddenHosts) > 0:
		return m.unlinkedWord()
	case len(stops) == 1:
		return m.nameOf(m.opt.Host) + " is the only host · Machine › Hosts adds one"
	}
	next := stops[(slices.Index(stops, m.boardHost)+1)%len(stops)]
	m.boardHost, m.peekAt = next, m.now()
	return ""
}

// unhide is h: on a hidden host, it shows that host in all hosts again;
// elsewhere it says what h is for.
func (m *Model) unhide() tea.Cmd {
	host, one := m.shownHost()
	switch {
	case one && m.hostHidden(host):
		return func() tea.Msg { return showHostMsg{host} }
	case len(m.hiddenHosts) == 0:
		return m.notice("no host is hidden")
	case !slices.ContainsFunc(m.hosts(), m.hostHidden):
		return m.notice(m.unlinkedWord())
	}
	return m.notice(m.keys.side.Unhide.Help().Key + " shows a hidden host · " + m.keys.board.Host.Help().Key + " goes to it")
}

// unlinkedWord is the notice of the hidden hosts that have no link: this
// machine, hidden while another host is home.
func (m Model) unlinkedWord() string {
	verb := " is hidden"
	if len(m.hiddenHosts) > 1 {
		verb = " are hidden"
	}
	return strings.Join(m.hiddenHosts, ", ") + verb + " · Machine › Hosts shows it"
}

// hostShown is the console after Show of host wrote the hidden file, or
// failed with err: then host stays hidden.
func (m Model) hostShown(host string, hidden []string, err error) (Model, tea.Cmd) {
	if err != nil {
		return m, m.notice("not saved: " + err.Error())
	}
	m.hiddenHosts = hidden
	return m, m.notice(m.nameOf(host) + " shows in all hosts")
}

// hostWord is the header's word for the view s shows: all hosts, or the
// one host all hosts is. A hidden host says so and how to show it; a host
// that does not answer says since when.
func (m Model) hostWord() string {
	host, one := m.shownHost()
	switch {
	case one && m.hostHidden(host):
		return m.boardHost + " · hidden from all hosts · " + m.keys.side.Unhide.Help().Key + " shows it"
	case one && m.stale(host):
		return m.unreachedWord(host)
	case !one && len(m.shownHosts()) == 1:
		return m.nameOf(m.shownHosts()[0]) + " ▾"
	}
	return cmp.Or(m.boardHost, "all hosts") + " ▾"
}

// unreachedWord is a host that does not answer, and since when.
func (m Model) unreachedWord(host string) string {
	return m.nameOf(host) + " not reached · since " + brief(m.now().Sub(m.stateAt(host)))
}
