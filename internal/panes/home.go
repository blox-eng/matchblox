package panes

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/keycmd"
)

// Home is the tmux session the console lives in, so a phone that connects
// lands in it and every pane has one way back to it.
const Home = "matchblox"

// HomeArgv is the command that runs the console in its own tmux session:
// -A attaches when the session is already there, so a second connection
// sees the same console. Inside tmux, or without tmux, it is nil and the
// console runs in place. -u keeps the client UTF-8: an ssh session with no
// locale, such as the phone's, would else see every glyph of the console
// as "_".
func HomeArgv(inTmux, haveTmux bool, self []string) []string {
	if inTmux || !haveTmux || len(self) == 0 {
		return nil
	}
	return append([]string{"tmux", "-u", "new-session", "-A", "-s", Home, "-e", HomeEnv + "=1"}, self...)
}

// SizeArgv makes the home window take the size of the device used last, so a
// phone that rotates, or a laptop that attaches, resizes the shared console.
func SizeArgv() []string {
	return []string{"tmux", "set-option", "-w", "-t", "=" + Home + ":", "window-size", "latest"}
}

// HomeKey is what the app sends to go back to the console: an APC string,
// which no keyboard types, so the key takes nothing from the apps in the
// panes and works whatever the prefix is.
const HomeKey = "\x1b_matchblox\x1b\\"

// homeUserKey is the user-keys slot of HomeKey: far from 0, where the
// person's own user keys start.
const homeUserKey = "900"

// runsConsole is true on a tmux pane that runs a console, in any session. A
// console under another name is not one to tmux (NamedConsole).
const runsConsole = "#{==:#{pane_current_command}," + Home + "}"

// goHome is the way back, as the words of a tmux if-shell: on a console's pane
// the key goes on to that console, whole, so its chat gives the keys back; a
// pane in copy mode leaves it first, as copy mode would read the key's bytes
// as its own commands. From any other pane the client goes to the console's
// session, and on to the console's pane there, as another window may be the
// one on screen.
var goHome = [3]string{
	runsConsole,
	`if-shell -F "#{pane_in_mode}" "send-keys -X cancel" ; send-keys -H ` + hexKeys(HomeKey),
	"switch-client -t =" + Home + " ; run-shell -C -t =" + Home + `: "#{W:#{P:#{?` + runsConsole +
		`,select-window -t #{window_id} ; select-pane -t #{pane_id} ; ,}}}"`,
}

// NamedConsole tells if a console started as arg0 is one to tmux: if not, the
// way back's test fails on its own pane.
func NamedConsole(arg0 string) bool { return filepath.Base(arg0) == Home }

// HomeKeyArgv makes HomeKey a key of the tmux server, and binds it and prefix
// m to goHome. It wins over a `bind m` in the person's tmux config until that
// config is sourced again. A client attached already gets it at once.
func HomeKeyArgv() []string {
	ifHome := []string{"if-shell", "-F", goHome[0], goHome[1], goHome[2]}
	argv := []string{"tmux", "set-option", "-s", "user-keys[" + homeUserKey + "]", HomeKey,
		";", "bind-key", "-n", "User" + homeUserKey}
	argv = append(append(argv, ifHome...), ";", "bind-key", "m")
	return append(argv, ifHome...)
}

// CommandKeysArgv makes each command key of the phone a key of the tmux
// server, in user-keys slots from 901 up (keycmd's table). A client in the
// console's session, on a pane the console still runs in, hands the key on
// to the console, whole, so its chat runs the command. Anywhere else the key
// does nothing: a command runs only from the chat, and a shell left where a
// console quit never reads it as typing.
func CommandKeysArgv() []string {
	const atConsole = "#{&&:#{==:#{session_name}," + Home + "},#{==:#{pane_current_command}," + Home + "}}"
	argv := []string{"tmux"}
	for _, c := range keycmd.All() {
		slot, seq := strconv.Itoa(c.Slot), keycmd.Sequence(c.ID)
		if len(argv) > 1 {
			argv = append(argv, ";")
		}
		argv = append(argv, "set-option", "-s", "user-keys["+slot+"]", seq,
			";", "bind-key", "-n", "User"+slot, "if-shell", "-F", atConsole, "send-keys -H "+hexKeys(seq))
	}
	return argv
}

// hexKeys is s as the hex bytes of tmux send-keys -H.
func hexKeys(s string) string {
	hex := make([]string, len(s))
	for i := range len(s) {
		hex[i] = fmt.Sprintf("%02x", s[i])
	}
	return strings.Join(hex, " ")
}

// HomeEnv is set in the home session: the console there is the session's
// only command, so on an error it waits for the person to read it.
const HomeEnv = "MATCHBLOX_HOME"

// WayBack is the tmux config of the way back to the console: prefix m (goHome,
// as HomeKeyArgv binds it), and a tap on "◂ matchblox" in the status line.
// "=matchblox" is the session of that exact name, never one that only starts
// with it. The label is added once, also when the config is sourced again.
// The default status-left-length (10) would cut the tap target off, so a
// shorter length grows to 40.
var WayBack = `# matchblox: the way back to the console
bind m if-shell -F '` + goHome[0] + `' '` + goHome[1] + `' '` + goHome[2] + `'
if -F "#{m:*range=user|matchblox*,#{status-left}}" "" "set -ga status-left '#[range=user|matchblox] ◂ matchblox #[norange]'"
if -F "#{<:#{status-left-length},40}" "set -g status-left-length 40"
bind -n MouseDown1Status if -F "#{==:#{mouse_status_range},matchblox}" "switch-client -t =matchblox" "select-window -t ="
`

// Mouse turns the tmux mouse on, so a tap on "◂ matchblox" works from a
// phone. It changes how every pane selects and scrolls, so the way back
// adds it only when the person asks.
const Mouse = `# matchblox: a tap on ◂ matchblox needs the mouse
set -g mouse on
`
