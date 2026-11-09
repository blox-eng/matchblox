package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

const chainNeedsMatch = "chain needs a match: run it from an agent's pane"

// memberOf is the only way to make a Member: a hand-made one that lacks the
// session id would be Same as the session yet keep another inbox.
func memberOf(x sample.Session) chains.Member {
	return chains.Member{SessionID: x.SessionID, Pane: x.Pane, PID: x.PID, Start: x.Start}
}

// chainAct runs a chain: act from the agent in a.From: chain:<pane> asks,
// chain:- leaves, chain:off and chain:on. A stoker's pane does none.
func (s *Service) chainAct(ctx context.Context, a proto.Act) proto.Result {
	if _, stokerPane := s.unitOf(a); stokerPane {
		return refuseText("a stoker joins no chain")
	}
	res, changed := s.changeChains(strings.TrimPrefix(a.RecID, proto.ChainPrefix), a.From)
	s.followChains(ctx)
	if changed {
		s.broadcast()
	}
	return res
}

func (s *Service) changeChains(arg, fromPane string) (res proto.Result, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, ok := s.sessionAtLocked(fromPane)
	if !ok {
		return refuseText(chainNeedsMatch), false
	}
	st := s.chainsLocked()
	if s.refreshChainsLocked(st) { // the act finds a cleared chat by its new id
		changed = true
		s.cur.Chains = chainsView(st)
		if err := s.saveChainsLocked(); err != nil {
			return proto.Result{Err: "chains: " + err.Error()}, true
		}
	}
	me := memberOf(from)
	before := slices.Clone(st.Chains)
	absorbed, survivor := "", ""
	switch arg {
	case "-":
		if _, in := st.ChainOf(me); !in {
			return proto.Result{Note: "not in a chain"}, changed
		}
		st.Leave(me)
		res.Note = "left the chain"
	case "off":
		st.SetOff(me, true)
		res.Note = "off: no notices, and no chains"
	case "on":
		st.SetOff(me, false)
		res.Note = "on"
	default:
		if s.sampleMissedLocked() { // its target may be one the sample did not read
			return refuseText("not now: matchblox could not read the sessions; try again"), changed
		}
		to, ok := s.sessionAtLocked(arg)
		if !ok {
			return refuseText("no agent session in " + arg + " any more"), changed
		}
		repo, why := s.chainable(st, from, to)
		if why != "" {
			return refuseText(why), changed
		}
		if c, ok := st.ChainOf(me); ok && slices.ContainsFunc(c.Members, memberOf(to).Same) {
			return proto.Result{Note: "already chained with " + to.Name}, changed
		}
		formed := false
		if formed, absorbed = st.Ask(me, memberOf(to), repo, s.now()); formed {
			c, _ := st.ChainOf(me)
			survivor = c.ID
			res.Note = fmt.Sprintf("chained with %s: %d matches", to.Name, len(c.Members))
		} else {
			res.Note = "asked " + to.Name + ": the chain forms when it chains back"
		}
	}
	s.noteChainsLocked(before, absorbed, survivor)
	s.cur.Chains = chainsView(st)
	if err := s.saveChainsLocked(); err != nil {
		res.Err = "chains: " + err.Error()
	}
	return res, true
}

// chainable is the repo of from, "" for none, or why from and to cannot
// chain. A chain is one host; its matches may work in any repo, or none.
// Callers hold mu.
func (s *Service) chainable(st *chains.Store, from, to sample.Session) (repo, why string) {
	switch {
	case memberOf(from).Same(memberOf(to)):
		return "", "a match cannot chain with itself"
	case to.Host != from.Host:
		return "", to.Name + " is on another host: a chain is one host"
	}
	for _, x := range []sample.Session{from, to} {
		if slices.ContainsFunc(st.Off, memberOf(x).Same) {
			return "", x.Name + " is off: it takes no chains"
		}
	}
	repo, _, _ = gitscan.RepoOf(s.cur.Git, from.Cwd)
	return repo, ""
}

