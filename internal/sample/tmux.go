package sample

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StokerServer is the tmux server matchblox keeps the stoker agent on
// (tmux -L matchblox), and StokerSession its session there: a server of
// its own, so the agent shows in none of the builder's tmux ls and
// session switcher.
const (
	StokerServer  = "matchblox"
	StokerSession = "stoker"
)

// OnServer is the id of pane id of the tmux server (-L) server: "%3@matchblox".
// Pane ids repeat across servers, so a pane of any server but the
// builder's ("") carries its server, and no tmux call can reach the
// builder's %3 with it.
func OnServer(id, server string) string {
	if server == "" {
		return id
	}
	return id + "@" + server
}

// SplitPane is the server of a pane id ("": the builder's) and the id
// that server knows the pane by.
func SplitPane(id string) (server, bare string) {
	if b, sv, ok := strings.Cut(id, "@"); ok && strings.HasPrefix(b, "%") {
		return sv, b
	}
	return "", id
}

// TmuxOn is the argv that reaches the tmux server server ("": the
// builder's). Every call aimed at a pane of matchblox's own server starts
// with it.
func TmuxOn(server string) []string {
	if server == "" {
		return []string{"tmux"}
	}
	return []string{"tmux", "-L", server}
}

// Tmux is the argv that reaches the server of the pane target, and the id
// that server knows it by.
func Tmux(target string) ([]string, string) {
	server, bare := SplitPane(target)
	return TmuxOn(server), bare
}

// Server is the tmux server of the session's pane: "" the builder's.
func (s Session) Server() string {
	server, _ := SplitPane(s.Pane)
	return server
}

// SocketPath is the socket tmux -L server listens on, its directory
// resolved as tmux resolves it: what $TMUX names inside that server.
func SocketPath(server string) string {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	return filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), server)
}

// Pane is one tmux pane.
type Pane struct {
	ID      string `json:"id"`     // %12
	Target  string `json:"target"` // session:window.pane
	Window  string `json:"window"`
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Path    string `json:"path"`
	Socket  string `json:"socket,omitempty"` // the tmux server's socket
	Server  string `json:"server,omitempty"` // the tmux server's pid
}

// PaneFormat is the -F format ParsePanes expects; fields are tab separated.
const PaneFormat = "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}\t#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}\t#{socket_path}\t#{pid}"

// TmuxPanes lists every pane on the default tmux server. No server is not an
// error: the console still shows the machine.
func TmuxPanes() ([]byte, error) { return TmuxPanesOn("") }

// TmuxPanesOn lists every pane on the tmux server server ("": the
// builder's), as TmuxPanes does.
func TmuxPanesOn(server string) ([]byte, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, nil
	}
	tmux := TmuxOn(server)
	out, err := output(tmux[0], append(tmux[1:], "list-panes", "-a", "-F", PaneFormat)...)
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
		p := Pane{ID: f[0], Target: f[1], Window: f[2], PID: pid, Command: f[4], Path: f[5]}
		if len(f) > 6 {
			p.Socket = f[6]
		}
		if len(f) > 7 {
			p.Server = f[7]
		}
		panes = append(panes, p)
	}
	return panes
}

// CapturePanes reads the visible text of each pane; a pane that is gone is
// left out.
func CapturePanes(panes []string) map[string]string {
	out := map[string]string{}
	for _, id := range panes {
		tmux, bare := Tmux(id)
		if b, err := output(tmux[0], append(tmux[1:], "capture-pane", "-p", "-t", bare)...); err == nil {
			out[id] = string(b)
		}
	}
	return out
}

// CapturePanesStyled reads each pane as CapturePanes does, with the
// escape sequences of its colours and attributes.
func CapturePanesStyled(panes []string) map[string]string {
	out := map[string]string{}
	for _, id := range panes {
		tmux, bare := Tmux(id)
		if b, err := output(tmux[0], append(tmux[1:], "capture-pane", "-p", "-e", "-t", bare)...); err == nil {
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

// ClientActivity is the last key of the builder in any tmux client: in a
// console or in an agent's pane. Zero when no client is attached.
func ClientActivity() time.Time {
	b, err := output("tmux", "list-clients", "-F", "#{client_activity}")
	if err != nil {
		return time.Time{}
	}
	var last time.Time
	for _, f := range strings.Fields(string(b)) {
		if sec, err := strconv.ParseInt(f, 10, 64); err == nil && time.Unix(sec, 0).After(last) {
			last = time.Unix(sec, 0)
		}
	}
	return last
}
