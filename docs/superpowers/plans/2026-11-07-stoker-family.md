# A family of stokers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each chain can have its own stoker agent, the person talks to a chain through its stoker, and the host (night) stoker heads the family.

**Architecture:** The service keeps one `stoker.Store` today (`Service.Stoker`) and about 64 places assume it. Task 3 turns that into a family of `stokerUnit`s with no change in behavior: the host unit wraps today's store and paths. Chain units live in `stokers/<id>/` under the state directory, run in tmux session `stoker-<id>` on the stoker server, and are found from the tmux session of the acting pane. Rules (scope, owner, gos, night) are checks on the unit of the act. The console gets a head row in each fuse, a family STOKER tab and a scoped start sheet.

**Tech Stack:** Go 1.26, Bubble Tea / lipgloss (console), tmux, plain HTML/JS for `www/`.

**Spec:** `docs/superpowers/specs/2026-11-06-stoker-family-design.md`

## Global Constraints

- With no chain stoker, every screen, `matchblox sessions --json` and `matchblox status` output stays byte for byte as today. Existing golden files must not change.
- The host stoker keeps today's paths (`stoker.json`, `stoker.jsonl`, `stoker-runs.jsonl`, `driven.json` in the state dir) and tmux session `stoker`.
- A chain stoker: tmux session `stoker-<stoker-id>` on `StokerServer`, store `stokers/<stoker-id>/` with the same file names. Stoker ids: `s1`, `s2`, … (never reused while a dir exists).
- `[stoker] max_chains = 4` by default.
- A chain stoker runs `[stoker] agent` on the default sign-in, started like the host stoker (`startArgv`), in the builder's home.
- No stoker starts by itself. Each start is the person's act or the host stoker's order.
- Copy is ASD-STE100: one idea per sentence, active, present tense. Exact strings below are fixed.
- No other product's name in code, docs or `www/`.
- Commits: signed Conventional Commits, `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never `--no-verify`.
- Every task ends with `go test ./...` and `go vet ./...` green.

Fixed strings:

| Where | Text |
|---|---|
| act outside its chain | `<who> is not in the chain of this stoker.` |
| host acts on an owned match | `<who> has the stoker of chain <name>: give it an order.` |
| cap | `the family has its <n> chain stokers: max_chains. Quit one on the STOKER tab first.` |
| go the giver lacks | `the order gives <go>, which its giver does not have` |
| person orders at night | `the night is sealed: answer the stoker's prompts, or give the order in the morning` |
| new session outside the repo | `a session of chain <name> starts in its repo: <repo>` |
| chain stoker gives an order | `a chain stoker gives no orders` |
| card: start | `started the stoker of chain <name> (<id>)` |
| card: dropped step | `dropped the <kill|go> for <session>: the stoker of chain <name> owns it now` |
| card: chain joined | `its chain joined <name>` |
| card: chain ended | `its chain ended` |
| dock/header with chain stokers | `stoker · <n> chains` (`· 1 chain` for one) |
| start sheet title | `Stoke the chain <name> (<n> matches)?` |
| start sheet prompt | `How should I run this chain?` |

## Review Focus

1. **The service restarts in the middle of a family run.** Units must come back from `stokers/*/`, a tmux session `stoker-<id>` with no store dir must be treated as a stoker pane (the guard denies) and its acts refused, and a store whose session is gone shows as not running. Test in Task 3 (`TestFamilyReloadsAfterRestart`, `TestUnknownStokerSessionFailsClosed`).
2. **A chain loses a member while its stoker is mid-step.** The scope check reads `chains.Store.Mates` at the time of the act, so a send to the former member fails with the scope text. Test in Task 5 (`TestScopeIsCheckedAtActTime`).
3. **The host stoker hits `max_chains` at night.** The order fails, the host gets the cap text as its result, and the host card logs it. Nothing starts. Test in Task 7 (`TestOrderAtCapIsRefusedAndLogged`).
4. **The night ends or the host loses a go while a chain stoker's session is about to merge.** The guard reads effective grants from that unit's `driven.json`, rewritten on each change of the giver. Test in Task 6 (`TestGiverLosesGoReachesGuard`).
5. **A session is renamed or moves pane while its chain has a stoker.** Ownership and scope use `chains.Member` (session id first), never the pane string. Test in Task 5 (`TestScopeFollowsRenamedMember`).

