package actions

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/host"
	"github.com/blox-eng/matchblox/internal/procfs"
)

// CheckGuard verifies, right before a step runs, what its evidence said.
func CheckGuard(g advice.Guard) error {
	if g.PID > 0 {
		p, ok := NewHost().Proc(g.PID)
		if !ok || p.StartTime != g.StartTicks {
			return fmt.Errorf("pid %d is no longer the process that was flagged", g.PID)
		}
	}
	if g.Worktree != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return gitscan.VerifyRemovable(ctx, gitscan.Git, g.Worktree, ProcessCwds())
	}
	return nil
}

// ProcessCwds is where every process of ours sits, so a worktree someone
// still works in is never offered for removal.
func ProcessCwds() []string {
	fs := NewHost()
	procs, _ := fs.Procs()
	out := make([]string, 0, len(procs))
	for pid := range procs {
		if cwd := fs.Cwd(pid); cwd != "" {
			out = append(out, cwd)
		}
	}
	return out
}

// NewHost reads /proc on Linux and the system's own APIs elsewhere.
func NewHost() procfs.Host {
	if runtime.GOOS == "linux" {
		return procfs.FS{Root: "/proc"}
	}
	return host.Host{}
}
