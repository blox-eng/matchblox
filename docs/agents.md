# Agents

matchblox finds every agent that runs in a tmux pane. Start each agent in
a tmux session. Each command gives the console row under it.

**Claude Code**

```sh
tmux new -s app claude
```
```
✦ busy         app        app:1.1   ██████░░░░  62%
```

**Codex**

```sh
tmux new -s api codex
```
```
╿ asks   1m    api        api:1.1   █████░░░░░  55%   ❋ you@example.com · Plus
```

**OpenCode**

```sh
tmux new -s web opencode
```
```
✦ busy         web        web:1.1   ███░░░░░░░  32%   ▣ API key
```

**Cursor**

```sh
tmux new -s app agent
```
```
╿ asks   1m    app        app:1.1   █░░░░░░░░░   6%   ⬢ you@example.com · Acme
```

Cursor's CLI counts whether you start it as `agent` or `cursor-agent`.
matchblox starts it with `--trust`, so its first turn does not wait on
the trust prompt; Cursor
then trusts the workspace's own config (its rules and MCP servers)
without asking.

**Any other agent**, for example aider

```sh
tmux new -s docs aider
```
```
╿ done   4m    docs       docs:1.1        —
```

matchblox knows claude, codex, opencode, gemini, cursor-agent, aider,
crush and qwen. Each one is a session wherever it is installed, before
matchblox's first run or after it. Add any other agent to `agents` in the
[config](config.md) by its command. The Codex app's `codex app-server`
processes are no session.

An agent that keeps no name of its own (all but Claude Code) is named as
its tmux window, which `r` renames, or as its tmux session while tmux names
the window after the command.

## Context use

Claude Code, Codex and OpenCode show their context use, read from the
agent's own files: the transcript, the rollout, the session storage.
Cursor gives it to its status line: open the door "Show Cursor's context"
in Sessions, or run `matchblox setup`, and each Cursor session shows its
context, model and window from its first turn. A status line of your own
keeps running inside it; without one, the line shows what Cursor's bar
showed (`Auto · 5.7%`). Before the first turn a session is "fresh".

Codex writes its context and limits to its rollout, a file under
`sessions` in `CODEX_HOME` (`~/.codex`). matchblox reads the rollout the
session holds open. When it holds none open, and always on macOS, the
rollout is the one in Codex's directory (the one `-C` names, else the
pane's) that began and was written since the session started. The rollout also gives the session its id,
so `z` parks it. Only the CLI's own sessions count: not a `codex exec`
run's, a thread of the Codex app or an editor, nor a subagent's.
`codex resume <id>` finds its rollout by the id. When no open file names the rollout, these
show `—` and "not measured", and have no id:

- two Codex sessions in one directory, even with a different
  `CODEX_HOME` each;
- a session after its `/new` or `/resume`;
- a session that had no turn yet when another Codex session started in
  its directory, one that has exited since;
- a session whose first turn came while matchblox was not running;
- a session resumed without its id (`codex resume --last`, the picker).

A session shows "not measured" while another session in its directory
writes after it.

A `codex exec` run you start in a pane shows in Sessions, and takes no
other session's rollout. One that another agent starts does not show.

The context use is a share of the session's window. matchblox uses the
first window it finds, in this order:

1. The window the agent reports for the session. Codex and OpenCode report
   it in their files. Claude Code gives it to its status line: open the
   door "Show your limits" in Sessions, or run `matchblox setup`, and each
   session reports its window from its first turn.
2. `[sessions.windows]` in the [config](config.md), by model-name prefix.
3. The window of a Claude model that matchblox knows, such as 1M for
   `claude-opus-5-5`. A model setting that ends in `[1m]` gives 1M.

When none of them gives a window, matchblox does not guess one. The row
shows `—` and the detail says "not measured", with what to do. The stoker
then compacts the session at `compact_tokens` only, never at a percentage.

## The account

On a wide screen, Sessions shows the account of each session: the
provider's mark, then the email and plan, or "API key". Only those two
fields are read from the agent's auth files; no token reaches the console.
Cursor's is the email and team its CLI keeps in `~/.cursor/cli-config.json`
(`⬢ you@example.com · Acme`), or in `<folder>/cfg/cli-config.json` for an
account, and USAGE lists the account. Its plan usage,
the share of the month's included usage as Cursor's `/usage` shows it,
needs `cursor_usage = true` under `[limits]`: matchblox then asks Cursor's
API with the login token Cursor's CLI keeps in the macOS Keychain (see
[Safety](safety.md)). Each Cursor account's usage reads that account's
`auth.json` instead (see [Limits](limits.md)). This is not tested yet with
a second real sign-in.

## Accounts

Claude Code, Codex and Cursor can run on more than one sign-in at a time.
Each extra sign-in is an account. The sign-in you already have is the
account `default`. matchblox never writes it. OpenCode has no accounts.
Cursor accounts run on macOS only for now: on Linux, Cursor keeps its
sign-in under `~/.config`, which the account's `HOME` links.

