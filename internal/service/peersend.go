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
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
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

// peerRetry ends a refusal of a peer send for the receiver's state: what
// the sender can do instead.
const peerRetry = ": try again when it is idle, or write the message to a file and ask the person at the console to relay it"

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
	if x.Agent == "opencode" {
		return proto.Result{Err: x.Name + " runs OpenCode, which matchblox cannot type into safely. It sees the chain in `matchblox chain`."}
	}
	// Not into the inbox either: the agent would read it in its refresh turn.
	if err := s.switchingErr(pane, nil); err != nil {
		return proto.Result{Err: err.Error()}
	}
	now := s.now()
	slot, err := s.peers.reserve(a.From, pane, x.Name, now)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	tag := peerTag(sender)
	line := tag + " " + a.Text
	rec := peerRecord{At: now, From: sender.Name, FromPane: a.From, To: x.Name, ToPane: pane, Len: len(a.Text), SHA256: sum(a.Text)}
	queued := proto.Result{Note: "queued for " + x.Name + "'s next step"}
	codex := x.Agent == "codex"
	// A Codex that rests takes one typed line at a time: a send while one
	// types waits for its next rest.
	typing := codex && s.resting(pane) && !s.peers.waits(memberOf(x)) && s.peers.claim(pane)
	if typing {
		defer s.peers.unclaim(pane)
	}
	switch {
	case !readsPrompt[x.Agent], !codex && !s.resting(pane):
		s.postMessage(chainID, x, line, now)
		rec.Via, rec.Result = "inbox", "queued"
		s.logPeer(rec)
		return queued
	case codex && !typing:
		s.peers.queue(peerLine{from: memberOf(sender), to: memberOf(x), line: line, tag: tag, rec: rec})
		s.spark(chainID, now)
		rec.Via, rec.Result = "at rest", "queued"
		s.logPeer(rec)
		return queued
	}
	res, typed, _, _ := s.typeLine(ctx, x, line, tag, peerLead, nil)
	if !typed {
		s.peers.release(slot)
		return res
	}
	if res.Err == "" {
		s.spark(chainID, now)
	}
	rec.Via, rec.Result = "typed", typedResult(res)
	s.logPeer(rec)
	return res
}

// peerTries is how many rests a waiting line gets to show in the prompt,
// and peerStale how long it waits for one.
const (
	peerTries = 3
	peerStale = 10 * time.Minute
)

