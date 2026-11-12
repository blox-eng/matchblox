package app

import (
	"slices"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/logos"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
)

// feels are the feel signals a state wrote.
func feels(t *testing.T, m Model, st proto.State) []string {
	t.Helper()
	_, cmd := m.Update(stateMsg(st))
	var out []string
	for _, r := range raws(cmd) {
		for _, f := range []string{keycmd.FeelNeeds, keycmd.FeelDone} {
			if r == logos.Passthrough(f) {
				out = append(out, f)
			}
		}
	}
	return out
}

func TestConsoleSignalsAFeelOnAStateChange(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := headModel(t, 160, st)
	asks := st
	asks.Queue = []queue.Item{{Pane: "%1", Name: "hearth-api", State: queue.StatePermission, Since: demoAt}}
	if got := feels(t, m, asks); !slices.Equal(got, []string{keycmd.FeelNeeds}) {
		t.Fatalf("a match that starts to ask felt %q", got)
	}
	done := st
	done.Queue = []queue.Item{{Pane: "%1", Name: "hearth-api", State: queue.StateFinished, Since: demoAt}}
	if got := feels(t, m, done); !slices.Equal(got, []string{keycmd.FeelDone}) {
		t.Fatalf("a match that finished felt %q", got)
	}
	both := st
	both.Queue = append(slices.Clone(asks.Queue), queue.Item{Pane: "%8", Name: "ember-billing", State: queue.StateFinished, Since: demoAt})
	if got := feels(t, m, both); !slices.Equal(got, []string{keycmd.FeelNeeds}) {
		t.Fatalf("an ask and a finish in one sample felt %q, want one needs", got)
	}
}

func TestNoFeelForBusyIdleChurn(t *testing.T) {
	st := headState(t, runningPlan(), true)
	st.Queue = nil
	m, _ := headModel(t, 160, st)
	churn := st
	churn.Sessions = slices.Clone(st.Sessions)
	for i := range churn.Sessions {
		churn.Sessions[i].Busy = !churn.Sessions[i].Busy
		churn.Sessions[i].Idle = time.Minute
	}
	if got := feels(t, m, churn); len(got) != 0 {
		t.Fatalf("busy and idle churn felt %q", got)
	}
	asks := st
	asks.Queue = []queue.Item{{Pane: "%1", Name: "hearth-api", State: queue.StatePermission, Since: demoAt}}
	m2, _ := headModel(t, 160, asks)
	if got := feels(t, m2, asks); len(got) != 0 {
		t.Fatalf("an ask that was there already felt %q", got)
	}
}
