package remote

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"regexp"
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
	if !ValidHost(j.Host) || !idRE.MatchString(j.ID) || !PrefixKey(j.Prefix) {
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
		setOption(g, "mouse", "off"))
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
	// resets it and reads again. A jump that ends by itself sends its
	// client home: tmux would pick a session of its own. The session also
	// ends when its client leaves it, by any way back (destroy-unattached),
	// and its ssh with it; the wrapper then drops the table (a hangup too).
	const wrap = `t=$1; h=$2; shift 2; trap 'tmux unbind-key -a -T "$t" 2>/dev/null; exit 0' HUP TERM; "$@"; s=$?; [ $s = 0 ] || { stty sane 2>/dev/null; printf "\nssh exited %s. Press Enter to go back.\n" "$s"; until read _; do sleep 0.2; done; }; for c in $(tmux list-clients -t "$TMUX_PANE" -F '#{client_tty}' 2>/dev/null); do tmux switch-client -c "$c" -t "=$h" 2>/dev/null; done; tmux unbind-key -a -T "$t" 2>/dev/null; exit 0`
	return tmuxList(
		append([]string{"new-session", "-d", "-s", s, "--", "/bin/sh", "-c", wrap, "sh", table, j.home()}, ssh...),
		setOption(s, sample.JumpMark, "1"),
		setOption(s, "detach-on-destroy", "off"),
		setOption(s, "key-table", table),
		setOption(s, "mouse", "on"),
		setOption(s, "status-left", "#[range=user|matchblox] ◂ console #[norange]"),
		[]string{"bind-key", "-T", table, "User900", "if", "-F", "1", back, ""},
		[]string{"bind-key", "-T", table, "MouseDown1Status", "if", "-F", "#{==:#{mouse_status_range},matchblox}", back, ""},
		[]string{"switch-client", "-t", "=" + s},
		// After the switch: an unattached session with it would end at once.
		setOption(s, "destroy-unattached", "on"),
	)
}
