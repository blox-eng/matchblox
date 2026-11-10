package panes

import (
	"os"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/keycmd"
)

// TestFlipFromAMarkedPaneTakesTheClientToTheConsole: ⌥s in an agent pane
// the service marked moves the client to the console's session, and the
// console reads the flip with the client's tty and the pane it came from.
func TestFlipFromAMarkedPaneTakesTheClientToTheConsole(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux("new-session", "-d", "-s", "work", "sleep", "100")
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, "@matchblox_match", "1")
	tmux(FlipKeysArgv()[1:]...)
	term := attach(t, sock, "work")
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == "work" })
	tty := tmux("list-clients", "-F", "#{client_tty}")
	time.Sleep(100 * time.Millisecond)
	if _, err := term.WriteString("\x1bs"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == Home })
	want := eventLine(uv.UnknownApcEvent(keycmd.FlipAPC(tty + " " + pane)))
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
}

// TestNextFromAMarkedPaneSaysNext: ⌥n hands the console the walk.
func TestNextFromAMarkedPaneSaysNext(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux("new-session", "-d", "-s", "work", "sleep", "100")
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, "@matchblox_match", "1")
	tmux(FlipKeysArgv()[1:]...)
	term := attach(t, sock, "work")
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == "work" })
	tty := tmux("list-clients", "-F", "#{client_tty}")
	time.Sleep(100 * time.Millisecond)
	if _, err := term.WriteString("\x1bn"); err != nil {
		t.Fatal(err)
	}
	want := eventLine(uv.UnknownApcEvent(keycmd.NextAPC(tty + " " + pane)))
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
}

// TestFlipKeyPassesThroughOnAnUnknownPane: a pane with no mark, a shell or
// an editor, reads ⌥s as typed, and the client stays.
func TestFlipKeyPassesThroughOnAnUnknownPane(t *testing.T) {
	client, typed := plainPane(t)
	if got := typedThen(t, client, typed, "\x1bs"); got != "^[sx" {
		t.Errorf("the pane read %q, want ^[s then x", got)
	}
}

// TestFlipAtTheConsoleIsItsKey: at the console ⌥s reaches it as the key.
func TestFlipAtTheConsoleIsItsKey(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux(FlipKeysArgv()[1:]...)
	term := attach(t, sock, Home)
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == Home })
	time.Sleep(100 * time.Millisecond)
	if _, err := term.WriteString("\x1bs"); err != nil {
		t.Fatal(err)
	}
	want := eventLine(uv.KeyPressEvent{Code: 's', Mod: uv.ModAlt})
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
}

// plainPane is a pane with no mark that writes what it reads, with the
// flip keys bound.
func plainPane(t *testing.T) (*os.File, string) {
	t.Helper()
	sock, tmux, _ := consolePane(t)
	typed := t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "sh", "-c", "stty raw -echo; exec cat -v > "+typed)
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux(FlipKeysArgv()[1:]...)
	waitFor(t, func() bool { return tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
	return attach(t, sock, "work"), typed
}
