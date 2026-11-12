package push

import (
	"encoding/json"
	"testing"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// hearth: ember asks, cinder burns; a stoker with a plan of 3, 1 done.
func hearthDoc() state.Doc {
	d := doc([]sample.Session{sess("ember", false, t0), sess("cinder", true, t0), sess("loose", true, t0)},
		[]queue.Item{{SessionID: "ember", Name: "ember-billing", State: queue.StatePermission, Since: t0, LastLine: "merge #4552?"}})
	d.Chains = &state.ChainsView{Chains: []chains.Chain{
		{ID: "%1#1", Name: "hearth", Members: []chains.Member{{SessionID: "ember"}, {SessionID: "cinder"}}},
		{ID: "%9#2", Name: "kindle", Members: []chains.Member{{SessionID: "loose-not-here"}}},
	}}
	v := stoker.View{}
	v.On = true
	v.Order = &stoker.Order{State: stoker.OrderRunning, Plan: []stoker.PlanStep{{N: 1}, {N: 2}, {N: 3}}}
	d.Stokers = []state.ChainStoker{{ID: "u1", Chain: "%1#1", Name: "hearth", View: v}}
	return d
}

func TestBuildPicksTheNeediestChain(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := hearthDoc()
	_, all := d.Stokers[0].View.PlanProgress()
	got := Build(d, fp, n, t0)
	if got.Chain == nil {
		t.Fatal("no chain")
	}
	want := TopChain{N: n.Chain("%1#1"), State: "asks", Steps: all, Stokers: 1}
	want.Done, _ = d.Stokers[0].View.PlanProgress()
	if *got.Chain != want || want.N == 0 || want.Steps != 3 {
		t.Fatalf("chain = %+v, want %+v", *got.Chain, want)
	}
}

func TestAChainOfOneHasNoChain(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := doc([]sample.Session{sess("loose", false, t0)},
		[]queue.Item{{SessionID: "loose", State: queue.StateQuestion, Since: t0}})
	if got := Build(d, fp, n, t0); got.Chain != nil || got.Top == nil {
		t.Fatalf("chain %+v top %+v: a session in no chain shows as today", got.Chain, got.Top)
	}
}

func TestABusyChainBurns(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	d := hearthDoc()
	d.Queue = nil
	if got := Build(d, fp, n, t0); got.Chain == nil || got.Chain.State != "burns" {
		t.Fatalf("chain = %+v", got.Chain)
	}
}

func TestAnOldPayloadDecodesWithoutChain(t *testing.T) {
	var cs ContentState
	if err := json.Unmarshal([]byte(`{"host":"f10f7812","burn":1,"wait":0,"accounts":[],"stoker":false}`), &cs); err != nil || cs.Chain != nil {
		t.Fatalf("cs %+v err %v", cs, err)
	}
	b, _ := json.Marshal(ContentState{Accounts: []AccountUse{}})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["chain"]; ok {
		t.Fatalf("an empty chain is sent: %s", b)
	}
}

func TestNamesIncludesChains(t *testing.T) {
	n := openNumbers(t, t.TempDir())
	_, _, cs := n.Names(nil, nil, hearthDoc().Chains.Chains)
	if cs[n.Chain("%1#1")] != "hearth" || cs[n.Chain("%9#2")] != "kindle" || len(cs) != 2 {
		t.Fatalf("chains = %v", cs)
	}
	m := openNumbers(t, n.dir)
	if m.Chain("%1#1") != n.Chain("%1#1") {
		t.Fatal("a chain's number is not kept")
	}
}

func TestBuzzNamesTheChain(t *testing.T) {
	d := hearthDoc()
	if got, want := BuzzLine("ws-1", d.Queue[0], d.Chains), `ws-1 · hearth · ember-billing asks: "merge #4552?"`; got != want {
		t.Fatalf("buzz = %q, want %q", got, want)
	}
	loose := queue.Item{SessionID: "loose", Name: "loose", State: queue.StateQuestion}
	if got, want := BuzzLine("ws-1", loose, d.Chains), "ws-1 · loose waits"; got != want {
		t.Fatalf("buzz = %q, want %q", got, want)
	}
}
