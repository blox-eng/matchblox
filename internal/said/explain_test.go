package said

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExplainShowsOnlyTheLinesTheRulesRead: a secret further up the pane is
// not a line the rules read, so the trace never carries it.
func TestExplainShowsOnlyTheLinesTheRulesRead(t *testing.T) {
	screen := "$ export TOKEN=sk-live-123\n" + strings.Repeat("  building the release notes\n", 20) + `
 Do you want to proceed?
 ❯ 1. Yes
   2. No
 Esc to cancel · Tab to amend
`
	tr := Explain(screen)
	for _, l := range tr.Lines {
		if strings.Contains(l, "sk-live-123") {
			t.Errorf("Lines holds the secret: %q", l)
		}
	}
	for _, want := range []string{"1. Yes", "2. No"} {
		if !strings.Contains(strings.Join(tr.Lines, "\n"), want) {
			t.Errorf("Lines %q miss the row %q", tr.Lines, want)
		}
	}
}

// TestExplainAgreesWithScreenAndMenu: Screen and Menu read their verdict
// from Explain's trace, so explain never names a rule that did not decide.
func TestExplainAgreesWithScreenAndMenu(t *testing.T) {
	screens := map[string]string{}
	files, err := os.ReadDir(filepath.Join("..", "..", "testdata", "menus"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		screens[f.Name()] = menuFixture(t, f.Name())
	}
	for _, c := range screenCases {
		screens[c.name] = c.screen
	}
	for s := range menuCases {
		screens[s] = s
	}
	for name, s := range screens {
		tr := Explain(s)
		line, kind := Screen(s)
		_, menu := Menu(s)
		if tr.Line != line || tr.Kind != kind || tr.MenuRead != menu {
			t.Errorf("%s: Explain = (%q, %v, menu %v), Screen = (%q, %v), Menu %v", name, tr.Line, tr.Kind, tr.MenuRead, line, kind, menu)
		}
		for _, h := range tr.Hits {
			if h.Line > 0 && tr.Lines[h.Line-1] != h.Text {
				t.Errorf("%s: %s hit %q is not line %d of %q", name, h.Rule, h.Text, h.Line, tr.Lines)
			}
		}
	}
}

func TestExplainReadsTheClaudeMenu(t *testing.T) {
	tr := Explain(menuFixture(t, "claude-bash-2.1.330.txt"))
	hits := map[string]Hit{}
	for _, h := range tr.Hits {
		hits[h.Rule] = h
	}
	if h := hits[RulePermits]; h.Text != "1. Yes" {
		t.Errorf("approval prompt hit = %+v, want the 1. Yes line", h)
	}
	if h := hits[RuleMenuHint]; h.Line != len(tr.Lines) || h.Text != "Esc to cancel · Tab to amend" {
		t.Errorf("menu key hint hit = %+v, want the last of %d lines", h, len(tr.Lines))
	}
	if h := hits[RuleWorking]; h.Line != 0 {
		t.Errorf("working line hit = %+v, want none", h)
	}
	if tr.Kind != Permits || !tr.MenuRead || tr.Line != "Do you want to proceed?" {
		t.Errorf("verdict = (%q, %v, menu %v)", tr.Line, tr.Kind, tr.MenuRead)
	}
}

func TestExplainNamesTheFallback(t *testing.T) {
	tr := Explain(`
  Ran the suite.
  The cache was cold. It is warm now.
╭──────────────────────────────────────────╮
│ >                                        │
╰──────────────────────────────────────────╯
  ? for shortcuts
`)
	if tr.Kind != None || tr.Line != "The cache was cold. It is warm now." {
		t.Fatalf("verdict = (%q, %v)", tr.Line, tr.Kind)
	}
	for _, h := range tr.Hits {
		switch {
		case h.Rule == RuleFallback && (h.Line == 0 || tr.Lines[h.Line-1] != tr.Line || !h.Decides):
			t.Errorf("fallback hit = %+v, want the line %q, deciding", h, tr.Line)
		case h.Rule != RuleFallback && h.Decides:
			t.Errorf("%s hit %+v decides a screen the fallback read", h.Rule, h)
		}
	}
}

// TestExplainShowsTheFallbackAboveTheWorkingLine: the fallback may read
// above the line that says the agent works; Lines then start there.
func TestExplainShowsTheFallbackAboveTheWorkingLine(t *testing.T) {
	tr := Explain(`
export TOKEN=sk-live-123
• Should I also update the docs?
  Running the suite again.
• Working (12s • esc to interrupt)

› Ask Codex to do anything
  ? for shortcuts · 100% context left
`)
	if tr.Line != "Running the suite again." || tr.Lines[0] != tr.Line {
		t.Errorf("Line %q, Lines %q: want Lines from the fallback line", tr.Line, tr.Lines)
	}
	for _, h := range tr.Hits {
		if (h.Rule == RuleWorking || h.Rule == RuleFallback) && h.Line == 0 {
			t.Errorf("%s has no line in %q", h.Rule, tr.Lines)
		}
	}
}
