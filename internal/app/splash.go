package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// splashFor is the start screen: the match strikes in less than 600 ms.
const splashFor = time.Duration(standEnd) * time.Millisecond

// splashTick is about 15 frames a second, the console's frame rate.
const splashTick = 66 * time.Millisecond

type splashMsg struct{}

// splashing tells if the start screen still plays.
func (m Model) splashing() bool {
	return !m.opt.NoMotion && !m.splashDone && m.now().Sub(m.splashAt) < splashFor
}

// splashCmd asks for the next frame of the start screen. No motion: none.
func (m Model) splashCmd() tea.Cmd {
	if m.opt.NoMotion {
		return nil
	}
	return tea.Tick(splashTick, func(time.Time) tea.Msg { return splashMsg{} })
}

// markLines is the mark at its current frame, centred in w columns.
func (m Model) markLines(w int) []string {
	t := splashFor
	if m.splashing() {
		t = m.now().Sub(m.splashAt)
	}
	pad := strings.Repeat(" ", max((w-markW)/2, 0))
	lines := drawMark(markFrame(t, m.dark))
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return lines
}

// splashView is the start screen alone: the mark in the middle of the
// screen and, under it, why the console waits.
func (m Model) splashView(w int) string {
	var lines []string
	why := ""
	if !m.have {
		why = strings.Repeat(" ", max((w-lipgloss.Width(m.waitingWhy()))/2, 0)) + m.st.faint.Render(m.waitingWhy())
	}
	// The reason, with its fix, must stay on screen: the mark gives way in a
	// short pane.
	if m.height >= markH/2+2 || why == "" {
		lines = m.markLines(w)
	}
	if why != "" {
		lines = append(lines, "", why)
	}
	top := max((m.height-len(lines))/2, 0)
	return strings.Repeat("\n", top) + strings.Join(lines, "\n")
}

func (m Model) waitingWhy() string {
	if m.flash != "" {
		return m.flash
	}
	return "waiting for the service…"
}
