# The console

From the top: the header, the tab row, the list beside the chat, and at
the bottom the key line.

The header shows the host, the host filter (`all hosts ▾`) when you have
more than one host, how many matches need you (`● 3 need you`) and
[the stoker](stoker.md): `stoker off`, `stoker`, or `☾ night → 07:00`.
A tap on `● 3 need you` shows `NEEDS YOU`, clears the chain and host
filters, and chooses the first session that needs you. The count holds
every wait on every host and in every chain, whatever the list hides.

The tabs are `sessions` and `machine`, turned by `Tab` and `Shift+Tab`,
or a tap on the name. The open one is uppercase. `machine !` has a
problem: a detached busy loop burns, or a plan's window is at 90% or
more. The stoker, Git, History and Panes are Machine's first rows:
`Enter` opens one, `Esc` comes back to Machine.

The key line at the bottom names where the keys go first, `writing to
flint-auth` or `in the list`, then at most five keys for that place. A
tap on a key presses it. It is also where you confirm a step or type a
search. See [Keys](keys.md).

## The list

The console opens on Sessions: the list of every match at the left, and
the chat of the chosen match at the right. The list puts each match in
one of four groups:

| Group | A match is here when |
|---|---|
| `NEEDS YOU` | It asks a question or a permission, or a stoker step for it waits for your go. A pull request it drives waits for your merge, and it does not work. Its pull request is stuck (`CI failed`, `conflicts`, `changes requested`) and it does not work. Or its turn ended today and it waits for input (`waits`, `paused`, `done`). |
| `WORKING` | It is `busy`, `agents`, `tests` or `shell`. |
| `HELD` | You parked it (`z`), or the stoker holds it and it asks nothing. |
| `IDLE` | Everything else: `idle`, `cold`, `stale` and `out`. |

A match you parked is in `HELD` whatever else is true, and the header
does not count it. A wait from before today is cold, and cold is `IDLE`.
Otherwise a match is in the first group whose rule holds. So a match the
stoker holds is in `NEEDS YOU` while it asks, and the header counts it.

Inside a group the oldest wait or the oldest start is first. `IDLE` puts
the newest first. A phone and a desk show the same order. A group with
no match takes no row: the header's count says it.

Before the first sample, the list says `reading your matches…`.

A row is one line: the match, the name (with `@ws-1` for a match of
another host while the list shows more than one host), the wait or the
pull request's badge (`#4551 ci`) at the right, then the agent's mark
(`✻` Claude Code, `❋` Codex, `⬢` Cursor, `▣` OpenCode, `·` any other). A
row has a second line only when its match needs you, with what it asks,
or when it is the chosen row, with what it does. A held row says why:
`held by the stoker · not the priority`, or `parked by you · z lets it
go`.

A pull request that no session drives has a row of its own, named by
its branch: in `NEEDS YOU` while it waits for your merge or is stuck,
else in `IDLE`. `Enter` or `N` on it starts a session on its branch (see
[Ships](ships.md)). A session outside tmux is a row by its process: its
line says it takes no text.

A chain is a head row, `≋ login   step 2 of 3 · 3`, then its members in
step order, each with a link (`├`, the last `└`). The chain sits in the
group of its most urgent member, and all its members draw under its head
there. Each group's count counts only the matches in that state.

