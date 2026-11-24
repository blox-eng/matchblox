# A builder sees which screen rule read a waiting session, and posts the screen in one step

Issue: #199 (third slice).

## Who, and the moment

A builder answers their agents from the matchblox console.
A Claude Code session waits for a permission, and its row says `unknown screen · Claude Code 2.1.331`.
Or a Cursor session waits and the console shows no digits, though the menu is plain to see in the pane.
The builder runs `matchblox agents`. The table says one screen is unknown, and its fix line says `update matchblox, or report it with this table`.
But the table holds no screen, and no one can tell which rule missed: the approval prompt, the working line, the menu key hint or the question.
To report it, the builder copies the pane by hand, removes their home path and host name, and guesses what matters.
Evidence: issue #199 asks for `matchblox agents explain <session>`. `cmd/matchblox/agents.go` prints the table and its fix line. `internal/service/unread.go` decides `unknown screen` from the hook and `said.Menu`. The daily canary (#489) covers Claude Code, Codex and OpenCode in CI only: a Cursor break, or a break that only one machine draws, still needs the builder's report.

## Why it matters

- A report without the screen cannot become a fixture in `testdata/menus`. The fix waits for a second round trip with the builder, often a day (estimated).
- A screen copied by hand can carry a home path, a host name or a user name into a public issue. matchblox keeps those out of every public text.
- When the console reads a menu wrong, nobody can see which rule matched. A wrong digit is a wrong approval, and the builder cannot tell if it can happen again.

## The experience

Happy path:

1. The builder runs `matchblox agents explain api`, with a session name, a pane or a target, as `peek` takes them.
2. matchblox prints what the service decided, and why:
   ```
   Session  api · Claude Code 2.1.331
   Hook     waits for a permission, for 14s
   Verdict  unknown screen: no rule reads a menu
   Rules    built into matchblox v0.58.0

   Rules on the last 8 lines with words
     working line        no match
     approval prompt     no match
     menu key hint       line 8  Esc to cancel · Tab to amend
     question            line 2  Do you want to make this edit?
   Menu     not read

   Screen (home path, host and user removed; check it before you post it)
     │ Do you want to make this edit?
     │ ❯ 1. Accept
     │   2. Accept, and don't ask again this session
     │   3. Decline
     ...

   Fix: report it at github.com/blox-eng/matchblox/issues with this output
   ```
3. On a screen the rules read, the same command shows the menu with its digits, so the builder sees what a digit would pick: `Menu  1 Yes · 2 Yes, and don't ask again · 3 No`. It prints no `Fix:` line.
4. `matchblox agents explain --file screen.txt` explains a saved screen. It shows no hook line and no session line, because a file has neither.
5. `--json` prints the same facts for scripts and for the canary's report.
6. The `agents` table's fix line becomes `a screen matchblox cannot read: run matchblox agents explain <session>, and report its output`.

When it goes wrong:

- The session is not found. The words: `no session api: matchblox sessions lists them`. Exit 1.
- The service does not run. The same words and exit code as `peek` give today.
- The screen changed since the row's note. explain shows the note the row shows now, and the screen it reads now. If the two disagree, a line says so: `the screen changed since the note: run it again`.
- The session's agent has no hooks (Codex, OpenCode). The hook line says `no hooks: screen only`. The rules still run, and their results still show.
- No rule matches and the hook does not wait. The verdict names the fallback: `none: the last line with words`, with that line.
- The file for `--file` cannot be read. The words name the path and the reason. Exit 1.

## Non-goals

- Rules that arrive without a matchblox release. That is a later slice.
- Local rule overrides.
- A rules-version column in the `agents` table. The version shows in explain only.
- A console or phone view of explain. The console's own words stay `unknown screen`.
- Posting the report for the builder. The builder reads the output and posts it.
- Explain for every session at once.

## Anti-goals (failure even if it works)

- explain types a key, or sends any text, into the pane.
- The output carries the builder's home path, host name or user name.
- explain's verdict disagrees with the row's note, and nothing says so.
- explain names a rule that did not decide the verdict, or invents a match.
- The screen in the output shows more than the lines the rules read, plus the menu, so an older secret from the scrollback travels into an issue.

## Surfaces

- An agent's tools: explain reads the pane that matchblox may type into, and must only read. Its anti-goal is "explain types a key".
- Personal data: the screen holds home paths, host and user names. Its anti-goal is "the output carries the builder's home path, host name or user name".
- Secrets: a pane can show a token or a key. explain removes no secret it cannot recognize. It shows only the lines the rules read, and it tells the builder to check the screen before they post it. Its anti-goal is "an older secret from the scrollback travels into an issue".

## Rulings

- 2026-11-23 — Ways shown: (A) `matchblox agents explain <who>`, answered by the service from what it saw; (B) a `--why` flag on `peek`, which reads the pane again and can disagree with the note; (C) answer Claude Code approvals through its PermissionRequest hook, so no digit is typed into a menu. Pick: A. — because it is small, it helps with the canary's first issue, and it explains what the service saw. C is a bigger change to how the console answers, and needs its own spec. — the owner.
- 2026-11-23 — The rules version shows in explain only, not as a column of `agents`. — because the rules are built in, so a column would repeat `matchblox version` until rules ship as data. — the owner, with the pick of A.
- 2026-11-23 — explain shows only the lines the rules read, plus the menu. — because the scrollback can hold older secrets, and the rules never look there. — the owner, with the spec's approval.

- 2026-11-23: spec approved. Decided by: the owner.

## Left to the builder

- How the CLI asks the service: a new message kind, or the one `peek` uses.
- The names of each rule in the output, if they are plain words a builder can read.
- How the output removes the home path, the host name and the user name. The canary's `scrub` in `internal/said` is the model.
- The JSON field names.

## Calls to check

- Should explain also take `--agent`, to read a file as one agent's screen? Recommend no — because matchblox's rules are not per agent today. Cost if wrong: a flag to add when rules become per-agent data.
- Should the service explain its last sample, or capture the pane again? Recommend a new capture, plus the row's note as it stands — because the sample may be seconds old, and the disagreement line covers the gap. Cost if wrong: a rare `the screen changed` line.

## Done when

- On a session whose row shows `unknown screen`, `matchblox agents explain <session>` prints the verdict `unknown screen`, the hook's wait, each rule with its line or `no match`, the rules version, and the screen. The screen contains no home path, host name or user name.
- On a session waiting at a menu the rules read, explain prints the menu with its digits, and the digits match what `matchblox send <who> --pick n` would choose.
- `matchblox agents explain --file testdata/menus/claude-bash-2.1.330.txt` prints the menu with its digits and no hook line.
- After explain runs, the pane holds the same text as before, and no key reached it.
- The `agents` table's fix line names `matchblox agents explain`.
