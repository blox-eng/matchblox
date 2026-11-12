package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
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
	t.Skip("dropped (Task 12 step 1): the STOKER tab goes (step 6b) and c copies (TestCCopiesTheLastMessage")
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
		"NIGHT · 19:50 → 02:50", "sealed", "“merge them on green", "✓ merges", "! releases, deploys and kills still ask you", "PLAN 3 of 5",
		"✓", "◐", "⏸", "spark-docs", "hold: not priority", "told 02:05, CI 6 of 9 jobs",
		"YOUR GO", "waits for you: kindle-web wants to run gh release create v1.4", "y gives the go, once",
		"STANDING RULES", "opened #4570", "TODAY", "RESUME: run the migration test",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
	machine := m
	machine.setTab(tabMachine)
	if row := strings.Split(screen(machine), "\n")[3]; !strings.Contains(row, "1 needs you") { // the go, not the plan step that waits on an event
		t.Fatalf("Machine's row stoker does not count what waits for the builder: %q", row)
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
	t.Skip("replaced by TestYOnAWaitingStepGivesItsGo (Task 12 step 1)")
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

// While the stoker is in charge, a on a failed step asks to take over first,
// as every jump on such a host does; the answer then goes into its pane.
func TestAOnAFailedStepAtNightAsksToTakeOver(t *testing.T) {
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
	ran := recordRuns(&m)
	next, cmd := key(selectStep(t, m, "p5"), "a")
	drain(cmd)
	if next.(Model).held == nil || len(*ran) != 0 {
		t.Fatalf("a: held %v ran %q, want the take-over question first", next.(Model).held != nil, *ran)
	}
	_, cmd = key(next, "n")
	drain(cmd)
	if len(*ran) != 1 || (*ran)[0][len((*ran)[0])-1] != pane {
		t.Fatalf("after the answer ran %q, want the jump to %s", *ran, pane)
	}
}

func TestOpeningTheTabSendsOneAck(t *testing.T) {
	m, f := loadedWith(t, 100, tabNight(screensState(t)))
	next := toStoker(m)
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

// tabFamily is the day's stoker of sheetScreen with its family: the
// stoker of chain hearth-api at work on its order, and, unless one, the
// stoker of chain kindle-web, ended once its chain joined hearth-api.
func tabFamily(st proto.State, one bool) proto.State {
	st = sheetScreen(st, true)
	at := demoAt.Add(-2 * time.Hour)
	live := proto.StokerView{Agent: "claude", PaneUp: true}
	live.On, live.Since = true, at
	live.Order = &proto.StokerOrder{ID: "r1", Words: "merge them on green", At: at, State: "running", Grants: []string{"merge"},
		Plan: []proto.StokerPlan{
			{N: 1, Session: "hearth-api", Text: "merge #4551 when green"},
			{N: 2, Session: "ember-billing", Text: "rebase on main, then merge #4552"},
		}}
	live.Run = []proto.StokerEntry{{ID: "1", At: at.Add(20 * time.Minute), Session: "hearth-api", Name: "hearth-api", Rule: "order", Step: 1, State: "done", Text: "merged #4551"}}
	st.Stokers = []state.ChainStoker{{ID: "s1", Chain: "c1", Name: "hearth-api", View: live}}
	if one {
		return st
	}
	ended := proto.StokerView{Agent: "claude"}
	ended.Since, ended.Ended = at, demoAt.Add(-30*time.Minute)
	ended.Card = &proto.StokerCard{Since: at, Ended: ended.Ended, Watched: 2, Compacted: 1, Ready: 2, Of: 2}
	ended.Run = []proto.StokerEntry{{ID: "family@1", At: ended.Ended, Rule: "family", State: "done", Text: "its chain joined hearth-api"}}
	st.Stokers = append(st.Stokers, state.ChainStoker{ID: "s2", Chain: "c2", Name: "kindle-web", View: ended})
	return st
}

// selectChain selects the card of the chain stoker id on the STOKER tab.
func selectChain(t *testing.T, m Model, id string) Model {
	t.Helper()
	return selectStep(t, m, "s:"+id)
}

// TestTheTabListsTheFamily: the STOKER tab shows the host stoker's card,
// then a card for each chain stoker under its chain's name. An ended one
// shows its card and why it ended, and takes no keys.
func TestTheTabListsTheFamily(t *testing.T) {
	m, _ := loadedWith(t, 104, tabFamily(screensState(t), false))
	m.setTab(tabStoker)
	m.height = 40 // the stoker's panel heads the screen (ruling 28)
	out := screen(m)
	host, chain, ended := strings.Index(out, "STOKER AGENT"), strings.Index(out, "CHAIN hearth-api"), strings.Index(out, "CHAIN kindle-web")
	if host < 0 || chain < host || ended < chain {
		t.Fatalf("the family is not host, hearth-api, kindle-web:\n%s", out)
	}
	for _, want := range []string{"“merge them on green”", "PLAN 1 of 2", "merged #4551", "its chain joined hearth-api", "ready 2/2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
	if slices.ContainsFunc(m.stokerRows(), func(r stokerRow) bool { return r.id == "s:s2" }) {
		t.Fatal("an ended chain stoker takes Enter and Q")
	}
}

// TestEnterOpensAChainStoker: Enter on a chain stoker's card opens its
// own tmux session in a popup.
func TestEnterOpensAChainStoker(t *testing.T) {
	m, _ := loadedWith(t, 104, tabFamily(screensState(t), false))
	m.setTab(tabStoker)
	m = selectChain(t, m, "s1")
	next, _ := key(m, "enter")
	if p := next.(Model).pending; p == nil || !slices.Equal(p.steps[0], []string{"tmux", "display-popup", "-E", "-w", "90%", "-h", "90%", "tmux -L matchblox attach -t stoker-s1"}) {
		t.Fatalf("pending %+v", p)
	}
	m.opt.OutsideTmux = true
	if out, _ := key(m, "enter"); out.(Model).flash != "outside tmux: run tmux -L matchblox attach -t stoker-s1 (prefix d comes back)" {
		t.Fatalf("flash %q", out.(Model).flash)
	}
}

// TestQOnAChainCardQuitsThatStoker: Q and a typed y on a chain stoker's
// card quit that stoker only; the box names its chain.
func TestQOnAChainCardQuitsThatStoker(t *testing.T) {
	m, f := loadedWith(t, 104, tabFamily(screensState(t), false))
	m.setTab(tabStoker)
	m = selectChain(t, m, "s1")
	next, _ := key(m, "Q")
	if s := next.(Model).render(); !strings.Contains(s, "Quit the stoker of chain hearth-api") || !strings.Contains(s, "tmux -L matchblox kill-session -t =stoker-s1") {
		t.Fatalf("Q does not name the chain:\n%s", s)
	}
	_, cmd := key(next, "y")
	cmd()
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:quit" || a[0].Confirm != "y" || a[0].Text != "s1" {
		t.Fatalf("acts %+v", a)
	}
}

// TestDockCountsChains: the dock counts the chain stokers at work,
// `STOKER · 2 chains`, and the header counts every stoker that runs, `≋ 2`
// (TestHeaderCountsRunningStokers). An ended one is not counted, and with
// none they read as today.
func TestDockCountsChains(t *testing.T) {
	two := tabFamily(screensState(t), false)
	two.Stokers[1].View.PaneUp = true // kindle-web's agent is up again
	for _, c := range []struct {
		st   proto.State
		dock string
	}{
		{two, "· 2 chains"},
		{tabFamily(screensState(t), false), "· 1 chain"},
		{tabFamily(screensState(t), true), "· 1 chain"},
	} {
		m, _ := loadedWith(t, 104, c.st)
		lines := strings.Split(screen(m), "\n")
		if !strings.Contains(lines[0], "≋ ") {
			t.Errorf("header %q has no stoker count", lines[0])
		}
		dock := ansi.Strip(strings.Join(m.stokerLines(40, false), "\n"))
		if !strings.Contains(dock, "STOKER "+c.dock) {
			t.Errorf("dock %q, want %q", dock, c.dock)
		}
	}
	m, _ := loadedWith(t, 104, sheetScreen(screensState(t), true))
	if s := screen(m); strings.Contains(s, "chain") {
		t.Fatalf("no chain stoker, and the screen counts chains:\n%s", s)
	}
}

// I1: Q quits a chain stoker whose run is on and whose agent is gone: the
// card shows Q, and Q with a typed y sends the quit.
func TestQOnAPanelessRunningChainCard(t *testing.T) {
	st := tabFamily(screensState(t), true)
	st.Stokers[0].View.PaneUp = false
	m, f := loadedWith(t, 104, st)
	m.setTab(tabStoker)
	m = selectChain(t, m, "s1")
	if !slices.ContainsFunc(m.stokerKeys(), func(h hint) bool { return h.key == m.keys.main.QuitAgent.Help().Key }) {
		t.Fatalf("no Q on the card: %+v", m.stokerKeys())
	}
	next, _ := key(m, "Q")
	if next.(Model).pending == nil {
		t.Fatalf("Q flashed %q", next.(Model).flash)
	}
	_, cmd := key(next, "y")
	cmd()
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:quit" || a[0].Text != "s1" {
		t.Fatalf("acts %+v", a)
	}
}

// I3: a go a chain stoker's run waits for is a row of its card: y gives
// it, with the stoker's id, and the tab's count holds it.
func TestYGivesAChainStokersGo(t *testing.T) {
	t.Skip("replaced by TestYOnAWaitingStepGivesItsGo (Task 12 step 1)")
	st := tabFamily(screensState(t), true)
	v := &st.Stokers[0].View
	v.Run = append(v.Run, proto.StokerEntry{ID: "go@1/merge:%3", At: v.Since.Add(time.Hour), Session: "s3", Name: "hearth-api", Pane: "%3",
		Rule: "go", State: "waits", Needs: "merge:%3", Want: "gh", Text: "hearth-api wants to run gh"})
	m, f := loadedWith(t, 104, st)
	before := m.stokerNeeds()
	m.setTab(tabStoker)
	if !strings.Contains(screen(m), "waits for you: hearth-api wants to run gh") {
		t.Fatalf("no go on the chain's card:\n%s", screen(m))
	}
	m = selectStep(t, m, "s:s1/ggo@1/merge:%3")
	_, cmd := key(m, "y")
	if cmd == nil {
		t.Fatalf("y sent nothing: %q", m.flash)
	}
	cmd()
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:grant" || a[0].Text != "s1 go@1/merge:%3" {
		t.Fatalf("acts %+v", a)
	}
	st.Stokers[0].View.Run = st.Stokers[0].View.Run[:len(st.Stokers[0].View.Run)-1]
	without, _ := loadedWith(t, 104, st)
	if before != without.stokerNeeds()+1 {
		t.Fatalf("the tab counts %d with the go, %d without", before, without.stokerNeeds())
	}
}

