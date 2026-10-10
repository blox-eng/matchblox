package advice

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestBuildRanksAndActs(t *testing.T) {
	snap := sample.Snapshot{
		Orphans: []sample.Orphan{{PID: 42, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init",
			Pane: "%3", PaneAlive: true, Target: "w:1.1", Kill: []string{"kill", "42"}}},
		Sessions: []sample.Session{
			{Pane: "%1", Name: "busy-one", Busy: true, Do: "compact", ContextPct: 90, Why: "context 90%"},
			{Pane: "%2", Name: "idle-one", Status: "idle", Do: "clear", ContextPct: 40, Why: "idle 2h"},
			{Pane: "%4", Name: "fine"},
		},
		GitPolling: []sample.GitPolling{{Checkout: "/w/app", Cores: 2}},
	}
	git := &gitscan.Report{Repos: []gitscan.Repo{{
		Path: "/w/app", Main: "main", Behind: 3,
		Worktrees: []gitscan.Worktree{
			{Path: "/w/app", Branch: "main", Sessions: 3, Dirty: 900},
			{Path: "/w/wt/a", Safe: true, Remove: []string{"git", "-C", "/w/app", "worktree", "remove", "/w/wt/a"}},
			{Path: "/w/wt/b", Safe: true, Remove: []string{"git", "-C", "/w/app", "worktree", "remove", "/w/wt/b"}},
		},
	}}}
	recs := Build(snap, git)

	var titles []string
	for _, r := range recs {
		titles = append(titles, r.Title)
	}
	// Two cores of git polling cost the machine more than one session's
	// context, so it ranks above the compact.
	want := []string{
		"Kill detached busy loop 42 (bash)",
		"Clean up /w/app",
		"Compact busy-one (%1)",
		"Update main in app",
		"Remove 2 merged worktrees of app",
		"Clear idle-one (%2)",
	}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("order:\n got %q\nwant %q", titles, want)
	}

	kill := recs[0]
	if kill.Second == nil || !kill.Second.Destructive || strings.Join(kill.Second.Steps[0], " ") != "kill 42" || kill.Primary == nil ||
		len(kill.Second.Guards) != 1 || kill.Second.Guards[0].PID != 42 {
		t.Errorf("kill rec = %+v", kill)
	}
	if recs[2].Second != nil {
		t.Error("a busy session must not be offered typed input")
	}
	if c := recs[5].Second; c == nil || !c.Destructive || strings.Join(c.Steps[0], " ") != "tmux send-keys -t %2 /clear Enter" {
		t.Errorf("clear action = %+v", c)
	}
	if rm := recs[4].Second; rm == nil || !rm.Destructive || len(rm.Steps) != 2 || len(rm.Guards) != 2 || rm.Guards[1].Worktree != "/w/wt/b" {
		t.Errorf("bulk remove = %+v", rm)
	}
	if ff := recs[3].Second; ff == nil || !ff.Destructive || !strings.HasSuffix(strings.Join(ff.Steps[0], " "), "git -C /w/app pull --ff-only") {
		t.Errorf("fast-forward = %+v", ff)
	}
	for _, r := range recs {
		if r.Primary == nil && r.Second == nil {
			t.Errorf("%q has no action", r.Title)
		}
	}
}

func TestNeverTypesIntoANonIdleSession(t *testing.T) {
	for _, status := range []string{"busy", "shell", ""} {
		snap := sample.Snapshot{Sessions: []sample.Session{{Pane: "%1", Name: "x", Status: status, Do: "clear"}}}
		if r := Build(snap, nil); len(r) != 1 || r[0].Second != nil {
			t.Errorf("status %q: %+v", status, r)
		}
	}
}

func TestNothingToDo(t *testing.T) {
	if recs := Build(sample.Snapshot{Sessions: []sample.Session{{Pane: "%1"}}}, nil); len(recs) != 0 {
		t.Fatalf("got %+v", recs)
	}
}