// deliverAtRest types the oldest line that waits for each Codex match that
// rests now: one line a pane each sample. A line refused before any key
// waits for a later rest, and so does one typed and cleared, up to
// peerTries; then its sender hears it did not go. A line that may still be
// in the prompt is dropped, so it is never typed twice; one for a match
// that is gone is dropped too. So is one that waited peerStale, or whose
// sender is no longer chained with its match or may no longer send, and
// its sender hears why.
func (s *Service) deliverAtRest(now time.Time) {
	for _, l := range s.peers.firsts() {
		x, ok := s.sessionOf(l.to)
		if !ok {
			s.peers.forget(l.to)
			continue
		}
		why := ""
		if _, chained := s.mateChain(l.from, l.to); !chained {
			why = "you are no longer chained"
		} else if s.ChainsSend == "off" {
			why = "sends are off on this host"
		} else if now.Sub(l.rec.At) >= peerStale {
			why = fmt.Sprintf("it waited %d minutes and went stale", int(peerStale.Minutes()))
		}
		if why != "" {
			s.peers.drop(l.n)
			s.mu.Lock()
			s.chainsLocked().Post(l.from, didNotGet(x.Name, why), now)
			s.mu.Unlock()
			continue
		}
		if !s.resting(x.Pane) || !s.peers.claim(x.Pane) {
			continue
		}
		res, typed, cleared, stuck := s.typeLine(context.Background(), x, l.line, l.tag, peerLead, nil)
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
// and Enter only once the prompt shows a line that starts with lead. A
// line that does not show in peerShowWait is cleared. typed tells if any
// key reached the pane, cleared if the line typed did not go and is cleared
// from the prompt, and stuck if it did not go and its clear failed: it may
// still be there. own is the refresh that types line as its request: only
// it passes the pane's switching mark. It is nil for any other line.
func (s *Service) typeLine(ctx context.Context, x sample.Session, line, tag, lead string, own *refreshRun) (res proto.Result, typed, cleared, stuck bool) {
	pane := x.Pane
	defer s.holdPane(pane)()
	if err := s.peerReady(pane, x, own); err != nil {
		return proto.Result{Err: err.Error()}, false, false, false
	}
	switch mode, err := s.paneInMode(pane); {
	case err != nil:
		return proto.Result{Err: "not delivered: " + x.Name + "'s pane cannot be read: " + err.Error()}, false, false, false
	case mode:
		return proto.Result{Err: "not delivered: " + x.Name + "'s pane is in copy mode"}, false, false, false
	}
	run := s.Actions
	if own != nil {
		run.Messageable = func(pane string, pid int) error { return s.messageableFor(pane, pid, own) }
		run.Stokable = func(pane, typed string) error { return s.stokableFor(pane, typed, own) }
	}
	g := advice.Guard{PID: x.PID, StartTicks: x.Start, RestPane: pane, StokePane: pane, MessagePane: pane, MessagePID: x.PID}
	res = run.Do(ctx, peerStep(panes.Literal(pane, line), g), "y")
	if len(res.Ran) == 0 {
		return refusedStep(res), false, false, false
	}
	if !s.showsLine(ctx, pane, lead) {
		cleared = s.clearLine(ctx, pane)
		return proto.Result{Ran: res.Ran, Err: "not delivered: " + x.Name + " did not show the line"}, true, cleared, !cleared
	}
	g.Typed = tag
	enter := run.Do(ctx, peerStep(panes.Keys(pane, "Enter"), g), "y")
	res.Ran = append(res.Ran, enter.Ran...)
	if len(enter.Ran) == 0 {
		// The line stays the mate's draft unless it goes: clear it while it is
		// still ours alone.
		if screen, ok := s.Capture([]string{pane})[pane]; ok && strings.HasPrefix(said.Draft(screen), tag) {
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
		res.Err, res.Skipped = res.Skipped[0]+peerRetry, nil
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

// showsLine reads the pane until its prompt shows a line that starts with
// lead, for up to peerShowWait.
func (s *Service) showsLine(ctx context.Context, pane, lead string) bool {
	deadline := time.Now().Add(peerShowWait)
	for {
		if screen, ok := s.Capture([]string{pane})[pane]; ok && strings.HasPrefix(said.Draft(screen), lead) {
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
	res := s.Actions.Do(context.WithoutCancel(ctx), advice.Action{Label: "peer send", Steps: [][]string{panes.Keys(pane, "C-u")}, Destructive: true}, "y")
	if res.Err != "" {
		fmt.Fprintln(os.Stderr, "matchblox: peer send: the line in", pane, "is not cleared:", res.Err)
		return false
	}
	return true
}

// paneInMode tells if tmux shows pane in copy mode or another mode, where
// keys move a cursor instead of reaching the agent.
func (s *Service) paneInMode(pane string) (bool, error) {
	tmux, id := sample.Tmux(pane)
	out, err := s.tmuxOut(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{pane_in_mode}"}))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// mateChain is the id of the chain from and to are both in.
func (s *Service) mateChain(from, to chains.Member) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chainsLocked().ChainOf(from)
	if !ok || !slices.ContainsFunc(c.Members, to.Same) {
		return "", false
	}
	return c.ID, true
}

// postMessage puts a send's line in x's inbox and sparks the chain.
func (s *Service) postMessage(chainID string, x sample.Session, line string, now time.Time) {
	s.mu.Lock()
	s.chainsLocked().Post(memberOf(x), line, now)
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

// peerReady is the receiver's state typing needs, read now: the same
// agent, held by nobody, no question or permission prompt, waiting for
// nobody, at rest, and no draft in its prompt. Only own, a refresh that
// types its own request, passes the pane's switching mark.
func (s *Service) peerReady(pane string, x sample.Session, own *refreshRun) error {
	if err := s.switchingErr(pane, own); err != nil {
		return err
	}
	if err := s.messageableFor(pane, x.PID, own); err != nil {
		return errors.New(err.Error() + peerRetry)
	}
	if it, ok := s.queued(pane); ok && stoker.Waits(it) {
		return errors.New(x.Name + " waits for you" + peerRetry)
	}
	if !s.resting(pane) {
		return errors.New(x.Name + " is not at rest" + peerRetry)
	}
	screen, ok := s.Capture([]string{pane})[pane]
	if !ok {
		return fmt.Errorf("the pane %s cannot be read%s", pane, peerRetry)
	}
	if _, kind := said.Screen(screen); kind == said.None && said.Draft(screen) != "" {
		return errors.New("not delivered: " + x.Name + " has a draft in its prompt")
	}
	if err := stokableScreen(pane, screen, ""); err != nil {
		return errors.New(err.Error() + peerRetry)
	}
	return nil
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

func peerTag(x sample.Session) string { return peerLead + tagName(x) + "]" }

// peerLead starts every peer line, as the prompt shows it.
const peerLead = "[from "

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
func (p *peerSends) forget(m chains.Member) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting = slices.DeleteFunc(p.waiting, func(l peerLine) bool { return l.to.Same(m) })
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
