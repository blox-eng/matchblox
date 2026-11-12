package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/charmbracelet/x/ansi"
)

// The peek is the old detail pane (ruling 35): under the chosen card's own
// lines it names an account that is not the default, the background work
// and the child runs. The card's own lines stay as Task 3 fixed them.

// peekOf is the frame's lines from the chosen card's head down to a blank
// line, at most a dozen: beside other columns, theirs come too.
func peekOf(t *testing.T, m Model, name string) string {
	t.Helper()
	lines := strings.Split(plainFrame(m), "\n")
	head := name[:min(len(name), 5)] // a narrow column cuts the name
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "▌") && strings.Contains(l, head) })
	if i < 0 {
		t.Fatalf("%s is not the chosen card:\n%s", name, plainFrame(m))
	}
	var out []string
	for _, l := range lines[i:min(i+12, len(lines))] {
		if strings.TrimSpace(l) == "" {
			break
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// TestThePeekNamesAnAccountThatIsNotTheDefault: the chosen card's peek
// names its account with its provider's mark, never the provider's name;
// on the default sign-in, and on a card that is not chosen, nothing.
func TestThePeekNamesAnAccountThatIsNotTheDefault(t *testing.T) {
	for _, w := range []int{44, 160} {
		st := queueState()
		st.Queue = nil
		st.Sessions[0].AccountDir, st.Sessions[0].Provider = "/home/u/.matchblox/accounts/claude/work", "anthropic"
		named, plain := st.Sessions[0], st.Sessions[1]
		m, _ := loadedWith(t, w, st)
		m = selectPane(t, m, named.Pane).(Model)
		if peek := peekOf(t, m, named.Name); !strings.Contains(peek, "✻ work") || strings.Contains(peek, "anthropic") {
			t.Errorf("width %d: the peek does not name the account by its mark:\n%s", w, peek)
		}
		m = selectPane(t, m, plain.Pane).(Model)
		if out := plainFrame(m); strings.Contains(out, "✻ work") {
			t.Errorf("width %d: a card that is not chosen names its account:\n%s", w, out)
		}
		if peek := peekOf(t, m, plain.Name); strings.Contains(peek, "team@example.com") {
			t.Errorf("width %d: the default sign-in is named:\n%s", w, peek)
		}
	}
}

// TestThePeekSaysTheBackgroundWork: an idle match's peek names the
// subagents and shells it left, how long the oldest has run, and each
// shell's command, cut to the column.
func TestThePeekSaysTheBackgroundWork(t *testing.T) {
	st := queueState()
	for i := range st.Sessions {
		if st.Sessions[i].Pane == "%1" {
			st.Sessions[i].Agents = 1
			st.Sessions[i].Shells = []sample.Shell{{PID: 2240632, Started: st.At.Add(-37 * time.Hour), Command: `while pgrep -f "node walk.mjs" >/dev/null; do sleep 15; done`}}
		}
	}
	m, _ := loadedWith(t, 160, st)
	s := sessionOn(t, st, "%1")
	m = selectPane(t, m, "%1").(Model)
	peek := peekOf(t, m, s.Name)
	for _, want := range []string{"1 subagent running · 1 shell · oldest 1d13h", "2240632  while pgrep"} {
		if !strings.Contains(peek, want) {
			t.Errorf("no %q in the peek:\n%s", want, peek)
		}
	}
}

// TestThePeekCountsAndNamesTheChildRuns: the child runs are a count on the
// peek's head and a tree under it, folded past four levels; no child is a
// card.
func TestThePeekCountsAndNamesTheChildRuns(t *testing.T) {
	st := queueState()
	p := sessionOn(t, st, "%1")
	top := &sample.ProcRef{PID: p.PID, Start: p.Start}
	a := &sample.ProcRef{PID: 901, Start: 1}
	st.Children = []sample.Session{
		{PID: 901, Start: 1, Name: "codex exec", Status: "busy", Parent: top},
		{PID: 902, Start: 2, Name: "claude -p", Status: "busy", Parent: top},
		{PID: 903, Start: 3, Name: "a1", Parent: a},
	}
	for _, w := range []int{44, 160} {
		m, _ := loadedWith(t, w, st)
		m = selectPane(t, m, "%1").(Model)
		peek := peekOf(t, m, p.Name)
		for _, want := range []string{"3 child runs", "├─ codex exec busy", "│ └─ a1", "└─ claude -p busy"} {
			if !strings.Contains(peek, want) {
				t.Errorf("width %d: no %q in the peek:\n%s", w, want, peek)
			}
		}
		if names := drawnNames(m); slices.Contains(names, "codex exec") || slices.Contains(names, "a1") {
			t.Errorf("width %d: a child run is a card: %v", w, names)
		}
		if out := plainFrame(selectPane(t, m, "%2").(Model)); strings.Contains(out, "codex exec") {
			t.Errorf("width %d: a card that is not chosen names its children:\n%s", w, out)
		}
	}

	deep := queueState()
	parent := top
	for i := 1; i <= 6; i++ {
		pid := 900 + i
		deep.Children = append(deep.Children, sample.Session{PID: pid, Start: uint64(i), Name: "link" + string(rune('0'+i)), Status: "busy", Parent: parent})
		parent = &sample.ProcRef{PID: pid, Start: uint64(i)}
	}
	m, _ := loadedWith(t, 160, deep)
	m = selectPane(t, m, "%1").(Model)
	if peek := peekOf(t, m, p.Name); !strings.Contains(peek, "6 child runs") || !strings.Contains(peek, "… 2 more") || strings.Contains(peek, "link5") {
		t.Errorf("deep children do not fold:\n%s", peek)
	}
}

// TestACardWithNoWindowPrintsNoZero: tokens with no window known (a model
// OpenCode's cache does not know) never print "/ 0" on the chosen card.
func TestACardWithNoWindowPrintsNoZero(t *testing.T) {
	st := outsideTmux(demoState(t))
	for i, s := range st.Sessions {
		if s.Agent == "opencode" {
			st.Sessions[i].Context, st.Sessions[i].Window, st.Sessions[i].ContextPct = "unmeasured", 0, 0
		}
	}
	m, _ := loadedWith(t, 160, st)
	for _, s := range m.snap.Sessions {
		if s.Agent == "opencode" {
			choose(&m, s.PID)
		}
	}
	if out := plainFrame(m); strings.Contains(out, "/ 0") {
		t.Errorf("a card prints / 0:\n%s", out)
	}
}

// TestThePeekOfAChainMateShowsItsInbox (ruling 40): the peek of a chain
// mate with sends that wait in its inbox counts them; a mate with an empty
// inbox, and a card that is not chosen, say nothing of it.
func TestThePeekOfAChainMateShowsItsInbox(t *testing.T) {
	st := headState(t, runningPlan(), true)
	for _, s := range st.Sessions {
		if s.Name == "hearth-api" {
			st.Chains.Mates = append(st.Chains.Mates, state.MateView{Member: memberOf(s), Inbox: 2})
		}
	}
	for _, w := range []int{44, 160} {
		m, _ := chainBoard(t, w, st, "hearth-api")
		if peek := peekOf(t, m, "hearth-api"); !strings.Contains(peek, "✉ 2 in its inbox") {
			t.Errorf("width %d: the peek does not count the inbox:\n%s", w, peek)
		}
		m, _ = chainBoard(t, w, st, "ember-billing")
		if out := plainFrame(m); strings.Contains(out, "✉") {
			t.Errorf("width %d: an inbox shows off the chosen card's peek:\n%s", w, out)
		}
	}
}

// TestThePeekSaysWhenNoWindowIsKnown (owner, 2026-11-12, "Peek's head"):
// tokens with no window known name the fix on the chosen card's peek; for
// Claude Code the status line gives the window, so the fix names setup.
func TestThePeekSaysWhenNoWindowIsKnown(t *testing.T) {
	for _, c := range []struct{ agent, want string }{
		{"claude", "158k tokens · no window known: run matchblox setup, or add the model to [sessions.windows]"},
		{"opencode", "158k tokens · no window known: add the model to [sessions.windows]"},
	} {
		st := queueState()
		st.Queue = nil
		st.Sessions[0].Agent, st.Sessions[0].Tokens = c.agent, 158_000
		st.Sessions[0].Context, st.Sessions[0].Window, st.Sessions[0].ContextPct = "unmeasured", 0, 0
		s := st.Sessions[0]
		m, _ := loadedWith(t, 160, st)
		m = selectPane(t, m, s.Pane).(Model)
		if peek := peekOf(t, m, s.Name); !strings.Contains(peek, "window known:") {
			t.Errorf("%s: the chosen card's peek does not name the fix:\n%s", c.agent, peek)
		}
		for _, w := range []int{36, 80} { // a column of NEEDS YOU, a phone's
			var said []string
			for _, l := range m.peekHead(sessionOn(t, st, s.Pane), w) {
				said = append(said, strings.TrimSpace(ansi.Strip(l)))
			}
			if got := strings.Join(said, " "); !strings.Contains(got, c.want) {
				t.Errorf("%s at %d: the peek says %q, want %q whole", c.agent, w, got, c.want)
			}
		}
		m = selectPane(t, m, st.Sessions[1].Pane).(Model)
		if out := plainFrame(m); strings.Contains(out, "no window known") {
			t.Errorf("%s: a card that is not chosen names the fix:\n%s", c.agent, out)
		}
	}
}
