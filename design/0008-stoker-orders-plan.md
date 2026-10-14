# The stoker takes orders: implementation plan (#69)

> **Superseded in part (2026-10-14).** The plan gate this plan builds (`stoker:go`, `stoker:drop`, the sheet's plan view, `planning` / `planned`) was removed on 2026-10-14: an order runs at once, the stoker keeps its own plan, and the gos are the builder's chips. See the section "Revised 2026-10-14: no plan gate" in `design/0007-stoker-orders.md`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship #69 as one PR. The stoker takes an order in plain words at launch, an agent the user starts in its own pane carries it out through the sessions by the user's `ways.md`, and every step lands on a STOKER card. Sessions shows why each quiet session is idle, `z` parks one, and `g` groups the list. The landing page, the README replay and a visible Duo `Folded · Open` switch show all of it.

**Architecture:** matchblox stays deterministic and starts no agent. The service gains holds, idle reasons, orders, wakes and a driven-pane file. A new set of CLI commands gives any agent its hands over the existing hello → `KindAct` → `KindResult` path. The console gains a launch sheet, a STOKER tab and three keys. A PreToolUse Bash hook reads the driven-pane file to deny merges, releases and deploys the order did not grant.

**Tech Stack:** Go 1.26, Bubble Tea v2, Lip Gloss v2, BurntSushi/toml, tmux, the existing replay test, plain HTML/CSS/JS on www.

**Spec:** `design/0007-stoker-orders.md` (= the body of issue #69). Mock: https://claude.ai/artifact/1cbN3jcXRZFc1aCMUJKddp. Where the mock and the spec differ (sort key), the spec wins.

## Global Constraints

- matchblox starts no agent and calls no model. It needs no API key.
- Every text typed into a pane goes through `panes.CheckAnswer` (`MaxAnswer = 500`). A brief or a wake longer than that is written to a file in the state dir and the pane gets one line: `Read <path> and follow it.`
- No action runs without its exact command on the screen (design/0001 §2.5). Each card step keeps its `Argv`.
- The stoker never ends, kills or interrupts anything. A hold queues one message and never sends Esc or Ctrl+C.
- `internal/app` imports no `hooks`, `setup`, `ships`, `stoker`, `service` (internal/arch_test.go). New types reach the console as `proto` aliases.
- The hook stays under 50 ms and never blocks on the service.
- Every new key is in a Key table of docs/keys.md (`TestTheDocsNameEveryKeyAndOnlyRealOnes`). Every new `matchblox …` command is in `commands`/`commandFlags` (`TestTheDocsShowOnlyRealCommands`).
- Copy, verbatim from the spec:
  - Hold message: `finish this step, then stop and wait`. Release message: `carry on`.
  - Hook deny reason: `the stoker's order did not give the go for this. Give it on the stoker's card (y) or stop the stoker.`
  - Launch sheet prompt: `How should I run tonight?` (night) / `How should I run?` (day).
  - No stoker pane door: `Start a stoker agent: tmux new -s stoker claude`.
  - Sealed refusal: `The night is sealed: n stops it, then launch a new one.`
  - Grants line: `✓ merges: you said "<the words>"`. Asks line: `! releases and deploys still ask you`.
- Defaults: `[stoker] pane = ""` (none), `max_steps = 60`. Wakes at most one a minute. Usage lines every 10% of the 5h and the week. Night-end wake 10 min before `night_ends`.
- Commit types: feat, fix, chore, test, docs, refactor, ci, perf, build, revert. `go test -race ./...` passes before each commit.
- Live checks only on a private tmux socket (`tmux -S <scratch>/tmux.sock`, every call). Never Martin's tmux, never his `matchblox serve`, no `go install`.

## Review Focus

1. **Two sessions share a name.** `matchblox send Weather …` must refuse and list the panes (`Weather is 2 sessions: %3, %7. Name the pane.`), never pick one. Owner: Task 4 (`TestResolveRefusesAmbiguousName`).
2. **A parked pane gets a new agent.** Holds key on the session ID. A new agent in the same pane is not parked. Owner: Task 1 (`TestHoldFollowsTheSessionNotThePane`).
3. **The stoker pane is configured but not running.** `stoker:order` returns `no stoker agent in "stoker": start it with tmux new -s stoker claude` at once. It never waits for a plan. Owner: Task 7 (`TestOrderWithoutStokerPaneFails`).
4. **The stoker acts on itself.** The stoker pane is never held, parked by the stoker, compacted by an order, nor reported in a wake about itself. Owner: Task 5 (`TestStokerCannotHoldItself`) and Task 8 (`TestWakeSkipsTheStokerPane`).
5. **The driven-pane file is missing or bad during a run.** The hook denies a guarded command in every pane while `stoker.json` says a run is on, and allows it otherwise. Owner: Task 9 (`TestHookFailsClosedOnBadDrivenFile`).

---

### Task 1: Holds: park and the stoker's hold, in the service

**Files:**
- Create: `internal/holds/holds.go`, `internal/holds/holds_test.go`
- Modify: `internal/state/state.go` (`Doc.Holds`), `internal/proto/proto.go` (alias `Hold = holds.Hold`), `internal/service/service.go` (field `Holds *holds.Store`, publish), `internal/service/actions.go` (acts `park:`, `hold:`, `release:`; `messageable` refusal), `internal/stoker/stoker.go` (`Plan` skips held), `cmd/matchblox/main.go` (open the store in `newService`)
- Test: `internal/service/holds_test.go`

**Interfaces:**
- Produces:
  - `type Hold struct { SessionID string "session_id"; Pane, Name string; By string /* "you" | "stoker" */; Why string; At time.Time }`
  - `func Open(dir string) (*Store, error)` (file `holds.json`, atomic write like `stoker.Store.Set`)
  - `func (s *Store) Set(h Hold) error`, `func (s *Store) Release(sessionID string) (Hold, bool, error)`, `func (s *Store) List() []Hold`, `func (s *Store) Of(sessionID string) (Hold, bool)`
  - Acts: `park:<pane>` toggles a hold `By:"you"`. `hold:<pane>` (Text = why) sets `By:"stoker"` and sends `finish this step, then stop and wait` through `s.message`. `release:<pane>` removes it and, for `By:"stoker"`, sends `carry on`.
  - `messageable` refuses a held session: `"%s is parked: z lets it go"` / `"%s is held by the stoker: z lets it go"`.

- [ ] **Step 1: Write the failing tests**
  - `TestHoldsRoundTrip`: Set two, Release one, reopen the dir, `List()` has the other.
  - `TestHoldFollowsTheSessionNotThePane`: park session `s1` in `%3`. A snapshot where `%3` runs `s2` shows no hold for `s2`, and `Doc.Holds` drops `s1` once it is gone for 24 h.
  - `TestParkTogglesAndRefusesMessages` (service): act `park:%3` → `Doc.Holds` has it. Act `message:%3` → `Err` = `"flint is parked: z lets it go"`. Act `park:%3` again → gone.
  - `TestStokerHoldQueuesOneMessage`: act `hold:%3` Text `not priority` while the session is busy → the fake runner got exactly `panes.Send("%3","finish this step, then stop and wait")` and no `Escape`/`C-c`.
  - `TestPlanSkipsHeld`: `stoker.Plan` with a held session due for compact returns no step for it.
- [ ] **Step 2: Run them, they fail.** `go test -race ./internal/holds ./internal/service ./internal/stoker -run 'Hold|Park|PlanSkipsHeld'` → FAIL (undefined).
- [ ] **Step 3: Implement.** `Plan` gains a `held map[string]bool` parameter (session IDs); update its callers. A hold whose session is not in the snapshot for 24 h is pruned on publish.
- [ ] **Step 4: Run them, they pass.** Same command → PASS. Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(service): park a session, or have the stoker hold it, and nothing types into it`

### Task 2: Why a session is idle

**Files:**
- Create: `internal/idle/idle.go`, `internal/idle/idle_test.go`
- Modify: `internal/sample/agent.go` (`Usage.Failed`), `internal/sample/sampler.go` (`Session.Failed`), `internal/ships/track.go` + `ships.go` (`Ship.Failed`), `internal/state/state.go` (`Doc.Reasons map[string]idle.Reason`, keyed by pane), `internal/proto/proto.go` (alias `IdleReason`), `internal/service/service.go` (fill on publish)

**Interfaces:**
- Produces:
  - `type Reason struct { Word string "word"; Tone string "tone" /* accent|ok|warn|neg|faint */ }`
  - `func Of(s sample.Session, waits bool, ship *ships.Ship, now time.Time) Reason`, zero Reason for a session that is not quiet.
  - `Session.Failed string` (`json:"failed,omitempty"`): set when the last turn ended on an API error or a `tool_result` with `is_error` and no assistant text after it. Value: the error's first line, cut to 40 runes.
  - `Ship.Failed string` (`json:"failed,omitempty"`): the failed check's name, where track.go builds `"CI failed · "+…`.
- Precedence and words (first match wins):
  1. ship green and waits for you → `#<n> waits your merge`, accent
  2. ship stuck with `Failed` → `CI red · <Failed>`, neg
  3. ship merged or live → `#<n> merged · done`, ok
  4. `s.Stopped()` → `<window> limit · back <limits.When(s.Out)>`, warn (`5h` or `week` from the account's window that is out)
  5. `s.Failed != ""` → `stuck: <Failed>`, neg
  6. Progress 100, or no Progress and no ask → `out of work`, faint
  7. else `idle`, faint

- [ ] **Step 1: Write the failing tests.** `TestReasonPrecedence` is table-driven, one row per rule above, plus one row where rules 1 and 4 both hold (rule 1 wins). `TestTranscriptFailedTurn` and `TestTranscriptRecoveredTurnIsNotFailed` (sample, using testdata transcripts). `TestShipFailedName` (ships).
- [ ] **Step 2: Run them, they fail.** `go test -race ./internal/idle ./internal/sample ./internal/ships -run 'Reason|Failed'` → FAIL.
- [ ] **Step 3: Implement.** `Of` reads only its inputs. The service fills `Doc.Reasons` for every quiet session, using the ship from the same claim the console's `shipOf` uses (pane, then session ID).
- [ ] **Step 4: Run them, they pass,** then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(service): every quiet session says why it is idle`

### Task 3: Sessions: the reason in the row, `z`, the HELD group, `g`

**Files:**
- Modify: `internal/app/sessions.go` (groups, rows), `internal/app/queue.go` (`stateWord`), `internal/app/lists.go` (sort within groups; column `idle` labelled `newest`), `internal/app/model.go` (`sessGroup`, keys `z`, `g`), `docs/keys.md`, `docs/console.md`
- Test: `internal/app/sessions_test.go`, `internal/app/lists_test.go`, goldens under `testdata/screens/`

**Interfaces:**
- Consumes: `proto.State.Reasons`, `proto.State.Holds` (Tasks 1, 2), act `park:<pane>` (Task 1).
- Produces:
  - `const ( groupNeed = iota; groupStatus; groupRepo; groupNone )` and `Model.sessGroup int`, replacing `grouped()`'s tie to the default sort.
  - Need groups: `WAITS FOR YOU`, `WORKING`, `QUIET`, `HELD`. Status groups: the state word. Repo groups: the session's repo (its git toplevel's base name, else its cwd's base name).
  - Held rows read `parked by you` or `held by stoker · <why>`, glyph `⏸`.
  - A quiet row shows `Reason.Word` in its tone, where the state word is, and takes the wait column's cells too.
  - `g` cycles the grouping and flashes `group: need|status|repo|none`. `z` sends `park:<pane>` for the selected session.

- [ ] **Step 1: Write the failing tests.**
  - `TestIdleReasonInRow` at 44, 86 and 132 columns: `#4556 waits your merge` shows in full.
  - `TestZParksAndHeldIsLast`: `z` sends `park:%3`. With that hold in the state, the row is in `HELD · 1`, the last group.
  - `TestGCyclesGrouping`: need → status → repo → none → need. Sort `s` applies inside each group.
  - `TestSortSaysNewest`: the flash reads `sort: newest`, and `S` reads `sort: newest, reversed`.
- [ ] **Step 2: Run them, they fail.** `go test -race ./internal/app -run 'IdleReason|ZParks|GCycles|SortSaysNewest'` → FAIL.
- [ ] **Step 3: Implement,** then add `z` and `g` to the Sessions Key table in docs/keys.md, and the groups to docs/console.md.
- [ ] **Step 4: Run them, they pass.** Update the goldens with `go test ./internal/app -run TestScreens -update`, read the diff, then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(console): z parks a session, g groups the list, and quiet rows say why`

### Task 4: CLI reads: `sessions`, `ships`, `usage`

**Files:**
- Create: `cmd/matchblox/cli.go`, `cmd/matchblox/cli_test.go`
- Modify: `cmd/matchblox/main.go` (`commands`, dispatch), `cmd/matchblox/docs_test.go` (`commandFlags`)

**Interfaces:**
- Consumes: `fromService(path, maxAge) (state.Doc, bool)` (main.go:620).
- Produces:
  - `matchblox sessions --json`: one object per session, with `name, pane, target, status, word, reason, held, context_pct, plan_pct, account, ship` (`{repo, number, stage, kind, note}` or null), `stoker` (true for the stoker pane).
  - `matchblox ships --json` prints `Doc.Ships`. `matchblox usage --json` prints `Doc.Limits`.
  - `func resolve(doc state.Doc, who string) (sample.Session, error)` matches a pane id (`%3`), a target (`app:1.1`) or a name. Two matches → `"<who> is <n> sessions: <panes>. Name the pane."`. None → `"no session named <who>"`.
  - Without `--json` each prints one aligned line per row. No service → the state file fallback `status` uses. Older than 15 s → error `matchblox is not running: matchblox serve`.

- [ ] **Step 1: Write the failing tests.** `TestSessionsJSONCarriesReasonAndHold`, `TestResolveRefusesAmbiguousName`, `TestResolveByPaneTargetName`, `TestCLINeedsAFreshService`.
- [ ] **Step 2: Run them, they fail.** `go test -race ./cmd/matchblox -run 'SessionsJSON|Resolve|FreshService'`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(cli): sessions, ships and usage as JSON, for any agent`

### Task 5: CLI hands: `send`, `compact`, `hold`, `release`, and who sent it

**Files:**
- Modify: `cmd/matchblox/cli.go`, `internal/proto/proto.go` (`Act.From`), `internal/service/actions.go` (act `compact:<pane>`, the stoker check, `max_steps`), `internal/service/stoker.go`, `internal/config/config.go` (`Stoker.Pane`, `Stoker.MaxSteps`), `config.example.toml`
- Test: `cmd/matchblox/cli_test.go`, `internal/service/stokerhands_test.go`

**Interfaces:**
- Consumes: `resolve` (Task 4), acts `message:`, `pick:`, `hold:`, `release:` (Task 1).
- Produces:
  - `proto.Act.From string` (`json:"from,omitempty"`): the CLI sets it to `$TMUX_PANE`.
  - `func (s *Service) fromStoker(a proto.Act) bool`: true when `a.From` is the pane of the tmux session named `cfg.Stoker.Pane`.
  - A stoker act is refused while the stoker rests (`"the stoker rests: a person works"`), after `max_steps` sends in the run (`"the run sent its <n> messages: max_steps"`), and on its own pane (`"the stoker does not act on itself"`).
  - `compact:<pane>` runs the advice compact action under the compactor's guards (at rest, not waiting, 30 min). A refusal says which guard.
  - CLI: `matchblox send <who> "<text>"`, `matchblox send <who> --pick <n>`, `matchblox compact <who>`, `matchblox hold <who> --why "<text>"`, `matchblox release <who>`. Exit 1 with the service's `Err` on stderr.
  - `config.Stoker{ NightEnds string "night_ends"; Pane string "pane"; MaxSteps int "max_steps" }`, with defaults `"07:00"`, `""`, `60`.

- [ ] **Step 1: Write the failing tests.** `TestStokerCannotHoldItself`, `TestStokerActsRefusedWhileResting`, `TestStokerMaxSteps` (61st send refused), `TestCompactActKeepsTheGuards` (busy pane refused), `TestSendPickRoutesToPick`, `TestExampleLoads` (extended for `pane`, `max_steps`).
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.** A stoker `message:` that succeeds is recorded in the run as an `Entry{Rule: "order", Argv: steps}` (Task 7 reads it).
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(cli): send, compact, hold and release, with the stoker's own limits`

### Task 6: Your ways: the file, its recipes, the door

**Files:**
- Create: `internal/stoker/ways.go`, `internal/stoker/ways_test.go`, `internal/setup/ways.go`, `internal/setup/ways_test.go`, `docs/ways.md`
- Modify: `internal/doors/doors.go` (`Ways`), `internal/setup/setup.go` (`Doors`, `Open`, `known`), `internal/config/config.go` (`WaysPath()`)

**Interfaces:**
- Produces:
  - `func WaysPath() string` = `filepath.Join(filepath.Dir(config.Path()), "ways.md")`.
  - `func Recipes(md []byte) []string`: the names of `# Recipe: <name>` headings, in file order, trimmed.
  - Door `ways`, title `Write your ways`. It is open when the file does not exist and writes the starter below. It never overwrites (Done when the file exists).
  - Starter `ways.md`, exact:

    ```markdown
    # Standing rules

    - When a session opens a PR: compact it, then send it /review.
    - Past 80% of the week: hold every session that is not a priority until the week resets.
    - Never touch a session I parked.

    # Priority

    Name the sessions or the issues that come first.

    # Recipe: merge-order

    1. Ready = a PR that is green, or only behind main.
    2. Order: fewest conflicts first, and a stacked PR after its base.
    3. Tell each session to read its issue, watch the PR before it, then rebase and merge on green.
    4. A failed rebase: ask me, stop that lane, and go on with the rest.
    ```

- [ ] **Step 1: Write the failing tests.** `TestRecipesInOrder`, `TestRecipesIgnoresOtherHeadings`, `TestWaysDoorWritesStarterOnce`, `TestWaysDoorNeverOverwrites`.
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement,** and write docs/ways.md: what the file is, the two conventions, the starter, and Martin-neutral examples.
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(setup): write your ways, the stoker's rules and recipes in your words`

### Task 7: Orders: launch, plan, go, steps, restart

**Files:**
- Create: `internal/stoker/order.go`, `internal/stoker/order_test.go`, `internal/stoker/brief.go`, `internal/service/orders.go`, `internal/service/orders_test.go`
- Modify: `internal/stoker/stoker.go` (`Mode.Order`, `Entry.Step`, `Entry.State`), `internal/stoker/store.go` (`View.Recipes`), `internal/service/actions.go` (route `stoker:order|plan|go|drop|step|grant`), `cmd/matchblox/cli.go` (`matchblox stoker plan|step`)

**Interfaces:**
- Consumes: `fromStoker` (Task 5), `Recipes`, `WaysPath` (Task 6), `s.message` (existing).
- Produces:
  - `type Order struct { ID string; Words string; Night bool; At time.Time; State string /* planning|planned|running|dropped|done */; Plan []PlanStep; Grants []string /* "merge" | "release" | "deploy" | "merge:#<n>" */; Unguarded []string }`
  - `type PlanStep struct { N int; Session, Text string }`
  - `Entry.Step int` (`json:"step,omitempty"`), `Entry.State string` (`json:"state,omitempty"`; doing|done|failed|waits), `Entry.Needs string` (`json:"needs,omitempty"`, e.g. `merge:#4556`)
  - `func Brief(o Order, ways, cmds string, prior []Entry) string`: the brief file's text. It must name the role, the ways path, every CLI command from Tasks 4, 5 and 7, the run id, and "write your plan with `matchblox stoker plan` and wait for go". On a restart it also lists the old plan with each step's last state.
  - Acts (console → service): `stoker:order` with Text = `{"words":…, "night":bool}`, `stoker:go`, `stoker:drop`, `stoker:grant` with Text = step number.
  - Acts (stoker → service, `From` must be the stoker pane): `stoker:plan` with Text = `{"steps":[…],"grants":[…]}`, `stoker:step` with Text = an `Entry` subset.
  - CLI: `matchblox stoker plan` reads stdin lines `<n>. <session>: <text>` and optional `grant: merge`. `matchblox stoker step <n> --state <s> [--needs merge:#<n>] "<text>"`. `matchblox stoker step 0 --rule "<rule>" …` logs a standing rule.
  - Service behaviour:
    - `stoker:order` with no `cfg.Stoker.Pane`: today's `stoker:on|night`, and the Order holds only the words.
    - With a pane: write `<state>/stoker-brief.md`, type `Read <path> and follow it.` into the stoker pane, set State planning.
    - On a night that runs: refuse with the sealed text.
    - By day: restart (the old Order is kept as `prior` in the brief).
    - `stoker:go` turns the stoker on (or a night), sets running and types `go`.
    - `stoker:drop` before go types `drop the order` and clears it.
    - A plan's session names that are Codex or OpenCode go to `Unguarded` when there is no merge grant.

- [ ] **Step 1: Write the failing tests.**
  - `TestOrderWithoutStokerPaneFails`
  - `TestOrderTypesOneReadLine` (the text ≤ 500 bytes, and the file holds the brief)
  - `TestPlanFromStokerOnly` (a plan act from another pane is refused)
  - `TestGoStartsNightAndTypesGo`
  - `TestNightIsSealed`
  - `TestDayOrderRestartsWithPrior`
  - `TestDropBeforeGo`
  - `TestStepLogsToRun`
  - `TestPlanStdinParse`
  - `TestBriefNamesEveryCommand` (it checks against the `commands` list)
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(stoker): it takes an order at launch, plans it, and logs every step`

### Task 8: Wakes

**Files:**
- Create: `internal/stoker/wake.go`, `internal/stoker/wake_test.go`
- Modify: `internal/service/stoker.go` (`stoke` sends wakes), `internal/service/orders.go`

**Interfaces:**
- Produces:
  - `type Seen struct { Turns map[string]time.Time; Stages map[string]int /* repo#n → stage */; Usage map[string]int /* account/window → pct/10 */ }`
  - `func Changes(prev, cur Seen, sessions []sample.Session, stokerPane string) []string`: one line per change, e.g. `Weather's turn ended: idle, #4556 waits your merge`, `new PR #4570 by Takeoff`, `#4551 moved to green`, `claude week passed 80%`. Changes about the stoker pane itself are skipped.
  - Service: pending lines are kept. Once the stoker pane is at rest (`stokable(pane,false)==nil`) and the last wake is ≥ 1 min old, it types `matchblox: <lines joined by "; ">`, or the file form when that is over 500 bytes. 10 min before `Until` it sends `the night ends at <hh:mm>`. On `stoker:rest` it sends `you rest: a person works now`. On wake it sends `back on: <lines since the rest>`. Nothing is sent while the stoker rests.

- [ ] **Step 1: Write the failing tests.** `TestWakeSkipsTheStokerPane`, `TestChangesTurnShipUsage`, `TestWakesCoalesceToOneAMinute`, `TestNoWakeWhileResting`, `TestBackOnListsWhatChanged`, `TestLongWakeGoesToFile`.
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(stoker): matchblox wakes the stoker when something it may care about happens`

### Task 9: The merge guard

**Files:**
- Create: `internal/stoker/driven.go`, `internal/stoker/driven_test.go`
- Modify: `internal/hooks/hook.go` (parse the Bash `command`), `cmd/matchblox/main.go` (`hook` writes a deny), `internal/setup/claude.go` (`matchers` becomes `map[string][]string`, PreToolUse gets `AskUserQuestion` and `Bash`), `internal/service/orders.go` (write `driven.json`, take a grant when the hook reports it used)
- Test: `cmd/matchblox/hook_test.go`, `internal/setup/claude_test.go`

**Interfaces:**
- Produces:
  - `driven.json` in the state dir: `type Driven struct { Run string; Panes map[string][]string /* pane → grants */ }`. It is written on each stoker send and each grant, and removed when the run ends.
  - `func ReadDriven(dir string) (Driven, error)` and `func Guarded(cmd string) (what string, n int, ok bool)`. It matches `gh pr merge [<n>]` (what `merge`), `gh release create|upload|edit` (`release`) and `gh workflow run` (`deploy`), also after `&&`, `;` or `cd x &&`.
  - `func Allowed(grants []string, what string, n int) bool`: `merge` allows every merge, `merge:#<n>` allows that one, `release` and `deploy` allow their own.
  - The hook, on PreToolUse Bash, for a guarded command in pane `$TMUX_PANE`:
    - If `driven.json` reads OK and the pane is in it without a grant, it denies.
    - If it reads bad or is missing while `stoker.ReadMode` says On, it denies.
    - Otherwise it allows.
    - Deny = stdout `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<deny reason>"}}`.
    - It still notifies the service as today. The service takes a used `merge:#<n>` grant out.
  - `stoker:grant <step>` adds that step's `Needs` to its session's pane and wakes the stoker with `go given for step <n>`.
  - The hooks door opens again on old installs because `runsMatchblox` finds no Bash entry. Nothing extra is needed beyond the matcher.

- [ ] **Step 1: Write the failing tests.**
  - `TestGuardedCommands` (table: `gh pr merge 12 --squash`, `cd x && gh pr merge`, `gh pr view 12` not guarded, `gh release create v1`, `gh workflow run Release`)
  - `TestHookDeniesUngrantedMerge`
  - `TestHookAllowsGrantedStep`
  - `TestHookAllowsUndrivenPane`
  - `TestHookFailsClosedOnBadDrivenFile`
  - `TestHookStaysUnder50ms`
  - `TestHooksDoorAddsBashMatcher`
  - `TestGrantIsUsedOnce`
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(hooks): a merge, release or deploy the order did not give stops in the session`

### Task 10: The launch sheet

**Files:**
- Create: `internal/app/sheet.go`, `internal/app/sheet_test.go`
- Modify: `internal/app/peek.go` (lift the frame out of `peekBox` into `func boxFrame(title []span, lines []string, foot []span, bw int) []string`), `internal/app/model.go` (`sheet *sheet`, a branch in Update next to the peek one, `n`/`f`/`o`), `internal/app/stoker.go` (`stokerKey` opens the sheet), `internal/app/view.go` (overlay like the peek), `docs/keys.md` (a Key table "The launch sheet")

**Interfaces:**
- Consumes: `View.Order`, `View.Recipes` (Task 7), acts `stoker:order|go|drop` (Task 7).
- Produces:
  - `type sheet struct { night, restart bool; text string; editing bool; chip int }`
  - Keys in the sheet: typed text, `backspace`, `tab` puts the next recipe name into the text, `enter` sends the order (editing) or `stoker:go` (plan shown), `e` back to editing, `esc` sends `stoker:drop` and closes.
  - Body: the prompt, the order, the recipe chips, then a hairline and `PLAN` with the plan lines, the grants line and the asks line, once `View.Order.State == planned`. While planning it shows `writing the plan…`. With no stoker pane it shows the door line and `enter` starts today's run.
  - `n` while a night runs ends it (as today). `f` while on turns it off (as today). `o` opens the sheet with `restart`, only while a day run is on. In a night it flashes the sealed text.

- [ ] **Step 1: Write the failing tests.**
  - `TestNOpensSheetAndSendsOrder`
  - `TestSheetShowsPlanAndGo`
  - `TestSheetEditRewrites`
  - `TestSheetEscDrops`
  - `TestSheetDoorWithoutStokerPane`
  - `TestOIsSealedAtNight`
  - `TestTabAddsRecipe`
  - Goldens: `sheet` at 44x31, 104x38 and 132x32.
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(console): n and f ask for the orders, and show the plan before it runs`

### Task 11: The STOKER tab

**Files:**
- Create: `internal/app/stokertab.go`, `internal/app/stokertab_test.go`
- Modify: `internal/app/tabs.go` (`tabStoker`, appended last so digits 1-6 keep their tabs), `internal/app/model.go` (`c` → `setTab(tabStoker)`, `y` on a waiting step), `internal/app/stokerdock.go` (whole-card view moves here), `docs/keys.md`, `docs/stoker.md`

**Interfaces:**
- Consumes: `View.Order`, `View.Run` entries with `Step`/`State`/`Needs` (Task 7), act `stoker:grant` (Task 9).
- Produces:
  - Body, top to bottom:
    1. `NIGHT · hh:mm → hh:mm` or `STOKER · since hh:mm`, and `sealed` at night.
    2. The words, quoted.
    3. `PLAN <done> of <n>`, then each step with its time, glyph (`✓ ◐ ✕ ⏸ ○`), session and text, plus the last entry's text as its outcome.
    4. `STANDING RULES` (step 0 entries).
    5. Today's compact entries, with their RESUME lines.
    6. When the run ended: `DONE <d> of <n> · <w> WAITS FOR YOU`.
  - `⏎` on a step shows its `Argv` lines. `y` on a `waits` step sends `stoker:grant <n>`. `a` on a failed step opens the peek on its session.

- [ ] **Step 1: Write the failing tests.** `TestStokerTabShowsWordsPlanAndRules`, `TestYGrantsAWaitingStep`, `TestEnterShowsTheCommand`, `TestAOnFailedStepOpensPeek`, `TestCOpensTheTab`. Goldens `stoker` at the three sizes, night and morning.
- [ ] **Step 2: Run them, they fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them, they pass.** Then `go test -race ./...`.
- [ ] **Step 5: Commit.** `feat(console): the stoker's card is a tab: your words, its plan, every step`

### Task 12: The acceptance run

**Files:**
- Create: `internal/service/orders_e2e_test.go`

**Interfaces:**
- Consumes: everything above. The fake stoker is a Go func that watches what the fake runner types into the stoker pane and answers with the acts the CLI would send (`From` = the stoker pane). The fake ships source steps three PRs `#1 ← #2 ← #3` (stacked) through open → ci → green → merged on the fake clock. The fake sessions merge when told and run the hook check through `Guarded`/`Allowed` with `driven.json`.

- [ ] **Step 1: Write the failing tests.**
  - `TestMergeOrderLandsTheStack`: the order "set the ready ones up in merge order and merge them on green", with grant `merge`. All three are merged in the order 1, 2, 3. The run's steps all end `done`.
  - `TestMergeOrderStopsOnRed`: #2's CI goes red. Step 2 is `failed` with `CI red · test`, and #3 is never told to merge.
  - `TestWithoutGrantEachMergeWaits`: the steps end `waits` with `needs merge:#<n>`, and the hook check denies. `stoker:grant` on step 1 lets exactly #1 through.
- [ ] **Step 2: Run them, they fail** (before the wiring is complete), then pass. `go test -race ./internal/service -run 'MergeOrder|WithoutGrant'`.
- [ ] **Step 3: Commit.** `test(service): a stack of three PRs lands in order through the stoker, or stops on red`

### Task 13: Docs and changelog

**Files:**
- Modify: `docs/stoker.md` (new sections: the stoker agent, orders, the run, the card, merges and the guard; the old "Turn it on" table gets the sheet), `docs/keys.md`, `docs/config.md` via `config.example.toml`, `docs/agents.md` (the stoker pane), `docs/ways.md` (Task 6), `README.md` (one line on orders), `CHANGELOG.md` (Unreleased), `mkdocs.yml` nav (ways)

- [ ] **Step 1: Write the docs.**
- [ ] **Step 2: Run the docs tests.** `go test -race ./internal/app ./cmd/matchblox -run 'Docs'` → PASS.
- [ ] **Step 3: Commit.** `docs: the stoker takes orders, follows your ways, and logs every step`

### Task 14: The replay, the landing page, the Duo switch

**Files:**
- Modify: `cmd/matchblox/replay_test.go` (new steps), `cmd/matchblox/replay_world_test.go` (a stoker pane named `stoker` in the demo world, plus a fake stoker as in Task 12), `www/index.html`, `www/app.js`, `www/styles.css`, regenerated `www/demo/*`

**Interfaces:**
- New replay steps, in order, after "who waits":
  1. `why it is idle`: the quiet rows with their reasons.
  2. `z parks`
  3. `g groups`
  4. `n: tonight's orders` (types the merge-order order)
  5. `the plan` (the fake stoker writes it)
  6. `go`
  7. `the night at work` (the STOKER tab, mid-run)
  8. The existing `the morning card` becomes the STOKER tab's morning view.
- The frames stay under the 150 KB gzip budget. Drop or shorten an older step if they don't.
- Switch: `<div class="foldswitch" role="group" aria-label="The phone"><button type="button" aria-pressed="true" data-fold="phone">Folded</button><button type="button" aria-pressed="false" data-fold="duo">Open</button></div>`, a sibling after `.fold` in `.stage`. It calls the same `set(to)` that the tap uses and stays in sync with `shape(to)`. `UNFOLDS_AT` stays.

- [ ] **Step 1: Write the failing test.** Add the steps and run `go test ./cmd/matchblox -run TestReplay` → FAIL on the new screens.
- [ ] **Step 2: Wire the world** until each step's `ok` holds, then `go test ./cmd/matchblox -run TestReplay -update`.
- [ ] **Step 3: Add the switch** to www. Check it with headless Chromium through Node (`require("/home/blox-master/dev/bloxdev/blox-web/node_modules/.pnpm/playwright@1.62.1/node_modules/playwright/index.js")`): the switch is visible at 390 and 1280 px, clicking `Open` sets `.fold.open`, keyboard Enter works, and there are no console errors.
- [ ] **Step 4: Run everything.** `go test -race ./...`, `go vet ./...`, `govulncheck ./...`.
- [ ] **Step 5: Commit.** `feat(www): the replay shows orders, holds and why a session is idle, and the Duo says it opens`

---

## After the tasks

1. A whole-branch review by an adversarial subagent, then fix and commit what it finds.
2. One live check on a private tmux socket, with a real Claude Code stoker in a `stoker` session and two throwaway sessions. Give an order and watch the plan, go, a wake and a step on the card. Never Martin's tmux.
3. Push, then open the PR (conventional title `feat: the stoker takes your orders and follows your ways (#69)`). Body: `Closes #69`, the cleanups, and what the live check showed. Merge, release and install wait for Martin's word.