The foot is one line: `+ new · ✧ 2 tips · ! 1 setup`. Each part opens in
the chat's place: `+ new` opens **New session**, `✧ 2 tips` the tips, `!
1 setup` the first open setup step, and `! ws-1 not reached` (or `!
ws-1 blocked`) that host's trouble and its fix. `Esc` gives the place
back to the chat. A part that has nothing to show takes no room.

The match of a row plays where it is. The match of each session that
asks breathes. A burning match flickers. `--no-motion` shows each end at
once.

### Where a lane comes from

An agent that reports its own state decides its group. matchblox reads
the pane only when no report exists.

- **Claude Code** reports its state in its own files. `waiting` puts the
  match in `NEEDS YOU`, with the reason Claude Code gives (`permission
  prompt`, `input needed`). `busy` and `shell` put it in `WORKING`. At
  `idle`, a turn that ended on a question asks, and a turn that ended
  today waits.
- **OpenCode** reports its state through the plugin that the setup step
  "Add the OpenCode plugin" installs. A permission or a question puts the
  match in `NEEDS YOU` until OpenCode has its answer. A retry stays in
  `WORKING`, and its row says `retrying · attempt 2`.
- **A report of a process that ended is no report.**
- **While an agent reports `busy` or `shell`, an old question does not
  put it in `NEEDS YOU`.** A stoker step that waits for your go
  still does.
- **A restart of the service loses no wait.** A waiting report is a wait
  again at once.
- **Other agents** (Codex, an OpenCode with no plugin, any agent in
  tmux): the last lines of the pane decide. matchblox does not read the
  older lines above them.

## The chat

The chat shows the chosen session: a head line, its conversation, then
its question with the options when it asks, and at the bottom its line,
`›   ⏎ send to flint-auth`. The head line holds the name, `@host`, the
agent's mark, the state word, the pull request's badge, the context use
(`ctx 62%`, `warn` from 70%, `neg` when the context is full), and
`ctrl+o expand · m menu`. A session in a chain has one more line: the
chain, its step, and the state of each member.

The line under the chat's head names the facts of the chosen session, and
shows only when it has one: the fix for tokens with no window known
(first), a chain mate's inbox (`✉ 1 in its inbox`), an account that is
not the default, the pull request when no ship names it, the subagents,
the shells and the child runs. The line is cut at the chat's width. The
window fix is never cut: when it does not fit with the other facts, it
wraps on lines of its own, and the other facts come after it.

The digits answer a question with one choice, given with its options
(Claude Code's question tool). Answer every other wait in the agent's
pane (`Ctrl+O`): a permission prompt, a question from Codex or OpenCode,
a call with more than one question, or a choice of several options.

- **The first press marks.** A digit, or a tap on an option, marks that
  option (`❯ 2.`), and the key line says `2 again sends · esc clears`.
  Nothing is sent.
- **The second press sends.** The same digit, or a tap on the same option,
  sends it. Another digit moves the mark. `Esc`, a move to another
  session, or a new question clears it.
- **After the send** the chat says `sent 2 · waits for <name>`, and its
  digits do nothing until the question changes. A notice says where the
  session went: `sent 2 to flint-auth · now in WORKING`.

For 400 ms after the chat opens on a session, after the console opens,
and after a tab or a filter changes, a digit marks nothing. A digit is
sent only while the agent asks the question the chat shows: a newer
question is refused (`not sent: <name> asks something new — look
again`). A question that the chat cannot answer says so:
`<name> asks in its pane · ctrl+o goes in`.

After an answer, a merge or a go, the list keeps the session chosen.

### One chain, one host

- `g` shows only the chain of the chosen session: the list narrows to
  the chain, and the chain's room takes the chat's place (see
  [A chain's room](#a-chains-room)). `Esc` shows every chain again.
- `s`, or a tap on the host filter in the header, shows the next host,
  then all hosts again. A row of another host says `@ws-1` after its name
  while the list shows more than one host. `Ctrl+O` on it goes into its
  pane.
- A host that matchblox cannot reach is a part of the list's foot:
  `! ws-1 not reached`. `Enter` on it shows
  `ws-1: not reached since 14:02.` in the chat's place, and `Enter` there
  dials it again. Its rows stay, faint, and the header does not count
  them.

### Notices

A notice shows above the key line for 2.5 seconds: what ran, what was
refused and why, and the age of the data when the service is slow
(`data is 40 s old · the service is slow`). A notice that does not fit
wraps, up to three lines. The header never shows a notice.

A key that changed its job says so once, the first time you press it:
`c copies now: the last message, whole.` Each says it once ever.

### The menu

`m` in the list, a right-click on a row, or a long-press on a phone,
opens the session's menu, under its row. `↑` `↓` move, `⏎` or a tap does, `Esc` or a tap outside
closes. Each line shows only where it applies:

1. `copy last message` (also `c`, with no menu)
2. `copy the question`
3. `copy the link of #4556`
4. `copy its name`
5. `show only the chain login` (also `g`)
6. The acts that destroy nothing, each with its key: `rename` (`r`),
   `stoke the chain login` (`F`) and `a new order` (`o`).
7. `go in` (also `Ctrl+O`)
8. `out of login`, only while the list shows that chain: it takes the
   match out of the chain, after a typed `y`.

The menu destroys nothing. A kill (`x`), a merge (`p`), a go (`y`), a
switch of the agent (`A`) and the stoker's stop (`f`) are keys of the
list. A kill and a merge show their commands in the consent box first,
and run on a typed `y`.

