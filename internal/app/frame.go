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
func RunFrame(args []string, redial func() (transport.Conn, error)) error {
	fs := flag.NewFlagSet("frame", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "")
	for _, name := range []string{"session", "host", "pane", "dir", "base"} {
		fs.String(name, "", "") // the review's; reviewFrameFromArgv reads them
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch strings.TrimSpace(*kind) {
	case "header", "bar", "review":
	default:
		return fmt.Errorf("frame needs --kind=header, bar or review")
	}
	if strings.TrimSpace(*kind) == "review" {
		// The review sends its paste on this conn: the text goes to the
		// service, never through a tmux option.
		c, _ := transport.Dial(transport.SocketPath(), time.Second)
		f, err := reviewFrameFromArgv(New(Options{Conn: c, Pane: os.Getenv("TMUX_PANE")}), args)
		if err == nil {
			_, err = tea.NewProgram(f).Run()
		}
		if c != nil {
			_ = c.Close()
		}
		return err
	}
	// The frame dials again as the list does, so a service that restarts or
	// that is not up yet shows its words and then the counts.
	c, derr := transport.Dial(transport.SocketPath(), time.Second)
	if c != nil {
		derr = nil
	}
	m := New(Options{Conn: c, DialErr: derr, Redial: redial, Pane: os.Getenv("TMUX_PANE")})
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
	rev  *reviewPane // the review's state, for kind review
	Model
}

type frameTick struct{}

func (m frameModel) Init() tea.Cmd {
	start := []tea.Cmd{m.pollSpace()}
	if m.kind == "review" {
		start = append(start, m.reviewLoad())
	}
	if err := m.opt.DialErr; err != nil && m.conn == nil {
		return tea.Batch(append(start, func() tea.Msg { return lostMsg{err: err} })...)
	}
	return tea.Batch(append(start, m.hello(), m.recv())...)
}

func (m frameModel) pollSpace() tea.Cmd {
	return tea.Tick(time.Second/4, func(time.Time) tea.Msg { return frameTick{} })
}

func (m frameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.kind == "review" && m.rev != nil {
		if cmd, ok := m.reviewTake(msg); ok {
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		next, cmd := m.Model.Update(msg)
		m.Model = next.(Model)
		return m, cmd
	case frameTick:
		switch m.kind {
		case "header":
			if got, ok := m.readSpace(); ok {
				m.space = got
			}
		case "bar":
			if got, ok := m.readMode(); ok {
				m.barMode = got
			}
		case "review":
			return m, tea.Batch(m.reviewTick(), m.pollSpace())
		}
		return m, m.pollSpace()
	case tea.MouseClickMsg:
		return m.tapHeader(msg.Mouse())
	case tea.KeyPressMsg:
		if m.kind == "review" && m.rev != nil {
			return m.reviewKey(msg)
		}
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
	if ms.Button != tea.MouseLeft {
		return m, nil
	}
	switch m.kind {
	case "bar":
		if press := m.barPress(ms.X, m.width); press != "" {
			return m, m.writeCogAct(press)
		}
		return m, nil
	case "header":
	default:
		return m, nil
	}
	chips, hits, _ := m.fitSpaces(m.spaceBudget(m.width))
	if chips != "" {
		x0 := ansi.StringWidth(" ▰ matchblox  ")
		for i := range hits {
			hits[i].x0 += x0
			hits[i].x1 += x0
		}
		if s, ok := spanAt(hits, ms.X); ok && s.press != "" {
			if s.press == spaceMore {
				return m, m.writeSpaceAct(spaceMore)
			}
			return m, m.setSpace(strings.TrimPrefix(s.press, spacePref))
		}
	}
	if m.cogAt(ms.X) {
		return m, m.writeCogAct("open")
	}
	return m, nil
}

// cogAt: x is on the header's ⚙, including the ! it shows in trouble.
func (m frameModel) cogAt(x int) bool {
	chips, _, _ := m.fitSpaces(m.spaceBudget(m.width))
	line := ansi.Strip(m.headerLine(chips))
	i := strings.LastIndex(line, "⚙")
	if i < 0 {
		return false
	}
	x0 := ansi.StringWidth(line[:i])
	word := "⚙"
	if strings.HasPrefix(line[i:], "⚙ !") {
		word = "⚙ !"
	}
	return x >= x0 && x < x0+ansi.StringWidth(word)
}

// headerLine is the header's row. Until the first snapshot, and while the
// connection is lost, it draws no count.
func (m frameModel) headerLine(chips string) string {
	need, done := -1, -1
	if m.have && !m.lost {
		need, done = m.needCount(), m.doneCount()
	}
	return headerBar(m.width, chips, need, done, m.cogMark())
}

func (m frameModel) View() tea.View {
	if m.kind == "header" {
		chips, _, _ := m.fitSpaces(m.spaceBudget(m.width))
		v := tea.NewView(m.headerLine(chips))
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	if m.kind == "bar" {
		v := tea.NewView(m.barLine(m.width))
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	if m.kind == "review" && m.rev != nil {
		return m.reviewView()
	}
	return tea.NewView(m.kind)
}

func (m Model) readMode() (string, bool) {
	run := m.tmuxRun()
	if run == nil {
		return "", false
	}
	out, err := run("show-option", "-wqv", "-t", m.opt.Pane, sample.ModeOption)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
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
