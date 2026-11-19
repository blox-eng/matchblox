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
  more, it lists the hosts. Tap a host to open its console, or
  `+ Pair a host` to add one. The console's `Hosts` button opens the list
  with either count, so you can remove even your only host.
- If the app says `Can't reach <host>.` and `New address?`, type the
  address that the device can reach, then tap `Try again`. The pairing
  stays.
- If the app says that the host has a new host key, the app does not
  connect. Check that the change is yours, then run `matchblox pair` on the
  host and scan the new code. A host key that does not match at pairing
  gets the same message, and the app shows the Address field: the address
  reaches another machine.
- When you quit the console, the app says that the console ended. Tap
  `Open again` to open it, or `Hosts` to go to the list.
- The console opens on the list. Tap a row to open its chat, and `◂` to
  come back. Tap an option of its question to mark it, and tap it again to
  send it. Long-press a row for its menu. See
  [The list](console.md#the-list).
- Inside tmux, the console may have the agent's own pane beside the list.
  On a phone one pane shows at a time: a tap on a row shows the agent
  full screen, and `◂` goes back to the list. The phone and the desk share
  one view, so a choice on the phone changes the right side of the desk.
  See [Keys](keys.md#the-agent-beside-the-list).
- While the console shows Sessions or Machine, the key belt is on its
  Board layout. Home is `◂`, then the keys of the chosen session
  (`1 answer`, `p merge`, `y go`), then `↑ ↓ ⏎`. The rest of the belt has
  `c copy reply` first, then `⌃o expand` and the list's keys: `s hosts`,
  `g chain`, `/`, `m menu`, `z park`, `N new`, `⇥ machine`, `x kill`. With
  a short card, `c copy reply` shows at rest. These keys act on
  the chosen session while you write in its line too. A letter you type
  on the keyboard types. While the line holds text, a digit types, so the
  digits are dim, and in the line `↑ ↓` are dim too. A key that does not
  apply is dim and sends nothing. In a pane, the belt goes back to the
  layout it had there.
- A tap on the belt is its own signal, and it names its phone. So with a
  desk and Pocket on one console, the belt's taps still act. On a phone,
  `Esc` then a quick letter types the letter in the line. `⌥` and a
  letter types it too: `⌥s` does not open the room there. A tap on
  `y go` does not give the go. The first tap arms it: a notice says what
  the go does, for example `y again gives the go: merge #4556`. Tap `y`
  again to give the go. Any other key or tap disarms it. While the
  consent box shows, the belt is `◂`, `y runs` and `esc cancels`: a tap
  on a row is never consent.
- **Text size:** pinch the terminal, or tap `A−` or `A+` under
  `Console text` in the Island screen. The size goes from 9 to 24 pt, and
  the phone keeps it for every host. The host gets the new columns and
  rows.
- **Back to the bottom:** after a swipe scrolls an agent's pane up, a `↓`
  button shows at the bottom right. Tap it to go back to the live output:
  tmux leaves its copy mode. A key you type hides it too.
- **Copy the last reply:** tap `c copy reply` on the Board layout. The
  console copies the chosen session's last message, whole, and the phone
  says `Copied`. The phone takes a copy from the host only in the 3 s after
  that tap or a key you type, and only up to 1 MB. The copy stays on this
  device.
- **Links:** tap a link in the terminal. The phone shows the link with
  `Open in Safari`, `Copy link` and `Cancel`. Only an http or https link
  opens; any other link can only be copied. A tap off a link clicks as
  before.
- A swipe on a host in the list removes the host and its key from the
  device. It does not remove the key on the host. Run
  `matchblox devices rm <name>` on the host for that.

### Try a demo

With no host, the pairing screen shows `Try a demo` below the scan. With two or
more hosts, the Hosts screen shows it beside `Island`. The scan view shows a
code of its own and scans it by itself. On a fresh install, the pairing screen
asks for the camera first, and `Try a demo` works whether you allow or deny it.
The demo's own scan turns on no camera and asks for no permission. Then the app
saves a host called `demo`.

The demo runs the real console on a made-up machine inside the app. It needs no
network and starts fresh each time it opens. The list opens with
flint-auth first in `NEEDS YOU`. Tap it to open its chat, with the options of
its question. Tap an option to mark it, then tap it again to send it.
flint-auth moves into `WORKING`. When
that agent finishes, a sheet says
`Your agents wait the same way.` It shows the line
`curl -fsSL https://matchblox.sh | sh` with `Copy`, and these buttons:

- `Send to my computer` opens the share sheet with the line and the link to get
  started.
- `Pair a host` goes to the pairing screen.
- `Keep looking` closes the sheet.

A tap on the demo does not raise the keyboard. The key belt's keyboard button
does. A layout you pick in the demo is not saved. If you quit the console, the
ended screen offers `Open again`, `Pair a host`, the line and `Send to my
computer`.

The phone cannot go into tmux, connect to another host, pair a device, update,
merge, make a worktree, clone, or send `^C` in the demo. The demo answers
`This is a demo. Install matchblox to do that.` Pairing a real host removes
`demo`.

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
  the console on that session: on it. If the session has ended, the
  console says `That session ended.`
- **The setting:** the `Island` entry in the app's console, next to `Hosts`.
  `Follow` is the one host that has the island (the host opened last). `Show`
  is `Sessions`, `Usage` or `Auto (Usage while the stoker runs)`. `Buzz` is
  on or off for each host. A host out of reach keeps its old setting until
  the next connect.
- **If the app could not register,** the Island screen says why under the
  host, with the time and the fix, for example `Not registered: no APNs
  device token — allow notifications for matchblox in Settings`. The line
  goes when a registration with that host succeeds. A host the app never
  tried has no line.
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

## Go into an agent

`Ctrl+O` goes into the agent's own tmux pane, and moves only the phone.

1. Tap an agent's row to open its chat: the chat shows the question the
   agent asks and its last words. Then press `Ctrl+O`, or tap `⌃o` on the
   key belt, to go into its pane.
2. On a host the stoker drives, the console asks `take over?` first. A
   session with no pane says `<name> runs outside tmux: no pane to go into`.
3. Type in the pane. The belt's keys (`Esc`, `Ctrl`, arrows, `Tab`, the
   prefix) reach the agent unchanged.
4. To come back, tap `◂` on the belt, tap `◂ matchblox` in the tmux status
   line, or press `prefix m`. The console then shows the chat you left.

With the agent beside the list, a tap on a row already shows its pane,
full screen on a phone. `◂` on the belt goes back to the list. `ctrl+q`
flips the keys on a hardware keyboard, and the belt has no key for it.
`g` on a chain's row shows up to 4 members' panes in one view, and `G`
opens the chain's room.

The demo has no tmux. There `Ctrl+O` opens a full-screen stand-in for the
pane, and the belt answers it: at a question the key line says `type 2 to
answer`. The stand-in closes when the screen is too short to show the
question.

### The key belt

The app's key bar is a belt. It is one row of keys across the full width
of the screen. It turns without end: after the last key comes the first
key again. A key that reaches a side of the phone folds round it and goes
behind the phone. Drag the belt with one finger. It slows after you lift
and stops with a key at the left side.

- **Home.** Each layout opens at home. At home, `◂` is at the left, and
  `↑` `↓` `⏎` are at the right (each is two cells wide).
- **Beside the list.** When the agent's pane shows beside the list, `◂`
  goes back to the list, and `⏎` shows the chosen agent. The belt has no
  `ctrl+q`: on the phone the way back to the list is `◂` or a tap on it.
- **A key sends when you lift.** A touch that moves 8 pt or more is a
  drag. A drag never presses a key.
- **The feel.** The phone gives one tick each time the belt passes halfway between two stops. Home gives a firmer click.
- **Size.** Each key is at least 44 pt tall. A one-cell key is at
  least 44 pt wide. The terminal turns off autocorrect, suggestions, smart
  quotes, smart dashes and auto capitalisation. So they never change a key
  you send.

- `◂` goes back. In a tmux pane it goes to the console. In the demo's
  stand-in it goes to the list.
- `^C` interrupts. `^C` twice stops Claude Code. A second `^C` within 1 s
  is held, and the key line says so.
- `⇞` and `⇟` send `PgUp` and `PgDn` (`ESC [ 5 ~` and `ESC [ 6 ~`). In a
  pane you went into on a host, the jump table of the host starts tmux copy
  mode one page up, and moves it one page. In a local pane they go to the
  agent.

The row above the belt is the handle row. It shows the name of the layout
(on Commands, the mark of the agent too), the grip, and a ghost keyboard
icon. A tap on the keyboard icon shows or hides the system keyboard. While
the keyboard is on screen, the icon is bright. The belt and the handle row
stay above the keyboard, or at the foot of the screen while the keyboard is
down. The keyboard icon never opens the tray.

#### Layouts and the matchbox tray

Tap the grip or the name, or pull up on the handle row, to open the
matchbox tray. Pull down to close it. Five matches lie in the tray, one for
each layout:

| Layout | Keys |
|---|---|
| Steer (the default) | `◂ ↑ ↓ ⏎ Esc ^C ⇞ ⇟` |
| Commands | `◂ ↑ ↓ ⏎`, then the commands of the agent you are in |
| tmux | `◂ ↑ ↓ ⏎ ⇞ ⇟ Tab ⇧Tab ^B` |
| Mine | The keys you chose |
| Board | `◂`, the chosen session's keys, `↑ ↓ ⏎`, then the list's keys |

The match of the layout in use burns. Tap another match: it strikes, and
the match you leave goes out. The tray closes 0.9 s after your tap. The
next time it opens, every match is new, and the one in use burns. The
screen does not change size while the tray is open. The app keeps the
layout you chose, and opens on it the next time. Board follows the console:
the app is on Board while the console shows Sessions or Machine, as
Commands follows the agent of the pane you are in.

#### Command keys

The Commands layout follows the agent whose pane you are in. Each command
key has the mark of its agent. The keys light in the pane of a known agent,
and a key types its command there, for example `/compact`, with no Return:
you send it. The console marks the panes of the agents it knows, and a key
types only into a pane marked as an agent of the command's kind. In any
other pane the keys are dim and type nothing. See [the keys](keys.md).

#### Mine

Tap `EDIT MINE` on the tray floor. The sheet lists all keys in groups. Tap
a key to add it to Mine, or to take it off. Drag a key to move it. `◂`
always stays: the sheet says `◂ always stays: it is the way back`. Mine is
one set for the device, for all hosts. It stays when you quit the app.

The Ctrl group starts with `Ctrl`. Add it to Mine to type any ctrl key. Tap
`Ctrl` and it stays lit. The next letter you type goes with ctrl, and `Ctrl`
goes dark. Tap it again to let it go. The pane gets ctrl with `b d e f g k l
n p r t u v w x y`, and with `a` and `c`. It sends no `ctrl+z`, which would
stop the agent, and no `ctrl+s` or `ctrl+q`, which stop and start the terminal.

A vertical swipe scrolls the console and a tmux pane. In the console's
list, a swipe chooses the session above or below. In the chat, a swipe
scrolls the chat back through the pane's history, and the chosen session
stays. A chat with no screen to scroll says so. The lines move with
your finger: one line for each line of travel. In a tmux pane each wheel
step scrolls 5 lines, so the app sends one step for each 5 lines of travel.
Swipe down for older lines, and up for newer lines. A flick carries on for
at most 0.5 s, and scrolls the same lines in the console and in a pane.
To select text, touch and hold, then tap Select and drag. Where the
program does not read the mouse, the terminal scrolls its own history.

## Go to an agent's pane and back

`Ctrl+O`, or `⌃o` on the key belt, goes into the agent's pane. In Panes,
a tap on a row goes into it the same way.

To come back in the matchblox app, tap `◂` on the belt, swipe in from
the left edge, tap `◂ matchblox` at the left of the tmux status line, or
press ⌘M or ctrl+M on a hardware keyboard. All of them work whatever your tmux prefix
is, and none needs the tmux mouse.

In another SSH app, tap `◂ matchblox`, or press `prefix m` (the tmux prefix,
then m). There the tap needs the tmux mouse on: press `m` in the preview of
"Add the way back" to turn it on with it, or add `set -g mouse on` to your
tmux config. Without it, use `prefix m`.

The console then shows the chat you left. The iOS app marks its own Return and taps, so a
jump moves only the phone.

## Switch an agent from the phone

Press `A` in the list, or tap `A` on the key line, to open Switch
agent. Tap the agent you want, then
`Enter`; the plan rises in a box above the key line. A typed `y` runs it (a
tap is never consent). The new agent starts in the same pane with the same name, on the account you last
started that agent on, and the console jumps to it.
See [`A` in Keys](keys.md).

## The phone layout

A phone is read by its shape at the tallest height seen at its width, so
the keyboard coming and going never changes the layout. In Matchblox
Pocket held upright, and on any screen under 73 columns, Sessions shows
the list or the chat, one at a time: the list
first. A tap on a row opens its chat, and `◂` comes back.

Machine shows usage and the machine.

The key line is at the bottom: an SSH app that shrinks the terminal for
its keyboard keeps it above the keys. A tap on a key presses it. In
Matchblox Pocket the key belt takes its place.

With a small font that makes the screen 73 columns or more in landscape,
the list and the chat show side by side. See
[Shapes](console.md#shapes).

Every key is on the key bar of an SSH app: letters, digits, arrows, `Enter`
and `Esc`. The matchblox app adds its key belt. See [Keys](keys.md).

## The hosts on the phone

The phone shows what the console of its host can open. If you connected
`ws-1` to `ws-2` yourself, a phone paired to `ws-1` sees `ws-2` in the
combined console and acts on it. The same phone reaches `ws-2` today from
Hosts: on Machine, `Enter` on a host's row opens it. matchblox sets up no key between hosts, so
a phone never sees a host that its host cannot open.

The phone does not combine the hosts that you paired in the app. Each host
you pair has its own console. See [Use the app](#use-the-app).