### Copy

`c` and the menu copy to your clipboard. A drag over text on a desk
selects it and copies it when you let go: a notice says `copied 42
characters`. On a phone a drag never selects: the menu copies.

The console copies with OSC 52, through each hop between it and your
screen. When the console's own tmux drops the copy, a notice says
`not copied: tmux drops it · tmux set -s set-clipboard on`, and the setup
step "Let copy through" makes that change on a typed `y`. tmux passes a
copy on only with `set-clipboard on`: `external`, tmux's default, and
`off` drop it.

The console cannot see the hops after its own tmux. So, once for a phone
and once for a desk, the setup step "Check that copy reaches you" copies a
6-letter code and asks you to paste it. A paste of that code marks the
screen as checked. Until then each copy says `copied: <what> · not checked
here`. A wrong paste says that a hop drops the copy.

| Hop | The setting it needs |
|---|---|
| The console's tmux | `set -s set-clipboard on`. The setup step "Let copy through" sets it. |
| Any other hop (SSH, your terminal, a tmux on your side, Matchblox Pocket) | Not checked yet. Each must pass OSC 52 on. Run "Check that copy reaches you" to prove the whole way. |

### Shapes

The console reads the screen's shape first: a screen is upright when its
rows × 2 are at least its columns.

| Shape | Sessions |
|---|---|
| Narrow (upright, or under 60 columns), and any screen that leaves the chat less than 40 columns | One of the two at a time: the list first. A tap on a row, or `Enter` in the list, opens its chat, and `◂` comes back. |
| Medium (60 to 99 columns) | The list, 32 columns wide, and the chat beside it. |
| Wide (100 columns and more) | The list, 40 columns wide, and the chat beside it. |

A keyboard that shrinks the screen never changes the shape: the shape
goes by the tallest height seen at the width. `Ctrl+L` hides the list on
any screen, and the chat takes the width.

### A new release

