package app

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/accounts"
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
	if fm == nil || fm.rename || fm.where != "/src/api" || fm.agent != "claude" || fm.at != fieldWhere {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "New session") || !strings.Contains(s, "blank: no new worktree") || !strings.Contains(s, "● claude") {
		t.Fatalf("the form:\n%s", s)
	}
	// branch, then the agent (→ picks codex), then the first message.
	next, _ = key(next, "tab")
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
	if fm := next.(Model).form; fm.at != fieldWhere || fm.where != "/src/api " || fm.whereAt != -1 {
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

func accountState() proto.State {
	st := lifeState()
	st.Agents = []string{"claude", "codex", "opencode"}
	st.Accounts = map[string][]string{
		"claude": {"default", "personal", "work"},
		"codex":  {"default"},
	}
	return st
}

// toAccount opens N and walks Tab to the account field.
func toAccount(t *testing.T) (tea.Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, 100, accountState())
	next, _ := key(m, "N")
	for range 4 {
		next, _ = key(next, "tab")
	}
	if next.(Model).form.at != fieldAccount {
		t.Fatalf("the cursor is on field %d, not the account", next.(Model).form.at)
	}
	return next, f
}

func TestAccountFieldCycles(t *testing.T) {
	next, _ := toAccount(t)
	if s := screen(next); !strings.Contains(s, "account") || !strings.Contains(s, "‹ default ›") {
		t.Fatalf("the form:\n%s", s)
	}
	for _, want := range []string{"personal", "work", "+ new account"} {
		next, _ = key(next, "right")
		if s := screen(next); !strings.Contains(s, "‹ "+want+" ›") && want != "+ new account" {
			t.Fatalf("want %s:\n%s", want, s)
		}
	}
	fm := next.(Model).form
	if !fm.newAccount || fm.account != "" {
		t.Fatalf("form %+v", fm)
	}
	next, _ = key(next, "left")
	if fm := next.(Model); fm.form.newAccount || fm.form.account != "work" {
		t.Fatalf("left from the name box: %+v", fm.form)
	}
}

func TestNewAccountShowsSetupLikeDefault(t *testing.T) {
	next, f := toAccount(t)
	for range 3 {
		next, _ = key(next, "right")
	}
	next = typeText(next, "work2")
	s := screen(next)
	for _, want := range []string{"setup", "‹ like default ›", "shares skills, plugins and settings", "with default"} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q:\n%s", want, s)
		}
	}
	// Enter asks for a typed y; the service gets the confirmed act.
	next, cmd := key(next, "enter")
	if cmd != nil || len(f.acts()) != 0 || next.(Model).pending == nil {
		t.Fatalf("a new account sent without its y: %+v", f.acts())
	}
	_, cmd = key(next, "y")
	cmd()
	acts := f.acts()
	if len(acts) != 1 || acts[0].RecID != "new:session" || acts[0].Confirm != "y" {
		t.Fatalf("sent %+v", acts)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(acts[0].Text), &n); err != nil || n != (proto.NewSession{Where: "/src/api", Agent: "claude", Account: "work2", NewAccount: true, Setup: "like-default"}) {
		t.Fatalf("%+v %v", n, err)
	}
}

func TestSetupCyclesAndIsSent(t *testing.T) {
	next, f := toAccount(t)
	for range 3 {
		next, _ = key(next, "right")
	}
	next = typeText(next, "w")
	next, _ = key(next, "tab")
	if next.(Model).form.at != fieldSetup {
		t.Fatalf("tab went to field %d", next.(Model).form.at)
	}
	next, _ = key(next, "right")
	if s := screen(next); !strings.Contains(s, "‹ copy of default ›") || !strings.Contains(s, "copy of default: copies setup files once") {
		t.Fatalf("copy:\n%s", s)
	}
	next, _ = key(next, "right")
	if s := screen(next); !strings.Contains(s, "‹ empty ›") || !strings.Contains(s, "empty: nothing shared, the agent's first run") {
		t.Fatalf("empty:\n%s", s)
	}
	next, _ = key(next, "enter")
	_, cmd := key(next, "y")
	cmd()
	var n proto.NewSession
	if err := json.Unmarshal([]byte(f.acts()[0].Text), &n); err != nil || n.Setup != "empty" {
		t.Fatalf("%+v %v", n, err)
	}
}

