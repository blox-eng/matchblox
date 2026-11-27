package app

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/sample"
)

// RunFrame is `matchblox frame --kind=header|bar|review`: a display pane
// of the desk. The header reads @matchblox_space; it does not own the
// filter. Clicks on a space are no-op until the list writes the option.
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
	_, err := tea.NewProgram(frameModel{kind: *kind}).Run()
	return err
}

type frameModel struct {
	kind, space string
	width       int
}

type frameTick struct{}

func (frameModel) Init() tea.Cmd {
	return tea.Tick(time.Second/4, func(time.Time) tea.Msg { return frameTick{} })
}

func (m frameModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case frameTick:
		m.space = readSpace()
		return m, tea.Tick(time.Second/4, func(time.Time) tea.Msg { return frameTick{} })
	}
	return m, nil
}

func (m frameModel) View() tea.View {
	if m.kind == "header" {
		chips := ""
		if m.space != "" && m.space != spaceAll {
			chips = m.space
		}
		return tea.NewView(headerBar(m.width, chips, 0, 0))
	}
	return tea.NewView(m.kind)
}

func readSpace() string {
	out, err := exec.Command("tmux", "show-option", "-wqv", sample.SpaceOption).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
