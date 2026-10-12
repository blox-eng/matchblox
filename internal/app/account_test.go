package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
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
		FS: procfs.FS{Root: filepath.Join(demo, "proc")}, Home: filepath.Join(demo, "home"), HomeAs: "/home/user",
		Tmux:    func() ([]byte, error) { return os.ReadFile(filepath.Join(demo, "tmux-panes.txt")) },
		Capture: sample.CaptureDir(filepath.Join(demo, "panes")), Rules: sample.DefaultRules,
		Agents: []string{"claude", "codex", "opencode"},
		Now:    func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) },
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

// rowOf is the screen line of a session's row: its name in the list
// column, never a recommendation that names it ("Compact x (%1)").
func rowOf(out, name string) string {
	lines := strings.Split(ansi.Strip(out), "\n")
	for _, l := range lines {
		if left := listOf(l, len([]rune(lines[0]))); strings.Contains(left, " "+name+" ") && !strings.Contains(left, "(%") {
			return l
		}
	}
	return ""
}

// choose selects the session with pid, as a pick on its row does.
func choose(m *Model, pid int) {
	m.chose, m.selPID, m.doorPick, m.recPick = true, pid, "", ""
}

// TestTheDetailNamesTheAccountWithItsProviderMark: the detail of a session
// names its account; the provider is a mark, not a name.
func TestTheDetailNamesTheAccountWithItsProviderMark(t *testing.T) {
	for name, want := range map[string]string{
		"hearth-api": "✻ team@example.com · Max",
		"codex":      "❋ team@example.com · Plus",
		"opencode":   "google · login",
	} {
		m := demoSessions(t, 160)
		for _, s := range m.snap.Sessions {
			if s.Name == name {
				choose(&m, s.PID)
			}
		}
		out := ansi.Strip(m.render())
		if !strings.Contains(out, want) {
			t.Errorf("%s: the detail lacks %q:\n%s", name, want, out)
		}
		if strings.Contains(out, "anthropic") || strings.Contains(out, "openai") {
			t.Errorf("%s: a provider name instead of its mark:\n%s", name, out)
		}
	}
	// Context use for every agent, on its row.
	out := demoSessions(t, 160).render()
	for name, want := range map[string]string{"codex": "55%", "opencode": "6%"} {
		if l := rowOf(out, name); !strings.Contains(l, want) {
			t.Errorf("%s: %q lacks %s", name, l, want)
		}
	}
}

// TestRowsLeaveTheAccountToTheDetail: no row carries the account, and
// every line fits, at every width.
func TestRowsLeaveTheAccountToTheDetail(t *testing.T) {
	for _, w := range []int{50, 100, 160} {
		out := demoSessions(t, w).render()
		if l := rowOf(out, "hearth-api"); l == "" || strings.Contains(listOf(l, w), "team@example.com") {
			t.Errorf("width %d: row %q", w, l)
		}
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("width %d: line %d is %d wide", w, i, lw)
			}
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

// TestSessionsSortByAccount: s reaches the account, so the sessions of one
// login sit together.
func TestSessionsSortByAccount(t *testing.T) {
	m := demoSessions(t, 160)
	m.sortOn(sortAccount)
	m.refresh()
	var got []string
	for _, s := range m.snap.Sessions {
		got = append(got, accountCell(s))
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("not sorted by account: %q", got)
		}
	}
	if !strings.Contains(ansi.Strip(m.render()), "SESSIONS · by account ▾") {
		t.Fatal("the list does not name the sort")
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
		m := demoSessions(t, w)
		for tab := range tabNames {
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
	if !shown {
		t.Fatal("positive control: no account label on any screen, so the check proves nothing")
	}
}

// TestADetailWithoutAWindowShowsNoFigureForIt: tokens without a window
// (a model OpenCode's cache does not know) never print "/ 0".
func TestADetailWithoutAWindowShowsNoFigureForIt(t *testing.T) {
	st := demoState(t)
	for i, s := range st.Sessions {
		if s.Agent == "opencode" {
			st.Sessions[i].Context, st.Sessions[i].Window, st.Sessions[i].ContextPct = "unmeasured", 0, 0
		}
	}
	m, _ := loadedWith(t, 160, st)
	m.tab = tabSessions
	for _, s := range m.snap.Sessions {
		if s.Agent == "opencode" {
			choose(&m, s.PID)
		}
	}
	s, _ := m.selectedSession()
	facts := strings.Join(m.facts(s), " · ")
	if out := ansi.Strip(m.render()); strings.Contains(out, "/ 0 tokens") || !strings.Contains(out, "no window known") ||
		!strings.Contains(facts, "63k tokens · no window known: add the model to [sessions.windows]") {
		t.Fatalf("detail:\n%s", out)
	}
}

// TestTheDetailShowsTheAccount: on an opened phone the detail names the
// selected session's account too.
func TestTheDetailShowsTheAccount(t *testing.T) {
	m := demoSessions(t, 86)
	for _, s := range m.snap.Sessions {
		if s.Agent == "codex" {
			choose(&m, s.PID)
		}
	}
	if out := strings.Join(strings.Fields(ansi.Strip(m.render())), " "); !strings.Contains(out, "team@example.com") {
		t.Fatalf("detail lacks the account:\n%s", ansi.Strip(m.render()))
	}
}
