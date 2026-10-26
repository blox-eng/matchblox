# Accounts on N: design

Date: 2026-10-22. Status: proposed. Issue: #170.
Related: #55 (accounts by hand, moving a blocked session), `design/0011-agent-switch.md` (`launchArgv`).

## Intent

A builder has several accounts of their own for each agent: for example
three Claude Code accounts, two Codex accounts and two Cursor CLI
accounts, personal and work. They run sessions on all of them side by
side. Today matchblox shows the limits of each Claude and Codex account
that a live session runs under, but N starts every session on the
default account. A second account means a hand-typed
`CLAUDE_CONFIG_DIR=… claude` in tmux, and for Cursor there is no way at
all: its sign-in is one keychain entry.

With this change, N asks for the account. The builder picks a known
account, or **new account**. A new account gets its own folder, the
agent starts there, and matchblox drives the agent to its sign-in
screen. The builder finishes the sign-in in the session. From then on
the account is in the list, and USAGE shows its limits next to the
others.

## Goals

1. **Pick the account on N.** The N form has an `account` field after
   `agent`, for `claude`, `codex` and `cursor-agent`. `matchblox new`
   takes the same choice as `--account <name>`.
2. **A new account signs in inside its session.** matchblox makes the
   folder, starts the agent there, and sends the keys that reach the
   agent's sign-in link. The builder opens the link and finishes.
3. **Accounts run side by side.** Sessions on different accounts of one
   agent run at the same time and never share a sign-in.
4. **USAGE shows every account.** Each account that a live session runs
   under has its own block in USAGE, for all three agents, with its
   name. Each session row names its account.
5. **The default stays the default.** N without an account choice starts
   exactly as today. A builder with one account sees no new step.
6. **Nothing breaks for a builder with plain dotfiles.** The Cursor
   account home (below) works on any `$HOME`, not only on a setup like
   the author's.

## Non-goals

- OpenCode accounts. OpenCode keeps one `auth.json` per data folder;
  it can follow later.
- Moving a blocked session to another account, and picking the account
  with the most room by itself (#55).
- Signing in for the builder. matchblox never types a password, never
  completes an OAuth step, and never reads a sign-in code from the pane.
- Removing an account (delete its folder by hand; documented).
- An account picker in Switch agent (A). A starts on the agent's last
  account (built later: see Revised while building).
- The account picker on the phone and in the iOS app.

## Anti-goals

- **Touching the default sign-in.** A new account must never write the
  default keychain entries or `~/.claude`, `~/.codex`, `~/.cursor`
  credentials. For Cursor this rules out `CURSOR_API_KEY` and any
  sign-in on Cursor's default store: both overwrite the one keychain
  sign-in (see Facts).
- **A copy of a secret.** matchblox reads a token only where the agent
  keeps it, as it does today for Cursor usage. It never copies a token
  into its own files or logs.
- **Hiding the account.** A session on a second account must never show
  under the default account's name or limits.

## Facts (probed 2026-10-22 on macOS)

Probed with Claude Code 2.1.308, codex-cli 0.163.0 and Cursor CLI
2026.10.19-6bbf2c4, in scratch folders. No sign-in was completed; every
row is tested up to the sign-in link or code unless it says "from code".

| | Claude Code | Codex | Cursor CLI |
| --- | --- | --- | --- |
| Account key | `CLAUDE_CONFIG_DIR` | `CODEX_HOME` | `AGENT_CLI_CREDENTIAL_STORE=file` + `HOME` |
| Sign-in kept in | keychain `Claude Code-credentials-<sha256(dir)[:8]>` | `$CODEX_HOME/auth.json` (default store `file`); keyring account `cli\|<sha256(realpath)[:16]>` | `$HOME/.cursor/auth.json`, mode 0600 |
| Fresh account shows | theme picker → login method → URL | sign-in picker | "Press any key to log in…" → URL |
| Keys to the sign-in link | `Enter`, `Enter` | `Down`, `Enter` (device code) | `Enter` |
| Account name read from | `<dir>/.claude.json` `oauthAccount` | `$CODEX_HOME/auth.json` | `$CURSOR_CONFIG_DIR/cli-config.json` `authInfo` |
| Limits from | status line tap keyed by the dir | rollout files under `$CODEX_HOME/sessions` | Cursor API with the token in that `auth.json` (from code) |

Details that shape the design:

- **Claude** hashes the `CLAUDE_CONFIG_DIR` string as given: no realpath,
  no trailing-slash trim. One spelling per account, always.
  `BROWSER=/usr/bin/true` stops a browser from opening; the URL still
  shows in the pane. The status line and hooks need
  `<dir>/settings.json`, which `CLAUDE_CONFIG_DIR=<dir> matchblox setup`
  writes.
- **Codex** refuses a `CODEX_HOME` that does not exist. Its daemon puts a
  socket at `$CODEX_HOME/app-server-control/app-server-control.sock`;
  macOS caps a socket path at 104 bytes. The browser sign-in binds port
  1455 and cancels another sign-in on that port; device code uses no
  port. In keyring mode there is no `auth.json`, so no account name. The
  TUI hands work to a per-home daemon that outlives the pane; whether
  the daemon or the TUI holds the rollout open is not tested.
- **Cursor**'s keychain names are fixed (`cursor-access-token`,
  `cursor-refresh-token`, account `cursor-user`); `CURSOR_CONFIG_DIR`
  moves only `cli-config.json`. `AGENT_CLI_CREDENTIAL_STORE=file` uses
  `os.homedir()`, so the account needs its own `HOME`. A valid
  `CURSOR_API_KEY` is exchanged and saved to the active store (from
  code), so on the default store it replaces the keychain sign-in.
  Cursor has no `/login` command; a session with no sign-in shows the
  sign-in screen by itself.

