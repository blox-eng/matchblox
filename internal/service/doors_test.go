package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/setup"
)

func doorsTest(t *testing.T) (*Service, string) {
	t.Helper()
	s := newTest(t, time.Hour)
	home := t.TempDir()
	s.Setup = &setup.Env{GOOS: "linux", Home: home, Exe: "/bin/matchblox", StateDir: filepath.Join(home, "state"),
		Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if name == "claude" || name == "tmux" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Source: func(string) error { return nil }}
	return s, filepath.Join(home, ".claude", "settings.json")
}

func openDoors(st proto.State) []string {
	var out []string
	for _, d := range st.Doors {
		if d.Open() {
			out = append(out, d.ID)
		}
	}
	return out
}

func findDoor(t *testing.T, st proto.State, id string) doors.Door {
	t.Helper()
	for _, d := range st.Doors {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no door %s in %+v", id, st.Doors)
	return doors.Door{}
}

func TestDoorsInTheSnapshot(t *testing.T) {
	s, _ := doorsTest(t)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	if got := openDoors(snapshot(t, c)); !slices.Equal(got, []string{doors.Hooks, doors.Limits, doors.SafetyNet, doors.WayBack, doors.Pair, doors.KeepRunning, doors.Guide, doors.Night, doors.Ways}) {
		t.Fatalf("open doors = %v", got)
	}
}

func TestNoSetupNoDoors(t *testing.T) {
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	if st := snapshot(t, c); st.Doors != nil {
		t.Fatalf("doors without a Setup: %+v", st.Doors)
	}
}

func TestDoorActWritesAfterATypedY(t *testing.T) {
	s, path := doorsTest(t)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	d := findDoor(t, snapshot(t, c), doors.Hooks)
	if r := act(t, c, "1", proto.Act{RecID: "door:hooks", Which: "primary", Text: d.Sum}); r.Err == "" {
		t.Fatalf("no typed y: %+v", r)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("written without a typed y")
	}
	r := act(t, c, "2", proto.Act{RecID: "door:hooks", Which: "primary", Confirm: "y", Text: d.Sum})
	if r.Err != "" || len(r.Ran) != 1 {
		t.Fatalf("result %+v", r)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "/bin/matchblox hook Stop") {
		t.Fatalf("settings.json:\n%s", b)
	}
	// The door folds at once, not on the next sample an hour later. The
	// result arrives after the fold, and the snapshot it kicked may come
	// before the result (act skips it), so read the state the next
	// snapshots carry.
	var now proto.State
	s.mu.Lock()
	now.Doors = s.cur.Doors
	s.mu.Unlock()
	if !findDoor(t, now, doors.Hooks).Done {
		t.Fatal("the door did not fold at once")
	}
}

func TestDoorActRefusesAChangedFile(t *testing.T) {
	s, path := doorsTest(t)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	d := findDoor(t, snapshot(t, c), doors.Hooks)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := act(t, c, "1", proto.Act{RecID: "door:hooks", Which: "primary", Confirm: "y", Text: d.Sum})
	if !strings.Contains(r.Err, "changed after the preview") {
		t.Fatalf("result %+v", r)
	}
}

func TestDoorCloseAct(t *testing.T) {
	s, _ := doorsTest(t)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	snapshot(t, c)
	if r := act(t, c, "1", proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}); r.Err != "" {
		t.Fatalf("result %+v", r)
	}
	// The snapshot the close kicked may come before the result, and act
	// skips it: read the state the next snapshots carry.
	var now proto.State
	s.mu.Lock()
	now.Doors = s.cur.Doors
	s.mu.Unlock()
	if !findDoor(t, now, doors.Guide).Closed {
		t.Fatal("the door did not close at once")
	}
}

func TestATerminalDoorRunsInTheConsole(t *testing.T) {
	s, _ := doorsTest(t)
	ctx := run(t, s)
	c := connectAs(t, ctx, s, proto.ClientConsole)
	snapshot(t, c)
	if r := act(t, c, "1", proto.Act{RecID: "door:guide", Which: "primary", Confirm: "y"}); !strings.Contains(r.Err, "terminal") {
		t.Fatalf("result %+v", r)
	}
}

