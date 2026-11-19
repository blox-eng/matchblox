package service

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// A copy made in a tmux server the service watches (a drag, a double-click,
// any new paste buffer) goes to each console, which puts it on the screen
// its person uses. tmux's set-clipboard stays as the person has it: no pane
// reads or writes the clipboard through tmux (spec R7).

const (
	// copyWatchSession is the watcher's own session: a control-mode client
	// needs one. It carries JumpMark, so the sampler does not list it.
	copyWatchSession = "matchblox-copywatch"
	copyLimit        = 1 << 20
	copyAgain        = time.Second
	copyServersEvery = 5 * time.Second
	noConsole        = "not copied: no matchblox console is connected · open the console"
)

// copyLoop keeps one copy watcher on each tmux server the sessions run on.
func (s *Service) copyLoop(ctx context.Context) {
	watched := map[string]context.CancelFunc{}
	t := time.NewTicker(copyServersEvery)
	defer t.Stop()
	for {
		now := s.copySockets()
		for sock, stop := range watched {
			if !now[sock] {
				stop()
				delete(watched, sock)
			}
		}
		for sock := range now {
			if watched[sock] == nil {
				wctx, stop := context.WithCancel(ctx)
				watched[sock] = stop
				go s.copyWatch(wctx, sock)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// copySockets are the sockets of the tmux servers the panes run on.
func (s *Service) copySockets() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	socks := map[string]bool{}
	for _, p := range s.cur.Panes {
		if p.Socket != "" {
			socks[p.Socket] = true
		}
	}
	return socks
}

// copyWatch watches the paste buffers of the tmux server at sock until ctx
// ends, attached again when its client ends.
func (s *Service) copyWatch(ctx context.Context, sock string) {
	for ctx.Err() == nil {
		s.copyWatchOnce(ctx, sock)
		select {
		case <-ctx.Done():
			return
		case <-time.After(copyAgain):
		}
	}
}

func copyTmux(ctx context.Context, sock string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "tmux", append([]string{"-S", sock}, args...)...).Output() //nolint:gosec // argv, never a shell string
}

// copyWatchOnce attaches one control-mode client and reads its notices until
// it ends. It never starts a server: on a socket with none it returns.
func (s *Service) copyWatchOnce(ctx context.Context, sock string) {
	if _, err := copyTmux(ctx, sock, "list-sessions"); err != nil {
		return
	}
	if _, err := copyTmux(ctx, sock, "has-session", "-t", "="+copyWatchSession); err != nil {
		target := "=" + copyWatchSession + ":"
		if _, err := copyTmux(ctx, sock, "new-session", "-d", "-s", copyWatchSession, ";",
			"set-option", "-t", target, "destroy-unattached", "off", ";",
			"set-option", "-t", target, sample.JumpMark, "1"); err != nil {
			return
		}
	}
	cmd := exec.CommandContext(ctx, "tmux", "-S", sock, "-C", "attach-session", //nolint:gosec // argv, never a shell string
		"-f", "no-output,ignore-size,read-only", "-t", "="+copyWatchSession)
	in, err := cmd.StdinPipe() // a control client ends when its input does
	if err != nil {
		return
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	defer cmd.Wait() //nolint:errcheck // the client ends when the session or the server does
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if name, ok := strings.CutPrefix(sc.Text(), "%paste-buffer-changed "); ok {
			s.copied(ctx, sock, name)
		}
	}
}

// copied sends the buffer name to each console, or tells the people at
// the server why it did not.
func (s *Service) copied(ctx context.Context, sock, name string) {
	if strings.HasPrefix(name, proto.CopyBuffer) {
		return // a console's own copy, on its way to a screen
	}
	text, err := copyTmux(ctx, sock, "show-buffer", "-b", name)
	if err != nil {
		return // the buffer went before we read it
	}
	if len(text) > copyLimit {
		copyNotice(ctx, sock, fmt.Sprintf("not copied: %.1f MB is over the 1 MB copy limit", float64(len(text))/(1<<20)))
		return
	}
	if !s.sendCopy(noControls(string(text))) {
		copyNotice(ctx, sock, noConsole)
	}
}

// sendCopy sends text to each console; false when none is connected. A
// copy, as a screen, goes only to a console: a client in an agent's pane is
// none, whatever its hello says, and the pane is read again at each copy.
func (s *Service) sendCopy(text string) bool {
	s.mu.Lock()
	var said []*client
	for cl := range s.clients {
		if cl.console {
			said = append(said, cl)
		}
	}
	s.mu.Unlock()
	var consoles []*client
	for _, cl := range said {
		if s.aConsole(true, cl.from) {
			consoles = append(consoles, cl)
		}
	}
	for _, cl := range consoles {
		go cl.c.Send(proto.KindCopy, "", proto.Copy{Text: text}) //nolint:errcheck // a console that left misses it
	}
	return len(consoles) > 0
}

// copyNotice shows text to each person's client of the server at sock.
func copyNotice(ctx context.Context, sock, text string) {
	out, err := copyTmux(ctx, sock, "list-clients", "-F", "#{client_control_mode} #{client_name}")
	if err != nil {
		return
	}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name, ok := strings.CutPrefix(l, "0 "); ok {
			_, _ = copyTmux(ctx, sock, "display-message", "-c", name, "-d", "4000", text)
		}
	}
}

// noControls drops the control codes from a copy, but tab and newline: a
// person's tmux copy has none, and an escape code from a program's buffer
// must not reach a clipboard a person pastes into a terminal.
func noControls(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, text)
}
