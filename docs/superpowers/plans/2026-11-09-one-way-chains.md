# One-way chains Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One command joins a team of matches. A match can be in several teams. A send to a mate is queued, never refused for the mate's state. Chains and sending reach panes only through a small mux interface.

**Architecture:** `internal/chains` turns from "chains plus requests" into named teams. The store keeps members per team, sender-tagged inbox items and unseen times. `internal/service` runs the join, leave and owner-removal acts, the inbox-first send and the pane-lives rule. A new `internal/mux` package holds the four pane operations that chains and sending need: type, press keys, read screens, tell copy mode. tmux implements it, and a fake implements it for tests. The console reads the new `state.ChainsView` for the fuse, the `· also in` suffix and the join toast.

**Tech Stack:** Go, Bubble Tea and lipgloss (internal/app), tmux.

**Spec:** `docs/superpowers/specs/2026-11-08-one-way-chains-design.md` (approved 2026-11-09). Builds on the uncommitted #333 fix on branch `chain`.

## Global Constraints

- A chain is a team. All members hear all members. Two teams that share a match do not hear each other.
- One command joins. No requests, no request TTL and no "asks to chain with you" notice remain.
- A team takes the name of the match that started it. A clash among this host's teams gets `-2`, `-3`. The name stays when that match leaves.
- The stoker is in no team. It joins no team and nobody joins it.
- The fuse keeps its column between the match and the name. Nothing about chains moves to the right of the row, except the faint `· also in <team>` suffix (D2).
- Inbox: `chains.InboxMax` = 20 items.
- Copy follows the spec character for character. Each exact string in this plan comes from the spec, or is marked **(call)**.
- Commits: each commit is a signed Conventional Commit. Never use `--no-verify`. Grep the staged diff against `~/.config/builder/denylist.txt` before each commit.
- No sibling product names in code, docs or commits.
- Before each commit: `go vet ./...`, `go test ./...` and `golangci-lint run` give 0 issues.

## Review Focus

1. **A store saved by an older build** (with `requests` and chains that have no `name`) loads. Each chain gets a name, and old requests are dropped without an error. → Task 1, `TestLoadOldStoreNamesChains`.
2. **A match in two teams sends to a mate it shares only one team with.** The send is allowed, and the news goes only to that team. → Task 4, `TestSendToAMateInOneOfTwoTeams`.
3. **A queued line whose mate leaves the team before delivery.** The sender hears `<mate> did not get your line: you are no longer chained`, and no `got your line` follows. → Task 4, `TestQueuedLineToAMateThatLeftIsDropped`.
4. **A sample that read no panes** (tmux error). Nobody leaves and no unseen time starts. → Task 5, `TestSampleMissKeepsTeamsAndTimes`.
5. **A team name that is also a match name.** `matchblox chain <name>` joins the team. → Task 2, `TestChainNameJoinsTheTeamBeforeTheMatch`.

---

## Calls made in this plan (for the owner to check)

