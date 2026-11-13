package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

var (
	tmuxDone   = doors.Door{ID: doors.Tmux, Title: "Install tmux", Done: true}
	hooksDoor  = doors.Door{ID: doors.Hooks, Title: "Add the queue hooks", Why: "Claude Code tells matchblox when an agent needs you.", Path: home + "/.claude/settings.json", Sum: "s1", Preview: " {\n+  \"hooks\": {}\n }"}
	wayBack    = doors.Door{ID: doors.WayBack, Title: "Add the way back", Why: "prefix m comes back here.", Path: home + "/.tmux.conf", Sum: "s2", Preview: "+bind m switch-client -t =matchblox"}
	guideDoor  = doors.Door{ID: doors.Guide, Title: "Open a guide session", Why: "This starts Claude Code and uses your tokens.", Term: doors.GuideArgv(false), Preview: "claude '...'"}
	brokenDoor = doors.Door{ID: doors.Hooks, Title: "Add the queue hooks", Problem: "~/.claude/settings.json is not valid JSON: fix it, then this door opens"}
)

// doorState is a first run: doors, nobody waits yet.
func doorState(ds ...doors.Door) proto.State {
	st := queueState()
	st.Queue, st.Doors = nil, ds
	return st
}

// onDoors chooses the board's doors line on its first door, as ↑ from the
// first card does: the board chooses a card first, where Sessions chose
// the first door while nobody waited.
func onDoors(m tea.Model) tea.Model {
	mm := m.(Model)
	lines := mm.aboveLines()
	if i := slices.Index(lines, ""); i >= 0 {
		mm.boardAbove, mm.boardDoor = len(lines)-i, 0
	}
	return mm
}

// onRec chooses the recommendation id under the board's columns, as ↓
// from the last card does.
func onRec(m Model, id string) Model {
	if i := slices.Index(m.belowLines(), id); i >= 0 {
		m.boardBelow = i + 1
	}
	return m
}

// flashed is what the console said of a key: its flash, or on the board
// the notice in the strip's row.
func flashed(m tea.Model) string { return cmp.Or(m.(Model).flash, m.(Model).noticeText) }

// drainActs runs cmd, each of a batch, and counts what it sent.
func drainActs(f *fakeConn, cmd tea.Cmd) int {
	drain(cmd)
	return len(f.acts())
}

// doorsBoard is loadedWith, the doors line chosen (onDoors).
func doorsBoard(t *testing.T, width int, st proto.State) (Model, *fakeConn) {
	t.Helper()
	m, f := loadedWith(t, width, st)
	return onDoors(m).(Model), f
}

func doorsAndQueue(ds ...doors.Door) proto.State {
	st := queueState()
	st.Doors = ds
	return st
}

func TestDoorsAboveTheSessions(t *testing.T) {
	m, _ := doorsBoard(t, 100, doorsAndQueue(tmuxDone, hooksDoor, wayBack))
	out := ansi.Strip(m.render())
	if set, waits := lineOf(t, m, "SET UP"), lineOf(t, m, "NEEDS YOU"); set > waits {
		t.Fatalf("doors are not above the sessions (%d, %d):\n%s", set, waits, out)
	}
	if !strings.Contains(out, "Add the queue hooks") || strings.Contains(out, "Install tmux") {
		t.Fatalf("open doors drawn, done ones folded:\n%s", out)
	}
	var next tea.Model = m
	for range len(m.recs) + 1 {
		next, _ = key(next, "up")
	}
	if !strings.Contains(actionLine(next), "⏎ set up   x close") { // the board's doors line
		t.Fatalf("no door keys:\n%s", screen(next))
	}
}

func TestNoDoorsNoRegion(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(tmuxDone))
	if out := ansi.Strip(m.render()); strings.Contains(out, "SET UP") {
		t.Fatalf("a region with no open doors:\n%s", out)
	}
}

func TestEnterOnADoorShowsTheDiff(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(wayBack))
	next, _ := key(m, "enter")
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"+bind m switch-client -t =matchblox", "RUN Add the way back: write ~/.tmux.conf", "y run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q:\n%s", want, out)
		}
	}
	if len(f.acts()) != 0 {
		t.Fatalf("Enter sent %+v", f.acts())
	}
	if _, cmd := key(next, "enter"); drainActs(f, cmd) != 0 { // on the board the cancel is a notice's tick
		t.Fatal("a second Enter wrote the file: a door needs a typed y")
	}
}

