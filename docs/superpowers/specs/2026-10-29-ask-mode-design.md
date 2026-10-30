# The chat: read and steer any agent's live screen from the console (#261)

## Intent

A builder runs about ten agents. They watch them on a wide desk screen and
from the iPhone app. When one agent asks or gets stuck, the builder wants to
read its whole screen and steer it with `↑ ↓ Enter Tab Shift+Tab Esc Ctrl+C`
and plain typing. Today that means leaving the console for the tmux pane, or
opening Termius on the phone.

The goal, in the owner's words: "we don't even need to jump into a
terminal". The owner wants to delete Termius from the phone. Later the same
view serves agents that run outside tmux (herdr).

## What the builder sees

The owner's three parts (2026-10-29):

1. **The pane, consolidated into the console.** There is no "ask mode" to
   enter and no jump to tmux. On a desk or an expanded phone, the right
   side always shows the selected session's live screen. On a phone that
   is not expanded, the session takes the whole screen.
2. **The simplified iOS keys.** Only the keys the builder needs, and each
   one large enough to hit with a finger.
3. **Stats consolidated.** Usage and Machine go in the left column on a
   desk. On a phone that is not expanded, they stay at the bottom, and a
   key cycles through them.

**The chat** is the selected session's live screen, drawn inside the
console. It is always shown. The only question is which side has the keys.
`Enter`, `→`, `a` or a click moves the keys to the chat. `Ctrl+]`, or a
click on the left, moves them back to Sessions. The key line always names
the side that has the keys. (My call: one key area must own `↑`/`↓`, so the
chat cannot take keys without a way to move them back.)

### Wide (a desk, `Wide` layout)

```
┌ Sessions ─────────────────────────┐┌ api-auth · waits · claude ───────────────────────────────────────┐
│ ● api-auth      waits   2m        ││ ⏺ I can fix the login in two ways.                                │
│ ◐ web-cart      works  12m        ││                                                                   │
│ ○ docs          idle    1h        ││ ╭───────────────────────────────────────────────────────────────╮ │
│ …                                 ││ │ Which way?                                                    │ │
├ Usage ────────────────────────────┤│ │ ❯ 1. Patch the token check                                    │ │
│ max   ▇▇▇▇▇▁▁▁ 61%  resets 3h     ││ │   2. Move to the new session store                            │ │
│ pro   ▇▇▁▁▁▁▁▁ 22%                ││ │   3. Type something.                                          │ │
├ Machine ──────────────────────────┤│ ╰───────────────────────────────────────────────────────────────╯ │
│ cpu 34%  mem 41/64G  disk 71%     ││                                                                   │
│ 10 agents  2 wait                 ││ ┌ Which way? ─────────────── (card) ┐                             │
└───────────────────────────────────┘│ │ ❯ 1 Patch the token check         │                             │
 ⏎ chat  a chat  ↑↓ select  …        │ │   2 Move to the new session store │                             │
                                     │ │   3 ▏                             │                             │
                                     │ └───────────────────────────────────┘                             │
                                     └───────────────────────────────────────────────────────────────────┘
                                      keys go to api-auth · ctrl+] back · ctrl+o into tmux
```

1. The left column stacks Sessions, Usage and Machine. Usage and Machine
   leave the dock at the bottom and go into this column.
2. The right side is the selected session's chat, at full height. It shows
   the live screen while Sessions has the keys. `↑`/`↓` in Sessions changes
   which session it shows.
3. `Enter` (or `a`, or a click on the right side) gives the keys to the
   chat. The chat's frame takes the accent colour. The key line says
   `keys go to <name>`.
4. In the chat, every key goes to the agent: `↑ ↓ ← → Enter Tab Shift+Tab
   Esc Ctrl+C Backspace` and plain typing. There are two exceptions:
   - `Ctrl+]`, a click on the left column, or the app's home gesture gives
     the keys back to Sessions.
   - `Ctrl+O` goes into the tmux pane, as `Enter` does today. It is a way
     out for a screen that the chat cannot handle.
5. If the pane is gone, the chat says `That session ended.` and Sessions
   gets the keys back.

### Medium (a phone opened up, or a narrow desk)

The list and the chat split at the middle, as the list and the detail split
today. The keys work as in Wide. Usage and Machine stay in the dock.

### Narrow (an upright phone)

1. Sessions fills the screen. Usage and Machine stay in the dock at the
   bottom, one panel at a time, and `u` (or a tap on the dock) cycles them,
   as it does today.
2. `Enter`, or a tap on a row, replaces the screen with that session's
   chat, with the keys in the chat. The title row reads `‹ Sessions ·
   <name> · <state>`.
