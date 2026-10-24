package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

func lifeState() proto.State {
	st := queueState()
	st.Agents = []string{"claude", "codex"}
	st.Sessions[0].Cwd = "/src/api"
	return st
}

func TestNOpensNewSessionWhereTheSessionWorks(t *testing.T) {
	m, f := loadedWith(t, 100, lifeState())
	next, _ := key(m, "N")
	fm := next.(Model).form
	if fm == nil || fm.rename || fm.where != "" || !fm.listOpen || next.(Model).whereSelected().path() != "/src/api" || fm.agent != "claude" || fm.at != fieldWhere {
		t.Fatalf("form %+v", fm)
	}
	// tab picks the session's directory and goes to branch.
	next, _ = key(next, "tab")
	if fm := next.(Model).form; fm.where != "/src/api" || fm.at != fieldBranch {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "New session") || !strings.Contains(s, "blank: no new worktree") || !strings.Contains(s, "● claude") {
		t.Fatalf("the form:\n%s", s)
	}
	// branch, then the agent (→ picks codex), then the first message.
	next = typeText(next, "feat/88")
	if fm := next.(Model).form; fm.autoName() != "api-feat-88" || !strings.Contains(screen(next), "api-feat-88") {
		t.Fatalf("name %q:\n%s", fm.autoName(), screen(next))
	}
	next, _ = key(next, "tab")
	next, _ = key(next, "tab")
	next, _ = key(next, "right")
	next, _ = key(next, "tab")
	next = typeText(next, "fix it")
	next, cmd := key(next, "enter")
	cmd()
	acts := f.acts()
	if len(acts) != 1 || acts[0].RecID != "new:session" || acts[0].Confirm != "y" {
		t.Fatalf("sent %+v", acts)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(acts[0].Text), &n); err != nil || n != (proto.NewSession{Where: "/src/api", Branch: "feat/88", Agent: "codex", First: "fix it"}) {
		t.Fatalf("%+v %v", n, err)
	}
	// A refusal stays in the form; a start closes it and says so.
	id := next.(Model).form.act
	next, _ = next.Update(resultMsg(proto.Result{ActID: id, Err: "the branch feat/88 is there already: start the session in its worktree"}))
	if fm := next.(Model).form; fm == nil || !strings.Contains(screen(next), "the branch feat/88 is there already") {
		t.Fatalf("refusal: %+v", fm)
	}
	next, cmd = key(next, "enter")
	cmd()
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Ran: [][]string{{"tmux"}}}))
	if nm := next.(Model); nm.form != nil || !strings.Contains(nm.flash, "started api-feat-88") || nm.lastAgent != "codex" {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
	// The next N starts from the agent this console picked last.
	if next, _ = key(next, "N"); next.(Model).form.agent != "codex" {
		t.Fatal("the last agent is not remembered")
	}
}

func TestNewSessionEscSendsNothing(t *testing.T) {
	m, f := loadedWith(t, 44, lifeState())
	next, _ := key(m, "N")
	next = typeText(next, "x")
	next, _ = key(next, "esc") // closes where's list
	next, _ = key(next, "esc")
	if next.(Model).form != nil || len(f.acts()) != 0 {
		t.Fatal("esc sent or kept the form")
	}
}

func TestNewSessionWithNoAgentSaysWhy(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "N")
	if nm := next.(Model); nm.form != nil || nm.flash != "no agent to start here: install claude, codex, opencode or cursor-agent" {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
}

func TestRRenamesTheSession(t *testing.T) {
	m, f := loadedWith(t, 100, lifeState())
	next, _ := key(m, "r")
	fm := next.(Model).form
	if fm == nil || !fm.rename || fm.pane != "%1" {
		t.Fatalf("form %+v", fm)
	}
	for range len(fm.name) {
		next, _ = key(next, "backspace")
	}
	next = typeText(next, "a.b")
	next, _ = key(next, "enter")
	if fm := next.(Model).form; fm == nil || !strings.Contains(fm.err, "a name is letters") || len(f.acts()) != 0 {
		t.Fatalf("a bad name was sent: %+v", fm)
	}
	for range 3 {
		next, _ = key(next, "backspace")
	}
	next = typeText(next, "ships")
	next, cmd := key(next, "enter")
	cmd()
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "rename:%1", Confirm: "y", Text: "ships"}) {
		t.Fatalf("sent %+v", acts)
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Skipped: []string{"alpha renames when it rests"}}))
	if nm := next.(Model); nm.form != nil || nm.flash != "alpha renames when it rests" {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
}

