package sample

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// tmuxUTF8 is TmuxOn with -u: format and capture output keep tabs and
// glyphs when LANG is unset. Without it, tmux in the C locale prints
// every non-printable as "_", and ParsePanes sees no panes.
func tmuxUTF8(server string) []string {
	a := TmuxOn(server)
	return append([]string{a[0], "-u"}, a[1:]...)
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
	// ServerStart is when the tmux server started (unix seconds): with
	// Server, it tells a new server that took a lost one's pid apart.
	ServerStart string `json:"server_start,omitempty"`
}

// PaneFormat is the -F format ParsePanes expects; fields are tab separated.
const PaneFormat = "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}\t#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}\t#{socket_path}\t#{pid}\t#{" + JumpMark + "}\t#{start_time}"

// JumpMark is the session option that marks a jump's session on either
// side (package remote sets it to 1): the sampler skips its panes.
const JumpMark = "@matchblox-jump"

// TmuxPanes lists every pane on the default tmux server. No server is not an
// error: the console still shows the machine.
func TmuxPanes() ([]byte, error) { return TmuxPanesOn("") }

// TmuxPanesOn lists every pane on the tmux server server ("": the
// builder's), as TmuxPanes does.
func TmuxPanesOn(server string) ([]byte, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, nil
	}
	tmux := tmuxUTF8(server)
	out, err := output(tmux[0], append(tmux[1:], "list-panes", "-a", "-F", PaneFormat)...)
	if err != nil {
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// jumpSessionRE is the host's session of a jump, as package remote names
// it: a console before JumpMark made it without the mark.
var jumpSessionRE = regexp.MustCompile(`^matchblox-jump-[0-9a-f]{8}$`)

// JumpSession is true for the name of a host's session of a jump without
// JumpMark: the sampler skips them too.
func JumpSession(name string) bool { return jumpSessionRE.MatchString(name) }

// ParsePanes reads the list of TmuxPanes. A pane listed again under another
// session (the phone's, grouped with its own) is the same pane: the first
// listing, its own session's, stays. A jump's session is skipped: on the
// host it groups the agent's and can sort first, and on this machine it
// holds only the ssh.
func ParsePanes(b []byte) []Pane {
	var panes []Pane
	seen := map[[2]string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		if session, _, _ := strings.Cut(f[1], ":"); JumpSession(session) || len(f) > 8 && f[8] == "1" {
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
		if len(f) > 9 {
			p.ServerStart = f[9]
		}
		if k := [2]string{p.Server, p.ID}; !seen[k] {
			seen[k] = true
			panes = append(panes, p)
		}
	}
	return panes
}

// CapturePanes reads the visible text of each pane; a pane that is gone is
// left out.
func CapturePanes(panes []string) map[string]string {
	out := map[string]string{}
	for _, id := range panes {
		server, bare := SplitPane(id)
		tmux := tmuxUTF8(server)
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
		server, bare := SplitPane(id)
		tmux := tmuxUTF8(server)
		if b, err := output(tmux[0], append(tmux[1:], "capture-pane", "-p", "-e", "-t", bare)...); err == nil {
			out[id] = string(b)
		}
	}
	return out
}

// CaptureScreen reads pane id's live screen for a console's view: its
// text with its styles, the lines the pane wrapped joined again, and if
// the program in it draws on the alternate screen. One tmux call reads
// both, so the flag is the text's.
func CaptureScreen(id string) (text string, alt, ok bool) {
	return captureScreen(output, id)
}

func captureScreen(run func(string, ...string) ([]byte, error), id string) (text string, alt, ok bool) {
	server, bare := SplitPane(id)
	tmux := tmuxUTF8(server)
	b, err := run(tmux[0], append(tmux[1:], "display-message", "-p", "-t", bare, "#{alternate_on}", ";", "capture-pane", "-p", "-e", "-J", "-t", bare)...)
	if err != nil {
		return "", false, false
	}
	flag, rest, found := strings.Cut(string(b), "\n")
	if !found || (flag != "0" && flag != "1") {
		return "", false, false
	}
	return rest, flag == "1", true
}

// PaneMouse tells if the program in pane id reads the mouse in SGR form, and
// the pane's size.
func PaneMouse(id string) (cols, rows int, ok bool) {
	return paneMouse(output, id)
}

func paneMouse(run func(string, ...string) ([]byte, error), id string) (cols, rows int, ok bool) {
	server, bare := SplitPane(id)
	tmux := tmuxUTF8(server)
	b, err := run(tmux[0], append(tmux[1:], "display-message", "-p", "-t", bare, "#{mouse_any_flag} #{mouse_sgr_flag} #{pane_width} #{pane_height}")...)
	if err != nil {
		return 0, 0, false
	}
	var anyFlag, sgr int
	if _, err := fmt.Sscan(string(b), &anyFlag, &sgr, &cols, &rows); err != nil {
		return 0, 0, false
	}
	return cols, rows, anyFlag == 1 && sgr == 1
}

// PaneHistory is how many lines of history pane id holds, and its height.
func PaneHistory(id string) (history, rows int, ok bool) {
	return paneHistory(output, id)
}

func paneHistory(run func(string, ...string) ([]byte, error), id string) (history, rows int, ok bool) {
	server, bare := SplitPane(id)
	tmux := tmuxUTF8(server)
	b, err := run(tmux[0], append(tmux[1:], "display-message", "-p", "-t", bare, "#{history_size} #{pane_height}")...)
	if err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscan(string(b), &history, &rows); err != nil || rows <= 0 {
		return 0, 0, false
	}
	return history, rows, true
}

// CaptureBackStyled reads pane id as CapturePanesStyled does, back lines up
// in its history: the rows lines a screen that far back shows.
func CaptureBackStyled(id string, back, rows int) (string, bool) {
	return captureBack(output, id, back, rows)
}

func captureBack(run func(string, ...string) ([]byte, error), id string, back, rows int) (string, bool) {
	server, bare := SplitPane(id)
	tmux := tmuxUTF8(server)
	start, end := strconv.Itoa(-back), strconv.Itoa(rows-1-back)
	b, err := run(tmux[0], append(tmux[1:], "capture-pane", "-p", "-e", "-J", "-t", bare, "-S", start, "-E", end)...)
	if err != nil {
		return "", false
	}
	return string(b), true
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
	b, err := output("tmux", "-u", "list-clients", "-F", "#{client_activity}")
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
