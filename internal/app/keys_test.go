package app

import (
	"reflect"
	"slices"
	"testing"

	bkey "charm.land/bubbles/v2/key"
)

// kmLayer is one keymap: its name, its bindings in field order, and the
// field names those bindings have.
type kmLayer struct {
	name   string
	binds  []bkey.Binding
	fields []string
}

func fieldNames(v any) []string {
	t := reflect.TypeOf(v)
	out := make([]string, t.NumField())
	for i := range out {
		out[i] = t.Field(i).Name
	}
	return out
}

func layersOf(k keymaps) (main kmLayer, modal []kmLayer) {
	main = kmLayer{"main", k.main.bindings(), fieldNames(k.main)}
	modal = []kmLayer{
		{"global", k.global.bindings(), fieldNames(k.global)},
		{"form", k.form.bindings(), fieldNames(k.form)},
		{"peek", k.peek.bindings(), fieldNames(k.peek)},
		{"sheet", k.sheet.bindings(), fieldNames(k.sheet)},
		{"search", k.search.bindings(), fieldNames(k.search)},
		{"pair", k.pair.bindings(), fieldNames(k.pair)},
		{"takeOver", k.takeOver.bindings(), fieldNames(k.takeOver)},
	}
	return main, modal
}

func allLayers(t *testing.T) []kmLayer {
	t.Helper()
	main, modal := layersOf(defaultKeys())
	s := defaultShellKeys()
	all := append([]kmLayer{main}, modal...)
	all = append(all,
		kmLayer{"shell", s.shell.bindings(), fieldNames(s.shell)},
		kmLayer{"shellTyping", s.typing.bindings(), fieldNames(s.typing)})
	for _, l := range all {
		if len(l.binds) != len(l.fields) {
			t.Errorf("%s: bindings() lists %d of %d fields", l.name, len(l.binds), len(l.fields))
		}
	}
	return all
}

// owners maps each key of a kmLayer to the field that holds it, and fails on
// a key two enabled bindings share.
func owners(t *testing.T, l kmLayer) map[string]string {
	t.Helper()
	out := map[string]string{}
	for i, b := range l.binds {
		if !b.Enabled() || i >= len(l.fields) {
			continue
		}
		for _, k := range b.Keys() {
			if prev, dup := out[k]; dup {
				t.Errorf("%s: key %q is in both %s and %s", l.name, k, prev, l.fields[i])
			}
			out[k] = l.fields[i]
		}
	}
	return out
}

func TestKeymapsHaveNoCollisions(t *testing.T) {
	layers := allLayers(t)
	main := owners(t, layers[0])
	for _, l := range layers {
		own := owners(t, l)
		if l.name == "main" || l.name == "shell" || l.name == "shellTyping" {
			continue
		}
		for k, f := range own {
			if m, shared := main[k]; shared && !shadowsMain[k] {
				t.Errorf("%s: key %q is in %s and in main %s: add it to shadowsMain if the kmLayer should shadow main", l.name, k, f, m)
			}
		}
	}
	for k := range shadowsMain {
		if _, ok := main[k]; !ok {
			t.Errorf("shadowsMain lists %q, which main does not use", k)
		}
	}
}

func TestEveryBindingHasHelp(t *testing.T) {
	for _, l := range allLayers(t) {
		for i, b := range l.binds {
			if i >= len(l.fields) {
				continue
			}
			if len(b.Keys()) == 0 {
				t.Errorf("%s.%s has no key", l.name, l.fields[i])
			}
			if b.Help().Key == "" {
				t.Errorf("%s.%s has no help key", l.name, l.fields[i])
			}
			if slices.Contains(b.Keys(), "") {
				t.Errorf("%s.%s has an empty key", l.name, l.fields[i])
			}
		}
	}
}
