# A builder recovers an agent, sees nothing while matchblox retries the list, then retries with ⏎

Issue: #200 (lands on PR #514). Status: in design, 2026-11-26.
Follows: mux-extract (`docs/superpowers/specs/2026-11-25-mux-extract-design.md`). This slice only changes recover after Start, when `ListStrict` returns an error.

## Who, and the moment

A builder recovers a waiting conversation (`⏎` on the waiting row, or `matchblox recover web`). The agent starts. After the usual 1.5s settle, `ListStrict` errors: permission on the socket, a killed `list-panes`, a protocol mismatch. Today they see `web not reopened: tmux did not list panes: run matchblox recover again`. Recover also records that conversation as live, so a second recover in the next minute can say `web is live` instead of listing again.

Evidence: owner, 2026-11-26 — “run matchblox recover again - can we adopt the Apple way of trying to recover ourselves before we display anything” and “silent at first then after X retries offer cta”. They picked silent retries, then a CTA, on the existing PR.

## Why it matters

A `ListStrict` error is often gone a moment later. Estimated: one extra second of silent retries saves a false “cannot”. A toast that tells them to type a command, while ⏎ on the same row already recovers, is the wrong fix.

## The experience

The builder recovers `web`. `⏎` still shows today’s `reopening web` at once. That is not this slice. After that, until recover finishes, they see no cannot notice and no new glyph.

Matchblox starts the agent, waits 1.5s as today, then lists panes. If the first list returns an error, it lists again twice, 400ms apart (three `ListStrict` in all). It still shows no cannot notice.

If any of those lists returns panes with no error, the rest of recover is today’s path: the session is alive → `web reopened`; the session is dead or missing → `web not reopened: <agent> exited at once: run cd <dir> && <cmd> to see why`, and recover still kills a dead session.

If all three lists error, then they see the CTA:

- Console strip: `web not reopened: Recover didn’t hear back · ⏎ retry`
- CLI line: `web  <agent>  cannot reopen  <dir>  Recover didn’t hear back · ⏎ retry`

The row stays waiting. ⏎ on that row, or `matchblox recover web`, confirms that start. It does not wait 1.5s again. It does not start a second session. It lists at once, then twice more 400ms apart if the list still errors.

- A later list succeeds and the session is alive → `web reopened`.
- A later list succeeds and the session is dead or missing → today’s exited-at-once Why, and recover still kills a dead session.
- All three lists error again → the same CTA.

A no-server or missing-socket result is an empty list, not a `ListStrict` error (`internal/mux/list.go`). Recover treats it as today: exited-at-once, not this CTA, no extra lists.

Ways it goes wrong (unchanged copy, no silent retry):

- The agent binary is missing, the folder is gone, git disagrees, history is missing, or another pid holds the conversation: today’s `cannot` Why, no list retry.
- Recover cannot start tmux: today’s `tmux did not start it (…): run matchblox recover web again`.

## Non-goals

- A new modal, a y-confirm, or a toast that takes a key. The CTA is ⏎ on the waiting row, which already sends recover (`internal/app/sidekeys.go`). Notices occupy the strip for 2.5s (`DESIGN.md`).
- Silent retries for every `RecoverCannot`. Only a `ListStrict` error after a start that this recover already ran, or a confirm of that start.
- Changing `recoverSettle` (1.5s) on the path that still Starts, the waiting-row glyph, or mux itself.
- Retrying `paneOfSession`’s one `ListStrict` (SignInByHand / new-account). That call stays one list.
- Treating no-server or a missing socket as this CTA.
- Herdr, desk chrome, or the #504 wrapper.

## Anti-goals (failure even if it works)

- A cannot notice or CLI cannot line appears before the three lists finish. Today’s `reopening <name>` after ⏎ may still show.
- ⏎ after the CTA starts a second tmux session for the same conversation.
- A `ListStrict` error still counts as “the agent exited at once”.
- A `ListStrict` error still records the conversation as live for a minute, so ⏎ says `web is live` and does not list.

