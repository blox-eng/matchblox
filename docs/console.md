# The console

From the top: the header, the tabs, the open tab, then usage and the
machine, and at the bottom the key line.

The header shows the host, how many agents wait for you (`● 3 wait for
you`; a tap selects the first) and [the stoker](stoker.md): `stoker off`,
`stoker`, or `☾ night → 07:00`. For a moment after an action it says what
ran instead.

The tabs are `sessions`, `ships`, `machine`, `git`, `history` and `panes`,
opened by the digits `1-6`. The open one is uppercase. A narrow screen shortens the
tabs far from the open one to their digit. `machine !` has a problem.

The key line at the bottom shows the keys of the open tab, and how old the
sample is. A tap on a key presses it. It is also where you confirm a step
or type a search. See [Keys](keys.md).

## Usage and the machine

Under the open tab, when there is room, the dock shows how much of each
plan your agents have used, and the machine:

```
 USAGE                                    │ MACHINE        ws-1 · 16 cores
 ✻ Max    5h   ▰▰▰▰▰▱▱▱ 62%  · 16:00      │ cpu  ▃▄▅▄▃▅▆▅ 38%   load 4.2
          wk   ▰▰▰▱▱▱▱▱ 41%  · Thu 09:00  │ mem  ▰▰▰▰▰▱▱▱ 41 of 64 GB
 ❋ Plus   5h   ▰▱▱▱▱▱▱▱ 12%  · 18:40      │ disk ▰▰▰▰▰▰▱▱ 71% of 1 TB
          wk   ▰▰▱▱▱▱▱▱ 30%  · Sat 11:00  │ runs 13 claude · 1 codex
```

- Each account shows its windows as `/usage` does: `5h` is this 5-hour
  window, `wk` this week, each with the percent used and when it resets.
  A bar turns `warn` from 60% and `neg` from 90%.
- The week ends with whether it lasts at your pace so far. Hours you do not
  usually work count as a tenth: set them with `quiet_hours` and
  `quiet_days` in the [config](config.md).
- `out Fri ~20:00`: the recommendations name the session of that account
  that burns the most. `Enter` jumps to it, and `x` compacts it while it is
  idle. The row then says what happened, or what failed and that the key
  tries again.
- On a phone the dock shows one of the two. `u`, or a tap on `machine ›`,
  switches.
- `disk` is the file system that holds your home directory, and `/` beside
  it when that is another one.
- Claude Code's status line gives the 5-hour window and the week only, so
  there is no `opus` or `sonnet` line.
- When the screen is short (a phone with its keyboard up), the dock folds
  away. The Machine tab has all of it.

Where the figures come from:

| Agent | What to do |
|---|---|
| Claude Code | Open the door "Show your limits" in Sessions. It adds matchblox to your status line; a status line you have keeps showing what it showed. |
| Codex | Nothing: Codex reports its limits after each turn. A plan with a monthly limit shows `mo`. |
| OpenCode | It shows "not measured": OpenCode keeps no limits. |

A line that says `as of 14:02` has not changed for an hour: the account's
agents have not worked since. Each reading is kept for five weeks in
`~/.local/state/matchblox/limits.jsonl`, on this machine only.
`matchblox status` has every figure under `limits`.

## The match

Each agent row starts with a match:

| Mark | Word | It means |
|---|---|---|
| `✦` | busy | The agent works. The match burns. |
| `✦` | agents | Its turn ended and its subagents still work: their results start its next turn. It does not wait for you. |
| `╿` | asks, waits, done, paused | The agent waits for you. The match is at rest. |
| `│` | its state | The context is full (at `compact_tokens`, 350k by default, or `compact_at`, 85% of its window). The match is spent, and Sessions says `! compact`. |
| `╿` or `│` | out | The plan's limit stops an idle session (`│` when it is also full). The next column says when it can work again: `19:50`, or the weekday when that is more than a day away. A session left a day or more keeps its word: `cold`, `stale`. |

## 1 Sessions

The console opens here. On top, while there are some: the stoker's card
and the setup steps (`SET UP`). Then every agent, by what it needs from
you, and last the recommendations, each with its evidence and its exact
command. `d` dismisses the selected recommendation until its evidence
changes; `h` folds them all to one line, and `h` again shows them. The
console remembers both.

- **WAITS FOR YOU**, the oldest wait first:
  - **asks**: the agent asks for a permission, or asks with its question
    tool.
  - **waits**: the agent asked a question, or waits for input.
  - **paused**: the turn ended with its progress bar under 100%.
  - **done**: the turn ended.