The setup steps hold the release steps: `! 1 setup` in the list's foot
opens the first. See
[Config](config.md#update) for each mode's rules.

| Step | When it shows |
|---|---|
| Update to v0.43.0 | The service found a newer release. `Enter` shows what it installs and what it checks first. A typed `y` installs it. The service and the console then start again on it, or the console says `quit and open matchblox again`. A failed install says so in the step until the next try: nothing was changed, `Enter` tries again, then the reason. With `mode = "auto"` it also says when auto installs it (`Auto installs it after Nov 13 at 09:00.`), or why auto waits: `Auto waits for the stoker run to end.` |
| Restart for v0.43.0 | The release is installed, and the service still runs the old one two minutes later. Quit matchblox and open it again. It adds `· provenance not checked` when no `gh` checked the build. |
| Release check fails | Every look for three days failed. It names the reason and `Run: matchblox update`. |

With `[update] mode = "auto"` the service installs a release by itself.
The console then says once `updated to v0.43.0 at 03:12`, or
`updated to v0.43.0 on Nov 11 at 03:12` when it was not today. It adds
`· provenance not checked` when no `gh` checked the build.

A console left open never puts its older build back over a newer service.
The key line says `R start again · service v0.43.0, console v0.42.0`, and
`R` starts the console again on the new release. When the console's own
binary is not the new one, it says
`quit and reopen · service v0.43.0, console v0.42.0` instead. On Pocket,
which has no key line, the row above the belt says it, and a tap on
`R start again` starts the console again.

A host on another release says so on its row in Hosts. See
[Remote hosts](remote.md#a-host-on-another-release).

### Go in

`Ctrl+O` goes into the chosen agent's pane, from the line or the list.
The key line then says the way back, `ctrl+a m`. The app's edge swipe,
⌘M or ctrl+M (the home gesture) comes back too, to the chat you left.

The iOS demo has no tmux: there `Ctrl+O` opens a stand-in for the pane:
the agent's screen over the whole screen, and every key goes to the
agent. Its title is `‹ sessions`. At a question its key line says `type
2 to answer`. `ctrl+a m` or the home gesture goes back to the list, on
the next session that waits.

- Context use is read from the agent's own files, as a share of the
  session's window. See [Agents](agents.md).
- A new session shows in the list at the next sample, about 2 seconds
  after it starts. A session from `N` or `matchblox new` shows at once.
- A session on an account folder shows the account's name after its own
  name, in muted text (`api work`). Until the account is signed in, its
  row says `signing in`, or `sign in by hand`. See
  [Agents](agents.md#accounts).
- An agent can draw a progress bar in its last reply:
  `Progress [████░░░░] <step>`. The console shows it as `▰▰▰▱▱ 75%`.
- `z` parks the chosen session, or lets it go.
- Agents that exited and wait for their parent are one faint part of the
  list's foot, `· 1 exited`. `e` lists them with the command that clears
  them.
- A `codex exec` or `claude -p` that an agent starts shows in
  `matchblox sessions` under that session, with `├─` and `└─`. The console
  never acts on it.

## Usage

Machine shows each account's windows, with the machine. A window at 90%
or more is named at the right of `USAGE`, for example
`nearly spent: OpenAI Plus wk 92%`, and the tab says `machine !`.

```
 USAGE
 ✻ Claude Max  5h   ▰▰▰▰▰▱▱▱ 62%  · 16:00
               wk   ▰▰▰▱▱▱▱▱ 41%  · Thu 09:00
 ❋ OpenAI Plus 5h   ▰▱▱▱▱▱▱▱ 12%  · 18:40
               wk   ▰▰▱▱▱▱▱▱ 30%  · Sat 11:00
```

- Each account starts with its provider: its logo in kitty or Ghostty
  (through tmux too) while every terminal on the console's session can
  draw it, else its mark. Then the provider's name and the plan
  (`✻ Claude Team`, `❋ OpenAI Team`), or the email's name when the
  provider has two accounts. With two or more hosts, a window's host cells
  that do not fit go whole onto the lines under it.
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
- `disk` is the file system that holds your home directory, and `/` beside
  it when that is another one.
- Claude Code's status line gives the 5-hour window and the week only, so
  there is no `opus` or `sonnet` line.

Where the figures come from:

| Agent | What to do |
|---|---|
| Claude Code | Open the setup step "Show your limits" from the list's foot. It adds matchblox to your status line; a status line you have keeps showing what it showed. |
| Codex | Nothing: Codex reports its limits after each turn, on Linux and macOS. A plan with a monthly limit shows `mo`. |
| OpenCode | It shows "not measured": OpenCode keeps no limits. |
| Cursor | Set `cursor_usage = true` under `[limits]` in the [config](config.md) (macOS). Cursor shows `mo`, the share of the month's included usage. See [Agents](agents.md#the-account). |

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
| `repo` | The name of the repo its folder is in (`web`; `work/api` when another repo is also `api`), or empty when matchblox knows no repo there. The line says `in web`. |
| `word` | What it does, in one word: `asks` (it waits for an answer or an approval), `busy` (a turn runs), `idle` (at rest), `background` (at rest, and a shell it started in the background still runs), or `agents` (its subagents still work). Read this one. |
| `status` | The status word the agent itself reports, as it reports it. Claude Code says `busy`, `idle`, `shell` or `waiting`; `shell` is a session at rest with a background shell, not a pane without an agent. OpenCode with the matchblox plugin says `busy`, `idle`, `waiting` or `retry`. `waiting` is a session that asks (its `word` is `asks`). `retry` is OpenCode that tries a failed call again by itself (its `word` is `busy`). A script must accept a word it does not know. |
| `reason` | Why a session at rest is idle (`#4556 waits your merge`, `CI red · …`, `stuck: …`, `out of work`, `idle`); empty while it works or asks. |
| `held` | Nothing types into it: you parked it, or the stoker held it. |
| `stoker` | It is the stoker agent, on matchblox's own tmux server. The list leaves it out. |
| `context_pct` | How full its context is, in percent. |
| `plan_pct` | The progress bar it drew in its last reply, or null. |
| `account` | The account its plan runs on: the email and plan, or `API key`. Empty when matchblox reads none, as on an account not signed in yet. |
| `account_dir` | The account folder it runs on, or empty on the default sign-in. See [Agents](agents.md#accounts). |
| `signing_in` | It runs on an account folder that is not signed in yet. |
| `ship` | The pull request it drives (`repo`, `number`, `stage`, `kind`, `note`), or null. |

`matchblox peek api` prints what the session said last and what it asks,
if anything: the question with its numbered options, or an approval that
waits in its pane. It acts on nothing. `--lines 40` keeps the last 40 lines of what it said (20 by
default, 0 for all); `--json` prints `name`, `pane`, `target`, `said`,
`asks` (the first line of the question), `questions` (each with its
`options`) and `approval`.

## Chain matches

A chain is a team of matches on one host that all hear each other, in one
repo or across repos. Run in an agent's pane, `matchblox chain api` joins
the chain of `api` at once, or forms one with it; `matchblox chain hearth`
joins the chain named hearth. A match may be in several chains, and each
hears only itself. `matchblox chain` lists your chains (`--json` prints
`chains`). `matchblox unchain [<chain>]` leaves your chain, or the one
named; `--all` leaves every chain. `matchblox chain --off` takes no chains
and no notices; `--on` takes them again. See [chains](agents.md#chains) for
what a match hears and what a chain grants.

In the list, a chain is a head row with its members under it (see
[The list](#the-list)). When the chosen session is in a chain, the links
of that chain light up. A match in two chains shows the chain that formed
first. A match is never drawn twice.

- When a match joins a chain, a notice says so for 2.5 seconds
  (`docs-bot joined hearth`). A new chain shows `hearth formed: hearth,
  web-cart`. The notice shows with `--no-motion` too.

## A chain's room

One chain and its stoker are the chain's room. `g` on a session of the
chain in the list, or `⌥s`, opens it; `⌥s` in an agent's pane flips there
and back (see [Keys](keys.md#a-chains-room)). The list shows only the
chain's sessions, and the room takes the chat's place, `CHAIN hearth` at
its head:

- **The stoker's card**: its order in quotes, the active step with one
  step on each side, then `↓ N more`. `≋ waking…` while a new stoker
  starts; `the stoker did not start: <why>` when it was refused;
  `≋ hearth's stoker is quiet since 14:02 · ⏎ asks it` when its run has
  said nothing for 60 s. `y` on a step gives the go it waits for.
- **The talk line**, under the card: `› tell hearth's stoker…`. Your words
  go to the chain's stoker; to a chain with no stoker they start one
  (`› what should hearth's stoker do?`).

On a phone the room takes the screen, and `◂` goes back to the list.
`Esc` closes the room and shows every chain. In the room, the menu of a session adds
`out of hearth`: it takes the match out of the chain after a typed `y`.
Its agent goes on, and the chain hears that it left.

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
| `matchblox kill api` | Ends the session: `tmux kill-pane`, then a `TERM` to its agent if it still runs. It is refused while the session works or needs you: `--anyway` kills it all the same. Its worktree and its transcript stay. |
| `matchblox switch api --agent codex` | Saves handover evidence, then replaces the agent in the same pane: same name, directory, park, ship and chain. The new agent starts on its last account, else `default`, with match awareness and a prompt to continue from the saved evidence. Codex starts with workspace trust and workspace-write network access. `--extra "<words>"` adds your instructions for this switch after that prompt, and they win over the handover; `--extra` with `--handoff` or `--preview` is refused before the service is asked. `--handoff "<line>"` replaces the prompt with your exact task; `--handoff ''` starts no task and captures no handover. While the match works or waits, `--anyway` switches it mid-turn. Inside tmux, your client jumps after the switch runs. An idle Claude Code or Codex agent is first asked to write its task record, and the switch waits up to 3 minutes for it, with a line on stderr; `Ctrl-C` switches now from the evidence, and `--no-refresh` skips the wait. |
| `matchblox switch api --agent codex --preview` | Reads the current handover evidence. It sends no agent input, writes no context files, and replaces no process. |
| `matchblox recover` | Lists the agents your tmux server ran, from the recovery record: each with its tmux session, agent, folder and state. **live**: an agent runs that conversation now. **can reopen**: its folder is there, on the branch it had, and its saved history is on disk. **cannot reopen**: the line says why and the fix (a folder that is gone, another branch or another repository, no saved history, an agent that is not installed, a conversation that another Claude Code process has open, or a session that ran on Bedrock, Vertex or an API key set in your shell: that sign-in came from your shell, so the line gives the command to run there instead of reopening it on your default sign-in). It changes nothing. The service keeps the record in `recover.json` in its state folder while your agents run: the agent, its conversation (it follows `/clear` and `/resume`), its tmux session, folder, checkout and tmux server. An agent you quit leaves the record. The agents of a server that died stay in it for 14 days. The record holds no transcript, prompt or environment value. Claude Code, Codex and Cursor are recorded; a Cursor line says its history is not checked, as matchblox does not read Cursor's history. `--json` prints the lines as JSON. |
| `matchblox recover --all` | Reopens each conversation that can reopen, each in a tmux session of its name on your server (`api-2` when `api` is taken), in its folder and on its account: `claude --resume <id>`, `codex resume <id>` or `cursor-agent --resume <id>`. It sends no prompt and replays no command: each agent waits for your next prompt. It says reopened only for an agent that still runs a moment later, `remain-on-exit` or not; one that exited at once is a cannot line with the command to run to see why, and its session is closed. It runs no git command that changes anything. A conversation that runs is left alone, so a second `recover --all` starts nothing. A line that could not reopen fails the command, with its fix. `matchblox recover api web` reopens those sessions only. `matchblox recover api --dir ~/src/api-moved` reopens a session whose folder moved, when the new folder is a checkout of the same repository (the same `origin`, or the same first commit) on the same branch; a relative `--dir` is read from the folder you run it in. An agent's pane may list them, but only you reopen them. |
| `matchblox scan` | Finds the Git repositories in your home folder (the folder itself, `~/dev`, `~/src`, `~/code`, `~/projects`, `~/work`, and `[git] repos` and `scan_roots`), four folders deep, so N's `where` offers them. They never show in Machine's git view. Running it is your yes: the service scans again every 30 minutes. `matchblox scan ~/w ~/oss` scans those folders instead of the usual ones; `matchblox scan off` stops the scans and forgets the list. It reads files only, runs no git and calls nothing on the network; the list stays on this host and the devices paired with it. On a Mac it skips the folders macOS guards unless you name them, and it skips network folders below a root; it waits as long as a scan can take, then prints what it found, each folder it skipped with its reason, and any error. An agent's pane cannot say yes for you. |

An act typed in a pane of the stoker's tmux session is the stoker's, and so
is an act from a session the stoker messaged in the run: what that session
sends counts toward the run's messages, and the session it sends to is the
stoker's from then on. Such an act is refused when no run is on (`the
stoker is off: no run`), while the stoker rests (`the stoker rests: a person
works`), on its own pane, and after `max_steps` messages in one run (60 by
default).

The launch sheet's and the stoker screen's verbs, and `z`, are yours at a
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

Each row starts with a match:

| Mark | Word | It means |
|---|---|---|
| `╿` | asks, waits, done, paused | The agent needs you. The match is struck, and its head breathes while it asks. |
| `✦` | busy, agents, tests, shell | The agent works. The match burns. `agents`: its turn ended and its subagents still work, and their results start its next turn. It does not need you. |
| `╽` | held | The stoker holds it, or you parked it. |
| `╵` | idle, cold, stale, out | The agent rests. `out`: the plan's limit stops it, and the row says when it can work again: `19:50`, or the weekday when that is more than a day away. A session left a day or more keeps its word: `cold`, `stale`. |

A full context (at `compact_tokens`, 350k by default, or `compact_at`,
85% of its window) shows on the chat's head: `ctx N%` in `neg`.

## Machine

The second tab. `↑` `↓` choose a row, `Enter` opens it over the body, and
`Esc` comes back to Machine on its row.

- **stoker**: the stoker's screen: your words and the gos, the plan with
  each step's outcome, the goes that wait for your `y`, the triggers, the
  standing rules, and today's compacts with their RESUME lines. Opening it
  marks the steps seen. See [the stoker](stoker.md).
- **git**: every git checkout that your sessions use: dirty paths, the
  default branch behind its remote, and worktrees that are merged and safe
  to remove. `Space` marks one, `X` marks every safe one, and `x` with a
  typed `y` removes the marked ones. `N` starts a session in a worktree.
- **history**: 24 hours of CPU, load, pressure, temperature, memory and
  swap.
- **panes**: every tmux pane, with its command. `Enter` goes to it. `s`
  sorts by the next column, and a tap on a column's name sorts by it.

Under the rows: usage (see [Usage](#usage)), then the machine: CPU per
core, load, pressure, memory and swap, temperature, CPU power limits,
GPUs, and the CPU share of the container groups you name in the
[config](config.md).

Then the detached busy loops: shells that keep burning CPU after their pane
is gone, named to the pane that started them. `x` and a typed `y` kill
one. While one burns, the tab says `machine !`.

Then `DEVICES`: each phone and key that opens this host, and `+ pair a
device`. See [The phone](phone.md).

In a console over several hosts, a row for each host follows. `Enter` on
one, or a second tap, opens Hosts, where you add or remove a host.
