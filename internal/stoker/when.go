package stoker

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

// RuleWhen marks a step a trigger fired: matchblox ran it itself, at the
// event the stoker named.
const RuleWhen = "when"

// TriggerStages are the stages a ship trigger waits for; red is CI failed.
var TriggerStages = []string{"ci", "green", "merged", "live", "red"}

// TriggerActs are the hands a trigger runs.
var TriggerActs = []string{"send", "compact", "release", "hold"}

// Trigger is one `matchblox stoker when`: an event (a ship that reaches a
// stage, or a session whose turn ends with nothing waiting there) and the
// one hand matchblox runs at it, as if the stoker ran it then. It fires
// once, and ends with the run.
type Trigger struct {
	ID string    `json:"id,omitempty"`
	At time.Time `json:"at,omitzero"` // registered: a turn that ended before it is not the event
	// Ship and Stage: the ship #Ship reaches Stage.
	Ship  int    `json:"ship,omitempty"`
	Stage string `json:"stage,omitempty"`
	// Rest and RestPane: the session whose turn ends.
	Rest     string `json:"rest,omitempty"`
	RestPane string `json:"rest_pane,omitempty"`
	// Do is the hand, Who and Pane the session it acts on, Text what a
	// send types or why a hold holds.
	Do   string `json:"do"`
	Who  string `json:"who"`
	Pane string `json:"pane,omitempty"`
	Text string `json:"text,omitempty"`
}

// Event is the trigger's event in words: "#4573 merged", "work rests".
func (t Trigger) Event() string {
	if t.Ship > 0 {
		return fmt.Sprintf("#%d %s", t.Ship, t.Stage)
	}
	return t.Rest + " rests"
}

// Action is the trigger's hand in words: "send Billed: carry on".
func (t Trigger) Action() string {
	switch t.Do {
	case "send":
		return "send " + t.Who + ": " + t.Text
	case "hold":
		return "hold " + t.Who + ": " + t.Text
	}
	return t.Do + " " + t.Who
}

// Done is the hand in the past, for the card: "sent Billed: carry on".
func (t Trigger) Done() string {
	switch t.Do {
	case "send":
		return "sent " + t.Who + ": " + t.Text
	case "compact":
		return "compacted " + t.Who
	case "release":
		return "released " + t.Who
	}
	return "held " + t.Who + ": " + t.Text
}

// Check refuses a trigger with no event or two, an unknown stage or hand,
// or a send or hold without its text.
func (t Trigger) Check() error {
	switch {
	case t.Ship > 0 == (t.Rest != ""):
		return fmt.Errorf("a trigger waits for one event: --ship <n> --stage <%s>, or --rest <session>", strings.Join(TriggerStages, "|"))
	case t.Ship > 0 && !slices.Contains(TriggerStages, t.Stage):
		return fmt.Errorf("%q is no stage: %s", t.Stage, strings.Join(TriggerStages, ", "))
	case t.Ship == 0 && t.Stage != "":
		return fmt.Errorf("--stage goes with --ship")
	case !slices.Contains(TriggerActs, t.Do):
		return fmt.Errorf("%q is no hand: send, compact, release or hold", t.Do)
	case t.Who == "":
		return fmt.Errorf("%s needs the session it acts on", t.Do)
	case t.Do == "send" && strings.TrimSpace(t.Text) == "":
		return fmt.Errorf("send needs its text")
	case t.Do == "hold" && strings.TrimSpace(t.Text) == "":
		return fmt.Errorf(`hold needs --why "<text>"`)
	}
	return nil
}

// Due tells if the trigger's event happened: the ship is at its stage or
// past it, or CI failed for red; the session's turn ended after the
// trigger was set and nothing waits there (waiting: the panes the queue
// lists as waiting for the builder).
func (t Trigger) Due(rep *ships.Report, sessions []sample.Session, waiting map[string]bool) bool {
	if t.Ship > 0 {
		if rep == nil {
			return false
		}
		for _, sh := range rep.Ships {
			if sh.Number == t.Ship && reached(sh, t.Stage) {
				return true
			}
		}
		return false
	}
	for _, x := range sessions {
		if x.Pane == t.RestPane {
			return x.TurnEnded.After(t.At) && x.Resting() && !waiting[x.Pane] && !x.Asks && !x.Permits
		}
	}
	return false
}

// reached tells if a ship is at a stage or past it; red is CI failed.
func reached(sh ships.Ship, stage string) bool {
	if stage == "red" {
		return sh.Kind == ships.Stuck && sh.Failed != ""
	}
	i := slices.Index(stageNames[:], stage)
	return i >= 0 && sh.Stage >= i
}

// Why is a hold's why as it is kept and shown: each control key a space,
// trimmed, not empty, and no longer than a line typed into a pane may be.
// A hold and a hold trigger take the same.
func Why(text string) (string, error) {
	why := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if why == "" {
		return "", errors.New(`a hold needs its why: --why "<text>"`)
	}
	if utf8.RuneCountInString(why) > panes.MaxAnswer {
		return "", fmt.Errorf("the why is longer than %d characters", panes.MaxAnswer)
	}
	return why, nil
}
