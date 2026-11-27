package push

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

var t0 = time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)

const fp = "SHA256:8Q94Er9GLB5qGPeqlE+5hCoHnmgKwAvlRvrZkV8Mnb0"

func doc(ss []sample.Session, q []queue.Item, as ...limits.Account) state.Doc {
	var d state.Doc
	d.Sessions, d.Queue, d.Limits = ss, q, as
	return d
}

func sess(id string, busy bool, started time.Time) sample.Session {
	return sample.Session{SessionID: id, Busy: busy, Started: started}
}

func TestAWaitComesBeforeABurn(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("burn", true, t0), sess("wait", false, t0.Add(-time.Hour))},
		[]queue.Item{{SessionID: "wait", State: queue.StateQuestion, Since: t0}})
	got := Build(d, fp, n, t0)
	if got.Top == nil || got.Top.State != "waits" || got.Top.N != n.Session("wait") {
		t.Fatalf("top = %+v", got.Top)
	}
	if got.Burn != 1 || got.Wait != 1 {
		t.Fatalf("burn %d wait %d", got.Burn, got.Wait)
	}
}

func TestTheOldestWaitComesFirst(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("new", false, t0), sess("old", false, t0)},
		[]queue.Item{
			{SessionID: "new", State: queue.StatePermission, Since: t0.Add(-time.Minute)},
			{SessionID: "old", State: queue.StatePermission, Since: t0.Add(-time.Hour)},
		})
	got := Build(d, fp, n, t0)
	if got.Top.N != n.Session("old") || got.Wait != 2 {
		t.Fatalf("top = %+v wait %d", got.Top, got.Wait)
	}
}

func TestTheLastPromptedBurnComesFirst(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("a", true, t0.Add(-time.Hour)), sess("b", true, t0)}, nil)
	got := Build(d, fp, n, t0)
	if got.Top.State != "burns" || got.Top.N != n.Session("b") || got.Burn != 2 {
		t.Fatalf("top = %+v", got.Top)
	}
}

func TestDoneAndOutHaveTheirStates(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("a", false, t0)},
		[]queue.Item{{SessionID: "a", State: queue.StateFinished, Since: t0}})
	if got := Build(d, fp, n, t0); got.Top.State != "done" || got.Wait != 0 {
		t.Fatalf("done: %+v wait %d", got.Top, got.Wait)
	}
	out := t0.Add(2 * time.Hour)
	s := sess("b", false, t0)
	s.Out = out
	got := Build(doc([]sample.Session{s}, nil), fp, n, t0)
	if got.Top.State != "out" || !got.Top.Out.Equal(out) {
		t.Fatalf("out: %+v", got.Top)
	}
}

func TestProgressGoesInStepsOf5(t *testing.T) {
	for in, want := range map[int]int{0: 0, 4: 0, 5: 5, 47: 45, 99: 95, 100: 100, 140: 100, -3: 0} {
		n := openNumbers(t, t.TempDir())
		s := sess("a", true, t0)
		s.Progress = &sample.Progress{Pct: in}
		got := Build(doc([]sample.Session{s}, nil), fp, n, t0)
		if got.Top.Pct != want {
			t.Errorf("pct %d -> %d, want %d", in, got.Top.Pct, want)
		}
	}
}

func TestQuietHasNoTop(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	got := Build(doc([]sample.Session{sess("a", false, t0)}, nil), fp, n, t0)
	if got.Top != nil || got.Burn != 0 || got.Wait != 0 {
		t.Fatalf("got %+v", got)
	}
	if got.Accounts == nil {
		t.Fatal("accounts must be [] not null")
	}
}

func acct(name string, five, week float64, now time.Time, f *limits.Outlook) limits.Account {
	return limits.Account{Provider: "anthropic", Account: name, State: limits.Measured,
		Windows: []limits.Window{
			{UsedPct: week, Minutes: limits.WeekMinutes, ResetsAt: now.Add(5 * 24 * time.Hour)},
			{UsedPct: five, Minutes: limits.BoxMinutes, ResetsAt: now.Add(2 * time.Hour)},
		}, Forecast: f}
}

