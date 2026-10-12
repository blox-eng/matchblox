# Keys

Every key works on the key bar of a phone SSH app: letters, digits, arrows,
`Enter` and `Esc`. No key needs `Ctrl`. The key line at the bottom shows
the keys of the open tab, and a tap on one presses it.

## The console

| Key | What it does |
|---|---|
| `↑` or `k` | Selects the row above. |
| `↓` or `j` | Selects the row below. |
| `Enter` | Shows the safe action of the row: go to the agent's pane (Sessions, Machine, Panes), open a shell in the worktree (Git), do a recommendation, show a setup step, or fold the stoker's steps (STOKED). |
| `x` | Shows the other action of the row: the second action of a recommendation, close a setup step or end a stale session (Sessions), kill a detached busy loop (Machine), remove the marked worktrees (Git). |
| `a` | Answers the selected agent in one line. A permission prompt is answered in its pane. |
| `1-6` | Opens a tab: Sessions, Ships, Machine, Git, History, Panes. |
| `p` | Opens Ships. |
| `Tab` or `Shift+Tab` | Opens the next or the previous tab. |
| `u` | Switches the dock of a phone between usage and the machine. |
| `/` | Searches the open tab. |
| `s` | Sorts Sessions and Panes by the next column. |
| `S` | Turns the sort around. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `r` | Scans the git checkouts again (Git). |
| `f` | Turns [the stoker](stoker.md) on or off: it compacts full idle sessions while you are away. While it rests, `f` turns it back on. |
| `n` | Starts a night: the stoker runs until the morning, then turns itself off. `n` again ends it. |
| `e` | Shows or hides the agents that exited and wait for their parent (Sessions). |
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

## Typing an answer or a search

| Key | What it does |
|---|---|
| `Enter` | Keeps the search, or shows the answer before it is sent. |
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

A tap selects a row, and the key line says `tap again: <command>`. A
second tap on the same row goes to the agent's pane, or, for an agent that
waits for an answer, opens the answer. Any other action (a
recommendation, a setup step, a destructive step) then shows `RUN` and
waits for `Enter` or a typed `y`, as from the keys. A tap on a tab opens
it, a tap on a key of the key line presses it, and a tap on `● N wait for
you` selects the first agent that waits. A tap on `machine ›` or `usage ›`
switches the dock. On Panes, a tap on a column header sorts by it.
