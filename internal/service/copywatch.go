package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/proto"
)

// A copy made in a tmux server the service watches (a drag, a double-click,
// any new paste buffer) goes to each console, which puts it on the screen
// its person uses. tmux's set-clipboard stays as the person has it, so no
// pane reads the clipboard; a buffer a program sets reaches it only under
// the console's guard (spec R7 and the 2026-11-20 ruling).

const (
	copyLimit        = 1 << 20
	copyEvery        = 300 * time.Millisecond
	copyServersEvery = 5 * time.Second
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
// ends: it reads the list every copyEvery and sends each buffer that came
// since the last read. It adds no session or client to the server and never
// starts one: on a socket with no server the read fails, and it reads again.
func (s *Service) copyWatch(ctx context.Context, sock string) {
	var seen map[string]bool // nil until the first read: what is there then is no new copy
	t := time.NewTicker(copyEvery)
	defer t.Stop()
	for {
		out, err := copyTmux(ctx, sock, "list-buffers", "-F", "#{buffer_name}\t#{buffer_created}\t#{buffer_size}")
		if err == nil {
			now := map[string]bool{}
			newest := ""
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if l == "" {
					continue
				}
				now[l] = true
				if seen != nil && !seen[l] && newest == "" {
					newest = l // list-buffers lists the newest first
				}
			}
			if name, _, ok := strings.Cut(newest, "\t"); ok {
				s.copied(ctx, sock, name)
			}
			seen = now
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func copyTmux(ctx context.Context, sock string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "tmux", append([]string{"-S", sock}, args...)...).Output() //nolint:gosec // argv, never a shell string
}

// copied sends the buffer name to each console. With no console it says
// nothing: tmux's own copy reaches a terminal attached to it.
func (s *Service) copied(ctx context.Context, sock, name string) {
	if strings.HasPrefix(name, proto.CopyBuffer) {
		return // a console's own copy, on its way to a screen
	}
	text, err := copyTmux(ctx, sock, "show-buffer", "-b", name)
	if err != nil {
		return // the buffer went before we read it
	}
	if len(text) > copyLimit {
		copyNotice(ctx, sock, fmt.Sprintf("not carried: %.1f MB is over matchblox's 1 MB copy limit", float64(len(text))/(1<<20)))
		return
	}
	s.sendCopy(noControls(string(text)))
}

// sendCopy sends text to each console. A copy, as a screen, goes only to a
// console: a client in an agent's pane is none, whatever its hello says, and
// the pane is read again at each copy.
func (s *Service) sendCopy(text string) {
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
