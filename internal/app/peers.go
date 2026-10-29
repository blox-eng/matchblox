package app

import (
	"cmp"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/hosts"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// Peer is a ticked host other than the home host: the console holds a
// stream to it next to the home one.
type Peer struct {
	Host string
	// Conn nil, with no DialErr: the console dials with Redial at its
	// start, so a slow host never holds the others.
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
	id                        int // 0 for Options.Peers; a link a reconcile opens gets its own
	conn                      transport.Conn
	redial                    func() (transport.Conn, error)
	dialErr                   error // why the first dial failed
	lost, redialing, answered bool
	silent                    bool // connected, and no state in silentAfter: a row says why
	backoff                   time.Duration
	blocked                   error
	mismatch                  bool // the peer runs another version: a blocked row, never a stream
	hello                     proto.Hello
	at                        time.Time   // the last stateMsg
	st                        proto.State // the last state; History, Found, Used kept from the first
	have                      bool
	cpu                       *series        // the Machine's graphs, as Model.history
	hist                      history.Series // the History tab's points, as Model.hist
	acts                      []string       // the act ids sent on conn with no result yet
	wrepos                    []whereRepo
	wfrom                     []string // what wrepos was built from, as Model's
}

// peerMsg tags a message with the peer and the conn it came from: after a
// redial, messages from the old conn are dropped. A redial's own messages
// (connMsg, redialFailed) carry no conn, but the id of the link that dialed.
type peerMsg struct {
	host string
	conn transport.Conn
	msg  tea.Msg
	id   int
}

// peerRedialMsg is a peer's redial tick; gen is as in redialMsg, id is the
// link's: a host hidden and shown again is a new link, and the old tick is
// not its own.
type peerRedialMsg struct {
	host string
	gen  int
	id   int
}

// redialAfter is the wait before the next redial after one that waited b.
func redialAfter(b time.Duration) time.Duration {
	return min(max(b*2, 250*time.Millisecond), 10*time.Second)
}

func links(peers []Peer) []link {
	var out []link
	for _, p := range peers {
		out = append(out, newLink(p, 0))
	}
	return out
}

func newLink(p Peer, id int) link {
	dials := p.Conn == nil && p.DialErr == nil && p.Redial != nil
	return link{host: p.Host, id: id, conn: p.Conn, redial: p.Redial, dialErr: p.DialErr, redialing: dials, cpu: newSeries(60)}
}

// hostCount is the hosts of the console: home and its peers.
func (m Model) hostCount() int { return 1 + len(m.peers) }

func (m Model) peerAt(host string) int {
	return slices.IndexFunc(m.peers, func(l link) bool { return l.host == host })
}

// peerStart says hello to peer i and reads from it, reports why its first
// dial failed, or dials it.
func (m Model) peerStart(i int) tea.Cmd {
	l := m.peers[i]
	switch {
	case l.dialErr != nil && l.conn == nil:
		err := l.dialErr
		return func() tea.Msg { return peerMsg{host: l.host, msg: lostMsg{err: err}} }
	case l.conn == nil && l.redialing:
		return m.dialPeer(i)
	}
	return tea.Batch(toPeer(l.host, helloOn(l.conn, m.opt.Binary)), m.peerRecv(i), m.watchPeer(i))
}

// watchPeer ticks once silentAfter passes on peer i's conn, as
// watchSilence does for the home host.
func (m Model) watchPeer(i int) tea.Cmd {
	host, c, id := m.peers[i].host, m.peers[i].conn, m.peers[i].id
	return tea.Tick(silentAfter, func(time.Time) tea.Msg { return peerMsg{host: host, conn: c, msg: silentMsg{c}, id: id} })
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

// helloOf is the hello a host sent; the zero one before it says hello.
func (m Model) helloOf(host string) proto.Hello {
	if host == m.opt.Host {
		return m.host
	}
	if i := m.peerAt(host); i >= 0 {
		return m.peers[i].hello
	}
	return proto.Hello{}
}

// refusal is why an act cannot go to host now; "" when it can. One host
// keeps today's checks and their words. A lost host refuses, however
// short the loss: its act would have no answer.
func (m Model) refusal(host string) string {
	var redialing bool
	var backoff time.Duration
	switch i := m.peerAt(host); {
	case host == m.opt.Host:
		if m.hostCount() == 1 || m.conn != nil && !m.lost && !m.mismatch {
			return ""
		}
		redialing, backoff = m.redialing, m.backoff
	case i >= 0:
		l := m.peers[i]
		if l.conn != nil && !l.lost && !l.mismatch {
			return ""
		}
		redialing, backoff = l.redialing, l.backoff
	}
	why := m.nameOf(host) + " is not connected."
	if redialing && backoff > 0 { // 0: its first dial runs
		why += " It tries again in " + backoff.String() + "."
	}
	return why
}

// hostName is a host as the console names it.
func hostName(host string) string { return cmp.Or(host, hosts.ThisMachine) }

// toPeer turns the messages of a home-shaped read or send into peerMsgs.
func toPeer(host string, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case fromConn:
			return peerMsg{host: host, conn: msg.conn, msg: msg.msg}
		case lostMsg:
			return peerMsg{host: host, conn: msg.conn, msg: msg}
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
		if m.quitting || l.redialing || l.mismatch || (msg.conn != nil && msg.conn != l.conn) {
			return m, nil
		}
		return m.peerLost(i, msg.err)
	case redialFailed:
		if msg.gen != m.opt.Gen || pm.id != l.id || !l.redialing {
			return m, nil
		}
		m.peers = slices.Clone(m.peers)
		m.peers[i].redialing = false
		return m.peerLost(i, msg.err)
	case connMsg:
		if msg.gen != m.opt.Gen || pm.id != l.id || m.quitting || !l.redialing {
			_ = msg.conn.Close() // the console or the redial that dialed it is gone
			return m, nil
		}
		stale := m.stale(l.host) // a short loss showed nothing: its end shows nothing
		m.peers = slices.Clone(m.peers)
		p := &m.peers[i]
		p.conn, p.redialing, p.answered, p.silent, p.acts = msg.conn, false, false, false, nil
		if stale {
			m.flash = p.host + ": connected again"
		}
		return m, tea.Batch(toPeer(p.host, helloOn(p.conn, m.opt.Binary)), m.peerRecv(i), m.watchPeer(i))
	}
	if pm.conn != l.conn {
		return m, nil
	}
	m.peers = slices.Clone(m.peers)
	p := &m.peers[i]
	switch msg := pm.msg.(type) {
	case silentMsg:
		// Its own read runs: this tick starts none.
		p.silent = pm.id == p.id && !p.answered && !p.lost && !p.mismatch
		return m, nil
	case helloMsg:
		p.hello = proto.Hello(msg)
		p.hello.Tmux = remote.HostTmux(p.hello.Tmux)
		if p.mismatch = p.hello.Version != proto.Version; p.mismatch {
			_ = p.conn.Close() // a row with the fix, and no more reads
			return m, nil
		}
		next, check := m.checkLogin(p.host)
		return next, tea.Batch(next.peerRecv(i), check)
	case stateMsg:
		st := proto.State(msg)
		st.History = cmp.Or(st.History, p.st.History)
		st.Found = cmp.Or(st.Found, p.st.Found)
		st.Used = cmp.Or(st.Used, p.st.Used)
		m.noteChanges(p.host, st.Sessions)
		first := !p.have
		p.st, p.at, p.have, p.answered, p.silent = st, m.now(), true, true, false
		p.cpu.push(st.Machine)
		switch {
		case msg.History != nil:
			p.hist = *msg.History
		case !first:
			p.hist.Append(history.FromSnapshot(st.Snapshot))
		}
		if from := m.whereFrom(p.host); msg.Found != nil || msg.Used != nil || !slices.Equal(from, p.wfrom) {
			p.wrepos, p.wfrom = m.whereRepos(p.host), from
		}
		if m.form != nil && m.form.host == p.host && m.form.whereList() && !m.form.cloneList() {
			f := *m.form
			m.form = &f
			m.refreshWhere(&f, m.whereSelected().path())
		}
		if p.lost {
			p.lost, p.backoff = false, 0
			if strings.HasPrefix(m.flash, p.host+": ") {
				m.flash = ""
			}
		}
		m.merge()
		m.keepSelection()
		next, order := m.queuedOrder(p.host)
		return next, tea.Batch(next.peerRecv(i), order)
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
// home host does, unless trying again cannot help. A short loss shows
// nothing: from staleAfter its rows and the key line say so. Acts in
// flight say at once that they have no answer.
func (m Model) peerLost(i int, err error) (tea.Model, tea.Cmd) {
	m.peers = slices.Clone(m.peers)
	p := &m.peers[i]
	if p.conn != nil {
		_ = p.conn.Close()
	}
	first := !p.lost
	p.lost, p.silent = true, false
	news := m.noAnswer(*p)
	m.batchLost(p.host, p.acts)
	p.acts = nil
	if blocking(err) {
		// Its row says why: the flash of the host goes.
		p.blocked = err
		if strings.HasPrefix(m.flash, p.host+": ") {
			m.flash = ""
		}
		return m, news
	}
	var stale tea.Cmd
	if first {
		stale = m.staleTick(p.at)
	}
	if p.redial == nil {
		return m, tea.Batch(news, stale)
	}
	p.redialing = true
	p.backoff = redialAfter(p.backoff)
	host, gen, id := p.host, m.opt.Gen, p.id
	return m, tea.Batch(news, stale, tea.Tick(p.backoff, func(time.Time) tea.Msg { return peerRedialMsg{host, gen, id} }))
}

// noAnswer says at once that l's acts in flight have no answer.
func (m *Model) noAnswer(l link) tea.Cmd {
	n := len(l.acts)
	if n == 0 {
		return nil
	}
	verb := "have"
	if n == 1 {
		verb = "has"
	}
	return m.say(l.host + ": " + plural(n, "act") + " " + verb + " no answer")
}

func (m Model) peerRedial(msg peerRedialMsg) (tea.Model, tea.Cmd) {
	i := m.peerAt(msg.host)
	if msg.gen != m.opt.Gen || i < 0 || msg.id != m.peers[i].id || !m.peers[i].redialing || m.quitting {
		return m, nil // a tick from a console that was closed, or a link that is gone
	}
	return m, m.dialPeer(i)
}

// dialPeer dials peer i in the background: its connMsg or redialFailed
// comes back with the link's id.
func (m Model) dialPeer(i int) tea.Cmd {
	redial, host, gen, id := m.peers[i].redial, m.peers[i].host, m.opt.Gen, m.peers[i].id
	return func() tea.Msg {
		c, err := redial()
		if err != nil {
			return peerMsg{host: host, msg: redialFailed{err, gen}, id: id}
		}
		return peerMsg{host: host, msg: connMsg{c, gen}, id: id}
	}
}

// reconcile makes peers the console's peers, in their order (R12): a link
// whose host stays is kept, with its stream, backoff and state; a dropped
// one is closed, and its late ticks find no link of theirs; a new one
// opens. A Peer of a kept host needs only its Host.
func (m Model) reconcile(peers []Peer) (Model, tea.Cmd) {
	var next []link
	var start []int
	for _, p := range peers {
		if i := m.peerAt(p.Host); i >= 0 {
			next = append(next, m.peers[i])
			continue
		}
		m.linked++
		start = append(start, len(next))
		next = append(next, newLink(p, m.linked))
	}
	var news []tea.Cmd
	for _, l := range m.peers {
		if slices.ContainsFunc(next, func(n link) bool { return n.host == l.host }) {
			continue
		}
		if l.conn != nil {
			_ = l.conn.Close()
		}
		news = append(news, m.noAnswer(l))
		m.dropLogin(l.host)
	}
	m.peers = next
	m.merge()
	m.keepSelection()
	for _, i := range start {
		news = append(news, m.peerStart(i))
	}
	return m, tea.Batch(news...)
}

// closeStreams closes the console's stream to every host.
func (m Model) closeStreams() {
	if m.conn != nil {
		_ = m.conn.Close()
	}
	for _, l := range m.peers {
		if l.conn != nil {
			_ = l.conn.Close()
		}
	}
}
