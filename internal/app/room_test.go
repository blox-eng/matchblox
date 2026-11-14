package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
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
// hearth-api's own report says it waits at a permission prompt, as Claude
// Code's does while the prompt shows.
func twoAsk(st *proto.State) {
	at := time.Date(2026, 10, 1, 14, 2, 0, 0, time.UTC)
	st.Queue = nil
	for i, s := range st.Sessions {
		switch s.Name {
		case "hearth-api":
			x := &st.Sessions[i]
			x.Busy, x.Status, x.Reported, x.WaitingFor = false, sample.StatusWaiting, true, "permission prompt"
			st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, SessionID: s.SessionID, Name: s.Name, State: queue.StatePermission, Since: at, LastLine: "may I force-push the rebase?"})
		case "ember-billing":
			st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, SessionID: s.SessionID, Name: s.Name, State: queue.StateQuestion, Since: at.Add(3 * time.Minute), LastLine: "which branch?"})
		}
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
	if !hasCard(m.openRoom(k), "spark-docs") {
		t.Fatal("the room of a chain of one draws no card of its match")
	}
}

// hasCard tells if the board draws the card of the match named name.
func hasCard(m Model, name string) bool {
	b, _ := m.boardDrawOf()
	for _, ss := range b.idx {
		if slices.ContainsFunc(ss, func(s sample.Session) bool { return s.Name == name }) {
			return true
		}
	}
	return false
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

// TestATeamWithOneMatchLeftStillHasItsRoom: chainIx drops a team with one
// present match; its room still shows it, a card on the narrowed board
// (eng review correction 5).
func TestATeamWithOneMatchLeftStillHasItsRoom(t *testing.T) {
	st := fuseState(t, []string{"hearth-api"})
	m, _ := roomOf(t, st, "hearth-api")
	var host string
	for _, s := range m.all.sessions {
		host = s.Host
	}
	k := chainKey{host, "c1"}
	if !hasCard(m.openRoom(k), "hearth-api") {
		t.Fatal("the room of a team with one match left draws no card of it")
	}
	if !slices.Contains(m.roomKeys(), k) {
		t.Fatalf("rooms %v lack %v", m.roomKeys(), k)
	}
}
