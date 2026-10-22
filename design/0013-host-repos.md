# The builder's repos on the host: design

Date: 2026-10-22. Status: built. Issue: #173.
Consumed by: #174 (where as a filtered list), #175 (start rows on an empty
Sessions), #176 (paste a git URL).

## Intent

A host that already has dozens of checkouts should offer them in N without
a typed path. matchblox finds them once, with the builder's consent, keeps
the list on disk, and refreshes it in the background. Nothing about the
scan leaves the host, and it never makes the console wait.

## Goals

1. **Ask once.** The first console on a host asks in the consent box (#158):
   `Find the Git repositories in your home folder?`, with `they stay on this
   host and your paired devices` under it, and the keys `y` / `n` / `p pick folders`. The answer is
   stored; it is never asked again.
2. **Scan in the service.** The service owns the host, so it scans, not the
   console. The scan runs in its own goroutine and never holds `s.mu` while
   it reads the disk.
3. **The walk** (as the issue says):
   - roots: `~`, `~/dev`, `~/src`, `~/code`, `~/projects`, `~/work`, plus
     `[git] repos` and the new `[git] scan_roots`. Picked folders replace
     the six defaults; config entries always add.
   - depth 4 below each root; a directory holding `.git` is recorded and
     not descended.
   - skips `node_modules`, `.cache`, `Library`, `vendor`, `.venv`, `target`,
     and every name starting with `.`.
   - follows no symlink below a root.
   - records path, remote URL, branch, and the time of the last checkout or
     commit.
4. **Cached.** The list is on disk; a new service sends it at once, before
   its first scan ends. Scans run after the first sample, then every 30
   minutes, and on `matchblox scan`. Each scan replaces the list: new
   repos join, removed ones drop.
5. **Found is not the Git tab.** Found repos feed `gitPaths()` behind N's
   `where`, after the Git tab's paths. The Git tab draws only
   `snap.Git`, as today.
6. **`matchblox scan`.** Records a `y` (running the command is the consent)
   and scans now. `matchblox scan <folder>...` records those folders as the
   picked roots. `matchblox scan off` records an `n` and deletes the cache.

## Non-goals

- Order and filter of the list (#174). Until then found repos follow the
  Git tab's paths, the most recent activity first.
- Cloning (#176), start rows (#175).
- Watching the roots for changes.
- Other users' home folders on a shared host: the defaults are all under
  the builder's home; only a configured or picked root can point outside.

## Anti-goals

- A scan without a `y` on record. The service reads the consent file before
  every scan, not once at start.
- A network call of any kind: no `gh`, no `git fetch`, no git subprocess at
  all. The scan reads files.
- A console that waits for the scan, or a snapshot every 2 s that grows by
  the whole list.
- The list going anywhere but this host and the consoles paired with it
  (owner, 2026-10-22: paired devices get it as they get the Git tab, and
  the box says so).
- A token in a remote URL reaching a console: `https://user:token@host/o/r`
  is sent as `https://host/o/r`.

## Constraints

- The console must not read the machine (`internal/arch_test.go`): it
  draws what the service sends.
- Old consoles and the phone app keep working: no `proto.Version` bump.
- A big home folder finishes in seconds.

## Open questions, settled (calls made for the owner, 2026-10-22)

### 1. Interval, not fsnotify

Rescan after the first sample, then every 30 minutes, and on
`matchblox scan`. A constant, not a config key.

Why: fsnotify watches one directory per descriptor on macOS (kqueue) and
hits `max_user_watches` on Linux; a depth-4 home has tens of thousands of
directories. It is also a new dependency (none in `go.mod` today), and a
new checkout appearing 30 minutes late costs one typed path. A scan that
reads files and no git is cheap enough to repeat.

### 2. Two files next to state.json

In `internal/state`, beside `Path()`:

- `$XDG_STATE_HOME/matchblox/scan.json`: the consent. `{"answer": "yes" |
  "no", "roots": [...], "at": <time>}`. Missing: never asked.
- `$XDG_STATE_HOME/matchblox/repos.json`: the cache, the last `Found`.

Two files, because the consent is a decision and the cache is disposable:
deleting or corrupting the cache never asks again. Both are written
atomically, as `state.Write` writes state.json (its temp-and-rename moves
into a shared helper). Mode `0o600`, directory `0o700`.

### 3. A field of proto.State, sent like History

Not inside `proto.GitReport`, and not a new message kind:

```go
// proto.State
Found *Found `json:"found,omitempty"`
```

It goes in the first snapshot to each console, and again only when it
changes (a per-client generation, as `histSent` does for History). Nil
means "unchanged since the last one"; a console keeps the last non-nil
value.

Why not `GitReport`: the git loop rebuilds it every 5 minutes from
sessions and config, and the Git tab draws it; found repos would have to be
copied into every report and filtered back out of the tab. Why not a new
kind: old consoles skip it safely, but the same data then needs a second
send path. Why not `state.Doc`: every snapshot, every 2 s, to every
console, would carry a few hundred repos, and state.json would grow with it.
Old consoles ignore the unknown `found` key.

## The data

In `internal/gitscan` (`find.go`), aliased in `proto` like `GitReport`:

```go
// Found is what a scan of the host found, and the consent it ran under.
type Found struct {
	Consent string        `json:"consent"`         // "" never asked, "yes", "no"
	Roots   []string      `json:"roots,omitempty"` // picked; empty: the defaults
	At      time.Time     `json:"at,omitempty"`    // the last scan's end; zero: none yet
	Took    time.Duration `json:"took_ns,omitempty"`
	Repos   []Checkout    `json:"repos,omitempty"`
	Errors  []string      `json:"errors,omitempty"`
}

// Checkout is one repository or linked worktree on the host.
type Checkout struct {
	Path   string    `json:"path"`
	Main   string    `json:"main,omitempty"`   // a linked worktree's main checkout
	Remote string    `json:"remote,omitempty"` // [git] remote's URL, else the first; no credentials
	Branch string    `json:"branch,omitempty"` // empty: detached
	Active time.Time `json:"active"`           // the last checkout or commit
}
```

What each consumer reads:

- #174: name and short path from `Path`, `Branch`, `Active` for "2h";
  worktrees indent under `Main`. "The last session started there" is #174's
  to add.
- #175: the most recent `Active` is "here" when the launch directory is not
  a repo.
- #176: `Remote` tells "already a clone of o/r"; a finished clone asks for a
  scan so the new repo joins.

How each field is read, without git:

- `.git` a directory: the main checkout; gitdir is `.git`. A file: a linked
  worktree; gitdir is its `gitdir:` line, `Main` the checkout that line
  points into (as `mainCheckout` does).
- `Branch`: `ref: refs/heads/<b>` in `<gitdir>/HEAD`.
- `Remote`: the `url` of `[remote "<name>"]` in the common gitdir's
  `config`, `<name>` being `[git] remote` (default `origin`), else the first
  remote.
- `Active`: the mtime of `<gitdir>/logs/HEAD` (git appends to it on every
  commit, checkout, merge and reset), else of `<gitdir>/HEAD`.

## The walk

- Each root is resolved once (`filepath.EvalSymlinks`, `~` expanded) and
  walked to its own depth: `~/dev/a/b/c/d` is depth 5 from `~` but 4 from
  `~/dev`. The deepest roots go first, and a directory already walked with
  as much depth left is not walked again, so overlapping roots cost
  nothing.
- Only the home is searched below its own `.git` (dotfiles kept in git);
  any other root that is a checkout, a configured repository say, is
  recorded and not entered, so its submodules and fixtures stay out.
- `os.ReadDir` only. An entry that is a symlink is never followed; an
  unreadable directory is skipped silently. A directory on another device
  than its root is not entered: a network mount may never answer.
- A `.git`, `HEAD`, `commondir` or `config` is read only when it is a
  regular file (or `.git` a directory), and 1 MiB of it at most: a pipe or
  a link to `/dev/zero` would hang or flood the walk.
- Bounds: 30 s and 100 000 directories per scan. Past either, the scan
  keeps what it found and says so in `Errors`. A read that blocks past the
  30 s and 5 s more is left to end on its own: the scan returns what it
  found and frees the next one.
- A remote URL keeps neither user nor password nor query, except an ssh
  URL's user.
- A walk is dropped if a no, or other roots, came while it ran.

## The consent box

A third question beside a pending step and the stoker's take-over, drawn
by `consentBox` with the same frame, padding and placement (DESIGN.md
"Consent"):

```
╭─ Find the Git repositories in your home folder? ──────╮
│                                                       │
│  they stay on this host and your paired devices       │
│  ~  ~/dev  ~/src  ~/code  ~/projects  ~/work          │
│                                                       │
╰─ y find ─ n never ─ p pick folders ─ esc later ───────╯
```

- It opens once the first snapshot with `Found.Consent == ""` arrives, and
  no form, peek or step is open.
- `y` sends the act `scan:yes`. `n` sends `scan:no`.
- `p` opens a form with one field, `folders`, the defaults in it, space
  separated; `⏎` sends `scan:yes` with the folders in `Text`, `esc` goes
  back to the box.
- `esc`, any other key, or a tap closes the box until the next console
  starts, and records nothing: a tap is never consent, and never a refusal.
- Another console's answer bumps `Found`; the box closes everywhere.

DESIGN.md's Consent entry gains the question and its keys.

## The acts

Handled in `internal/service/actions.go` `dispatch`, beside `rescan:git`:

- `scan:yes`: writes `scan.json` (`Text`, when set, is the picked roots,
  one per line), then scans now. The result returns when the scan ends,
  so `matchblox scan` can print `found 42 repositories in 1.3s`.
- `scan:no`: writes `scan.json` with `no`, deletes `repos.json`, and
  sends `Found` with no repos.

A console and the CLI both send them; neither is destructive, so no typed
`y` beyond the box's own key.

Who may say yes (call made for the owner): only a person. `scan:yes` is
refused from an agent's pane, the stoker's and the sessions it drives
included, with the rule `new:session` already has (`sessionAt(a.From)`
finds a session): `only you say yes to a scan, at a console or in your own
shell`. `scan:no` is taken from anyone: a refusal widens nothing.

Roots on a later yes (call made for the owner): a `scan:yes` with an empty
`Text` keeps the roots on record, so `matchblox scan` refreshes the walk the
builder agreed to and never widens it. The box's `y` on a host never asked
has none on record: the defaults. `matchblox scan <folder>...` and the
box's `p` replace them.

## Left to the builder

- Names of unexported helpers, and how the service loop shares a scan in
  flight with an act that asks for one.
- The exact `matchblox scan` output, within the tone of `matchblox status`.
- docs/config.md text for `scan_roots`; CHANGELOG entry.
