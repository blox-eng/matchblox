package service

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	if wt == "" {
		s.mu.Unlock()
		return
	}
	old, had := s.heads[wt]
	s.readingHead(wt, 1)
	s.mu.Unlock()
	cur, line, ok := s.readMove(wt, old, had)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readingHead(wt, -1)
	if ok && s.moveLocked(wt, old, had, cur) && line != "" {
		s.postLocked(s.inTreeLocked(wt, me), x.Name+" "+line, true, now)
	}
}

// readingHead counts the hook reads of wt's HEAD in flight, by d. Callers
// hold mu.
func (s *Service) readingHead(wt string, d int) {
	if s.headReads == nil {
		s.headReads = map[string]int{}
	}
	if s.headReads[wt] += d; s.headReads[wt] <= 0 {
		delete(s.headReads, wt)
	}
}

// inTreeLocked are the matches in worktree wt, me among them: they share
// the checkout, so a move of its HEAD is no news to any. Callers hold mu.
func (s *Service) inTreeLocked(wt string, me chains.Member) []chains.Member {
	in := []chains.Member{me}
	for _, y := range s.cur.Sessions {
		if _, ywt, ok := gitscan.RepoOf(s.cur.Git, y.Cwd); ok && ywt == wt && !memberOf(y).Same(me) {
			in = append(in, memberOf(y))
		}
	}
	return in
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
// as someone in it. Git runs out of mu, and out of the sample loop: publish
// starts it in headRun.
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
	if len(st.Chains) == 0 { // nothing to read
		maps.DeleteFunc(s.heads, func(wt string, _ head) bool { return s.headReads[wt] == 0 })
		maps.DeleteFunc(s.chainWaits, func(id string, _ bool) bool { return !s.queue.Waits(id) })
		s.mu.Unlock()
		return
	}
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
		return s.headReads[wt] == 0 && !slices.ContainsFunc(looks, func(l look) bool { return l.wt == wt })
	})
	maps.DeleteFunc(s.chainWaits, func(id string, _ bool) bool { return !s.queue.Waits(id) })
	s.mu.Unlock()
	posted := false
	for _, l := range looks {
		cur, line, ok := s.readMove(l.wt, l.old, l.had)
		if !ok {
			continue
		}
		s.mu.Lock()
		if s.moveLocked(l.wt, l.old, l.had, cur) && line != "" {
			posted = true
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
	if posted {
		s.broadcast() // the spark
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
	sha := cur.commit[:min(7, len(cur.commit))]
	switch {
	case !had || cur == old:
		return cur, "", true
	case cur.branch == "HEAD" && old.branch != "HEAD":
		return cur, "switched to a detached HEAD at " + sha, true
	case cur.branch != old.branch:
		return cur, "switched to " + cur.branch, true
	}
	how, _ := s.readGit(wt, "reflog", "-1", "--format=%gs", "HEAD")
	switch how = strings.TrimSpace(how); {
	case strings.HasPrefix(how, "commit"):
		return cur, commitLine(s.filesChanged(wt, old, cur), s.firstFiles(wt, old, cur)), true
	case strings.HasPrefix(how, "pull"):
		n := s.filesChanged(wt, old, cur)
		return cur, fmt.Sprintf("pulled %d %s into %s", n, plural(n, "file"), cur.branch), true
	}
	return cur, "moved " + cur.branch + " to " + sha, true // a reset, a rebase, a merge
}

// filesChanged is how many files differ between old and cur, as git's
// shortstat counts them; 0 when git does not say.
func (s *Service) filesChanged(wt string, old, cur head) int {
	out, err := s.readGit(wt, "diff", "--shortstat", old.commit, cur.commit)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.Fields(out + " 0")[0])
	return n
}

// firstFiles are the first three paths that differ between old and cur.
func (s *Service) firstFiles(wt string, old, cur head) []string {
	out, err := s.readGit(wt, "diff", "--name-only", old.commit, cur.commit)
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(out, "\n") {
		if f = strings.TrimSpace(f); f != "" && len(files) < 3 {
			files = append(files, f)
		}
	}
	return files
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
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

// commitLine says what a commit changed: its n files and the first paths.
func commitLine(n int, first []string) string {
	first = first[:min(3, len(first), n)]
	switch {
	case n == 0:
		return "committed"
	case len(first) == 0:
		return fmt.Sprintf("committed %d %s", n, plural(n, "file"))
	case n == 1:
		return "committed 1 file in " + first[0]
	case n == len(first):
		return fmt.Sprintf("committed %d files in %s", n, strings.Join(first, ", "))
	default:
		return fmt.Sprintf("committed %d files in %s, and %d more", n, strings.Join(first, ", "), n-len(first))
	}
}

// readGit runs a read-only git command in dir: s.Git, else git with a short
// timeout, its paths unquoted and its stderr out of the answer.
func (s *Service) readGit(dir string, args ...string) (string, error) {
	if s.Git != nil {
		return s.Git(dir, args...)
	}
	return readOnlyGit(context.Background(), dir, true, append([]string{"-c", "core.quotePath=false"}, args...)...)
}

// lone runs one call of a slow job at a time, in its own goroutine: a start
// while one runs is skipped.
type lone struct {
	busy atomic.Bool
	wg   sync.WaitGroup
}

func (l *lone) start(f func()) {
	if !l.busy.CompareAndSwap(false, true) {
		return
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		defer l.busy.Store(false)
		f()
	}()
}

// wait returns once the run in progress, if any, ends.
func (l *lone) wait() { l.wg.Wait() }

// typingWait bounds how long Serve waits for acts that type; each step has
// its own timeout well inside it.
var typingWait = 5 * time.Second

// inFlight counts calls in progress. Unlike a WaitGroup, a call may start
// while another goroutine waits.
type inFlight struct {
	mu   sync.Mutex
	n    int
	idle chan struct{}
}

func (f *inFlight) add() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == 0 {
		f.idle = make(chan struct{})
	}
	f.n++
}

func (f *inFlight) done() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n--; f.n == 0 {
		close(f.idle)
	}
}

// wait returns true once no call is in progress, false after d.
func (f *inFlight) wait(d time.Duration) bool {
	f.mu.Lock()
	if f.n == 0 {
		f.mu.Unlock()
		return true
	}
	idle := f.idle
	f.mu.Unlock()
	select {
	case <-idle:
		return true
	case <-time.After(d):
		return false
	}
}

func (s *Service) hookSessionLocked(id string) (sample.Session, bool) {
	for _, x := range s.cur.Sessions {
		if x.SessionID == id && id != "" {
			return x, true
		}
	}
	return sample.Session{}, false
}
