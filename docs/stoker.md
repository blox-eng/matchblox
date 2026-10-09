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

## Which sessions it compacts

A Claude Code session in tmux that is:

- idle, at or above `compact_at` (85% by default),
- not waiting for you: no question, no permission prompt,
- not left for a day or more (`cold`),
- not compacted by the stoker in the last 30 minutes.

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

**STOKED** is the first thing in the queue:

```text
 STOKED 23:10 → 07:00 · 2
 ▌ 01:33  ╿ app-feature      %1         176k → 9k
       RESUME: run the migration test, then open the PR
   03:02  ╿ app-review       %2         skipped: the pane %2 has text you typed
```

- `176k → 9k`: the context before and after the compact.
- `RESUME:` the agent's own line on how to continue. Paste it, or just
  say "go on".
- `skipped: …`: what stopped the step. The session is as you left it.
- `typed, not sent: …`: the compact is in the prompt without its `Enter`.
  Send it or clear it.

`Enter` folds STOKED. Every step is also in `stoker.jsonl`, in
`$XDG_STATE_HOME/matchblox` (`~/.local/state/matchblox` by default): the
exact command, what the pane showed, the result and the RESUME line.

## End the night earlier or later

```toml
[stoker]
night_ends = "06:30"
```
