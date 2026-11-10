package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/state"
)

// roomOf is the console on st and the key of the chain session name is in.
func roomOf(t *testing.T, st proto.State, name string) (Model, chainKey) {
	t.Helper()
	m, _ := headModel(t, 160, st)
	for _, s := range m.all.sessions {
		if s.Name == name {
			if teams := m.teamsOf(s); len(teams) > 0 {
				return m, chainKey{s.Host, teams[0].ID}
			}
			return m, oneKey(s)
		}
	}
	t.Fatalf("no session %s", name)
	return m, chainKey{}
}

func kinds(rows []roomRow) []roomKind {
	var out []roomKind
	for _, r := range rows {
		out = append(out, r.kind)
	}
	return out
}

func roomRowOf(t *testing.T, rows []roomRow, kind roomKind, has string) roomRow {
	t.Helper()
	for _, r := range rows {
		if r.kind == kind && strings.Contains(r.text, has) {
			return r
		}
	}
	t.Fatalf("no row of kind %d with %q in:\n%s", kind, has, rowsText(rows))
	return roomRow{}
}

func rowsText(rows []roomRow) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.text + "\n")
	}
	return b.String()
}

// twoAsk makes hearth-api ask at 14:02 and ember-billing at 14:05.
func twoAsk(st *proto.State) {
	at := time.Date(2026, 10, 1, 14, 2, 0, 0, time.UTC)
	st.Queue = nil
	for _, s := range st.Sessions {
		switch s.Name {
		case "hearth-api":
			st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, SessionID: s.SessionID, Name: s.Name, State: queue.StatePermission, Since: at, LastLine: "may I force-push the rebase?"})
		case "ember-billing":
			st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, SessionID: s.SessionID, Name: s.Name, State: queue.StateQuestion, Since: at.Add(3 * time.Minute), LastLine: "which branch?"})
		}
	}
}

func TestRoomRowsLeadWithTheOldestNeed(t *testing.T) {
	st := headState(t, runningPlan(), true)
	twoAsk(&st)
	m, k := roomOf(t, st, "hearth-api")
	rows := m.roomRows(k)
	if len(rows) == 0 || rows[0].kind != roomNeed || !strings.Contains(rows[0].text, "hearth-api asks: may I force-push the rebase?") {
		t.Fatalf("the oldest need does not lead:\n%s", rowsText(rows))
	}
	if _, more, ok := m.needOf(k); !ok || more != 1 {
		t.Fatalf("more = %d, %v; want 1", more, ok)
	}
}

func TestRoomWithNoNeedHasNoNeedRow(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, k := roomOf(t, st, "hearth-api")
	if slices.Contains(kinds(m.roomRows(k)), roomNeed) {
		t.Fatalf("a need row with no need:\n%s", rowsText(m.roomRows(k)))
	}
}

func TestRoomCardShowsTheActiveStepAndOneEachSide(t *testing.T) {
	v := runningPlan()
	for n := 6; n <= 8; n++ {
		v.Order.Plan = append(v.Order.Plan, proto.StokerPlan{N: n, Text: "step"})
	}
	v.Run = append(v.Run, proto.StokerEntry{ID: "p3", Step: 3, State: "done", At: demoAt.Add(-time.Hour)})
	m, k := roomOf(t, headState(t, v, true), "hearth-api")
	var steps []string
	var more []string
	for _, r := range m.roomRows(k) {
		switch r.kind {
		case roomStep:
			steps = append(steps, r.id)
		case roomMore:
			more = append(more, r.text)
		}
	}
	if !slices.Equal(steps, []string{"3", "4", "5"}) || !slices.Equal(more, []string{"↓ 5 more"}) {
		t.Fatalf("steps %v more %v", steps, more)
	}
	m.room = &room{k: k, unfolded: true}
	steps = steps[:0]
	for _, r := range m.roomRows(k) {
		if r.kind == roomStep {
			steps = append(steps, r.id)
		}
	}
	if len(steps) != 8 {
		t.Fatalf("unfolded steps %v", steps)
	}
}

func TestRoomOfAChainOfOneNamesTheSharedStoker(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "spark-docs")
	rows := m.roomRows(k)
	roomRowOf(t, rows, roomCard, "≋ the stoker · every session in no chain")
	if talk := rows[len(rows)-1]; talk.kind != roomTalk || talk.text != "› tell the stoker…" {
		t.Fatalf("talk = %+v", talk)
	}
	roomRowOf(t, rows, roomMatch, "spark-docs")
}

func TestRoomWithNoStokerAsksForTheOrder(t *testing.T) {
	m, k := roomOf(t, fuseState(t, mockupChain), "hearth-api")
	rows := m.roomRows(k)
	if talk := rows[len(rows)-1]; talk.kind != roomTalk || talk.text != "› what should hearth's stoker do?" {
		t.Fatalf("talk = %+v", talk)
	}
}

func TestRoomTalkNamesARunningStoker(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	rows := m.roomRows(k)
	if talk := rows[len(rows)-1]; talk.text != "› tell hearth's stoker…" {
		t.Fatalf("talk = %+v", talk)
	}
}