## Surfaces

No sensitive surface touched. Checked: authentication, money, uploads, webhooks, secrets, CI, an agent's tools, personal data.

## Rulings

- 2026-11-26 — Silent `ListStrict` retries in the service, then ⏎ CTA; no toast key; retries only for a list error. — owner picked way 1.
- 2026-11-26 — Land on PR #514, not a new PR. — owner: “refine this into the existing PR”.
- 2026-11-26 — X is two extra lists after the first (three in all), 400ms apart. Extra wait is 0.8s on top of 1.5s settle. — owner, by approving this spec.
- 2026-11-26 — CTA Why is `Recover didn’t hear back · ⏎ retry`. — owner, by approving this spec.
- 2026-11-26 — Stop at the first successful list; that snapshot is the one settled uses. — owner, leftover calls on approval.
- 2026-11-26 — Confirm skips the 1.5s settle and the install/folder checks; it lists the session Start opened (`As`, including `web-2`). — owner, leftover calls on approval.
- 2026-11-26 — Preview after the CTA stays `cannot` with that Why, not `can reopen`. — owner, leftover calls on approval.
- 2026-11-26: spec approved. Decided by: the owner.

## Left to the builder

- How recover remembers an unconfirmed start so the next recover of that conversation lists and does not call Start (a map other than `reopened`, a Why marker, or another shape). Tests must show: first recover starts once; ⏎ after the CTA starts zero times; a later successful list with a live session yields `reopened`; a later successful list with a dead session yields exited-at-once.
- Whether the 400ms pause is a named constant next to `recoverSettle`.

## Done when

A person runs these from the PR worktree. Each name is a test the builder adds.

- [ ] `go test ./internal/service -run TestRecoverListGlitchThenLiveReopens -count=1`: first two `ListStrict` calls error, the third returns a live session; the row is `reopened`; Why is empty.
- [ ] `go test ./internal/service -run TestRecoverListGlitchGivesRetryCTA -count=1`: three `ListStrict` errors; Why is `Recover didn’t hear back · ⏎ retry`; State is `cannot`; Start ran once.
- [ ] `go test ./internal/service -run TestRecoverRetryAfterListCTADoesNotStartAgain -count=1`: after that CTA, a second recover of the same conversation starts zero times; three more list errors still yield the same Why.
- [ ] `go test ./internal/service -run TestRecoverRetryAfterListCTAReopensWhenListWorks -count=1`: after that CTA, a second recover lists a live session and the row is `reopened`.
- [ ] `go test ./internal/service -run TestRecoverRetryAfterListCTAExitedAtOnce -count=1`: after that CTA, a second recover lists a dead session and Why is today’s exited-at-once line.
- [ ] `go test ./internal/service -run TestRecoverMissingAgentHasNoListRetry -count=1`: missing agent binary still shows today’s install Why; `ListStrict` ran zero times.
- [ ] `go test ./cmd/matchblox -run TestRecoverPrintsListRetryCTA -count=1`: a cannot row with that Why prints `cannot reopen` and that Why, and the command fails.

## Cold read

Cold read: 2026-11-26, 1 fresh reader(s). Blocker 0, major 9, nit 2. Fixed 11, contested 0, for the person 0, declined 0. Second read: 7 left.

Second read, for the person (not edited after this read):

- Major — stop at first successful list, or always run three?
- Major — confirm skip of 1.5s has no named timing test.
- Major — confirm: skip install/folder checks, or run `recoverRow` again?
- Major — remember `As` (`web-2`) so confirm lists the session Start opened.
- Major — preview (`matchblox recover` with no names) after the CTA: still `can reopen`, or keep the CTA Why?
- Nit — “returns panes” vs empty list with no error.
- Nit — existing `TestRecoverDoesNotTreatAListFailureAsExited` still expects the old Why.
