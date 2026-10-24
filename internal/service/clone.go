package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/clone"
	"github.com/blox-eng/matchblox/internal/gitremote"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
)

// cloneListed is how long a clone that ended stays in the snapshots.
const cloneListed = time.Minute

// cloneStopWait bounds how long clone:stop waits for the partial folder to go.
const cloneStopWait = 10 * time.Second

// cloneJob is one clone the service runs or ran, by target, under s.mu.
type cloneJob struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // closed when the run ended
	c      proto.Clone
	end    time.Time // zero while it runs
}

// ExpandHome makes a path from config absolute: a leading ~ is the home
// folder, and a relative path is taken from the home folder.
func ExpandHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Clean(p)
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok || p == "~" {
		p = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(home, p)
	}
	return filepath.Clean(p)
}

// tildeIn shows the home folder as ~ in text: only whole paths.
func tildeIn(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, home)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		rest := s[i+len(home):]
		b.WriteString(s[:i])
		if rest == "" || rest[0] == '/' || strings.ContainsRune(" \t\n:,;)'\"", rune(rest[0])) {
			b.WriteString("~")
		} else {
			b.WriteString(home)
		}
		s = rest
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// gitProtocol is the protocol the builder's gh clones with, asked once:
// "ssh", or else "https".
func (s *Service) gitProtocol() string {
	s.protoOnce.Do(func() {
		s.protocol = "https"
		look := s.LookPath
		if look == nil {
			look = exec.LookPath
		}
		gh, err := look("gh")
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, gh, "config", "get", "git_protocol").Output() //nolint:gosec // argv, never a shell string
		if err == nil && strings.TrimSpace(string(out)) == "ssh" {
			s.protocol = "ssh"
		}
	})
	return s.protocol
}

// remoteOf reads what was typed as a git URL. Only the shorthand o/r asks
// for the builder's protocol.
func (s *Service) remoteOf(text string) (gitremote.Remote, bool) {
	if s.cloneRemote != nil {
		return s.cloneRemote(text)
	}
	t := strings.TrimSpace(text)
	r, ok := gitremote.Parse(t, "https")
	if ok && r.URL != strings.TrimSuffix(t, "/") {
		r, ok = gitremote.Parse(t, s.gitProtocol())
	}
	return r, ok
}

// freeName is a free name from the service's random source.
func (s *Service) freeName(repo string, taken func(string) bool) string {
	s.cloneMu.Lock()
	defer s.cloneMu.Unlock()
	if s.cloneRand == nil {
		s.cloneRand = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(os.Getpid()))) //nolint:gosec // a folder name, not a secret
	}
	return clone.FreeName(repo, taken, s.cloneRand)
}

// cloneRoot is CloneRoot, clean; "" when none is set.
func (s *Service) cloneRoot() string {
	if s.CloneRoot == "" {
		return ""
	}
	return filepath.Clean(s.CloneRoot)
}

// planClone finds where a clone of r goes, offline: it reads the folders
// on disk, the found list and the used record, never the network.
func (s *Service) planClone(r gitremote.Remote) (proto.ClonePlan, error) {
	root := s.cloneRoot()
	if root == "" {
		return proto.ClonePlan{}, errors.New("no clone_root is set")
	}
	s.mu.Lock()
	running := map[string]bool{}
	for t, j := range s.clones {
		if j.end.IsZero() {
			running[t] = true
		}
	}
	found := slices.Clone(s.found.Repos)
	var used []string
	for _, u := range s.used.Repos {
		used = append(used, u.Path)
	}
	s.mu.Unlock()

	taken := func(name string) bool {
		t := filepath.Join(root, name)
		return exists(t) || running[t] || s.siblingHeld(root, t)
	}
	p := proto.ClonePlan{Repo: r.Slug(), URL: r.URL, Target: filepath.Join(root, r.Repo)}
	same := false
	switch fi, err := os.Stat(p.Target); {
	case err == nil && fi.IsDir():
		if c, ok := gitscan.ReadCheckout(p.Target, "origin"); !ok {
			p.Taken = "not a repo"
		} else if gitremote.Same(c.Remote, r.URL) {
			same = true
		} else {
			p.Taken = "another repo"
		}
	case exists(p.Target):
		p.Taken = "a file" // a file, or a link that leads nowhere
	case taken(r.Repo):
		p.Taken = "being cloned"
	}

	seen := map[string]bool{p.Target: true}
	var known []string
	for _, c := range found {
		if c.Main == "" && !seen[c.Path] && gitremote.Same(c.Remote, r.URL) {
			if fi, err := os.Stat(c.Path); err == nil && fi.IsDir() {
				seen[c.Path] = true
				known = append(known, c.Path)
			}
		}
	}
	for _, u := range used {
		if seen[u] {
			continue
		}
		if c, ok := gitscan.ReadCheckout(u, "origin"); ok && c.Main == "" && gitremote.Same(c.Remote, r.URL) {
			seen[u] = true
			known = append(known, u)
		}
	}
	slices.Sort(known)
	if same {
		p.Have = append([]string{p.Target}, known...)
		p.Fresh = filepath.Join(root, s.freeName(r.Repo, taken))
	} else {
		p.Have = known
		if p.Taken != "" {
			p.Target = filepath.Join(root, s.freeName(r.Repo, taken))
		}
	}
	return p, nil
}

