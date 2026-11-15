# Keys

Every key of the console works on the key bar of a phone SSH app: letters,
digits, arrows, `Enter` and `Esc`. No key of the console needs `Ctrl`.
`Ctrl+O` is for a hardware keyboard. On a phone,
`◂` on the app's key belt (or its edge swipe) goes back, `^C` on the belt
interrupts, and a swipe scrolls. The key line at
the bottom shows the keys of the open tab. A tap on a key presses it.

## The board

The key line shows the keys of the chosen card first, then `c` and `s`,
then `Tab`. A key that does not fit leaves whole, from
the right.

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Chooses the card above or below. The chosen card is the peek. From a column's first card, `↑` chooses the setup steps above the board; from its last card, `↓` chooses the ghost row, then the recommendations. |
| `Enter` or `a` | Goes into the match's pane. On a ship that no session drives, opens New session in its worktree. On a setup step, the ghost row or a recommendation, shows its action. On a phone, a tap on the chosen card does it too. |
| `Ctrl+O` | Goes into the chosen agent's pane, from every tab. |
| A digit, 1 to 9 | Marks that option of the chosen card's question. The same digit again sends it. Another digit moves the mark, and `Esc` clears it. See [the peek](console.md#the-peek). |
| `p` | Merges the card's leading pull request when it waits for your merge: a box shows `gh pr merge <n> --squash --match-head-commit <sha>`, and it runs on a typed `y`. On any other card it says `no ship of <name> waits for a merge`. |
| `x` | Kills the match: a box shows `tmux kill-pane -t <pane>`, then `kill -TERM <pid>` for an agent still running, and runs them on a typed `y`. A session that works or needs you asks for a second `y`. Its worktree and its transcript stay. On a setup step, closes it; on a recommendation, shows its second action. |
| `y` | Gives the go that a stoker step waits for. On Pocket the first `y` only arms it, and the card says what it does (`y again: merge #4556`). A second `y` gives it. |
| `c` | Copies what the agent said last. |
| `m` | Opens the card's menu: copy, show its chain, go in. |
| `g` | Shows only the chain of the chosen card: its room. `Esc` shows all. |
| `]`, `[` | Shows the next or the previous chain. |
| `s` | Shows the next host, then all hosts. |
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
| `⌥s` | Opens the room of the chosen card's chain, as `g` does. A session in no chain opens its chain of one, which the host's stoker drives. |
| `Esc` | Closes the menu, then clears a marked answer, then the search, then the chain. It never leaves the console, except on a host that has not answered yet: then it goes back to Hosts. `Ctrl+M`, prefix `m` and the phone's back button do the same in the console. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |
| `R` | Starts the console again on the binary the service runs. The key line offers it first, `R start again · service v0.43.0, console v0.42.0`, when the service on this machine runs a newer release than the console and the console's own binary was installed over. Otherwise the line says `quit and reopen`. |

A key that changed its job with the board says so once, the first time
you press it: `c`, `s`, `g`, `p`, `x` on a card whose ship waits for its
merge, and a digit.

## The list and the chat

The keys go to one of two places: the chat's line or the list. The key
line names the place first, `writing to flint-auth` or `in the list`, then
at most five keys for it.

In the line, every letter types, `j`, `k` and `q` too. A paste goes in as
one piece. A digit answers only with the line empty and a question on
the screen; else it types.