Start a session on an account with `N` (the account line) or
`matchblox new --account work`. Make an account with `+ new account` in
`N`, or with `matchblox new --account work --new-account`. Add
`--setup like-default|copy|empty` to pick its setup. The default is
`like-default`. `--setup` needs `--new-account`, and `--new-account` needs
`--account`.

`N` opens on the agent you last started, and the account line on the
account you last started for that agent. If there is none, or the account
is gone, it opens on `default`. matchblox remembers both across restarts.
A new account becomes the last account only when matchblox sees a session
on it that is signed in. Until then, `N` and Switch agent use the account
from before.
`matchblox new` without `--account` starts on `default` and does not change
the account `N` opens on. A console that has started something keeps its
own last agent. The agent the service remembers, which a CLI start can
change, opens only a console that has started nothing yet, for example
after a restart.

Switch agent (`A`, or `matchblox switch`) starts the new agent on the
account you last started that agent on, while it is still there; else on
`default`. In the console, the confirm line names the account. A switch
never makes an account, and does not change the account `N` opens on. A
switch on a second real sign-in is not tested yet.

Where an account lives:

- Each account is a folder, `~/.matchblox/accounts/<agent>/<name>`, with
  mode 0700. `<agent>` is `claude`, `codex` or `cursor-agent`.
- A name is 1 to 10 characters: lowercase letters, digits and dashes.
  `default` is not a folder. Codex keeps a socket in the folder, and a
  socket path has a length cap: with a long home path, use a shorter name.
- Remove an account: delete its folder.
- A Claude Code or Codex folder you made by hand, such as
  `~/.claude-work`, is listed while a session of that agent runs on it.
  matchblox never makes such a folder, and its row never says
  `signing in`. A Cursor folder counts as an
  account only under `~/.matchblox/accounts/cursor-agent`.
- On a Codex folder you made by hand, matchblox does not add
  `-c cli_auth_credentials_store="file"`: the folder can keep its sign-in
  in the keyring.

The setup of a new account:

- **like default** links the default's entries into the account. Every
  account shares one setup: skills, plugins, settings and hooks.
- **copy of default** copies those entries once. The account can then
  change on its own. Settings can hold tokens: the copies stay on this
  machine, in the account folder. A copy takes seconds, and on APFS files
  are cloned copy-on-write, so they use no extra disk; where the file
  system refuses to clone, a file is copied byte by byte. A link in the
  default that points inside the default points to the same place in the
  account, so the account reads its own files. A link that points
  outside the default, or at an entry the account keeps for itself (the
  lists below), stays on the default's file and is named. The copy stops
  at 200,000 entries, 16 GiB copied byte by byte, or 90 seconds, and names
  each entry it left out. A cut copy is marked, and goes on where it
  stopped at the account's next start. Cloning is tested on macOS only; on Linux it is
  not tested yet.
- **empty** shares nothing. The agent starts as on its first run.

Whatever the setup, matchblox never shares these entries. They stay per
account:

- Claude Code: `.claude.json`, `.credentials.json`, `projects`,
  `sessions`, `history.jsonl`, `backups`, `remote-settings.json*`,
  `policy-limits.json*`, `stats-cache.json` and `state`.
- Codex: `auth.json`, `sessions`, `archived_sessions`, `history.jsonl`,
  `session_index.jsonl`, `*.sqlite*`, `app-server-control`,
  `app-server-daemon`, `packages`, `.tmp` and `installation_id`: the
  accounts do not share one Codex install id.
- Cursor, in `home/.cursor`: `auth.json`, `cli-config.json`, `chats`,
  `projects`, `agent-cli-state.json`, `statsig-cache.json`, `ai-tracking`
  and `managed`.

These lists are checked against the folders of Claude Code 2.1.308,
codex-cli 0.163.0 and Cursor 2026.10.19. A later version can add an entry
that holds state: matchblox then shares it until the list names it.

Each account folder also has `.matchblox-setup`, one line with its setup.
At each start of a `like-default` account, matchblox links the entries the
default got since. It never replaces or removes an entry. When a link
fails, the session starts all the same, and the result says "some shared
entries did not link" with what failed. A copy account
goes on with a cut copy at its start and is otherwise left as it is, and an
empty account stays as it is. A folder with no `.matchblox-setup` is
`like-default`.

User scope MCP servers sit in `~/.claude.json`, which matchblox does not
share, as it can hold tokens. In an account, use a plugin or a project
`.mcp.json` instead.

Cursor runs an account with a `HOME` of its own. That `HOME` links the
entries of your real `HOME`, except `.cursor` and `.matchblox`, so git,
ssh, gh and your shell work as usual. matchblox starts an account with
`AGENT_CLI_CREDENTIAL_STORE=file`, so Cursor keeps the sign-in in a file
and not in the Keychain entry of your default login. This is not tested
yet with a second real sign-in. Never set `CURSOR_API_KEY` for an account:
it replaces the account's sign-in with the key's.