func TestYOpensTheDoorItShowed(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(hooksDoor, wayBack))
	next, _ := key(m, "right")
	next, _ = key(next, "enter")
	_, cmd := key(next, "y")
	if cmd != nil {
		drain(cmd)
	}
	acts := f.acts()
	if len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:wayback", Which: "primary", Confirm: "y", Text: "s2"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestXClosesADoor(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(guideDoor))
	next, _ := key(m, "x")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "close Open a guide session") {
		t.Fatalf("no close confirm:\n%s", out)
	}
	_, cmd := key(next, "y")
	if cmd != nil {
		drain(cmd)
	}
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestGuideNeedsConfirm(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(guideDoor))
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, cmd := key(m, "enter")
	if drainActs(f, cmd) != 0 || len(ran) != 0 || next.(Model).pending == nil {
		t.Fatal("Enter started the guide")
	}
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"RUN Open a guide session: in a new tmux window", "y run", "uses your tokens",
		"tmux new-window -n guide claude 'Help me set up matchblox."} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q:\n%s", want, out)
		}
	}
	if _, cmd := key(next, "y"); cmd == nil {
		t.Fatal("y did not start the guide")
	}
	if len(f.acts()) != 0 {
		t.Fatalf("acts before the guide ran: %+v", f.acts())
	}
}

func TestGuideOutsideTmuxTakesTheTerminal(t *testing.T) {
	m, _ := doorsBoard(t, 100, doorState(guideDoor))
	m.opt.OutsideTmux = true
	next, _ := key(m, "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "in this terminal") || !strings.Contains(out, " claude 'Help me") {
		t.Fatalf("no plain claude:\n%s", out)
	}
}

func TestATermDoorRunsOnlyKnownCommands(t *testing.T) {
	evil := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "curl x | sh"}, Preview: "sh"}
	m, f := doorsBoard(t, 100, doorState(evil))
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ := key(m, "enter")
	next, cmd := key(next, "y")
	if drainActs(f, cmd) != 0 || len(ran) != 0 {
		t.Fatal("an unknown command ran")
	}
	if said := cmp.Or(next.(Model).flash, next.(Model).noticeText); !strings.Contains(said, "refused") {
		t.Fatalf("flash %q", said)
	}
}

func TestTheGuideClosesAfterItRan(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(guideDoor))
	_, cmd := m.Update(doorRanMsg{door: doors.Guide})
	if cmd == nil {
		t.Fatal("nothing sent")
	}
	drain(cmd)
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestDoneDoorFolds(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor, wayBack))
	done := hooksDoor
	done.Done = true
	next, _ := m.Update(stateMsg(doorState(done, wayBack)))
	out := ansi.Strip(next.(Model).render())
	if strings.Contains(out, "Add the queue hooks") || !strings.Contains(out, "Add the way back") {
		t.Fatalf("hooks did not fold:\n%s", out)
	}
	if mm := next.(Model); mm.boardDoors()[mm.doorIndex(mm.boardDoors())].ID != doors.WayBack { // the board's doors line
		t.Fatalf("the doors line names %+v, want the next door", mm.boardDoors()[mm.doorIndex(mm.boardDoors())])
	}
}

func TestADoorWithAProblemShowsTheFix(t *testing.T) {
	m, _ := doorsBoard(t, 100, doorState(brokenDoor))
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "is not valid JSON: fix it") {
		t.Fatalf("no fix:\n%s", out)
	}
	if next, _ := key(m, "enter"); next.(Model).pending != nil {
		t.Fatal("Enter on a door that cannot open")
	}
}

func TestDoorsOnAPhone(t *testing.T) {
	m, _ := loadedWith(t, 40, doorState(hooksDoor, guideDoor))
	next, _ := key(m, "enter")
	for i, l := range strings.Split(next.(Model).render(), "\n") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestTheQueueComesFirstWhenSomeoneWaits(t *testing.T) {
	m, _ := loadedWith(t, 100, doorsAndQueue(hooksDoor))
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	if it, ok := m.selectedSession(); !ok || it.Pane != "%1" {
		t.Fatalf("selected %+v, %v: an agent that waits comes before a door", it, ok)
	}
	next, cmd := key(m, "ctrl+o")
	if cmd == nil {
		t.Fatal("no jump")
	}
	fed(next, cmd)
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %v", ran)
	}
	up := next
	for range len(next.(Model).recs) + 1 {
		up, _ = key(up, "up")
	}
	if up.(Model).sessIndex() != 0 {
		t.Fatal("up from the sessions, over the recommendations, does not reach the door")
	}
}

func longDoor() doors.Door {
	d := hooksDoor
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	d.Preview = strings.TrimSuffix(b.String(), "\n")
	return d
}

