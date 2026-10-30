# The console

From the top: the header, the tabs, the open tab, then usage and the
machine, and at the bottom the key line. On a wide desk, usage and the
machine sit in the left column, under Sessions.

The header shows the host, how many agents wait for you (`● 3 wait for
you`; a tap selects the first) and [the stoker](stoker.md): `stoker off`,
`stoker`, or `☾ night → 07:00`. For a moment after an action it says what
ran instead.

The tabs are `sessions`, `ships`, `machine`, `git`, `history`, `panes` and `stoker`,
opened by the digits `1-7`. The open one is uppercase. A narrow screen shortens the
tabs far from the open one to their digit. `machine !` has a problem.

The key line at the bottom shows the keys of the open tab, and how old the
sample is. A tap on a key presses it. It is also where you confirm a step
or type a search. See [Keys](keys.md).

## Usage, the machine and the stoker

Under the open tab, when there is room, the dock shows how much of each
plan your agents have used, the machine, and [the stoker](stoker.md). On a
wide desk they are not a dock: they stack in the left column under Sessions
(see [the chat](#the-chat)). Everywhere else they stay in the dock at the
bottom.

```
 USAGE                                         │ MACHINE        ws-1 · 16 cores
 ✻ Claude Max  5h   ▰▰▰▰▰▱▱▱ 62%  · 16:00      │ cpu  ▃▄▅▄▃▅▆▅ 38%   load 4.2
               wk   ▰▰▰▱▱▱▱▱ 41%  · Thu 09:00  │ mem  ▰▰▰▰▰▱▱▱ 41 of 64 GB
 ❋ OpenAI Plus 5h   ▰▱▱▱▱▱▱▱ 12%  · 18:40      │ disk ▰▰▰▰▰▰▱▱ 71% of 1 TB
               wk   ▰▰▱▱▱▱▱▱ 30%  · Sat 11:00  │ runs 13 claude · 1 codex
```

- Each account starts with its provider: its logo in kitty or Ghostty
  (through tmux too) while every terminal on the console's session can
  draw it, else its mark. Then the provider's name and the plan
  (`✻ Claude Team`, `❋ OpenAI Team`), or the email's name when the
  provider has two accounts. A dock under 50 columns leaves out the plan,
  and its bars shrink so the reset times stay whole.
- A phone or any other terminal that attaches to the console's session
  brings back the marks within about 2 seconds. `NO_COLOR` and tmux 3.2
  or older show the marks. The console lets images through its own pane
  only (`allow-passthrough`), and gives the pane its own setting back when
  it ends. The logos come from
  [lobe-icons](https://github.com/lobehub/lobe-icons) (MIT).
- Each account shows its windows as `/usage` does: `5h` is this 5-hour
  window, `wk` this week, each with the percent used and when it resets.
  A bar turns `warn` from 60% and `neg` from 90%.
- The week ends with whether it lasts at your pace so far. Hours you do not
  usually work count as a tenth: set them with `quiet_hours` and
  `quiet_days` in the [config](config.md).
- `out Fri ~20:00`: the recommendations name the session of that account
  that burns the most. `Enter` jumps to it, and `x` compacts it while it is
  idle. The row then says what happened, or what failed and that the key
  tries again.
- On a narrow screen the dock shows one panel at a time. `u`, or a tap on
  the word at its right (`machine ›`), goes to the next: usage, the
  machine, the stoker. While a run is in charge, or has steps you have not seen, the
  stoker takes the dock until you switch.
- Wider than that, two panels side by side: usage and the machine, or
  usage and the stoker while it claims the dock.
- On a wide desk, the panels stack under Sessions: usage, the machine, and
  the stoker while it claims the dock. Usage and the machine keep their
  rows. When the list does not fit, the list and the recommendations give
  way. A list cut short ends in `↓ <n> more`. The hint never covers the
  selected row. When the list cannot keep 3 rows and the hint, the panels
  fold. The machine goes first, then all.
- An account folder shows by its name (`work`), the default sign-in by
  its plan or email. A Claude Code account that is not signed in yet says
  `not measured · signing in: the limits show once the account is signed
  in`.
- With `cursor_usage` on, a Cursor line with no figures yet says why:
  `signing in: …` (a new account), `no Cursor login token read yet` (no
  login; matchblox reads it again every 30 s), `Cursor's login token could
  not be read` (the read was denied or failed; read again after 5
  minutes), or `Cursor's API has not answered yet` (the token is read, or
  the call gave no figures yet).
- When the body leaves rows free, the dock grows into them: each account's
  forecast and resets, the top processes, the stoker's recent steps. The
  detail beside the list shows more of what the agent said.
- `c`, or a tap on the stoker's panel, opens the STOKER tab: your words, its plan with each step's outcome, its standing rules and today's steps.
- `disk` is the file system that holds your home directory, and `/` beside
  it when that is another one.
- Claude Code's status line gives the 5-hour window and the week only, so
  there is no `opus` or `sonnet` line.
- When the screen is short (a phone with its keyboard up), the dock folds
  away. The Machine tab has all of it.

Where the figures come from:

| Agent | What to do |
|---|---|
| Claude Code | Open the door "Show your limits" in Sessions. It adds matchblox to your status line; a status line you have keeps showing what it showed. |
| Codex | Nothing: Codex reports its limits after each turn, on Linux and macOS. A plan with a monthly limit shows `mo`. |
| Cursor | Set `cursor_usage = true` under `[limits]` (macOS). See [Agents](agents.md#the-account). |
| OpenCode | It shows "not measured": OpenCode keeps no limits. |
| Cursor | Set `cursor_usage = true` under `[limits]` in the [config](config.md). Cursor shows `mo`, the share of the month's included usage. See [Agents](agents.md#the-account). |

A line that says `as of 14:02` has not changed for an hour: the account's
agents have not worked since. Each reading is kept for five weeks in
`~/.local/state/matchblox/limits.jsonl`, on this machine only.
`matchblox status` has every figure under `limits`.

## Read it from a script or an agent

`matchblox sessions`, `matchblox ships` and `matchblox usage` print one line
a row; with `--json` they print JSON. `matchblox ships --json` and
`matchblox usage --json` print the ships and the accounts' limits.
`matchblox sessions` says "sign in by hand" on the line of a new account that matchblox gave up signing in. `matchblox usage` names an account folder by its name; `--json` gives its
folder as `dir`. They need
`matchblox serve` running (or a console), and else say so. Each row of
`matchblox sessions --json` has:

| Field | It is |
|---|---|
| `name` | The session's name (Claude Code's, or the agent's). |
| `pane`, `target` | Its tmux pane (`%3`) and target (`app:1.1`). Any of `name`, `pane` and `target` names it to a hand command. |
| `word` | What it does, in one word: `asks` (it waits for an answer or an approval), `busy` (a turn runs), `idle` (at rest), `background` (at rest, and a shell it started in the background still runs), or `agents` (its subagents still work). Read this one. |
| `status` | The status word the agent itself reports, as it reports it. Claude Code says `busy`, `idle` or `shell`; `shell` is a session at rest with a background shell, not a pane without an agent. |
| `reason` | Why a session at rest is idle (`#4556 waits your merge`, `CI red · …`, `stuck: …`, `out of work`, `idle`); empty while it works or asks. |
| `held` | Nothing types into it: you parked it, or the stoker held it. |
| `stoker` | It is the stoker agent, on matchblox's own tmux server. Sessions leaves it out. |
| `context_pct` | How full its context is, in percent. |
| `plan_pct` | The progress bar it drew in its last reply, or null. |
| `account` | The account its plan runs on: the email and plan, or `API key`. Empty when matchblox reads none, as on an account not signed in yet. |
| `account_dir` | The account folder it runs on, or empty on the default sign-in. See [Agents](agents.md#accounts). |
| `signing_in` | It runs on an account folder that is not signed in yet. |
| `ship` | The pull request it drives (`repo`, `number`, `stage`, `kind`, `note`), or null. |

`matchblox peek api` prints what the session said last and what it asks,
if anything: the question with its numbered options, or an approval that
waits in its pane. It shows what the console's question card shows, and it acts on
nothing. `--lines 40` keeps the last 40 lines of what it said (20 by
default, 0 for all); `--json` prints `name`, `pane`, `target`, `said`,
`asks` (the first line of the question), `questions` (each with its
`options`) and `approval`.

## Act on a session from a script or an agent

`<who>` is a pane (`%3`), a target (`app:1.1`) or a name; a name two sessions
share is refused, never guessed. Each command asks the running service, which
checks what the console checks, and prints the exact keys it sent; `send`
starts with `send api: typed`. A refusal is printed on stderr, with exit
code 1, and names the command and why (`send api: refused: …`). A send
whose text went in and whose `Enter` a guard or tmux stopped (a permission
prompt came up between the two) says `send api: typed, not sent (Enter not
pressed): …`: the text waits in the prompt, so do not send it again. When the
service did not answer, the command says whether the act went: `send api:
not typed: …` when it never reached the service, and `send api: not known
whether it was typed: peek api before you send again` when the service
took it and the answer never came.

| Command | It does |
|---|---|
| `matchblox send api "run the tests"` | Types the line and Enter into the session. It queues if the session works, and is refused at a question or a permission prompt. |
| `matchblox send api --pick 2` | Answers the question the session asks with its second option. |
| `matchblox compact api` | Types `/compact` into a Claude Code session at rest that waits for nothing and was not compacted in the last 30 minutes. |
| `matchblox hold api --why "not the priority"` | Queues "finish this step, then stop and wait" and holds the session: nothing else types into it. |
| `matchblox release api` | Lets a session the stoker held go, and tells it "carry on". A session you parked stays parked: only `z` at a console lets it go (`parked by you: only z lets it go`). |
| `matchblox new "fix the flaky test"` | Starts Claude Code here, in a tmux session of its own named after the directory, with the words as its first message. `--where <path>` starts it elsewhere (`--where <url>` clones a git URL, `https://…`, `git@host:o/r` or `gh:o/r`, into `[git] clone_root`, or uses the clone you have, then starts; it waits up to 30 minutes. A bare `o/r` is a folder here, as any path), `--agent codex` (or `opencode`, `cursor-agent`) starts another agent, `--branch <b>` first adds a worktree for a new branch beside the repository (`<repo>-<b>`, from origin's default branch, tracking nothing), `--name <name>` names it. `--account <name>` starts it on an account of that agent. `--new-account` makes that account first. `--setup like-default`, `copy` or `empty` picks its setup, and the default is `like-default`. A new account starts without a first message, and the command prints `sign in in <session>`. It waits as long as a copy of the default can take, and says "refused" when matchblox refused the start, "failed" when it ran and failed, and "stopped after" the step that ran when a guard stopped the rest. A start on an account that another start or switch is busy with is refused at once. The tmux session, its window and Claude Code's own name are that one name. A first message that starts with a dash is refused: the agent would read it as a flag. |
| `matchblox rename api ships-flake` | Names the session, its tmux window, its tmux session when it is that session's one window, and Claude Code (`/rename`), all alike. A Claude Code session that works takes its name when it rests (`api renames when it rests`). A name is one word of letters, digits, dashes and underscores, and no other tmux session may have it. |
| `matchblox kill api` | Ends the session: `tmux kill-pane`, then a `TERM` to its agent if it still runs. It is refused while the session works or waits for you: `--anyway` kills it all the same. Its worktree and its transcript stay. |
| `matchblox switch api --agent codex` | Saves handover evidence, then replaces the agent in the same pane: same name, directory, park and ship. The new agent starts on its last account, else `default`, with match awareness and a prompt to continue from the saved evidence. Codex starts with workspace trust and workspace-write network access. `--handoff "<line>"` keeps your exact task; `--handoff ''` starts no task and captures no handover. While the match works or waits, `--anyway` switches it mid-turn. Inside tmux, your client jumps after the switch runs. |
| `matchblox switch api --agent codex --preview` | Reads the current handover evidence. It sends no agent input, writes no context files, and replaces no process. |
| `matchblox recover` | Lists the agents your tmux server ran, from the recovery record: each with its tmux session, agent, folder and state. **live**: an agent runs that conversation now. **can reopen**: its folder is there, on the branch it had, and its saved history is on disk. **cannot reopen**: the line says why and the fix (a folder that is gone, another branch or another repository, no saved history, an agent that is not installed, a conversation that another Claude Code process has open, or a session that ran on Bedrock, Vertex or an API key set in your shell: that sign-in came from your shell, so the line gives the command to run there instead of reopening it on your default sign-in). It changes nothing. The service keeps the record in `recover.json` in its state folder while your agents run: the agent, its conversation (it follows `/clear` and `/resume`), its tmux session, folder, checkout and tmux server. An agent you quit leaves the record. The agents of a server that died stay in it for 14 days. The record holds no transcript, prompt or environment value. Claude Code, Codex and Cursor are recorded; a Cursor line says its history is not checked, as matchblox does not read Cursor's history. `--json` prints the lines as JSON. |
| `matchblox recover --all` | Reopens each conversation that can reopen, each in a tmux session of its name on your server (`api-2` when `api` is taken), in its folder and on its account: `claude --resume <id>`, `codex resume <id>` or `cursor-agent --resume <id>`. It sends no prompt and replays no command: each agent waits for you. It says reopened only for an agent that still runs a moment later, `remain-on-exit` or not; one that exited at once is a cannot line with the command to run to see why, and its session is closed. It runs no git command that changes anything. A conversation that runs is left alone, so a second `recover --all` starts nothing. A line that could not reopen fails the command, with its fix. `matchblox recover api web` reopens those sessions only. `matchblox recover api --dir ~/src/api-moved` reopens a session whose folder moved, when the new folder is a checkout of the same repository (the same `origin`, or the same first commit) on the same branch; a relative `--dir` is read from the folder you run it in. An agent's pane may list them, but only you reopen them. |
| `matchblox scan` | Finds the Git repositories in your home folder (the folder itself, `~/dev`, `~/src`, `~/code`, `~/projects`, `~/work`, and `[git] repos` and `scan_roots`), four folders deep, so N's `where` offers them. They never show in the Git tab. Running it is your yes: the service scans again every 30 minutes. `matchblox scan ~/w ~/oss` scans those folders instead of the usual ones; `matchblox scan off` stops the scans and forgets the list. It reads files only, runs no git and calls nothing on the network; the list stays on this host and the devices paired with it. On a Mac it skips the folders macOS guards unless you name them, and it skips network folders below a root; it waits as long as a scan can take, then prints what it found, each folder it skipped with its reason, and any error. An agent's pane cannot say yes for you. |

An act typed in a pane of the stoker's tmux session is the stoker's, and so
is an act from a session the stoker messaged in the run: what that session
sends counts toward the run's messages, and the session it sends to is the
stoker's from then on. Such an act is refused when no run is on (`the
stoker is off: no run`), while the stoker rests (`the stoker rests: a person
works`), on its own pane, and after `max_steps` messages in one run (60 by
default).

The launch sheet's and the STOKER tab's verbs, and `z`, are yours at a
console only. A command or an agent that sends them is refused (`only you
do this, at the console`).

The stoker agent keeps its plan, logs its steps and sets its triggers
with commands of its own. matchblox takes them only from the stoker's
pane.

| Command | It does |
|---|---|
| `matchblox stoker plan` | Reads the plan on stdin (a here-document, `matchblox stoker plan <<'EOF'`), one step a line (`1. api: merge #4556 on green`), and puts it on the card in place of the plan before it. Nobody approves it. An `asks:` or `grant:` line, which older briefs asked for, is read and ignored: the gos are your chips. |
| `matchblox stoker step 1 --state doing "told api to rebase"` | Logs a step of the plan to the card: `doing`, `done`, `failed` or `waits`, and prints `step 1 logged: doing`. `--needs merge:#4556` asks you for a go the step needs: the stoker only asks, and only your `y` gives it; a guarded command the guard denies lands on the card by itself. |
| `matchblox stoker step 0 --rule "compact on PR" --state done "compacted api"` | Logs what a standing rule of your ways did. |
| `matchblox stoker when --ship 4573 --stage merged send api "rebase on main"` | Sets a trigger matchblox fires itself, once: at a pull request's stage (`ci`, `green`, `merged`, `live`, `red`) or, with `--rest <who>`, at the end of a session's turn. The hand is `send`, `compact`, `release` or `hold <who> --why "…"`. `--list` prints the run's triggers, `--drop <id>` drops one. |

## The match

Each agent row starts with a match:

| Mark | Word | It means |
|---|---|---|
| `✦` | busy | The agent works. The match burns. |
| `✦` | agents | Its turn ended and its subagents still work: their results start its next turn. It does not wait for you. |
| `╿` | asks, waits, done, paused | The agent waits for you. The match is at rest. |
| `│` | its state | The context is full (at `compact_tokens`, 350k by default, or `compact_at`, 85% of its window). The match is spent, and Sessions says `! compact`. |
| `╿` or `│` | out | The plan's limit stops an idle session (`│` when it is also full). The next column says when it can work again: `19:50`, or the weekday when that is more than a day away. A session left a day or more keeps its word: `cold`, `stale`. |

## 1 Sessions

The console opens here. On top, while there are some: the setup steps
(`SET UP`). Then every agent, by what it needs from
you, and last the recommendations, each with its evidence and its exact
command. `d` dismisses the selected recommendation until its evidence
changes; `h` folds them all to one line, and `h` again shows them. The
console remembers both.

- **WAITS FOR YOU**, the oldest wait first:
  - **asks**: the agent asks for a permission, or asks with its question
    tool.
  - **waits**: the agent asked a question, or waits for input.
  - **paused**: the turn ended with its progress bar under 100%.
  - **done**: the turn ended.
- **WORKING**: `busy`, `agents` (its subagents work), `tests` or `shell`
  (a shell it left running).
- **QUIET**: `idle`, then `cold` after one day, then `stale` after seven
  days, the newest first. On a stale session, `x` and a typed `y` end the
  agent.
- **HELD**, last: sessions you parked with `z` (`⏸ parked by you`) or the
  stoker holds (`⏸ held by stoker · why`). Nothing types into them.
- **`+ start a new session here`**, under the sessions: `Enter` opens New
  session on that folder, the cursor on the agent. Here is the repo you
  launched the console in, else the one you used last; from the phone or
  over SSH, the one you used last. With no session it comes first, and
  two more rows start in your next most recent repos.

A `codex exec` or `claude -p` that an agent starts shows under that session
with `├─` and `└─`, in `matchblox sessions` too. The console shows it and
never acts on it. A link is drawn only when `/proc` proves it. A child
never changes how its parent reads: the child's own row shows its work.

A session at rest that does not ask you says why where its state word
would be, in a few words: `#4556 waits your merge`, `CI red · lint`,
`stuck: …`, `back 14:20`. It replaces `done`, `paused` and `idle`.

A row shows the match, the name, the state, how long it waits and its
context use (`warn` from 70%). A wide screen adds the plan and the ship the
session drives with its track (`#4551 ●━◉┄○┄○`, see [Ships](ships.md)); with
no ships read, the open pull request of its worktree (`#N`) and its tmux
place. A session that drives more than one ship leads with the one that
most needs you (stuck, then waiting for you, in flight, merged, live) and
counts the others: `+1 stuck` or `+1 waits` when one of them needs you,
else the furthest stage among them, as in `#4551 ●━◉┄○┄○ +1 merged`. On a
narrow row the count drops its word, then goes, before the ship does. The
detail lists the first three ships of the session in that order, then
`+N more · p ships`.

On a narrow screen the selected row opens: what the agent said last, its
ships, its plan and its keys. The console reads the screen's shape (rows × 2
≥ columns) before its width: a phone held upright under 90 columns is a
phone, and from 90 columns (a small font) the chat of the selected agent is
a pane beside the list. Wider, the same: the chat sits beside the list. A
session with no pane to type into shows its detail there instead, with the
account, the tokens and the background work.

`Enter` gives the keys to the chat (see below). `Ctrl+O` jumps to the pane.
Come back from a jump and the next agent that waits is already selected.

### The chat

The chat is the live screen of the selected agent, drawn inside the
console. You do not leave the console to use it. On a wide desk and on a
phone held wide, it is always the right side of Sessions. `↑` `↓` change
which agent it shows. Its frame title is `<name> · <state>`. The title is
in the accent while the chat has the keys. Under the title, a strip shows
the session's account, its PR and its background work. It shows only what
the session has, on one line or two. A session with none of these has no
strip. On an upright phone the chat has no strip.

```
╭─ Sessions ────────────────────╮╭─ api-auth · asks ───────────────────────────╮
│ ● api-auth      asks    2m    ││ I can fix the login in two ways.            │
│ ◐ web-cart      busy   12m    ││                                             │
│ ○ docs          idle    1h    ││ ╭─ api-auth asks ─────────────────────────╮ │
├─ Usage ───────────────────────┤│ │ › 1 Patch the token check               │ │
│ max   ▇▇▇▇▇▁▁▁ 61%            ││ │   2 Move to the new session store       │ │
├─ Machine ─────────────────────┤│ │   3 Type an answer…                     │ │
│ cpu 34%  mem 41/64G          ││ ╰─ ⏎ send ─────────────────────────────────╯ │
╰───────────────────────────────╯╰─────────────────────────────────────────────╯
 keys go to api-auth · ctrl+] back · ctrl+o into tmux
```

`Enter`, `a` and `→` (where `→` has no other use) give the chat the keys,
and so does a click on it. `Ctrl+]` and a click on the left give them back.
`Ctrl+O` goes into the agent's pane in tmux. While the chat has the keys, the key line says `keys go to <name> ·
ctrl+] back · ctrl+o into tmux`.

In the chat, all keys go to the agent, `Esc` too. They go in order, one at
a time, to the agent the chat showed. A paste goes as text, and a newline
in it goes as `Enter`. If the pane runs a different agent now, nothing is
sent: `not sent: <pane> runs a different agent now`. If the pane is gone,
the chat says `That session ended.` and Sessions has the keys again. Only a
console sends keys: the service refuses a script or an agent with `keys go
only from a console`.

A second `^C` to the same agent within 1 s is held, because two stop Claude
Code: `^C held: one went 0.6 s ago, ^C again stops the agent`, or `^C held:
one waits to go, ^C again stops the agent`. A third goes.

When Claude Code asks with its question tool, the question card sits at the
foot of the chat. `↑` `↓` move, `Space` ticks an option of a question with
several choices, typing fills `Type an answer…`, `Enter` sends, and a tap
picks an option. Several questions come one at a time. In auto mode the
options come from the queue hooks (see
[Agents](agents.md#how-an-agent-tells-that-it-waits)), so add them again
when the setup step asks. The options go to Claude Code's own menu with the
keys it takes, never as typed text, and only while the pane shows the page
they answer: if you answered in the pane yourself, nothing more is sent.
The header says `sending to flint-auth…`, then `sent to flint-auth`; an
answer that did not go says `not sent to flint-auth` and why on the key
line.

While the card shows, the agent's own menu leaves the screen: the lines from
its last rule down. The card is the one place to answer. While you scroll up
through the history, the screen stays as the agent drew it.

There is no card at a permission prompt. Its keys go to the agent as they
are: the chat shows the prompt, and `Enter` approves it.

The service refuses any `Enter` for a question that started after the
screen your console showed: `not sent: <name> asks something new — look
again`. The card has a second guard with the same words. For 400 ms after a
card first shows, and after it moves to the next question, it sends nothing.
So an `Enter` never answers a question you have not read.

The mouse wheel over the chat, or a swipe on a phone, shows the earlier
lines of the agent's pane, 3 lines for each step, up to 2000 lines. The
wheel down goes back toward the live screen. While the chat shows the
history, its title adds `↑ <n> lines up`, its frame is dim and the key
line says `a key goes back to live`. The first
key then goes back to the live screen and goes to no agent: a key, a paste
or a key of the card. The key line says `back to live — the agent did not
get that key`. The chat is blank until the live screen comes, and keys go
nowhere until then. A tap on the title also goes back to the live screen, and so
does a change of the selected session or a click on the list. The service
refuses an `Enter` that answers a screen of the history.

At night, the first key that sends something to an agent asks "Take over?".
Keys that only move on the card do not ask.

On a narrow screen, Sessions fills the screen and the dock stays at the
bottom, and `u` cycles it. `Enter`, or a tap on a row, replaces the screen
with the chat of that session. The title row reads `‹ Sessions · <name> ·
<state>`. A tap on `‹ Sessions`, the app's edge swipe or ⌘M (the home
gesture), or `Ctrl+]` goes back.

- Context use is read from the agent's own files, as a share of the
  session's window. Without a window the row shows `—`. See
  [Agents](agents.md).
- A new session shows in Sessions at the next sample, about 2 seconds
  after it starts. A session from `N` or `matchblox new` shows at once.
- The account is the provider's mark, then the email and plan, or
  "API key": `✻` Anthropic, `❋` OpenAI, `⬢` Cursor, `▣` OpenCode.
- A session on an account folder shows the account's name after its own
  name, in muted text (`api work`). Until the account is signed in, the
  row says `signing in`, or `sign in by hand`. An ask or a wait shows over
  it. See [Agents](agents.md#accounts).
- An agent can draw a progress bar in its last reply:
  `Progress [████░░░░] <step>`. The console shows it as `▰▰▰▱▱ 75%`.
- `s` sorts by another column (`newest` is the one with the least idle
  time first) inside each group; `S` turns it around.
- `g` groups the list by need (the groups above), by state word, by repo or
  not at all (one flat list). `z` parks the selected session, or lets it go.
- Agents that exited and wait for their parent are one faint line under the
  sessions. `e` lists them with the command that clears them.

## 2 Ships

The pull requests your sessions drive, each on its track from open to
live. `p` opens it. See [Ships](ships.md).

## 3 Machine

Usage first, then the machine: CPU per core, load, pressure, memory and
swap, temperature, CPU power limits, GPUs, and the CPU share of the
container groups you name in the [config](config.md).

Then the detached busy loops: shells that keep burning CPU after their pane
is gone, named to the pane that started them. `x` and a typed `y` kill
one. While one burns, the tab says `machine !`.

## 4 Git

Every git checkout that your sessions use: dirty paths, the default branch
behind its remote, and worktrees that are merged and safe to remove.
`Space` marks one, `X` marks every safe one, and `x` with a typed `y`
removes the marked ones.

## 5 History

24 hours of CPU, load, pressure, temperature, memory and swap.

## 6 Panes

Every tmux pane, with its command. `Enter` goes to it.
