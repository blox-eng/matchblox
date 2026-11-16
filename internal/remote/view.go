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
	if name == "" || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	return filepath.IsAbs(socket) && !strings.ContainsAny(socket, "#;\n")
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
	const wrap = `n=$1; g=$2; sock=$3; shift 3; e=$(mktemp) || exit 1; "$@" 2>"$e"; s=$?; if [ $s = 0 ]; then m="$n ended."; else tmux -S "$sock" kill-session -t "=$g" 2>/dev/null; m="$n cannot be shown: $(tr '\n' ' ' <"$e" | sed 's/ *$//')."; fi; rm -f "$e"; stty sane 2>/dev/null; printf '\n%s ctrl+q: back to the list\n' "$m"; until read _; do sleep 0.2; done`
	argv := []string{"/bin/sh", "-c", wrap, "sh", name, JumpSessionName(j.ID), socket, "env", "-u", "TMUX", "tmux", "-S", socket}
	for _, a := range j.Remote()[1:] {
		if a == ";" {
			a = `\;`
		}
		argv = append(argv, a)
	}
	return argv
}
