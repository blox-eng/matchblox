package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
)

func usedPaths(u state.Used) []string {
	var out []string
	for _, r := range u.Repos {
		out = append(out, r.Path)
	}
	return out
}

func TestNewSessionRecordsWhereItStarted(t *testing.T) {
	s, _ := lifeTest(t)
	s.UsedPath = filepath.Join(t.TempDir(), "used.json")
	api := filepath.Join(t.TempDir(), "api")
	if err := os.Mkdir(api, 0o700); err != nil {
		t.Fatal(err)
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: api + "/", Agent: "claude", Name: "fresh"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Equal(got, []string{api}) {
		t.Fatalf("used.json %q", got)
	}
	st, _ := s.stateFor(&client{})
	if st.Used == nil || !slices.Equal(usedPaths(*st.Used), []string{api}) {
		t.Fatalf("snapshot %+v", st.Used)
	}
	// a refused one adds nothing
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: filepath.Join(api, "nope"), Agent: "claude", Name: "x"})); res.Err == "" {
		t.Fatal("not refused")
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); !slices.Equal(got, []string{api}) {
		t.Fatalf("used.json after a refusal %q", got)
	}
}

func TestNewSessionWithABranchRecordsRepoAndWorktree(t *testing.T) {
	s, _ := lifeTest(t)
	s.UsedPath = filepath.Join(t.TempDir(), "used.json")
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(tmp, "api")
	for _, c := range [][]string{{"init", "-q", repo}, {"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", c...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %s", err, out)
		}
	}
	if res := s.act(t.Context(), newAct(t, proto.NewSession{Where: repo, Branch: "feat/1-x", Agent: "claude"})); res.Err != "" {
		t.Fatal(res.Err)
	}
	wt := filepath.Join(tmp, "api-feat-1-x")
	got := usedPaths(state.ReadUsed(s.UsedPath))
	if len(got) != 2 || !slices.Contains(got, repo) || !slices.Contains(got, wt) {
		t.Fatalf("used.json %q", got)
	}
}

func TestUsedIsSentOnceThenOnChange(t *testing.T) {
	s, _ := lifeTest(t)
	s.UsedPath = filepath.Join(t.TempDir(), "used.json")
	s.loadUsed()
	cl := &client{}
	if st, _ := s.stateFor(cl); st.Used == nil {
		t.Fatal("the first snapshot has no Used")
	}
	if st, _ := s.stateFor(cl); st.Used != nil {
		t.Fatalf("unchanged, but sent: %+v", st.Used)
	}
	s.addUsed("/src/api")
	st, _ := s.stateFor(cl)
	if st.Used == nil || !slices.Equal(usedPaths(*st.Used), []string{"/src/api"}) {
		t.Fatalf("after addUsed: %+v", st.Used)
	}
}

func TestUsedNeedsNoScanConsent(t *testing.T) {
	s, _ := lifeTest(t)
	s.Find = nil
	s.ConsentPath = filepath.Join(t.TempDir(), "scan.json")
	if err := state.WriteConsent(s.ConsentPath, state.Consent{Answer: "no"}); err != nil {
		t.Fatal(err)
	}
	s.UsedPath = filepath.Join(t.TempDir(), "used.json")
	if err := state.WriteUsed(s.UsedPath, state.Used{}.Add(s.now(), "/src/old")); err != nil {
		t.Fatal(err)
	}
	s.loadUsed()
	s.addUsed("/src/new")
	st, _ := s.stateFor(&client{})
	if st.Used == nil || !slices.Equal(usedPaths(*st.Used), []string{"/src/new", "/src/old"}) {
		t.Fatalf("%+v", st.Used)
	}
	if got := usedPaths(state.ReadUsed(s.UsedPath)); len(got) != 2 {
		t.Fatalf("used.json %q", got)
	}
}