func TestBadNameSaysWhyAtOnce(t *testing.T) {
	next, f := toAccount(t)
	for range 3 {
		next, _ = key(next, "right")
	}
	next = typeText(next, "Work")
	if s := screen(next); !strings.Contains(s, "a–z, 0–9 and -, up to 10") {
		t.Fatalf("no reason before Enter:\n%s", s)
	}
	next, _ = key(next, "enter")
	if nm := next.(Model); nm.pending != nil || nm.form == nil || len(f.acts()) != 0 {
		t.Fatal("a bad name went on")
	}
	for _, bad := range []string{"default", "abcdefghijk"} {
		fm := *next.(Model).form
		fm.account = bad
		nm := next.(Model)
		nm.form = &fm
		if s := screen(nm); !strings.Contains(s, "a–z, 0–9 and -, up to 10") {
			t.Fatalf("%q passes:\n%s", bad, s)
		}
	}
}

func TestFirstIsDimmedForANewAccount(t *testing.T) {
	m, f := loadedWith(t, 100, accountState())
	next, _ := key(m, "N")
	for range 5 {
		next, _ = key(next, "tab")
	}
	next = typeText(next, "fix it")
	for range 2 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "shift+tab")
	for next.(Model).form.at != fieldAccount {
		next, _ = key(next, "shift+tab")
	}
	for range 3 {
		next, _ = key(next, "right")
	}
	next = typeText(next, "work2")
	s := screen(next)
	if !strings.Contains(s, "after sign-in") || strings.Contains(s, "fix it") {
		t.Fatalf("first:\n%s", s)
	}
	next, _ = key(next, "enter")
	_, cmd := key(next, "y")
	cmd()
	if got := f.acts()[0].Text; strings.Contains(got, "fix it") {
		t.Fatalf("the first message was sent: %s", got)
	}
}

func TestAgentChangeResetsAccount(t *testing.T) {
	next, _ := toAccount(t)
	next, _ = key(next, "right")
	next, _ = key(next, "shift+tab")
	next, _ = key(next, "right") // codex
	fm := next.(Model).form
	if fm.agent != "codex" || fm.account != "" || fm.newAccount {
		t.Fatalf("form %+v", fm)
	}
	next, _ = key(next, "left") // claude again
	if s := screen(next); !strings.Contains(s, "‹ default ›") {
		t.Fatalf("account kept:\n%s", s)
	}
}

func TestOpencodeShowsNoAccount(t *testing.T) {
	next, _ := toAccount(t)
	next, _ = key(next, "shift+tab")
	next, _ = key(next, "right")
	next, _ = key(next, "right") // opencode
	if fm := next.(Model).form; fm.agent != "opencode" || slices.Contains(fm.fields(), fieldAccount) {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); strings.Contains(s, "account") {
		t.Fatalf("the form:\n%s", s)
	}
}