## Decisions

### The builder's

- **An account is another sign-in, not another setup.** #55 says it:
  the account folder "shares everything but the login" with the
  default. Skills, plugins, CLAUDE.md, AGENTS.md, MCP servers, rules,
  hooks and the status line are the default's, through links.
- **Cursor accounts sign in inside the session, on their own `HOME`,**
  not on an API key. The builder accepts linking the dotfiles into that
  home, as long as a builder with other dotfiles gets a working setup
  too.
- **matchblox triggers the sign-in** by sending keys to the new pane.
- **A new account offers how to start its setup** (the builder's ask,
  2026-10-22: "clone the main account"). One `setup` choice, shown only
  for a new account, `like default` preselected:
  - `like default`: the links below. One setup for every account.
  - `copy of default`: a one-time copy of the same entries, so the
    account can diverge. Sign-in files and the agent's own state are
    never copied; user-scope MCP servers stay out, as with links.
  - `empty`: nothing shared; the agent's own first run.
  The confirm line names the choice: `new claude account "work2", like
  default: sign in in the session`.

### The author's (open to change on review)

- **Where accounts live:** `~/.matchblox/accounts/<agent>/<name>/`, made
  with mode 0700. Not under `~/.config`, which many people keep in a
  public dotfiles repository; these folders hold sign-ins.
- **An account name** is 1–10 characters from `[a-z0-9-]`. The default
  account is called `default` and is not a folder. For Codex, matchblox
  also checks that `<folder>/app-server-control/app-server-control.sock`
  (43 bytes after the folder) is at most 103 bytes on macOS and 107 on
  Linux, and refuses the name with the reason if it is not. With a
  user name of 13 characters or fewer, every 10-character name fits.
