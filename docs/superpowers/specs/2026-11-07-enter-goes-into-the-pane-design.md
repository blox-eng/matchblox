# Enter goes into the pane

Issue #334. Owner, 2026-11-07.

## Intent

A builder opens the console to reach the agent that waits for them. Today
`Enter` on a session of the console's own host gives the keys to the chat: a
copy of the agent's screen that matchblox draws, that forwards each key, and
that re-implements the agent's question menus, paste rules and ^C. tmux
already does all of that, better, in the agent's real pane.

The owner's words: "always tmux", "we dont need to reinvent the wheel", the
chat stays only for the iOS demo (it has no tmux), and "not the smallest
variant … the best one against the vision".

So: one gesture reaches the agent everywhere. `Enter` goes into its real
tmux pane on the desk, on the phone, on this host or another. Sessions keeps
what tmux cannot give from a list: a live, read-only view of the selected
agent, its state and its question, and the next waiting agent selected when
the builder comes back. The demo, where no tmux can run, shows an honest
stand-in for the pane: the same gesture, the same belt, the same way back.

## Ruling (2026-11-07)

This overrules the README's "Steer without leaving: the chat is the live
screen of the selected agent … `Enter` or `a` gives it the keys" and the key
half of `2026-11-01-every-screen-chat-design.md`. Its layout rules for the
live view (shape is sticky, a view never hides its question) still hold for
the read-only view. Reason: the chat re-implements tmux and drifts from the
agent's own UI (#325, #263 and the menu-copy bugs of #262 are that drift).

## Goals

1. `Enter`, `→` and a tap on a session row go into that session's tmux
   pane, on every host and every device. `Ctrl+O` stays as a second name.
2. The jump moves only the terminal that asked. With the desk and the phone
   attached to the shared `matchblox` session, a jump from the phone never
   moves the desk, and the reverse.
3. Sessions keeps a live, read-only view of the selected agent: its screen,
   its question, its state. The wheel scrolls it back. It takes no keys.
4. The way back is unchanged: `prefix m`, the app's home key, or a tap on
   `‹ Sessions` in the app. On return, the next waiting agent is selected.
5. The phone's key belt drives the real pane: Esc, Ctrl, arrows, Tab, and
   the prefix. Its command keys (`/compact` and the rest) type into the
   real pane too.
6. The iOS demo: `Enter` opens a full-screen stand-in for the pane. The belt
   types into it, the same back key returns, and the demo's task (answer an
   agent) still completes and still leads to the invite.
7. A notification or island tap goes into the pane (`focusJump`), never the
   chat.

## Non-goals

- No new way to answer an agent from the list. The agent's own menu in its
  own pane is the answer path.
- No change to `matchblox send`, chains, the queue, or remote host jumps
  beyond goal 2.
- No new terminal emulator. The read-only view keeps today's capture and
  drawing.

## Anti-goals

- A demo that behaves differently from a paired phone. If the demo shows
  something a real builder never sees, it fails even if it works.
- A jump that moves another person's screen.
- Keeping the key-giving chat alive "just in case" behind a flag.

## What exists today

- Entry points to the chat: `Enter` on a home-host row (`internal/app/
  model.go:1363`), `a`/`→` (`keys.go:100`), taps (`mouse.go:50-160`), the
  wheel (`chat.go:1170`), notification focus when the session waits
  (`model.go:1093`), Ships' stuck ships (`ships.go:474`) and the Stoker's
  failed steps (`stokertab.go:239`).
- The live screen is pushed by the service: the console sends
  `watch:<pane>` with its size; `watchLoop` captures every 150 ms
  (`service/watch.go`). Drawing is about 495 lines of `chat.go`.
- Key-giving is about 525 lines of `chat.go`, plus `menutap.go` (240) and
  the service's `keys:` act (`service/keys.go`, 134) behind
  `keysOnlyFromConsole`. Menu quiet (~100), ^C (~45), paste guard.
- The phone shares the `matchblox` tmux session: `matchblox` started
  outside tmux execs `tmux new-session -A -s matchblox`
  (`cmd/matchblox/main.go:287`). So the phone's jump is a real
  `switch-client`.
- `jump` runs `tmux switch-client -t <pane>` with no `-c`
  (`model.go:1587`): it moves whichever client tmux deems current.
  `focusJump` already targets `-c <tty>` (`model.go:1103`).
- The app lights its command keys only while the console says the chat has
  the keys (`AgentSignal`, `tellAgent` in `chat.go:761`), and never inside a
  tmux pane (`ConsoleView.swift:313`). `CommandKeysArgv` does nothing in a
  pane on purpose (`panes/home.go:76`).
- The demo's `World` stands in for tmux and takes `send-keys`
  (`internal/demo/world.go:143`). Its task completes on a menu pick sent
  from the chat (`script.go:69`).

## Approaches

| | What | Trade-off |
|---|---|---|
| A | Keep the chat, used only by the demo | Cheapest. Keeps ~900 lines for one screen, and the demo shows a product real builders never get. |
| **B (chosen)** | The demo fakes the jump: a full-screen stand-in built from the read-only view plus key forwarding into `World` | One gesture on every screen; the demo is honest. Deletes the key-giving chat everywhere. |
| C | A "pane mirror" with keys everywhere, no jump | The chat under a new name; rebuilds the wheel. Rejected. |

Owner picked B, 2026-11-07.

