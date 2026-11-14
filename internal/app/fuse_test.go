package app

import (
	"cmp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// fuseState is the demo machine as the mockup draws it: kindle-web asks,
// hearth-api and flint-auth work, ember-billing and spark-docs are quiet.
// Each list of names is one chain.
func fuseState(t *testing.T, chained ...[]string) proto.State {
	t.Helper()
	st := screensState(t)
	keep := []string{"kindle-web", "hearth-api", "ember-billing", "flint-auth", "spark-docs"}
	st.Sessions = slices.DeleteFunc(st.Sessions, func(s sample.Session) bool { return !slices.Contains(keep, s.Name) })
	pane := map[string]string{}
	for i := range st.Sessions {
		s := &st.Sessions[i]
		pane[s.Name] = s.Pane
		s.Do, s.Shells, s.Agents, s.Progress, s.Ask = "", nil, 0, nil, nil
		switch s.Name {
		case "hearth-api", "flint-auth":
			s.Busy, s.Status, s.Idle = true, "busy", 0
		case "ember-billing":
			s.Busy, s.Status, s.Idle = false, "idle", 3*time.Minute
		case "spark-docs":
			s.Busy, s.Status, s.Idle = false, "idle", 9*time.Minute
		}
	}
	st.Queue = slices.DeleteFunc(st.Queue, func(it queue.Item) bool { return it.Pane != pane["kindle-web"] })
	st.Ships, st.Children = nil, nil
	st.Chains = &state.ChainsView{}
	for i, names := range chained {
		c := chains.Chain{ID: "c" + string(rune('1'+i)), Name: fuseTeams[i], Repo: "/home/user/src/hearth"}
		for _, n := range names {
			for _, s := range st.Sessions {
				if s.Name == n {
					c.Members = append(c.Members, memberOf(s))
				}
			}
		}
		st.Chains.Chains = append(st.Chains.Chains, c)
	}
	return st
}

// fuseTeams name the chains of fuseState, in order.
var fuseTeams = []string{"hearth", "web", "docs"}

// fuseModel is the console on fuseState, on the demo's still clock, with
// the session named pick picked.
func fuseModel(t *testing.T, width int, st proto.State, pick string) Model {
	t.Helper()
	var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.tab = tabSessions
	if pick != "" {
		for _, s := range m.snap.Sessions {
			if s.Name == pick {
				choose(&m, s.PID)
			}
		}
	}
	return m
}

var mockupChain = []string{"kindle-web", "hearth-api", "ember-billing"}

func pidOf(m Model, name string) int {
	for _, s := range m.snap.Sessions {
		if s.Name == name {
			return s.PID
		}
	}
	return 0
}

// joinModel is the console after a first state with hearth of two, then a
// state where state has the given chains.
func joinModel(t *testing.T, width int, next ...[]string) Model {
	t.Helper()
	var m tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(stateMsg(fuseState(t, mockupChain[:2])))
	m, _ = m.Update(stateMsg(fuseState(t, next...)))
	return m.(Model)
}

func TestJoinToastAndSpark(t *testing.T) {
	if m := joinModel(t, 160, mockupChain); saidNow(m) != "ember-billing joined hearth · out of hearth is in its menu, in the room" {
		t.Fatalf("toast = %q", saidNow(m))
	}
}

func TestJoinToastNarrow(t *testing.T) {
	if m := joinModel(t, 40, mockupChain); saidNow(m) != "ember-billing joined hearth · m: out" {
		t.Fatalf("toast = %q", saidNow(m))
	}
}

// saidNow is the news the console said last: the header's toast, or on the
// board the notice in the strip's row.
func saidNow(m Model) string { return cmp.Or(m.toast, m.noticeText) }

func TestNewTeamToast(t *testing.T) {
	if m := joinModel(t, 160, mockupChain[:2], []string{"flint-auth", "spark-docs"}); saidNow(m) != "web formed: flint-auth, spark-docs" {
		t.Fatalf("toast = %q", saidNow(m))
	}
}

func TestNoJoinNoToast(t *testing.T) {
	if m := joinModel(t, 160, mockupChain[:2]); m.toast != "" {
		t.Fatalf("toast = %q", m.toast)
	}
}

func TestOutOfTeamAsks(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain), "")
	var s sample.Session
	for _, x := range m.snap.Sessions {
		if x.Name == "ember-billing" {
			s = x
		}
	}
	a := m.outOfTeam(s, "hearth")
	if a.label != "take ember-billing out of hearth" || a.rec != "chain:out:"+s.Pane+"@hearth" || !a.destructive {
		t.Fatalf("%+v", a)
	}
	if got := a.String(); got != "take ember-billing out of hearth" { // the confirm shows it: no steps
		t.Fatalf("confirm says %q", got)
	}
}

