package service

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

const chainNeedsMatch = "chain needs a match: run it from an agent's pane"

// memberOf is the only way to make a Member: a hand-made one that lacks the
// session id would be Same as the session yet keep another inbox.
func memberOf(x sample.Session) chains.Member {
	return chains.Member{SessionID: x.SessionID, Pane: x.Pane, PID: x.PID, Start: x.Start}
}

// chainAct runs a chain: act. From an agent's pane: chain:<pane> joins
// the team of that match, or forms one with it; chain:@<team> joins a team
// by name; chain:- leaves the only team, chain:-@<team> that team and
// chain:-* every team; chain:off and chain:on. From the console only:
// chain:out:<pane>@<team> takes that match out of that team.
func (s *Service) chainAct(a proto.Act) proto.Result {
	res, changed := s.changeChains(strings.TrimPrefix(a.RecID, proto.ChainPrefix), a)
	if changed {
		s.broadcast()
	}
	return res
}

const noStokerChain = "the stoker joins no chain"

// offText is the refusal for a match that is off.
func offText(name string) string {
	return name + " is off: it takes no chains. It can run: matchblox chain --on"
}

// runOne is the refusal that lists one command per team, as matchblox
// never picks a team itself.
func runOne(lead, verb string, teams []chains.Chain, more ...string) string {
	lines := []string{lead + ". Run one:"}
	for _, c := range teams {
		lines = append(lines, "  matchblox "+verb+" "+c.Name)
	}
	for _, m := range more {
		lines = append(lines, "  "+m)
	}
	return strings.Join(lines, "\n")
}

// andList is "a", "a and b", or "a, b and c".
func andList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func teamNames(cs []chains.Chain) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func (s *Service) changeChains(arg string, a proto.Act) (res proto.Result, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.chainsLocked()
	if s.refreshChainsLocked(st) { // the act finds a cleared chat by its new id
		changed = true
		s.cur.Chains = chainsView(st)
		if err := s.saveChainsLocked(); err != nil {
			return proto.Result{Err: "chains: " + err.Error()}, true
		}
	}
	var did bool
	if out, ok := strings.CutPrefix(arg, "out:"); ok {
		res, did = s.takeOutLocked(st, out, a)
	} else {
		from, ok := s.sessionAtLocked(a.From)
		if !ok {
			return refuseText(chainNeedsMatch), changed
		}
		res, did = s.chainFromLocked(st, arg, from, a)
	}
	if !did {
		return res, changed
	}
	s.cur.Chains = chainsView(st)
	if err := s.saveChainsLocked(); err != nil {
		res.Err = "chains: " + err.Error()
	}
	return res, true
}

// chainFromLocked runs a chain act of the match from, and tells if it
// changed the chains. Callers hold mu.
func (s *Service) chainFromLocked(st *chains.Store, arg string, from sample.Session, a proto.Act) (proto.Result, bool) {
	me := memberOf(from)
	teams := st.TeamsOf(me)
	switch {
	case arg == "-":
		switch len(teams) {
		case 0:
			return proto.Result{Note: "not in a chain"}, false
		case 1:
			st.LeaveTeam(me, teams[0].Name)
			return proto.Result{Note: "left " + teams[0].Name}, true
		}
		return refuseText(runOne("you are in "+andList(teamNames(teams)), "unchain", teams, "matchblox unchain --all")), false
	case arg == "-*":
		if len(teams) == 0 {
			return proto.Result{Note: "not in a chain"}, false
		}
		st.Leave(me)
		return proto.Result{Note: "left " + andList(teamNames(teams))}, true
	case strings.HasPrefix(arg, "-@"):
		team := strings.TrimPrefix(arg, "-@")
		if !st.LeaveTeam(me, team) {
			return proto.Result{Note: "not in " + team}, false
		}
		return proto.Result{Note: "left " + team}, true
	case arg == "off":
		st.SetOff(me, true)
		return proto.Result{Note: "off: no notices, and no chains"}, true
	case arg == "on":
		st.SetOff(me, false)
		return proto.Result{Note: "on"}, true
	case s.fromStoker(a):
		return refuseText(noStokerChain), false
	case strings.HasPrefix(arg, "@"):
		team := strings.TrimPrefix(arg, "@")
		if _, ok := st.Team(team); !ok {
			return refuseText("no chain is named " + team), false
		}
		return s.joinedLocked(st, from, team, func() (chains.Chain, error) { return st.Join(me, team) })
	}
	if s.sampleMissedLocked() { // its target may be one the sample did not read
		return refuseText("not now: matchblox could not read the sessions; try again"), false
	}
	if s.fromStoker(proto.Act{From: arg}) {
		return refuseText(noStokerChain), false
	}
	to, ok := s.sessionAtLocked(arg)
	if !ok {
		return refuseText("no agent session in " + arg + " any more"), false
	}
	switch theirs := st.TeamsOf(memberOf(to)); len(theirs) {
	case 0:
		repo, why := s.chainable(st, from, to)
		if why != "" {
			return refuseText(why), false
		}
		return s.joinedLocked(st, from, from.Name, func() (chains.Chain, error) {
			return st.Form(me, memberOf(to), from.Name, repo)
		})
	case 1:
		return s.joinedLocked(st, from, theirs[0].Name, func() (chains.Chain, error) { return st.Join(me, theirs[0].Name) })
	default:
		return refuseText(runOne(fmt.Sprintf("%s is in %d chains", to.Name, len(theirs)), "chain", theirs)), false
	}
}

