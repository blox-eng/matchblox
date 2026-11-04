package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/proto"
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

// peerAct is the pane a peer send types into, when act a is one: a plain
// send (message:<pane>), peer sends are on ([peers] send), the service
// found the sender's pane itself, and that pane is not the stoker's.
func (s *Service) peerAct(a proto.Act) (string, bool) {
	pane, ok := strings.CutPrefix(a.RecID, "message:")
	if !ok || !s.PeerSend || !a.FromKnown || s.fromStoker(a) {
		return "", false
	}
	return pane, true
}

// peerSend sends one line from the session sender to the session in
// pane: through the first of PeerInboxes that reaches it, else typed, as
// "[from <sender>] <text>" and Enter. The tag is matchblox's, from
// the pane the service found for the sender. It goes only to a session at
// rest that waits for nobody and shows no menu, question or draft, read
// before the send and again right before each key, as a stoker step is.
// It is no stoker step: it counts in no run, drives no pane and gives no
// go.
func (s *Service) peerSend(ctx context.Context, sender sample.Session, pane string, a proto.Act) proto.Result {
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
	// One peer send at a time: the limits are checked and spent as one.
	s.peers.mu.Lock()
	defer s.peers.mu.Unlock()
	now := s.now()
	if err := s.peers.check(a.From, pane, x.Name, now); err != nil {
		return proto.Result{Err: err.Error()}
	}
	name := tagName(sender)
	rec := peerRecord{At: now, From: name, FromPane: a.From, To: x.Name, ToPane: pane, Len: len(a.Text), SHA256: sum(a.Text)}
	for _, in := range s.PeerInboxes {
		if !in.Reaches(x) {
			continue
		}
		// The agent's own inbox queues the message and names the sender
		// itself: no key goes to the pane, so no screen guard applies.
		if err := in.Deliver(ctx, PeerMessage{From: name, FromPane: a.From, To: x, Text: a.Text}); err != nil {
			return proto.Result{Err: "not delivered to " + x.Name + "'s " + in.Name() + ": " + err.Error()}
		}
		s.peers.sent = append(s.peers.sent, peerSent{from: a.From, to: pane, at: now})
		rec.Via, rec.Result = in.Name(), "sent"
		s.logPeer(rec)
		return proto.Result{Ran: [][]string{{in.Name(), x.Name}}}
	}
	tag := "[from " + name + "]"
	release := s.holdPane(pane)
	defer release()
	if err := s.peerReady(pane, x); err != nil {
		return proto.Result{Err: err.Error() + peerRetry}
	}
	res := s.Actions.Do(ctx, peerAction(pane, x, tag+" "+a.Text, tag), a.Confirm)
	if len(res.Ran) > 0 {
		s.latchRest(pane)
		s.peers.sent = append(s.peers.sent, peerSent{from: a.From, to: pane, at: now})
		rec.Via, rec.Result = "typed", "sent"
		if res.Err != "" || len(res.Ran) < 2 {
			rec.Result = "typed, not sent"
		}
		s.logPeer(rec)
	}
	if res.Err == "" && len(res.Ran) == 0 && len(res.Skipped) > 0 {
		res.Err, res.Skipped = res.Skipped[0]+peerRetry, nil
	}
	return res
}

// PeerMessage is one peer send for an inbox: the sender's name and pane,
// as matchblox found them, the receiving session and the text.
type PeerMessage struct {
	From, FromPane string
	To             sample.Session
	Text           string
}

// PeerInbox is an agent's own way to take a message from another session,
// where it has one: the message waits in the agent's queue, with its
// sender, instead of keys typed into its pane. A session no inbox reaches
// gets the typed send, which works for every agent.
type PeerInbox interface {
	// Name names the inbox in the log and in the result.
	Name() string
	// Reaches tells if the inbox delivers to session x.
	Reaches(x sample.Session) bool
	// Deliver hands m to the agent. An error delivered nothing.
	Deliver(ctx context.Context, m PeerMessage) error
}

// peerReady is the receiver's state a peer send needs, read now: the same
// agent, held by nobody, no question or permission prompt, waiting for
// nobody, at rest, and the pane stokable.
func (s *Service) peerReady(pane string, x sample.Session) error {
	if err := s.messageable(pane, x.PID); err != nil {
		return err
	}
	if it, ok := s.queued(pane); ok && stoker.Waits(it) {
		return errors.New(x.Name + " waits for you")
	}
	if !s.resting(pane) {
		return fmt.Errorf("%s is not at rest", x.Name)
	}
	return s.stokable(pane, "")
}

// peerAction types line and Enter into pane, each step guarded as a stoker
// step is: the same agent, at rest, nothing it asks, and the pane read
// again. Before the Enter the prompt shows the tag, where the agent's
// prompt can be read.
func peerAction(pane string, x sample.Session, line, tag string) advice.Action {
	g := advice.Guard{PID: x.PID, StartTicks: x.Start, RestPane: pane, StokePane: pane, MessagePane: pane, MessagePID: x.PID}
	typed := g
	if readsPrompt[x.Agent] {
		typed.Typed = tag
	}
	return advice.Action{Label: "peer send", Steps: panes.Send(pane, line), Destructive: true, Guards: []advice.Guard{g, typed}}
}

// checkPeerText refuses text that is not one line: a newline, a control
// key, a line or paragraph separator, or a bidi control that would make
// the line read as another.
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
	return unicode.IsControl(r) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Bidi_Control, r)
}

// tagName is the sender's name for its tag: no character that would end
// the tag or the line. Its pane when no name is left.
func tagName(x sample.Session) string {
	name := strings.Map(func(r rune) rune {
		if r == ']' || notInALine(r) {
			return -1
		}
		return r
	}, x.Name)
	if strings.TrimSpace(name) == "" {
		return x.Pane
	}
	return name
}

// peerSent is one peer send that typed.
type peerSent struct {
	from, to string
	at       time.Time
}

// peerSends are the peer sends of the last hour, oldest first. mu is held
// from a send's check to its record.
type peerSends struct {
	mu   sync.Mutex
	sent []peerSent
}

// check is nil when from may send to the pane to (the session toName) at
// now; else when it may. It drops the sends older than an hour.
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
	Via      string    `json:"via"`    // typed, or the inbox's name
	Result   string    `json:"result"` // sent, or typed, not sent
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
