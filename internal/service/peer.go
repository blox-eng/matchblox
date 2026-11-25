package service

import (
	"errors"
	"os/exec"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/sample"
)

// unplacedSender refuses a send whose sender has neither a pane nor a
// terminal: a process that left its agent's tree with setsid or nohup
// would pass for the person.
const unplacedSender = "not sent: matchblox cannot tell who sends this. Send from a terminal, a tmux pane or the console."

// unplacedAnswerer refuses an answer or a pick from such a process.
const unplacedAnswerer = "not answered: matchblox cannot tell who answers this. Answer from a terminal, a tmux pane or the console."

// unplacedMerger refuses a merge from such a process: orderable's stoker
// check needs a pane to read, so a caller with none would skip it.
const unplacedMerger = "not merged: matchblox cannot tell who merges this. Merge from a terminal, a tmux pane or the console."

// callerPane is the pane of the client process pid: its process tree's,
// else its terminal's when an agent runs there, for an agent's process
// that left the tree. "" for a person's terminal outside every agent's
// pane. placed is false when pid has no pane and no terminal to tell.
func (s *Service) callerPane(pid int) (pane string, placed bool) {
	if p := s.PeerPane(pid); p != "" {
		return p, true
	}
	if s.TTYPane == nil {
		return "", false
	}
	p, ok := s.TTYPane(pid)
	if !ok {
		return "", false
	}
	if _, agent := s.sessionAt(p); agent {
		return p, true
	}
	return "", true
}

// ttyDevice is a terminal's device, by its major and minor numbers.
type ttyDevice struct{ major, minor uint32 }

// ttyDev splits a tty_nr of /proc/<pid>/stat into its major and minor
// numbers, as the kernel packs them.
func ttyDev(nr uint32) ttyDevice {
	return ttyDevice{major: (nr >> 8) & 0xfff, minor: nr&0xff | (nr>>12)&0xfff00}
}

// paneOfTTY is the tmux pane whose terminal is pid's controlling terminal,
// read from the system.
func (s *Service) paneOfTTY(pid int) (string, bool) {
	return s.paneOfTerminal(pid, ttyOf, devOf)
}

// paneOfTerminal is the tmux pane, on the builder's server or the
// stoker's, whose terminal (dev of its #{pane_tty}) is pid's controlling
// terminal (tty), or else the pane of the agent that runs on that terminal:
// "" for none, as when tmux runs no server. ok is false
// when pid has no terminal, or tmux fails another way: then nothing
// tells whose the terminal is.
func (s *Service) paneOfTerminal(pid int, tty func(int) (ttyDevice, bool), dev func(string) (ttyDevice, bool)) (pane string, ok bool) {
	want, ok := tty(pid)
	if !ok {
		return "", false
	}
	for _, m := range s.paneMuxes() {
		listed, err := m.ListStrict()
		if err != nil {
			if noTmuxServer(err) {
				continue
			}
			return "", false
		}
		for _, p := range listed {
			if d, ok := dev(p.TTY); ok && d == want {
				return p.ID, true
			}
		}
	}
	// An agent behind matchblox's redraw guard (#399) runs on the guard's
	// terminal, which is no pane's: a process on it belongs to that agent's
	// pane, also after it left the agent's tree.
	s.mu.Lock()
	sessions := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	for _, x := range sessions {
		if d, ok := tty(x.PID); ok && d == want && x.Pane != "" {
			return x.Pane, true
		}
	}
	return "", true
}

// paneMuxes are the multiplexers whose panes a peer can run in: the
// builder's, then the stoker's when the service keeps a stoker server.
func (s *Service) paneMuxes() []mux.Mux {
	if s.StokerServer == "" {
		return []mux.Mux{s.mux()}
	}
	return []mux.Mux{s.mux(), s.stokerMux()}
}

// noTmuxServer tells if tmux is not installed: then no pane holds any
// terminal. A tmux with no server is an empty list, not an error; any other
// failure is an error, and then nothing tells whose the terminal is.
func noTmuxServer(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

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
	for _, m := range s.paneMuxes() {
		listed, _ := m.List()
		for _, p := range listed {
			if session, _, _ := strings.Cut(p.Target, ":"); p.Jump == "1" || sample.JumpSession(session) {
				continue // a jump's pane holds the ssh, not the peer
			}
			panes = append(panes, sample.Pane{ID: p.ID, PID: p.PID})
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

// PaneOfProcess is the pane whose first process is pid or one of its
// parents, read from the system: what PeerPane does with the panes tmux
// lists.
func PaneOfProcess(pid int, panes []sample.Pane) string {
	return paneOfPID(pid, parentOf, panes)
}