// TestAClearIsNoJoin: a member renamed by a /clear (a new session id in
// the same pane) is not a match that joined.
func TestAClearIsNoJoin(t *testing.T) {
	var m tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m, _ = m.Update(stateMsg(fuseState(t, mockupChain[:2])))
	st := fuseState(t, mockupChain[:2])
	for i := range st.Sessions {
		if st.Sessions[i].Name == "hearth-api" {
			st.Sessions[i].SessionID = "cleared"
			st.Chains.Chains[0].Members[1] = memberOf(st.Sessions[i])
		}
	}
	m, _ = m.Update(stateMsg(st))
	if got := m.(Model).toast; got != "" {
		t.Fatalf("toast = %q", got)
	}
}

// stokerPane is the pane of the chain stoker s1's agent on its own server.
const stokerPane = "%9@matchblox"

// headState is fuseState with the mockup's chain (c1) given chain stoker
// s1 in v; with agent, its agent runs in tmux session stoker-s1 on the
// stoker's server.
func headState(t *testing.T, v proto.StokerView, agent bool, chained ...[]string) proto.State {
	t.Helper()
	if len(chained) == 0 {
		chained = [][]string{mockupChain}
	}
	st := fuseState(t, chained...)
	st.StokerServer = "matchblox"
	if agent {
		st.Sessions = append(st.Sessions, sample.Session{Host: st.Sessions[0].Host, Name: "stoker-s1", PID: 9001, Pane: stokerPane,
			Target: "stoker-s1:0.0", Status: "busy", Busy: true, Cwd: "/home/user"})
	}
	st.Stokers = []state.ChainStoker{{ID: "s1", Chain: "c1", Name: "kindle-web", View: v}}
	return st
}

// runningPlan is a run on, with two steps of five of its plan done.
func runningPlan() proto.StokerView {
	v := proto.StokerView{PaneUp: true}
	v.On = true
	o := proto.StokerOrder{State: "running", Words: "ship the chain"}
	for n := 1; n <= 5; n++ {
		o.Plan = append(o.Plan, proto.StokerPlan{N: n, Text: "step"})
	}
	v.Order = &o
	for n := 1; n <= 2; n++ {
		v.Run = append(v.Run, proto.StokerEntry{ID: "p" + string(rune('0'+n)), Step: n, State: "done", At: demoAt.Add(-time.Hour)})
	}
	return v
}

