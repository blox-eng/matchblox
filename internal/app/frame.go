package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/transport"
)

// RunFrame is `matchblox frame --kind=header|bar|review`: a display pane
// of the desk. The header hello/recv's the same service as the list, reads
// @matchblox_space, and Views the space chips. It does not own the filter.
// A click on a chip sends the same pickSpace act the list `[` `]` use.
func RunFrame(args []string) error {
	fs := flag.NewFlagSet("frame", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch strings.TrimSpace(*kind) {
	case "header", "bar", "review":
	default:
		return fmt.Errorf("frame needs --kind=header, bar or review")
	}
	c, _ := transport.Dial(transport.SocketPath(), time.Second)
	m := New(Options{Conn: c, Pane: os.Getenv("TMUX_PANE")})
	_, err := tea.NewProgram(newFrame(*kind, m)).Run()
	if c != nil {
		_ = c.Close()
	}
	return err
}

func newFrame(kind string, m Model) frameModel {
	m.frameKind = kind
	m.deskFrame = true
	return frameModel{kind: kind, Model: m}
}

type frameModel struct {
	kind string
	Model
}

type frameTick struct{}

func (m frameModel) Init() tea.Cmd {
	return tea.Batch(m.hello(), m.recv(), m.pollSpace())
}

func (m frameModel) pollSpace() tea.Cmd {
	return tea.Tick(time.Second/4, func(time.Time) tea.Msg { return frameTick{} })
}

func (m frameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		next, cmd := m.Model.Update(msg)
		m.Model = next.(Model)
		return m, cmd
	case frameTick:
		if got, ok := m.readSpace(); ok {
			m.space = got
		}
		return m, m.pollSpace()
	case tea.MouseClickMsg:
		return m.tapHeader(msg.Mouse())
	case tea.KeyPressMsg:
		return m, nil
	}
	next, cmd := m.Model.Update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	nm.frameKind = m.kind
	m.Model = nm
	return m, cmd
}

func (m frameModel) tapHeader(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.kind != "header" || ms.Button != tea.MouseLeft {
		return m, nil
	}
	chips, hits, _ := m.fitSpaces(m.spaceBudget(m.width))
	if chips == "" {
		return m, nil
	}
	x0 := ansi.StringWidth(" ▰ matchblox  ")
	for i := range hits {
		hits[i].x0 += x0
		hits[i].x1 += x0
	}
	s, ok := spanAt(hits, ms.X)
	if !ok || s.press == "" {
		return m, nil
	}
	if s.press == spaceMore {
		return m, m.writeSpaceAct(spaceMore)
	}
	return m, m.setSpace(strings.TrimPrefix(s.press, spacePref))
}

func (m frameModel) View() tea.View {
	if m.kind == "header" {
		chips, _, _ := m.fitSpaces(m.spaceBudget(m.width))
		v := tea.NewView(headerBar(m.width, chips, m.needCount(), m.doneCount()))
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	return tea.NewView(m.kind)
}

func (m Model) readSpace() (string, bool) {
	run := m.tmuxRun()
	if run == nil {
		return "", false
	}
	out, err := run("show-option", "-wqv", "-t", m.opt.Pane, sample.SpaceOption)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func (m Model) writeSpaceAct(act string) tea.Cmd {
	run := m.tmuxRun()
	if run == nil {
		return nil
	}
	console := m.opt.Pane
	return func() tea.Msg {
		writeSpace(run, console, act)
		return nil
	}
}