// N1: a chain stoker's plan step that waits for your go is a row of its
// card: y gives it, with the stoker's id.
func TestYGivesAChainStokersPlanStep(t *testing.T) {
	t.Skip("replaced by TestYOnAWaitingStepGivesItsGo (Task 12 step 1)")
	st := tabFamily(screensState(t), true)
	v := &st.Stokers[0].View
	v.Run = append(v.Run, proto.StokerEntry{ID: "2", At: v.Since.Add(time.Hour), Session: "ember-billing", Name: "ember-billing", Pane: "%4",
		Rule: "order", Step: 2, State: "waits", Needs: "merge:%4", Text: "merge #4552"})
	m, f := loadedWith(t, 104, st)
	m.setTab(tabStoker)
	m = selectStep(t, m, "s:s1/p2")
	if line := actionLine(m); !strings.Contains(line, "y give the go") {
		t.Fatalf("the key line does not offer the go: %q", line)
	}
	before := screen(m)
	if shown, _ := key(m, "enter"); screen(shown) == before {
		t.Fatalf("⏎ on the step shows nothing:\n%s", before)
	}
	_, cmd := key(m, "y")
	if cmd == nil {
		t.Fatalf("y sent nothing: %q", m.flash)
	}
	cmd()
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:grant" || a[0].Text != "s1 2" {
		t.Fatalf("acts %+v", a)
	}
}

