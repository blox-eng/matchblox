package service

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
)

// TestEveryKeyThatAnswersNeedsTheWaitSeen: at a permission prompt or a
// question, a digit picks, Esc turns it down, an arrow moves its cursor and
// Tab its field. None of them goes for a screen taken before the wait began,
// so a typed line never approves a prompt the person did not see (main's
// rule for the peek's answer, now for the chat's keys). ^C and Backspace
// answer nothing and go.
func TestEveryKeyThatAnswersNeedsTheWaitSeen(t *testing.T) {
	for _, st := range []string{queue.StatePermission, queue.StateQuestion} {
		for _, k := range []string{"Enter", "lit:1", "lit:yes", "Escape", "Up", "Down", "Tab", "BTab"} {
			s, ran := keysTest(t)
			since := time.Now()
			s.cur.Queue = []queue.Item{{Pane: "%1", State: st, Since: since}}
			a := key(k)
			a.Seen = since.Add(-time.Millisecond)
			if res := s.act(t.Context(), a); res.Err != "not sent: flint asks something new — look again" || len(*ran) != 0 {
				t.Errorf("%s %s: res %+v ran %q", st, k, res, *ran)
			}
		}
		for _, k := range []string{"C-c", "BSpace"} {
			s, ran := keysTest(t)
			since := time.Now()
			s.cur.Queue = []queue.Item{{Pane: "%1", State: st, Since: since}}
			a := key(k)
			a.Seen = since.Add(-time.Millisecond)
			if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
				t.Errorf("%s %s: res %+v ran %q", st, k, res, *ran)
			}
		}
	}
}

// TestTypingAfterATurnEndsStillGoes: a finished turn waits for a prompt;
// text typed into its input box answers nothing, only Enter sends it.
func TestTypingAfterATurnEndsStillGoes(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StateFinished, Since: since}}
	a := key("lit:1 more thing")
	a.Seen = since.Add(-time.Second)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// TestADigitOnAPermissionMenuOfTheHistory: the pane shows the approval
// menu, and a screen of the history does not: no answering key goes for it.
func TestADigitOnAPermissionMenuOfTheHistory(t *testing.T) {
	s, ran := keysTest(t)
	s.cur.Sessions[0].Permits = true
	at := s.upScreens.stamp("%1", s.now, true)
	a := key("lit:1")
	a.Seen = at
	if res := s.act(t.Context(), a); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
}

// TestAWaitIsSeenOnlyAfterItSettles: the hook says a wait began before the
// agent draws its menu, so a screen taken just after Since may still be the
// one before it. A key answers the wait only from a screen taken a settle
// after Since.
func TestAWaitIsSeenOnlyAfterItSettles(t *testing.T) {
	s, ran := keysTest(t)
	since := time.Now()
	s.cur.Queue = []queue.Item{{Pane: "%1", State: queue.StatePermission, Since: since}}
	a := key("Enter")
	a.Seen = since.Add(10 * time.Millisecond)
	if res := s.act(t.Context(), a); res.Err != "not sent: flint asks something new — look again" || len(*ran) != 0 {
		t.Fatalf("just after Since: res %+v ran %q", res, *ran)
	}
	a.Seen = since.Add(waitSettle)
	if res := s.act(t.Context(), a); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("after the settle: res %+v ran %q", res, *ran)
	}
}

// TestAnUnchangedScreenIsResentOnlyOnceTheWaitSettled: a still screen gets
// a new stamp for a wait only once the settle is over, so an Enter on a
// resend of the screen from before the menu never passes.
func TestAnUnchangedScreenIsResentOnlyOnceTheWaitSettled(t *testing.T) {
	s, _, ctx := watchTest(t, map[string]string{"%1": idleScreen})
	c := connectAs(t, ctx, s, proto.ClientConsole)
	r := read(c)
	if res := r.act(t, "w", proto.WatchPrefix+"%1"); res.Err != "" {
		t.Fatal(res.Err)
	}
	first := r.screenThat(t, func(proto.Screen) bool { return true })
	since := first.At.Add(time.Millisecond)
	s.mu.Lock()
	s.cur.Queue = []queue.Item{{Pane: "%1", Name: "%1", State: queue.StatePermission, Since: since}}
	s.mu.Unlock()
	time.Sleep(waitSettle / 2)
	if n := len(r.got()); n != 1 {
		t.Fatalf("the still screen came again before the wait settled: %+v", r.got())
	}
	again := r.screenThat(t, func(sc proto.Screen) bool { return sc.At.After(first.At) })
	if again.At.Before(since.Add(waitSettle)) {
		t.Fatalf("the resend is stamped %v, before %v", again.At, since.Add(waitSettle))
	}
	if res := s.act(ctx, proto.Act{Console: true, RecID: proto.KeysPrefix + "%1", Text: "Enter", PID: 100, Seen: again.At}); res.Err != "" {
		t.Fatalf("enter on the settled screen: %+v", res)
	}
}

// TestNoEnterBeforeTheConsoleSawThePane: an Enter from a console that has
// drawn no screen of the pane yet (Seen zero) submits nothing it has not
// shown, waiting or not.
func TestNoEnterBeforeTheConsoleSawThePane(t *testing.T) {
	s, ran := keysTest(t)
	s.cur.Queue = nil // flint waits for nothing
	if res := s.act(t.Context(), key("Enter")); res.Err != "not sent: the console has not shown flint's screen yet — look again" || len(*ran) != 0 {
		t.Fatalf("res %+v ran %q", res, *ran)
	}
	if res := s.act(t.Context(), key("Down")); res.Err != "" || len(*ran) != 1 {
		t.Fatalf("a key that sends nothing: res %+v ran %q", res, *ran)
	}
}