func TestATapPicksAnAgentAndAField(t *testing.T) {
	m, _ := loadedWith(t, 44, lifeState())
	next, _ := key(m, "N")
	next, _ = key(next, "tab") // picks where: the other fields show
	sc := next.(Model).draw(44)
	if len(sc.formAgents) != 2 {
		t.Fatalf("agent spans %+v", sc.formAgents)
	}
	c := sc.formAgents[1]
	tapped, _ := next.Update(tea.MouseClickMsg{X: sc.peekX + c.x0, Y: sc.formAgentsAt, Button: tea.MouseLeft})
	if fm := tapped.(Model).form; fm.agent != "codex" || fm.at != fieldAgent {
		t.Fatalf("form %+v", fm)
	}
	// A tap on the first message's line puts the cursor there.
	for i, p := range sc.peekRows {
		if p == "field:4" {
			tapped, _ = tapped.Update(tea.MouseClickMsg{X: sc.peekX + 4, Y: sc.peekAt + i, Button: tea.MouseLeft})
		}
	}
	if fm := tapped.(Model).form; fm.at != fieldFirst {
		t.Fatalf("the tap left the cursor on %d", fm.at)
	}
}

// switchState is lifeState with nothing waiting, on app-feature (claude in
// %1): one y switches it.
func switchState(t *testing.T, st proto.State) (tea.Model, *fakeConn) {
	t.Helper()
	st.Queue = nil
	m, f := loadedWith(t, 100, st)
	return selectPane(t, m, "%1"), f
}

func TestAOpensSwitchAgentAndJumpsAfter(t *testing.T) {
	next, f := switchState(t, lifeState())
	var ran [][]string
	m := next.(Model)
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ = key(m, "A")
	fm := next.(Model).form
	if fm == nil || !fm.swap || fm.agent != "codex" || fm.first != proto.DefaultHandoff {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "Switch agent") || !strings.Contains(s, "handoff") {
		t.Fatalf("the form:\n%s", s)
	}
	if next, _ = key(next, "left"); next.(Model).form.agent != "codex" {
		t.Fatalf("← left codex for %q", next.(Model).form.agent)
	}
	next, _ = key(next, "enter")
	nm := next.(Model)
	if nm.form != nil || nm.pending == nil || nm.pending.rec != "switch:%1" || !strings.Contains(nm.pending.say, "claude → codex") || len(f.acts()) != 0 {
		t.Fatalf("form %+v pending %+v acts %+v", nm.form, nm.pending, f.acts())
	}
	next, cmd := key(next, "y")
	cmd()
	acts := f.acts()
	if len(acts) != 1 || acts[0].RecID != "switch:%1" || acts[0].Confirm != "y" {
		t.Fatalf("sent %+v", acts)
	}
	var sw proto.SwitchAgent
	if err := json.Unmarshal([]byte(acts[0].Text), &sw); err != nil || sw != (proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}) {
		t.Fatalf("%+v %v", sw, err)
	}
	// A refusal jumps nowhere and says why.
	act := next.(Model).jumpAfter.act
	failed, _ := next.Update(resultMsg(proto.Result{ActID: act, Err: "codex is not installed here"}))
	if fm := failed.(Model); fm.jumpAfter.act != "" || !strings.Contains(fm.flash, "codex is not installed here") {
		t.Fatalf("jump %+v flash %q", fm.jumpAfter, fm.flash)
	}
	// A switch that ran jumps to the pane.
	done, cmd := next.Update(resultMsg(proto.Result{ActID: act, Ran: [][]string{{"tmux"}}}))
	c := cmd().(tea.BatchMsg)
	c[len(c)-1]()
	if len(ran) != 1 || strings.Join(ran[0], " ") != "tmux switch-client -t %1" || done.(Model).flash != "switched app-feature to codex" {
		t.Fatalf("ran %v flash %q", ran, done.(Model).flash)
	}
}

