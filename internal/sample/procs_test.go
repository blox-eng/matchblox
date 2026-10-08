package sample

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// TestTopIsStableOnTies: processes with the same CPU keep one order, so
// the list does not reshuffle on every sample.
func TestTopIsStableOnTies(t *testing.T) {
	procs := map[int]procfs.Proc{}
	for pid := 100; pid < 140; pid++ {
		procs[pid] = procfs.Proc{PID: pid, Comm: "idle"}
	}
	for range 20 {
		var snap Snapshot
		s := newFixtureSampler(t.TempDir())
		s.owners = map[procKey]string{}
		s.sampleProcs(&snap, procs, nil, 2, time.Unix(0, 0))
		if len(snap.Top) != topN {
			t.Fatalf("%d rows", len(snap.Top))
		}
		for i, p := range snap.Top {
			if p.PID != 100+i {
				t.Fatalf("row %d is pid %d, want %d", i, p.PID, 100+i)
			}
		}
	}
}
