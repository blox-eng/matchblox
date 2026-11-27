package service

import (
	"context"
	"maps"
	"strings"
)

// startBranchOf is the branch dir is on now: `git rev-parse --abbrev-ref
// HEAD`. "" when dir is no checkout, git does not answer, or HEAD is
// detached: the review then diffs against the host's default branch.
func (s *Service) startBranchOf(ctx context.Context, dir string) string {
	git := s.WorkGit
	if git == nil {
		git = workGit
	}
	out, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(out)
	if err != nil || branch == "" || branch == "HEAD" || strings.ContainsAny(branch, "\n\x00") {
		return ""
	}
	return branch
}

// recordStartBranch keeps the branch a session started from, by the
// session's name, for the review. Only a start the owner confirmed in a
// folder that had a branch comes here.
func (s *Service) recordStartBranch(name, branch string) {
	if name == "" || branch == "" {
		return
	}
	s.mu.Lock()
	next := maps.Clone(s.cur.StartBranch)
	if next == nil {
		next = map[string]string{}
	}
	next[name] = branch
	s.cur.StartBranch = next
	delete(s.startSeen, name) // a new session of that name: seen again from its first sample
	s.mu.Unlock()
	s.broadcast()
}

// pruneStartBranch forgets the start branch of a session that was seen
// and is gone, so a later session by hand under the same name does not
// inherit it. A start the sampler has not seen yet stays. Hold s.mu.
func (s *Service) pruneStartBranch() {
	if len(s.cur.StartBranch) == 0 {
		return
	}
	live := map[string]bool{}
	for _, x := range s.cur.Sessions {
		live[x.Name] = true
	}
	var next map[string]string
	for name := range s.cur.StartBranch {
		switch {
		case live[name]:
			if s.startSeen == nil {
				s.startSeen = map[string]bool{}
			}
			s.startSeen[name] = true
		case s.startSeen[name]:
			if next == nil {
				next = maps.Clone(s.cur.StartBranch)
			}
			delete(next, name)
			delete(s.startSeen, name)
		}
	}
	if next != nil {
		if len(next) == 0 {
			next = nil
		}
		s.cur.StartBranch = next
	}
}
