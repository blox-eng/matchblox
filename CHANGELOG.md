# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.26.1] - 2026-10-28

### Fixed

- **A swipe scrolls in a host jump again, and a drag selects there.** Each
  jump's session has its own key table since v0.25.0, and tmux then skips
  its default mouse keys: on a phone a swipe in a jump scrolled nothing.
  Each jump's table now holds tmux's own mouse keys for a pane (wheel,
  click, drag, border drag), and the host's side takes the mouse, so a
  swipe scrolls the agent's history there. (#240)
- **A connect from Hosts asks ssh's questions in the console.** It gave
  the raw terminal to `matchblox connect`, so ssh asked for a host key or a
  password as plain text. Now a host key, a password or a passphrase shows
  in the same field as a jump's, and `esc` ends the ssh. A login by key,
  agent or Tailscale SSH asks nothing. A failure stays on Hosts and says
  why. `matchblox connect` typed in a shell still asks in that terminal.
  Not tested yet with a real ssh on macOS. (#242)

## [0.26.0] - 2026-10-27

### Changed

- **A session's row counts every pull request it drives, not only the
  first.** The row leads with the ship that most needs you (stuck, then
  waiting for you, in flight, merged, live) and adds a count of the others:
  `+1 stuck` or `+1 waits` when one of them needs you, else the furthest
  stage among them (`+1 merged`), and `+1` when the row is narrow. The
  count folds away before the track or the number. The detail, and the
  opened row on a phone, list the first three ships of the session, each
  with its track, then `+N more · p ships`. A list where each session has
  one ship looks as it did. (#235)
- **Every reader names the same ship for a session.** The idle reason on
  a row, `matchblox sessions --json` and the stoker's brief name the ship
  the row leads with, not the first in the tracker's report, so a row no
  longer shows one PR's track beside another PR's word. (#235)
- **A ship belongs to a session by its session id when both have one.**
  An agent started in a pane where another ran no longer takes that
  agent's ships; while a session's id is not read yet, its pane still
  pairs it. A Claude Code `/clear` starts a new session id, so the ships a
  session had before a `/clear` leave its row, its idle reason, `--json`
  and the stoker's brief. The Ships tab still lists them, but says their
  agent no longer runs: Enter and `a` do not reach it. Not tested yet with
  a live `/clear`. The stoker's merge guard still pairs by pane, as before,
  so it still lets that agent merge those PRs. (#235)

## [0.25.1] - 2026-10-27

### Added

- **The console asks the login of a host once, in the console.** One ssh
  connection per host is shared for 10 minutes, in
  `$XDG_RUNTIME_DIR/matchblox/ssh` or `/tmp/matchblox-<uid>/ssh`. It takes
  no forwarding from `~/.ssh/config`, and ends 15 seconds after the
  network goes. A password, a passphrase or a new host key
  shows in a masked field in the console, never in the raw terminal. A host
  key needs a typed `yes`.
  ([#214](https://github.com/blox-eng/matchblox/issues/214))
- **The console offers your key for a host that asks a password.** It takes
  the most secure key first: a security key in your agent, then a key in
  your agent or `~/.ssh`, then a new key with a passphrase. A typed `y`
  installs it, and the install only appends to `authorized_keys`. On macOS
  the console also offers the Keychain lines for `~/.ssh/config`, which
  every OpenSSH reads, not only Apple's.
  ([#214](https://github.com/blox-eng/matchblox/issues/214))
- **A jump works on a Tailscale SSH host.** The console shows the link of
  Tailscale's check (`c` copies it), says why a tailnet policy refuses you,
  and warns on the Hosts row that the gate does not hold there.
  ([#214](https://github.com/blox-eng/matchblox/issues/214))

### Fixed

- **A jump from a console to a host no longer opens matchblox inside
  matchblox.** The jump opens in its own tmux session, with one status line.
  `◂ console` (tmux 3.4 or newer) or the home key goes back and ends the
  jump, and so does prefix `m`: a jump that you leave ends, and nothing
  runs on unseen. A failed ssh waits for Enter, then goes back to the
  console.
  ([#214](https://github.com/blox-eng/matchblox/issues/214))

## [0.25.0] - 2026-10-27

### Added

- **A match knows its workspace and how to find its peers.** New and
  switched sessions receive a private environment brief and a task record
  that survives compaction. Repository instruction files stay unchanged.
- **Switches preserve handover evidence before replacing the agent.** The
  capture contains the outgoing task record, recent conversation text when
  available, the last reply and checked Git state. Missing context stays
  unknown. The console previews the evidence before confirmation;
  `matchblox switch api --agent codex --preview` reads it without switching.

### Fixed

- An agent on a tmux server that matchblox does not list no longer takes
  the row of a listed pane with the same pane id. It shows outside tmux.
- **Codex starts with workspace trust and workspace-write network access**
  through both `new` and `switch`. The settings apply to that invocation.
  Sandbox mode and command approval policy stay as configured. A folder
  just cloned still asks, as Cursor does. An explicit untrusted project or
  an unreadable account configuration stops the launch.

## [0.24.0] - 2026-10-26

### Added

- **N's `where` is a list of your repositories.** While `where` has focus,
  the list fills the form box and the other lines fold to one faint line.
  Each repo shows its worktrees under it, with the path, the branch, the age
  and `N live`: the sessions in the repo and in its worktrees together, and
  no match glyph on a repo row. SUGGESTED comes first (a repo with a live
  session, then where matchblox last started one, then the last activity),
  then ALL by name. Typing filters; a path that starts with `/`, `~` or `.`
  matches whole folders only, and a path that is not a known folder is the
  first row. matchblox records where its sessions started in `used.json` next
  to its state. It needs no scan consent, and `matchblox scan off` keeps it.
  An empty list says `no repos known yet · type a path, or paste a git URL`,
  and `matchblox scan finds your repos` when the scan was never answered.
  ([#174](https://github.com/blox-eng/matchblox/issues/174))
- **N clones a git URL.** Paste `https://…`, `ssh://…`, `git@host:o/r` or
  `gh:o/r` in `where` and the list shows `start in <path> (already a clone
  of o/r)`, `clone o/r into <folder>` and `clone anew into <folder>`. A bare
  `o/r` filters your repos first and shows the clone rows after. `⏎` on a
  clone row starts the clone in the service and goes to branch; `where`
  shows `cloning o/r… 42%`, the last `⏎` waits (`starts when the clone is
  done`) and `esc` stops the clone and deletes the partial folder. A clone
  goes through a hidden `.<name>.cloning` folder and takes its name when it
  is done. Beside it, `.<name>.cloning.owner` holds the pid of the service
  that clones, until the clone ends. A leftover folder is cleared only when
  that file names a process that is gone; with no file, or a live owner, it
  is left alone and its name counts as taken. A taken folder name gets a match word (`ember`, `flint`, `spark`
  and more), then a 4-character tail. `[git] clone_root` (default `~/src`)
  is where clones go; `o/r` uses the protocol of `gh config get
  git_protocol`, else https. `matchblox new --where <url>` clones (or
  reuses a clone), then starts, and waits up to 30 minutes: it takes a URL
  or `gh:o/r`, and a bare `o/r` there is a folder, so a typo never clones.
  A start that waits for its clone keeps the account you picked, and a new
  account still asks for its typed `y` once the clone is done.
  ([#176](https://github.com/blox-eng/matchblox/issues/176))

## [0.23.2] - 2026-10-26

### Fixed

- **A service that waited for the socket restarts for an install made
  meanwhile.** A service started by the user unit that waited while you
  installed a new matchblox used to take the new binary for its own once it
  took over, and run the old code on, with no console to replace it.
  ([#212](https://github.com/blox-eng/matchblox/issues/212))

## [0.23.1] - 2026-10-26

### Fixed

- **A service whose binary was removed before it watched it stops.** It
  stops as uninstalled, instead of running on. The serve tests wait for the
  watch's first look rather than a fixed time, so they no longer fail under
  load.
  ([#212](https://github.com/blox-eng/matchblox/issues/212))

## [0.23.0] - 2026-10-24

### Added

- **Sessions shows which session started which.** A `codex exec` or
  `claude -p` that an agent starts shows as a branch under that session,
  instead of hidden or a stray row. The link is proven from /proc, never
  guessed. ([#206](https://github.com/blox-eng/matchblox/issues/206))

## [0.22.0] - 2026-10-24

### Added

- **A new session starts on the account you pick, or on a new one.** `N`
  picks the account. Claude Code and Codex sessions can run on an account
  folder beside your default sign-in, and Cursor sessions on macOS only for
  now. A new account is set up like the default, copied from it, or empty.
  matchblox presses through the agent's first screens to its sign-in link,
  and you finish the sign-in in the session. matchblox never writes the
  default sign-in. That the agents also leave the default sign-in as it was
  is not tested yet with a second real sign-in.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **`N` opens on the agent and the account you last started.** matchblox
  remembers the last agent, and the last account of each agent, across
  restarts. `matchblox new` without `--account` does not change the account
  `N` opens on. A console that has started something keeps its own last
  agent. The agent the service remembers opens only a console that has
  started nothing yet, for example after a restart. A new account becomes
  the account `N` opens on only once a session on it is signed in.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **Switch agent (`A`) starts the new agent on its last account.** This is
  the account you last started that agent on, while it is still there;
  else `default`. The confirm line names it. A switch never makes an
  account, and it does not change the account `N` opens on. A switch on a
  second real sign-in is not tested yet.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **`matchblox new` takes `--account <name>`, `--new-account` and
  `--setup like-default|copy|empty`.** `--setup` needs `--new-account`, and
  `--new-account` needs `--account`.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **A start on a new account asks for a typed `y`.** The row says
  `signing in` until the account is signed in, and `sign in by hand` when
  matchblox waits 20 seconds on a screen without a next screen it knows.
  An ask or a wait shows over both words. matchblox never reads a sign-in
  code and never types a password.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- A start or a switch on an account drops the agent's sign-in environment
  that the tmux server can pass on (such as `ANTHROPIC_API_KEY`,
  `OPENAI_API_KEY` or `CURSOR_API_KEY`), so the account's own sign-in is
  used. A start on `default` keeps it.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- The stoker starts sessions on `default` only.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- Cursor accounts read their plan usage from the account's `auth.json`, with
  `cursor_usage = true` (not tested yet with a second real sign-in).
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- USAGE and Sessions name each account, and a new account shows `signing in`
  until it is. A folder that you made by hand never shows `signing in`.
  `matchblox usage` names each account folder, and `matchblox sessions
  --json` gives `account_dir` and `signing_in`.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))

### Fixed

- A service whose socket file was removed before it began to watch it
  stops all the same, so the next console starts a new one; it used to
  run on, unreachable.
- **A copy of the default is cloned, and its own.** Files are cloned
  copy-on-write where the file system can, and copied byte by byte where
  it refuses. A link in the default that points inside the default points
  to the same place in the account, so the account reads its own files and
  never the default's. A link that points outside the default, or at an
  entry the account does not share, stays on the default's file and is
  named. The copy stops at 200,000 entries, 16 GiB copied byte by byte,
  or 90 seconds, and names every entry it left out. A cut copy is marked,
  and goes on where it stopped at the account's next start. Measured on
  macOS (APFS) with a real 2.0 GB `~/.claude` of 117 links: a copy took
  4.6 to 4.8 seconds and used no extra disk, all links into the default
  were kept inside the account, and a resume over a finished copy took
  about 0.16 seconds.
  Cloning on Linux is compiled and linted only: not tested yet.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **A start or a switch on an account says what it shared, skipped and
  refused.** It names each link still shared with the default and each
  entry not copied (five of each, then "and N more"), and says when a copy
  is cut and goes on at the next start. `N`, Switch agent and `matchblox
  new` show these notes after the start, and a new account names the
  session to sign in in (`sign in in <session>`). A start or a switch on an
  account that another start or switch is busy with is refused at once as
  busy, and one whose folder went away meanwhile is refused as gone. `matchblox new`
  waits the copy's 90 seconds on top of its 30 for a new or copy account,
  and so does `matchblox switch` when the agent's last account is a copy.
  Both say "failed" for a start that ran and failed and "refused" only
  when matchblox refused it, and `matchblox new` says "stopped after" the
  step that ran when a guard stopped the rest. `matchblox sessions` says
  "sign in by hand" on the line of a new account that matchblox gave up
  signing in.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **An agent's pane cannot start a session on an account, new or not, even
  when it says it is a console.**
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **The sign-in of a new account presses only what it sees.** matchblox
  sends a key only when the option it picks is the highlighted one, looks
  at the screen again right before the key, and sends it only to the agent
  first seen in the pane. It stops for good, and the row says `sign in by
  hand`, when you type in the pane or when the screen changes in any other
  way: a reflow when a client of another size attaches counts. It also
  says `sign in by hand` after 20 seconds on a screen without a next screen
  it knows. A key you type in the few milliseconds between its last look
  and its key can still be answered: tmux cannot close that gap. An agent
  that redraws slowly after a key is waited for. The screens, highlight
  marks and option labels were checked live on codex-cli 0.163.0 and
  Claude Code 2.1.308 up to the login choice; Cursor's screen and the
  sign-in after the choice are not tested yet.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **A switch runs on the account its confirm named, or is refused.** `A`
  and `matchblox switch` send the account they would start the agent on.
  If the agent's last account changed meanwhile, the switch is refused
  with `the last <agent> account is now <x>: switch again`, and nothing
  moves. A hand-made folder named two ways shows, and runs, under the
  spelling the list shows; a last account a start would refuse shows, and
  runs, as `default`.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **`N` names each account folder it does not offer, and why.** A folder
  under `~/.matchblox/accounts/<agent>/` whose name is not a-z, 0-9 and -,
  up to 10, was left out of the account list without a word. The form now
  names them under the account field, `not listed: Team (not a-z, 0-9 and
  -, up to 10)` (three named, then "and N more"), and an unreadable
  accounts folder says `accounts not read:` with its error.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **The default config folder is the default, however it is reached.** A
  `~/.claude` that is a symlink, with `CLAUDE_CONFIG_DIR` set to its real
  path, or `~/.Claude` on macOS, showed as a hand-made account. Both sides
  of the comparison now name a folder one way, links resolved and letter
  case as on disk. The account folders and a session's folder are read
  once, not at every sample, and a folder that failed to read is read
  again. A link retargeted while a process lives keeps that process's
  first reading.
- **A new account shares only the setup the list names.** Settings,
  instructions, commands, skills, plugins, hooks and a few caches are
  shared. Every other entry of the default stays with it, including one a
  later agent version adds: Claude Code's daemon and its key, token
  backups and job logs are never linked or copied.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- **A Codex account and the default share a directory.** matchblox counts
  Codex sessions per directory within each Codex home, so a session on an
  account and one on the default in the same directory each show their
  context and limits. Two sessions on one home in one directory still show
  "not measured".
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- An agent that the Codex app-server runs (such as `codex exec`) is not
  a session, and a `node` launch of `codex app-server` is no session either.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))
- A Cursor login that you just signed in to reads its plan usage within 30
  seconds, not after up to 5 minutes. Only a missing login is read again
  every 30 seconds. A token read that is denied or fails otherwise (the
  Keychain or `auth.json`) waits the normal interval, and USAGE says
  "Cursor's login token could not be read". USAGE says that Cursor's API
  has not answered yet only while the token is read or after matchblox
  asked the API.
  ([#170](https://github.com/blox-eng/matchblox/issues/170))

## [0.21.0] - 2026-10-23

### Added

- **The phone buzzes when an agent waits, with the app closed.** A new
  wait buzzes the phone through a small relay that stores nothing. The
  buzz text is sealed to the phone. Run `matchblox serve` with the door
  "Keep matchblox running" and no console needs to be open. Set
  `[push] off = true` to stop it.
- **The island shows the session that matters most, or your usage.** The
  Live Activity has two faces, Sessions and Usage, and the app's `Island`
  screen sets which one, which host it follows and which hosts buzz. A wait
  breaks through the Usage face for 60 s.
- **A tap opens that session.** The activity, its `Answer` and a buzz open
  the console on that session after Face ID; with no session on top, the
  host's console. The app needs iOS 18.

### Changed

- **The app needs iOS 18.** It needed 17.2. The island's channel needs
  iOS 18.
- **A phone paired before this release gets an id.** At start the service
  gives each old phone line in `~/.ssh/authorized_keys` an id, with a
  backup, so the phone can register for the buzz without pairing again.
  A line whose matchblox is older than v0.13 stays as it is: that gate
  would refuse the id. `serve.log` says how many lines it changed and how
  many it left.
- **`matchblox devices` shows the push of each phone.** A phone's row says
  `buzz on` or `buzz off` (with why: `[push] off`, or no host key), and
  `island yes` or `island no`. The service's last push failure shows under
  the list.
- **`[push] relay` names the relay.** It is `https://matchblox.sh`.

## [0.20.0] - 2026-10-23

### Added

- **`serve`, `status`, and `setup` warn when Tailscale SSH bypasses the
  gate.** With Tailscale SSH on, the host's MagicDNS name and Tailscale
  IP on port 22 are answered by tailscaled, so permanent paired keys'
  `restrict` and ForcedCommand never run on that path. The warning names
  `tailscale set --ssh=false` (or an OpenSSH address/port) as the fix;
  the docs cover the same. Pair still refuses before the QR when that
  path would answer the code ([#184](https://github.com/blox-eng/matchblox/pull/184)).
  ([#185](https://github.com/blox-eng/matchblox/issues/185))

### Fixed

- **`z` parks the right Codex session.** A `codex exec` run in a pane took
  the rollout of a Codex session that started after it in the same
  directory, with its id, so `z` on the run parked the session. Before its
  first turn, a Codex session took the rollout of a Codex app or editor
  thread there, of a subagent, or of a Codex session that started there
  after it and exited, also after matchblox restarts, or while it was
  down. A Codex started with
  `-C <dir>` counts in `<dir>`, and is measured from it. Now only the
  CLI's own sessions count, and a `codex exec` or `codex review` row takes
  none, with options before it or not.
- **A resumed Codex session shows its context.** `codex resume <id>` finds
  its rollout by the id, on whatever day it began, also from an npm install
  or with options such as `-m`. After you edit an earlier prompt (Esc Esc),
  Codex writes on in a new rollout, and matchblox follows it. After a
  `/new` or `/resume` in a Codex session (of a session from its
  directory, however old), or while another session in its directory (or
  the one `-C` names) is written after it, it shows "not measured" at the
  next sample. A resume that is not found shows "not measured", not
  "fresh", and so does a session with no rollout of its own yet, in a
  directory where an older Codex CLI rollout has been written since it
  started.
- **`z` says why a Codex session has no id.** One whose rollout cannot be
  told apart says so, not "park it after its first turn".

## [0.19.0] - 2026-10-23

### Added

- **N offers the repositories on your host, found once with your yes.**
  The first console asks `Find the Git repositories in your home folder?`
  (`y` / `n` / `p pick folders`) and never asks again. After a yes the
  service looks four folders deep in `~`, `~/dev`, `~/src`, `~/code`,
  `~/projects`, `~/work`, `[git] repos` and the new `[git] scan_roots`,
  reads each checkout's path, remote, branch and last use without running
  git, keeps the list on disk and scans again every 30 minutes. The list
  feeds N's `where`, not the Git tab, and goes nowhere but this host and
  its paired devices. `matchblox scan` scans now, in the folders given;
  `matchblox scan off` stops and forgets.
  The scan never waits on a folder that may not answer. On macOS it skips
  the folders macOS guards (`~/Documents`, `~/Desktop`, `~/Downloads`,
  `~/Library`, `~/Pictures`, `~/Movies`, `~/Music`) when the walk reaches
  them from above, so macOS never asks for them; a folder you name with
  `p` or `matchblox scan <folder>` is walked. Network folders below a
  root (NFS, SMB, sshfs, rclone and other FUSE remotes) are skipped by
  the mount table without being touched, while local mounts are walked. A
  folder that does not answer keeps its repos from the last scan and is
  not waited on again for 6 hours, unless you name it. Every folder
  skipped is named with its reason, in the console and in `matchblox scan`.
  Repos show in one form, symlinks resolved and letter case as on disk;
  a remote's password or token never shows. The console's question takes
  no key for a moment after it shows, never sits over a confirm, and says
  how the scan went: none found, stopped early only when a bound cut it.
  In `p`, a folder in double quotes keeps its spaces, and the folders
  that are not there are named and replace nothing; `matchblox scan` waits
  as long as a scan can take and says "refused" only for a refused yes.
  ([#173](https://github.com/blox-eng/matchblox/issues/173))

## [0.18.0] - 2026-10-22

### Added

- **USAGE shows each provider's logo in kitty and Ghostty**, through tmux
  too, while every terminal on the console's session can draw it; a phone
  or another terminal attached there brings back the marks. matchblox
  lets images pass through its own pane only. The logos are lobe-icons'
  (MIT). ([#183](https://github.com/blox-eng/matchblox/issues/183))

### Changed

- **USAGE names the provider of each account.** Two plans called "Team"
  read as `✻ Claude Team` and `❋ OpenAI Team`; Cursor, OpenCode and any
  other provider are named too. On a narrow dock the meter shrinks so
  the reset times stay whole.
  ([#183](https://github.com/blox-eng/matchblox/issues/183))

### Fixed

- **A known agent installed after the first run shows in Sessions.**
  The first run froze the agents it found into `agents`, so Codex
  installed later (or a session switched from Claude Code to Codex)
  never showed. Every known agent (claude, codex, opencode, gemini,
  cursor-agent, aider, crush, qwen) now counts wherever it is installed;
  `agents` adds others. The Codex app's `codex app-server` daemons are
  no session.
  ([#179](https://github.com/blox-eng/matchblox/issues/179))
- **Codex shows its context and limits.** macOS lists no open files,
  and Codex 0.163 closes its rollout after each write, so no open file
  named it and the session read as unmeasured with no limits. The rollout is now also found as the newest one
  of the session's directory begun and written since it started, when it
  is the only Codex there and the rollout is not a `codex exec` run's.
  ([#179](https://github.com/blox-eng/matchblox/issues/179))
- **`z` parks a Codex session.** Codex has no hooks, so its session had
  no id and could not be held; the id now comes from its rollout, and its
  pane still tells busy from idle.
  ([#182](https://github.com/blox-eng/matchblox/issues/182))

## [0.17.4] - 2026-10-22

### Fixed

- **`matchblox pair` stops up front when sshd cannot use the one-time
  key.** With Tailscale SSH on the address of the code (MagicDNS name,
  short name, or Tailscale IP on port 22), or when the host can see that
  `AuthorizedKeysFile` is not `~/.ssh/authorized_keys` (`sshd -T -C`, or
  the global line in `sshd_config`), pair showed a code that could never
  work and the phone waited five minutes. The host now refuses before the
  code, in the terminal and in the console, and names the fix. The
  console points at `tailscale set --ssh=false` for Enter; `--address`
  stays a terminal command. `--address` with an address that OpenSSH
  serves still pairs.
  ([#102](https://github.com/blox-eng/matchblox/issues/102),
  [#120](https://github.com/blox-eng/matchblox/issues/120))
- **The app tells a refused connection from a silent host.** A host that
  exits with status 126 or 127 and no reply used to read as "did not
  answer". The app now offers another address (Try again) and names
  Tailscale SSH as the host-side fix.
  ([#102](https://github.com/blox-eng/matchblox/issues/102),
  [#120](https://github.com/blox-eng/matchblox/issues/120))

## [0.17.3] - 2026-10-22

### Fixed

- **An agent started after `matchblox serve` shows in Sessions in a
  container too.** A new tmux session (or `matchblox new`) after the
  first process scan used to wait for the slow process clock (~10s), so
  a check at ~6s looked empty (`sessions: 0`, one idle pane). Listing
  panes every sample already wakes a full scan for a new pane or an
  agent command; regression tests pin the late-start case.
  ([#113](https://github.com/blox-eng/matchblox/issues/113))

## [0.17.2] - 2026-10-22

### Fixed

- **Sessions shows agents when the host has no UTF-8 locale.** With
  `LANG` unset, tmux printed the sampler's tab-separated pane list with
  `_` for each tab, so no panes were parsed and Sessions was empty. The
  sampler now runs those tmux calls with `-u` (list and capture), so
  tabs stay intact.
  ([#112](https://github.com/blox-eng/matchblox/issues/112))

### Removed

- **AUR (`matchblox-bin`) publishing and install docs.** AUR account
  registration is unavailable, so Publish no longer writes or pushes an
  AUR package. Homebrew (`blox-eng/tap/matchblox`) stays. Install with
  the script, Homebrew, or Go.

## [0.17.1] - 2026-10-22

### Fixed

- **`z` parks a Cursor session.** Cursor has no session file, so its chat
  id never reached the hold. The status line and hooks now name it, and
  park works after the first turn.

## [0.17.0] - 2026-10-22

### Added

- **`matchblox update` gets the latest release, and Homebrew and the AUR
  install matchblox.** The update makes the same checks as the install
  script: the checksum and, with `gh` logged in, that CI built it. When a
  check fails, nothing changes. When Homebrew or the AUR installed
  matchblox, it tells you their command instead:
  `brew install blox-eng/tap/matchblox`, `yay -S matchblox-bin`.
  ([#95](https://github.com/blox-eng/matchblox/issues/95))

## [0.16.0] - 2026-10-22

### Added

- **The app takes you back to matchblox in one move.** From any pane,
  swipe in from the left edge, tap `◂ matchblox` in the tmux status line,
  or press ⌘M on a hardware keyboard. It no longer takes the tmux prefix
  and `m` on the phone keyboard. The console gives tmux a key of its own
  for it, which no keyboard types, so it works whatever your prefix is
  and takes no key from the apps in your panes. The tap no longer needs
  the tmux mouse in the app.
  ([#121](https://github.com/blox-eng/matchblox/issues/121))

## [0.15.0] - 2026-10-22

### Added

- **`matchblox switch <who> --agent <name>` runs another agent in a session's
  pane:** same name, directory, park and ship, with a handoff line to start
  from. Switch it from the CLI, or `A` in Sessions.
- **`A switch` sits under the selected session, and a tap presses it.** A
  phone's opened row and the detail beside the list carry the session's
  keys; a tap on one presses it, so the iPhone app switches an agent with
  no keyboard. On a narrow phone the plan and the keys get a line each, so
  `! compact` is no longer cut.
- **Sessions says `outside tmux` for an agent with no pane.** The row and
  `matchblox sessions` name the place, instead of leaving it blank.

### Changed

- **A step asks for consent in a box, not on the bottom line** (#158).
  Kill, switch, merge, remove, connect, a jump and the stoker's take-over
  show what runs in a box over the dimmed screen: its title says what it
  does, its lines the exact commands, its foot the key. On a phone it rises
  above the key line; wider, it sits in the middle. A door's long diff keeps
  its confirm on the key line.

## [0.14.0] - 2026-10-21

### Added

- **Cursor sessions show their context, model and account.** A new setup
  step, "Show Cursor's context", taps Cursor's status line: each Cursor
  session then shows its context use and window from its first turn, as
  Claude Code's do, and Cursor's bar still shows. The account is the email
  and team in `~/.cursor/cli-config.json`, marked `⬢`, and USAGE lists it.
- **Cursor's plan usage in USAGE, when you turn it on.** With
  `cursor_usage = true` under `[limits]`, matchblox asks Cursor's API every
  5 minutes for the share of the month's included usage, as Cursor's
  `/usage` shows it, and when it resets. It reads the login token Cursor's
  CLI keeps in the macOS Keychain for each call, and never keeps it.
### Fixed

- **A renamed Cursor, Codex or OpenCode session shows its new name.** An
  agent that keeps no name of its own is named as its tmux window (which
  `r` renames), or its tmux session, no longer as its command.
- **`matchblox new` starts Cursor trusting the folder you chose**
  (`--trust`), so it does not stop at Cursor's trust prompt.
- **A Cursor session at its trust prompt waits for you.** "Do you trust the
  contents of this directory?" shows in the queue, as its approval menu
  does.

### Security

- **Built with Go 1.26.10.** It fixes ten advisories in Go's own `net/http`,
  `net/textproto`, `crypto/tls` and `crypto/x509` (GO-2026-6603 to 6617 and
  GO-2026-6809), which Cursor's usage call makes reachable.

## [0.13.0] - 2026-10-21

### Added

- **The box opens: pair a device in one gesture.** The door "Pair a
  device" and `DEVICES` on the Machine tab open the pair page in the
  console. It shows the matchbox with the code on its label. When the app
  reaches the host the box knocks; when it claims the code the box opens,
  the match strikes, and the label shows the device's name. Then the page
  shows when the device is in and what it did first. A device sees
  `DEVICES` but cannot pair or remove: it says to use the host's own
  terminal.
- **The app has no pairing form.** The scan is the consent: the box on the
  screen catches the code and opens, with haptics, and the console slides
  in. A device that cannot scan can paste the link. A failure shows the fix
  on the screen.
- **The pair code is half its area.** The v2 link packs the same facts in
  binary, so the code is at most 57 × 29 cells (v1 was 77 × 39) and fits a
  normal terminal.

### Changed

- **Build the app again with this release.** An older app cannot read the
  v2 link and says "This code is damaged…". The new app reads v1 and v2.

## [0.12.0] - 2026-10-21

### Added

- **Cursor's CLI is an agent like the others.** Sessions shows it, started
  as `agent` or `cursor-agent`. The setup step "Add the Cursor hooks" puts
  matchblox in `~/.cursor/hooks.json`, so a finished turn waits for you as
  `done` and a reply that asks as `asks`; its approval menu is read from
  its pane. `matchblox new --agent cursor-agent` and `[stoker] agent =
  "agent"` start it. It shows no context use or account yet, and no merge
  guard watches it.

## [0.11.0] - 2026-10-21

### Added

- **The app runs on the iPad.** It was an iPhone app, so an iPad showed it
  in a small phone window. It now fills the screen and turns to each side.
- **The app has an icon: the matchblox mark.** It was blank on the home
  screen. `make icon` makes it from `ios/AppIcon.svg`.

## [0.10.3] - 2026-10-21

### Fixed

- **The phone shows the console's glyphs, not `_`.** The app's ssh session
  sends no locale, so tmux took the phone for an ASCII terminal and drew
  each match, bar, `▰`, `●` and `◂ matchblox` as `_`. The console now
  starts tmux with `-u`, so its client is UTF-8 whatever the locale.

## [0.10.2] - 2026-10-21

### Fixed

- **Context use is a share of the session's real window.** A session of a
  1M model, such as Opus 5.5 without `[1m]` in the model setting, showed
  its context against 200k: 99% while it had room for 800k more, then 20%
  past 200k. The stoker could compact it at about 170k. The window now
  comes from the agent first: Claude Code's status line gives it for each
  session. Then from `[sessions.windows]`, then from the Claude models that
  matchblox knows. Without a window the session is "not measured" and the
  row shows `—`; matchblox no longer guesses 200k. `[sessions.windows]`
  no longer wins over a window that the agent reports.
  ([#118](https://github.com/blox-eng/matchblox/issues/118))
- **A new session shows in Sessions within a second.** It took up to 10
  seconds: the processes were read every fifth sample. tmux is now listed
  at every sample, and a new pane, or a pane that starts an agent, has its
  processes read at once. A session from `N` or `matchblox new` shows at
  once. ([#119](https://github.com/blox-eng/matchblox/issues/119))
- **A session working through its subagents no longer waits for you.** A
  turn that dispatches background subagents ends at once, and its Stop hook
  put the session in "waits for you" while the subagents worked, and while
  the turn their results start ran. Subagent-driven development showed as
  waiting most of the time. A turn that ended asking nothing now stays out
  of the queue while the session works, and comes back when the work ends,
  waiting since then, not since the turn that dispatched the subagents. A
  question or an approval still waits.
- **A peek shows the question, not the progress line under it.** `asks` in
  `matchblox peek` and in the stoker's wakes was the reply's last line,
  often the `Progress [...]` line that ends each reply.

## [0.10.1] - 2026-10-15

### Fixed

- **Releases carry their binaries again.** v0.8.0, v0.9.0 and v0.10.0 were
  published with none: the repository's releases are immutable, and a
  published release takes no assets. A release now stays a draft until its
  binaries, checksums and SBOMs are attached, and the install script
  kept installing v0.7.0 meanwhile. This release is the first to
  carry what v0.8.0 to v0.10.0 added.

## [0.10.0] - 2026-10-14

### Added

- **Start, rename and kill a session from matchblox.** `N` opens New
  session: where (a path, or the Git tab's repositories and worktrees), an
  optional branch for a new worktree beside the repository, a name, the
  agent (the installed Claude Code, Codex or OpenCode) and a first message.
  It starts in a tmux session of its own, and the tmux session, its window
  and Claude Code's own name are that one name. `r` renames a session the
  same way, all alike; a busy Claude Code session takes its name when it
  rests. `x` kills any session, not only a stale one: its exact commands
  on the RUN line, a typed `y`, and a second `y` for one that works or
  waits for you. Its worktree and transcript stay. All three are on the
  key line, a tap away on a phone, which drops the spare keys first. The
  same as commands: `matchblox new`, `matchblox rename`, `matchblox kill`.
- **The stoker's hands start, rename and kill sessions.** `matchblox new`
  works only in a run and only on your tmux server, and the guard watches
  the session it starts. `matchblox kill` ends only a session at rest you
  did not park, with the new `kills` go chip or your `y` on the card.

## [0.9.0] - 2026-10-14

### Added

- **The stoker takes orders, in your own words.** `n` and `f` start a
  stoker agent (Claude Code, or `[stoker] agent`) on a tmux server of
  matchblox's own, out of your `tmux ls`; `Enter` on the STOKER tab opens
  it in a popup, `Q` quits it. Then they open a launch sheet: say how to run tonight, turn on
  the gos you give, and `Enter` starts the run at once. The brief carries
  the machine as it is, so the agent needs no lookup to start, and it keeps
  its own plan on the card, rewriting it as it goes: nobody approves it. A
  night is sealed once it runs. By day, `o` sends a new order as one line,
  `New order: …`, and the run goes on with its done steps. matchblox calls
  no model. The stoker's own
  session stays out of Sessions, and the header says when it waits for you.
- **Your ways.** One markdown file, `ways.md`, next to the config, holds
  your standing rules and your `# Recipe: <name>` procedures. The launch
  sheet offers each recipe by name. The setup door **Write your ways**
  writes a starter. See [Your ways](https://docs.matchblox.com/ways/).
- **The stoker acts through your sessions, and matchblox wakes it** when a
  session asks something or its turn ends (with the first line of the
  question, or the last line it said), a pull request appears or moves, a
  usage window passes another 10%, and before the night ends, at most once
  a minute. Its hands are `matchblox peek`, `send`, `compact`, `hold` and
  `release`, under the guards of the console, and `max_steps` (60) bounds
  the messages of a run. The setup step "Add the queue hooks" lets them
  run in Claude Code without a prompt.
- **Triggers the service fires itself.** `matchblox stoker when` sets one
  for the run: at a pull request's stage (`--ship 4573 --stage merged`,
  `red` for CI failed) or at the end of a session's turn (`--rest api`),
  matchblox sends, compacts, releases or holds as the stoker said, once,
  under the same guards, logs it to the card and tells the stoker. The
  stoker sets a trigger where it would otherwise poll.
- **The STOKER tab** shows your words and the gos, the plan, what waits
  for your go, the triggers, the standing rules and every step with its
  outcome and the exact command it ran. `y` gives the go a step waits for,
  and `a` opens the session of a failed step.
- **A merge, a release or a deploy needs its go.** The gos are the chips
  on the launch sheet (`Tab` to them, `Space` turns one on), never read
  from your words. In a session the stoker messaged, a `PreToolUse` hook
  stops `gh pr merge`, `gh release` and `gh workflow run` without one, and
  matchblox puts the command on the card by itself
  (`⏸ waits for you: api wants to run gh pr merge 4556`), where `y` lets
  one command of that kind through, once. The session reads why it was stopped, and a
  here-document a command reads as text (`cat > notes.md <<'EOF'`) needs
  no go. Codex and OpenCode have no such hook, and the card says so.
- **Sessions say why they are idle** (`#4556 waits your merge`, `CI red`,
  `stuck: …`, `back 14:20`). `z` parks a session so nothing types into it,
  and `g` groups the list by need, state, repo or not at all.
- **`matchblox sessions`, `ships` and `usage`** print one line a row, or
  JSON with `--json`, for a script or an agent.
- **The matchblox iPhone app opens your console** (#88). Scan the code of
  `matchblox pair` once, and the app opens the console of the host. Face ID
  or the passcode unlocks the key each time the app opens. The key stays in
  the Secure Enclave, and each host gets its own key. The app trusts only
  the host key from the code, so a host with a new host key does not open
  until you pair again. With two or more hosts, the app lists them, and a
  swipe removes one. When the app can't reach a host, it asks for a new
  address and keeps the pairing. Build the app from `ios/` until it is on
  TestFlight.


## [0.8.0] - 2026-10-14

### Added

- **`matchblox devices` lists the phones and laptops that open this host,
  and removes one.** `matchblox devices rm <name|fingerprint>` removes a
  key and keeps a backup of `authorized_keys`. A laptop shows the comment
  of its key as its name, which is `matchblox` for a key from
  `matchblox connect`.
- **`matchblox pair` shows a one-time QR code for the coming iPhone app**
  (#88). The code holds a key that works once and that sshd refuses after
  5 minutes. Stop the command, or close the terminal, and the key is gone.
  When a phone pairs, the host prints its name and fingerprint, so a phone
  that is not yours is seen at once. A key that already opens a login on
  the host cannot pair, so a pairing never changes your own ssh login.
- **A phone key opens the console.** The new key kind (`gate phone`) starts
  the stream and the console. A laptop key from `matchblox connect` keeps
  the stream alone.

### Changed

- **The home session uses `window-size latest`.** Its size now follows the
  device you used last, so a phone and a laptop on one session do not
  keep the size of the smaller one.

## [0.7.0] - 2026-10-13

### Changed

- **matchblox.com shows the console on an iPhone Duo.** Folded it plays the
  phone's one column; it unfolds sideways into the desk layout partway
  through, and a tap folds it again. It replaces the Phone, Wide and Desk
  switch, and the 107×60 recording that only the switch used.

## [0.6.0] - 2026-10-12

### Added

- **The console fits the screen it is on.** A closed phone shows one line
  a session, grouped by what each needs from you (waits for you, working,
  quiet), and the selected one opens in place with what it said and its
  keys. A screen held upright is read by its shape, so its keyboard does
  not change the layout: under 90 columns it is this one column, from 90
  (a phone with a small font) the detail is a drawer beside the list. On a
  screen held sideways, from 60 columns, the selected session's detail
  sits beside the list. Every column of a list lines up, the name taking
  what is left, and the selected row is washed across the whole pane with
  its name in the accent, the others muted.
- **Answer or message from a peek.** `a`, or a tap on a selected session,
  opens the conversation: the detail pane itself where there is one, the
  list staying beside it, else a panel at the bottom of a phone, with
  what the agent said last and your answer; a session
  that works gets your line as a message, which Claude Code queues. `→`
  and `←` (a swipe on a phone), or a tap on `next →` and `← prev`, move
  the peek to the next or previous session. When Claude
  Code asks with its question tool, its options are there, numbered: a digit
  or a tap picks one, several questions come one at a time, and a question
  of several choices ticks with `Space`. `Enter` sends, and the peek moves
  to the next session that waits.
- **Ships.** Every pull request a session drives, on one track from open to
  live: CI, a conflict, a review, the merge and the deploy, said in words.
  `x` merges a ship that waits for you, after a typed `y`, pinned to the
  commit that was read green. Name the
  workflows that deploy a repository under the new `[ships.deploy]` config
  key; by default every run on the merge commit counts.
  A running stage says how long it has run (`merged · deploying · 2h`,
  `CI · 6 of 9 jobs · 12m`) or waited (`merged · deploy queued · 1h`). A
  failed run on the merge commit counts once a newer run of its workflow
  on the default branch passed.
- **Usage, the machine and the stoker in the rows the list leaves free.**
  Each account's windows as `/usage` says them, the machine (cpu, memory,
  the disk your home is on, the agents that run) and the stoker, whose
  panel takes the dock while a run is in charge; `c` opens its whole card.
  With rows to spare the dock grows: forecasts, the top processes, the
  recent steps.
- **Recommendations you can put away.** They sit under the sessions now.
  `d` dismisses one until its evidence changes, and `h` folds them all to
  one line; the console remembers both.

### Changed

- The tabs are Sessions, Ships, Machine, Git, History and Panes, each
  named, and the digits `1-6` follow that order. The Queue is the top of
  Sessions, the processes are on Machine, and `p` opens Ships.
- Usage speaks in `/usage` percentages and keeps its forecast; the matches
  and sparks of the header are gone.
- An answer goes on `Enter`: the peek shows what goes where, so there is no
  typed `y` for it any more.
- The keys are at the bottom of the screen, each a tap target, where a
  phone app keeps them above its keyboard.
- **The queue hooks include the question tool.** A `PreToolUse` hook for
  `AskUserQuestion` tells the console what Claude Code asks before its menu
  draws, so the answer peek shows the options in auto mode too, where the
  question reaches the transcript only once it is answered. The step "Add
  the queue hooks" asks to be opened again, with the one entry it adds.

## [0.5.1] - 2026-10-12

### Fixed

- **A session working through its subagents reads `agents`, not idle.**
  Its turn ended, but their results start its next turn: the queue no
  longer lists it as waiting for you, and the stoker still compacts it at
  that turn's end when its pace calls for it. A turn that ended with only
  background shells running stays idle.

## [0.5.0] - 2026-10-12

### Added

- **The stoker compacts each session at the moment that suits it.** It
  follows each session's pace (tokens per point of its progress bar): one
  that finishes under the line is left alone, one whose next step would
  cross it is compacted at the end of this turn. The card says what it
  decided for every session, and why.
- **A session that ended its turn is idle, even while its background
  works.** Its detail shows the subagents and shells it left running, and a
  shell that has run for an hour or more can be ended with `x`.
- **A net under every session.** A door turns Claude Code's own
  auto-compact on at the stoker's line, the only thing that compacts inside
  a turn.

### Fixed

- The site's news line names the latest release: each release writes it
  from the first headline of its changelog section, and links to the
  release.

## [0.4.0] - 2026-10-11

### Added

- **See which sessions your plan's limit stopped, and until when.** A
  session whose turn the limit refused, or whose account spent its window,
  says `out` with the time it can work again, in Sessions and in
  `matchblox status`.
- **The stoker's card tells the truth about the limit.** A run the limit
  stopped says so on its first line (`limit out 11:51 → 14:50`), and a
  session that is out is not counted as ready.
- **A 1M window can fill.** `compact_tokens` (350k by default) fills a
  session before `compact_at`'s percentage does, so the stoker and the
  `! compact` mark act on large windows too. To keep the percentage alone,
  set `compact_tokens = 0`.
- **Every stoker card is kept** in `stoker-runs.jsonl`: a new run never
  erases the one before it.

## [0.3.0] - 2026-10-11

### Added

- **See all of matchblox before you install it.** The replay on
  matchblox.com and in the README walks one evening into the morning: an
  answer typed in one line, a compact and a worktree removal you confirm,
  a busy loop killed, then a night the stoker keeps burning and its score
  at 07:00.

### Fixed

- **A session without hooks shows what it asked** on its queue row, so `a`
  answers a question you can read.
- **A removed worktree leaves the Git tab at once**, not at the next scan.
- One merged worktree and one process are named in the singular.

## [0.2.0] - 2026-10-11

### Added

- **You can see the night.** While the stoker or a night runs, the whole
  console goes dark blue and only the matches keep their fire.
- **Take over with one key.** The first time you act on a session during a
  run, the console asks `Take over?`. `y` rests the stoker while you work,
  and it goes on by itself once you leave. `n` keeps it on.
- **Wake up to the night's score.** Every night and every stoker run ends
  with a card in the queue, even when there was nothing to do: what it
  watched and compacted, the tokens it gave back, and `ready 6/7`, how
  many sessions can go on, with what each of the others needs.
  `matchblox status` shows it too.

## [0.1.1] - 2026-10-11

### Fixed

- **docs.matchblox.com shows the docs of the latest release**, with the
  release install. It showed the text from before the first release.

## [0.1.0] - 2026-10-10

The first release: one console for every coding agent you run in tmux, on
the host, over SSH and from a phone.

### Added

- **The queue**: every agent that asks for permission, waits for input or
  finished its turn, oldest first, with its last line. `Enter` goes to its
  pane, and `a` answers in one line after a typed `y`. Claude Code reaches
  the queue through its hooks; Codex, OpenCode, Gemini CLI, aider,
  cursor-agent and any command named in `agents` reach it from their pane.
- **Each session is a match**: `✦` burning while it works, `╿` at rest
  while it waits, `│` spent at the compact limit.
- **Sessions**: how full each context is (Claude Code, OpenCode, and Codex
  on Linux), the queue's word (`asks`, `waits`, `done`, `paused`), the
  account each one runs under, and its open pull request as `#N`.
- **The machine**: load, memory and health, detached busy loops, git
  checkouts and merged worktrees, recommendations with one guarded action
  each, and 24 h of history.
- **The stoker**: press `n` before you sleep, or `f` for a lunch. Each
  Claude Code session that fills its context compacts itself and leaves one
  RESUME line. In the morning, STOKED shows each one, its context before and
  after. A session that waits for you, or holds a line you started to type,
  is left as it is.
- **Your limits**: one line for each account under the metrics, with the
  matches left in this 5-hour window, the sparks left this week, and if the
  week lasts (`out Fri ~20:00`). When it will not, the queue names the
  session that burns the most, one key from a compact. Claude Code's come
  from your status line, Codex reports its own.
- **Any terminal, even a phone**: a tap selects, a second tap acts. The way
  back, `prefix m` or a tap on `◂ matchblox`, returns to the console from
  any pane; `m` in its preview turns the tmux mouse on for the tap.
- **Remote hosts**: `matchblox connect <host>` connects a host once, with a
  key that can start only the console's stream, and `matchblox <host>`
  opens its console.
- **Setup in the queue**: tmux, the queue hooks, your limits, the way back
  and a guide session, each shown as the exact change before a typed `y`.
- **Install in one line**: `curl -fsSL https://matchblox.sh | sh` installs
  the release binary for Linux or macOS, amd64 or arm64, its checksum and
  build provenance checked, and starts it. `go install` builds the same
  version, and `matchblox version` names it.
- **Docs** at [docs.matchblox.com](https://docs.matchblox.com).

[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.26.1...HEAD
[0.26.1]: https://github.com/blox-eng/matchblox/compare/v0.26.0...v0.26.1
[0.26.0]: https://github.com/blox-eng/matchblox/compare/v0.25.1...v0.26.0
[0.25.1]: https://github.com/blox-eng/matchblox/compare/v0.25.0...v0.25.1
[0.25.0]: https://github.com/blox-eng/matchblox/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/blox-eng/matchblox/compare/v0.23.2...v0.24.0
[0.23.2]: https://github.com/blox-eng/matchblox/compare/v0.23.1...v0.23.2
[0.23.1]: https://github.com/blox-eng/matchblox/compare/v0.23.0...v0.23.1
[0.23.0]: https://github.com/blox-eng/matchblox/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/blox-eng/matchblox/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/blox-eng/matchblox/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/blox-eng/matchblox/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/blox-eng/matchblox/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/blox-eng/matchblox/compare/v0.17.4...v0.18.0
[0.17.4]: https://github.com/blox-eng/matchblox/compare/v0.17.3...v0.17.4
[0.17.3]: https://github.com/blox-eng/matchblox/compare/v0.17.2...v0.17.3
[0.17.2]: https://github.com/blox-eng/matchblox/compare/v0.17.1...v0.17.2
[0.17.1]: https://github.com/blox-eng/matchblox/compare/v0.17.0...v0.17.1
[0.17.0]: https://github.com/blox-eng/matchblox/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/blox-eng/matchblox/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/blox-eng/matchblox/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/blox-eng/matchblox/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/blox-eng/matchblox/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/blox-eng/matchblox/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/blox-eng/matchblox/compare/v0.10.3...v0.11.0
[0.10.3]: https://github.com/blox-eng/matchblox/compare/v0.10.2...v0.10.3
[0.10.2]: https://github.com/blox-eng/matchblox/compare/v0.10.1...v0.10.2
[0.10.1]: https://github.com/blox-eng/matchblox/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/blox-eng/matchblox/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/blox-eng/matchblox/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/blox-eng/matchblox/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/blox-eng/matchblox/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/blox-eng/matchblox/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/blox-eng/matchblox/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/blox-eng/matchblox/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/blox-eng/matchblox/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/blox-eng/matchblox/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/blox-eng/matchblox/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/blox-eng/matchblox/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/blox-eng/matchblox/releases/tag/v0.1.0