// pruneChainsLocked keeps up the members whose session id changed in their
// pane, drops the members and asks the sessions no longer hold, tells the
// mates of a match that left, and saves on a change. prev are the sessions
// of the sample before, which name a match that is gone. Callers hold mu.
func (s *Service) pruneChainsLocked(now time.Time, prev []sample.Session) error {
	st := s.chainsLocked()
	if s.sampleMissedLocked() {
		return nil // every match would look gone
	}
	refreshed := s.refreshChainsLocked(st)
	before, asks, offs := slices.Clone(st.Chains), len(st.Requests), len(st.Off)
	live := func(m chains.Member) bool {
		_, ok := s.sessionOfLocked(m)
		return ok
	}
	left := st.Prune(live, now)
	maps.DeleteFunc(s.cleared, func(id string, _ bool) bool {
		return !slices.ContainsFunc(s.cur.Sessions, func(x sample.Session) bool { return x.SessionID == id })
	})
	s.noteChainsLocked(before, "", "")
	for _, l := range left {
		text := s.leftText(l, prev)
		for _, c := range before {
			if c.ID != l.Chain {
				continue
			}
			for _, m := range c.Members {
				if !slices.ContainsFunc(left, func(o chains.Left) bool { return o.Who.Same(m) }) {
					st.Post(m, text, now)
				}
			}
		}
	}
	if !refreshed && len(left) == 0 && len(st.Requests) == asks && len(st.Off) == offs {
		return nil
	}
	return s.saveChainsLocked()
}

// refreshChainsLocked renames each stored member whose pane, pid and start
// a session holds now under another id (a cleared chat), and tells if any
// changed. A chained member that a clear renamed is told its chain at its
// next news. A sample that read no id keeps the one stored, so a clear seen
// through it is still a clear. A partial session list renames no one
// wrongly. Callers hold mu.
func (s *Service) refreshChainsLocked(st *chains.Store) bool {
	return st.Refresh(func(m chains.Member) (chains.Member, bool) {
		for _, x := range s.cur.Sessions {
			if x.Pane != "" && x.Pane == m.Pane && x.PID == m.PID && x.Start == m.Start {
				n := memberOf(x)
				if n.SessionID == "" {
					n.SessionID = m.SessionID
				}
				if _, chained := st.ChainOf(m); chained && m.SessionID != "" && n.SessionID != m.SessionID {
					if s.cleared == nil {
						s.cleared = map[string]bool{}
					}
					s.cleared[n.SessionID] = true
				}
				return n, true
			}
		}
		return chains.Member{}, false
	})
}

// leftText is the news of l: the match is gone. Callers hold mu.
func (s *Service) leftText(l chains.Left, prev []sample.Session) string {
	name := l.Who.Pane
	if x, ok := s.sessionOfLocked(l.Who); ok {
		name = x.Name
	} else if i := slices.IndexFunc(prev, func(x sample.Session) bool { return memberOf(x).Same(l.Who) }); i >= 0 {
		name = prev[i].Name
	}
	return name + " left the chain"
}

// sampleMissedLocked tells if the last sample did not read its sessions.
// Callers hold mu.
func (s *Service) sampleMissedLocked() bool {
	return slices.ContainsFunc(s.cur.Errors, func(e string) bool {
		return strings.HasPrefix(e, "tmux: ") || strings.HasPrefix(e, "proc: ")
	})
}

// chainsLocked is the store, read from ChainsPath at first use. Callers hold mu.
func (s *Service) chainsLocked() *chains.Store {
	if s.Chains == nil {
		s.Chains = loadChains(s.ChainsPath)
	}
	return s.Chains
}

// loadChains reads path. A file that does not read is moved aside to
// path.bad, and the chains start empty.
func loadChains(path string) *chains.Store {
	if path == "" {
		return &chains.Store{}
	}
	st, err := chains.Load(path)
	if err == nil {
		return st
	}
	fmt.Fprintln(os.Stderr, "matchblox: chains.json did not read, so chains start empty:", err)
	if err := os.Rename(path, path+".bad"); err != nil {
		fmt.Fprintln(os.Stderr, "matchblox: chains.json:", err)
	}
	return &chains.Store{}
}

func (s *Service) saveChainsLocked() error {
	if s.ChainsPath == "" {
		return nil
	}
	return s.Chains.Save(s.ChainsPath)
}

