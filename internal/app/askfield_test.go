package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/askpass"
)

// fakeAsk is an askpass server with no socket: Next waits until its
// context ends, and Env names the host.
type fakeAsk struct{}

func (fakeAsk) Next(ctx context.Context) (askpass.Prompt, func(string, bool), error) {
	<-ctx.Done()
	return askpass.Prompt{}, nil, ctx.Err()
}

func (fakeAsk) Env(host string) []string {
	return []string{"SSH_ASKPASS=/bin/matchblox", "SSH_ASKPASS_REQUIRE=force", "MATCHBLOX_ASKPASS=/s", "MATCHBLOX_ASKPASS_TOKEN=t", "MATCHBLOX_ASKPASS_HOST=" + host}
}

// answers records what the field answered.
type answers struct {
	got  []string
	oks  []bool
	sent int
}

func (a *answers) answer(s string, ok bool) {
	a.got, a.oks = append(a.got, s), append(a.oks, ok)
	a.sent++
}

// asking is the console with an ssh prompt open.
func asking(t *testing.T, p askpass.Prompt) (Model, *answers) {
	t.Helper()
	m, _ := loadedWith(t, 100, fixtureState())
	m.opt.Ask = fakeAsk{}
	m.askCtx, m.askStop = context.WithCancel(context.Background())
	t.Cleanup(m.askStop)
	a := &answers{}
	next, _ := m.Update(askMsg{prompt: p, answer: a.answer})
	m = next.(Model)
	if m.ask == nil {
		t.Fatal("the prompt did not open the field")
	}
	return m, a
}

func typeIn(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = key(m, string(r))
	}
	return m
}

func TestAskFieldMasks(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "builder@ws-1's password: "})
	next := typeIn(m, "abc")
	out := ansi.Strip(next.(Model).render())
	if !strings.Contains(out, "•••") || strings.Contains(out, "abc") {
		t.Fatalf("the field must mask what is typed:\n%s", out)
	}
	if !strings.Contains(out, "ws-1") || !strings.Contains(out, "builder@ws-1's password:") {
		t.Fatalf("the field must name the host and show the prompt:\n%s", out)
	}
	next, _ = key(next, "enter")
	if a.sent != 1 || a.got[0] != "abc" || !a.oks[0] {
		t.Fatalf("answers %q %v", a.got, a.oks)
	}
	if next.(Model).ask != nil {
		t.Fatal("the field stays open after its answer")
	}
}

func TestAskFieldEscCancels(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: "})
	next, _ := key(typeIn(m, "ab"), "esc")
	if a.sent != 1 || a.oks[0] || a.got[0] != "" {
		t.Fatalf("esc must cancel: answers %q %v", a.got, a.oks)
	}
	nm := next.(Model)
	if nm.ask != nil {
		t.Fatal("the field stays open after esc")
	}
	if !strings.Contains(nm.flash, "cancelled") || !strings.Contains(nm.flash, "ssh") {
		t.Fatalf("flash %q", nm.flash)
	}
}

// TestAskFieldCtrlCCancels: ctrl+c on the field cancels the ssh; it does
// not quit the console with a prompt open.
func TestAskFieldCtrlCCancels(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: "})
	next, _ := key(m, "ctrl+c")
	if a.sent != 1 || a.oks[0] || next.(Model).quitting {
		t.Fatalf("ctrl+c: answers %v, quitting %v", a.oks, next.(Model).quitting)
	}
}

func TestAskFieldConfirmNeedsTypedYes(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Are you sure you want to continue connecting (yes/no/[fingerprint])? ", Confirm: true})
	next, _ := key(typeIn(m, "y"), "enter")
	if a.sent != 0 || next.(Model).ask == nil {
		t.Fatalf("y alone answered: %q", a.got)
	}
	if !strings.Contains(ansi.Strip(next.(Model).render()), "type yes") {
		t.Fatalf("the field must say to type yes:\n%s", ansi.Strip(next.(Model).render()))
	}
	key(typeIn(next, "es"), "enter")
	if a.sent != 1 || a.got[0] != "yes" || !a.oks[0] {
		t.Fatalf("answers %q %v", a.got, a.oks)
	}
}

func TestAskFieldRefusesEmptyNewPassphrase(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Enter passphrase (empty for no passphrase): "})
	next, _ := key(m, "enter")
	if a.sent != 0 || next.(Model).ask == nil {
		t.Fatalf("an empty passphrase was sent: %q", a.got)
	}
	if out := ansi.Strip(next.(Model).render()); !strings.Contains(out, "a passphrase is needed") {
		t.Fatalf("the field must say why:\n%s", out)
	}
	key(typeIn(next, "pw"), "enter")
	if a.sent != 1 || a.got[0] != "pw" {
		t.Fatalf("answers %q", a.got)
	}
}

