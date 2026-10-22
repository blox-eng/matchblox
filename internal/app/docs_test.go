package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	bkey "charm.land/bubbles/v2/key"

	"github.com/blox-eng/matchblox/internal/docscheck"
)

// docKey turns a key as the docs write it into the name a binding holds.
// Words match in any case.
var docKey = map[string]string{
	"enter": "enter", "⏎": "enter", "esc": "esc", "↑": "up", "↓": "down", "←": "left", "→": "right",
	"tab": "tab", "shift+tab": "shift+tab", "space": "space", "ctrl+c": "ctrl+c", "ctrl+o": "ctrl+o",
	"backspace": "backspace",
}

func keyName(doc string) string {
	if k, ok := docKey[strings.ToLower(doc)]; ok && len(doc) > 1 {
		return k
	}
	if k, ok := docKey[doc]; ok {
		return k
	}
	return doc
}

// keyLayer is the keys of one screen: groups are its bindings (the keys
// of one act, and the key its help shows), all is every key of them.
type keyLayer struct {
	groups [][]string
	all    map[string]bool
}

// boundKeys reads the keys from the keymaps: Model is every keymap of the
// console, Shell both of Hosts.
func boundKeys() map[string]*keyLayer {
	k, s := defaultKeys(), defaultShellKeys()
	screens := map[string][][]bkey.Binding{
		"Model": {k.global.bindings(), k.main.bindings(), k.form.bindings(), k.peek.bindings(), k.sheet.bindings(),
			k.search.bindings(), k.pair.bindings(), k.takeOver.bindings(), k.confirm.bindings()},
		"Shell": {s.shell.bindings(), s.typing.bindings()},
	}
	layers := map[string]*keyLayer{}
	for sc, maps := range screens {
		l := &keyLayer{all: map[string]bool{}}
		for _, b := range slices.Concat(maps...) {
			g := append(slices.Clone(b.Keys()), b.Help().Key)
			for _, key := range g {
				l.all[key] = true
			}
			l.groups = append(l.groups, g)
		}
		layers[sc] = l
	}
	return layers
}

// TestTheDocsNameEveryKeyAndOnlyRealOnes holds the keys of the README and
// the docs to the console: a key the console does not handle fails, in a
// Key table or in prose. A Key table under a Hosts heading is held to Hosts,
// any other to the console. And every key the console or Hosts handles is
// in a Key table of its own screen.
func TestTheDocsNameEveryKeyAndOnlyRealOnes(t *testing.T) {
	files, err := docscheck.Files("../..")
	if err != nil {
		t.Fatal(err)
	}
	refs, err := docscheck.Keys(files)
	if err != nil {
		t.Fatal(err)
	}
	layers := boundKeys()
	if len(layers["Model"].groups) < 10 || len(layers["Shell"].groups) < 4 {
		t.Fatalf("read %d console and %d Hosts key groups: the keymaps are broken", len(layers["Model"].groups), len(layers["Shell"].groups))
	}
	tabs := fmt.Sprintf("1-%d", len(tabNames))
	named := map[string]map[string]bool{"Model": {}, "Shell": {}}
	for _, r := range refs {
		k := keyName(r.Text)
		screens := []string{"Model", "Shell"}
		if r.Table {
			screens = []string{"Model"}
			if strings.Contains(r.Section, "Hosts") {
				screens = []string{"Shell"}
			}
		}
		ok := false
		for _, sc := range screens {
			if layers[sc].all[k] {
				ok = true
				if r.Table {
					named[sc][k] = true
				}
			}
		}
		if !ok {
			t.Errorf("%s:%d (%s): the docs name the key %q, and %s does not handle it", r.File, r.Line, r.Section, r.Text, strings.Join(screens, " or "))
		}
	}
	if !named["Model"][tabs] {
		t.Errorf("no Key table names %q, the tab keys", tabs)
	}
	for sc, l := range layers {
		seen := map[string]bool{}
		for _, g := range l.groups {
			if seen[strings.Join(g, " ")] {
				continue
			}
			seen[strings.Join(g, " ")] = true
			found := false
			for _, k := range g {
				found = found || named[sc][k]
			}
			if !found {
				t.Errorf("%s handles %s, and no Key table of its screen names it", sc, strings.Join(g, " or "))
			}
		}
	}
}
