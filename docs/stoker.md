# The stoker

You leave your agents at work: for the night, for lunch, for a meeting.
While you are away, some sessions fill their context and stop. The stoker
keeps them lit. It compacts each full session and asks the agent for one
line that says how to continue. When you are back, the console shows what
the stoker did, then the queue.

The stoker uses no model of its own and answers nothing. It sends one
command, and only when the host says the session is idle.

## Turn it on

| Key | What it does | The header shows |
|---|---|---|
| `f` | Turns the stoker on, until you turn it off. `f` again turns it off. | `stoker` |
| `n` | Starts a night: the stoker runs until `night_ends` (07:00 by default), then turns itself off. `n` again ends the night now. | `night → 07:00` |

The stoker runs in the service, not in the console. Close the console and
the stoker keeps working. It also keeps its state when the service restarts.

The first time, the queue shows **Try a night** under SET UP. `Enter`
there starts a night. The step folds after the first run.

## What it does

While the stoker is on, the service checks each session after each sample.
A session gets a compact when all of these are true:

- It is a Claude Code session in a tmux pane.
- It is idle, and it was idle for less than 24 hours. A cold session gets
  nothing: nobody comes back to it, so a compact would only spend tokens.
- Its context is at or above `compact_at` (85% by default).
- It does not wait for you: no question and no permission prompt.
- The stoker did not send it a step in the last 30 minutes.

Right before the send, the service checks again that the pane is idle. Then
it types this into the pane, with `Enter`:

```text
/compact Keep the task, the branch and last commit, the files you changed, what to read first, every open decision and the next step. End with one line that starts with RESUME: and says how to continue.
```

A run sends at most 30 steps.

## What it never does

- It never answers a question or a permission prompt. A session that waits
  stays in the queue for you.
- It never sends "continue". The session stays where the compact left it.
- It never ends, kills or removes anything. Those wait for you, as in the
  day.

## When you are back

The console shows **STOKED** above the queue, with the time of the run:

```text
 STOKED 23:10 → 07:00 · 2
 ▌ 01:33  app-feature      %1         176k → 9k
       RESUME: run the migration test, then open the PR
   03:02  app-review       %2         skipped: session busy
```

Each row is one step: the time, the session, its pane, and the result. A
compact shows the context before and after it, then the RESUME line the
agent wrote. "skipped: session busy" means the session started to work
right before the send, so the stoker sent nothing.

`Enter` on STOKED folds it. The next run shows again.

Each step also goes to `~/.local/state/matchblox/stoker.jsonl`: the time,
the session, the rule, the exact command, what the host said before the
send, the result, and the RESUME line.

## What it costs

The stoker itself costs nothing: it reads files and tmux on your machine.
Each compact costs what a `/compact` you type costs: the agent's model
reads the context once and writes a summary. A session that you continue
in the morning compacts at its limit anyway, so the stoker only does it
earlier. Cold sessions are left alone for this reason.

## Set the end of a night

```toml
[stoker]
night_ends = "07:00"   # n runs the stoker until this time
```

## Agents

The stoker compacts Claude Code only. Codex and OpenCode have a `/compact`,
but it takes no instruction, so it cannot ask for the RESUME line. Their
sessions stay as they are and show in the queue as usual.