func TestUsageHasTheWindowsAndTheForecastOfEachAccount(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	out := t0.Add(time.Hour)
	d := doc(nil, nil,
		acct("a", 62.4, 31, t0, &limits.Outlook{Lasts: true}),
		acct("b", 93, 71.6, t0, &limits.Outlook{Out: out}))
	got := Build(d, fp, n, t0)
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts %+v", got.Accounts)
	}
	a, b := got.Accounts[0], got.Accounts[1]
	if a.N != n.Account("anthropic", "a") || len(a.Windows) != 2 || a.Windows[0].Name != "5h" ||
		a.Windows[0].Pct != 62 || a.Windows[1].Name != "wk" || a.Windows[1].Pct != 31 || !a.Lasts {
		t.Fatalf("a = %+v", a)
	}
	if b.Lasts || !b.RunsOut.Equal(out) || b.Windows[1].Pct != 72 {
		t.Fatalf("b = %+v", b)
	}
	if !a.Windows[0].Resets.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("resets %v", a.Windows[0].Resets)
	}
}

// An account no session works for now does not lead the glance: an idle
// Cursor login at 100% of its month kept the watch red (2026-11-26).
func TestAnAccountWithNoWorkingSessionIsIdle(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	busy := sample.Session{SessionID: "s1", Busy: true, Provider: "anthropic", Account: "a"}
	rests := sample.Session{SessionID: "s2", Provider: "anthropic", Account: "b"}
	d := doc([]sample.Session{busy, rests}, nil,
		acct("a", 10, 24, t0, &limits.Outlook{Lasts: true}),
		acct("b", 0, 100, t0, nil))
	got := Build(d, fp, n, t0)
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts %+v", got.Accounts)
	}
	idle := map[int]bool{}
	for _, a := range got.Accounts {
		idle[a.N] = a.Idle
	}
	if idle[n.Account("anthropic", "a")] || !idle[n.Account("anthropic", "b")] {
		t.Fatalf("idle %v", idle)
	}
	if !usageMoved([]AccountUse{{N: 1, Lasts: true}}, []AccountUse{{N: 1, Lasts: true, Idle: true}}) {
		t.Fatal("an account that goes idle sends a new state")
	}
}

// A window at 100 % while a session of its account still works stops
// nothing: Cursor goes on past its included month. It led the glance in red
// over Claude's 30 % week (2026-11-27).
func TestAFullWindowThatStillWorksIsSpentNotOut(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	works := sample.Session{SessionID: "c", Busy: true, Provider: "cursor", Account: "w"}
	month := limits.Account{Provider: "cursor", Account: "w", State: limits.Measured,
		Windows: []limits.Window{{UsedPct: 100, Minutes: 30 * 24 * 60, ResetsAt: t0.Add(20 * 24 * time.Hour)}}}
	full := acct("a", 100, 40, t0, nil) // a stopped Claude: no session works
	got := Build(doc([]sample.Session{works}, nil, month, full), fp, n, t0)
	byN := map[int]AccountUse{}
	for _, a := range got.Accounts {
		byN[a.N] = a
	}
	if w := byN[n.Account("cursor", "w")].Windows; len(w) != 1 || !w[0].Spent {
		t.Fatalf("the month that still works is not spent: %+v", w)
	}
	for _, w := range byN[n.Account("anthropic", "a")].Windows {
		if w.Spent {
			t.Fatalf("a full window with no session working is spent: %+v", w)
		}
	}
}

// The island leads with what needs the person: asks before a finished turn,
// the oldest first, at most 3, and the count of all (2026-11-27).
func TestWhatNeedsYouLeadsInOrder(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	ids := []string{"w1", "d1", "p1", "d2", "b"}
	var ss []sample.Session
	for _, id := range ids {
		ss = append(ss, sess(id, id == "b", t0))
	}
	q := []queue.Item{
		{SessionID: "w1", State: queue.StateQuestion, Since: t0.Add(-2 * time.Minute)},
		{SessionID: "d1", State: queue.StateFinished, Since: t0.Add(-5 * time.Minute)},
		{SessionID: "p1", State: queue.StatePermission, Since: t0.Add(-1 * time.Minute)},
		{SessionID: "d2", State: queue.StateFinished, Since: t0.Add(-4 * time.Minute)},
	}
	got := Build(doc(ss, q), fp, n, t0)
	want := []Need{{n.Session("w1"), "question"}, {n.Session("p1"), "command"}, {n.Session("d1"), "done"}}
	if got.Need != 4 || !slices.Equal(got.Needs, want) {
		t.Fatalf("need %d %+v, want 4 %+v", got.Need, got.Needs, want)
	}
	if got.Top == nil || got.Top.N != n.Session("w1") {
		t.Fatalf("top %+v, want w1", got.Top)
	}
}