// clonePlan is the clone:plan act: Text is what was typed.
func (s *Service) clonePlan(text string) proto.Result {
	r, ok := s.remoteOf(text)
	if !ok {
		return proto.Result{Err: "not a git URL"}
	}
	p, err := s.planClone(r)
	if err != nil {
		return proto.Result{Err: err.Error()}
	}
	return proto.Result{Clone: &p}
}

// cloneStart is the clone:start act: it checks the target again, then
// clones in the background, on the service's context. The snapshots say
// how far it is.
func (s *Service) cloneStart(ctx context.Context, text string) proto.Result {
	var cs proto.CloneStart
	if err := json.Unmarshal([]byte(text), &cs); err != nil {
		return proto.Result{Err: "the clone does not read: " + err.Error()}
	}
	r, ok := s.remoteOf(cs.URL)
	if !ok {
		return proto.Result{Err: "not a git URL"}
	}
	target := ExpandHome(strings.TrimSpace(cs.Target))
	j, msg := s.registerClone(ctx, r, target)
	if msg != "" {
		return proto.Result{Err: msg}
	}
	go s.runClone(j.ctx, j, r)
	return proto.Result{CloneID: j.c.ID}
}

// registerClone checks target again and lists a clone of r into it, whose
// context ends with ctx. It returns a message when the clone cannot start.
func (s *Service) registerClone(ctx context.Context, r gitremote.Remote, target string) (*cloneJob, string) {
	root := s.cloneRoot()
	if root == "" {
		return nil, "no clone_root is set"
	}
	if base := filepath.Base(target); filepath.Dir(target) != root || strings.HasSuffix(base, ".cloning") || strings.Contains(base, ".cloning.stale-") {
		return nil, tildeIn(fmt.Sprintf("a clone goes into a folder of its own in %s: %s is not one", root, target))
	}
	if err := os.MkdirAll(root, 0o755); err != nil { //nolint:gosec // the builder's own source folder
		return nil, tildeIn(fmt.Sprintf("cannot make %s: %v", root, err))
	}
	s.mu.Lock()
	if j := s.clones[target]; j != nil && j.end.IsZero() {
		s.mu.Unlock()
		return nil, tildeIn(target + " is being cloned")
	}
	if exists(target) {
		s.mu.Unlock()
		return nil, tildeIn(target + " is there already")
	}
	trash := ""
	if sib := clone.Cloning(target); exists(sib) {
		t, ok := staleAway(root, target) // no clone into target runs: checked above
		if !ok {
			s.mu.Unlock()
			return nil, tildeIn(sib + " is there already")
		}
		trash = t
	}
	jctx, cancel := context.WithCancel(ctx)
	s.cloneSeq++
	j := &cloneJob{ctx: jctx, cancel: cancel, done: make(chan struct{}), c: proto.Clone{ID: s.cloneSeq, Target: target, Repo: r.Slug(), Pct: -1}}
	if s.clones == nil {
		s.clones = map[string]*cloneJob{}
	}
	s.clones[target] = j
	s.mu.Unlock()
	if trash != "" {
		_ = os.RemoveAll(trash)
	}
	s.broadcast()
	return j, ""
}

