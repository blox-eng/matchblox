package service

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

const chooseScreen = `Do you want to proceed?
❯ 1. Yes
  2. Yes, and don't ask again for go test commands
  3. No, and tell Claude what to do differently (esc)
`

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
	res := s.act(t.Context(), chooseAct("%3", "2\x00Yes, and don't ask again for go test commands"))
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
		if res := s.act(t.Context(), chooseAct("%3", "1\x00Yes")); res.Err != "" || len(res.Ran) != 1 {
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
	res := s.act(t.Context(), chooseAct("%3", "2\x00Yes, and don't ask again for go test commands"))
	if res.Err != "not sent: flint-auth's menu changed — look again" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesNoMenu: the menu went.
func TestChooseRefusesNoMenu(t *testing.T) {
	s, ran := chooseTest(t, "● Done.\n\n❯ \n")
	res := s.act(t.Context(), chooseAct("%3", "1\x00Yes"))
	if res.Err != "not sent: flint-auth shows no menu" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesAnotherAgent: a new agent runs in the pane.
func TestChooseRefusesAnotherAgent(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	a := chooseAct("%3", "1\x00Yes")
	a.PID = 77
	if res := s.act(t.Context(), a); res.Err != "not sent: %3 runs a different agent now" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestChooseIsConsoleOnly: no agent, stoker or CLI picks for the owner.
func TestChooseIsConsoleOnly(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	a := chooseAct("%3", "1\x00Yes")
	a.Console = false
	if res := s.act(t.Context(), a); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
	a = chooseAct("%3", "1\x00Yes")
	a.From = "%3" // from inside the agent's own pane
	if res := s.act(t.Context(), a); res.Err == "" || len(*ran) != 0 {
		t.Fatalf("from the pane: %+v, ran %q", res, *ran)
	}
}

// TestChooseRefusesABadText: one digit 1-9, then the option's text.
func TestChooseRefusesABadText(t *testing.T) {
	s, ran := chooseTest(t, chooseScreen)
	for _, text := range []string{"x\x00Yes", "0\x00Yes", "10\x00Yes", "1Yes", "1\x00"} {
		if res := s.act(t.Context(), chooseAct("%3", text)); res.Err == "" {
			t.Errorf("%q: %+v", text, res)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}