func TestConfirmNamesTheAccount(t *testing.T) {
	m, f := loadedWith(t, 100, accountState())
	next, _ := key(m, "N")
	fm := *next.(Model).form
	fm.where = "/work/app"
	nm := next.(Model)
	nm.form = &fm
	next = nm
	for range 4 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "right")
	next, _ = key(next, "right") // work
	next, cmd := key(next, "enter")
	cmd()
	if nm := next.(Model); nm.pending != nil || len(f.acts()) != 1 {
		t.Fatalf("an existing account asked: %+v %+v", nm.pending, f.acts())
	}
	if line := actionLine(next); !strings.Contains(line, "start claude in /work/app on work") {
		t.Fatalf("flash %q", line)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(f.acts()[0].Text), &n); err != nil || n.Account != "work" || n.NewAccount || n.Setup != "" || f.acts()[0].Confirm != "y" {
		t.Fatalf("%+v %v", n, err)
	}
	// A new account says what happens and where the sign-in is.
	next, _ = key(next, "esc")
	next, _ = key(next, "N")
	fm = *next.(Model).form
	fm.where = "/work/app"
	nm = next.(Model)
	nm.form = &fm
	next = nm
	for range 4 {
		next, _ = key(next, "tab")
	}
	for range 3 {
		next, _ = key(next, "right")
	}
	next = typeText(next, "work2")
	next, _ = key(next, "enter")
	if line := actionLine(next); !strings.Contains(line, `new claude account "work2", like default: sign in in the session`) {
		t.Fatalf("confirm %q", line)
	}
}

func TestDefaultAccountSendsAtOnce(t *testing.T) {
	next, f := toAccount(t)
	_, cmd := key(next, "enter")
	cmd()
	acts := f.acts()
	var n proto.NewSession
	if len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Account != "default" || n.NewAccount {
		t.Fatalf("sent %+v", acts)
	}
}

// lastAccountState is accountState with the accounts last started.
func lastAccountState(last map[string]string) proto.State {
	st := accountState()
	st.Accounts["codex"] = []string{"default", "team"}
	st.LastAccount = last
	return st
}

func TestNOpensOnTheLastAccount(t *testing.T) {
	m, _ := loadedWith(t, 100, lastAccountState(map[string]string{"claude": "work"}))
	next, _ := key(m, "N")
	for range 4 {
		next, _ = key(next, "tab")
	}
	if fm := next.(Model).form; fm.account != "work" || fm.newAccount {
		t.Fatalf("form %+v", fm)
	}
	if s := screen(next); !strings.Contains(s, "‹ work ›") {
		t.Fatalf("the form:\n%s", s)
	}
}

func TestAgentChangeTakesThatAgentsLastAccount(t *testing.T) {
	m, _ := loadedWith(t, 100, lastAccountState(map[string]string{"claude": "work", "codex": "team"}))
	next, _ := key(m, "N")
	for range 3 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "right") // codex
	if fm := next.(Model).form; fm.agent != "codex" || fm.account != "team" {
		t.Fatalf("form %+v", fm)
	}
	next, _ = key(next, "left") // claude again
	if fm := next.(Model).form; fm.agent != "claude" || fm.account != "work" {
		t.Fatalf("form %+v", fm)
	}
}

func TestGoneLastAccountFallsBackToDefault(t *testing.T) {
	m, _ := loadedWith(t, 100, lastAccountState(map[string]string{"claude": "gone"}))
	next, _ := key(m, "N")
	if fm := next.(Model).form; fm.account != "" || fm.newAccount || fm.accountShown() != "default" {
		t.Fatalf("form %+v", fm)
	}
}

func TestNewAccountIsNeverPreselected(t *testing.T) {
	m, _ := loadedWith(t, 100, lastAccountState(map[string]string{"claude": newAccountChoice, "codex": newAccountChoice}))
	next, _ := key(m, "N")
	if fm := next.(Model).form; fm.newAccount || fm.accountShown() != "default" {
		t.Fatalf("form %+v", fm)
	}
	for range 3 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "right") // codex
	if fm := next.(Model).form; fm.newAccount || fm.accountShown() != "default" {
		t.Fatalf("form %+v", fm)
	}
}

func TestDefaultStartNamesItsAccount(t *testing.T) {
	next, f := toAccount(t)
	_, cmd := key(next, "enter")
	cmd()
	var n proto.NewSession
	if acts := f.acts(); len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Account != "default" {
		t.Fatalf("sent %+v", acts)
	}
	// No account field, no account named.
	next, f = toAccount(t)
	next, _ = key(next, "shift+tab")
	next, _ = key(next, "right")
	next, _ = key(next, "right") // opencode
	_, cmd = key(next, "enter")
	cmd()
	n = proto.NewSession{}
	if acts := f.acts(); len(acts) != 1 || json.Unmarshal([]byte(acts[0].Text), &n) != nil || n.Agent != "opencode" || n.Account != "" {
		t.Fatalf("sent %+v %+v", n, acts)
	}
}

