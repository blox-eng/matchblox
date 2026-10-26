package remote

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
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

var (
	idRE     = regexp.MustCompile(`^[0-9a-f]{8}$`)
	homeRE   = regexp.MustCompile(`^[A-Za-z0-9_@\[\]-]+$`)
	prefixRE = regexp.MustCompile(`^(C-|M-|C-M-)?[A-Za-z0-9\]^_@\[]$`)
	// jumpSessionRE is the host's session of a jump, or this machine's
	// session of a jump to a host.
	jumpSessionRE = regexp.MustCompile(`^matchblox-(jump|[A-Za-z0-9_@%\[\]-]+)-[0-9a-f]{8}$`)
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

// JumpSessionName is the host's private session of a jump.
func JumpSessionName(id string) string { return "matchblox-jump-" + id }

// JumpSession is true for the name of a jump's session on either side: the
// sampler skips them.
func JumpSession(name string) bool { return jumpSessionRE.MatchString(name) }

// PrefixKey is a tmux key name that is safe in a tmux command and a format
// string: one character, with the C-, M- or C-M- modifier.
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
	cmds = append(cmds, setOption(g, "destroy-unattached", "on"))
	if j.Outer {
		cmds = append(cmds, setOption(g, "status", "off"))
	} else {
		cmds = append(cmds, setOption(g, "status", "on"), setOption(g, "status-left", "◂ console: "+j.Prefix+" m "))
	}
	cmds = append(cmds,
		setOption(g, "prefix", "None"),
		setOption(g, "key-table", table),
		setOption(g, "mouse", "off"))
	if j.Pane != "" {
		// A pane id names its window; the window is selected in the
		// session that has it. Real tmux decides if that is g (Task 2).
		cmds = append(cmds, []string{"select-window", "-t", j.Pane}, []string{"select-pane", "-t", j.Pane})
	}
	return tmuxList(append(cmds, []string{"attach-session", "-t", "=" + g})...)
}

// Local is the one tmux command list that runs on this machine. It starts
// the ssh in a session of its own, binds the way back to the console, and
// switches the client there. The session ends when ssh ends; a failed ssh
// waits for Enter so the error can be read. ssh stays argv.
func (j Jump) Local(ssh []string) []string {
	s := HostSessionName(j.Host, j.ID)
	back := "switch-client -t =" + j.home()
	const wrap = `ssh "$@"; s=$?; [ $s = 0 ] || { printf "\nssh exited %s. Press Enter to go back.\n" "$s"; read _; }; exit 0`
	return tmuxList(
		append([]string{"new-session", "-d", "-s", s, "--", "/bin/sh", "-c", wrap, "sh"}, ssh...),
		setOption(s, "detach-on-destroy", "off"),
		setOption(s, "key-table", "matchblox-host"),
		setOption(s, "mouse", "on"),
		setOption(s, "status-left", "#[range=user|matchblox] ◂ console #[norange]"),
		[]string{"bind-key", "-T", "matchblox-host", "User900", "switch-client", "-t", "=" + j.home()},
		[]string{"bind-key", "-T", "matchblox-host", "MouseDown1Status", "if", "-F", "#{==:#{mouse_status_range},matchblox}", back, ""},
		[]string{"switch-client", "-t", "=" + s},
	)
}
