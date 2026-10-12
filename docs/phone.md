# The phone

On a phone, the console is the control pane: you connect to it, not to
tmux. You see who waits, the context of each session and the load. You go
into a pane and back without tmux keys.

## Open the console when you connect

matchblox must be on the host, and the setup step "Add the way back" must
be done.

**Termius**: edit the host, and set its startup snippet to:

```sh
matchblox
```

**Any SSH app or terminal**:

```sh
ssh -t ws-1 matchblox
```

If ssh does not find matchblox, use its full path: `~/.local/bin/matchblox`,
or `~/go/bin/matchblox` after `go install`.

Outside tmux, `matchblox` attaches to the tmux session `matchblox`, or
starts it. A second phone or laptop attaches to the same session.

## Go to an agent and back

1. Tap an agent in Sessions. The key line says `tap again: <command>`.
2. Tap it again. You are in the agent's pane.
3. To come back, tap `◂ matchblox` at the left of the tmux status line, or
   press `prefix m` (the tmux prefix, then m).

The tap on `◂ matchblox` needs the tmux mouse on. Press `m` in the preview
of "Add the way back" to turn it on with it, or add `set -g mouse on` to your
tmux config. Without it, use `prefix m`.

The console then selects the next agent that waits.

## Answer without leaving the console

Tap an agent that waits, then tap it again: the answer peek rises from the
bottom, over the dimmed list, with what the agent said last. Tap an option,
or type a line and press `Enter`; it goes to that agent's pane at once,
and the peek moves to the next agent that waits. `Tab` skips one, `Esc` or a
tap outside the peek closes it. See [the answer
peek](console.md#the-answer-peek).

## The phone layout

Under 60 columns, the console has one column. Each agent is one line,
grouped by what it needs from you: who waits first. The selected one opens
in place: what it said last, its ship, its plan and its keys. Only
the selected setup step shows its detail.

Under the list, when there is room, the dock shows usage or the machine:
`u`, or a tap on `machine ›`, switches. With the keyboard up the dock folds
away; the Machine tab has it.

The key line is at the bottom: an SSH app that shrinks the terminal for
its keyboard keeps it above the keys. A tap on a key presses it.

Open the phone to 60 columns or more and the list and the detail of the
selected agent sit side by side.

Every key is on the key bar of an SSH app: letters, digits, arrows, `Enter`
and `Esc`. See [Keys](keys.md).
