package mux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
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

// HomeEnv is the env var that marks the home console's own session. It is
// the same name as panes.HomeEnv: mux must not import panes (panes imports
// sample, and sample imports mux).
const HomeEnv = "MATCHBLOX_HOME"

// Type is the send-keys step that types text into pane, as literal text. A
// final ";" goes as "\;" so tmux does not read it as a command end.
func (t Tmux) Type(pane, text string) []string {
	if t, ok := strings.CutSuffix(text, ";"); ok {
		text = t + `\;`
	}
	tmux, id := t.targetArgv(pane)
	return slices.Concat(tmux, []string{"send-keys", "-t", id, "-l", "--", text})
}

// Keys is the send-keys step that presses keys (tmux key names) in pane.
func (t Tmux) Keys(pane string, keys ...string) []string {
	tmux, id := t.targetArgv(pane)
	return slices.Concat(tmux, []string{"send-keys", "-t", id}, keys)
}

func (t Tmux) Load(pane, name, text string) error {
	tmux, _ := t.targetArgv(pane)
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
	tmux, id := t.targetArgv(pane)
	out, err := t.run(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{bracket_paste_flag}"}))
	return err == nil && strings.TrimSpace(out) == "0"
}

func (t Tmux) Paste(pane, name string) []string {
	tmux, id := t.targetArgv(pane)
	return slices.Concat(tmux, []string{"paste-buffer", "-p", "-d", "-b", name, "-t", id})
}

func (t Tmux) Drop(pane, name string) error {
	tmux, _ := t.targetArgv(pane)
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
	tmux, id := t.targetArgv(pane)
	out, err := t.run(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{pane_in_mode}"}))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

func (t Tmux) TypedAt(pane string) time.Time {
	tmux, id := t.targetArgv(pane)
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
	tmux, id := t.targetArgv(pane)
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

// Output runs args on this server and gives what tmux prints. ctx ends the
// read. With Run set, Run reads it, as it does for every other call.
func (t Tmux) Output(ctx context.Context, args ...string) (string, error) {
	argv := append(t.commandArgv(), args...)
	if t.Run != nil {
		return t.Run(argv)
	}
	c := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv built here, never a shell string
	out, err := c.Output()
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

// Command is the argv prefix that reaches the server ("": the builder's):
// the program, its flags (-u keeps tabs and glyphs in output when LANG is
// unset), then -L server. Every call aimed at a pane starts with it. It
// returns a new slice each time, so the caller may append.
func Command(server string, flags ...string) []string {
	argv := append([]string{"tmux"}, flags...)
	if server == "" {
		return argv
	}
	return append(argv, "-L", server)
}

func serverArgv(server string) []string { return Command(server) }

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
	return []string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", HomeEnv}
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
func (t Tmux) Rename(pane, name string) [][]string {
	tmux, id := t.targetArgv(pane)
	return [][]string{slices.Concat(tmux, []string{"rename-window", "-t", id, name})}
}

// RenameSession names the session of pane. Rename leaves it out: the
// caller asks for this step only when the session has one window.
func (t Tmux) RenameSession(pane, name string) []string {
	tmux, id := t.targetArgv(pane)
	return slices.Concat(tmux, []string{"rename-session", "-t", id, name})
}

// Kill ends pane, then sends TERM to pid when pid is greater than zero.
// The second step names that pid and no other pane.
func (t Tmux) Kill(pane string, pid int) [][]string {
	tmux, id := t.targetArgv(pane)
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

// HasSession is the step that asks if the session name is on this server:
// it fails when it is not. "=" names that session.
func (t Tmux) HasSession(name string) []string {
	return slices.Concat(t.commandArgv(), []string{"has-session", "-t", "=" + name})
}

// Respawn starts argv again in pane. The pane id stays: the step is
// respawn-pane -k, never a new session.
func (t Tmux) Respawn(pane, dir string, env []string, argv []string) [][]string {
	tmux, bare := t.targetArgv(pane)
	step := slices.Concat(tmux, []string{"respawn-pane", "-k", "-t", bare})
	if dir != "" {
		step = append(step, "-c", dir)
	}
	step = append(step, unsetEnv()...)
	step = append(step, env...)
	step = append(step, argv...)
	return [][]string{step}
}

// Split opens a pane beside target and returns its id, with the server
// (OnServer) as List writes it. The command follows --, so no word of argv
// is a flag.
func (t Tmux) Split(target string, argv []string) (string, error) {
	return t.split(target, 0, argv)
}

// Beside opens a pane to the right of target without taking the keys, and
// gives it percent of the window's width. argv follows --. The id is as
// Split gives it.
func (t Tmux) Beside(target string, percent int, argv []string) (string, error) {
	return t.split(target, percent, argv)
}

// split is split-window. A percent above 0 is the side-by-side split: -h
// -d -t target -l percent%.
func (t Tmux) split(target string, percent int, argv []string) (string, error) {
	prefix, server := t.commandArgv(), t.Server
	if sv, bare := SplitPane(target); sv != "" {
		prefix, server, target = serverArgv(sv), sv, bare
	}
	step := slices.Concat(prefix, []string{"split-window"})
	if percent > 0 {
		step = append(step, "-h", "-d")
	}
	step = append(step, "-t", target)
	if percent > 0 {
		step = append(step, "-l", strconv.Itoa(percent)+"%")
	}
	step = append(step, "-P", "-F", "#{pane_id}")
	if len(argv) > 0 {
		step = append(step, "--")
		step = append(step, argv...)
	}
	out, err := t.run(step)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if !strings.HasPrefix(id, "%") {
		return "", errors.New("tmux split-window printed no pane id")
	}
	return OnServer(id, server), nil
}

// Focus brings pane to the front: select-window, then select-pane. It does
// not attach and it does not switch a client.
func (t Tmux) Focus(pane string) [][]string {
	tmux, id := t.targetArgv(pane)
	return [][]string{
		slices.Concat(tmux, []string{"select-window", "-t", id}),
		slices.Concat(tmux, []string{"select-pane", "-t", id}),
	}
}

// SwitchClient is the step that moves the client that asked to pane.
func (t Tmux) SwitchClient(pane string) []string {
	tmux, id := t.targetArgv(pane)
	return slices.Concat(tmux, []string{"switch-client", "-t", id})
}

// NewWindow opens a window of the client's own server. A name or a dir that
// is empty is left to tmux; argv follows the flags, and an empty argv is
// the shell.
func (Tmux) NewWindow(name, dir string, argv ...string) []string {
	step := Command("")
	step = append(step, "new-window")
	if name != "" {
		step = append(step, "-n", name)
	}
	if dir != "" {
		step = append(step, "-c", dir)
	}
	return append(step, argv...)
}

// ViewCmd is "tmux": the name tmux shows for a pane that runs a nested tmux
// client, such as the agent pane's view.
func (Tmux) ViewCmd() string { return "tmux" }

// lastClientBound is how long LastClient waits for tmux: the guard asks
// on a key press.
const lastClientBound = 500 * time.Millisecond

// LastClient asks tmux for the clients of session. "=" names that session.
// A tmux error or no session gives "": the guard is not a wall.
func (t Tmux) LastClient(session string) (string, time.Time, error) {
	step := slices.Concat(t.commandArgv(), []string{"list-clients", "-t", "=" + session, "-F", "#{client_activity} #{client_created} #{client_tty}"})
	var out string
	var err error
	if t.Run != nil {
		out, err = t.Run(step)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), lastClientBound)
		defer cancel()
		var b []byte
		b, err = exec.CommandContext(ctx, step[0], step[1:]...).Output() //nolint:gosec // argv built here, never a shell string
		out = string(b)
	}
	if err != nil {
		return "", time.Time{}, nil
	}
	tty, created := lastClientOf(out)
	return tty, created, nil
}

// lastClientOf is the tty and the creation time of the client with the
// latest activity in the output of list-clients. "" when no line parses.
func lastClientOf(out string) (tty string, created time.Time) {
	best := int64(-1)
	for l := range strings.Lines(out) {
		f := strings.Fields(l)
		if len(f) != 3 {
			continue
		}
		at, err := strconv.ParseInt(f[0], 10, 64)
		made, err2 := strconv.ParseInt(f[1], 10, 64)
		if err == nil && err2 == nil && at > best {
			best, tty, created = at, f[2], time.Unix(made, 0)
		}
	}
	return tty, created
}

// Attach gives the terminal to session. It is a step: the caller hands
// over the terminal.
func (t Tmux) Attach(session string) []string {
	return slices.Concat(t.commandArgv(), []string{"attach-session", "-t", session})
}

// Caller is the pane id tmux gave this process.
func (Tmux) Caller() string { return os.Getenv("TMUX_PANE") }

// targetArgv is the tmux prefix that reaches pane, and the id that server knows
// the pane by. A pane with an @server suffix names its own server; one with
// none is on this socket.
func (t Tmux) targetArgv(target string) (argv []string, bare string) {
	server, bare := SplitPane(target)
	if server == "" {
		return t.commandArgv(), bare
	}
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

func utf8On(server string) []string { return Command(server, "-u") }

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
