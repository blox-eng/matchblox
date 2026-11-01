package panes

import (
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestASemicolonStaysText: tmux reads an argv word that ends in ";" as the
// end of a command, so "ls;" would type "ls" and ";" nothing. Each literal
// word keeps its text whole.
func TestASemicolonStaysText(t *testing.T) {
	for text, want := range map[string]string{
		";":    `\;`,
		"ls;":  `ls\;`,
		"a;b;": `a;b\;`,
		`a\;`:  `a\\;`,
		"a;b":  "a;b",
	} {
		if got := Literal("%4", text); !reflect.DeepEqual(got, []string{"tmux", "send-keys", "-t", "%4", "-l", "--", want}) {
			t.Errorf("Literal(%q) = %q", text, got)
		}
		if got := Send("%4", text)[0]; got[len(got)-1] != want {
			t.Errorf("Send(%q) = %q", text, got)
		}
	}
	ask := said.Ask{Questions: []said.Question{{Question: "Go?", Options: []said.Option{{Label: "yes"}}}}}
	steps, _ := Pick("%4", ask, said.Answer{Choices: []said.Choice{{Text: "wait;"}}})
	if !hasWord(steps, `wait\;`) {
		t.Errorf("Pick lost the semicolon: %q", steps)
	}
}

func hasWord(steps [][]string, word string) bool {
	for _, s := range steps {
		if s[len(s)-1] == word {
			return true
		}
	}
	return false
}

// TestASemicolonReachesThePane types through a real tmux: what the pane
// shows is what was typed, every ";" kept.
func TestASemicolonReachesThePane(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", "s", "cat >/dev/null") // the tty echoes each line once
	pane := tmux("display-message", "-p", "-t", "s", "#{pane_id}")
	for _, text := range []string{";", "ls;", "a;b;", `a\;`} {
		step := Literal(pane+"@"+sock, text)
		if out, err := exec.Command(step[0], step[1:]...).CombinedOutput(); err != nil { //nolint:gosec // test argv
			t.Fatalf("%q: %v\n%s", step, err, out)
		}
		tmux("send-keys", "-t", pane, "Enter")
	}
	want := ";\nls;\na;b;\n" + `a\;`
	for deadline := time.Now().Add(3 * time.Second); !strings.HasPrefix(tmux("capture-pane", "-p", "-t", pane), want); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the pane shows %q, want %q", tmux("capture-pane", "-p", "-t", pane), want)
		}
	}
}
