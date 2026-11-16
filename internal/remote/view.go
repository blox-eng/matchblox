package remote

import (
	"path/filepath"
	"strings"
	"unicode"
)

// ViewAllowed is what a view may be: a pane of this machine (no host, no
// Dir), a display name without control characters, and the absolute path
// of the console's tmux socket, which tmux must not parse or expand.
func ViewAllowed(j Jump, name, socket string) bool {
	if j.Host != "" || j.Pane == "" || j.Dir != "" || !jumpShapeAllowed(j) {
		return false
	}
	return ViewNameAllowed(name) && filepath.IsAbs(socket) && !strings.ContainsAny(socket, "#;\n")
}

// ViewNameAllowed is a name a view prints: not empty, no control
// characters.
func ViewNameAllowed(name string) bool {
	return name != "" && strings.IndexFunc(name, unicode.IsControl) < 0
}

// viewEnd is the end of a view's script: it says $m and the way back, and
// waits for a key, so the pane stays until the console respawns it.
const viewEnd = `stty sane 2>/dev/null; printf '\n%s ctrl+q: back to the list\n' "$m"; until read _; do sleep 0.2; done`

// tmuxArgs writes a lone ";" of argv as "\;": tmux would read it as the end
// of its own command, and hands "\;" to the pane as ";".
func tmuxArgs(argv []string) []string {
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == ";" {
			a = `\;`
		}
		out = append(out, a)
	}
	return out
}

// PeerViewArgv is the argv a pane runs to show a peer's agent: its ssh,
// then a line that says how it ended, and a wait. ssh's own words stay
// above that line. name is an argument of the script, never part of it.
func PeerViewArgv(name string, ssh []string) []string {
	const wrap = `n=$1; shift; trap 'exit 0' HUP TERM; "$@"; s=$?; if [ $s = 0 ]; then m="$n ended."; else m="$n cannot be shown: ssh exited $s."; fi; ` + viewEnd
	return append([]string{"/bin/sh", "-c", wrap, "sh", name}, tmuxArgs(ssh)...)
}

// ViewArgv is the argv a pane runs to show j.Pane: a nested tmux client of
// the console's server, attached to a private session grouped with the
// agent's. TMUX is unset, or tmux would refuse to nest. When the client
// ends the pane says why and waits for a key, so the message stays until
// the console respawns the pane. name and socket are arguments of the
// script, never part of it. The argv is for a tmux command line (split-window,
// respawn-pane): tmux would read a lone ";" as the end of its own command,
// so the list's ";" are written "\;", which tmux hands to the shell as ";".
func ViewArgv(j Jump, name, socket string) []string {
	j.Outer = true
	// A failed list leaves the session it made (it is not yet one that
	// ends when its client goes), so the wrapper kills it.
	const wrap = `n=$1; g=$2; sock=$3; shift 3; e=$(mktemp) || exit 1; trap 'rm -f "$e"' EXIT; trap 'exit 0' HUP TERM; "$@" 2>"$e"; s=$?; if [ $s = 0 ]; then m="$n ended."; else tmux -S "$sock" kill-session -t "=$g" 2>/dev/null; m="$n cannot be shown: $(tr '\n' ' ' <"$e" | sed 's/ *$//')."; fi; rm -f "$e"; ` + viewEnd
	argv := []string{"/bin/sh", "-c", wrap, "sh", name, JumpSessionName(j.ID), socket, "env", "-u", "TMUX", "tmux", "-S", socket}
	return append(argv, tmuxArgs(j.Remote()[1:])...)
}
