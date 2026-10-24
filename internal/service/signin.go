package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// signInWait and signInEvery are how long the driver waits on each of a new
// account's screens, and how often it looks at the pane.
const (
	signInWait  = 20 * time.Second
	signInEvery = 500 * time.Millisecond
)

// SignInByHand is the note a pane gets when matchblox gave up driving its
// sign-in: the builder finishes it.
const SignInByHand = "by hand"

// signInScreen is one of an agent's first screens on a new account: its
// title, shown anywhere on it; the label of the option its key picks, or
// moves from, which must be the highlighted line ("" on a screen without
// options); and the key. final marks the screen after which the link
// shows.
type signInScreen struct {
	title string
	pick  string
	keys  []string
	final bool
}

// signInScreens are the first screens of each agent on a new account, in
// the order the agent shows them. Codex's two screens are one picker: Down
// moves from its first option to device code, and Enter picks that. The
// titles, labels and highlight marks (signInMarks) were checked live on
// codex-cli 0.163.0 and Claude Code 2.1.308, up to the login choice;
// Cursor 2026.10.19's screen was probed for its title only.
var signInScreens = map[string][]signInScreen{
	"claude": {
		{"Choose the text style", "Dark mode", []string{"Enter"}, false},
		{"Select login method", "1. Claude account with subscription", []string{"Enter"}, true},
	},
	"codex": {
		{"Sign in with ChatGPT", "1. Sign in with ChatGPT", []string{"Down"}, false},
		{"Sign in with ChatGPT", "2. Sign in with Device Code", []string{"Enter"}, true},
	},
	sample.CursorAgent: {{"Press any key to log in", "", []string{"Enter"}, true}},
}

// signInMarks are the marks each agent puts, followed by a space, at the
// start of the selected line of a list: Codex 0.163.0 draws an ASCII >
// (› was not seen live; it is kept from the earlier reading), Claude Code ❯.
// Cursor's sign-in screen has no list.
var signInMarks = map[string][]string{"claude": {"❯"}, "codex": {">", "›"}}

// highlighted is the selected line of a list on screen, without its mark
// and without the check mark Claude Code puts on the current choice, or ""
// when no line, or more than one, carries the agent's mark. A frame drawn
// around the list is skipped.
func highlighted(agent, screen string) string {
	var out string
	n := 0
	for line := range strings.SplitSeq(screen, "\n") {
		line = strings.TrimRight(strings.TrimRight(strings.TrimLeft(line, " \t│┃|"), " \t│┃|"), " \t")
		for _, mark := range signInMarks[agent] {
			if rest, ok := strings.CutPrefix(line, mark+" "); ok {
				rest = strings.TrimSpace(rest)
				rest = strings.TrimPrefix(rest, "✔ ")
				out, n = strings.TrimSpace(strings.TrimSuffix(rest, " ✔")), n+1
				break
			}
		}
	}
	if n != 1 {
		return ""
	}
	return out
}

// isOption tells if the highlighted line sel is the option label: the
// whole label, alone or followed by its " · " description. "Dark mode" is
// not "Dark mode (colorblind-friendly)".
func isOption(sel, label string) bool {
	return sel == label || strings.HasPrefix(sel, label+" · ")
}

// signInStep is the screen of agent that screen shows: its index in the
// agent's table, or -1; ready tells that its key may go, its option being
// the highlighted line. The screen is cleaned of its styles first, and the
// later title wins: one still showing an earlier prompt above a later one
// is at the later one. Screens with the same title are one picker: the
// one whose option is highlighted is the step.
func signInStep(agent, screen string) (step int, ready bool) {
	screen = said.Clean(screen)
	steps := signInScreens[agent]
	at := -1
	for i := len(steps) - 1; i >= 0; i-- {
		if strings.Contains(screen, steps[i].title) {
			at = i
			break
		}
	}
	if at < 0 {
		return -1, false
	}
	sel := highlighted(agent, screen)
	first := at
	for i := at; i >= 0 && steps[i].title == steps[at].title; i-- {
		first = i
	}
	for i := first; i <= at; i++ {
		if steps[i].pick == "" || sel != "" && isOption(sel, steps[i].pick) {
			return i, true
		}
	}
	return first, false
}

