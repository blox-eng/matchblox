package app

import (
	"strings"
	"testing"

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
	names, _ := accountNames(m.snap.Limits, false)
	want := []string{"Claude builder", "OpenAI Plus", "Claude work"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("name %d is %q, want %q", i, names[i], w)
		}
	}
	// One default block of a provider is named by its plan, as it was.
	st.Limits = st.Limits[:1]
	m, _ = loadedWith(t, 132, st)
	if names, _ := accountNames(m.snap.Limits, false); names[0] != "Claude Max" {
		t.Errorf("a lone default block is named %q, want Claude Max", names[0])
	}
}

func TestRowSaysSigningIn(t *testing.T) {
	for _, w := range []int{44, 160} { // the chosen card says what the row said
		st := queueState()
		st.Queue = nil // no ask or wait: the row says the sign-in
		st.Sessions[0].AccountDir = "/home/u/.matchblox/accounts/claude/work"
		st.Sessions[0].SigningIn = true
		pane := st.Sessions[0].Pane
		m, _ := loadedWith(t, w, st)
		m = selectPane(t, m, pane).(Model)
		out := screen(m)
		if !strings.Contains(out, "signing in") {
			t.Errorf("width %d: the row does not say signing in:\n%s", w, out)
		}
		st.SignIns = map[string]string{pane: "by hand"}
		m, _ = loadedWith(t, w, st)
		m = selectPane(t, m, pane).(Model)
		if out := screen(m); !strings.Contains(out, "sign in by hand") || strings.Contains(out, "signing in") {
			t.Errorf("width %d: a sign-in by hand says so:\n%s", w, out)
		}
	}
}

// TestAsksWinsOverSigningIn: a session that signs in and asks or waits
// shows the ask, so a wait is never hidden.
func TestAsksWinsOverSigningIn(t *testing.T) {
	for _, w := range []int{44, 160} {
		st := queueState()
		for i := range st.Sessions {
			if st.Sessions[i].Pane == "%1" {
				st.Sessions[i].AccountDir = "/home/u/.matchblox/accounts/claude/work"
				st.Sessions[i].SigningIn = true
			}
		}
		m, _ := loadedWith(t, w, st)
		m = selectPane(t, m, "%1").(Model) // the chosen card says what the row said
		row := screen(m)
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
	m, _ := loadedWith(t, 160, st)
	m = selectPane(t, m, st.Sessions[0].Pane).(Model) // the chosen card says what the row said
	if out := screen(m); !strings.Contains(out, "sign in by hand") {
		t.Errorf("the row does not say sign in by hand:\n%s", out)
	}
}
