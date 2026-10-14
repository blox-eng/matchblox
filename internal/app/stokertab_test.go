package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// tabNight is the demo machine in the middle of a night with an order: three
// steps done, one under way, one waiting for the builder's go, a standing
// rule that fired, and a compact.
func tabNight(st proto.State) proto.State {
	at := demoAt.Add(-3 * time.Hour)
	o := &proto.StokerOrder{
		ID: "r1", Words: "merge them on green, and a new PR gets a compact and /review", Night: true, At: at, State: "running",
		Grants: []string{"merge"},
		Plan: []proto.StokerPlan{
			{N: 1, Session: "spark-docs", Text: "hold: not priority"},
			{N: 2, Session: "hearth-api", Text: "merge #4551 when green"},
			{N: 3, Session: "ember-billing", Text: "merge #4552 when green, then the next"},
			{N: 4, Session: "flint-auth", Text: "rebase on main, then merge #4545 on green"},
			{N: 5, Session: "kindle-web", Text: "release the build"},
		},
	}
	logged := func(id string, after time.Duration, step int, state, session, text string) proto.StokerEntry {
		return proto.StokerEntry{ID: id, At: at.Add(after), Session: session, Name: session, Rule: "order", Step: step, State: state, Text: text}
	}
	merge := logged("3", 80*time.Minute, 2, "done", "hearth-api", "merged #4551")
	merge.Argv = [][]string{{"tmux", "send-keys", "-t", "%2", "merge #4551 when it is green", "Enter"}}
	waits := logged("6", 100*time.Minute, 5, "waits", "kindle-web", "the build is green, waits for the release")
	waits.Pane = "%1"
	// The release the guard denied: matchblox put it on the card itself.
	asked := proto.StokerEntry{ID: "8", At: at.Add(101 * time.Minute), Session: "kindle-web", Name: "kindle-web", Pane: "%1", Rule: "go",
		State: "waits", Needs: "release", Want: "gh release create v1.4", Argv: [][]string{{"gh release create v1.4"}},
		Text: "kindle-web wants to run gh release create v1.4"}
	rule := logged("5", 90*time.Minute, 0, "done", "spark-docs", "opened #4570: compacted, sent /review")
	rule.Rule = "compact"
	v := proto.StokerView{Unseen: true}
	v.On, v.Since, v.Until, v.Nightly, v.Order = true, at, demoAt.Add(4*time.Hour), true, o
	v.Run = []proto.StokerEntry{
		logged("1", time.Minute, 1, "done", "spark-docs", "held"),
		merge,
		logged("2", 70*time.Minute, 3, "done", "ember-billing", "merged #4552, CI green"),
		rule,
		logged("4", 95*time.Minute, 4, "doing", "flint-auth", "told 02:05, CI 6 of 9 jobs"),
		waits,
		asked,
		{ID: "7", At: at.Add(2 * time.Hour), Session: "a", Name: "hearth-api", Pane: "%2", Rule: "compact", Why: "past 80%",
			Result: "sent", Before: 176000, After: 9400, Resume: "run the migration test", Back: at.Add(2*time.Hour + 2*time.Minute)},
	}
	st.Stoker = &v
	return st
}

// tabMorning is the same night, ended: the last step failed.
func tabMorning(st proto.State) proto.State {
	st = tabNight(st)
	v := *st.Stoker
	o := *v.Order
	o.State = "done"
	v.Order = &o
	v.On, v.Until, v.Ended = false, time.Time{}, demoAt
	v.Run = append([]proto.StokerEntry(nil), v.Run...)
	for i, e := range v.Run {
		switch e.Rule {
		case "go":
			v.Run[i].Pane = "%gone" // the session that asked ended too
			continue
		}
		switch e.Step {
		case 4:
			v.Run[i].State, v.Run[i].Text = "done", "merged #4545"
		case 5:
			v.Run[i].State, v.Run[i].Text, v.Run[i].Needs, v.Run[i].Pane = "failed", "the release asks which tag to cut", "", "%gone"
		}
	}
	v.Card = &proto.StokerCard{Night: true, Since: v.Since, Ended: demoAt, Watched: 5, Compacted: 1, Ready: 4, Of: 5}
	st.Stoker = &v
	return st
}