A start or a switch on an account drops the sign-in environment that the
tmux server can pass on, so the account's own sign-in is used:

- Claude Code: `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` and
  `ANTHROPIC_AUTH_TOKEN`.
- Codex: `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN` and `OPENAI_API_KEY`.
- Cursor: `CURSOR_API_KEY` and `CURSOR_AUTH_TOKEN`.

A start on `default` keeps them.

Codex: each account's sessions read their logs and limits from that
account's home, never from the default's. The app-server process (`codex
app-server`) is not a session: Sessions does not list it. An agent that
the app-server runs (such as `codex exec`) is not a session either.

A start or a switch on an account says what happened: each link still
shared with the default and each entry not copied are named (five of each,
then "and N more"), and so is a copy that is cut and goes on at the next
start. A start on an account that another start is setting up is refused
at once, and so is one whose folder went away meanwhile. A session in an
agent's pane cannot start a session on an account, even as a console.
When a switch's confirm named an account and the agent's last account
changed meanwhile, the switch is refused and nothing moves.

A new account starts in a session where you sign in. matchblox presses
through the first screens of the agent to its sign-in link: for Claude
Code, the theme and the login method; for Codex, the device code; for
Cursor, any key. It presses a key only when the option the key picks is
the highlighted one, looks at the screen again right before the key, and
sends it only to the agent it first saw in the pane. It stops for good when
you type in the pane, and when the screen changes in any other way: a
reflow when a client of another size attaches counts. You finish the
sign-in in the session. Until then the row says `signing in`. If matchblox
does not recognise the screens within 20 seconds, cannot find the pane, or
stopped because you typed, the row says `sign in by hand`. These screens
were checked on codex-cli 0.163.0 and Claude Code 2.1.308 up to the login
choice; Cursor's screen and the sign-in after the choice are not tested
yet. A key you type between its last look and its key can still be
answered: tmux cannot close that gap. An ask or a wait of the session shows over both words. matchblox
never reads a sign-in code from the pane and never types a password. If a
new account does not start, matchblox removes the folder it made for it.

`N` names each account folder it leaves out of the account list and why
(`not listed: Team (not a-z, 0-9 and -, up to 10)`; three named, then
"and N more"), and says `accounts not read:` when the accounts folder
cannot be read. A default folder reached by a link or in another letter
case is the default, not an account.

In the console, a start on a new account asks for a typed `y`, and a start
on an account that exists runs on `Enter`. `matchblox new` sends the
confirm itself, waits as long as a copy can take, and says `sign in in
<session>` for a new account. It says "refused" when matchblox refused the
start, "failed" when it ran and failed, and "stopped after" the step that
ran when a guard stopped the rest. `matchblox sessions` says "sign in by
hand" on the line of a new account matchblox gave up signing in.

## How an agent tells that it waits

Claude Code tells its waits through hooks: the setup step "Add the queue
hooks" adds them. One of them, `PreToolUse` for the question tool
(`AskUserQuestion`), hands the console the question and its options before
the menu draws: in auto mode Claude Code writes the question to its
transcript only once it is answered, so the hook is how the answer peek
shows the options. A second, `PreToolUse` for `Bash`, is the stoker's merge
guard: in a session the stoker drove, it stops `gh pr merge`, `gh release`
and `gh workflow run` unless the chip on the launch sheet gave the go, or
you gave it with `y` on the STOKER tab. The same step adds permission rules
(`Bash(matchblox sessions:*)`, `ships`, `usage`, `send`, `compact`, `hold`,
`release`, `stoker` and `peek`), so the stoker's hands run without a
prompt. The rules are in your user settings, so every Claude Code session
may call these commands without a prompt. The service refuses a `send`,
`compact`, `hold` or `release` typed in a pane that hosts an agent and is
neither the stoker's nor one the stoker drives (`only the stoker and the
sessions it drives act from an agent's pane: this one is neither`). Your
own shell runs no agent, so the commands work there. `peek` and the
other reads work from any pane. An install from before these hooks or
rules shows the step again, with the entries it adds. Every other agent is read from its pane: busy
while the pane changes, waiting when it is idle and its last lines ask.

Cursor tells its turns through its own hooks: the setup step "Add the
Cursor hooks" adds `matchblox hook --cursor` to `~/.cursor/hooks.json`, for
`sessionStart`, `sessionEnd`, `beforeSubmitPrompt`, `stop`,
`afterAgentResponse` and `postToolUse`, and for nothing else: Cursor
ignores the whole file, without a word, when one event in it is one it
does not take. A turn that ends waits for you as `done`, or `asks` when its
reply (from `afterAgentResponse`) ends on a question; one you stop with Esc
does not. No Cursor hook fires when its approval menu
(`Run this command?`) draws, so that wait is read from its pane. The hooks
are global, so an `agent -p` you run counts only in a pane where Cursor
runs.

An approval menu (Codex, OpenCode, Cursor) shows as `asks`. Answer it in its pane,
where `Enter` takes the selected choice.

See [Limits](limits.md) for what is not measured yet.
