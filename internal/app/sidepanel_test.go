package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func panelText(m Model, w, h int) string {
	return strings.Join(plainLines(m.sidePanel(w, h)), "\n")
}

func TestNothingNeedsYouTakesNoRow(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	if got := panelText(m, 40, 10); strings.Contains(got, "NEEDS YOU") || !strings.Contains(got, "IDLE · 1") {
		t.Fatalf("panel:\n%s", got)
	}
	head, _, _ := m.header(100)
	if got := plainLines([]string{head})[0]; !strings.Contains(got, "✓ nothing needs you") {
		t.Fatalf("header %q", got)
	}
}

func TestPanelDrawsHeadsAndALineBetweenGroups(t *testing.T) {
	a, b := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour)
	st := boardState(a, b)
	st.Queue = append(st.Queue, waitFor(a, queue.StatePermission, time.Minute))
	got := panelText(boardModel(t, st), 40, 12)
	i, j := strings.Index(got, "NEEDS YOU · 1  −"), strings.Index(got, "IDLE · 1  −")
	if i < 0 || j < i || !strings.Contains(got[i:j], "────") {
		t.Fatalf("panel:\n%s", got)
	}
}

func TestFootHidesSetupWhenNoStepIsOpen(t *testing.T) {
	st := boardState(boardMatch("hearth-api", time.Hour))
	m := boardModel(t, st)
	m.recs, m.doors = nil, nil
	lines := plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new" {
		t.Fatalf("foot %q", got)
	}
	m.recs = fixtureState().Recommendations[:1]
	m.doors = []doors.Door{{ID: "tmux", Title: "tmux"}}
	lines = plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new · ✧ 1 tip · ! 1 setup" {
		t.Fatalf("foot %q", got)
	}
	m.recs = fixtureState().Recommendations[:2]
	lines = plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new · ✧ 2 tips · ! 1 setup" {
		t.Fatalf("foot %q", got)
	}
}

func TestPanelKeepsTheChosenRowInView(t *testing.T) {
	var ss []sample.Session
	for i := range 25 {
		ss = append(ss, boardMatch(fmt.Sprintf("m%02d", i), time.Hour))
	}
	m := boardModel(t, boardState(ss...))
	order := m.sideOrder()
	m.sideAt = order[len(order)-1]
	last := m.sideGroups()[0].units[len(order)-1].members[0].Name
	lines := m.sidePanel(60, 12)
	if len(lines) != 12 {
		t.Fatalf("%d lines, want 12", len(lines))
	}
	if got := panelText(m, 60, 12); !strings.Contains(got, last) {
		t.Fatalf("chosen %s is out of view:\n%s", last, got)
	}
	if !slices.ContainsFunc(plainLines(lines), func(l string) bool { return strings.Contains(l, "+ new") }) {
		t.Fatal("no foot")
	}
}

func TestStatesWordsShow(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	loading := m
	loading.have = false
	if got := panelText(loading, 50, 5); !strings.Contains(got, "reading your matches…") {
		t.Fatalf("loading:\n%s", got)
	}
	m.filter = "x"
	if got := panelText(m, 60, 5); !strings.Contains(got, `Nothing matches "x". esc clears the search.`) {
		t.Fatalf("search:\n%s", got)
	}
}

func TestHeadCountsWhatTheSearchShows(t *testing.T) {
	var ss []sample.Session
	for i := range 8 {
		ss = append(ss, boardMatch(fmt.Sprintf("m%02d", i), time.Hour))
	}
	ss[3].Name = "kindle-web"
	m := boardModel(t, boardState(ss...))
	m.filter = "kindle"
	if got := panelText(m, 50, 10); !strings.Contains(got, "IDLE · 1") {
		t.Fatalf("panel:\n%s", got)
	}
}

func TestPanelOfNoWidthDrawsNothing(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	for _, w := range []int{0, -3} {
		if got := m.sidePanel(w, 8); got != nil {
			t.Fatalf("w=%d drew %q", w, got)
		}
	}
}

