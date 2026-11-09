package panes

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// SizedMark is the window option an older service set on a window it sized
// to a chat: the window-size it had before ("-": it set none) and its size
// then, as "latest 120x40". A service that starts puts each one back
// (#351: no window is sized any more).
const SizedMark = "@matchblox-sized"

// maxSize is the most tmux takes for a window, either way.
const maxSize = 10000

// windowSizes are the values of tmux's window-size option.
var windowSizes = []string{"largest", "smallest", "manual", "latest"}

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
	return cols, rows, err1 == nil && err2 == nil && cols > 0 && rows > 0 && cols <= maxSize && rows <= maxSize
}

// SizedWindows lists every window of server with its mark, empty where it
// has none.
func SizedWindows(server string) []string {
	return slices.Concat(sample.TmuxOn(server), []string{"list-windows", "-a", "-F", "#{window_id}\t#{" + SizedMark + "}"})
}
