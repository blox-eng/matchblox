# Config

matchblox works without a config. The first run writes
`~/.config/matchblox/config.toml` with goals for this machine (`load1_over`
is the number of cores) and the agents it finds. Every known agent is a
session without it; `agents` adds others. Every key is optional.

The service reads the config when it starts. To apply a change, stop the
service and run `matchblox` again. The lock file next to the service socket
holds its pid. With `XDG_RUNTIME_DIR` set (most Linux systems):

```sh
kill "$(cat "$XDG_RUNTIME_DIR/matchblox/matchblox.sock.lock")"
matchblox
```

Without it (macOS), the lock is in the temp directory:

```sh
kill "$(cat "${TMPDIR:-/tmp}/matchblox-$(id -u)/matchblox.sock.lock")"
matchblox
```

To use another file, give `matchblox --config <file>`.

## Every key

```toml
--8<-- "config.example.toml"
```

## The repositories N offers

The first console on a host asks once: `Find the Git repositories in your
home folder?` With a yes, the service looks four folders deep in the home
folder itself, `~/dev`, `~/src`, `~/code`, `~/projects` and `~/work`, then
in `[git] repos` and `[git] scan_roots`, and N's `where` offers what it
finds. `p pick folders` scans the folders you type instead of the usual
ones: a space between them, and a folder in double quotes keeps its spaces
(`"~/My Work" ~/dev`). The folders you type that are not there are named,
and nothing is recorded: the folders on record stay. The two config keys
always add. It skips `node_modules`, `vendor`, `target`, `Library` and
hidden folders, follows no symlink, and does not look inside a repository
(a home folder kept in git is the one searched below its `.git`). It reads
files only: no git runs, nothing goes on the network. It scans again every
30 minutes.

Some folders are not walked, and the scan says which and why, in the
console and in `matchblox scan`:

- **Folders macOS guards**, on a Mac: `~/Documents`, `~/Desktop`,
  `~/Downloads`, `~/Library`, `~/Pictures`, `~/Movies` and `~/Music`, when
  the walk reaches them from above, so macOS never asks for them. A folder
  you name (`p pick folders`, or `matchblox scan <folder>`), itself or
  anything inside it, is walked, and macOS asks then.
- **Network folders** below a root (NFS, SMB, sshfs, rclone and other FUSE
  remotes, autofs), found by the mount table without touching them. A root
  you name is always walked. Local mounts (btrfs subvolumes, ZFS datasets,
  bind mounts, a second disk) are walked.
- **A folder that does not answer** within the bound. It is listed, keeps
  the repositories found there last time, and is not waited on again for 6
  hours, unless you name it.

A repository shows once, by its real path (symlinks resolved, letter case
as on disk), and its remote never shows a password or token.

The answer is in `~/.local/state/matchblox/scan.json` and the list in
`repos.json` beside it. The repositories found never show in the Git tab.
`matchblox scan` scans now; `matchblox scan off` stops and forgets the list.

## Where N clones

N's `where` takes a git URL: `https://…`, `ssh://…`, `git@host:o/r` or
`gh:o/r`. A clone goes into `[git] clone_root`, `~/src` by default, in a
folder named after the repo (`~/src/api`). When that name is taken by
another repo, a file or a clone in progress, the folder gets a match word
(`ember`, `flint`, `spark`, then more) and, when all are taken, a word with
a 4-character tail. `o/r` on its own uses the protocol that
`gh config get git_protocol` says, else https. The clone is a full clone and
never asks for a password: git has no terminal, so a repo that needs one
says why and how to sign in. A URL with a token in it is refused: an
`https` URL takes no user or password, an `ssh` URL only a plain user name
(`git@`), and none takes a query or a fragment. While it runs, a
clone works in a hidden `.<name>.cloning` folder, with a
`.<name>.cloning.owner` file beside it that names the service. After a crash,
matchblox clears that folder only when the owner file names a process that is
gone; a folder it did not make is left alone, and its name counts as taken.

