# Every host in one console: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One console holds every ticked host: one Sessions queue, rows that
name their host, acts that go to the row's host, `stale` in place, plus list
only (`v`) and the now line.

**Architecture:** Today's `Model` fields stay the **home host** (the first
ticked host in Hosts order), untouched, so one host draws exactly as today.
Each other ticked host is a **peer link**: its own conn, redial, backoff,
lost, blocked, hello and last `proto.State`. After any state, the console
rebuilds the merged lists; every session and queue item carries its host as
a console-only field. Every act takes the host from its row and goes on that
host's conn.

**Tech Stack:** Go 1.26, Bubble Tea v2 (`charm.land/bubbletea/v2`),
lipgloss v2.0.6 (`Canvas`, `CellAt`, `SetCell`), golden screens in
`testdata/screens/`.

**Spec:** `design/0017-every-host.md` (rulings R1 to R15). Read it first;
this plan argues from it.

## Global Constraints

- No change to any service, to `internal/proto`'s wire or to the gate. New
  fields on wire types are `json:"-"` (non-goal 2).
- With one host ticked, every existing golden screen is byte for byte
  unchanged (anti-goal 4, test 5 of the spec). Run `go test ./internal/app -run TestScreens` after every task.
- New or changed golden frames need the owner's yes before `-update`. Stop
  and ask.
- Copy is exact, from the spec: `3 hosts`, `· gpu-1 stale`, `· 1 hidden`,
  `gpu-1 is not connected. It tries again in 8s.`,
  `gpu-1: connection lost. Tries again in 8s.`,
  `2 hosts lost: gpu-1, ws-1. Tries again in 8s.`, `HOSTS · 3 · 2 SHOWN`,
  `hidden`, `stoker 2/3`, `stale · as of 14:02`, `list only v`,
  `full view v`, `list only`.
- Stale (R5): two or more hosts, and 30 s or more since that host's last
  `stateMsg`. One host keeps today's `m.lost && m.have`.
- Docs and UI words in ASD-STE100: one idea per sentence, active, present.
- Each commit: signed Conventional Commit, `(#198)` in the subject, no
  `--no-verify`, no WIP; grep the staged diff for the names in
  `~/.config/builder/denylist.txt` first. Never name another product as the
  source of an idea.
