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
// expands the options first. On the console's pane it sends C-q to
// matchblox; on any other pane it goes to the console. -Z keeps a zoomed
// window zoomed, so the phone stays one pane.
const flipBeside = `#{?#{==:#{pane_id},#{` + sample.ConsoleOption + `}},` +
	`send-keys -t #{` + sample.ConsoleOption + `} ` + BesideKey + `,` +
	`select-pane -Z -t #{` + sample.ConsoleOption + `}}`

// ShellFlipArgv binds t in the view session's key table. On the shell pane
// it selects the agent. On any other pane it types t. It is not a root
// binding, so an agent pane still receives t. writeDesk also writes AGENT
// on the console pane named by @matchblox_desk. That pane is this machine's
// only: a peer's tmux must not be given it, because pane ids repeat.
func ShellFlipArgv(table string, writeDesk bool) []string {
	act := "run-shell -C 'select-pane -t #{" + sample.ShellAgentOption + "}'"
	if writeDesk {
		act = "run-shell -C 'select-pane -t #{" + sample.ShellAgentOption + "} ; set-option -w -t #{" + sample.ShellDeskOption + "} " + sample.ModeOption + " AGENT'"
	}
	return onHome("bind-key", "-T", table, "t", "if-shell", "-F", "#{"+sample.ShellOption+"}", act, "send-keys -l t")
}

// BesideKeysArgv binds BesideKey on the console's tmux server, in the
// console's session and in windows that have an agent beside the list.
func BesideKeysArgv() []string {
	guard := "#{&&:" + atHome + ",#{" + sample.BesideOption + "}}"
	return onHome("bind-key", "-n", BesideKey, "if-shell", "-F", guard, "run-shell -C '"+flipBeside+"'", "send-keys "+BesideKey)
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
	` ; set-option -wu -t #{window_id} ` + sample.HeaderOption + ` ; set-option -wu -t #{window_id} ` + sample.BarOption +
	` ; set-option -wu -t #{window_id} ` + sample.ModeOption +
	` ; set-option -wu -t #{window_id} ` + sample.ConsoleOption + ` ; ` +
	`#{P:#{?#{||:#{==:#{pane_id},#{` + sample.BesideOption + `}},#{||:#{==:#{pane_id},#{` + sample.HeaderOption + `}},#{||:#{==:#{pane_id},#{` + sample.BarOption + `}},#{m:* #{pane_id} *, #{` + sample.ChainOption + `} }}}},kill-pane -t #{pane_id} ; ,}},}},}}`

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
	return onHome("set-hook", "-g", "pane-exited["+slot+"]", hook,
		";", "set-hook", "-g", "pane-died["+slot+"]", hook)
}
