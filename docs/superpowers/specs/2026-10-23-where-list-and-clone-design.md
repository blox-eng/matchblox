# N's where: a list of the builder's repos, and a git URL that clones (#174, #176)

Status: for approval (2026-10-24). One pull request for both issues.
The issues hold the original spec and Done-when lists; this document
records what the brainstorm changed and decided, and wins where they
differ.

## Intent

N must feel like the continuation of the builder's thought. They press N
and the repo they meant is the first row, or three letters away. If the
repo is not on the host yet, they paste its URL and matchblox clones it
and starts there. They never leave the console to `git clone`, and they
never hunt with ← → through repos in an order unrelated to their work.

Inspiration (the builder liked these): the Where picker of a desktop agent app's New
session page (Suggested first, then all by name; one ranked list once you
type) and its branch tree (branches under their repo with tree lines);
a terminal agent multiplexer's "follow" (a new workspace starts where you are) and "an open
checkout is focused, never opened twice".

## Goals

1. `where` opens a list of repos on focus, with no extra key.
2. The most likely repo is the first row: ranked by recent use.
3. Typing filters with a fuzzy match on name and path.
4. Worktrees show under their repo as a tree.
5. A pasted git URL clones into `clone_root` and starts the session there,
   with every folder conflict handled and nothing ever overwritten.
6. The clone runs in the service (the phone and the CLI get it too).

## Non-goals

