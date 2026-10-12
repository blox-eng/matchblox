# Keys

Every key works on the key bar of a phone SSH app: letters, digits, arrows,
`Enter` and `Esc`. No key needs `Ctrl`. The key line at the bottom shows
the keys of the open tab, and a tap on one presses it.

## The console

| Key | What it does |
|---|---|
| `↑` or `k` | Selects the row above. |
| `↓` or `j` | Selects the row below. |
| `Enter` | Shows the safe action of the row: go to the agent's pane (Sessions, Machine, Panes), open a shell in the worktree (Git), do a recommendation, show a setup step, or fold the stoker's steps (STOKED). On Ships, selects the session that drives the ship in Sessions. |
| `x` | Shows the other action of the row: the second action of a recommendation, close a setup step or end a stale session (Sessions), kill a detached busy loop (Machine), remove the marked worktrees (Git), merge a ship that waits for your merge (Ships). |
| `a` | Opens the peek on the selected agent: what it said, and your answer to an agent that waits or a message to any other (Claude Code queues a message typed while it works). The key line says `a answer` or `a message`. On Ships, opens it on the session of a stuck ship. |
| `1-6` | Opens a tab: Sessions, Ships, Machine, Git, History, Panes. |
| `p` | Opens Ships. |
| `Tab` or `Shift+Tab` | Opens the next or the previous tab. |
| `u` | Cycles the dock of a phone: usage, the machine, the stoker. While a run is in charge, or has steps you have not seen, the stoker's panel takes the dock until you press `u`. |
| `c` | Opens the stoker's whole card: every step with its why and RESUME line. In the right pane where there is one, on a phone on the Machine tab. `Enter` marks it seen and folds it, `Esc` closes it. |
| `/` | Searches the open tab. |
| `s` | Sorts Sessions and Panes by the next column. |
| `S` | Turns the sort around. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `r` | Scans the git checkouts again (Git). |
| `f` | Turns [the stoker](stoker.md) on or off: it compacts full idle sessions while you are away. While it rests, `f` turns it back on. |
| `n` | Starts a night: the stoker runs until the morning, then turns itself off. `n` again ends it. |
| `e` | Shows or hides the agents that exited and wait for their parent (Sessions). |
| `d` | Dismisses the selected recommendation until its evidence changes (Sessions). |
| `h` | Folds the recommendations to one line, or shows them again (Sessions). |
| `Esc` | Clears the search. With nothing to clear, goes back to Hosts. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |

## An action before it runs

The key line shows `RUN` and the exact command.

| Key | What it does |
|---|---|
| `Enter` | Runs a safe action. |
| `y` | Runs a destructive action. `Enter` never does. |
| `y`, `n` | Answer "Take over?" while the stoker runs: `y` rests it while you work, `n` keeps it on. |
| `↓` or `j` | Scrolls a long change. `y` works only after its last line was on the screen. |
| `m` | Turns the choice of a setup step on or off: the tmux mouse, in "Add the way back". |
| `Esc` | Cancels. Any other key cancels too. |

## The answer peek

The peek is the review: `Enter` sends what it shows to the pane named in
its title, with no second confirm. Every other printable key, `q` too, is
text on the typing line.

| Key | What it does |
|---|---|
| `Enter` | Sends the typed line, or the option under the cursor. With several questions, goes to the next one first. At a permission prompt, goes to its pane instead. |
| A digit | Picks an option: the question goes on at once. With several choices, ticks it. The digit after the options opens the typing line. |
| `↑` or `↓` | Moves between the options and the typing line. |
| `Space` | Ticks the option under the cursor, when the question takes several. |
| `→` or `←` | Moves the peek to the next or the previous agent, in the order of Sessions (who waits first), wrapping. A phone's horizontal swipe sends them. `Tab` and `Shift+Tab` do the same. |
| `Backspace` | Deletes the last character. |
| `Esc` | Closes the peek. Nothing is sent. |
| `Ctrl+C` | Quits the console. |

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
| `Esc` | Leaves the list of hosts to add. |
| `q` or `Ctrl+C` | Quits. |

## A phone

A tap selects a row. A tap on an agent, once it is selected, opens the
peek; a tap on an option of the peek picks it, a tap on a key of its foot
(`⏎ send`, `← prev`, `next →`, `esc`) presses it, and a tap outside it closes
it. On any other row the key line says `tap again: <command>`, and a
second tap on the same row runs it. On Ships, a tap on the selected ship
selects its session in Sessions. Any other action (a
recommendation, a setup step, a destructive step) then shows `RUN` and
waits for `Enter` or a typed `y`, as from the keys. A tap on a tab opens
it, a tap on a key of the key line presses it, and a tap on `● N wait for
you` selects the first agent that waits. A tap on `machine ›` or `usage ›`
switches the dock. On Panes, a tap on a column header sorts by it.
