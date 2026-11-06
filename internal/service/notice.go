package service

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// pair is a match that was told of another in its repo.
type pair struct{ to, of chains.Member }

// delivery is the news of one hook's ask: its text, and what it took from
// the store and would mark as told, held until the hook says it heard it.
type delivery struct {
	text    string
	me      chains.Member
	taken   chains.Taken
	asks    []chains.Request
	notices map[pair][2]string
}

// newsHeardWait is how long the service holds news a hook took before it
// gives it back, unheard.
var newsHeardWait = 200 * time.Millisecond

// news is what the match in n.Pane has not heard yet, in blocks: its inbox,
// the requests to chain with it, then the same-repo matches it can chain
// with. It empties the inbox; heard marks the rest as told, unheard gives
// the inbox back.
func (s *Service) news(n proto.News) delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.sessionAtLocked(n.Pane)
	if !ok {
		return delivery{}
	}
	st := s.chainsLocked()
	d := delivery{me: memberOf(x), taken: st.Take(memberOf(x))}
	var blocks []string
	if texts := d.taken.Texts(); len(texts) > 0 {
		blocks = append(blocks, "[matchblox] since your last turn:\n  "+strings.Join(texts, "\n  "))
	}
	if !slices.ContainsFunc(st.Off, d.me.Same) {
		var asks []string
		asks, d.asks = s.requestsLocked(st, d.me)
		blocks = append(blocks, asks...)
		if s.ChainsNotice {
			var notices []string
			notices, d.notices = s.noticesLocked(st, x)
			blocks = append(blocks, notices...)
		}
	}
	d.text = strings.Join(blocks, "\n\n")
	s.holdLocked(d, true)
	return d
}

// holdLocked marks d's requests and notices as held by a delivery, or no
// longer. Callers hold mu.
func (s *Service) holdLocked(d delivery, held bool) {
	if s.heldAsks == nil {
		s.heldAsks, s.heldNotices = map[chains.Request]bool{}, map[pair]bool{}
	}
	for _, r := range d.asks {
		if held {
			s.heldAsks[r] = true
		} else {
			delete(s.heldAsks, r)
		}
	}
	for p := range d.notices {
		if held {
			s.heldNotices[p] = true
		} else {
			delete(s.heldNotices, p)
		}
	}
}

// heard marks d's requests and notices as told.
func (s *Service) heard(d delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holdLocked(d, false)
	for _, r := range d.asks {
		if s.heardAsks == nil {
			s.heardAsks = map[chains.Request]bool{}
		}
		s.heardAsks[r] = true
	}
	for p, b := range d.notices {
		if s.noticed == nil {
			s.noticed = map[pair][2]string{}
		}
		s.noticed[p] = b
	}
}

// unheard gives d's news back to the front of its inbox, and its requests
// and notices back to the next hook.
func (s *Service) unheard(d delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holdLocked(d, false)
	s.chainsLocked().GiveBack(d.me, d.taken)
}

// requestsLocked tells me, once each, who asks to chain with it: the lines,
// and the requests they tell. Callers hold mu.
func (s *Service) requestsLocked(st *chains.Store, me chains.Member) ([]string, []chains.Request) {
	maps.DeleteFunc(s.heardAsks, func(r chains.Request, _ bool) bool { return !slices.Contains(st.Requests, r) })
	var out []string
	var told []chains.Request
	for _, r := range st.Requests {
		if !r.To.Same(me) || s.heardAsks[r] || s.heldAsks[r] {
			continue
		}
		from, ok := s.sessionOfLocked(r.From)
		if !ok {
			continue
		}
		told = append(told, r)
		out = append(out, fmt.Sprintf("[matchblox] %s asks to chain with you. To accept, run:\n  matchblox chain %s", from.Name, from.Name))
	}
	return out, told
}

// noticesLocked tells x of each match in its repo it is not chained with
// and has no request with: once per pair, again after either changes
// branch. It returns the lines, and the branches of each pair they tell.
// Callers hold mu.
func (s *Service) noticesLocked(st *chains.Store, x sample.Session) ([]string, map[pair][2]string) {
	maps.DeleteFunc(s.noticed, func(p pair, _ [2]string) bool {
		_, to := s.sessionOfLocked(p.to)
		_, of := s.sessionOfLocked(p.of)
		return !to || !of
	})
	repo, wt, ok := gitscan.RepoOf(s.cur.Git, x.Cwd)
	if !ok {
		return nil, nil
	}
	me := memberOf(x)
	mates := st.Mates(me)
	asked := func(other chains.Member) bool {
		return slices.ContainsFunc(st.Requests, func(r chains.Request) bool {
			return r.From.Same(me) && r.To.Same(other) || r.From.Same(other) && r.To.Same(me)
		})
	}
	var out []string
	told := map[pair][2]string{}
	for _, y := range s.cur.Sessions {
		other := memberOf(y)
		yRepo, yWt, ok := gitscan.RepoOf(s.cur.Git, y.Cwd)
		if !ok || yRepo != repo || y.Host != x.Host || other.Same(me) || slices.ContainsFunc(mates, other.Same) ||
			slices.ContainsFunc(st.Off, other.Same) || asked(other) {
			continue
		}
		p, branches := pair{me, other}, [2]string{s.branchLocked(wt), s.branchLocked(yWt)}
		if was, ok := s.noticed[p]; ok && was == branches || s.heldNotices[p] {
			continue
		}
		told[p] = branches
		on := branches[1]
		if on == "" || on == "HEAD" {
			on = "a detached HEAD"
		}
		out = append(out, fmt.Sprintf("[matchblox] %s also works in %s, on %s.\n  To hear from it, run: matchblox chain %s",
			y.Name, filepath.Base(repo), on, y.Name))
	}
	return out, told
}

// branchLocked is wt's branch: the last HEAD read of a chained worktree,
// else the git scan's. "" or "HEAD": detached. Callers hold mu.
func (s *Service) branchLocked(wt string) string {
	if h, ok := s.heads[wt]; ok {
		return h.branch
	}
	if s.cur.Git != nil {
		for _, r := range s.cur.Git.Repos {
			for _, w := range r.Worktrees {
				if w.Path == wt {
					return w.Branch
				}
			}
		}
	}
	return ""
}