func TestAnOlderSampleNeverBringsAFoldedDoorBack(t *testing.T) {
	s, _ := doorsTest(t)
	gen := s.doorsGen()
	stale := s.doors() // a sample reads the doors…
	i := slices.IndexFunc(stale, func(d doors.Door) bool { return d.ID == doors.Hooks })
	folded := slices.Clone(stale)
	folded[i].Done = true
	s.mu.Lock() // …an act folds one meanwhile…
	s.cur.Doors, s.gen = folded, s.gen+1
	s.mu.Unlock()
	s.storeDoors(gen, stale) // …and the sample must not undo it
	if !s.State().Doors[i].Done {
		t.Fatal("the older sample showed the folded door again")
	}
	s.storeDoors(s.doorsGen(), stale)
	if s.State().Doors[i].Done {
		t.Fatal("a current sample was dropped")
	}
}

func TestWayBackActTurnsTheMouseOnOnlyWhenChosen(t *testing.T) {
	for _, which := range []string{"primary", "also"} {
		s, _ := doorsTest(t)
		ctx := run(t, s)
		c := connectAs(t, ctx, s, proto.ClientConsole)
		d := findDoor(t, snapshot(t, c), doors.WayBack)
		if d.Also == nil {
			t.Fatalf("no mouse choice: %+v", d)
		}
		if r := act(t, c, "1", proto.Act{RecID: "door:wayback", Which: which, Confirm: "y", Text: d.Sum}); r.Err != "" {
			t.Fatalf("%s: result %+v", which, r)
		}
		b, _ := os.ReadFile(d.Path)
		if on := strings.Contains(string(b), "set -g mouse on"); on != (which == "also") {
			t.Fatalf("%s: tmux.conf:\n%s", which, b)
		}
	}
}

// TestOnlyAPersonAtAConsoleOpensADoor: a door writes the person's files,
// and the update door replaces the binary. A command, a session the stoker
// drives and a session's pane with a console hello are each refused; a
// console opens both.
func TestOnlyAPersonAtAConsoleOpensADoor(t *testing.T) {
	s, ran := handsTest(t)
	setupS, path := doorsTest(t)
	s.Setup = setupS.Setup
	src := &fakeUpdates{latest: "v0.43.0"}
	s.Binary, s.UpdateMode, s.Update = "v0.42.0", config.UpdateOffer, src
	s.loadUpdate()
	s.checkUpdate(s.now())
	var hooks doors.Door
	for _, d := range s.doors() {
		if d.ID == doors.Hooks {
			hooks = d
		}
	}
	for _, id := range []struct{ rec, text string }{{"door:" + doors.Update, "v0.43.0"}, {"door:" + doors.Hooks, hooks.Sum}} {
		for _, c := range []struct {
			who  string
			a    proto.Act
			want string
		}{
			{"a command", proto.Act{RecID: id.rec, Which: "primary", Confirm: "y", Text: id.text}, "only you do this, at the console"},
			{"the stoker", fromStokerAct(id.rec, id.text), "the stoker opens no door: the doors are yours"},
			{"a session's console", proto.Act{RecID: id.rec, Which: "primary", Confirm: "y", Text: id.text, Console: true, From: "%1", FromPlaced: true}, "only you open or close a door, at a console"},
		} {
			c.a.Which = "primary"
			if r := s.act(t.Context(), c.a); r.Err != c.want {
				t.Errorf("%s, %s: %+v", id.rec, c.who, r)
			}
		}
	}
	if _, err := os.Stat(path); err == nil || len(src.installed()) != 0 || len(*ran) != 0 {
		t.Fatalf("refused acts wrote %v, installed %v, ran %q", err == nil, src.installed(), *ran)
	}
	for _, id := range []struct{ rec, text string }{{"door:" + doors.Update, "v0.43.0"}, {"door:" + doors.Hooks, hooks.Sum}} {
		if r := s.act(t.Context(), proto.Act{RecID: id.rec, Which: "primary", Confirm: "y", Text: id.text, Console: true}); r.Err != "" {
			t.Fatalf("%s from a console: %+v", id.rec, r)
		}
	}
	if _, err := os.Stat(path); err != nil || len(src.installed()) != 1 {
		t.Fatalf("a console's acts: written %v, installed %v", err, src.installed())
	}
}
