package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// The limits of peer sends: one send from a session to another in
// PeerPairEvery, and PeerSendsPerHour sends from one session in an hour.
const (
	PeerPairEvery    = 30 * time.Second
	PeerSendsPerHour = 20
)

const sendOff = `send is off on this host: [chains] send = "off"`

// didNotGet is what the sender of a waiting line hears when it is dropped
// before it went: who did not get it, and why.
func didNotGet(who, why string) string { return who + " did not get your line: " + why }

// peerShowWait is how long a typed line has to show in the prompt before
// matchblox clears it.
var peerShowWait = 2 * time.Second

// isSend tells if act a is a plain send (message:<pane>), and to which pane.
func isSend(a proto.Act) (string, bool) {
	return strings.CutPrefix(a.RecID, "message:")
}

// peerAct is the pane a peer send goes to, when act a is one: a plain send
// whose sender the service found itself, by the caller's pid, and that is
// not the stoker's.
func (s *Service) peerAct(a proto.Act) (string, bool) {
	pane, ok := isSend(a)
	if !ok || !a.FromKnown || s.fromStoker(a) {
		return "", false
	}
	return pane, true
}

// peerSend sends one line from the session sender to its chain-mate in
// pane, as "[from <sender> <pane>] <text>". The tag is matchblox's, from the
// pane the service found for the sender. A mate that rests and reads a
// prompt line gets it typed; a busy one hears it at its next step (Codex:
// typed once it rests); OpenCode gets none. It is no stoker step: it counts
// in no run, drives no pane and gives no go.
func (s *Service) peerSend(ctx context.Context, sender sample.Session, pane string, a proto.Act) proto.Result {
	if s.ChainsSend == "off" {
		return proto.Result{Err: sendOff}
	}
	if a.Confirm != "y" {
		return proto.Result{Err: "needs confirm"}
	}
	if err := checkPeerText(a.Text); err != nil {
		return proto.Result{Err: err.Error()}
	}
	switch {
	case s.fromStoker(proto.Act{From: pane}):
		return proto.Result{Err: "no agent acts on the stoker's own session"}
	case pane == a.From:
		return proto.Result{Err: "a session does not send to itself"}
	}
	x, ok := s.sessionAt(pane)
	if !ok {
		return proto.Result{Err: "no agent session in " + pane + " any more"}
	}
	chainID, ok := s.mateChain(memberOf(sender), memberOf(x))
	if !ok {
		return proto.Result{Err: fmt.Sprintf("%s is not in your chain. Run `matchblox chain %s` first.", x.Name, x.Name)}
	}
	now := s.now()
	slot, err := s.peers.reserve(a.From, pane, x.Name, now)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	tag := peerTag(sender)
	line := tag + " " + a.Text
	rec := peerRecord{At: now, From: sender.Name, FromPane: a.From, To: x.Name, ToPane: pane, Len: len(a.Text), SHA256: sum(a.Text)}
	inbox := func(note string) proto.Result {
		s.postMessage(chainID, sender, x, line, now)
		rec.Via, rec.Result = "inbox", "queued"
		s.logPeer(rec)
		return proto.Result{Note: note}
	}
	switch {
	case x.Agent == "opencode":
		return inbox("kept for " + x.Name + ": OpenCode takes no typed lines. It shows in matchblox peek " + x.Name + ".")
	case !readsPrompt[x.Agent]:
		return inbox("queued for " + x.Name + ": it takes no typed lines. It hears the line at its next step.")
	}
	codex := x.Agent == "codex"
	wait := func(why string) proto.Result {
		if !codex {
			return inbox("queued for " + x.Name + ": " + why + ". It hears the line at its next step.")
		}
		s.peers.queue(peerLine{from: memberOf(sender), to: memberOf(x), line: line, tag: tag, rec: rec})
		s.spark(chainID, now)
		rec.Via, rec.Result = "at rest", "queued"
		s.logPeer(rec)
		return proto.Result{Note: queuedFor(x, why)}
	}
	why, err := s.peerWait(pane, x)
	switch {
	case err != nil:
		s.peers.release(slot)
		return proto.Result{Err: err.Error()}
	case why != "":
		return wait(why)
	case codex && s.peers.waits(memberOf(x)):
		return wait("a line to it waits before yours")
	}
	// A Codex that rests takes one typed line at a time: a send while one
	// types waits for its next rest.
	if !s.peers.claim(pane) {
		return wait("a line to it types now")
	}
	res, typed, cleared, stuck := s.typeLine(ctx, x, line, tag)
	s.peers.unclaim(pane)
	if !typed || cleared {
		why, _ := s.peerWait(pane, x)
		if !typed && why == "" {
			why = "its prompt would not take it now"
		} else if why == "" {
			why = "its prompt did not show the line"
		}
		return wait(why)
	}
	if stuck || res.Err != "" {
		rec.Via, rec.Result = "typed", typedResult(res)
		s.logPeer(rec)
		return res
	}
	s.spark(chainID, now)
	rec.Via, rec.Result = "typed", typedResult(res)
	s.logPeer(rec)
	return proto.Result{Ran: res.Ran, Note: "sent"}
}

