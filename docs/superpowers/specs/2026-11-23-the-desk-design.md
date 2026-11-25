# A builder at a desk sees which repo needs them, what finished, and what the machine does, and reviews an agent's change without leaving the console

Builds on the sidebar console (#411, spec `2026-11-14-sidebar-console-design.md`). Where the two differ on a desk, this spec wins. The phone keeps the sidebar console as it is.
Mockup: the owner's artifact "The desk" (https://claude.ai/artifact/2DonTkiHhk5uPQMVYyssu5, version 10 (v9 approved 2026-11-23; v10 adds design review D3–D5), drawn cell by cell from the replay's glyphs and inks; approved by the owner on 2026-11-23). A copy goes to `design/0022-the-desk.html` when the spec is approved. Where this spec and the mockup differ, this spec wins.

## Who, and the moment

The owner, at a 16-inch desk, with 8 to 15 agents in tmux across 3 to 5 repos and 2 or more hosts. They come back after 20 minutes away. They want to know, in one look: which repo needs them, which agent finished and waits for a review, and whether a host or a plan is in trouble. Then they read what an agent changed and tell it what to fix.
Evidence: the owner's request (2026-11-23), after a head-to-head with two other agent consoles. Both show a per-agent timer and a done state. One puts the machine and usage in a footer, a shell beside the agent, and diff comments that go back to the agent.

## Why it matters

- A finished turn sits in IDLE beside sessions that went quiet hours ago. The owner opens each one to find what finished (estimated: 3 to 6 opens per return).
- No row says how long it has been in its state, so the oldest wait is not obvious (observed: only the wait shows a time).
- The machine and usage live behind `⇥`. A plan near its limit or a busy loop shows only when the owner goes there (the strip that showed them went with the board on 2026-11-14).
- To run the tests of an agent's work, the owner leaves the console (`⌃o`, a split, `ctrl+a m` back).
- To review an agent's change, the owner opens GitHub or a second terminal, then types file and line by hand into the chat.

## The experience

### The screen (desk)