- Finding repos on the host (#173, merged): this uses its list.
- The `account` field (#170, in flight on `feat/170-accounts`). It must be
  able to reuse the same list widget later; do not build it here.
- Forking, picking a remote branch to check out, hosts other than GitHub
  for the `o/r` shorthand, shallow clones.
- Stars or pins on repos.

## Anti-goals (a failure even if it works)

- A local path that the builder typed becomes a clone.
- A clone overwrites or writes into any folder that exists.
- ← → cycling stops working for a builder who uses it today.
- A match glyph (`✦` `╿` `│`) on a repo row: DESIGN.md §4 rule 2 keeps it
  for one agent session.
- A spinner (DESIGN.md §5: there are none).
- The list moves the screen while the builder types (no jump of the box
  height on every key: the list area keeps its height while it is open).

## Constraints

- Keys are `bkey.Binding` values in `internal/app/keys.go`; the router is
  `focus(k)`; hints and footers come from the bindings (`hintFor` /
  `footFor`). `TestNoKeyStringLiterals` and `TestKeymapsHaveNoCollisions`
  stay green. `docs/keys.md` names every new or changed key.
- DESIGN.md for colours, glyphs, row anatomy and voice (ASD-STE100, the
  house voice in the issues). A change to a glyph or token changes
  DESIGN.md in the same pull request.
- `matchblox new --where <path>` keeps working unchanged. An old client
  that sends a path keeps working with a new service.
- Commits signed and conventional, ending with the Co-Authored-By line.

## What it looks like (decided)

Layout **C**: while `where` has focus, the list fills the form box. The
other fields fold to one faint line, `then  branch · name · agent <agent> ·
first`, and come back when the builder picks a row or leaves `where`.

### 1. On focus, nothing typed

```
╭─ New session ─────────────────────────────────────────────────╮
│ where  › a repo, a path, or a git URL▏                        │
│                                                               │
│ SUGGESTED                                                     │
│▌matchblox        ~/dev                main        now  2 live │
│  ├─ feat/174     .wt/matchblox-174    #201        2h   1 live │
│  ╰─ fix/185      .wt/matchblox-185    #198        1d          │
│  shop-api        ~/dev                dev         40m  1 live │
│  notes           ~/src                main        1h          │
│                                                               │
│ ALL · 52 · by name                                            │
│  advent-2025     ~/src/fun            main        11mo        │
│  …                                                            │
│                                                               │
│ then  branch · name · agent claude · first                    │
╰─ ⏎ pick · ↑↓ move · ←→ repo · tab next · esc keep the path ──╯
```

- `SUGGESTED`: five repos at most, in this order: a repo with a live
  session, then the last time a matchblox session started there, then
  the repo's own last activity (`Checkout.Active`), then by name.
- `ALL · N · by name`: every known repo, by name. A repo in SUGGESTED is
  not repeated here.
- Labels are `faint`, uppercase, like the Sessions group labels.
- Each repo row: the selection mark `▌` (on `wash`), name (`fg`), short
  path (`muted`, `~`-shortened, the repo's parent folder), current branch
  (`muted`; `detached` when none), how long ago (`muted`, the console's
  existing age format: `now`, `40m`, `2h`, `3d`, `2w`, `11mo`), and `N
  live` (`faint`) when sessions run there.
- **Worktrees always show under their repo** as a tree, `├─` and `╰─` in
  `faint`. A worktree row puts its branch first (that is what the builder
  looks for), then its folder (short), its pull request number when one is
  known (`#201`), how long ago, and `N live`. No fold and unfold: ← → keep
  their meaning (below).
- The list scrolls inside the box; the box keeps one height while open.
- "Known repos" are the Git tab's repos and worktrees, #173's found
  checkouts, and the repos in matchblox's own record of session starts
  and clones (Recency, below), each path once. The record makes a repo
  known with no scan consent: it is matchblox's own use, not a scan.
- **What N opens on** (*my call*): what `openNew` fills today (the
  selected session's `Cwd`, the selected Git tab worktree, else the first
  known repo) becomes the **selected row**, not text. The text stays
  empty, so SUGGESTED shows. N on a session still means "start here":
  N ⏎ ⏎ starts a new session next to it. A pre-filled folder that is not
  a known repo shows as the first row of SUGGESTED.
- **Empty states:** no repos known: `no repos known yet · type a path, or
  paste a git URL`, and, when the scan was never asked, the scan's own key
  hint (`s finds your repos`, from its binding).

### 2. Typing

```
│ where  › mbx▏                                                 │
│                                                               │
│▌matchblox        ~/dev                main        now  2 live │
│  mbxctl          ~/src/tools          dev         2w          │
│  matchblox-site  ~/src                main        3d          │
│  3 of 52                                                      │
```

- One flat ranked list. No group labels.
- Fuzzy subsequence match on name and path. Word starts, path-segment
  starts and runs of letters score higher; recency only breaks near-ties
  (a much better text match always wins). `mbx` finds
  `~/dev/matchblox`.
- Matched letters are marked: `fg` bold with an underline (the builder
  checks this against the replay's token rule; if underline is not
  allowed, bold alone).
- A worktree matches on its branch and path; when it matches, it shows
  under its repo with the tree line.
- `N of M` in `faint` under the list.
- No match: `no repo matches "xyz" · ⏎ uses it as a path` in `muted`.
  ⏎ then sends the text as a path, as today.

### 3. A git URL

```
│ where  › https://github.com/o/r▏                              │
│                                                               │
│▌↓ clone o/r into ~/src/r                                      │
```

See "Clone" below for every case of the row.

## Keys (decided)

While `where` has focus and the list is open:

| Key | Does |
|---|---|
| typing | filters |
| `↑` `↓` | move through the rows (repos and worktrees) |
| `←` `→` | step to the previous / next **repo** among the rows on screen, as today, and select it; `where` shows its path but the filter keeps what was typed (*my call*: text set by ← → is a selection, not a query) |
| `⏎` | picks the row: `where` is set, the list closes, focus goes to `branch`. On a clone row it starts the clone (below) |
| `tab` | picks the selected row too, and goes to the next field |
| `shift+tab` | the field before (none today: stays) |
| `esc` | closes the list and keeps the typed text as a path; a second `esc` closes the form, as today |
| `⌫` | erases, as today |

Notes:

- Today ↓ and ↑ are also "next / previous field" in the form. While the
  list is open on `where` they move in the list; on every other field
  they keep their meaning. The router decides this in the form's layer.
- `←` `→` cycle repos in the same order as the list (SUGGESTED, then ALL),
  not the old Git-tab order. This keeps cycling for those who use it.
- Focus back on `where` (shift+tab, a tap) opens the list again, filtered
  by the current text.
- A tap on a row picks it (the form already maps taps to lines).
- `docs/keys.md` "New session, Rename and Switch agent" changes to say all
  of this.

## Clone (#176, decided)

### What counts as a git URL

- `https://github.com/o/r`, with or without `.git` (and any https host:
  the target is `<clone_root>/<r>`).
- `git@host:o/r.git`, `ssh://…`.
- `gh:o/r` and `o/r`: GitHub shorthand. It expands to the protocol
  `gh config get git_protocol` names, else https (*my call*). The auth
  failure names the fix for that protocol.
- **A local path never becomes a clone** (*my call* on how):
  - In the console, `where` refuses relative paths today, so `o/r` (one
    slash, no leading `~`, `.` or `/`) is always the shorthand.
  - In the CLI, `--where o/r` is always a folder, joined to the directory
    the CLI runs in (as today): a typo never clones. The CLI clones a URL
    with a scheme, `user@host:path`, or `gh:o/r` (builder's call, D2).
- A paste shows the clone row on the same key, before any check.
  matchblox never reads the clipboard by itself.

### The target folder

- `clone_root` is a new config key, default `~/src`. **Builder's call:
  where it lives** in `config.toml` (likely `[git] clone_root`, next to
  `scan_roots`); document it in the config docs.
- Target: `<clone_root>/<r>`. `clone_root` is made if it does not exist.

### The cases of the clone row (decided by the service, shown before ⏎)

| On disk | The rows | ⏎ on a row |
|---|---|---|
| a known repo anywhere has the same remote | `start in ~/dev/r (already a clone of o/r)`, selected, above the clone row | picks that path, as any repo row |
| the target is the same repo | `start in ~/src/r (already a clone of o/r)`, selected, then `clone anew into ~/src/r-ember` | picks the path / starts the clone anew |
| nothing at the target | `clone o/r into ~/src/r` | starts the clone |
| another repo, a non-repo folder, or a file at the target | `clone o/r into ~/src/r-ember (~/src/r is another repo)` | starts the clone into the free name |

- **The same repo** (*my call*): the remote URLs match after normalising
  (https vs ssh, `.git`, the scheme's own port, case of the host; case of
  owner and repo only on github.com, gitlab.com and bitbucket.org, which
  ignore it: another server may hold `team/Repo` and `team/repo` as two
  repos). The check is offline:
  it reads the remotes on disk and the found list's `Remote`, never the
  network. (A root-commit check would need a fetch: dropped.)
- No `c` key (*my call*): "start there" and "clone anew" are two rows,
  picked with ↑↓ ⏎, because every printable key on `where` is text.
- **A free name**: `<r>-<word>`, the word picked at random from the words
  whose folder is free: `ember flint spark cinder tinder wick flare char
  soot coal`. When every word is taken: `<r>-<word>-<4 chars>`, the 4
  chars random from `[a-z0-9]` (`r-ember-k7f2`). The random source is
  injected so tests are deterministic.
- Nothing is written to the target name until the builder presses ⏎, and
  the name shown is the name used. **Builder's call:** how to hold the
  name between the plan and the clone (the service re-checks at clone
  time; if the name was taken meanwhile, it says so and offers the next
  one, never writes into it).

### ⏎ on a clone row: the clone runs while the builder fills the rest (decided)

- ⏎ on a clone row **starts the clone at once**, in the service, and
  focus goes to `branch`, as after any pick. The list closes.
- The `where` line shows `cloning o/r… 42%` (`muted`), from git's
  progress, while the builder fills branch, name, agent and first. No
  spinner; the number changes. Before git gives a percent: `cloning
  o/r…`.
- The final ⏎ starts the session. If the clone still runs, the line says
  `starts when the clone is done`, and the session starts by itself when
  it ends, with the fields as they were at that ⏎. So N, paste, ⏎, ⏎ is
  the whole flow.
- `esc` (in any field) stops the clone and deletes the partial folder
  (only the folder this clone made); the form stays open with the URL in
  `where`. A second `esc` closes the form. Closing the form, or editing
  `where` to something else, stops the clone the same way.
- A full clone (no `--depth`).

### When it ends

- Success: the session starts there exactly as any N session does
  (branch, name, agent, first all apply; a branch makes a worktree after
  the clone). The new repo goes into matchblox's own record (Recency),
  so it is a known repo and in SUGGESTED next time, with or without scan
  consent. A clone never writes `repos.json` without consent.
- Failure: said in the form, in plain words, and nothing starts:
  - no access or no auth: `no access to o/r: run gh auth login, or add an
    SSH key` (name the fix that fits the URL's scheme),
  - not found: `o/r is not on github.com`,
  - no network: `no network: github.com does not answer`,
  - disk full: `the disk is full: o/r is not cloned`.
  The partial folder is deleted in each case.

### Where it runs

- The service clones (it owns the host). The console never runs git for
  this.
- `matchblox new --where <url>` does the same from the CLI: plan, clone,
  start. With an "already a clone" target the CLI starts there (the
  builder's usual wish) and says so; **builder's call** whether a flag
  (`--fresh`) asks for a clone anew.

## Recency: the data it needs

- "The last time a matchblox session started there" is not recorded
  today. The service records it when `newSession` succeeds: the repo's
  path (the main checkout for a worktree, and the worktree itself) and
  the time, kept in the state directory next to `repos.json`, capped
  (for example the newest 200). A clone adds its folder too. It is
  written with or without scan consent (it is matchblox's own use);
  `scan off` does not forget it.
- It reaches the console with the found list or beside it in
  `proto.State`. **Builder's call:** the exact shape, as long as an old
  console ignores it and a service without a scan still sends it.
- "Live" counts come from the sessions the console already has (their
  `Cwd`), matched to the repo or worktree path. No new wire data.
- Pull request numbers for worktree rows come from what the Git tab
  already knows. When none is known, the cell is empty.

## Wire (the parts decided)

- `proto.NewSession.Where` may hold a git URL. The service recognises it
  (the issue allows either this or a new field; this keeps old clients
  working with no change). **Builder's call** whether a separate
  "plan a clone" act carries the row's text and the free name before ⏎,
  and how clone progress reaches the console (a new optional field in
  `proto.State` fits how `Found` travels; acts today answer once).
- Cancel is an act from the console (esc) and a context cancel in the CLI.

## Left to the builder

- The fuzzy scorer's exact weights (meet: `mbx` → matchblox first among
  50+ repos; a three-letter query narrows to the one wanted).
- The list widget's code shape. It should be one widget the `account`
  field (#170) can reuse.
- How the router splits ↑↓ between list and fields (one place, in the
  form's layer, from bindings).
- Tests: unit tests for URL recognition, target naming, same-repo check,
  ranking; replay frames for the open list, a filtered list, a clone row,
  each conflict row, the progress line and each failure; a service test
  that a clone never writes into an existing folder.
- Docs: `docs/keys.md`, the config docs for `clone_root`, README/landing
  where N is described, the changelog. DESIGN.md if the matched-letter
  style or the list labels add a rule.

## Done when

From #174:

- With 50+ known repos the most recently used repo is the first row, and
  three typed letters narrow the list to the one wanted.
- An empty filter result says so and offers the typed text as a path.
- `matchblox new --where <path>` works with no change.
- ← → still step through repos.

From #176:

- Pasting a GitHub URL into N clones the repo and starts a session there,
  with no other step.
- Pasting the URL of a repo already cloned at the target offers the
  existing checkout and never clones twice.
- A name collision with another repo never overwrites anything, and the
  new folder name is shown before the clone starts.
- An old client still works.

And: `TestNoKeyStringLiterals`, `TestKeymapsHaveNoCollisions` and the
replay tests pass; `docs/keys.md` names the changed ↑↓ ← → ⏎ tab esc.