1. **D5 (team of one with a live plan) moves to #304.** No team has a stoker or a plan today (#304 is open). A seam with no caller is code nobody runs. This plan keeps today's rule: a team of one ends. Task 7 posts D5 on #304.
2. **D9's `u` key binds in the room (#326).** `u` is the dock key in Sessions today. This plan ships the act and the confirm text. matchblox-ux binds `u` (and `m`) in the room.
3. **"First team" for D2 is store order.** The store keeps teams in the order they formed. A shared match draws in the first team in that order that holds it. This differs from "joined first" only when a match joins an older team after a newer one.
4. **The off-refusal names the real flag:** `It can run: matchblox chain --on`. The spec's `matchblox chain on` is not a command.
5. **`(in <repo>)` on the join notice shows only when the repos differ.** This is what every other chain news line does (`elsewhereLocked`).
6. **No CLI wrapper for D11.** Every answer already starts with its outcome. The terminal wraps.
7. **A new team toasts too:** `<team> formed: <a>, <b>`. The spec covers a join to an existing team only.
8. **The sender lookup in `internal/service/peer.go` stays on tmux.** It finds who sends, not what a pane shows. It moves with #200 step one. The no-tmux guard covers the chain and send files listed in Task 3.

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/chains/chains.go` | Teams, names, members, moves, unseen times, inbox | 1, 4, 5 |
| `internal/service/chains.go` | Join, leave and owner-removal acts and their answers; prune and refresh | 2, 5 |
| `internal/service/notice.go` | News delivery; the request notice is removed | 2, 4 |
| `internal/service/chainnews.go` | Fan-out to the mates of every team of a match | 1 |
| `internal/mux/mux.go` (new) | The `Mux` interface | 3 |
| `internal/mux/tmux.go` (new) | The tmux backend | 3 |
| `internal/mux/muxtest/fake.go` (new) | The fake backend for tests | 3 |
| `internal/service/peersend.go` | The inbox-first send, D3, D4, D7 | 3, 4 |
| `internal/state/state.go` | `ChainsView`: teams and mate data for the room | 1, 5 |
| `cmd/matchblox/cli.go` | `chain`, `unchain` and the team listing | 2 |
| `internal/app/fuse.go`, `internal/app/sessions.go`, `internal/app/match.go` | Shared-match fuse, `· also in`, the join toast, the out act | 6 |
| `docs/agents.md`, `docs/console.md`, `CHANGELOG.md` | Docs | 7 |

---

### Task 1: Teams in the store

**Files:**
- Modify: `internal/chains/chains.go`, `internal/service/chainnews.go:258-282`, `internal/service/notice.go`, `internal/state/state.go:84-88`
- Test: `internal/chains/chains_test.go`

**Interfaces:**
- Produces:
  - `type Chain struct { ID, Name, Repo string; Members []Member; Spark time.Time }`. `Name` is json `"name"`. `ID` stays the stable key for sparks.
  - `var ErrOff, ErrSelf, ErrIn = errors.New(...)`
  - `func (s *Store) Form(a, b Member, name, repo string) (Chain, error)`: a new team `[a, b]`. Its name is `name`, made unique with `-2`, `-3`. Gives ErrOff when a or b is off, and ErrSelf when a is the same as b.
  - `func (s *Store) Join(m Member, team string) (Chain, error)`: adds m to the team. Gives ErrOff when m is off. Gives ErrIn, with the team, when m is in it already.
  - `func (s *Store) LeaveTeam(m Member, team string) bool`: a team of one ends.
  - `func (s *Store) Leave(m Member)`: leaves every team and drops m's inbox (as today).
  - `func (s *Store) TeamsOf(m Member) []Chain`: the teams in store order.
  - `func (s *Store) Team(name string) (Chain, bool)`
  - `func (s *Store) Shared(a, b Member) (Chain, bool)`: the first team that holds both.
  - `func (s *Store) Mates(m Member) []Member`: the union over m's teams, without duplicates.
  - `Prune` returns one `Left{Who, Chain: team ID}` for each team a gone member was in.
  - Removed: `Request`, `RequestTTL`, `Store.Requests`, `Ask`, `merge`, `side`, `ChainOf`. `ChainsView.Requests` goes too.

- [ ] **Step 1: Write the failing tests** in `internal/chains/chains_test.go`. Replace the `Ask` tests.

```go
func TestFormNamesTheTeamAfterItsStarter(t *testing.T) {
	var s Store
	c, err := s.Form(a, b, "hearth", "/r")
	if err != nil || c.Name != "hearth" || len(c.Members) != 2 { t.Fatalf("%+v %v", c, err) }
	c2, _ := s.Form(c3, d, "hearth", "/r")
	if c2.Name != "hearth-2" { t.Fatalf("clash: %q", c2.Name) }
}
func TestJoinAddsToOneTeamOnly(t *testing.T) {
	var s Store
	s.Form(a, b, "hearth", "")
	s.Form(b, c3, "web", "")          // b in two teams
	if _, err := s.Join(d, "web"); err != nil { t.Fatal(err) }
	if got := len(s.TeamsOf(b)); got != 2 { t.Fatalf("b teams %d", got) }
	if slices.ContainsFunc(s.Mates(a), d.Same) { t.Fatal("hearth hears web's d") }
}
func TestJoinRefusesOffAndAlreadyIn(t *testing.T)   // ErrOff for an off m; ErrIn for a second Join
func TestFormRefusesOffSelf(t *testing.T)           // ErrOff when either is off; ErrSelf for Form(a, a)
func TestLeaveTeamEndsATeamOfOne(t *testing.T)      // Form(a,b,"h"); LeaveTeam(a,"h") → no team "h"
func TestNameStaysWhenTheStarterLeaves(t *testing.T) // Form(a,b,"h"); Join(c,"h"); LeaveTeam(a,"h") → Team("h") still exists with b, c
func TestPruneLeavesEveryTeam(t *testing.T)         // b in hearth and web; live(b)=false → two Left, one per team ID
func TestLoadOldStoreNamesChains(t *testing.T)      // a file with "requests":[...] and a chain with no "name" loads with no error; Requests are gone; Name == "" (the service names it, Task 2)
```

- [ ] **Step 2: Run the tests and check that they fail.**
Run: `go test ./internal/chains`
Expected: build failure: `s.Form undefined`.

- [ ] **Step 3: Implement the interfaces above in `internal/chains/chains.go`.** `newID` stays. `holds` drops its request clause. `Refresh` no longer walks requests. Update the package doc: "the teams of matches that hear each other, and an inbox of news for each match".

- [ ] **Step 4: Move the callers to the new API.**
  - `chainnews.go postLocked`: for each `from` member, loop `st.TeamsOf(m)`. Spark each team, and post to the members of that team only.
  - `chainnews.go:43` and `:146`: `len(st.TeamsOf(me)) > 0`.
  - `notice.go`: delete `requestsLocked`, `heldAsks`, `heardAsks`, `delivery.asks`, and the `asked` clause of `noticesLocked`.
  - `notice.go clearedLocked`: list `st.Mates`. The line ends `To leave, run: matchblox unchain` (unchanged).
  - `service/chains.go`: `chainsView` copies `Chains` only. `pruneChainsLocked` drops its request counts. `markClearedLocked` tests `len(st.TeamsOf(m)) > 0`.
  - `peersend.go mateChain`: `st.Shared(from, to)`.
  - Keep the build green. Task 2 rewrites `changeChains`; for now its default case calls `Form` or `Join`.

- [ ] **Step 5: Run the tests and check that they pass.**
Run: `go test ./internal/chains ./internal/service`
Expected: PASS. The service request tests are deleted with their code: `TestRequest*` and the `asks to chain` cases in `notice_test.go`.

- [ ] **Step 6: Commit.**
`git commit -S -m "feat(chains): a chain is a named team, and a match may be in several"`

---

### Task 2: Join, leave and owner-removal acts and their answers

**Files:**
- Modify: `internal/service/chains.go:38-118`, `cmd/matchblox/cli.go:817-963`, `internal/proto/proto.go:154-157`
- Test: `internal/service/chains_test.go`, `cmd/matchblox/cli_test.go`

**Interfaces:**
- Consumes: Task 1's `Form`, `Join`, `LeaveTeam`, `TeamsOf`, `Team`, `ErrOff`, `ErrIn`.
- Produces: the act words after `proto.ChainPrefix` (`chain:`). Task 6 uses `out:`.

| Act | Meaning | Who may send it |
|---|---|---|
| `chain:<pane>` | join the team of the match in pane, or form one with it | an agent's pane |
| `chain:@<team>` | join the team by name | an agent's pane |
| `chain:-` | leave the only team | an agent's pane |
| `chain:-@<team>` | leave that team | an agent's pane |
| `chain:-*` | leave every team | an agent's pane |
| `chain:out:<pane>@<team>` | take that match out of that team | the console only: `a.Console` set and `a.From` is no agent session |
| `chain:off`, `chain:on` | unchanged | an agent's pane |

Exact answers (the `Note`, or a refusal `Err`):

| Case | Text |
|---|---|
| joined or formed | `joined <team>: <N> matches. To leave it, run: matchblox unchain <team>` |
| who is in 2+ teams | `<who> is in <K> chains. Run one:\n  matchblox chain <t1>\n  matchblox chain <t2>` |
| bare unchain in 2+ | `you are in <t1> and <t2>. Run one:\n  matchblox unchain <t1>\n  matchblox unchain <t2>\n  matchblox unchain --all`. With 3+ teams the list reads `a, b and c`. |
| off (either side) | `<name> is off: it takes no chains. It can run: matchblox chain --on` **(call 4)** |
| already in | `already in <team>` **(call)** |
| left | `left <team>`. For `-*`: `left <t1> and <t2>` |
| in no team | `not in a chain` (unchanged) |
| to or from the stoker | `the stoker joins no chain` **(call)** |
| owner removal: console answer | `took <name> out of <team>` **(call)** |
| owner removal: the match hears | `[matchblox] the owner took you out of <team>` |
| each mate hears on a join | `[matchblox] <name> joined the chain` plus `elsewhereLocked` **(call 5)** |

- A new team takes the caller's session `Name`. A stored team with an empty `Name` (an old store) is named at its first refresh, after the session name of its first member that has a session, then `-2`. Do this in `refreshChainsLocked`.
- A join sparks the team (`st.Spark`), so the console plays the spark (D8).

- [ ] **Step 1: Write the failing service tests** in `internal/service/chains_test.go`, with the existing chain test harness.

```go
func TestChainJoinsTheOnlyTeamOfWho(t *testing.T)      // b in "hearth"; a runs chain:<b.pane> → Note "joined hearth: 3 matches. To leave it, run: matchblox unchain hearth"; b and c get "[matchblox] a joined the chain" in their inbox
func TestChainFormsATeamNamedAfterTheCaller(t *testing.T) // neither in a team → team named a.Name, 2 matches
func TestChainRefusesWhoInTwoTeams(t *testing.T)       // Err == "web-cart is in 2 chains. Run one:\n  matchblox chain hearth\n  matchblox chain web"
func TestChainByTeamName(t *testing.T)                 // chain:@hearth joins
func TestChainRefusesOff(t *testing.T)                 // Err == "b is off: it takes no chains. It can run: matchblox chain --on", both when b is off and when a is off (then a's name)
func TestBareUnchainInTwoTeamsRefuses(t *testing.T)    // exact text from the table, 2 and 3 teams
func TestUnchainTeamAndAll(t *testing.T)               // chain:-@hearth → "left hearth"; chain:-* → "left hearth and web"
func TestOwnerTakesAMatchOut(t *testing.T)             // Console:true, From "" → Note "took d out of hearth"; d's inbox has "[matchblox] the owner took you out of hearth"
func TestAnAgentCannotTakeAnotherOut(t *testing.T)     // chain:out from an agent pane → Err, team unchanged
func TestStokerJoinsNoChain(t *testing.T)              // both directions → "the stoker joins no chain"
func TestOldStoreTeamsGetNames(t *testing.T)           // a chain with Name "" → named after its first member's session at the first refresh
```

- [ ] **Step 2: Run them and check that they fail.**
Run: `go test ./internal/service -run 'Chain|Unchain|Owner|Stoker|OldStore'`
Expected: FAIL on the answer texts.

- [ ] **Step 3: Rewrite `changeChains(arg string, a proto.Act)`** in `internal/service/chains.go` to the table above. Its signature takes the act, so `out:` can check `a.Console`. Helpers:
  - `func runOne(verb string, teams []string) string` builds the `Run one:` lines.
  - `func andList(names []string) string` builds `a`, `a and b`, `a, b and c`.

  `chainable` keeps the host and off checks. Its off text gains the `--on` sentence.

- [ ] **Step 4: Write the failing CLI tests** in `cmd/matchblox/cli_test.go`.

```go
func TestChainNameJoinsTheTeamBeforeTheMatch(t *testing.T) // doc has a team "hearth" and a session named "hearth" → act RecID "chain:@hearth"
func TestChainWhoBecomesPane(t *testing.T)                 // "web-cart" (no team of that name) → "chain:<pane>"
func TestUnchainWords(t *testing.T)                        // [] → "chain:-"; ["hearth"] → "chain:-@hearth"; --all → "chain:-*"; two words → error "unchain takes one chain: matchblox unchain [<chain>|--all]"
func TestWriteChainListsEachTeam(t *testing.T)             // a pane in two teams lists "hearth: a, b" then "web: a, c"; JSON {"chains":[{"name":"hearth","members":[...]}]}; none → "not in a chain. To join one, run: matchblox chain <who>"
```

- [ ] **Step 5: Implement the CLI changes.**
  - `chainAct`: a word that names a team in `doc.Chains` gives `chain:@<name>`. Otherwise `resolve` gives `chain:<pane>`.
  - `cliUnchain(words []string, all bool, …)`: add the `--all` flag where the command's flags are parsed.
  - `writeChain`: list every team of the pane, with JSON key `chains`. The old `chain` and `requests` keys go: no caller outside this repo reads them **(call)**.

- [ ] **Step 6: Run the tests and check that they pass.**
Run: `go test ./internal/service ./cmd/...`
Expected: PASS.

- [ ] **Step 7: Commit.**
`git commit -S -m "feat(chains): one command joins a team; unchain names the team"`

---

### Task 3: The mux slice

**Files:**
- Create: `internal/mux/mux.go`, `internal/mux/tmux.go`, `internal/mux/tmux_test.go`, `internal/mux/muxtest/fake.go`
- Modify: `internal/service/service.go:133-145,395`, `internal/service/peersend.go:213-310,353-375`
- Test: `internal/mux/tmux_test.go`, `internal/arch_test.go`

**Interfaces:**
- Produces:

```go
// Package mux is what chains and sending do to a pane, whatever runs the panes.
package mux