3. A tap on `‹ Sessions`, the app's edge swipe or `Ctrl+]` goes back to
   Sessions. You stay in the console all the time.

### The question card

When the session waits on Claude Code's question tool (`said.Ask`), a card
sits at the bottom of the chat. It holds the parsed question and its
options, drawn by matchblox to fit the width of the view. The card does
what the peek's question rows do today:

- `↑`/`↓` move its cursor, `Space` ticks an option of a multi-choice
  question, typing fills the "Type something" row, and `Enter` sends the
  answer through the existing `pick:` act.
- A tap on an option picks it.
- When the card sends, the key line shows the peek's not-sent and sent
  lines.

When no question is open, or when the screen shows a permission prompt,
there is no card. The keys go to the agent as they are.

2026-10-30: while the card shows, the chat drops the agent's own menu (the
lines from the screen's last rule down). Reason: two lists of the same
options cost a phone a third of its screen, and one way to answer is enough.
The history is never cut.

### The iOS key bar

1. SwiftTerm's default accessory goes away. In its place is a bar of seven
   keys and no more: `↑ ↓ ⏎ Tab ⇧Tab Esc ^C`. The system keyboard does all
   the typing.
2. Each key is at least 44 pt and works on the first tap. The bar stays
   above the keyboard in both orientations. When the keyboard is down, the
   bar stays at the bottom of the screen.
3. Autocorrect, suggestions, smart quotes, smart dashes and auto
   capitalisation are off on the terminal view.
4. The keys send these bytes: `ESC [ A`, `ESC [ B`, `CR`, `HT`, `ESC [ Z`,
   `ESC`, `ETX`.

## Calls settled (2026-10-29, the owner)

- **Enter at a question.** In the chat, `Enter` always goes to the agent. It
  picks the option under the cursor, approves a permission prompt or sends
  what was typed. No `Enter` goes into the pane. This replaces the issue's
  ruling that "a second empty Enter goes into the pane". Reason: the chat
  is the whole interface, and going into tmux is the case to remove.
  `Ctrl+O` stays as the way into tmux (my call: it costs nothing and is a
  way out).
- **Esc.** `Esc` goes to the agent, because Claude Code needs it to cancel a
  question, close a menu or stop a turn. `Ctrl+]`, a click or tap on
  Sessions, or the app's home gesture goes back. The owner agreed.
- **The peek.** The nine-line peek goes away as a mode of its own. `a`
  opens the chat. The peek's question rendering lives on as the question
  card. Reason: two ways to answer is one too many, and an upright phone
  cannot read a 200-column screen without the card.
- **#122 and #141.** Close #122 as replaced by the fixed seven keys. Keep
  #141 open for font size and row taps.
- **Agent state.** matchblox keeps its own parser for agent state. It
  reads no OSC 7501.

## Goals

- Every Done-when item of #261, with "ask mode" read as the chat.
- One way to answer: the chat. The peek, its goldens and its docs go away.

## Non-goals