func TestSwitchOfABusySessionTakesASecondY(t *testing.T) {
	st := lifeState()
	st.Sessions[0].Busy = true
	next, f := switchState(t, st)
	next, _ = key(next, "A")
	next, _ = key(next, "enter")
	next, cmd := key(next, "y")
	if cmd != nil {
		cmd()
	}
	if line := actionLine(next); len(f.acts()) != 0 || !strings.Contains(line, "app-feature works: y again switches it mid-turn") {
		t.Fatalf("one y switched a session that works: %q %+v", line, f.acts())
	}
	_, cmd = key(next, "y")
	cmd()
	if acts := f.acts(); len(acts) != 1 || !strings.Contains(acts[0].Text, `"anyway":true`) || !strings.Contains(acts[0].Text, `"agent":"codex"`) {
		t.Fatalf("sent %+v", acts)
	}
}

func TestSwitchRefusesOutsideTmuxAndOneAgent(t *testing.T) {
	st := lifeState()
	for i := range st.Sessions {
		st.Sessions[i].Pane = ""
	}
	m, _ := loadedWith(t, 100, st)
	s, _ := m.selectedSession()
	next, _ := key(m, "A")
	if nm := next.(Model); nm.form != nil || nm.flash != s.Name+" is outside tmux: nothing to switch" {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
	st = lifeState()
	st.Agents = []string{"claude"}
	next, _ = switchState(t, st)
	next, _ = key(next, "A")
	if nm := next.(Model); nm.form != nil || nm.flash != "no other agent to switch to here" {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
}

func TestSwitchRefusesABadHandoffInTheForm(t *testing.T) {
	next, f := switchState(t, lifeState())
	next, _ = key(next, "A")
	next, _ = key(next, "tab")
	for range len(proto.DefaultHandoff) {
		next, _ = key(next, "backspace")
	}
	next = typeText(next, "--yolo")
	next, _ = key(next, "enter")
	if nm := next.(Model); nm.form == nil || nm.form.err != "the handoff does not start with -" || nm.pending != nil || len(f.acts()) != 0 {
		t.Fatalf("form %+v pending %+v", nm.form, nm.pending)
	}
}

func TestSwitchEscSendsNothing(t *testing.T) {
	next, f := switchState(t, lifeState())
	next, _ = key(next, "A")
	next, _ = key(next, "esc")
	if nm := next.(Model); nm.form != nil || nm.pending != nil || len(f.acts()) != 0 {
		t.Fatal("esc sent or kept the form")
	}
}

// A phone has no A key to hand: the opened row, and the detail beside the
// list, carry A switch, and a tap on it opens the form.
func TestATapOnSwitchOpensTheForm(t *testing.T) {
	for _, w := range []int{50, 120} {
		st := lifeState()
		st.Queue = nil
		m, _ := loadedWith(t, w, st)
		next := selectPane(t, m, "%1")
		out := strings.Split(ansi.Strip(next.(Model).render()), "\n")
		y := slices.IndexFunc(out[:len(out)-1], func(l string) bool { return strings.Contains(l, "A switch") })
		if y < 0 {
			t.Fatalf("%d columns: no A switch above the key line:\n%s", w, strings.Join(out, "\n"))
		}
		x := len([]rune(out[y][:strings.Index(out[y], "A switch")]))
		tapped, _ := tap(next, x, y)
		if fm := tapped.(Model).form; fm == nil || !fm.swap {
			t.Fatalf("%d columns: a tap on A switch opened %+v", w, fm)
		}
	}
}

// TestFormSpaceIsTextOnWhereAndPicksOnAgent: Right binds space for the
// agent line, and the where line is text, so space types there.
func TestFormSpaceIsTextOnWhereAndPicksOnAgent(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := key(m, "N")
	next, _ = key(next, "space")
	if fm := next.(Model).form; fm.at != fieldWhere || fm.where != " " || fm.picked {
		t.Fatalf("space on where: %+v", fm)
	}
	for range 3 { // branch, name, agent
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "space")
	if fm := next.(Model).form; fm.at != fieldAgent || fm.agent != "codex" {
		t.Fatalf("space on agent: %+v", fm)
	}
}

// The clone in the form (#176).

const cloneURL = "https://github.com/o/r"

// runAll runs cmd and, for a batch, each of its commands but the first
// (the receive, which waits for the connection). A lone receive is left
// waiting.
func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-got:
	case <-time.After(200 * time.Millisecond):
		return
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b[1:] {
			if c != nil {
				c()
			}
		}
	}
}

