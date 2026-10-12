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
			{ID: "1", At: since.Add(31 * time.Minute), Session: "a", Name: "app-feature", Pane: "%1", Rule: "compact", Why: "would pass 80% within the next step",
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

// The header's right says the stoker: off, on, or the night with its end;
// a phone drops the end.
func TestStatusShowsStokerOrNight(t *testing.T) {
	m, _ := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	if h := ansi.Strip(m.head(100)); !strings.HasSuffix(h, " stoker ") {
		t.Fatalf("header %q", h)
	}
	until := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	m, _ = loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: until}}))
	if h := ansi.Strip(m.head(100)); !strings.Contains(h, "☾ night → 07:00") {
		t.Fatalf("night header %q", h)
	}
	m, _ = loadedWith(t, 40, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: until}}))
	if h := ansi.Strip(m.head(40)); !strings.Contains(h, "☾ night") || strings.Contains(h, "07:00") {
		t.Fatalf("phone header %q", h)
	}
	m, _ = loadedWith(t, 100, stokerState(proto.StokerView{}))
	if h := ansi.Strip(m.head(100)); !strings.Contains(h, "stoker off") || strings.Contains(h, "night") {
		t.Fatalf("off header %q", h)
	}
}

func TestStokedListsRunsWithResume(t *testing.T) {
	for _, w := range []int{40, 100} {
		st := stokerState(stokedRun(fixtureState().At))
		m0, _ := loadedWith(t, w, st)
		if out := ansi.Strip(m0.render()); strings.Contains(out, "STOKED") || !strings.Contains(out, "STOKER") {
			t.Fatalf("width %d: the run is not in the dock alone:\n%s", w, out)
		}
		m := openCard(m0).(Model)
		out := ansi.Strip(m.render())
		for _, want := range []string{"STOKED", "app-feature", "176k → 9k", "would pass 80%", "RESUME: run the migration test", "app-review", "skipped: session busy"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: no %q:\n%s", w, want, out)
			}
		}
		// The session keeps its match, as everywhere an agent shows.
		row := strings.Split(out, "\n")[lineOf(t, m, "STOKED")+1]
		if !strings.Contains(row, "app-feature") || !strings.ContainsAny(row, "✦╿│") {
			t.Fatalf("width %d: no match on the step row %q", w, row)
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
	sendKey(openCard(m), "enter")
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

// sendKey presses k and runs the command it returns, each of a batch.
func sendKey(m tea.Model, k string) tea.Model {
	next, cmd := key(m, k)
	if cmd == nil {
		return next
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				next.Update(c())
			}
		}
		return next
	}
	next.Update(msg)
	return next
}

// On a phone a long host name gives way: the night's word stays whole,
// never cut.
func TestNightFitsAPhoneWithALongHostName(t *testing.T) {
	until := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	m, _ := loadedWith(t, 40, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: until}}))
	m.opt.Host = "build-workstation"
	h := ansi.Strip(m.head(40))
	if !strings.Contains(h, "☾ night") || strings.Contains(h, "…") {
		t.Fatalf("phone header %q", h)
	}
}

func TestStokedShowsAReadBackWithoutCounts(t *testing.T) {
	at := fixtureState().At
	v := stokedRun(at)
	v.Run = v.Run[:1]
	v.Run[0].After, v.Run[0].Back = 0, at.Add(-time.Hour)
	m, _ := loadedWith(t, 100, stokerState(v))
	out := ansi.Strip(openCard(m).(Model).render())
	if !strings.Contains(out, "176k → compacted") || !strings.Contains(out, "RESUME: run the migration test") {
		t.Fatalf("no read back:\n%s", out)
	}
}

// runningNight is a night in charge with a step done.
func runningNight(at time.Time) proto.StokerView {
	v := stokedRun(at)
	v.On, v.Until, v.Ended, v.Nightly = true, at.Add(4*time.Hour), time.Time{}, true
	v.Watched = []string{"a", "b", "c"}
	return v
}