// chainsView copies what the doc shows, so a later change to the store does
// not reach a doc already sent. Nil: no chain and no ask.
func chainsView(st *chains.Store) *state.ChainsView {
	if len(st.Chains) == 0 && len(st.Requests) == 0 {
		return nil
	}
	v := &state.ChainsView{Chains: slices.Clone(st.Chains), Requests: slices.Clone(st.Requests)}
	for i := range v.Chains {
		v.Chains[i].Members = slices.Clone(v.Chains[i].Members)
	}
	return v
}

// elsewhereLocked is what a line from from to to adds when they work in
// different repos: " (in <repo>)", or " (in no repo matchblox knows)". "" when they share
// one, or either is gone. Callers hold mu.
func (s *Service) elsewhereLocked(from, to chains.Member) string {
	x, okFrom := s.sessionOfLocked(from)
	y, okTo := s.sessionOfLocked(to)
	if !okFrom || !okTo {
		return ""
	}
	repo, _, _ := gitscan.RepoOf(s.cur.Git, x.Cwd)
	if other, _, _ := gitscan.RepoOf(s.cur.Git, y.Cwd); repo == other {
		return ""
	}
	if repo == "" {
		return " (in no repo matchblox knows)"
	}
	return " (in " + gitscan.Name(s.cur.Git, repo) + ")"
}

func (s *Service) sessionOfLocked(m chains.Member) (sample.Session, bool) {
	for _, x := range s.cur.Sessions {
		if memberOf(x).Same(m) {
			return x, true
		}
	}
	return sample.Session{}, false
}

// awaitJoin marks the session that chain unit u started in pane, in the
// tmux session name, to join u's chain once a sample finds it there. No
// pane: nothing tells the session, and it joins nothing.
func (s *Service) awaitJoin(u *stokerUnit, pane, name string) {
	if u == nil || s.chainIDOf(u) == "" || pane == "" {
		return
	}
	s.mu.Lock()
	u.joins = append(u.joins, &pendingJoin{pane: pane, name: name, at: s.now()})
	s.mu.Unlock()
}

// joinNewLocked puts each session a chain unit started in its chain, once
// the sample finds its agent in the pane the start made, in the tmux
// session of its name and in the chain's repo. A pane that is gone, or now
// holds another session or another agent, is let go, and so is one not
// found within stokerStartsIn. It saves on a join. Callers hold mu, and
// call joinedOut once they let it go: the joins are the units a session
// joined, and the lines what each failed join says on its unit's card.
func (s *Service) joinNewLocked(now time.Time) (out joins, err error) {
	if s.cur.Git == nil || s.sampleMissedLocked() {
		return out, nil
	}
	st := s.chainsLocked()
	for _, u := range s.family() {
		u.joins = slices.DeleteFunc(u.joins, func(j *pendingJoin) bool {
			late := now.Sub(j.at) >= stokerStartsIn
			if s.cur.At.Before(j.at) {
				return late // a sample from before the start
			}
			p := slices.IndexFunc(s.cur.Panes, func(p sample.Pane) bool { return p.ID == j.pane })
			if p < 0 {
				return true // gone
			}
			if name, _, _ := strings.Cut(s.cur.Panes[p].Target, ":"); name != j.name {
				return true // the pane moved to another session
			}
			i := slices.IndexFunc(s.cur.Sessions, func(x sample.Session) bool { return x.Pane == j.pane })
			if i < 0 {
				return late
			}
			x := s.cur.Sessions[i]
			switch {
			case x.TmuxSession() != j.name:
				return true
			case j.sessionID == "":
				j.sessionID = x.SessionID
			case x.SessionID != j.sessionID:
				return true // another agent runs there now
			}
			c, ok := chainByID(st, s.chainIDOf(u))
			if ok {
				if repo, _, _ := gitscan.RepoOf(s.cur.Git, x.Cwd); repo != c.Repo {
					return late
				}
			}
			if err := st.Join(s.chainIDOf(u), memberOf(x)); err != nil {
				text := fmt.Sprintf("%s did not join chain %s: %s", x.Name, chainName(c, s.cur.Sessions), err)
				if !ok {
					text = fmt.Sprintf("%s did not join chain %s: %s", x.Name, s.chainIDOf(u), err)
				}
				out.lines = append(out.lines, unitLine{u, familyLine(now, text)})
				return true
			}
			if !slices.Contains(out.units, u) {
				out.units = append(out.units, u)
			}
			return true
		})
	}
	if len(out.units) == 0 {
		return out, nil
	}
	return out, s.saveChainsLocked()
}

