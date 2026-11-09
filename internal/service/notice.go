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
	notices map[pair][2]string
	cleared string // the session id told its chain after a /clear
}

// newsHeardWait is how long the service holds news a hook took before it
// gives it back, unheard.
var newsHeardWait = 200 * time.Millisecond

// news is what the match in n.Pane has not heard yet, in blocks: its inbox,
// then the same-repo matches it can chain with. It empties the inbox; heard marks the rest as told, unheard gives
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
	if line := s.clearedLocked(st, x); line != "" {
		d.cleared = x.SessionID
		blocks = append(blocks, line)
	}
	if texts := d.taken.Texts(); len(texts) > 0 {
		blocks = append(blocks, "[matchblox] since your last turn:\n  "+strings.Join(texts, "\n  "))
	}
	if !slices.ContainsFunc(st.Off, d.me.Same) && s.ChainsNotice {
		var notices []string
		notices, d.notices = s.noticesLocked(st, x)
		blocks = append(blocks, notices...)
	}
	d.text = strings.Join(blocks, "\n\n")
	s.holdLocked(d, true)
	return d
}

// holdLocked marks d's notices as held by a delivery, or no longer.
// Callers hold mu.
func (s *Service) holdLocked(d delivery, held bool) {
	if s.heldNotices == nil {
		s.heldNotices = map[pair]bool{}
	}
	for p := range d.notices {
		if held {
			s.heldNotices[p] = true
		} else {
			delete(s.heldNotices, p)
		}
	}
	if d.cleared != "" {
		if held {
			if s.heldCleared == nil {
				s.heldCleared = map[string]bool{}
			}
			s.heldCleared[d.cleared] = true
		} else {
			delete(s.heldCleared, d.cleared)
		}
	}
}

// heard marks d's requests and notices as told.
func (s *Service) heard(d delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holdLocked(d, false)
	delete(s.cleared, d.cleared)
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

// clearedLocked is the line that tells x, whose chat a /clear started
// afresh, who it is chained with: once, "" when there is nothing to tell.
// Callers hold mu.
func (s *Service) clearedLocked(st *chains.Store, x sample.Session) string {
	if !s.cleared[x.SessionID] || s.heldCleared[x.SessionID] {
		return ""
	}
	var names []string
	for _, m := range st.Mates(memberOf(x)) {
		if y, ok := s.sessionOfLocked(m); ok {
			names = append(names, y.Name)
		}
	}
	if len(names) == 0 {
		delete(s.cleared, x.SessionID)
		return ""
	}
	slices.Sort(names)
	return "[matchblox] you are chained with " + strings.Join(names, ", ") + ". To leave, run: matchblox unchain"
}

// noticesLocked tells x of each match in its repo it is not chained with: once per pair, again after either changes
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
	var out []string
	told := map[pair][2]string{}
	for _, y := range s.cur.Sessions {
		other := memberOf(y)
		yRepo, yWt, ok := gitscan.RepoOf(s.cur.Git, y.Cwd)
		if !ok || yRepo != repo || y.Host != x.Host || other.Same(me) || slices.ContainsFunc(mates, other.Same) ||
			slices.ContainsFunc(st.Off, other.Same) {
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