## The design

### The jump

- `Enter`, `→`, a row tap and `Ctrl+O` run the session's primary action,
  which is the jump. `openChat` and `chatOn` go.
- Inside tmux, the jump names the asking client. The console runs in one
  pane that the desk and the phone share, so it learns the client from
  tmux: the key that asked makes its client the most recently active one
  (`#{client_tty}` of the latest client, as `lastclient.go` reads it). The
  jump then runs `remote.FocusArgv` for that tty, as `focusJump` does: a
  grouped throwaway session, so no other client on the pane's session
  follows. One code path for the desk, the phone and a notification.
  (Call, 2026-11-07: the key just pressed is the freshest activity tmux
  has; a grouped session is what keeps another screen still.)
- When tmux names no client, or two clients were active within the same
  50 ms, the jump is refused with `two screens moved at once: press Enter
  again`, never guessed to the desk.
- Another host: `remote.Jump.Local` gets the same `-c <tty>` on its final
  `switch-client`, so goal 2 holds there too.
- Outside tmux (the desk ran with fixtures or no tmux): today's
  select-window, select-pane, `attach-session` stays.
- Another host: today's `jumpOnHost` stays.
- On a host the Stoker drives, `Enter` asks `take over?` first, as every
  jump there already does (`asksTakeOver`). (Call: going into the pane is
  taking over.)
- On Ships, `a` on a stuck ship jumps into its pane; on the Stoker tab, `a`
  on a failed step jumps into the pane it ran in. `a` stays a second name
  for the jump on Sessions. (Call.)
- A session with no pane: the row says `runs outside tmux: no pane to go
  into`, the existing words.
- On return, `nextAfter` selects the next waiting agent, as today.

### The read-only view

- It keeps `watch:<pane>`, `screenMsg`, the size dwell and today's drawing
  and placement rules. It never takes keys, so `chatKeys`, the key line's
  "keys go to …", and the "too short to show" refusal go. A screen too short
  for the question shows the question card, not the screen.
- The wheel scrolls it back through the service's existing scroll-back
  (`CaptureBack`).
- Its key line names the way in: `enter goes into <name>`.

### The phone

- The belt's plain keys already send terminal bytes. In a real pane they
  reach tmux and the agent unchanged.
- Command keys type into the real pane (owner, 2026-11-08). A new tmux
  binding, beside `CommandKeysArgv`, types the command into the current
  pane when that pane runs an agent matchblox knows; elsewhere it does
  nothing, as today. The app lights its command keys while it is in a pane
  of a known agent (its status line shows the way back and the console has
  named the agent), not only in the chat.
- `AgentSignal` (the `agent=<id>` APC) stays: the console sends it for the
  agent it jumps to, so the app knows which agent's commands to offer.

### The demo

- `Enter` on a demo session opens the stand-in: the selected agent's
  screen full-screen, a top line `‹ Sessions` like a real jump's status
  line, and the belt. Keys go to `World.Run` as `send-keys`, the same as the
  chat sends today, so the task still completes.
- The demo is answered the way a real pane is (owner, 2026-11-08): with the
  belt, not a tap. The stand-in's hint says `type 2 to answer`; the belt's
  keyboard key raises the keyboard; a digit, or the arrows then Enter,
  picks. `World` learns arrow moves on a menu. The demo UI tests
  (`DemoConsoleTests.swift`) are rewritten for the belt path, and the
  invite still follows the answer.
- The back key (`ctrl+a m`, the app's home key, a tap on `‹ Sessions`)
  returns to Sessions with the next waiting agent selected.
- `intoPane`'s demo refusal goes; the stand-in replaces it. Other demo
  refusals stay.
- It reuses the read-only view's drawing; it is not a second chat. Its only
  key handling is "forward to `World`" plus the back key.

### What is deleted

- Key-giving: `chatKey`, `pressed`, `heldKeys`, `chatPaste`, `tellAgent`'s
  key half, `commandKey`, `pasteTexts`, `toAgent`, the send queue,
  `sendKey`, `keyAnswered`, `keepChat`, `reopenChat`, `wheelChat`'s open.
- `menutap.go`, menu quiet (`menuQuiet`, `chatHushed`, `atMenu`), the ^C
  window, the paste guard.
- The service's `keys:` act and `keysOnlyFromConsole`, once no caller is
  left (the demo forwards into `World`, not the service).
- The Stoker takeover's hold for chat keys (`stoker.go:182-211`).
- Their tests, and the docs sections: `docs/console.md` "The chat",
  `docs/keys.md` "The chat", `docs/phone.md` 245-281, README 77-81.

### Docs

- README "Steer without leaving" becomes "Go straight in": `Enter` goes
  into the agent's own pane; Sessions shows its live screen read-only.
- `docs/console.md`, `docs/keys.md`, `docs/phone.md` rewritten for the jump
  and the read-only view; `docs/phone.md` says what the belt does in a pane.
- `CHANGELOG.md` under Unreleased: Changed.

## Answered questions

1. Command keys inside a real pane: they type into it (owner, 2026-11-08).
2. The demo's answer: with the belt, like a real pane (owner, 2026-11-08).

## Left to the builder

- Exact names, the split of `chat.go` into the view and the demo stand-in.
- Whether `→` keeps a meaning where a row has no pane.
- The order of deletions, provided each commit builds and its tests pass.
