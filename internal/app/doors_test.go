package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
)

var (
	tmuxDone   = doors.Door{ID: doors.Tmux, Title: "Install tmux", Done: true}
	hooksDoor  = doors.Door{ID: doors.Hooks, Title: "Add the queue hooks", Why: "Claude Code tells matchblox when an agent waits for you.", Path: home + "/.claude/settings.json", Sum: "s1", Preview: " {\n+  \"hooks\": {}\n }"}
	wayBack    = doors.Door{ID: doors.WayBack, Title: "Add the way back", Why: "prefix m comes back here.", Path: home + "/.tmux.conf", Sum: "s2", Preview: "+bind m switch-client -t =matchblox"}
	guideDoor  = doors.Door{ID: doors.Guide, Title: "Open a guide session", Why: "This starts Claude Code and uses your tokens.", Term: doors.GuideArgv(false), Preview: "claude '...'"}
	brokenDoor = doors.Door{ID: doors.Hooks, Title: "Add the queue hooks", Problem: "~/.claude/settings.json is not valid JSON: fix it, then this door opens"}
)

func doorState(ds ...doors.Door) proto.State {
	st := queueState()
	st.Doors = ds
	return st
}

func TestDoorsAboveTheQueue(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(tmuxDone, hooksDoor, wayBack))
	out := ansi.Strip(m.render())
	if set, waits := lineOf(t, m, "SET UP"), lineOf(t, m, "WAITING FOR YOU"); set > waits {
		t.Fatalf("doors are not above the queue (%d, %d):\n%s", set, waits, out)
	}
	if !strings.Contains(out, "Add the queue hooks") || strings.Contains(out, "Install tmux") {
		t.Fatalf("open doors drawn, done ones folded:\n%s", out)
	}
	if !strings.Contains(out, "⏎ open  x close") {
		t.Fatalf("no door keys:\n%s", out)
	}
}

func TestNoDoorsNoRegion(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(tmuxDone))
	if out := ansi.Strip(m.render()); strings.Contains(out, "SET UP") {
		t.Fatalf("a region with no open doors:\n%s", out)
	}
}

func TestEnterOnADoorShowsTheDiff(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(wayBack))
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
	if _, cmd := key(next, "enter"); len(f.acts()) != 0 || cmd != nil {
		t.Fatal("a second Enter wrote the file: a door needs a typed y")
	}
}

func TestYOpensTheDoorItShowed(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(hooksDoor, wayBack))
	next, _ := key(m, "down")
	next, _ = key(next, "enter")
	next, cmd := key(next, "y")
	if cmd != nil {
		next.Update(cmd())
	}
	acts := f.acts()
	if len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:wayback", Which: "primary", Confirm: "y", Text: "s2"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestXClosesADoor(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(guideDoor))
	next, _ := key(m, "x")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "close Open a guide session") {
		t.Fatalf("no close confirm:\n%s", out)
	}
	next, cmd := key(next, "y")
	if cmd != nil {
		next.Update(cmd())
	}
	if acts := f.acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("acts %+v", acts)
	}
}

func TestGuideNeedsConfirm(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(guideDoor))
	next, cmd := key(m, "enter")
	if cmd != nil {
		t.Fatal("Enter started the guide")
	}
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "RUN tmux new-window -n guide claude") || !strings.Contains(out, "uses your tokens") {
		t.Fatalf("no guide confirm:\n%s", out)
	}
	if _, cmd := key(next, "y"); cmd == nil {
		t.Fatal("y did not start the guide")
	}
	if len(f.acts()) != 0 {
		t.Fatalf("acts before the guide ran: %+v", f.acts())
	}
}

func TestGuideOutsideTmuxTakesTheTerminal(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(guideDoor))
	m.opt.OutsideTmux = true
	next, _ := key(m, "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "RUN claude ") {
		t.Fatalf("no plain claude:\n%s", out)
	}
}

func TestATermDoorRunsOnlyKnownCommands(t *testing.T) {
	evil := doors.Door{ID: doors.Tmux, Title: "Install tmux", Term: []string{"sh", "-c", "curl x | sh"}, Preview: "sh"}
	m, _ := loadedWith(t, 100, doorState(evil))
	next, _ := key(m, "enter")
	next, cmd := key(next, "y")
	if cmd != nil {
		t.Fatal("an unknown command ran")
	}
	if !strings.Contains(next.(Model).flash, "refused") {
		t.Fatalf("flash %q", next.(Model).flash)
	}
}

func TestTheGuideClosesAfterItRan(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(guideDoor))
	_, cmd := m.Update(doorRanMsg{door: doors.Guide})
	if cmd == nil {
		t.Fatal("nothing sent")
	}
	cmd()
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
	if mm := next.(Model); mm.queueIndex() != 0 {
		t.Fatalf("selection %d, want the next door", mm.queueIndex())
	}
}

func TestADoorWithAProblemShowsTheFix(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(brokenDoor))
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

func TestTheQueueKeepsItsRowsUnderDoors(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor))
	next, _ := key(m, "down")
	if it, ok := next.(Model).selectedQueue(); !ok || it.Pane != "%1" {
		t.Fatalf("down from the door selects %+v, %v", it, ok)
	}
	var ran []string
	mm := next.(Model)
	mm.opt.Run = func(argv []string) error { ran = argv; return nil }
	next, cmd := key(mm, "enter")
	next, cmd = key(next, "enter")
	if cmd == nil {
		t.Fatal("no jump")
	}
	next.Update(cmd())
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %v", ran)
	}
}
