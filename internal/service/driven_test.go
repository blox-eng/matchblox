package service

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// guardedRun is a running order whose plan grants releases, after the
// stoker sent step 1 to work (%1), with the stoker agent idle in %9.
func guardedRun(t *testing.T) (*Service, *[][]string, *time.Time) {
	t.Helper()
	s, ran := orderTest(t)
	clock := night0
	s.Now = func() time.Time { return clock }
	s.Capture = func(panes []string) map[string]string {
		out := map[string]string{}
		for _, p := range panes {
			out[p] = idleScreen
		}
		return out
	}
	planned(t, s, "merge them on my word", false)
	s.stoke(t.Context(), clock) // the wakes see the run
	mustAct(t, s, fromStokerAct("message:%1", "merge #4556 on green"))
	return s, ran, &clock
}

// grantAct is y on the card's row whose last entry is id.
func grantAct(id string) proto.Act {
	return proto.Act{Console: true, RecID: "stoker:grant", Confirm: "y", Text: id}
}

// stepID is the id of step n's last logged entry in the run.
func stepID(t *testing.T, s *Service, n int) string {
	t.Helper()
	id := ""
	for _, e := range s.Stoker.View().Run {
		if e.Logged() && e.Step == n {
			id = e.ID
		}
	}
	if id == "" {
		t.Fatalf("the run has no step %d", n)
	}
	return id
}

// goSteps are the go steps matchblox put on the card, oldest first.
func goSteps(s *Service) []stoker.Entry {
	var out []stoker.Entry
	for _, e := range s.Stoker.View().Run {
		if e.Rule == stoker.RuleGo {
			out = append(out, e)
		}
	}
	return out
}

// denies is the hook's guard in pane: a pane of the stoker's server
// (%9@mbtest) runs on its socket, any other on the builder's.
func denies(t *testing.T, s *Service, pane, cmd string) bool {
	t.Helper()
	sock := "/tmp/tmux-test/default"
	server, id := sample.SplitPane(pane)
	if server != "" {
		sock = s.stokerSocket()
	}
	return stoker.Denies(s.Stoker.Dir(), sock, id, stoker.Guards(cmd))
}

func TestStokerSendMarksThePaneWithTheOrdersGrants(t *testing.T) {
	s, _, _ := guardedRun(t)
	d, err := stoker.ReadDriven(s.Stoker.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if got, in := d.Panes["%1"]; !in || !slices.Equal(got, []string{"release"}) || len(d.Panes) != 1 {
		t.Fatalf("driven %+v", d)
	}
	switch {
	case !denies(t, s, "%1", "gh pr merge 4556"):
		t.Fatal("the order gave no merge, yet the driven pane may merge")
	case denies(t, s, "%1", "gh release create v1"):
		t.Fatal("the order's release was denied")
	case denies(t, s, "%2", "gh pr merge 4556"):
		t.Fatal("a pane the stoker never drove was denied")
	}
}

func TestGrantIsUsedOnce(t *testing.T) {
	s, ran, clock := guardedRun(t)
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":1,"state":"waits","needs":"merge:#4556","text":"green, waits for the merge"}`))
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":2,"state":"doing","text":"rebasing"}`))
	if res := s.act(t.Context(), fromStokerAct("stoker:grant", stepID(t, s, 1))); res.Err != "the stoker does not give itself the go" {
		t.Fatalf("the stoker's own grant: %q", res.Err)
	}
	for text, want := range map[string]string{stepID(t, s, 2): "the step waits for no go", "x": "the run has no such step"} {
		if res := s.act(t.Context(), grantAct(text)); res.Err != want {
			t.Errorf("grant %s: %q, want %q", text, res.Err, want)
		}
	}
	mustAct(t, s, grantAct(stepID(t, s, 1)))
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the go given for step 1 did not let its merge through")
	}
	if !denies(t, s, "%1", "gh pr merge 4557") {
		t.Fatal("the go for #4556 let another merge through")
	}

	// The stoker hears of it at its next wake.
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if said := typed(*ran, "%9@mbtest"); !slices.Contains(said, "matchblox: go given for work: merge:#4556") {
		t.Fatalf("the stoker was told %q", said)
	}

	// A denied call uses nothing; the merge that ran uses its go.
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "gh pr merge 4556", Guards: stoker.Guards("gh pr merge 4556"), Denied: true})
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a denied call used the go")
	}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "cd api && gh pr merge 4556 --squash", Guards: stoker.Guards("cd api && gh pr merge 4556 --squash")})
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the go was used and still lets the merge through")
	}
	if denies(t, s, "%1", "gh release create v1") {
		t.Fatal("using the merge took the order's release")
	}
}