matchblox keeps where its sessions started in `~/.local/state/matchblox/used.json`.
N's `where` lists those folders. It needs no scan consent, and `matchblox
scan off` keeps it.

## The stoker's agent

`[stoker] agent` is the command of the stoker agent. matchblox starts it
itself, on a tmux server of its own (`tmux -L matchblox`, session
`stoker`), the first time you press `f` or `n`. It is `claude` by default.
`codex`, `opencode` and `cursor-agent` (Cursor, also installed as `agent`)
work too. The command must be on the service's `PATH`.

`[stoker] max_chains` is the most chain stokers that run at once, one for
each chain. It is 4 by default, and 0 or less means 4. Each chain stoker is
one more agent that uses tokens, and each one counts its own `max_steps`.
A start past the cap is refused with
`the family has its 4 chain stokers: max_chains. Quit one on the STOKER tab first.`
A chain stoker runs the agent of `[stoker] agent`, on the default sign-in.
See [A family of stokers](stoker.md#a-family-of-stokers).

```toml
[stoker]
max_chains = 2
```

`[stoker] pane` is gone: you no longer start the stoker in a pane of your
own. `matchblox status` warns while the config still names it, and nothing
reads it. Remove the line. See [the stoker](stoker.md#the-stoker-agent).

## Chains

`[chains] notice` tells a match, between its steps, of another match in
its repo that it could chain with. It is `true` by default. With `false`, a
match still hears its chain's news, and who joins its chain.

`[chains] send` is who a match's `matchblox send` reaches. `"chain"`, the
default, is the matches it shares a chain with. `"off"` is no other match. Any other
value is an error. Either way, a send from a terminal, a tmux pane or the
console goes as before, and one with neither a pane nor a terminal is
refused. See [chains](agents.md#chains).

One match opts out alone with `matchblox chain --off`.

## Update

`[update] mode` is what the host does about a newer release. It is `"offer"`
by default. Any other value is an error that names the three modes. Each
machine has its own.

| Mode | What the host does | What leaves the machine |
| --- | --- | --- |
| `"offer"` | Finds a release and offers it in the console. You install it. | One request a day to find the latest release. The download of the release when you install it. |
| `"auto"` | Installs a release a day after it first sees it, with no stoker run on and no clone running. | The same request, then the download of the release. |
| `"off"` | Does nothing. | Nothing, until you press `u` on this host's row in another console. |

```toml
[update]
mode = "auto"
```

The mode, like every key, is read when the service starts. After you
change it, start the service again: see the top of this page.

The service looks when it starts and then once a day. The console never
looks. A build that is not a release (a `dev` build, or one from a
checkout) never looks, and a prerelease is never offered. A look that
fails keeps the release it found before, and never reads as up to date.
One failed look, or two days of them, stay quiet: `matchblox status` gives
the reason in `update.err`. After three days of failed looks, counted from
the first failed look, Sessions shows the step "Release check fails" under
`SET UP`, with the reason and `Run: matchblox update`. A look that works starts the count again. Closing
the step hides it until a look works and the looks then fail for three more
days.

**offer.** Sessions shows the step "Update to v0.43.0" under `SET UP`, with
"matchblox v0.43.0 is out. This machine runs v0.42.0."
`Enter` shows the release it installs and the file it replaces. It also
says what the install checks first: the checksum always, the build's
provenance if `gh` is installed and logged in. A typed `y` installs it, with the same
checks as `matchblox update`. The service then runs again on the new
release, and the console starts again on it. When the console cannot start
itself again, it says `updated to v0.43.0 · quit and open matchblox again`.
Your sessions run on: they live in tmux. When the install fails, the offer
says `The install failed, nothing was changed. Enter tries again, or run: matchblox update.`
and then the reason, until the next try. The row shows the first 12
characters of each checksum; `matchblox update` prints them whole. `x` closes the offer of that
release only, and the next release shows a new one. Until then,
`matchblox update` installs it. `matchblox setup` does not show it.

**auto.** The service installs the release by itself, with no console
open. It waits a day from when this machine first saw the release, and for
a time when no stoker run is on and no clone runs. A newer release during
the wait starts the day again. The offer shows during the wait, and says
when auto installs the release: `Auto installs it after Nov 13 at 09:00.`,
in the time of the host. When the day
has passed and work is in flight, the offer says why it waits:
`Auto waits for the stoker run to end.` or `Auto waits for a clone to end.`
Closing the offer does not stop the install: to skip a release on this
machine, use `"offer"` or `"off"`.

The install makes the same checks as `matchblox update` and records which
ones it made. The console then says
once `updated to v0.43.0 at 03:12` (`on Nov 11 at 03:12` when it was not
today), or why the update failed and the command to run. The service checks
the build's provenance only with a `gh` that is installed and logged in for
its user: without one it installs on the checksum alone, and the console
adds `· provenance not checked`, as do the step of an install that waits on
its restart and the result of `u` on a host. Auto installs a
release once when the install works. A failed install is tried again after
the next look.

An install from the offer or from `u` waits for a clone that runs, which
the restart would cut off: it says `a clone of o/r runs. Update when it ends.`
A stoker run does not hold it: the run goes on after the restart. From the
start of any install, from the offer, from auto or from `u`,
until matchblox runs again on the new release, a new clone is refused: see
[Limits](limits.md). When the service still runs the old release two
minutes after the install, or starts again on the old release, Sessions
shows the step "Restart for v0.43.0" with
`v0.43.0 is installed, but this service still runs v0.42.0. Quit matchblox and open it again.`
A new clone then says `v0.43.0 is installed. Quit matchblox and open it again.`
While the service waits for that restart, the step shows under every mode
and over a newer release, a second install answers with the same words, and
`x` does not close the step. After a start on the old release, `x` closes it
until a newer release comes out.

**off.** No look, no request and no offer. A person who presses `u` on this
host's row in another console still updates it: see
[A host on another release](remote.md#a-host-on-another-release).

Homebrew owns the binary it installed, so matchblox never replaces it. The
offer says `brew upgrade matchblox`, and `"auto"` does nothing.

Only a person at a console installs from the offer, updates a host or opens
any step under `SET UP`: on the host, over SSH, or in the phone app. An
agent's session, a session the stoker drives and a `matchblox` command are
refused. `matchblox update` in your own shell is yours, and updates this
machine as before. See [Limits](limits.md) for what this does not stop.
