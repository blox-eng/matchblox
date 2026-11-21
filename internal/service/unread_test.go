package service

import (
	"os"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// unread0 is when the hook says the session waits.
var unread0 = time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)

func flintAt(permits bool) sample.Session {
	return sample.Session{Agent: "claude", PID: 42, Pane: "%1", Name: "flint", SessionID: "s1", Status: "idle", Permits: permits}
}

// unreadTest is a service that sampled flint before its hook said it waits
// for kind.
func unreadTest(t *testing.T, kind string) *Service {
	t.Helper()
	s := newTest(t, time.Hour)
	s.publish(sample.Snapshot{At: unread0.Add(-time.Second), Sessions: []sample.Session{flintAt(false)}}, func(state.Doc) {})
	s.Hook(hooks.Event{Name: "Notification", Kind: kind, SessionID: "s1", Pane: "%1", At: unread0, Message: "Claude needs your permission to use Bash"})
	return s
}

func sampleFlint(s *Service, after time.Duration, sess sample.Session) bool {
	s.publish(sample.Snapshot{At: unread0.Add(after), Sessions: []sample.Session{sess}}, func(state.Doc) {})
	return unknownScreen(s)
}

func unknownScreen(s *Service) bool {
	for _, x := range s.State().Sessions {
		if x.Pane == "%1" {
			return x.UnknownScreen
		}
	}
	return false
}

func TestAnUnreadPermissionIsAnUnknownScreen(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	if sampleFlint(s, time.Second, flintAt(false)) {
		t.Fatal("unknown screen within the grace time")
	}
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen after the grace time and two samples")
	}
}

func TestOneSampleIsNotEnough(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	if sampleFlint(s, 5*time.Second, flintAt(false)) {
		t.Fatal("unknown screen from one sample")
	}
}

func TestAReadMenuIsNeverUnknown(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	for d := time.Second; d <= 10*time.Second; d += time.Second {
		if sampleFlint(s, d, flintAt(true)) {
			t.Fatalf("unknown screen at +%s on a menu that reads", d)
		}
	}
}

func TestAMenuThatReadsClearsIt(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	sampleFlint(s, time.Second, flintAt(false))
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen")
	}
	if sampleFlint(s, 4*time.Second, flintAt(true)) {
		t.Fatal("a menu that reads kept the unknown screen")
	}
}

func TestTheWaitEndingClearsIt(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	sampleFlint(s, time.Second, flintAt(false))
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen")
	}
	s.Hook(hooks.Event{Name: "PostToolUse", SessionID: "s1", Pane: "%1", At: unread0.Add(4 * time.Second)})
	if unknownScreen(s) {
		t.Fatal("the wait ended and the unknown screen stayed")
	}
}

// A question waits for typed text: no menu is on its screen to read.
func TestAQuestionIsNeverUnknown(t *testing.T) {
	s := unreadTest(t, hooks.KindIdlePrompt)
	for d := time.Second; d <= 10*time.Second; d += time.Second {
		if sampleFlint(s, d, flintAt(false)) {
			t.Fatalf("unknown screen at +%s on a question", d)
		}
	}
}

// The question tool's menu is read from its call, not from the screen.
func TestAnAskIsNeverUnknown(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	asking := flintAt(false)
	asking.Ask = &said.Ask{ID: "toolu_1"}
	for d := time.Second; d <= 10*time.Second; d += time.Second {
		if sampleFlint(s, d, asking) {
			t.Fatalf("unknown screen at +%s on an ask", d)
		}
	}
}

// An agent with no hooks reports its own permission wait: the spec keeps
// the note to hooked agents.
func TestAReportedPermissionIsNeverUnknown(t *testing.T) {
	s := newTest(t, time.Hour)
	opencode := sample.Session{Agent: "opencode", PID: 42, Pane: "%1", Name: "flint", SessionID: "o1", Status: sample.StatusWaiting,
		Reported: true, WaitingFor: "permission", StatusAt: unread0}
	for d := time.Second; d <= 10*time.Second; d += time.Second {
		if sampleFlint(s, d, opencode) {
			t.Fatalf("unknown screen at +%s on a reported wait", d)
		}
	}
}