// joins is what joinNewLocked leaves to do out of mu.
type joins struct {
	units []*stokerUnit
	lines []unitLine
}

type unitLine struct {
	u *stokerUnit
	e stoker.Entry
}

// joinedOut writes the cards' lines of j, and hands each unit whose run
// is on the host's work on its new matches. It runs out of mu.
func (s *Service) joinedOut(j joins) {
	for _, l := range j.lines {
		_ = l.u.store.Append(l.e)
	}
	for _, u := range j.units {
		if u.store.Mode().On {
			s.handOver(u)
		}
	}
}

func chainByID(st *chains.Store, id string) (chains.Chain, bool) {
	i := slices.IndexFunc(st.Chains, func(c chains.Chain) bool { return c.ID == id })
	if i < 0 {
		return chains.Chain{}, false
	}
	return st.Chains[i], true
}

// chainEvent is what a change of the chains did to one chain: it ended,
// or it joined survivor. name is its name before the change.
type chainEvent struct {
	chain, survivor, name string
}

// noteChainsLocked queues for followChains what a change did to the chains
// before it: absorbed joined survivor, and each other chain that is gone
// ended. Callers hold mu.
func (s *Service) noteChainsLocked(before []chains.Chain, absorbed, survivor string) {
	st := s.chainsLocked()
	for _, c := range before {
		switch {
		case c.ID == absorbed:
			s.chainLife = append(s.chainLife, chainEvent{chain: c.ID, survivor: survivor, name: chainName(c, s.cur.Sessions)})
		case !slices.ContainsFunc(st.Chains, func(o chains.Chain) bool { return o.ID == c.ID }):
			s.chainLife = append(s.chainLife, chainEvent{chain: c.ID, name: chainName(c, s.cur.Sessions)})
		}
	}
}

// followChains makes each chain stoker follow what happened to its chain
// since the last call: the stoker of a chain that ended stops, and the
// stokers of two chains that joined become one. It runs out of mu.
func (s *Service) followChains(ctx context.Context) {
	s.mu.Lock()
	events := s.chainLife
	s.chainLife = nil
	s.mu.Unlock()
	for _, e := range events {
		if e.survivor == "" {
			s.chainEnded(ctx, e.chain)
		} else {
			s.chainsJoined(ctx, e.survivor, e.chain, e.name)
		}
	}
}

// chainEnded stops the stoker of a chain that ended: it says so on its
// card, its run ends and its agent quits.
func (s *Service) chainEnded(ctx context.Context, chainID string) {
	if u := s.unitOfChain(chainID); u != nil {
		s.retire(ctx, u, "its chain ended", "its chain ended")
	}
}

// chainsJoined follows the join of the chain absorbed, named name, into
// survivor. A stoker of absorbed alone takes survivor as its chain. With a
// stoker on each side, the stoker of survivor stays: the other says so on
// its card and stops, and its running order goes to the one that stays,
// with only the goes both had. The stoker that stays takes over the
// host's work on its new matches.
func (s *Service) chainsJoined(ctx context.Context, survivor, absorbed, name string) {
	a, b := s.unitOfChain(absorbed), s.unitOfChain(survivor)
	switch {
	case a == nil && b == nil:
		return
	case b == nil: // the stoker of absorbed stays, unless a start took survivor meanwhile
		if b = s.takeChain(a, survivor); b != a {
			s.yieldTo(ctx, a, b, name)
		}
	case a != nil:
		s.yieldTo(ctx, a, b, name)
	}
	if b.store.Mode().On {
		s.handOver(b)
	}
	s.refreshStoker()
}

