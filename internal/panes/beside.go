package panes

import (
	"strconv"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/sample"
)

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
	guard := "#{&&:" + atHome + ",#{" + sample.BesideOption + "}}"
	return []string{"tmux", "bind-key", "-n", BesideKey, "if-shell", "-F", guard, "run-shell -C '" + flipBeside + "'", "send-keys " + BesideKey}
}

// endBeside, in a window whose console pane was #{hook_pane}, forgets the
// agent pane and a chain view's panes, and kills them. Only the consoles'
// sessions are read, so a window it shares with a grouped session is ended
// once: a second kill-pane of a pane gone would stop the list. The options
// go first: tmux expands the whole list before it runs it, and the last
// kill-pane ends the window, where a set-option after it would fail. With
// no client, tmux keeps that error and shows it over the next console.
const endBeside = `#{S:#{?` + atHome + `,#{W:#{?#{==:#{` + sample.ConsoleOption + `},#{hook_pane}},` +
	`set-option -wu -t #{window_id} ` + sample.BesideOption + ` ; set-option -wu -t #{window_id} ` + sample.ChainOption +
	` ; set-option -wu -t #{window_id} ` + sample.ConsoleOption + ` ; ` +
	`#{P:#{?#{||:#{==:#{pane_id},#{` + sample.BesideOption + `}},#{m:* #{pane_id} *, #{` + sample.ChainOption + `} }},kill-pane -t #{pane_id} ; ,}},}},}}`

// BesideHookArgv ends the agent pane with the console's, however the console
// ended (a crash too), so the next matchblox attaches to no agent without a
// list. tmux runs only global hooks for a pane that is gone (3.4: the pane
// and its window are no target any more), so the hooks are global and find
// the window by its console. A respawn of the console pane (-k) and the
// console's own restart, which runs the binary in its place, end no pane:
// the agent pane stays for the console to adopt. pane-died is the same end
// under remain-on-exit. The hooks use MarkKey's slot.
func BesideHookArgv() []string {
	slot := strconv.Itoa(keycmd.MarkSlot)
	hook := `run-shell -C "` + endBeside + `"`
	return []string{"tmux", "set-hook", "-g", "pane-exited[" + slot + "]", hook,
		";", "set-hook", "-g", "pane-died[" + slot + "]", hook}
}