type Mux interface {
	// Type is the step that types text into pane as it is.
	Type(pane, text string) []string
	// Keys is the step that presses keys (key names) in pane.
	Keys(pane string, keys ...string) []string
	// Screens reads the visible text of panes; a pane that is gone is missing.
	Screens(panes []string) map[string]string
	// InMode tells if pane shows copy mode or another mode, where keys move a cursor.
	InMode(pane string) (bool, error)
}
```

  - Type and Keys return argv steps, because `Actions.Do` runs each step behind the stoker's guard. The guard stays as it is.
  - `mux.Tmux{Run func(argv []string) (string, error)}` implements Mux. Type uses `panes.Literal`, Keys uses `panes.Keys`, Screens uses `sample.CapturePanesStyled`, and InMode reads `display-message -p #{pane_in_mode}` through `Run`.
  - `muxtest.Fake{Screen map[string]string; Mode map[string]bool; Typed [][]string}`. Its steps are `[]string{"fake", "type", pane, text}` and `[]string{"fake", "keys", pane, k...}`.
  - `Service.Mux mux.Mux`. Set it to `mux.Tmux{Run: s.tmuxOut}` where `service.go:395` sets `Capture`. `s.Capture` stays for the stoker's guard (#200 step one).

- [ ] **Step 1: Write the failing tests.**
  - `internal/mux/tmux_test.go`:
    - `TestTmuxTypeEscapesAFinalSemicolon`: `Type("%1", "a;")` gives `panes.Literal("%1", "a;")`.
    - `TestTmuxInModeReadsDisplayMessage`: a fake `Run` gets `display-message -p -t %1 #{pane_in_mode}`. Output `"1\n"` gives true.
  - `internal/arch_test.go`: `TestChainAndSendNameNoTmux`. Read `internal/service/{peersend,chains,chainnews,notice}.go`. Fail on any of: `sample.Tmux`, `tmuxOut`, `panes.Literal`, `panes.Keys`, `s.Capture(`, `"tmux"`.