// TestAStepsReleaseIsUsedOnce: Ruling S, a step's go of any kind is good
// for one call; the order's own goes last the run.
func TestAStepsReleaseIsUsedOnce(t *testing.T) {
	s, _, clock := guardedRun(t)
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":1,"state":"waits","needs":"deploy","text":"waits for the deploy"}`))
	mustAct(t, s, grantAct(stepID(t, s, 1)))
	if denies(t, s, "%1", "gh workflow run Release") {
		t.Fatal("the step's deploy was denied")
	}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "gh workflow run Release", Guards: stoker.Guards("gh workflow run Release")})
	if !denies(t, s, "%1", "gh workflow run Release") {
		t.Fatal("the step's deploy went through twice")
	}
	// The order's release is used by no call.
	for range 2 {
		s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "gh release create v1", Guards: stoker.Guards("gh release create v1")})
	}
	if denies(t, s, "%1", "gh release create v1") {
		t.Fatal("a release used up the order's go")
	}
}

// TestTheBuildersMessageFreesThePane: Ruling R, the last input of the pane
// is the builder's, so the guard lets go of it.
func TestTheBuildersMessageFreesThePane(t *testing.T) {
	s, _, _ := guardedRun(t)
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the driven pane merged")
	}
	// A command's message is no person at the keys, whoever sends it.
	mustAct(t, s, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it"})
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a CLI message freed the pane")
	}
	mustAct(t, s, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it now", Console: true})
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the builder's own word did not free the pane")
	}
}

func TestDrivenEndsWithTheRun(t *testing.T) {
	s, _, _ := guardedRun(t)
	path := filepath.Join(s.Stoker.Dir(), "driven.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	mustAct(t, s, proto.Act{Console: true, RecID: "stoker:off", Confirm: "y"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the run ended and driven.json stays: %v", err)
	}
	if denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a merge was denied with no run")
	}
}

func TestDrivenComesBackWhileTheRunIsOn(t *testing.T) {
	s, _, clock := guardedRun(t)
	path := filepath.Join(s.Stoker.Dir(), "driven.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	s.stoke(t.Context(), *clock)
	d, err := stoker.ReadDriven(s.Stoker.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if _, in := d.Panes["%1"]; !in {
		t.Fatalf("the mark of %%1 was lost: %+v", d)
	}
}

func TestTheStokersOwnPanesAreGuardedDuringARun(t *testing.T) {
	s, _, clock := guardedRun(t)
	s.mu.Lock()
	s.cur.Panes = []sample.Pane{{ID: "%9@mbtest", Target: "stoker:1.1", Socket: "/tmp/tmux-test/mbtest"}, {ID: "%11@mbtest", Target: "stoker:2.1", Socket: "/tmp/tmux-test/mbtest"}, {ID: "%1", Target: "app:1.1"}}
	s.mu.Unlock()
	s.stoke(t.Context(), *clock)
	d, err := stoker.ReadDriven(s.Stoker.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if d.StokerSocket != "/tmp/tmux-test/mbtest" {
		t.Fatalf("the stoker's server is %q", d.StokerSocket)
	}
	// The builder's %9 is not the stoker's: the order's release holds there.
	if denies(t, s, "%9", "gh release create v1") {
		t.Fatal("the builder's %9 was taken for the stoker's")
	}
	for _, pane := range []string{"%9@mbtest", "%11@mbtest"} {
		// The order gives releases: still none in the stoker's own panes.
		for _, cmd := range []string{"gh pr merge 4556", "gh release create v1"} {
			if !denies(t, s, pane, cmd) {
				t.Fatalf("%s may run %q", pane, cmd)
			}
		}
	}
	if denies(t, s, "%1", "gh release create v1") {
		t.Fatal("the driven pane lost the order's release")
	}
	// A sample that read no tmux keeps the panes it knew.
	s.mu.Lock()
	s.cur.Panes = nil
	s.mu.Unlock()
	s.refreshStoker()
	if !denies(t, s, "%11@mbtest", "gh pr merge 4556") {
		t.Fatal("a missed tmux read opened the stoker's pane")
	}
	mustAct(t, s, proto.Act{Console: true, RecID: "stoker:off", Confirm: "y"})
	if denies(t, s, "%9@mbtest", "gh pr merge 4556") {
		t.Fatal("no run, yet the stoker's pane is guarded")
	}
}

func TestASpooledCallUsesItsStepsGo(t *testing.T) {
	s, _, clock := guardedRun(t)
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":1,"state":"waits","needs":"merge:#4556","text":"green"}`))
	mustAct(t, s, grantAct(stepID(t, s, 1)))
	// The merge's hook missed the service and went to the spool.
	s.Spool = filepath.Join(t.TempDir(), "spool.jsonl")
	if err := hooks.Append(s.Spool, hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "gh pr merge 4556", Guards: stoker.Guards("gh pr merge 4556")}); err != nil {
		t.Fatal(err)
	}
	s.readSpool()
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("a spooled merge left its go for a second one")
	}
}

// TestACallUsesTheGoTheHookSaw: the hook guards the raw line and sends
// its guards with the scrubbed one, so a scrubbed line that no longer
// reads as a merge still uses the merge's go.
func TestACallUsesTheGoTheHookSaw(t *testing.T) {
	s, _, clock := guardedRun(t)
	mustAct(t, s, fromStokerAct("stoker:step", `{"step":1,"state":"waits","needs":"merge:#5","text":"green"}`))
	mustAct(t, s, grantAct(stepID(t, s, 1)))
	raw := `echo "set A='1" && gh pr merge 5 && echo 'ok'`
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: hooks.Scrub(raw), Guards: stoker.Guards(raw)})
	if !denies(t, s, "%1", "gh pr merge 5") {
		t.Fatal("the merge ran and its go is still there")
	}
}

