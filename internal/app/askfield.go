package app

import (
	"context"
	"slices"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/askpass"
)

// AskSource gives the prompts of the ssh the console starts (design §3.4):
// askpass.Server.
type AskSource interface {
	Next(context.Context) (askpass.Prompt, func(answer string, ok bool), error)
	Env(host string) []string
}

// askField is one ssh prompt in the console, above every other layer: the
// host, the prompt's text, and what the builder types, masked.
type askField struct {
	prompt askpass.Prompt
	answer func(string, bool)
	typed  []rune
	note   string // why ⏎ did not answer
}

// askMsg is the next prompt of the server, for the console of gen.
type askMsg struct {
	prompt askpass.Prompt
	answer func(string, bool)
	err    error
	gen    int
}

// askGoneMsg: the ssh of the prompt with this Gone stopped asking.
type askGoneMsg struct{ gone <-chan struct{} }

// waitAsk waits for the next prompt, as long as the console lives.
func (m Model) waitAsk() tea.Cmd {
	if m.opt.Ask == nil || m.askCtx == nil {
		return nil
	}
	ask, ctx, gen := m.opt.Ask, m.askCtx, m.opt.Gen
	return func() tea.Msg {
		p, answer, err := ask.Next(ctx)
		return askMsg{prompt: p, answer: answer, err: err, gen: gen}
	}
}

// stopAsk ends the console's wait for prompts, and its login check: an
// open prompt is cancelled.
func (m Model) stopAsk() {
	if m.askStop != nil {
		m.askStop()
	}
	if m.checkStop != nil {
		m.checkStop()
	}
}

// asked opens the field on a prompt. The server closed, or the console
// ended: no more prompts.
func (m Model) asked(msg askMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		return m, nil
	case msg.gen != m.opt.Gen || m.quitting || m.ask != nil:
		msg.answer("", false) // a prompt of a console that closed
		return m, nil
	}
	m.ask = &askField{prompt: msg.prompt, answer: msg.answer}
	m.pending, m.held = nil, nil // as a tap does: no key after the prompt confirms them
	gone := msg.prompt.Gone
	if gone == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		<-gone // it closes after the answer too
		return askGoneMsg{gone}
	}
}

// askGone closes the field of a prompt whose ssh stopped asking.
func (m Model) askGone(msg askGoneMsg) (tea.Model, tea.Cmd) {
	if m.ask == nil || m.ask.prompt.Gone != msg.gone {
		return m, nil
	}
	return m.closeAsk("", false, m.goneFlash())
}

// goneFlash says that the prompt's ssh ended before it had an answer.
func (m Model) goneFlash() string {
	return "ssh stopped asking: the ssh to " + m.askHost() + " ended before your answer"
}

// closeAsk answers the prompt, drops what was typed, and waits for the
// next prompt.
func (m Model) closeAsk(answer string, ok bool, flash string) (Model, tea.Cmd) {
	f := m.ask
	f.answer(answer, ok)
	clear(f.typed)
	m.ask = nil
	if flash != "" {
		m.flash = flash
	}
	return m, m.waitAsk()
}

func (m Model) askHost() string {
	switch {
	case m.ask == nil:
		return m.opt.Host
	case m.ask.prompt.Host != "":
		return m.ask.prompt.Host
	}
	return "a host"
}

// asking: the field is on top, with a prompt, with Tailscale's link, or
// waiting for the next prompt of an open-first (sshd waits after a wrong
// password).
func (m Model) asking() bool { return m.ask != nil || m.opening || m.loginURL != "" }

// askKey is a key on the field: ⏎ answers, esc and ctrl+c cancel the ssh,
// any other key is text. While the field waits, esc and ctrl+c end the
// open-first or the check, c copies Tailscale's link, and it takes every
// other key for nothing.
func (m Model) askKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	ak := m.keys.ask
	if m.ask == nil {
		switch {
		case bkey.Matches(k, ak.Copy) && m.loginURL != "":
			m.flash = "link copied"
			return m, m.clip(m.loginURL)
		case !bkey.Matches(k, ak.Cancel, m.keys.global.Quit):
		case m.openStop != nil:
			m.openStop()
			m.askCancelled = true
			m.flash = "cancelled: the login to " + m.opt.Host + "…"
		case m.checking && m.checkStop != nil:
			m.checkStop()
			m.checkCancelled = true
			m.flash = "cancelled: the Tailscale check of " + m.opt.Host + "…"
		}
		return m, nil
	}
	f := *m.ask
	switch {
	case bkey.Matches(k, ak.Cancel, m.keys.global.Quit):
		m.askCancelled = true
		if m.openStop != nil {
			m.openStop() // ssh would ask again: the open ends here
		}
		return m.closeAsk("", false, "cancelled: the ssh to "+m.askHost()+" gets no answer")
	case bkey.Matches(k, ak.Send):
		return m.sendAsk()
	case bkey.Matches(k, ak.Erase):
		if n := len(f.typed); n > 0 {
			f.typed = slices.Clip(f.typed[:n-1])
		}
	case k.Text != "":
		f.typed = typedRunes(f.typed, k.Text)
	default:
		return m, nil
	}
	f.note = ""
	m.ask = &f
	return m, nil
}

// askPaste types a paste into the field, without its line breaks.
func (m Model) askPaste(text string) Model {
	f := *m.ask
	f.typed, f.note = typedRunes(f.typed, text), ""
	m.ask = &f
	return m
}

// typedRunes appends the printable runes of text to a copy of typed.
func typedRunes(typed []rune, text string) []rune {
	out := slices.Clip(typed)
	for _, r := range text {
		if r >= ' ' && r != 0x7f {
			out = append(out, r)
		}
	}
	return out
}

