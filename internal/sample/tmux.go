package sample

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// Pane is one tmux pane.
type Pane struct {
	ID      string `json:"id"`     // %12
	Target  string `json:"target"` // session:window.pane
	Window  string `json:"window"`
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

// PaneFormat is the -F format ParsePanes expects; fields are tab separated.
const PaneFormat = "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}\t#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}"

// TmuxPanes lists every pane on the default tmux server. No server is not an
// error: the console still shows the machine.
func TmuxPanes() ([]byte, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, nil
	}
	out, err := output("tmux", "list-panes", "-a", "-F", PaneFormat)
	if err != nil {
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

func ParsePanes(b []byte) []Pane {
	var panes []Pane
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(f[3])
		panes = append(panes, Pane{ID: f[0], Target: f[1], Window: f[2], PID: pid, Command: f[4], Path: f[5]})
	}
	return panes
}
