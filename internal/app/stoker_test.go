package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

func stokerState(v proto.StokerView) proto.State {
	st := queueState()
	st.Stoker = &v
	return st
}

// stokedRun is a night with two steps the builder has not seen.
func stokedRun(at time.Time) proto.StokerView {
	since := at.Add(-8 * time.Hour)
	return proto.StokerView{
		Mode: proto.StokerMode{Since: since, Ended: at.Add(-time.Hour)},
		Run: []proto.StokerEntry{
			{ID: "1", At: since.Add(31 * time.Minute), Session: "a", Name: "app-feature", Pane: "%1", Rule: "compact",
				Result: "sent", Guard: "idle", Before: 176000, After: 9400, Resume: "run the migration test, then open the PR", Back: since.Add(33 * time.Minute)},
			{ID: "2", At: since.Add(2 * time.Hour), Session: "b", Name: "app-review", Pane: "%2", Rule: "compact",
				Result: "skipped: session busy", Guard: "the session in %2 is no longer idle", Before: 180000},
		},
		Unseen: true,
	}
}

func TestFKeyTogglesTheStoker(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(proto.StokerView{}))
	sendKey(m, "f")
	m, g := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	sendKey(m, "f")
	if a, b := f.acts(), g.acts(); len(a) != 1 || a[0].RecID != "stoker:on" || len(b) != 1 || b[0].RecID != "stoker:off" {
		t.Fatalf("acts %+v, %+v", a, b)
	}
}

func TestNKeyStartsAndEndsTheNight(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	sendKey(m, "n")
	night := proto.StokerMode{On: true, Until: time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)}
	m, g := loadedWith(t, 100, stokerState(proto.StokerView{Mode: night}))
	sendKey(m, "n")
	if a, b := f.acts(), g.acts(); len(a) != 1 || a[0].RecID != "stoker:night" || len(b) != 1 || b[0].RecID != "stoker:off" {
		t.Fatalf("acts %+v, %+v", a, b)
	}
}

func TestStatusShowsStokerOrNight(t *testing.T) {
	m, _ := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	if h := ansi.Strip(m.header(100)); !strings.Contains(h, "stoker") {
		t.Fatalf("header %q", h)
	}
	until := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	m, _ = loadedWith(t, 40, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: until}}))
	if h := ansi.Strip(m.header(40)); !strings.Contains(h, "night → 07:00") {
		t.Fatalf("phone header %q", h)
	}
	m, _ = loadedWith(t, 100, stokerState(proto.StokerView{}))
	if h := ansi.Strip(m.header(100)); strings.Contains(h, "stoker") || strings.Contains(h, "night") {
		t.Fatalf("off header %q", h)
	}
}

func TestStokedListsRunsWithResume(t *testing.T) {
	for _, w := range []int{40, 100} {
		st := stokerState(stokedRun(fixtureState().At))
		m, _ := loadedWith(t, w, st)
		out := ansi.Strip(m.render())
		for _, want := range []string{"STOKED", "app-feature", "176k → 9k", "RESUME: run the migration test", "app-review", "skipped: session busy"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: no %q:\n%s", w, want, out)
			}
		}
		// The session keeps its match, as everywhere an agent shows.
		row := strings.Split(out, "\n")[lineOf(t, m, "STOKED")+1]
		if !strings.Contains(row, "app-feature") || !strings.ContainsAny(row, "✦╿│") {
			t.Fatalf("width %d: no match on the step row %q", w, row)
		}
		if lineOf(t, m, "STOKED") > lineOf(t, m, "WAITING FOR YOU") {
			t.Fatalf("width %d: STOKED is under the queue:\n%s", w, out)
		}
		for i, line := range strings.Split(m.render(), "\n") {
			if lipgloss.Width(line) > w {
				t.Fatalf("width %d: line %d is %d wide", w, i, lipgloss.Width(line))
			}
		}
	}
}

func TestEnterFoldsStoked(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(stokedRun(fixtureState().At)))
	sendKey(m, "enter")
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "stoker:ack", Which: "primary"}) {
		t.Fatalf("acts %+v", acts)
	}
	seen := stokedRun(fixtureState().At)
	seen.Unseen = false
	m, _ = loadedWith(t, 100, stokerState(seen))
	if out := ansi.Strip(m.render()); strings.Contains(out, "STOKED") {
		t.Fatalf("a seen run still shows:\n%s", out)
	}
}

func TestNightDoorStartsTheNight(t *testing.T) {
	night := doors.Door{ID: doors.Night, Title: "Try a night", Why: "Going to sleep? n keeps your sessions lit until morning."}
	m, f := loadedWith(t, 100, doorState(night))
	if out := ansi.Strip(m.render()); !strings.Contains(out, "Going to sleep?") {
		t.Fatalf("no night door:\n%s", out)
	}
	next, _ := key(m, "enter")
	sendKey(next, "enter")
	if acts := f.acts(); len(acts) != 1 || acts[0].RecID != "stoker:night" {
		t.Fatalf("acts %+v", acts)
	}
}

// sendKey presses k and runs the command it returns.
func sendKey(m tea.Model, k string) tea.Model {
	next, cmd := key(m, k)
	if cmd != nil {
		next.Update(cmd())
	}
	return next
}

// On a phone a long host name leaves no room for the time: the word stays
// whole, never cut.
func TestNightFitsAPhoneWithALongHostName(t *testing.T) {
	until := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	m, _ := loadedWith(t, 40, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: until}}))
	m.opt.Host = "build-workstation"
	h := ansi.Strip(m.header(40))
	if !strings.Contains(h, "· night") || strings.Contains(h, "…") {
		t.Fatalf("phone header %q", h)
	}
}

func TestStokedShowsAReadBackWithoutCounts(t *testing.T) {
	at := fixtureState().At
	v := stokedRun(at)
	v.Run = v.Run[:1]
	v.Run[0].After, v.Run[0].Back = 0, at.Add(-time.Hour)
	m, _ := loadedWith(t, 100, stokerState(v))
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "176k → compacted") || !strings.Contains(out, "RESUME: run the migration test") {
		t.Fatalf("no read back:\n%s", out)
	}
}
