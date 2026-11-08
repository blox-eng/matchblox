package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// closeTheMachine answers each seam left to the real machine with the demo's
// line, so that no default of Options runs a command.
func closeTheMachine(opt Options) Options {
	refused := errors.New(opt.Demo)
	if opt.Run == nil {
		opt.Run = func([]string) error { return refused }
	}
	if opt.Exec == nil {
		opt.Exec = func(_ []string, done func(error) tea.Msg) tea.Cmd {
			return func() tea.Msg { return done(refused) }
		}
	}
	if opt.Clients == nil {
		opt.Clients = func() ([]string, error) { return nil, refused }
	}
	if opt.ClientList == nil {
		opt.ClientList = func() (string, error) { return "", refused }
	}
	if opt.LastClient == nil {
		opt.LastClient = func() (string, time.Time, error) { return "", time.Time{}, refused }
	}
	if opt.Check == nil {
		opt.Check = func(context.Context, []string, func(string)) (string, int) { return opt.Demo, 255 }
	}
	if opt.SSH == nil {
		opt.SSH = func(context.Context, []string, []string, func(string)) (string, int) { return opt.Demo, 255 }
	}
	return opt
}

// leavesTheDemo: a runs on the machine or goes to another: the terminal, tmux,
// ssh, a door's command or the pair page. Each is something a phone cannot do.
func (a action) leavesTheDemo() bool {
	return a.nav || a.attach || a.term || a.remote || a.connect || a.pair || a.climb != nil
}

// inDemo is the console's words after each message on the demo host: an act a
// phone cannot do never gets as far as its confirm, and however a refusal
// reached the console, the person reads the line itself.
func (m Model) inDemo() Model {
	line := m.opt.Demo
	if line == "" {
		return m
	}
	if m.pending != nil && m.pending.leavesTheDemo() {
		m.pending, m.flash = nil, line
	}
	if m.form != nil && strings.Contains(m.form.err, line) {
		m.form, m.flash = nil, line
	}
	if strings.Contains(m.flash, line) {
		m.flash = line
	}
	if strings.Contains(m.recRun.out, line) {
		m.recRun.out = line
	}
	return m
}

// keyRows are the hair above the key line and the key line. The demo's line
// does not fit a phone's width: where it shows, it takes the hair's row too.
func (m Model) keyRows(w int) (hair, keys string, spans []span) {
	hair = m.st.hair.Render(strings.Repeat("─", w))
	keys, spans = m.keyLine(w)
	if m.opt.Demo == "" || ansi.StringWidth(m.opt.Demo)+1 <= w || m.flash != m.opt.Demo ||
		keys != fit(" "+m.st.text.Render(m.flash), w) {
		return hair, keys, spans
	}
	first, second, ok := strings.Cut(m.opt.Demo, ". ") // each sentence its own row
	if !ok || ansi.StringWidth(first)+2 > w || ansi.StringWidth(second)+1 > w {
		return hair, keys, spans
	}
	return fit(" "+m.st.text.Render(first+"."), w), fit(" "+m.st.text.Render(second), w), nil
}
