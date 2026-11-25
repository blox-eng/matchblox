package mux

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
)

// commandBound is how long one tmux read waits. A wedged tmux must not
// freeze the read. The sampler uses the same bound.
const commandBound = 10 * time.Second

// Tmux is the tmux backend for one socket. Server is the -L label:
// "" the builder's, "matchblox" the stoker's. Socket, when set, is a -S
// path and List uses it instead of -L. Run runs a tmux command and returns
// what it prints (nil: exec); RunIn runs one with stdin (nil: exec);
// Capture reads screens (nil: with their styles, as the stoker's guard
// reads them). LookPath finds tmux (nil: exec.LookPath).
type Tmux struct {
	Server   string
	Socket   string
	LookPath func(string) (string, error)
	Run      func(argv []string) (string, error)
	RunIn    func(argv []string, stdin string) error
	Capture  func(panes []string) map[string]string
}

var _ Mux = Tmux{}

func (Tmux) Type(pane, text string) []string { return panes.Literal(pane, text) }

func (Tmux) Keys(pane string, keys ...string) []string { return panes.Keys(pane, keys...) }

func (t Tmux) Load(pane, name, text string) error {
	tmux, _ := targetArgv(pane)
	argv := slices.Concat(tmux, []string{"load-buffer", "-b", name, "-"})
	if t.RunIn != nil {
		return t.RunIn(argv, text)
	}
	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv built here, never a shell string
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

// NoBracketedPaste reads #{bracket_paste_flag}. The format exists from
// tmux 3.7; an older tmux prints nothing, and then it cannot tell.
func (t Tmux) NoBracketedPaste(pane string) bool {
	tmux, id := targetArgv(pane)
	out, err := t.run(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{bracket_paste_flag}"}))
	return err == nil && strings.TrimSpace(out) == "0"
}

func (Tmux) Paste(pane, name string) []string {
	tmux, id := targetArgv(pane)
	return slices.Concat(tmux, []string{"paste-buffer", "-p", "-d", "-b", name, "-t", id})
}

func (t Tmux) Drop(pane, name string) error {
	tmux, _ := targetArgv(pane)
	_, err := t.run(slices.Concat(tmux, []string{"delete-buffer", "-b", name}))
	return err
}

func (t Tmux) Screens(ps []string) map[string]string {
	if t.Capture != nil {
		return t.Capture(ps)
	}
	return captureStyled(ps)
}

func (t Tmux) InMode(pane string) (bool, error) {
	tmux, id := targetArgv(pane)
	out, err := t.run(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{pane_in_mode}"}))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

func (t Tmux) TypedAt(pane string) time.Time {
	tmux, id := targetArgv(pane)
	out, err := t.run(slices.Concat(tmux, []string{"list-clients", "-F", "#{client_activity} #{pane_id}"}))
	if err != nil {
		return time.Time{}
	}
	var last time.Time
	for l := range strings.Lines(out) {
		at, on, ok := strings.Cut(strings.TrimSpace(l), " ")
		secs, err := strconv.ParseInt(at, 10, 64)
		if !ok || err != nil || on != id {
			continue
		}
		if t := time.Unix(secs, 0); t.After(last) {
			last = t
		}
	}
	return last
}

// MatchOption is the pane option that marks an agent pane the service
// samples.
const MatchOption = "@matchblox_match"

func (t Tmux) Mark(pane string, on bool) error {
	tmux, id := targetArgv(pane)
	argv := slices.Concat(tmux, []string{"set-option", "-p", "-t", id, MatchOption, "1"})
	if !on {
		argv = slices.Concat(tmux, []string{"set-option", "-p", "-u", "-t", id, MatchOption})
	}
	_, err := t.run(argv)
	return err
}

func (t Tmux) run(argv []string) (string, error) {
	if t.Run != nil {
		return t.Run(argv)
	}
	out, err := exec.Command(argv[0], argv[1:]...).Output() //nolint:gosec // argv built here, never a shell string
	return string(out), err
}

// OnServer is the id of pane id on the tmux server (-L) server:
// "%3@matchblox". Pane ids repeat across servers, so a pane of any server
// but the builder's ("") carries its server.
func OnServer(id, server string) string {
	if server == "" {
		return id
	}
	return id + "@" + server
}

// SplitPane is the server of a pane id ("": the builder's) and the id that
// server knows the pane by.
func SplitPane(id string) (server, bare string) {
	if b, sv, ok := strings.Cut(id, "@"); ok && strings.HasPrefix(b, "%") {
		return sv, b
	}
	return "", id
}

func serverArgv(server string) []string {
	if server == "" {
		return []string{"tmux"}
	}
	return []string{"tmux", "-L", server}
}

// commandArgv is the tmux prefix for a command on this socket. A socket
// path wins, as it does for List: one Tmux is one server.
func (t Tmux) commandArgv() []string {
	if t.Socket != "" {
		return []string{"tmux", "-S", t.Socket}
	}
	return serverArgv(t.Server)
}

// unsetEnv is the env command lifecycle puts in front of a start, and the
// command a respawn runs: git never prompts, and the home console does not
// mark this session as its own.
func unsetEnv() []string {
	return []string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", panes.HomeEnv}
}

// Start is the one step that opens the session. It does not run. EmptyExit
// is the stoker's start: the server reads no tmux.conf, the window keeps
// the command's name, and the server ends with its last session.
func (t Tmux) Start(s Start) [][]string {
	step := append(unsetEnv(), t.commandArgv()...)
	if s.EmptyExit {
		step = append(step, "-f", os.DevNull)
	}
	step = append(step, "new-session")
	if s.Detached {
		step = append(step, "-d")
	}
	if s.Name != "" {
		step = append(step, "-s", s.Name)
		if !s.EmptyExit {
			step = append(step, "-n", s.Name)
		}
	}
	if s.Dir != "" {
		step = append(step, "-c", s.Dir)
	}
	for _, e := range s.Env {
		step = append(step, "-e", e)
	}
	step = append(step, s.Argv...)
	if s.EmptyExit {
		step = append(step, ";", "set-option", "-s", "exit-empty", "on")
	}
	return [][]string{step}
}

// Rename names the window in pane. It does not type into the agent, and it
// does not rename the session: that step needs the one-window flag.
func (Tmux) Rename(pane, name string) [][]string {
	tmux, id := targetArgv(pane)
	return [][]string{slices.Concat(tmux, []string{"rename-window", "-t", id, name})}
}

// Kill ends pane, then sends TERM to pid when pid is greater than zero.
// The second step names that pid and no other pane.
func (Tmux) Kill(pane string, pid int) [][]string {
	tmux, id := targetArgv(pane)
	steps := [][]string{slices.Concat(tmux, []string{"kill-pane", "-t", id})}
	if pid > 0 {
		steps = append(steps, []string{"kill", "-TERM", strconv.Itoa(pid)})
	}
	return steps
}

// KillSession ends the session name on this server. "=" names that session.
func (t Tmux) KillSession(name string) []string {
	return slices.Concat(t.commandArgv(), []string{"kill-session", "-t", "=" + name})
}

// Respawn starts argv again in pane. The pane id stays: the step is
// respawn-pane -k, never a new session.
func (Tmux) Respawn(pane, dir string, env []string, argv []string) [][]string {
	tmux, bare := targetArgv(pane)
	step := slices.Concat(tmux, []string{"respawn-pane", "-k", "-t", bare})
	if dir != "" {
		step = append(step, "-c", dir)
	}
	step = append(step, unsetEnv()...)
	step = append(step, env...)
	step = append(step, argv...)
	return [][]string{step}
}

func targetArgv(target string) (argv []string, bare string) {
	server, bare := SplitPane(target)
	return serverArgv(server), bare
}

// listArgv is the utf-8 argv for this socket. -u keeps tabs in -F output
// when LANG is unset; without it tmux in the C locale prints them as "_".
func (t Tmux) listArgv() []string {
	argv := []string{"tmux", "-u"}
	if t.Socket != "" {
		return append(argv, "-S", t.Socket)
	}
	if t.Server != "" {
		return append(argv, "-L", t.Server)
	}
	return argv
}

func utf8On(server string) []string {
	a := serverArgv(server)
	return append([]string{a[0], "-u"}, a[1:]...)
}

func captureStyled(panes []string) map[string]string {
	out := map[string]string{}
	for _, id := range panes {
		server, bare := SplitPane(id)
		argv := append(utf8On(server), "capture-pane", "-p", "-e", "-t", bare)
		b, err := commandOutput(argv)
		if err != nil {
			continue
		}
		out[id] = string(b)
	}
	return out
}

func commandOutput(argv []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandBound)
	defer cancel()
	c := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv built here, never a shell string
	c.WaitDelay = time.Second
	return c.Output()
}
