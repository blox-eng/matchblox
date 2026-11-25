package service

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// paneLine is one line of what list-panes -a prints in mux.PaneFormat.
func paneLine(id, target string, pid int, tty string, dead bool) string {
	d := "0"
	if dead {
		d = "1"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%d\tsleep\t/\t\t\t\t\t%s\t%s\n", id, target, "win", pid, tty, d)
}

// startedPanes makes s.TmuxOut list one pane %42 for each session a new-session
// step made: the pane a service finds after it starts a session. Call it
// after the test sets s.Actions.Run.
func startedPanes(s *Service) {
	var mu sync.Mutex
	var names []string
	inner := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		if i := slices.Index(argv, "new-session"); i >= 0 {
			if j := slices.Index(argv[i:], "-s"); j >= 0 && i+j+1 < len(argv) {
				mu.Lock()
				names = append(names, argv[i+j+1])
				mu.Unlock()
			}
		}
		return inner(argv)
	}
	s.TmuxOut = func([]string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		var b strings.Builder
		for _, n := range names {
			b.WriteString(paneLine("%42", n+":1.1", 1, "/dev/pts/9", false))
		}
		return b.String(), nil
	}
}

// The stoker's multiplexer is the stoker's server, even when the service's
// own is a fake: a step for the stoker never reaches the builder's server.
func TestStokerMuxIsTheStokersServer(t *testing.T) {
	s := &Service{StokerServer: "mbtest"}
	if got, want := s.stokerMux().KillSession("stoker"), []string{"tmux", "-L", "mbtest", "kill-session", "-t", "=stoker"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := s.mux().KillSession("api"), []string{"tmux", "kill-session", "-t", "=api"}; !slices.Equal(got, want) {
		t.Errorf("the builder's server: got %q, want %q", got, want)
	}
	if got, want := (&Service{}).stokerMux().HasSession("stoker"), []string{"tmux", "-L", "matchblox", "has-session", "-t", "=stoker"}; !slices.Equal(got, want) {
		t.Errorf("no server set: got %q, want %q", got, want)
	}
}

// The pane of a peer is the pane whose first process is the peer or one of
// its parents, on the builder's server or the stoker's. A jump's pane never
// holds it.
func TestPaneOfPeerReadsBothServers(t *testing.T) {
	s := newTest(t, 0)
	s.StokerServer = "mbtest"
	me := os.Getpid()
	s.TmuxOut = func(argv []string) (string, error) {
		if slices.Contains(argv, "mbtest") {
			return paneLine("%9", "stoker:0.0", me, "/dev/pts/2", false), nil
		}
		return paneLine("%1", "app:0.0", os.Getppid(), "/dev/pts/1", false), nil
	}
	if got := s.paneOfPeer(me); got != "%9@mbtest" {
		t.Errorf("a peer in the stoker's pane: %q", got)
	}
	s.TmuxOut = func(argv []string) (string, error) {
		if slices.Contains(argv, "mbtest") {
			return "", nil
		}
		jump := "%3\tjump:0.0\twin\t" + fmt.Sprint(me) + "\tssh\t/\t\t\t1\t\t/dev/pts/3\t0\n"
		return paneLine("%2", "app:0.0", me, "/dev/pts/1", false) + jump, nil // the jump is last: it would win
	}
	if got := s.paneOfPeer(me); got != "%2" {
		t.Errorf("a peer in the builder's pane, with a jump listed: %q", got)
	}
}

// A new session's pane is the one of the session of that exact name: a
// session with the same prefix is not it.
func TestPaneOfSessionIsOfThatName(t *testing.T) {
	s := newTest(t, 0)
	s.TmuxOut = func([]string) (string, error) {
		return paneLine("%1", "fresh-2:0.0", 1, "/dev/pts/1", false) + paneLine("%5", "fresh:0.0", 5, "/dev/pts/5", false), nil
	}
	if got := s.paneOfSession("fresh"); got != "%5" {
		t.Errorf("got %q, want %%5", got)
	}
	if got := s.paneOfSession("gone"); got != "" {
		t.Errorf("a session that is not there: %q", got)
	}
}
