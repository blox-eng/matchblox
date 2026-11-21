# Keys

Every key of the console works on the key bar of a phone SSH app: letters,
digits, arrows, `Enter` and `Esc`. No key of the console needs `Ctrl`.
`Ctrl+O` is for a hardware keyboard. On a phone,
`◂` on the app's key belt (or its edge swipe) goes back, `^C` on the belt
interrupts, a swipe or the match head scrolls, and a tap on the head is
`Enter`. In the matchblox app,
your own belt layout can hold the console's keys: see
[Mine](phone.md#mine). The key line at
the bottom shows the keys of the open tab. A tap on a key presses it.

## The list and the chat

This section is the drawn chat: the demo, a console with no tmux, and a
console outside its home session. On a desk in tmux there is no line, and
the keys go to the list or to the agent's pane: see
[The agent beside the list](#the-agent-beside-the-list). The rows below
that name the line, or a typed digit, do not act there.

The keys go to one of two places: the chat's line or the list. The key
line names the place first, `writing to flint-auth` or `in the list`, then
at most five keys for it.

In the line, every letter types, `j`, `k`, `q` and `R` too. A paste goes
in as one piece, in the line that shows: the chosen session's line, or
the room's talk line while it types. The tips, a setup step and a host's
trouble take no text: a paste there says so. A digit answers only with
the line empty and a question on the screen; else it types.

| Key | What it does |
|---|---|
| `Enter` | In the line, sends the text to the session the line names, through its own host. In the list, goes back to the line. With the agent beside the list, it shows the chosen agent and gives it the keys. |
| `Backspace` | In the drawn chat, erases the last character of the line. |
| A digit, 1 to 9 | With the line empty, marks that option of the question on the screen. The same digit again sends it. A question that opened less than half a second ago takes no digit: the digit types. On a session the stoker holds, 1 marks "Let it go", and 1 again lets it go. |
| `↑`, `↓` | With the line empty, or in the list, chooses the session above or below, as the list draws them. In the list, `↓` past the last row chooses the parts of the foot one by one, and `↑` goes back up. `Enter` opens the chosen part. |
| `Esc` | Closes the menu. In the line, clears a marked answer, then moves the keys to the list. In the list, clears the search, then closes the chain view beside the list, then closes the chain's room, then shows all chains. It never leaves the console, except on a host that has not answered yet: then it goes back to Hosts. `Ctrl+M`, prefix `m` and the phone's back button do the same in the console. On a phone, `Esc` then a quick letter types the letter in the line. |
| `s` | In the list, shows the next host, then all hosts. It goes to each host, a hidden host too. With one host shown, all hosts is that host, so `s` goes from host to host. With one host only, a notice says so. |
| `h` | In the list, on a hidden host, shows that host in all hosts again. Machine › Hosts then ticks the host. On another host, a notice says what `h` does. |
| `g` | In the list, shows only the chain of the chosen session: its room. `Esc` shows all. With the agent beside the list, `g` instead shows up to 4 of the chain's agents side by side, each in its own pane, and the list marks them `▸`. `g` again, or `Esc`, shows one agent again: the chosen row's. |
| `G` | With the agent beside the list, opens the room of the chosen session's chain, as `g` does without it. |
| `⌥s` | Opens the room of the chosen session's chain, as `g` does. A session in no chain opens its chain of one, which the host's stoker drives. In the matchblox app, `⌥s` types `s` in the line, and the belt's `g` opens the room. |
| `/` | In the list, searches the list. |
| `m` | In the list, opens the menu of the chosen session. It names the acts that destroy nothing, with their keys: rename, stoke the chain, a new order. A kill, a merge, a go, a switch and the stoker's stop are keys of the list only. |
| `c` | In the list, copies what the agent said last. |
| `z` | In the list, parks the chosen session: nothing types into it, and the stoker leaves it alone. `z` again lets it go. In the line of a parked session, `z` does nothing: `Esc`, then `z`. |
| `N` | In the list, opens **New session**: where it starts (a path, or `←` `→` through your repositories and worktrees, then the ones found on the host, the most recent first), a branch for a new worktree beside the repository (blank: none), its name, the agent (`←` `→`, from the installed Claude Code, Codex, OpenCode and Cursor), for Claude Code, Codex and Cursor the account (see below), and a first message (blank: it waits for your first prompt; a new account has none). `Tab` goes to the next line. |
| `x` | In the list, kills the chosen session: a box shows the commands, and they run on a typed `y`. A session that works or needs you asks for a second `y`. |
| `r` | In the list, renames the chosen session: its tmux window, its tmux session when it is that session's one window, and Claude Code's own name (`/rename`), all alike. A Claude Code session that works takes its new name when it rests. |
| `A` | In the list, switches the agent of the chosen session: choose another installed agent (`←` `→`), how it starts (`←` `→` or a tap: **continue** from the handover, or **no task**), and any extra instructions for this switch. The confirm names the switch, its account and its launch settings. A session that works or waits needs a second `y`. A switch that continues first asks an idle Claude Code or Codex agent to write its task record and waits up to 3 minutes, showing the wait in the bottom rows; `Esc` switches now. |
| `p` | In the list, merges the pull request of the chosen session when it waits for your merge. A box shows the command, and it runs on a typed `y`. |
| `y` | In the list, gives the go that a stoker step waits for on the chosen session. On Pocket the first `y` only arms it, and a second `y` gives it. |
| `f` | In the list, on the host of the chosen session, turns [the stoker](stoker.md) off while it is on. While it rests, turns it back on. Otherwise opens the launch sheet for a day run. |
| `n` | In the list, on the host of the chosen session, opens the launch sheet for a night: the stoker runs until the morning, then turns itself off. While a night runs, `n` ends it. |
| `o` | In the list, on the host of the chosen session, gives a new order while a day run is on: the launch sheet opens with the run's gos, and the order goes to the stoker as one line. The run goes on, and done steps stay done. A night is sealed: `n` stops it first. |
| `F` | In the list, stokes the chain of the chosen session: the start sheet, `Stoke the chain <name> (<n> matches)?`, takes the order and the gos of a stoker for that chain. See [A family of stokers](stoker.md#a-family-of-stokers). |
| `d` | In the tips, dismisses the chosen tip until its evidence changes. |
| `Tab` or `Shift+Tab` | Goes to Machine, or back to the list and the chat. |
| `Ctrl+O` | Goes into the chosen agent's pane, from the line or the list. The key line then says the way back, `ctrl+a m`. |
| `Ctrl+L` | Hides the list, or shows it again. With the list hidden, the chat takes the width, and the header names each session that needs you. A tap on a name opens its chat. On a phone, the list and the chat take turns. |
| `◂` or `←` | On a phone, goes from the chat back to the list. A tap on `◂` at the left of the chat's head does it too. On a desk, `←` in the line moves the keys to the list, as `◂` does on a phone. |
| `e` | In the list, shows the agents that exited and wait for their parent, dim at the end of `IDLE`: for each parent, how many, the command that makes it reap them, and their pids. `e` again hides them. The `IDLE` count counts only the sessions that run. While they are hidden, the foot counts them: `· 1 exited`. |
| `q` or `Ctrl+C` | Quits the console: `Ctrl+C` from every tab, `q` from Machine. In the line and the list, `q` types. The service keeps running. |
| `R` | In the list, in the tips, in a setup step and in a host's trouble, starts the console again on the binary the service runs. In the line and in the room, `R` types. The key line offers it first, `R start again · service v0.43.0, console v0.42.0`, when the service on this machine runs a newer release than the console and the console's own binary was installed over. Where `R` types, the key line names the way to the list first: `esc R start again` in the line, `◂ R start again` in the room. A tap on these words starts the console again from every place, on Pocket too. Otherwise the line says `quit and reopen`. |

### The agent beside the list

On a desk, the console runs inside tmux in its home session `matchblox`.
There the console's window has two panes: the list, and the agent's own
pane. The right pane is a live view of the agent's pane, so typing there is
typing into the agent: its `/commands`, `@files`, `Esc`, menus, and the
mouse wheel for scroll and copy. The list is 40 columns wide, or 32 on a
mid, tall screen.

| Key | What it does |
|---|---|
| `Enter` | In the list, gives the agent on the right the keys. If the right side does not show the chosen agent yet, it shows it first. |
| A click on a row | Chooses the row and shows its agent on the right at once. The keys stay in the list, so `↑` and `↓` go on choosing. From the agent's pane, a click on the list takes the keys back to the list. A click on the agent's pane gives it the keys. |
| `↑`, `↓` | Choose the next row. When the choice rests, the right side shows that agent, and the keys stay in the list. A peer's rows show after you let one of its agents show in this run; on a host the stoker drives, and on one pane at a time, only `Enter` shows. |
| `Ctrl+O` | Still goes fully into the agent, as before. |

tmux takes these chords, not the console, so they are in a table of their
own:

| Chord | What it does |
|---|---|
| `ctrl+q` | Flips the keys between the list and the agent's pane. It acts only in the console's window. In any other pane or window, `ctrl+q` types as usual. |
| `ctrl+a m` | Comes back to the list: prefix `m`, a tap on `◂ matchblox`, and the phone's back all go to the list. |

A host that the stoker drives asks its take-over question first, and a
peer's consent box comes first, as with `Ctrl+O`. The agent of a peer
opens on the right over ssh. When the agent ends, the right pane says
`flint-auth ended. ctrl+q: back to the list`. If the agent cannot be
shown, the pane says why. A pane of the console's own window never shows
on the right: `Ctrl+O` opens it.

Text typed in the list does not go to the agent. The key line says
`the agent pane takes text · ⏎ goes into flint-auth`.

On a narrow window, and on Matchblox Pocket held upright, one pane shows
at a time. A tap on a row shows the agent full screen, and `◂` goes back
to the list. The phone shares the first console: a choice on the phone
changes the right side of that console.

### Each terminal, its own console

`matchblox` in a terminal joins the console `matchblox` while no terminal
shows it. Another terminal opens a console of its own, `matchblox-2`,
with its own list, its own choice and its own agent beside it. A key, a
click or `↑` `↓` in one terminal never moves another. `ctrl+q` flips the
keys in the terminal's own console. `ctrl+a m` and a tap on `◂ matchblox`
go back to the console of the terminal that pressed them, also from an
agent opened with `Ctrl+O`. When a terminal closes, its console ends and
the agents run on; a look at another tmux session keeps it. The first console stays for the next
terminal, and the phone always joins it.

The drawn chat and its line stay in the demo (and the iOS demo), in a
console with no tmux, and in a console outside its home session or started
for a host.

On a desk the list is 40 columns wide, with the chat beside it. On a
narrower screen the list is 32 columns wide. A phone, and a screen that
leaves the chat less than 40 columns, shows one of the two at a time:
the list first. A tap on a row, or `Enter` in the list, opens its chat.
A tap on a group's head folds the group: the head keeps its count, and
its `−` turns to a plus sign. A tap on the head again unfolds it. The
foot opens each of its parts in the chat's place, beside the list:
`+ new` opens **New session**, `✧ 2 tips` the tips, `! 1 setup` the
first open setup step, and `! ws-2 not reached` (or `! ws-2 blocked`)
that host's trouble and its fix. `Esc` gives the place back to the chat.
A notice shows over the lines above the chat's line and the foot, and a
tap on it does nothing, except on the notice of a newer release: that
tap starts the console again. In the tips, `↑` `↓` choose, `Enter` does
the chosen tip and `d` dismisses it. In a setup step, `Enter` sets it up
and `x` closes it, after a typed `y`. In a host's trouble, `Enter` runs
the fix or dials the host again. In these three places `R` starts the
console again, when a newer release waits. A drag in the chat copies
only the chat's text.

A key that changed its job says so once, the first time you press it:
`c`, `g`, `p`, `x` on a session whose pull request waits for its merge,
and a digit.

A key with no job in the line or the list does not do nothing: a notice
names the keys that place takes. Any other letter in the list moves the
keys back to the line and types there. The line of a parked session is
off: it says `parked: esc, then z lets it go`. No letter in the line
acts on a session. A message to a host that is not reached stays in the line, and the
line says `not sent: ws-2 is not reached`. Nothing sends it again by
itself. The rows of that host stay, dimmed. On that host, the header
says `ws-2 not reached · since 2m`. When the chosen session ends, the
next session in the list is chosen, and its line says
`flint-auth ended · the text is not sent`.

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

A room is one chain and its stoker: `g` in the list or `⌥s` opens it. The
list shows only the chain's sessions, and the room takes the chat's place:
its name, the stoker's card and steps, then the talk line. The room has
the keys. The key line shows the keys of the chosen line.

| Key | What it does |
|---|---|
| `↑`, `↓` | Chooses the line above or below: the stoker's card, its steps, the talk line. |
| `Enter` | On `↓ N more`, unfolds the plan. On the talk line, starts typing. On a quiet stoker's line, starts typing to it. |
| `y` | Gives the go a step waits for. Every go (a merge, a release, a deploy, a kill) acts outside the session: it shows its command first and runs on a typed `y`. |
| `⌥s` | Goes back to the session you flipped from. With none, starts typing on the talk line. |
| `⌥n` | Goes into the chain's first match. |
| `◂` or `←` | Gives the chat its place back and the keys to the list. The list still shows only the chain, and the key line says `g room`: `g` opens the room again. |
| `Esc` | Stops typing (the words stay). Else closes the room, shows every chain and gives the chat its place back. |
| `Backspace` | Erases the last character while you type. |

`out of <chain>` in a session's menu (`m`) takes the match out of the chain,
after a typed `y`. Its agent goes on, and the chain hears that it left.
The menu has it only while the list shows that chain.

While the talk line types, every key is text until `Enter` sends it to the
chain's stoker or `Esc` stops. The first words to a chain with no stoker
start one, with no launch sheet; a refusal keeps the words in the line.

### The chain view beside the list

With the agent beside the list, `g` on a chain's row, or on a member's row,
shows up to 4 of the chain's agents on the right, each in its own pane,
most urgent first. With 2 members, one pane is above the other. With 3 or
4, the panes make a 2×2 grid. The list marks each member on screen with
`▸`.

| Key | What it does |
|---|---|
| `g` | In the list, opens the chain view. `g` again closes it. |
| `G` | Opens the chain's room, the stoker's talk. This is what `g` did before in this mode. |
| `Esc` | Closes the chain view. With a search on, it clears the search first. |

A click on a pane gives it the keys. The chords are tmux's:

| Chord | What it does |
|---|---|
| `prefix z` | Zooms the pane the keys are in, and again unzooms. |
| `ctrl+q` | From a pane back to the list, and from the list to the member used last. |

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
| `Ctrl+O` | On a failed step (`✕`), goes into its session's pane. |
| `a` | Says once that `Ctrl+O` goes into a failed step's session now. |

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

A keyboard never changes the layout: the shape goes by the tallest
height seen at the width.

| Key | What it does |
|---|---|
| `Ctrl+O` | Goes into the chosen agent's tmux pane, from the line or the list. |
| `ctrl+a m` | In the iOS demo, which has no tmux, `Ctrl+O` opens a stand-in for the pane: the agent's screen over the whole screen, and every key goes to the agent. Its title is `‹ sessions`. `ctrl+a`, then `m` within 1 s, goes back to the list. |

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

The list is the first screen, full width. A tap on a row opens its chat,
and `◂` comes back.

| The owner wants to | By hand |
|---|---|
| open a chat | Tap its row. |
| answer | Tap an option in the chat, then tap it again to send it. |
| go in | Tap `⌃o` on the key belt, or press `Ctrl+O`. |
| open the menu | Long-press a row, or press `m` in the list. |
| act | Tap a key on the key line. A destructive act still waits for a typed `y`. |
| come back | `◂`, the belt's `◂`, or the edge swipe. |
| show an agent beside the list | Tap its row. On a narrow window the agent takes the whole screen, and `◂` goes back to the list. |

A vertical swipe over the list, or a wheel step there, chooses the
session above or below. Over the chat it scrolls the chat back through
the pane's history, and the choice and a marked answer stay. A chat with
no screen to scroll says so. While the tips or the room have the chat's
place, a wheel step there chooses the tip or the room's row above or
below. Over a setup step or a host's trouble, a notice says that nothing
there scrolls. The wheel never goes to the pane under the place. On a
phone a drag never selects text: the menu copies. A tap is never
consent: a tap on `y` marks the go, and a second tap gives it. A tap on
a key of the key line presses it, and a tap on `● N need you` shows
`NEEDS YOU` and chooses the first session that needs you. A tap on the
header's stoker word opens the stoker agent. On Panes, a tap on a column
header sorts by it.

A long-press needs Matchblox Pocket, or a terminal that reports the
release of a press. Elsewhere, `m` opens the menu.

The app's command keys type a command of the agent into its pane, for
example `/compact`, with no Return: you send it. The console marks the
panes of the agents it knows, and a command key types only into a pane
marked as an agent of the command's kind. Anywhere else it does nothing.

The app's `⇞` and `⇟` keys send `PgUp` and `PgDn`. In a pane you jumped
to on a host, `PgUp` starts tmux copy mode one page up, and `PgDn` moves
one page down in it.