// A capture that fails reads no menu, the same as a menu no rule knows:
// once set, the note stays until a menu reads or the wait ends.
func TestNoBlink(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	sampleFlint(s, time.Second, flintAt(false))
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen")
	}
	for d := 4 * time.Second; d <= 8*time.Second; d += time.Second {
		if !sampleFlint(s, d, flintAt(false)) {
			t.Fatalf("the unknown screen went at +%s with no menu read", d)
		}
	}
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s1", Pane: "%1", At: unread0.Add(9 * time.Second)})
	if !unknownScreen(s) {
		t.Fatal("a hook with no new sample cleared the unknown screen")
	}
}

func TestChooseRefusesAnUnknownScreen(t *testing.T) {
	b, err := os.ReadFile("../../testdata/menus/unknown-wording.txt")
	if err != nil {
		t.Fatal(err)
	}
	s, ran := chooseTest(t, string(b))
	res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", chooseScreen)))
	if res.Err != "not sent: flint-auth shows no menu — answer it in its pane" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// The agent rewords its numbered menu: menu.Read still reads the options,
// but no rule does, so no digit goes to the screen the row calls unknown.
func TestChooseRefusesAnUnknownScreenThatTheConsoleReads(t *testing.T) {
	b, err := os.ReadFile("../../testdata/menus/unknown-numbered.txt")
	if err != nil {
		t.Fatal(err)
	}
	screen := string(b)
	if menu.Read(screen) == nil {
		t.Fatal("menu.Read reads nothing")
	}
	s := unreadTest(t, hooks.KindPermission)
	sampleFlint(s, time.Second, flintAt(false))
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen")
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	s.Capture = func([]string) map[string]string { return map[string]string{"%1": screen} }
	x := flintAt(false)
	a := proto.Act{RecID: proto.ChoosePrefix + "%1", Text: pickOf("1", "Go ahead", screen), PID: x.PID, Start: x.Start, Confirm: "y", Console: true}
	if res := s.act(t.Context(), a); res.Err != "matchblox cannot read this screen: answer it in its pane" || len(ran) != 0 {
		t.Fatalf("res %+v ran %q", res, ran)
	}
}

// Claude Code reports busy while its confirmation menu waits: a busy
// session on a screen no rule reads is still an unknown screen, and no
// digit goes to it.
func TestABusyMenuNoRuleReadsIsAnUnknownScreen(t *testing.T) {
	b, err := os.ReadFile("../../testdata/menus/unknown-numbered.txt")
	if err != nil {
		t.Fatal(err)
	}
	screen := string(b)
	s := unreadTest(t, hooks.KindPermission)
	busy := flintAt(false)
	busy.Busy, busy.Status = true, "busy"
	sampleFlint(s, time.Second, busy)
	if !sampleFlint(s, 3*time.Second, busy) {
		t.Fatal("no unknown screen on a busy session at a menu no rule reads")
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	s.Capture = func([]string) map[string]string { return map[string]string{"%1": screen} }
	a := proto.Act{RecID: proto.ChoosePrefix + "%1", Text: pickOf("1", "Go ahead", screen), PID: busy.PID, Start: busy.Start, Confirm: "y", Console: true}
	if res := s.act(t.Context(), a); res.Err != unknownScreenRefusal || len(ran) != 0 {
		t.Fatalf("res %+v ran %q", res, ran)
	}
}

func TestKeysRefuseAnUnknownScreen(t *testing.T) {
	s := unreadTest(t, hooks.KindPermission)
	sampleFlint(s, time.Second, flintAt(false))
	if !sampleFlint(s, 3*time.Second, flintAt(false)) {
		t.Fatal("no unknown screen")
	}
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	for _, text := range []string{proto.LitPrefix + "1", "Enter"} {
		a := key(text)
		a.Seen = unread0.Add(4 * time.Second)
		if res := s.act(t.Context(), a); res.Err != "matchblox cannot read this screen: answer it in its pane" || len(ran) != 0 {
			t.Fatalf("%s: res %+v ran %q", text, res, ran)
		}
	}
}
