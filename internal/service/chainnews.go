package service

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/sample"
)

// head is where a worktree's HEAD points: its commit, and its branch
// ("HEAD" when detached).
type head struct{ commit, branch string }

// movesHead finds a shell line that can move HEAD: git commit, switch or
// checkout, past git's own options.
var movesHead = regexp.MustCompile(`(^|[\s;&|(])git(\s+-[cC]\s+\S+|\s+-\S+)*\s+(commit|switch|checkout)\b`)

// chainNews posts what a hook event tells the chain-mates of its match: a
// finish, a wait for the person, or a commit or branch change its shell ran.
func (s *Service) chainNews(ev hooks.Event) {
	now := s.now()
	s.mu.Lock()
	text := s.turnNewsLocked(ev)
	x, ok := s.hookSessionLocked(ev.SessionID)
	if !ok {
		s.mu.Unlock()
		return
	}
	me := memberOf(x)
	if _, chained := s.chainsLocked().ChainOf(me); !chained {
		s.mu.Unlock()
		return
	}
	if text != "" {
		s.postLocked([]chains.Member{me}, x.Name+" "+text, false, now)
	}
	wt := ""
	if ev.Name == "PostToolUse" && movesHead.MatchString(ev.Command) {
		_, wt, _ = gitscan.RepoOf(s.cur.Git, cmp.Or(ev.Cwd, x.Cwd))
	}
	old, had := s.heads[wt]
	s.mu.Unlock()
	if wt == "" {
		return
	}
	cur, line, ok := s.readMove(wt, old, had)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.moveLocked(wt, old, had, cur) && line != "" {
		s.postLocked([]chains.Member{me}, x.Name+" "+line, true, now)
	}
}

// turnNewsLocked is the news of a Stop, or of a wait for the person that
// starts: once per wait. Callers hold mu, after the queue took ev.
func (s *Service) turnNewsLocked(ev hooks.Event) string {
	waits := s.queue.Waits(ev.SessionID)
	starts := waits && !s.chainWaits[ev.SessionID]
	if waits {
		if s.chainWaits == nil {
			s.chainWaits = map[string]bool{}
		}
		s.chainWaits[ev.SessionID] = true
	} else {
		delete(s.chainWaits, ev.SessionID)
	}
	switch {
	case starts:
		return "waits for the owner"
	case ev.Name == "Stop":
		return "finished"
	}
	return ""
}

// chainHeads reads the HEAD of each worktree that holds a chained match and
// posts a move to the mates: by name when the worktree holds one match, else
// as someone in it. Git runs out of mu.
func (s *Service) chainHeads(now time.Time) {
	type look struct {
		wt  string
		in  []sample.Session
		old head
		had bool
	}
	s.mu.Lock()
	if s.sampleMissedLocked() {
		s.mu.Unlock()
		return
	}
	st := s.chainsLocked()
	byTree := map[string][]sample.Session{}
	for _, x := range s.cur.Sessions {
		if _, wt, ok := gitscan.RepoOf(s.cur.Git, x.Cwd); ok {
			byTree[wt] = append(byTree[wt], x)
		}
	}
	var looks []look
	for wt, in := range byTree {
		if slices.ContainsFunc(in, func(x sample.Session) bool { _, ok := st.ChainOf(memberOf(x)); return ok }) {
			old, had := s.heads[wt]
			looks = append(looks, look{wt, in, old, had})
		}
	}
	slices.SortFunc(looks, func(a, b look) int { return strings.Compare(a.wt, b.wt) })
	maps.DeleteFunc(s.heads, func(wt string, _ head) bool {
		return !slices.ContainsFunc(looks, func(l look) bool { return l.wt == wt })
	})
	maps.DeleteFunc(s.chainWaits, func(id string, _ bool) bool { return !s.queue.Waits(id) })
	s.mu.Unlock()
	for _, l := range looks {
		cur, line, ok := s.readMove(l.wt, l.old, l.had)
		if !ok {
			continue
		}
		s.mu.Lock()
		if s.moveLocked(l.wt, l.old, l.had, cur) && line != "" {
			name := "someone in " + filepath.Base(l.wt)
			if len(l.in) == 1 {
				name = l.in[0].Name
			}
			from := make([]chains.Member, len(l.in))
			for i, x := range l.in {
				from[i] = memberOf(x)
			}
			s.postLocked(from, name+" "+line, true, now)
		}
		s.mu.Unlock()
	}
}

// readMove reads wt's HEAD and, when it moved from old, the line that says
// how. Out of mu: git is slow.
func (s *Service) readMove(wt string, old head, had bool) (cur head, line string, ok bool) {
	out, err := s.readGit(wt, "rev-parse", "HEAD", "--abbrev-ref", "HEAD")
	f := strings.Fields(out)
	if err != nil || len(f) != 2 {
		return head{}, "", false
	}
	cur = head{commit: f[0], branch: f[1]}
	switch {
	case !had || cur == old:
		return cur, "", true
	case cur.branch == "HEAD" && old.branch != "HEAD":
		return cur, "switched to a detached HEAD at " + cur.commit[:min(7, len(cur.commit))], true
	case cur.branch != old.branch:
		return cur, "switched to " + cur.branch, true
	}
	out, err = s.readGit(wt, "diff", "--name-only", old.commit, cur.commit)
	if err != nil {
		return cur, commitLine(nil), true
	}
	var files []string
	for _, f := range strings.Split(out, "\n") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	return cur, commitLine(files), true
}

// moveLocked records cur as wt's HEAD unless another read did since old,
// and tells if the move is news. Callers hold mu.
func (s *Service) moveLocked(wt string, old head, had bool, cur head) bool {
	if now, has := s.heads[wt]; has != had || now != old {
		return false // the hook, or a sample, named it first
	}
	if s.heads == nil {
		s.heads = map[string]head{}
	}
	s.heads[wt] = cur
	return had && cur != old
}

// postLocked puts text in the inbox of every mate of from, never in one of
// from's own; spark marks their chains. Callers hold mu.
func (s *Service) postLocked(from []chains.Member, text string, spark bool, now time.Time) {
	st := s.chainsLocked()
	var to []chains.Member
	for _, m := range from {
		c, ok := st.ChainOf(m)
		if !ok {
			continue
		}
		if spark {
			st.Spark(c.ID, now)
		}
		for _, o := range st.Mates(m) {
			if !slices.ContainsFunc(from, o.Same) && !slices.ContainsFunc(to, o.Same) {
				to = append(to, o)
			}
		}
	}
	for _, o := range to {
		st.Post(o, text, now)
	}
	if spark {
		s.cur.Chains = chainsView(st)
	}
}

// commitLine says what a commit changed: its file count and up to three paths.
func commitLine(files []string) string {
	switch n := len(files); {
	case n == 0:
		return "committed"
	case n == 1:
		return "committed 1 file in " + files[0]
	case n <= 3:
		return fmt.Sprintf("committed %d files in %s", n, strings.Join(files, ", "))
	default:
		return fmt.Sprintf("committed %d files in %s, and %d more", n, strings.Join(files[:3], ", "), n-3)
	}
}

func (s *Service) readGit(dir string, args ...string) (string, error) {
	if s.Git != nil {
		return s.Git(dir, args...)
	}
	return matchGit(context.Background(), dir, args...)
}

func (s *Service) hookSessionLocked(id string) (sample.Session, bool) {
	for _, x := range s.cur.Sessions {
		if x.SessionID == id && id != "" {
			return x, true
		}
	}
	return sample.Session{}, false
}