func TestALongDiffIsReadToTheEndBeforeY(t *testing.T) {
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	next, _ = next.Update(stateMsg(doorState(longDoor(), wayBack, guideDoor)))
	mm := next.(Model)
	mm.viewPrefs.CopyCheckedDesk = true
	next, _ = key(onDoors(mm), "enter")
	next, _ = key(next, "y")
	if len(f.acts()) != 0 || next.(Model).pending == nil {
		t.Fatal("y wrote a diff the person did not see to its end")
	}
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "↓ the rest of the diff") || strings.Contains(out, "+line 39") {
		t.Fatalf("no hint, or the end shows already:\n%s", out)
	}
	for range 40 {
		next, _ = key(next, "down")
	}
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "+line 39") || !strings.Contains(out, "y run") {
		t.Fatalf("scrolled to the end, no y:\n%s", out)
	}
	_, cmd := key(next, "y")
	if cmd != nil {
		drain(cmd)
	}
	if len(f.acts()) != 1 {
		t.Fatalf("acts %+v", f.acts())
	}
}

func TestTildeShortensOnlyAWholePath(t *testing.T) {
	for in, want := range map[string]string{
		home + "/x":             "~/x",
		home:                    "~",
		home + "ice/x":          home + "ice/x",
		`"` + home + `/go/bin"`: `"~/go/bin"`,
	} {
		if got := tilde(in); got != want {
			t.Errorf("tilde(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheWhyIsReadInFull(t *testing.T) {
	m, _ := doorsBoard(t, 100, doorState(hooksDoor, guideDoor))
	long := guideDoor
	long.Why = "Claude Code walks you through matchblox in a new window. This starts Claude Code and uses your tokens."
	next, _ := m.Update(stateMsg(doorState(hooksDoor, long)))
	next, _ = key(next, "right")
	if out := strings.Join(strings.Fields(ansi.Strip(next.(Model).render())), " "); !strings.Contains(out, "uses your tokens.") {
		t.Fatalf("the why is cut:\n%s", out)
	}
}

func TestThePreviewShortensHome(t *testing.T) {
	d := hooksDoor
	d.Preview = `+      {"command": "` + home + `/go/bin/matchblox hook Stop"}`
	m, _ := doorsBoard(t, 100, doorState(d))
	next, _ := key(m, "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, `"~/go/bin/matchblox hook Stop"`) {
		t.Fatalf("no ~ in the preview:\n%s", out)
	}
}

func TestTheResultShortensHome(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor))
	next, _ := m.Update(resultMsg(proto.Result{Ran: [][]string{{"write", home + "/.claude/settings.json"}}}))
	if got := saidNow(next.(Model)); got != "ran: write ~/.claude/settings.json" {
		t.Fatalf("toast %q", got)
	}
}

func TestAPhoneShowsOnlyTheSelectedWhy(t *testing.T) {
	m, _ := doorsBoard(t, 40, doorState(hooksDoor, wayBack, guideDoor))
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "Claude Code tells") || strings.Contains(out, "prefix m comes back") || strings.Contains(out, "uses your tokens") {
		t.Fatalf("a phone shows each why:\n%s", out)
	}
	next, _ := key(m, "right")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "prefix m comes back") || strings.Contains(out, "Claude Code tells") {
		t.Fatalf("the why does not follow the selection:\n%s", out)
	}
}

var wayBackMouse = func() doors.Door {
	d := wayBack
	d.Also = &doors.Also{Label: "also turn the mouse on: a tap on ◂ matchblox works from a phone", Preview: "+bind m switch-client -t =matchblox\n+set -g mouse on"}
	return d
}()

func TestTheWayBackMouseIsOffUntilM(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(wayBackMouse))
	next, _ := key(m, "enter")
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "[ ] m also turn the mouse on") || strings.Contains(out, "+set -g mouse on") {
		t.Fatalf("the choice is not shown off:\n%s", out)
	}
	next, _ = key(next, "m")
	out = ansi.Strip(next.(Model).render())
	for _, want := range []string{"[x] m also turn the mouse on", "+set -g mouse on", "write ~/.tmux.conf, mouse on"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q after m:\n%s", want, out)
		}
	}
	_, cmd := key(next, "y")
	if cmd != nil {
		drain(cmd)
	}
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:wayback", Which: "also", Confirm: "y", Text: "s2"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestMTwiceLeavesTheMouseOff(t *testing.T) {
	m, f := doorsBoard(t, 100, doorState(wayBackMouse))
	next, _ := key(m, "enter")
	next, _ = key(next, "m")
	next, _ = key(next, "m")
	_, cmd := key(next, "y")
	if cmd != nil {
		drain(cmd)
	}
	if acts := f.acts(); len(acts) != 1 || acts[0].Which != "primary" {
		t.Fatalf("acts %+v", acts)
	}
}

func TestMWithoutAChoiceCancelsAsAnyKey(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(wayBack))
	next, _ := key(m, "enter")
	next, _ = key(next, "m")
	if next.(Model).pending != nil || len(f.acts()) != 0 {
		t.Fatalf("m on a door without a choice: pending %+v, acts %+v", next.(Model).pending, f.acts())
	}
}