// TestAskFieldEraseAndPaste: backspace drops the last rune; a paste is
// typed, without its line breaks.
func TestAskFieldEraseAndPaste(t *testing.T) {
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: "})
	next, _ := key(typeIn(m, "abx"), "backspace")
	next, _ = next.Update(tea.PasteMsg{Content: "cd\r\n"})
	key(next, "enter")
	if a.sent != 1 || a.got[0] != "abcd" {
		t.Fatalf("answers %q", a.got)
	}
}

// TestAskFieldGoneHelper: a prompt whose ssh stopped asking closes the
// field, frees the server's turn, and says so.
func TestAskFieldGoneHelper(t *testing.T) {
	gone := make(chan struct{})
	m, a := asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: ", Gone: gone})
	close(gone)
	next, _ := m.Update(askGoneMsg{gone: gone})
	nm := next.(Model)
	if nm.ask != nil || a.sent != 1 || a.oks[0] {
		t.Fatalf("field %v, answers %v", nm.ask, a.oks)
	}
	if !strings.Contains(nm.flash, "ssh stopped asking") {
		t.Fatalf("flash %q", nm.flash)
	}
	// Enter on a prompt whose helper left before the message came.
	gone2 := make(chan struct{})
	m, a = asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: ", Gone: gone2})
	close(gone2)
	next, _ = key(typeIn(m, "pw"), "enter")
	if next.(Model).ask != nil || a.oks[0] || !strings.Contains(next.(Model).flash, "ssh stopped asking") {
		t.Fatalf("enter on a gone prompt: answers %q %v, flash %q", a.got, a.oks, next.(Model).flash)
	}
	// A late message of an earlier prompt leaves the open one.
	m, _ = asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: ", Gone: make(chan struct{})})
	next, _ = m.Update(askGoneMsg{gone: gone})
	if next.(Model).ask == nil {
		t.Fatal("a late message closed the open field")
	}
}

// TestAskFieldShowsTheQuestion: whitespace-only lines collapse, and a text
// taller than the screen drops lines from the top: the question stays.
func TestAskFieldShowsTheQuestion(t *testing.T) {
	text := strings.Repeat("\n \n\t\n", 50) + strings.Repeat("banner line\n", 60) + "builder@ws-1's password: "
	m, _ := asking(t, askpass.Prompt{Host: "ws-1", Text: text})
	for _, h := range []int{12, 30} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 44, Height: h})
		out := ansi.Strip(next.(Model).render())
		if !strings.Contains(out, "builder@ws-1's password:") || !strings.Contains(out, "•") && !strings.Contains(out, "›") {
			t.Fatalf("height %d: the question is off screen:\n%s", h, out)
		}
		if n := strings.Count(out, "\n") + 1; n > h {
			t.Fatalf("height %d: %d lines", h, n)
		}
	}
	if got := askLines("\n \n \n\nquestion? ", 40); len(got) != 1 || got[0] != "question? " {
		t.Fatalf("askLines %q", got)
	}
	if got := askLines("a\n \n\t\nb", 40); strings.Join(got, "|") != "a||b" {
		t.Fatalf("askLines %q", got)
	}
}

// TestAskFieldAboveEverything: a prompt drops a confirm that waits (as a
// tap does): no key typed into the field, or after it, confirms it.
func TestAskFieldAboveEverything(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	m.tab = tabSessions
	next, _ := key(m, "enter")
	if next.(Model).pending == nil {
		t.Fatal("no confirm")
	}
	a := &answers{}
	next, _ = next.Update(askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "Password: "}, answer: a.answer})
	if next.(Model).pending != nil {
		t.Fatal("the confirm stays under the prompt")
	}
	next, _ = key(next, "y")
	next, _ = key(next, "enter")
	if a.sent != 1 || a.got[0] != "y" || next.(Model).pending != nil {
		t.Fatalf("answers %q, pending %v", a.got, next.(Model).pending)
	}
}

// TestAskFieldWaitsForTheNext: an answered prompt asks the server for the
// next one; a prompt of a closed console is cancelled.
func TestAskFieldWaitsForTheNext(t *testing.T) {
	m, _ := asking(t, askpass.Prompt{Host: "ws-1", Text: "Password: "})
	if _, cmd := key(m, "enter"); cmd == nil {
		t.Fatal("no wait for the next prompt")
	}
	a := &answers{}
	m.opt.Gen = 2
	next, _ := m.Update(askMsg{prompt: askpass.Prompt{Host: "ws-1", Text: "late: "}, answer: a.answer, gen: 1})
	if a.sent != 1 || a.oks[0] || next.(Model).ask.prompt.Text == "late: " {
		t.Fatalf("a late prompt of a closed console: answers %v", a.oks)
	}
}
