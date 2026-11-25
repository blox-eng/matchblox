package mux

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// Pane is one pane on this multiplexer.
type Pane struct {
	ID, Target, Window, Command, Path, Socket, ServerPID, TTY, Jump, ServerStart string
	PID                                                                          int
	Dead                                                                         bool
}

// jumpMark is the session option that marks a jump's session. List reports
// it and keeps the pane: the sampler decides which sessions to skip.
const jumpMark = "@matchblox-jump"

// PaneFormat is the list-panes -F format List parses. Fields are tab
// separated: today's pane format, then the pane's tty and whether it is dead.
const PaneFormat = "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}\t#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}\t#{socket_path}\t#{pid}\t#{" + jumpMark + "}\t#{start_time}\t#{pane_tty}\t#{pane_dead}"

// List reads every pane on t's socket. No tmux, or a tmux that exits, is an
// empty list: the console still shows the machine. A pane tmux lists twice
// (a grouped session) stays twice, so a jump listed first cannot hide the
// pane's own session.
func (t Tmux) List() ([]Pane, error) {
	if err := t.look(); err != nil {
		return nil, nil
	}
	argv := append(t.listArgv(), "list-panes", "-a", "-F", PaneFormat)
	out, err := t.output(argv)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, nil
		}
		return nil, err
	}
	return parsePanes(out, t.Server), nil
}

func (t Tmux) look() error {
	look := t.LookPath
	if look == nil {
		look = exec.LookPath
	}
	_, err := look("tmux")
	return err
}

func (t Tmux) output(argv []string) (string, error) {
	if t.Run != nil {
		return t.Run(argv)
	}
	b, err := commandOutput(argv)
	return string(b), err
}

func parsePanes(out, server string) []Pane {
	var panes []Pane
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(f[3])
		p := Pane{ID: OnServer(f[0], server), Target: f[1], Window: f[2], PID: pid, Command: f[4], Path: f[5]}
		if len(f) > 6 {
			p.Socket = f[6]
		}
		if len(f) > 7 {
			p.ServerPID = f[7]
		}
		if len(f) > 8 {
			p.Jump = f[8]
		}
		if len(f) > 9 {
			p.ServerStart = f[9]
		}
		if len(f) > 10 {
			p.TTY = f[10]
		}
		if len(f) > 11 {
			p.Dead = f[11] == "1"
		}
		panes = append(panes, p)
	}
	return panes
}
