package app

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// The match is the element of matchblox (DESIGN.md §4): one cell for each
// session, and the character is the state. ✦ burns while the agent works,
// ╿ rests while it waits for the person, │ is spent when its context is at
// the compact limit. Spent comes first. Only fire and a session that asks
// move; a change plays once: the strike, the flame going out, the burn.
type matchKind int

const (
	matchLit matchKind = iota
	matchUnlit
	matchBurnt
)

const (
	animTick  = 100 * time.Millisecond  // 10 wakes a second, only while something moves
	flameStep = 200 * time.Millisecond  // the flame changes 5 times a second
	breathFor = 2400 * time.Millisecond // one slow breath of a match that asks
	strikeFor = 800 * time.Millisecond  // flash, spark, catch
	outFor    = 400 * time.Millisecond  // the flame dies down
	fadeFor   = 600 * time.Millisecond  // the head burns away
	landGlow  = 1200 * time.Millisecond // a card that changes column glows where it lands
)

// changeKind is a change of state that plays once.
type changeKind int

const (
	changeStrike changeKind = iota
	changeOut
	changeBurn
	changeSpark // news ran down a chain's fuse
)

type change struct {
	kind changeKind
	at   time.Time
}

type animMsg struct{}

func (m Model) marks() markColours {
	if m.dark {
		return darkMark
	}
	return lightMark
}

func (m Model) burnt(s sample.Session) bool {
	return s.Context == "known" && m.full().Reached(s)
}

func (m Model) matchOf(s sample.Session) matchKind {
	switch {
	case m.burnt(s):
		return matchBurnt
	case s.Busy:
		return matchLit
	}
	return matchUnlit
}

// asks tells if a session waits for an answer or a permission.
func (m Model) asks(s sample.Session) bool {
	it, ok := m.waitOf(s)
	return ok && (it.State == queue.StatePermission || it.State == queue.StateQuestion)
}

// matchFor is the match of a session at this moment: its character and colour.
func (m Model) matchFor(s sample.Session) (string, color.Color) {
	k, mk, now := m.matchOf(s), m.marks(), m.now()
	if !m.opt.NoMotion {
		if c, ok := m.changes[paneOf(s)]; ok {
			e := now.Sub(c.at)
			switch {
			case c.kind == changeStrike && k == matchLit && e < 200*time.Millisecond:
				return "╿", mk.Core // the head flashes
			case c.kind == changeStrike && k == matchLit && e < 500*time.Millisecond:
				return "·", mk.Core // a spark catches
			case c.kind == changeStrike && k == matchLit && e < strikeFor:
				return "✧", mk.Amber // it grows
			case c.kind == changeOut && k == matchUnlit && e < outFor/2:
				return "✧", mk.Amber
			case c.kind == changeOut && k == matchUnlit && e < outFor:
				return "✧", mk.Ember
			case c.kind == changeBurn && k == matchBurnt && e < fadeFor:
				return "╿", mk.Ember
			}
		}
	}
	switch k {
	case matchLit:
		if m.opt.NoMotion {
			return "✦", mk.Flame
		}
		step := now.UnixMilli() / flameStep.Milliseconds()
		flames := [8]color.Color{mk.Flame, mk.Amber, mk.Core, mk.Amber, mk.Flame, mk.Flame, mk.Core, mk.Amber}
		ch := "✦"
		if step%6 == 5 {
			ch = "✧"
		}
		return ch, flames[step%8]
	case matchUnlit:
		if !m.opt.NoMotion && m.asks(s) {
			phase := float64(now.UnixMilli()%breathFor.Milliseconds()) / float64(breathFor.Milliseconds())
			level := int(math.Round(3 * (1 - math.Cos(2*math.Pi*phase)) / 2))
			if level > 0 {
				return "╿", mk.Breath[level-1]
			}
		}
		return "╿", mk.Head
	}
	return "│", m.st.theme.Faint
}

func (m Model) matchCell(s sample.Session) string {
	ch, c := m.matchFor(s)
	return lipgloss.NewStyle().Foreground(c).Render(ch) + " "
}