// cloneForNew is what a git URL in new:session's Where comes to: where the
// session starts, and a line that says what happened. It clones on the
// act's own goroutine, and stops when ctx ends.
func (s *Service) cloneForNew(ctx context.Context, r gitremote.Remote) (dir, said, errText string) {
	p, err := s.planClone(r)
	if err != nil {
		return "", "", err.Error()
	}
	if len(p.Have) > 0 {
		return p.Have[0], fmt.Sprintf("%s is already at %s: started there", p.Repo, tildeIn(p.Have[0])), ""
	}
	j, msg := s.registerClone(ctx, r, p.Target)
	if msg != "" {
		return "", "", msg
	}
	s.runClone(j.ctx, j, r)
	s.mu.Lock()
	failed := j.c.Err
	s.mu.Unlock()
	if failed != "" {
		return "", "", failed
	}
	return p.Target, fmt.Sprintf("cloned %s into %s", p.Repo, tildeIn(p.Target)), ""
}

func (s *Service) runClone(ctx context.Context, j *cloneJob, r gitremote.Remote) {
	run := s.cloneRun
	if run == nil {
		run = clone.Run
	}
	err := run(ctx, r, j.c.Target, func(pct int) {
		s.mu.Lock()
		changed := j.c.Pct != pct
		j.c.Pct = pct
		s.mu.Unlock()
		if changed {
			s.broadcast()
		}
	})
	if err == nil {
		s.addUsed(j.c.Target) // a known repo before a console sees Done
	}
	s.mu.Lock()
	j.end = s.now()
	if err != nil {
		j.c.Err = tildeIn(err.Error())
	} else {
		j.c.Done = true
	}
	s.mu.Unlock()
	j.cancel()
	close(j.done)
	s.broadcast()
}

// cloneStop is the clone:stop act: Text is the target. It waits, within a
// bound, until the partial folder is gone.
func (s *Service) cloneStop(text string) proto.Result {
	target := ExpandHome(strings.TrimSpace(text))
	s.mu.Lock()
	j := s.clones[target]
	s.mu.Unlock()
	if j == nil {
		return proto.Result{Err: tildeIn(target + " is not being cloned")}
	}
	j.cancel()
	select {
	case <-j.done:
	case <-time.After(cloneStopWait):
	}
	return proto.Result{}
}

// cloneList is the clones for a snapshot; a clone that ended a minute ago
// goes. Hold s.mu.
func (s *Service) cloneList() []proto.Clone {
	now := s.now()
	var out []proto.Clone
	for t, j := range s.clones {
		if !j.end.IsZero() && now.Sub(j.end) >= cloneListed {
			delete(s.clones, t)
			continue
		}
		out = append(out, j.c)
	}
	slices.SortFunc(out, func(a, b proto.Clone) int { return strings.Compare(a.Target, b.Target) })
	return out
}

// stopClones stops the clones that run and waits, at most d, until each
// has removed its partial folder: a clone cut off by the service's end
// leaves nothing behind.
func (s *Service) stopClones(d time.Duration) {
	s.mu.Lock()
	var running []*cloneJob
	for _, j := range s.clones {
		if j.end.IsZero() {
			running = append(running, j)
		}
	}
	s.mu.Unlock()
	limit := time.After(d)
	for _, j := range running {
		j.cancel()
	}
	for _, j := range running {
		select {
		case <-j.done:
		case <-limit:
			return
		}
	}
}

// staleAway moves aside the work folder of a clone that no longer runs
// (the service ended while it ran), so its name is free again. Only a real
// folder named as target's sibling directly in clone_root goes; it returns
// where it went, for the caller to remove outside s.mu. Hold s.mu, and
// know no clone into target runs.
func staleAway(root, target string) (string, bool) {
	sib := clone.Cloning(target)
	if filepath.Dir(sib) != root {
		return "", false
	}
	fi, err := os.Lstat(sib)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	trash := fmt.Sprintf("%s.stale-%d", sib, time.Now().UnixNano())
	if os.Rename(sib, trash) != nil {
		return "", false
	}
	return trash, true
}

// siblingHeld tells if target's work folder holds its name: a clone into
// it runs, or the folder cannot be cleared. A stale one is removed.
func (s *Service) siblingHeld(root, target string) bool {
	if !exists(clone.Cloning(target)) {
		return false
	}
	s.mu.Lock()
	if j := s.clones[target]; j != nil && j.end.IsZero() {
		s.mu.Unlock()
		return true
	}
	trash, ok := staleAway(root, target)
	s.mu.Unlock()
	if ok {
		_ = os.RemoveAll(trash)
	}
	return !ok
}
