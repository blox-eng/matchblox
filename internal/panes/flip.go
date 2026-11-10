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

// FlipElsewhere and NextElsewhere are what prefix s and prefix n say on a pane with
// no agent matchblox samples.
const (
	FlipElsewhere = "prefix s flips from an agent's pane to its chain's room"
	NextElsewhere = "prefix n walks a chain's agents from one of their panes"
)

// MatchOption marks an agent pane the service samples (mux.MatchOption).
const MatchOption = "@matchblox_match"

// flipFrom keeps, for the hand-off, the client that pressed and the pane it
// pressed in. A format inside send-keys -H is not expanded, so the binding
// writes them here first, and run-shell -C reads them back at the console.
const flipFrom = "@matchblox_flip"

// handOff is what the flip key does in a marked pane, as a tmux command
// list: note the client's tty and pane, take the client to the console as
// the way back does (goHome), and hand the console the kind, the tty and
// the pane, as one APC.
func handOff(kind string) string {
	note := `run-shell -C "set-option -g ` + flipFrom + ` '#{client_tty} #{pane_id}'"`
	hand := `run-shell -C -t =` + Home + `: "#{W:#{P:#{?` + runsConsole +
		`,send-keys -t #{pane_id} -H ` + hexKeys(keycmd.FlipPrefix(kind)) +
		` ; send-keys -t #{pane_id} -l '#{` + flipFrom + `}' ; send-keys -t #{pane_id} -H 1b 5c ; ,}}}"`
	return note + " ; " + goHome[2] + " ; " + hand
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
		{NextKey, "next", keycmd.NextAPC(""), "n", NextElsewhere, keycmd.NextSlot},
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