// sendAsk answers the prompt with what was typed. A yes/no prompt needs a
// typed yes or no; a new key's passphrase cannot be empty.
func (m Model) sendAsk() (tea.Model, tea.Cmd) {
	f := *m.ask
	select {
	case <-f.prompt.Gone:
		return m.closeAsk("", false, m.goneFlash())
	default:
	}
	text := string(f.typed)
	switch {
	case f.prompt.Confirm && text != "yes" && text != "no":
		f.note = "type yes or no, then " + m.keys.ask.Send.Help().Key
	case text == "" && newPassphrase(f.prompt.Text):
		f.note = "a passphrase is needed: matchblox never makes a key without one"
	default:
		return m.closeAsk(text, true, "")
	}
	m.ask = &f
	return m, nil
}

// newPassphrase tells ssh-keygen's question for a new key's passphrase.
func newPassphrase(text string) bool { return strings.Contains(text, "empty for no passphrase") }

// askLines are the prompt's lines in w columns: a run of blank or
// whitespace-only lines is one blank line, and none leads or ends. The
// spaces of a line stay (a host key's randomart).
func askLines(text string, w int) []string {
	var out []string
	for l := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(l) == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(l, max(w, 4), ""), "\n")...)
	}
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

// askBox is the field in a frame of matchblox's own: the host in its
// title, the prompt's lines, the masked input and its keys. The prompt
// comes from ssh or the server, so it is only ever text inside the frame.
// It is at most maxH lines: the prompt's first lines go first, the
// question (its last line) stays.
func (m Model) askBox(bw, maxH int) []string {
	st, f := m.st, m.ask
	iw := bw - 4 - 2*consentPad
	text := askLines(f.prompt.Text, iw)
	input := []string{st.accent.Render("› ") + st.text.Render(m.askInput()) + st.accent.Render("▏")}
	if f.note != "" {
		input = append(input, st.warn.Render("! ")+st.text.Render(f.note))
	}
	room := max(maxH-2-3-len(input), 1) // the frame, the blank rows, the input
	if len(text) > room {
		text = text[len(text)-room:]
		if room > 1 {
			text[0] = "…"
		}
	}
	for i, l := range text {
		text[i] = st.muted.Render(l)
	}
	ak := m.keys.ask
	send := "send"
	if f.prompt.Confirm {
		send = "answer"
	}
	title := st.accent.Bold(true).Render("ssh " + m.askHost() + " asks")
	foot := []footPart{st.footKey(ak.Send, send, "", st.accent), st.footKey(ak.Cancel, "cancel the ssh", " ─ ", st.accent)}
	box, _ := boxFrame(st.hair, title, padded([][]string{text, input}, bw), foot, bw)
	return box
}

// linkBox is the field with Tailscale's check link, an OSC 8 link, while
// its ssh waits for the builder's browser. The link is a plain
// login.tailscale.com URL (remote.VisitURL); the console never opens it.
func (m Model) linkBox(bw int) []string {
	st, ak := m.st, m.keys.ask
	iw := max(bw-4-2*consentPad, 4)
	var text []string
	for _, l := range askLines("Tailscale SSH asks for a check. Open this link in a browser, then wait here:", iw) {
		text = append(text, st.muted.Render(l))
	}
	link := []string{""}
	for l := range strings.SplitSeq(ansi.Hardwrap(m.loginURL, iw, false), "\n") {
		link = append(link, ansi.SetHyperlink(m.loginURL)+st.accent.Render(l)+ansi.ResetHyperlink())
	}
	title := st.accent.Bold(true).Render("ssh " + m.askHost() + " asks")
	foot := []footPart{st.footKey(ak.Copy, "copy the link", "", st.accent), st.footKey(ak.Cancel, "cancel the ssh", " ─ ", st.accent)}
	box, _ := boxFrame(st.hair, title, padded([][]string{text, link}, bw), foot, bw)
	return box
}

// waitBox is the field between two prompts of an open-first: it holds
// the keys until ssh asks again or ends.
func (m Model) waitBox(bw int) []string {
	st := m.st
	title := st.accent.Bold(true).Render("ssh " + m.askHost() + " asks")
	foot := []footPart{st.footKey(m.keys.ask.Cancel, "cancel the ssh", "", st.accent)}
	box, _ := boxFrame(st.hair, title, padded([][]string{{st.muted.Render("waiting for ssh…")}}, bw), foot, bw)
	return box
}

// askInput is what was typed: a • per rune, or the words of a yes/no
// prompt, which are no secret.
func (m Model) askInput() string {
	f := m.ask
	if f.prompt.Confirm {
		return string(f.typed)
	}
	return strings.Repeat("•", len(f.typed))
}

// overlayAsk lays the field over the dimmed screen, which fills the
// terminal's rows: on a phone it rises from the key line, wider it sits
// in the middle.
func (m Model) overlayAsk(sc *drawn, w int) {
	for len(sc.lines) < m.height {
		sc.lines = append(sc.lines, "")
	}
	phone := m.layoutAt(w) == Narrow || m.upright()
	bw := min(w, 72)
	if !phone {
		bw = min(72, w-8)
	}
	var box []string
	switch {
	case m.ask != nil:
		box = m.askBox(bw, len(sc.lines)-3)
	case m.loginURL != "":
		box = m.linkBox(bw)
	default:
		box = m.waitBox(bw)
	}
	m.overlayBox(sc, w, bw, phone, box, make([]string, len(box)), nil)
}
