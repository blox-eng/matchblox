package sample

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// CapturePanes reads the visible text of each pane; a pane that is gone is
// left out.
func CapturePanes(panes []string) map[string]string {
	out := map[string]string{}
	for _, id := range panes {
		if b, err := output("tmux", "capture-pane", "-p", "-t", id); err == nil {
			out[id] = string(b)
		}
	}
	return out
}

// CaptureDir reads pane text from <dir>/<id>.txt (id without its %): the
// panes of a fixture machine.
func CaptureDir(dir string) func([]string) map[string]string {
	return func(panes []string) map[string]string {
		out := map[string]string{}
		for _, id := range panes {
			if b, err := os.ReadFile(filepath.Join(dir, strings.TrimPrefix(id, "%")+".txt")); err == nil {
				out[id] = string(b)
			}
		}
		return out
	}
}
