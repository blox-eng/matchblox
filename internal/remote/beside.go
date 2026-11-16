package remote

import (
	"strconv"

	"github.com/blox-eng/matchblox/internal/sample"
)

const besideHint = `printf "⏎ on a row shows its agent here.\n"; read _`

// SplitArgv opens the agent's pane beside the console's, listW columns wide
// in a window of windowW (0: unknown, the agent gets 60%). -d keeps the keys
// in the list; -P prints the new pane's id.
func SplitArgv(console string, listW, windowW int) []string {
	rest := 60
	if windowW > listW {
		rest = 100 - listW*100/windowW
	}
	return []string{"tmux", "split-window", "-h", "-d", "-t", console, "-l", strconv.Itoa(rest) + "%",
		"-P", "-F", "#{pane_id}", "--", "/bin/sh", "-c", besideHint}
}

// BesideArgv names the two panes in the window, for the key that flips
// between them, and paints the border of the pane that has the keys.
func BesideArgv(console, agent, accent string) []string {
	set := func(name, value string) []string { return []string{"set-option", "-w", "-t", console, name, value} }
	return tmuxList(
		set(sample.ConsoleOption, console),
		set(sample.BesideOption, agent),
		set("pane-active-border-style", "fg="+accent),
	)
}

// ListWidthArgv keeps the list's pane listW columns wide.
func ListWidthArgv(console string, listW int) []string {
	return []string{"tmux", "resize-pane", "-t", console, "-x", strconv.Itoa(listW)}
}

// ZoomArgv zooms pane to the whole window, or gives the window back to both
// panes. It acts only when the window is not so already, so the keys stay
// where the person put them.
func ZoomArgv(pane string, zoom bool) []string {
	if zoom {
		return []string{"tmux", "if-shell", "-F", "#{window_zoomed_flag}", "", "select-pane -t " + pane + " ; resize-pane -Z -t " + pane}
	}
	return []string{"tmux", "if-shell", "-F", "#{window_zoomed_flag}", "resize-pane -Z -t " + pane}
}
