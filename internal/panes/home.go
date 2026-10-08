package panes

// Home is the tmux session the console lives in, so a phone that connects
// lands in it and every pane has one way back to it.
const Home = "matchblox"

// HomeArgv is the command that runs the console in its own tmux session:
// -A attaches when the session is already there, so a second connection
// sees the same console. Inside tmux, or without tmux, it is nil and the
// console runs in place.
func HomeArgv(inTmux, haveTmux bool, self []string) []string {
	if inTmux || !haveTmux || len(self) == 0 {
		return nil
	}
	return append([]string{"tmux", "new-session", "-A", "-s", Home, "-e", HomeEnv + "=1"}, self...)
}

// HomeEnv is set in the home session: the console there is the session's
// only command, so on an error it waits for the person to read it.
const HomeEnv = "MATCHBLOX_HOME"

// WayBack is the tmux config of the way back to the console: prefix m, and
// a tap on "◂ matchblox" in the status line. "=matchblox" is the session of
// that exact name, never one that only starts with it. The label is added
// once, also when the config is sourced again. The default
// status-left-length (10) would cut the tap target off, so a shorter length
// grows to 40.
const WayBack = `# matchblox: the way back to the console
bind m switch-client -t =matchblox
if -F "#{m:*range=user|matchblox*,#{status-left}}" "" "set -ga status-left '#[range=user|matchblox] ◂ matchblox #[norange]'"
if -F "#{<:#{status-left-length},40}" "set -g status-left-length 40"
bind -n MouseDown1Status if -F "#{==:#{mouse_status_range},matchblox}" "switch-client -t =matchblox" "select-window -t ="
`
