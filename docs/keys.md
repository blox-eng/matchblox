# Keys

Every key of the console works on the key bar of a phone SSH app: letters,
digits, arrows, `Enter` and `Esc`. No key of the console needs `Ctrl`.
`Ctrl+O` is for a hardware keyboard. On a phone,
`◂` on the app's key belt (or its edge swipe) goes back, `^C` on the belt
interrupts, and a swipe scrolls. The key line at
the bottom shows the keys of the open tab. A tap on a key presses it.

## The board

The key line shows the keys of the chosen card first, then `c`, `v`, `s`
and `u`, then `←→` and `Tab`. A key that does not fit leaves whole, from
the right.

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Chooses the card above or below. The chosen card is the peek. From a column's first card, `↑` chooses the setup steps above the board; from its last card, `↓` chooses the ghost row, then the recommendations. |
| `←` or `→` | Goes to the column at the left or the right, past a hidden or an empty column. Past the last column, shows a hidden column again. On the setup steps' line, moves through the steps. |
| `Enter` or `a` | Goes into the match's pane. On a ship that no session drives, opens New session in its worktree. On a setup step, the ghost row or a recommendation, shows its action. On a phone, a tap on the chosen card does it too. |
| `Ctrl+O` | Goes into the chosen agent's pane, from every tab. |
| A digit, 1 to 9 | Marks that option of the chosen card's question. The same digit again sends it. Another digit moves the mark, and `Esc` clears it. See [the peek](console.md#the-peek). |
| `p` | Merges the card's leading pull request when it waits for your merge: a box shows `gh pr merge <n> --squash --match-head-commit <sha>`, and it runs on a typed `y`. On any other card it says `no ship of <name> waits for a merge`. |
| `x` | Kills the match: a box shows `tmux kill-pane -t <pane>`, then `kill -TERM <pid>` for an agent still running, and runs them on a typed `y`. A session that works or needs you asks for a second `y`. Its worktree and its transcript stay. On a setup step, closes it; on a recommendation, shows its second action. |
| `y` | Gives the go that a stoker step waits for. On Pocket the first `y` only arms it, and the card says what it does (`y again: merge #4556`). A second `y` gives it. |
| `c` | Copies what the agent said last. |
| `m` | Opens the card's menu: copy, show its chain, go in, hide its column. |
| `v` | Shows every card whole, or as one line. A phone and a desk each keep their own choice. |
| `g` | Shows only the chain of the chosen card: its room. `Esc` shows all. |
| `]`, `[` | Shows the next or the previous chain. |
| `s` | Shows the next host, then all hosts. |
| `u` | Turns the strip: usage, the machine, hidden. |
| `Tab` or `Shift+Tab` | Goes to the board, or to Machine. |
| `/` | Searches the board. A search narrows every column. |
| `N` | Opens **New session**: where it starts (a path, or `←` `→` through your repositories and worktrees, then the ones found on the host, the most recent first), a branch for a new worktree beside the repository (blank: none), its name, the agent (`←` `→`, from the installed Claude Code, Codex, OpenCode and Cursor), for Claude Code, Codex and Cursor the account (see below), and a first message (blank: it waits for your first prompt; a new account has none). `Tab` goes to the next line. |
| `r` | Renames the session: its tmux window, its tmux session when it is that session's one window, and Claude Code's own name (`/rename`), all alike. A Claude Code session that works takes its new name when it rests. |
| `A` | Switches the agent of the session: choose another installed agent (`←` `→`), how it starts (`←` `→` or a tap: **continue** from the handover, or **no task**), and any extra instructions for this switch. The confirm names the switch, its account and its launch settings. A session that works or waits needs a second `y`. A switch that continues first asks an idle Claude Code or Codex agent to write its task record and waits up to 3 minutes, showing the wait in the bottom rows; `Esc` switches now. |
| `z` | Parks the chosen session: nothing types into it, and the stoker leaves it alone. Its card goes to `QUIET`. `z` again lets it go. |
| `f` | Turns [the stoker](stoker.md) off while it is on. While it rests, turns it back on. Otherwise opens the launch sheet for a day run. |
| `F` | Stokes the chain of the chosen card: the start sheet, `Stoke the chain <name> (<n> matches)?`, takes the order and the gos of a stoker for that chain. See [A family of stokers](stoker.md#a-family-of-stokers). |
| `n` | Opens the launch sheet for a night: the stoker runs until the morning, then turns itself off. While a night runs, `n` ends it. |
| `o` | Gives a new order while a day run is on: the launch sheet opens with the run's gos, and the order goes to the stoker as one line. The run goes on, and done steps stay done. A night is sealed: `n` stops it first. |
| `e` | Shows or hides the agents that exited and wait for their parent. |
| `d` | Dismisses the chosen recommendation until its evidence changes. |
| `h` | Folds the recommendations to one line, or shows them again. |
| `⌥s` | Opens the room of the chosen card's chain, as `g` does. A session in no chain opens its chain of one, which the host's stoker drives. |
| `Esc` | Closes the menu, then clears a marked answer, then the search, then the chain. It never leaves the console. `Ctrl+M`, prefix `m` and the phone's back button do the same in the console. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |
| `R` | Starts the console again on the binary the service runs. The key line offers it first, `R start again · service v0.43.0, console v0.42.0`, when the service on this machine runs a newer release than the console and the console's own binary was installed over. Otherwise the line says `quit and reopen`. |

A key that changed its job with the board says so once, the first time
you press it: `c`, `v`, `s`, `g`, `u`, `p`, `x` on a card whose ship waits
for its merge, and a digit.

## Machine

`Tab` opens it. The rows are the stoker, git, history and panes, then the
detached busy loops, the devices and, over several hosts, the hosts.

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Chooses the row above or below. |
| `Enter` | Opens the view of the row: the stoker, git, history or panes. On a busy loop, goes to the pane that started it. On `+ pair a device`, opens the pair page. On a host, opens Hosts. On Git, opens a shell in the worktree. On Panes, goes to the pane. |
| `Esc` | Goes back from a view to Machine, on its row. |
| `x` | Kills a detached busy loop, removes a device, or removes the marked worktrees (Git), after a typed `y`. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `r` | Scans the git checkouts again (Git). |
| `s` | Sorts Panes by the next column. |
| `Q` | Quits the stoker agent, on the stoker's view (see below). |

## A chain's room

A room is the board narrowed to one chain: `g`, `⌥s` or a tap on a card's
stub opens it. It adds the stoker's card above the columns and the talk
line under them. The board's keys work on its cards. The key line shows
the keys of the chosen line.

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Chooses the line above or below: the stoker's steps, the cards, the talk line. |
| `Enter` | On a card, goes into the match's pane. On `↓ N more`, unfolds the plan. On the talk line, starts typing. On a quiet stoker's line, starts typing to it. |
| `y` | Gives the go a step waits for. Every go (a merge, a release, a deploy, a kill) acts outside the session: it shows its command first and runs on a typed `y`. |
| `]` | Opens the next chain's room. |
| `[` | Opens the previous chain's room. |
| `⌥s` | Goes back to the session you flipped from. With none, starts typing on the talk line. |
| `⌥n` | Goes into the chain's first match. |
| `Esc` | Stops typing (the words stay). Else closes the room and shows every chain. |
| `Backspace` | Erases the last character while you type. |

`out of <chain>` in a card's menu (`m`) takes the match out of the chain,
after a typed `y`. Its agent goes on, and the chain hears that it left.
The menu has it only while the board shows that chain.

While the talk line types, every key is text until `Enter` sends it to the
chain's stoker or `Esc` stops. The first words to a chain with no stoker
start one, with no launch sheet; a refusal keeps the words in the line.

### The flip from a pane

In an agent's pane, `⌥s` goes to the room of its chain, and `⌥s` there comes
back. `⌥n` walks the chain's matches, then the room, then the first match
again. Each terminal comes back to its own session: a phone and a desk keep
theirs apart. In a shell or an editor the keys type as always: matchblox
binds them only in the panes of the agents it samples.

A terminal that does not send Option as Meta types `ß` for `⌥s`. Use
prefix `s`, or set Option as Meta: in Terminal.app, Settings
→ Profiles → Keyboard → **Use Option as Meta key**; in iTerm2, Settings →
Profiles → Keys → **Left Option key: Esc+**. Ghostty and kitty send it as
Meta.

matchblox moves tmux's session tree from prefix s to prefix S. Prefix n
stays tmux's next window: the walk has only `⌥n`.

## The stoker's view

The row `stoker` on Machine opens it. The first row is the stoker agent,
which matchblox runs on a tmux server of its own. Every other row is a
step the stoker took or planned, a command that waits for your go, or a
trigger that fired. The key line shows the keys of the selected row.
With [chain stokers](stoker.md#a-family-of-stokers), a `CHAIN <name>` card
for each one follows the host stoker's card.

| Key | What it does |
|---|---|
| `Enter` | On the stoker agent's row, opens the stoker (`⏎ open the stoker`): inside tmux in a popup, `tmux display-popup -E -w 90% -h 90% 'tmux -L matchblox attach -t stoker'`, which detaching (prefix `d`) closes; outside tmux the console names the command to run. This is where you answer the stoker's own prompts. On a `CHAIN <name>` card, opens the pane of that chain's stoker. On a step, shows the exact command it ran, or the one it waits to run, or hides it again. |
| `Q` | Quits the stoker agent and its tmux session, after a typed `y`: `tmux -L matchblox kill-session -t =stoker`. `f` only turns the stoker off and leaves the agent running, so its context carries to the next order. On a `CHAIN <name>` card, `Q` and a typed `y` quit the stoker of that chain only. |
| `y` | Gives the go a step waits for (`⏸ waits for your go: api-gw wants to run gh pr merge 4556`). The go is good for one command of that kind in that session, and the stoker hears of it at its next wake. |
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

From 152 columns, the right side of the board is the live view: the chosen
agent's own screen, read-only. Under 152 columns there is no view: a
second tap on the chosen card goes into its pane. The view never resizes
the agent's window. A keyboard never changes the layout: the shape goes by
the tallest height seen at the width. The view shows only a session of
the console's own host.

The view takes no keys: to answer or type, go into the agent's pane.

| Key | What it does |
|---|---|
| `Enter` or `a` | Goes into the chosen agent's tmux pane (help `go in`). A tap on the view, or a second tap on the chosen card, does it too. |
| `ctrl+a m` | In the iOS demo, which has no tmux, `Enter` opens a stand-in for the pane: the agent's screen over the whole screen, and every key goes to the agent. Its title is `‹ board`. `ctrl+a`, then `m` within 1 s, goes back to the board. |

The mouse wheel over the view scrolls it back through the pane's history,
3 lines for each wheel step. The key line says how far back you are:
`↑ 12 lines back`. Scrolling only looks: it types nothing into the pane.

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
| `u` | Updates a host on an older release than the console. The host looks for the latest release and installs it, even with `mode = "off"`. |
| `d` | Dismisses the note of a host on another release, until either release changes. |
| `y` | Confirms the remove, the connect or the update. |
| `Backspace` | Deletes the last character of a host you type. |
| `Esc` | Leaves the list of hosts to add. |
| `q` or `Ctrl+C` | Quits. |

## A phone

The board is one column a page. A tap on a page's mark at the top opens
that page, and a tap on the open page's mark hides it. In Matchblox
Pocket, a sideways swipe turns the page too.

| The owner wants to | By hand |
|---|---|
| see the next column | Swipe sideways, or tap the column's mark at the top. |
| hide a column | Tap the name of the column on the screen. Tap `+ name` to bring it back. |
| peek | Tap a card. |
| go in | Tap the chosen card again. |
| answer | Tap an option in the peek, then tap it again to send it. |
| open the menu | Long-press a card, or press `m`. |
| act | Tap a key on the card or on the key line. A destructive act still waits for a typed `y`. |
| come back | The belt's `◂`, or the edge swipe. |
| turn the strip | Tap the strip. |

A vertical swipe scrolls the column. On a phone a drag never selects text:
the menu copies. A tap is never consent: a tap on a card's `y go` marks
the go, and a second tap gives it. A tap on a key of the key line presses
it, and a tap on `● N need you` shows `NEEDS YOU` and chooses the first
card that needs you. A tap on the header's stoker word opens the stoker
agent. On Panes, a tap on a column header sorts by it.

A long-press and a sideways swipe need Matchblox Pocket, or a terminal
that reports the release of a press. Elsewhere, `m` opens the menu and
`←` `→` turn the page.

The app's command keys type a command of the agent into its pane, for
example `/compact`, with no Return: you send it. The console marks the
panes of the agents it knows, and a command key types only into a pane
marked as an agent of the command's kind. Anywhere else it does nothing.

The app's `⇞` and `⇟` keys send `PgUp` and `PgDn`. In a pane you jumped
to on a host, `PgUp` starts tmux copy mode one page up, and `PgDn` moves
one page down in it.
