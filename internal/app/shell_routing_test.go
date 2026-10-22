package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// shellAt builds the Hosts layer a routing row starts from: the plain list,
// a host being typed, a pending connect, or a pending remove.
func shellAt(t *testing.T, at string) *shellRig {
	t.Helper()
	switch at {
	case "list":
		return newShell(t, []string{"ws-1"}, nil, "", false)
	case "typing":
		r := newShell(t, nil, nil, "", true)
		r.key(t, "down")
		r.key(t, "enter") // another machine
		r.key(t, "enter") // type a host
		if r.sh.input == nil {
			t.Fatal("no host is being typed")
		}
		return r
	case "connect":
		r := shellAt(t, "typing")
		for _, k := range strings.Split("ws-3", "") {
			r.key(t, k)
		}
		r.key(t, "enter")
		if r.sh.pending == nil {
			t.Fatal("enter set no pending connect")
		}
		return r
	case "remove":
		r := shellAt(t, "list")
		r.key(t, "down")
		r.key(t, "x")
		if r.sh.pending == nil {
			t.Fatal("x set no pending remove")
		}
		return r
	}
	t.Fatalf("no shell state %q", at)
	return nil
}

func TestShellKeyRouting(t *testing.T) {
	text := func(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }
	type row struct {
		name string
		at   string
		key  tea.KeyPressMsg
		quit bool
		want func(t *testing.T, r *shellRig, s Shell)
	}
	rows := []row{
		{"list: ctrl+c quits", "list", ctrlC, true, nil},
		{"list: q quits", "list", text("q"), true, nil},
		{"typing: ctrl+c quits", "typing", ctrlC, true, nil},
		{"typing: q is typed", "typing", text("q"), false, func(t *testing.T, _ *shellRig, s Shell) {
			if s.input == nil || *s.input != "q" {
				t.Fatalf("input %v", s.input)
			}
		}},
		{"typing: a letter is typed", "typing", text("w"), false, func(t *testing.T, _ *shellRig, s Shell) {
			if s.input == nil || *s.input != "w" {
				t.Fatalf("input %v", s.input)
			}
		}},
		{"connect pending: ctrl+c quits", "connect", ctrlC, true, nil},
		{"connect pending: y connects", "connect", text("y"), false, func(t *testing.T, r *shellRig, s Shell) {
			if s.pending != nil {
				t.Fatal("the connect stayed pending")
			}
		}},
		{"connect pending: n cancels", "connect", text("n"), false, func(t *testing.T, _ *shellRig, s Shell) {
			if s.pending != nil || !strings.HasPrefix(s.flash, "cancelled: ") {
				t.Fatalf("pending %v, flash %q", s.pending, s.flash)
			}
		}},
		{"connect pending: q cancels", "connect", text("q"), false, func(t *testing.T, _ *shellRig, s Shell) {
			if s.pending != nil || !strings.HasPrefix(s.flash, "cancelled: ") {
				t.Fatalf("pending %v, flash %q", s.pending, s.flash)
			}
		}},
		{"remove pending: y removes", "remove", text("y"), false, nil},
		{"remove pending: enter cancels", "remove", tea.KeyPressMsg{Code: tea.KeyEnter}, false, func(t *testing.T, _ *shellRig, s Shell) {
			if s.pending != nil || !strings.HasPrefix(s.flash, "cancelled: ") {
				t.Fatalf("pending %v, flash %q", s.pending, s.flash)
			}
		}},
	}
	for _, rw := range rows {
		t.Run(rw.name, func(t *testing.T) {
			r := shellAt(t, rw.at)
			removed := ""
			r.sh.opt.Remove = func(h string) error { removed = h; return nil }
			next, cmd := r.sh.Update(rw.key)
			s := next.(Shell)
			if got := quits(cmd); got != rw.quit {
				t.Fatalf("quit %v, want %v", got, rw.quit)
			}
			if rw.at == "remove" && rw.key.Text == "y" && removed != "ws-1" {
				t.Fatalf("removed %q", removed)
			}
			if rw.at == "connect" && rw.key.Text == "y" {
				r.sh = s
				r.drain(cmd)
				if len(r.ran) != 1 || strings.Join(r.ran[0], " ") != "/bin/matchblox connect ws-3" {
					t.Fatalf("ran %q", r.ran)
				}
			}
			if rw.want != nil {
				rw.want(t, r, s)
			}
		})
	}
}
