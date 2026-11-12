package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

const demo = "../../testdata/demo"

// demoState is the demo machine with Claude Code, Codex and OpenCode, at
// the replay's clock.
func demoState(t *testing.T) proto.State {
	t.Helper()
	s := &sample.Sampler{
		FS: procfs.FS{Root: filepath.Join(demo, "proc"), PageSize: procfs.FixturePage}, Home: filepath.Join(demo, "home"), HomeAs: "/home/user",
		Tmux:    func() ([]byte, error) { return os.ReadFile(filepath.Join(demo, "tmux-panes.txt")) },
		Capture: sample.CaptureDir(filepath.Join(demo, "panes")), Rules: sample.DefaultRules,
		LimitsDir: filepath.Join(demo, "state"),
		Agents:    []string{"claude", "codex", "opencode"},
		Now:       func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) },
	}
	snap := s.Sample()
	q := queue.New()
	q.Merge(snap.Sessions, snap.At)
	return proto.State{Doc: state.Doc{Snapshot: snap, Recommendations: advice.Build(snap, nil), Queue: q.Items()}}
}

func demoSessions(t *testing.T, w int) Model {
	t.Helper()
	m, _ := loadedWith(t, w, demoState(t))
	m.tab = tabSessions
	return m
}

// demoDetails is demoSessions with every session outside tmux: with no
// pane to chat in, the selected one shows its detail beside the list.
func demoDetails(t *testing.T, w int) Model {
	t.Helper()
	m, _ := loadedWith(t, w, outsideTmux(demoState(t)))
	m.tab = tabSessions
	return m
}

func outsideTmux(st proto.State) proto.State {
	st.Sessions = slices.Clone(st.Sessions)
	for i := range st.Sessions {
		st.Sessions[i].Pane = ""
	}
	return st
}

// rowOf is the screen line of a session's card: its name, never a
// recommendation that names it ("Compact x (%1)").
func rowOf(out, name string) string {
	for _, l := range strings.Split(ansi.Strip(out), "\n") {
		if strings.Contains(l, " "+name+" ") && !strings.Contains(l, "(%") {
			return l
		}
	}
	return ""
}

// choose selects the session with pid, as a tap on its card does.
func choose(m *Model, pid int) {
	m.chose, m.selPID, m.doorPick, m.recPick = true, pid, "", ""
	for _, s := range m.snap.Sessions {
		if s.PID == pid {
			m.selHost = s.Host
			m.boardChooseCard(m.columnOf(s), s)
			return
		}
	}
}

// TestAProviderWithoutAMarkKeepsItsName: an OpenCode provider that has
// no mark shows its id.
func TestAProviderWithoutAMarkKeepsItsName(t *testing.T) {
	if got := accountCell(sample.Session{Provider: "zen", Account: "API key"}); got != "zen · API key" {
		t.Fatalf("got %q", got)
	}
	if got := accountCell(sample.Session{Provider: "bedrock"}); got != "bedrock" {
		t.Fatalf("got %q", got)
	}
	if got := accountCell(sample.Session{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

// TestNoCredentialReachesTheScreen: the demo's auth files hold tokens next
// to the fields matchblox reads. No tab shows one, at any width.
func TestNoCredentialReachesTheScreen(t *testing.T) {
	var secrets []string
	var codex struct {
		Tokens map[string]string `json:"tokens"`
	}
	b, err := os.ReadFile(filepath.Join(demo, "home", ".codex", "auth.json"))
	if err != nil || json.Unmarshal(b, &codex) != nil {
		t.Fatal(err)
	}
	for _, v := range codex.Tokens {
		secrets = append(secrets, v)
	}
	secrets = append(secrets, "made-up")
	shown := false
	for _, w := range []int{50, 100, 160, 220} {
		for _, m := range []Model{demoSessions(t, w), demoDetails(t, w)} {
			for tab := range screenNames() { // the tabs and Machine's views
				m.tab = tab
				out := ansi.Strip(m.render())
				shown = shown || strings.Contains(out, "team@example.com")
				for _, s := range secrets {
					if strings.Contains(out, s) {
						t.Fatalf("width %d tab %d shows %q", w, tab, s)
					}
				}
			}
		}
	}
	if !shown {
		t.Fatal("positive control: no account label on any screen, so the check proves nothing")
	}
}
