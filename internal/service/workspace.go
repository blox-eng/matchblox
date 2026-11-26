package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// gitRunner runs a read-only Git command in dir and answers with its output.
type gitRunner func(ctx context.Context, dir string, args ...string) (string, error)

// workGit answers with Git's stdout only, so a warning never reads as a path.
// It drops the caller's GIT_* variables: GIT_DIR and its kin override -C.
func workGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-C", dir}, args...)...) //nolint:gosec // fixed read-only git commands, separate argv
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") })
	var out, stderr matchOutput
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, said.Clean(stderr.String()))
	}
	return out.String(), nil
}

// workspaceLine finds the first line that starts with "Workspace: " at
// column 0. An indented or quoted line is not one.
func workspaceLine(record string) (string, bool) {
	for l := range strings.Lines(record) {
		if rest, ok := strings.CutPrefix(l, "Workspace: "); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// workFolder picks where a switch starts the next agent. A record with no
// Workspace line keeps the launch folder. A line counts only when it names
// a worktree of the launch folder's repository; work is then its top level.
// A worktree that holds the launch folder keeps the launch folder. Any
// other line is refused, and the match stays in the launch folder.
// A nil git runs the real Git.
func workFolder(ctx context.Context, git gitRunner, record, launch string) (work, refused string) {
	path, ok := workspaceLine(record)
	if !ok {
		return launch, ""
	}
	if git == nil {
		git = workGit
	}
	if top := worktreeOf(ctx, git, path, launch); top != "" {
		return top, ""
	}
	return launch, shownPath(path) + " is not a worktree of this repository: it starts in " + launch
}

// shownPathLimit is the longest agent-written path a refusal shows as is.
const shownPathLimit = 256

// shownPath is an agent-written path as a refusal shows it: as written when
// it is short and clean, else quoted and cut, so no control key reaches
// the builder's terminal or the confirm.
func shownPath(p string) string {
	if len(p) <= shownPathLimit && said.Clean(p) == p {
		return p
	}
	if len(p) > shownPathLimit {
		p = strings.ToValidUTF8(p[:shownPathLimit], "") + "…"
	}
	return fmt.Sprintf("%q", p)
}

// worktreeOf answers with the top level of path when path is a worktree of
// launch's repository, else "". When that worktree is launch's own, it
// answers with launch as given. Outside Git only launch itself passes.
func worktreeOf(ctx context.Context, git gitRunner, path, launch string) string {
	if !filepath.IsAbs(path) {
		return ""
	}
	dir, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	home, err := filepath.EvalSymlinks(launch)
	if err != nil {
		return ""
	}
	common, err := gitPath(ctx, git, home, "--git-common-dir")
	if err != nil {
		if dir == home {
			return launch
		}
		return ""
	}
	if common == "" {
		return ""
	}
	if theirs, err := gitPath(ctx, git, dir, "--git-common-dir"); err != nil || theirs != common {
		return ""
	}
	// Inside .git there is no top level, so Git fails and the path is refused.
	top, err := gitPath(ctx, git, dir, "--show-toplevel")
	if err != nil || top == "" {
		return ""
	}
	// A shared common dir only proves the folder points at the repository.
	// The repository's own worktree list says which folders are its worktrees.
	list, err := git(ctx, home, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	for l := range strings.Lines(list) {
		p, ok := strings.CutPrefix(strings.TrimRight(l, "\n"), "worktree ")
		if !ok {
			continue
		}
		if p, err := filepath.EvalSymlinks(p); err == nil && p == top {
			// A launch in this worktree keeps its own folder, a subfolder too.
			if ours, err := gitPath(ctx, git, home, "--show-toplevel"); err == nil && ours == top {
				return launch
			}
			return top
		}
	}
	return ""
}

// gitPath asks git rev-parse for an absolute path, with its links resolved.
// Empty output answers "".
func gitPath(ctx context.Context, git gitRunner, dir, flag string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", nil
	}
	return filepath.EvalSymlinks(out)
}

// workKey is one task record as last written, for the folder its match was
// launched in: a new write or another launch folder asks again.
type workKey struct {
	record string
	mod    time.Time
	size   int64
	launch string
}

type workAnswer struct{ work, refused string }

// workCache holds the last answer of workFolder for each record, so that
// mergeQueue, under s.mu, never waits on Git. A miss goes to workLoop.
type workCache struct {
	mu      sync.Mutex
	answers map[string]workEntry // by record path
	pending map[workKey]bool
	asks    chan workKey
	filling sync.WaitGroup
}

type workEntry struct {
	key    workKey
	answer workAnswer
	at     time.Time
}

// workFresh is how long an answer stands without asking Git again: a
// worktree made or removed with the record unchanged shows within it.
const workFresh = 30 * time.Second

// look answers for key, or sends key to workLoop once and answers false.
// An answer older than workFresh is still given, and asked again behind
// it. It never blocks: with workLoop behind, the key is asked again later.
func (c *workCache) look(key workKey, now time.Time) (workAnswer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.answers[key.record]; ok && e.key == key {
		if now.Sub(e.at) > workFresh {
			c.ask(key)
		}
		return e.answer, true
	}
	c.ask(key)
	return workAnswer{}, false
}

// ask sends key to workLoop unless it is asked already. Hold c.mu.
func (c *workCache) ask(key workKey) {
	if c.pending[key] {
		return
	}
	c.filling.Add(1)
	select {
	case c.asks <- key:
		if c.pending == nil {
			c.pending = map[workKey]bool{}
		}
		c.pending[key] = true
	default:
		c.filling.Done()
	}
}

func (c *workCache) keep(key workKey, a workAnswer, ok bool, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		if c.answers == nil {
			c.answers = map[string]workEntry{}
		}
		c.answers[key.record] = workEntry{key, a, at}
	}
	delete(c.pending, key)
}

// forget drops record's answer, so the next merge asks Git again.
func (c *workCache) forget(record string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.answers, record)
}