// TestADeniedMergeWaitsForYouOnTheCard: with merges off, a driven
// session's merge is denied, and matchblox itself puts it on the card as
// a step that waits for y; y lets exactly that merge through, once.
func TestADeniedMergeWaitsForYouOnTheCard(t *testing.T) {
	s, ran, clock := guardedRun(t)
	deny := func(pane, cmd string) {
		s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: pane, At: *clock, Command: cmd, Guards: stoker.Guards(cmd),
			Denied: denies(t, s, pane, cmd)})
	}
	deny("%1", "gh pr merge 4556 --squash")
	deny("%1", "gh pr merge 4556 --squash") // the same go is asked once
	deny("%2", "gh pr merge 4570")          // no pane the stoker drives
	deny("%9@mbtest", "gh pr merge 4556")   // the stoker's own
	gs := goSteps(s)
	if len(gs) != 1 {
		t.Fatalf("go steps %+v", gs)
	}
	g := gs[0]
	if g.State != stoker.StepWaits || g.Needs != "merge:#4556" || g.Pane != "%1" || g.Name != "work" ||
		g.Text != "work wants to run gh pr merge 4556 --squash" || g.Want != "gh pr merge 4556 --squash" || g.Logged() == false {
		t.Fatalf("the go step %+v", g)
	}
	mustAct(t, s, grantAct(g.ID))
	if denies(t, s, "%1", "gh pr merge 4556 --squash") {
		t.Fatal("y did not let the merge through")
	}
	if !denies(t, s, "%1", "gh pr merge 4557") {
		t.Fatal("y let another merge through")
	}
	if res := s.act(t.Context(), grantAct(g.ID)); res.Err != "the step waits for no go" {
		t.Fatalf("a second y: %q", res.Err)
	}
	if gs := goSteps(s); len(gs) != 2 || gs[1].State != stoker.StepDone || gs[1].Text != "go given: gh pr merge 4556 --squash" {
		t.Fatalf("the go given is not on the card: %+v", gs)
	}
	*clock = clock.Add(time.Minute)
	s.stoke(t.Context(), *clock)
	if said := typed(*ran, "%9@mbtest"); !slices.Contains(said, "matchblox: go given for work: gh pr merge 4556 --squash") {
		t.Fatalf("the stoker was told %q", said)
	}
	// The merge runs and uses its go; the next one asks again.
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: "gh pr merge 4556 --squash", Guards: stoker.Guards("gh pr merge 4556 --squash")})
	deny("%1", "gh pr merge 4556 --squash")
	if gs := goSteps(s); len(gs) != 3 || gs[2].State != stoker.StepWaits {
		t.Fatalf("a merge after its go was used did not ask again: %+v", gs)
	}
}

// TestABareMergeWaitsForTheSessionsOwn: a merge that names no pull request
// asks for the session's own merge, and y lets its branch's merge through.
func TestABareMergeWaitsForTheSessionsOwn(t *testing.T) {
	s, _, clock := guardedRun(t)
	ship := func(n int, pane string) proto.Ship {
		return proto.Ship{Repo: "o/r", Number: n, Session: ships.SessionRef{Pane: pane}, Evidence: ships.Created}
	}
	s.mu.Lock()
	s.cur.Ships = &ships.Report{Ships: []proto.Ship{ship(4569, "%1"), ship(4570, "%2")}}
	s.mu.Unlock()
	cmd := "gh pr merge --squash"
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd), Denied: denies(t, s, "%1", cmd)})
	gs := goSteps(s)
	if len(gs) != 1 || gs[0].Needs != "merge:@%1" {
		t.Fatalf("go steps %+v", gs)
	}
	mustAct(t, s, grantAct(gs[0].ID))
	switch {
	case denies(t, s, "%1", cmd):
		t.Fatal("y did not let the branch's merge through")
	case !denies(t, s, "%1", "gh pr merge 4570"):
		t.Fatal("y let work merge cx's PR")
	}
}

