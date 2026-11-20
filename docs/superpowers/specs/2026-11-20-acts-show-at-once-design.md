# A builder sees a kill, a rename or a park land in the row the moment they confirm it

Issues: #211 (acts show at once), #450 (one toast form).

## Who, and the moment

A builder at the console, with several sessions in the list. They select a session, press `x` and type `y` to kill it. Or they rename it in the form. The row stays as it was until the next sample: up to 2 s (the default `interval`), plus the time the agent needs to exit after its pane goes. They wonder if the act worked, and they press `x` again.

Evidence: issue #211; `internal/config/config.go` (interval 2 s); the act path in `internal/app/model.go` (`m.flash = "sent: …"`, then nothing until a sample).

## Why it matters

Kill and rename are the console's most common acts on a row. Each one leaves 2 s or more of doubt (estimated: one sample interval plus the agent's exit). A second `x` on a session that is already going asks for a second confirm and can kill the wrong row if the list moves.

## The experience

- **Kill.** After the typed `y`, the row turns faint at once and reads `killing`. The cursor skips it. The row goes when a sample no longer has that session.
  - The service answers with an error: the row comes back as it was, and the console says `kill failed: <the service's reason>`. `x` kills again.
  - Three samples still have the session: the row comes back, and the console says `<name> is still running after the kill. Press x to kill it again.`
- **Rename.** After the form closes, the row shows the new name at once, faint until a sample shows it.
  - The service answers with an error: the row shows the old name again, the console says `rename failed: <the service's reason>`, and the open form keeps the reason by the typed name.
  - Three samples still show the old name: the row shows the old name, and the console says the same words with "the rename did not take".
- **Park.** After `z`, the row moves to HELD at once, faint, with `parking`. A second `z` on a parked row lets it go the same way, with `letting go`.
  - The service answers with an error: the row goes back to its group, and the console says `park failed: <the service's reason>`.
  - Three samples still show the old hold: the row goes back, with "the park did not take".
- **HELD is the last group** in the desktop list: NEEDS YOU, WORKING, IDLE, HELD. A parked session is one the builder put away on purpose, so it sits below the idle ones.
- Nothing changes for a lost connection: the act already says `not sent, no connection`, and it gets no pending state.
- Another console or phone sees the act only when its sample shows it.

## One place the console tells you things (#450)

Today the console has three forms for one idea: `flash` (the key line, about 245 places set it), `toast` (the header, `m.say`) and `notice` (the board's notice row, which also carries standing states from `stateNotice`). A ship read error such as `blox-master: not read: <repo>: <error>` stays on the key line for as long as the read fails. It looks like a crash, takes the row the keys use, and names no fix.

- One form: a toast stack in a corner, over the list. It never takes the key line.
- An event (`sent: kill api`, a pending act that failed) shows as a toast and goes after a moment. An error toast stays longer than an event toast.
- A standing error (GitHub not read, no service, old data) shows once as a toast with its fix. Then it stays as one small mark, `⚠ <n>`, until it is fixed. A key or a tap on the mark shows the full text of each.
- Success is quiet. Each error names its fix.

## Non-goals

- No change to which acts ask for a typed `y`.
- No change to the service, the snapshot or the wire (`proto`).
- No pending state for acts with no predictable result: answers, picks, the stoker's steps, doors.
- No sound and no desktop alert (#97 owns that).
- No change to what the service reports as errors.
- No pending state in the native Pocket app views (Swift). Pocket's console is the same Go console, so it gets the change.

## Anti-goals

- A standing error goes away from the screen while it still holds.
- A toast covers the row the cursor is on, or the key line.
- A failed kill or rename looks like it worked. The pending state always ends in the sample's truth or a visible error.
- A new session that reuses the pid of a killed one shows as `killing`.
- A pending row stays faint after the session is truly gone, or after the console reconnects.

## Surfaces

No sensitive surface touched. Checked: authentication, money, uploads, webhooks, secrets, CI, an agent's tools, personal data. The kill act and its typed `y` stay as they are; only the console's drawing changes.

## Rulings

- 2026-10-24: the pending state lives in the console only, keyed by the act id and by the session's pid and start time (`sample.Session.PID`, `Start`). The service and the snapshot do not change. Reason: each device shows only its own acts, and a reused pid never inherits a state. Decided by: the owner (issue #211).
- 2026-10-24: a pending kill times out after 3 samples. Reason: the agent can take a sample or two to exit after its pane goes. Decided by: the owner (issue #211, proposal).
- 2026-11-20: scope is kill, rename and park, and HELD moves to the end of the desktop list. Reason: the owner wants park too; a park moves the row into HELD. Decided by: the owner.
- 2026-11-20: the console's three message forms and the standing "not read:" line become one toast form (#450), in the same PR as #211. Reason: the owner asked for it in this PR; the pending acts report their errors through it. Decided by: the owner.
- 2026-11-20: way A, the usual optimistic-UI practice: the client shows the expected result at once, and the authoritative state (the sample) confirms or reverts it. Decided by: the owner ("whatever is the best practice").
  - **A. Console-side pending acts, drawn over the rows** (picked). Pro: no wire change, each device sees only its own acts. Con: every place that draws a session row must read the pending state.
  - **B. The service marks the session `dying` in the next snapshot.** Pro: every device sees it. Con: breaks the ruling above, and changes the snapshot and the service.
  - **C. Ask for a sample at once after each act's result.** Pro: tiny. Con: the kill still lags while the agent exits, and there is no `killing` mark.

- 2026-11-20: spec approved. Decided by: the owner.
- 2026-11-20: a "sample" is a new snapshot time of the act's host; a snapshot sent again does not count. A refused rename says `rename failed: <reason>` as an error toast, and its open form keeps the reason by the typed name. Decided by: the owner, after review.

## Left to the builder

- Where the pending state lives in `Model`, and how `jumpAfter`/`startAfter` share it.
- How the rows (list, the beside panel) read it, and the exact faint style.
- How "3 samples" is counted.
- The toast's corner, width, life and the key that opens the `⚠` mark, within the rules above.
- Whether the 245 `flash` writes move in one go or by a shim that routes `flash` into the toast.

## Calls to check

- **The old Sessions list (`groupNames`) already ends in HELD.** Recommendation: change only the desktop list's order. Cost if wrong: none; both then end in HELD.

## Done when

- In the console, confirm a kill: the row reads `killing` and is faint before any new sample, and the cursor skips it. After a sample without the session, the row is gone.
- Make the service refuse a kill (a test service that answers `Err`): the row comes back with `kill failed: <reason>`.
- Keep the session alive through 3 samples after a kill: the row comes back with the "still running" words.
- Rename a session: the new name shows before any new sample. After an `Err` result, the old name is back with `rename failed: <reason>`.
- Park a session: it shows in HELD, faint with `parking`, before any new sample; an `Err` result puts it back with `park failed: <reason>`.
- Open the console on the desktop with one session of each state: the groups read NEEDS YOU, WORKING, IDLE, HELD, top to bottom.
- A ship read error shows one toast with its fix, then a `⚠ 1` mark; the key line keeps its keys. The mark goes when the read works again.
- Grep finds one function that shows a message to the person: no `m.flash =`, `m.toast =` or `notice(` write outside it.
- A new session that reuses the killed session's pid, with a different start time, is not drawn as `killing`.