// queuedFor is what the sender of a line that waits for a Codex rest
// hears: why it waits. A Claude Code line waits in the inbox instead, and
// is heard at the match's next step.
func queuedFor(x sample.Session, why string) string {
	return "queued for " + x.Name + ": " + why + ". It gets the line at its next rest."
}

// peerTries is how many rests a waiting line gets to show in the prompt.
const peerTries = 3

// deliverAtRest types the oldest line that waits for each Codex match that
// rests now: one line a pane each sample. A line refused before any key
// waits for a later rest, and so does one typed and cleared, up to
// peerTries; then its sender hears it did not go. A line that may still be
// in the prompt is dropped, so it is never typed twice. A line waits
// while its match's pane lives, however long; one for a match that left
// its team, or whose sender is no longer chained with it or may no longer
// send, is dropped, and its sender hears why. A sample that missed the
// sessions changes nothing.
func (s *Service) deliverAtRest(now time.Time) {
	s.mu.Lock()
	missed := s.sampleMissedLocked()
	s.mu.Unlock()
	if missed { // every mate would look gone
		return
	}
	for _, l := range s.peers.firsts() {
		x, ok := s.sessionOf(l.to)
		if !ok {
			if _, chained := s.mateChain(l.from, l.to); chained {
				continue // its pane lives: the line waits for its agent
			}
			gone := s.peers.forget(l.to)
			s.mu.Lock()
			for _, g := range gone {
				s.chainsLocked().Post(g.from, didNotGet(g.rec.To, "it is gone"), now)
			}
			s.mu.Unlock()
			continue
		}
		why := ""
		if _, chained := s.mateChain(l.from, l.to); !chained {
			why = "you are no longer chained"
		} else if s.ChainsSend == "off" {
			why = "sends are off on this host"
		}
		if why != "" {
			s.peers.drop(l.n)
			s.mu.Lock()
			s.chainsLocked().Post(l.from, didNotGet(x.Name, why), now)
			s.mu.Unlock()
			continue
		}
		if why, err := s.peerWait(x.Pane, x); err != nil || why != "" || !s.peers.claim(x.Pane) {
			continue
		}
		res, typed, cleared, stuck := s.typeLine(context.Background(), x, l.line, l.tag)
		s.peers.unclaim(x.Pane)
		if !typed {
			continue
		}
		rec := l.rec
		rec.At, rec.ToPane, rec.Via, rec.Result = now, x.Pane, "typed at rest", typedResult(res)
		s.logPeer(rec)
		if cleared && s.peers.tried(l.n) < peerTries {
			continue
		}
		s.peers.drop(l.n)
		note := ""
		switch {
		case cleared:
			note = fmt.Sprintf("%s could not take your line: it did not show after %d tries", x.Name, peerTries)
		case stuck:
			note = didNotGet(x.Name, "matchblox could not clear it")
		case res.Err != "":
			note = didNotGet(x.Name, "its prompt would not take it")
		default:
			note = gotYourLine(x.Name, "typed", now)
		}
		if note != "" {
			s.mu.Lock()
			s.chainsLocked().Post(l.from, note, now)
			s.mu.Unlock()
		}
	}
}

