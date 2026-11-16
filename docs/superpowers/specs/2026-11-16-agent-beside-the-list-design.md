# The agent's own pane beside the list

Approved approach A by the owner on 2026-11-16 ("A: real tmux pane beside
list"). This document turns it into a design; its calls are marked
"proposed".

## Who, and the moment

The owner works at a desk with many agents. They read the list, choose the
agent that needs them, and want to *be in that agent*: type into its own
prompt, answer its menus, press Esc, use `/commands` and `@files`, scroll and
copy with the mouse. Today (v0.46.0) the right side is a drawn copy of the
agent's pane, and matchblox's own line under it sends text as a message.
Owner, 2026-11-16: "the sending of messages to the harness instead of being
in the harness" is one of the two main bugs.

## Intent

The right side of the console is the agent's real terminal. Keys go to it by
tmux, not through matchblox. The list stays beside it, one key or one click
away. This is the 2026-11-07 ruling again (#334): "always tmux", "we dont
need to reinvent the wheel".

## Approaches

| Approach | Trade-off | Chosen |
|---|---|---|
| A1. A tmux client of the agent in a right-hand pane of the console's window (nested attach to a session grouped with the agent's, as the jump to a peer does today) | Real harness; the agent's pane never moves; a peer works the same way over ssh. A nested client: the outer prefix must stay the console's. | **Yes** (proposed; within the owner's A) |
| A2. `join-pane` the agent's pane into the console's window | Real harness, one tmux level. The agent's own session ends when its only pane moves out (recovery reads it as lost); names derived from the pane's target read "matchblox"; every client of the console's window sees the move (the #334 anti-goal); a peer's pane cannot be joined. | No: research 2026-11-16 |
| B. The console forwards each key to a drawn copy | Peers alike; a copy with lag and no mouse; the wheel #334 said not to rebuild. | No: owner 2026-11-16 |

## The design

- **The window.** On a desk, the console's window (`matchblox` session) holds
  two tmux panes: the list (the console process, 40 columns; 32 on a mid
  screen) and the **agent pane** beside it. The console makes the agent pane
  with `split-window -h` when it starts in its home window, and keeps it.
- **What runs in the agent pane.** The chosen session's view:
  - this host: `tmux` with `TMUX` unset, running `Jump.Remote()`'s list with
    `Outer: true` for the chosen pane: a session grouped with the agent's
    (`matchblox-jump-<id>`), its own prefix off, `mouse on`, the agent's pane
    selected, `destroy-unattached on`, then `attach-session`;
  - a peer: `JumpSSH` to the host with the same list.
  - The agent keeps its own session, window, pane id and name. No pane moves.
  - This host's view is a nested client of the same tmux server, which no
    jump does today: a local jump is `switch-client`. The view's session sets
    `detach-on-destroy on`, so the client ends when the agent ends, never
    moves to another session. A pane of the console's own window is never
    shown in the agent pane (it would show itself); such a session opens with
    `ctrl+o`, as today.
  - The agent pane carries the agent's kind (`@matchblox_agent`), so the
    command keys and Pocket's keys, which tmux takes in the outer client
    first, reach the agent as they reach a jump today. The console finds its
    own pane by its pane id (a window option), not by the active pane.
- **Choosing a row** with ⏎, or a click or a tap on it, respawns the agent
  pane (`respawn-pane -k`) with that session's view and gives it the keys.
  The old view's session ends with its client. ↑↓ in the list move only the
  choice: the agent pane keeps the agent it shows until ⏎. The take-over
  question on a host the stoker drives, and a peer's wait for Enter, come
  first, as they do for `ctrl+o` today. (Owner's call 2 below, settled:
  follow on ⏎ only.)
- **Focus.**
  - The way back to the list: the console's own way-back keys (`prefix m`,
    the APC key 900, a tap on the list) select the list pane when the agent
    pane is in the console's window. A click on the list works too.
  - In the agent pane every key reaches the agent, Esc too. Only the outer
    `prefix` and the server's own root bindings stay tmux's, as in a jump
    today.
- **The line and the digits** (the drawn chat, #411/#423) are not on a desk
  any more. They stay where there is no agent pane: the iOS demo, a console
  with no tmux, and the phone's text box (below).
- **A phone (Pocket) and a narrow screen** (where the list and 40 columns of
  agent do not fit; the window's width decides, not the list pane's) show one pane at a time: the console zooms the
  pane in use (`resize-pane -Z`). A tap on a row zooms the agent pane; `◂`
  (Pocket's back) or the way-back key zooms the list. The way back always
  ends on the list: it selects the console pane, and zooms it where one
  pane shows. Pocket's text box
  types into the agent pane as keys, as it does in a jump today.
- **The header** keeps the list's counts. The agent pane has no matchblox
  header: its status line is off (`Outer`).
- **Sizes.** The agent's window takes the agent pane's size while it shows,
  as in a jump today (`window-size latest`). It goes back when the owner
  works in the agent's own session.
- **The console's own keys** (`m` menu, `x`, `p`, `z`, `g`, `s`, `/`, …) act
  in the list, as today. In the agent pane they are the agent's keys.

- **Devices share one view** (owner, 2026-11-16: "one shared view is
  best"). Every device on the console's window sees and drives the same list
  and agent pane, as they share one console today. A choice on the phone
  changes the desk's right side. The tmux tools for agents work the same
  way: devices attach to one shared session. A view per device is not built.

## A chain on one screen (second step)

Owner, 2026-11-16: "on desktop, if I can see all sessions in a chain, like
on a single screen I can see (up to X sessions) their panes in one screen".

- On a desk, `g` on a chain's row (today: the chain's room) opens the
  **chain view**: the list stays on the left, and the right side holds one
  agent pane for each member of the chain, tiled (`select-layout tiled` on
  the right-hand panes only). Each pane is that member's own view, as above.
- Up to 4 members show (proposed: 4, the most that leave each agent 60 × 15
  or more on a 16-inch laptop). A chain with more shows its most urgent 4,
  and the list marks which members are on screen.
- A click on a pane, or ⏎ on a member's row, gives that pane the keys.
  `prefix z` (tmux's zoom) shows one member full size and back.
- `esc` in the list, or `g` again, closes the chain view: the right side
  shows one agent again, the chosen row's.
- The chain's room (its stoker talk) opens with a key in the list (proposed:
  `G`), as the chat's place opens it today.
- This step builds after the single agent pane works: it uses the same view,
  more than once.

## Non-goals

- `join-pane`, `move-pane` or `swap-pane` of an agent's pane.
- A matchblox line on a desk, or a matchblox key map inside the agent pane
  beyond the way back.
- Changing `matchblox sessions --json`, `peek`, `send`, or the `ctrl+o` jump.

## Anti-goals

- An agent's own session, name or pane id changes because it was shown.
- A key typed for the agent lands in the list, or the reverse, with no sign
  of which pane has the keys (the active pane's border shows it, proposed:
  the agent pane's border in the accent colour while it has the keys).
- Walking the list with ↑↓ attaches and resizes every agent on the way.
- The console's window is left with a dead or stale agent pane after the
  agent ends: the pane says the session ended, and names the way back.

## Left to the builder

- How the user-key bindings (mark, belt, tap, viewer hook) find the console
  pane when the agent pane is active (`atConsole` reads the active pane
  today): by the console pane's id, kept in a window option.
- The exact respawn and debounce mechanics, and how a failed attach shows.
- Which existing tests move from the drawn chat to the demo and phone paths.

## Calls to check

1. **The phone keeps the drawn chat and line, or zooms the real pane?**
   Proposed: zoom the real pane, because the owner chose "type straight into
   the pane" and Pocket's belt already sends keys to a jump. Cost if wrong:
   the phone's TUI is hard to type into; the drawn chat is still in the code
   for the demo, so going back is small.
2. **↑↓ in the list moves the agent pane after a pause, or only ⏎ does?**
   Settled after the builder check: only ⏎, so the take-over question and a
   peer's wait come first. Cost if wrong: the right side lags the choice
   until ⏎; a pause-follow for this host only is a small addition.
3. **The list's width on a desk:** 40 columns as today. The agent gets the
   rest.

## Done when

- At a desk, choose `kindle-web`, press ⏎: its own Claude Code prompt is on
  the right, and typing `/help` there opens Claude Code's help.
- At its permission prompt, `2` answers it, in the agent's own menu.
- `prefix m` gives the keys back to the list; ↓ to `flint-auth`, ⏎: the right
  side is flint-auth's own pane.
- `tmux list-sessions` shows `kindle-web` and `flint-auth` unchanged, and
  `matchblox sessions` names both as before.
- A session on a peer host opens on the right the same way, over ssh.
- On Pocket, a tap on a row shows the agent full screen; `◂` shows the list.
- On a desk, `g` on the chain `login` shows `kindle-web`, `flint-auth` and
  `hearth-api` each in its own pane beside the list; a click on one types
  into that agent.
