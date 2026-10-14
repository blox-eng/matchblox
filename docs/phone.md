# The phone

On a phone, the console is the control pane: you connect to it, not to
tmux. You see who waits, the context of each session and the load. You go
into a pane and back without tmux keys.

## The matchblox app

The matchblox iPhone app is not in the App Store or on TestFlight yet. To
get it now, build it on a Mac with Xcode: see
[ios/README.md](https://github.com/blox-eng/matchblox/blob/main/ios/README.md).
Without a Mac, use [Without the app](#without-the-app).

The app pairs with one scan. It needs no account and no password.

1. On the host, run:

   ```sh
   matchblox pair
   ```

   matchblox shows a QR code. The code works once. It expires after 5
   minutes.
2. In the app, scan the code with the iPhone camera.
3. Type a name for the phone. The name has 1 to 40 letters, digits,
   spaces and `. _ - '`.
4. The app opens the console of the host.

A host can have a name that the phone cannot reach, such as its name on
your home network. Change the address on the pairing screen before the
app connects. Or run `matchblox pair --address <host>`. Add `--port` if
sshd does not listen on port 22.

If the app says "This code was used or expired", run `matchblox pair` on
the host again.

When the phone pairs, `matchblox pair` prints the name and fingerprint of
that phone. If it is not your phone, remove it at once with
`matchblox devices rm <fingerprint>`.

### Use the app

- Each time the app opens, Face ID or the passcode unlocks the key. The key
  stays in the Secure Enclave of the iPhone. Each host has its own key.
- With one host, the app opens straight into its console. With two or
  more, it lists the hosts. Tap a host to open its console. Tap
  `+ Pair a host` to add one.
- If the app says `Can't reach <host>.` and `New address?`, type the
  address that the iPhone can reach, then tap `Try again`. The pairing
  stays.
- If the app says that the host has a new host key, the app does not
  connect. Check that the change is yours, then run `matchblox pair` on the
  host and scan the new code.
- When you quit the console, the app says that the console ended. Tap
  `Open again` to open it. With two or more hosts, tap `Hosts` to go to
  the list.
- A swipe on a host in the list removes the host and its key from the
  iPhone. It does not remove the key on the host. Run
  `matchblox devices rm <name>` on the host for that.

### Remove a phone

To remove a phone, run `matchblox devices` on the host. It lists each phone
and laptop that opens the host. Then run:

```sh
matchblox devices rm <name>
```

You can give the fingerprint instead of the name. See
[Remote hosts](remote.md#see-and-remove-the-devices-of-a-host).

## Without the app

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

Tap an agent, then tap it again: on a one-column phone the peek rises from
the bottom, over the dimmed list, with what the agent said last; held
upright with a small font, the drawer on the right becomes the
conversation and the list stays beside it. Tap an option, or type a line
and press `Enter`; it goes to that agent's pane at once. An agent that
works gets it as a message, which Claude Code queues. After an answer the
peek moves to the next agent that waits. A swipe left or right, or a tap
on `← prev` or `next →`, moves it along the list; `Esc` or a tap outside
the peek closes it. See [the answer
peek](console.md#the-answer-peek).

## The phone layout

A phone held upright is read by its shape, so the keyboard coming and
going does not change the layout. With a small font that makes it 90
columns or more, the list sits on the left and the selected agent's detail
in a drawer on the right, and the dock shows two panels side by side. On its side it
goes by its width. Under 60 columns, the console has one column. Each agent is one line,
grouped by what it needs from you: who waits first. The selected one opens
in place: what it said last, its ship, its plan and its keys. Only
the selected setup step shows its detail.

Under the list, when there is room, the dock shows usage, the machine or
the stoker: `u`, or a tap on the word at its right, goes to the next. While
the stoker runs, its panel shows by itself until you switch; a tap on it
opens its whole card on the Machine tab. With the keyboard up the dock
folds away; the Machine tab has it.

The key line is at the bottom: an SSH app that shrinks the terminal for
its keyboard keeps it above the keys. A tap on a key presses it.

Turn the phone on its side, at 60 columns or more, and the list and the
detail of the selected agent sit side by side.

Every key is on the key bar of an SSH app: letters, digits, arrows, `Enter`
and `Esc`. See [Keys](keys.md).