// typeLine types line and Enter into x's pane, which rests: never over a
// draft or into a pane in copy mode, each key guarded as a stoker step is,
// and Enter only once the prompt shows the line. A line that does not show
// in peerShowWait is cleared. typed tells if any key reached the pane,
// cleared if the line typed did not go and is cleared from the prompt, and
// stuck if it did not go and its clear failed: it may still be there.
func (s *Service) typeLine(ctx context.Context, x sample.Session, line, tag string) (res proto.Result, typed, cleared, stuck bool) {
	pane := x.Pane
	defer s.holdPane(pane)()
	// Read again under the hold: the look before it may be stale.
	switch why, err := s.peerWait(pane, x); {
	case err != nil:
		return proto.Result{Err: err.Error()}, false, false, false
	case why != "":
		return proto.Result{Err: why}, false, false, false
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start, RestPane: pane, StokePane: pane, MessagePane: pane, MessagePID: x.PID}
	res = s.Actions.Do(ctx, peerStep(s.mux().Type(pane, line), g), "y")
	if len(res.Ran) == 0 {
		return refusedStep(res), false, false, false
	}
	if !s.showsLine(ctx, pane) {
		cleared = s.clearLine(ctx, pane)
		return proto.Result{Ran: res.Ran, Err: "not delivered: " + x.Name + " did not show the line"}, true, cleared, !cleared
	}
	g.Typed = tag
	enter := s.Actions.Do(ctx, peerStep(s.mux().Keys(pane, "Enter"), g), "y")
	res.Ran = append(res.Ran, enter.Ran...)
	if len(enter.Ran) == 0 {
		// The line stays the mate's draft unless it goes: clear it while it is
		// still ours alone.
		if screen, ok := s.mux().Screens([]string{pane})[pane]; ok && strings.HasPrefix(said.Draft(screen), tag) {
			cleared = s.clearLine(ctx, pane)
			stuck = !cleared
		}
		enter = refusedStep(enter)
		return proto.Result{Ran: res.Ran, Err: enter.Err}, true, cleared, stuck
	}
	s.latchRest(pane)
	return res, true, false, false
}

func peerStep(step []string, g advice.Guard) advice.Action {
	return advice.Action{Label: "peer send", Steps: [][]string{step}, Destructive: true, Guards: []advice.Guard{g}}
}

// refusedStep is the result of a step that did not run, its guard's
// refusal as the error.
func refusedStep(res proto.Result) proto.Result {
	if res.Err == "" && len(res.Skipped) > 0 {
		res.Err, res.Skipped = res.Skipped[0], nil
	}
	return res
}

func typedResult(res proto.Result) string {
	switch {
	case res.Err == "" && len(res.Ran) == 2:
		return "sent"
	case len(res.Ran) == 1 && strings.HasSuffix(res.Err, "did not show the line"):
		return "typed, cleared"
	}
	return "typed, not sent"
}