func TestNoChoiceMeansTheFirstInOrder(t *testing.T) {
	a, b := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour)
	st := boardState(a, b)
	st.Queue = append(st.Queue, waitFor(b, queue.StatePermission, time.Minute))
	m := boardModel(t, st)
	m.sideAt = paneKey{}
	first := m.sideOrder()[0]
	if first != paneOf(b) {
		t.Fatalf("fixture: sideAt %v, first %v", m.sideAt, first)
	}
	for _, l := range plainLines(m.sidePanel(50, 12)) {
		if strings.HasPrefix(l, "▌") {
			if !strings.Contains(l, "hearth-api") {
				t.Fatalf("chosen row %q, want hearth-api", l)
			}
			return
		}
	}
	t.Fatal("no row is chosen")
}

func TestChainLinksLightWhenAMemberIsChosen(t *testing.T) {
	m := loginChain(t, nil)
	other := boardMatch("spark-docs", time.Hour)
	m.all.sessions = append(m.all.sessions, other)
	lit := func(at paneKey) string {
		m.sideAt = at
		return strings.Join(m.sidePanel(60, 20), "\n")
	}
	var flint sample.Session
	for _, s := range m.all.sessions {
		if s.Name == "flint-auth" {
			flint = s
		}
	}
	on, off := lit(paneOf(flint)), lit(paneOf(other))
	head := func(out string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "≋ login") {
				return l
			}
		}
		return ""
	}
	if head(on) == "" || head(on) == head(off) {
		t.Fatalf("head not lit by a member:\n%q\n%q", head(on), head(off))
	}
	if !strings.Contains(on, m.st.accent.Render("├")) || strings.Contains(off, m.st.accent.Render("├")) {
		t.Fatalf("links: on has accent %v, off has accent %v", strings.Contains(on, m.st.accent.Render("├")), strings.Contains(off, m.st.accent.Render("├")))
	}
}

// TestACutListSaysHowManyAreBelow: a list cut by the screen says how many
// sessions it hides below and above, and how many of them need you.
func TestACutListSaysHowManyAreBelow(t *testing.T) {
	m := demoSized(t, 132, 14)
	m.toList()
	lines := plainLines(m.sidePanel(40, 9))
	last := strings.TrimSpace(lines[len(lines)-2])
	if !strings.HasPrefix(last, "↓ ") || !strings.Contains(last, " more") {
		t.Fatalf("the cut list does not say what is below:\n%s", strings.Join(lines, "\n"))
	}
	for range len(m.sideOrder()) - 1 { // to the last row, not past it to the foot
		m = pressOn(m, "down")
	}
	lines = plainLines(m.sidePanel(40, 9))
	if first := strings.TrimSpace(lines[0]); !strings.HasPrefix(first, "↑ ") || !strings.Contains(first, " more") {
		t.Fatalf("the list scrolled down does not say what is above:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "need") {
		t.Errorf("the marks do not say how many hidden sessions need you:\n%s", strings.Join(lines, "\n"))
	}
}

// TestAWaitingMateDrawsInItsTeam: a member that waits for recover draws
// under its chain's head, after the live members, in the wait color.
func TestAWaitingMateDrawsInItsTeam(t *testing.T) {
	m := waitingLogin(t, "kindle-web")
	out := m.sidePanel(60, 20)
	plain := plainLines(out)
	head := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "≋ login") })
	row := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "◌ kindle-web") })
	if head < 0 || row < head {
		t.Fatalf("no waiting row under the head:\n%s", strings.Join(plain, "\n"))
	}
	if !strings.Contains(plain[row], "waits for recover") || !strings.Contains(plain[row], "└") {
		t.Fatalf("row %q, want the last link and waits for recover", plain[row])
	}
	if !strings.Contains(plain[head], "step 2 of 3 · 2 + 1 wait") {
		t.Fatalf("head %q, want step 2 of 3 · 2 + 1 wait", plain[head])
	}
	if !strings.Contains(out[row], m.st.wait.Render("◌ kindle-web")) || !strings.Contains(out[head], m.st.wait.Render("1 wait")) {
		t.Fatalf("the waiting row or the head's wait is not in the wait color:\n%q\n%q", out[head], out[row])
	}
}

