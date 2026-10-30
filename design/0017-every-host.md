# Every host in one console: design

Date: 2026-10-29. Status: built. Issue: #198. Related: #96, #15.
Mockups: the brainstorm page of 2026-10-29 (frames for Hosts, Sessions,
stale, the phone, the dock, the now line and list only).

## Intent

A builder runs agents on the laptop and on two hosts (`ws-1`, `gpu-1`).
Today the console shows one host per screen: `esc` goes back to Hosts and
`Enter` opens another. An agent that waits on `gpu-1` stays unseen while the
builder looks at `ws-1`.

After this change one console holds every host the builder ticks. Sessions
is one queue, grouped by need as today, and a wait on any host is at the
top. Each row names its host. Each act goes to the row's own host. A lost
host's rows stay where they are and say `stale`, and the other hosts keep
going.

The promise, as one line: **no agent waits unseen.** The test of it: an
agent on host B starts to wait, and its row is at the top of the console on
the laptop within one snapshot of B (5 s).

## Goals

1. Sessions lists the sessions of every ticked host in one queue, in the
   groups and the sort of today.
2. A row names its host when two or more hosts are ticked. With one host
   ticked, every screen is exactly today's.
3. Each host connects, reconnects and fails on its own. A lost host's rows
   keep their last state and say `stale`. A host that cannot connect says
   why and shows its fix, as today, in one row.
4. Every act on a row (`a`, `Enter`, `x`, `r`, park, pick, the peek) goes to
   the row's own host through that host's service.
5. Ships, the dock and the Machine tab show every ticked host.
6. Hosts is the tick list: `space` shows or hides a host, `Enter` opens the
   console of the ticked hosts.
7. **List only** (`v`): the detail pane and the dock fold, and the list
   takes the screen. It is the default on a phone shape.
8. **The now line**: at 107 columns and wider, each session row has a lane
   on one shared time scale, and one rule for now runs down through the
   whole list.

## Non-goals

1. **The phone's combined list** (ruling 2026-10-29). The phone attaches to
   one host's console. A combined list there needs that host to hold a
   matchblox key to the other hosts, so one host becomes a hub. That
   changes what a matchblox key can reach (0005 §4 A01, docs/remote.md
   "Keep the key private"). It gets its own design. In v1 matchblox sets up
   no host-to-host key; the phone sees what its host's console can open
   (R15), with list only.
2. No change to any service and no change to the wire. A host that runs an
   older matchblox works in the combined console.
3. No merge in the service, and no new process (approaches B and C below).
4. No filter inside Sessions that hides a host. Which hosts show is set on
   Hosts only.
5. No `try now` key. The console retries by itself, with the backoff of
   today.
6. No key that turns the lanes off. They show when there is room.
7. No state history in a service. The console fills the lanes from the
   snapshots it receives.
