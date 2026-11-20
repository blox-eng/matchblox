package remote

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Jump is one jump into a host's agent pane or a new shell there. Exactly
// one of Pane (a pane id) and Dir (an absolute directory) is set.
type Jump struct {
	Host, Pane, Dir string
	// Prefix is the tmux prefix key of this machine, as tmux names it.
	Prefix string
	// Home is the session of this machine that holds the console, the
	// target of the way back. "" means matchblox.
	Home string
	// ID is 8 lowercase hex digits, random per jump.
	ID string
	// Outer is true when the console runs inside tmux.
	Outer bool
	// ClientTTY is the terminal device of the client that asked, the only
	// client the jump moves. "" means tmux's current client.
	ClientTTY string
	// View is true for a view shown beside the list, not a jump: its session
	// ends with the agent's pane instead of moving to another pane.
	View bool
}

const defaultHome = "matchblox"

// MinLocalTmux is the oldest tmux on this machine: the CI floor. The way
// back by a tap needs 3.4 (user mouse ranges and mouse_status_range, per
// tmux CHANGES); on 3.2 and 3.3 the home key still works.
const MinLocalTmux = "3.2"

// MinHostTmux is the oldest tmux on a host. run-shell -C is the newest
// feature a jump uses: tmux CHANGES lists it for 3.2. Deduced from CHANGES;
// no tmux older than 3.4 was run.
const MinHostTmux = "3.2"

var (
	idRE     = regexp.MustCompile(`^[0-9a-f]{8}$`)
	homeRE   = regexp.MustCompile(`^[A-Za-z0-9_@\[\]-]+$`)
	prefixRE = regexp.MustCompile("^(C-|M-|C-M-)?([A-Za-z0-9\\]^_@\\[`]|Space)$")
)

// NewJumpID is 8 random hex digits.
func NewJumpID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system has no randomness
	}
	return hex.EncodeToString(b)
}

// HostSessionName is this machine's session for a jump to a host. tmux
// renames "." and ":" in a session name, so they become "_" here.
func HostSessionName(host, id string) string {
	return "matchblox-" + strings.NewReplacer(".", "_", ":", "_").Replace(host) + "-" + id
}

// KeyTable is the key table of a jump's session on this machine: its own,
// as its way back names its own session.
func KeyTable(id string) string { return "matchblox-host-" + id }

// JumpSessionName is the host's private session of a jump.
func JumpSessionName(id string) string { return "matchblox-jump-" + id }

// PrefixKey is a tmux key name that is safe in a tmux command and a format
// string: one character or Space, with the C-, M- or C-M- modifier.
func PrefixKey(s string) bool { return prefixRE.MatchString(s) }

// JumpAllowed is what a jump may be: every value in a command that tmux
// parses or format-expands is a name or id of a fixed shape, a pane id, or
// an absolute directory without "#" or ";".
func JumpAllowed(j Jump) bool {
	return ValidHost(j.Host) && jumpShapeAllowed(j)
}

func jumpShapeAllowed(j Jump) bool {
	if !idRE.MatchString(j.ID) || !PrefixKey(j.Prefix) {
		return false
	}
	if j.ClientTTY != "" && !ClientTTY(j.ClientTTY) {
		return false
	}
	if j.Home != "" && !homeRE.MatchString(j.Home) {
		return false
	}
	if (j.Pane == "") == (j.Dir == "") {
		return false
	}
	if j.Pane != "" {
		return PaneID(j.Pane)
	}
	return filepath.IsAbs(j.Dir) && !strings.ContainsAny(j.Dir, "#;")
}

func (j Jump) home() string {
	if j.Home == "" {
		return defaultHome
	}
	return j.Home
}

// tmuxList joins tmux commands into one command list.
func tmuxList(cmds ...[]string) []string {
	argv := []string{"tmux"}
	for i, c := range cmds {
		if i > 0 {
			argv = append(argv, ";")
		}
		argv = append(argv, c...)
	}
	return argv
}

func setOption(session, name, value string) []string {
	return []string{"set-option", "-t", "=" + session + ":", name, value}
}

// A session's key-table hides root, so a jump binds its own mouse keys:
// without them a swipe scrolls nothing and a drag selects nothing (#240).

// relayMouse binds in table tmux's own mouse keys (its defaults in 3.2 to
// 3.5), for a pane whose program is another tmux client (the desk side of a
// host jump): that client gets every event as it comes.
func relayMouse(table string) [][]string {
	const app = "#{||:#{pane_in_mode},#{mouse_any_flag}}"
	return [][]string{
		{"bind-key", "-T", table, "WheelUpPane", "if", "-F", app, "send-keys -M", "copy-mode -e"},
		{"bind-key", "-T", table, "MouseDown1Pane", "if", "-F", "1", "select-pane -t = ; send-keys -M", ""},
		{"bind-key", "-T", table, "MouseDrag1Pane", "if", "-F", app, "send-keys -M", "copy-mode -M"},
		{"bind-key", "-T", table, "MouseDrag1Border", "resize-pane", "-M"},
	}
}