- [ ] **Step 2: Run them and check that they fail.**
Run: `go test ./internal/mux ./internal -run 'Tmux|NameNoTmux'`
Expected: FAIL: no package `mux`. The guard lists `peersend.go` lines.

- [ ] **Step 3: Implement the `internal/mux` package and `muxtest.Fake`.** Then move the send code onto it:
  - `typeLine` uses `s.Mux.Type` and `s.Mux.Keys`.
  - `showsLine`, `clearLine` and `peerReady` use `s.Mux.Screens`.
  - `paneInMode` becomes `s.Mux.InMode` and is deleted.
  - Service tests that set `Capture` for sends set `Mux: &muxtest.Fake{...}`.

- [ ] **Step 4: Run the tests and check that they pass.**
Run: `go test ./internal/... -run 'Tmux|NameNoTmux|Peer'`
Expected: PASS.

- [ ] **Step 5: Commit.**
`git commit -S -m "refactor(mux): chains and sends reach panes through internal/mux"`

---

### Task 4: A send is an inbox

**Files:**
- Modify: `internal/service/peersend.go:69-250,353-375`, `internal/chains/chains.go` (Item, Post, put), `internal/service/notice.go:104-121` (heard)
- Test: `internal/service/peersend_test.go`, `internal/chains/chains_test.go`