func TestOpenRoomPutsTheCursorOnTheNeedElseTheTalkLine(t *testing.T) {
	st := headState(t, runningPlan(), true)
	twoAsk(&st)
	m, k := roomOf(t, st, "hearth-api")
	if m = m.openRoom(k); m.room == nil || m.roomRows(k)[m.room.at].kind != roomNeed {
		t.Fatalf("the cursor is not on the need: %+v", m.room)
	}
	st.Queue = nil
	m, k = roomOf(t, st, "hearth-api")
	m = m.openRoom(k)
	if rows := m.roomRows(k); rows[m.room.at].kind != roomTalk {
		t.Fatalf("the cursor is on %+v, not the talk line", rows[m.room.at])
	}
}

func TestMatchRowCarriesMarkRepoAndInbox(t *testing.T) {
	st := headState(t, runningPlan(), true)
	for _, s := range st.Sessions {
		if s.Name == "hearth-api" {
			st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Inbox: 2})
		}
	}
	m, k := roomOf(t, st, "hearth-api")
	row := roomRowOf(t, m.roomRows(k), roomMatch, "hearth-api")
	if !strings.HasPrefix(row.text, "✻ hearth-api") || !strings.Contains(row.text, "✉ 2") {
		t.Fatalf("row = %q", row.text)
	}
	if strings.Contains(row.text, "hearth-api ·") || strings.Contains(row.text, " in hearth-api") {
		t.Fatalf("a repo on a team in one repo: %q", row.text)
	}
}

func TestMatchRowNamesTheRepoWhenTheTeamSpansTwo(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	m.git = &proto.GitReport{Repos: []gitscan.Repo{
		{Path: "/work/hearth-api", Worktrees: []gitscan.Worktree{{Path: "/work/hearth-api"}}},
		{Path: "/work/billing", Worktrees: []gitscan.Worktree{{Path: "/work/billing"}, {Path: "/work/billing-v2"}}},
	}}
	for i := range m.all.sessions {
		if m.all.sessions[i].Name == "ember-billing" {
			m.all.sessions[i].Cwd = "/work/billing-v2"
		}
	}
	row := roomRowOf(t, m.roomRows(k), roomMatch, "ember-billing")
	if !strings.Contains(row.text, "ember-billing  billing") {
		t.Fatalf("no repo on a team in two repos: %q", row.text)
	}
	if row := roomRowOf(t, m.roomRows(k), roomMatch, "hearth-api"); strings.Contains(row.text, "hearth-api  hearth-api") {
		t.Fatalf("a repo named as its match is shown twice: %q", row.text)
	}
}

func TestAnUnseenMateStaysFaint(t *testing.T) {
	st := headState(t, runningPlan(), true)
	gone := chains.Member{SessionID: "gone-1", Pane: "%77"}
	st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, gone)
	st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: gone, Unseen: demoAt.Add(-4 * time.Minute)})
	m, k := roomOf(t, st, "hearth-api")
	row := roomRowOf(t, m.roomRows(k), roomMatch, "not seen · 4m")
	if !row.faint {
		t.Fatalf("an unseen mate is not faint: %+v", row)
	}
}

func TestAMatchInTwoTeamsSaysAlsoIn(t *testing.T) {
	st := fuseState(t, mockupChain, []string{"hearth-api", "flint-auth"})
	m, k := roomOf(t, st, "kindle-web")
	if row := roomRowOf(t, m.roomRows(k), roomMatch, "hearth-api"); !strings.Contains(row.text, "· also in web") {
		t.Fatalf("row = %q", row.text)
	}
}

// TestATeamWithOneMatchLeftStillHasItsRoom: chainIx drops a team with one
// present match; its room still lists it (eng review correction 5).
func TestATeamWithOneMatchLeftStillHasItsRoom(t *testing.T) {
	st := fuseState(t, []string{"hearth-api"})
	m, _ := roomOf(t, st, "hearth-api")
	var host string
	for _, s := range m.all.sessions {
		host = s.Host
	}
	k := chainKey{host, "c1"}
	roomRowOf(t, m.roomRows(k), roomMatch, "hearth-api")
	if !slices.Contains(m.roomKeys(), k) {
		t.Fatalf("rooms %v lack %v", m.roomKeys(), k)
	}
}

// TestRoomGitCountsAMergedWorktreeWithNoSession: a worktree of the team's
// repo counts, though no session works in it (eng review correction 4).
func TestRoomGitCountsAMergedWorktreeWithNoSession(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	m.git = &proto.GitReport{Repos: []gitscan.Repo{{Path: "/work/hearth-api", Worktrees: []gitscan.Worktree{
		{Path: "/work/hearth-api"},
		{Path: "/work/hearth-api-old", Branch: "old", Merged: true, Safe: true},
	}}, {Path: "/work/other", Worktrees: []gitscan.Worktree{{Path: "/work/other"}}}}}
	if row := roomRowOf(t, m.roomRows(k), roomGit, "GIT"); row.text != "GIT  2 worktrees · 1 merged · 1 safe to remove" {
		t.Fatalf("git = %q", row.text)
	}
}

func TestRoomWithNoShipsOrWorktreesSaysSo(t *testing.T) {
	m, k := roomOf(t, headState(t, runningPlan(), true), "hearth-api")
	rows := m.roomRows(k)
	roomRowOf(t, rows, roomShips, "SHIPS  no ships yet")
	roomRowOf(t, rows, roomGit, "GIT  no worktrees")
}