Top to bottom, all drawn by matchblox: the header, a rule, the body (the list at the left, the agent's real screen at the right), a rule, the bar. The agent's screen is its own tmux window, shown through a terminal built into matchblox. The key line goes. matchblox draws the edge between the list and the agent with today's half colouring: the top half in `accent` when the list has the keys, the bottom half when the agent has them.

### The header: the work

One line. Left: the mark and `matchblox`. Then the spaces. At the right: `● N need you · ✓ N done`, then `⚙`.

- **A space** is a repo with at least one live session: `all · matchblox ●2 · base · ledge ●1`. `●N` counts the sessions that need the owner in that repo. `all` has no count, because `● N need you` already says it (one owner per fact).
- A tap on a space, or `[` and `]`, filters the list to that repo. The open space is in the accent. `all` shows every repo.
- With one repo only, no space shows.
- Spaces that do not fit end in `+3 ▾`. A tap opens the rest as a menu.
- A session outside any git checkout goes in a space named for its folder.
- The space filter and the host filter (`s`) combine. The list's empty state says which filter hides what: `Nothing in base on gpu-1. esc shows all.`

### The list: clearer states

Groups, in order: `NEEDS YOU`, `DONE`, `WORKING`, `IDLE`, `HELD`. HELD stays last, as the 2026-11-20 plan "acts show at once" ruled. An empty group takes no row (as today).

- **DONE** holds a session that ended its turn after the owner last opened its chat. Its second line is what the agent said last (`said Footer chips are in, 14 tests pass`). When the owner opens its chat, it moves to IDLE. `✓ N done` in the header counts these sessions.
- **Time in state** is on every row, at the right, before the agent's mark, as the wait is today: `10m ✻`. The group names the state, so the row says no state word (one owner per fact). `now` means under a minute. A row with a PR badge shows the badge, then the time. In NEEDS YOU and DONE the oldest comes first. The order inside the other groups does not change while the states stay the same (as today).
- **Chains stay whole.** A chain is one unit in the group of its most urgent member, by the group order: needs you, then done, then works, then idle, then held. A member that finished shows its done mark under the chain head and never leaves the chain. Each group's count counts only the sessions in that state.
- **The done mark** is `✓` in `match-ember`, still (design review D3, below). Each state has its own glyph: works `✦`, asks `╿` (breathing `match-head`), done `✓`, idle `╵`, held `╽`, spent `│`. The group head carries the word. §4 changes first, by its rule 7.

### The bar: the mode and the machine

One line, at the bottom, under a rule. It replaces the Machine tab as the place where trouble shows, and the key line as the place that says where the keys go.

- **Far left, the mode:** one uppercase word that names where the keys go, styled like the open tab today: `LIST` in `muted`; `AGENT`, `REVIEW`, `COMMENT`, `SHELL`, `MACHINE`, `KEYS` in `accent` bold. No box. The half of the edge on the side of the pane with the keys stays in `accent`, as today.
- **The canon, in cells:** the mockup's table "The canon, in cells" is the rule for gutter, spacing, separators, lines, corners, focus and inks. Only a sheet that floats (`?`, ⚙, the review) has corners: a rounded border in `faint`, the title in its top edge, 1 cell of padding inside.
- **Far right, `? keys`.** When the agent or the shell has the keys, `ctrl+q list` shows before it, as the way out.
- **Left, the hosts:** each host in view as a chip: `● ws-1 cpu 34% mem 61%`. `s▸` at the end steps the hosts, as today (it moves here from the header). When the chips do not fit, they fold to `3 hosts s▸`. A host with trouble always keeps its own chip.
- **Middle, the usage:** each account's 5-hour and weekly window, with the time to each reset: `✻ work 5h 62% ↻2h48m · wk 40% ↻4d`.
- **Right, the stoker:** `stoker off`, `≋ stoker`, `☾ night → 07:00` or `stoker rests`, in today's inks.
- **Trouble** takes colour on its own chip only: `⚠ busy loop` (neg), `not reached · since 2m` (warn), a plan within 10% of its limit (warn). Nothing else in the footer has colour.
- A tap on a chip opens that part of the cog's sheet.
- A notice shows at the foot of the list for 2.5 s, as today.

### Keys: one sheet for help and actions

The terminal keeps ⌘ for itself, so ⌘K and ⌘, never reach the console. Each desktop habit maps to a key the terminal passes on.

- **`?`** opens the keys sheet over the body: every key, grouped by the place it works (anywhere, the list, a review, the shell). Typing filters it, `↑↓` chooses, and `⏎` runs the chosen action. `esc` clears the filter, then closes the sheet. It is the help and the command palette in one.
- **`,`** opens ⚙, as ⌘, opens settings in a desktop app. On the desk `⇥` no longer opens Machine; on the phone it still does.
- **A mode's own keys** show in its own pane, never in the bar: the review's last line says `1 comment · c comment · ] [ file · ⏎ send review to ux-pass`.
- **`esc`** backs out one step, always: a comment, then the review, then a filter.
- When the agent's pane or the shell has the keys, `?` and `,` type there. matchblox takes no key the agent needs. `ctrl+q` gives the keys back to the list.
- With no match: `No key does "xyz". esc clears it.`

### The cog: Machine as settings

Machine is no longer a tab. `⚙` in the header, `,`, or a tap on a bar chip opens it as a sheet over the chat: usage, machine health, detached busy loops, the stoker, git worktrees that are safe to remove, history, tmux panes, hosts. Its content does not change. `esc` closes it and returns to the same chat and the same row. `⚙` shows `!` in neg while a detached busy loop burns (it moves here from the Machine tab).

### The shell under the agent

- `t` in the list opens a shell under the agent's pane, in the session's own folder and worktree. It is a real tmux pane in the agent's own tmux window. It lives on after matchblox quits, after an ssh drop and after `⌃o`.
- `t` again moves the keys between the agent and the shell. The divider says `shell · ~/dev/matchblox/wt/ux-pass · t switches`.
- The chat's head shows `▣ shell` while a shell is open, and the row shows `▣` before the agent's mark. With no shell, neither shows.
- The shell closes when its shell exits (`exit`, `ctrl+d`). More splits: tmux's own keys after `⌃o`.
- Fails: a session on a host that is not reached: `not opened: gpu-1 is not reached`. A session with no folder: `not opened: ux-pass has no folder`.

### Review an agent's change

- `D` in the list shows the session's diff in the chat's place: its worktree, committed and uncommitted, against the branch it started from. The head line: `ux-pass · diff against main · 4 files +212 −38   esc chat`. The files are on one line under it. `]` and `[` move between files.
- On a line, `c` writes a comment. A comment shows under its line with `✎`. Comments collect, like a review on GitHub.
- `⏎` sends the review to the agent as one message: each comment with its `file:line` and the line it is on. The review's last line says `3 comments · ⏎ send review to ux-pass`.
- After the send: `sent a review of 3 comments to ux-pass · now in WORKING`, and the chat shows again.
- `esc` goes back to the chat. Unsent comments stay with the session until they are sent or cleared, and the chat's head says `✎ 3 unsent · D reviews`.
- To a busy agent, the review waits for the end of its turn, as a message does today.
- Fails: no worktree: `No changes to review: ux-pass has no git folder.` No changes: `ux-pass has no changes against main.` A base branch that is gone: `main is gone on ws-1. The diff shows against origin/main.` A diff too large to draw: the file list shows, and each file opens on its own.

## Non-goals

- No change to the phone's screen, Pocket's belt or `matchblox sessions --json`, `peek` and `send`.
- No editor, file tree, embedded browser, phone emulator or computer use. Each one turns matchblox into an IDE.
- No second shell, no tabs inside a session, no layout of free splits.
- No review comments posted to GitHub. The review goes to the agent only.
- No settings page beyond what Machine shows today.
- No new agent integration, and no parser of an agent's chat. DONE comes from the states matchblox already reads.

## Anti-goals (failure even if it works)

- A wait is not on the screen, or not counted, while the owner looks at the desk: a space filter or a host filter hides a wait, and the header's count does not show it.
- A chain draws in two groups, or a member draws outside its chain.
- A session leaves DONE without the owner opening its chat, or stays in DONE after they open it.
- A review, or a comment, reaches a session other than the one its head names.
- The diff draws an escape sequence from a file's content, and it moves the cursor, changes the title or writes to the clipboard.
- The footer flashes or changes colour while nothing is wrong.
- The shell opens in a folder other than the session's own.
- A key does nothing, with no word why.
- Typing in the agent through matchblox feels slower than in the agent's own pane: a key's echo takes more than 16 ms at the 95th percentile.
- The agent's screen inside matchblox differs from the same screen in its own pane: a colour, a wide character, the cursor or a menu is wrong.
- The owner's terminal selection copies the list and the agent's screen together, with no way to copy only the agent's text.

## Surfaces

- **An agent's tools:** a review goes into an agent's prompt with lines of repo code. Anti-goal: the review message carries a control character, an escape sequence or text past the paste cap. It goes through the same act as a paste (`paste:<pane>`), with the same strip and cap.
- **Personal data:** the footer shows host names and account names. Anti-goal: a real host, user or account name reaches the replay frames, the mockup or `www/`. The replay and the mockup use the demo's names only.
- **An agent's tools, the other way:** the agent's output now runs through matchblox's terminal. Anti-goal: an agent's output (or a file it prints) sets the owner's terminal title, writes the owner's clipboard (OSC 52), or sends a query whose answer types into the agent. The built-in terminal keeps every such sequence inside its own screen.
- Checked and not touched: authentication, money, uploads, webhooks, secrets, CI.

## Rulings

- 2026-11-23: one spec and one mockup for all six parts (header, footer, cog, states, shell, review). Ways: one spec for the frame first; all six; states first. Picked: all six. Decided by: the owner.
- 2026-11-23: the header holds the repo spaces. Ways: repo spaces; open chat tabs; hosts as tabs. Picked: repo spaces. Decided by: the owner.
- 2026-11-23: the header is the work, the footer is the machine; the hosts move to the footer; `all` carries no count; `⚙` sits at the far right and opens Machine as a sheet. Reason: navigation at the top, status at the bottom, one owner per fact (the owner asked "what would Apple do?"). Proposed by the builder, accepted with the footer's pick.
- 2026-11-23: the footer shows the hosts, their health, the usage with each reset time, and the stoker. This reverses the 2026-11-14 ruling that removed the strip, on the desk only. Ways: hosts + health + usage; health + usage only; everything Machine has. Picked: hosts + health + usage. Decided by: the owner. The reset time is borrowed; unlabeled numbers are not.
- 2026-11-23: a DONE group under NEEDS YOU, and time in state on every row. Ways: a DONE group; an unread dot in IDLE; done counts as needs you. Picked: a DONE group. Decided by: the owner, with "do not break the chains".
- 2026-11-23: one shell under the agent. Ways: a shell under the agent; tabs inside the session; free splits. Picked: a shell under the agent. Decided by: the owner.
- 2026-11-23: a review collects comments and sends once. Ways: review then one send; each comment at once; a read-only diff. Picked: review then one send. Decided by: the owner.
- 2026-11-23: one bar at the bottom, not a footer and a key line; `?` opens one sheet that is help and command palette; `,` opens ⚙; a mode badge and an accent rule say where the keys go; a mode's keys sit in its pane. Reason: two bottom rows break Nielsen 8, and an always-on key line repeats what a learned owner knows; ⌘ never reaches a terminal app. Asked by the owner ("is that really Nielsen?"), mapped by the builder from vim, tmux, helix, lazygit and k9s. The table "The rules this desk follows" in the mockup is the record.
- 2026-11-23: before the PR merges, one mandatory pass brings every surface to the new desk: the landing page (`www/`: words, stills, the desk and phone replays), `README.md` and the docs, and Matchblox Pocket (the belt's keys, the App Store text and screenshots). The PR does not merge until that pass is done. Decided by: the owner.
- 2026-11-23: spec approved. Decided by: the owner.
- 2026-11-23: the mockup (version 9) is approved, with its table "The canon, in cells". Lines join where they meet (`┬ ┴ ┤`), and a joint takes the ink of the half of the edge it touches. Decided by: the owner ("looks good").
- 2026-11-23: `?`, ⚙ and the review are sheets that float over the agent's screen, drawn by matchblox. Decided by: the owner (with the wrapper ruling below).
- 2026-11-23: matchblox is the wrapper: it draws the whole screen (the header on top, the bar at the bottom, the list, the half edge, the sheets), and shows the agent's real screen through a terminal built into matchblox, fed by the nested tmux client the view runs today (`internal/remote/view.go`, `ViewArgv`). The owner loves today's half edge, and a bottom line closes the screen as today. Ways: tmux status lines at the bottom; a wrapper; status lines now and a wrapper spike later. Picked: the wrapper, now. It replaces the two rulings of the same day for frame panes and for status lines, and the 2026-11-16 ruling that the right side is a tmux pane beside the list (the agent's screen stays the real one, not a drawn chat). Decided by: the owner.
- 2026-11-23: notices keep their place at the foot of the list pane, as today, because nothing can float over the agent's own pane. Proposed by the builder from the ruling above, not confirmed.
- 2026-11-23: each state has its own glyph, so no state rests on colour or motion alone (DESIGN.md §4 rule 1). Done is `✓` in `match-ember`; idle `╵`, held `╽`. The ways: a glyph per state; keep `╿` and add a word in chains; keep as mocked. Decided by: the owner (design review D3).
- 2026-11-23: a session counts as seen when its agent gets the keys (`⏎`, a click, `ctrl+q`, `⌃o`), never when a choice only shows it, because on the wrapper desk `↑↓` shows each agent and would empty DONE. The ways: when it gets the keys; after a dwell; an explicit key. Decided by: the owner (design review D5).
- 2026-11-23: on a switch the keys go to LIST and the agent region says `opening <name>…` until the new agent's first frame, so no key reaches the wrong agent. With no sessions or an unreached host, the region says why in words. The ways: words and keys to LIST; the last frame dimmed; blank until ready. Decided by: the owner (design review D6).
- 2026-11-23: the wrapper stays, confirmed after the hybrid (today's split, the bar as a tmux status line) was shown; the multiplexing of today (each screen's own view of one agent, `⌃o`, `ctrl+a m`) stays as it is. Decided by: the owner (design review D9a).
- 2026-11-23: on an upgraded server, the first desk start closes the old beside viewer pane and unbinds tmux's `C-q`, and says so once at the list foot. The agent's own session is never touched. Decided by: the owner (design review D9).
- 2026-11-23: on the desk Machine simply moves to `,`; `⇥` there is unbound and shows no hint. This is the one exception to the anti-goal "a key does nothing with no word why". The ways: a hint on every press; a hint 3 times; `⇥` as an alias. Decided by: the owner (design review D10, "just move it").
- 2026-11-23: the wrapper frame draws at every width wider than the phone's layout. The list is 40 columns from 140 up, 32 from 100 to 139, 28 below; the agent takes the rest. The ways: from 100 columns; from 120; at any desk width. Decided by: the owner (design review D16). The builder notes the risk: below about 90 columns the agent region is under 60 columns, narrow for Claude Code's menus.
- 2026-11-23: a mockup of the whole desk first, then one PR. Ways: mockup then four PRs; four PRs with no mockup; one PR. Picked: mockup, then one PR. Decided by: the owner.
- 2026-11-24: the console draws at 120 FPS, so a typed key echoes in 16 ms or less at p95 through the renderer (measured 9.5 ms). The latency check runs in normal builds; the race build skips it. The ways: 120 FPS; stay at 60 and loosen the bar; the hybrid. Decided by: the owner.
- 2026-11-24: a prompt that waits for a key takes the bar: the mode word stays first, then the prompt and its keys in place of the chips (`CONFIRM  kill flint-auth?  y kill · esc keep`; a confirm's mode word is `CONFIRM` in accent bold), and the chips return once it is answered. This covers the confirms (kill, merge, door steps), the room's typing line, `esc R start again`, the copy check's way out, a menu's keys and `ctrl+o`'s way back. Notices stay passive at the list's foot. The ways: the bar gives way; prompts at the list's foot; a key row while something waits. Decided by: the owner.
- 2026-11-24: the header's marks keep a home on the desk: a standing `⚠` error and the old-data mark sit in the head row's right side before `⚙`, in warn ink, and a tap on `⚠` says the error again; the stoker count joins the bar's stoker chip (`stoker on ≋ 2`); the names of who waits follow `● N need you` in the head row while the list is hidden; the host word is the bar's host chips. Decided by: the owner.
- 2026-11-24: the ⚙ sheet works with Machine's own keys, as the stoker and night shift views do; esc steps back, and ctrl+q always closes every sheet and gives the keys to the list, whatever the content does. The ways: the sheet works; ⇥ back for now; read-only. Decided by: the owner.

## Left to the builder

- How the bar's chips fold below 130 columns, beyond the rule that a chip with trouble never folds. (The desk width and the list's width are ruled: design review D16.)
- How DONE knows the owner opened a chat (where "last opened" is kept, per screen or per owner).
- The diff's drawing: colours from the console tokens, word-level marks, how many lines of context.
- The exact text of the review message the agent gets, inside the anti-goals above.
- The order and the fold rule of the footer's chips at narrower desk widths.

## Calls to check

- **The terminal library.** I recommend `github.com/charmbracelet/x/vt` (the same project as the `x/term` the console uses) and `github.com/creack/pty`, two new dependencies. If wrong, the fidelity test fails, and the builder swaps the library behind one interface.
- **The latency bar of 16 ms.** One frame at 60 Hz. I recommend it as the bar the done-when line checks. If wrong, the owner feels lag the test misses.

- **"Last opened" is per owner, not per screen.** I recommend one record for all the owner's screens, so a chat read on the phone leaves DONE on the desk too. If wrong, the desk shows done work the owner already read.
- **The key for the diff is `D`**, because `d` dismisses. If wrong, the owner learns a key that clashes with a later one.
- **`t` works only from the list.** In the chat line letters type. I recommend that; `?` lists it. If wrong, the shell is hard to find.
- **No key line, even for a new owner.** I recommend `? keys` alone, plus the first-run setup that already teaches the keys. If wrong, a first-day owner does not find `t` or `D`. The fix is cheap: show the five keys of the place for the first week.
- **Spaces count sessions that need the owner, not DONE.** I recommend that, so the dots mean one thing. If wrong, a finished repo looks calm.
- **The landing page's words.** I recommend "Every repo. Every agent. One desk." If wrong, the release pass rewrites the headline only.

## Done when

- At 160 columns with three repos, the owner taps `ledge ●1`: the list shows only ledge's sessions, and `● 3 need you` still counts all three waits.
- An agent ends its turn: its row moves to DONE with `done now` and what it said. The owner opens its chat: it moves to IDLE, and `✓ N done` drops by one.
- A chain with one member that asks and one that finished sits whole in NEEDS YOU. When the ask is answered, the whole chain moves to DONE.
- With a demo busy loop on gpu-1, the footer shows `⚠ busy loop` on gpu-1's chip only, and `⚙` shows `!`. A tap on the chip opens the cog's sheet on that loop. `esc` returns to the same chat.
- `t` on ux-pass opens a shell in its worktree. `t` moves the keys to the shell. The owner quits matchblox and starts it again: the shell is still there.
- `D` on ux-pass shows its diff against main. The owner comments on two lines and presses `⏎`: the agent's pane shows one message with both `file:line` references, and the row moves to WORKING.
- A file in the diff that holds an escape sequence draws as visible text, and the terminal's title does not change.
- The desk has one line under the body. `?` opens the keys sheet; the owner types `shell` and presses `⏎`, and a shell opens under the chosen agent.
- `,` opens ⚙ from the list. With the shell's keys, `,` types a comma into the shell, and the bar shows `ctrl+q list`.
- On a phone-width screen, the console looks as it does today.
- At 160 columns, Claude Code inside matchblox shows the same screen as in its own pane, cell for cell, for a menu, a diff and a permission prompt (the test compares the two captures).
- 1000 keys typed into the agent through matchblox echo in 16 ms or less at the 95th percentile, measured on the desk.
- A drag over the agent's screen selects only its text, and `c` copies it to the laptop's clipboard over ssh.
- The screen closes at the bottom: a rule, then the bar.
- Before the merge: the landing page shows the new desk's stills and replay; `README.md` and every doc name the bar, `?`, `,`, DONE, `t` and `D`, and no doc names the key line or the Machine tab; Pocket's belt has `?`, `,`, `t` and `D`, and its App Store text and screenshots show the new desk. `.github/scripts/landing-guard.sh` passes with no "unchanged" line.
