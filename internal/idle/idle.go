// Package idle says why a quiet session is idle, in the few words its row
// shows.
package idle

import (
	"fmt"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

// Reason is a few words and the tone they are drawn in: accent, ok, warn,
// neg or faint. The zero Reason: the session is not quiet.
type Reason struct {
	Word string `json:"word"`
	Tone string `json:"tone"`
}

// Of is why a quiet session is idle: the first that holds of the ship it
// drives, the plan's limit, a failed turn, no turn yet, no work left. waits: the session
// asks the person a question or for permission, and has no reason to give. accts are the plan limits, which name the window
// that is out.
func Of(s sample.Session, asking bool, ship *ships.Ship, accts []limits.Account, now time.Time) Reason {
	if !atRest(s, asking) {
		return Reason{}
	}
	switch {
	case ship != nil && ship.Kind == ships.WaitsForYou && ship.Note == ships.NoteWaitsForMerge:
		return Reason{fmt.Sprintf("#%d waits your merge", ship.Number), "accent"}
	case ship != nil && ship.Kind == ships.Stuck && ship.Failed != "":
		return Reason{"CI red · " + ship.Failed, "neg"}
	case ship != nil && ship.Stage >= ships.StageMerged:
		return Reason{fmt.Sprintf("#%d merged · done", ship.Number), "ok"}
	case s.Stopped():
		return Reason{window(s, accts, now) + " limit · back " + limits.When(s.Out, now, ""), "warn"}
	case s.Failed != "":
		return Reason{"stuck: " + s.Failed, "neg"}
	case s.Context == "fresh":
		// No turn since it started or was cleared: no work to run out of.
		return Reason{"idle", "faint"}
	case s.Progress != nil && s.Progress.Pct >= 100, s.Progress == nil && !s.Asks:
		return Reason{"out of work", "faint"}
	}
	return Reason{"idle", "faint"}
}

// atRest: no turn runs, no background shell works, and the session does not
// ask the person anything.
func atRest(s sample.Session, asking bool) bool {
	return !s.Busy && !asking && !s.Asks && !s.Permits && s.Ask == nil && len(s.Shells) == 0
}

// window names the window of the session's account that is spent; the 5
// hours when none reads as spent.
func window(s sample.Session, accts []limits.Account, now time.Time) string {
	for _, a := range accts {
		if a.Provider != s.Provider || a.Account != s.Account {
			continue
		}
		for _, u := range a.Uses(now) {
			if u.Pct >= 100 {
				return strings.Replace(u.Name, "wk", "week", 1)
			}
		}
	}
	return "5h"
}
