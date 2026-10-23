# Config

matchblox works without a config. The first run writes
`~/.config/matchblox/config.toml` with goals for this machine: `load1_over`
is the number of cores, and the agents it finds (the known agents count
without it; `agents` adds others). Every key is optional.

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

## The stoker's agent

`[stoker] agent` is the command of the stoker agent. matchblox starts it
itself, on a tmux server of its own (`tmux -L matchblox`, session
`stoker`), the first time you press `f` or `n`. It is `claude` by default.
`codex`, `opencode` and `agent` (Cursor) work too. The command must be on the service's
`PATH`.

`[stoker] pane` is gone: you no longer start the stoker in a pane of your
own. `matchblox status` warns while the config still names it, and nothing
reads it. Remove the line. See [the stoker](stoker.md#the-stoker-agent).
