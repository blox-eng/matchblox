package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/panes"
)

// tmuxConf is the file tmux reads: ~/.tmux.conf, else the XDG one when it
// is there, else a new ~/.tmux.conf.
func tmuxConf(e Env) string {
	home := filepath.Join(e.Home, ".tmux.conf")
	if _, err := os.Stat(home); err == nil {
		return home
	}
	dir := e.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(e.Home, ".config")
	}
	if xdg := filepath.Join(dir, "tmux", "tmux.conf"); fileExists(xdg) {
		return xdg
	}
	return home
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

var wayBackMark = strings.SplitN(panes.WayBack, "\n", 2)[0]

func wayBackDoor(e Env) doors.Door {
	path := tmuxConf(e)
	d := doors.Door{ID: doors.WayBack, Title: "Add the way back", Path: path,
		Why: "From any pane, prefix m or a tap on ◂ matchblox in the status line comes back here. prefix m was the tmux key that marks a pane."}
	_, old, _, err := read(path)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	next := withWayBack(old, false)
	if string(next) == string(old) {
		d.Done = true
		return d
	}
	d.Sum, d.Preview = Sum(old), diff(string(old), string(next))
	if !mouseOn.Match(old) {
		d.Also = &doors.Also{Label: "also turn the mouse on: a tap on ◂ matchblox works from a phone, and every pane selects and scrolls with the mouse",
			Preview: diff(string(old), string(withWayBack(old, true)))}
	}
	return d
}

// mouseOn finds a line that turns the tmux mouse on.
var mouseOn = regexp.MustCompile(`(?m)^[ \t]*set(-option)?([ \t]+-[a-zA-Z]+)*[ \t]+mouse[ \t]+on\b`)

// AddWayBack appends panes.WayBack to the tmux config once, with
// panes.Mouse when the person chose it, then loads it into the running
// server.
func AddWayBack(tmuxConf, seen string, mouse bool, source func(string) error) (backup string, err error) {
	backup, err = edit(tmuxConf, seen, func(old []byte) ([]byte, error) { return withWayBack(old, mouse), nil })
	if err != nil {
		return backup, err
	}
	if err := source(tmuxConf); err != nil && !errors.Is(err, ErrNoServer) {
		return backup, fmt.Errorf("written; tmux source-file: %w", err)
	}
	return backup, nil
}

func withWayBack(old []byte, mouse bool) []byte {
	s := string(old)
	if strings.Contains(s, wayBackMark) {
		return old
	}
	if s != "" {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n"
	}
	s += panes.WayBack
	if mouse {
		s += panes.Mouse
	}
	return []byte(s)
}
