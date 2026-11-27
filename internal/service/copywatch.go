package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/blox-eng/matchblox/internal/mux"
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
	m := copyMux(sock)
	var seen map[mux.Buffer]bool // nil until the first read: what is there then is no new copy
	t := time.NewTicker(copyEvery)
	defer t.Stop()
	for {
		bufs, err := m.ListBuffers(ctx)
		if err == nil {
			now := map[mux.Buffer]bool{}
			newest := ""
			for _, b := range bufs {
				now[b] = true
				if seen != nil && !seen[b] && newest == "" {
					newest = b.Name // the list has the newest first
				}
			}
			if newest != "" {
				s.copied(ctx, m, newest)
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

// copyMux is the multiplexer of the server at sock (-S). It has no Run: a
// read ends with the watcher's context.
func copyMux(sock string) mux.Mux { return mux.Tmux{Socket: sock} }

// copied sends the buffer name to each console. With no console it says
// nothing: tmux's own copy reaches a terminal attached to it.
func (s *Service) copied(ctx context.Context, m mux.Mux, name string) {
	if strings.HasPrefix(name, proto.CopyBuffer) {
		return // a console's own copy, on its way to a screen
	}
	text, err := m.ShowBuffer(ctx, name)
	if err != nil {
		return // the buffer went before we read it
	}
	if len(text) > copyLimit {
		m.Notice(ctx, fmt.Sprintf("not carried: %.1f MB is over matchblox's 1 MB copy limit", float64(len(text))/(1<<20)))
		return
	}
	s.sendCopy(noControls(text))
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
