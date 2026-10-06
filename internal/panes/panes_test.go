package panes

import (
	"reflect"
	"testing"
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
