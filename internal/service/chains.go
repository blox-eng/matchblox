package service

import (
	"fmt"
	"os"
	"path/filepath"
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

// chainAct runs a chain: act from the agent in a.From: chain:<pane> asks,
// chain:- leaves, chain:off and chain:on.
func (s *Service) chainAct(a proto.Act) proto.Result {
	res, changed := s.changeChains(strings.TrimPrefix(a.RecID, proto.ChainPrefix), a.From)
	if changed {
		s.broadcast()
	}
	return res
}

func (s *Service) changeChains(arg, fromPane string) (res proto.Result, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sampleMissedLocked() { // a cleared chat would look like a new match
		return refuseText("not now: matchblox could not read the sessions; try again"), false
	}
	from, ok := s.sessionAtLocked(fromPane)
	if !ok {
		return refuseText(chainNeedsMatch), false
	}
	st := s.chainsLocked()
	me := memberOf(from)
	switch arg {
	case "-":
		if _, in := st.ChainOf(me); !in {
			return proto.Result{Note: "not in a chain"}, false
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
		to, ok := s.sessionAtLocked(arg)
		if !ok {
			return refuseText("no agent session in " + arg + " any more"), false
		}
		repo, why := s.chainable(st, from, to)
		if why != "" {
			return refuseText(why), false
		}
		if c, ok := st.ChainOf(me); ok && slices.ContainsFunc(c.Members, memberOf(to).Same) {
			return proto.Result{Note: "already chained with " + to.Name}, false
		}
		if st.Ask(me, memberOf(to), repo, s.now()) {
			c, _ := st.ChainOf(me)
			res.Note = fmt.Sprintf("chained with %s: %d matches", to.Name, len(c.Members))
		} else {
			res.Note = "asked " + to.Name + ": the chain forms when it chains back"
		}
	}
	s.cur.Chains = chainsView(st)
	if err := s.saveChainsLocked(); err != nil {
		res.Err = "chains: " + err.Error()
	}
	return res, true
}

// chainable is the repo from and to share, or why they cannot chain.
// Callers hold mu.
func (s *Service) chainable(st *chains.Store, from, to sample.Session) (repo, why string) {
	switch {
	case memberOf(from).Same(memberOf(to)):
		return "", "a match cannot chain with itself"
	case to.Host != from.Host:
		return "", to.Name + " is on another host: a chain is one host"
	}
	fromRepo, _, ok := gitscan.RepoOf(s.cur.Git, from.Cwd)
	if !ok {
		return "", from.Name + " is in no repo matchblox knows: a chain is one repo"
	}
	toRepo, _, ok := gitscan.RepoOf(s.cur.Git, to.Cwd)
	if !ok {
		return "", to.Name + " is in no repo matchblox knows: a chain is one repo"
	}
	if toRepo != fromRepo {
		return "", fmt.Sprintf("%s works in %s, not %s: a chain is one repo", to.Name, filepath.Base(toRepo), filepath.Base(fromRepo))
	}
	for _, x := range []sample.Session{from, to} {
		if slices.ContainsFunc(st.Off, memberOf(x).Same) {
			return "", x.Name + " is off: it takes no chains"
		}
	}
	return fromRepo, ""
}

// pruneChainsLocked keeps up the members whose session id changed in their
// pane, drops the members and asks the sessions no longer hold, tells the
// mates of a match that left, and saves on a change. prev are the sessions
// of the sample before, which name a match that is gone. Callers hold mu.
func (s *Service) pruneChainsLocked(now time.Time, prev []sample.Session) error {
	st := s.chainsLocked()
	if s.cur.Git == nil {
		return nil // no repo is known yet: every match would look gone
	}
	if s.sampleMissedLocked() {
		return nil // every match would look gone
	}
	refreshed := st.Refresh(func(m chains.Member) (chains.Member, bool) {
		for _, x := range s.cur.Sessions {
			if x.Pane != "" && x.Pane == m.Pane && x.PID == m.PID && x.Start == m.Start {
				return memberOf(x), true
			}
		}
		return chains.Member{}, false
	})
	before, asks, offs := slices.Clone(st.Chains), len(st.Requests), len(st.Off)
	live := func(m chains.Member) (string, bool) {
		x, ok := s.sessionOfLocked(m)
		if !ok {
			return "", false
		}
		repo, _, _ := gitscan.RepoOf(s.cur.Git, x.Cwd)
		return repo, true
	}
	left := st.Prune(live, now)
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

// leftText is the news of l: where the match went, when it is in another
// repo. Callers hold mu.
func (s *Service) leftText(l chains.Left, prev []sample.Session) string {
	name := l.Who.Pane
	if x, ok := s.sessionOfLocked(l.Who); ok {
		name = x.Name
	} else if i := slices.IndexFunc(prev, func(x sample.Session) bool { return memberOf(x).Same(l.Who) }); i >= 0 {
		name = prev[i].Name
	}
	if l.Repo == "" {
		return name + " left the chain"
	}
	return fmt.Sprintf("%s left the chain: it moved to %s", name, filepath.Base(l.Repo))
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

func (s *Service) sessionOfLocked(m chains.Member) (sample.Session, bool) {
	for _, x := range s.cur.Sessions {
		if memberOf(x).Same(m) {
			return x, true
		}
	}
	return sample.Session{}, false
}
