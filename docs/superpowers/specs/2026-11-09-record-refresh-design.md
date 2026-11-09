# An idle agent writes its task record before a switch (#336)

## Intent

A builder switches a match to another agent. The outgoing agent knows the
task best, but today the switch only scrapes evidence: the transcript, the
pane and Git. When the outgoing agent is idle and can still answer,
matchblox now asks it to write its task record for this switch first. The
receiving agent then starts from a record its predecessor wrote on purpose,
not from a reconstruction.

Design 0016 proposed this step and deferred it on 2026-11-08 ("needs
turn-completion detection, and its own issue"). This is that issue.

## Goals

- A switch from an idle Claude Code or Codex agent carries a task record
  that agent wrote for this switch.
- The builder sees the wait and can end it: one timer, the same on desktop,
  phone and the iOS app (the iOS app renders the console at about 44
  columns).
- A busy, waiting, usage-stopped, Cursor or OpenCode agent still switches
  with the evidence capture, and the capture says why there was no refresh.

## Non-goals

- No refresh for Cursor or OpenCode: no verified transcript gives their turn
  end. They keep today's evidence capture.
- No refresh outside a switch (no `matchblox refresh` command).
- No change to the evidence capture itself, beyond the refresh lines below.
- No new setting or config key. The default is on; esc and `--no-refresh`
  skip it.

## Anti-goals

- The switch must never hang: every path ends at the time limit.
- The refresh request must never pass as the builder's words in the next
  capture (the 0.37.0 rule: old switch prompts, peers and records never
  pass as the builder's).
- A builder prompt typed during the wait must never be lost or overridden:
  the switch refuses instead, as a turn started during the capture does
  today.

## Constraints

- The switch is one service act (`switch:<pane>`, `proto.SwitchAgent`),
  called by the console and by `matchblox switch`. The console, phone and
  iOS app all render the same Go console (`internal/app`).
- Turn end detection exists: `internal/stoker/when.go:120` fires a rest
  trigger when `x.TurnEnded.After(t) && x.Resting() && !waiting && !x.Asks
  && !x.Permits`. `TurnEnded` comes from the verified transcript, so only
  Claude Code and Codex have it.
- Typing into an idle pane exists: `typeLine` in
  `internal/service/peersend.go` (no draft overwrite, no copy mode, guarded
  keys, Enter only once the line shows). It currently checks for a `[from `
  peer tag in the prompt.
- `sample.Session.Stopped()` tells a usage-stopped session.
- Older clients and services must keep working (the proto comments already
  name this rule for `ExpectedPID` and `Account`).

## Approaches

1. **The service runs the wait (picked).** On the confirmed switch, the
   service types the refresh request, watches the turn end, publishes the
   wait in its state, then captures and switches. Every console shows the
   same timer; esc sends a "switch now" act.
2. **Each client orchestrates.** The console sends "refresh", watches the
   state, then sends "switch". Rejected: the console, the CLI and any later
   client each need their own copy of the wait, and they drift.
3. **Switch when it rests.** The act returns at once and a one-shot trigger
   switches later. Rejected by the builder: a deferred switch is more state
   to show and cancel, and the builder is at the console at that moment.
4. **A separate refresh step.** Rejected by the builder: the builder must
   remember the first step.

The builder picked waiting inside the switch (2026-11-09), and the
service-side build with the defaults below (2026-11-09).

## Decided

- **When it refreshes.** By default, when the outgoing session is idle
  (`Resting`, not waiting on the builder, no menu), not `Stopped()`, runs
  Claude Code or Codex with a verified transcript and a managed task record,
  and the switch carries a handoff (a **no task** start captures nothing,
  so it does not refresh). A busy or waiting session switched with
  `Anyway` does not refresh.
- **The request.** A fixed service prompt, typed as one line, that names
  no agent, so every past request has the same shape: a fixed lead, the
  quoted record path, a fixed tail. For example: "Update your task record
  at \"<path>\" for a handover now. Record the goal, latest direction,
  progress, decisions, verification and next action. Do no other work."
  The exact words are the builder's call within this shape; once built,
  they stay fixed, as the switch prompt's do.
- **Turn end.** The refresh ends when the session's `TurnEnded` is after the
  moment the request went and the session rests again with no menu, the
  same test as the stoker's rest trigger.
- **Time limit.** 3 minutes from the request. At the limit the switch goes
  ahead from the evidence.
- **Esc.** During the wait, esc (or the CLI's Ctrl-C) ends the wait and the
  switch goes ahead from the evidence at once. It does not cancel the
  switch. (A builder who wants no switch can still kill or let it finish;
  see "Left to the builder".)
- **The console.** The confirm stays the switch only. After y, the wait
  shows in the console's bottom rows, not inside the form, for example:

  ```
   ◐ claude writes its task record 0:42
   esc switches now
  ```

  Other consoles show `writes its record` on the row. It fits 44 columns.
- **The capture says which record it carries.** One line in the task
  record section: "written for this switch" (the record's mtime is after
  the request), or "not refreshed: <reason>", with reasons such as busy,
  waits for you, out of usage until <time>, Cursor/OpenCode give no turn
  end, the refresh timed out after 3:00, skipped with esc, skipped with
  `--no-refresh`, the request did not reach the pane (and why), it ended its turn without
  writing the record.
- **The excerpt.** Any refresh request matchblox ever sent is recognised
  by its exact shape (lead, a quoted path whose base is `handover.md`, the
  exact tail), as `earlierSwitch` recognises switch prompts in
  `internal/service/matchcontext.go`, and replaced by the mark
  `[matchblox refresh request]`. That covers a request left behind by a
  switch that timed out, was refused or failed. The refresh reply stays:
  it is the agent's own summary. (Builder-check answer, 2026-11-09.)
- **A turn that ends with no write.** When the turn ends and the record's
  mtime did not move, the switch goes ahead at once, with the reason "it
  ended its turn without writing the record". Waiting out the limit would
  only delay the same result. (Builder-check answer, 2026-11-09.)
- **The race.** If the builder's own prompt lands during the wait (a turn
  starts that the request did not start, or a prompt row after the request
  that is not the request), the switch refuses: "<name> took your prompt
  while it wrote its record: switch again". The session PID and start
  checks after the capture stay.
- **Other input during the wait.** The pane is marked as switching for the
  whole wait. Peer sends to it refuse at once: "<name> switches agents now:
  send again after". The stoker's steps and its rest trigger skip that pane.
  These refuse fast. They never wait on the pane's mutex, so nothing hangs
  for 3 minutes. A second switch or a kill of the pane refuses with
  "switching already". Only the builder's own typing in the pane can still
  race, and that refuses the switch as above. (The builder chose this on
  2026-11-09.)
- **The command line.** `matchblox switch` refreshes too, and prints the
  wait as it goes. `--no-refresh` skips it.
- **Protocol.** `proto.SwitchAgent` gains `NoRefresh bool`
  (`json:"no_refresh,omitempty"`). An older client omits it and gets the
  refresh. An older service ignores it and never refreshes.

## Left to the builder

- How the service publishes the wait (a field on the published state keyed
  by pane, with the start time and the limit) and how the console renders
  it from that state.
- How "switch now" reaches the running act (a second act on the same pane,
  or a cancel of the first act's context that the switch reads as "skip").
- Whether `typeLine` gains a parameter for the prompt check, or the refresh
  gets its own small typing path. The guards (no draft, no copy mode,
  guarded keys, Enter once shown) must hold either way.
- Where the switching mark lives, and how each guard reads it: peer
  delivery, the stoker's steps and rest trigger, switch, and kill.
- The order of the wait relative to `holdAccount`, and that `--preview`
  never refreshes.
- How an older console, with no wait view, behaves while a newer service
  waits: it gets the result up to 3 minutes later. Check that no client
  timeout cuts the act short, and say so in the changelog if one does.
- Tests: each "not refreshed" reason; the turn end; the limit; esc; the race
  refusal; the exact-match excerpt drop; `--no-refresh`; the 44-column view.
