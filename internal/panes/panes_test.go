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

// TestKeysToTheStokersServerGoThere: a pane of matchblox's own server is
// typed into on that server, by the id it knows it by; never the
// builder's pane of the same id.
func TestKeysToTheStokersServerGoThere(t *testing.T) {
	got := Send("%0@matchblox-test", "carry on")
	want := [][]string{
		{"tmux", "-L", "matchblox-test", "send-keys", "-t", "%0", "-l", "--", "carry on"},
		{"tmux", "-L", "matchblox-test", "send-keys", "-t", "%0", "Enter"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	ask := said.Ask{Questions: []said.Question{{Question: "Go?", Options: []said.Option{{Label: "yes"}, {Label: "no"}}}}}
	steps, _ := Pick("%0@matchblox-test", ask, said.Answer{Choices: []said.Choice{{Picks: []int{1}}}})
	if want := []string{"tmux", "-L", "matchblox-test", "send-keys", "-t", "%0", "1"}; !reflect.DeepEqual(steps[0], want) {
		t.Fatalf("pick: got %q", steps)
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
		name  string
		ask   said.Ask
		ans   said.Answer
		want  [][]string
		pages []int
	}{
		{"one choice", said.Ask{Questions: []said.Question{{Options: opts(3)}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{2}}}},
			[][]string{key("2")}, []int{0}},
		{"one choice, typed", said.Ask{Questions: []said.Question{{Options: opts(3)}}},
			said.Answer{Choices: []said.Choice{{Text: "Plovdiv"}}},
			[][]string{key("4"), Pause, key("-l", "--", "Plovdiv"), key("Enter")}, []int{0}},
		{"several choices", said.Ask{Questions: []said.Question{{Options: opts(3), Multi: true}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{1, 3}}}},
			[][]string{key("1"), key("3"), key("Down", "Down", "Down", "Down"), key("Enter"), Pause, key("1")}, []int{0, 5}},
		{"two questions, a tick and a typed line, then one choice", said.Ask{Questions: []said.Question{{Options: opts(3), Multi: true}, {Options: opts(2)}}},
			said.Answer{Choices: []said.Choice{{Picks: []int{2}, Text: "zinc"}, {Picks: []int{2}}}},
			[][]string{key("2"), key("Down", "Down", "Down"), key("-l", "--", "zinc"), key("Down"), key("Enter"), Pause,
				key("2"), Pause, key("1")}, []int{0, 6, 8}},
	} {
		got, pages := Pick("%3", c.ask, c.ans)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		if !reflect.DeepEqual(pages, c.pages) {
			t.Errorf("%s: pages %v, want %v", c.name, pages, c.pages)
		}
	}
}