// paneMouse binds in table the mouse keys of an agent's own pane. A press
// goes to no agent: tmux holds it back, and a release with no drag gives the
// agent the whole click (wholeClick). An agent that got the press of a drag
// would never get its release, which tmux's copy mode takes.
//
// A drag, a double-click and a triple-click select in tmux's copy mode in
// every agent, as tmux's own defaults do for a pane that reads no mouse: the
// copy is tmux's, of this pane only, with no padding and soft wraps joined.
func paneMouse(table string) [][]string {
	const (
		app   = "#{||:#{pane_in_mode},#{mouse_any_flag}}"
		inUse = "#{pane_in_mode}"
	)
	sel := func(what string) string {
		return "select-pane -t = ; if -F '" + inUse + "' '' 'copy-mode -H' ; send-keys -X " + what +
			" ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel"
	}
	return [][]string{
		{"bind-key", "-T", table, "WheelUpPane", "if", "-F", app, "send-keys -M", "copy-mode -e"},
		{"bind-key", "-T", table, "MouseDown1Pane", "if", "-F", inUse, "select-pane -t = ; send-keys -M", "select-pane -t ="},
		{"bind-key", "-T", table, "MouseUp1Pane", "if", "-F", "#{&&:#{mouse_any_flag},#{!:" + inUse + "}}", "run-shell '" + wholeClick + "'", ""},
		{"bind-key", "-T", table, "MouseDrag1Pane", "if", "-F", inUse, "send-keys -M", "copy-mode -M"},
		{"bind-key", "-T", table, "DoubleClick1Pane", "if", "-F", "1", sel("select-word"), ""},
		{"bind-key", "-T", table, "TripleClick1Pane", "if", "-F", "1", sel("select-line"), ""},
		{"bind-key", "-T", table, "MouseDrag1Border", "resize-pane", "-M"},
	}
}

// wholeClick is the shell line that gives the pane under the mouse a press
// and a release of button 1 at the mouse's cell, in the pane's own mouse
// encoding, as hex bytes: tmux puts only digits into it.
const wholeClick = "tmux send-keys -t #{pane_id} -H #{?mouse_sgr_flag," + sgrClick + "," + normalClick + "}"

const (
	sgrX   = "$(printf %s #{e|+:#{mouse_x},1} | od -An -tx1)"
	sgrY   = "$(printf %s #{e|+:#{mouse_y},1} | od -An -tx1)"
	normXY = "$(printf %x #{e|+:#{mouse_x},33}) $(printf %x #{e|+:#{mouse_y},33})"
	// ESC [ < 0 ; x ; y M, then the same ending in m.
	sgrClick = "1b 5b 3c 30 3b " + sgrX + " 3b " + sgrY + " 4d 1b 5b 3c 30 3b " + sgrX + " 3b " + sgrY + " 6d"
	// ESC [ M with button 0, then 3 (a release), each plus 32.
	normalClick = "1b 5b 4d 20 " + normXY + " 1b 5b 4d 23 " + normXY
)

// panePage binds in table PgUp to a page up in tmux's copy mode; in it,
// copy mode's own table pages, and paging back to the bottom leaves it, so
// the next Enter goes to the agent. Outside it PgDn goes to the pane.
func panePage(table string) [][]string {
	return [][]string{
		{"bind-key", "-T", table, "PPage", "copy-mode", "-eu"},
		{"bind-key", "-T", table, "NPage", "if", "-F", "#{pane_in_mode}", "send-keys -X page-down", "send-keys NPage"},
	}
}

// Remote is the one tmux command list that runs on the host. It binds the
// way back for this machine's prefix, then opens a private session grouped
// with the pane's (or a new one in Dir) with no prefix of its own, and
// attaches it. Grouped sessions share windows and have each their own
// current window, so no other client of the host moves.
func (j Jump) Remote() []string {
	g := JumpSessionName(j.ID)
	table := "matchblox-jump-" + j.Prefix
	wait := table + "-prefix"
	cmds := [][]string{
		{"bind-key", "-T", table, j.Prefix, "switch-client", "-T", wait},
		{"bind-key", "-T", wait, "d", "detach-client"},
		{"bind-key", "-T", wait, "m", "detach-client"},
	}
	cmds = append(cmds, paneMouse(table)...)
	cmds = append(cmds, panePage(table)...)
	if j.Pane != "" {
		cmds = append(cmds, []string{"new-session", "-d", "-t", j.Pane, "-s", g})
	} else {
		cmds = append(cmds, []string{"new-session", "-d", "-s", g, "-c", j.Dir})
	}
	if j.Outer {
		cmds = append(cmds, setOption(g, "status", "off"))
	} else {
		cmds = append(cmds, setOption(g, "status", "on"), setOption(g, "status-left", "◂ console: "+j.Prefix+" m "))
	}
	cmds = append(cmds,
		setOption(g, sample.JumpMark, "1"),
		setOption(g, "prefix", "None"),
		setOption(g, "prefix2", "None"),
		setOption(g, "key-table", table),
		// The host's tmux takes the mouse, so a swipe scrolls the agent's
		// history there; its table has no binding of the host's own.
		setOption(g, "mouse", "on"))
	if j.Pane != "" {
		// select-window -t %12 picks the session that has the pane, and
		// that can be the agent's own: it would move its clients. The
		// window id is the only target that names a window of g, so
		// run-shell -C reads it from the pane and runs the command (a
		// tmux command, no shell). select-pane moves no window.
		cmds = append(cmds,
			[]string{"run-shell", "-C", "-t", j.Pane, "select-window -t =" + g + ":#{window_id}"},
			[]string{"select-pane", "-t", j.Pane})
	}
	if j.View && j.Pane != "" {
		cmds = append(cmds, setOption(g, sample.ViewOption, j.Pane),
			// The agent's window closed: g would move to another window of
			// the agent's session, maybe the console's own.
			[]string{"set-hook", "-t", "=" + g + ":", "session-window-changed", `run-shell -C -t "=` + g + `:" "` + ViewGone + `"`})
	}
	// destroy-unattached goes last: run-shell lets the server run, and the
	// server ends an unattached session that has the option (seen in the
	// two-server test), before attach-session has a client for it.
	cmds = append(cmds, setOption(g, "destroy-unattached", "on"))
	return tmuxList(append(cmds, []string{"attach-session", "-t", "=" + g})...)
}

