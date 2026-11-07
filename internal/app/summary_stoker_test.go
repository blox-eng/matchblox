package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

func TestSummaryListsTheChainStokers(t *testing.T) {
	if out := Summary(state.Doc{}); strings.Contains(out, "chain ") {
		t.Fatalf("chain lines without a family:\n%s", out)
	}
	went := time.Now().Add(-time.Hour)
	var on stoker.View
	on.On = true
	on.Order = &stoker.Order{State: stoker.OrderRunning, Went: went, Plan: []stoker.PlanStep{{N: 1}, {N: 2}, {N: 3}}}
	on.Run = []stoker.Entry{{Step: 1, State: stoker.StepDone, At: went.Add(time.Minute)}, {Step: 2, State: stoker.StepDoing, At: went.Add(time.Minute)}}
	var d state.Doc
	d.Stokers = []state.ChainStoker{{ID: "s1", Chain: "c1", Name: "api", View: on}, {ID: "s2", Chain: "c2", Name: "web"}}
	out := Summary(d)
	for _, want := range []string{"  chain api (s1): on · plan 1/3\n", "  chain web (s2): off · plan 0/0\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
	}
}