func selectStep(t *testing.T, m Model, id string) Model {
	t.Helper()
	for i, r := range m.stokerRows() {
		if r.id == id {
			m.stokerSel = i
			return m
		}
	}
	t.Fatalf("no row %q in %+v", id, m.stokerRows())
	return m
}

func TestCOpensTheTab(t *testing.T) {
	m, _ := loadedWith(t, 100, tabNight(screensState(t)))
	next, _ := key(m, "c")
	if got := next.(Model).tab; got != tabStoker {
		t.Fatalf("c opened tab %d", got)
	}
	if next, _ := key(m, "7"); next.(Model).tab != tabStoker {
		t.Fatal("7 does not open the stoker tab")
	}
}

func TestStokerTabShowsWordsPlanAndRules(t *testing.T) {
	m, _ := loadedWith(t, 100, tabNight(screensState(t)))
	m.setTab(tabStoker)
	var end tea.Model = m
	for range 12 {
		end, _ = key(end, "down")
	}
	out := screen(m) + screen(end)
	for _, want := range []string{
		"NIGHT · 19:50 → 02:50", "sealed", "“merge them on green", "✓ merges", "! releases and deploys still ask you", "PLAN 3 of 5",
		"✓", "◐", "⏸", "spark-docs", "hold: not priority", "told 02:05, CI 6 of 9 jobs",
		"YOUR GO", "waits for you: kindle-web wants to run gh release create v1.4", "y gives the go, once",
		"STANDING RULES", "opened #4570", "TODAY", "RESUME: run the migration test",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
	if !strings.Contains(strings.Split(out, "\n")[1], "STOKER 1") { // the go, not the plan step that waits on an event
		t.Fatalf("the tab does not count what waits for the builder:\n%s", out)
	}
	for _, w := range []int{44, 132} {
		m, _ := loadedWith(t, w, tabNight(screensState(t)))
		m.setTab(tabStoker)
		fitsWidth(t, m, w)
	}
}

// A restart reuses step numbers: an entry from before the order is not a
// step of its plan.
func TestStokerTabShowsOnlyTheCurrentOrdersSteps(t *testing.T) {
	st := tabNight(screensState(t))
	old := proto.StokerEntry{ID: "0", At: st.Stoker.Order.At.Add(-time.Hour), Session: "x", Rule: "order", Step: 5, State: "done", Text: "from the old order"}
	st.Stoker.Run = append([]proto.StokerEntry{old}, st.Stoker.Run...)
	m, _ := loadedWith(t, 100, st)
	m.setTab(tabStoker)
	if out := screen(m); strings.Contains(out, "from the old order") {
		t.Fatalf("an old order's step shows:\n%s", out)
	}
}

// By day the old run goes on until the new order's go: what it logs
// meanwhile is not a step of the new plan.
func TestStokerTabCountsStepsFromTheGo(t *testing.T) {
	st := tabNight(screensState(t))
	o := st.Stoker.Order
	o.Went = o.At.Add(30 * time.Second)
	o.Plan = append(o.Plan, proto.StokerPlan{N: 6, Session: "x", Text: "the last step"})
	old := proto.StokerEntry{ID: "0", At: o.At.Add(10 * time.Second), Session: "x", Rule: "order", Step: 6, State: "done", Text: "from the old run"}
	st.Stoker.Run = append([]proto.StokerEntry{old}, st.Stoker.Run...)
	m, _ := loadedWith(t, 100, st)
	m.setTab(tabStoker)
	if out := screen(m); strings.Contains(out, "from the old run") {
		t.Fatalf("a step from before the go shows:\n%s", out)
	}
}

func TestMorningSaysWhatWaitsForYou(t *testing.T) {
	m, _ := loadedWith(t, 100, tabMorning(screensState(t)))
	m.setTab(tabStoker)
	var next tea.Model = m
	for range 8 {
		next, _ = key(next, "down")
	}
	out := screen(m) + screen(next)
	for _, want := range []string{"NIGHT · 19:50 → 22:50", "ended 22:50", "DONE 4 of 5 · 1 WAITS FOR YOU", "✕", "the release asks which tag to cut", "THE RUN · ready 4/5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

func TestYGrantsAWaitingStep(t *testing.T) {
	m, f := loadedWith(t, 100, tabNight(screensState(t)))
	m.setTab(tabStoker)
	if line := actionLine(selectStep(t, m, "g8")); !strings.Contains(line, "y give the go") {
		t.Fatalf("the key line does not offer the go: %q", line)
	}
	sendKey(selectStep(t, m, "g8"), "y")
	acts := grants(f)
	if len(acts) != 1 || acts[0].RecID != "stoker:grant" || acts[0].Text != "8" {
		t.Fatalf("acts %+v", acts)
	}
	if line := actionLine(selectStep(t, m, "p5")); strings.Contains(line, "y give the go") {
		t.Fatalf("a step that waits on an event offers a go: %q", line)
	}
	next, _ := key(selectStep(t, m, "p2"), "y") // a step that waits for nothing
	sendKey(selectStep(t, m, "p2"), "y")
	if len(grants(f)) != 1 || !strings.Contains(next.(Model).flash, "y gives the go") {
		t.Fatalf("y on a done step: %+v, %q", grants(f), next.(Model).flash)
	}
}

// grants are the go acts a fake connection got.
func grants(f *fakeConn) []proto.Act {
	var out []proto.Act
	for _, a := range f.acts() {
		if a.RecID == "stoker:grant" {
			out = append(out, a)
		}
	}
	return out
}

func TestEnterShowsTheCommand(t *testing.T) {
	m, f := loadedWith(t, 100, tabNight(screensState(t)))
	m.setTab(tabStoker)
	shown, _ := key(selectStep(t, m, "p2"), "enter")
	if out := screen(shown); !strings.Contains(out, "$ tmux send-keys -t %2 merge #4551 when it is green Enter") {
		t.Fatalf("no command:\n%s", out)
	}
	hidden, _ := key(shown, "enter")
	if out := screen(hidden); strings.Contains(out, "$ tmux send-keys") {
		t.Fatalf("a second Enter keeps the command:\n%s", out)
	}
	if len(f.acts()) != 0 {
		t.Fatalf("looking at a command sent %+v", f.acts())
	}
	logged, _ := key(selectStep(t, m, "p1"), "enter")
	if out := screen(logged); !strings.Contains(out, "no command: the stoker agent logged this step itself") {
		t.Fatalf("a logged step says nothing:\n%s", out)
	}
}

func TestAOnFailedStepOpensPeek(t *testing.T) {
	st := tabMorning(screensState(t))
	m, _ := loadedWith(t, 100, st)
	m.setTab(tabStoker)
	pane := ""
	for _, s := range m.all.sessions {
		if s.Name == "kindle-web" {
			pane = s.Pane
		}
	}
	for i, e := range m.stoker.Run {
		if e.Step == 5 {
			m.stoker.Run[i].Pane = pane
		}
	}
	if line := actionLine(selectStep(t, m, "p5")); !strings.Contains(line, "a look") {
		t.Fatalf("the key line does not offer a: %q", line)
	}
	next, _ := key(selectStep(t, m, "p5"), "a")
	if p := next.(Model).peek; p == nil || p.pane != pane {
		t.Fatalf("a did not open the peek on %q: %+v", pane, p)
	}
	other, _ := key(selectStep(t, m, "p2"), "a")
	if other.(Model).peek != nil || !strings.Contains(other.(Model).flash, "failed step") {
		t.Fatalf("a on a done step: %+v", other.(Model).flash)
	}
}

// While the stoker is in charge, looking at a failed step's session is
// acting on a session: the builder is asked to take over first.
func TestAOnAFailedStepAsksToTakeOver(t *testing.T) {
	st := tabNight(screensState(t))
	m, _ := loadedWith(t, 100, st)
	m.setTab(tabStoker)
	pane := ""
	for _, s := range m.all.sessions {
		if s.Name == "kindle-web" {
			pane = s.Pane
		}
	}
	for i, e := range m.stoker.Run {
		if e.Step == 5 {
			m.stoker.Run[i].State, m.stoker.Run[i].Pane = "failed", pane
		}
	}
	asked, _ := key(selectStep(t, m, "p5"), "a")
	if am := asked.(Model); am.held == nil || am.peek != nil {
		t.Fatalf("a did not ask: held %v peek %v", am.held != nil, am.peek)
	}
	answered, _ := key(asked, "n")
	if p := answered.(Model).peek; p == nil || p.pane != pane {
		t.Fatalf("n did not go on to the peek: %+v", p)
	}
}

func TestOpeningTheTabSendsOneAck(t *testing.T) {
	m, f := loadedWith(t, 100, tabNight(screensState(t)))
	next := sendKey(m, "7")
	sendKey(next, "down")
	var acks int
	for _, a := range f.acts() {
		if a.RecID == "stoker:ack" {
			acks++
		}
	}
	if acks != 1 {
		t.Fatalf("%d acks: %+v", acks, f.acts())
	}
}

// A failed step whose session ended offers no look: it says what resolves it.
func TestAFailedStepWithoutASessionSaysWhatToDo(t *testing.T) {
	for _, c := range []struct {
		st   proto.State
		want string
	}{
		{tabMorning(screensState(t)), "its session ended · f starts the stoker with a new order"},
		{func() proto.State {
			st := tabNight(screensState(t))
			st.Stoker.Run[5].State, st.Stoker.Run[5].Pane = "failed", "%gone"
			return st
		}(), "its session ended · n stops the night"},
		{func() proto.State {
			st := tabNight(screensState(t))
			st.Stoker.Until = time.Time{}
			st.Stoker.Run[5].State, st.Stoker.Run[5].Pane = "failed", "%gone"
			return st
		}(), "its session ended · o gives a new order"},
	} {
		m, _ := loadedWith(t, 100, c.st)
		m.setTab(tabStoker)
		if m.stokerSel != m.firstNeed() || m.stokerRows()[m.stokerSel].state() != "failed" {
			t.Fatalf("the failed step is not selected: %d", m.stokerSel)
		}
		out := screen(m)
		if !strings.Contains(out, c.want) || strings.Contains(out, "a shows") || strings.Contains(actionLine(m), "a look") {
			t.Fatalf("want %q:\n%s", c.want, out)
		}
	}
}

func TestTheTabOpensOnTheStepThatWaits(t *testing.T) {
	m, _ := loadedWith(t, 100, tabNight(screensState(t)))
	m.setTab(tabStoker)
	if r, _ := m.stokerSelected(); r.id != "g8" || !strings.Contains(actionLine(m), "y give the go") {
		t.Fatalf("selected %q: %s", r.id, actionLine(m))
	}
}

// TestTheCardShowsTheTriggers: E, the triggers that wait are under
// WAITING ON, and the ones that fired are steps of their own.
func TestTheCardShowsTheTriggers(t *testing.T) {
	st := tabNight(screensState(t))
	st.Stoker.Triggers = []proto.StokerTrigger{{ID: "t2", Ship: 4570, Stage: "merged", Do: "send", Who: "flint-auth", Text: "rebase on main"}}
	st.Stoker.Run = append(st.Stoker.Run, proto.StokerEntry{ID: "w1", At: st.Stoker.Since.Add(2 * time.Hour), Name: "ember-billing", Pane: "%3",
		Rule: "when", State: "done", Text: "when #4552 merged → sent ember-billing: rebase on main", Argv: [][]string{{"tmux", "send-keys"}}})
	m, _ := loadedWith(t, 132, st)
	m.setTab(tabStoker)
	var end tea.Model = m
	for range 14 {
		end, _ = key(end, "down")
	}
	out := screen(m) + screen(end)
	for _, want := range []string{"WAITING ON", "○ t2 when #4570 merged → send flint-auth: rebase on main", "TRIGGERS", "when #4552 merged → sent ember-billing: rebase on main"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
	rules := out[strings.Index(out, "STANDING RULES"):]
	if i := strings.Index(rules, "TODAY"); i > 0 {
		rules = rules[:i]
	}
	if strings.Contains(rules, "when #4552") {
		t.Fatalf("a fired trigger shows as a standing rule:\n%s", out)
	}
}

// agentState is the console with the stoker agent up on its own tmux
// server, resting.
func agentState(v proto.StokerView) proto.State {
	if v.Agent == "" {
		v.Agent = "claude"
	}
	st := stokerState(v)
	st.StokerServer = "matchblox"
	return st
}

// TestEnterOpensTheStokerInAPopup: the STOKER tab's first row is the
// stoker agent; ⏎ shows the popup's command, ⏎ again opens it.
func TestEnterOpensTheStokerInAPopup(t *testing.T) {
	m, _ := loadedWith(t, 100, agentState(proto.StokerView{PaneUp: true, Ready: true}))
	m.setTab(tabStoker)
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	if s := screen(m); !strings.Contains(s, "STOKER AGENT  claude · rests") || !strings.Contains(s, "⏎ open the stoker") || !strings.Contains(s, "Q quit it") {
		t.Fatalf("no agent row:\n%s", s)
	}
	next, _ := key(m, "enter")
	popup := "tmux display-popup -E -w 90% -h 90% tmux -L matchblox attach -t stoker"
	if s := next.(Model).render(); !strings.Contains(s, "tmux display-popup -E -w 90% -h 90%") || ran != nil {
		t.Fatalf("⏎ must show the command and wait; ran %v\n%s", ran, s)
	}
	next, cmd := key(next, "enter")
	next.Update(cmd())
	if strings.Join(ran, " ") != popup {
		t.Fatalf("ran %q", ran)
	}
	// Outside tmux the console says what to run.
	m.opt.OutsideTmux = true
	if out, _ := key(m, "enter"); out.(Model).flash != "outside tmux: run tmux -L matchblox attach -t stoker (prefix d comes back)" {
		t.Fatalf("flash %q", out.(Model).flash)
	}
	// Not running: nothing to open, and f is the way.
	m, _ = loadedWith(t, 100, agentState(proto.StokerView{}))
	m.setTab(tabStoker)
	if s := screen(m); !strings.Contains(s, "not running · f starts it") {
		t.Fatalf("no way to start it:\n%s", s)
	}
	if out, _ := key(m, "enter"); out.(Model).pending != nil || out.(Model).flash != "the stoker is not running: f or n starts it" {
		t.Fatalf("flash %q", out.(Model).flash)
	}
}

// TestATapOnTheHeadersStokerOpensIt: the stoker waits for you (its trust
// prompt, a permission): a tap on the header's word opens it.
func TestATapOnTheHeadersStokerOpensIt(t *testing.T) {
	st := agentState(proto.StokerView{Mode: proto.StokerMode{On: true}, PaneUp: true})
	st.Sessions = append(st.Sessions, sample.Session{Agent: "claude", Pane: "%0@matchblox", Target: "stoker:0.0", Name: "stoker", Permits: true})
	m, _ := loadedWith(t, 100, st)
	sc := m.draw(100)
	if !strings.Contains(sc.lines[0], "stoker waits for you") || sc.stokerAt.press == "" {
		t.Fatalf("header %q, span %+v", sc.lines[0], sc.stokerAt)
	}
	next, _ := tap(m, sc.stokerAt.x0+1, 0)
	if p := next.(Model).pending; p == nil || !slices.Equal(p.steps[0], []string{"tmux", "display-popup", "-E", "-w", "90%", "-h", "90%", "tmux -L matchblox attach -t stoker"}) {
		t.Fatalf("pending %+v", p)
	}
}

// TestQQuitsTheStokerOnAY: Q on the STOKER tab ends the stoker agent and
// its session, and only on a typed y.
func TestQQuitsTheStokerOnAY(t *testing.T) {
	m, f := loadedWith(t, 100, agentState(proto.StokerView{PaneUp: true, Ready: true}))
	m.setTab(tabStoker)
	next, _ := key(m, "Q")
	if s := next.(Model).render(); !strings.Contains(s, "tmux -L matchblox kill-session -t =stoker") {
		t.Fatalf("Q does not show the command:\n%s", s)
	}
	if out, _ := key(next, "enter"); len(f.acts()) != 0 || out.(Model).pending != nil {
		t.Fatal("enter quit the stoker without a y")
	}
	next, _ = key(m, "Q")
	_, cmd := key(next, "y")
	cmd()
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:quit" || a[0].Confirm != "y" {
		t.Fatalf("acts %+v", a)
	}
}
