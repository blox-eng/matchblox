package panes

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Home is the tmux session the console lives in, so a phone that connects
// lands in it and every pane has one way back to it.
const Home = "matchblox"

// HomeOption marks a tmux session as a console's own: each terminal has one.
// Every guard reads it, never the session's name.
const HomeOption = "@matchblox_home"

// atHome is true in a console's own session. The first console's name
// counts too: a console an older matchblox started has no mark.
const atHome = "#{||:#{" + HomeOption + "},#{==:#{session_name}," + Home + "}}"

// HomeMarkArgv marks the session of the console's pane as a console's own.
func HomeMarkArgv(pane string) []string {
	return []string{"tmux", "set-option", "-t", pane, HomeOption, "1"}
}

// HomeArgv is the command that runs the console in its own tmux session,
// name (HomeSession): -A attaches when the session is already there. Inside
// tmux, or without tmux, it is nil and the console runs in place. -u keeps
// the client UTF-8: an ssh session with no locale, such as the phone's,
// would else see every glyph of the console as "_".
func HomeArgv(inTmux, haveTmux bool, name string, self []string) []string {
	if inTmux || !haveTmux || len(self) == 0 {
		return nil
	}
	return append([]string{"tmux", "-u", "new-session", "-A", "-s", name, "-e", HomeEnv + "=1"}, self...)
}

// JoinEnv, set to 1, makes a console join the first console, shown or not:
// the phone shows the desk's console, not one of its own.
const JoinEnv = "MATCHBLOX_JOIN"

// HomeSession is the session a new console runs in, from the terminals each
// session has: the first console's while no terminal shows it, or to join
// it; else matchblox-<n>, the first that no session has, so each terminal
// has a console of its own.
func HomeSession(attached map[string]int, join bool) string {
	if join || attached[Home] == 0 {
		return Home
	}
	for n := 2; ; n++ {
		if name := Home + "-" + strconv.Itoa(n); attached[name] == 0 {
			if _, taken := attached[name]; !taken {
				return name
			}
		}
	}
}

// HomeListArgv lists each session with its terminals, for HomeSession.
func HomeListArgv() []string {
	return []string{"tmux", "list-sessions", "-F", "#{session_name} #{session_attached}"}
}

// ParseHomeList reads HomeListArgv's listing.
func ParseHomeList(listing string) map[string]int {
	attached := map[string]int{}
	for _, line := range strings.Split(listing, "\n") {
		if name, n, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			attached[name], _ = strconv.Atoi(n)
		}
	}
	return attached
}

// OrphanListArgv lists each session with its terminals and the console it
// went into an agent from, for Orphaned.
func OrphanListArgv() []string {
	return []string{"tmux", "list-sessions", "-F", "#{session_name}\t#{session_attached}\t#{" + sample.BackOption + "}"}
}

// Orphaned tells if the console of the session name has no terminal any
// more: no terminal shows it, and none is in an agent it opened. The first
// console is never orphaned: it waits for the next terminal, as before.
func Orphaned(name, listing string) bool {
	if name == Home {
		return false
	}
	own := false
	for _, line := range strings.Split(listing, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) < 2 {
			continue
		}
		f = append(f, "") // a line trimmed of its empty last field
		shown := f[1] != "0"
		if f[0] == name {
			if shown {
				return false
			}
			own = true
		}
		if f[2] == name && shown {
			return false
		}
	}
	return own
}

