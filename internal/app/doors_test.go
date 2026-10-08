package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

// doorState is a first run: doors, nobody waits yet.
func doorState(ds ...doors.Door) proto.State {
	st := queueState()
	st.Queue, st.Doors = nil, ds
	return st
}

func doorsAndQueue(ds ...doors.Door) proto.State {
	st := queueState()
	st.Doors = ds
	return st
}

func TestDoorsAboveTheQueue(t *testing.T) {
	m, _ := loadedWith(t, 100, doorsAndQueue(tmuxDone, hooksDoor, wayBack))
	out := ansi.Strip(m.render())
	if set, waits := lineOf(t, m, "SET UP"), lineOf(t, m, "WAITING FOR YOU"); set > waits {
		t.Fatalf("doors are not above the queue (%d, %d):\n%s", set, waits, out)
	}
	if !strings.Contains(out, "Add the queue hooks") || strings.Contains(out, "Install tmux") {
		t.Fatalf("open doors drawn, done ones folded:\n%s", out)
	}
	if next, _ := key(m, "up"); !strings.Contains(ansi.Strip(next.(Model).render()), "⏎ open  x close") {
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
	if len(acts) != 1 || !reflect.DeepEqual(acts[0], proto.Act{RecID: "door:wayback", Which: "primary", Confirm: "y", Text: "s2"}) {
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
	if acts := f.acts(); len(acts) != 1 || !reflect.DeepEqual(acts[0], proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
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
	m, _ := loadedWith(t, 100, doorState(guideDoor))
	m.opt.OutsideTmux = true
	next, _ := key(m, "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "in this terminal") || !strings.Contains(out, " claude 'Help me") {
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
	if acts := f.acts(); len(acts) != 1 || !reflect.DeepEqual(acts[0], proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
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

func TestTheQueueComesFirstWhenSomeoneWaits(t *testing.T) {
	m, _ := loadedWith(t, 100, doorsAndQueue(hooksDoor))
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	if it, ok := m.selectedQueue(); !ok || it.Pane != "%1" {
		t.Fatalf("selected %+v, %v: an agent that waits comes before a door", it, ok)
	}
	next, _ := key(m, "enter")
	next, cmd := key(next, "enter")
	if cmd == nil {
		t.Fatal("no jump")
	}
	next.Update(cmd())
	if strings.Join(ran, " ") != "tmux switch-client -t %1" {
		t.Fatalf("ran %v", ran)
	}
	if up, _ := key(next, "up"); up.(Model).queueIndex() != 0 {
		t.Fatal("up from the queue does not reach the door")
	}
}

func TestAFoldedDoorPassesToTheNext(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor, wayBack, guideDoor))
	next, _ := key(m, "down")
	done := wayBack
	done.Done = true
	next, _ = next.Update(stateMsg(doorState(hooksDoor, done, guideDoor)))
	if d, ok := next.(Model).selectedDoor(); !ok || d.ID != doors.Guide {
		t.Fatalf("selected %+v, want the guide after the way back folded", d)
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
	next, _ = key(next, "enter")
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
	next, cmd := key(next, "y")
	if cmd != nil {
		next.Update(cmd())
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
	m, _ := loadedWith(t, 100, doorState(hooksDoor, guideDoor))
	long := guideDoor
	long.Why = "Claude Code walks you through matchblox in a new window. This starts Claude Code and uses your tokens."
	next, _ := m.Update(stateMsg(doorState(hooksDoor, long)))
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "uses your tokens.") {
		t.Fatalf("the why is cut:\n%s", out)
	}
}

func TestOneBlankLineUnderTheDoors(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor))
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	for i := lineOf(t, m, "SET UP"); i < lineOf(t, m, "WAITING FOR YOU"); i++ {
		if strings.TrimSpace(lines[i]) == "" && strings.TrimSpace(lines[i+1]) == "" {
			t.Fatalf("two blank lines at %d:\n%s", i, strings.Join(lines, "\n"))
		}
	}
}

func TestThePreviewShortensHome(t *testing.T) {
	d := hooksDoor
	d.Preview = `+      {"command": "` + home + `/go/bin/matchblox hook Stop"}`
	m, _ := loadedWith(t, 100, doorState(d))
	next, _ := key(m, "enter")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, `"~/go/bin/matchblox hook Stop"`) {
		t.Fatalf("no ~ in the preview:\n%s", out)
	}
}

func TestTheResultShortensHome(t *testing.T) {
	m, _ := loadedWith(t, 100, doorState(hooksDoor))
	next, _ := m.Update(resultMsg(proto.Result{Ran: [][]string{{"write", home + "/.claude/settings.json"}}}))
	if got := next.(Model).flash; got != "ran: write ~/.claude/settings.json" {
		t.Fatalf("flash %q", got)
	}
}

func TestAPhoneShowsOnlyTheSelectedWhy(t *testing.T) {
	m, _ := loadedWith(t, 40, doorState(hooksDoor, wayBack, guideDoor))
	out := ansi.Strip(m.render())
	if !strings.Contains(out, "Claude Code tells") || strings.Contains(out, "prefix m comes back") || strings.Contains(out, "uses your tokens") {
		t.Fatalf("a phone shows each why:\n%s", out)
	}
	next, _ := key(m, "down")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "prefix m comes back") || strings.Contains(out, "Claude Code tells") {
		t.Fatalf("the why does not follow the selection:\n%s", out)
	}
}

var hostsDoor = doors.Door{ID: doors.Hosts, Title: "Pick your hosts", Why: "matchblox <host> opens the console of a host.",
	Path: home + "/.config/matchblox/config.toml", Sum: "s3", Choices: []string{"ws-1", "ws-2"}}

func TestHostsDoorPicksWithSpace(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(hostsDoor))
	next, _ := key(m, "enter")
	out := ansi.Strip(next.(Model).render())
	for _, want := range []string{"+[remote]", `"ws-1",`, `"ws-2",`, "space picks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `+  "ws-1"`) {
		t.Fatalf("a host is picked before the builder picks it:\n%s", out)
	}
	next, _ = key(next, "down")
	next, _ = key(next, " ")
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, `+  "ws-2",`) || strings.Contains(out, `+  "ws-1"`) {
		t.Fatalf("space must pick the host under the cursor:\n%s", out)
	}
	next, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("y sent nothing")
	}
	next.Update(cmd())
	want := proto.Act{RecID: "door:hosts", Which: "primary", Confirm: "y", Text: "s3", Picks: []string{"ws-2"}}
	if acts := f.acts(); len(acts) != 1 || !reflect.DeepEqual(acts[0], want) {
		t.Fatalf("acts %+v, want %+v", acts, want)
	}
}

func TestHostsDoorNeedsAPick(t *testing.T) {
	m, f := loadedWith(t, 100, doorState(hostsDoor))
	next, _ := key(m, "enter")
	next, cmd := key(next, "y")
	if cmd != nil {
		next.Update(cmd())
	}
	if len(f.acts()) != 0 {
		t.Fatal("y with no host picked wrote the file")
	}
	if !strings.Contains(next.(Model).flash, "space picks") || next.(Model).pending == nil {
		t.Fatalf("flash %q, pending %v: want the hint and the picker still open", next.(Model).flash, next.(Model).pending)
	}
}

// Hosts are picked on the machine the builder connects from, not on a host.
func TestRemoteHidesTheHostsDoor(t *testing.T) {
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Host: "ws-1"})
	next, _ := m.Update(stateMsg(doorState(hostsDoor, wayBack)))
	if ds := next.(Model).doors; len(ds) != 1 || ds[0].ID != doors.WayBack {
		t.Fatalf("doors %+v", ds)
	}
}