// TestAStartWhoseLinksFailedClosesTheForm: a start that ran though some
// shared entries did not link closes the form and says both.
func TestAStartWhoseLinksFailedClosesTheForm(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := key(m, "N")
	next, cmd := key(next, "enter")
	cmd()
	note := proto.NotLinked + "open /home/u/.claude: permission denied"
	next, _ = next.Update(resultMsg(proto.Result{ActID: next.(Model).form.act, Ran: [][]string{{"tmux"}}, Skipped: []string{note}}))
	if nm := next.(Model); nm.form != nil || !strings.Contains(nm.flash, "started api") || !strings.Contains(nm.flash, note) {
		t.Fatalf("form %+v flash %q", nm.form, nm.flash)
	}
}

// TestStateUpdateDoesNotMoveTheAccountField: a state that comes while the
// form is open, with another last account, leaves the account picked.
func TestStateUpdateDoesNotMoveTheAccountField(t *testing.T) {
	m, _ := loadedWith(t, 100, lastAccountState(map[string]string{"claude": "work"}))
	next, _ := key(m, "N")
	for range 4 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "left") // personal
	if fm := next.(Model).form; fm.account != "personal" {
		t.Fatalf("form %+v", fm)
	}
	next, _ = next.Update(stateMsg(lastAccountState(map[string]string{"claude": "work", "codex": "team"})))
	if fm := next.(Model).form; fm == nil || fm.account != "personal" || fm.newAccount {
		t.Fatalf("after a state: form %+v", fm)
	}
}

// TestNOpensOnTheServicesLastAgent: a console that has started nothing yet,
// as after a restart, opens N on the agent the service names and that
// agent's last account.
func TestNOpensOnTheServicesLastAgent(t *testing.T) {
	st := lastAccountState(map[string]string{"claude": "work", "codex": "team"})
	st.LastAgent = "codex"
	m, _ := loadedWith(t, 100, st)
	next, _ := key(m, "N")
	if fm := next.(Model).form; fm.agent != "codex" || fm.account != "team" {
		t.Fatalf("form %+v", fm)
	}
	// This console's own last start comes first.
	mm := m
	mm.lastAgent = "claude"
	next, _ = key(mm, "N")
	if fm := next.(Model).form; fm.agent != "claude" || fm.account != "work" {
		t.Fatalf("form %+v", fm)
	}
	// An agent the host does not list: the first agent.
	st.LastAgent = "gone"
	m, _ = loadedWith(t, 100, st)
	next, _ = key(m, "N")
	if fm := next.(Model).form; fm.agent != m.agents[0] {
		t.Fatalf("form %+v", fm)
	}
}

// TestSwitchConfirmNamesTheAccount: the confirm of a switch names the
// account the new agent starts on, its last one, else default.
func TestSwitchConfirmNamesTheAccount(t *testing.T) {
	for _, c := range []struct {
		last map[string]string
		want string
	}{
		{map[string]string{"codex": "team"}, "switch app-feature: claude → codex in %1, /src/api, on team, then jump there"},
		{nil, "switch app-feature: claude → codex in %1, /src/api, on default, then jump there"},
		{map[string]string{"codex": "gone"}, "on default, then jump there"},
	} {
		st := lastAccountState(c.last)
		next, _ := switchState(t, st)
		next, _ = key(next, "A")
		next, _ = key(next, "enter")
		if p := next.(Model).pending; p == nil || !strings.Contains(p.say, c.want) {
			t.Fatalf("%v: pending %+v, want %q", c.last, p, c.want)
		}
	}
}