- **WORKING**: `busy`, `agents` (its subagents work), `tests` or `shell`
  (a shell it left running).
- **QUIET**: `idle`, then `cold` after one day, then `stale` after seven
  days, the newest first. On a stale session, `x` and a typed `y` end the
  agent.

A row shows the match, the name, the state, how long it waits and its
context use (`warn` from 70%). A wide screen adds the plan and the ship the
session drives with its track (`#4551 ●━◉┄○┄○`, see [Ships](ships.md)); with
no ships read, the open pull request of its worktree (`#N`) and its tmux
place.

On a phone the selected row opens: what the agent said last, its ship, its
plan and its keys. The console reads the screen's shape (rows × 2 ≥
columns) before its width: a phone held upright under 90 columns is a
phone, and from 90 columns (a small font) its detail is a drawer beside
the list. Wider, the same shows in a detail beside the list, with
the account, the tokens and the background work.

`Enter` jumps to the pane. Come back from a jump and the next agent that
waits is already selected.

### The answer peek

`a` on an agent opens the conversation with it. Where the detail sits
beside the list, the detail pane becomes the conversation: the list stays
lit on the left and follows the peek, and `Esc` gives the pane back. On a
one-column phone it is a panel along the bottom, over the dimmed list. On an agent that waits it answers; on
any other it sends a message, titled `To <name>`, which Claude Code queues
while it works.

```
╭─ flint-auth asks · 1 of 2 ─────────────────────────────────╮
│ The refresh endpoint is in and its tests pass. One choice  │
│ is yours before I wire the rotation: what should a refresh │
│ do with the token it was given?                            │
│                                                            │
│ Should a refresh also rotate the token?                    │
│ › 1 Rotate it  the old one stops at once                   │
│   2 Rotate, keep the old one a minute                      │
│   3 Keep the token                                         │
│   4 Type an answer…                                        │
╰─ ⏎ question 2 ─ ← prev  3/7  next → ─ esc ─────────────────╯
```

It shows what the agent said in this turn, its end up to nine lines (a
longer reply starts with `…`), its Markdown as plain text. When Claude
Code asks with its question tool, its options are numbered and the last
one types an answer; several questions come one at a time, and a question
of several choices ticks with `Space`. In auto mode the options come from
the queue hooks (see [Agents](agents.md#how-an-agent-tells-that-it-waits)),
so add them again when the setup step asks. Otherwise you type one line.

`Enter` sends it to the pane named in the title: the peek is the review,
so there is no second confirm. The options go to Claude Code's own menu
with the keys it takes, never as typed text, and only while the pane shows
the page they answer: if you answered in the pane yourself, nothing more
is sent. The header says `sending to flint-auth…`, then `sent to
flint-auth`; an answer that did not go says `not sent to flint-auth` and
why on the key line. The peek moves to the next agent that waits, or
closes. There no option is picked, and for a moment it takes no key, so a
second `Enter` never answers a question you have not read. `→` and `←`
move the peek to the next and the previous agent, `Esc` closes. Every
letter is text. A permission prompt is answered in its
pane: its peek shows the prompt, and `Enter` goes there.

- Context use is read from the agent's own files, or shows "not measured".
  See [Agents](agents.md).
- The account is the provider's mark, then the email and plan, or
  "API key": `✻` Anthropic, `❋` OpenAI, `▣` OpenCode.
- An agent can draw a progress bar in its last reply:
  `Progress [████░░░░] <step>`. The console shows it as `▰▰▰▱▱ 75%`.
- `s` sorts by another column and shows one flat list; `S` turns it
  around.
- Agents that exited and wait for their parent are one faint line under the
  sessions. `e` lists them with the command that clears them.

## 2 Ships

The pull requests your sessions drive, each on its track from open to
live. `p` opens it. See [Ships](ships.md).

## 3 Machine

Usage first, then the machine: CPU per core, load, pressure, memory and
swap, temperature, CPU power limits, GPUs, and the CPU share of the
container groups you name in the [config](config.md).

Then the detached busy loops: shells that keep burning CPU after their pane
is gone, named to the pane that started them. `x` and a typed `y` kill
one. While one burns, the tab says `machine !`.

## 4 Git

Every git checkout that your sessions use: dirty paths, the default branch
behind its remote, and worktrees that are merged and safe to remove.
`Space` marks one, `X` marks every safe one, and `x` with a typed `y`
removes the marked ones.

## 5 History

24 hours of CPU, load, pressure, temperature, memory and swap.

## 6 Panes

Every tmux pane, with its command. `Enter` goes to it.