**Interfaces:**
- Consumes: Task 1's `Shared`. Task 3's `Service.Mux` and `muxtest.Fake`.
- Produces:
  - `type Item struct { At time.Time; Text string; From Member; seq uint64 }`. `From` is the zero value for news, and the sender for a sent line.
  - `func (s *Store) PostLine(to, from Member, text string, now time.Time) (dropped []Item)`
  - `func (s *Store) Post(to Member, text string, now time.Time) (dropped []Item)`: it returns the sent lines it pushed out.
  - `func (s *Store) InboxCount(m Member) (n int, full bool)`: `full` is true when any item was dropped since the last Take.
  - `func (s *Service) peerWait(pane string, x sample.Session) (why string, err error)`: `err` means the mate is gone or another agent runs there, so the send is refused. A non-empty `why` means the line waits.

`peerWait`'s why, in this order:

| Condition (from `messageable`, `queued`, `resting`, the screen) | why |
|---|---|
| asks a question | `it asks a question now` |
| a permission prompt | `it shows a permission prompt` |
| held by the stoker | `the stoker holds it` |
| parked | `it is parked` |
| waits for you (`stoker.Waits`) | `it waits for you` |
| not at rest | `it is busy` |
| draft in the prompt | `it has a draft in its prompt` |
| copy mode (`Mux.InMode`) | `its pane is in copy mode` |
| screen unreadable | `its pane cannot be read` |

Answers:

| Case | Note |
|---|---|
| typed now | `sent` |
| waits (Claude Code, Cursor: inbox; Codex: the at-rest queue) | `queued for <mate>: <why>. It gets the line at its next rest.` |
| OpenCode (D7) | `kept for <mate>: OpenCode takes no typed lines. It shows in matchblox peek <mate>.` |

