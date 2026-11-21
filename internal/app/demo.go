package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
		m.pending = nil
		m.sayError(line)
	}
	if m.form != nil && strings.Contains(m.form.err, line) {
		m.form = nil
		m.sayError(line)
	}
	if m.flash != line && strings.Contains(m.flash, line) {
		m.unsay(m.flash) // the line says it alone, once
		m.sayError(line)
	}
	if strings.Contains(m.recRun.out, line) {
		m.recRun.out = line
	}
	return m
}

// keyRows are the hair above the key line and the key line. While this
// console's switch waits for the record, the wait takes the hair's row and
// esc's word the key line.
func (m Model) keyRows(w int) (hair, keys string, spans []span) {
	if wait, keys, spans, ok := m.waitRows(w); ok {
		return wait, keys, spans
	}
	keys, spans = m.keyLine(w)
	return m.st.hair.Render(strings.Repeat("─", w)), keys, spans
}
