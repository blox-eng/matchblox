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

1. Tap an agent in the queue. The action line says `tap again: <command>`.
2. Tap it again. You are in the agent's pane.
3. To come back, tap `◂ matchblox` at the left of the tmux status line, or
   press `prefix m` (the tmux prefix, then m).

The tap on `◂ matchblox` needs the tmux mouse on. Press `m` in the preview
of "Add the way back" to turn it on with it, or add `set -g mouse on` to your
tmux config. Without it, use `prefix m`.

The console then selects the next agent that waits.

## The phone layout

Under 60 columns, the console has one column. The queue comes first, and
each row has two lines. Only the selected setup step shows its detail.
The action line is at the top, above the keyboard.

Every key is on the key bar of an SSH app: letters, digits, arrows, `Enter`
and `Esc`. See [Keys](keys.md).
