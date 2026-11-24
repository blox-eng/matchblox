package app

import (
	"slices"
	"strings"
	"testing"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// TestTheSheetHasEveryKey: every binding in main, board and side appears
// once in its group. A binding with no help text fails and names it.
func TestTheSheetHasEveryKey(t *testing.T) {
	k := defaultKeys()
	groups := sheetGroups(k)
	wantNames := []string{"main", "board", "side"}
	gotNames := make([]string, len(groups))
	for i, g := range groups {
		gotNames[i] = g.name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("groups %q, want %q", gotNames, wantNames)
	}
	owned := map[string]bool{} // group\x00help key\x00desc → a binding owns it
	for _, layer := range []struct {
		name   string
		binds  []bkey.Binding
		fields []string
	}{
		{"main", k.main.bindings(), fieldNames(k.main)},
		{"board", k.board.bindings(), fieldNames(k.board)},
		{"side", k.side.bindings(), fieldNames(k.side)},
	} {
		g := groups[slices.IndexFunc(groups, func(g keyGroup) bool { return g.name == layer.name })]
		for i, b := range layer.binds {
			name := layer.fields[i]
			if b.Help().Desc == "" {
				t.Errorf("%s.%s has no help text", layer.name, name)
				continue
			}
			id := layer.name + "\x00" + b.Help().Key + "\x00" + b.Help().Desc
			owned[id] = true
			if !slices.ContainsFunc(g.rows, func(r keyRow) bool {
				return r.bind.Help().Key == b.Help().Key && r.bind.Help().Desc == b.Help().Desc
			}) {
				t.Errorf("%s.%s (%s %s) is missing from the sheet's %s group", layer.name, name, b.Help().Key, b.Help().Desc, layer.name)
			}
		}
		seenRow := map[string]bool{}
		for _, r := range g.rows {
			id := layer.name + "\x00" + r.bind.Help().Key + "\x00" + r.bind.Help().Desc
			if seenRow[id] {
				t.Errorf("%s %s appears twice in %s", r.bind.Help().Key, r.bind.Help().Desc, layer.name)
			}
			seenRow[id] = true
			if !owned[id] {
				t.Errorf("the %s group has %s %s, which no %s binding owns", layer.name, r.bind.Help().Key, r.bind.Help().Desc, layer.name)
			}
		}
	}
}

// TestTypingFiltersTheSheet: typing "shell" leaves one row; ⏎ runs it.
func TestTypingFiltersTheSheet(t *testing.T) {
	m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(36), false)
	m = pressOn(m, "?")
	if m.over != "keys" {
		t.Fatal("? opened no keys sheet")
	}
	m = typeText(m, "shell").(Model)
	plain := ansi.Strip(m.render())
	if !strings.Contains(plain, "t") || !strings.Contains(plain, "a shell under the agent") {
		t.Fatalf("filter shell does not show t a shell under the agent:\n%s", plain)
	}
	if strings.Count(plain, "a shell under the agent") != 1 {
		t.Fatalf("filter shell left more than one shell row:\n%s", plain)
	}
	if strings.Contains(plain, "machine") {
		t.Fatalf("filter shell still shows machine:\n%s", plain)
	}
	m = pressOn(m, "enter")
	if m.over != "" {
		t.Fatalf("⏎ left the sheet open (%q); want it to run t and close", m.over)
	}
}

// TestNoKeyMatches: a miss says so; esc clears, then closes.
func TestNoKeyMatches(t *testing.T) {
	m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(36), false)
	m = typeText(pressOn(m, "?"), "xyz").(Model)
	plain := ansi.Strip(m.render())
	const miss = `No key does "xyz". esc clears it.`
	if !strings.Contains(plain, miss) {
		t.Fatalf("miss reads %q, want %q", plain, miss)
	}
	m = pressOn(m, "esc")
	if m.over != "keys" {
		t.Fatalf("esc closed the sheet; want it to clear the filter first")
	}
	plain = ansi.Strip(m.render())
	if strings.Contains(plain, `No key does "xyz"`) || strings.Contains(plain, "xyz") {
		t.Fatalf("esc left the filter:\n%s", plain)
	}
	m = pressOn(m, "esc")
	if m.over != "" {
		t.Fatalf("second esc left the sheet %q", m.over)
	}
}

// keyRecorder notes each key the agent's screen takes. It does not forward
// to the real screen: sendKey writes a pipe nobody reads here.
type keyRecorder struct {
	agentScreen
	got []string
}

func (r *keyRecorder) sendKey(k uv.KeyPressEvent) {
	r.got = append(r.got, tea.KeyPressMsg(k).String())
}

// TestQuestionMarkTypesInTheAgent: with the agent's keys, ? reaches the agent.
func TestQuestionMarkTypesInTheAgent(t *testing.T) {
	m := withAgent(t, mockDesk(t, 160, 40, true), agentRows(10), true)
	rec := &keyRecorder{agentScreen: m.agent.term.scr}
	m.agent.term.scr = rec
	m = pressOn(m, "?")
	if m.over == "keys" {
		t.Fatal("? opened the keys sheet with the agent's keys")
	}
	if !slices.Contains(rec.got, "?") {
		t.Fatalf("? did not reach the agent: got %q", rec.got)
	}
}