- The refusals left are unchanged: not in a shared team, sends off, not one line, the rate limit, self, the stoker.
- When typing fails before a key lands, the line goes to the inbox with the same `queued for` answer, not an error.
- When a line was typed and then cleared, it also goes to the inbox.
- A line that may still be in the prompt (`stuck`) keeps today's error.
- D3: in `heard(d)`, for each taken item with a non-zero `From`, post to From `[matchblox] <mate> got your line (heard 15:04)` and spark the shared team. In `deliverAtRest`, a typed line posts `(typed 15:04)`. Use the local time, format `15:04`. A line typed at once in `peerSend` gets no notice.
- D4: for each dropped item with a non-zero From, post to From `[matchblox] 1 line to <mate> dropped: its inbox is full`. Use one post per sender and mate, as `N lines` when N > 1 **(call)**.

- [ ] **Step 1: Write the failing tests.**

```go
// peersend_test.go
func TestSendToAMateThatAsksQueues(t *testing.T)        // Claude at rest with a question → Note "queued for b: it asks a question now. It gets the line at its next rest."; b's inbox holds "[from a %1] hi"
func TestSendWhyForEachWait(t *testing.T)               // table over the why table: each state → its exact why
func TestSendTypedAtOnceSaysSent(t *testing.T)          // Note == "sent"; no "got your line" ever posted to a
func TestHeardLineTellsTheSender(t *testing.T)          // queued → news(b) → heard → a's inbox has "[matchblox] b got your line (heard 14:02)"; the team's Spark moved
func TestCodexTypedAtRestTellsTheSender(t *testing.T)   // deliverAtRest → "(typed 14:02)"
func TestSendToOpenCodeIsKept(t *testing.T)             // Note "kept for oc: OpenCode takes no typed lines. It shows in matchblox peek oc."
func TestSendToAMateInOneOfTwoTeams(t *testing.T)       // a in hearth+web, b only in web → queued/sent, not refused
func TestQueuedLineToAMateThatLeftIsDropped(t *testing.T) // inbox line, then b leaves web → a hears "b did not get your line: you are no longer chained"; no "got your line" later
func TestTypingRefusedBeforeAKeyQueues(t *testing.T)    // fake Mux InMode true at type time → inbox, Note "queued for b: its pane is in copy mode. …"
// chains_test.go
func TestDroppedLineTellsItsSender(t *testing.T)        // 21 PostLine from a → Post returns 1 dropped item From a
func TestInboxCountFull(t *testing.T)                   // 21 items → (20, true); after Take → (0, false)
```

  For `TestQueuedLineToAMateThatLeftIsDropped`: when a member leaves a team, `Store` drops inbox lines whose From no longer shares a team with it, and returns them. The service posts `didNotGet(...)` for each. Task 1's signatures change here: `LeaveTeam(m Member, team string) (ok bool, dropped []Item)` and `Leave(m Member) (dropped []Item)`. Update their callers from Tasks 1 and 2.

- [ ] **Step 2: Run them and check that they fail.**
Run: `go test ./internal/chains ./internal/service -run 'Send|Heard|Codex|Dropped|Inbox|Queued|Typing'`
Expected: FAIL.

- [ ] **Step 3: Implement the store changes, `peerWait`, and the new `peerSend` branches.** `peerReady` becomes `peerWait`. Its errors with `peerRetry` go, and `peerRetry` is deleted.

- [ ] **Step 4: Run all service and chains tests.** The old tests that expected a refusal for the mate's state are rewritten to expect `queued for`.
Run: `go test ./internal/chains ./internal/service`
Expected: PASS.

- [ ] **Step 5: Commit.**
`git commit -S -m "feat(send): a line to a mate waits in its inbox, and the sender hears when it lands"`

---

### Task 5: A mate stays while its pane lives (D6), and the room's data

**Files:**
- Modify: `internal/chains/chains.go`, `internal/service/chains.go:120-240`, `internal/state/state.go:84-88`, `internal/service/lifecycle_test.go`
- Test: `internal/chains/chains_test.go`, `internal/service/chains_test.go`

**Interfaces:**
- Consumes: Task 4's `InboxCount`.
- Produces:
  - `Store.Unseen map[string]time.Time`, json `"unseen,omitempty"`. It is keyed like the inbox. `Refresh` renames it, and `Prune` drops keys that `holds` no longer names.
  - `func (s *Store) MarkUnseen(m Member, now time.Time)`: keeps the earliest time.
  - `func (s *Store) MarkSeen(m Member)`
  - `func (s *Store) UnseenSince(m Member) (time.Time, bool)`
  - `type MateView struct { Member chains.Member; Inbox int; Full bool; Unseen time.Time }`. `Unseen` uses json `omitzero`.
  - `ChainsView{ Chains []chains.Chain; Mates []MateView }`. There is one MateView per distinct member of any team. #326's room reads it.

