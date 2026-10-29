package app

import (
	"cmp"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// Peer is a ticked host other than the home host: the console holds a
// stream to it next to the home one.
type Peer struct {
	Host string
	Conn transport.Conn
	// Redial connects again after the connection drops. Nil: no retry.
	Redial func() (transport.Conn, error)
	// DialErr is why the first dial failed; the peer starts with it.
	DialErr error
}

// link is the console's stream to one peer, with what the home host keeps
// in Model's own fields.
type link struct {
	host                      string
	conn                      transport.Conn
	redial                    func() (transport.Conn, error)
	lost, redialing, answered bool
	backoff                   time.Duration
	blocked                   error
	hello                     proto.Hello
	at                        time.Time   // the last stateMsg
	st                        proto.State // the last state; History, Found, Used kept from the first
	have                      bool
	acts                      []string // the act ids sent on conn with no result yet
}

// peerMsg tags a message with the peer and the conn it came from: after a
// redial, messages from the old conn are dropped. A redial's own messages
// (connMsg, redialFailed) carry no conn.
type peerMsg struct {
	host string
	conn transport.Conn
	msg  tea.Msg
}

// peerRedialMsg is a peer's redial tick; gen is as in redialMsg.
type peerRedialMsg struct {
	host string
	gen  int
}

// redialAfter is the wait before the next redial after one that waited b.
func redialAfter(b time.Duration) time.Duration {
	return min(max(b*2, 250*time.Millisecond), 10*time.Second)
}

func links(peers []Peer) []link {
	var out []link
	for _, p := range peers {
		out = append(out, link{host: p.Host, conn: p.Conn, redial: p.Redial})
	}
	return out
}

func (m Model) peerAt(host string) int {
	return slices.IndexFunc(m.peers, func(l link) bool { return l.host == host })
}

// peerStart says hello to peer i and reads from it, or reports why its first
// dial failed.
func (m Model) peerStart(i int) tea.Cmd {
	l := m.peers[i]
	if err := m.opt.Peers[i].DialErr; err != nil && l.conn == nil {
		return func() tea.Msg { return peerMsg{host: l.host, msg: lostMsg{err: err}} }
	}
	return tea.Batch(toPeer(l.host, helloOn(l.conn, m.opt.Binary)), m.peerRecv(i))
}

func (m Model) peerRecv(i int) tea.Cmd {
	l := m.peers[i]
	return toPeer(l.host, recvOn(l.conn))
}

// sendPeer sends a on peer i and keeps its id until its result comes, so a
// lost conn can say which acts have no answer.
func (m *Model) sendPeer(i int, a proto.Act) tea.Cmd {
	m.peers = slices.Clone(m.peers)
	l := &m.peers[i]
	id, cmd := m.sendOn(l.conn, a)
	l.acts = append(slices.Clone(l.acts), id)
	return toPeer(l.host, cmd)
}

// toPeer turns the messages of a home-shaped read or send into peerMsgs.
func toPeer(host string, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case fromConn:
			return peerMsg{host, msg.conn, msg.msg}
		case lostMsg:
			return peerMsg{host, msg.conn, msg}
		default:
			return msg
		}
	}
}

func (m Model) peerUpdate(pm peerMsg) (tea.Model, tea.Cmd) {
	i := m.peerAt(pm.host)
	if i < 0 {
		if c, ok := pm.msg.(connMsg); ok {
			_ = c.conn.Close() // the peer is gone
		}
		return m, nil
	}
	l := m.peers[i]
	switch msg := pm.msg.(type) {
	case lostMsg:
		if m.quitting || l.redialing || (msg.conn != nil && msg.conn != l.conn) {
			return m, nil
		}
		return m.peerLost(i, msg.err)
	case redialFailed:
		if msg.gen != m.opt.Gen || !l.redialing {
			return m, nil
		}
		m.peers = slices.Clone(m.peers)
		m.peers[i].redialing = false
		return m.peerLost(i, msg.err)
	case connMsg:
		if msg.gen != m.opt.Gen || m.quitting || !l.redialing {
			_ = msg.conn.Close() // the console or the redial that dialed it is gone
			return m, nil
		}
		m.peers = slices.Clone(m.peers)
		p := &m.peers[i]
		p.conn, p.redialing, p.answered = msg.conn, false, false
		m.flash = p.host + ": connected again"
		return m, tea.Batch(toPeer(p.host, helloOn(p.conn, m.opt.Binary)), m.peerRecv(i))
	}
	if pm.conn != l.conn {
		return m, nil
	}
	m.peers = slices.Clone(m.peers)
	p := &m.peers[i]
	switch msg := pm.msg.(type) {
	case helloMsg:
		p.hello = proto.Hello(msg)
		p.hello.Tmux = remote.HostTmux(p.hello.Tmux)
	case stateMsg:
		st := proto.State(msg)
		st.History = cmp.Or(st.History, p.st.History)
		st.Found = cmp.Or(st.Found, p.st.Found)
		st.Used = cmp.Or(st.Used, p.st.Used)
		p.st, p.at, p.have, p.answered = st, m.now(), true, true
		if p.lost {
			p.lost, p.backoff = false, 0
			if strings.HasPrefix(m.flash, p.host+": ") {
				m.flash = ""
			}
		}
	case resultMsg:
		at := slices.Index(p.acts, msg.ActID)
		if at < 0 {
			return m, m.peerRecv(i) // not an act sent on this conn: its own went with the lost one
		}
		p.acts = slices.Delete(slices.Clone(p.acts), at, at+1)
		mm, cmd := m.result(proto.Result(msg))
		return mm, tea.Batch(mm.peerRecv(i), cmd)
	}
	return m, m.peerRecv(i)
}

// peerLost closes peer i's conn and dials again after the backoff, as the
// home host does, unless trying again cannot help.
func (m Model) peerLost(i int, err error) (tea.Model, tea.Cmd) {
	m.peers = slices.Clone(m.peers)
	p := &m.peers[i]
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.lost = true
	if blocking(err) {
		p.blocked = err
		return m, nil
	}
	m.flash = p.host + ": connection lost: " + err.Error()
	if n := len(p.acts); n > 0 {
		verb := "have"
		if n == 1 {
			verb = "has"
		}
		m.flash += "; " + plural(n, "act") + " " + verb + " no answer"
		p.acts = nil
	}
	if p.redial == nil {
		return m, nil
	}
	p.redialing = true
	p.backoff = redialAfter(p.backoff)
	m.flash += "; trying again in " + p.backoff.String()
	host, gen := p.host, m.opt.Gen
	return m, tea.Tick(p.backoff, func(time.Time) tea.Msg { return peerRedialMsg{host, gen} })
}

func (m Model) peerRedial(msg peerRedialMsg) (tea.Model, tea.Cmd) {
	i := m.peerAt(msg.host)
	if msg.gen != m.opt.Gen || i < 0 || !m.peers[i].redialing || m.quitting {
		return m, nil // a tick from a console that was closed, or a peer that is gone
	}
	redial, host, gen := m.peers[i].redial, msg.host, m.opt.Gen
	return m, func() tea.Msg {
		c, err := redial()
		if err != nil {
			return peerMsg{host: host, msg: redialFailed{err, gen}}
		}
		return peerMsg{host: host, msg: connMsg{c, gen}}
	}
}
