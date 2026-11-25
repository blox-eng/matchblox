package panes

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
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

// TestFlipReachesAConsoleRunInPlace: with the console run in place in a
// session of another name (#354), ⌥s takes the client to it and hands it
// the flip.
func TestFlipReachesAConsoleRunInPlace(t *testing.T) {
	sock, tmux, got := consolePaneIn(t, "desk")
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
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == "desk" })
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

// TestJumpFromAMarkedPaneSaysJump: ⌥4 hands the console the digit, the
// client's tty and the pane, and leaves the client where it is: the
// console moves it, or types the key back for a phone.
func TestJumpFromAMarkedPaneSaysJump(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux("new-session", "-d", "-s", "work", "sleep", "100")
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, "@matchblox_match", "1")
	tmux(FlipKeysArgv()[1:]...)
	tmux(JumpKeysArgv()[1:]...)
	term := attach(t, sock, "work")
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == "work" })
	tty := tmux("list-clients", "-F", "#{client_tty}")
	time.Sleep(100 * time.Millisecond)
	if _, err := term.WriteString("\x1b4"); err != nil {
		t.Fatal(err)
	}
	want := eventLine(uv.UnknownApcEvent(keycmd.JumpPrefix(4) + tty + " " + pane + "\x1b\\"))
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
	if at := tmux("list-clients", "-F", "#{client_session}"); at != "work" {
		t.Errorf("the client went to %q before the console answered", at)
	}
}

// TestFlipKeyPassesThroughOnAnUnknownPane: a pane with no mark, a shell or
// an editor, reads ⌥s as typed, and the client stays.
func TestFlipKeyPassesThroughOnAnUnknownPane(t *testing.T) {
	client, typed := plainPane(t)
	if got := typedThen(t, client, typed, "\x1bs"); got != "^[sx" {
		t.Errorf("the pane read %q, want ^[s then x", got)
	}
}

// TestJumpKeyPassesThroughOnAnUnknownPane: ⌥4 in a pane with no mark
// types as it did before the jump keys.
func TestJumpKeyPassesThroughOnAnUnknownPane(t *testing.T) {
	client, typed := plainPane(t)
	if got := typedThen(t, client, typed, "\x1b4"); got != "^[4x" {
		t.Errorf("the pane read %q, want ^[4 then x", got)
	}
}

// TestJumpKeyPassesThroughWithNoConsole: with no console running, a quit
// or one starting again, ⌥4 in a marked pane reaches the agent as typed,
// as the phone's Esc and 4 do: nothing else would read it.
func TestJumpKeyPassesThroughWithNoConsole(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	typed := t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "sh", "-c", "stty raw -echo; exec cat -v > "+typed)
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, MatchOption, "1")
	tmux(FlipKeysArgv()[1:]...)
	tmux(JumpKeysArgv()[1:]...)
	waitFor(t, func() bool { return tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
	client := attach(t, sock, "work")
	if got := typedThen(t, client, typed, "\x1b4"); got != "^[4x" {
		t.Errorf("the pane read %q, want ^[4 then x", got)
	}
}

// TestJumpKeyPassesThroughWhenHomeHasNoConsole: the console quit, and its
// session lives on in another window. ⌥4 in a marked pane still reaches the
// agent as typed: no console there would read it.
func TestJumpKeyPassesThroughWhenHomeHasNoConsole(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", Home, "sleep", "100")
	tmux("set-option", "-t", Home, HomeOption, "1")
	typed := t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "sh", "-c", "stty raw -echo; exec cat -v > "+typed)
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, MatchOption, "1")
	tmux(FlipKeysArgv()[1:]...)
	tmux(JumpKeysArgv()[1:]...)
	waitFor(t, func() bool { return tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
	client := attach(t, sock, "work")
	if got := typedThen(t, client, typed, "\x1b4"); got != "^[4x" {
		t.Errorf("the pane read %q, want ^[4 then x", got)
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
// flip and jump keys bound.
func plainPane(t *testing.T) (*os.File, string) {
	t.Helper()
	sock, tmux, _ := consolePane(t)
	typed := t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "sh", "-c", "stty raw -echo; exec cat -v > "+typed)
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux(FlipKeysArgv()[1:]...)
	tmux(JumpKeysArgv()[1:]...)
	waitFor(t, func() bool { return tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
	return attach(t, sock, "work"), typed
}

// TestPrefixSFlipsFromAMarkedPane: prefix s is ⌥s for a terminal that does
// not send Option as Meta.
func TestPrefixSFlipsFromAMarkedPane(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux("new-session", "-d", "-s", "work", "sleep", "100")
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, "@matchblox_match", "1")
	tmux(FlipKeysArgv()[1:]...)
	term := attach(t, sock, "work")
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == "work" })
	tty := tmux("list-clients", "-F", "#{client_tty}")
	time.Sleep(100 * time.Millisecond)
	for _, k := range []string{"\x02", "s"} {
		if _, err := term.WriteString(k); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	want := eventLine(uv.UnknownApcEvent(keycmd.FlipAPC(tty + " " + pane)))
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
}

// TestPrefixSAtTheConsoleIsItsKey: prefix s at the console is ⌥s there.
func TestPrefixSAtTheConsoleIsItsKey(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux(FlipKeysArgv()[1:]...)
	term := attach(t, sock, Home)
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == Home })
	time.Sleep(100 * time.Millisecond)
	for _, k := range []string{"\x02", "s"} {
		if _, err := term.WriteString(k); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	want := eventLine(uv.KeyPressEvent{Code: 's', Mod: uv.ModAlt})
	waitFor(t, func() bool { return strings.Contains(strings.Join(paneRead(t, got), "\n"), want) })
}