// paneKey is a pane on a host: pane ids repeat across hosts.
type paneKey struct{ host, pane string }

// paneOf is the key of s: its pane; outside tmux its process and the
// process's start ("pid:4242@9113"), so a reused pid is a new key; with no
// process its session id ("sid:…"). A session with none of them has the
// empty key, which no row can choose.
func paneOf(s sample.Session) paneKey {
	switch {
	case s.Pane != "":
		return paneKey{s.Host, s.Pane}
	case s.PID != 0:
		return paneKey{s.Host, "pid:" + strconv.Itoa(s.PID) + "@" + strconv.FormatUint(s.Start, 10)}
	case s.SessionID != "":
		return paneKey{s.Host, "sid:" + s.SessionID}
	}
	return paneKey{s.Host, ""}
}

// tmuxPane: k names a tmux pane (%12), not a process outside tmux nor the
// row of a ship no session drives.
func (k paneKey) tmuxPane() bool { return strings.HasPrefix(k.pane, "%") }

// noteChanges records the changes between a host's last state and its
// next, so each plays once.
func (m *Model) noteChanges(host string, next []sample.Session, chains *state.ChainsView) {
	had := m.have
	if host != m.opt.Host {
		i := m.peerAt(host)
		had = i >= 0 && m.peers[i].have
	}
	if m.opt.NoMotion || !had {
		return
	}
	if m.changes == nil {
		m.changes = map[paneKey]change{}
	}
	now := m.now()
	was := map[string]sample.Session{}
	for _, s := range m.snap.Sessions {
		if s.Host == host {
			was[s.Pane] = s
		}
	}
	for _, s := range next {
		prev, ok := was[s.Pane]
		if !ok {
			continue
		}
		at, old := paneKey{host, s.Pane}, m.matchOf(prev)
		switch k := m.matchOf(s); {
		case k == matchLit && old != matchLit:
			m.changes[at] = change{changeStrike, now}
		case k == matchUnlit && old == matchLit:
			m.changes[at] = change{changeOut, now}
		case k == matchBurnt && old != matchBurnt:
			m.changes[at] = change{kind: changeBurn, at: now}
		}
	}
	m.noteSparks(host, chains, now)
	for at, c := range m.changes {
		if now.Sub(c.at) > landGlow {
			delete(m.changes, at)
		}
	}
}

// moving tells if anything on the screen moves: fire, a session that asks,
// or a change still playing.
func (m Model) moving() bool {
	if m.opt.NoMotion {
		return false
	}
	for _, s := range m.snap.Sessions {
		if k := m.matchOf(s); k == matchLit || (k == matchUnlit && m.asks(s)) {
			return true
		}
	}
	for _, c := range m.changes {
		if m.now().Sub(c.at) < landGlow {
			return true
		}
	}
	return false
}

// animate starts the ticks when something moves and none run yet.
func (m *Model) animate() tea.Cmd {
	if m.animating || !m.moving() {
		return nil
	}
	m.animating = true
	return tea.Tick(animTick, func(time.Time) tea.Msg { return animMsg{} })
}

// sideMatch is the match of s in a list row: struck and red when it needs
// the owner, burning while it works, unlit when held, burnt out when idle.
// A lit match flickers and the head of one that asks breathes, as on every
// match (DESIGN.md §4).
func (m Model) sideMatch(s sample.Session, h sideState) string {
	mk, ch, c := m.marks(), "╿", m.marks().Head
	switch {
	case h == sideWork && m.matchOf(s) == matchLit, h == sideNeed && m.asks(s):
		ch, c = m.matchFor(s)
	case h == sideWork:
		ch, c = "✦", mk.Flame
	case h == sideHeld:
		ch, c = "╽", m.st.theme.Muted
	case h == sideIdle:
		ch, c = "╵", m.st.theme.Faint
	}
	return lipgloss.NewStyle().Foreground(c).Bold(h == sideNeed).Render(ch)
}