// TestATeamOfWaitingMatesStillDraws: a chain whose members all wait still
// draws its head and its rows, in the IDLE group.
func TestATeamOfWaitingMatesStillDraws(t *testing.T) {
	m := waitingLogin(t, "kindle-web", "flint-auth", "wick-mobile")
	plain := plainLines(m.sidePanel(60, 20))
	text := strings.Join(plain, "\n")
	idle := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "IDLE") })
	head := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "≋ login") })
	if idle < 0 || head < idle || strings.Count(text, "◌ ") != 3 || strings.Contains(text, "NEEDS YOU") {
		t.Fatalf("the waiting team does not draw in IDLE:\n%s", text)
	}
	// No live row is drawn: the list chooses the first waiting row, and it
	// takes its second line.
	if !strings.Contains(plain[head], "3 wait") || !strings.Contains(plain[head+1], "▌├ ◌ kindle-web") || !strings.Contains(plain[head+4], "└ ◌ wick-mobile") {
		t.Fatalf("head %q, first row %q, last row %q", plain[head], plain[head+1], plain[head+4])
	}
}

// TestNoWaitingMateNoChange: a doc with no member that waits draws the list
// as it did before a member could wait.
func TestNoWaitingMateNoChange(t *testing.T) {
	const before = " \x1b[38;2;119;111;96mNEEDS YOU · 1\x1b[m\x1b[38;2;119;111;96m  −\x1b[m                                           \n \x1b[1;38;2;195;165;106m≋ login\x1b[m                                    \x1b[38;2;166;157;141mstep 2 of 3 · 3\x1b[m \n\x1b[38;2;195;165;106m▌\x1b[m\x1b[38;2;195;165;106m├\x1b[m \x1b[38;2;119;111;96m╵\x1b[m \x1b[1;38;2;235;230;220mkindle-web\x1b[m                                        \x1b[38;2;166;157;141m10m\x1b[m \x1b[38;2;166;157;141m✻\x1b[m\n \x1b[38;2;195;165;106m│\x1b[m   \x1b[38;2;119;111;96midle\x1b[m                                                   \n \x1b[38;2;195;165;106m├\x1b[m \x1b[1;38;2;119;111;96m│\x1b[m \x1b[1;38;2;235;230;220mflint-auth\x1b[m                                         \x1b[38;2;166;157;141m4m\x1b[m \x1b[38;2;166;157;141m✻\x1b[m\n \x1b[38;2;195;165;106m│\x1b[m   \x1b[38;2;119;111;96masks\x1b[m                                                   \n \x1b[38;2;195;165;106m└\x1b[m \x1b[38;2;232;130;46m✦\x1b[m \x1b[38;2;235;230;220mwick-mobile\x1b[m                                          \x1b[38;2;166;157;141m\x1b[m \x1b[38;2;166;157;141m✻\x1b[m\n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n                                                            \n \x1b[38;2;195;165;106m+ new\x1b[m                                                      "
	plain := loginChain(t, nil)
	st := loginState(nil)
	for _, s := range st.Sessions {
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s)})
	}
	mates := boardModel(t, st)
	for name, m := range map[string]Model{"no mates": plain, "mates with no wait": mates} {
		if got := strings.Join(m.sidePanel(60, 20), "\n"); got != before {
			t.Errorf("%s: the list changed:\n%q", name, got)
		}
	}
	// A member that is gone with no wait (its pane lives, no agent shows)
	// draws no waiting row.
	gone := loginState(nil)
	gone.Sessions = slices.DeleteFunc(gone.Sessions, func(s sample.Session) bool {
		if s.Name != "kindle-web" {
			return false
		}
		gone.Chains.Mates = append(gone.Chains.Mates, state.MateView{Member: memberOf(s), Unseen: time.Now()})
		return true
	})
	if got := strings.Join(boardModel(t, gone).sidePanel(60, 20), "\n"); strings.Contains(got, "◌") || strings.Contains(got, "kindle-web") {
		t.Errorf("a gone member with no wait draws:\n%s", got)
	}
}

// waitingKindle is loginChain with kindle-web gone from the host: it waits
// for recover since since, with conversation c-3 and inbox lines that
// wait. The clock reads 22:50 on 1 Oct, local time. wick-mobile is chosen.
func waitingKindle(t *testing.T, since time.Time, inbox int) Model {
	t.Helper()
	st := loginState(nil)
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool {
		if s.Name != "kindle-web" {
			return false
		}
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Inbox: inbox, Wait: &state.WaitView{
			Name: s.Name, Agent: "claude", Conversation: "c-3", Since: since,
			Forget: time.Date(2026, 10, 31, 22, 14, 0, 0, time.Local),
		}})
		return true
	})
	m := boardModel(t, st)
	m.opt.Now = func() time.Time { return time.Date(2026, 10, 1, 22, 50, 0, 0, time.Local) }
	m.sideAt = paneOf(matchOf(t, m, "wick-mobile"))
	m.toList()
	return m
}

