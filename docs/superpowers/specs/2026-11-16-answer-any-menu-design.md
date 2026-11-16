# Answer any agent's menu from the chat

Approved by the owner on 2026-11-16 ("the best way it is").

## Who, and the moment

The owner reads a session in the console's chat, often on the phone. The
agent waits on a menu: a permission prompt, a question with several parts,
a Codex or OpenCode choice. Today the digits answer only Claude Code's
question tool with one question. For every other menu the owner must go
into the agent's pane (`ctrl+o`), which on a phone is a full TUI on a small
screen. Owner, 2026-11-16: "if the harness is asking a question and I am in
the peek and not fully immersed I cannot answer the question at all."

## Intent

Every menu an agent draws can be answered from the chat, with the two
presses the digits already use, on every agent, on a phone and on a desk.

## Approaches

| Approach | Trade-off | Chosen |
|---|---|---|
| A. Read the menu from the pane's screen, send the digit key with a guard | Works for every agent and every menu. Depends on the screen reader; a missed menu falls back to the pane. | **Yes**, owner 2026-11-16 |
| B. Each agent's own channel (a Claude Code permission hook, plugins) | Exact, no screen reading. Per agent and per version; a blocking hook can hang an agent when matchblox is down; Codex has no channel. | No |
| C. A now, B later for Claude Code permissions | B only if A proves weak there. | Kept as the next step, not built |

## The design

- **A menu** is a run of numbered lines (`❯ 1. Yes`, `2. No`) in the cut pane,
  the last such run above the agent's input, while the session is in
  `NEEDS YOU`. The reader is the one `paneAsks` already uses
  (`agentMenuLine`). It does not need a question from the transcript.
- **Answering** stays two presses: a digit or a tap marks the option
  (`❯ 2.`), and the same digit sends it. The 400 ms dead time after a menu
  appears stays. Every digit answer goes by the new act, Claude Code's
  question tool too: v0.46.0 sent its digit as a `keys:` act, which the
  real service refuses (`keys go to an agent in its own pane`; only the
  demo takes it), so on a real machine no digit answered at all.
- **The act** is new: `choose:<pane>`, with the digit and the option's text
  as the console read it. The service reads the pane again under the pane's
  lock. It sends the digit key only if line N of the menu still starts with
  that text, and the agent in the pane is still the one the console saw.
  Otherwise it refuses: `not sent: <name>'s menu changed — look again`.
  It is console-only, like `paste:`. `keys:` stays demo-only.
- **An option that asks for text** ("No, and tell Claude what to do"): the
  pick goes as any other. The agent then opens its text box, the session
  no longer shows a menu, and the line sends the owner's text as a message.
- **No menu read:** the session needs the owner, and the reader finds no
  menu. The line says `answer in its pane · ctrl+o`.
- **Several questions** in one call: the agent draws them one at a time.
  Each one is a menu of its own.

## Non-goals

- Hooks or plugins for each agent (approach B).
- Moving through a menu with the arrow keys, or a menu with more than 9
  options.
- Changing `matchblox send`, `peek`, or `sessions --json`.

## Anti-goals

- A digit approves something the owner did not see: the menu changed, a new
  prompt came, or another agent runs in the pane.
- A digit typed as text into a question box picks an option.
- A refused pick fails silently: the line always says why.

## Left to the builder

- Where the reader stops, so that the agent's own numbered text above the
  menu is not read as a menu.
- The exact key each agent takes for a pick (the digit, or the digit then
  Enter). The default is the digit alone.
- How the service compares the option text: after ANSI is stripped, and with
  the marker and spaces trimmed.

## Not proven until tried on real agents

- Codex and OpenCode take the digit key as the pick.
- The reader finds the menu in a real Codex and OpenCode pane.

## Done when

- At a Claude Code permission prompt, `2` then `2` in the chat approves with
  "don't ask again", and the pane shows the agent going on.
- The menu changes between the two presses: the second press says
  `not sent: … menu changed — look again`, and nothing goes to the pane.
- On a phone, a tap on an option, then a second tap, answers it.
- A session that needs the owner with no menu read says
  `answer in its pane · ctrl+o`.
