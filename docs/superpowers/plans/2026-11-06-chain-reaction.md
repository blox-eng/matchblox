# Chain reaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Matches in one repo learn about each other, join a chain both sides agree to, hear each other's news through their hooks, and message only chain-mates; Sessions draws the chain as a fuse, and the landing page shows it.

**Architecture:** A new `internal/chains` package owns chains, requests and inboxes as plain data. The service holds one `chains.Store`, saves it as `chains.json`, publishes it in `state.Doc`, fills inboxes from hook events and HEAD reads, and answers a new `KindNews` request that the hook makes within its 30 ms budget. Sends go through the peer-send path of draft #297, ported here and gated to chain-mates. The console reads `Doc.Chains` to draw the fuse.

**Tech Stack:** Go (Bubble Tea console, tmux), Claude Code and Cursor hooks, static site (`www/`).

**Spec:** `docs/superpowers/specs/2026-11-06-chain-reaction-design.md` (approved 2026-11-06).

**Branches:** work on `feat/299-chain-reaction`, cut from `design/299-chain-reaction`, in a worktree that is not `~/dev/matchblox`. #297's code comes in by `git show origin/feat/peer-send:<path>`; never merge or push that branch.

## Global Constraints

- The hook never makes the agent wait: the news request has a 30 ms budget (`hookTimeout`). On a timeout the hook prints nothing and the news stays in the inbox.
- News reaches a busy match only through its hooks. Only a match at rest is typed into, and only claude and codex (`readsPrompt`).
- A chain forms only when both sides chain. `chain <who>` from one side shares nothing: no news, no send, no fuse.
- A request lapses after one hour, or when either match leaves the repo.
- Chains live on one host. A chain's matches are in one repo: two worktrees of one repo are one repo.
- The inbox holds at most 20 items, the oldest dropped first, with a count of the dropped ones ("and 3 more").
- Send limits from #297 stay: `PeerPairEvery = 30 * time.Second`, `PeerSendsPerHour = 20`.
- The tag is `[from <name> <pane>]`.
- Config: `[chains] notice = true`, `[chains] send = "chain"` (or `"off"`). No `[peers]` alias.
- Copy (verbatim):
  - notice: `[matchblox] <who> also works in <repo>, on <branch>.` then `  To hear from it, run: matchblox chain <who>`
  - request: `[matchblox] <who> asks to chain with you. To accept, run:` then `  matchblox chain <who>`
  - news head: `[matchblox] since your last turn:`
  - send refused: `<who> is not in your chain. Run ` + "`matchblox chain <who>`" + ` first.`
  - OpenCode refused: `<who> runs OpenCode, which matchblox cannot type into safely. It sees the chain in ` + "`matchblox chain`" + `.`
  - leave: `<who> left the chain: it moved to <repo>`
- Fuse glyphs: `╮` first, `┤` middle, `╯` last, `│` through child rows and the opened block, with `─` before each from 70 columns of list. Stub `─╴`, the selected row's stub `━╸`. Label suffix ` · chain of <n>`, and ` · <n> hidden` under a filter.
- No other product's name in code, docs or commits. Run the denylist scan before each commit. Signed Conventional Commits, never `--no-verify`.
- tmux tests only on throwaway servers (`tmuxtest.Server`).

## Review Focus

1. **A send while the target is mid-draft.** A person half-typed a prompt in a codex pane at rest. Expect: the typed message never submits the draft (`stokable` match), and on a mismatch the line is cleared and the send says not delivered. Pinned in Task 6.
2. **Two worktrees of one repo, one checkout per match.** Expect: they count as one repo for notices and chains, and HEAD moves are named per worktree. Pinned in Task 3 and Task 4.
3. **The service is down or restarts mid-chain.** Expect: the hook prints nothing within budget; after a restart, chains come back from `chains.json`, and members that are gone are pruned. Pinned in Task 2 and Task 5.
4. **A narrow phone with a waiting chain-mate.** Expect: at 44 columns the waiting row is whole, the fuse is one column, and the opened block keeps it. Pinned in Task 8.
5. **A renamed tmux window that impersonates a chain-mate.** Expect: the tag carries the pane id, so `[from peer-send %14]` cannot come from `%22`. Pinned in Task 6.