// showsLine reads the pane until its prompt shows a peer line, for up to
// peerShowWait.
func (s *Service) showsLine(ctx context.Context, pane string) bool {
	deadline := time.Now().Add(peerShowWait)
	for {
		if screen, ok := s.mux().Screens([]string{pane})[pane]; ok && strings.HasPrefix(said.Draft(screen), "[from ") {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// clearLine empties the prompt line of pane, even after the act ends: a
// typed line must not stay behind as a draft. It tells if the clear ran.
func (s *Service) clearLine(ctx context.Context, pane string) bool {
	res := s.Actions.Do(context.WithoutCancel(ctx), advice.Action{Label: "peer send", Steps: [][]string{s.mux().Keys(pane, "C-u")}, Destructive: true}, "y")
	if res.Err != "" {
		fmt.Fprintln(os.Stderr, "matchblox: peer send: the line in", pane, "is not cleared:", res.Err)
		return false
	}
	return true
}

// mateChain is the id of the chain from and to are both in.
func (s *Service) mateChain(from, to chains.Member) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chainsLocked().Shared(from, to)
	return c.ID, ok
}

// postMessage puts sender's line in x's inbox and sparks the chain.
func (s *Service) postMessage(chainID string, sender, x sample.Session, line string, now time.Time) {
	s.mu.Lock()
	s.chainsLocked().PostLine(memberOf(x), memberOf(sender), line, now)
	s.mu.Unlock()
	s.spark(chainID, now)
}

// spark marks the chain and sends the consoles the spark at once.
func (s *Service) spark(chainID string, now time.Time) {
	s.mu.Lock()
	st := s.chainsLocked()
	st.Spark(chainID, now)
	s.cur.Chains = chainsView(st)
	s.mu.Unlock()
	s.broadcast()
}

func (s *Service) sessionOf(m chains.Member) (sample.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionOfLocked(m)
}

// peerWait is why the receiver cannot take a typed line now, read now:
// "" when it can. err: the receiver is gone, or another agent runs in its
// pane, so the line has nobody to wait for.
func (s *Service) peerWait(pane string, x sample.Session) (why string, err error) {
	cur, ok := s.sessionAt(pane)
	switch {
	case !ok:
		return "", fmt.Errorf("no agent session in %s any more", pane)
	case cur.PID != x.PID:
		return "", fmt.Errorf("another agent runs in %s now", pane)
	case cur.Ask != nil:
		return "it asks a question now", nil
	case cur.Permits:
		return "it shows a permission prompt", nil
	}
	if h, held := s.heldIn(pane, cur); held {
		if h.By == holds.ByStoker {
			return "the stoker holds it", nil
		}
		return "it is parked", nil
	}
	if it, ok := s.queued(pane); ok {
		switch {
		case it.State == queue.StatePermission:
			return "it shows a permission prompt", nil
		case stoker.Waits(it):
			return "it waits for you", nil
		}
	}
	if !s.resting(pane) {
		return "it is busy", nil
	}
	if mode, err := s.mux().InMode(pane); err != nil {
		return "its pane cannot be read", nil
	} else if mode {
		return "its pane is in copy mode", nil
	}
	screen, ok := s.mux().Screens([]string{pane})[pane]
	if !ok {
		return "its pane cannot be read", nil
	}
	switch _, kind := said.Screen(screen); {
	case kind == said.Permits:
		return "it shows a permission prompt", nil
	case kind == said.Asks:
		return "it asks a question now", nil
	case said.Draft(screen) != "":
		return "it has a draft in its prompt", nil
	}
	return "", nil
}

// gotYourLine is what the sender of a line that waited hears once its
// match got it: typed into its prompt, or heard at its next step.
func gotYourLine(name, how string, at time.Time) string {
	return fmt.Sprintf("[matchblox] %s got your line (%s %s)", name, how, at.Format("15:04"))
}

// checkPeerText refuses text that is not one line: a newline, a control
// key, a line or paragraph separator, or a hidden format character (a
// bidi control, a zero-width space) that would make the line read as
// another. The zero-width joiner of an emoji sequence passes.
func checkPeerText(text string) error {
	if err := panes.CheckAnswer(text); err != nil {
		return errors.New(strings.Replace(err.Error(), "the answer", "the message", 1))
	}
	if strings.ContainsFunc(text, notInALine) {
		return errors.New("the message must be one line, without control keys")
	}
	return nil
}

func notInALine(r rune) bool {
	const zwj = 0x200d
	return unicode.IsControl(r) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r) && r != zwj || unicode.Is(unicode.Bidi_Control, r)
}

// tagName is the sender for its tag: its name, without a character that
// would end the tag or the line, then its pane.
func tagName(x sample.Session) string {
	name := strings.Map(func(r rune) rune {
		if r == ']' || notInALine(r) {
			return -1
		}
		return r
	}, x.Name)
	if name = strings.TrimSpace(name); name == "" {
		return x.Pane
	}
	return name + " " + x.Pane
}

func peerTag(x sample.Session) string { return "[from " + tagName(x) + "]" }

// peerSent is one peer send that counts in the limits; n tells it from
// the others.
type peerSent struct {
	from, to string
	at       time.Time
	n        uint64
}

// peerLine is a line from one match that waits for its Codex match to
// rest; tries counts the rests it was typed at and did not show.
type peerLine struct {
	from, to  chains.Member
	line, tag string
	rec       peerRecord
	n         uint64
	tries     int
}

// peerSends are the peer sends of the last hour, oldest first, the lines
// that wait for a rest, and the Codex panes a line types into now. mu is
// held only to read or change them, never while a send types.
type peerSends struct {
	mu      sync.Mutex
	sent    []peerSent
	waiting []peerLine
	typing  map[string]bool
	last    uint64
}

// claim takes pane for one typed line; false when a line types there now.
func (p *peerSends) claim(pane string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.typing[pane] {
		return false
	}
	if p.typing == nil {
		p.typing = map[string]bool{}
	}
	p.typing[pane] = true
	return true
}

func (p *peerSends) unclaim(pane string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.typing, pane)
}