func TestAlertRecs(t *testing.T) {
	const gb = 1 << 30
	snap := sample.Snapshot{
		Machine: sample.Machine{MemAvail: 40 * gb, SwapUsed: 12 * gb},
		Alerts:  []sample.Alert{{Key: "swap-stuck", Evidence: "swap 12 GB"}, {Key: "load1", Evidence: "load1 50"}},
		Top:     []sample.ProcRow{{PID: 7, Comm: "node", CPU: 300, Owner: "pane %5"}},
	}
	recs := Build(snap, nil)
	if len(recs) != 2 || recs[0].Primary == nil || strings.Join(recs[0].Primary.Steps[0], " ") != "tmux switch-client -t %5" {
		t.Fatalf("load rec: %+v", recs)
	}
	if sw := recs[1].Second; sw == nil || !sw.Destructive || sw.Steps[0][len(sw.Steps[0])-1] != "swapoff -a && swapon -a" {
		t.Fatalf("swap rec: %+v", recs[1])
	}

	// Not enough free RAM to take the swap back: no offer.
	snap.Machine.MemAvail = 13 * gb
	snap.Top[0].Owner = "system"
	if recs := Build(snap, nil); len(recs) != 0 {
		t.Fatalf("got %+v", recs)
	}
}

func TestRecIDIsStableAndDistinct(t *testing.T) {
	snap := sample.Snapshot{Sessions: []sample.Session{
		{Pane: "%1", Name: "a", Busy: true, Do: "compact", ContextPct: 90, Why: "context 90%"},
		{Pane: "%2", Name: "b", Busy: true, Do: "compact", ContextPct: 91, Why: "context 91%"},
	}}
	first, again := Build(snap, nil), Build(snap, nil)
	if len(first) < 2 {
		t.Fatalf("want 2 recs, got %d", len(first))
	}
	seen := map[string]bool{}
	for i, r := range first {
		if r.ID == "" || r.ID != again[i].ID {
			t.Fatalf("rec %q: id %q is not stable (%q)", r.Title, r.ID, again[i].ID)
		}
		if seen[r.ID] {
			t.Fatalf("id %q repeats", r.ID)
		}
		seen[r.ID] = true
	}
}

