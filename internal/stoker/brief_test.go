package stoker

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// TestBriefScopesToChain: a chain stoker's brief names its chain, and
// shows only the chain's members under the machine.
func TestBriefScopesToChain(t *testing.T) {
	board := Board{At: time.Date(2026, 11, 7, 22, 0, 0, 0, time.UTC), StokerServer: "mb", Sessions: []sample.Session{
		{Agent: "claude", Pane: "%3", Name: "api-auth", Status: "idle"},
		{Agent: "claude", Pane: "%4", Name: "api-db", Status: "idle"},
		{Agent: "claude", Pane: "%9", Name: "www", Status: "idle"},
	}}
	host := Brief(Order{ID: "run-1", Words: "keep going"}, "ways.md", Hands, board)
	if !strings.Contains(host, "www (%9)") || strings.Contains(host, "You are the stoker of chain") {
		t.Fatalf("the host's brief changed:\n%s", host)
	}
	board.Chain = &BriefChain{Name: "api-auth", Repo: "/dev/api", Members: []string{"api-auth (%3)", "api-db (%4)"}}
	got := Brief(Order{ID: "run-2", Words: "merge on green"}, "ways.md", Hands, board)
	for _, want := range []string{"You are the stoker of chain api-auth. You act on its matches only.", "- api-auth (%3):", "- api-db (%4):", "/dev/api"} {
		if !strings.Contains(got, want) {
			t.Errorf("the brief lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "www") {
		t.Fatalf("the brief names a session outside the chain:\n%s", got)
	}
}

// The host's brief lists each chain, its stoker (or none) and that
// stoker's plan, and how to give a chain an order.
func TestHostBriefListsChains(t *testing.T) {
	board := Board{At: time.Date(2026, 11, 7, 22, 0, 0, 0, time.UTC), Chains: []ChainRow{
		{Name: "api-auth", StokerID: "s1", State: "night", PlanDone: 1, PlanAll: 3},
		{Name: "www", StokerID: "s2", State: "off"},
		{Name: "docs"},
	}}
	got := Brief(Order{ID: "run-1", Words: "keep going", Night: true}, "ways.md", Hands, board)
	for _, want := range []string{
		"## The chains",
		"- api-auth: stoker s1 · night · plan 1/3",
		"- www: stoker s2 · off · no plan",
		"- docs: no stoker",
		`Give a chain an order: matchblox stoker order <chain> "<text>"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the brief lacks %q:\n%s", want, got)
		}
	}
	board.Chains = nil
	if got := Brief(Order{ID: "run-1"}, "ways.md", Hands, board); strings.Contains(got, "chain") {
		t.Fatalf("a brief with no chains names one:\n%s", got)
	}
}
