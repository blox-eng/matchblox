package main

import (
	"os/exec"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
)

// orphanEvery is how often a second console checks that a terminal still
// has it.
const orphanEvery = 2 * time.Second

// listSessions is tmux's listing for panes.Orphaned.
func listSessions() (string, error) {
	out, err := exec.Command("tmux", panes.OrphanListArgv()[1:]...).Output()
	return string(out), err
}

// watchOrphan quits the console of the session name once no terminal has
// it, on two checks in a row: one is a terminal between two sessions. The
// console's exit ends its session and the agent views beside it; the
// agents run on. It ends with the quit, or when stop closes.
func watchOrphan(name string, list func() (string, error), quit func(), every time.Duration, stop <-chan struct{}) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	alone := 0
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		listing, err := list()
		if err != nil || !panes.Orphaned(name, listing) {
			alone = 0
			continue
		}
		if alone++; alone == 2 {
			quit()
			return
		}
	}
}
