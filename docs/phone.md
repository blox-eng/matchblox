# The phone

On a phone, the console is the control pane: you connect to it, not to
tmux. You see who waits, the context of each session and the load. You go
into a pane and back without tmux keys.

## The matchblox app

**Matchblox Pocket** — the matchblox app for iPhone and iPad — is in beta on
TestFlight: [join First Strikers](https://testflight.apple.com/join/t2rtbjHz).
Install the TestFlight app, open the link, and it installs. You can also build
it yourself on a Mac with Xcode: see
[ios/README.md](https://github.com/blox-eng/matchblox/blob/main/ios/README.md).
Without the app, use [Without the app](#without-the-app).

The app pairs with one scan. It needs no account and no password. Build
the app again with each release that changes the pair link: an older app
cannot read the new code.

1. On the host, open the console and choose the door "Pair a device", or
   `+ pair a device` under `DEVICES` on the Machine tab. Or run:

   ```sh
   matchblox pair
   ```

   matchblox shows a box with a QR code on its label. The code works once.
   It expires after 5 minutes. `c` copies the link. `esc` stops, and the
   code no longer works.
2. In the app, point the camera at the code. The scan is the consent: there
   is no form and no Pair button. The box on the phone catches the code and
   opens, at the same time as the box on the host. The app names the device
   after the phone: `iPhone` or `iPad`. A second one is `iPad 2`.
3. The console of the host slides in. On the host, the page says that the
   device is in, then what it did first.

If the camera cannot read the code, copy the link with `c` on the host, and
tap `Paste` in the app. The Camera app also opens the link.

A host can have a name that the phone cannot reach, such as its name on
your home network. Run `matchblox pair --address <host>`. The console pairs
on port 22. If sshd listens on another port, run `matchblox pair --port <n>`.
If the app says `Can't reach <host>.`,
type the address that the device can reach, then tap `Try again`.

The host stops before it shows a code if sshd cannot use the one-time key.
It says so, with the fix, when Tailscale SSH answers on the address that
the code names, or when it can read that sshd uses a different
`AuthorizedKeysFile` than `~/.ssh/authorized_keys` (`sshd -T`, or the global
line in `/etc/ssh/sshd_config`). Turn Tailscale SSH off, or run
`matchblox pair --address <host>` with an address that OpenSSH serves. Or
set `AuthorizedKeysFile` to `%h/.ssh/authorized_keys`, reload sshd, then
pair again. In the console, Enter only helps after `tailscale set
--ssh=false`; for `--address`, quit and run `matchblox pair` in a terminal.
If Tailscale SSH still answers, the app says that the host refused this
connection (with Try again for another address), not that it did not answer.

Pairing is not the whole story. With Tailscale SSH still on
(`tailscale debug prefs` shows `"RunSSH": true`), a connection to the
host's MagicDNS name or Tailscale IP on port 22 is answered by
tailscaled, not OpenSSH. Permanent laptop and phone keys in
`~/.ssh/authorized_keys` — including their `restrict` option and
ForcedCommand (`matchblox gate …`) — are ignored on that path. Anyone
Tailscale SSH admits gets what that policy allows, not the gate. Turn it
off with
`tailscale set --ssh=false`, or connect on an address and port that
OpenSSH serves (`matchblox pair --address` / `--port`, or the same in the
app). A Tailscale ACL can limit what Tailscale SSH allows; it does not
run ForcedCommand. `matchblox serve`, `matchblox status`, and
`matchblox setup` warn when Tailscale SSH is on.

If the app says "This code was used or expired", open Pair a device on the
host again, or run `matchblox pair` again, and scan the new code.

When a device pairs, the host shows its name and fingerprint. If it is not
yours, remove it at once: press `x` on it in `DEVICES`, or run
`matchblox devices rm <fingerprint>`.

A device sees `DEVICES` in its console but cannot pair or remove: it says
`Pair from the host's own terminal.` or `Remove devices from the host's own
terminal.`

### Use the app

- Each time the app opens, Face ID or the passcode unlocks the key. The key
  stays in the Secure Enclave of the device. Each host has its own key.
- With one host, the app opens straight into its console. With two or
  more, it lists the hosts. Tap a host to open its console. Tap
  `+ Pair a host` to add one.
- If the app says `Can't reach <host>.` and `New address?`, type the
  address that the device can reach, then tap `Try again`. The pairing
  stays.
- If the app says that the host has a new host key, the app does not
  connect. Check that the change is yours, then run `matchblox pair` on the
  host and scan the new code. A host key that does not match at pairing
  gets the same message, and the app shows the Address field: the address
  reaches another machine.
- When you quit the console, the app says that the console ended. Tap
  `Open again` to open it. With two or more hosts, tap `Hosts` to go to
  the list.
- A swipe on a host in the list removes the host and its key from the
  device. It does not remove the key on the host. Run
  `matchblox devices rm <name>` on the host for that.

### Remove a device

To remove a device, press `x` on it in `DEVICES` on the Machine tab, and
type `y`. Or run `matchblox devices` on the host. It lists each phone and
laptop that opens the host. Then run:

```sh
matchblox devices rm <name>
```

You can give the fingerprint instead of the name. See
[Remote hosts](remote.md#see-and-remove-the-devices-of-a-host).

## The buzz and the island

With the app closed, the phone buzzes when an agent waits, and a Live
Activity (the island) shows the session that matters most, or your usage.
It needs iOS 18, and the host must run `matchblox serve`: the door "Keep
matchblox running" in `matchblox setup` keeps it running with no console
open. The door is done once the unit runs; while a console's service holds
the socket, the unit's one waits and takes over when it stops. The app
registers with the host each time it opens, after Face ID or the passcode.
Nothing connects to a host from the background.

- **The buzz:** a new wait, when a session asks for a permission or a
  choice. At most one buzz each 30 s for each host. A tap on it opens the
  session.
- **The island:** the match of the session that matters most, and its state.
  A new wait, done or out goes at once. Progress moves in steps of 5 %, at
  most each 3 minutes. The Usage face moves when a window changes by 2 % or
  more, or resets, at most each 5 minutes. A wait breaks through the Usage
  face for 60 s.
- **A tap opens the session:** the app asks for Face ID, connects and opens
  the console on that session: on its chat. If the session has ended, the
  console says `That session ended.`
- **The setting:** the `Island` entry in the app's console, next to `Hosts`.
  `Follow` is the one host that has the island (the host opened last). `Show`
  is `Sessions`, `Usage` or `Auto (Usage while the stoker runs)`. `Buzz` is
  on or off for each host. A host out of reach keeps its old setting until
  the next connect.
- **If a push fails,** the Machine tab says `push: <why>`. If the island
  needs the app, it says `island: open matchblox on the phone to renew`.
- **`[push] off = true`** in the config stops the sender: no buzz, no island
  update, and the host never connects to the relay. A phone can still
  register: the host keeps its registration and sends nothing.
- **`matchblox devices`** shows `buzz on` or `buzz off`, and `island yes` or
  `island no` (yes when the phone registered its island with this host),
  next to each phone. A phone that registered for the buzz shows
  `buzz off ([push] off)` or `buzz off (no host key)` when the host sends
  nothing. The push line of the running service, if any, follows the list.

### What leaves the host in clear

The host sends to the relay, and the relay to Apple. An update to a Live
Activity cannot be encrypted, so it holds only:

- numbers: counts, session and account numbers, and percents;
- states and progress;
- times: when the newest wait began (`wait_at`), when a window resets, and
  when an account runs out (`out Fri ~20:00`); and the stale date that Apple
  keeps with an update, which is the end of the 60 s break-through;
- whether the stoker runs;
- window short names: `5h`, `wk`, `mo`;
- `host`: 8 characters of the fingerprint of the host key.

No session name, account name, provider, path or text is in an update.

**The buzz has one sealed blob.** It is a line of the host's label, the
session's name and a state word, with the last line of what the agent said,
cut to 120 characters. The host seals it to the notification key of that
phone (HPKE), and only the app's Notification Service Extension can open it.
The relay and Apple see only ciphertext, the session number and `host`. The
alert that Apple shows before the extension runs says only `matchblox`, with
a sound; the start of an island has the title and the body `matchblox`.

The phone shows the opened line when it is unlocked, and on the lock screen
too when Show Previews is Always. With the default, When Unlocked, a locked
phone shows `An agent waits` instead. To keep the line off the lock screen,
set Settings > Notifications > matchblox > Show Previews to When Unlocked or
Never.

The app turns a number into a name on the phone, from the names the host
sends back when the app registers, over ssh (`session 12` and `account 3`
where it knows none). The relay stores
nothing and logs no payload. A state is under 2 KB.

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

## Read and steer an agent

The chat is the agent's live screen, inside the console. You do not leave
the console to answer an agent. See [the chat](console.md#the-chat).

1. Tap an agent in Sessions. On a one-column phone, its chat fills the
   screen. The title row reads `‹ Sessions · <name> · <state>`. Held wide,
   the chat is the pane beside the list: tap the agent again, or tap the
   chat, to give it the keys.
2. Type. Every key goes to the agent. When the agent asks with its
   question tool, a card shows at the foot. Tap an option, or type and
   press `Enter`.
3. To go back to Sessions, tap `‹ Sessions` or `◂` on the key bar. You can
   also swipe in from the left edge, or press ⌘M on a hardware keyboard.
   `Ctrl+]` does it too.

The app's key bar has nine keys: `◂ ↑ ↓ ⏎ Tab ⇧Tab Esc ^C ⌨`. Each key
works at the first tap. Each key is at least 44 pt tall. `↑` and `↓` are
wider than the other keys and 52 pt tall. The system keyboard types the
rest. The terminal turns off autocorrect, suggestions, smart quotes, smart
dashes and auto capitalisation. So they never change a key you send.

- `◂` goes back. In the chat it goes to Sessions. In a tmux pane it goes
  to the console.
- `^C` interrupts. `^C` twice stops Claude Code. A second `^C` within 1 s
  is held, and the key line says so.
- `⌨` shows or hides the system keyboard. While the keyboard is up, `⌨` is
  in the accent colour. While the keyboard is down, the key bar stays at
  the foot of the screen.

A vertical swipe scrolls the console and a tmux pane. The lines move with
your finger: one line for each line of travel. In a tmux pane each wheel
step scrolls 5 lines, so the app sends one step for each 5 lines of travel.
Swipe down for older lines, and up for newer lines. A flick carries on for
at most 0.5 s, and scrolls the same lines in the console and in a pane.
To select text, touch and hold, then tap Select and drag. Where the
program does not read the mouse, the terminal scrolls its own history.

## Go to an agent's pane and back

`Ctrl+O` in the chat goes to the agent's pane in tmux. A phone has no
`Ctrl+O`: a tap on a row of Panes does it. Tap the row, and the
key line says `tap again: <command>`. Tap it again. You are in the pane.

To come back in the matchblox app, tap `◂` on the key bar, swipe in from
the left edge, tap `◂ matchblox` at the left of the tmux status line, or
press ⌘M on a hardware keyboard. All four work whatever your tmux prefix
is, and none needs the tmux mouse.

In another SSH app, tap `◂ matchblox`, or press `prefix m` (the tmux prefix,
then m). There the tap needs the tmux mouse on: press `m` in the preview of
"Add the way back" to turn it on with it, or add `set -g mouse on` to your
tmux config. Without it, use `prefix m`.

The console then selects the next agent that waits.

## Switch an agent from the phone

The selected agent's keys are under it: on a one-column phone at the foot
of its opened row, held upright with a small font at the foot of the
drawer. Tap `A switch` to open Switch agent. Tap the agent you want, then
`Enter`; the plan rises in a box above the key line. A typed `y` runs it (a
tap is never consent). The new agent starts in the same pane with the same name, on the account you last
started that agent on, and the console jumps to it.
See [`A` in Keys](keys.md).

## The phone layout

A phone held upright is read by its shape, so the keyboard coming and
going does not change the layout. With a small font that makes it 90
columns or more, the list sits on the left and the selected agent's detail
in a drawer on the right, and the dock shows two panels side by side. On
its side it goes by its width. Under 60 columns, the console has one
column. Each agent is one line, grouped by what it needs from you: who waits first. The selected one opens
in place: what it said last, its ships, its plan and its keys. Only
the selected setup step shows its detail.

Under the list, when there is room, the dock shows usage, the machine or
the stoker (a wide desk puts them in the left column instead). `u`, or a
tap on the word at its right, goes to the next. While
the stoker runs, its panel shows by itself until you switch; a tap on it
opens the Stoker tab. With the keyboard up the dock
folds away; the Machine tab has it.

The key line is at the bottom: an SSH app that shrinks the terminal for
its keyboard keeps it above the keys. A tap on a key presses it.

Turn the phone on its side, at 60 columns or more, and the list and the
detail of the selected agent sit side by side.

Every key is on the key bar of an SSH app: letters, digits, arrows, `Enter`
and `Esc`. The matchblox app adds its nine keys. See [Keys](keys.md).
