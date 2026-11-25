package panes

import (
	"strconv"

	"github.com/blox-eng/matchblox/internal/keycmd"
)

// FlipKey goes between a session and its chain's room; NextKey walks the
// chain's matches, then its stoker. No agent uses them (the key check,
// spec § The flip), and every pane that is not an agent's gets them as typed.
const (
	FlipKey = "M-s"
	NextKey = "M-n"
)

// FlipElsewhere is what prefix s says on a pane with no agent matchblox
// samples.
const FlipElsewhere = "prefix s flips from an agent's pane to its chain's room"

// MatchOption marks an agent pane the service samples (mux.MatchOption).
const MatchOption = "@matchblox_match"

// flipFrom keeps, for the hand-off, the client that pressed and the pane it
// pressed in. A format inside send-keys -H is not expanded, so the binding
// writes them here first, and run-shell -C reads them back at the console.
const flipFrom = "@matchblox_flip"

// handOff is what the flip key does in a marked pane, as a tmux command
// list: note the client's tty and pane, take the client to the console as
// the way back does (goHome), and hand the console of the session it is
// in now the kind, the tty and the pane, as one APC.
func handOff(kind string) string {
	return noteFrom + " ; " + goHome[2] + " ; " + handIn("#{==:#{session_name},#{client_session}}", keycmd.FlipPrefix(kind))
}

// jumpOff is what ⌥ and digit n do in a marked pane: it hands the console
// the digit, the tty and the pane as handOff does, but leaves the client
// where it is. The console moves it, or types the key back for a phone, which
// has no ⌥ and sent Esc then the digit. The console is the one goTo finds.
// With no console at all the key goes on to the pane as typed.
func jumpOff(n int) string {
	prefix := keycmd.JumpPrefix(n)
	return noteFrom + ` ; run-shell -C "set-option -g ` + goToOption + ` ` + consoleOf + `" ; if-shell -F "` + toAlive + `" { ` +
		handIn("#{==:#{session_name},#{"+goToOption+"}}", prefix) + ` } { if-shell -F "` + homeExists + `" { ` +
		handIn("#{==:#{session_name},"+Home+"}", prefix) + ` } { if-shell -F "` + consolePaneExists + `" { ` +
		handIn("1", prefix) + ` } { send-keys M-` + strconv.Itoa(n) + ` } } }`
}

// consolePaneExists is not empty while any pane is one handIn sends to.
const consolePaneExists = "#{S:#{W:#{P:#{?" + isConsolePane + ",1,}}}}"

// noteFrom keeps the client's tty and pane in flipFrom.
const noteFrom = `run-shell -C "set-option -g ` + flipFrom + ` '#{client_tty} #{pane_id}'"`

// handIn sends prefix, flipFrom and ESC \ to each console pane of the
// sessions where the format in is true.
func handIn(in, prefix string) string {
	return `run-shell -C "#{S:#{?` + in + `,#{W:#{P:#{?` + isConsolePane +
		`,send-keys -t #{pane_id} -H ` + hexKeys(prefix) +
		` ; send-keys -t #{pane_id} -l '#{` + flipFrom + `}' ; send-keys -t #{pane_id} -H 1b 5c ; ,}}},}}"`
}

// FlipKeysArgv binds ⌥s and ⌥n, and the phone's flip and next buttons
// (user keys), on the console's tmux server. In a pane the service marked
// as an agent's they go to the console (handOff); in any other pane the
// key goes on as typed, so a shell or an editor keeps it, and at the
// console it is the console's own key.
func FlipKeysArgv() []string {
	argv := []string{"tmux"}
	for _, k := range []struct {
		key, kind, apc, prefix, elsewhere string
		slot                              int
	}{
		{FlipKey, "flip", keycmd.FlipAPC(""), "s", FlipElsewhere, keycmd.FlipSlot},
		{NextKey, "next", keycmd.NextAPC(""), "", "", keycmd.NextSlot}, // prefix n stays tmux's next window
	} {
		slot := strconv.Itoa(k.slot)
		bind := []string{"if-shell", "-F", "#{" + MatchOption + "}", handOff(k.kind), "send-keys " + k.key}
		if len(argv) > 1 {
			argv = append(argv, ";")
		}
		argv = append(argv, "set-option", "-s", "user-keys["+slot+"]", k.apc,
			";", "bind-key", "-n", k.key)
		argv = append(argv, bind...)
		argv = append(append(argv, ";", "bind-key", "-n", "User"+slot), bind...)
		if k.prefix == "" {
			continue
		}
		// The prefix key is the fallback for a terminal that does not send
		// Option as Meta. Elsewhere than an agent's pane or the console it
		// says where it works, and types nothing.
		prefix := []string{"if-shell", "-F", "#{" + MatchOption + "}", handOff(k.kind),
			`if-shell -F "` + runsConsole + `" "send-keys ` + k.key + `" "display-message '` + k.elsewhere + `'"`}
		argv = append(append(argv, ";", "bind-key", k.prefix), prefix...)
	}
	// tmux's own prefix s, the session tree, moves to prefix S.
	return append(argv, ";", "bind-key", "S", "choose-tree", "-Zs")
}

// JumpKeysArgv binds ⌥1 … ⌥9, beside FlipKeysArgv: one command would be
// too long for tmux. In a marked pane each goes straight to that session's
// pane (jumpOff); in any other pane it goes on as typed.
func JumpKeysArgv() []string {
	argv := []string{"tmux"}
	for n := 1; n <= 9; n++ {
		if n > 1 {
			argv = append(argv, ";")
		}
		key := "M-" + strconv.Itoa(n)
		argv = append(argv, "bind-key", "-n", key, "if-shell", "-F", "#{"+MatchOption+"}", jumpOff(n), "send-keys "+key)
	}
	return argv
}
