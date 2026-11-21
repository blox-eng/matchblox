# A builder sees when matchblox cannot read a waiting agent's screen, and which agent version drew it

Issue: #199 (first slice).

## Who, and the moment

A builder runs Claude Code and Cursor sessions in tmux and answers them from the matchblox console or the phone.
Overnight, Claude Code updates itself and changes the words of its approval menu.
In the morning a session waits for a permission. Its hook says so, and the row shows that it waits.
But the menu parser in `internal/said` no longer reads the new menu, so the console offers no choices, or offers the wrong ones.
The builder does not know whether to trust the digits, and nothing says that the agent's update caused it.
Evidence: issue #199. `internal/sample/sampler.go` `readMenus` reads a hooked session's menu only from the screen. `internal/queue/queue.go` sets the waiting state from the hook alone.

## Why it matters

- A digit that the console sends to a menu it misread picks a choice the builder did not mean: a wrong approval of a tool run. One misread is one wrong action in a repository (estimated, from the parser's design: there is no guard after a version change).
- Until a matchblox release adds the new wording, every waiting session of that agent is a guess, on every host. Today the gap is the time to the next release, often a day (estimated).
- The builder cannot report it well: no command says which agent version is on the host.

## The experience

Happy path:

1. A session waits. Its hook says "permission", and the console reads the menu or the prompt on its screen. Nothing changes from today.
2. A session waits, the hook says so, and no screen rule reads a menu or a prompt on its screen for a short time. The row and the chat say `unknown screen · Claude Code 2.3.1`. The chat adds the fix: `answer it in its pane: ctrl+o opens it` (the key that goes into a pane; the chat names whatever it is bound to).
3. The console offers no digits for that session, and a digit sends nothing to it. ctrl+o opens the agent's pane, as it does today.
4. When the screen reads again, or the session stops waiting, the note goes away.
5. `matchblox agents` lists each agent on this host: its name, the version seen, the number of its sessions, and how many show `unknown screen` now.
   ```
   AGENT        VERSION   SESSIONS  UNKNOWN SCREEN
   Claude Code  2.3.1     4         1
   Cursor       2025.11   1         0
   ```
   A line under the table names the fix when a count is above zero: `a screen matchblox cannot read: update matchblox, or report it with this table`.

When it goes wrong:

- matchblox cannot read an agent's version. The row says `unknown screen · Claude Code, version not read`. `matchblox agents` shows `not read` in the VERSION column and the reason under the table. It never shows a guessed version.
- An agent has no hooks (Codex, OpenCode). Its sessions never show `unknown screen`. `matchblox agents` lists them, with `no hooks: screen only` in place of the count.
- The phone gets the same words as the console, on the row of the session.

## Non-goals

- New rules that arrive without a matchblox release. That is a later slice of #199.
- Rule override files in `~/.config/matchblox/agents/`.
- `matchblox agents explain <session>`.
- The scheduled CI job that tests the latest agent releases.
- `unknown screen` for agents with no hooks (Codex, OpenCode).
- A change to how a menu is parsed or which keys an answer sends.

## Anti-goals (failure even if it works)

- A digit reaches a session that shows `unknown screen`.
- `unknown screen` shows on a screen that matchblox reads well, for example while the agent still draws its menu after the hook fired.
- The note blinks on and off on one waiting session.
- A version that matchblox guessed, or that comes from another session, shows as fact.
- `matchblox agents` prints a home path, a host name or a user name.
- Reading a version runs an agent binary that the builder did not start, or runs it with a shell.

## Surfaces

- An agent's tools — the console types keys into an agent's pane, and a misread menu turns a digit into a wrong approval. The first anti-goal covers it: no digit reaches an unknown screen. The last anti-goal covers how a version is read.
- Checked and not touched: authentication, money, uploads, webhooks, secrets, CI, personal data (version strings only; the path anti-goal keeps home paths out).

## Rulings

- 2026-11-21 — The trigger is a disagreement: the hook says the session waits, and no screen rule reads a menu or a prompt. The ways were: (A) the hook disagrees with the screen: few false alarms and it catches the real harm, but it covers only hooked agents; (B) a tested-versions list: covers every agent but warns at each agent release; (C) a guess from the screen's shape: no version data, but the guess is a regex that breaks too. Picked A — because it speaks only when an agent waits and matchblox cannot show it. — the owner.
- 2026-11-21 — The first slice is the note, the digit block and `matchblox agents`. Remote rules, overrides and `explain` come later — because the note and the digit block alone stop the wrong approval. — the owner, with the spec's approval.
- 2026-11-21: spec approved, with the three recommendations under Calls to check. Decided by: the owner.

## Left to the builder

- How long the disagreement lasts before the note shows, and how it avoids a blink.
- Where the version comes from for each agent: the install path, the transcript, the hook payload or a `--version` call.
- How the version travels to the console and the phone, in the existing session record.
- The table's exact layout and its `--json` form.

## Calls to check

- The grace time before the note shows. Recommend 3 s, or two samples, whichever is longer — because Claude Code fires the hook before it draws the menu. Cost if wrong: too short gives false alarms; too long lets a builder press a digit on a misread menu first. The digit block holds once the note shows. Within the grace time a digit still reaches a numbered menu that the console reads and no rule does, so a longer grace time leaves that window open longer.
- Whether the digit block holds during the grace time too. Recommend yes: digits wait until a rule reads the menu — because a digit sent in that time is the same wrong approval. Cost if wrong: a builder who is very fast waits a moment longer.
- Codex's version. Recommend that it comes from its install path, not `codex --version` — because the anti-goal forbids running a binary that the builder did not start. Cost if wrong: Codex shows `not read` until a later slice.

## Done when

- On a test host, a Claude Code session waits on a menu whose words no rule knows. Within the grace time its row and chat say `unknown screen · Claude Code <its version>`, and a digit sends nothing to it.
- The same session with a menu that a rule reads shows its choices, and no note, at every sample.
- `matchblox agents` on the host shows each running agent with its version, its sessions and its unknown-screen count, and shows `not read` with a reason for an agent whose version it cannot read.
- `matchblox agents` output contains no home path, host name or user name.
- The phone shows the same note on that session's row.
