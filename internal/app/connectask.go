package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

// connectDoneMsg: the connect of the field of gen ended, with the stderr
// and exit of `matchblox connect`.
type connectDoneMsg struct {
	gen    int
	stderr string
	exit   int
}

// connectField is the field of a connect on Hosts (#242): a console with no
// service, only its ssh prompts, so a connect asks as a jump does. It asks
// only what ssh asks: a login by key, agent or Tailscale SSH asks nothing.
func (s Shell) connectField(host string, gen int) Model {
	b := s.opt.Base
	m := New(Options{Host: host, Ask: b.Ask, SSH: b.SSH, Gen: gen, Clip: b.Clip, Layered: true})
	m.width, m.height, m.dark, m.st = s.width, s.height, s.dark, s.st
	m.splashDone = true
	m.editLogin(host, func(l *hostLogin) { l.tailscale = s.tailscale[host] })
	return m
}

// connect runs argv (`matchblox connect <host>`) in the background, with
// its ssh's prompts in the field, until it ends or esc ends it.
func (m Model) connect(argv []string) (Model, tea.Cmd) {
	ctx, stop := context.WithCancel(m.askCtx)
	m.editLogin(m.opt.Host, func(l *hostLogin) { l.opening, l.openStop, l.askCancelled = true, stop, false })
	ssh, env, gen := m.ssh(), m.opt.Ask.Env(m.opt.Host), m.opt.Gen
	run := streamed(ctx, gen, m.loginOf(m.opt.Host).tailscale, openTimeout, openTimeout, func(ctx context.Context, line func(string)) tea.Msg {
		defer stop()
		stderr, exit := ssh(ctx, argv, env, line)
		return connectDoneMsg{gen: gen, stderr: stderr, exit: exit}
	})
	return m, tea.Batch(m.waitAsk(), run)
}

// connectErr is why a connect failed, nil when it did not: the last line
// `matchblox connect` printed that is not ssh's debug, which names the fix.
func connectErr(msg connectDoneMsg, cancelled bool) error {
	switch {
	case cancelled:
		return errors.New("cancelled: the ssh got no answer")
	case msg.exit == 0:
		return nil
	}
	// A host can write there: its control characters never reach the screen.
	last := strings.TrimPrefix(remote.Said(msg.stderr), "matchblox: ")
	if last == "" {
		last = "matchblox connect exited " + strconv.Itoa(msg.exit)
	}
	return errors.New(last)
}

// connectUpdate gives the field of a running connect its messages and
// every key: nothing under it acts. ok is false for a message it leaves.
func (s Shell) connectUpdate(msg tea.Msg) (Shell, tea.Cmd, bool) {
	f := *s.connecting
	var next tea.Model
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case connectDoneMsg:
		if msg.gen != f.opt.Gen {
			return s, nil, true
		}
		f.stopAsk()
		s.connecting = nil
		cancelled := f.loginOf(f.opt.Host).askCancelled
		return s, func() tea.Msg { return connectedHostMsg{f.opt.Host, connectErr(msg, cancelled)} }, true
	case askMsg:
		next, cmd = f.asked(msg)
	case askGoneMsg:
		next, cmd = f.askGone(msg)
	case urlMsg:
		next, cmd = f.linkShown(msg)
	case tea.KeyPressMsg:
		next, cmd = f.askKey(msg)
	case tea.PasteMsg:
		if f.ask != nil {
			next = f.askPaste(msg.Content)
		} else {
			next = f
		}
	case tea.MouseClickMsg, tea.MouseWheelMsg:
		return s, nil, true
	default:
		return s, nil, false
	}
	f = next.(Model)
	if f.flash != "" {
		s.flash, f.flash = f.flash, ""
	}
	s.connecting = &f
	return s, cmd, true
}