- Before the PR: `go test ./...` and `golangci-lint run`. A known flake
  (#232) gets one rerun.

## Review Focus

1. **Two hosts with the same pane id `%12` and the same PID.** A selection,
   a fold, a rename in flight or a peek must never cross hosts. Pinned in
   Task 3 (selection) and Task 4 (every act site).
2. **A peer dies while an act to it is in flight.** Its result never comes;
   a flash must not wait forever, and a later result from the redialed
   conn must not be taken for it. Pinned in Task 2.
3. **A host is ticked, then hidden, while its redial tick is pending.** The
   late tick must not reopen it. Pinned in Task 7.
4. **The home host is lost while peers are fine.** The other hosts' rows
   must keep working, and the screen must not turn into today's whole-screen
   `blockedText`. Pinned in Task 5.
5. **A terminal resize while the lanes show.** The now column moves with
   the width, and no line gets a stray `│`. Pinned in Task 11.

---

### Task 1: Hidden hosts on disk

**Files:**
- Modify: `internal/hosts/hosts.go`
- Test: `internal/hosts/hosts_test.go`

**Interfaces:**
- Produces: `HiddenPath() string` (`~/.config/matchblox/hidden`),
  `Hidden(path string) []string`, `Hide(path, host string) error`,
  `Show(path, host string) error`, and `const ThisMachine = "this machine"`.

- [ ] **Step 1: Write the failing tests**
  - `TestHideAddsOneLine`: a file with `# mine\nws-1\n`; `Hide(p, "gpu-1")`
    gives exactly `# mine\nws-1\ngpu-1\n`.
  - `TestShowRemovesOnlyThatLine`: `Show(p, "ws-1")` on the above keeps
    `# mine` and `gpu-1` byte for byte.
  - `TestHiddenReadsThisMachine`: the line `this machine` reads back as
    `ThisMachine`; `Hidden` of a missing file is `nil`, no error.
  - `TestHideTwiceIsOneLine`.
- [ ] **Step 2: Run them and see them fail:** `go test ./internal/hosts -run 'Hide|Show|Hidden'`.
- [ ] **Step 3: Implement** with the same add-or-remove-one-line rules as
  `Add` and `Remove` (share their helpers; R7). `ThisMachine` is accepted
  though `ValidHost` refuses it.
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit:** `feat: matchblox keeps the hosts you hide (#198)`.

### Task 2: Peer links

**Files:**
- Create: `internal/app/peers.go`
- Modify: `internal/app/model.go` (Options, Model, `update`)
- Test: `internal/app/peers_test.go`

**Interfaces:**
- Produces:
  ```go
  type Peer struct {
      Host    string
      Conn    transport.Conn
      Redial  func() (transport.Conn, error)
      DialErr error
  }
  // Options.Peers []Peer: the other ticked hosts, in Hosts order.

  type link struct {
      host      string
      conn      transport.Conn
      redial    func() (transport.Conn, error)
      lost, redialing, answered bool
      backoff   time.Duration
      blocked   error
      hello     proto.Hello
      at        time.Time   // the last stateMsg
      st        proto.State // the last state; History, Found, Used kept from the first
      have      bool
  }
  // Model.peers []link, copied on write (as peekSent is).
  type peerMsg struct { host string; conn transport.Conn; msg tea.Msg }
  type peerRedialMsg struct { host string; gen int }
  func (m Model) peerRecv(i int) tea.Cmd   // as recv, wraps in peerMsg
  func (m Model) peerAt(host string) int    // -1 when not a peer
  ```
- `Init` sends a hello and starts `peerRecv` for each peer with a conn; a
  peer with `DialErr` starts lost or blocked, as `Init` does for home.

- [ ] **Step 1: Write the failing tests** with two `fakeConn`s (home, `ws-1`):
  - `TestPeerStateIsKept`: a state on the peer conn sets `peers[0].st`,
    `have`, `at`; the home fields do not change.
  - `TestPeerLostRedialsOnItsOwn`: EOF on the peer gives `lost`, a backoff
    of 250 ms, and a `peerRedialMsg`; the home conn is not closed and home
    `m.lost` stays false.
  - `TestPeerBlockedStopsRedial`: a `remote.ErrNotConnected` on the peer
    sets `blocked` and schedules nothing.
  - `TestLateMessageOfAnOldPeerConnIsDropped`: after a redial, a state from
    the old conn changes nothing.
  - `TestPeerResultIsHandledAsHomeResult`: a `resultMsg` on the peer conn
    with a known act id ends that act (the same `update` path as home).
  - `TestResultOfAnActLostWithItsPeer` (Review Focus 2): an act sent to the
    peer, then EOF: the flash names the host and says the act has no answer;
    a later result with that id on the new conn is ignored.
- [ ] **Step 2: Run them and see them fail:** `go test ./internal/app -run Peer`.
- [ ] **Step 3: Implement.** Reuse the backoff formula and `blocking` of the
  home `lostMsg` case; do not duplicate the redial body: extract the part
  both need into one function on `*link`-like fields if it reads cleanly.
  Act ids stay one counter (`m.acts`) for every host, so a result id is
  unique in the console.
- [ ] **Step 4: Run them, then the whole package:** `go test ./internal/app`.
  Goldens unchanged.
- [ ] **Step 5: Commit:** `feat: a console holds a stream to each other host (#198)`.

### Task 3: One merged list, each row with its host

**Files:**
- Modify: `internal/sample/sampler.go` (`Session.Host string \`json:"-"\``),
  `internal/queue/queue.go` (`Item.Host string \`json:"-"\``),
  `internal/ships/ships.go` (`Ship.Host string \`json:"-"\``)
- Modify: `internal/app/model.go` (the `stateMsg` case, `selPID`,
  `keepSelection`, `selected`), `internal/app/lists.go`,
  `internal/app/sessions.go` (`waitOf`, `holdOf`, the reason and rename
  lookups, `childrenOf`)
- Test: `internal/app/merge_test.go`

**Interfaces:**
- Consumes: `Model.peers`, `link.st` (Task 2).
- Produces:
  - `func (m *Model) merge()`: builds `m.all` and the drawn copies
    (`m.snap.Sessions`, `m.snap.Children`, `m.queue`) from home and every
    peer with `have`, each item's `Host` set (home: `m.opt.Host`). Called at
    the end of the home `stateMsg` case and of a peer state. Not at a draw.
  - `func (m Model) hostCount() int`: home plus peers.
  - `Model.selHost string` beside `selPID`: the selection is (host, PID) (R4).
  - `func (m Model) stateOf(host string) *proto.State`: the last state of
    that host (home: a view of today's fields), for lookups keyed by pane
    (`Holds`, `Reasons`, `Renames`, `SignIns`).
  - `func (m Model) machineOf(host string) sample.Machine` and
    `limitsOf(host string) []limits.Account`, for Task 8.
- With one host, `merge` must give exactly today's lists (no copies made
  when there are no peers).

- [ ] **Step 1: Write the failing tests:**
  - `TestPeerSessionsJoinTheList`: home has `api-auth`, `ws-1` has
    `train-loop`; `m.all.sessions` has both, with `Host` `""` and `ws-1`.
  - `TestAWaitOnAPeerSortsAboveHomeWork`: `train-loop` waits on the peer;
    it is the first row of WAITS FOR YOU, above home's working rows.
  - `TestSamePaneOnTwoHostsAreTwoRows` (Review Focus 1): both hosts have
    `%12` and PID 4242; both rows show; selecting the peer's and receiving a
    new home state keeps the peer's row selected.
  - `TestLookupsByPaneStayOnTheirHost`: a hold on home `%12` does not mark
    the peer's `%12`; the same for a reason and a rename.
  - `TestOneHostMergeIsToday`: with no peers, `m.all` equals what today's
    code built for `fixtureState()` (compare to a copy taken before merge).
- [ ] **Step 2: Run them and see them fail:** `go test ./internal/app -run 'Merge|Peer|SamePane|Lookups|OneHost'`.
- [ ] **Step 3: Implement.** Ties inside a group break by host, in Hosts
  order (home first). Every lookup that keys by pane goes through
  `stateOf(s.Host)`.
- [ ] **Step 4: Run the package; goldens unchanged.**
- [ ] **Step 5: Commit:** `feat: sessions lists the agents of every host in one queue (#198)`.

### Task 4: Acts go to the row's host

**Files:**
- Modify: `internal/app/model.go` (`send`, the generic act at the primary
  and secondary, park), `lifeform.go` (kill, rename, switch, switch-preview,
  new:session), `peek.go` (answer, message, pick), `tabs.go`, `remote.go`
  (`forHost`, `jumpOnHost`), `ghost.go`, `wherelist.go`
- Test: `internal/app/routing_hosts_test.go`

**Interfaces:**
- Consumes: `Session.Host`, `peerAt`, `link.conn` (Tasks 2, 3).
- Produces:
  - `func (m *Model) sendTo(host string, a proto.Act) tea.Cmd`: picks the
    home conn when `host == m.opt.Host`, else that peer's. `send(a)` stays
    as `sendTo(m.opt.Host, a)` for the acts that are not a row's (doors,
    scan, clones, recs, pairing: they stay the home host's in v1, see Calls).
  - `func (m Model) refusal(host string) string`: `""` when the host can
    take an act; else `<host> is not connected. It tries again in <backoff>.`
  - `func (m Model) forHost(host string, ...)` and `jumpOnHost(host, pane)`:
    the host is a parameter, never `m.opt.Host`.
  - The ghost row and `n`: the host of the selected row, else home; the
    where list reads that host's `Found` and `Used` (`stateOf(host)`), and
    its label names the host when `hostCount() > 1`.

- [ ] **Step 1: Write the failing tests.** Two hosts, both with `%12`; one
  table test, a case per act site. Each selects the peer's row, presses the
  key (and `y` where it confirms), and asserts the act is in the peer
  `fakeConn`'s `acts()` with the same `RecID` as today (`kill:%12`, …) and
  not in home's: kill, rename, switch, switch-preview, new:session, park,
  answer, message, pick, the generic primary. Then:
  - `TestJumpToAPeerRowUsesThatHost`: `Enter` on the peer row builds
    `remote.JumpSSH` with `Host: "ws-1"`.
  - `TestActOnAStaleRowIsRefused`: the peer lost for 31 s; `x` sends nothing
    and the flash is `ws-1 is not connected. It tries again in 1s.` (the
    backoff at that moment).
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** Thread the row's host through each site;
  remove every read of `m.opt.Host` that serves a row (`grep -n 'm.opt.Host' internal/app/*.go` is the checklist).
- [ ] **Step 4: Run the package; goldens unchanged.**
- [ ] **Step 5: Commit:** `feat: each act goes to its row's own host (#198)`.

### Task 5: The host word, stale and blocked

**Files:**
- Modify: `internal/app/sessions.go` (`rowIn`, `taggedName`, `waitCell`,
  `rowWord`), `view.go` (`header`, `drawScreen`, the key line)
- Create: `internal/app/hostrows.go` (stale and blocked words and rows)
- Test: `internal/app/hostrows_test.go`, `screens_test.go` (new frames)

**Interfaces:**
- Consumes: `hostCount`, `link.at`, `link.lost`, `link.blocked`.
- Produces:
  - `func (m Model) stale(host string) bool` (R5).
  - `func (m Model) lostLine() string`: the key-line text for lost hosts.
  - The header says `<n> hosts` in place of a host name when
    `hostCount() > 1`.
  - A stale row shows `as of HH:MM` (the host's last state) where the plan
    column is, when the row has that column.
  - The host word: muted, after the name and before the account, only when
    `hostCount() > 1`; the account goes first when the row is short, then
    the host is cut with `…` (R11).
  - A blocked host (home or peer, when `hostCount() > 1`) is one row at the
    end of Sessions: `<host>  <today's blocked words>  ⏎ <today's fix>`;
    `Enter` on it runs today's fix action for that host.

- [ ] **Step 1: Write the failing tests:**
  - `TestHostWordOnlyWithTwoHosts`, `TestHeaderSaysThreeHosts`.
  - `TestStaleRowSaysAsOf`.
  - `TestStaleStartsAt30s`: lost at t; at t+29 s no frame differs from
    t−1 s; at t+30 s the peer rows say `stale`, are faint, keep their group,
    and the header ends with `· ws-1 stale`.
  - `TestStaleAgeTurnsAmberAtAMinute`.
  - `TestTwoLostHostsLine`: the key line is exactly
    `2 hosts lost: gpu-1, ws-1. Tries again in 8s.`
  - `TestHomeLostPeersWork` (Review Focus 4): home lost and blocked, the
    peer fine: no whole-screen `blockedText`, one blocked row, the peer's
    rows act.
  - `TestOneHostStaleIsToday`: one host, lost: the header says `stale` at
    once, as today.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** The 30 s needs a tick: one `tea.Tick` per lost
  peer at `at+30s`, dropped when the peer comes back.
- [ ] **Step 4: Run the package.** Old goldens unchanged.
- [ ] **Step 5: Ask the owner** to approve new frames `sessions3-{44x31,86x30,132x32}`, `stale3-86x30`, `blocked3-86x30`; then
  `go test ./internal/app -run TestScreens -update` for those only.
- [ ] **Step 6: Commit:** `feat: rows name their host, and a lost host says stale in place (#198)`.

### Task 6: Login, the key ladder and the stoker per host

**Files:**
- Create: `internal/app/hostlogin.go`
- Modify: `login.go`, `ladder.go`, `askfield.go`, `connectask.go`,
  `remote.go`, `stoker.go`, `stokertab.go`, `model.go` (the login and
  take-over fields)
- Test: `internal/app/hostlogin_test.go`, `stoker_hosts_test.go`

**Interfaces:**
- Produces:
  - `type hostLogin struct` holding today's per-host login fields: `login`,
    `checking`, `checked`, `jumpLater`, `opening`, `notNow`, `checkStop`,
    `checkCancelled`, `tailscale`, `loginURL`, `note`. `Model.logins
    map[string]hostLogin`, copied on write; home uses `logins[m.opt.Host]`.
  - The ask field holds one prompt; `Model.askQueue []askMsg` holds the
    next; the field's title names the host when `hostCount() > 1` (R13).
  - The background check runs at open for each ticked remote host and never
    prompts.
  - The stoker take-over (`held`, `stoker:rest`, `touchRest`, `s`) reads the
    host of the row acted on (R14). `stokerClaims(host string) bool`.
- One host: every field reads as today.

- [ ] **Step 1: Write the failing tests:**
  - `TestTwoHostsPromptOneAtATime`: jumps to `ws-1` and `gpu-1` rows both
    get a password prompt; the field shows `ws-1`'s, names it, and shows
    `gpu-1`'s only after the first is answered.
  - `TestChecksAtOpenNeverPrompt`.
  - `TestLadderIsPerHost`: `notNow` on `ws-1` still offers the key on
    `gpu-1`.
  - `TestStokerRestIsThatHostsOnly`: the stoker runs on `ws-1` and `gpu-1`;
    a key on a `ws-1` row asks the take-over question; `y` sends
    `stoker:rest` on `ws-1`'s conn only.
  - `TestStokerBadgeCounts`: `stoker 2/3`.
  - `TestStokerTabSectionPerHost`: one section per host with a stoker view,
    headed by its name, in Hosts order.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** Move the fields into `hostLogin` mechanically
  first (package green, goldens unchanged), then add the second host.
- [ ] **Step 4: Run the package.**
- [ ] **Step 5: Commit:** `feat: each host keeps its own ssh login and stoker in one console (#198)`.

### Task 7: Hosts as the tick list, and the shell keeps the streams

**Files:**
- Modify: `internal/app/layers.go`, `keys.go` (`shellKeys.Tick` on
  `space`), `cmd/matchblox/main.go` (ShellOptions, the start, R8)
- Test: `internal/app/layers_test.go` (or the file that tests `Shell`),
  `cmd/matchblox/main_test.go` if the start has one

**Interfaces:**
- Consumes: `hosts.Hidden`, `Hide`, `Show`, `ThisMachine` (Task 1);
  `Options.Peers` (Task 2).
- Produces:
  - `ShellOptions.Hidden func() []string`, `Hide func(string) error`,
    `Show func(string) error`.
  - `func (s Shell) ticked() []string`: `""` for this machine, then the
    added hosts, minus the hidden, in Hosts order.
  - `func (s Shell) openTicked(first bool) (Shell, tea.Cmd)`: home = the
    first ticked host; peers = the rest, each from `s.opt.Open(host)`.
  - `func (m Model) reconcile(peers []Peer) (Model, tea.Cmd)`: keeps the
    links whose host stays, closes the dropped ones (and drops their
    pending ticks), opens the new (R12).
  - `back()` keeps `s.cur` and its streams; `Enter` calls `reconcile` when
    the home host is unchanged, else opens a new console (see Calls).
- Hosts rows: a tick (`●` ticked, `○` hidden), the live note
  `<n> sessions · <n> waits` of a ticked host, `hidden` for a hidden one,
  the label `HOSTS · <all> · <ticked> SHOWN`, the key line
  `show/hide space  open ⏎  connect c  quit q`.
- R8: `matchblox` with added hosts calls `openTicked`; `matchblox ws-1`
  stays `Start: "ws-1"`, one host.
- The header's hidden count (R6): `· <n> hidden` when any host is hidden.

- [ ] **Step 1: Write the failing tests:**
  - `TestSpaceHidesAndShows`: `space` on `gpu-1` calls `Hide("gpu-1")` and
    the row turns `○ … hidden`.
  - `TestEnterOpensTheTickedHosts`: ticked `this machine`, `ws-1`: home is
    `""`, one peer `ws-1`.
  - `TestEscKeepsTheStreams`: `esc` then `Enter` with no tick change: no
    conn is closed or dialed, the selection is kept.
  - `TestHidingAPeerClosesItAndItsTick` (Review Focus 3): a peer lost with a
    pending `peerRedialMsg`; hide it and `Enter`; the late tick dials
    nothing.
  - `TestHeaderCountsHidden`.
  - `TestStartOpensTheTickedHosts` (R8).
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the package and `go test ./cmd/matchblox`.**
- [ ] **Step 5: Ask the owner** to approve `hoststicks-{44x31,86x30}`; update those frames only.
- [ ] **Step 6: Commit:** `feat: hosts is the list of hosts you tick into one console (#198)`.

### Task 8: Ships, the dock, Machine, History, Git and Panes across hosts

**Files:**
- Modify: `internal/app/ships.go`, `dock.go`, `panels.go`, `panels2.go`,
  `tabs.go` (Git, Panes, History counts)
- Test: `internal/app/tabs_hosts_test.go`

**Interfaces:**
- Consumes: `stateOf`, `machineOf`, `limitsOf`, `Ship.Host` (Task 3).
- Produces:
  - Ships: every host's ships, each with `Host`; `driver` and `shipsOf`
    match within one host only.
  - Git and Panes: merged rows with the host word when `hostCount() > 1`.
  - Machine and History: one section per host, headed by its name, in
    Hosts order; a stale host's section and dock line say
    `stale · as of HH:MM`.
  - Usage: rows per (host, account), the same account label together, each
    with its host word; never merged or summed (R10).

- [ ] **Step 1: Write the failing tests:**
  - `TestShipsOfEveryHost`, `TestDriverMatchesWithinItsHost` (two hosts,
    the same session id: each ship's driver is its own host's).
  - `TestUsageRowsPerHostNeverMerged`: `work` on two hosts gives two rows,
    side by side.
  - `TestDockMachineLinePerHost`, `TestStaleMachineLine`.
  - `TestMachineTabSectionsInHostsOrder`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the package; old goldens unchanged.**
- [ ] **Step 5: Ask the owner** to approve `ships3-132x32`, `machine3-104x38`; update those only.
- [ ] **Step 6: Commit:** `feat: ships, usage and the machine show every host (#198)`.

### Task 9: List only

**Files:**
- Create: `internal/app/listonly.go`
- Modify: `keys.go` (`mainKeys.ListOnly` on `v`), `view.go`
  (`drawScreen`, `panes`, the key line), `model.go`
- Test: `internal/app/listonly_test.go`

**Interfaces:**
- Produces:
  - `type viewPrefs struct{ ListOnlyPhone, ListOnlyDesk bool }`, kept as
    `view.json` in `Options.StateDir` (in memory when it is `""`), read in
    `New`. The phone default is true (R9): a missing file means
    `{ListOnlyPhone: true}`.
  - `func (m Model) listOnly() bool`: the flag of the current shape class:
    phone when `m.upright()`, else desk.
  - In list only: `panes` gives `detailW = 0`, no dock, and the selected row
    opens in place (the `Narrow` path); `a` opens the answer box over the
    list.
  - Key line: `list only v` on the full view, `full view v` in list only,
    and `list only` at its right edge while on; the word is a tap target.

- [ ] **Step 1: Write the failing tests:**
  - `TestVFoldsThePaneAndTheDock` at 92×40 landscape (desk class).
  - `TestVFlipsOnlyTheCurrentShape`: flip at desk; resize to portrait; the
    phone flag is unchanged.
  - `TestListOnlyIsKeptAcrossARestart` (a temp `StateDir`).
  - `TestPhoneStartsListOnly`: a portrait 96×60 screen with no file opens
    with no pane.
  - `TestWaitsStillShowInListOnly`: the header keeps `2 wait`.
  - `TestVIsFree`: `TestKeymapsHaveNoCollisions` passes with the new key.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the package.** The phone default changes today's
  portrait frames that split (`*-*` with rows ≥ cols/2 and cols ≥ 90). List
  them.
- [ ] **Step 5: Ask the owner** to approve those changed frames and the new
  `listonly-{92x40,132x32}`; update those only.
- [ ] **Step 6: Commit:** `feat: list only folds the pane and the dock, by default on a phone (#198)`.

### Task 10: The lane history

**Files:**
- Create: `internal/app/lanes.go`
- Test: `internal/app/lanes_test.go`

**Interfaces:**
- Produces:
  ```go
  const laneCell = 5 * time.Minute
  const lanePast, laneAhead = 24, 12
  type laneState uint8 // laneNone, laneIdle, laneWork, laneWait (most urgent wins)
  type laneKey struct{ host string; pid int }
  // Model.lanes map[laneKey][lanePast]laneState, with laneStart time.Time:
  // the cell boundary the ring is aligned to. Copied on write.
  func (m *Model) noteLanes(now time.Time) // from m.all.sessions, after merge
  func (m Model) laneOf(s sample.Session, now time.Time) [lanePast]laneState
  ```
- The state of a session in a cell: `groupWaits` → wait, `groupWorking` →
  work, else idle. A cell the console never saw is `laneNone`. A session
  gone for one full window is dropped from the map.

- [ ] **Step 1: Write the failing tests:**
  - `TestLaneCellTakesTheMostUrgent`: work then wait in one cell → wait.
  - `TestLaneBeforeOpenIsBlank`: every cell before the first state is
    `laneNone` (non-goal 7).
  - `TestLaneShifts`: 7 minutes later the ring moved one cell.
  - `TestLaneStopsAtAStaleHost`: a peer without a state for 12 minutes: its
    last two cells are `laneNone`.
  - `TestLaneIsPerHost`: the same PID on two hosts keeps two rings.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement**, called at the end of `merge`.
- [ ] **Step 4: Run the package.**
- [ ] **Step 5: Commit:** `feat: the console keeps two hours of each session's state (#198)`.

### Task 11: The now line

**Files:**
- Create: `internal/app/nowline.go`
- Modify: `sessions.go` (`sessTracks`, `rowIn`: the lane column),
  `view.go` (the axis lines, the overlay at the end of the frame)
- Test: `internal/app/nowline_test.go`

**Interfaces:**
- Consumes: `laneOf` (Task 10), `limitsOf` and the wait cell's reset time.
- Produces:
  - `tracks.lane int`: `lanePast + 1 + laneAhead` cells plus room for a
    label, given only when the list is 107 columns or wider; dropped before
    the plan and the ship when the name would get under 12.
  - `func laneCells(l [lanePast]laneState, reset time.Time, now time.Time) string`:
    `━` work (muted), `━` wait (warn), `╌` idle (faint), blank none, `‹`
    faint when the session started before the window; the now cell blank;
    then `┄` (faint) to the reset of a limit, and ` <n>h resets` (muted).
  - Two lines above the list: the axis `−2h  −1h  now  +1h` and `┊` ticks at
    −2h and −1h.
  - `func overlayNow(frame string, x, top, bottom int, st styles) string`:
    draws `frame` on a `lipgloss.NewCanvas(w, h)`, then for each line from
    `top` to `bottom` sets the cell at `x`: blank → `│`; `─` → `┬` on `top`
    and `┴` on `bottom`; `━` → `┿`; `╌` → `┼`; `┊` → `│`; any other glyph
    keeps itself. Every changed cell takes the rule's colour and keeps its
    background. Applied in `render` when the lanes show.
- The rule's colour: the accent, unless the night ink reads better in both
  themes (left to the builder by the spec).

- [ ] **Step 1: Write the failing tests:**
  - `TestNowRuleIsOneColumn`: at 132 columns, every line from the top
    hairline to the bottom one has the rule glyph at the same cell `x`, and
    no other line has one.
  - `TestNowRuleKeepsTheSelectedWash`: the cell at `x` on the selected row
    keeps the wash background.
  - `TestNowRuleCrossesBars`: a working row has `┿` at `x` when its last
    cell is work.
  - `TestNoLaneUnder107`: at 106 columns of list, no lane and no rule.
  - `TestResizeMovesTheRule` (Review Focus 5): 132 then 120 columns: the
    rule is at the new `x` only.
  - `TestLimitRunsToItsReset`: a session on a limit that resets in 35
    minutes has seven `┄` and ` 5h resets`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the package.** Today's frames at 107 columns and wider
  change (the lanes show). List them.
- [ ] **Step 5: Ask the owner** to approve those frames and `nowline-132x32`; update those only. Check the rule on the phone's terminal
  (SwiftTerm) on a device or the simulator: no gaps between rows. Write
  down the result in the PR.
- [ ] **Step 6: Commit:** `feat: sessions draws each row on one time scale with a line for now (#198)`.

### Task 12: Docs and the changelog

**Files:**
- Modify: `docs/remote.md` ("Open the console of a host" becomes "See every
  host in one console"), `docs/phone.md` (list only; the phone shows what
  its host's console can open, R15), `CHANGELOG.md` `[Unreleased]`,
  `design/0005-remote-mode.md` §5 (point to 0017), `design/0017-every-host.md`
  (Status: built, and a "Changed while building" list if anything changed)
- Test: `internal/docscheck` and `internal/app/docs_test.go` (they run in
  `go test ./...`)

- [ ] **Step 1: Write the docs** in ASD-STE100, with the exact words of the
  screens.
- [ ] **Step 2: Run** `go test ./internal/docscheck ./internal/app -run Docs`. Expected: PASS.
- [ ] **Step 3: Run the gates:** `go test ./...` and `golangci-lint run`.
  Expected: PASS (rerun a #232 flake once).
- [ ] **Step 4: Commit:** `docs: every host in one console, list only and the now line (#198)`.

## Calls made in this plan

Each is mine; the owner can overturn it.

1. **Home and peers.** Today's fields stay the home host's, and other hosts
   are peer links. This keeps one host byte for byte (anti-goal 4) and
   avoids a rewrite of 90 fields.
2. **The home host is the first ticked host.** When the tick set changes
   the home host, `Enter` opens a new console, and the lanes start again.
   When the home host stays, `reconcile` keeps everything (R12).
3. **Recs, doors, clones, the scan question and pairing stay the home
   host's in v1.** The spec names Sessions, Ships, Git, Panes, Machine,
   History, Stoker and the dock. Recs and doors are not waits.
4. **`Host` is a console-only field** (`json:"-"`) on `sample.Session`,
   `queue.Item` and `ships.Ship`. A service never sets it.
5. **List only is kept in `view.json`**, not in the recs' prefs file. It is
   a different thing.
