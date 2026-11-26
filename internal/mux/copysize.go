package mux

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// runCtx is run, except that a read with no Run ends with ctx: a copy
// watcher dies with its context, and a tmux it waits for dies with it.
func (t Tmux) runCtx(ctx context.Context, argv []string) (string, error) {
	if t.Run != nil {
		return t.Run(argv)
	}
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output() //nolint:gosec // argv built here, never a shell string
	return string(out), err
}

// ListBuffers reads the paste buffers of the server, newest first, as tmux
// lists them. It adds no session or client and never starts a server.
func (t Tmux) ListBuffers(ctx context.Context) ([]Buffer, error) {
	out, err := t.runCtx(ctx, slices.Concat(t.commandArgv(), []string{"list-buffers", "-F", "#{buffer_name}\t#{buffer_created}\t#{buffer_size}"}))
	if err != nil {
		return nil, err
	}
	var bufs []Buffer
	for l := range strings.Lines(out) {
		name, rest, ok := strings.Cut(strings.TrimRight(l, "\r\n"), "\t")
		if !ok {
			continue
		}
		created, size, _ := strings.Cut(rest, "\t")
		bufs = append(bufs, Buffer{Name: name, Created: created, Size: size})
	}
	return bufs, nil
}

// ShowBuffer reads the text of buffer name.
func (t Tmux) ShowBuffer(ctx context.Context, name string) (string, error) {
	return t.runCtx(ctx, slices.Concat(t.commandArgv(), []string{"show-buffer", "-b", name}))
}

// noticeFor is how long a notice stays on a client, in milliseconds.
const noticeFor = "4000"

// Notice shows text on each client that is no control client.
func (t Tmux) Notice(ctx context.Context, text string) {
	out, err := t.runCtx(ctx, slices.Concat(t.commandArgv(), []string{"list-clients", "-F", "#{client_control_mode} #{client_name}"}))
	if err != nil {
		return
	}
	for l := range strings.Lines(out) {
		if name, ok := strings.CutPrefix(strings.TrimRight(l, "\r\n"), "0 "); ok {
			_, _ = t.runCtx(ctx, slices.Concat(t.commandArgv(), []string{"display-message", "-c", name, "-d", noticeFor, text}))
		}
	}
}

// SizedMark is the window option an older service set on a window it sized
// to a chat: the window-size it had before ("-": it set none) and its size
// then, as "latest 120x40". A service that starts puts each one back
// (#351: no window is sized any more).
const SizedMark = "@matchblox-sized"

// maxSize is the most tmux takes for a window, either way.
const maxSize = 10000

// windowSizes are the values of tmux's window-size option.
var windowSizes = []string{"largest", "smallest", "manual", "latest"}

// SizedWindows lists every window of the server with its mark. A window
// with no mark is not in the list.
func (t Tmux) SizedWindows() ([]SizedWindow, error) {
	out, err := t.run(slices.Concat(t.commandArgv(), []string{"list-windows", "-a", "-F", "#{window_id}\t#{" + SizedMark + "}"}))
	if err != nil {
		return nil, err
	}
	var ws []SizedWindow
	for l := range strings.Lines(out) {
		if window, mark, ok := strings.Cut(strings.TrimRight(l, "\r\n"), "\t"); ok && mark != "" {
			ws = append(ws, SizedWindow{window, mark})
		}
	}
	return ws, nil
}

// RestoreWindow puts back the window-size of a mark, and the size of a
// window that was manual, then drops the mark. A window that set none, or
// a value tmux does not know, takes the server's again: tmux sizes it by
// its clients.
func (t Tmux) RestoreWindow(window, mark string) error {
	argv := t.commandArgv()
	prev, size, _ := strings.Cut(mark, " ")
	cols, rows, sized := parseSize(size)
	switch {
	case prev == "manual" && sized:
		argv = append(argv, "set-option", "-w", "-t", window, "window-size", prev, ";",
			"resize-window", "-t", window, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows), ";")
	case slices.Contains(windowSizes, prev):
		argv = append(argv, "set-option", "-w", "-t", window, "window-size", prev, ";")
	default:
		argv = append(argv, "set-option", "-w", "-u", "-t", window, "window-size", ";")
	}
	_, err := t.run(append(argv, "set-option", "-w", "-u", "-t", window, SizedMark))
	return err
}

// parseSize reads "120x40" within tmux's bounds.
func parseSize(s string) (cols, rows int, ok bool) {
	c, r, _ := strings.Cut(s, "x")
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	return cols, rows, err1 == nil && err2 == nil && cols > 0 && rows > 0 && cols <= maxSize && rows <= maxSize
}
