package panes

import "github.com/blox-eng/matchblox/internal/sample"

// BesideKey flips between the list and the agent beside it. Only the
// console's window takes it; anywhere else it types as usual.
const BesideKey = "C-q"

// flipBeside is the flip, as a tmux command list for run-shell -C, which
// expands the options first. On the console's pane it goes back to the last
// pane, or to the agent when there is none; on any other pane it goes to the
// console. -Z keeps a zoomed window zoomed, so the phone stays one pane.
const flipBeside = `#{?#{==:#{pane_id},#{` + sample.ConsoleOption + `}},` +
	`if-shell -F "#{P:#{?pane_last,1,}}" "last-pane -Z" "select-pane -Z -t #{` + sample.BesideOption + `}",` +
	`select-pane -Z -t #{` + sample.ConsoleOption + `}}`

// BesideKeysArgv binds BesideKey on the console's tmux server, in the
// console's session and in windows that have an agent beside the list.
func BesideKeysArgv() []string {
	guard := "#{&&:#{==:#{session_name}," + Home + "},#{" + sample.BesideOption + "}}"
	return []string{"tmux", "bind-key", "-n", BesideKey, "if-shell", "-F", guard, "run-shell -C '" + flipBeside + "'", "send-keys " + BesideKey}
}