// pastedURL is N with cloneURL pasted into where.
func pastedURL(t *testing.T, w int) (tea.Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, w, whereState(t))
	next, _ := key(m, "N")
	next, cmd := next.Update(tea.PasteMsg{Content: cloneURL})
	if cmd != nil {
		cmd()
	}
	return next, f
}

// planned is cloneURL pasted, and the service's plan for it.
func planned(t *testing.T, w int, p proto.ClonePlan) (tea.Model, *fakeConn) {
	t.Helper()
	next, f := pastedURL(t, w)
	p.Repo, p.URL = "o/r", cloneURL
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.planAct, Clone: &p}))
	return next, f
}

// cloneState is whereState with the clones the service lists.
func cloneState(t *testing.T, cs ...proto.Clone) stateMsg {
	st := whereState(t)
	st.Clones = cs
	return stateMsg(st)
}

func rowTexts(rows []whereRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.text)
	}
	return out
}

func TestPastedURLShowsTheCloneRowAtOnce(t *testing.T) {
	next, f := pastedURL(t, 100)
	fm := next.(Model).form
	if len(fm.rows) != 1 || fm.rows[0].kind != rowClone || !strings.Contains(screen(next), "clone o/r") {
		t.Fatalf("rows %+v:\n%s", fm.rows, screen(next))
	}
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "clone:plan" || acts[0].Text != cloneURL {
		t.Fatalf("sent %+v", acts)
	}
	// The same text again asks no second plan.
	next, cmd := next.Update(tea.PasteMsg{Content: ""})
	if cmd != nil {
		cmd()
	}
	if acts := f.acts(); len(acts) != 1 {
		t.Fatalf("sent %+v", acts)
	}
}

func TestPlanOffersTheExistingClone(t *testing.T) {
	next, _ := planned(t, 100, proto.ClonePlan{Have: []string{home + "/src/r"}, Target: home + "/src/r", Fresh: home + "/src/r-ember"})
	fm := next.(Model).form
	want := []string{"start in ~/src/r (already a clone of o/r)", "clone anew into ~/src/r-ember"}
	if got := rowTexts(fm.rows); !slices.Equal(got, want) || fm.sel != 0 || fm.rows[0].kind != rowHave || fm.rows[1].kind != rowClone {
		t.Fatalf("rows %q sel %d", got, fm.sel)
	}
	if s := screen(next); !strings.Contains(s, want[0]) || !strings.Contains(s, want[1]) {
		t.Fatalf("the list:\n%s", s)
	}
	// ⏎ on the clone that is there takes its path, as a repo row.
	next, _ = key(next, "enter")
	if fm := next.(Model).form; fm.where != home+"/src/r" || fm.at != fieldBranch || fm.cloning != "" {
		t.Fatalf("form %+v", fm)
	}
}

func TestPlanNamesTheTakenFolder(t *testing.T) {
	next, _ := planned(t, 100, proto.ClonePlan{Target: home + "/src/r-ember", Taken: "another repo"})
	want := "clone o/r into ~/src/r-ember (~/src/r is another repo)"
	if got := rowTexts(next.(Model).form.rows); !slices.Equal(got, []string{want}) || !strings.Contains(screen(next), want) {
		t.Fatalf("rows %q:\n%s", got, screen(next))
	}
	// The session takes the clone's folder's name.
	if next, _ = key(next, "enter"); next.(Model).form.autoName() != "r-ember" {
		t.Fatalf("name %q", next.(Model).form.autoName())
	}
	next, _ = planned(t, 100, proto.ClonePlan{Target: home + "/src/r-ember", Taken: "being cloned"})
	if got := rowTexts(next.(Model).form.rows); !slices.Equal(got, []string{"clone o/r into ~/src/r-ember (~/src/r is being cloned)"}) {
		t.Fatalf("rows %q", got)
	}
}

// cloning is cloneURL planned into ~/src/r, ⏎ on its row, and the
// service's yes to the clone:start.
func cloning(t *testing.T, w int) (tea.Model, *fakeConn) {
	t.Helper()
	next, f := planned(t, w, proto.ClonePlan{Target: home + "/src/r"})
	next, cmd := key(next, "enter")
	if cmd != nil {
		cmd()
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.startAct, CloneID: 1}))
	return next, f
}

