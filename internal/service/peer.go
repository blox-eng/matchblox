package service

import (
	"github.com/shirou/gopsutil/v4/process"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/procfs"
	"github.com/blox-eng/matchblox/internal/sample"
)

// paneOfPeer is the tmux pane the process pid runs in, by its parents and
// the pid tmux started in each pane: the panes of the last sample, then
// tmux read now for a pane opened since, on the builder's server and on
// the stoker's (its ids carry the server). "" outside every pane.
func (s *Service) paneOfPeer(pid int) string {
	s.mu.Lock()
	panes := s.cur.Panes
	s.mu.Unlock()
	if p := paneOfPID(pid, parentOf, panes); p != "" {
		return p
	}
	out, _ := sample.TmuxPanes()
	panes = sample.ParsePanes(out)
	if s.StokerServer != "" {
		out, _ := sample.TmuxPanesOn(s.StokerServer)
		for _, p := range sample.ParsePanes(out) {
			p.ID = sample.OnServer(p.ID, s.StokerServer)
			panes = append(panes, p)
		}
	}
	return paneOfPID(pid, parentOf, panes)
}

// onServer names the pane of a hook on the stoker's own server (sock)
// with that server: its $TMUX_PANE alone is a pane id the builder's
// server has too.
func (s *Service) onServer(ev hooks.Event, sock string) hooks.Event {
	if server, _ := sample.SplitPane(ev.Pane); s.StokerServer != "" && server == "" && ev.Pane != "" && ev.Socket != "" && ev.Socket == sock {
		ev.Pane = sample.OnServer(ev.Pane, s.StokerServer)
	}
	return ev
}

// hookFrom tells if a hook event comes from the agent it names. A hook
// runs inside its agent's process tree, so the pane of its pid is the
// event's pane, and the session it names is not one the sample or the
// queue places in another pane. Any other event, the stoker's above all,
// would set a session's state it has no word in.
func (s *Service) hookFrom(pid int, ev hooks.Event) bool {
	if s.PeerPane(pid) != ev.Pane {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.SessionID != "" {
		for _, x := range s.sampled {
			if x.SessionID == ev.SessionID {
				return x.Pane == ev.Pane
			}
		}
	}
	p := s.queue.Pane(ev.SessionID)
	return p == "" || p == ev.Pane
}

// paneOfPID is the pane whose first process is pid or one of its parents.
func paneOfPID(pid int, parent func(int) (int, bool), panes []sample.Pane) string {
	first := make(map[int]string, len(panes))
	for _, p := range panes {
		if p.PID > 0 {
			first[p.PID] = p.ID
		}
	}
	for cur, hops := pid, 0; cur > 1 && hops < 64; hops++ {
		if id, ok := first[cur]; ok {
			return id
		}
		next, ok := parent(cur)
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

// parentOf is the parent pid of pid: from /proc, else from the system.
func parentOf(pid int) (int, bool) {
	if p, ok := (procfs.FS{Root: "/proc"}).Proc(pid); ok {
		return p.PPID, true
	}
	p, err := process.NewProcess(int32(pid)) //nolint:gosec // a pid fits in int32
	if err != nil {
		return 0, false
	}
	pp, err := p.Ppid()
	if err != nil {
		return 0, false
	}
	return int(pp), true
}

// PaneOfProcess is the pane whose first process is pid or one of its
// parents, read from the system: what PeerPane does with the panes tmux
// lists.
func PaneOfProcess(pid int, panes []sample.Pane) string {
	return paneOfPID(pid, parentOf, panes)
}
