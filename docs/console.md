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

Before the first sample, the list says `reading your matches…`. A list
that the screen cuts says what it hides, and how many of those need you:
`↓ 3 more · 1 needs you` under the rows, `↑ 2 more` over them.

A row is one line: the match, the name (with `@ws-1` for a match of
another host while the list shows more than one host), the wait or the
pull request's badge (`#4551 ci`) at the right, then the agent's mark
(`✻` Claude Code, `❋` Codex, `⬢` Cursor, `▣` OpenCode, `·` any other). A
row has a second line only when its match needs you, with what it asks,
or when it is the chosen row, with what it does. A held row says why:
`held by the stoker · not the priority`, or `parked by you · z lets it
go`.

The list shows only sessions. A pull request that no session drives has
no row, and it does not count as needing you (see [Ships](ships.md)). A
session outside tmux is a row by its process: its line says it takes no
text.

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

On a desk, the console runs inside tmux in its home session `matchblox`,
and the right side is not drawn. It is the agent's own pane, beside the
list. Typing there is typing into the agent, with its `/commands`,
`@files`, `Esc`, menus and the mouse wheel. `↑` and `↓` show the chosen
agent there as you move; `Enter` or a click on a row gives it the keys.
`ctrl+q` flips the keys between the list and the agent. A box (a consent,
the launch sheet, a form, the ssh prompt) takes the whole window while it
is open, with the list and the agent dim behind it. See
[The agent beside the list](keys.md#the-agent-beside-the-list).

The chat below is drawn in the demo (and the iOS demo), in a console with
no tmux, and in a console outside its home session or started for a host.

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

### The line

The line is part of the drawn chat. With the agent beside the list, there
is no line: text typed in the list does not go to the agent, and the key
line says `the agent pane takes text · ⏎ goes into flint-auth`.

The line sends one message to the session it names, on that session's
host. The list can move while you type: the message still goes to the
session in the line's name. A message that cannot go stays in the line,
with the reason. Nothing sends it again by itself.

- **A short message** goes as typed text, then `Enter`.
- **A paste** goes in as one piece. A text with a new line or a tab, or one
  over 500 characters, goes as a bracketed paste: the agent gets it as one paste,
  and the console then presses `Enter`. The console strips control
  characters from it, and it takes up to 16384 bytes. A paste works only in
  the console. `matchblox send` does not change.
- **A busy agent** that takes no typed text while it works (OpenCode) gets
  the message when its turn ends. The line says `queued for its next
  turn`. One message waits for each session. Claude Code and Codex take
  the text at once and queue it themselves.
- **A session at a menu**, a question to pick from or a permission
  prompt, gets the message once the menu is answered: typed text would
  pick there, and `Enter` would approve. The line says `queued until its
  question is answered`. Answer the menu with the digits, or in its pane.
- **A session that ends** with a message in wait gives the text back to
  its line: `flint-auth ended · the text is not sent`.

The digits answer the menu the agent draws: a permission prompt, a
question, a Codex or OpenCode choice, options 1 to 9. Claude Code's
question tool goes as its structured answer. Any other menu goes as the
digit, and the service types it only while that option, and the lines
above the menu, still say what the chat showed. A menu that picks many
(checkboxes) is answered in the agent's pane (`Ctrl+O`).

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
again`), and so is a menu that changed (`not sent: <name>'s menu
changed — look again`). A session that needs you with no menu on its
screen says so on the line: `answer in its pane · ctrl+o`.

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

A drag over an agent's words selects them in tmux and copies them when you
let go. A double-click copies a word, and a triple-click copies a line.
This works in every agent: Claude Code, Codex, OpenCode, Cursor and any
other. The selection stays inside the agent's pane, so a paste never holds
the list beside it. tmux drops the padding spaces at the line ends, and a
line that only wrapped on the screen pastes as one line.

A click still reaches the agent. It arrives when the button comes up. A
drag never clicks anything in the agent. The wheel scrolls the agent as
before.

`c` and the menu copy too. A drag over the list's own text copies it.

Each copy goes to the screen that you use: the screen that had your last
key or click in the last 10 seconds. A notice says `copied: 42
characters`. A copy that comes from a host says so: `copied: 42
characters from ws-1`. The copy reaches you over SSH, also from an agent
on another host beside the list.

matchblox never changes tmux's `set-clipboard`. With `set-clipboard on`,
any program in any pane can read your last copy. Instead, the console
puts each copy on your screen with tmux's own `load-buffer -w`, which
works with every value of the option.

A copy from a host goes to your clipboard only while your console shows
that host (its agent beside the list, or a jump into it), and only when
you touched the screen in the last 2 seconds. A program on a host then
cannot fill your clipboard while you look elsewhere.

| What happens | What you see | What to do |
|---|---|---|
| No screen of yours had input lately | `not copied: no screen of yours had input in the last 10 s` | Press a key on the screen you use, and copy again. |
| No console is connected where you copied | tmux says `not copied: no matchblox console is connected · open the console` | Run `matchblox`. |
| The copy is over 1 MB | tmux says `not copied: 1.4 MB is over the 1 MB copy limit` | Copy less. |
| tmux refuses the copy | `not copied: <tmux's words>` | Follow tmux's words. |

An agent's own copy command, such as `/copy` in Claude Code, writes OSC 52.
tmux drops it unless you set `set-clipboard on` yourself. Your terminal's
own selection (Shift-drag on most terminals) does not know the panes: it
takes the list too. Press `ctrl+q` first to show one pane.

On a phone a drag never selects: the menu and `c copy reply` copy.

Once for a phone and once for a desk, the setup step "Check that copy
reaches you" copies a 6-letter code and asks you to paste it. A paste of
that code marks the screen as checked. Until then each copy says
`copied: <what> · not checked here`. A wrong paste says that a hop drops
the copy.

### Shapes

The console reads the screen's shape first: a screen is upright when its
rows × 2 are at least its columns. Only Matchblox Pocket shows one pane
for an upright screen. A desk terminal of the same shape, such as half a
laptop screen at 107 × 60, shows the list beside the chat.

| Shape | Sessions |
|---|---|
| Narrow in Matchblox Pocket (upright, or under 60 columns), and any screen that leaves the chat less than 40 columns | One of the two at a time: the list first. A tap on a row, or `Enter` in the list, opens its chat, and `◂` comes back. |
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
Beside the list, the agent's pane is already on the right: `Ctrl+O` goes
fully into it, and `ctrl+q` flips the keys without leaving the console's
window.
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

With the agent beside the list, `g` does not open the room. It shows the
chain's agents side by side: up to 4 members' own panes on the right, most
urgent first, and `▸` in the list for each one on screen. `G` opens the
room. See [Keys](keys.md#the-chain-view-beside-the-list).

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
