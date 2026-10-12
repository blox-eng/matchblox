package said

import (
	"errors"
	"fmt"
)

// AskTool is Claude Code's question tool: it shows a menu in the pane and
// waits for the person's pick.
const AskTool = "AskUserQuestion"

// Ask is a call of the question tool that waits for its answer, as the
// transcript holds its input.
type Ask struct {
	ID        string     `json:"id"` // the tool call's id
	Questions []Question `json:"questions"`
}

// Question is one question of a call: the tool shows them one at a time.
type Question struct {
	Question string   `json:"question"`
	Header   string   `json:"header,omitempty"`
	Options  []Option `json:"options"`
	Multi    bool     `json:"multiSelect,omitempty"`
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Answer is what the person picked for each question of a call.
type Answer struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

// Choice answers one question: the options picked (from 1), and a line
// typed into the menu's own field. A single choice has one of the two.
type Choice struct {
	Picks []int  `json:"picks,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Check refuses an answer that is not one of this call: another call, a
// question left out, an option it does not have, two picks where one is
// asked. The typed text is checked where it is typed.
func (a Ask) Check(ans Answer) error {
	if ans.ID != a.ID {
		return errors.New("the session asks another question now")
	}
	if len(ans.Choices) != len(a.Questions) {
		return fmt.Errorf("%d answers to %d questions", len(ans.Choices), len(a.Questions))
	}
	for i, c := range ans.Choices {
		q := a.Questions[i]
		seen := map[int]bool{}
		for _, p := range c.Picks {
			if p < 1 || p > len(q.Options) || seen[p] {
				return fmt.Errorf("question %d has no option %d", i+1, p)
			}
			seen[p] = true
		}
		n := len(c.Picks)
		if c.Text != "" {
			n++
		}
		switch {
		case n == 0:
			return fmt.Errorf("question %d has no answer", i+1)
		case !q.Multi && n > 1:
			return fmt.Errorf("question %d takes one answer", i+1)
		}
	}
	return nil
}
