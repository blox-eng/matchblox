package app

import (
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/logos"
)

// graphicsMsg says whether every terminal that shows the console draws
// images, and which ones they are (logos.Viewers).
type graphicsMsg struct {
	can bool
	key string
}

// graphicsEvery is how often the console asks who shows it: a client that
// attaches gets its logos within that.
const graphicsEvery = 2 * time.Second

// askGraphics asks, off the update loop, who shows the console now.
func (m *Model) askGraphics() tea.Cmd {
	g := m.opt.Graphics
	if g == nil || time.Since(m.gfxAsked) < graphicsEvery {
		return nil
	}
	m.gfxAsked = time.Now()
	return func() tea.Msg {
		can, key := g()
		return graphicsMsg{can, key}
	}
}

// graphics draws the logos while every viewer can, and sends them again
// to new viewers or a new background: tmux passes an image only to the
// clients that show the pane when it is sent.
func (m Model) graphics(msg graphicsMsg) (Model, tea.Cmd) {
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