func TestEnterOnCloneStartsItAndMovesOn(t *testing.T) {
	next, f := cloning(t, 100)
	acts := f.acts()
	var cs proto.CloneStart
	if len(acts) != 2 || acts[1].RecID != "clone:start" || json.Unmarshal([]byte(acts[1].Text), &cs) != nil || cs != (proto.CloneStart{URL: cloneURL, Target: home + "/src/r"}) {
		t.Fatalf("sent %+v", acts)
	}
	fm := next.(Model).form
	if fm.at != fieldBranch || fm.listOpen || fm.where != cloneURL || fm.cloning != home+"/src/r" {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "cloning o/r…") || strings.Contains(s, "cloning o/r… 42%") {
		t.Fatalf("before a percent:\n%s", s)
	}
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 42}))
	if s := screen(next); !strings.Contains(s, "cloning o/r… 42%") {
		t.Fatalf("the where line:\n%s", s)
	}
}

func TestEnterWhileCloningStartsWhenDone(t *testing.T) {
	next, f := cloning(t, 100)
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 42}))
	next = typeText(next, "feat/9")
	next, cmd := key(next, "enter")
	if cmd != nil {
		cmd()
	}
	if acts := f.acts(); len(acts) != 2 || !next.(Model).form.startWhenDone || !strings.Contains(screen(next), "starts when the clone is done") {
		t.Fatalf("sent %+v:\n%s", acts, screen(next))
	}
	next, cmd = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 100, Done: true}))
	runAll(cmd)
	acts := f.acts()
	var n proto.NewSession
	if len(acts) != 3 || acts[2].RecID != "new:session" || json.Unmarshal([]byte(acts[2].Text), &n) != nil || n.Where != home+"/src/r" || n.Branch != "feat/9" {
		t.Fatalf("sent %+v", acts)
	}
	// Its result closes the form as any start does.
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act}))
	if nm := next.(Model); nm.form != nil || !strings.Contains(nm.flash, "started r-feat-9") {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
}

func TestEnterAfterTheCloneStartsAtOnce(t *testing.T) {
	next, f := cloning(t, 100)
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 100, Done: true}))
	next, cmd := key(next, "enter")
	if cmd != nil {
		cmd()
	}
	var n proto.NewSession
	if acts := f.acts(); len(acts) != 3 || acts[2].RecID != "new:session" || json.Unmarshal([]byte(acts[2].Text), &n) != nil || n.Where != home+"/src/r" {
		t.Fatalf("sent %+v", acts)
	}
}

func TestEscStopsTheClone(t *testing.T) {
	next, f := cloning(t, 100)
	next, cmd := key(next, "esc")
	if cmd != nil {
		cmd()
	}
	acts := f.acts()
	if len(acts) != 3 || acts[2] != (proto.Act{RecID: "clone:stop", Text: home + "/src/r"}) {
		t.Fatalf("sent %+v", acts)
	}
	if fm := next.(Model).form; fm == nil || fm.where != cloneURL || fm.cloning != "" || fm.startWhenDone {
		t.Fatalf("form %+v", fm)
	}
	// A second esc closes the form.
	if next, _ = key(next, "esc"); next.(Model).form != nil {
		t.Fatal("the second esc kept the form")
	}
}

func TestEditingWhereStopsTheClone(t *testing.T) {
	next, f := cloning(t, 100)
	next, _ = next.Update(keyPress("shift+tab")) // back on where: no list while it clones
	if fm := next.(Model).form; fm.at != fieldWhere || fm.listOpen || !strings.Contains(screen(next), "cloning o/r…") {
		t.Fatalf("form %+v:\n%s", fm, screen(next))
	}
	next, cmd := next.Update(keyPress("backspace"))
	if cmd != nil {
		cmd()
	}
	acts := f.acts()
	if fm := next.(Model).form; len(acts) != 3 || acts[2] != (proto.Act{RecID: "clone:stop", Text: home + "/src/r"}) || fm.cloning != "" || fm.where != cloneURL[:len(cloneURL)-1] {
		t.Fatalf("sent %+v form %+v", acts, fm)
	}
}

func TestCloneFailureShowsInTheForm(t *testing.T) {
	next, f := cloning(t, 100)
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: -1, Err: "no network: github.com does not answer"}))
	if fm := next.(Model).form; !strings.Contains(screen(next), "! no network: github.com does not answer") || fm.cloning != "" {
		t.Fatalf("form %+v:\n%s", fm, screen(next))
	}
	// The final ⏎ starts nothing: it goes back to the clone rows, planned
	// afresh.
	next, cmd := key(next, "enter")
	if cmd != nil {
		cmd()
	}
	if acts := f.acts(); len(acts) != 3 || acts[2].RecID != "clone:plan" || !next.(Model).form.whereList() {
		t.Fatalf("sent %+v", acts)
	}
}

