package app

import (
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/logos"
)

// graphicsMsg says whether every terminal that shows the console draws
// images, and which ones they are (logos.Viewers). gen is the console
// that asked: the shell's next console drops an older one's answer.
type graphicsMsg struct {
	gen int
	can bool
	key string
}

// graphicsTickMsg is the console's own clock for asking who shows it, so
// the logos turn off for a viewer that cannot draw even while no state
// arrives.
type graphicsTickMsg struct{ gen int }

// graphicsEvery is how often the console asks who shows it: a client that
// attaches gets its logos, or turns them off, within that.
const graphicsEvery = 2 * time.Second

// graphicsTick starts the clock: its first tick asks at once.
func (m Model) graphicsTick(after time.Duration) tea.Cmd {
	if m.opt.Graphics == nil {
		return nil
	}
	gen := m.opt.Gen
	if after == 0 {
		return func() tea.Msg { return graphicsTickMsg{gen} }
	}
	return tea.Tick(after, func(time.Time) tea.Msg { return graphicsTickMsg{gen} })
}

// askGraphics asks, off the update loop, who shows the console now: one
// question at a time, so a wedged tmux holds one, not one more each tick.
func (m Model) askGraphics(msg graphicsTickMsg) (Model, tea.Cmd) {
	if msg.gen != m.opt.Gen {
		return m, nil
	}
	next := m.graphicsTick(graphicsEvery)
	if m.gfxBusy {
		return m, next
	}
	m.gfxBusy = true
	m.gfxAsks++
	g, gen := m.opt.Graphics, m.opt.Gen
	return m, tea.Batch(next, func() tea.Msg {
		can, key := g()
		return graphicsMsg{gen, can, key}
	})
}

// graphics draws the logos while every viewer can, and sends them again
// to new viewers or a new background: tmux passes an image only to the
// clients that show the pane when it is sent.
func (m Model) graphics(msg graphicsMsg) (Model, tea.Cmd) {
	if msg.gen != m.opt.Gen {
		return m, nil
	}
	m.gfxBusy = false
	if !msg.can {
		m.logos, m.gfxKey = false, ""
		return m, nil
	}
	m.logos = true
	key := msg.key + "/" + strconv.FormatBool(m.dark)
	if key == m.gfxKey {
		return m, nil
	}
	m.gfxKey = key
	return m, tea.Raw(logos.Transmit(m.dark, !m.opt.OutsideTmux))
}

// accountMark leads an account in USAGE: its provider's logo while the
// viewers draw images, else its mark (the name after it tells providers
// apart; the theme keeps hue for events). With logos every mark takes
// their two columns.
func (m Model) accountMark(provider string) string {
	if m.logos {
		if c := logos.Cells(provider); c != "" {
			return c
		}
	}
	mark := providerMarks[provider]
	if mark == "" {
		mark = "·"
	}
	if m.logos {
		mark += " "
	}
	return m.st.muted.Render(mark)
}