// SizeArgv makes the window of the console's pane take the size of the
// device used last, so a phone that rotates, or a laptop that attaches,
// resizes the shared console.
func SizeArgv(pane string) []string {
	return []string{"tmux", "set-option", "-w", "-t", pane, "window-size", "latest"}
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

// isConsolePane is runsConsole, or the pane the window names as its console
// (@matchblox_console): an agent beside the list runs a nested tmux, so the
// console is found by that option when the agent pane has the keys.
const isConsolePane = "#{||:" + runsConsole + ",#{==:#{pane_id},#{" + sample.ConsoleOption + "}}}"

// goHome is the way back, as the words of a tmux if-shell: on a console's pane
// the key goes on to that console, whole, so the console reads it itself; a
// pane in copy mode leaves it first, as copy mode would read the key's bytes
// as its own commands. From any other pane it is goTo.
var goHome = [3]string{
	runsConsole,
	`if-shell -F "#{pane_in_mode}" "send-keys -X cancel" ; send-keys -H ` + hexKeys(HomeKey),
	goTo,
}

// goTo takes the client to its own console: the console of the session it
// is in, or of the session it went into an agent from (sample.BackOption); with
// neither, or when that console has ended, to the first console's session.
// It goes on to the console's pane there, as another window may be the one
// on screen. With no such session, it goes to a console run in place in a
// pane of any other session (#354). With no console at all, it says how to
// open one, not tmux's bare "can't find session". The console's name waits
// in the server option goToOption, as a format in a session's loop reads
// that session's options, never the client's.
var goTo = `run-shell -C "set-option -g ` + goToOption + ` ` + consoleOf + `" ; if-shell -F "` + toAlive + `" { run-shell -C "` + jumpTo + `" } { ` +
	`if-shell -F "` + homeExists + `" { switch-client -t =` + Home + " ; run-shell -C -t =" + Home + `: "#{W:#{P:#{?` + isConsolePane +
	`,select-window -t #{window_id} ; select-pane -Z -t #{pane_id} ; ,}}}" } { if-shell -F "` + consoleExists +
	`" { run-shell -C "#{S:#{W:#{P:#{?` + runsConsole + `,switch-client -t #{pane_id} ; ,}}}}" } { display-message "` + NoHome + `" } } }`

// goToOption is the console the way back goes to, as goTo found it.
const goToOption = "@matchblox_to"

// consoleOf is the name of the client's own console's session.
const consoleOf = "#{?" + atHome + ",#{session_name},#{?#{" + sample.BackOption + "},#{" + sample.BackOption + "}," + Home + "}}"

// toAlive is "1" while the console in goToOption has its session.
const toAlive = "#{S:#{?#{&&:#{==:#{session_name},#{" + goToOption + "}}," + atHome + "},1,}}"

// jumpTo switches the client to the session in goToOption and selects its
// console's pane there.
const jumpTo = "#{S:#{?#{==:#{session_name},#{" + goToOption + "}},switch-client -t #{session_id} ; #{W:#{P:#{?" + isConsolePane +
	",select-window -t #{window_id} ; select-pane -Z -t #{pane_id} ; ,}}},}}"

// homeExists is "1" while the tmux server has the console's session.
const homeExists = "#{S:#{?#{==:#{session_name}," + Home + "},1,}}"

// consoleExists is not empty while a pane of any session runs a console.
const consoleExists = "#{S:#{W:#{P:#{?" + runsConsole + ",1,}}}}"

// NoHome is what the way back says when no pane runs a console.
const NoHome = "No matchblox console here: run matchblox in a shell to open one"

// NamedConsole tells if a console started as arg0 is one to tmux: if not, the
// way back's test fails on its own pane.
func NamedConsole(arg0 string) bool { return filepath.Base(arg0) == Home }

// HomeKeyArgv makes HomeKey a key of the tmux server, binds it and prefix m
// to goHome, and a tap on "◂ matchblox" in the status line to goTo. It wins
// over the person's tmux config, an older way back included, until that
// config is sourced again. A client attached already gets it at once.
func HomeKeyArgv() []string {
	ifHome := []string{"if-shell", "-F", goHome[0], goHome[1], goHome[2]}
	argv := []string{"tmux", "set-option", "-s", "user-keys[" + homeUserKey + "]", HomeKey,
		";", "bind-key", "-n", "User" + homeUserKey}
	argv = append(append(argv, ifHome...), ";", "bind-key", "m")
	argv = append(argv, ifHome...)
	return append(argv, ";", "bind-key", "-n", "MouseDown1Status", "if-shell", "-F", statusTap, goTo, "select-window -t =")
}

// CommandKeysArgv makes each command key of the phone a key of the tmux
// server, in user-keys slots from 901 up (keycmd's table). In a pane the
// console marked as an agent of the command's kind (@matchblox_agent) the
// key types the command's text there, with no Return: the builder sends
// it. Anywhere else the key does nothing, so a shell or another agent never
// reads it as typing.
func CommandKeysArgv() []string {
	argv := []string{"tmux"}
	for _, c := range keycmd.All() {
		slot, seq := strconv.Itoa(c.Slot), keycmd.Sequence(c.ID)
		if len(argv) > 1 {
			argv = append(argv, ";")
		}
		argv = append(argv, "set-option", "-s", "user-keys["+slot+"]", seq,
			";", "bind-key", "-n", "User"+slot, "if-shell", "-F", "#{==:#{@matchblox_agent},"+c.Agent+"}", "send-keys -l "+c.Text)
	}
	return argv
}

// atConsole is true for a client in the console's session, on a pane the
// console runs in, or on any pane of a window that names its console
// (@matchblox_console): the agent pane beside the list too.
const atConsole = "#{&&:" + atHome + ",#{||:" + runsConsole + ",#{" + sample.ConsoleOption + "}}}"

// handTarget is the pane the key hands go to: the window's console when it
// names one, else the active pane.
const handTarget = "#{?" + sample.ConsoleOption + ",#{" + sample.ConsoleOption + "},#{pane_id}}"

// MarkKeyArgv makes the app's marker key a key of the tmux server. At the
// console it hands the console the tty of the client that pressed, without
// typing anything into an agent pane; elsewhere it does nothing. run-shell -C
// expands #{client_tty} and #{pane_id} before the three send-keys run.
func MarkKeyArgv() []string {
	slot := strconv.Itoa(keycmd.MarkSlot)
	hand := "run-shell -C 'send-keys -t " + handTarget + " -H " + hexKeys(keycmd.MarkedPrefix) +
		" ; send-keys -t " + handTarget + " -l #{client_tty} ; send-keys -t " + handTarget + " -H 1b 5c'"
	return []string{"tmux", "set-option", "-s", "user-keys[" + slot + "]", keycmd.MarkKey,
		";", "bind-key", "-n", "User" + slot, "if-shell", "-F", atConsole, hand}
}

// BeltKeyArgv makes the app's answer to the board's keys a key of the tmux
// server. At the console it hands the console the tty of the client that
// answered, as MarkKeyArgv does; elsewhere it does nothing.
func BeltKeyArgv() []string {
	slot := strconv.Itoa(keycmd.BeltSlot)
	hand := "run-shell -C 'send-keys -t " + handTarget + " -H " + hexKeys(keycmd.BeltFromPrefix) +
		" ; send-keys -t " + handTarget + " -l #{client_tty} ; send-keys -t " + handTarget + " -H 1b 5c'"
	return []string{"tmux", "set-option", "-s", "user-keys[" + slot + "]", keycmd.BeltKey,
		";", "bind-key", "-n", "User" + slot, "if-shell", "-F", atConsole, hand}
}

// TapKeysArgv makes each act key of the app's belt a key of the tmux
// server, each in its own slot from keycmd.TapSlot. At the console it hands
// the console the key and the tty of the client that tapped, as
// BeltKeyArgv does; elsewhere it does nothing.
func TapKeysArgv() []string {
	argv := []string{"tmux"}
	for i, k := range keycmd.TapKeys {
		slot := strconv.Itoa(keycmd.TapSlot + i)
		hand := "run-shell -C 'send-keys -t " + handTarget + " -H " + hexKeys(keycmd.TappedPrefix+k+";") +
			" ; send-keys -t " + handTarget + " -l #{client_tty} ; send-keys -t " + handTarget + " -H 1b 5c'"
		if len(argv) > 1 {
			argv = append(argv, ";")
		}
		argv = append(argv, "set-option", "-s", "user-keys["+slot+"]", keycmd.TapKey(k),
			";", "bind-key", "-n", "User"+slot, "if-shell", "-F", atConsole, hand)
	}
	return argv
}

// ViewerHookArgv tells the console the moment a terminal attaches to it or
// switches to it, so the console announces the marks to that terminal then:
// tmux passes the announce only to the clients that show the pane when it is
// sent. Without it, the console's own 2 s check finds the new terminal, and a
// key before that is taken as the desk's. The hooks use MarkKey's slot.
func ViewerHookArgv() []string {
	slot := strconv.Itoa(keycmd.MarkSlot)
	hook := `if-shell -F '` + atConsole + `' "run-shell -C 'send-keys -t ` + handTarget + ` -H ` + hexKeys(keycmd.ViewerKey) + `'"`
	return []string{"tmux", "set-hook", "-g", "client-attached[" + slot + "]", hook,
		";", "set-hook", "-g", "client-session-changed[" + slot + "]", hook}
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
// A tap on the label goes as goTo does. The label is added once, also when the config is sourced again.
// The default status-left-length (10) would cut the tap target off, so a
// shorter length grows to 40.
var WayBack = `# matchblox: the way back to the console
bind m if-shell -F '` + goHome[0] + `' '` + goHome[1] + `' '` + goHome[2] + `'
if -F "#{m:*range=user|matchblox*,#{status-left}}" "" "set -ga status-left '#[range=user|matchblox] ◂ matchblox #[norange]'"
if -F "#{<:#{status-left-length},40}" "set -g status-left-length 40"
bind -n MouseDown1Status if -F "` + statusTap + `" '` + goTo + `' "select-window -t ="
`

// statusTap is true for a tap on "◂ matchblox" in the status line.
const statusTap = "#{==:#{mouse_status_range}," + Home + "}"

// Mouse turns the tmux mouse on, so a tap on "◂ matchblox" works from a
// phone. It changes how every pane selects and scrolls, so the way back
// adds it only when the person asks.
const Mouse = `# matchblox: a tap on ◂ matchblox needs the mouse
set -g mouse on
`
