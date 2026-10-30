# Keys

Every key of the console works on the key bar of a phone SSH app: letters,
digits, arrows, `Enter` and `Esc`. No key of the console needs `Ctrl`.
`Ctrl+]` and `Ctrl+O` in the chat are for a hardware keyboard. On a phone,
the app's edge swipe goes back, and its key bar sends `^C`. The key line at
the bottom shows the keys of the open tab. A tap on a key presses it.

## The console

| Key | What it does |
|---|---|
| `↑` or `k` | Selects the row above. |
| `↓` or `j` | Selects the row below. |
| `Enter` | Shows the safe action of the row: give the chat the keys (Sessions), go to the agent's pane (Machine, Panes), open New session on the folder of `+ start a new session` (Sessions), open a shell in the worktree (Git), do a recommendation, show a setup step. On Ships, selects the session that drives the ship in Sessions. |
| `x` | Shows the other action of the row: the second action of a recommendation, close a setup step or kill the session (Sessions), kill a detached busy loop (Machine), remove the marked worktrees (Git), merge a ship that waits for your merge (Ships). Kill shows its commands, `tmux kill-pane -t <pane>`, then `kill -TERM <pid>` for an agent still running, and runs them on a typed `y`. A session that works or waits for you asks for a second `y`. Its worktree and its transcript stay. |
| `N` | Opens **New session** (Sessions, Git): where it starts (a path, or `←` `→` through the Git tab's repositories and worktrees, then the ones found on the host, the most recent first), a branch for a new worktree beside the repository (blank: none), its name, the agent (`←` `→`, from the installed Claude Code, Codex, OpenCode and Cursor), for Claude Code, Codex and Cursor the account (see below), and a first message (blank: it waits for you; a new account has none). `Tab` goes to the next line, `⏎` starts it in a tmux session of its own, `Esc` sends nothing. The row shows in Sessions within seconds. |
| `r` | Renames the session (Sessions): its tmux window, its tmux session when it is that session's one window, and Claude Code's own name (`/rename`), all alike. A Claude Code session that works takes its new name when it rests. On Git, `r` scans the git checkouts again. |
| `A` | Switches the agent of the session (Sessions): choose another installed agent (`←` `→`) and edit or clear its handoff. The plan names the account and launch settings. A nonempty handoff loads an evidence preview before `y` can run the switch. A session that works or waits needs a second `y`. The switch saves the latest evidence before replacing the agent. The console jumps to the pane after. |
| `a` or `→` | Gives the chat the keys on the selected agent (Sessions). `→` does it where it has no other use. The key line says `a chat`. On Ships, opens the chat on the session of a stuck ship. |
| `1-7` | Opens a tab: Sessions, Ships, Machine, Git, History, Panes, Stoker. |
| `p` | Opens Ships. |
| `Tab` or `Shift+Tab` | Opens the next or the previous tab. |
| `u` | Cycles the dock of a narrow screen: usage, the machine, the stoker. A wide screen stacks all three under the list. While a run is in charge, or has steps you have not seen, the stoker's panel takes the dock until you press `u`. |
| `c` | Opens the STOKER tab: your words and the gos, the plan with each step's outcome, the goes that wait for you, the triggers, the standing rules, today's compacts with their RESUME lines. Opening it marks the steps seen. |
| `/` | Searches the open tab. |
| `s` | Sorts Sessions and Panes by the next column. |
| `S` | Turns the sort around. |
| `g` | Groups Sessions by need, by state, by repo, or not at all. The sort works inside each group. |
| `z` | Parks the selected session (Sessions): nothing types into it, and the stoker leaves it alone. `z` again lets it go. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `f` | Turns [the stoker](stoker.md) off while it is on. While it rests, turns it back on. Otherwise opens the launch sheet for a day run. |
| `n` | Opens the launch sheet for a night: the stoker runs until the morning, then turns itself off. While a night runs, `n` ends it. |
| `o` | Gives a new order while a day run is on: the launch sheet opens with the run's gos, and the order goes to the stoker as one line. The run goes on, and done steps stay done. A night is sealed: `n` stops it first. |
| `e` | Shows or hides the agents that exited and wait for their parent (Sessions). |
| `d` | Dismisses the selected recommendation until its evidence changes (Sessions). |
| `h` | Folds the recommendations to one line, or shows them again (Sessions). |
| `Esc` | Clears the search. With nothing to clear, goes back to Hosts. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |

## The STOKER tab

`c` opens it, and so does its digit. The first row is the stoker agent,
which matchblox runs on a tmux server of its own. Every other row is a
step the stoker took or planned, a command that waits for your go, or a
trigger that fired. The key line shows the keys of the selected row.

| Key | What it does |
|---|---|
| `Enter` | On the stoker agent's row, opens the stoker (`⏎ open the stoker`): inside tmux in a popup, `tmux display-popup -E -w 90% -h 90% 'tmux -L matchblox attach -t stoker'`, which detaching (prefix `d`) closes; outside tmux the console names the command to run. This is where you answer the stoker's own prompts. On a step, shows the exact command it ran, or the one it waits to run, or hides it again. |
| `Q` | Quits the stoker agent and its tmux session, after a typed `y`: `tmux -L matchblox kill-session -t =stoker`. `f` only turns the stoker off and leaves the agent running, so its context carries to the next order. |
| `y` | Gives the go a step waits for (`⏸ waits for you: api-gw wants to run gh pr merge 4556`). The go is good for one command of that kind in that session, and the stoker hears of it at its next wake. |
| `a` | On a failed step (`✕`), opens the chat on its session. |

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

## The chat

The chat is the right side of Sessions: the live screen of the selected
agent. It shows on a wide screen and on a phone held wide. On a narrow
screen, `Enter` or a tap on a row opens it over the whole screen. Sessions
keeps the keys until you give them to the chat.

| Key | What it does |
|---|---|
| `Enter`, `a` or `→` | On Sessions, gives the chat the keys. A click on the chat does it too. The key line then says `keys go to <name> · ctrl+] back · ctrl+o into tmux`. |
| `Ctrl+]` | Gives the keys back to Sessions. A click on the left does it too. On a narrow screen, it goes back to Sessions. |
| `Ctrl+O` | Goes to the selected agent's pane in tmux. It works on Sessions too. |
| `Ctrl+C` | Sends `^C` to the agent. A second one within 1 s is held. See below. |

In the chat, every other key goes to the agent: `↑ ↓ ← →`, `Enter`, `Tab`,
`Shift+Tab`, `Esc`, `Backspace` and the letters. `Esc` goes to the agent
too. There it cancels a question, closes a menu or stops a turn. The keys
go in the order you press them, one at a time. A paste goes as text. Each
newline (CR, LF or CRLF) goes as `Enter`. Every key goes to the agent that
the chat showed. If the pane runs a different agent now, nothing is sent:
`not sent: <pane> runs a different agent now`.

The service also guards `Enter`. If the question started after the screen
your console showed, the `Enter` is refused: `not sent: <name> asks
something new — look again`. So an `Enter` never answers a question you
have not seen.

`^C` twice stops Claude Code. A second `^C` within 1 s to the same agent is
held, and the key line says why: `^C held: one went 0.6 s ago, ^C again
stops the agent`, or `^C held: one waits to go, ^C again stops the agent`.
A third `^C` goes to the agent.

If the pane is gone, the chat says `That session ended.`

### The agent's history

The mouse wheel over the chat shows the earlier lines of the agent's pane.
On a phone, a swipe over the chat does the same. Each step of the wheel
moves 3 lines, up to 2000 lines. The wheel down goes back toward the live
screen. While the chat shows the history, its title adds `↑ <n> lines up`
and its frame is dim, not in the accent.

While the chat shows the history, the first key goes back to the live
screen and goes to no agent. This is true for every key, a paste and a key
of the question card. The key line says `back to live — the agent did not
get that key`. Press the key again to send it. A tap on the chat's title
also goes back to the live screen. The chat also goes back to the live
screen when you select a different session or give the keys back to
Sessions.

At night, the first key of a run that sends something to an agent asks
"Take over?". `y` rests the stoker while you work, and `n` keeps it on. Keys
that only move the cursor of the question card do not ask.

Only a console sends keys to an agent. A script or an agent gets
`keys go only from a console`.

On a narrow screen the title row reads `‹ Sessions · <name> · <state>`. A
tap on `‹ Sessions`, the app's edge swipe, ⌘M on a hardware keyboard or
`Ctrl+]` goes back to Sessions.

### The question card

When Claude Code asks with its question tool, a card shows at the foot of
the chat. It holds the question and its options. A permission prompt has no
card: its keys go to the agent as they are.

| Key | What it does |
|---|---|
| `Enter` | Sends the typed line, or the option under the cursor. With several questions, goes to the next one first. |
| `1-9` | Picks an option. With several choices, ticks it. |
| `↑` or `↓` | Moves between the options and the typing row. |
| `Space` | Ticks the option under the cursor, when the question takes several. |
| `Backspace` | Deletes the last character of the typing row. |

Typing fills the `Type an answer…` row. A tap on an option picks it. A tap
on a key of the card's foot presses it.

The card has a second guard. It sends nothing for 400 ms after it first
shows, and after it moves to the next question. A send in that time is
refused with the same words: `not sent: <name> asks something new — look
again`. So an `Enter` meant for what was there before never answers a
question you have not read.

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
| `Enter` | Opens the console of the host, or adds a host. |
| `x` | Removes a host from the list. matchblox stays on that host. |
| `y` | Confirms the remove or the connect. |
| `Backspace` | Deletes the last character of a host you type. |
| `Esc` | Leaves the list of hosts to add. |
| `q` or `Ctrl+C` | Quits. |

## A phone

A tap selects a row. A swipe over the list moves the selection. A swipe
over the chat shows the agent's history (see
[The agent's history](#the-agents-history)). On a narrow screen, a tap on
an agent opens its chat over the whole screen. Where the chat sits beside the list, a tap on the
selected agent, or on the chat, gives the chat the keys. A tap on an option
of the question card picks it, a tap on a key of its foot presses it, and a
tap on the left gives the keys back to Sessions. On any other row the key line says `tap again: <command>`, and a
second tap on the same row runs it. On Ships, a tap on the selected ship
selects its session in Sessions. Any other action (a
recommendation, a setup step, a destructive step) then shows `RUN` and
waits for `Enter` or a typed `y`, as from the keys. A tap on a tab opens
it, a tap on a key of the key line presses it, and a tap on `● N wait for
you` selects the first agent that waits. A tap on the header's stoker word
(`stoker waits for you` among them) opens the stoker agent, as `Enter` on
its row does. A tap on `machine ›` or `usage ›`
switches the dock. On Panes, a tap on a column header sorts by it.