---

### Task 1: The chains package

**Files:**
- Create: `internal/chains/chains.go`, `internal/chains/chains_test.go`

**Interfaces:**
- Produces:
  - `type Member struct { SessionID, Pane string; PID int; Start uint64 }` with `func (m Member) Same(o Member) bool` (SessionID when both have one, else pane, pid and start).
  - `type Chain struct { ID string; Repo string; Members []Member }`
  - `type Request struct { From, To Member; At time.Time }`
  - `type Item struct { At time.Time; Text string }`
  - `type Store struct { Chains []Chain; Requests []Request; Off []Member; inbox map[string][]Item; dropped map[string]int }` (exported fields are what `chains.json` and `state.Doc` carry; the inboxes are in memory only).
  - `func (s *Store) Ask(from, to Member, repo string, now time.Time) (formed bool)`: forms the chain when `to` (or a member of `to`'s chain) already asked `from` (or a member of its chain); else records a request. Chains of both sides merge into one.
  - `func (s *Store) Leave(m Member)`; `func (s *Store) SetOff(m Member, off bool)` (off also leaves).
  - `func (s *Store) Prune(live func(Member) (repo string, ok bool), now time.Time) (left []Left)` with `type Left struct { Who Member; Repo string; Chain string }`: drops members that are gone or in another repo, requests past `RequestTTL = time.Hour` or across repos, and chains of one.
  - `func (s *Store) ChainOf(m Member) (Chain, bool)`; `func (s *Store) Mates(m Member) []Member`.
  - `func (s *Store) Post(to Member, text string, now time.Time)`; `func (s *Store) Take(m Member) []Item` (empties it; adds `and <n> more` when dropped).
  - `const InboxMax = 20`.
  - `func Load(path string) (*Store, error)` (missing file: empty store) and `func (s *Store) Save(path string) error` (temp file, then rename, mode 0600, as `stoker.WriteDriven` does).

- [ ] **Step 1: Write the failing tests** in `chains_test.go`:
  - `TestOneSideAloneFormsNoChain`: `Ask(a,b)` → `formed == false`, `ChainOf(a)` false, one request.
  - `TestBothSidesFormAChain`: `Ask(a,b)` then `Ask(b,a)` → formed, one chain with a and b, no requests.
  - `TestAMemberOfTheChainCanAccept`: chain {a,b}; `Ask(c,a)`; `Ask(b,c)` → chain {a,b,c}.
  - `TestTwoChainsMergeIntoOne`: chains {a,b} and {c,d}; `Ask(a,c)`, `Ask(d,b)` → one chain of four.
  - `TestARequestLapsesAfterAnHour`: request at t; `Prune` at t+61m → no request.
  - `TestAMatchThatMovesLeaves`: chain {a,b,c} in repo r; live says c is in r2 → c out, `Left{c, "r2", id}` returned; chain {a,b} stays.
  - `TestAChainOfOneEnds`: chain {a,b}; b gone → no chain.
  - `TestOffLeavesAndRefusesAsks`: chain {a,b}; `SetOff(b,true)` → chain gone; `Ask(a,b)`, `Ask(b,a)` → not formed.
  - `TestTheInboxKeepsTwentyAndCountsTheRest`: 23 posts → `Take` gives 20 items, the newest, plus a last item `and 3 more`; a second `Take` is empty.
  - `TestSaveAndLoad`: round trip of chains, requests and off; inboxes are not saved; file mode 0600.

- [ ] **Step 2:** `go test ./internal/chains/` → FAIL (package does not exist).
- [ ] **Step 3:** Implement. Chain IDs: the first member's pane plus a counter is enough; IDs only need to be stable while the chain lives.
- [ ] **Step 4:** `go test -race ./internal/chains/` → PASS.
- [ ] **Step 5:** Commit `feat(chains): chains that both sides agree to, with an inbox for each match`.

### Task 2: The service holds the chains and publishes them

**Files:**
- Modify: `internal/service/service.go` (Service fields, sample loop, `HandlePeer` act routing), `internal/state/state.go` (`Doc.Chains`), `cmd/matchblox/main.go` (`s.ChainsPath`)
- Create: `internal/service/chains.go`, `internal/service/chains_test.go`

**Interfaces:**
- Consumes: Task 1's `Store`, `Member`, `Prune`.
- Produces:
  - `state.Doc.Chains *ChainsView` with `type ChainsView struct { Chains []chains.Chain; Requests []chains.Request }`, filled on each publish.
  - Service fields `Chains *chains.Store`, `ChainsPath string`.
  - `func memberOf(x sample.Session) chains.Member`.
  - `func (s *Service) chainAct(a proto.Act) proto.Result` for acts whose `RecID` starts with `proto.ChainPrefix = "chain:"` (`chain:<pane>` asks, `chain:-` leaves, `chain:off` and `chain:on`). The sender is `a.From`, which must be an agent pane (`sessionAt(a.From)`); the console and the CLI outside a pane get `chain needs a match: run it from an agent's pane`.
  - Ask refuses another repo with `<who> works in <repo>, not <repo>: a chain is one repo` and another host's session with `<who> is on another host: a chain is one host`.

- [ ] **Step 1: Failing tests** in `internal/service/chains_test.go`, with `newTest` and two sessions in panes `%1`, `%2` whose `Cwd` is in one repo of a fake `gitscan.Report`:
  - `TestChainActFormsAChainOnlyBothWays`: act from `%1` `chain:%2` → no chain in the next `Doc.Chains`, one request; act from `%2` `chain:%1` → one chain.
  - `TestChainActRefusesAnotherRepo`: `%3` in another repo → the exact error above.
  - `TestChainActRefusesTheConsole`: `a.From == ""` → `chain needs a match: run it from an agent's pane`.
  - `TestChainsSurviveARestart`: form a chain, build a new Service on the same `ChainsPath` → `Doc.Chains` holds it.
  - `TestAGoneMemberIsPrunedOnSample`: publish a snapshot without `%2` → no chain.
- [ ] **Step 2:** `go test ./internal/service/ -run Chain` → FAIL.
- [ ] **Step 3:** Implement. Prune on every sample with `live` from the snapshot and Task 3's `RepoOf`. Save after each change. Post each `Left` to the remaining mates' inboxes with the leave copy.
- [ ] **Step 4:** `go test -race ./internal/service/ -run Chain` → PASS.
- [ ] **Step 5:** Commit `feat(service): the service keeps chains, saves them and shows them`.

### Task 3: One repo of a session, for the service and the console

**Files:**
- Modify: `internal/gitscan/gitscan.go` (add `RepoOf`), `internal/app/sessions.go:119-140` (`repoOf` calls it)
- Test: `internal/gitscan/gitscan_test.go`

**Interfaces:**
- Produces: `func RepoOf(r *Report, cwd string) (repoPath, worktreePath string, ok bool)`: the longest repo or worktree path that is `cwd` or a parent of it. `Model.repoOf` keeps its signature and its fallback to `filepath.Base(cwd)`.

- [ ] **Step 1: Failing test** `TestRepoOfFindsTheWorktreesRepo`: a report with repo `/r` and worktree `/r-wt`; `RepoOf(r, "/r-wt/internal")` → `("/r", "/r-wt", true)`; `"/rx"` → not ok (prefix without a slash is no parent).
- [ ] **Step 2:** FAIL. **Step 3:** Implement, then make `Model.repoOf` use it. **Step 4:** `go test ./internal/gitscan/ ./internal/app/` → PASS.
- [ ] **Step 5:** Commit `refactor(gitscan): one way to find the repo of a session`.

### Task 4: News comes in

**Files:**
- Modify: `internal/hooks/hook.go` (PostToolUse of Bash keeps a scrubbed `Command`), `internal/service/service.go` (`Hook`), `internal/service/chains.go`
- Create: `internal/service/chainnews.go`, `internal/service/chainnews_test.go`

**Interfaces:**
- Consumes: Task 2's store and `memberOf`; `hooks.Event`; `queue` waits.
- Produces:
  - `func (s *Service) chainNews(ev hooks.Event)`, called from `Hook`: Stop → `<name> finished`; a wait for the person → `<name> waits for the owner`; PostToolUse of Bash whose command runs `git commit`, `git switch` or `git checkout` → it reads the worktree's HEAD and posts the commit or branch line at once.
  - `func (s *Service) chainHeads(now time.Time)`, called each sample: `git rev-parse HEAD` and the branch of each worktree that holds a chained match, held in `heads map[string]head`. A move posts `<name> committed <n> files in <up to 3 paths>` (files from `git diff --name-only <old> <new>`) or `<name> switched to <branch>`. `<name>` is `someone in <worktree base>` when the worktree holds more than one match. A move the hook already posted is not posted twice.
  - Seam: `Service.Git func(dir string, args ...string) (string, error)` (nil: exec `git -C dir`).
  - News goes to every mate of the source, never to the source itself.
- [ ] **Step 1: Failing tests** with a fake `Git`:
  - `TestAStopReachesTheMatesOnly`: chain {%1,%2,%3}; Stop of `%1` → `Take(%2)` and `Take(%3)` hold `peer-send finished`, `Take(%1)` empty.
  - `TestACommitIsNamedOnce`: PostToolUse Bash `git commit -m x` in `%1` → `%2` gets `peer-send committed 2 files in a.go, b.go`; the next `chainHeads` posts nothing more.
  - `TestAHeadMoveInASharedWorktreeNamesNoOne`: `%1` and `%2` in one worktree, `%3` in another, all chained; HEAD moves in the shared one → `%3` gets `someone in matchblox-299 committed …`.
  - `TestUnchainedMatchesHearNothing`: Stop of an unchained `%4` → no inbox grows.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** `go test -race ./internal/service/ ./internal/hooks/` → PASS.
- [ ] **Step 5:** Commit `feat(chains): commits, branch changes, finishes and waits reach chain-mates`.

### Task 5: The hook asks for news and prints it

**Files:**
- Modify: `internal/proto/proto.go` (`KindNews`, `News`, `NewsReply`), `internal/service/service.go` (`HandlePeer` answers `KindNews`), `cmd/matchblox/main.go` (`hook`, `progressPrompt`), `internal/transport/socket.go` (`Ask`)
- Create: `internal/service/notice.go`, tests in `internal/service/notice_test.go` and `cmd/matchblox/hook_test.go`

**Interfaces:**
- Consumes: Task 1's `Take`, Task 2's store, Task 3's `RepoOf`, config `[chains] notice`.
- Produces:
  - `proto.KindNews`; `type News struct { Pane, SessionID, Event string }`; `type NewsReply struct { Text string }`.
  - `func Ask(path string, timeout time.Duration, kind proto.Kind, body any, reply any) error` in transport: one dial, hello, one message, one reply, all inside `timeout`.
  - `func (s *Service) news(n proto.News) string`: the news head and items from `Take`, then the notice lines: a pending request to this match (request copy), and once per pair a same-repo match that is not chained (notice copy). A pair is noticed again only after either changes branch. Returns "" when there is nothing.
  - In `hook`: for `UserPromptSubmit` and `SessionStart` (Claude Code), print the text as plain lines; for `PostToolUse`, print `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":<text>}}`. Cursor's `beforeSubmitPrompt` prints nothing until the check in Task 10 says it can carry text.
- [ ] **Step 1: Failing tests:**
  - `TestNewsGivesItemsThenEmpties` (service): two items for `%2` → reply holds the head and both lines; a second ask → "".
  - `TestNoticeOncePerPairUntilABranchChange` (service): `%1`, `%2` in one repo, unchained → first ask for `%1` has the notice with `%2`; the second has none; after `%2`'s branch changes, it shows again.
  - `TestNoticeOffInConfig` (service): `[chains] notice = false` → no notice, news still flows.
  - `TestHookPrintsNewsAsContext` (cmd): a fake service answering `KindNews` with "x" → `PostToolUse` prints the JSON above; `UserPromptSubmit` prints `x`.
  - `TestHookStaysInBudgetWithTheServiceStuck` (cmd): a service that accepts and never answers → `hook` returns in under 50 ms and prints nothing (extend the existing up, down or stuck test).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** `go test -race ./internal/service/ ./cmd/matchblox/ ./internal/transport/` → PASS.
- [ ] **Step 5:** Commit `feat(chains): the hook brings each match its news between steps`.

### Task 6: Send reaches chain-mates only

**Files:**
- Create (ported from `origin/feat/peer-send`): `internal/service/peersend.go`, `internal/service/peersend_test.go`
- Modify: `internal/service/actions.go` (route a `message:` from an agent pane through `peerAct`), `internal/service/service.go` (`HandlePeer` sets `a.From, a.FromKnown`), `internal/proto/proto.go` (`Act.FromKnown`), `internal/config/config.go` (`Chains{Notice bool; Send string}`, defaults `true`, `"chain"`), `config.example.toml`

**Interfaces:**
- Consumes: #297's `peerSend`, `checkPeerText`, `tagName`, `peerReady`, `peerAction`, `peerSends.check`, `logPeer`; Task 1's `Mates`, `Post`.
- Produces: `peerSend` with these changes from #297:
  - Refuses a target outside the sender's chain (send refused copy) and any send when `[chains] send = "off"`.
  - A busy target: `Post` to its inbox, result note `queued for <who>'s next step`.
  - A codex target that is busy: queued, and typed by `func (s *Service) deliverAtRest(now time.Time)` (each sample) once it is at rest.
  - An opencode or cursor target: the OpenCode copy for opencode; cursor gets the inbox only.
  - Refuses a pane in a mode (`#{pane_in_mode}` is 1): `not delivered: <who>'s pane is in copy mode`.
  - After typing, polls the screen up to 2 s for a draft that starts with `[from `; on no match it clears the line (`C-u`) and returns `not delivered: <who> did not show the line`.
  - Holds `peers.mu` only while it reserves the rate slot; the typing runs outside it, under the act's context.
  - `checkPeerText` also refuses `unicode.Cf`.
  - `tagName` gives `<name> <pane>`.
  - A sender with `pid > 0` that `PeerPane` cannot place is refused when the client is not the console: `not sent: matchblox cannot tell which match sends this`.
- [ ] **Step 1:** Port #297's two files as they are, run `go test ./internal/service/ -run Peer` → PASS (the baseline). Commit `feat(service): bring in peer send from #297`.
- [ ] **Step 2: Failing tests:**
  - `TestSendOutsideTheChainIsRefused`: the exact copy; no `send-keys` ran.
  - `TestSendToABusyMateQueues`: busy `%2` → no `send-keys`; `Take(%2)` holds `[from peer-send %1] hi`.
  - `TestCodexGetsItsQueuedLineAtRest`: busy codex `%2`; queue; `%2` goes idle; `deliverAtRest` → one `send-keys -l` with the tagged line.
  - `TestOpenCodeIsRefused`: the exact copy.
  - `TestCopyModeIsRefused`: `pane_in_mode` 1 → not delivered.
  - `TestALineThatDoesNotShowIsCleared`: the screen never shows `[from ` → `C-u` ran and the result says not delivered.
  - `TestTheTagNamesThePane`: tag of `%14` named `peer-send` is `[from peer-send %14]`.
  - `TestHiddenFormatCharactersAreRefused`: text with U+200B → refused.
  - `TestAStuckPaneDoesNotBlockOtherSends`: one send whose typing blocks; a second send to another mate returns within 1 s.
  - `TestAnOrphanSenderIsRefused`.
  - `TestChainsConfigDefaults` (config): `Default().Chains == Chains{Notice: true, Send: "chain"}`; `send = "maybe"` → a load error naming the two values.
- [ ] **Step 3:** FAIL. **Step 4:** Implement. **Step 5:** `go test -race -count=3 ./internal/service/ ./internal/config/` → PASS.
- [ ] **Step 6:** Commit `feat(chains): send reaches chain-mates only, and never lands in a draft`.

### Task 7: The commands

**Files:**
- Modify: `cmd/matchblox/main.go` (`commands`, the `run` switch, flags `--off`, `--on`), `cmd/matchblox/cli.go` (`cliChain`), `internal/setup/claude.go` (`HandRules`), `cmd/matchblox/help.go` or wherever `send` is described
- Test: `cmd/matchblox/cli_test.go`, `internal/setup/claude_test.go`

**Interfaces:**
- Consumes: Task 2's `chain:` acts and `Doc.Chains`.
- Produces: `func cliChain(words []string, off, on, asJSON bool, w io.Writer, path string) error`:
  - `matchblox chain <who>`: resolves `<who>` as `send` does (`resolve`), sends `chain:<pane>` with `From = $TMUX_PANE`. Prints `asked <who>: the chain forms when it chains back` or `chained with <who>: <n> matches`.
  - `matchblox chain`: lists the caller's chain and requests; `--json` gives `{"chain": [...], "requests": [...]}`.
  - `matchblox unchain`: `chain:-`. `matchblox chain --off` and `--on`.
  - `HandRules` adds `Bash(matchblox chain:*)` and `Bash(matchblox unchain:*)`.
- [ ] **Step 1: Failing tests:** `TestChainPrintsAskedThenChained`, `TestChainListJSON`, `TestUnchain`, `TestHandRulesAllowChain` (the rule strings are present).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** `go test ./cmd/matchblox/ ./internal/setup/` → PASS.
- [ ] **Step 5:** Commit `feat(cli): matchblox chain, unchain and chain --off`.

### Task 8: The fuse in Sessions

**Files:**
- Create: `internal/app/fuse.go`, `internal/app/fuse_test.go`
- Modify: `internal/app/sessions.go` (`sessionRows`, `rowIn`, `childRows` callers, `opened`, `sessTracks`), `internal/app/match.go` (the spark as a `changeKind`), `internal/app/lists.go` if the need sort lives there

**Interfaces:**
- Consumes: `m.snap.Doc.Chains` (Task 2's `ChainsView`).
- Produces:
  - `func (m Model) chainOf(s sample.Session) (chains.Chain, bool)`.
  - `func (m Model) fuseWidth(w int) int`: 0 when no shown session is chained, 2 from 70 columns, else 1.
  - `func (m Model) fuseCell(i int, ss []sample.Session, sel int, w int) string`: the cell of row `i` per the glyph rules; continuation `│` for child rows and opened lines.
  - The need grouping moves each chain into the group of its neediest member, members in need order, the chain whole; the group label adds ` · chain of <n>`.
  - A session that shares its repo (`Model.repoOf`) with another, where the two are not chained, gets a faint ` · same repo · chain?` after its name while the name has room.
  - `changeSpark` plays once down the fuse when a chain-mate commits or sends (an item posted to a mate); none with `NoMotion`.
- [ ] **Step 1: Failing tests** (golden strings of the rows, as `sessions_test.go` does), on the demo sessions of the mockup:
  - `TestFuseJoinsAChainUnderNeed`: kindle-web waits, hearth-api busy, ember-billing idle, chained → the three rows under `WAITS FOR YOU · 1 · chain of 3`, with `─╮`, `─┤`, `─╯` at 100 columns.
  - `TestFuseStubsInStatus`: the same under `g` = status → stubs; with hearth-api selected, the mates' stubs in the accent style, its own `━╸`.
  - `TestFuseIsWholeInRepo`.
  - `TestTwoChainsNeverTouch`.
  - `TestAFilterSaysHowManyAreHidden`: ` · 1 hidden`.
  - `TestFuseOnAPhone`: 44 columns: one-column fuse; the opened block's lines carry `│` and its hairlines start after it; the waiting row is whole.
  - `TestFuseRunsPastChildRows`.
  - `TestSameRepoHint`: two unchained sessions in one repo → both rows show ` · same repo · chain?` at 100 columns; at 44 columns a name with no room shows no hint.
  - `TestNoChainNoFuse`: the rows equal today's output byte for byte.
  - `TestSparkPlaysOnceAndNotWithNoMotion`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** `go test ./internal/app/` → PASS.
- [ ] **Step 5:** Run the console over `testdata/demo` on a throwaway tmux server (`tmux -L chainlook -f /dev/null`, with `HOME` and the XDG dirs in a temp dir) at 150 and 44 columns, with a `chains.json` seeded in the temp state dir, and capture each grouping. Expect the mockup's rows.
- [ ] **Step 6:** Commit `feat(console): a fuse joins the matches of a chain in Sessions`.

### Task 9: The landing section

**Files:**
- Modify: `www/index.html` (a new `<section class="agents" aria-labelledby="chain-h">` after the `agents-h` section and before `stoker-h`), `www/app.js`, `www/styles.css`

**Interfaces:**
- Produces: `<h2 id="chain-h">Chain reaction.</h2>`, a lede, the three beats (They notice, They chain, They burn together, with the mockup's copy), and an SVG row of matches.
- Motion, from the mockup page (the owner's artifact "Chain reaction design"): the matches catch one by one 90 ms apart; 900 ms after the last lights, they go out in order, 90 ms apart, each leaving a charred head and three puffs of smoke over 2.6 s; then a dark match catches at random every 180–650 ms and burns 1.6–4.2 s. It starts when the section scrolls into view (the site's `IntersectionObserver` at 0.2) and stops when it leaves. With `prefers-reduced-motion`, every match burns, still.
- [ ] **Step 1:** Build it, using the site's tokens and `still()`.
- [ ] **Step 2:** Check: `www` has no test suite; open `www/index.html` in the browser where one is available, else serve it with `python3 -m http.server` and check that the page loads with no console error. State under Not proven what was not seen.
- [ ] **Step 3:** Commit `feat(www): chain reaction on the landing page`.

### Task 10: Docs, and the two checks

**Files:**
- Modify: `docs/agents.md` ("A match knows its surroundings" gains the chain commands, and says that a chain gives a match the right to message its chain-mates and nothing more), `docs/config.md` (`[chains]`), `docs/console.md` (the fuse), `CHANGELOG.md` (Unreleased, Added, with #299)

- [ ] **Step 1: Check Claude Code's `PostToolUse` `additionalContext`.** On a throwaway tmux server, run `claude` with a settings file whose `PostToolUse` hook prints the JSON of Task 5 with a marker word, ask it to run `true`, then ask it what the marker word is. If it cannot say, news arrives at turn start only: remove the `PostToolUse` print, and say so in agents.md.
- [ ] **Step 2: Check Cursor's `beforeSubmitPrompt`** in Cursor's hook docs. If it can add text to the turn, print the news there as for `UserPromptSubmit`, with a test. If not, agents.md says Cursor gets news only through `matchblox chain`.
- [ ] **Step 3:** Write the docs. Every doc sentence about what a chain grants matches the code.
- [ ] **Step 4:** `go vet ./... && go test -race ./...` → PASS.
- [ ] **Step 5:** Commit `docs: chain reaction in the agents, config and console docs`.

## Done when

- The spec's Done when list holds, checked on a throwaway tmux server with two Claude Code sessions in one repo.
- #297's draft is closed with a pointer to this branch (the owner's yes first).
