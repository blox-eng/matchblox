package service

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

// pocketTest is chooseTest with the callers' panes: 200 runs inside
// flint-auth's pane, 555 in a terminal of the person's own.
func pocketTest(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran := chooseTest(t, chooseScreen)
	s.PeerPane = func(pid int) string { return map[int]string{200: "%3"}[pid] }
	return s, ran
}

func pocketChoose(text string) proto.Act {
	a := chooseAct("%3", text)
	a.Console = false // the hello decides it
	return a
}

// TestPocketHelloFromAgentPaneRefused: an agent that runs `matchblox
// choose` or `menu` says hello as Pocket too; from its own pane, by its
// pid or by its claim, it picks nothing and reads nothing.
func TestPocketHelloFromAgentPaneRefused(t *testing.T) {
	s, ran := pocketTest(t)
	pick := pocketChoose(pickOf("2", "Yes, and don't ask again for go test commands", chooseScreen))
	claimed := pick
	claimed.From = "%3"
	for name, c := range map[string]struct {
		pid int
		a   proto.Act
	}{"pid in the agent's pane": {200, pick}, "claims the agent's pane": {0, claimed}} {
		if res := actOver(t, s, c.pid, proto.ClientPocket, c.a); !strings.HasSuffix(res.Err, "only the console picks from a menu") {
			t.Errorf("%s: %+v", name, res)
		}
		read := proto.Act{RecID: proto.MenuPrefix + "%3", From: c.a.From}
		if res := actOver(t, s, c.pid, proto.ClientPocket, read); res.Menu != nil || !strings.HasSuffix(res.Err, "only the console reads a menu") {
			t.Errorf("%s read the menu: %+v", name, res)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// TestPocketChooses: Pocket's pick goes through the checked path and types
// the digit.
func TestPocketChooses(t *testing.T) {
	s, ran := pocketTest(t)
	res := actOver(t, s, 555, proto.ClientPocket, pocketChoose(pickOf("2", "Yes, and don't ask again for go test commands", chooseScreen)))
	if res.Err != "" || len(*ran) != 1 || (*ran)[0][len((*ran)[0])-1] != "2" {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
	if res := actOver(t, s, 555, proto.ClientPocket, proto.Act{RecID: proto.MenuPrefix + "%3"}); res.Err != "" || res.Menu == nil || len(res.Menu.Options) != 3 {
		t.Fatalf("Pocket read no menu: %+v", res)
	}
}

// TestPocketChooseStale: a pick signed on an older screen types nothing.
func TestPocketChooseStale(t *testing.T) {
	s, ran := pocketTest(t)
	old := "Do you want to proceed?\n❯ 1. Yes\n  2. Yes, and don't ask again for go test commands\n"
	res := actOver(t, s, 555, proto.ClientPocket, pocketChoose(pickOf("2", "Yes, and don't ask again for go test commands", old)))
	if !strings.HasSuffix(res.Err, "flint-auth's menu changed — look again") || len(*ran) != 0 {
		t.Fatalf("%+v, ran %q", res, *ran)
	}
}

// TestPocketIsNoConsoleElsewhere: Pocket is a console for a menu only; its
// message frees no pane the stoker drove, as the person's console would.
func TestPocketIsNoConsoleElsewhere(t *testing.T) {
	s, _, _ := guardedRun(t)
	s.PeerPane = func(int) string { return "" }
	if res := actOver(t, s, 777, proto.ClientPocket, proto.Act{RecID: "message:%1", Confirm: "y", Text: "merge it"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if !denies(t, s, "%1", "gh pr merge 4556") {
		t.Fatal("Pocket's message freed the pane")
	}
}
