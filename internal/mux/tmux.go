package mux

import (
	"os/exec"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Tmux is the tmux backend. Run runs a tmux command and returns what it
// prints (nil: exec); RunIn runs one with stdin (nil: exec); Capture reads
// screens (nil: with their styles, as the stoker's guard reads them).
type Tmux struct {
	Run     func(argv []string) (string, error)
	RunIn   func(argv []string, stdin string) error
	Capture func(panes []string) map[string]string
}

var _ Mux = Tmux{}

func (Tmux) Type(pane, text string) []string { return panes.Literal(pane, text) }

func (Tmux) Keys(pane string, keys ...string) []string { return panes.Keys(pane, keys...) }

func (t Tmux) Load(pane, name, text string) error {
	tmux, _ := sample.Tmux(pane)
	argv := slices.Concat(tmux, []string{"load-buffer", "-b", name, "-"})
	if t.RunIn != nil {
		return t.RunIn(argv, text)
	}
	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv built here, never a shell string
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

func (Tmux) Paste(pane, name string) []string {
	tmux, id := sample.Tmux(pane)
	return slices.Concat(tmux, []string{"paste-buffer", "-p", "-d", "-b", name, "-t", id})
}

func (t Tmux) Drop(pane, name string) error {
	tmux, _ := sample.Tmux(pane)
	_, err := t.run(slices.Concat(tmux, []string{"delete-buffer", "-b", name}))
	return err
}

func (t Tmux) Screens(ps []string) map[string]string {
	if t.Capture == nil {
		return sample.CapturePanesStyled(ps)
	}
	return t.Capture(ps)
}

func (t Tmux) InMode(pane string) (bool, error) {
	tmux, id := sample.Tmux(pane)
	out, err := t.run(slices.Concat(tmux, []string{"display-message", "-p", "-t", id, "#{pane_in_mode}"}))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// MatchOption is the pane option that marks an agent pane the service
// samples.
const MatchOption = "@matchblox_match"

func (t Tmux) Mark(pane string, on bool) error {
	tmux, id := sample.Tmux(pane)
	argv := slices.Concat(tmux, []string{"set-option", "-p", "-t", id, MatchOption, "1"})
	if !on {
		argv = slices.Concat(tmux, []string{"set-option", "-p", "-u", "-t", id, MatchOption})
	}
	_, err := t.run(argv)
	return err
}

func (t Tmux) run(argv []string) (string, error) {
	if t.Run != nil {
		return t.Run(argv)
	}
	out, err := exec.Command(argv[0], argv[1:]...).Output() //nolint:gosec // argv built here, never a shell string
	return string(out), err
}