The rule in `pruneChainsLocked`:
- A member is live while its session shows, or while `m.Pane` is in `s.cur.Panes` (an `ID` match).
- A live member with no session gets `MarkUnseen(m, now)`. A member with a session gets `MarkSeen`.
- A member whose pane is gone leaves every team. Its mates hear `<name> left the chain` (as today).
- A sample that missed (`sampleMissedLocked`) changes nothing (as today).
- #333 reconciled: the pane rule covers the switch gap. Delete `switchGrace` and `switchingLocked`. Keep `Move`, `switchedToLocked` and `markClearedLocked`: they rename the member to the new agent.

- [ ] **Step 1: Write the failing tests.**

```go
func TestEmptyPaneKeepsTheMateUnseen(t *testing.T)     // session gone, pane listed → still in team; UnseenSince == first miss; a later miss keeps the first time
func TestClosedPaneLeaves(t *testing.T)                // pane gone → leaves; mates' inbox "b left the chain"
func TestSampleMissKeepsTeamsAndTimes(t *testing.T)    // Errors ["tmux: …"] → no change, no Unseen set
func TestSeenAgainClearsUnseen(t *testing.T)
func TestChainsViewCarriesMates(t *testing.T)          // MateView per member: inbox 3 → Inbox 3; 21 → Inbox 20, Full true; unseen → Unseen set
```

  In `lifecycle_test.go`, `TestASwitchWithNoNewAgentLeavesTheChain` becomes `TestASwitchWithNoNewAgentStaysWhileItsPaneLives`, with a second phase: the pane closes, so the member leaves. `TestASwitchKeepsTheChain` and `TestASwitchTheGuardStopsKeepsNoMove` stay as they are.

- [ ] **Step 2: Run them and check that they fail.**
Run: `go test ./internal/chains ./internal/service -run 'Pane|SampleMiss|Seen|ChainsView|Switch'`
Expected: FAIL.

- [ ] **Step 3: Implement the store fields, the prune rule and `chainsView(st, now)`.** `chainsView` returns nil when there is no team.

- [ ] **Step 4: Run the tests with the race detector.**
Run: `go test -race -count=3 ./internal/chains ./internal/service`
Expected: PASS.

- [ ] **Step 5: Commit.**
`git commit -S -m "feat(chains): a mate stays in its team while its pane lives"`

---

### Task 6: The console

**Files:**
- Modify: `internal/app/fuse.go:56-125`, `internal/app/sessions.go:500-530`, `internal/app/match.go:170-180`, `internal/app/model.go` (toast at `say`)
- Test: `internal/app/fuse_test.go` and the Sessions goldens at 80 and 40 columns

**Interfaces:**
- Consumes: Task 1's `Chain.Name` and store order. Task 2's act `chain:out:<pane>@<team>`. Task 5's `ChainsView`.
- Produces:
  - `func (m Model) alsoIn(s sample.Session) string`: `" · also in web"` for each team after the first (`" · also in web, docs"` for two **(call)**), or "".
  - `func (m *Model) noteJoins(host string, next *state.ChainsView, now time.Time) tea.Cmd`: the toast for each member new to a team since the last view.
  - `func (m Model) outOfTeam(s sample.Session, team string) action`: `label` and `say` are `take <name> out of <team>? y`, `rec` is `chain:out:<pane>@<team>`, and `destructive` is true. #326 binds it to `u` in the room (call 2).

Exact copy:

| Case | Text |
|---|---|
| a join, width ≥ 50 | `<name> joined <team> · u in its room takes it out` |
| a join, width < 50 | `<name> joined <team> · u: out` |
| a new team | `<team> formed: <a>, <b>` **(call 7)** |

- [ ] **Step 1: Write the failing tests** in `internal/app/fuse_test.go`.

```go
func TestSharedMatchDrawsInItsFirstTeam(t *testing.T)   // a in hearth (first) and web → a's row in hearth's fuse; web's fuse skips a's row
func TestSharedMatchRowSaysAlsoIn(t *testing.T)         // a's row ends in faint " · also in web"; it is cut before the name is cut
func TestOtherTeamKeepsAStub(t *testing.T)              // web = {a, b}; b's row shows the stub ─╴ (present counts a)
func TestJoinToastAndSpark(t *testing.T)                // a new member in hearth → toast "d joined hearth · u in its room takes it out"; the spark is on
func TestJoinToastNarrow(t *testing.T)                  // width 40 → "d joined hearth · u: out"
func TestNewTeamToast(t *testing.T)                     // "hearth formed: hearth, web-cart"
func TestOutOfTeamAsks(t *testing.T)                    // outOfTeam(d, "hearth").say == "take d out of hearth? y"; y sends RecID "chain:out:%4@hearth"
func TestNoMotionNoSpark(t *testing.T)                  // --no-motion: the toast shows, the spark does not
```

  Regenerate the Sessions goldens at 80 and 40 columns with the shared-match fixture. Check each golden diff by eye.

