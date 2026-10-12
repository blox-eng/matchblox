package panes

import (
	"reflect"
	"testing"

	"github.com/blox-eng/matchblox/internal/said"
)

func TestTmuxSendIsLiteral(t *testing.T) {
	got := Send("%4", "; rm -rf /")
	want := [][]string{
		{"tmux", "send-keys", "-t", "%4", "-l", "--", "; rm -rf /"},
		{"tmux", "send-keys", "-t", "%4", "Enter"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

// A text that starts with a dash is text, not a tmux flag.
func TestTmuxSendDashText(t *testing.T) {
	if got := Send("%4", "-y"); got[0][5] != "--" || got[0][6] != "-y" {
		t.Fatalf("got %q", got)
	}
}

func TestAnswerText(t *testing.T) {
	for text, ok := range map[string]bool{
		"yes":                     true,
		"  ":                      false,
		"":                        false,
		"two\nlines":              false,
		"bell\a":                  false,
		string(make([]byte, 600)): false,
	} {
		if err := CheckAnswer(text); (err == nil) != ok {
			t.Errorf("%q: err %v", text, err)
		}
	}
}

// TestPickDrivesTheQuestionMenu pins the keys Claude Code's question tool
// takes, as a live pane showed them: a digit picks one choice, ticks one of
// several; the typing row is reached by its digit (one choice) or by Down
// (several); several questions end on the review page's "1".
func TestPickDrivesTheQuestionMenu(t *testing.T) {
	opts := func(n int) []said.Option { return make([]said.Option, n) }
	key := func(k ...string) []string { return append([]string{"tmux", "send-keys", "-t", "%3"}, k...) }
	for _, c := range []struct {
		name string
		ask  said.Ask
		ans  said.Answer
		want [][]string
	}{
		{"one choice", said.Ask{Questions: []said.Question{{Options: opts(3)}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{2}}}},
			[][]string{key("2")}},
		{"one choice, typed", said.Ask{Questions: []said.Question{{Options: opts(3)}}},
			said.Answer{Choices: []said.Choice{{Text: "Plovdiv"}}},
			[][]string{key("4"), Pause, key("-l", "--", "Plovdiv"), key("Enter")}},
		{"several choices", said.Ask{Questions: []said.Question{{Options: opts(3), Multi: true}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{1, 3}}}},
			[][]string{key("1"), key("3"), key("Down", "Down", "Down", "Down"), key("Enter"), Pause, key("1")}},
		{"two questions, a tick and a typed line, then one choice", said.Ask{Questions: []said.Question{{Options: opts(3), Multi: true}, {Options: opts(2)}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{2}, Text: "zinc"}, {Picks: []int{2}}}},
			[][]string{key("2"), key("Down", "Down", "Down"), key("-l", "--", "zinc"), key("Down"), key("Enter"), Pause,
				key("2"), Pause, key("1")}},
	} {
		if got := Pick("%3", c.ask, c.ans); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}