func TestTildePathIsNoClone(t *testing.T) {
	for _, p := range []string{"~/o/r", "./o/r"} {
		m, f := loadedWith(t, 100, whereState(t))
		next, _ := key(m, "N")
		next, cmd := next.Update(tea.PasteMsg{Content: p})
		if cmd != nil {
			cmd()
		}
		if fm := next.(Model).form; slices.ContainsFunc(fm.rows, func(r whereRow) bool { return r.kind == rowClone }) || len(f.acts()) != 0 {
			t.Fatalf("%s: rows %+v sent %+v", p, fm.rows, f.acts())
		}
	}
}

// TestDrawClone prints the form with a clone row and with the clone's
// line, for a look: go test -run TestDrawClone -v.
func TestDrawClone(t *testing.T) {
	for _, w := range []int{100, 44} {
		next, _ := planned(t, w, proto.ClonePlan{Have: []string{home + "/src/r"}, Target: home + "/src/r", Fresh: home + "/src/r-ember"})
		t.Logf("%d columns, the plan:\n%s", w, screen(next))
		next, _ = cloning(t, w)
		next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 42}))
		t.Logf("%d columns, cloning:\n%s", w, screen(next))
		next, _ = key(next, "enter")
		t.Logf("%d columns, waiting:\n%s", w, screen(next))
		for i, l := range strings.Split(next.(Model).render(), "\n") {
			if lw := lipgloss.Width(l); lw > w {
				t.Fatalf("%d columns: line %d is %d wide", w, i, lw)
			}
		}
	}
}

// pressRun is key with its command run; a batch's receive is left out.
func pressRun(m tea.Model, k string) tea.Model {
	next, cmd := key(m, k)
	if cmd == nil {
		return next
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b[1:] {
			if c != nil {
				c()
			}
		}
	}
	return next
}

// planResult is the service's plan for cloneURL, as the result of the
// form's clone:plan in flight.
func planResult(next tea.Model, p proto.ClonePlan) (tea.Model, tea.Cmd) {
	p.Repo, p.URL = "o/r", cloneURL
	return next.Update(resultMsg(proto.Result{ActID: next.(Model).form.planAct, Clone: &p}))
}

// A retry into the same target is not taken for the clone that failed
// there, which the service still lists for a minute.
func TestCloneRetryIgnoresTheOldFailure(t *testing.T) {
	next, f := cloning(t, 100)
	failed := proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: -1, Err: "no network: github.com does not answer"}
	next, _ = next.Update(cloneState(t, failed))
	next = pressRun(next, "enter") // back to the clone rows: a fresh plan
	if acts := f.acts(); len(acts) != 3 || acts[2].RecID != "clone:plan" {
		t.Fatalf("sent %+v", acts)
	}
	next, _ = planResult(next, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter") // the clone again: its start is in flight
	if acts := f.acts(); len(acts) != 4 || acts[3].RecID != "clone:start" {
		t.Fatalf("sent %+v", acts)
	}
	next, _ = next.Update(cloneState(t, failed)) // built before the service took the start
	if fm := next.(Model).form; fm.cloning != home+"/src/r" || strings.Contains(screen(next), "! no network") {
		t.Fatalf("the old failure was taken: %+v", fm)
	}
	start := next.(Model).form.startAct
	next = pressRun(next, "esc") // the stop waits for the start (R14)
	next, cmd := next.Update(resultMsg(proto.Result{ActID: start}))
	runAll(cmd)
	if acts := f.acts(); len(acts) != 5 || acts[4] != (proto.Act{RecID: "clone:stop", Text: home + "/src/r"}) || next.(Model).form.cloning != "" {
		t.Fatalf("sent %+v", acts)
	}
}

func TestRefusedStartAsksAFreshPlan(t *testing.T) {
	next, f := planned(t, 100, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter")
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.startAct, Err: "~/src/r is there already"}))
	if fm := next.(Model).form; fm.cloning != "" || !strings.Contains(screen(next), "! ~/src/r is there already") {
		t.Fatalf("form %+v:\n%s", fm, screen(next))
	}
	pressRun(next, "enter")
	if acts := f.acts(); len(acts) != 3 || acts[2].RecID != "clone:plan" || acts[2].Text != cloneURL {
		t.Fatalf("sent %+v", acts)
	}
}

