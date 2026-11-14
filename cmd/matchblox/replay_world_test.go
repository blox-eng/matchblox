package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blox-eng/matchblox/internal/replay"
	"github.com/blox-eng/matchblox/internal/testhome"
)

func TestPrivateFindsThisMachineInAFrame(t *testing.T) {
	frame := func(text string) []replay.Frame {
		return []replay.Frame{{Lines: [][]replay.Span{{{Text: text}}}}}
	}
	me := []string{"dev-3", "/home/someone"}
	for _, text := range []string{" dev-3 waits", "cwd /home/someone/work", "dev-3"} {
		if err := private(frame(text), me); err == nil {
			t.Errorf("%q passed", text)
		}
	}
	for _, text := range []string{" dev-30 waits", "devdev-3x", " ws-1 · hearth-api"} {
		if err := private(frame(text), me); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
}

// TestASessionNamedLikeAWordOfTheRepoIsNoLeak: a tmux session called
// "work" or "docs" names nothing of the machine, since the fixture and the
// console use those words themselves.
func TestASessionNamedLikeAWordOfTheRepoIsNoLeak(t *testing.T) {
	words, err := repoWords("../../internal", "../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	got := identifying([]string{"work", "web", "docs", "claude", "matchblox", "main", "dev-3", "Ledge Payments"}, words)
	if want := []string{"dev-3", "Ledge Payments"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// thisMachine is what names the machine the test runs on: its host name,
// user, home and git identity, and the name of each tmux session and
// Claude Code session on it.
func thisMachine() []string {
	var out []string
	add := func(n string) {
		if n = strings.TrimSpace(n); len(n) >= 3 {
			out = append(out, n)
		}
	}
	for _, k := range []string{"user.name", "user.email"} {
		if b, err := exec.Command("git", "config", k).Output(); err == nil {
			add(string(b))
		}
	}
	h, _ := os.Hostname()
	add(h)
	add(os.Getenv("USER"))
	home, _ := os.UserHomeDir()
	add(home)
	// The tests run off the builder's tmux servers (testhome.Run), so the
	// names come from those servers by socket.
	for _, sock := range testhome.BuildersTmux {
		if b, err := exec.Command("tmux", "-S", sock, "list-sessions", "-F", "#{session_name}").Output(); err == nil { //nolint:gosec // reads names only
			for _, n := range strings.Split(string(b), "\n") {
				add(n)
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(home, ".claude*", "sessions", "*.json"))
	for _, f := range files {
		var e struct{ Name string }
		if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &e) == nil {
			add(e.Name)
		}
	}
	return out
}

var word = regexp.MustCompile(`[\w-]+`)

// repoWords are the words of every file under dirs.
func repoWords(dirs ...string) (map[string]bool, error) {
	words := map[string]bool{}
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, w := range word.FindAll(b, -1) {
				words[string(w)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return words, nil
}

// identifying drops the names the repository uses itself: a tmux session
// called "work" or "docs" names nothing of the machine.
func identifying(names []string, words map[string]bool) []string {
	var out []string
	for _, n := range names {
		if !words[n] {
			out = append(out, n)
		}
	}
	return out
}

// private fails when a frame names this machine: a name as a whole word,
// on any line.
func private(frames []replay.Frame, names []string) error {
	if len(names) == 0 {
		return nil
	}
	// One pattern for every name: a frame line is read once, not once a name.
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	re := regexp.MustCompile(`(?:^|[^\w-])(` + strings.Join(quoted, "|") + `)(?:$|[^\w-])`)
	for _, f := range frames {
		for _, l := range f.Lines {
			var b strings.Builder
			for _, sp := range l {
				b.WriteString(sp.Text)
			}
			if m := re.FindStringSubmatch(b.String()); m != nil {
				return fmt.Errorf("the frame at %v names this machine (%q): %q", f.At, m[1], b.String())
			}
		}
	}
	return nil
}

// machineNames is identifying(thisMachine(), the repository's words), read
// once for every size of the replay: the words are every file of internal
// and of the demo's tree.
var machineNames = sync.OnceValues(func() ([]string, error) {
	words, err := repoWords("../../internal", "../../testdata/demo")
	if err != nil {
		return nil, err
	}
	return identifying(thisMachine(), words), nil
})