// sized is the console on st at w by h.
func sized(t *testing.T, w, h int, st proto.State) Model {
	t.Helper()
	m, _ := loadedWith(t, w, st)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

// TestTheStokerLivesInTheDock: while a run is in charge the stoker's panel
// takes a phone's dock until u switches it, and the machine's place
// beside usage on a drawer; a wide desk shows all three. A tap on the
// panel opens the whole card: in the right pane, or on a phone's Machine
// tab.
func TestTheStokerLivesInTheDock(t *testing.T) {
	st := stokerState(runningNight(fixtureState().At))
	phone := sized(t, 44, 31, st)
	out := screen(phone)
	if !strings.Contains(out, "STOKER") || !strings.Contains(out, "night → ") || strings.Contains(out, "USAGE") ||
		!strings.Contains(out, "176k → 9k") || !strings.Contains(out, "· watching 3 · 1 compacted") {
		t.Fatalf("44x31: the stoker does not take the dock:\n%s", out)
	}
	if strings.Contains(out, "STOKED") {
		t.Fatalf("44x31: the card is on top of Sessions:\n%s", out)
	}
	switched, _ := key(phone, "u")
	if out := screen(switched); !strings.Contains(out, "USAGE") || strings.Contains(out, "STOKER") {
		t.Fatalf("44x31: u does not switch away from the stoker:\n%s", out)
	}
	y := lineOf(t, phone, "STOKER")
	card, _ := tap(phone, 3, y+1)
	if cm := card.(Model); cm.tab != tabMachine || !strings.Contains(screen(card), "RESUME: run the migration test") {
		t.Fatalf("44x31: a tap on the stoker does not open its card on Machine:\n%s", screen(card))
	}

	drawer := sized(t, 107, 101, st)
	out = screen(drawer)
	at := strings.Split(out, "\n")[lineOf(t, drawer, "USAGE")]
	if !strings.Contains(at, "STOKER") || strings.Contains(out, "MACHINE") {
		t.Fatalf("107x101: the stoker does not stand in the machine's place:\n%s", out)
	}
	sc := drawer.draw(107)
	card, _ = tap(drawer, sc.stx0+3, sc.dockAt+1)
	if cm := card.(Model); !cm.stokerOpen || cm.tab != tabSessions || !strings.Contains(screen(card), "RESUME: run the migration test") {
		t.Fatalf("107x101: a tap on the stoker does not open its card in the pane:\n%s", screen(card))
	}
	if closed, _ := key(card, "esc"); closed.(Model).stokerOpen {
		t.Fatal("esc does not close the card")
	}

	desk := sized(t, 192, 60, st)
	at = strings.Split(screen(desk), "\n")[lineOf(t, desk, "USAGE")]
	if !strings.Contains(at, "MACHINE") || !strings.Contains(at, "STOKER") {
		t.Fatalf("192x60: the desk does not show all three:\n%s", screen(desk))
	}

	off := sized(t, 192, 60, stokerState(proto.StokerView{}))
	if out := screen(off); !strings.Contains(out, "off · f starts it · n starts a night") {
		t.Fatalf("the stoker off says nothing to do:\n%s", out)
	}
}

// TestStokerNewsIsAToast: what the stoker did shows in the header.
func TestStokerNewsIsAToast(t *testing.T) {
	at := fixtureState().At
	on := runningNight(at)
	off := proto.StokerView{}
	one := on
	one.Run = on.Run[:1]
	ended := quietNight()
	rests := on
	rests.Rests = at
	for _, c := range []struct {
		was, next proto.StokerView
		want      string
	}{
		{off, on, "the stoker runs the night → "},
		{one, on, "the stoker: app-review skipped: session busy"},
		{on, ended, "the stoker ended · ready 7/7"},
		{on, rests, "the stoker rests"},
		{on, on, ""},
	} {
		m, _ := loadedWith(t, 100, stokerState(c.was))
		next, _ := m.Update(stateMsg(stokerState(c.next)))
		if got := next.(Model).toast; c.want == "" && got != "" || !strings.HasPrefix(got, c.want) {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}
