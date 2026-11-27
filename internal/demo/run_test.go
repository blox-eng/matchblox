package demo_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
)

// askPane is the demo session that waits on a question's menu.
const askPane = "%2"

func runMenu(t *testing.T, run func(string) (string, int), who string) proto.Menu {
	t.Helper()
	out, status := run("menu " + who + " --json")
	if status != 0 {
		t.Fatalf("menu %s: %s", who, out)
	}
	var m proto.Menu
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("menu %s: %v: %s", who, err, out)
	}
	return m
}

// waitForSessions runs `sessions --json` until the demo's service has
// sampled its sessions.
func waitForSessions(t *testing.T, run func(string) (string, int)) []map[string]any {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		out, status := run("sessions --json")
		if status != 0 {
			t.Fatalf("sessions: %s", out)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("sessions: %v: %s", err, out)
		}
		if len(rows) > 0 {
			return rows
		}
	}
	t.Fatal("the demo never listed its sessions")
	return nil
}

func TestDemoRunMenuReadsTheLiveScreen(t *testing.T) {
	d, _ := start(t)
	waitForSessions(t, d.Run)
	screen, err := os.ReadFile(filepath.Join(tree, "panes", "2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := menu.Read(string(screen))
	m := runMenu(t, d.Run, askPane)
	if len(m.Options) != len(want) || len(want) == 0 || m.Chosen != menu.Chosen(string(screen)) || m.Sign != menu.Sign(string(screen)) {
		t.Fatalf("menu %+v, want %d options, chosen %d", m, len(want), menu.Chosen(string(screen)))
	}
	for _, o := range m.Options {
		if o.Text != want[o.N] {
			t.Errorf("option %d is %q, want %q", o.N, o.Text, want[o.N])
		}
	}
	if m := runMenu(t, d.Run, "%1"); len(m.Options) != 0 || m.Options == nil || m.Sign != "" {
		t.Fatalf("a session with no menu reads %+v", m)
	}
}

func TestDemoRunChooseAnswersTheMenu(t *testing.T) {
	d, _ := start(t)
	waitForSessions(t, d.Run)
	m := runMenu(t, d.Run, askPane)
	text := m.Options[1].Text
	stale := "choose " + askPane + " 2 --text " + quote(text) + " --sign 0123456789abcdef"
	if out, status := d.Run(stale); status != 3 || out != "the question changed" {
		t.Fatalf("a stale sign: %d %q", status, out)
	}
	if typed(d.Ran()) {
		t.Fatal("a stale sign typed a key")
	}
	if out, status := d.Run("choose " + askPane + " 2 --text " + quote(text) + " --sign " + m.Sign); status != 0 {
		t.Fatalf("choose: %d %s", status, out)
	}
	if !typed(d.Ran()) {
		t.Fatalf("choose typed no 2: %q", d.Ran())
	}
	if m := runMenu(t, d.Run, askPane); len(m.Options) != 0 {
		t.Fatalf("the menu stays after the pick: %+v", m)
	}
}

// TestDemoRunPickAfterTheAgentMovedIsStale (Surface: an agent's tools): the
// agent's cursor moves between the menu read and the pick, as when its
// question is drawn again; the pick types nothing and exits as a host's
// does, so Pocket says the question changed.
func TestDemoRunPickAfterTheAgentMovedIsStale(t *testing.T) {
	d, _ := start(t)
	waitForSessions(t, d.Run)
	m := runMenu(t, d.Run, askPane)
	if err := d.MoveCursor(); err != nil {
		t.Fatal(err)
	}
	if again := runMenu(t, d.Run, askPane); again.Sign == m.Sign {
		t.Fatalf("the moved cursor kept the sign %s", m.Sign)
	}
	pick := "choose " + askPane + " 2 --text " + quote(m.Options[1].Text) + " --sign " + m.Sign
	if out, status := d.Run(pick); status != 3 || out != "the question changed" {
		t.Fatalf("a pick after the cursor moved: %d %q", status, out)
	}
	if typed(d.Ran()) {
		t.Fatal("a stale pick typed a key")
	}
}

func typed(ran [][]string) bool {
	return slices.ContainsFunc(ran, func(argv []string) bool {
		return len(argv) > 4 && argv[1] == "send-keys" && argv[3] == askPane && argv[len(argv)-1] == "2"
	})
}

// quote is the quoting Pocket gives a word.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// TestDemoRunSessionsHaveOpenNumbers: each session with an id has its own
// number, as on a host; one with no id (wick-mobile's OpenCode) has none.
func TestDemoRunSessionsHaveOpenNumbers(t *testing.T) {
	d, _ := start(t)
	rows := waitForSessions(t, d.Run)
	seen := map[float64]bool{}
	for _, r := range rows {
		n, numbered := r["n"].(float64)
		if numbered && (n < 1 || seen[n]) || r["name"] == "" || r["word"] == "" || r["pane"] == "" {
			t.Fatalf("row %v", r)
		}
		seen[n] = numbered
		if r["pane"] == askPane && !numbered {
			t.Fatalf("the asking session has no number: %v", r)
		}
	}
	again := waitForSessions(t, d.Run)
	for i := range rows {
		if again[i]["n"] != rows[i]["n"] {
			t.Fatalf("a session's number changed: %v then %v", rows[i], again[i])
		}
	}
}

// TestDemoRunRefusesOtherCommands (Surface: an agent's tools): the demo
// runs Pocket's three commands and nothing else.
func TestDemoRunRefusesOtherCommands(t *testing.T) {
	d, _ := start(t)
	for _, line := range []string{
		"rm -rf /",
		"",
		"sessions",
		"sessions --json --all",
		"menu " + askPane,
		"menu " + askPane + " --json; rm -rf /",
		"menu '" + askPane + " --json",
		"choose " + askPane + " 2 --sign s --text t",
		"send " + askPane + " hello",
	} {
		if out, status := d.Run(line); status != 1 || out != "not a demo command" {
			t.Errorf("%q: %d %q", line, status, out)
		}
	}
	if len(d.Ran()) != 0 {
		t.Fatalf("a refused command ran %q", d.Ran())
	}
}