// A step that moves the person's own terminal runs in the console; every
// other step runs on the service host.
func TestOnlyGoToActionsAreNav(t *testing.T) {
	snap := sample.Snapshot{
		Orphans: []sample.Orphan{{PID: 42, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init",
			Pane: "%3", PaneAlive: true, Target: "w:1.1", Kill: []string{"kill", "42"}}},
		Sessions: []sample.Session{
			{Pane: "%1", Name: "busy-one", Busy: true, Do: "compact", ContextPct: 90, Why: "context 90%"},
			{Pane: "%2", Name: "idle-one", Status: "idle", Do: "clear", ContextPct: 40, Why: "idle 2h"},
		},
		GitPolling: []sample.GitPolling{{Checkout: "/w/app", Cores: 2}},
	}
	git := &gitscan.Report{Repos: []gitscan.Repo{{Path: "/w/app", Main: "main", Behind: 3,
		Worktrees: []gitscan.Worktree{{Path: "/w/wt/a", Safe: true, Remove: []string{"git", "worktree", "remove", "/w/wt/a"}}}}}}
	n := 0
	for _, r := range Build(snap, git) {
		for _, a := range []*Action{r.Primary, r.Second} {
			if a == nil {
				continue
			}
			goTo := a.Steps[0][0] == "tmux" && (a.Steps[0][1] == "switch-client" || a.Steps[0][1] == "new-window")
			if a.Nav != goTo {
				t.Errorf("%q: Nav=%v for %v", r.Title, a.Nav, a.Steps)
			}
			if goTo {
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("no go-to action in the fixture")
	}
}

// The id binds every step the person saw: a rec whose later steps change
// is a different rec, so an act cannot run steps that were never shown.
func TestRecIDBindsEveryStep(t *testing.T) {
	a := Rec{Title: "Remove 2 merged worktrees", Second: &Action{Steps: [][]string{{"git", "worktree", "remove", "/w/a"}, {"git", "worktree", "remove", "/w/b"}}}}
	b := Rec{Title: "Remove 2 merged worktrees", Second: &Action{Steps: [][]string{{"git", "worktree", "remove", "/w/a"}, {"git", "worktree", "remove", "/w/c"}}}}
	c := Rec{Title: "t", Primary: &Action{Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}, Second: &Action{Steps: [][]string{{"kill", "1"}}}}
	d := Rec{Title: "t", Primary: &Action{Steps: [][]string{{"tmux", "switch-client", "-t", "%1"}}}, Second: &Action{Steps: [][]string{{"kill", "2"}}}}
	if recID(a) == recID(b) {
		t.Fatal("a changed later step kept the id")
	}
	if recID(c) == recID(d) {
		t.Fatal("a changed secondary action kept the id")
	}
}

// TestAWeekThatRunsShortRecommendsItsHottestSession: when an account's week
// will not last, the queue names the session of that account that burns
// the most, with a jump to it and, while it is idle, a guarded /compact.
func TestAWeekThatRunsShortRecommendsItsHottestSession(t *testing.T) {
	out := time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC)
	sparks := 3
	acct := limits.Account{Provider: "anthropic", Account: "a@example.com · Max", State: limits.Measured,
		Sparks: &sparks, Forecast: &limits.Outlook{Out: out}}
	other := limits.Account{Provider: "openai", Account: "b@example.com · Plus", State: limits.Measured,
		Sparks: &sparks, Forecast: &limits.Outlook{Lasts: true}}
	snap := sample.Snapshot{
		At:     time.Date(2026, 10, 13, 14, 0, 0, 0, time.UTC),
		Limits: []limits.Account{acct, other},
		Sessions: []sample.Session{
			{Name: "docs", Pane: "%1", Provider: "anthropic", Account: acct.Account, Burn30m: 9000, Tokens: 40000, Status: "idle"},
			{Name: "api-auth", Pane: "%2", Provider: "anthropic", Account: acct.Account, Burn30m: 42000, Tokens: 182000, Status: "idle"},
			{Name: "codex", Pane: "%3", Provider: "openai", Account: other.Account, Burn30m: 90000},
			// A session idle a day burns nothing: a compact saves no week.
			{Name: "old", Pane: "%4", Provider: "anthropic", Account: acct.Account, Tokens: 190000, Status: "idle", Idle: 25 * time.Hour},
		},
	}
	var got []Rec
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "week") {
			got = append(got, r)
		}
	}
	if len(got) != 1 {
		t.Fatalf("recs: %+v", got)
	}
	r := got[0]
	if r.Title != "The week runs short: compact api-auth" || r.Level != "warn" || !strings.HasPrefix(r.Evidence, "out Wed ~19:00 · ") {
		t.Fatalf("title %q level %q", r.Title, r.Level)
	}
	if !strings.Contains(r.Evidence, "a@example.com · Max: 3 sparks left") || !strings.Contains(r.Evidence, "api-auth burns 42k tokens in 30 min") {
		t.Fatalf("evidence %q", r.Evidence)
	}
	if r.Primary == nil || !r.Primary.Nav || strings.Join(r.Primary.Steps[0], " ") != "tmux switch-client -t %2" {
		t.Fatalf("primary %+v", r.Primary)
	}
	if r.Second == nil || !r.Second.Destructive || r.Second.Guards[0].IdlePane != "%2" || strings.Join(r.Second.Steps[0], " ") != "tmux send-keys -t %2 /compact Enter" {
		t.Fatalf("secondary %+v", r.Second)
	}
	// The forecast moves every sample: the rec keeps its id, so an act
	// confirmed meanwhile still finds it.
	id := r.ID
	acct.Forecast.Out = out.Add(time.Hour)
	snap.Limits[0] = acct
	for _, x := range Build(snap, nil) {
		if strings.Contains(x.Title, "week") && x.ID != id {
			t.Fatalf("the id moved with the forecast: %s → %s", id, x.ID)
		}
	}
	// A spent week: a compact saves nothing, no rec.
	zero := 0
	spent := acct
	spent.Sparks = &zero
	snap.Limits[0] = spent
	for _, x := range Build(snap, nil) {
		if strings.Contains(x.Title, "week") {
			t.Fatalf("a rec for a spent week: %+v", x)
		}
	}
	snap.Limits[0] = acct
	// A busy session gets the jump, never a typed command.
	snap.Sessions[1].Status = "busy"
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "week") && r.Second != nil {
			t.Fatalf("a busy session got a /compact: %+v", r.Second)
		}
	}
	// With nobody burning, the biggest context costs the most per turn.
	for i := range snap.Sessions {
		snap.Sessions[i].Burn30m = 0
	}
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "week") && (!strings.HasSuffix(r.Title, "compact api-auth") || !strings.HasSuffix(r.Evidence, "api-auth holds the biggest context, 182k")) {
			t.Fatalf("with no burn: %q, %q", r.Title, r.Evidence)
		}
	}
	// No session on the account but a cold one: nothing to act on, no rec
	// (the header warns).
	snap.Sessions = snap.Sessions[2:]
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "week") {
			t.Fatalf("a rec with no session: %+v", r)
		}
	}
}
