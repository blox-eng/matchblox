# The board

Issue: #372. Status: sketch approved by the owner, 2026-11-10 ("Looks awesome, lets lock this in"); engineering review and design review, 2026-11-11, with the owner's answers written in below. Date: 2026-11-10.
Sketch: `design/0019-the-board.html` (the owner's artifact "The Board", version 6). Open it in a browser. It runs at 40, 80 and 160 columns. It was brought up to the two reviews of 2026-11-11 (artifact version 8). Where it and this document still differ, this document applies.
Builds on: #326 (the chain room), #346 (Enter goes into the pane), #299 (chains), design/0017 (every host).
Merge order: #355 → #361 → #326 → #367, then this. Code starts after #326 lands, or after matchblox-chain agrees the file boundaries (see "The chain room").

## Why

The owner runs 4 to 25 agent sessions on one or two machines. The owner opens the console to answer one question: which session needs me, and what does each one do now?

Today the answer is spread over seven tabs and a dock. The owner said the Ships, Stoker and other tabs are never opened. The owner also said: the most use must come from Matchblox Pocket, and the console must "behave like an app": analog where the hand touches, digital where a key is faster.

Measured on the owner's machine, 2026-11-10 (`matchblox sessions --json`): 25 sessions. 2 ask, 13 work, 10 are idle, 14 are held, and 15 drive a pull request. So two cards matter, ten must be out of the way, and the pull request earns its own column.

Success: the owner opens the console on any screen, sees every match in the column of what it needs, and resolves the first card with one key or one tap.

## Goals

1. The console's home is a board. Each column is a state, and each card is one match.
2. The card names the one thing the match needs, and the key that does it.
3. The chosen card opens in place (the peek): its last message, and its question with the options.
4. The same board works on Pocket at 40 columns, by touch alone.
5. A chain and a host are filters on the board. Neither is a screen to go into.
6. Everything else is on one tab, Machine, which the owner drills into.
7. Text that the owner copies in the console reaches the clipboard of the machine in the owner's hand, through every hop.
8. Each tab, dock and key that the board replaces leaves the code, the docs and the site.

## Non-goals

- No drag and drop. A match changes column when its state changes. The owner never moves a card.
- No swimlanes. Most sessions are chains of one, so a lane for each chain is a row for each card.
- No fifth column. Held, review and stuck are words on a card.
- No change to `matchblox sessions --json`, `peek`, `send`, or any command that agents and scripts read.
- No new native screen in the Pocket app. The app draws the console, so it gets the board. The app changes in three places, each its own task: the Board layout on the belt, the swipe and the long-press, and the clipboard check.
- No new way to sort. The order inside a column is fixed (below).
- No timer that turns the bottom strip, and no setting for one.

## Anti-goals

The work fails, even if it runs, when:

- A match that waits for the owner is not on the screen and not counted on it.
- A filter (a hidden column, a chain, a host) hides a wait with no sign.
- A key answers a question that the owner has not seen.
- A tap is taken as consent for a destructive act.
- A card moves to another column and the owner cannot see where it went.
- A host that cannot be reached loses its cards with no word.
- The console says `copied` when the text did not reach a clipboard.
- A column moves or changes width by itself while the owner reads. At Medium a section may move when the waits or the chosen card change, when `QUIET` opens, or when the owner hides or shows a section. `SHIPPING` may also move when the count of ships changes under 4 (owner, 2026-11-14).

## Owner's decisions

All dated 2026-11-10.

- The design is made first as an HTML artifact. It must port cleanly to Pocket and follow DESIGN.md.
- A click or a tap on a column's name hides the column.
- A right-click opens a context menu with "copy last message".
- The left and right arrows move between columns.
- `v` switches verbose and non-verbose cards.
- Highlighting a card peeks at it and opens its question.
- Copy passes through every hop (matchblox → the owner's laptop → tmux), "no exceptions".
- Chains are considered on the board, with the lessons of the common board tools. The owner agrees with the lessons in "What the board tools learned".
- The Machine tab holds the context stats (burn, usage, configs, accounts) and the repos, with menus to drill into. It is not central.
- A work grid in the style of a git profile, with a colour for each harness: Claude Code, Codex, OpenCode, other, unknown.
- Hosts are in one layer. The owner does not go back to a host list and in again.
- The match's motion is reworked for the board.
- A bottom strip that the owner turns on and off, and that shows the machine and a weekly view.
- Each card shows a mark for its harness.
- Each row that the owner opens on Machine has charts.
- On Pocket the board's keys are not a line of shortcuts at the bottom of the console. They are part of the app's belt and its layouts.

## My calls

The owner asked me to decide these from first principles (2026-11-10). Each can be overturned at the review of this document.

| Call | Reason |
|---|---|
| The board is the home, in place of #326's chain list. | Two lists of the same matches make the owner choose before looking. One home. |
| The chain's room is the board narrowed to that chain, plus the stoker's card and the talk line. | The room's own list of matches, ships and worktrees repeats the board. The card and the talk line are the only parts that a room alone has. |
| The digits answer a question from the peek. Two guards apply. | Seeing the options and choosing one is direct. Going in first adds a step and no safety. The guards remove the stray key. |
| The bottom strip turns on `u` or a tap only. There is no setting. | Only fire and a match that waits may move (DESIGN.md §1). A line that changes by itself asks for attention it has not earned. |
| One daily record is allowed, and it ships second. | "No new data sources" in #372 guards the board against growth. The work grid has no other source. The board ships without it. |
| The harness colours appear on Machine only. | The board keeps one accent (DESIGN.md §1 rule 4). |
| A tap on `● N need you` clears every filter. | It is the one control that must always reach a wait. |
| Digits no longer open tabs or columns. | They answer questions, and two tabs need only `Tab`. |
| The order inside a column never changes while the state does not. | A list that reorders under the eye is the fault the chain room's review already ruled out. |

## Approaches

- **A. The board is the home (picked).** Columns are states. A chain and a host narrow it. The room is the narrowed board with the stoker's card. Cost: it changes two approved parts of #326 (the home list and the room's layout), so the two sessions must agree who builds what.
- **B. The board is a third tab beside the chain list.** #326 stays as approved. Cost: two homes show the same matches, and the owner chooses between them on every open.
- **C. The board's cards are chains.** It keeps #326's rule that the chain is the only grouping. Cost: a chain of five hides which match needs the owner, which is the question #372 sets.
- **D. The board lives inside a room.** No clash with #326's home. Cost: the owner never sees every match on one screen.

A does the most with the fewest screens. The common board tools all keep the state as the columns and put the group in a second dimension. None makes the group a column or a second home.

## The design

### Columns

Four columns, left to right. A match is in exactly one: the leftmost whose rule holds.

| Column | A match is here when |
|---|---|
| `NEEDS YOU` | it asks a question or a permission, or a stoker step for it waits for the owner's go (these two hold even while it is busy or runs a background shell); a pull request it drives waits for the owner's merge; its ship is stuck (`CI failed`, `conflicts`, `changes requested`) and it is not busy; or its turn ended today and it waits for input (`waits`, `paused`, `done` of today's `WAITS FOR YOU`). A wait since yesterday is cold, and cold is `QUIET`, as today. |
| `WORKING` | it is `busy`, `agents`, `tests` or `shell`. |
| `SHIPPING` | it is at rest, it needs nothing, and a pull request it drives is in flight (`ci`, `green` with auto-merge, `merged` and deploying). |
| `QUIET` | everything else: `idle`, `cold`, `stale`, `out`, and held or parked. |

A match that the owner parked (`z`) is in `QUIET` whatever else is true of it, and `● N need you` does not count it: the owner said "not now". Its card says so (`⏸ parked · asks`). A match that the stoker holds follows the rule like any other, so one that asks is in `NEEDS YOU` and is counted: the owner did not choose that hold. A match that works with a full context stays in `WORKING` and shows the spent match.

Order inside a column: `NEEDS YOU` by the oldest wait first. `WORKING` by when the session started, the oldest first (the sample has no time for the turn's start; owner, 2026-11-11). `SHIPPING` by when the ship reached its stage, the oldest first. Both times are in the sample today, so a phone and a desk show one order, a restart keeps it, and a card keeps its place while it stays. `QUIET` by the newest first. Members of one chain that are in the same column sit together.

An empty `NEEDS YOU` says `nothing needs you` in `faint` (owner, 2026-11-11: "needs you" is the product's phrase, in the header, the column, the canon, the docs and the site). Any other empty column shows its name and `· 0`. No column hides or changes width by itself.

### Where a lane comes from

Added 2026-11-12. The owner, after a first use of the board: "sometimes stuff are in needs you, and I can see the lane is working and not needing me, same for quiet". The owner chose to fix it in this pull request, for Claude Code and OpenCode both.

What went wrong, live on 2026-11-12:
- A session showed a question menu but sat in `QUIET`. Its hook was lost when the service restarted.
- A session ran two shells and two agents but sat in `NEEDS YOU`. An old question in its transcript won over live work.

The rule: **an agent that reports its own state decides its lane.** matchblox guesses only when no report exists.

- **Claude Code** writes `<config dir>/sessions/<pid>.json` itself. matchblox reads `status`, `waitingFor` and `statusUpdatedAt`:

  | `status` | Lane input | The card says |
  |---|---|---|
  | `waiting` | asks | the reason in `waitingFor`: `permission prompt`, `input needed`, `dialog open`, and others as written |
  | `busy` | works | as today |
  | `shell` | works (the turn ended and background shells or agents run) | `shell` |
  | `idle` | today's rules: a turn that ended on a question asks, a turn that ended today waits | as today |

- **OpenCode** reports nothing outside its process by default. `matchblox setup` installs a plugin, `matchblox.ts`, in OpenCode's global plugin folder. The plugin writes `<matchblox state dir>/opencode/<pid>.json` in the same shape: `status` is `busy`, `waiting`, `retry` or `idle`. `waitingFor` is `permission` or `question`. `retry` carries the attempt count and the time of the next try. A permission or a question waits until its own reply event clears it, matched by its id. Retry is `works`, and the card says `retrying · attempt N`.
- **A report from a dead process is no report.** matchblox checks that the pid lives and started when the report says.
- **A live report beats an old guess.** With a report present, `busy` or `shell` means no ask from the transcript or the hook queue puts the card in `NEEDS YOU`. A stoker step that waits for the owner's go still does, because that wait belongs to matchblox.
- **A restart loses no wait.** After the service starts, the queue seeds each `waiting` report as an ask.
- **No report** (Codex, an older OpenCode, other agents): the pane decides, as today, but only from the live lines at the bottom of the pane, never from the transcript above them.

### The card

Lines, top to bottom. Each is cut with `…`, never wrapped, except where the peek says so.

1. The selection mark `▌`, the match (DESIGN.md §4), the name, `@host` when the board shows more than one host and the match is on another one. At the right: the harness mark, then the wait.
2. The state word and what it says: `asks Which way do we fix the login?`, `green #4556 waits for your merge`, `busy writes the token refresh test`. The word takes the accent in `NEEDS YOU` and `muted` elsewhere.
3. The ship and its track (`#4551 ●━◉┄○┄○`), or the stoker's step (`≋ step 2 of 3`), then the context (`ctx 62%`, `warn` from 70%, `neg` at the compact line). At the right: the chain's stub. On a card under 26 columns this line drops parts in this order until it fits (owner, 2026-11-11): the chain's stub, a context under 70%, then the track's undone stages (`┄○`) from the right, never inside a stage. A context of 70% or more stays. Line 1 cuts the name with `…` and keeps the harness mark and the wait. Nothing is cut inside a run of glyphs.
4. The keys, in `NEEDS YOU` and on the chosen card: `p merge #4556   ⏎ go in`.

The harness mark names the agent that runs in the pane: `✻` Claude Code, `❋` Codex, `⬢` Cursor, `▣` OpenCode, `·` any other. It is `muted`. It comes from the session's agent (`Session.Agent`). The account rows keep today's rule: their mark names the provider of the plan (`providerMarks` in `internal/app/dock.go`), and they do not change (owner, 2026-11-11). DESIGN.md §6 gains one sentence: a card's mark names the agent, and an account's mark names the provider. So OpenCode on an Anthropic key shows `▣` on its card and `✻` on its usage row.

**Two densities.** `v` switches them, and each screen (a phone, a desk) keeps its own choice. Verbose: every card shows lines 1 to 3, and line 4 in `NEEDS YOU`. One line: every card shows line 1, and `NEEDS YOU` adds line 2. A phone starts with verbose in `NEEDS YOU` only.

**The peek.** The chosen card is always whole, in both densities, on the `wash`. It adds, under line 3:

- what the agent said last, in quotes, three lines at most;
- its question's options, numbered, each wrapped to two lines at most.

The peek needs no key: moving the selection is the peek. From 152 columns, where the live view shows the chosen match at the right, the chosen card adds no quote and no options: the live view has them (owner, 2026-11-11). Its keys and the digits still act. A card in `QUIET` on a wide screen is one line, and its peek adds the last message.

**Answering from the peek: arm, then send** (owner, 2026-11-11). The digits answer only a question that the sample gives with its options: one question, one choice (`Session.Ask` today, Claude Code's question tool). Every other wait shows its first line and `⏎ go in`, with no digits: a permission prompt, a question from Codex or OpenCode, a call with several questions, and a multi-select. The owner answers those in the pane.

- **The first press arms.** A digit, or a tap on an option, marks that option: `❯ 1. Patch the token check` on the `wash`, with `❯` in the accent, and the card's key line says `1 again sends · esc clears`. Nothing is sent.
- **The second press sends.** The same digit, or a tap on the same option, sends it. Another digit moves the mark. `esc`, a move of the peek, or a new question clears the mark. So no single press, and no old habit, can answer.
- **The sent state.** After the send the card says `sent 1 · waits for <name>` in `muted`, and its digits are dead until the question changes. The notice says where the card went: `sent 1 to <name> · now in WORKING`.

Three guards stay under that:

- **The digit is tied to the question the card drew.** The card's options come from the sample, so the key carries the time of that sample as `Seen`, and the console sends it only while the session's question is the one it drew (`Ask.ID`). The service refuses a key whose `Seen` is zero or is not after the question began (`internal/service/keys.go`), so a question that is newer than the sample is refused there too. The pane's screen plays no part, and the peek watches nothing. The service's guard does not change.
- **The dead time.** The digits arm nothing for 400 ms after the peek lands on a card, after the console opens, and after a tab or a filter changes. The "Find the Git repositories" box has this rule today (`internal/app/scanask.go`).
- **The service's guard stays:** a key is refused when the question started after the sample that the card drew (`not sent: <name> asks something new — look again`).

After an answer, a merge or a go, the selection goes to the first card of `NEEDS YOU`, as Sessions does today after a jump.

### The accent

The accent is on one card (owner, 2026-11-11; DESIGN.md §1 rule 4): the chosen card's `▌`, its state word and its keys, and the `❯` of an armed option on it. It is also on `● N need you` and on the open tab. Nothing else takes it. The state word of a waiting card that is not chosen is `fg` bold, and its keys are `muted`. The open column's name is `fg` bold. The lit stubs of the chosen card's chain are `fg` (the glyph still changes to `━╸`). Text under a drag is on the `wash`.

### States

| State | The owner sees |
|---|---|
| Before the first sample | The column names with no counts, and one `faint` line with the Pulse (DESIGN.md §5): `reading your matches…`. Never a zero, and never `nothing needs you` (§6: "before its first state only the name, never zero figures"). |
| An answer was sent | The card says `sent 1 · waits for <name>` in `muted`; its digits are dead until the question changes. The notice: `sent 1 to <name> · now in WORKING`. |
| No session at all | No columns. The ghost row `+ start a new session here` leads and has the cursor, and two quick starts follow, as DESIGN.md §6 has it today. |
| The service is lost | The board stays, in the dimmed inks the console uses today for a lost service, with `no service · it redials` in the notice row. No key sends. |
| A search matches nothing | Every column keeps its name with `· 0`, and one line says `nothing matches "<text>" · esc clears`. |
| A wait on a host that is not reached | The card is `faint` with `as of 14:02`. The header does not count it, and it takes no digits. |
| A column has no card | Its name and `· 0`. `NEEDS YOU` says `nothing needs you`. |

### Notices

Every toast, every refusal and the age of the data show in one place: the strip's row, in place of the strip, for 2.5 s (owner, 2026-11-11). A notice that does not fit wraps, up to three lines, on every shape, so its fix is never cut (2026-11-12: at 40 columns the lost-service notice needs three). With the strip hidden, the row appears for the notice only. The header never changes for a notice: `● N need you` and the host filter stay. When the sample is older than the console's stale limit, the row says `data is 40 s old · the service is slow` until it is fresh.

**A key that changed its job says so once.** The first press of `c`, `v`, `s`, `g`, `u`, `p`, `x` on a card whose ship waits, and of a digit, shows one notice, and the key still does its new job: `c copies now. The stoker is on its card.`, `v changes the cards now: verbose, or one line.`, `s changes the host now. The order of a column is fixed.`, `g shows one chain now. esc shows all.`, `u turns the strip now.`, `p merges now. The ships are on their cards.`, `x kills. p merges #4556.`, `A digit marks an answer now. Press it again to send.` Each shows once ever; the state directory keeps which.

### Hiding a column

A tap or a click on a column's name hides the column. The other columns take its room. A hidden column waits on the tab row as `+ work 3`, and a tap there shows it again and selects it. On a phone the hidden names are at the end of the page marks.

The header's `● N need you` counts every wait on every host and in every chain, whatever is hidden. A tap on it shows `NEEDS YOU`, clears the chain and host filters, and selects the first wait. Each screen keeps its own hidden columns, in the state directory, as `view.json` keeps the layout today.

When every column is hidden the body says how to bring one back.

### Chains

- A card of a chain has a stub at the right of line 3: `─╴login` in `faint`. When the chosen card is in a chain, the stubs of that chain read `━╸login` in the accent, in every column. This is the rule Sessions has today for a view that splits a chain.
- `g`, or a tap on a stub, narrows the board to that chain. The tab row then says `chain login · 3   esc all`. `esc` shows every chain again. `]` and `[` go to the next and the previous chain, as in #326's room.
- The narrowed board is the chain's room. It adds two regions that only a room has, above the columns: the stoker's card (its order, the active step and one step on each side, `y` gives a go) and, under the columns, the talk line (`› tell login's stoker…`). Both are #326's, unchanged. On Narrow the stoker is its own page, first in the row (`STOKER  need 1  work 1  ship 1`), with the card and the talk line on it; the other pages are the columns (owner, 2026-11-11). #326's need line is the `NEEDS YOU` column. #326's SHIPS and GIT lines go: the ships are on the cards, and the worktrees are on Machine under the repo.
- A match is never drawn twice.

### Hosts

- The board opens on all hosts that the owner ticked (design/0017). A host is a filter: `s`, or a tap on the host's name in the header (`all hosts ▾`), turns to the next host, then back to all.
- A card of another host says `@macbook` after its name while the board shows more than one host.
- `⏎` on a card of another host goes into its pane, as today.
- A host that cannot be reached keeps one line at the top of the board: `macbook: not reached since 14:02 · ⏎ try again`. Its cards stay, in `faint`, with their last known state and `as of 14:02`. They leave only when the owner removes the host.
- The Hosts screen stays for adding and removing a host. `Esc` on the board no longer goes to it. It opens from Machine.

### The bottom strip

One line above the key line. `u`, or a tap on it, turns it: usage, the machine, this week, hidden, usage again. The word at its right names what the next turn shows (`machine ›`, `week ›`, `hide ›`), as the dock's word does today. Each screen keeps its own choice. When it is hidden, the body takes the row.

| Face | It shows |
|---|---|
| usage | each account's 5-hour window and week, as the dock's usage panel at its fewest lines |
| machine | CPU and memory of the host in the filter; with all hosts, each host's CPU |
| week | a bar for each day of this week (the matches at work), and each account's week |

The strip turns by itself in one case, which DESIGN.md already allows the dock: while a detached busy loop burns, or a plan's limit is within 10%, the strip shows that face with the `warn` or `neg` word, until the owner turns it.

### The context menu

A right-click on a card, or a long-press on a phone (480 ms, and only on a terminal that reports the release of a press; a tap still acts on the press), selects the card and opens a menu at the pointer. On a phone it rises from the key line, as the consent box does. `↑` `↓` move, `⏎` or a tap does, `esc` or a tap outside closes.

1. `copy last message` (also `c` on the board, with no menu)
2. `copy the question` (when it asks)
3. `copy the link of #4556` (when it drives a pull request)
4. `copy its name`
5. `show only the chain login` (when it is in a chain)
6. `go in`
7. `hide <column>`

No destructive act is in the menu. `x` keeps its consent box.

### Copy

- Every copy goes through the console's one `clip` (`internal/app/pairpage.go`): `Options.Clip` when the host gives one, else OSC 52 through `tea.SetClipboard`, as the pair link's `c` does today.
- **Any text:** a drag over text in the console selects it, in the accent, and copies it on release. The header says `copied 42 characters`. The console owns the mouse, so it does this itself; it does not hand the drag to tmux.
- **Each hop must pass it on.** The console reads the tmux it runs in (`set-clipboard`, and `allow-passthrough` for its own pane, which it already sets for images). When that tmux would drop the write, the copy says so in the header, with the fix: `not copied: tmux drops it · tmux set -s set-clipboard on`. A new door, "Let copy through", does it on `⏎`. Only `set-clipboard on` passes an application's copy: with `external`, which is tmux's default, and with `off`, tmux ignores it, so the console treats both as "tmux drops it" (found in the build, 2026-11-11).
- **The console proves the whole path once for each screen** (owner, 2026-11-11). The console cannot read the hops after its own tmux (SSH, the owner's terminal, a tmux on the owner's side, Pocket). So a door, "Check that copy reaches you", copies a 6-letter code and asks for a paste. A paste of that code marks this screen (a phone, or a desk) as checked in `view.json`. Until then each copy says `copied: <what> · not checked here`, and the door is first in the doors line above the cards. The toast takes no key: `⏎` stays "go in". After it, `copied: <what>`. A wrong paste says that a hop drops the copy, and points to the table below.
- The table: `docs/console.md` gets a table: each terminal and the one setting it needs. Pocket must take OSC 52 into the iOS clipboard; if it does not today, that is a task of this work.
- **Paste** is unchanged: a paste goes to the agent as text, with today's guard at a menu.

### Keys

| Key | On the board |
|---|---|
| `↑` `↓`, `k` `j` | Choose the card above or below. The choice is the peek. |
| `←` `→` | Go to the column at the left or the right. They skip a hidden or an empty column. |
| `⏎`, `a` | Go into the match's pane (#346). On a phone, a tap on the chosen card. |
| `1`…`9` | Answer the chosen card's question, with the two guards. |
| `x` | Kill the match. Always, and only, through the consent box. |
| `p` | Merge the card's leading ship when it waits for the merge, through the consent box (owner, 2026-11-11: one destructive key has one meaning). On any other card it says `no ship of <name> waits for a merge`. |
| `y` | Give the go that a stoker step waits for. |
| `c` | Copy the last message. |
| `m` | Open the menu on the chosen card. |
| `v` | Verbose cards, or one line each. |
| `g` | Narrow to the chosen card's chain. `esc` shows all. |
| `]` `[` | The next and the previous chain. |
| `s` | The next host, then all hosts. |
| `u` | Turn the bottom strip. |
| `Tab` | The board, or Machine. |
| `/` | Search the board. A search narrows every column. |
| `z` `N` `r` `A` `e` `d` `h` `f` `F` `n` `o` | As today, on the chosen card. |
| `esc` | Close the menu, then clear the search, then the chain filter. It never leaves the console, except on a host that has not answered yet: then it goes back to Hosts. |
| `q`, `Ctrl+C` | Quit the console. |

Keys that change their job: `c` (was: open the STOKER tab), `v` (was: list only), `s` (was: sort), `g` (was: group), `u` (was: cycle the dock). `p` (was: open Ships) now merges, and `x` no longer merges. Keys that go: `1-7` as tabs, `S`, and `Space` and `X` outside Machine's repo view.

The key line shows the keys of the chosen card first, then `c`, `v`, `s`, `u`, then `←→` and `tab`. A key that does not fit leaves whole, from the right. A narrow terminal that is not Pocket shows `c`, `v`, `s`, `u` only: the chosen card already shows its own keys. Pocket shows no key line (see "The belt on Pocket").

### Touch

| The owner wants to | By hand |
|---|---|
| see the next column | Swipe sideways. The column names at the top are the page marks. |
| hide a column | Tap the name of the column on the screen. Tap `+ name` to bring it back. |
| peek | Tap a card. |
| go in | Tap the chosen card again. |
| answer | Tap an option in the peek. |
| open the menu | Long-press a card. |
| act | Tap a key on the card or on the belt. A destructive act still waits for a typed `y`. A go is armed by the first tap on its `y` (on the card, or on the belt's `y go`), and given by a second (owner, 2026-11-13). |
| come back | The belt's `◂`, or the edge swipe. |
| turn the strip | Tap the strip. |

Nothing drags, so a swipe is always a page and a long-press is always the menu. A vertical swipe scrolls the column. On a phone a drag never selects text: the menu copies.

**What Pocket must send.** Today the console gets a tap as a left click and a vertical swipe as the wheel. It gets no sideways swipe, no long-press and no release. So the app's terminal view gets one small change, in `ios/`: a sideways swipe sends `←` or `→`, and a long-press sends a right click at its cell. Until the app has it, every act still has a way: the page marks are taps, the belt has `←` `→`, and `m` opens the menu on the chosen card. In a narrowed chain a sideways swipe is the next column, not the next chain as #326 has it; `]` and `[` change the chain.

### The belt on Pocket

On Pocket the console draws no key line and no hairline over it. The cards take the two rows. The keys are on the app's belt (`docs/phone.md`, "The key belt"), on a fifth layout named **Board**. It is a fifth match in the matchbox tray.

- **It follows the console.** The app is on Board while the console shows the board or Machine, as Commands follows the agent whose pane the owner is in. When the owner goes into a pane, the belt goes back to the layout it had there. The owner can still choose another layout from the tray.
- **Home** is `◂`, then the keys of the chosen card in the accent (at most three, each with its word under it: `1 answer`, `2 answer`, or `p merge`, or `y go`), then `↑ ↓ ⏎` (owner, 2026-11-11). So what the owner does most needs no drag, and `◂` and `↑ ↓ ⏎` are where every layout has them. The tail, in this order: `c copy`, `m menu`, `g chain`, `v one line`, `s hosts`, `u strip`, `⇥ machine`, `x kill`, `←`, `→`, `]`, `[`, `/`, `z park`. Every key of the board is on the belt.
- **A key that does not apply is dim** and sends nothing, as a command key is in a pane that is not its agent's.
- **The consent box** still rises in the console, with its `RUN` line. While it shows, the belt is `◂`, `y runs`, `esc cancels`. A tap is never consent: `y` on the belt is the typed `y`.
- **A go: arm, then send** (owner, 2026-11-13). The console cannot tell the belt's `y` from a typed one, so on Pocket the first `y` on a card whose stoker step waits for a go only arms it. The card's key line then says what the go does (`y again: merge #4556 · esc clears`), and the belt lights `y`. A second `y` gives the go. Any other key or tap disarms it, as does the end of its notice. A desk's typed `y` still gives the go at once.
- **What the console tells the app.** The app must know the chosen card's keys. The console sends them when they change, the way it tells the app today which agent a pane runs (`@matchblox_agent`, `internal/panes/home.go`) and that it is at the top (the terminal title, #326). The builder picks the one channel that is already there; no new protocol.
- **A terminal that is not Pocket** (an SSH app, a narrow tmux pane) keeps the key line, because it has no belt. The console knows Pocket by what the app says when it connects.

### Layout

The console's shape rule decides (DESIGN.md §6, Widths; `layout()` in `internal/app/layout.go`). Three shapes, with today's limits:

| Shape | The board |
|---|---|
| Narrow (portrait under 90 columns, or under 60) | One column a page. The top line is the page marks: the open column in capitals with its count and `−`, the others short (`need 4  WORKING 3 −  ship 3  quiet 5`). No tab row; `Tab` and the key line reach Machine. |
| Medium (60 to 99, and portrait from 90) | `NEEDS YOU`, `WORKING` and `SHIPPING` stacked from top to bottom, each the full width; `WORKING` and `SHIPPING` one row a match, the chosen one opened (owner, 2026-11-14; see `2026-11-14-stacked-board-design.md`). `QUIET` is a drawer under them: one line with the names, which opens to six rows when the selection moves into it. |
| Wide (100 and over) | Four columns at 36, 25, 25 and 18 of 104, scaled to the width up to 151 columns. From 152 columns the columns keep those widths and the chosen match's live view takes the rest at the right (at least 44 columns), as #346 draws it. |

A hidden column gives its room to the others in the same proportions. On Wide the live view takes it. A column whose cards do not fit ends in `↓ N more`, and scrolls so that the chosen card is whole.

The screen, top to bottom: the header (the mark, the host filter, `● N need you`, the stoker word; never a toast), the tab row (`BOARD 15   machine`, the chain crumb, the hidden columns), a hairline, the body, the strip, a hairline, the key line. The setup doors stay above the columns, the ghost row `+ start a new session here` and the recommendations stay under them, each folded to one line when the columns need the rows. `↑` from the first card of a column chooses the doors line, and `↓` from the last card chooses the ghost row, then the recommendations. Each opens in place and takes today's keys.

### The match on the board

DESIGN.md §4 stays the source. The board adds where each motion plays.

| Where | What moves |
|---|---|
| `NEEDS YOU` | The head of each match that asks breathes (2.4 s). Nothing else in the column moves. |
| `WORKING` | The flame flickers, as today. |
| A card that changes column | **Land**, a new row of DESIGN.md §5 (owner, 2026-11-11), 1.2 s. Into `WORKING`: the strike plays (800 ms) and the name is `flame-core`. Into any other column: the going out plays (400 ms) and the card's lines sit on the `wash`, with no fire. The card does not travel across the screen. |
| `SHIPPING` | Still. When a ship passes a stage, one bright cell runs along its track once, as the spark runs down a fuse today. |
| `QUIET` | Nothing. The head cools with age: `match-head` while idle, `match-ember` when cold, `faint` when stale, parked or held. |
| Pocket | The app's haptics (DESIGN.md §5): the rigid impact on a strike. The buzz of §9 for a new wait while the app is closed does not change. |

`--no-motion` shows each end state at once. Motion comes from the clock (§4 rule 4), so the replay tests hold.

## What the reviews changed

The owner approved the sketch of 2026-11-10 (version 7). The reviews of 2026-11-11 changed these points, by the owner's answers. The sketch now shows them (version 8), except where the last table says so:

| The design of 2026-11-10 | The reviewed design |
|---|---|
| A digit or a tap answers at once | The first press arms, the second sends |
| `x` merges or kills | `x` kills, `p` merges |
| The accent on every waiting card | The accent on the chosen card and the count |
| `No match waits for you.` | `nothing needs you` |
| A toast in the header | Notices in the strip's row |
| Belt home `◂ ← → ↑ ↓ ⏎` | Belt home `◂`, the card's keys, `↑ ↓ ⏎` |
| The name glows wherever a card lands | Fire only into `WORKING`; the `wash` elsewhere |
| The peek repeats beside the live view | No repeat from 152 columns |
| The peek watches nothing, and `Seen` is not shown | `Seen` is the sample's time; the question's `Ask.ID` is checked |
| `WORKING` in the sample's order | By the session's start |
| Invented names (`api-auth`, `web-cart`) | The canon's fleet (DESIGN.md §7): `hearth-api`, `flint-auth`, `spark-docs`, `kindle-web`, `ember-billing`, `cinder-ci`, `wick-mobile`, on `ws-1` |

One point the owner kept as it was, against the design review's advice: on a phone a tap on the open page's mark hides that page.

The sketch does not show: the loading state, the lost service, an empty search, a host that is not reached, the 400 ms dead time, the paste-back check's door, `/` and `z`, and more than one chain.

## Machine

The one other tab. A list of rows in five groups. `↑` `↓` choose, `⏎` opens a row over the body, `esc` comes back. `s` narrows to one host here too.

| Group | Row | Opened, it shows |
|---|---|---|
| `ACCOUNTS` | one for each account: the 5-hour window, the week, what burns most | A chart of the 5-hour window as it filled. A chart of the week by day. The forecast in words. The session that burns most (`⏎` goes to it). The account's folder and its matches. |
| `CONTEXT` | the count of matches, how many are over 70%, how many must compact | A chart of how the fullest match filled over six hours. Then every match, the fullest first, with `x compact when it rests`. |
| `WORK` | the work grid (not a row to open) | — |
| `REPOS` | one for each repository: matches, worktrees, what is safe to remove, behind its remote | A chart of the work in the repository by day. Then its worktrees with today's Git keys: `⏎` shell, `N`, `Space`, `X`, `x`, `r`. |
| `THE MACHINES` | one for each host in the filter: CPU, memory, disk | Charts of CPU and memory over 24 hours (today's History). Disk, the panes (`⏎` lists them, today's Panes), the devices and `+ pair a device`, the detached busy loops (`x` kills). On a host row: add or remove a host. |

The tab reads `machine !` in `neg` while a detached busy loop burns, as today.

**Charts.** One value a column, in eighth-blocks (`▁▂▃▄▅▆▇█`), three or four rows tall, with the top and the bottom of the scale at the left in `faint` and the ends of the time axis under it. A chart takes one token: `muted`, or `warn` and `neg` by the dock's own limits. A value that was not measured is a gap, never a zero.

**The work grid.** One cell a day, seven rows (Monday to Sunday), a column a week: 16 weeks on a phone, 30 on Medium, 52 on Wide. The hue of a cell is the harness that worked most on that day. Its strength (four steps toward the ground) is how much: the minutes that matches burned. A day with no work is `·` in `hair`. A legend line names the hues. The grid follows the host filter.

A day with two harnesses shows one hue. The opened repo and account views are where the mix is exact. This is a known loss, taken for the shape the owner asked for.

**Tokens.** Five new tokens, used on Machine only. They change DESIGN.md §2.1 in the same pull request.

| Token | Dark | Light | Use |
|---|---|---|---|
| `harness-claude` | `#D4896A` | `#B5623F` | a day that Claude Code led |
| `harness-codex` | `#7FB69A` | `#3F8A66` | a day that Codex led |
| `harness-opencode` | `#9F8FD1` | `#6E5BB5` | a day that OpenCode led |
| `harness-other` | `#6FA8C7` | `#3B7FA3` | a day that Cursor or another agent led |
| (none) | `faint` | `faint` | an agent the console could not name |

The values are a start. The design review may tune them. Each must pass the replay's token test, and each must stay apart from `flame`, `match-head`, `accent` and `night`.

## Data

| What | Source | State |
|---|---|---|
| The board, the peek, the strip's usage and machine faces | the service's sample, as today | exists |
| The account charts | `limits.jsonl`: five weeks of readings, Claude Code and Codex | exists |
| The machine charts | the 24-hour history | exists |
| The work grid, the strip's week face, the repo chart | **new:** a daily record | to build |
| The context chart | **new:** the same record, or the samples the service already holds for six hours | to decide in the plan |

**The daily record.** One append-only file in the state directory, `work.jsonl`. The service adds to the day's line as it samples: for each host, repository and harness, the minutes that a match burned. It is on this machine only, kept for 53 weeks, and `docs/privacy.md` names it. A day before the record began is `·`. Nothing is back-filled from transcripts.

## The chain room

This design changes two parts of #326's approved design. The owner knows (2026-11-10). matchblox-chain must agree before either session builds them.

| #326 says | This design says |
|---|---|
| The console opens on a list of chains. | The console opens on the board. The chain list does not exist. |
| A room has the need line, the card, the matches, SHIPS, GIT and the talk line. | A room is the board narrowed to the chain, with the card above the columns and the talk line under them. |
| The tabs are `chains · machine`. | The tabs are `board · machine`. |
| Machine holds usage, CPU, history and panes. | The same, plus accounts, context, the work grid and repos, each with a view to open. |
| `1` and `2` open the two tabs. | `Tab` changes the tab. The digits answer questions only. |
| In a room, a sideways swipe is the next chain. | A sideways swipe is the next column. `]` and `[` change the chain. |

What stays #326's, untouched: the stoker's card and its plan, the talk line and typing, the flip (`⌥s`, `⌥n`, prefix `s`; prefix `n` stays tmux's next window), the back ladder, waking a stoker with no sheet, the removal of the groupings, the phone's Live Activity and the buzz.

The split of the work, to agree:

- **matchblox-chain** builds the card, the talk line, the flip, the back ladder and the removal of the Ships, Git, History, Panes and Stoker tabs. Those five removals are not tasks of this work unless #326 lands without them.
- **matchblox-board** builds the board, the peek, the filters, the strip, the menu, copy, Machine and its views.
- The file that both need is the home's body (`internal/app/sessions.go` today). The one that lands second rebases.

If #326 lands as approved before this is agreed, this design still applies: the board replaces the chain list in a later pull request, and the room's layout changes then.

## What goes

Each is its own commit, so that it can be reverted alone. A removal that #326 has made is not made again.

| Part | Why | Where its use went |
|---|---|---|
| The dock (usage, the machine, the stoker panels) | The strip and Machine hold it. | the strip, Machine |
| `u` as the dock's cycle, `v` as list only | The dock and the detail pane go. | `u` the strip, `v` the density |
| `s`, `S` (sort) and `g` (group) | The columns are the grouping, and their order is fixed. | — |
| `1-7`, `p`, `c` as tab keys | Two tabs. | `Tab` |
| The Ships tab | The ship is on its card, and `x` merges there. | the card |
| The Git tab | It is a repo's view on Machine. | Machine |
| The History and Panes tabs | They are a machine's view on Machine. | Machine |
| The Stoker tab | The step is on the card, and the card is in the room. | the card, the room |
| `Esc` to Hosts | A host is a filter. | `s`, Machine |

`matchblox ships`, `matchblox usage` and every other command stay.

## Docs, the canon and the site

- `DESIGN.md`: §2.1 (the tokens), §4 rule 6 (the board is a place that shows the match), §5 (no new motion: the table says where each plays), §6 (the screen, the board, the card, the strip, the widths, Machine). The 2026-11-01 note about Sessions is replaced.
- `README.md`, `docs/index.md`, `docs/start.md`, `docs/console.md`, `docs/keys.md`, `docs/ships.md`, `docs/stoker.md`, `docs/phone.md`, `docs/remote.md`: each place that names a tab, the dock, a key that changed or "waits for you" (owner, 2026-11-11: the README, the docs, the landing page and the videos match the board when it merges).
- **The replays** (the moving pictures of the README and the site: `www/demo/replay-desk.svg`, `replay-phone.svg`, the frames and the still in `www/index.html`) are recorded again from a script that shows the board: one gold card, an answer armed and sent, a strike, a merge with `p`, a hidden column, a narrowed chain.
- **The app's demo** and its words in `docs/phone.md` follow the board and arm then send.
- `www/`: matchbloxrelease owns it. This session sends it a brief for each change that the landing page shows (the replay of the home, the tab names, the keys). The feat pull request carries the `www/` change before it merges.
- `CHANGELOG.md`: the keys that changed their job are named as breaking for the hand, not for scripts.

## Tests

- **A fixed fleet.** One fixture of 15 sessions with the canon's names (DESIGN.md §7: the Hearth fleet on `ws-1`, and a second host `gpu-1`), in the sketch's shape ( 4 need the owner, 3 work, 3 ship, 5 are quiet, one chain of 3, two hosts). Goldens of the board at 40×34, 80×31 and 160×38, in both densities, with each column hidden, narrowed to the chain, and narrowed to a host.
- **Every row is as wide as its screen**, in each golden. The sketch's own check does this.
- **The column rule:** a table test, one case for each line of the Columns table, and one for each pair that could clash (held and asks, busy and a stuck ship).
- **The guards:** a digit within 400 ms of a new peek is not sent; a digit after a newer question is refused; a tap never runs a destructive act.
- **The count:** with `NEEDS YOU` hidden, a chain filter and a host filter all on, the header still counts every wait, and a tap on it clears all three.
- **A host that is not reached** keeps its cards, in `faint`, with `as of`.
- **Copy:** the console's `clip` gets the exact text for each menu line and for a drag. With `set-clipboard off` or `external` in the console's tmux, the copy says the fix and does not say `copied`.
- **Motion:** the replay draws the strike and the going out on the card where it lands, from the clock.
- **The record:** a day's line sums a host, a repository and a harness; a missing day draws `·`; a chart draws a gap for a value that was not measured.
- `go test ./...` passes, and CI is green on Linux and macOS.

## Not verified

- OSC 52 from this host through the owner's laptop and a tmux on it. It is a test in the plan, on the owner's real path, before any claim.
- That Pocket takes OSC 52 into the iOS clipboard.
- That a right-click, a release and a motion reach the console through tmux. The console handles only the left click and the wheel today, so it must ask the terminal for button and motion events, and that is new.
- The sketch was checked by its renderer (every row as wide as its screen, in each state). It was not looked at in a browser by its author.
- The harness token values, against the replay's token test and in a light terminal.

## Left to the builder

- The exact widths between the three shapes, from the shape rule.
- How the setup doors, the ghost row and the recommendations fold when the columns need the rows.
- Whether the context chart reads the daily record or the samples the service already holds.
- The words of each toast and refusal, by DESIGN.md §7.

## The order of the work

1. **The board** (the home, the card, the peek, the densities, hiding, the chain and host filters, the strip's usage and machine faces, the menu, copy, the motion, the harness mark), with the removals as their own commits, the docs and the site.
2. **Machine** and its views, with the charts that today's data can draw (accounts, machines), and the repo and context views without their charts.
3. **The daily record**, the work grid, the strip's week face, the repo and context charts, and the tokens.

Each is one pull request. #372 closes with the first; the second and the third are issues of their own, opened when the plan is approved.