// takeChain makes chain unit u the stoker of chain, which has none: under
// the start's lock, so no start gives chain a second one. It is the unit
// of chain after: u, or the one a start made first.
func (s *Service) takeChain(u *stokerUnit, chain string) *stokerUnit {
	s.fam.start.Lock()
	defer s.fam.start.Unlock()
	if o := s.unitOfChain(chain); o != nil {
		return o
	}
	if err := s.relinkChain(u, chain); err != nil {
		_ = u.store.Append(familyLine(s.now(), fmt.Sprintf("its chain joined %s, and its link did not change: %s", s.chainNameOf(chain), err)))
	}
	return u
}

// yieldTo stops chain unit a, whose chain named name joined the chain of
// b. The order a runs goes to b, with only the goes both have: a join
// never gives b a go it did not have. A run of b that is off starts with
// it.
func (s *Service) yieldTo(ctx context.Context, a, b *stokerUnit, name string) {
	words := ""
	if m := a.store.Mode(); m.On {
		if o := runningOrder(m); o != nil {
			words = strings.TrimSpace(o.Words)
		}
	}
	grants := stoker.Given(s.grantsOf(b), s.grantsOf(a)) // before a's run ends
	joined := s.chainNameOf(s.chainIDOf(b))
	s.retire(ctx, a, "its chain joined "+joined, "the stoker of chain "+joined+" owns it now")
	if words == "" {
		return
	}
	if s.nightOf(b) { // the night is sealed: no order reaches it
		_ = b.store.Append(familyLine(s.now(), fmt.Sprintf("the order of the joined chain %s waits for the morning: %s", name, words)))
		return
	}
	var err error
	if b.store.Mode().On {
		if err = s.rebrief(ctx, b); err == nil {
			line := fmt.Sprintf("New order (from the joined chain %s): %s", name, strings.Join(strings.Fields(words), " "))
			err = s.sayOrder(ctx, b, words, line, grants, s.now())
		}
	} else {
		s.fam.mu.Lock()
		giver := b.link.Giver
		s.fam.mu.Unlock()
		err = s.rerunChain(ctx, b, words, grants, giver)
	}
	if err != nil {
		_ = b.store.Append(familyLine(s.now(), fmt.Sprintf("the order of the joined chain %s did not reach this stoker: %s", name, err)))
	}
}

// nightOf: a run of chain unit u is a night run: the one on, or the one an
// order would start while the host's night runs.
func (s *Service) nightOf(u *stokerUnit) bool {
	if m := u.store.Mode(); m.On {
		return m.Nightly
	}
	h := s.hostUnit()
	if h == nil {
		return false
	}
	m := h.store.Mode()
	return m.On && m.Nightly
}

// retire stops chain unit u for good with text on its card: its link says
// it ended, each kill or go that waits for a y is dropped with why, its run
// ends and its agent quits. Its dir and card stay, for the morning card and
// the STOKER tab; the cap counts it no more, and no chain is its again.
func (s *Service) retire(ctx context.Context, u *stokerUnit, text, why string) {
	now := s.now()
	_ = u.store.Append(familyLine(now, text))
	if err := s.endLink(u); err != nil {
		_ = u.store.Append(familyLine(now, "its link did not change: "+err.Error()))
	}
	s.dropWaits(u, func(pane, _ string) (string, bool) {
		if x, ok := s.sessionAt(pane); ok && x.Name != "" {
			return x.Name, true
		}
		return pane, true
	}, why)
	if res := s.stokerAct(u, "off", now); res.Err != "" {
		_ = u.store.Append(familyLine(now, "its run did not end: "+res.Err))
	}
	if err := s.quitAgent(ctx, u); err != nil {
		_ = u.store.Append(familyLine(now, "its agent did not quit: "+err.Error()))
	}
}

// quitAgent ends chain unit u's agent and its tmux session, when it is
// there, with what waited to be typed into it or to join its chain.
func (s *Service) quitAgent(ctx context.Context, u *stokerUnit) error {
	if s.StokerServer == "" || !s.stokerThere(ctx, u) {
		return nil
	}
	if res := s.Actions.Do(ctx, advice.Action{Label: "quit the stoker", Steps: [][]string{s.stokerSessionArgv(u, "kill-session")}, Destructive: true}, "y"); res.Err != "" {
		return errors.New(res.Err)
	}
	s.mu.Lock()
	u.startedAt, u.brief, u.dropped, u.joins = time.Time{}, nil, nil, nil
	s.mu.Unlock()
	return nil
}
