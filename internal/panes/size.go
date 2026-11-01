package panes

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// SizedMark is the window option that marks a window a chat sized: the
// window-size it had before ("-": it set none) and its size then, as
// "latest 120x40". A service that ends without putting the window back
// finds the mark when it starts again.
const SizedMark = "@matchblox-sized"

// The least and the most a chat sizes an agent's window to: tmux takes no
// window over 10000 either way.
const (
	MinCols, MinRows = 20, 5
	MaxCols, MaxRows = 10000, 10000
)

// unsetSize stands, in a mark, for a window that set no window-size.
const unsetSize = "-"

// windowSizes are the values of tmux's window-size option.
var windowSizes = []string{"largest", "smallest", "manual", "latest"}

// WindowOf reads the window that holds pane: its id, how many panes it
// has, its size and its mark.
func WindowOf(pane string) []string {
	tmux, id := sample.Tmux(pane)
	return slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{window_id}\t#{window_panes}\t#{window_width}x#{window_height}\t#{" + SizedMark + "}"})
}

// WindowSizeOf reads the window-size the window sets itself: empty where
// it takes the server's.
func WindowSizeOf(server, window string) []string {
	return slices.Concat(sample.TmuxOn(server), []string{"show-options", "-w", "-v", "-t", window, "window-size"})
}

// MarkOf is the mark of a window whose own window-size was prev (as
// WindowSizeOf prints it) and whose size was size.
func MarkOf(prev, size string) string {
	if prev = strings.TrimSpace(prev); prev == "" {
		prev = unsetSize
	}
	return prev + " " + size
}

// SizeWindow sizes the window of pane to cols by rows, within tmux's
// bounds, and prints the pane's size after: a window-size of manual keeps
// it so while clients come and go. A mark is set first where one is
// given: the first size of a window marks it, a later one keeps the mark.
func SizeWindow(pane, window, mark string, cols, rows int) []string {
	tmux, id := sample.Tmux(pane)
	argv := slices.Clone(tmux)
	if mark != "" {
		argv = append(argv, "set-option", "-w", "-t", window, SizedMark, mark, ";")
	}
	cols, rows = min(max(cols, MinCols), MaxCols), min(max(rows, MinRows), MaxRows)
	return append(argv,
		"set-option", "-w", "-t", window, "window-size", "manual", ";",
		"resize-window", "-t", window, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows), ";",
		"display-message", "-p", "-t", id, "#{pane_width} #{pane_height}")
}

// RestoreWindow puts back the window-size of a mark, and the size of a
// window that was manual, then drops the mark. A window that set none, or
// a value tmux does not know, takes the server's again: tmux sizes it by
// its clients.
func RestoreWindow(server, window, mark string) []string {
	argv := sample.TmuxOn(server)
	prev, size, _ := strings.Cut(mark, " ")
	cols, rows, sized := parseSize(size)
	switch {
	case prev == "manual" && sized:
		argv = append(argv, "set-option", "-w", "-t", window, "window-size", prev, ";",
			"resize-window", "-t", window, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows), ";")
	case slices.Contains(windowSizes, prev):
		argv = append(argv, "set-option", "-w", "-t", window, "window-size", prev, ";")
	default:
		argv = append(argv, "set-option", "-w", "-u", "-t", window, "window-size", ";")
	}
	return append(argv, "set-option", "-w", "-u", "-t", window, SizedMark)
}

// parseSize reads "120x40" within tmux's bounds.
func parseSize(s string) (cols, rows int, ok bool) {
	c, r, _ := strings.Cut(s, "x")
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	return cols, rows, err1 == nil && err2 == nil && cols > 0 && rows > 0 && cols <= MaxCols && rows <= MaxRows
}

// SizedWindows lists every window of server with its mark, empty where it
// has none.
func SizedWindows(server string) []string {
	return slices.Concat(sample.TmuxOn(server), []string{"list-windows", "-a", "-F", "#{window_id}\t#{" + SizedMark + "}"})
}