- A key bar that the builder can arrange (#122).
- Swipe for arrows (#123).
- A shell to a host that does not run matchblox, SFTP or port forwarding.
- A terminal emulator in the TUI. The chat shows what tmux already holds.
- Scrollback in the chat. Added on 2026-10-30, on the owner's ask (#263):
  the wheel scrolls the agent's history. See docs/console.md.
- Agents outside tmux (herdr). The design keeps the screen source behind
  one function, so that a later source can replace it.

## Anti-goals (failure even if it works)

- A key typed in the chat reaches a different agent from the one shown.
- `Enter` sends an answer to a question that the builder has not seen.
- The chat lags the pane by more than one sample.
- The phone's keyboard adds suggestions or autocorrect inside a key
  sequence.
- An upright phone is harder to read than it is today.
- A second `^C` tap within one second goes out silently. In Claude Code,
  two `^C` presses close the agent.

## Approaches

**How the console gets the live screen.**

1. **Watch one pane with `capture-pane` (picked).** The console tells the
   service which pane it shows (`watch:<pane>`). The service captures that
   pane every 150 ms with `capture-pane -p -e`, and once more right after
   each key act. It sends a new `screen` message to that console only when
   the text changes. This reuses `sample.CapturePanesStyled`, works the
   same for local and remote consoles, and costs one tmux exec per
   150 ms. The lag anti-goal holds: 150 ms is far less than the 2 s
   sample.
2. **A tmux control-mode stream (`tmux -C`, `%output`).** This is lower in
   latency. But it sends raw output bytes, so the console needs a terminal
   emulator to rebuild the screen. The issue rules that out.
3. **A nested tmux client in the right side.** This means a pty and an
   emulator inside Bubble Tea. It has the same problem as approach 2, and
   it also fights the console's own layout.

**How keys reach the agent.** This is a new act, `keys:<pane>`. Its `Text`
holds one key: a tmux key name (`Up`, `Down`, `Enter`, `Tab`, `BTab`,
`Escape`, `C-c`, `BSpace`, `Left`, `Right`) or literal text sent with
`-l --`. The service runs `panes.Keys`.

- **Right agent.** The act carries the PID that the console showed. The
  service refuses the act when the pane's agent PID differs (see #198: a
  new process on an old pid). The refusal says
  `not sent: <pane> runs a different agent now`.
- **Order.** The service runs each act in its own goroutine, so the
  console keeps the order. It has at most one `keys:` act in flight. Keys
  pressed while one is in flight queue in the console and go in order.
  Plain text that follows plain text goes as one literal run.
- **No dedupe.** `keys:` skips the service's `acts.begin` dedupe. Two `↓`
  presses are two acts, not "already done".
- **Seen before sent.** Each `screen` message carries `At`, the time of its
  capture. An `Enter` act carries the `At` of the frame that the console
  showed (`Act.Seen`). If the session's wait started after that frame (the
  queue item's `Since` is later than `Seen`), the service refuses with
  `not sent: <name> asks something new — look again`. Typing and arrows
  carry no such guard: they change nothing on their own.
- **`^C` twice.** A second `Ctrl+C` within one second is held. The key line
  says `^C held: one went 0.6 s ago, ^C again stops the agent`. A third
  press within the next second sends it.
- **Console only.** `keys:` and `watch:` are refused for CLI clients and for
  acts from the stoker's server (`fromStoker`). An agent must not drive
  another agent's screen key by key.

**Fitting the screen into the view.** The pane is often wider than the
chat. My call:

- Trim trailing blanks on each line.
- Cut lines made only of box-drawing characters (rules, frame edges) to
  the view's width.
- Wrap other lines with `ansi.Wrap` and keep the bottom of the screen in
  view, where the prompt and menus are.
- On Wide and Medium, also offer `tmux resize-pane`. This is out of scope:
  the chat must not resize a pane that the builder may be attached to.

## Constraints

- The console is a client of the service (`proto`). The chat must work the
  same over a remote connection and in the iOS app, which runs the console
  over SSH in SwiftTerm.
- The pane acts and their guards live in `internal/service/actions.go` and
  `hands.go`. `keys:` joins `paneActs`, not `typedPane`. The stoker's order
  limits stay as they are.
- `internal/stoker/peek.go` and `matchblox peek` (CLI) are a different
  thing and stay.
- Screen goldens are in `testdata/screens`. New files:
  `chat-200x50.txt` (the wide chat with a card), `chat-132x32.txt`,
  `chat-44x31.txt` (the phone's chat) and `chatended-44x31.txt`. The
  `peek-*.txt` files go away.
- The iOS part builds on a Mac or in CI (`.github/workflows/ios.yml`).
  There is no Mac build on this host.

## Code that changes

- `internal/app/layout.go` and `dock.go`: Wide puts Usage and Machine into
  the left column, and the dock stops drawing on Wide.
- `internal/app/view.go`, `panels2.go`, `sessions.go`: the right side is the
  chat on Wide and Medium, and Narrow swaps to it.
- `internal/app/keys.go`: `Enter` and `a` on Sessions open the chat.
  `Ctrl+O` goes into the pane. The key line changes.
- `internal/app/peek.go`: becomes `chat.go`. The peek's box, ring and
  overlay go away. `peekBox`'s question rows become the card, and
  `peekChoose`/`sendPeek` stay for the card's `pick:`.
- `internal/app/stokertab.go`, `ships`: `a` on a stuck ship or a failed
  step opens the chat of its session.
- `internal/proto`: a `screen` kind (`pane`, `at`, `text`), `Act.Seen`,
  and the `watch:` and `keys:` acts.
- `internal/service`: the watch loop, `keys:` with its guards, and the
  console-only check.
- `ios/Matchblox/ConsoleView.swift`, `ConsoleTerminalView.swift`: the
  seven-key bar, the text traits, and the home gesture, which the console
  now also takes as "back to Sessions".
- Docs: `docs/keys.md` (the answer peek becomes the chat),
  `docs/console.md` (the dock, the wide layout, the phone's shape rule,
  the chat), `docs/phone.md` (the key bar), and the DESIGN.md and
  design/0006 references to the peek.

## Left to the builder

- The exact width split on Wide. The left column takes the width of
  Sessions with its columns, capped at 40%.
- The watch interval within 100–250 ms, if 150 ms measures badly.
- How the home APC (`panes.HomeKey`) is parsed inside the console.
- Whether Medium shows Usage and Machine in the left column too, when the
  height allows it.
