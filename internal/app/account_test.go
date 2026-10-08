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

func rowOf(out, name string) string {
	for _, l := range strings.Split(ansi.Strip(out), "\n") {
		if strings.Contains(l, " "+name+" ") {
			return l
		}
	}
	return ""
}

// TestSessionsShowEachAccountWithItsProviderMark: a wide screen has an
// ACCOUNT column; the provider is a mark, not a name.
func TestSessionsShowEachAccountWithItsProviderMark(t *testing.T) {
	out := demoSessions(t, 160).render()
	if !strings.Contains(ansi.Strip(out), "ACCOUNT") {
		t.Fatalf("no ACCOUNT column at 160:\n%s", ansi.Strip(out))
	}
	for name, want := range map[string]string{
		"api-auth": "✻ dev@example.com · Max",
		"codex":    "❋ dev@example.com · Plus",
		"opencode": "✻ login",
	} {
		l := rowOf(out, name)
		if !strings.Contains(l, want) {
			t.Errorf("%s: %q lacks %q", name, l, want)
		}
		if strings.Contains(l, "anthropic") || strings.Contains(l, "openai") {
			t.Errorf("%s: a provider name instead of its mark: %q", name, l)
		}
	}
	// Context use for every agent.
	for name, want := range map[string]string{"codex": "55%", "opencode": "32%"} {
		if l := rowOf(out, name); !strings.Contains(l, want) {
			t.Errorf("%s: %q lacks %s", name, l, want)
		}
	}
}

// TestTheAccountColumnIsDroppedFirst: without room the column goes, and
// every line still fits.
func TestTheAccountColumnIsDroppedFirst(t *testing.T) {
	for _, w := range []int{100, 50} {
		out := demoSessions(t, w).render()
		if strings.Contains(out, "ACCOUNT") || strings.Contains(out, "dev@example.com") {
			t.Errorf("width %d shows the account", w)
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
	if !strings.Contains(ansi.Strip(m.render()), "ACCOUNT▾") {
		t.Fatal("the header does not mark the account column")
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
		for tab := range 7 {
			m.tab = tab
			out := ansi.Strip(m.render())
			shown = shown || strings.Contains(out, "dev@example.com")
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
	m, _ := loadedWith(t, 120, st)
	m.tab = tabSessions
	for _, s := range m.snap.Sessions {
		if s.Agent == "opencode" {
			m.selPID = s.PID
		}
	}
	out := ansi.Strip(m.render())
	if strings.Contains(out, "/ 0 tokens") || !strings.Contains(out, "63k tokens · no window known: add the model to [sessions.windows]") {
		t.Fatalf("detail:\n%s", out)
	}
}