// TestSwitchSendsTheAccountItsConfirmNamed: the act carries the account the
// confirm names, its last one else default.
func TestSwitchSendsTheAccountItsConfirmNamed(t *testing.T) {
	for _, c := range []struct {
		last map[string]string
		want string
	}{
		{map[string]string{"codex": "team"}, "team"},
		{nil, "default"},
		{map[string]string{"codex": "gone"}, "default"},
	} {
		next, _ := switchState(t, lastAccountState(c.last))
		next, _ = key(next, "A")
		next, _ = key(next, "enter")
		p := next.(Model).pending
		var sw proto.SwitchAgent
		if p == nil || json.Unmarshal([]byte(p.text), &sw) != nil || sw.Account != c.want {
			t.Fatalf("%v: pending %+v, want account %q", c.last, p, c.want)
		}
	}
}

// TestSwitchWithANotLinkedNoteJumps: a switch whose account's new shared
// entries did not all link ran: it jumps, and the flash carries the note.
func TestSwitchWithANotLinkedNoteJumps(t *testing.T) {
	next, _ := switchState(t, lifeState())
	var ran [][]string
	m := next.(Model)
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ = key(m, "A")
	next, _ = key(next, "enter")
	next, cmd := key(next, "y")
	cmd()
	act := next.(Model).jumpAfter.act
	note := proto.NotLinked + "permission denied"
	done, cmd := next.Update(resultMsg(proto.Result{ActID: act, Ran: [][]string{{"tmux"}}, Skipped: []string{note}}))
	if f := done.(Model).flash; !strings.Contains(f, "switched app-feature to codex") {
		t.Fatalf("flash %q: the switch ran", f) // before cmd: a lone recv waits forever
	}
	c := cmd().(tea.BatchMsg)
	c[len(c)-1]()
	if len(ran) != 1 || !strings.Contains(done.(Model).flash, "switched app-feature to codex") || !strings.Contains(done.(Model).flash, note) {
		t.Fatalf("ran %v flash %q", ran, done.(Model).flash)
	}
}

// toNewCodexAccount opens N, picks codex and "+ new account" named w2, and
// presses Enter: the confirm waits for its y.
func toNewCodexAccount(t *testing.T) (tea.Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, 100, accountState())
	next, _ := key(m, "N")
	for range 3 {
		next, _ = key(next, "tab")
	}
	next, _ = key(next, "right") // codex
	next, _ = key(next, "tab")
	next, _ = key(next, "right") // + new account
	next = typeText(next, "w2")
	if fm := next.(Model).form; fm.agent != "codex" || !fm.newAccount || fm.account != "w2" {
		t.Fatalf("form %+v", fm)
	}
	next, _ = key(next, "enter")
	if next.(Model).pending == nil {
		t.Fatal("no confirm")
	}
	return next, f
}

// TestNewAccountConfirmLeftKeepsTheLastAgent: a start on a new account sets
// the last agent only when it ran, not when its confirm opens.
func TestNewAccountConfirmLeftKeepsTheLastAgent(t *testing.T) {
	next, _ := toNewCodexAccount(t)
	next, _ = key(next, "esc")
	if nm := next.(Model); nm.pending != nil || nm.lastAgent != "" {
		t.Fatalf("after esc: pending %+v last agent %q", nm.pending, nm.lastAgent)
	}
	if next, _ = key(next, "N"); next.(Model).form.agent != "claude" {
		t.Fatalf("N opens on %q, want the agent before", next.(Model).form.agent)
	}
	// A start that the service refused leaves it too.
	next, _ = toNewCodexAccount(t)
	next, cmd := key(next, "y")
	cmd()
	id := "a" + strconv.Itoa(next.(Model).acts)
	next, _ = next.Update(resultMsg(proto.Result{ActID: id, Err: "the codex account w2 is there already: start on it"}))
	if got := next.(Model).lastAgent; got != "" {
		t.Fatalf("after a refusal: last agent %q", got)
	}
	// A start that ran sets it.
	next, _ = toNewCodexAccount(t)
	next, cmd = key(next, "y")
	cmd()
	id = "a" + strconv.Itoa(next.(Model).acts)
	next, _ = next.Update(resultMsg(proto.Result{ActID: id, Ran: [][]string{{"tmux"}}}))
	if got := next.(Model).lastAgent; got != "codex" {
		t.Fatalf("after a start: last agent %q, want codex", got)
	}
}