// headModel is the console on st, on the demo's still clock, with its
// conn.
func headModel(t *testing.T, width int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	f := newFake()
	var next tea.Model = New(Options{NoMotion: true, Conn: f, Now: func() time.Time { return demoAt }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.tab = tabSessions
	return m, f
}

// asker makes the stoker's agent in st ask a question.
func asker(st *proto.State) {
	for i := range st.Sessions {
		if s := &st.Sessions[i]; s.Pane == stokerPane {
			s.Busy, s.Status, s.Asks = false, "idle", true
			s.Ask = &said.Ask{ID: "q1", Questions: []said.Question{{Question: "merge #812 now?"}}}
		}
	}
}

// TestHeadScreens: the head row at 150 and 44 columns, a head that asks,
// and the start sheet of a chain.
func TestHeadScreens(t *testing.T) {
	st := headState(t, runningPlan(), true)
	check(t, st, "chainhead", 150, 30, nil)
	check(t, st, "chainhead", 44, 31, nil)
	asks := headState(t, runningPlan(), true, []string{"hearth-api", "ember-billing"})
	asker(&asks)
	check(t, asks, "chainheadasks", 150, 30, nil)
	check(t, asks, "chainheadasks", 44, 31, nil)
	start := append([]string{"F"}, strings.Split("merge on green", "")...)
	for _, size := range [][2]int{{44, 31}, {104, 38}} {
		check(t, fuseState(t, mockupChain), "chainsheet", size[0], size[1], append(start, "tab", "space", "tab"))
	}
}

func pick(t *testing.T, m Model, pane string) Model {
	t.Helper()
	for _, s := range m.snap.Sessions {
		if s.Pane == pane {
			choose(&m, s.PID)
			return m
		}
	}
	t.Fatalf("no row for %s", pane)
	return m
}

// wentInto presses k on m and says the tmux commands it ran.
func wentInto(t *testing.T, m Model, k string) (Model, [][]string) {
	t.Helper()
	ran := recordRuns(&m)
	next, cmd := key(m, k)
	drain(cmd)
	return next.(Model), *ran
}

// TestAOnAMemberStaysItsChat: a chain with no stoker keeps a on a member
// for that member: into its own pane.
func TestAOnAMemberStaysItsChat(t *testing.T) {
	m := fuseModel(t, 160, fuseState(t, mockupChain), "hearth-api")
	pane := m.snap.Sessions[slices.IndexFunc(m.snap.Sessions, func(s sample.Session) bool { return s.Name == "hearth-api" })].Pane
	next, ran := wentInto(t, m, "a")
	if len(ran) != 1 || strings.Join(ran[0], " ") != "tmux switch-client -t "+pane || next.sheet != nil {
		t.Fatalf("ran %q, sheet %v", ran, next.sheet)
	}
}

// TestTheHostStokerAskingCountsAsToday: with no chain stoker, the host
// stoker's agent at a menu adds nothing to the header's count.
func TestTheHostStokerAskingCountsAsToday(t *testing.T) {
	today := fuseState(t)
	today.StokerServer = "matchblox"
	asking := today
	asking.Sessions = append(slices.Clone(today.Sessions), sample.Session{Host: today.Sessions[0].Host, Name: "stoker", PID: 9002,
		Pane: "%8@matchblox", Target: "stoker:0.0", Status: "idle", Asks: true, Permits: true})
	header := func(st proto.State) string {
		m, _ := headModel(t, 160, st)
		return strings.SplitN(screen(m), "\n", 2)[0]
	}
	if got, want := header(asking), header(today); got != want {
		t.Fatalf("header %q, want %q", got, want)
	}
}

// TestPeerChainDrawsItsFuse holds the console's side of #389 in place: a
// chain of a peer's two sessions with no name and no mates, as v0.38.1 sent
// it, draws its stub on both cards of the board. It passes without this
// branch too: the cause of #389 is not known.
func TestPeerChainDrawsItsFuse(t *testing.T) {
	peer := hostState(sess(2, "flint-auth", "%2", nil), sess(3, "spark-docs", "%3", nil))
	c := chains.Chain{ID: "c1"}
	for _, s := range peer.Sessions {
		c.Members = append(c.Members, memberOf(s))
	}
	peer.Chains = &state.ChainsView{Chains: []chains.Chain{c}}
	m, _ := twoHosts(t, hostState(sess(1, "hearth-api", "%1", nil)), peer)
	m = boardOn(m)
	for _, name := range []string{"flint-auth", "spark-docs"} {
		if stub := stubOf(t, m, name); stub == "" {
			t.Errorf("%s's card in a peer's chain has no stub", name)
		}
	}
}
