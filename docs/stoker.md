# The stoker

Go to sleep with ten agents at work and wake up to ten sessions ready to
go on. While you are away, the stoker compacts each session that fills its
context and has it write one line: how to continue. In the morning you read
that line and pick up where the agent left off.

## Turn it on

| Key | What it does | The header shows |
|---|---|---|
| `n` | A night: the stoker runs until `night_ends` (07:00 by default), then stops. `n` again stops it now. | `night → 07:00` |
| `f` | The stoker on, for lunch or a meeting, until `f` again. | `stoker` |

It runs in the service: close the console, restart the service, and it
keeps going. The first time, **Try a night** under SET UP starts one.

## While it runs

The whole console goes dark blue, and only the matches keep their fire.
One glance, at 03:00 on your phone, tells you the stoker is in charge.

Look around as you like. The first time you act on a session (`Enter` on a
row, `x`, `a`, or a second tap), the action line asks:

```text
 The stoker is on. Take over?  y rests it while you work · n keeps it on
```

- `y`: the stoker rests, the console shows the day, and the header says
  `stoker rests`. What you pressed runs. Work anywhere in tmux, in the
  console or in an agent's pane: the stoker goes on by itself 15 minutes
  after your last key, or a couple of minutes after you close the last
  console. `f` turns it back on now.
- `n`: the stoker stays on, and what you pressed runs.

It asks once a run. A step already sent finishes, and it shows in the
morning.

## Which sessions it compacts

A Claude Code session in tmux that is:

- idle: its turn ended, even while subagents or background shells it
  started still run (the transcript says so, not Claude Code's busy flag),
- due, at the moment that suits it (below),
- not stopped by its plan's limit (`out`): a compact needs the model too,
- not waiting for you: no question, no permission prompt,
- not left for a day or more (`cold`),
- not compacted by the stoker in the last 30 minutes.

## When it compacts

The line is `compact_at` (85% of the window by default) or `compact_tokens`,
whichever is lower. A session past the line is compacted at its next
boundary. Before that, the stoker follows each session's pace: the tokens
it spends for each point of its progress bar (`Progress [████░░░░]`), from
the first moment the run saw it.

- It finishes under the line: left alone. Its context is worth more than
  the room a compact would give it.
- The next 10 points of its bar would cross the line: compacted now, at the
  end of this turn, before the next step starts.
- Otherwise it waits, and says where it will compact.

A session without a bar, or whose bar has moved less than 5 points, has no
pace: only the line counts.

Claude Code's own auto-compact is the net under the stoker: it is the only
thing that compacts inside a turn. The door "Keep a net under every
session" turns it on at the same line (`autoCompactEnabled` and
`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` in `~/.claude/settings.json`); a
percentage you set there yourself stays.

Right before it types, and again before the `Enter`, it reads the pane. A
permission prompt, a question or a line you started to type stops the step.
A night sends 30 steps at most.

It types this:

```text
/compact Keep the task, the branch and last commit, the files you changed, what to read first, every open decision and the next step. End with one line that starts with RESUME: and says how to continue.
```

It never answers a question or a permission prompt, never sends
"continue", and never ends, kills or removes anything.

## In the morning

The night's card is the first thing in the queue, even after a night with
nothing to do:

```text
 NIGHT 23:10 → 07:00 · ready 5/7
 ▌ limit out 04:12 → 06:40
   7 watched · 1 was full · 1 compacted · 1 waited · 386k tokens given back
   api-gw           waits for you
   app-docs         out until 09:10
   billing          69% · finishes ~79% · left alone
   01:33  ╿ app-feature      %1         395k → 9k
      would pass 80% within the next step
       RESUME: run the migration test, then open the PR
   03:02  ╿ app-review       %2         skipped: the pane %2 has text you typed
```

- `limit out 04:12 → 06:40`: your plan's limit stopped the sessions, from
  then until its reset. It comes first, because nothing worked in between.
- `ready 5/7`: of the sessions at work when the night began, how many can
  go on now: still there, not stopped by the limit, not waiting for you,
  and not full. Each one that is not ready is named, with what it needs.
  One that waits for you is in the queue below; `Enter` there takes you
  to it. One that is `out until 09:10` can work after its reset: a turn
  the limit cut off, Claude Code continues by itself; one you left idle
  waits for your next prompt. `out until 09:10 · needs a compact · past
  80% · 820k` also needs a compact after the reset.
- `69% · finishes ~79% · left alone`: a session the stoker chose not to
  compact, and why.
- `would pass 80% within the next step`: why the stoker compacted then.
- `395k → 9k`: the context before and after the compact.
- `RESUME:` the agent's own line on how to continue. Paste it, or just
  say "go on".
- `skipped: …`: what stopped the step. The session is as you left it.
- `typed, not sent: …`: the compact is in the prompt without its `Enter`.
  Send it or clear it.

A stoker run during the day ends with the same card, headed `STOKED`.
`Enter` folds it. Until then, `matchblox status` shows it too:

```text
stoker: night 23:10 → 07:00 · ready 5/7 · 7 watched · 1 was full · 1 compacted · 1 waited · 386k tokens given back
  limit out 04:12 → 06:40
  not ready: api-gw (waits for you)
  not ready: app-docs (out until 09:10)
  left alone: billing (69% · finishes ~79%)
```

Every step is also in `stoker.jsonl`, in `$XDG_STATE_HOME/matchblox`
(`~/.local/state/matchblox` by default): the exact command, what the pane
showed, the result and the RESUME line. Every card is kept beside it in
`stoker-runs.jsonl`, so the next run never erases the last one.

## Compact sooner or later

The line is the lower of `compact_at` of the window and `compact_tokens`.
To keep one percentage for every window, and the same line as Claude
Code's net:

```toml
[sessions]
compact_at = 80
compact_tokens = 0
```

## Background work

A session that ended its turn with work still running says so in its
detail: `BACKGROUND 1 agent · 2 shells · oldest 1d13h`, with each shell's
pid and command. A background shell that has run for an hour or more is a
recommendation in the queue: `Enter` jumps to its session, `x` and a typed
`y` end it. A wait loop such as `while pgrep -f "node walk.mjs"` finds its
own command line and never ends.

## End the night earlier or later

```toml
[stoker]
night_ends = "06:30"
```