// reserve spends a slot of the limits for a send from to to, or says when
// the next is allowed.
func (p *peerSends) reserve(from, to, toName string, now time.Time) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.check(from, to, toName, now); err != nil {
		return 0, err
	}
	p.last++
	p.sent = append(p.sent, peerSent{from: from, to: to, at: now, n: p.last})
	return p.last, nil
}

// release gives back slot n: its send reached no pane.
func (p *peerSends) release(n uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = slices.DeleteFunc(p.sent, func(e peerSent) bool { return e.n == n })
}

func (p *peerSends) queue(l peerLine) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last++
	l.n = p.last
	p.waiting = append(p.waiting, l)
}

func (p *peerSends) waits(m chains.Member) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.ContainsFunc(p.waiting, func(l peerLine) bool { return l.to.Same(m) })
}

// firsts is the oldest waiting line of each match.
func (p *peerSends) firsts() []peerLine {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []peerLine
	for _, l := range p.waiting {
		if !slices.ContainsFunc(out, func(o peerLine) bool { return o.to.Same(l.to) }) {
			out = append(out, l)
		}
	}
	return out
}

// tried counts one more try of line n, and returns the count.
func (p *peerSends) tried(n uint64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.waiting {
		if p.waiting[i].n == n {
			p.waiting[i].tries++
			return p.waiting[i].tries
		}
	}
	return 0
}

func (p *peerSends) drop(n uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting = slices.DeleteFunc(p.waiting, func(l peerLine) bool { return l.n == n })
}

// forget drops every line that waits for m.
func (p *peerSends) forget(m chains.Member) (gone []peerLine) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting = slices.DeleteFunc(p.waiting, func(l peerLine) bool {
		if l.to.Same(m) {
			gone = append(gone, l)
			return true
		}
		return false
	})
	return gone
}

// check is nil when from may send to the pane to (the session toName) at
// now; else when it may. It drops the sends older than an hour. Callers
// hold mu.
func (p *peerSends) check(from, to, toName string, now time.Time) error {
	var kept []peerSent
	var mine []time.Time
	var last time.Time
	for _, e := range p.sent {
		if now.Sub(e.at) >= time.Hour {
			continue
		}
		kept = append(kept, e)
		if e.from != from {
			continue
		}
		mine = append(mine, e.at)
		if e.to == to && e.at.After(last) {
			last = e.at
		}
	}
	p.sent = kept
	if !last.IsZero() && now.Sub(last) < PeerPairEvery {
		next := last.Add(PeerPairEvery)
		return fmt.Errorf("one send to %s in %s: the next is allowed at %s (in %s)", toName, PeerPairEvery, next.Format("15:04:05"), next.Sub(now).Round(time.Second))
	}
	if len(mine) >= PeerSendsPerHour {
		next := mine[len(mine)-PeerSendsPerHour].Add(time.Hour)
		return fmt.Errorf("%d sends an hour from one session: the next is allowed at %s (in %s)", PeerSendsPerHour, next.Format("15:04:05"), next.Sub(now).Round(time.Second))
	}
	return nil
}

// peerRecord is the log line of a peer send. It holds the text's length
// and sha256, never the text.
type peerRecord struct {
	At       time.Time `json:"at"`
	From     string    `json:"from"`
	FromPane string    `json:"from_pane"`
	To       string    `json:"to"`
	ToPane   string    `json:"to_pane"`
	Len      int       `json:"len"`
	SHA256   string    `json:"sha256"`
	Via      string    `json:"via"`    // typed, typed at rest, inbox or at rest
	Result   string    `json:"result"` // sent, queued, typed, cleared, or typed, not sent
}

func sum(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// logPeer writes a peer send to PeerLog, and to stderr, which is serve.log
// when a console started the service.
func (s *Service) logPeer(r peerRecord) {
	fmt.Fprintf(os.Stderr, "matchblox: peer send %s via %s: %s (%s) to %s (%s), %d bytes, sha256 %s\n",
		r.Result, r.Via, r.From, r.FromPane, r.To, r.ToPane, r.Len, r.SHA256)
	if s.PeerLog == "" {
		return
	}
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.PeerLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // the service's own state folder
	if err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: peers.jsonl:", err)
		return
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: peers.jsonl:", err)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: peers.jsonl:", err)
	}
}