// JumpSSH is the builder's own ssh to host that runs j.Remote() there.
// master is the options of a shared connection, or nil. tmux parses the
// whole list before it runs it, but a command can still fail while it
// runs (the agent's pane closed): the session it made then ends, as it is
// not yet one that ends when its client goes.
func JumpSSH(host string, j Jump, master []string) []string {
	return append(termSSH(master), host, onHost(j.hostCommand()...))
}

// hostCommand is j.Remote() that ends the jump's session when the list
// fails.
func (j Jump) hostCommand() []string {
	const cleanup = `g=$1; shift; "$@" || { s=$?; tmux kill-session -t "=$g" 2>/dev/null; exit $s; }`
	return append([]string{"/bin/sh", "-c", cleanup, "sh", JumpSessionName(j.ID)}, j.Remote()...)
}

func (j Jump) switchClient(session string) []string {
	if j.ClientTTY == "" {
		return []string{"switch-client", "-t", "=" + session}
	}
	return []string{"switch-client", "-c", j.ClientTTY, "-t", "=" + session}
}

// Local is the one tmux command list that runs on this machine. It starts
// the ssh in a session of its own, binds the way back to the console, and
// switches the client there. The session ends when ssh ends, or when its
// client leaves it by any way back; a failed ssh waits for Enter so the
// error can be read. ssh stays argv.
func (j Jump) Local(ssh []string) []string {
	s, table := HostSessionName(j.Host, j.ID), KeyTable(j.ID)
	back := "unbind-key -a -T " + table + " ; switch-client -t =" + j.home()
	// A failed ssh waits for Enter, as its client looks at it. An ssh that
	// was killed leaves the terminal raw and non-blocking, so the wait
	// resets it and reads again. A killed ssh can stop in the middle of an
	// escape sequence, and the line would vanish into it: CAN ends a CSI
	// or OSC, and ST (ESC \) ends a DCS, where tmux reads CAN as data. A
	// jump that ends by itself sends its client home: tmux would pick a
	// session of its own. The session also
	// ends when its client leaves it, by any way back (destroy-unattached),
	// and its ssh with it; the wrapper then drops the table (a hangup too).
	const wrap = `t=$1; h=$2; shift 2; trap 'tmux unbind-key -a -T "$t" 2>/dev/null; exit 0' HUP TERM; "$@"; s=$?; [ $s = 0 ] || { stty sane 2>/dev/null; printf "\030\033\\\\\nssh exited %s. Press Enter to go back.\n" "$s"; until read _; do sleep 0.2; done; }; for c in $(tmux list-clients -t "$TMUX_PANE" -F '#{client_tty}' 2>/dev/null); do tmux switch-client -c "$c" -t "=$h" 2>/dev/null; done; tmux unbind-key -a -T "$t" 2>/dev/null; exit 0`
	return tmuxList(slices.Concat([][]string{
		append([]string{"new-session", "-d", "-s", s, "--", "/bin/sh", "-c", wrap, "sh", table, j.home()}, ssh...),
		setOption(s, sample.JumpMark, "1"),
		setOption(s, "detach-on-destroy", "off"),
		setOption(s, "key-table", table),
		setOption(s, "mouse", "on"),
		setOption(s, "status-left", "#[range=user|matchblox] ◂ console #[norange]"),
		{"bind-key", "-T", table, "User900", "if", "-F", "1", back, ""},
		{"bind-key", "-T", table, "MouseDown1Status", "if", "-F", "#{==:#{mouse_status_range},matchblox}", back, ""},
	}, relayMouse(table), [][]string{
		j.switchClient(s),
		// After the switch: an unattached session with it would end at once.
		setOption(s, "destroy-unattached", "on"),
	})...)
}
