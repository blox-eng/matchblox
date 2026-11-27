# A builder at a desk sees which repo needs them and what finished, with the agent's own pane beside the list

Mux extract (#514 on `origin/main`) is the floor: List, Split, Start, Focus, Kill and the beside view. This spec is the **desk**: spaces, DONE, ages, bar, `?`, `,`, `t` and `D`, with the agent still a mux pane. It is not a herdr slice. #205 stays out.
Start the work from that `origin/main`, not from an older checkout.
GitHub PR #504 named the look, then drew the agent through a vt wrapper. That wrapper does not merge. Copy its mockup to `design/0022-the-desk.html` only after this spec is approved, as a picture. The agent region in that picture is cells. On the real desk it is a mux pane.

## Who, and the moment

The owner, at a 16-inch desk, with 8 to 15 agents in tmux across 3 to 5 repos and 2 or more hosts. They come back after 20 minutes. They want one look: which repo needs them, which agent finished, whether a host or a plan is in trouble. Then they read a change and tell the agent what to fix. The agent pane they already have must stay a real tmux pane: drag, wheel, copy-mode and cell fidelity as today.
Evidence: owner, 2026-11-25 — freeze native tmux, extract mux, then chrome; the #504 wrapper is not the lasting agent view. Owner, 2026-11-27 — frame panes; `D` takes the agent's seat; `?` and `,` stay in the list; the full desk product, no herdr.

## Why it matters

- A finished turn sits in IDLE. Estimated: 3 to 6 opens per return to find what finished.
- Machine and usage live behind `⇥`. A plan near its limit shows only when they go there.
- The desk already named spaces, DONE, ages, the bar, `?`, `,`, `t` and `D`, then drew the agent through a vt wrapper. Mux extract forbids that wrapper as the lasting view. Estimated cost if skipped: they keep today's list with no DONE or bar, or they ship a wrapper that drops native mouse and copy-mode.

## The experience

The builder opens matchblox on a desk, inside tmux, in the home session, at 100 columns or more.

They see four mux panes in one window:

```
header   matchblox   all · matchblox ●2 · base · ledge ●1     ● 3 need you · ✓ 2 done   ⚙
list (40 col)     |  agent's own pane (nested tmux client of that agent's window)
bar      LIST   ● ws-1 cpu 34% mem 61%  s▸   ✻ work 5h 62% ↻2h48m · wk 40% ↻4d   ≋ stoker   ? keys
```

The list, the header and the bar are matchblox. The agent pane is today's view (`ViewArgv`) of the agent's **window**, so a shell split in that window is visible. Typing there types into the agent. Drag, wheel and copy-mode belong to tmux.

The key line is gone. The Machine tab is gone on this desk frame. `,` opens ⚙.

A terminal under 100 columns, and Matchblox Pocket, keep today's sidebar console (list, or one zoomed pane). They do not grow the four-pane frame.

### Who has the keys

One place has the keys at a time.

| Mode | Pane that has the keys | `[` `]` | `c` | `ctrl+q` |
|---|---|---|---|---|
| LIST | the list | next and previous space | copy, as today | flip to the agent |
| CONFIRM | the list | spaces | copy | cancel, then list |
| KEYS or MACHINE (sheets) | the list, sheet on top | unused in the sheet; filter is typing | unused | close the sheet, stay LIST |
| AGENT | the agent pane | types into the agent | types into the agent | flip to the list |
| SHELL | the shell pane in the agent's window | types into the shell | types into the shell | back to the list |
| REVIEW | the agent seat, now the diff | previous and next file | comment this line | restore `ViewArgv`, keys to LIST |
| COMMENT | the review, on the comment line | unused | unused | cancel the comment line, stay REVIEW |

`⇥` on the four-pane desk is unbound and silent. On the sidebar console (phone, Pocket, a narrow terminal) `⇥` still opens Machine.

### Header: the work

One line. Left: the mark and `matchblox`. Then the spaces. Right: `● N need you · ✓ N done`, then `⚙`.

A space is a repo with at least one live session: `all · matchblox ●2 · base · ledge ●1`. `●N` counts sessions that need the owner in that repo, not DONE. `all` has no count. A tap, or `[` `]` from LIST, filters the list to that repo. One repo: no space chips. Spaces that do not fit end in `+3 ▾`. A session outside git uses its folder name. Space filter and host filter (`s`) combine. Empty: `Nothing in base on gpu-1. esc shows all.`

### List: clearer states

Groups, in order: `NEEDS YOU`, `DONE`, `WORKING`, `IDLE`, `HELD`. An empty group takes no row.

A session **needs you** when it asks, waits for a merge, or waits for a stoker go. That always wins. It sits in `NEEDS YOU`, even if its last turn just ended.

DONE holds a session whose last turn ended (the sample goes from working or busy to idle, with no question and no permission) after the owner last gave that agent the keys, and that does not need you. Its second line is `said ` then what the agent said last. The time on the row is `now` while that is under a minute. When the agent gets the keys, the row moves to IDLE. `✓ N done` counts sessions in the DONE group only. A done mark on a chain member that still sits under a NEEDS YOU head does not add to that count.

The agent gets the keys when the owner presses `⏎` on its row, clicks or taps the row (today that shows the agent and gives it the keys), `ctrl+q` onto the agent, or `⌃o`. `↑` `↓` only move the choice. They do not leave DONE.

Time in state sits at the right, before the agent's mark: `10m ✻`. A PR badge shows before the time. NEEDS YOU and DONE: oldest first. Other groups keep today's order.

Chains stay whole in the group of the most urgent member (needs you, then done, then works, then idle, then held). A finished member that is not itself a wait keeps a `✓` under the chain head.

Glyphs, and `DESIGN.md` §4 in this same PR: works `✦`, asks `╿`, done `✓` (`match-ember`), idle `╵`, held `╽`, spent `│`. Each glyph still has one meaning.

`g` on a chain still tiles up to four members in the **agent seat**, as today. Header and bar stay. `G` still opens the chain's room.

### Bar: the mode and the machine

One line under the body. Far left: the mode word. `LIST` in `muted`. `AGENT`, `REVIEW`, `COMMENT`, `SHELL`, `MACHINE`, `KEYS`, `CONFIRM` in `accent` bold when that place has the keys.

Then hosts (`● ws-1 cpu 34% mem 61%` `s▸`; fold to `3 hosts s▸` when they do not fit; a host in trouble never folds), usage (`✻ work 5h 62% ↻2h48m · wk 40% ↻4d`), stoker (`stoker off` / `≋ stoker` / `☾ night → 07:00` / `stoker rests`, with `≋ N` when more than one runs). Far right: `? keys`. When AGENT or SHELL has the keys, `ctrl+q list` shows before it.

Trouble takes colour on its own chip only: `⚠ busy loop` (neg), `not reached · since 2m` (warn), a plan within 10% of its limit (warn). A tap on a chip opens that part of ⚙. A notice stays at the foot of the list for 2.5 s.

A confirm, a consent, or a prompt that waits for a key shows in the bar (`CONFIRM  kill flint-auth?  y kill · esc keep`). The list still receives `y` and `esc`. The chips return when it is answered.

The vertical edge between list and agent shows which side has the keys (accent on that side). Header and bar use a normal pane border.

### Keys

`?` from LIST opens the keys sheet **over the list**. Groups, in this order:

- anywhere: `ctrl+q`, `ctrl+o`, `n`, `s`, `esc`
- the list: `⏎` `↑` `↓` `[` `]` `t` `D` `g` `G` `z` `x` `/` `m`
- a review: `c` `]` `[` `⏎` `esc`
- the shell: `t` (flip), `ctrl+d` (the shell's own)
- ⚙: today's Machine keys, then `esc`

Typing filters the sheet, `↑↓` chooses, `⏎` runs. `esc` clears the filter, then closes. With no match: `No key does "xyz". esc clears it.`

`,` from LIST opens ⚙ **over the list**: usage, machine, busy loops, stoker, git worktrees, history, panes, hosts. Content is today's Machine. `esc` closes.

When AGENT or SHELL has the keys, `?` and `,` type there. matchblox takes no key the agent needs.

### `t` shell

`t` from LIST (and `?` then `shell` then `⏎`, the same act) opens a shell **under the agent in the agent's own tmux window**, in that session's folder. The desk's agent pane is a nested client of that window, so the split is on screen. It lives after matchblox quits, after ssh drops, and after `⌃o`. `t` again moves the keys between agent and shell. The divider says `shell · ~/dev/matchblox/wt/ux-pass · t switches`. The list row shows the word `shell` in `muted` before the agent's mark (not `▣`, which is OpenCode). The shell ends when the shell exits.

Fails: `not opened: gpu-1 is not reached`. `not opened: ux-pass has no folder`.

### `D` review

`D` from LIST **replaces the agent pane** with the session's diff. The nested client is gone until REVIEW ends.

The base is the branch the session started from, recorded at start. If that name is missing, the host's default branch (`git symbolic-ref refs/remotes/origin/HEAD`, else `main`, else `master`). Head: `ux-pass · diff against main · 4 files +212 −38   esc chat`. `]` `[` move files.

On a line, `c` opens a one-line field under that line: `›  ⏎ add · esc cancel`. `⏎` stores the comment on that line with `✎`. A second `c` on the same line replaces it. There is no edit list in this slice. `esc` on the field cancels that comment and stays REVIEW.

`⏎` with no field open sends one message to the agent: each comment with its `file:line` and the line of source. After send: `sent a review of 3 comments to ux-pass · now in WORKING`, `ViewArgv` returns, keys go to LIST.

Fails: `No changes to review: ux-pass has no git folder.` `ux-pass has no changes against main.` `main is gone on ws-1. The diff shows against origin/main.` A diff too large: the file list shows, each file opens on its own.

### First start after the upgrade

The first desk start on an upgraded server closes the old two-pane viewer pane, opens header and bar beside the list and the agent, and says so once at the list foot. It keeps today's `ctrl+q` bind and the window options `@matchblox_console`, `@matchblox_beside`, `@matchblox_chain`. The agent's own session is never touched.

### Wrong paths (words the person sees)

| What happened | What they see | Fix |
|---|---|---|
| Header or bar pane did not open | `the desk frame did not open: <error>` at the list foot | Restart matchblox; the list still runs |
| Agent pane did not open | `the agent pane did not open:` then the error, as today | `⏎` again on the row |
| `D` cannot take the agent seat | `review did not open: <error>. esc back to the list` | `esc`; the agent view returns if it can |
| Host not reached | rows dim; bar chip `not reached · since 2m`; send stays in the line | `s` or ⚙ Hosts; send again |
| No sessions | `No sessions yet. + new session starts one here (n).` | `n` |
| Narrow terminal or Pocket | today's sidebar console | none — the four-pane frame does not open |

## Approaches

1. **Frame panes.** Header, list, agent, bar are mux panes. The agent stays today's nested tmux client. Sheets cannot float over the agent: `D` takes its seat; `?` and `,` stay in the list.
2. **Hybrid status line.** Keep today's two-pane split. The bar is a tmux status line. Cheap. The bar cannot draw chips, confirms or `?` as matchblox.
3. **List only.** Header and bar live in the list pane. The agent is full height. The screen does not close under the agent.
4. **Wrapper.** Matchblox re-draws the agent through vt. The desk spec picked this. Mux extract forbids it as the lasting view.

The owner picked **1** on 2026-11-27.

## Non-goals

- A herdr adapter (#205), and any work that makes herdr the next slice.
- Shipping the desk **wrapper** (nested tmux client → pty → vt). PR #504's wrapper does not merge as the lasting agent view.
- Changing the sidebar console's layout on Pocket or on a terminal under 100 columns. The merge pass still names `?` `,` `t` `D` on Pocket's belt and in the store text.
- Changing `matchblox sessions --json`, `peek` or `send`.
- An editor, file tree, embedded browser, phone emulator or computer use.
- A second shell, tabs inside a session, or free splits.
- Review comments posted to GitHub. The review goes to the agent only.
- tmux control mode as the transport.
- Pulling SSH into mux. Remote stays the hop; the view on a peer is still JumpSSH.
- `D` on another host's session. It stays a named refusal (`review did not open: <host> is another machine …`). No peer-paste IPC is built. — ruled 2026-11-27, the controller of the fix wave, after the whole-branch review (Minor 3).
- A persisted record of turn ends. DONE needs a turn end that the console saw, or a KeysAt that the session's last activity is after. A session first seen at rest is IDLE, also after a service restart. — ruled 2026-11-27, the controller of the fix wave (Important 1).

## Anti-goals (failure even if it works)

- The builder loses native select, wheel, copy-mode or cell fidelity on the agent pane.
- A wait is not on the screen, or not counted, while a space or host filter is on, and the header's `● N need you` hides it.
- A chain draws in two groups, or a member draws outside its chain.
- A session that does not need you leaves DONE without the owner giving that agent the keys, or stays in DONE after they do. An ask, a merge wait or a stoker go may pull it to NEEDS YOU.
- A review, or a comment, reaches a session other than the one its head names.
- The diff draws an escape sequence from a file, and it moves the cursor, changes the title or writes the clipboard.
- The bar flashes or changes colour while nothing is wrong.
- The shell opens in a folder other than the session's own.
- A key does nothing, with no word why — except `⇥` on the four-pane desk, which is silent.
- `?` or `,` replaces the agent pane, or `D` draws only in the list.
- An agent's pane is killed or respawned as a side effect of opening the frame, when the builder did not ask.

## Surfaces

- an agent's tools — mux starts, types, pastes, focuses, kills and respawns agent panes; `t` splits a shell; `D` pastes a review into the agent. Harm: a send, a kill or a review hits the wrong pane, or a spawn runs with the wrong env. Anti-goals: review target, shell folder, pane killed as a side effect.
- secrets — spawn env, paste buffers, screen capture and a review message can hold tokens or repo lines. Harm: they reach a log, an error, or another pane's buffer. Anti-goal: the diff's escape sequences.
- personal data — the bar shows host names and account names. Harm: a real host, user or account name reaches the replay, the mockup or `www/`. Anti-goal: keep demo names only in those files.

Checked, not touched: authentication, money, uploads, webhooks, CI. The agent's bytes do not run through a matchblox vt.

## Rulings

- 2026-11-16 — the right side is the agent's own pane, a nested tmux client, not a drawn chat. — the owner (agent beside the list).
- 2026-11-23 — desk content: spaces, DONE, ages, bar, `?`, `,`, ⚙, `t`, `D`; phone keeps the sidebar console. — the owner (desk spec).
- 2026-11-23 — matchblox is the wrapper. — the owner (desk spec). **Superseded 2026-11-25 / 2026-11-27.**
- 2026-11-25 — freeze native tmux UX, extract mux, then chrome. The wrapper is not the lasting agent view. — the owner (mux extract).
- 2026-11-27 — the chrome frame is four mux panes: header, list, agent, bar. The agent stays `ViewArgv`. — the owner (picked way 1).
- 2026-11-27 — `D` replaces the agent pane; `?` and `,` stay in the list; `esc` or `ctrl+q` brings the agent view back. — the owner.
- 2026-11-27 — one spec for the whole native desk. The plan builds frame, spaces, DONE, ages, bar, `?` and `,` first, then `t`, then `D`. — the owner.
- 2026-11-27 — this work is the full desk product. Herdr (#205) is not in scope and is not next. Mux extract is the floor, not the story. — the owner.
- 2026-11-27 — needs-you beats DONE. `✓ N done` counts the DONE group only. The four-pane frame opens from 100 columns. `g` still tiles in the agent seat. The nested client shows the agent's window, so `t` is visible. The shell tag on the row is the word `shell`, not `▣`. `ctrl+q` closes a sheet, else leaves REVIEW, else flips list and agent. After a sent review, keys go to LIST. — proposed, not confirmed (builder, from the cold read).
- 2026-11-27 — the bar is display. Keys for list, confirms and list-sheets stay in the list pane. AGENT, SHELL and REVIEW have the keys in their own pane. — proposed, not confirmed.
- 2026-11-27 — "last opened" is per owner, across screens, and means the agent got the keys, not a list highlight. Spaces count needs-you, not DONE. `t` works only from the list (and from `?`). No key line. The diff key is `D`. — proposed, not confirmed (carried from the desk spec's calls).
- 2026-11-27: spec approved. Decided by: the owner. The Calls recommendations stand (`N` for new, NEEDS YOU split, second `t`, `ctrl+q` bind, bar is display, needs-you beats DONE, frame from 100 columns).

## Left to the builder

- How header and bar attach (one matchblox with extra ttys, or thin clients of the same service). The person sees four panes and one set of keys as above.
- How a click on a header space or a bar chip reaches the list when those panes are display.
- How the vertical edge between list and agent shows which side has the keys, across four tmux panes.
- How the bar's chips fold below 130 columns, beyond "trouble never folds".
- Where the per-owner "last given keys" record lives.
- The diff's drawing: colours, word-level marks, context lines, and the exact review message, inside the anti-goals.
- The order of plan tasks, as long as frame + spaces + DONE + ages + bar + `?` + `,` land before `t` and `D`.
- Whether PR #504 closes, rebases onto this, or stays as history. This spec does not merge its vt.

## Calls to check

- **Bar is display; keys stay in the list.** Recommendation: yes — because three key sinks fight `ctrl+q`. Cost if wrong: confirms type into a one-line bar pane, or `y` never reaches the list.
- **Landing headline.** Recommendation: "Every repo. Every agent. One desk." — because the desk already used it. Cost if wrong: the release pass rewrites one line.
- **Pocket belt in the merge pass.** Recommendation: name `?` `,` `t` `D` on the belt, keep the sidebar layout — because this spec does not rewrite the phone screen. Cost if wrong: Pocket's keys lie, or this spec grows an iOS layout rewrite.
- **Needs-you beats DONE.** Recommendation: yes — because a wait must stay on the screen. Cost if wrong: an agent that asks after a turn sits in DONE and the owner misses it.
- **Four-pane frame from 100 columns.** Recommendation: yes, matching today's mid-width list. Cost if wrong: a 90-column desk is cramped, or a 110-column desk never gets the bar.
- **New session stays `N`.** The empty state and the keys sheet in this spec say `n`. Today `N` starts a session and `n` starts a night (`docs/keys.md`). Recommendation: treat those `n` as `N`. Cost if wrong: night starts when they mean new.
- **NEEDS YOU keeps today's waits that this spec does not move to DONE.** Stuck PR, conflicts, changes requested stay in NEEDS YOU. A turn that ended with no question and no permission is DONE. Recommendation: that split. Cost if wrong: a red PR sits in IDLE, or DONE swallows a wait.
- **Second `t`.** Recommendation: LIST `t` opens or, if a shell exists, flips to SHELL. SHELL `t` flips to AGENT. AGENT does not steal `t`. Cost if wrong: the agent cannot type `t`, or the owner cannot leave the shell.
- **`ctrl+q` bind.** Recommendation: one tmux bind in this window: if the list pane is active, send the chord to matchblox (close sheet, leave REVIEW, else flip); if the agent or shell is active, today's flip to the list. Cost if wrong: sheets never see `ctrl+q`, or the agent never gets a flip.

## Done when

- [ ] At 160 columns with three repos, the owner taps `ledge ●1`: the list shows only ledge; `● 3 need you` still counts every wait.
- [ ] An agent goes from busy to idle with no question: its row sits in DONE, time `now`, second line `said ` and the last words. The owner presses `⏎` on it: the row moves to IDLE, and `✓ N done` drops by one.
- [ ] A chain with one member that asks and one that finished sits whole in NEEDS YOU. `✓ N done` does not count the nested finished member.
- [ ] With a demo busy loop on gpu-1, the bar shows `⚠ busy loop` on that chip only, and `⚙` shows `!`. A tap opens ⚙ on that loop. `esc` returns to the same row.
- [ ] `t` on ux-pass opens a shell in its worktree, visible under the agent in the desk's agent pane. Quit and start matchblox: the shell is still there.
- [ ] `D` on ux-pass hides the agent view and shows the diff. `c` on a line, type, `⏎`, `c` on a second line, type, `⏎`, then `⏎` with no field: the agent pane is back, it shows one message with both `file:line` lines, and the row is WORKING.
- [ ] A file in the diff that holds an escape sequence draws as visible text. The terminal title does not change.
- [ ] `?` opens over the list, not over the agent. Typing `shell` and `⏎` is the same act as `t`.
- [ ] `,` opens ⚙ over the list. With the agent's keys, `,` types a comma into the agent, and the bar shows `ctrl+q list`.
- [ ] Drag, wheel and copy-mode in the agent pane belong to tmux. A drag selects only that pane's text.
- [ ] On a terminal under 100 columns, and on Pocket, the console is today's sidebar, not the four-pane frame.
- [ ] The first start after upgrade shows one notice, then the four-pane frame. The agent's own session still runs. `ctrl+q` still flips.
- [ ] `DESIGN.md` §4 names the six glyphs above, each with one meaning.
- [ ] Before merge: the landing page shows the desk; `README.md` and the docs name the bar, `?`, `,`, DONE, `t` and `D`, and do not name the key line or the Machine tab on the desk. Pocket's belt names `?` `,` `t` `D`. `.github/scripts/landing-guard.sh` passes with no "unchanged" line.

## Cold read

Cold read: 2026-11-27, 2 fresh reader(s). Blocker 1, major 15, nit 2. Fixed 17, contested 1, for the person 0, declined 1. Second read: 24 left.

Contested (first read): Follows claimed mux extract had not landed, because this checkout's `internal/mux` is older. Evidence: `origin/main` `internal/mux/mux.go` has List, Split, Start, Focus, Kill; #514 merged. The spec already says start from that `origin/main`.

Declined (first read, nit): landing headline vs `www/index.html`. The headline sits under Calls.

Second read (not fixed): `n` vs `N`; which rows stay in NEEDS YOU; second `t` from SHELL vs AGENT; `ctrl+q` tmux bind vs sheets; click on a DONE row; park vs ask; incomplete `?` map; `esc` in REVIEW; diff size; where the start branch is stored; first-start notice words; `G`'s seat; `+3 ▾`; proposed rulings vs body; Pocket belt crowding; `DESIGN.md` §6; board vs list as "today's sidebar"; this tree vs `origin/main` mux; every console window vs one; nits on landing, docs list, ⚙ tap, REVIEW `⏎`. The four blockers are under Calls.
Changed after the read: Follows, Who (evidence), Non-goals (herdr), Rulings (full desk, no herdr).