func TestUsageSkipsWhatItCannotName(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	a := limits.Account{Provider: "p", Account: "a", State: limits.Measured, Windows: []limits.Window{
		{UsedPct: 10, Minutes: 3 * 24 * 60}, {UsedPct: 20, Minutes: 30 * 24 * 60}}}
	un := limits.Account{Provider: "p", Account: "none", State: limits.Unmeasured}
	got := Build(doc(nil, nil, un, a), fp, n, t0)
	if len(got.Accounts) != 1 || len(got.Accounts[0].Windows) != 1 || got.Accounts[0].Windows[0].Name != "mo" {
		t.Fatalf("got %+v", got.Accounts)
	}
}

func TestStokerOnIsInTheState(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc(nil, nil)
	if Build(d, fp, n, t0).Stoker {
		t.Fatal("no stoker is on")
	}
	d.Stoker = &stoker.View{Mode: stoker.Mode{On: true}}
	if !Build(d, fp, n, t0).Stoker {
		t.Fatal("stoker on is lost")
	}
}

func TestTheContentStateStaysUnder2KB(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	var as []limits.Account
	for i := range 5 {
		a := acct(fmt.Sprintf("acct-%d", i), 99, 99, t0, &limits.Outlook{Out: t0.Add(time.Hour)})
		a.Windows = append(a.Windows, limits.Window{UsedPct: 88, Minutes: 30 * 24 * 60, ResetsAt: t0.Add(20 * 24 * time.Hour)})
		as = append(as, a)
	}
	s := sess("a", false, t0)
	s.Out = t0.Add(time.Hour)
	s.Progress = &sample.Progress{Pct: 95}
	d := doc([]sample.Session{s}, []queue.Item{{SessionID: "a", State: queue.StatePermission, Since: t0}}, as...)
	got := Build(d, fp, n, t0)
	if len(got.Accounts) != 4 {
		t.Fatalf("accounts = %d, want 4", len(got.Accounts))
	}
	for i := 1; i < len(got.Accounts); i++ {
		if got.Accounts[i-1].N >= got.Accounts[i].N {
			t.Fatal("accounts not by number")
		}
	}
	b, _ := json.Marshal(got)
	if len(b) >= 2048 {
		t.Fatalf("%d bytes", len(b))
	}
}

func TestBuildMatchesTheFixture(t *testing.T) {
	want, err := os.ReadFile("../../testdata/push/content-state.json")
	if err != nil {
		t.Fatal(err)
	}
	n := openNumbers(t, t.TempDir())
	for _, id := range []string{"s1", "s2", "s3"} {
		n.Session(id)
	}
	n.Account("anthropic", "a")
	n.Account("anthropic", "b")
	now := time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)
	s3 := sess("s3", false, now)
	s3.Progress = &sample.Progress{Pct: 47}
	s3.Out = time.Date(2026, 10, 21, 16, 40, 0, 0, time.UTC)
	wk := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	h5 := time.Date(2026, 10, 21, 17, 0, 0, 0, time.UTC)
	mk := func(name string, f, w float64, o *limits.Outlook) limits.Account {
		return limits.Account{Provider: "anthropic", Account: name, State: limits.Measured, Forecast: o,
			Windows: []limits.Window{{UsedPct: f, Minutes: limits.BoxMinutes, ResetsAt: h5}, {UsedPct: w, Minutes: limits.WeekMinutes, ResetsAt: wk}}}
	}
	s1, s2 := sess("s1", true, now), sess("s2", true, now)
	s1.Provider, s1.Account = "anthropic", "a"
	s2.Provider, s2.Account = "anthropic", "b"
	d := doc([]sample.Session{s1, s2, s3},
		[]queue.Item{{SessionID: "s3", State: queue.StatePermission, Since: now}},
		mk("a", 62, 31, &limits.Outlook{Lasts: true}),
		mk("b", 93, 71, &limits.Outlook{Out: time.Date(2026, 10, 21, 16, 15, 0, 0, time.UTC)}))
	d.Stoker = &stoker.View{Mode: stoker.Mode{On: true}}
	got, _ := json.Marshal(Build(d, "SHA256:"+"8Q94EmFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE", n, now))
	// The fixture is also the state of a host from before the needs list, which the
	// phone's tests decode as such; this host adds what needs the person on top of it.
	var cs ContentState
	if err := json.Unmarshal(want, &cs); err != nil {
		t.Fatal(err)
	}
	if cs.Need != 0 || cs.Needs != nil {
		t.Fatal("the fixture names needs: it is no longer the state of an older host")
	}
	cs.Needs, cs.Need = []Need{{N: 3, Kind: "command"}}, 1
	wantNow, _ := json.Marshal(cs)
	if string(wantNow) != string(got) {
		t.Fatalf("\n got %s\nwant %s", got, wantNow)
	}
}