// TestAWaitingRowIsChosen: ↓ from the last live member chooses the waiting
// row. Its second line says when its server stopped and names the keys.
func TestAWaitingRowIsChosen(t *testing.T) {
	for _, c := range []struct {
		since time.Time
		want  string
	}{
		{time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), "its server stopped 22:14 · ⏎ reopen · m menu"},
		{time.Date(2026, 9, 30, 9, 5, 0, 0, time.Local), "its server stopped 30 Sep · ⏎ reopen · m menu"},
	} {
		m := pressOn(waitingKindle(t, c.since, 0), "down")
		if k := m.chosenSide(); !strings.HasPrefix(k.pane, "wait:") {
			t.Fatalf("↓ from the last live member chose %v, want the waiting row", k)
		}
		if s, ok := m.sideChosen(); ok {
			t.Fatalf("the waiting row gives the session %s to act on", s.Name)
		}
		plain := plainLines(m.sidePanel(60, 20))
		row := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "◌ kindle-web") })
		if row < 0 || row+1 >= len(plain) {
			t.Fatalf("no waiting row:\n%s", strings.Join(plain, "\n"))
		}
		if !strings.HasPrefix(plain[row], "▌") || !strings.Contains(plain[row+1], c.want) {
			t.Errorf("the chosen waiting row reads:\n%s\n%s\nwant the mark and %q", plain[row], plain[row+1], c.want)
		}
	}
}

// TestTheChatShowsAWaitingMate: with no agent pane beside the list, the
// chat's place of a waiting row tells why it waits and what to do.
func TestTheChatShowsAWaitingMate(t *testing.T) {
	m := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 2), "down")
	chat := strings.Join(plainLines(m.chatPanel(100, 14)), "\n")
	for _, want := range []string{
		"kindle-web  waits for recover · in login",
		"Its tmux server stopped at 22:14.",
		"Its conversation is saved: claude c-3",
		"It stays in login until it is back, or until the record forgets it on 31 Oct.",
		"✉ 2 lines wait for it",
		"⏎ reopen it",
		"m out of login",
	} {
		if !strings.Contains(chat, want) {
			t.Errorf("the chat's place has no %q:\n%s", want, chat)
		}
	}
	none := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down")
	if chat := strings.Join(plainLines(none.chatPanel(100, 14)), "\n"); strings.Contains(chat, "✉") {
		t.Errorf("with no line waiting, the chat's place still says ✉:\n%s", chat)
	}
}

// TestAWaitingRowTakesNoSessionKey: the keys that act on a session do
// nothing on a waiting row. They say why, and send no act.
func TestAWaitingRowTakesNoSessionKey(t *testing.T) {
	for _, key := range []string{"x", "p", "r", "c"} {
		m := pressOn(waitingKindle(t, time.Date(2026, 10, 1, 22, 14, 0, 0, time.Local), 0), "down")
		before := len(boardFake(m).allActs())
		m = pressOn(m, key)
		if n := len(boardFake(m).allActs()); n != before || m.form != nil || m.pending != nil {
			t.Errorf("%s on a waiting row: %d acts, form %v, pending %v", key, n-before, m.form != nil, m.pending)
		}
		if !strings.Contains(m.noticeText, "kindle-web waits for recover") {
			t.Errorf("%s on a waiting row says %q", key, m.noticeText)
		}
	}
}

// TestAMatchWaitingInTwoTeamsIsOneRow: a waiting match in two teams draws
// once, in the first team, as a live match in two chains does: a second
// row with the same key would stop ↓ and draw two chosen rows.
func TestAMatchWaitingInTwoTeamsIsOneRow(t *testing.T) {
	st := loginState(nil)
	var kindle chains.Member
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool {
		if s.Name != "kindle-web" {
			return false
		}
		kindle = memberOf(s)
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: kindle, Wait: &state.WaitView{Name: s.Name, Agent: "claude"}})
		return true
	})
	st.Chains.Chains = append(st.Chains.Chains, chains.Chain{ID: "c2", Name: "hearth", Members: []chains.Member{kindle, st.Chains.Chains[0].Members[1]}})
	m := boardModel(t, st)
	var keys []paneKey
	for _, k := range m.sideOrder() {
		if k.waits() {
			keys = append(keys, k)
		}
	}
	if len(keys) != 1 {
		t.Fatalf("waiting keys = %v, want one", keys)
	}
	if got := strings.Count(strings.Join(m.sidePanel(60, 20), "\n"), "◌ kindle-web"); got != 1 {
		t.Fatalf("kindle-web draws %d times", got)
	}
}