// A bare o/r still filters: the repos that match first, the clone after.
func TestShorthandStillFilters(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	next, _ := key(m, "N")
	next, cmd := next.Update(tea.PasteMsg{Content: "dev/match"})
	cmd()
	fm := next.(Model).form
	rows := selectable(fm.rows)
	if len(rows) < 2 || rows[0].kind != rowRepo || rows[0].repo.path != "/dev/matchblox" || rows[len(rows)-1].kind != rowClone || next.(Model).whereSelected().path() != "/dev/matchblox" {
		t.Fatalf("rows %+v", rows)
	}
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "clone:plan" {
		t.Fatalf("sent %+v", acts)
	}
	// No repo matches: the clone row is selected.
	next, _ = key(m, "N")
	next, _ = next.Update(tea.PasteMsg{Content: "zz/qq"})
	if r := next.(Model).whereSelected(); r.kind != rowClone || len(next.(Model).form.rows) != 1 {
		t.Fatalf("rows %+v", next.(Model).form.rows)
	}
}

// ⏎ before the plan is kept, and done when the plan lands.
func TestEnterBeforeThePlanIsKept(t *testing.T) {
	next, f := pastedURL(t, 100)
	next = pressRun(next, "enter")
	if acts := f.acts(); len(acts) != 1 {
		t.Fatalf("sent %+v", acts)
	}
	next, cmd := planResult(next, proto.ClonePlan{Target: home + "/src/r"})
	runAll(cmd)
	var cs proto.CloneStart
	if acts := f.acts(); len(acts) != 2 || acts[1].RecID != "clone:start" || json.Unmarshal([]byte(acts[1].Text), &cs) != nil || cs.Target != home+"/src/r" || next.(Model).form.at != fieldBranch {
		t.Fatalf("sent %+v", acts)
	}
	// A plan with the clone there already picks it.
	next, f = pastedURL(t, 100)
	next, _ = next.Update(keyPress("tab"))
	next, cmd = planResult(next, proto.ClonePlan{Have: []string{home + "/src/r"}, Target: home + "/src/r", Fresh: home + "/src/r-ember"})
	runAll(cmd)
	if fm := next.(Model).form; fm.where != home+"/src/r" || fm.at != fieldBranch || len(f.acts()) != 1 {
		t.Fatalf("form %+v sent %+v", fm, f.acts())
	}
}

// A clone the snapshots no longer list, and never saw done, stopped.
func TestCloneGoneFromTheSnapshot(t *testing.T) {
	next, _ := cloning(t, 100)
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 42}))
	next, _ = next.Update(cloneState(t))
	if fm := next.(Model).form; fm.cloning != "" || !strings.Contains(screen(next), "! the clone of o/r stopped before it was done") {
		t.Fatalf("form %+v:\n%s", fm, screen(next))
	}
}

// A snapshot built before the service took the start may come after its
// yes: an empty one does not say the clone stopped.
func TestStaleEmptySnapshotAfterTheStart(t *testing.T) {
	next, _ := cloning(t, 100)
	next, _ = next.Update(cloneState(t))
	if fm := next.(Model).form; fm.cloning != home+"/src/r" || fm.err != "" {
		t.Fatalf("a stale snapshot stopped the clone: %+v", fm)
	}
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: 10}))
	next, _ = next.Update(cloneState(t))
	if fm := next.(Model).form; fm.cloning != "" || !strings.Contains(fm.err, "stopped before it was done") {
		t.Fatalf("form %+v", fm)
	}
}

// retried is a clone into ~/src/r that failed (id 1), and a retry the
// service took as id 2.
func retried(t *testing.T) (tea.Model, proto.Clone) {
	t.Helper()
	next, _ := cloning(t, 100)
	old := proto.Clone{ID: 1, Target: home + "/src/r", Repo: "o/r", Pct: -1, Err: "no network: github.com does not answer"}
	next, _ = next.Update(cloneState(t, old))
	next = pressRun(next, "enter") // a fresh plan
	next, _ = planResult(next, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter")
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.startAct, CloneID: 2}))
	return next, old
}

