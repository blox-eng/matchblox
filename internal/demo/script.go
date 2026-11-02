package demo

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// hostName is what the header says of the machine.
const hostName = "demo"

// fixtureNow is the time the fixture's files were written at: the machine's
// clock when the demo starts. The agents' ages are counted from it.
var fixtureNow = time.Date(2026, 10, 1, 22, 50, 0, 0, time.UTC)

// The agent that waits at the top, and what it says when its turn ends.
const (
	askPane   = "%2"
	askTokens = 66000
	askText   = "Rotated on each refresh, with the old token kept a minute. The tests pass."
)

// turnLasts is how long the agent works on the answer.
var turnLasts = 3 * time.Second

// clockFrom is the fixture's time plus the time since start, as the replay's
// clock runs.
func clockFrom(start time.Time) func() time.Time {
	return func() time.Time { return fixtureNow.Add(time.Since(start)) }
}

// nameHost writes the name the demo shows to the temp copy. The fixture
// itself keeps its own, which its goldens use.
func nameHost(root string) error {
	return os.WriteFile(filepath.Join(root, "proc", "sys", "kernel", "hostname"), []byte(hostName+"\n"), 0o644)
}

// movingFS is the fixture's /proc with an uptime that runs, so the age of a
// process grows as the clock does.
type movingFS struct {
	procfs.FS
	start time.Time
}

func (f movingFS) Uptime() float64 { return f.FS.Uptime() + time.Since(f.start).Seconds() }

// script is the demo's one task: answer an agent, and its turn ends.
type script struct {
	world *World

	mu      sync.Mutex
	armed   bool
	pane    string
	stopped bool
	timer   *time.Timer
	running sync.WaitGroup
	done    atomic.Bool
}

// answered is World.Picked: the agent's turn ends a while after the answer.
// The first answer to any session counts, a pick on a menu or a typed reply.
func (s *script) answered(pane string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.armed || s.stopped {
		return
	}
	s.armed = true
	s.pane = pane
	s.timer = time.AfterFunc(turnLasts, s.finish)
}

func (s *script) finish() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.running.Add(1)
	pane := s.pane
	s.mu.Unlock()
	defer s.running.Done()
	tokens := 0
	if pane == askPane {
		tokens = askTokens
	}
	if s.world.Finish(pane, tokens, askText) == nil {
		s.done.Store(true)
	}
}

// stop ends the script: no turn finishes after it returns.
func (s *script) stop() {
	s.mu.Lock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	s.running.Wait()
}