- **The account shares the default's setup through links.** On each
  start, matchblox links every entry of the default folder into the
  account folder, except the entries that hold the sign-in or the
  agent's own state. It adds links for new entries and never replaces
  an entry that is not a link. There is no list of files to share, so
  a setup the author never saw works too.
  - Claude: account folder = `CLAUDE_CONFIG_DIR`. Shared: every entry
    of `~/.claude`. Own: `.claude.json` (it holds `oauthAccount`),
    `.credentials.json`, `backups/`, `projects/`, `sessions/`, `history.jsonl`. The
    sign-in itself is in the keychain under the folder's hash. The
    status line and hooks come with the shared `settings.json`. The
    default's `.claude.json` is `~/.claude.json`, in `HOME`; it also
    holds user-scope MCP servers. matchblox does not copy them (they
    can hold tokens): an account has the plugin and project MCP servers
    only, and the docs say so. Project `.mcp.json` files work as usual.
  - Codex: account folder = `CODEX_HOME`. Shared: every entry of
    `~/.codex`, `config.toml` included, except own ones. Own:
    `auth.json`, `sessions/`, `archived_sessions/`, `history.jsonl`,
    `*.sqlite*`, `app-server-control/`, `packages/`, `.tmp/`. The start
    adds `-c cli_auth_credentials_store="file"` to the argv, so the
    account always has an `auth.json` and a name, and the shared
    `config.toml` is never copied or edited.
  - Cursor: `<folder>/home` = `HOME`, `<folder>/cfg` =
    `CURSOR_CONFIG_DIR`, with `AGENT_CLI_CREDENTIAL_STORE=file`.
    `<folder>/home` links every top-level entry of the real `HOME`
    except `.cursor`. `<folder>/home/.cursor` is a real folder that
    links every entry of `~/.cursor` (so `hooks.json`, `mcp.json`,
    rules and skills are shared, and matchblox's Cursor hooks fire)
    except `auth.json`, `cli-config.json`, `chats/`, `projects/`. `Library`
    is a link, so the macOS keychain and git's `osxkeychain` helper work
    as before. `<folder>/cfg/cli-config.json` is made once with only the
    default's `statusLine` (and `permissions`), so matchblox's status
    line works from the first screen; Cursor adds `authInfo` to it at
    sign-in. Whether Cursor keeps the fields it did not write is spike 1.
  - The same linking rule with an empty default (a builder with no
    dotfiles, no skills) gives an empty account, which works.
- **The known accounts** for an agent are `default`, plus each folder
  under `~/.matchblox/accounts/<agent>/`, plus the config folder of any
  live session of that agent (an account made by hand, such as
  `~/.claude-work`, shows with its folder as its name).
- **The env goes on the command, not on the tmux session.** The start
  step runs `env VAR=… <argv>` through `new-session`. tmux's session
  environment stays clean, so a split pane or the builder's own shell
  in that session does not get the account's `HOME`.
  - Claude: `CLAUDE_CONFIG_DIR=<folder>` (exact string, no trailing
    slash), plus `BROWSER=/usr/bin/true` on a new account.
  - Codex: `CODEX_HOME=<folder>`.
  - Cursor: `AGENT_CLI_CREDENTIAL_STORE=file`, `HOME=<folder>/home`,
    `CURSOR_CONFIG_DIR=<folder>/cfg`.
- **Switch agent (A) starts the new agent on `default`.** `respawn-pane`
  runs the new argv with no account env, because the env is on the
  command, not the session. The confirm line says "on default". An
  account for A can follow later. (Revised while building: A starts on
  the agent's last account.)
- **New account, step by step:**
  1. Make the folder and its links (above).
  2. Start the agent there. No first message: the session has no
     sign-in yet.
  3. Wait until the pane shows the agent's first screen (the text in
     Facts), then send its keys. Give up after 20 s and say so; the
     builder can still sign in by hand.
  4. The row says `signing in` until the sampler reads an account name
     in the folder.
- **What names an account.** A session gets a new field, `AccountDir`
  (the account folder; empty for `default`), next to `Account` (the
  sign-in label, as today). USAGE blocks are keyed by provider and
  `AccountDir` when `AccountDir` is set, else by provider and `Account`
  as today. So:
  - `default` keeps today's name (plan or email). Goal 5 holds.
  - An account folder's block is named by the account's name.
  - Two new accounts with no sign-in yet never merge: their folders
    differ.
  - `limits.Account` gets `Dir` too. Both fields are `omitempty`: an old
    client, `matchblox usage` and the phone read the JSON as before.
- **USAGE per account:**
  - Claude: the readings are keyed by the folder today; with
    `AccountDir` the block is named by the account.
  - Codex: works today with `auth.json`. If the daemon turns out to
    hold the rollout (spike 3), matchblox reads the newest file under
    `$CODEX_HOME/sessions` started after the pane.
  - Cursor: `cursor_usage` today polls one keychain token and gives
    that reading to every Cursor session. This must change in the same
    release as Cursor accounts, or an account session shows the
    default's month: the poll becomes one per Cursor account. `default`
    reads the keychain as today; an account folder reads `accessToken`
    from `<folder>/home/.cursor/auth.json` when a call is due and drops
    it after. Same 5-minute interval per account.
- **The sampler and a foreign `HOME`:** `envOf` today reads nothing for
  a process whose `HOME` differs from the builder's, unless its env names
  a folder. A Cursor account process names `CURSOR_CONFIG_DIR`, and its
  `HOME` lies under `~/.matchblox/accounts/cursor-agent/`; the sampler
  trusts that `HOME` for Cursor's `auth.json` and nothing else.
- **Platforms:** macOS first; that is where every fact was probed.
  - On Linux, Claude keeps the sign-in in `<folder>/.credentials.json`
    and Codex in `auth.json`: both work with the same folders.
  - Cursor on Linux already uses a file store, at
    `$XDG_CONFIG_HOME/cursor/auth.json`. There an account sets
    `XDG_CONFIG_HOME=<folder>/xdg` instead of `HOME`, and needs no
    mirror. Read from Cursor's code, not tested.
  - Cursor usage stays behind today's macOS gate on `default`; an
    account folder's usage reads a file, so it runs on both.
- **`proto.NewSession` gets `Account string`,** optional (`""` and
  `default` mean the default), and `NewAccount bool`. An old client
  that sends neither starts on `default`.
- **Remote, phone and iOS:** they show the account name on rows and in
  USAGE through the new fields. The account picker is in the console
  and the CLI only.
- **The stoker** keeps starting sessions on `default` only.

## What an account sees

An account is the default setup with another sign-in.

| The builder's setup | Claude account | Codex account | Cursor account |
| --- | --- | --- | --- |
| Skills, plugins, `settings.json` (hooks, status line), `CLAUDE.md`, `agents/`, `commands/` | shared (links) | — | `~/.claude/skills` through the linked `HOME` |
| `AGENTS.md`, prompts, `config.toml`, skills | — | shared (links) | — |
| `~/.cursor` hooks, rules, skills, `mcp.json` | — | — | shared (links) |
| Dotfiles: git, ssh, gh, shell | real `HOME` | real `HOME` | linked |
| Own | sign-in, `.claude.json`, history, projects | sign-in, sessions, history, state | sign-in, `cli-config.json`, chats |

Because the shared entries are links, a skill or plugin installed from
any account lands in the default folder and reaches every account. A
setup tool that writes to `${CLAUDE_CONFIG_DIR:-~/.claude}/skills`
writes through the link. A real file that replaces a link (a tool that
rewrites `settings.json` in place) makes the account drift; the start
step never replaces it, and a check for drift belongs to the setup
tool (see below).

Follow-up outside matchblox: a setup tool such as builder can check
each account under `~/.matchblox/accounts/` for drift, suggest accounts
on N to a builder who runs many sessions, and offer to move user-scope
MCP servers into a plugin or a project `.mcp.json` so every account has
them.

## UX sketch

```
New session
  where    ~/src/app
  branch
  name     app-fix
  agent    claude
  account  ‹ work ›        default · personal · work · + new account
  first    fix the flaky test
```

`+ new account` turns the field into a name box, and a `setup` row
appears under it: `‹ like default ›  copy of default · empty`, with one
grey line that says what the choice does. Confirm says:
`start claude in ~/src/app on a new account "work2": sign in in the
session`. `first` is cleared and greyed for a new account.

USAGE today has one block per account, named by plan or by email (`accountNames`). With accounts, a block from an account folder is named by that account:

```
 USAGE
 ✻ personal  5h ▓▓▓░░░░ 41%  ↻ 2h10   week ▓░░░ 18%
 ✻ work      5h ▓▓▓▓▓▓░ 88%  ↻ 0h40   week ▓▓▓░ 61%
 ❋ work      5h ▓░░░░░░ 12%  ↻ 4h05   week ░░░░  9%
 ⬢ personal  month ▓▓░░░░ 33%
```

## Left to the builder

- The exact first-screen strings matched before sending keys, and how
  they live with agent updates (a table per agent version, or a looser
  match).
- Whether Codex sessions on an account start with `--no-daemon`, after
  checking who holds the rollout.
- The CLI shape for a new account (`matchblox new --account work2 --new-account`
  or `--account +work2`).
- Docs: `docs/agents.md`, `docs/limits.md`, `docs/keys.md`,
  `config.example.toml`, CHANGELOG.

## Revised while building

- Spike results (part 1, no sign-in): a mirrored `HOME` keeps the shell rc, the git identity, signed commits (Secretive), gh, ssh and the keychain. An empty `HOME` fails cleanly. Cursor keeps the seeded `statusLine` and permissions. A linked Claude folder shows the same plugins and skills.
- Spike 3 (Codex 0.163): the per-home app-server daemon holds the rollout, not the pane's codex. `--no-daemon` would move it back. matchblox does not change how Codex starts. It finds the rollout by cwd and start time (Task 4b).
- `backups` is a Claude own entry: a linked `backups` would let Claude restore the default's `.claude.json` into an account.
- A new-account start is a destructive action (typed `y`). An existing account starts on Enter.
- "sign in by hand" is a per-pane note (`state.Doc.SignIns`), shown on the row.
- Hand-made folders are listed, and start only while a live session of that agent uses them.
- Not yet tested with real second sign-ins: spikes 2 and 5, and the signed-in halves of spikes 1 and 4.
- Cursor accounts run on macOS only for now. On Linux the mirrored `HOME` links `~/.config`, where Cursor keeps the default sign-in, so an account would write through it. The service refuses a Cursor account off macOS ("Cursor accounts run on macOS only for now"), and `state.Doc.Accounts` has no Cursor entry there, so `N` shows no account field.
- Each account folder has `.matchblox-setup` (one line: like-default, copy or empty; mode 0600), written when the folder is made. Each start of a like-default account runs Prepare again, which links new default entries and never replaces one. Copy and empty accounts are not prepared again. A folder with no marker (made before it) counts as like-default. The marker is an own entry. A new account that fails to start (Prepare or the tmux step) has its folder removed, when that start made it. A lock per account folder spans the check, Prepare and the start (Task 12): of two confirms of one new account, the second is refused as there already and never removes the first one's folder. A failed Prepare of an existing like-default account does not stop its start: the result's skip says "some shared entries did not link" with the first error.
- Codex rollouts from the daemon are assigned greedily within a sample: the live sessions of one `CODEX_HOME` and cwd that hold no rollout, the latest-started first, each take the newest rollout that no session holds or has taken and that began after them. Sessions get different logs, matched by start order; one with none left is unmeasured. The match is right when each session's first turn comes in start order; other orders are not tested.
- Own entries added after the final review, checked against the real folders of Claude Code 2.1.308, codex-cli 0.163.0 and Cursor 2026.10.19: Claude `remote-settings.json*`, `policy-limits.json*`, `stats-cache.json`, `state`; Codex `app-server-daemon`, `session_index.jsonl`; Cursor `agent-cli-state.json`, `statsig-cache.json`, `ai-tracking`, `managed`; and `.matchblox-setup` for all three. The Cursor `HOME` mirror skips `.matchblox`. Cursor's seeded `cli-config.json` carries `permissions` as well as `statusLine`.
- Last account and last agent (Task 10 and the addendum). The service keeps them in `last-accounts.json` in the state directory (mode 0600, written atomically): `{"accounts": {<agent>: <label>}, "last_agent": <agent>}`. A file of the older form, the bare map of labels, still loads. Only a start that names its account (`Account` not empty) changes that agent's account. The console names it always for an agent with accounts, `default` too. `matchblox new` without `--account` stays on default and changes no account. Each start that ran in full changes the last agent, the CLI's too; the stoker's starts change nothing. `state.Doc.LastAccount` sends only accounts still listed in `Accounts`, and `state.Doc.LastAgent` only an agent in `Agents`. `N` opens on this console's last agent, else `Doc.LastAgent`, else the first agent, and then on that agent's last account. The stoker starts on default only: the service refuses a stoker start that names another account or a new one ("the stoker starts on default only").
- Switch agent (A) on the last account (owner decision, addendum). A starts the new agent on the account that agent was last started on, through the same check as a start: an account folder that is there, or a hand-made folder a live session runs on. Anything else, and a Cursor account off macOS, is `default`. A switch never makes an account and does not change the remembered account. The env goes on the `respawn-pane` command (`env K=V… argv`), with the agent's account argv (Codex: `-c cli_auth_credentials_store="file"`). A like-default account is prepared again under the account's lock; when that fails, the switch runs all the same and the result's skip says "some shared entries did not link". The confirm line names the account: `switch <who>: claude → codex in %12, /work/app, on work, then jump there` (`on default` without one; no account for an agent without accounts). Not tested yet with a second real sign-in.
- Codex `installation_id` is own (Task 12): the accounts do not share one install id. What Codex does with an account that has none is not tested yet.

## Spikes before the plan

1. **Cursor home mirror:** in a mirrored home, Cursor keeps the
   `statusLine` that matchblox wrote into `cfg/cli-config.json` after
   sign-in, matchblox's Cursor hooks fire, and a signed-in session runs `git status`, `git commit -S`, `gh auth status` and an
   ssh push, and the builder's shell loads as usual. Repeat with an
   empty real `HOME` (a builder with no dotfiles).
2. **Two sign-ins at once:** one real second account per agent, signed
   in through the flow above while a default session runs; the default
   keychain entries keep their modify dates.
3. **Codex rollout holder:** after one turn, which process has the
   rollout open.
4. **Shared Claude and Codex setup:** an account folder with the links
   loads the default's skills, plugins and MCP servers, and the
   default session keeps working while the account runs.
5. **Cursor usage on a file account:** the usage call with the token from
   that account's `auth.json`.