// M1: with no chain stoker the dock's head line is today's, byte for byte.
func TestTheDockWithNoChainIsTodaysBytes(t *testing.T) {
	m, _ := loadedWith(t, 104, sheetScreen(screensState(t), true))
	st := m.st
	want := spread(st.label.Render(" STOKER"), st.stoker.Render(m.stokerWord())+" ", 40)
	if got := m.stokerLines(40, false)[0]; got != want {
		t.Fatalf("dock head %q, want %q", got, want)
	}
}

// M3: the morning card's chain line fits the card's width.
func TestTheMorningCardsChainLineFits(t *testing.T) {
	m, _ := loadedWith(t, 44, screensState(t))
	since := time.Date(2026, 10, 10, 23, 10, 0, 0, time.Local)
	c := proto.StokerCard{Night: true, Since: since, Ended: since.Add(8 * time.Hour), Watched: 3, Ready: 3, Of: 3,
		Chains: []proto.StokerChainCard{{Name: "api-authentication-service", Stoker: "s1", Card: proto.StokerCard{Night: true, Watched: 12, Compacted: 10, Ready: 11, Of: 12}}}}
	for _, l := range m.scoreLines(c, "", 44) {
		if w := lipgloss.Width(l); w > 44 {
			t.Fatalf("a line of %d columns: %q", w, ansi.Strip(l))
		}
	}
}

