package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func roomText(m Model) string { return rowsText(m.roomRows(m.room.k)) }

func TestRoomStateWakingAfterTheFirstOrder(t *testing.T) {
	m, _ := roomWith(t, fuseState(t, mockupChain), "hearth-api")
	m, _ = send(t, at(t, m, roomTalk, "›"), "enter", "g", "o", "enter")
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act}))
	if m = next.(Model); !strings.Contains(roomText(m), "≋ waking…") {
		t.Fatalf("no waking:\n%s", roomText(m))
	}
}

func TestRoomStateTheStokerDidNotStart(t *testing.T) {
	m, _ := roomWith(t, fuseState(t, mockupChain), "hearth-api")
	m, _ = send(t, at(t, m, roomTalk, "›"), "enter", "g", "o", "enter")
	next, _ := m.Update(resultMsg(proto.Result{ActID: m.room.act, Err: "max_chains is 4"}))
	m = next.(Model)
	if !strings.Contains(roomText(m), "the stoker did not start: max_chains is 4") || m.room.draft != "go" {
		t.Fatalf("draft %q:\n%s", m.room.draft, roomText(m))
	}
}

func TestRoomStateAQuietStoker(t *testing.T) {
	v := runningPlan()
	v.Ready = false
	at := time.Date(2026, 10, 1, 14, 2, 0, 0, time.Local)
	v.Order.At = at
	for i := range v.Run {
		v.Run[i].At = at
	}
	st := headState(t, v, true)
	m, k := roomOf(t, st, "hearth-api")
	m.opt.Now = func() time.Time { return at.Add(61 * time.Second) }
	rows := m.roomRows(k)
	if !strings.Contains(rowsText(rows), "≋ hearth's stoker is quiet since 14:02 · ⏎ asks it") {
		t.Fatalf("no quiet line:\n%s", rowsText(rows))
	}
}

func TestRoomStateTheChainHasNoMatches(t *testing.T) {
	st := fuseState(t, mockupChain)
	st.Sessions = []sample.Session{st.Sessions[0]}
	for _, s := range fuseState(t, mockupChain).Sessions {
		if s.Name == "flint-auth" {
			st.Sessions = []sample.Session{s}
		}
	}
	st.Chains.Chains[0].Members = nil
	m, _ := headModel(t, 160, st)
	k := chainKey{m.opt.Host, "c1"}
	if !strings.Contains(rowsText(m.roomRows(k)), "the chain has no matches · + start one here") {
		t.Fatalf("rows:\n%s", rowsText(m.roomRows(k)))
	}
}