// TestALineWithTwoGuardedCommandsNeedsAGoForEach: one y lets one guarded
// command through, so a line that runs two asks twice, and the call that
// runs uses up both gos.
func TestALineWithTwoGuardedCommandsNeedsAGoForEach(t *testing.T) {
	s, _, clock := guardedRun(t)
	cmd := "gh pr merge 4556 --squash && gh pr merge 4556 --admin"
	ask := func() stoker.Entry {
		t.Helper()
		*clock = clock.Add(time.Second)
		s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd), Denied: denies(t, s, "%1", cmd)})
		gs := goSteps(s)
		if len(gs) == 0 || gs[len(gs)-1].State != stoker.StepWaits {
			t.Fatalf("the denied line did not ask: %+v", gs)
		}
		return gs[len(gs)-1]
	}
	mustAct(t, s, grantAct(ask().ID))
	if !denies(t, s, "%1", cmd) {
		t.Fatal("one y let two merges through")
	}
	mustAct(t, s, grantAct(ask().ID))
	if denies(t, s, "%1", cmd) {
		t.Fatal("two y did not let the line through")
	}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd)})
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("the line ran and a go is still there")
	}
}

// TestABareMergesGoIsUsedByOneMerge: the y on a merge that names no pull
// request lets one merge through, and the next asks again.
func TestABareMergesGoIsUsedByOneMerge(t *testing.T) {
	s, _, clock := guardedRun(t)
	s.mu.Lock()
	s.cur.Ships = &ships.Report{Ships: []proto.Ship{{Repo: "o/r", Number: 4569, Session: ships.SessionRef{Pane: "%1"}, Evidence: ships.Created}}}
	s.mu.Unlock()
	cmd := "gh pr merge --squash"
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd), Denied: denies(t, s, "%1", cmd)})
	gs := goSteps(s)
	if len(gs) != 1 || gs[0].Needs != "merge:@%1" {
		t.Fatalf("go steps %+v", gs)
	}
	mustAct(t, s, grantAct(gs[0].ID))
	if denies(t, s, "%1", cmd) {
		t.Fatal("y did not let the branch's merge through")
	}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd)})
	for _, again := range []string{cmd, "gh pr merge 4569"} {
		if !denies(t, s, "%1", again) {
			t.Fatalf("the go outlived its merge: %q went through", again)
		}
	}
}

// TestTwoYAtOnceGiveOneGo: two consoles press y on the same go step at the
// same moment; it gives one go, and the card says so once.
func TestTwoYAtOnceGiveOneGo(t *testing.T) {
	s, _, clock := guardedRun(t)
	cmd := "gh pr merge 4556 --squash"
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: *clock, Command: cmd, Guards: stoker.Guards(cmd),
		Denied: denies(t, s, "%1", cmd)})
	g := goSteps(s)[0]
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { s.act(t.Context(), grantAct(g.ID)) })
	}
	wg.Wait()
	d, err := stoker.ReadDriven(s.Stoker.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(d.Steps["%1"]); n != 1 {
		t.Fatalf("%d gos for one step: %v", n, d.Steps["%1"])
	}
	if gs := goSteps(s); len(gs) != 2 {
		t.Fatalf("go steps %+v", gs)
	}
}

// TestAShipNobodyOwnsIsNoSessionsOwn: a ship whose session is gone has no
// pane, and a session with no pane does not take it for its own merge.
func TestAShipNobodyOwnsIsNoSessionsOwn(t *testing.T) {
	s := &Service{}
	s.cur.Sessions = []sample.Session{{SessionID: "s1", Name: "work"}, {SessionID: "s2", Pane: "%2"}}
	s.cur.Ships = &ships.Report{Ships: []proto.Ship{
		{Repo: "o/r", Number: 4569, Evidence: ships.Created},
		{Repo: "o/r", Number: 4570, Session: ships.SessionRef{Pane: "%2", ID: "s2"}, Evidence: ships.Branch},
	}}
	own := s.owns()
	if prs := own[""].PRs; len(prs) != 0 {
		t.Errorf("the paneless session owns %v", prs)
	}
	if prs := own["%2"].PRs; !slices.Equal(prs, []int{4570}) {
		t.Errorf("%%2 owns %v, want [4570]", prs)
	}
}