8. No containers or cloud hosts (#15), and no other tmux servers (#96).

## Anti-goals

The change fails, even when it works, if:

1. A wait on a lost host disappears from view: a lost host's waiting rows
   stay at the top. A hidden host is never silent either: the header says
   how many hosts are hidden.
2. An act on a row reaches another host than the row's. Two hosts have
   panes with the same id (`%12`) all the time.
3. A matchblox key reaches a host that it does not reach today.
4. A one-host builder sees any change.
5. A reconnect of two seconds flashes the screen.

## Rulings

Each one is dated, with its reason. "Mine" marks a call the builder (the
agent) made; the person can overturn it.

| # | Ruling | Why |
|---|---|---|
| R1 | The merge is in the console: one stream per ticked host (approach A). Owner, 2026-10-29. | Each service stays one host, no key reaches further, and a row's host is plain data that every act carries. |
| R2 | The phone's combined list is not in v1 (non-goal 1). Owner, 2026-10-29. | It is a one-way door for security; the UI parts are two-way doors. |
| R3 | List only and the now line are in v1. Owner, 2026-10-29 ("LGTM" to "both"). | Both serve "no agent waits unseen" on small and large screens. |
| R4 | A row's identity is (host, pane) on screen, and (host, PID) for the selection. Mine. | Pane ids repeat across hosts; PIDs too. |
| R5 | With two or more hosts, `stale` shows from 30 s after a host's last snapshot. One host keeps today's rule. Mine. | A short reconnect must not flash (anti-goal 5), and a one-host builder sees no change (anti-goal 4). |
| R6 | A hidden host is not connected. The header says `· 1 hidden`, with no count of its waits. Mine. | A tick decides which streams exist, so hiding a slow or sleeping host stops its redials. A count from a stream that is closed would be an invented number. |
| R7 | Hidden hosts are kept in `~/.config/matchblox/hidden`, one per line; the line `this machine` hides the local host. Mine. | The hosts file keeps its bytes (0005 §2). A new host shows by default. `this machine` has a space, so no ssh host has that name. |
| R8 | `matchblox` with added hosts opens the console of the ticked hosts, not Hosts. `matchblox ws-1` opens `ws-1` alone, as today. Mine. | The combined console is the place to start; Hosts is one `esc` away. |
| R9 | List only is the default on a phone shape (portrait) and off on a desk; `v` flips it for the current shape class, and the flip is kept in the state directory, as folded recs are. Mine. | The laptop and the phone can share one console (the shared tmux session), so one switch for both would fight. |
| R10 | Usage rows are per (host, account). Rows with the same account label sit together, each with its host word. They are never merged or added. Mine. | Nothing on the wire proves that two hosts' `default` accounts are one login, so a merge could join two limits into one wrong row. |
| R11 | A long host name is cut with `…` on a row; the detail shows it whole. Mine. | No new setting. |
| R12 | `Shell` owns the streams, not the console. `esc` to Hosts keeps them open. `Enter` dials the newly ticked hosts, closes the newly hidden ones and keeps the rest, with their lanes, selection and backoff. Mine. | The Hosts notes are live, and a trip to Hosts must not wipe the lanes or replay the splash. |
| R13 | The ssh login of each remote host is its own: its master, its background check, its key ladder and its Tailscale link. The checks run for each ticked host at open and never prompt. A prompt comes only at a jump. One field shows at a time, names its host, and the next waits. Mine. | Two hosts must not race for one field, and opening three hosts must not ask three questions. |
| R14 | The stoker is per host. A key on a row asks about that row's host's stoker, and `y` rests that one only. A touch goes to the host acted on. `f` starts the stoker of the selected row's host. Mine. | A stoker owns one host. Resting one that the builder did not touch would be a surprise. |
| R15 | A phone sees what its host's console can open, as today. If the builder connected `ws-1` to `gpu-1` themselves, a phone paired to `ws-1` sees and acts on `gpu-1` in the combined console. v1 never sets up host-to-host keys. Owner, 2026-10-29. | It widens no key: `esc` → Hosts → `gpu-1` from that phone reaches the same host today. Non-goal 1 is about matchblox making a hub, not about one the builder made. |

## The experience

### Hosts

```
 ▰ matchblox · hosts
──────────────────────────────────────────────────────────────
 HOSTS · 3 · 2 SHOWN
 ▌ ● this machine          3 sessions
   ● ws-1                  5 sessions · 1 waits
   ○ gpu-1                 hidden · 2 sessions · 1 waits
     + add a host
──────────────────────────────────────────────────────────────
 show/hide space  open ⏎  connect c  quit q
```

- `space` flips the tick of the selected host and writes `hidden` (R7).
- `Enter` opens one console over the ticked hosts. With none ticked, it
  opens the selected host alone and says so in the key line.
- The note of a ticked host is live (R12): its count of sessions and of
  waits. A hidden host is not connected (R6), so its note is `hidden` and
  nothing else: never an invented zero.
- The rows of today stay: `+ add a host`, the first-run question, the
  Tailscale warning.

### Sessions

```
 ▰ matchblox · 3 hosts                         2 wait · train-loop 4m · stoker
──────────────────────────────────────────────────────────────────────────────
 WAITS FOR YOU · 2
 ▌ train-loop gpu-1         waits    4m   62%  ▲ clear         #211 ▰▰▰▱
   api-auth ws-1 · work     asks     1m   31%                  #244 ▰▰▱▱
 WORKING · 4
   docs-sweep ws-1          edits         18%  ▰▰▰▱▱
```

- The header says `3 hosts` instead of a host name when two or more are
  ticked. It names a lost host: `· gpu-1 stale`.
- The host is a muted word after the name, before the account
  (`ws-1 · work`). When the row has no room, the account goes first, then
  the host is cut (R11).
- The groups and the sort are today's, across hosts. The order of a group
  breaks ties by host, in Hosts order.
- The ghost row and `N` start a session on the host of the selected row,
  or on this machine when no row is selected. The where list of `N` shows
  that host's repos and names the host in its label.

### A host is lost

```
 ▰ matchblox · 3 hosts · gpu-1 stale                          1 wait · stoker
──────────────────────────────────────────────────────────────────────────────
 WAITS FOR YOU · 2
 ▌ train-loop gpu-1         stale   40s   62%  as of 14:02
   api-auth ws-1 · work     asks     1m   31%
──────────────────────────────────────────────────────────────────────────────
 gpu-1: connection lost. Tries again in 8s.
```

- The rows of the lost host keep their place and their last state. Their
  text is faint, the word is `stale` and the wait cell is the age since the
  host last spoke: faint under a minute, amber from a minute.
- The word `stale` and the dimming start at 30 s (R5). Under 30 s nothing
  changes.
- An act on a stale row is refused in the console:
  `gpu-1 is not connected. It tries again in 8s.`
- The key line names the lost host and its next try while any host is
  lost. Two lost hosts: `2 hosts lost: gpu-1, ws-1. Tries again in 8s.`
- A host that cannot connect at all (not connected, not installed, an
  older matchblox, a host key unknown or changed: the `blocked` errors of
  today) has no rows to keep. It shows one row at the end of the list, with
  today's words and fix: `gpu-1  is not connected  ⏎ connect`.

### Other tabs

- A tab that lists things (Ships, Git, Panes) merges the hosts' rows and
  names each row's host, as Sessions does. A ship's driver carries its
  host.
- A tab about one machine (Machine, History, Stoker) shows one section for
  each ticked host, in Hosts order, headed by the host's name.
- The stoker stays one per host (R14). The header badge counts the hosts
  where it runs: `stoker 2/3`.
- The dock: usage per (host, account), the same account label together
  (R10), and one machine line per host. A lost host's machine line says `stale · as of 14:02`.

### List only

- `v` folds the detail pane and the dock. The list takes the full width
  and height and gets the columns of that width.
- The selected row opens in place under itself, as on an upright phone.
  `a` opens the answer box over the list.
- The header keeps who waits. A lost host still takes the key line.
- The key line says `list only v` on the full view and `full view v` in
  list only, and `list only` stays at its right edge while it is on. On the
  phone, the word is a tap target.
- On an upright phone there is no pane, so `v` folds the one-panel dock.
- R9: the default is on for a phone shape; `v` flips the current shape
  class.

### The now line

```
─────────────────────────────────────────────────────────────────────┬────────────
                                             −2h         −1h         │ now   +1h
                                             ┊           ┊           │
 WAITS FOR YOU · 2                                                   │
 ▌ train-loop gpu-1      stale    12m   62%   ━━━━━━━━━━━━━━━━━━━━   │
   api-auth ws-1         asks      1m   31%                  ━━━━━━━━│
 QUIET · 3                                                           │
   bench ws-1            limit  15:40   21%      ━━━━━━━━━━━╌╌╌╌╌╌╌╌╌│┄┄┄┄┄┄┄ 5h resets
─────────────────────────────────────────────────────────────────────┴────────────
```

- From 107 columns of list (a desk, or list only on a wide screen), each
  session row has a lane: 2 hours back and 1 hour ahead, 5 minutes a cell.
- A cell is the most urgent state the console saw in its 5 minutes: a wait
  (amber `━`), work (`━`), or idle (faint `╌`). A cell the console did not
  see is blank. The lane never invents a past (non-goal 7): it fills from
  the moment the console opens.
- To the right of now, a session on a limit runs dotted (`┄`) to the reset
  of its window. Nothing else has a known future time today; a hold has no
  end time.
- One rule for now runs down from the top hairline to the bottom one, at
  the same column, through every line: the axis, the group labels, the
  blank rows and the selected row's wash. It meets the hairlines as `┬` and
  `┴`.
- A stale host's bars stop where it last spoke, short of the line, at the
  same place on each of its rows.

## How it is built

### The console holds one stream per host

- `Model` holds a list of hosts in Hosts order. Each has what one host has
  today: its `conn`, `Redial`, `Gen`, `lost`, `backoff`, `redialing`,
  `blocked`, its `Hello`, the time of its last `stateMsg`, and the state
  that `stateMsg` unpacks (`snap`, `queue`, `recs`, `git`, `shipReport`,
  `stoker`, `holds`, `reasons`, `renames`, `accounts`, the history and the
  rest).
- Every message from a connection already carries its conn (`fromConn`).
  It now also carries its host index, and each loop (recv, redial, the
  silence timer) runs per host.
- `m.all` (the lists the tabs draw) is built from every host's state after
  each `stateMsg`, not at each draw. Each item knows its host.
- `Shell` owns the per-host streams (R12). `Shell.open` takes a list of
  hosts and reconciles: it dials the new, closes the hidden and keeps the
  rest. One host is today's path.
- The login machinery is per host (R13): `login.go`, `ladder.go`,
  `askfield.go`, `connectask.go` and `jumpLater` read the row's host, not
  `m.opt.Host`. The field holds one prompt at a time, with a queue.
- The stoker take-over (`held`, `stoker:rest`, `touchRest`, `s`) is per
  host (R14).
- `cmd/matchblox` opens each host with `openHost`, as today. `ensureService`
  stays the local path.

### A row's identity

- Each list item carries its host (an index into the host list, or the
  host's name). The selection is (host, PID) (R4).
- `send(a proto.Act)` becomes `send(host, a)`: it picks that host's conn.
  The act's `RecID` (`kill:%12`, `answer:%12`) is unchanged: the host's
  service reads it as today.
- Every place that sends an act or opens a jump takes the host from the
  row, never from `m.opt.Host`: `lifeform.go` (kill, rename, switch,
  switch-preview, new:session), `model.go` (park, the generic primary and
  secondary), `peek.go` (answer, message, pick), `tabs.go` and
  `app/remote.go` (`forHost`, `jumpOnHost`). A test fails if any of them
  reads `m.opt.Host` for a row's act.
- A result comes back on the conn that sent the act, so it belongs to that
  host.
- The stoker merge guard (`service/driven.go` `owns()`) and every match of
  ships by session id run inside one host's service. They never see two
  hosts, so they do not change. The console's own matches (`proto.ShipsOf`,
  `proto.ShipDrives` in `app/ships.go`) match only within one host.

### Stale and blocked

- `stale` is per host: lost, and 30 s or more since its last `stateMsg`
  (R5). The header, the rows, the dock and the key line read it.
- `blocked` is per host. A blocked host is a row, not the whole screen.
  The whole-screen `blockedText` stays for one host.

### Hosts and ticks

- `internal/hosts` gets `Hidden(path)`, `Hide(path, host)` and
  `Show(path, host)` on `~/.config/matchblox/hidden`, with the same rules as
  the hosts file: add or remove one line, never rewrite.
- The Hosts rows of today get a tick and a live note.

### List only

- A model field per shape class (phone, desk), kept in the state
  directory, as the folds of the recs are (R9).
- `drawScreen` takes `detailW = 0` and no dock when list only is on, and
  the list opens the selected row in place, as `Narrow` does.

### The now line

- The console keeps, per (host, session), a ring of 24 cells of 5 minutes,
  filled from each `stateMsg`. It costs nothing on any host.
- The lane is a column of the row, as the plan and the ship are; `sessTracks`
  gives it room from 107 columns of list.
- The rule is drawn last, over the screen. lipgloss v2 (in `go.mod`) has a
  `Canvas` with `CellAt` and `SetCell`. The console draws the screen onto a
  canvas, then walks the now column from the top hairline to the bottom one
  and sets each cell: a blank to `│`, a hairline to `┬` or `┴`, `━` to `┿`,
  `╌` to `┼`; a letter keeps its glyph and takes the line's colour; every
  cell keeps its background.
- The phone's terminal (SwiftTerm) must draw `│` from edge to edge of the
  cell, so the rule has no gaps. Check it on the device before the PR.

## Approaches

| | Where the merge is | For | Against |
|---|---|---|---|
| **A (picked)** | The console holds one stream per ticked host. | No service or wire change; older hosts work; each service stays one host; the stoker guard is unchanged; a row's host is plain data. | `Model` changes most: its per-host state becomes a list. |
| B | The local service dials each host and serves one merged `State`. | The console barely changes; any client of the local service gets every host. | A service holds remote keys and reaches other machines for the first time (the door R2 closed); it rewrites every id and act; ssh failures in a service started with no terminal have nobody to show them to. |
| C | A merge layer between the connections and an unchanged `Model`. | A small `Model` diff. | Every act is translated twice; the selection and the guards see synthetic ids. Two names for one row is the bug factory. |

## Evidence: the tests

Each is a test, not a claim.

1. Two fake hosts whose sessions share a pane id (`%12`): `x`, `a`, `r`,
   `Enter` and the peek on each row send on that row's host only. One test
   per act site.
2. A wait on the second host sorts above work on the first.
3. Two hosts, one lost: under 30 s no frame changes; from 30 s its rows say
   `stale`, keep their group, and refuse acts with the words above; the
   other host's rows act as normal.
4. One host blocked: one row with today's words and fix; `Enter` runs the
   fix; the other host works.
5. One host ticked: every golden screen of today is unchanged (anti-goal 4).
6. A hidden host: it has no stream, and the header says `· 1 hidden`.
7. `hidden`: hide, show and read keep the other lines and comments
   byte for byte; the line `this machine` hides the local host.
8. Usage: an account label on two hosts gives two rows side by side, never
   merged.
12. Two hosts both need a prompt at a jump: one field at a time, each named
    with its host.
13. A stoker key on a `ws-1` row rests `ws-1`'s stoker only.
14. `esc` to Hosts and back keeps the streams, the lanes and the selection.
9. List only: `v` folds the pane and the dock and flips only the current
   shape class; it is kept across a restart.
10. The now line: the rule is at one column on every line from hairline to
    hairline; it keeps the selected row's background; a cell before the
    console opened is blank; a stale host's bars stop at its last snapshot.
11. A reconnect: each host redials on its own backoff; a late message of a
    closed conn is dropped (the gen checks of today, per host).

New golden screens: Sessions with three hosts (44, 86, 132 columns), a
stale host, a blocked host, Hosts with ticks, list only, and the now line
at 132 columns. Regenerating goldens needs the owner's yes.

## Docs and changelog

- `docs/remote.md`: "Open the console of a host" becomes "See every host
  in one console": ticks, the host word, `stale`, acts by the row's host.
- `docs/phone.md`: list only, and that the phone shows one host.
- CHANGELOG `[Unreleased]`: the combined console, list only, the now line.
- 0005 §5 "A queue merged across hosts" points here.

## Not in v1

- The phone's combined list through a hub host: its own design (R2).
- A future for a hold, if a hold gets an end time.
- `matchblox --host X …` for scripts.

## Left to the builder

- The shape of the per-host state inside `Model` (a slice of a struct is
  the obvious one), and how `m.all` items carry their host.
- The colour of the now rule (the accent, or the night ink of the stoker),
  inside the theme.
- The exact width at which the lane drops when the name would get under
  12 columns, by the rule `sessTracks` has today.

## Changed while building (2026-10-29)

Each item changes the text above. The change wins.

- **R10 (owner).** Usage is one row for each account label. Each host's
  reading is a short cell on that row, in Hosts order:
  `work  5h  ws-1 58% · gpu-1 61%`. A reading is never merged, added or
  averaged. A short line keeps its window and drops whole cells, with ` …`.
- **The home host's word.** The home host's row word is its own name from
  its hello. `this machine` shows only before the hello.
- **A lost peer.** It sets no flash before 30 s. The key line copy shows
  once the host is stale. Acts lost with a host show as a toast:
  `ws-1: 1 act has no answer`.
- **Acts and lost hosts.** The console refuses an act whenever its host is
  lost, for any time. `stale` is display only. While the first dial of a
  peer runs, the refusal is `<host> is not connected.`, with no "tries
  again" part. A jump to a lost host is not refused: it uses the builder's
  own ssh, not the stream.
- **Dials.** Peers dial in the background, so a slow host never stalls the
  console. A remote home host (this machine hidden) still dials as one
  host does today.
- **Connect from Hosts.** It reopens the ticked hosts. With none ticked,
  `Enter` opens the selected host alone, and the key line says
  `No host is ticked: <host> opens alone.`
- **Git.** The host is on each repository heading, not on each worktree
  row.
- **The stoker.** The dock's claim stays the home host's. The Stoker tab
  shows a peer's section for reading only: take-over, rest, touch and `f`
  are per host.
- **Not per host in v1.** Recs, doors, clones, the scan question and
  pairing stay the home host's.
- **List only, the key line.** On the full view, `list only v` is the first
  hint to drop. In list only, `full view v` stays: a row's own keys outrank
  it on a narrow row. The mark goes first, then the age.
- **The now line.** The now column is its own column, right of the 24 past
  cells. With ships read, the lane drops before the ship. With no ships, the
  place drops first, then the lane, then the plan. The console does not
  draw the rule while a peek, a sheet, a form, a consent or an ask box is
  up. The rule passes the opened row's own lines (what the agent said, its
  keys): drawn through them, it turned a space between two words into `│`
  (review of the PR, 2026-10-30).
- **Where the lanes show.** They show only where the list is 107 columns
  or wider: in list only on a desk (press `v`), or on a very wide screen.
  The list of the full view is at most 78 columns.
- **A lane's key.** A lane is keyed by host, pid and the start of the
  process, so a new process on an old pid starts empty.
- **The site's phone demo.** It starts in the full view, so its dock story
  still plays.
- **Key names.** The stoker key is `f`, not `s`: `s` sorts. The New
  session key is `N`, not `n`: `n` opens the night sheet. R14 and
  §Sessions use the correct keys now.
- **Older hosts (non-goal 2).** An older matchblox works only at the
  console's protocol version. A host at another protocol version is a
  blocked row: `is version N, the console is version M  ⏎ connect`. A
  newer host shows `update the console` and has no `⏎`.
- **A silent host.** A peer that connects and sends no state in 3 s is a
  row with today's words:
  `ws-1  does not answer  run ssh ws-1 matchblox status to see why`. Its
  first state removes the row. `Enter` on the row shows the same words.
- **Try again.** `⏎ try again` and `⏎ connect` dial the host again from
  any link, also a link that Hosts opened. A host that cannot dial again
  keeps its row, and the key line says `ws-1 cannot try again from this
  console`.
