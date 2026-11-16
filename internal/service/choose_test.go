package service

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
)

const chooseScreen = `Bash command
  go test ./...
Do you want to proceed?
❯ 1. Yes
  2. Yes, and don't ask again for go test commands
  3. No, and tell Claude what to do differently (esc)
`

// pickOf is a choose: act's text: option n, its whole text, and the
// signature of the screen the console saw.
func pickOf(n, text, screen string) string { return n + "\x00" + text + "\x00" + menu.Sign(screen) }

func chooseAct(pane, text string) proto.Act {
	x := flintAuth()
	return proto.Act{RecID: proto.ChoosePrefix + pane, Text: text, PID: x.PID, Start: x.Start, Confirm: "y", Console: true}
}

// chooseTest is the service with flint-auth in %3, its screen, and the
// commands its actions ran.
func chooseTest(t *testing.T, screen string) (*Service, *[][]string) {
	t.Helper()
	x := flintAuth()
	x.Permits, x.Busy, x.Status = true, false, "waiting"
	s, _ := holdsTest(t, x)
	was := chooseSettle
	chooseSettle = 50 * time.Millisecond // a screen that never redraws holds the pane this long
	t.Cleanup(func() { chooseSettle = was })
	var ran [][]string
	s.Actions.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	s.Capture = func(ps []string) map[string]string {
		if len(ps) != 1 || ps[0] != "%3" {
			t.Fatalf("captured %q", ps)
		}
		return map[string]string{"%3": screen}
	}
	return s, &ran
}

// TestChooseTypesTheDigitWhenTheMenuStands: option 2 still says what the
// console saw: the digit goes, as text, once.
func TestChooseTypesTheDigitWhenTheMenuStands(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	res := s.act(t.Context(), chooseAct("%3", pickOf("2", "Yes, and don't ask again for go test commands", chooseScreen)))
	if res.Err != "" || len(res.Ran) != 1 {
		t.Fatalf("%+v", res)
	}
	if want := []string{"tmux", "send-keys", "-t", "%3", "-l", "--", "2"}; len(*ran) != 1 || !slices.Equal((*ran)[0], want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

// TestChooseTwiceToTwoMenus: the same pick at the next prompt is a new
// answer, not a repeat.
func TestChooseTwiceToTwoMenus(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	for range 2 {
		if res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", chooseScreen))); res.Err != "" || len(res.Ran) != 1 {
			t.Fatalf("%+v", res)
		}
	}
	if len(*ran) != 2 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestChooseRefusesAChangedMenu: option 2 says something else now: the
// digit would answer what the owner did not see.
func TestChooseRefusesAChangedMenu(t *testing.T) {
	s, ran := chooseTest(t, "Do you want to proceed?\n❯ 1. Yes\n  2. No\n")
	res := s.act(t.Context(), chooseAct("%3", pickOf("2", "Yes, and don't ask again for go test commands", chooseScreen)))
	if res.Err != "not sent: flint-auth's menu changed — look again" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesNoMenu: the menu went.
func TestChooseRefusesNoMenu(t *testing.T) {
	s, ran := chooseTest(t, "● Done.\n\n❯ \n")
	res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", chooseScreen)))
	if res.Err != "not sent: flint-auth shows no menu" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesAnotherAgent: a new agent runs in the pane.
func TestChooseRefusesAnotherAgent(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	a := chooseAct("%3", pickOf("1", "Yes", chooseScreen))
	a.PID = 77
	if res := s.act(t.Context(), a); res.Err != "not sent: %3 runs a different agent now" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseIsConsoleOnly: no agent, stoker or CLI picks for the owner.
func TestChooseIsConsoleOnly(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	a := chooseAct("%3", pickOf("1", "Yes", chooseScreen))
	a.Console = false
	if res := s.act(t.Context(), a); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
	a = chooseAct("%3", pickOf("1", "Yes", chooseScreen))
	a.From = "%3" // from inside the agent's own pane
	if res := s.act(t.Context(), a); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("from the pane: %+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesABadText: one digit 1-9, then the option's text.
func TestChooseRefusesABadText(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	sign := menu.Sign(chooseScreen)
	for _, text := range []string{"x\x00Yes\x00" + sign, "0\x00Yes\x00" + sign, "10\x00Yes\x00" + sign, "1Yes", "1\x00\x00" + sign, "1\x00Yes", "1\x00Yes\x00"} {
		if res := s.act(t.Context(), chooseAct("%3", text)); res.Err == "" {
			t.Errorf("%q: %+v", text, res)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestChooseRefusesAnotherCommand: a new prompt with the same options asks
// about another command. "1. Yes" there approves what the owner never saw.
func TestChooseRefusesAnotherCommand(t *testing.T) {
	s, ran := chooseTest(t, strings.Replace(chooseScreen, "go test ./...", "rm -rf build/", 1))
	res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", chooseScreen)))
	if res.Err != "not sent: flint-auth's menu changed — look again" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseNeedsTheWholeText: the start of an option's text is not the
// option.
func TestChooseNeedsTheWholeText(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	res := s.act(t.Context(), chooseAct("%3", pickOf("2", "Yes, and", chooseScreen)))
	if res.Err != "not sent: flint-auth's menu changed — look again" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseReadsTheScreenTheConsoleSees: the console's screen comes from
// ScreenOf, which joins the lines tmux wrapped. The service reads the same
// way, so a prompt with a long, wrapped command still signs alike.
func TestChooseReadsTheScreenTheConsoleSees(t *testing.T) {
	long := strings.Replace(chooseScreen, "go test ./...", "go test ./internal/service/... -run TestChooseReadsTheScreenTheConsoleSees -count=1", 1)
	wrapped := strings.Replace(long, "-run ", "-run\n", 1) // capture-pane with no -J
	s, ran := chooseTest(t, wrapped)
	s.ScreenOf = func(pane string) (string, bool, bool) { return long, false, pane == "%3" }
	res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", long)))
	if res.Err != "" || len(*ran) != 1 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestASecondChooseWaitsForTheRedraw: two consoles pick on one prompt. The
// first digit goes; the agent then draws its next prompt, with the same
// options for another command. The second pick reads the new prompt and
// is refused: it never lands on a prompt its owner did not see.
func TestASecondChooseWaitsForTheRedraw(t *testing.T) {
	next := strings.Replace(chooseScreen, "go test ./...", "rm -rf build/", 1)
	s, _ := chooseTest(t, chooseScreen)
	chooseSettle = time.Second
	var mu sync.Mutex
	var typed time.Time
	var ran [][]string
	s.Actions.Run = func(argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		ran, typed = append(ran, argv), time.Now()
		return nil
	}
	s.Capture = func([]string) map[string]string {
		mu.Lock()
		defer mu.Unlock()
		if !typed.IsZero() && time.Since(typed) > 100*time.Millisecond {
			return map[string]string{"%3": next} // the agent redrew
		}
		return map[string]string{"%3": chooseScreen}
	}
	if res := s.act(t.Context(), chooseAct("%3", pickOf("1", "Yes", chooseScreen))); res.Err != "" {
		t.Fatalf("the first: %+v", res)
	}
	res := s.act(t.Context(), chooseAct("%3", pickOf("2", "Yes, and don't ask again for go test commands", chooseScreen)))
	mu.Lock()
	defer mu.Unlock()
	if res.Err != "not sent: flint-auth's menu changed — look again" || len(ran) != 1 {
		t.Fatalf("the second: %+v, ran %q", res, ran)
	}
}
