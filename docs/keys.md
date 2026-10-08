# Keys

Every key works on the key bar of a phone SSH app: letters, digits, arrows,
`Enter` and `Esc`. No key needs `Ctrl`. The action line under the tabs
shows the keys of the open tab.

## The console

| Key | What it does |
|---|---|
| `↑` or `k` | Selects the row above. |
| `↓` or `j` | Selects the row below. |
| `Enter` | Shows the safe action of the row: go to the agent's pane (Queue, Sessions, Procs, Panes), open a shell in the worktree (Git), do a recommendation, or show a setup step. |
| `x` | Shows the other action of the row: end a stale session (Sessions), kill a detached busy loop (Procs), remove a worktree (Git), close a setup step (Queue). |
| `a` | Answers the selected agent in one line. A permission prompt is answered in its pane. |
| `1-7` | Opens a tab: Queue, Sessions, Machine, Procs, Git, History, Panes. |
| `Tab` or `Shift+Tab` | Opens the next or the previous tab. |
| `/` | Searches the open tab. |
| `s` | Sorts Sessions and Panes by the next column. |
| `S` | Turns the sort around. |
| `Space` | Marks a worktree that is safe to remove (Git). |
| `X` | Marks every worktree that is safe to remove (Git). |
| `r` | Scans the git checkouts again (Git). |
| `e` | Shows or hides the agents that exited and wait for their parent (Sessions). |
| `Esc` | Clears the search. With nothing to clear, goes back to Hosts. |
| `q` or `Ctrl+C` | Quits the console. The service keeps running. |

## An action before it runs

The action line shows `RUN` and the exact command.

| Key | What it does |
|---|---|
| `Enter` | Runs a safe action. |
| `y` | Runs a destructive action. `Enter` never does. |
| `↓` or `j` | Scrolls a long change. `y` works only after its last line was on the screen. |
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
| `q` | Quits. |

## A phone

A tap selects a row. A second tap on the same row runs its safe action: the
action line says `tap again: <command>` between the two taps. A tap on a
tab opens it. A tap on a column header sorts by it.