// TestANewAccountStartSaysWhereToSignInAndWhatItSkipped: a start on a new
// account that ran names the session to sign in in, and its notes.
func TestANewAccountStartSaysWhereToSignInAndWhatItSkipped(t *testing.T) {
	next, _ := toNewCodexAccount(t)
	name := next.(Model).pending.done
	next, cmd := key(next, "y")
	cmd()
	id := "a" + strconv.Itoa(next.(Model).acts)
	note := proto.NotCopied + "/home/u/.codex/pipe: not a file, folder or link"
	next, _ = next.Update(resultMsg(proto.Result{ActID: id, Ran: [][]string{{"tmux"}}, Skipped: []string{note, proto.CopyGoesOn}}))
	want := name + "; " + note + "; " + proto.CopyGoesOn
	if nm := next.(Model); !strings.HasPrefix(name, "started ") || !strings.Contains(name, ": sign in in ") || nm.flash != want || nm.lastAgent != "codex" {
		t.Fatalf("flash %q, want %q; last agent %q", nm.flash, want, nm.lastAgent)
	}
}

// skippedState is accountState with folders the service did not list.
func skippedState(skips ...accounts.Skip) proto.State {
	st := accountState()
	st.AccountsSkipped = map[string][]accounts.Skip{"claude": skips}
	return st
}

func TestTheFormNamesTheFoldersNotListed(t *testing.T) {
	why := "not a-z, 0-9 and -, up to 10"
	for name, c := range map[string]struct {
		skips []accounts.Skip
		want  string
	}{
		"one":    {[]accounts.Skip{{Path: "/home/u/a/My_Acct", Why: why}}, "not listed: My_Acct (not a-z, 0-9"},
		"shared": {[]accounts.Skip{{Path: "/home/u/a/A", Why: why}, {Path: "/home/u/a/b_", Why: why}}, "not listed: A, b_ (" + why + ")"},
		"mixed": {[]accounts.Skip{{Path: "/home/u/a/A", Why: "gone"}, {Path: "/home/u/a/b", Why: "other"}},
			"not listed: A (gone), b (other)"},
		"many": {[]accounts.Skip{{Path: "/a/A", Why: "x"}, {Path: "/a/B", Why: "x"}, {Path: "/a/C", Why: "x"}, {Path: "/a/D", Why: "x"}, {Path: "/a/E", Why: "x"}},
			"not listed: A, B, C, and 2 more (x)"},
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := loadedWith(t, 100, skippedState(c.skips...))
			next, _ := key(m, "N")
			for range 4 {
				next, _ = key(next, "tab")
			}
			if s := screen(next); !strings.Contains(s, c.want) {
				t.Fatalf("no %q:\n%s", c.want, s)
			}
		})
	}
}

func TestTheFormSaysNothingWhenAllAreListed(t *testing.T) {
	next, _ := toAccount(t)
	if s := screen(next); strings.Contains(s, "not listed") {
		t.Fatalf("the form:\n%s", s)
	}
}

func TestAnUnreadableRootIsShownShortened(t *testing.T) {
	long := "open /home/u/.matchblox/accounts/claude/" + strings.Repeat("x", 120) + ": permission denied"
	m, _ := loadedWith(t, 100, skippedState(accounts.Skip{Path: "/home/u/.matchblox/accounts/claude", Why: long}))
	next, _ := key(m, "N")
	for range 4 {
		next, _ = key(next, "tab")
	}
	s := screen(next)
	if !strings.Contains(s, "accounts not read: open /home/u/.matchblox/accou…") || strings.Contains(s, "permission denied") {
		t.Fatalf("the form:\n%s", s)
	}
}