// A retry follows its own clone: the last try's record, listed for a
// minute, says nothing of it, even in a snapshot after the start's yes.
func TestStaleOldErrAfterTheStart(t *testing.T) {
	next, old := retried(t)
	for _, st := range []stateMsg{cloneState(t, old), cloneState(t), cloneState(t, old)} {
		next, _ = next.Update(st)
	}
	if fm := next.(Model).form; fm.cloning != home+"/src/r" || fm.err != "" || !strings.Contains(screen(next), "cloning o/r…") {
		t.Fatalf("the old failure was taken: %+v", fm)
	}
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 2, Target: home + "/src/r", Repo: "o/r", Pct: 30}))
	if !strings.Contains(screen(next), "cloning o/r… 30%") {
		t.Fatalf("the line:\n%s", screen(next))
	}
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 2, Target: home + "/src/r", Repo: "o/r", Pct: 30, Err: "the disk is full: o/r is not cloned"}))
	if fm := next.(Model).form; fm.cloning != "" || !strings.Contains(fm.err, "the disk is full") {
		t.Fatalf("form %+v", fm)
	}
}

// A retry that fails before it is ever listed running says why at once.
func TestFastFailingRetryShowsItsReason(t *testing.T) {
	next, _ := retried(t)
	next, _ = next.Update(cloneState(t, proto.Clone{ID: 2, Target: home + "/src/r", Repo: "o/r", Pct: -1, Err: "o/r is not on github.com"}))
	if fm := next.(Model).form; fm.cloning != "" || !strings.Contains(screen(next), "! o/r is not on github.com") {
		t.Fatalf("form %+v:\n%s", fm, screen(next))
	}
}

// esc while the start is on its way stops the clone when the start lands.
func TestEscWhileTheStartIsOnItsWay(t *testing.T) {
	next, f := planned(t, 100, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter")
	start := next.(Model).form.startAct
	next = pressRun(next, "esc")
	if fm := next.(Model).form; fm == nil || fm.cloning != "" || len(f.acts()) != 2 {
		t.Fatalf("form %+v sent %+v", fm, f.acts())
	}
	next, cmd := next.Update(resultMsg(proto.Result{ActID: start}))
	runAll(cmd)
	acts := f.acts()
	if len(acts) != 3 || acts[2] != (proto.Act{RecID: "clone:stop", Text: home + "/src/r"}) {
		t.Fatalf("sent %+v", acts)
	}
	// Its refusal, the clone gone already, is no error of the form.
	next, _ = next.Update(resultMsg(proto.Result{ActID: "a3", Err: "~/src/r is not being cloned"}))
	if fm := next.(Model).form; fm.err != "" || strings.Contains(next.(Model).flash, "not being cloned") {
		t.Fatalf("form %+v flash %q", fm, next.(Model).flash)
	}
	// esc twice closes the form: the stop goes all the same.
	next, f = planned(t, 100, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter")
	start = next.(Model).form.startAct
	next = pressRun(pressRun(next, "esc"), "esc")
	next, cmd = next.Update(resultMsg(proto.Result{ActID: start}))
	runAll(cmd)
	if acts := f.acts(); next.(Model).form != nil || len(acts) != 3 || acts[2].RecID != "clone:stop" {
		t.Fatalf("sent %+v", acts)
	}
	// A start refused after esc needs no stop.
	next, f = planned(t, 100, proto.ClonePlan{Target: home + "/src/r"})
	next = pressRun(next, "enter")
	start = next.(Model).form.startAct
	next = pressRun(next, "esc")
	_, cmd = next.Update(resultMsg(proto.Result{ActID: start, Err: "~/src/r is there already"}))
	runAll(cmd)
	if acts := f.acts(); len(acts) != 2 {
		t.Fatalf("sent %+v", acts)
	}
}

// Moving the selection after an early ⏎ drops it: only a new ⏎ picks.
func TestArrowsDropAnEarlyEnter(t *testing.T) {
	for _, k := range []string{"down", "up", "left", "right"} {
		next, f := pastedURL(t, 100)
		next = pressRun(next, "enter")
		next, _ = next.Update(keyPress(k))
		next, cmd := planResult(next, proto.ClonePlan{Target: home + "/src/r"})
		runAll(cmd)
		if acts := f.acts(); len(acts) != 1 || next.(Model).form.cloning != "" {
			t.Fatalf("%s: sent %+v", k, acts)
		}
	}
}