---

### Task 1: Stoker family layout in `internal/stoker`

**Files:**
- Create: `internal/stoker/family.go`
- Test: `internal/stoker/family_test.go`

**Interfaces:**
- Produces:
  - `const HostID = ""`
  - `func FamilyDir(state, id string) string` — `state` for `HostID`, else `state/stokers/<id>`.
  - `func FamilyIDs(state string) ([]string, error)` — the ids of `stokers/*/` that hold a `link.json`, sorted by number (`s2` before `s10`). A missing `stokers/` is no ids and no error.
  - `func NextID(ids []string) string` — `s<max+1>`; `s1` for none.
  - `func SessionName(id string) string` — `sample.StokerSession` for `HostID`, else `"stoker-" + id`.
  - `func IDOfSession(name string) (id string, ok bool)` — inverse; `ok` false for any other name.
  - `type Link struct { ID string; Chain string; Giver string; Grants []string }` with json tags `id`, `chain`, `giver` (`"person"` or `"host"`), `grants`.
  - `func ReadLink(dir string) (Link, error)`, `func WriteLink(dir string, l Link) error` — `link.json`, atomic write (same pattern as `WriteDriven`).

- [ ] **Step 1: Write the failing tests**
  - `TestFamilyDir`: `FamilyDir("/st", HostID) == "/st"`, `FamilyDir("/st", "s2") == "/st/stokers/s2"`.
  - `TestFamilyIDsSortsByNumber`: dirs `s10`, `s2`, `junk` (no link.json) → `[]string{"s2","s10"}`.
  - `TestNextID`: `nil → "s1"`, `{"s1","s3"} → "s4"`.
  - `TestSessionNameRoundTrip`: `IDOfSession(SessionName("s4")) == ("s4", true)`; `IDOfSession("stoker") == ("", true)`; `IDOfSession("api") ok == false`.
  - `TestLinkRoundTrip`.
- [ ] **Step 2: Run** `go test ./internal/stoker -run 'Family|NextID|SessionName|Link'` — FAIL (undefined).
- [ ] **Step 3: Implement** `family.go`.
- [ ] **Step 4: Run** the same — PASS.
- [ ] **Step 5: Commit** `feat(stoker): the layout of a family of stokers (#304)`

### Task 2: The merge guard reads the whole family

**Files:**
- Modify: `internal/stoker/driven.go` (`Deny`, around line 507)
- Modify: the hook's call of `stoker.Deny` in `cmd/matchblox` (find with `rg -n "stoker.Deny\(" cmd`)
- Test: `internal/stoker/deny_test.go`, `cmd/matchblox/hook_test.go`

**Interfaces:**
- Consumes: `FamilyDir`, `FamilyIDs` (Task 1).
- Produces: `func DenyFamily(state, socket, pane string, gs []Guard) string`.

