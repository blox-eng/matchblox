package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/docscheck"
	"github.com/blox-eng/matchblox/internal/remote"
)

// internalCommands run from a hook, an ssh forced command or a connect;
// the docs may name them and need not.
var internalCommands = []string{"hook", "authorize", "gate"}

// docHosts are the hosts public text may name (DESIGN.md §7): the demo
// host, or a placeholder. Any other word is a typo of a command.
var docHosts = []string{"ws-1", "<host>"}

// TestTheDocsShowOnlyRealCommands holds every `matchblox …` line of the
// README, CONTRIBUTING and the docs to the binary: the command is one it
// has (or a host), each flag is one it parses, and every command a builder
// types is shown somewhere.
func TestTheDocsShowOnlyRealCommands(t *testing.T) {
	files, err := docscheck.Files("../..")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := docscheck.Commands(files)
	if err != nil {
		t.Fatal(err)
	}
	fl, _ := newFlags()
	shown := map[string]bool{}
	for _, r := range refs {
		args := strings.Fields(r.Text)[1:]
		cmd := "console"
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			switch {
			case slices.Contains(commands, args[0]) || slices.Contains(internalCommands, args[0]):
				cmd = args[0]
			case slices.Contains(docHosts, args[0]) && (args[0] == "<host>" || remote.ValidHost(args[0])):
				cmd = "<host>"
			default:
				t.Errorf("%s:%d: %q: %q is not a matchblox command", r.File, r.Line, r.Text, args[0])
				continue
			}
			args = args[1:]
		}
		shown[cmd] = true
		for _, a := range args {
			if !strings.HasPrefix(a, "--") {
				continue
			}
			name, _, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if fl.Lookup(name) == nil {
				t.Errorf("%s:%d: %q: matchblox has no flag --%s", r.File, r.Line, r.Text, name)
			}
		}
	}
	for _, c := range append([]string{"console", "<host>"}, commands...) {
		if !shown[c] {
			t.Errorf("the docs never show `matchblox %s`", strings.TrimPrefix(c, "console"))
		}
	}
}