// driveSignIn waits for the first screens of the agent in pane, a new
// account's, and sends the keys that reach its sign-in link, each screen's
// once. It never reads the link or the code: the builder finishes the
// sign-in. It stops, and notes the pane "by hand", when a screen does not
// come within the wait (counted again from each key it sends), when a key's guard fails, or when the builder takes over.
// When ctx ends first it stops without a note. It never kills the pane.
//
// It acts only on a still screen: two looks a tick apart that agree, the
// quiet moment the console's scan question also waits. It looks once more
// right before a key and sends only if the screen is the same; a key the
// builder types in the few milliseconds between that look and the send
// (the guard check runs there) can still be answered: tmux cannot close
// that gap. A key goes
// only at a ready step (signInStep), after the last one it sent, and only
// to the agent first sampled in the pane (its pid and start guard it).
//
// The builder took over when a still screen differs from the last still
// sign-in screen the driver saw, unless the driver's own key explains it:
// a key went since, and the screen is at a later ready step. Anything else
// (a highlight moved, a later screen with another option, a screen it does
// not know, such as a reflow when a client of another size attaches) stops
// it for good, even if a known screen comes back.
func (s *Service) driveSignIn(ctx context.Context, pane, agent, name string) {
	wait, every := s.signInWait, s.signInEvery
	if wait <= 0 {
		wait = signInWait
	}
	if every <= 0 {
		every = signInEvery
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(every)
	defer tick.Stop()
	steps := signInScreens[agent]
	look := func() (string, bool) {
		screen, ok := s.Capture([]string{pane})[pane]
		return said.Clean(screen), ok
	}
	byHand := func() {
		s.signIns.set(pane, SignInByHand)
		s.noteSignIn()
	}
	var (
		last    string         // the previous look
		seen    string         // the last still sign-in screen
		who     sample.Session // the agent the keys go to, first sampled
		done    = -1           // the last step whose key went
		pending bool           // a key went since seen
	)
	for {
		screen, ok := look()
		still := ok && screen == last
		last = screen
		if still {
			i, ready := signInStep(agent, screen)
			if seen != "" && screen != seen && (!pending || !ready || i <= done) {
				byHand()
				return
			}
			// The screen the key left, still shown while the agent redraws,
			// keeps the key pending.
			if i >= 0 && screen != seen {
				seen, pending = screen, false
			}
			if ready && i > done {
				switch sent, stop := s.pressSignIn(ctx, pane, screen, &who, steps[i].keys, look); {
				case stop:
					byHand()
					return
				case sent && steps[i].final:
					return
				case sent:
					done, pending = i, true
					deadline.Reset(wait) // the next screen gets the whole wait
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			byHand()
			return
		case <-tick.C:
		}
	}
}

// pressSignIn sends ks to the agent in pane when the pane still shows
// screen and holds the agent first sampled there (who, set on the first
// call that finds one). sent tells the keys went; stop tells the driver
// must stop: another agent is in the pane, or a key did not go. With no
// agent sampled yet, or a screen that moved, nothing goes and the driver
// looks again.
func (s *Service) pressSignIn(ctx context.Context, pane, screen string, who *sample.Session, ks []string, look func() (string, bool)) (sent, stop bool) {
	x, ok := s.sessionAt(pane)
	switch {
	case !ok:
		return false, false
	case who.PID == 0:
		*who = x
	case x.PID != who.PID || x.Start != who.Start:
		return false, true
	}
	if again, ok := look(); !ok || again != screen {
		return false, false
	}
	if !s.sendSignIn(ctx, pane, *who, ks) {
		return false, true
	}
	return true, false
}

// sendSignIn presses ks in pane, one send-keys each, each guarded by the
// agent's pid and start, and tells if they all went.
func (s *Service) sendSignIn(ctx context.Context, pane string, who sample.Session, ks []string) bool {
	act := advice.Action{Label: "sign in"}
	g := advice.Guard{PID: who.PID, StartTicks: who.Start}
	for i, k := range ks {
		if i > 0 {
			act.Steps = append(act.Steps, panes.Pause)
			act.Guards = append(act.Guards, g)
		}
		act.Steps = append(act.Steps, panes.Keys(pane, k))
		act.Guards = append(act.Guards, g)
	}
	release := s.holdPane(pane)
	defer release()
	res := s.Actions.Do(ctx, act, "")
	return res.Err == "" && len(res.Skipped) == 0 && len(res.Ran) == len(act.Steps)
}

// noteSignIn puts the notes in the state the consoles get now.
func (s *Service) noteSignIn() {
	s.mu.Lock()
	s.cur.SignIns = s.signIns.all()
	s.mu.Unlock()
	s.broadcast()
}

// signInState keeps the notes on the sign-ins, by pane, or by session name
// when the pane was not found.
type signInState struct {
	mu   sync.Mutex
	note map[string]string
}

func (r *signInState) set(pane, note string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.note == nil {
		r.note = map[string]string{}
	}
	r.note[pane] = note
}

// all are the notes. Nil: none.
func (r *signInState) all() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyLocked()
}

// keep drops the notes of panes and sessions that are gone and returns the
// rest.
func (r *signInState) keep(here map[string]bool) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for pane := range r.note {
		if !here[pane] {
			delete(r.note, pane)
		}
	}
	return r.copyLocked()
}

func (r *signInState) copyLocked() map[string]string {
	if len(r.note) == 0 {
		return nil
	}
	out := make(map[string]string, len(r.note))
	for k, v := range r.note {
		out[k] = v
	}
	return out
}
