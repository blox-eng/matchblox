package ships

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// The stages of a ship's track: open → ci → green → merged → live.
const (
	StageOpen = iota
	StageCI
	StageGreen
	StageMerged
	StageLive
)

// What the current stage asks.
const (
	Running     = "running"       // it moves on its own
	WaitsForYou = "waits-for-you" // a person takes the next step
	Stuck       = "stuck"         // it will not move until someone fixes it
	Done        = "done"          // live
	Unknown     = "unknown"       // GitHub could not be read
)

// Stage is one stage of the detail: when it was reached, and the note of
// the current one.
type Stage struct {
	At   time.Time `json:"at,omitzero"`
	Note string    `json:"note,omitempty"`
}

type trackOut struct {
	Stage  int
	Kind   string
	Note   string
	Stages [5]Stage
	LiveAt time.Time
}

// noRunsGrace is how long after a merge a commit with no workflow run is
// still waiting for its first one. Past it, nothing deploys: it is live.
const noRunsGrace = 5 * time.Minute

// track places a pull request on its track and says its state in words.
// deploy names the workflows that deploy; empty: every run.
func track(f facts, deploy []string, now time.Time) trackOut {
	var t trackOut
	t.Stages[StageOpen].At = f.Created
	t.Stages[StageCI].At = f.Checks.Started
	green := f.Checks.Total > 0 && f.Checks.Failed == 0 && f.Checks.Completed >= f.Checks.Total
	if green {
		t.Stages[StageGreen].At = f.Checks.Done
	}
	switch f.State {
	case "MERGED":
		t.Stages[StageMerged].At = f.Merged
		mergedTrack(&t, f, deploy, now)
	case "CLOSED":
		t.Stage, t.Kind, t.Note = StageOpen, Done, "closed"
	default:
		openTrack(&t, f, green)
	}
	t.Stages[t.Stage].Note = t.Note
	return t
}

func openTrack(t *trackOut, f facts, green bool) {
	switch {
	case green:
		t.Stage = StageGreen
	case f.Checks.Total == 0 && (f.MergeState == "CLEAN" || f.MergeState == "HAS_HOOKS" || f.MergeState == "UNSTABLE"):
		green, t.Stage = true, StageGreen // nothing to check, nothing blocks
	case f.Checks.Total == 0:
		t.Stage = StageOpen
	default:
		t.Stage = StageCI
	}
	base := f.Base
	if base == "" {
		base = "main"
	}
	switch {
	case f.Mergeable == "CONFLICTING" || f.MergeState == "DIRTY":
		t.Kind, t.Note = Stuck, "conflicts with "+base
	case f.Checks.Failed > 0:
		t.Kind, t.Note = Stuck, "CI failed"
		if f.Checks.FailedName != "" {
			t.Note += " · " + f.Checks.FailedName
		}
	case f.MergeState == "BEHIND":
		t.Kind, t.Note = Stuck, "behind "+base
	case f.Draft:
		t.Kind, t.Note = Running, "draft"
	case t.Stage == StageCI:
		t.Kind, t.Note = Running, fmt.Sprintf("CI · %d of %d jobs", f.Checks.Completed, f.Checks.Total)
	case !green:
		t.Kind, t.Note = Running, "no checks yet"
	case f.Review == "CHANGES_REQUESTED":
		t.Kind, t.Note = Stuck, "changes requested"
	case f.Review == "REVIEW_REQUIRED":
		t.Kind, t.Note = WaitsForYou, "waits for review"
	case f.MergeState == "BLOCKED":
		// Every check it has passed, no review holds it: a branch rule
		// still does, most often a required check that has not reported.
		t.Stage, t.Kind, t.Note = StageCI, Running, "waits for a required check"
	case f.Mergeable == "UNKNOWN" || f.Mergeable == "" || f.MergeState == "UNKNOWN":
		t.Kind, t.Note = Running, "checking mergeability"
	case f.AutoMerge:
		t.Kind, t.Note = Running, "auto-merge on"
	default:
		t.Kind, t.Note = WaitsForYou, "green · waits for your merge"
	}
}

// mergedTrack is live when every run on the merge commit finished and none
// failed. A cancelled run, or a skipped run of a named deploy workflow,
// counts when a newer run of its workflow on the default branch succeeded.
func mergedTrack(t *trackOut, f facts, deploy []string, now time.Time) {
	t.Stage = StageMerged
	runs, deploying, bad := 0, false, ""
	for _, r := range f.Runs {
		if len(deploy) > 0 && !slices.Contains(deploy, r.Workflow) {
			continue
		}
		runs++
		if r.Updated.After(t.LiveAt) {
			t.LiveAt = r.Updated
		}
		switch {
		case r.Status != "COMPLETED":
			deploying = true
		case r.Conclusion == "SUCCESS" || r.Conclusion == "NEUTRAL":
		case r.Conclusion == "SKIPPED" && len(deploy) == 0:
			// Every run counts, and a workflow that skips on push (one
			// triggered by comments) deploys nothing. A named deploy
			// workflow that skipped waits for a newer run to succeed.
		case r.Conclusion == "CANCELLED" || r.Conclusion == "SKIPPED":
			switch r.Later {
			case "success":
			case "running", "?":
				deploying = true
			default:
				if bad == "" {
					bad = "deploy " + strings.ToLower(r.Conclusion) + " · " + r.Workflow
				}
			}
		default:
			if bad == "" || !strings.HasPrefix(bad, "deploy failed") {
				bad = "deploy failed · " + r.Workflow
			}
		}
	}
	switch {
	case bad != "":
		t.Kind, t.Note = Stuck, bad
	case deploying || runs == 0 && now.Sub(f.Merged) < noRunsGrace:
		t.Kind, t.Note = Running, "merged · deploying"
	default:
		if t.LiveAt.IsZero() {
			t.LiveAt = f.Merged
		}
		t.Stage, t.Kind, t.Note = StageLive, Done, "live"
		t.Stages[StageLive].At = t.LiveAt
	}
}
