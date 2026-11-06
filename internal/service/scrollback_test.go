package service

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// scrollTest watches %1, whose agent does not read the mouse and has hist
// lines of history. A capture back shows "back <n>".
func scrollTest(t *testing.T, hist int) (*reader, *atomic.Int32) {
	t.Helper()
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	s.PaneMouse = func(string) (int, int, bool) { return 0, 0, false }
	s.PaneHistory = func(string) (int, int, bool) { return hist, 24, true }
	var backs atomic.Int32
	s.CaptureBack = func(pane string, back, rows int) (string, bool) {
		backs.Add(1)
		return fmt.Sprintf("back %d of %d", back, rows), true
	}
	r := read(connectAs(t, ctx, s, proto.ClientConsole))
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	waitScreen(t, r, func(sc proto.Screen) bool { return sc.Text == idleScreen && sc.Back == 0 })
	return r, &backs
}

// waitScreen waits for a screen that ok accepts.
func waitScreen(t *testing.T, r *reader, ok func(proto.Screen) bool) proto.Screen {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		for _, sc := range r.got() {
			if ok(sc) {
				return sc
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no such screen in %+v", r.got())
	return proto.Screen{}
}

func TestAWheelScrollsBackAPaneThatIgnoresTheMouse(t *testing.T) {
	r, _ := scrollTest(t, 100)
	if res := r.actText(t, "u1", proto.KeysPrefix+"%1", proto.WheelUp); res.Err != "" {
		t.Fatal(res.Err)
	}
	waitScreen(t, r, func(sc proto.Screen) bool { return sc.Back == 3 && sc.Text == "back 3 of 24" })
	r.actText(t, "u2", proto.KeysPrefix+"%1", proto.WheelUp)
	waitScreen(t, r, func(sc proto.Screen) bool { return sc.Back == 6 })
	r.actText(t, "d1", proto.KeysPrefix+"%1", proto.WheelDown)
	waitScreen(t, r, func(sc proto.Screen) bool { return sc.Back == 3 && sc.Text == "back 3 of 24" })
}

func TestScrollBackStopsAtTheTopOfTheHistory(t *testing.T) {
	r, _ := scrollTest(t, 4)
	r.actText(t, "u1", proto.KeysPrefix+"%1", proto.WheelUp)
	r.actText(t, "u2", proto.KeysPrefix+"%1", proto.WheelUp)
	sc := waitScreen(t, r, func(sc proto.Screen) bool { return sc.Back == 4 })
	if sc.Text != "back 4 of 24" {
		t.Fatalf("screen %+v", sc)
	}
}

func TestAWheelDownOnTheLiveScreenDoesNothing(t *testing.T) {
	r, backs := scrollTest(t, 100)
	if res := r.actText(t, "d", proto.KeysPrefix+"%1", proto.WheelDown); res.Err != "" {
		t.Fatal(res.Err)
	}
	time.Sleep(3 * watchEvery)
	if n := backs.Load(); n != 0 {
		t.Fatalf("%d captures back on the live screen", n)
	}
}

func TestTheFirstKeyAfterScrollingOnlyComesBackToLive(t *testing.T) {
	r, _ := scrollTest(t, 100)
	r.actText(t, "u", proto.KeysPrefix+"%1", proto.WheelUp)
	waitScreen(t, r, func(sc proto.Screen) bool { return sc.Back == 3 })
	res := r.actText(t, "k", proto.KeysPrefix+"%1", "Enter")
	if !strings.Contains(res.Err, "back to the live screen") {
		t.Fatalf("Enter while scrolled back: %+v", res)
	}
	deadline := time.Now().Add(waitLimit)
	for {
		g := r.got()
		if last := g[len(g)-1]; last.Back == 0 && last.Text == idleScreen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no live screen after the key: %+v", g)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAPaneThatReadsTheMouseStillGetsTheWheel(t *testing.T) {
	var backs atomic.Int32
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	s.PaneMouse = func(string) (int, int, bool) { return 80, 24, true }
	s.PaneHistory = func(string) (int, int, bool) { return 100, 24, true }
	s.CaptureBack = func(string, int, int) (string, bool) { backs.Add(1); return "", true }
	r2 := read(connectAs(t, ctx, s, proto.ClientConsole))
	r2.act(t, "w", proto.WatchPrefix+"%1")
	res := r2.actText(t, "u", proto.KeysPrefix+"%1", proto.WheelUp)
	// The act goes on to the agent, which needs the pid the console saw.
	if !strings.Contains(res.Err, "different agent") {
		t.Fatalf("wheel to a mouse agent: %+v", res)
	}
	if n := backs.Load(); n != 0 {
		t.Fatalf("%d captures back for a mouse agent", n)
	}
}