| Key | What it does |
|---|---|
| `Enter` | In the line, sends the text to the session the line names, through its own host. In the list, goes back to the line. |
| `Backspace` | Erases the last character of the line. |
| A digit, 1 to 9 | With the line empty, marks that option of the question on the screen. The same digit again sends it. A question that opened less than half a second ago takes no digit: the digit types. On a session the stoker holds, 1 marks "Let it go", and 1 again lets it go. |
| `↑`, `↓` | With the line empty, or in the list, chooses the session above or below, as the list draws them. In the list, `↓` past the last row chooses the parts of the foot one by one, and `↑` goes back up. `Enter` opens the chosen part. |
| `Esc` | In the line, clears a marked answer, then moves the keys to the list. In the list, clears the search, then closes the chain's room, then shows all chains. |
| `s` | In the list, shows the next host, then all hosts. It goes to each host, a hidden host too. With one host shown, all hosts is that host, so `s` goes from host to host. With one host only, a notice says so. |
| `h` | In the list, on a hidden host, shows that host in all hosts again. Machine › Hosts then ticks the host. On another host, a notice says what `h` does. |
| `g` | In the list, shows only the chain of the chosen session. `Esc` shows all. |
| `/` | In the list, searches the list. |
| `m` | In the list, opens the menu of the chosen session. |
| `c` | In the list, copies what the agent said last. |
| `z` | In the list, parks the chosen session, or lets it go. In the line of a parked session, lets it go. |
| `N` | In the list, opens **New session**. |
| `Tab` | Goes to Machine. |
| `Ctrl+O` | Goes into the chosen agent's pane, from the line or the list. The key line then says the way back, `ctrl+a m`. |
| `Ctrl+L` | Hides the list, or shows it again. With the list hidden, the chat takes the width, and the header names each session that needs you. A tap on a name opens its chat. On a phone, the list and the chat take turns. |
| `◂` or `←` | On a phone, goes from the chat back to the list. A tap on `◂` at the left of the chat's head does it too. On a desk, `←` in the line moves the keys to the list, as `◂` does on a phone. |
| `e` | In the list, shows the agents that exited and wait for their parent, dim at the end of `IDLE`, each with the word `exited`. `e` again hides them. The `IDLE` count counts only the sessions that run. While they are hidden, the foot counts them: `· 1 exited`. |

On a desk the list is 40 columns wide, with the chat beside it. On a
narrower screen the list is 32 columns wide. A phone, and a screen that
leaves the chat less than 40 columns, shows one of the two at a time: the
list first. A tap on a row, or `Enter` in the list, opens its chat. A tap
on a group's head folds the group: the head keeps its count, and its `−`
turns to a plus sign. A tap on the head again unfolds it. The foot opens
each of its parts in the chat's place, beside the list: `+ new` opens
**New session**, `✧ 2 tips` the tips, and `! 1 setup` the first open
setup step. `Esc` gives the place back to the chat. A notice shows over
the lines above the chat's line and the foot, and a tap on it does
nothing. In the tips, `↑` `↓` choose, `Enter` does the chosen tip and `d`
dismisses it. A drag in the chat copies only the chat's text.

A key with no job in the line or the list does not do nothing: a notice
names the keys that place takes. Any other letter in the list moves the keys back to the line and types
there. The line of a parked session is off: it says
`parked: z lets it go`. A message to a host that is not reached stays in the line, and the
line says `not sent: ws-2 is not reached`. Nothing sends it again by
itself. The rows of that host stay, dimmed. On that host, the header says
`ws-2 not reached · since 2m`. When the chosen session ends, the next session in the list is
chosen, and its line says `flint-auth ended · the text is not sent`.

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

## Go in

A second tap on the chosen card goes into its pane. A keyboard never
changes the layout: the shape goes by the tallest height seen at the
width.

| Key | What it does |
|---|---|
| `Enter` or `a` | Goes into the chosen agent's tmux pane (help `go in`). A second tap on the chosen card does it too. |
| `ctrl+a m` | In the iOS demo, which has no tmux, `Enter` opens a stand-in for the pane: the agent's screen over the whole screen, and every key goes to the agent. Its title is `‹ board`. `ctrl+a`, then `m` within 1 s, goes back to the board. |

The mouse wheel over the stand-in scrolls it back through the pane's
history, 3 lines for each wheel step. The key line says how far back you
are: `↑ 12 lines back`. Scrolling only looks: it types nothing into the
pane.

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
| `Space` | Shows or hides the host. All hosts leaves a hidden host out, and the header counts it. `s` still goes to it. |
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
that page. In Matchblox
Pocket, a sideways swipe turns the page too.

| The owner wants to | By hand |
|---|---|
| see the next column | Swipe sideways, or tap the column's mark at the top. |
| peek | Tap a card. |
| go in | Tap the chosen card again. |
| answer | Tap an option in the peek, then tap it again to send it. |
| open the menu | Long-press a card, or press `m`. |
| act | Tap a key on the card or on the key line. A destructive act still waits for a typed `y`. |
| come back | The belt's `◂`, or the edge swipe. |

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