func TestWaitAtIsTheNewestWait(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("a", true, t0), sess("b", true, t0)},
		[]queue.Item{
			{SessionID: "a", State: queue.StatePermission, Since: t0.Add(-time.Hour)},
			{SessionID: "b", State: queue.StateQuestion, Since: t0.Add(-time.Minute)},
			{SessionID: "a", State: queue.StateFinished, Since: t0},
		})
	got := Build(d, fp, n, t0)
	if !got.WaitAt.Equal(t0.Add(-time.Minute)) {
		t.Fatalf("WaitAt = %v", got.WaitAt)
	}
	if got := Build(doc([]sample.Session{sess("a", true, t0)}, nil), fp, n, t0); !got.WaitAt.IsZero() {
		t.Fatalf("no wait, WaitAt = %v", got.WaitAt)
	}
}

// TestThePhoneReadsTheAgentsReport (spec "A live report beats an old
// guess"): the phone counts what the board shows. A hooked ask under a
// report of work is no wait, and peek reads no ask; a report of a wait
// after a finished hook is one.
func TestThePhoneReadsTheAgentsReport(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	for _, status := range []string{"shell", "busy"} {
		q := queue.New()
		q.Apply(hooks.Event{Name: "Stop", SessionID: "flint", Pane: "%2", Asks: true, At: t0.Add(-time.Minute)})
		s := sample.Session{SessionID: "flint", Pane: "%2", Name: "flint-auth", Status: status, Busy: status == "busy",
			Reported: true, Asks: true, Said: "Done.\n\nShall I open the PR?", Started: t0.Add(-time.Hour)}
		q.Merge([]sample.Session{s}, t0)
		if got := Build(doc([]sample.Session{s}, q.Items()), fp, n, t0); got.Wait != 0 {
			t.Errorf("report %s: the phone counts %d waits", status, got.Wait)
		}
		if p := stoker.PeekOf(s, q.Items()); p.Asks != "" {
			t.Errorf("report %s: peek asks %q", status, p.Asks)
		}
	}

	q := queue.New()
	q.Apply(hooks.Event{Name: "Stop", SessionID: "flint", Pane: "%2", At: t0.Add(-time.Hour)})
	s := sample.Session{SessionID: "flint", Pane: "%2", Name: "flint-auth", Status: "waiting", Reported: true,
		WaitingFor: "input needed", Idle: time.Minute, Started: t0.Add(-2 * time.Hour)}
	q.Merge([]sample.Session{s}, t0)
	if got := Build(doc([]sample.Session{s}, q.Items()), fp, n, t0); got.Wait != 1 {
		t.Errorf("a reported wait after a finished hook: the phone counts %d waits", got.Wait)
	}
}

// TestThePhoneCountsOpencodesNewStatuses (owner, 2026-11-13: "Allow
// them"): OpenCode's plugin can say waiting and retry. A wait counts as a
// wait, and a retry as a burn: OpenCode retries by itself.
func TestThePhoneCountsOpencodesNewStatuses(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	retry := sample.Session{SessionID: "ses_wick", Pane: "%7", Name: "wick-mobile", Agent: "opencode", Status: "retry", Busy: true,
		Reported: true, Retry: 2, Started: t0.Add(-time.Hour)}
	wait := sample.Session{SessionID: "ses_ember", Pane: "%8", Name: "ember-billing", Agent: "opencode", Status: "waiting",
		Reported: true, WaitingFor: "permission", Idle: time.Minute, Started: t0.Add(-time.Hour)}
	q := queue.New()
	q.Merge([]sample.Session{retry, wait}, t0)
	got := Build(doc([]sample.Session{retry, wait}, q.Items()), fp, n, t0)
	if got.Burn != 1 || got.Wait != 1 {
		t.Errorf("burn %d wait %d, want the retry burning and the wait waiting", got.Burn, got.Wait)
	}
	if w := stoker.Word(retry); w != "busy" {
		t.Errorf("the stoker reads a retry as %q", w)
	}
	if w := stoker.Word(wait); w != "asks" {
		t.Errorf("the stoker reads a wait as %q", w)
	}
}