// joinedLocked runs join, by which from joins or forms team, and tells the
// other members and the consoles. Callers hold mu.
func (s *Service) joinedLocked(st *chains.Store, from sample.Session, team string, join func() (chains.Chain, error)) (proto.Result, bool) {
	c, err := join()
	switch {
	case errors.Is(err, chains.ErrIn):
		return proto.Result{Note: "already in " + team}, false
	case errors.Is(err, chains.ErrOff):
		return refuseText(offText(from.Name)), false
	case err != nil:
		return refuseText(err.Error()), false
	}
	me, now := memberOf(from), s.now()
	for _, o := range c.Members {
		if !o.Same(me) {
			st.Post(o, "[matchblox] "+from.Name+" joined the chain"+s.elsewhereLocked(me, o), now)
		}
	}
	st.Spark(c.ID, now)
	return proto.Result{Note: fmt.Sprintf("joined %s: %d matches. To leave it, run: matchblox unchain %s", c.Name, len(c.Members), c.Name)}, true
}

// takeOutLocked takes the match in pane out of team: out is <pane>@<team>.
// Only the person at the console may. Callers hold mu.
func (s *Service) takeOutLocked(st *chains.Store, out string, a proto.Act) (proto.Result, bool) {
	if _, agent := s.sessionAtLocked(a.From); !a.Console || agent {
		return refuseText("only the person at the console takes a match out of a chain"), false
	}
	pane, team, _ := strings.Cut(out, "@")
	c, ok := st.Team(team)
	if !ok {
		return refuseText("no chain is named " + team), false
	}
	i := slices.IndexFunc(c.Members, func(m chains.Member) bool { return m.Pane == pane })
	if i < 0 {
		return refuseText("no match in " + pane + " is in " + team), false
	}
	m, name := c.Members[i], pane
	if x, ok := s.sessionOfLocked(m); ok {
		name = x.Name
	}
	st.LeaveTeam(m, team)
	st.Post(m, "[matchblox] the owner took you out of "+team, s.now())
	return proto.Result{Note: "took " + name + " out of " + team}, true
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
			return "", offText(x.Name)
		}
	}
	repo, _, _ = gitscan.RepoOf(s.cur.Git, from.Cwd)
	return repo, ""
}

