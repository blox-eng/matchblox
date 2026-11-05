package service

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// pair is a match that was told of another in its repo.
type pair struct{ to, of chains.Member }

// news is what the match in n.Pane has not heard yet, in blocks: its inbox,
// the requests to chain with it, then the same-repo matches it can chain
// with. It empties the inbox. "": nothing new.
func (s *Service) news(n proto.News) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.sessionAtLocked(n.Pane)
	if !ok {
		return ""
	}
	st := s.chainsLocked()
	me := memberOf(x)
	var blocks []string
	if items := st.Take(me); len(items) > 0 {
		lines := []string{"[matchblox] since your last turn:"}
		for _, it := range items {
			lines = append(lines, "  "+it.Text)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	if !slices.ContainsFunc(st.Off, me.Same) {
		blocks = append(blocks, s.requestsLocked(st, me)...)
		if s.ChainsNotice {
			blocks = append(blocks, s.noticesLocked(st, x)...)
		}
	}
	return strings.Join(blocks, "\n\n")
}

// requestsLocked tells me, once each, who asks to chain with it. Callers
// hold mu.
func (s *Service) requestsLocked(st *chains.Store, me chains.Member) []string {
	maps.DeleteFunc(s.heardAsks, func(r chains.Request, _ bool) bool { return !slices.Contains(st.Requests, r) })
	var out []string
	for _, r := range st.Requests {
		if !r.To.Same(me) || s.heardAsks[r] {
			continue
		}
		from, ok := s.sessionOfLocked(r.From)
		if !ok {
			continue
		}
		if s.heardAsks == nil {
			s.heardAsks = map[chains.Request]bool{}
		}
		s.heardAsks[r] = true
		out = append(out, fmt.Sprintf("[matchblox] %s asks to chain with you. To accept, run:\n  matchblox chain %s", from.Name, from.Name))
	}
	return out
}

// noticesLocked tells x of each match in its repo it is not chained with
// and has no request with: once per pair, again after either changes
// branch. Callers hold mu.
func (s *Service) noticesLocked(st *chains.Store, x sample.Session) []string {
	maps.DeleteFunc(s.noticed, func(p pair, _ [2]string) bool {
		_, to := s.sessionOfLocked(p.to)
		_, of := s.sessionOfLocked(p.of)
		return !to || !of
	})
	repo, wt, ok := gitscan.RepoOf(s.cur.Git, x.Cwd)
	if !ok {
		return nil
	}
	me := memberOf(x)
	mates := st.Mates(me)
	asked := func(other chains.Member) bool {
		return slices.ContainsFunc(st.Requests, func(r chains.Request) bool {
			return r.From.Same(me) && r.To.Same(other) || r.From.Same(other) && r.To.Same(me)
		})
	}
	var out []string
	for _, y := range s.cur.Sessions {
		other := memberOf(y)
		yRepo, yWt, ok := gitscan.RepoOf(s.cur.Git, y.Cwd)
		if !ok || yRepo != repo || y.Host != x.Host || other.Same(me) || slices.ContainsFunc(mates, other.Same) ||
			slices.ContainsFunc(st.Off, other.Same) || asked(other) {
			continue
		}
		p, branches := pair{me, other}, [2]string{s.branchLocked(wt), s.branchLocked(yWt)}
		if was, ok := s.noticed[p]; ok && was == branches {
			continue
		}
		if s.noticed == nil {
			s.noticed = map[pair][2]string{}
		}
		s.noticed[p] = branches
		on := branches[1]
		if on == "" || on == "HEAD" {
			on = "a detached HEAD"
		}
		out = append(out, fmt.Sprintf("[matchblox] %s also works in %s, on %s.\n  To hear from it, run: matchblox chain %s",
			y.Name, filepath.Base(repo), on, y.Name))
	}
	return out
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