Rule: a pane on the stoker server (`socket == d.StokerSocket` of any member) → `DenyStokerPane`. Else run today's `Deny(dir, …)` for the host dir and each family dir; a pane that one unit's `driven.json` lists is decided by that unit alone. A pane no unit lists is decided as today by the host dir (fail closed with `DenyLost` when any unit's run is on and its `driven.json` is missing).

- [ ] **Step 1: Write the failing tests**
  - `TestDenyFamilyChainUnitAllowsItsGo`: host off; `stokers/s1/driven.json` drives `%7` with grants `["merge"]`, run on → `DenyFamily(st, sock, "%7", merge) == ""`.
  - `TestDenyFamilyOtherUnitsGoDoesNotLeak`: `s1` drives `%7` with `merge`, `s2` drives `%8` with none → `%8` merge denied.
  - `TestDenyFamilyLostDrivenFailsClosed`: `s1` mode on, no `driven.json` → `DenyLost` for any pane.
  - `TestDenyFamilyNoFamilyIsToday`: with no `stokers/`, for each case in the existing `Deny` table, `DenyFamily == Deny`.
  - In `hook_test.go`: the hook denies a merge in a pane that only `s1` drives without the go.
- [ ] **Step 2: Run** `go test ./internal/stoker ./cmd/matchblox -run 'DenyFamily|Hook'` — FAIL.
- [ ] **Step 3: Implement** `DenyFamily`; switch the hook to it.
- [ ] **Step 4: Run** — PASS. Then `go test ./...`.
- [ ] **Step 5: Commit** `feat(guard): the merge guard watches every stoker of the family (#304)`

### Task 3: The service keeps a family of stoker units (no change in behavior)

**Files:**
- Create: `internal/service/family.go`
- Modify: every file that reads `s.Stoker`, `s.startedAt` or the driven state: `stoker.go`, `orders.go`, `hands.go`, `driven.go`, `wakes.go`, `lifecycle.go`, `when.go`, `service.go`, `stokeragent.go`
- Test: `internal/service/family_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - `type stokerUnit struct { id string; link stoker.Link; store *stoker.Store; startedAt time.Time }` — `link.Chain` empty for the host.
  - `func (s *Service) hostUnit() *stokerUnit` — wraps `s.Stoker` (fixtures keep setting `Service.Stoker`); nil when `s.Stoker` is nil.
  - `func (s *Service) family() []*stokerUnit` — host first, then chain units by id.
  - `func (s *Service) unitByID(id string) *stokerUnit`
  - `func (s *Service) unitOfChain(chainID string) *stokerUnit`
  - `func (s *Service) unitOf(a proto.Act) (u *stokerUnit, stokerPane bool)` — replaces `fromStoker`. `stokerPane` is true for any pane on `StokerServer`. `u` comes from `stoker.IDOfSession` on the pane's tmux session; `u == nil` with `stokerPane == true` means an unknown stoker session.
  - `func (s *Service) loadFamily() error` — reads `stoker.FamilyIDs`, opens each store, reads its link. Called where `s.Stoker` is opened today.
  - Each existing method that used `s.Stoker` or `s.startedAt` takes `u *stokerUnit` or is called once per unit. Keep names; add the parameter.

- [ ] **Step 1: Write the failing tests**
  - `TestFamilyIsHostOnlyWithoutChains`: a fixture service with `Stoker` set → `len(s.family()) == 1`, `s.family()[0].id == stoker.HostID`.
  - `TestFamilyReloadsAfterRestart`: write `stokers/s1/link.json` + an on mode, build a new service, `loadFamily()` → `unitByID("s1").store.Mode().On`.
  - `TestUnknownStokerSessionFailsClosed`: an act from a pane on `StokerServer` in tmux session `stoker-s9` with no store → `unitOf` returns `(nil, true)` and `orderable` refuses with `no agent acts on the stoker's own session`.
- [ ] **Step 2: Run** `go test ./internal/service -run Family` — FAIL.
- [ ] **Step 3: Implement** `family.go`, then move each `s.Stoker` use behind a unit. Rule of the refactor: with only the host unit, every path does what it does today.
- [ ] **Step 4: Run** `go test ./...` — PASS, with no golden file changed (`git status` shows no `testdata` change).
- [ ] **Step 5: Commit** `refactor(service): the stoker is the first unit of a family (#304)`

### Task 4: Start a chain stoker

**Files:**
- Modify: `internal/service/stokeragent.go` (start argv takes the session name), `internal/service/family.go`
- Modify: `internal/stoker/brief.go` (a chain section)
- Modify: `internal/config/config.go` (`Stoker.MaxChains int`, toml `max_chains`, default 4)
- Modify: `cmd/matchblox/main.go` (`stoker new`)
- Test: `internal/service/family_test.go`, `internal/stoker/brief_test.go` (create if absent), `internal/config/config_test.go`, `cmd/matchblox/cli_test.go`

**Interfaces:**
- Consumes: Tasks 1, 3.
- Produces:
  - Act `stoker:new` with `Text` = JSON `{"chain":"<who>","words":"…","grants":["merge"]}`.
  - `func (s *Service) startChainStoker(ctx context.Context, who, words string, grants []string, giver string) (*stokerUnit, error)` — resolves `who` (a chain name, a member name or a chain id) to a chain; reuses the chain's unit if one exists (then it is an order: Task 7); refuses at `MaxChains` with the cap text; makes the dir, writes `link.json`, starts `SessionName(id)` with `startArgv`, writes the brief to `stokers/<id>/stoker-brief.md`, types `Read <path> and follow it.`, turns its mode on (day) and records the order with `grants`; appends `started the stoker of chain <name> (<id>)` to the host store.
  - `func chainName(c chains.Chain, sessions []sample.Session) string` — the name of the first member that has one, else the chain id.
  - `stoker.BriefChain struct { Name string; Members []string; Repo string }`; `Brief` gains `Chain *BriefChain`. A brief with a chain lists only its members under the machine, and says: `You are the stoker of chain <name>. You act on its matches only.`
  - CLI: `matchblox stoker new --chain <who> [--go merges,releases,deploys,kills] ["<order>"]`. No `--go`: no grants.

- [ ] **Step 1: Write the failing tests**
  - `TestStartChainStokerMakesItsUnit`: a chain of `%3`,`%4` → after start, `stokers/s1/link.json` has the chain id, `giver:"person"`, the fake tmux saw `new-session -d -s stoker-s1`, and the pane got `Read …/stokers/s1/stoker-brief.md and follow it.`
  - `TestStartChainStokerAtCap`: `MaxChains: 1`, one unit exists → error text equals the cap text with `<n>` = 1.
  - `TestStartNeedsAChain`: `who` in no chain → error `<who> is in no chain.`
  - `TestBriefScopesToChain`: the brief names only member sessions.
  - `TestConfigMaxChainsDefault`: 4.
  - `TestCLIStokerNewParsesGo`: `--go merges,kills` → grants `["merge","kill"]` (use today's grant words from `stoker.CheckGrant`).
- [ ] **Step 2: Run** the new tests — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...` — PASS.
- [ ] **Step 5: Commit** `feat(stoker): start a stoker for one chain (#304)`

### Task 5: One owner for each match: scope and the host's refusal

**Files:**
- Modify: `internal/service/hands.go` (`orderable`)
- Modify: `internal/service/chains.go` (join on `new`)
- Modify: `internal/chains/chains.go` (`Join`)
- Test: `internal/service/family_test.go`, `internal/chains/chains_test.go`

**Interfaces:**
- Consumes: Task 3 `unitOf`, `unitOfChain`.
- Produces:
  - `func (s *Service) ownerOf(x sample.Session) *stokerUnit` — the chain unit of `x`'s chain when its run is on; else the host unit.
  - `func (st *Store) Join(chainID string, m Member) error` in `internal/chains` — adds `m` with no ask; error when `m` is off (`SetOff`) or already in another chain.
  - In `orderable`: for a chain unit, a pane act on a non-mate fails with the scope text; for the host unit, a pane act on a match whose `ownerOf` is a chain unit fails with the owner text. `stoker:order` from a chain unit fails with `a chain stoker gives no orders`.
  - A `new` from a chain unit: the session joins the chain (`Join`) once the sample finds it; a `--where` outside the chain's repo fails with the repo text.

- [ ] **Step 1: Write the failing tests**
  - `TestChainStokerActsOnMatesOnly`: `send` to a mate passes, to `%9` fails with `api-x is not in the chain of this stoker.`
  - `TestHostRefusedOnOwnedMatch`: chain unit run on → host `send` to a member fails with `api-auth has the stoker of chain api-auth: give it an order.`
  - `TestHostActsWhenChainStokerRests`: chain unit mode off → host `send` passes.
  - `TestScopeIsCheckedAtActTime`: member leaves the chain, then the chain unit's `send` to it fails with the scope text.
  - `TestScopeFollowsRenamedMember`: a member's pane changes (same session id) → still a mate.
  - `TestChainStokerNewJoinsChain`, `TestChainStokerNewOutsideRepo`.
  - `TestJoinRefusesOff` in `chains_test.go`.
  - `TestChainStokerReadsWholeHost`: `peek` on a non-mate passes.
- [ ] **Step 2–4:** run (FAIL), implement, run `go test ./...` (PASS).
- [ ] **Step 5: Commit** `feat(stoker): a chain stoker drives its chain only, and each match has one owner (#304)`

### Task 6: Gos follow the giver

**Files:**
- Modify: `internal/service/driven.go` (`updateDriven` writes each unit's effective grants)
- Modify: `internal/service/family.go`
- Test: `internal/service/family_test.go`

**Interfaces:**
- Produces:
  - `func (s *Service) grantsOf(u *stokerUnit) []string` — host: its order's grants (today). Chain unit: its order's grants ∩ `link.Grants` ∩ (host's current grants when `link.Giver == "host"`). A host with no run on gives a host-given unit no grants.
  - `func checkGiven(grants, giver []string) error` — the first grant not in `giver` → `the order gives <go>, which its giver does not have`.
  - Each change of the host's order, mode or night calls `updateDriven` for every unit, so each `driven.json` holds `grantsOf(u)`.

- [ ] **Step 1: Write the failing tests**
  - `TestGrantsOfHostGivenIsIntersection`: host `merge,release`, link `merge` → `["merge"]`.
  - `TestGiverLosesGoReachesGuard`: host order drops `merge` → `stokers/s1/driven.json` grants lose `merge`; `stoker.DenyFamily` then denies the merge in the unit's driven pane.
  - `TestNightEndClearsHostGivenGos`.
  - `TestPersonGivenKeepsItsGos`: host off → a person-given unit keeps `merge`.
  - `TestCheckGiven`.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(stoker): a chain stoker's gos are never more than its giver's (#304)`

### Task 7: Give a chain an order

**Files:**
- Modify: `internal/service/orders.go`
- Modify: `cmd/matchblox/main.go` (`stoker order`)
- Test: `internal/service/orders_test.go` (or `family_test.go`), `cmd/matchblox/cli_test.go`

**Interfaces:**
- Consumes: `startChainStoker` (Task 4), `checkGiven` (Task 6), `unitOf` (Task 3).
- Produces:
  - Act `stoker:order-chain` with `Text` JSON `{"chain":"<who>","words":"…","grants":[…]}`; giver is `"host"` when `unitOf` is the host unit, `"person"` when the act is not from the stoker server.
  - `func (s *Service) chainOrder(ctx context.Context, who, words string, grants []string, giver string) proto.Result` — no unit: `startChainStoker`. A unit with a run on by day: today's day-order path on that unit (`New order: <words>` typed into `stoker-<id>`, recorded on its card). The person at night: the sealed text. The host at night: allowed. Every order also appends a line to the host card.
  - CLI: `matchblox stoker order <chain> [--go …] "<text>"`. With no `--go` the grants are none for the person, and the host's current grants for the host.

- [ ] **Step 1: Write the failing tests**
  - `TestHostOrderStartsChainStoker`, `TestHostOrderToRunningUnitTypesNewOrder` (pane gets `New order: merge on green`), `TestPersonOrderAtNightIsSealed`, `TestHostOrderAtNightPasses`, `TestOrderAtCapIsRefusedAndLogged`, `TestHostOrderDefaultsToItsGrants`, `TestCLIStokerOrder`.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(stoker): the host stoker and the person give a chain an order (#304)`

### Task 8: The night seals the family, and a start takes over the host's steps

**Files:**
- Modify: `internal/service/stoker.go` (`stokerAct` for `night`/`off`, `stoke` loop over the family), `internal/service/holds.go`, `internal/service/when.go`
- Modify: `internal/stoker/brief.go` (host brief lists the chains)
- Test: `internal/service/family_test.go`, `internal/stoker/brief_test.go`

**Interfaces:**
- Produces:
  - `n`: each chain unit whose run is on gets `Mode.Night(now, NightEnds)`; a unit started during the night starts with `Night`. `n` again and the night's end turn every unit off. `f` touches the host unit only.
  - Host brief: a `Chains` list of `{Name, StokerID, State, PlanDone, PlanAll}`, with the line `Give a chain an order: matchblox stoker order <chain> "<text>"`.
  - `func (s *Service) handOver(u *stokerUnit)` — called when a chain unit's run starts: host holds (`holds.ByStoker`) on its members move to the unit; host triggers that name its members move; each host card entry that waits for a `y` (kill or go) for a member is dropped with the dropped-step text.
  - The morning card of the host lists each chain unit with its card's score line.

- [ ] **Step 1: Write the failing tests**
  - `TestNightSealsRunningChainUnits`, `TestNightEndStopsTheFamily`, `TestFTouchesHostOnly`, `TestHandOverMovesHoldsAndTriggers`, `TestHandOverDropsWaitingKillWithLine`, `TestHostBriefListsChains`, `TestMorningCardListsChains`.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(stoker): the night runs the family, and a chain stoker takes over its matches (#304)`

### Task 9: A chain stoker follows its chain's life

**Files:**
- Modify: `internal/service/chains.go` (`pruneChainsLocked`, the join path that calls `chains.Store.merge`)
- Modify: `internal/chains/chains.go` (report a join: `Ask` returns the absorbed chain id)
- Test: `internal/service/family_test.go`, `internal/chains/chains_test.go`

**Interfaces:**
- Produces:
  - `func (st *Store) Ask(from, to Member, repo string, now time.Time) (formed bool, absorbed string)` — `absorbed` is the id of the chain that merged into the other, `""` otherwise. Update callers.
  - `func (s *Service) chainEnded(chainID string)` — its unit writes `its chain ended` on its card, ends its run and quits its tmux session.
  - `func (s *Service) chainsJoined(survivor, absorbed string)` — one unit: its `link.Chain` becomes `survivor`. Two: the absorbed side's unit writes `its chain joined <name>`, quits; its latest order words are typed into the survivor as `New order (from the joined chain <name>): <words>` with `grants = grantsOf(survivor) ∩ grantsOf(absorbed)`.

- [ ] **Step 1: Write the failing tests**
  - `TestChainEndQuitsItsStoker`, `TestJoinKeepsTheOnlyStokerAndRelinks`, `TestJoinOfTwoStokersKeepsSurvivor`, `TestJoinCarriesOrderWithCommonGos`, `TestAskReportsAbsorbed`.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(stoker): a chain stoker ends and joins with its chain (#304)`

### Task 10: The family in state and JSON

**Files:**
- Modify: `internal/state/state.go`, `internal/service/service.go` (sample publish), `cmd/matchblox/cli.go` (sessions JSON row), `matchblox status` text
- Test: `internal/state/state_test.go`, `cmd/matchblox/cli_test.go`

**Interfaces:**
- Produces:
  - `state.ChainStoker struct { ID string; Chain string; Name string; View stoker.View }` json `id`, `chain`, `name`, `view`; `State.Stokers []ChainStoker` json `stokers,omitempty`. `State.Stoker` stays the host's.
  - Sessions JSON row: `"stoker_id"` and `"chain"`, both `omitempty`, so host-only output is unchanged.
  - `matchblox status --text`: one line per chain unit under `stoker:`, `  chain <name> (<id>): <state> · plan <done>/<all>`.

- [ ] **Step 1: Write the failing tests** — `TestStateCarriesChainStokers`, `TestSessionsJSONMarksChainStoker`, `TestSessionsJSONUnchangedWithoutFamily` (compare to today's golden).
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(status): the family of stokers in state, sessions and status (#304)`

### Task 11: Sessions: the head row, `a` and the start sheet

**Files:**
- Modify: `internal/app/fuse.go`, `internal/app/sessions.go`, `internal/app/sheet.go`, the `a` key path (find with `rg -n "mk.Answer|Answer:" internal/app`)
- Test: `internal/app/fuse_test.go`, `internal/app/sheet_test.go`, golden files under `internal/app/testdata`

**Interfaces:**
- Consumes: `State.Stokers` (Task 10).
- Produces:
  - `func (m Model) headOf(k chainKey) (state.ChainStoker, bool)`.
  - The fuse draws the head row first in its chain: `✦ stoker   <name> · plan <done>/<all>[ · asks: <question>]`, glyph by state as for sessions. The chain's group is the neediest of its rows and its head.
  - `a` on the head row: its pane in the chat window, as for any session; with its run off, the start sheet. `a` on a fuse without a head: the start sheet. The sheet reuses the launch sheet with the title and prompt texts, the go chips, and sends `stoker:new`.

- [ ] **Step 1: Write the failing tests**
  - Golden: chain with a head row at 150 and 44 columns; a head that asks moves the chain to WAITS FOR YOU.
  - `TestAOnHeadOpensStokerPane`, `TestAOnHeadWithoutRunOpensSheet`, `TestAOnBareFuseOpensSheet`, `TestSheetSendsStokerNew` (Text JSON has chain, words, grants).
  - Every existing golden file unchanged.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(console): talk to a chain through its stoker (#304)`

### Task 12: The STOKER tab, the dock and the header show the family

**Files:**
- Modify: `internal/app/stokertab.go`, `internal/app/stokerdock.go`, `internal/app/dock.go`, `internal/app/view.go` (header word)
- Test: `internal/app/stokertab_test.go`, golden files

**Interfaces:**
- Produces:
  - STOKER tab: the host card, then one card per chain unit titled `chain <name>`. `Enter` opens that unit's pane (`tmux -L matchblox attach -t stoker-<id>` / popup as today). `Q` + `y` quits that unit only (act `stoker:quit` with `Text` = id). CLI: `matchblox stoker quit --chain <who>`; with no `--chain`, today's host quit.
  - Dock and header: `stoker · <n> chains` when `n ≥ 1`.

- [ ] **Step 1: Write the failing tests** — golden of the tab with two chain cards; `TestQuitQuitsOneUnit`; `TestCLIStokerQuitChain`; `TestDockCountsChains`; golden of tab/dock without chains unchanged.
- [ ] **Step 2–4:** run, implement, run `go test ./...`.
- [ ] **Step 5: Commit** `feat(console): the STOKER tab, dock and header show the family (#304)`

### Task 13: The landing page and the docs

**Files:**
- Modify: `www/index.html` (the `.chain` section), `www/app.js` (chain animation, around line 502), `www/styles.css`
- Modify: `docs/stoker.md` (new section "A family of stokers"), `docs/agents.md` (chains), `docs/limits.md`, `docs/config.md` (`max_chains`), `docs/keys.md` (`a` on a head row), `CHANGELOG.md`
- Test: the docs check (`go test ./internal/docscheck`), headless render

**Interfaces:**
- Produces: in the chain section, a stoker mark at the head of each row and a new beat after the fire domino: the night stoker lights the head of each row, then each head lights its matches. Copy: `Talk to a chain. Its stoker answers.` and the commands `matchblox stoker new --chain <who>` and `matchblox stoker order <chain> "<text>"`. `prefers-reduced-motion` shows the lit end state with no motion.

- [ ] **Step 1:** Write the docs sections from the spec's "The experience", with the fixed strings above.
- [ ] **Step 2:** Run `go test ./internal/docscheck` — PASS.
- [ ] **Step 3:** Build the landing beat. Render `www/index.html` in headless Chrome, dark, light and at 390 px; save screenshots to the scratchpad and check the beat and its reduced-motion end state.
- [ ] **Step 4:** Run `rg -n -i -f ~/.config/builder/denylist.txt www docs` — no match, and read the diff of `www/` for any other product's name.
- [ ] **Step 5: Commit** `docs(www): the landing page and the docs show the family of stokers (#304)`

### Task 14: Prove it live

- [ ] **Step 1:** Build: `go build -o /tmp/claude-…/scratchpad/mb ./cmd/matchblox`. On a throwaway tmux server, start two sessions in one repo, chain them, run `mb stoker new --chain <a> "say hello to each match"` with a fake agent command (`[stoker] agent = "cat"` style) and check: `stoker-s1` exists, the brief is typed, `mb sessions --json` shows the head, `mb status --text` shows the chain line.
- [ ] **Step 2:** Capture the console at 150 and 44 columns with the head row.
- [ ] **Step 3:** Record what ran and what did not in the PR body ("Proven" / "Not proven").