// wait returns when every key asked so far is answered.
func (c *workCache) wait() { c.filling.Wait() }

// workLoop answers the work cache's misses one at a time until ctx ends,
// and tells the consoles each answer.
func (s *Service) workLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-s.works.asks:
			a, ok := s.fillWork(ctx, key)
			s.works.keep(key, a, ok, s.now())
			if ok {
				s.mu.Lock()
				s.setWorks()
				s.mu.Unlock()
				s.broadcast()
			}
			s.works.filling.Done()
		}
	}
}

// fillWork checks key's record. A record written again while it was read
// gives no answer: the next merge asks for the new write.
func (s *Service) fillWork(ctx context.Context, key workKey) (workAnswer, bool) {
	b, err := os.ReadFile(key.record)
	if err != nil {
		return workAnswer{}, false
	}
	if k, ok := workKeyOf(key.record, key.launch); !ok || k != key {
		return workAnswer{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	work, refused := workFolder(ctx, s.WorkGit, string(b), key.launch)
	return workAnswer{work, refused}, true
}

// switchFolder is where a switch of x starts the next agent, from its task
// record as it is now. A session with no folder or no managed record keeps
// its launch folder and runs no Git.
func (s *Service) switchFolder(ctx context.Context, x sample.Session) (work, refused string) {
	record := s.managedRecord(x)
	if record == "" || x.Cwd == "" {
		return x.Cwd, ""
	}
	b, err := os.ReadFile(record)
	if err != nil {
		return x.Cwd, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return workFolder(ctx, s.WorkGit, string(b), x.Cwd)
}

func workKeyOf(record, launch string) (workKey, bool) {
	fi, err := os.Stat(record)
	if err != nil {
		return workKey{}, false
	}
	return workKey{record, fi.ModTime(), fi.Size(), launch}, true
}

// setWorks sets Work and WorkRefused on each session the cache has an
// answer for. The sessions may share their array with s.sampled, so a
// change goes to a copy. Hold s.mu.
func (s *Service) setWorks() {
	sessions, copied, now := s.cur.Sessions, false, s.now()
	for i, x := range sessions {
		record := s.managedRecord(x)
		if record == "" || x.Cwd == "" {
			continue
		}
		key, ok := workKeyOf(record, x.Cwd)
		if !ok {
			continue
		}
		a, ok := s.works.look(key, now)
		if !ok || x.Work == a.work && x.WorkRefused == a.refused {
			continue
		}
		if !copied {
			sessions, copied = slices.Clone(sessions), true
		}
		sessions[i].Work, sessions[i].WorkRefused = a.work, a.refused
	}
	s.cur.Sessions = sessions
}
