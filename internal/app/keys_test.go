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
	decl   []bkey.Binding // the struct fields, in declared order
}

func fieldBinds(v any) []bkey.Binding {
	rv := reflect.ValueOf(v)
	out := make([]bkey.Binding, rv.NumField())
	for i := range out {
		out[i] = rv.Field(i).Interface().(bkey.Binding)
	}
	return out
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
	main = kmLayer{"main", k.main.bindings(), fieldNames(k.main), fieldBinds(k.main)}
	modal = []kmLayer{
		{"global", k.global.bindings(), fieldNames(k.global), fieldBinds(k.global)},
		{"form", k.form.bindings(), fieldNames(k.form), fieldBinds(k.form)},
		{"peek", k.peek.bindings(), fieldNames(k.peek), fieldBinds(k.peek)},
		{"sheet", k.sheet.bindings(), fieldNames(k.sheet), fieldBinds(k.sheet)},
		{"search", k.search.bindings(), fieldNames(k.search), fieldBinds(k.search)},
		{"pair", k.pair.bindings(), fieldNames(k.pair), fieldBinds(k.pair)},
		{"confirm", k.confirm.bindings(), fieldNames(k.confirm), fieldBinds(k.confirm)},
		{"takeOver", k.takeOver.bindings(), fieldNames(k.takeOver), fieldBinds(k.takeOver)},
		{"scan", k.scan.bindings(), fieldNames(k.scan), fieldBinds(k.scan)},
		{"ask", k.ask.bindings(), fieldNames(k.ask), fieldBinds(k.ask)},
	}
	return main, modal
}

func allLayers(t *testing.T) []kmLayer {
	t.Helper()
	main, modal := layersOf(defaultKeys())
	s := defaultShellKeys()
	all := append([]kmLayer{main}, modal...)
	all = append(all,
		kmLayer{"shell", s.shell.bindings(), fieldNames(s.shell), fieldBinds(s.shell)},
		kmLayer{"shellTyping", s.typing.bindings(), fieldNames(s.typing), fieldBinds(s.typing)})
	for _, l := range all {
		if len(l.binds) != len(l.fields) {
			t.Errorf("%s: bindings() lists %d of %d fields", l.name, len(l.binds), len(l.fields))
		}
		for i, b := range l.binds {
			if i < len(l.fields) && !reflect.DeepEqual(b, l.decl[i]) {
				t.Errorf("%s: bindings()[%d] is not field %s", l.name, i, l.fields[i])
			}
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
	seen := map[string]bool{}
	for _, l := range layers {
		own := owners(t, l)
		if l.name == "main" || l.name == "shell" || l.name == "shellTyping" {
			continue
		}
		seen[l.name] = true
		for k, f := range own {
			if m, shared := main[k]; shared && !shadowsMain[l.name][k] {
				t.Errorf("%s: key %q is in %s and in main %s: add it to shadowsMain[%q] if the layer should shadow main", l.name, k, f, m, l.name)
			}
		}
		for k := range shadowsMain[l.name] {
			_, mine := own[k]
			_, theirs := main[k]
			if !mine || !theirs {
				t.Errorf("shadowsMain[%q] lists %q, which the layer binds: %v, and main uses: %v", l.name, k, mine, theirs)
			}
		}
	}
	for name := range shadowsMain {
		if !seen[name] {
			t.Errorf("shadowsMain lists the layer %q, which has no keymap", name)
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