// TestARightClickOpensTheMenuOfAWaitingMatch: a right-click, or a long
// press on a phone, opens the waiting match's menu, as m does.
func TestARightClickOpensTheMenuOfAWaitingMatch(t *testing.T) {
	m := waitingLogin(t, "kindle-web")
	var k paneKey
	for _, o := range m.sideOrder() {
		if o.waits() {
			k = o
		}
	}
	got, _ := m.sideMenuOn(k, 2, 5)
	mm := got.(Model)
	if mm.menu == nil || len(mm.menu.items) == 0 || mm.menu.items[0].label != "reopen it" {
		t.Fatalf("menu = %+v", mm.menu)
	}
}

// waitingInTwoTeams is loginChain with kindle-web and wick-mobile gone from
// the host: both wait for recover. kindle-web is in login and in hearth.
func waitingInTwoTeams(t *testing.T) Model {
	t.Helper()
	st := loginState(nil)
	var kindle chains.Member
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool {
		if s.Name != "kindle-web" && s.Name != "wick-mobile" {
			return false
		}
		if s.Name == "kindle-web" {
			kindle = memberOf(s)
		}
		st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Wait: &state.WaitView{Name: s.Name, Agent: "claude", Conversation: "c-" + s.Name}})
		return true
	})
	st.Chains.Chains = append(st.Chains.Chains, chains.Chain{ID: "c2", Name: "hearth", Members: []chains.Member{kindle, st.Chains.Chains[0].Members[1]}})
	return boardModel(t, st)
}

// footReopenSpan is the span of the foot's ↻ part, and the foot's words.
func footReopenSpan(m Model) (span, string, bool) {
	line, spans := m.sideFoot(80)
	for _, sp := range spans {
		if strings.HasPrefix(sp.press, footReopen) {
			return sp, plainLines([]string{line})[0], true
		}
	}
	return span{}, plainLines([]string{line})[0], false
}

// TestTheFootCountsMatchesToReopen: the foot counts each match that waits
// for recover once, also when it waits in two teams. With none, the foot
// has no ↻ part.
func TestTheFootCountsMatchesToReopen(t *testing.T) {
	if _, line, ok := footReopenSpan(waitingInTwoTeams(t)); !ok || !strings.Contains(line, "↻ 2 to reopen") {
		t.Fatalf("the foot reads %q, want ↻ 2 to reopen", line)
	}
	if _, line, ok := footReopenSpan(loginChain(t, nil)); ok || strings.Contains(line, "↻") {
		t.Fatalf("with no waiting match the foot reads %q", line)
	}
}

// TestTheFootReopensThemAll: a tap on ↻ asks for a typed y and names the
// matches. y sends one recover act with all their conversations, as JSON.
func TestTheFootReopensThemAll(t *testing.T) {
	m := waitingInTwoTeams(t)
	sp, _, ok := footReopenSpan(m)
	if !ok {
		t.Fatal("the foot has no ↻ part")
	}
	next, _ := m.footTap([]span{sp}, sp.x0)
	asked := next.(Model)
	f := boardFake(asked)
	if len(f.acts()) != 0 || asked.pending == nil || !asked.pending.destructive || asked.pending.String() != "reopen 2 matches: kindle-web, wick-mobile?" {
		t.Fatalf("a tap on ↻: acts %+v, pending %+v", f.acts(), asked.pending)
	}
	if a, convs := reopenAct(t, pressOn(asked, "y")); a.RecID != "recover" || a.Confirm != "y" || !slices.Equal(convs, []string{"c-kindle-web", "c-wick-mobile"}) {
		t.Fatalf("y sent %+v, conversations %q", a, convs)
	}

	one := waitingLogin(t, "kindle-web")
	sp, _, _ = footReopenSpan(one)
	next, _ = one.footTap([]span{sp}, sp.x0)
	if p := next.(Model).pending; p == nil || p.String() != "reopen kindle-web?" {
		t.Fatalf("one waiting match: pending %+v", p)
	}
}
