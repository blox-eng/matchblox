package app

import (
	"fmt"
	"strings"

	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// deskMinWidth is the narrowest window that opens the four-pane frame.
const deskMinWidth = 100

const deskFrameNotice = "desk frame on · the agent pane is still yours"

func frameArgv(self, kind string) []string {
	if self == "" {
		self = "matchblox"
	}
	return []string{self, "frame", "--kind=" + kind}
}

// liveOption is the window option name's pane when that pane still exists.
func liveOption(run func(args ...string) (string, error), console, name string) string {
	out, err := run("show-option", "-wqv", "-t", console, name)
	pane := strings.TrimSpace(out)
	if err != nil || !livePane(run, pane) {
		return ""
	}
	return pane
}

func writeMode(run func(args ...string) (string, error), console, mode string) {
	_, _ = run("set-option", "-w", "-t", console, sample.ModeOption, mode)
}

func setDeskOptions(run func(args ...string) (string, error), console, agent, accent, header, bar string) error {
	if _, err := run(remote.BesideArgv(console, agent, accent)[1:]...); err != nil {
		return err
	}
	for _, kv := range [][2]string{{sample.HeaderOption, header}, {sample.BarOption, bar}} {
		if kv[1] == "" {
			continue
		}
		if _, err := run("set-option", "-w", "-t", console, kv[0], kv[1]); err != nil {
			return err
		}
	}
	writeMode(run, console, "LIST")
	return nil
}

func openTwoPane(mx mux.Mux, run func(args ...string) (string, error), console, accent string, listW, windowW int) besideMsg {
	if pane := keptPane(run, console); pane != "" {
		if _, err := run(remote.BesideArgv(console, pane, accent)[1:]...); err != nil {
			return besideMsg{err: err}
		}
		return besideMsg{pane: pane}
	}
	pane, err := splitBeside(mx, run, console, accent, listW, windowW)
	return besideMsg{pane: pane, err: err}
}

func openFrame(mx mux.Mux, run func(args ...string) (string, error), d deskOpen) besideMsg {
	old := keptPane(run, d.console)
	header, bar := liveOption(run, d.console, sample.HeaderOption), liveOption(run, d.console, sample.BarOption)
	notice := ""
	if old != "" && (header == "" || bar == "") {
		_, _ = run("kill-pane", "-t", old)
		old = ""
		if !d.framed {
			notice = deskFrameNotice
		}
	}
	if old != "" && header != "" && bar != "" {
		if err := setDeskOptions(run, d.console, old, d.accent, header, bar); err != nil {
			return besideMsg{err: err}
		}
		return besideMsg{pane: old, framed: true}
	}
	h, err := mx.SplitEdge(d.console, frameArgv(d.self, "header"), "above", 1)
	if err != nil {
		msg := openTwoPane(mx, run, d.console, d.accent, d.listW, d.windowW)
		if msg.err != nil {
			return msg
		}
		msg.err = fmt.Errorf("the desk frame did not open: %w", err)
		return msg
	}
	b, err := mx.SplitEdge(d.console, frameArgv(d.self, "bar"), "below", 1)
	if err != nil {
		_, _ = run("kill-pane", "-t", h)
		msg := openTwoPane(mx, run, d.console, d.accent, d.listW, d.windowW)
		if msg.err != nil {
			return msg
		}
		msg.err = fmt.Errorf("the desk frame did not open: %w", err)
		return msg
	}
	pane, err := splitBeside(mx, run, d.console, d.accent, d.listW, d.windowW)
	if err != nil {
		return besideMsg{pane: pane, err: err}
	}
	if err := setDeskOptions(run, d.console, pane, d.accent, h, b); err != nil {
		return besideMsg{pane: pane, err: err}
	}
	return besideMsg{pane: pane, notice: notice, framed: true}
}

type deskOpen struct {
	console, accent, self string
	listW, windowW        int
	pocket, framed        bool
}

func openDesk(mx mux.Mux, run func(args ...string) (string, error), d deskOpen) besideMsg {
	if d.windowW >= deskMinWidth && !d.pocket {
		return openFrame(mx, run, d)
	}
	return openTwoPane(mx, run, d.console, d.accent, d.listW, d.windowW)
}
