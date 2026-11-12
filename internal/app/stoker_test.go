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

func TestFKeyTurnsTheStokerOffOrOpensTheSheet(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(proto.StokerView{}))
	if next := sendKey(m, "f").(Model); next.sheet == nil || next.sheet.night {
		t.Fatalf("f with the stoker off: %+v", next.sheet)
	}
	m, g := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	sendKey(m, "f")
	if a, b := f.acts(), g.acts(); len(a) != 0 || len(b) != 1 || b[0].RecID != "stoker:off" {
		t.Fatalf("acts %+v, %+v", a, b)
	}
}

func TestNKeyOpensTheNightSheetOrEndsTheNight(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(proto.StokerView{Mode: proto.StokerMode{On: true}}))
	if next := sendKey(m, "n").(Model); next.sheet == nil || !next.sheet.night {
		t.Fatalf("n by day: %+v", next.sheet)
	}
	night := proto.StokerMode{On: true, Until: time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)}
	m, g := loadedWith(t, 100, stokerState(proto.StokerView{Mode: night}))
	sendKey(m, "n")
	if a, b := f.acts(), g.acts(); len(a) != 0 || len(b) != 1 || b[0].RecID != "stoker:off" {
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
		m := fromMachine(t, m0, "stoker") // the stoker's panel heads the screen (ruling 28)
		if top := strings.Split(screen(m), "\n")[3]; !strings.Contains(top, "STOKER") {
			t.Fatalf("width %d: the screen's top is %q, not the stoker's panel", w, top)
		}
		out := ansi.Strip(m.render())
		for _, want := range []string{"TODAY", "app-feature", "176k → 9k", "would pass 80%", "RESUME: run the migration test", "app-review", "skipped: session busy"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: no %q:\n%s", w, want, out)
			}
		}
		// The session keeps its match, as everywhere an agent shows.
		row := strings.Split(out, "\n")[lineOf(t, m, "TODAY")+1]
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

func TestOpeningTheTabFoldsStoked(t *testing.T) {
	m, f := loadedWith(t, 100, stokerState(stokedRun(fixtureState().At)))
	toStoker(m)
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "stoker:ack", Which: "primary"}) {
		t.Fatalf("acts %+v", acts)
	}
	seen := stokedRun(fixtureState().At)
	seen.Unseen = false
	m, _ = loadedWith(t, 100, stokerState(seen))
	if out := ansi.Strip(m.render()); strings.Contains(out, "TODAY") {
		t.Fatalf("a seen run still shows:\n%s", out)
	}
}

func TestNightDoorStartsTheNight(t *testing.T) {
	t.Skip("replaced by TestTheBoardReachesDoorsGhostAndRecommendations (Task 12 step 1)")
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

// toStoker opens the STOKER screen as c did before c copied on the board
// (Machine's row stoker opens it at Task 12 step 6b), and sends its ack.
func toStoker(m tea.Model) tea.Model {
	next, cmd := m.(Model).openStoker().seeStoker()
	drain(cmd)
	return next
}

// sendKey presses k and runs the command it returns, each of a batch. A
// command that waits longer than a step of the service, as a notice's
// tick does, gives up.
func sendKey(m tea.Model, k string) tea.Model {
	next, cmd := key(m, k)
	for _, msg := range msgsWithin(cmd, 250*time.Millisecond) {
		next.Update(msg)
	}
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

// The demo host's header drops the stoker badge; everyone else's keeps it.
func TestOnlyTheDemoHeaderDropsTheStokerBadge(t *testing.T) {
	m, _ := loadedWith(t, 44, stokerState(proto.StokerView{}))
	if h := ansi.Strip(m.head(44)); !strings.Contains(h, "stoker off") {
		t.Fatalf("header %q has no stoker badge", h)
	}
	m.opt.Demo = "This is a demo."
	if h := ansi.Strip(m.head(44)); strings.Contains(h, "stoker") {
		t.Fatalf("the demo header %q shows the stoker badge", h)
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

// TestTheStokerLivesInTheDock: while a run is in charge the stoker's
// panel (its state, what it watches, its last step) heads the STOKER
// screen that Machine's row stoker opens, on a phone and on a desk; off,
// the screen says how to start it (ruling 28). The board has no dock.
func TestTheStokerLivesInTheDock(t *testing.T) {
	st := stokerState(runningNight(fixtureState().At))
	for _, size := range [][2]int{{44, 31}, {192, 60}} {
		m := fromMachine(t, sized(t, size[0], size[1], st), "stoker")
		lines := strings.Split(screen(m), "\n")
		top := strings.Join(lines[3:3+len(m.stokerLines(size[0], false))], "\n")
		for _, want := range []string{"STOKER", "night → ", "watching 3 · 1 compacted", "app-review"} {
			if !strings.Contains(top, want) {
				t.Fatalf("%dx%d: the screen's top lacks %q:\n%s", size[0], size[1], want, screen(m))
			}
		}
		if strings.Contains(top, "TODAY") {
			t.Fatalf("%dx%d: the panel is the card:\n%s", size[0], size[1], top)
		}
		if board := sized(t, size[0], size[1], st); strings.Contains(screen(board), "watching 3") {
			t.Fatalf("%dx%d: the board draws the stoker's panel:\n%s", size[0], size[1], screen(board))
		}
	}
	off := fromMachine(t, sized(t, 192, 60, stokerState(proto.StokerView{})), "stoker")
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
		if got := saidNow(next.(Model)); c.want == "" && got != "" || !strings.HasPrefix(got, c.want) {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}