// pruneChainsLocked keeps up the members whose session id changed in their
// pane, keeps a member whose pane lives but shows no agent (unseen since its
// first miss), drops a member whose pane is gone or shows another agent that
// no switch started, tells the mates of a match
// that left, and saves on a change. prev are the sessions
// of the sample before, which name a match that is gone. Callers hold mu.
func (s *Service) pruneChainsLocked(now time.Time, prev []sample.Session) error {
	st := s.chainsLocked()
	s.tellLostLocked(st, now)
	if s.sampleMissedLocked() {
		return nil // every match would look gone
	}
	refreshed := s.refreshChainsLocked(st)
	before, offs, unseen := slices.Clone(st.Chains), len(st.Off), maps.Clone(st.Unseen)
	panes := map[string]bool{}
	for _, p := range s.cur.Panes {
		panes[p.ID] = true
	}
	if s.mateNames == nil {
		s.mateNames = map[string]string{}
	}
	for _, x := range slices.Concat(prev, s.cur.Sessions) {
		if len(st.TeamsOf(memberOf(x))) > 0 {
			s.mateNames[x.Pane] = x.Name
		}
	}
	live := func(m chains.Member) bool {
		if x, ok := s.sessionOfLocked(m); ok {
			st.MarkSeen(m)
			s.mateNames[m.Pane] = x.Name
			return true
		}
		if !panes[m.Pane] {
			return false
		}
		if _, other := s.sessionAtLocked(m.Pane); other {
			if _, moving := st.Moving(m); !moving {
				return false // another agent runs in its pane: it is not this match
			}
		}
		st.MarkUnseen(m, now)
		return true
	}
	left := st.Prune(live, now)
	defer maps.DeleteFunc(s.mateNames, func(pane, _ string) bool { return !panes[pane] })
	maps.DeleteFunc(s.cleared, func(id string, _ bool) bool {
		return !slices.ContainsFunc(s.cur.Sessions, func(x sample.Session) bool { return x.SessionID == id })
	})
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
	if !refreshed && len(left) == 0 && len(st.Off) == offs && maps.Equal(st.Unseen, unseen) {
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
	renamed := s.refreshMembersLocked(st)
	return s.nameTeamsLocked(st) || renamed
}

// nameTeamsLocked names each team an older build saved with no name, after
// its first member that has a session now, and tells if any got one.
// Callers hold mu.
func (s *Service) nameTeamsLocked(st *chains.Store) (named bool) {
	for i := range st.Chains {
		if st.Chains[i].Name != "" {
			continue
		}
		for _, m := range st.Chains[i].Members {
			if x, ok := s.sessionOfLocked(m); ok {
				st.Chains[i].Name, named = st.FreeName(x.Name), true
				break
			}
		}
	}
	return named
}

func (s *Service) refreshMembersLocked(st *chains.Store) bool {
	return st.Refresh(func(m chains.Member) (chains.Member, bool) {
		if x, ok := s.switchedToLocked(st, m); ok {
			s.markClearedLocked(st, m, memberOf(x))
			return memberOf(x), true
		}
		for _, x := range s.cur.Sessions {
			if x.Pane != "" && x.Pane == m.Pane && x.PID == m.PID && x.Start == m.Start {
				n := memberOf(x)
				if n.SessionID == "" {
					n.SessionID = m.SessionID
				}
				if m.SessionID != "" {
					s.markClearedLocked(st, m, n)
				}
				return n, true
			}
		}
		return chains.Member{}, false
	})
}

// moveChainsLocked records that a switch starts another agent in x's pane,
// or, with on false, that it did not, and saves on a change. Callers hold mu.
func (s *Service) moveChainsLocked(x sample.Session, on bool) error {
	st, m := s.chainsLocked(), memberOf(x)
	_, moving := st.Moving(m)
	switch {
	case on:
		st.Move(m, s.now())
		if _, now := st.Moving(m); !now && !moving {
			return nil
		}
	case moving:
		st.Stay(m)
	default:
		return nil
	}
	return s.saveChainsLocked()
}

// markClearedLocked marks n, a new chat in m's place, to be told its chain at
// its next news. Callers hold mu.
func (s *Service) markClearedLocked(st *chains.Store, m, n chains.Member) {
	if len(st.TeamsOf(m)) == 0 || n.SessionID == m.SessionID {
		return
	}
	if s.cleared == nil {
		s.cleared = map[string]bool{}
	}
	s.cleared[n.SessionID] = true
}

// switchedToLocked is the agent a switch started in m's pane, once its
// session id shows there. Callers hold mu.
func (s *Service) switchedToLocked(st *chains.Store, m chains.Member) (sample.Session, bool) {
	if _, ok := st.Moving(m); !ok {
		return sample.Session{}, false
	}
	x, ok := s.sessionAtLocked(m.Pane)
	if !ok || x.SessionID == "" || x.PID == m.PID && x.Start == m.Start {
		return sample.Session{}, false
	}
	return x, true
}

// leftText is the news of l: the match is gone. Callers hold mu.
func (s *Service) leftText(l chains.Left, prev []sample.Session) string {
	name := l.Who.Pane
	if x, ok := s.sessionOfLocked(l.Who); ok {
		name = x.Name
	} else if i := slices.IndexFunc(prev, func(x sample.Session) bool { return memberOf(x).Same(l.Who) }); i >= 0 {
		name = prev[i].Name
	} else if n, ok := s.mateNames[l.Who.Pane]; ok {
		name = n
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
// not reach a doc already sent. Nil: no chain.
func chainsView(st *chains.Store) *state.ChainsView {
	if len(st.Chains) == 0 {
		return nil
	}
	v := &state.ChainsView{Chains: slices.Clone(st.Chains)}
	for i := range v.Chains {
		v.Chains[i].Members = slices.Clone(v.Chains[i].Members)
		for _, m := range v.Chains[i].Members {
			if slices.ContainsFunc(v.Mates, func(o state.MateView) bool { return o.Member.Same(m) }) {
				continue
			}
			mv := state.MateView{Member: m}
			mv.Inbox, mv.Full = st.InboxCount(m)
			mv.Unseen, _ = st.UnseenSince(m)
			v.Mates = append(v.Mates, mv)
		}
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

// tellLostLocked tells the sender of each line lost since the last sample
// why its match will not hear it: once for each match it sent to. Callers
// hold mu.
func (s *Service) tellLostLocked(st *chains.Store, now time.Time) {
	type pair struct {
		from, to chains.Member
		why      string
	}
	var order []pair
	count := map[pair]int{}
	for _, l := range st.TakeLost() {
		p := pair{l.Item.From, l.To, l.Why}
		if count[p] == 0 {
			order = append(order, p)
		}
		count[p]++
	}
	for _, p := range order {
		name := p.to.Pane
		if x, ok := s.sessionOfLocked(p.to); ok {
			name = x.Name
		}
		text := didNotGet(name, "you are no longer chained")
		if p.why == chains.LostFull {
			lines := "1 line"
			if n := count[p]; n > 1 {
				lines = fmt.Sprintf("%d lines", n)
			}
			text = "[matchblox] " + lines + " to " + name + " dropped: its inbox is full"
		}
		st.Post(p.from, text, now)
	}
}
