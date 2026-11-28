package service

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func startBranchRepo(t *testing.T, branch string) string {
	t.Helper()
	dir, err := os.MkdirTemp(t.TempDir(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][]string{
		{"init", "-q", "-b", branch, dir},
		{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x"},
	} {
		cmd := exec.Command("git", c...)
		cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") })
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %q: %v\n%s", c, err, out)
		}
	}
	return dir
}

func TestStartRecordsTheBranchItStartedFrom(t *testing.T) {
	s, _ := lifeTest(t)
	repo := startBranchRepo(t, "dev-x")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: repo, Agent: "claude", Name: "fresh"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := s.State().StartBranch["fresh"]; got != "dev-x" {
		t.Fatalf("StartBranch %v, want fresh on dev-x", s.State().StartBranch)
	}
	st, _ := s.stateFor(&client{})
	if st.StartBranch["fresh"] != "dev-x" {
		t.Fatalf("the console does not get it: %v", st.StartBranch)
	}
}

func TestAPlanRecordsNoStartBranch(t *testing.T) {
	s, _ := lifeTest(t)
	repo := startBranchRepo(t, "dev-x")
	a := newAct(t, proto.NewSession{Where: repo, Agent: "claude", Name: "fresh"})
	a.Confirm = ""
	s.act(t.Context(), a)
	if got := s.State().StartBranch; len(got) != 0 {
		t.Fatalf("a plan recorded %v", got)
	}
}

func TestStartOutsideGitRecordsNoStartBranch(t *testing.T) {
	s, _ := lifeTest(t)
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: t.TempDir(), Agent: "claude", Name: "fresh"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := s.State().StartBranch; len(got) != 0 {
		t.Fatalf("recorded %v for a folder with no git", got)
	}
}

// A session in a new worktree has its own new branch checked out. That is
// no base: the review falls back to the default branch it came from.
func TestStartInANewWorktreeRecordsNoStartBranch(t *testing.T) {
	s, _ := lifeTest(t)
	repo := startBranchRepo(t, "main")
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: repo, Branch: "feat/1-x", Agent: "claude", Name: "fresh"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := s.State().StartBranch; len(got) != 0 {
		t.Fatalf("recorded %v for a worktree", got)
	}
}

func TestStartBranchGoesWithItsSession(t *testing.T) {
	s, _ := lifeTest(t)
	s.recordStartBranch("fresh", "dev-x")
	s.publish(lifeSnap(false), func(state.Doc) {}) // fresh is not in the sample yet
	if got := s.State().StartBranch["fresh"]; got != "dev-x" {
		t.Fatalf("a start the sampler has not seen was forgotten: %v", s.State().StartBranch)
	}
	snap := lifeSnap(false)
	snap.Sessions = append(snap.Sessions, sample.Session{Agent: "claude", PID: 150, Start: 7, Pane: "%5", Target: "fresh:1.1", SessionID: "s5", Name: "fresh", Status: "idle"})
	s.publish(snap, func(state.Doc) {})
	if got := s.State().StartBranch["fresh"]; got != "dev-x" {
		t.Fatalf("a live session lost its branch: %v", s.State().StartBranch)
	}
	s.publish(lifeSnap(false), func(state.Doc) {}) // it ended
	if got := s.State().StartBranch; len(got) != 0 {
		t.Fatalf("an ended session keeps its branch for a later one by hand: %v", got)
	}
}
