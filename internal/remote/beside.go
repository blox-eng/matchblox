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
// panes. It acts only when pane's window is not so already, so the keys stay
// where the person put them.
func ZoomArgv(pane string, zoom bool) []string {
	if zoom {
		return []string{"tmux", "if-shell", "-F", "-t", pane, "#{window_zoomed_flag}", "", "select-pane -t " + pane + " ; resize-pane -Z -t " + pane}
	}
	return []string{"tmux", "if-shell", "-F", "-t", pane, "#{window_zoomed_flag}", "resize-pane -Z -t " + pane}
}

// ChainArgv shows a chain's views in the agent pane beside and in panes
// split from it, in the console's right column only: views[0] in beside,
// 2 views one above the other, 3 or 4 a 2 × 2 grid, the 4th cell only with
// a 4th view. Each split prints "i %id", the view it shows and its pane, and
// the window's @matchblox_chain names beside and every pane split.
func ChainArgv(beside string, views [][]string) []string {
	cmds := [][]string{
		append([]string{"respawn-pane", "-k", "-t", beside, "--"}, views[0]...),
		{"set-option", "-w", "-t", beside, sample.ChainOption, beside},
	}
	// A split without -d makes its pane the target of the commands after it
	// that name none: the 4th view splits the 3rd, and each set-option adds
	// the pane just split.
	split := func(i int, dir string, target ...string) {
		c := append(append([]string{"split-window", dir}, target...), "-P", "-F", strconv.Itoa(i)+" #{pane_id}", "--")
		cmds = append(cmds, append(c, views[i]...), []string{"set-option", "-awF", sample.ChainOption, " #{pane_id}"})
	}
	switch len(views) {
	case 2:
		split(1, "-v", "-t", beside)
	case 3, 4:
		split(2, "-v", "-t", beside)
		if len(views) == 4 {
			split(3, "-h")
		}
		split(1, "-h", "-t", beside)
	}
	return tmuxList(cmds...)
}
