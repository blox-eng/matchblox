package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
)

// accountBlocks is a default Claude block and a Claude block of the account
// "work", both measured.
func accountBlocks() []limits.Account {
	at := limitsState().At
	win := []limits.Window{{UsedPct: 41, Minutes: limits.BoxMinutes, ResetsAt: at.Add(2 * 3600e9)}}
	return []limits.Account{
		{Provider: "anthropic", Account: "you@example.com · Max", State: limits.Measured, At: at, Windows: win},
		{Provider: "anthropic", Account: "you@example.com · Max", Dir: "/home/u/.matchblox/accounts/claude/work", State: limits.Measured, At: at, Windows: win},
	}
}

func TestUsageNamesAccountBlocks(t *testing.T) {
	st := limitsState()
	st.Limits = accountBlocks()
	m, _ := loadedWith(t, 132, st)
	out := screen(m)
	for _, want := range []string{"✻ Claude Max ", "✻ Claude work "} {
		if !strings.Contains(out, want) {
			t.Errorf("USAGE lacks %q:\n%s", want, out)
		}
	}
}

func TestHandMadeFolderIsNamedByItsBase(t *testing.T) {
	st := limitsState()
	st.Limits = accountBlocks()
	st.Limits[1].Dir = "/home/u/.claude-old"
	m, _ := loadedWith(t, 132, st)
	if out := screen(m); !strings.Contains(out, "✻ Claude .claude-old ") {
		t.Errorf("USAGE lacks the hand-made folder's name:\n%s", out)
	}
}

func TestDefaultBlocksKeepTheirNames(t *testing.T) {
	st := limitsState()
	m, _ := loadedWith(t, 132, st)
	names, _ := m.accountNames(false)
	want := []string{"Claude builder", "OpenAI Plus", "Claude work"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("name %d is %q, want %q", i, names[i], w)
		}
	}
	// One default block of a provider is named by its plan, as it was.
	st.Limits = st.Limits[:1]
	m, _ = loadedWith(t, 132, st)
	if names, _ := m.accountNames(false); names[0] != "Claude Max" {
		t.Errorf("a lone default block is named %q, want Claude Max", names[0])
	}
}

func TestRowSaysSigningIn(t *testing.T) {
	for _, w := range []int{44, 80, 132} {
		st := queueState()
		st.Queue = nil // no ask or wait: the row says the sign-in
		st.Sessions[0].AccountDir = "/home/u/.matchblox/accounts/claude/work"
		st.Sessions[0].SigningIn = true
		pane := st.Sessions[0].Pane
		m, _ := loadedWith(t, w, st)
		m.tab = tabSessions
		out := screen(m)
		if !strings.Contains(out, "signing in") {
			t.Errorf("width %d: the row does not say signing in:\n%s", w, out)
		}
		st.SignIns = map[string]string{pane: "by hand"}
		m, _ = loadedWith(t, w, st)
		m.tab = tabSessions
		if out := screen(m); !strings.Contains(out, "sign in by hand") || strings.Contains(out, "signing in") {
			t.Errorf("width %d: a sign-in by hand says so:\n%s", w, out)
		}
	}
}

// TestAsksWinsOverSigningIn: a session that signs in and asks or waits
// shows the ask, so a wait is never hidden.
func TestAsksWinsOverSigningIn(t *testing.T) {
	for _, w := range []int{44, 132} {
		st := queueState()
		for i := range st.Sessions {
			if st.Sessions[i].Pane == "%1" {
				st.Sessions[i].AccountDir = "/home/u/.matchblox/accounts/claude/work"
				st.Sessions[i].SigningIn = true
			}
		}
		m, _ := loadedWith(t, w, st)
		m.tab = tabSessions
		row := rowOf(m.render(), "app-feature")
		if !strings.Contains(row, "asks") || strings.Contains(row, "signing in") {
			t.Errorf("width %d: the row hides its ask: %q", w, row)
		}
	}
}

// TestRowSaysByHandForItsSessionName: a new account whose pane was not
// found is noted by its tmux session's name.
func TestRowSaysByHandForItsSessionName(t *testing.T) {
	st := queueState()
	st.Queue = nil // no ask or wait: the row says the sign-in
	st.Sessions[0].AccountDir = "/home/u/.matchblox/accounts/claude/work"
	st.Sessions[0].SigningIn = true
	st.Sessions[0].Target = "fresh:1.1"
	st.SignIns = map[string]string{"fresh": "by hand"}
	m, _ := loadedWith(t, 132, st)
	m.tab = tabSessions
	if out := screen(m); !strings.Contains(out, "sign in by hand") {
		t.Errorf("the row does not say sign in by hand:\n%s", out)
	}
}

func TestRowNamesANonDefaultAccount(t *testing.T) {
	st := queueState()
	st.Sessions[0].AccountDir = "/home/u/.matchblox/accounts/claude/work"
	m, _ := loadedWith(t, 132, st)
	m.tab = tabSessions
	if out := screen(m); !strings.Contains(out, st.Sessions[0].Name+" work") {
		t.Errorf("the row does not name its account:\n%s", out)
	}
	// Narrow, the row keeps the tag after the name.
	narrow, _ := loadedWith(t, 60, st)
	narrow.tab = tabSessions
	if row := ansi.Strip(strings.Join(narrow.sessionRow(st.Sessions[0], false, 60), "\n")); !strings.Contains(row, st.Sessions[0].Name+" work") {
		t.Errorf("narrow, the row does not name its account: %q", row)
	}
	// With fewer than four cells left after the name, the tag is dropped.
	if got := ansi.Strip(m.taggedName("app-feature", "work", false, len("app-feature")+4)); got != "app-feature" {
		t.Errorf("room 3: %q, want the tag dropped", got)
	}
	if got := ansi.Strip(m.taggedName("app-feature", "work", false, len("app-feature")+5)); got != "app-feature work" {
		t.Errorf("room 4: %q, want the tag", got)
	}
}

// TestAccountTagKeepsAGapBeforeTheWord: wide or narrow, a tag cut short or a
// name longer than its column leaves a space before the state word.
func TestAccountTagKeepsAGapBeforeTheWord(t *testing.T) {
	for _, name := range []string{"app-feature", "app-feature-with-a-very-long-name-indeed"} {
		for w := 40; w <= 110; w += 5 {
			st := queueState()
			st.Sessions[0].Name = name
			st.Sessions[0].AccountDir = "/home/u/.matchblox/accounts/claude/workbench"
			m, _ := loadedWith(t, w, st)
			m.tab = tabSessions
			for _, line := range m.sessionRow(st.Sessions[0], false, w) {
				row := ansi.Strip(line)
				if i := strings.Index(row, "asks"); i > 0 && row[i-1] != ' ' {
					t.Errorf("width %d, %q: no gap before the word: %q", w, name, row)
				}
				if lipgloss.Width(row) > w {
					t.Errorf("width %d: row too wide: %q", w, row)
				}
			}
		}
	}
}
