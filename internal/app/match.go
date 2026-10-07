package app

import (
	"image/color"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
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
)

// changeKind is a change of state that plays once.
type changeKind int

const (
	changeStrike changeKind = iota
	changeOut
	changeBurn
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
	return s.Context == "known" && m.compactAt() > 0 && s.ContextPct >= m.compactAt()
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

// asks tells if the session in a pane waits for an answer or a permission.
func (m Model) asks(pane string) bool {
	for _, it := range m.queue {
		if it.Pane == pane {
			return it.State == queue.StatePermission || it.State == queue.StateQuestion
		}
	}
	return false
}

// matchFor is the match of a session at this moment: its character and colour.
func (m Model) matchFor(s sample.Session) (string, color.Color) {
	k, mk, now := m.matchOf(s), m.marks(), m.now()
	if !m.opt.NoMotion {
		if c, ok := m.changes[s.Pane]; ok {
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
		if !m.opt.NoMotion && m.asks(s.Pane) {
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

// queueCell is the match of the session a queue row names.
func (m Model) queueCell(it queue.Item) string {
	for _, s := range m.snap.Sessions {
		if s.Pane == it.Pane {
			return m.matchCell(s)
		}
	}
	return lipgloss.NewStyle().Foreground(m.marks().Head).Render("╿") + " "
}

// queueMatch is unlit (the session waits), or burnt when its context is full.
func (m Model) queueMatch(it queue.Item) matchKind {
	for _, s := range m.snap.Sessions {
		if s.Pane == it.Pane && m.burnt(s) {
			return matchBurnt
		}
	}
	return matchUnlit
}

// noteChanges records the changes between the last state and the next, so
// each plays once.
func (m *Model) noteChanges(next []sample.Session) {
	if m.opt.NoMotion || !m.have {
		return
	}
	if m.changes == nil {
		m.changes = map[string]change{}
	}
	now := m.now()
	was := map[string]matchKind{}
	for _, s := range m.snap.Sessions {
		was[s.Pane] = m.matchOf(s)
	}
	for _, s := range next {
		old, ok := was[s.Pane]
		if !ok {
			continue
		}
		switch k := m.matchOf(s); {
		case k == matchLit && old != matchLit:
			m.changes[s.Pane] = change{changeStrike, now}
		case k == matchUnlit && old == matchLit:
			m.changes[s.Pane] = change{changeOut, now}
		case k == matchBurnt && old != matchBurnt:
			m.changes[s.Pane] = change{changeBurn, now}
		}
	}
	for pane, c := range m.changes {
		if now.Sub(c.at) > time.Second {
			delete(m.changes, pane)
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
		if k := m.matchOf(s); k == matchLit || (k == matchUnlit && m.asks(s.Pane)) {
			return true
		}
	}
	for _, c := range m.changes {
		if m.now().Sub(c.at) < time.Second {
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
