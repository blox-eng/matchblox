package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
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
	if !strings.Contains(out, "stoker: off\n  chain api (s1)") {
		t.Fatalf("host off: no stoker: off before the chain lines:\n%s", out)
	}
	d.Stoker = &proto.StokerView{}
	d.Stoker.On = true
	if out := Summary(d); strings.Contains(out, "stoker: off") || !strings.Contains(out, "stoker: stoker\n  chain api") {
		t.Fatalf("host on:\n%s", out)
	}
	for _, want := range []string{"  chain api (s1): on · plan 1/3\n", "  chain web (s2): off · plan 0/0\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
	}
}
