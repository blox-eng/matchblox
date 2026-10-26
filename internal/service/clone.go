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
var cloneStopWait = 10 * time.Second

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
	if rest, ok := cutHome(p); ok {
		p = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(home, p)
	}
	return filepath.Clean(p)
}

// cutHome is p after a leading "~/", or "" for a bare "~"; ok when p
// starts at the home folder.
func cutHome(p string) (rest string, ok bool) {
	if p == "~" {
		return "", true
	}
	return strings.CutPrefix(p, "~/")
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
// on disk, the found list, the used record and the Git tab's repos, never
// the network.
func (s *Service) planClone(r gitremote.Remote) (proto.ClonePlan, error) {
	root := s.cloneRoot()
	if root == "" {
		return proto.ClonePlan{}, errors.New("no clone_root is set")
	}
	remote := s.GitRemote // [git] remote
	if remote == "" {
		remote = "origin"
	}
	s.mu.Lock()
	running := map[string]bool{} // by lower case: see cloningInto
	for t, j := range s.clones {
		if j.end.IsZero() {
			running[strings.ToLower(t)] = true
		}
	}
	found := slices.Clone(s.found.Repos)
	// The used record's places and the Git tab's repos: each is read now.
	var used []string
	for _, u := range s.used.Repos {
		used = append(used, u.Path)
	}
	if s.cur.Git != nil {
		for _, g := range s.cur.Git.Repos {
			used = append(used, g.Path)
		}
	}
	s.mu.Unlock()

	taken := func(name string) bool {
		t := filepath.Join(root, name)
		return exists(t) || running[strings.ToLower(t)] || s.siblingHeld(root, t)
	}
	p := proto.ClonePlan{Repo: r.Slug(), URL: r.URL, Target: filepath.Join(root, r.Repo)}
	same := false
	switch fi, err := os.Stat(p.Target); {
	case err == nil && fi.IsDir():
		if c, ok := gitscan.ReadCheckout(p.Target, remote); !ok {
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
		if c.Main != "" || seen[c.Path] || !gitremote.Same(c.Remote, r.URL) {
			continue
		}
		// The list may be days old: the checkout is read now.
		if now, ok := gitscan.ReadCheckout(c.Path, remote); ok && now.Main == "" && gitremote.Same(now.Remote, r.URL) {
			seen[c.Path] = true
			known = append(known, c.Path)
		}
	}
	for _, u := range used {
		if seen[u] {
			continue
		}
		if c, ok := gitscan.ReadCheckout(u, remote); ok && c.Main == "" && gitremote.Same(c.Remote, r.URL) {
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
	j, msg := s.registerClone(ctx, r, target, cs.Token)
	if msg != "" {
		return proto.Result{Err: msg}
	}
	go s.runClone(j.ctx, j, r)
	return proto.Result{CloneID: j.c.ID}
}

// registerClone checks target again and lists a clone of r into it, whose
// context ends with ctx, with the token its start carried. It returns a
// message when the clone cannot start.
func (s *Service) registerClone(ctx context.Context, r gitremote.Remote, target, token string) (*cloneJob, string) {
	root := s.cloneRoot()
	if root == "" {
		return nil, "no clone_root is set"
	}
	if base := filepath.Base(target); filepath.Dir(target) != root || strings.Contains(base, ".cloning") {
		return nil, tildeIn(fmt.Sprintf("a clone goes into a folder of its own in %s: %s is not one", root, target))
	}
	if err := os.MkdirAll(root, 0o755); err != nil { //nolint:gosec // the builder's own source folder
		return nil, tildeIn(fmt.Sprintf("cannot make %s: %v", root, err))
	}
	s.mu.Lock()
	if s.cloningInto(target) {
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
	if s.cloneSeq == 0 {
		s.cloneSeq = time.Now().UnixNano() // ids never repeat across a restart
	}
	s.cloneSeq++
	j := &cloneJob{ctx: jctx, cancel: cancel, done: make(chan struct{}), c: proto.Clone{ID: s.cloneSeq, Target: target, Repo: r.Slug(), Pct: -1, Token: token}}
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
// session starts, a line that says what happened, and whether it cloned:
// a checkout already there is reused. It clones on the act's own
// goroutine, and stops when ctx ends.
func (s *Service) cloneForNew(ctx context.Context, r gitremote.Remote) (dir, said string, cloned bool, errText string) {
	p, err := s.planClone(r)
	if err != nil {
		return "", "", false, err.Error()
	}
	if len(p.Have) > 0 {
		return p.Have[0], fmt.Sprintf("%s is already at %s: started there", p.Repo, tildeIn(p.Have[0])), false, ""
	}
	j, msg := s.registerClone(ctx, r, p.Target, "")
	if msg != "" {
		return "", "", false, msg
	}
	s.runClone(j.ctx, j, r)
	s.mu.Lock()
	failed := j.c.Err
	s.mu.Unlock()
	if failed != "" {
		return "", "", false, failed
	}
	return p.Target, fmt.Sprintf("cloned %s into %s", p.Repo, tildeIn(p.Target)), true, ""
}

// runClone runs the clone j and ends its record, whatever the run does: a
// panic in it is the job's error, and the job never stays busy.
func (s *Service) runClone(ctx context.Context, j *cloneJob, r gitremote.Remote) {
	var err error
	defer func() {
		if p := recover(); p != nil {
			err = errors.New("the clone stopped on an error in matchblox")
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
	}()
	run := s.cloneRun
	if run == nil {
		run = clone.Run
	}
	err = run(ctx, r, j.c.Target, func(pct int) {
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
}

// cloneStop is the clone:stop act: Text is a proto.CloneStop, or the
// bare target from an older console. It stops only the clone of that ID
// and waits, within a bound, until the partial folder is gone. A clone
// that was done is said in Note: its folder stays.
func (s *Service) cloneStop(text string) proto.Result {
	cs := proto.CloneStop{Target: text}
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		if err := json.Unmarshal([]byte(text), &cs); err != nil {
			return proto.Result{Err: "the stop does not read: " + err.Error()}
		}
	}
	target := ExpandHome(strings.TrimSpace(cs.Target))
	s.mu.Lock()
	j := s.clones[target]
	s.mu.Unlock()
	if j == nil {
		return proto.Result{Err: tildeIn(target + " is not being cloned")}
	}
	if cs.ID != 0 && cs.ID != j.c.ID { // ID never changes: no lock
		return proto.Result{Skipped: []string{"that clone is not running"}}
	}
	j.cancel()
	select {
	case <-j.done:
	case <-time.After(cloneStopWait):
	}
	s.mu.Lock()
	done := j.c.Done
	s.mu.Unlock()
	if done {
		return proto.Result{Note: fmt.Sprintf("the clone of %s was done: it is in %s", j.c.Repo, tildeIn(target))}
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
// folder named as target's sibling directly in clone_root goes, and only
// when matchblox made it and its maker is gone: the owner file beside it
// names a pid that is not alive. A folder with no owner file, or whose
// owner lives, is not touched. It returns where the folder went, for the
// caller to remove outside s.mu. Hold s.mu, and know no clone into target
// runs in this service.
func staleAway(root, target string) (string, bool) {
	sib := clone.Cloning(target)
	if filepath.Dir(sib) != root {
		return "", false
	}
	if pid, ok := clone.Owner(target); !ok || clone.Alive(pid) {
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
	_ = os.Remove(clone.OwnerFile(target))
	return trash, true
}

// cloningInto tells if a clone that runs works in target's work folder.
// A disk that ignores case makes .Foo.cloning and .foo.cloning one
// folder: a target that differs only in case counts, as does one whose
// work folder is the same. Hold s.mu.
func (s *Service) cloningInto(target string) bool {
	sib, sibErr := os.Lstat(clone.Cloning(target))
	for t, j := range s.clones {
		if !j.end.IsZero() {
			continue
		}
		if strings.EqualFold(t, target) {
			return true
		}
		if sibErr == nil {
			if fi, err := os.Lstat(clone.Cloning(t)); err == nil && os.SameFile(fi, sib) {
				return true
			}
		}
	}
	return false
}

// siblingHeld tells if target's work folder holds its name: a clone into
// it runs, or the folder is not one matchblox made and left (see
// staleAway). A stale one is removed.
func (s *Service) siblingHeld(root, target string) bool {
	if !exists(clone.Cloning(target)) {
		return false
	}
	s.mu.Lock()
	if s.cloningInto(target) {
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
