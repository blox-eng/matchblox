package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// RunFrame is `matchblox frame --kind=header|bar|review`: a display pane
// of the desk. Clicks are no-op until later tasks.
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

type frameModel struct{ kind string }

func (frameModel) Init() tea.Cmd { return nil }

func (m frameModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m frameModel) View() tea.View { return tea.NewView(m.kind) }