- [ ] **Step 2: Run them and check that they fail.**
Run: `go test ./internal/app -run 'Shared|AlsoIn|Stub|Toast|OutOfTeam|NoMotion'`
Expected: FAIL.

- [ ] **Step 3: Implement the console changes.**
  - `indexChains`: `present` counts every member of a team that the host has, the shared ones too. `of` keeps the first team.
  - `sessions.go`: append `alsoIn` in `st.faint` after `sameRepoHint`, under the same width rule.
  - `match.go:175`: call `noteJoins` next to `noteSparks`. It returns `m.say(...)`.

- [ ] **Step 4: Run all app tests.**
Run: `go test ./internal/app`
Expected: PASS.

- [ ] **Step 5: Commit.**
`git commit -S -m "feat(console): a shared match keeps one fuse, and a join shows a toast"`

---

### Task 7: Docs, handoffs and the landing page words

**Files:**
- Modify: `docs/agents.md` (chains section), `docs/console.md` (fuse and toast rows), `CHANGELOG.md` (Unreleased → Added and Changed)
- Test: `go test ./cmd/... ./internal/docscheck/...`

- [ ] **Step 1: Update the docs.**
  - `docs/agents.md`:
    - One command joins: `matchblox chain <who>` or `<team>`.
    - A match may be in several teams, and each team hears only itself.
    - The `unchain` forms.
    - Sends wait in the inbox. Add the `queued for`, `got your line` and `kept for` lines.
    - The pane-lives rule.
    - Delete "Nothing is shared until both sides chain" and the request text.
  - `docs/console.md`: the `· also in` suffix, the join toast, and the `--no-motion` note.
  - `CHANGELOG.md`:
    - Added: "One command joins a chain".
    - Added: "A match may be in several chains".
    - Changed: "A send to a busy mate waits in its inbox instead of being refused".
    - Changed: "A mate stays in its chain while its pane lives".
    - Removed: "Chain requests".

- [ ] **Step 2: Run the docs tests.**
Run: `go test ./cmd/... ./internal/docscheck/...`
Expected: PASS.

- [ ] **Step 3: Commit.**
`git commit -S -m "docs: one-way chains, several chains per match, sends that wait"`

- [ ] **Step 4: Handoffs** (messages. Each goes through `matchblox send`, at most 500 characters, 30 s apart):
  - **To matchblox-ux (#326):**
    - `ChainsView.Mates` carries `Inbox`, `Full` and `Unseen` per member.
    - `outOfTeam` is ready for `u` in the room.
    - `m` writes a line.
    - `✉ 20+` comes from `Full`.
    - OpenCode's count is `faint`.
  - **To matchbloxrelease:** the chain section's words, verbatim:
    - Lede: "Chain your agents into a team with one command. Every match in the team hears the others: commits, pushes and questions, across repos and agents."
    - Beat 1: "One command joins. `matchblox chain web-cart` puts you in its team, or starts one."
    - Beat 2: "Nothing bounces. A line to a busy mate waits in its inbox, lands at its next rest, and you hear when it got it."
    - Beat 3: "Any agent, any repo. Claude Code, Codex, Cursor and OpenCode share one team, and the console shows each with its repo."
  - **On #304:** a comment with D5, after the owner's yes (an external write): "A team of one stays while its stoker has a live plan, with the room line `hearth · 1 match: run matchblox chain hearth from another match to join`."
  - Each peer's confirmation goes in the handover record.

---

## Spec coverage

| Spec item | Task |
|---|---|
| One-command join, D1 names, team of `<who>` / new team | 1, 2 |
| Ambiguous join, D12 unchain, off refusal | 2 |
| Mates hear a join; requests go away | 2, 1 |
| Stoker in no team | 2 |
| D5 team of one | moved to #304 (call 1) |
| D6 pane lives, `not seen` data | 5 |
| Sending refusals left; queue; D3; D4; D7 | 4 |
| D9 take a match out | 2 (act), 6 (confirm), #326 (key) |
| D8 toast and spark; D11 narrow | 6 |
| D2 shared match fuse and `· also in` | 6 |
| D10 room tokens; room rows | #326 (Task 7 handoff) |
| Mux layer and no-tmux guard | 3 |
| Landing page | 7 |
| Colour never the only signal; `--no-motion` | 6 (`TestNoMotionNoSpark`), 5 (`Full` carries the number) |
