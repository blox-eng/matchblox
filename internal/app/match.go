package app

import (
	"image/color"
	"math"
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

// landed tells if the card of pane k still glows where it landed: for
// landGlow after the match struck or went out.
func (m Model) landed(k paneKey) bool {
	c, ok := m.changes[k]
	if m.opt.NoMotion || !ok || c.kind != changeStrike && c.kind != changeOut {
		return false
	}
	e := m.now().Sub(c.at)
	return e >= 0 && e < landGlow
}

// quietTone is the token of a QUIET head: it cools with age.
func quietTone(s sample.Session, held bool) string {
	switch age := s.Age(); {
	case held || age == "stale":
		return "faint"
	case age == "cold":
		return "match-ember"
	}
	return "match-head"
}

// quietCell is the match of a card in QUIET, where nothing moves: spent
// is faint, fire stands still, and a head at rest cools with age.
func (m Model) quietCell(s sample.Session) string {
	mk := m.marks()
	ch, c := "╿", mk.Head
	switch k := m.matchOf(s); {
	case k == matchBurnt:
		ch, c = "│", m.st.theme.Faint
	case k == matchLit:
		ch, c = "✦", mk.Flame
	default:
		_, held := m.holdOf(s)
		switch quietTone(s, held) {
		case "faint":
			c = m.st.theme.Faint
		case "match-ember":
			c = mk.Ember
		}
	}
	return lipgloss.NewStyle().Foreground(c).Render(ch) + " "
}

// paneKey is a pane on a host: pane ids repeat across hosts.
type paneKey struct{ host, pane string }

func paneOf(s sample.Session) paneKey { return paneKey{s.Host, s.Pane} }

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
	was := map[string]matchKind{}
	for _, s := range m.snap.Sessions {
		if s.Host == host {
			was[s.Pane] = m.matchOf(s)
		}
	}
	for _, s := range next {
		old, ok := was[s.Pane]
		if !ok {
			continue
		}
		at := paneKey{host, s.Pane}
		switch k := m.matchOf(s); {
		case k == matchLit && old != matchLit:
			m.changes[at] = change{changeStrike, now}
		case k == matchUnlit && old == matchLit:
			m.changes[at] = change{changeOut, now}
		case k == matchBurnt && old != matchBurnt:
			m.changes[at] = change{changeBurn, now}
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