// TestHeaderCountsRunningStokers: two chain stokers at work and the host
// stoker off read `≋ 2`; none and the host off, `stoker off`.
func TestHeaderCountsRunningStokers(t *testing.T) {
	two := tabFamily(screensState(t), false)
	two.Stokers[1].View.PaneUp = true
	two.Stoker = &proto.StokerView{}
	m, _ := loadedWith(t, 104, two)
	if head := strings.Split(screen(m), "\n")[0]; !strings.HasSuffix(strings.TrimRight(head, " "), "≋ 2") {
		t.Fatalf("header %q, want ≋ 2", head)
	}
	none := sheetScreen(screensState(t), true)
	none.Stoker = &proto.StokerView{}
	m, _ = loadedWith(t, 104, none)
	if head := strings.Split(screen(m), "\n")[0]; !strings.HasSuffix(strings.TrimRight(head, " "), "stoker off") {
		t.Fatalf("header %q, want stoker off", head)
	}
}

// fromMachine is m on Machine, with the row that opens the screen name
// chosen and ⏎ pressed on it.
func fromMachine(t *testing.T, m Model, name string) Model {
	t.Helper()
	m.setTab(tabMachine)
	at := slices.IndexFunc(machineViews, func(v machineView) bool { return v.name == name })
	if at < 0 {
		t.Fatalf("Machine has no row %q", name)
	}
	m.selectRow(at)
	return pressOn(m, "enter")
}

// TestMachineOpensTheStoker: the host stoker's screen is Machine's row
// stoker, its first (ruling 28): ⏎ opens today's STOKER screen, the
// stoker's panel at its top, and esc comes back to Machine on that row.
// The tab row has no stoker of its own.
func TestMachineOpensTheStoker(t *testing.T) {
	m, _ := loadedWith(t, 100, stokerState(stokedRun(fixtureState().At)))
	m = pressOn(m, "tab")
	if m.tab != tabMachine {
		t.Fatalf("tab from the board opens tab %d", m.tab)
	}
	if line, _ := m.tabsLine(100); strings.Contains(strings.ToLower(ansi.Strip(line)), "stoker") {
		t.Fatalf("the tab row names the stoker: %q", ansi.Strip(line))
	}
	if v, ok := m.selectedMachineView(); !ok || v.name != "stoker" {
		t.Fatalf("Machine opens on %+v, want its row stoker", v)
	}
	if row := strings.Split(screen(m), "\n")[3]; !strings.HasPrefix(strings.TrimLeft(row, " ▌"), "stoker") {
		t.Fatalf("Machine's first row is %q", row)
	}
	m = pressOn(m, "enter")
	lines := strings.Split(screen(m), "\n")
	if m.tab != tabStoker || !strings.Contains(lines[3], "STOKER") {
		t.Fatalf("⏎ on the row: tab %d, the screen's top %q", m.tab, lines[3])
	}
	for _, want := range []string{"TODAY", "RESUME: run the migration test", "esc machine"} {
		if !strings.Contains(screen(m), want) {
			t.Fatalf("the STOKER screen lacks %q:\n%s", want, screen(m))
		}
	}
	if !strings.Contains(strings.Split(screen(m), "\n")[1], "MACHINE") {
		t.Fatalf("the tab row does not name Machine open: %q", strings.Split(screen(m), "\n")[1])
	}
	m = pressOn(m, "esc")
	if v, ok := m.selectedMachineView(); m.tab != tabMachine || !ok || v.name != "stoker" {
		t.Fatalf("esc: tab %d on %+v", m.tab, v)
	}
}
