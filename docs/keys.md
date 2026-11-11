# Keys

Every key of the console works on the key bar of a phone SSH app: letters,
digits, arrows, `Enter` and `Esc`. No key of the console needs `Ctrl`.
`Ctrl+O` is for a hardware keyboard. On a phone,
`◂` on the app's key belt (or its edge swipe) goes back, `^C` on the belt
interrupts, and a swipe scrolls. The key line at
the bottom shows the keys of the open tab. A tap on a key presses it.

## The console

| Key | What it does |
|---|---|
| `↑` or `k` | Selects the row above. |
| `↓` or `j` | Selects the row below. |
| `Enter` | Shows the safe action of the row: go into the agent's pane (Sessions, Machine, Panes), open New session on the folder of `+ start a new session` (Sessions), open a shell in the worktree (Git), do a recommendation, show a setup step. On Ships, selects the session that drives the ship in Sessions. |
| `x` | Shows the other action of the row: the second action of a recommendation, close a setup step or kill the session (Sessions), kill a detached busy loop (Machine), remove the marked worktrees (Git), merge a ship that waits for your merge (Ships). Kill shows its commands, `tmux kill-pane -t <pane>`, then `kill -TERM <pid>` for an agent still running, and runs them on a typed `y`. A session that works or waits for you asks for a second `y`. Its worktree and its transcript stay. |
| `N` | Opens **New session** (Sessions, Git): where it starts (a path, or `←` `→` through the Git tab's repositories and worktrees, then the ones found on the host, the most recent first), a branch for a new worktree beside the repository (blank: none), its name, the agent (`←` `→`, from the installed Claude Code, Codex, OpenCode and Cursor), for Claude Code, Codex and Cursor the account (see below), and a first message (blank: it waits for you; a new account has none). `Tab` goes to the next line, `⏎` starts it in a tmux session of its own, `Esc` sends nothing. The row shows in Sessions within seconds. |
| `r` | Renames the session (Sessions): its tmux window, its tmux session when it is that session's one window, and Claude Code's own name (`/rename`), all alike. A Claude Code session that works takes its new name when it rests. On Git, `r` scans the git checkouts again. |
| `A` | Switches the agent of the session (Sessions): choose another installed agent (`←` `→`), how it starts (`←` `→` or a tap: **continue** from the handover, or **no task**), and any extra instructions for this switch. The confirm names the switch, its account and its launch settings. A session that works or waits needs a second `y`. A switch that continues first asks an idle Claude Code or Codex agent to write its task record and waits up to 3 minutes, showing the wait in the bottom rows; `esc` switches now, from the evidence. It then saves the latest evidence before replacing the agent. The console jumps to the pane after. |
| `a` or `→` | Goes into the selected agent's pane (Sessions). `→` does it where it has no other use. On Ships, goes into the pane of the session of a stuck ship. On the head row of a chain, goes into the pane of the chain's stoker. |
| `1-7` | Opens a tab: Sessions, Ships, Machine, Git, History, Panes, Stoker. |
| `p` | Opens Ships. |
| `Tab` or `Shift+Tab` | Opens the next or the previous tab. |
| `u` | Cycles the dock of a narrow screen: usage, the machine, the stoker. Where Sessions splits, all three stack under the list, and the machine folds before the list is cut; while a run is in charge, its stoker keeps its place as usage does, and the machine folds first. While a run is in charge, or has steps you have not seen, the stoker's panel takes the dock until you press `u`. |
| `v` | Folds the detail pane and the dock, so the list takes the screen. The selected row opens in place. The rows the list leaves free show usage, then the machine, each whole at its fewest lines; the list is never cut for them. `v` again shows them. A phone and a desk each keep their own choice. A phone starts with list only. |
| `c` | Opens the STOKER tab: your words and the gos, the plan with each step's outcome, the goes that wait for you, the triggers, the standing rules, today's compacts with their RESUME lines. Opening it marks the steps seen. |
| `/` | Searches the open tab. |
| `s` | Sorts Sessions and Panes by the next column. |
| `S` | Turns the sort around. |
| `g` | Groups Sessions by need, by state, by repo, or not at all. The sort works inside each group. |
| `z` | Parks the selected session (Sessions): nothing types into it, and the stoker leaves it alone. `z` again lets it go. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `f` | Turns [the stoker](stoker.md) off while it is on. While it rests, turns it back on. Otherwise opens the launch sheet for a day run. |
| `F` | Stokes the chain of the selected row (Sessions): the start sheet, `Stoke the chain <name> (<n> matches)?`, takes the order and the gos of a stoker for that chain. On a chain with a stoker, `F` does what `a` does on its head row. On a desk, a tap on the fuse does the same. See [A family of stokers](stoker.md#a-family-of-stokers). |
| `n` | Opens the launch sheet for a night: the stoker runs until the morning, then turns itself off. While a night runs, `n` ends it. |
| `o` | Gives a new order while a day run is on: the launch sheet opens with the run's gos, and the order goes to the stoker as one line. The run goes on, and done steps stay done. A night is sealed: `n` stops it first. |
| `e` | Shows or hides the agents that exited and wait for their parent (Sessions). |
| `d` | Dismisses the selected recommendation until its evidence changes (Sessions). |
| `h` | Folds the recommendations to one line, or shows them again (Sessions). |
| `Esc` | Clears the search. With nothing to clear, goes back to Hosts. `Ctrl+M`, prefix `m` and the phone's back button do the same in the console. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |

## The STOKER tab

`c` opens it, and so does its digit. The first row is the stoker agent,
which matchblox runs on a tmux server of its own. Every other row is a
step the stoker took or planned, a command that waits for your go, or a
trigger that fired. The key line shows the keys of the selected row.
With [chain stokers](stoker.md#a-family-of-stokers), a `CHAIN <name>` card
for each one follows the host stoker's card.

| Key | What it does |
|---|---|
| `Enter` | On the stoker agent's row, opens the stoker (`⏎ open the stoker`): inside tmux in a popup, `tmux display-popup -E -w 90% -h 90% 'tmux -L matchblox attach -t stoker'`, which detaching (prefix `d`) closes; outside tmux the console names the command to run. This is where you answer the stoker's own prompts. On a `CHAIN <name>` card, opens the pane of that chain's stoker. On a step, shows the exact command it ran, or the one it waits to run, or hides it again. |
| `Q` | Quits the stoker agent and its tmux session, after a typed `y`: `tmux -L matchblox kill-session -t =stoker`. `f` only turns the stoker off and leaves the agent running, so its context carries to the next order. On a `CHAIN <name>` card, `Q` and a typed `y` quit the stoker of that chain only. |
| `y` | Gives the go a step waits for (`⏸ waits for you: api-gw wants to run gh pr merge 4556`). The go is good for one command of that kind in that session, and the stoker hears of it at its next wake. |
| `a` | On a failed step (`✕`), goes into its session's pane. |

## An action before it runs

The key line shows `RUN` and the exact command.

| Key | What it does |
|---|---|
| `Enter` | Runs a safe action. |
| `y` | Runs a destructive action. `Enter` never does. |
| `y`, `n` | Answer "Take over?" while the stoker runs: `y` rests it while you work, `n` keeps it on. |
| `y`, `n`, `p` | Answer "Find the Git repositories in your home folder?", asked once per host: `y` finds them for N, `n` never asks again, `p` picks the folders. `Esc` asks at the next start. The box takes no answer for a moment after it shows, so a key meant for what was there before does not answer it. After a yes the line under it says how many it found, that it found none, why the scan stopped early, how many folders it could not read, or that the list could not be saved, and names the folders it skipped. The box is not drawn over a confirm or any other question. |
| `↓` or `j` | Scrolls a long change. `y` works only after its last line was on the screen. |
| `m` | Turns the choice of a setup step on or off: the tmux mouse, in "Add the way back". |
| `Esc` | Cancels. Any other key cancels too. |

## New session, Rename and Switch agent

A tap on a line puts the cursor there, and a tap on an agent picks it.

On where, New session lists your repos, each with its worktrees under it:
SUGGESTED first (a repo with a live session, then where matchblox last
started one, then the repo's last activity), then ALL by name. Typing
filters the list; a paste goes in as typed. A tap on a row picks it. The
other lines show again when you pick a row or leave where.

A git URL on where (`https://…`, `git@host:o/r`, `ssh://…` or `gh:o/r`)
shows clone rows in place of the repos: `start in <path> (already a clone
of o/r)` for a clone you have, and `clone o/r into <folder>`. A bare `o/r`
still filters: the repos that match come first, the clone rows after. A path
from the root, the home or here (`/…`, `~/…`, `./…`) is never a URL.
`Enter` on a clone row starts the clone and goes to branch; where shows
`cloning o/r… 42%` while you fill the rest.

| Key | What it does |
|---|---|
| `Tab` | Goes to the next line. `Shift+Tab` goes back. On where with the list open, picks the selected row first; `Shift+Tab` stays. |
| `↓` or `↑` | On where with the list open, moves through the rows, repos and worktrees. On every other line, goes to the next or the previous line. |
| `←` or `→` | On where, selects the previous or the next repo of the list and shows its path; the list keeps what you typed. On agent, picks the agent before or after; Switch agent leaves out the one the session runs. |
| `Enter` | On where with the list open, picks the selected row and goes to branch; on a clone row, starts the clone. With no repo that matches, takes what you typed as a path. Else starts the session, renames it, or shows the switch's plan; while the clone runs, the session starts when it is done. A refusal stays in the form, with why. |
| `Backspace` | Erases. On a path that `←` or `→` selected, goes back to what you typed. |
| `Esc` | On where with the list open, closes the list and keeps the text as a path. While a clone runs, stops it and deletes its partial folder; the form stays, with the URL in where. Else closes the form. Nothing is sent. Editing where also stops the clone. |

New session shows an account line when the agent keeps accounts: Claude Code, Codex, and Cursor on macOS. A new account adds a setup line.

| Field | What it does |
|---|---|
| account | `←` `→` picks `default`, an account that exists, or `+ new account`. The form opens on the agent you last started, and the field on the account you last started for the agent. If there is none, or the account is gone, it opens on `default`. It never opens on `+ new account`. For a new account, type its name. The name has 1 to 10 lowercase letters, digits or dashes. A new account has no first message. |
| setup | Shows for a new account. `←` `→` picks `like default` (links the default's setup, so every account shares one), `copy of default` (a one-time copy) or `empty` (nothing shared). |

A start on a new account is a destructive action: the plan shows `RUN` and waits for a typed `y`. A start on an account that exists runs on `Enter`.

## The live view

The right side of Sessions is the live view: the selected agent's own
screen, read-only. Sessions splits 40/60 (the list at least 38 columns, at
most 78) when the view gets at least 44 columns and the body has room for
its title, its account line and 16 rows for an agent's menu. On a phone, in
list only, or on a screen on its side with the keyboard up, there is no
view: a tap on a row goes into its pane, and the selected row opens in
place, with the question the agent asks. The view never resizes the agent's window. A keyboard never changes the layout: the shape goes by the
tallest height seen at the width. The view is the console's own host's:
another host's row is list only.

The view takes no keys: to answer or type, go into the agent's pane. The key
line names the way in: `enter goes into <name>`.

| Key | What it does |
|---|---|
| `Enter`, `a` or `→` | Goes into the selected agent's tmux pane (help `go in`). A tap on the view, a second tap on the selected row, or on one column the first tap, does it too. |
| `Ctrl+O` | Goes into the selected agent's tmux pane, from every tab. |
| `ctrl+a m` | In the iOS demo, which has no tmux, `Enter` opens a stand-in for the pane: the agent's screen over the whole screen, and every key goes to the agent. `ctrl+a`, then `m` within 1 s, goes back to Sessions, on the next agent that waits. |

The mouse wheel over the view, or a swipe on a phone, scrolls it back
through the pane's history, 3 lines for each wheel step. The key line says
how far back you are: `↑ 12 lines back`. Scrolling only looks: it types
nothing into the pane.

If the pane is gone, the key line says `That session ended.`

## The launch sheet

`n` and `f` open it, and `o` by day. With no stoker agent running,
matchblox starts it as the sheet opens, and the sheet says `starting the
stoker…` until the agent rests. It asks for the order and the gos you
give, and `Enter` starts the run at once. On the order line every
printable key is text.

| Key | What it does |
|---|---|
| `Enter` | Sends the order, and the run starts. While the stoker agent starts, or is busy mid-turn, the order waits and goes once it rests. By day, after `o`, it goes to the stoker as one line and the run goes on. |
| `Tab` | Takes the cursor to the chips: the gos (merges, releases, deploys), then the recipes of your ways file. |
| `Backspace` | Deletes the last character. |
| `Esc` | Closes the sheet. Nothing is sent. |
| `Ctrl+C` | Quits the console. |

On the chips:

| Key | What it does |
|---|---|
| `→` or `←` | Moves to the next or the previous chip. |
| `Space` | Turns a go chip on or off, or puts a recipe into the order. A go on lets a session the stoker drives merge, release or deploy; off, that command waits for your `y`. A tap on a go chip turns it on or off too. |
| `Tab` or `Esc` | Goes back to the order line. Any other printable key goes back too, and is typed there. |
| `Enter` | Sends the order, as on the order line. |

## Typing a search

| Key | What it does |
|---|---|
| `Enter` | Keeps the search. |
| `Esc` | Cancels. |
| `Backspace` | Deletes the last character. |

## Hosts

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Selects a host. |
| `Space` | Shows or hides the host. A hidden host has no stream, and the header counts it. |
| `Enter` | Opens one console of the ticked hosts, or adds a host. With no host ticked, it opens the selected host. |
| `c` | Connects the host again with `matchblox connect`. |
| `x` | Removes a host from the list. matchblox stays on that host. |
| `y` | Confirms the remove or the connect. |
| `Backspace` | Deletes the last character of a host you type. |
| `Esc` | Leaves the list of hosts to add. |
| `q` or `Ctrl+C` | Quits. |

## A phone

A tap selects a row. A swipe over the list moves the selection. A swipe
over the live view scrolls it back (see [The live view](#the-live-view)). On a
narrow screen, a tap on an agent goes into its pane. Where the view sits beside the
list, a second tap on the selected agent, or a tap on the view, goes into
its pane. On any other row the key line says `tap again: <command>`, and a
second tap on the same row runs it. On Ships, a tap on the selected ship
selects its session in Sessions. Any other action (a
recommendation, a setup step, a destructive step) then shows `RUN` and
waits for `Enter` or a typed `y`, as from the keys. A tap on a tab opens
it, a tap on a key of the key line presses it, and a tap on `● N wait for
you` selects the first agent that waits. A tap on the header's stoker word
(`stoker waits for you` among them) opens the stoker agent, as `Enter` on
its row does. A tap on `machine ›` or `usage ›`
switches the dock. On Panes, a tap on a column header sorts by it.

The app's command keys type a command of the agent into its pane, for
example `/compact`, with no Return: you send it. The console marks the
panes of the agents it knows, and a command key types only into a pane
marked as an agent of the command's kind. Anywhere else it does nothing.

The app's `⇞` and `⇟` keys send `PgUp` and `PgDn`. In a pane you jumped
to on a host, `PgUp` starts tmux copy mode one page up, and `PgDn` moves
one page down in it.
