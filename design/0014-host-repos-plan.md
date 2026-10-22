# The builder's repos on the host: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a consent-gated, background scan of the host's Git checkouts whose
list feeds N's `where`, never the Git tab.

**Architecture:** `gitscan.Find` walks the roots and reads `.git` files (no
git process). `internal/state` keeps the consent (`scan.json`) and the cache
(`repos.json`). The service runs the scan in its own loop, gates it on the
consent, and sends `proto.State.Found` once per console and on change. The
console asks in the consent box and appends found paths to `gitPaths()`.

**Tech Stack:** Go, Bubble Tea v2, the repo's JSON-lines proto.

**Spec:** `design/0013-host-repos.md`.

## Global Constraints

- Depth 4 below each root. Skip `node_modules`, `.cache`, `Library`,
  `vendor`, `.venv`, `target`, and every name starting with `.`.
- Default roots: `~`, `~/dev`, `~/src`, `~/code`, `~/projects`, `~/work`.
  Picked roots replace them; `[git] repos` and `[git] scan_roots` always add.
- Rescan every 30 minutes (constant `scanEvery`), after the first sample,
  and on `scan:yes`.
- Bounds: 30 s and 100 000 directories per scan.
- No git subprocess, no network call in the scan.
- Remote URLs lose their userinfo (`https://u:t@h/o/r` → `https://h/o/r`).
- `internal/app` imports neither `gitscan` nor `state` writers
  (`internal/arch_test.go`); it reads `proto.Found` / `proto.Checkout`.
- No `proto.Version` bump.
- Box copy, exact: title `Find the Git repositories in your home folder?`,
  line `they stay on this host and your paired devices`, foot `y find ─ n
  never ─ p pick folders ─ esc later`.
- Refusal copy, exact: `only you say yes to a scan, at a console or in your
  own shell`.
- Comments: terse, constraints only (as the surrounding code).

## Review Focus

1. A root that does not exist, or `~/dev` missing: skipped, no error shown.
   → Task 1 `TestFindSkipsMissingRoot`.
2. A symlink in `~` pointing at `/` or at another root: never followed.
   → Task 1 `TestFindFollowsNoSymlink`.
3. A corrupt `repos.json` or `scan.json`: the cache reads empty; a corrupt
   consent reads as never asked. → Task 2 `TestReadCorruptFiles`.
4. Two consoles: one answers, the other's box closes on the next snapshot.
   → Task 6 `TestScanBoxClosesWhenAnsweredElsewhere`.
5. An old service (no `found` key) with a new console: no box, `gitPaths()`
   as today. → Task 6 `TestNoBoxWithoutFound`.

---

### Task 1: `gitscan.Find`, the walk

**Files:**
- Create: `internal/gitscan/find.go`, `internal/gitscan/find_test.go`

**Interfaces:**
- Produces:
  ```go
  type Found struct { Consent string; Roots []string; At time.Time; Took time.Duration; Repos []Checkout; Errors []string } // JSON tags as the design
  type Checkout struct { Path, Main, Remote, Branch string; Active time.Time }
  const FindDepth = 4
  type FindOpts struct { Depth int; Remote string; MaxDirs int; Budget time.Duration }
  func Find(ctx context.Context, roots []string, o FindOpts) (repos []Checkout, errs []string)
  func ScanRoots(home string, picked, repos, extra []string) []string
  func StripCredentials(url string) string
  ```

- [ ] **Step 1: Write the failing tests** (a helper `mkRepo(t, dir, branch, remoteURL)` writes `.git/HEAD`, `.git/config`, `.git/logs/HEAD` with a set mtime; `mkWorktree` writes a `.git` file whose `gitdir:` points at `<main>/.git/worktrees/<name>` with its own `HEAD` and `logs/HEAD`)
  - `TestFindReadsACheckout`: one repo at `root/a`; got `Path` = `root/a`, `Branch` = `main`, `Remote` = the URL of `[remote "origin"]`, `Active` = the mtime of `logs/HEAD`.
  - `TestFindDetachedHead`: HEAD holds a sha; `Branch` = `""`.
  - `TestFindRemoteFallsBackToFirst`: only `[remote "up"]`; `Remote` = its URL.
  - `TestFindLinkedWorktree`: `Main` = the main checkout's path, `Branch` from the worktree's own HEAD, `Remote` from the main's config.
  - `TestFindStopsAtGit`: `root/a/.git` and `root/a/sub/.git`: only `root/a`.
  - `TestFindDepth`: repos at depth 4 found, at depth 5 not (`Depth: 4`).
  - `TestFindSkips`: repos under each of `node_modules`, `.cache`, `Library`, `vendor`, `.venv`, `target`, `.hidden`: none found.
  - `TestFindFollowsNoSymlink`: `root/link -> other/` holding a repo: not found.
  - `TestFindSkipsMissingRoot`: roots `[missing, root]`: the repo in `root`, `errs` empty.
  - `TestFindOverlappingRoots`: roots `[home, home/dev]`, repo at `home/dev/a/b/c/d` (depth 5 from home, 4 from dev): found once.
  - `TestFindMaxDirs`: `MaxDirs: 3` over a wide tree: `errs` has one line naming the bound; what was found is kept.
  - `TestStripCredentials`: `https://u:t@github.com/o/r.git` → `https://github.com/o/r.git`; `git@github.com:o/r.git` unchanged; `ssh://git@h/o/r` unchanged (a user and no password is not a secret).
  - `TestScanRoots`: no picked → home + the five defaults + repos + extra, `~/x` expanded, duplicates dropped; picked `[~/w]` → `~/w` + repos + extra.

- [ ] **Step 2:** `go test ./internal/gitscan/ -run 'Find|Strip|ScanRoots'` → FAIL (undefined).
- [ ] **Step 3: Implement** in `find.go`. Walk with `os.ReadDir`; a visited map `dir → depth left` skips a dir already walked with as much left; resolve each root with `filepath.EvalSymlinks` once. A `.git` entry (dir or file) records the dir and stops. Parse config with a line scanner (`[remote "x"]` then `url = …`), no new dependency. Sort the result by `Path`. `ctx` and `Budget` checked once per directory.
- [ ] **Step 4:** same command → PASS.
- [ ] **Step 5:** commit `feat: gitscan finds the checkouts under a set of roots (#173)`.

### Task 2: consent and cache files in `internal/state`

**Files:**
- Create: `internal/state/scan.go`, `internal/state/scan_test.go`
- Modify: `internal/state/state.go` (temp-and-rename moves to `writeJSON(path string, v any) error`, file mode `0o600`)

**Interfaces:**
- Consumes: `gitscan.Found`.
- Produces:
  ```go
  type Consent struct { Answer string `json:"answer"`; Roots []string `json:"roots,omitempty"`; At time.Time `json:"at"` }
  func ScanPath() string   // <dir of Path()>/scan.json
  func ReposPath() string  // <dir of Path()>/repos.json
  func ReadConsent(path string) Consent             // missing or corrupt: zero
  func WriteConsent(path string, c Consent) error
  func ReadFound(path string) gitscan.Found          // missing or corrupt: zero
  func WriteFound(path string, f gitscan.Found) error
  ```

- [ ] **Step 1: Write the failing tests:** `TestConsentRoundTrip`, `TestFoundRoundTrip`, `TestReadCorruptFiles` (garbage bytes → zero values), `TestScanFilesNextToState` (`XDG_STATE_HOME=/x` → `/x/matchblox/scan.json`, `/x/matchblox/repos.json`), `TestWriteIsPrivate` (mode `0o600`).
- [ ] **Step 2:** `go test ./internal/state/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** `go test ./internal/state/ ./internal/service/ ./cmd/...` → PASS (state.Write's callers unchanged).
- [ ] **Step 5:** commit `feat: the scan's consent and cache live next to state.json (#173)`.

### Task 3: config `scan_roots`, proto `Found`

**Files:**
- Modify: `internal/config/config.go` (`Git.ScanRoots []string \`toml:"scan_roots"\`` with comment `// more folders to find repositories in, after a yes`), `internal/config/config_test.go`
- Modify: `internal/proto/proto.go` (aliases `Found = gitscan.Found`, `Checkout = gitscan.Checkout`; `State.Found *Found \`json:"found,omitempty"\`` with the comment: set in a console's first snapshot and when it changes; nil: as the last one), `internal/proto/proto_test.go`

- [ ] **Step 1: Write the failing tests:** `TestLoadScanRoots` (a file with `[git]\nscan_roots = ["/srv/x"]` → `cfg.Git.ScanRoots == ["/srv/x"]`); `TestStateFoundRoundTrip` (marshal `State{Found: &Found{Consent: "yes", Repos: …}}`, unmarshal, equal) and `TestStateWithoutFound` (a `State` without `Found` marshals with no `"found"` key).
- [ ] **Step 2:** `go test ./internal/config/ ./internal/proto/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** same → PASS.
- [ ] **Step 5:** commit `feat: [git] scan_roots, and the found repos on the wire (#173)`.

### Task 4: the service scans, gated on the consent

**Files:**
- Create: `internal/service/found.go`, `internal/service/found_test.go`
- Modify: `internal/service/service.go` (fields, `client.foundGen`, `stateFor`, `Run` starts `foundLoop` and kicks it after the first sample), `internal/service/actions.go` (`dispatch` cases; the refusal in `act`)

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces (exported fields on `Service`):
  ```go
  // Find scans the host from roots. Nil: no scan, and no Found sent (fixtures).
  Find func(ctx context.Context, roots []string) ([]gitscan.Checkout, []string)
  // ScanRoots are a scan's roots for the roots picked (empty: the defaults).
  ScanRoots func(picked []string) []string
  // ConsentPath and ReposPath are the scan's files. Empty: kept in memory.
  ConsentPath, ReposPath string
  ```
  Act ids: `scan:yes` (Text: picked roots, one per line; empty keeps those on record), `scan:no`.
  `const scanEvery = 30 * time.Minute`.

- [ ] **Step 1: Write the failing tests** (a fake `Find` counting calls and returning one checkout; temp files):
  - `TestNoScanWithoutYes`: consent missing, `Run` past the first sample: `Find` called 0 times; the first snapshot's `Found` = `{Consent: ""}`.
  - `TestNoScanAfterNo`: consent `no` on disk: 0 calls.
  - `TestScanYesScansAndRecords`: `act(scan:yes)` from a console: `Find` called once with `ScanRoots(nil)`; `scan.json` answer `yes`; `repos.json` holds the checkout; the result has no `Err`.
  - `TestScanYesKeepsPickedRoots`: consent `yes` roots `[/w]`; `scan:yes` with empty Text → `ScanRoots([/w])`; with Text `/a\n/b` → `ScanRoots([/a /b])` and those recorded.
  - `TestScanNoDropsCache`: after a yes, `scan:no` → `repos.json` gone, `State().Found` has no repos, consent `no`.
  - `TestScanYesRefusedFromAgentPane`: `From` a pane a sampled session sits in, not `Console` → `Err` = the exact refusal; `Find` not called; consent unchanged. `scan:no` from the same pane is taken.
  - `TestCacheSentBeforeFirstScan`: consent `yes`, `repos.json` with one checkout, `Find` blocking: the first snapshot carries that checkout.
  - `TestFoundOncePerConsole`: two `stateFor` calls on one client: the first has `Found`, the second nil; after `scan:no`, the next has it again.
  - `TestNoFoundWithoutFind`: `Find` nil: `stateFor` never sets `Found`.
- [ ] **Step 2:** `go test ./internal/service/ -run 'Scan|Found|NoScan|Cache'` → FAIL.
- [ ] **Step 3: Implement.** `s.found gitscan.Found` and `s.foundGen uint64` under `s.mu`; a bump of `foundGen` and `broadcast()` on every change. One `scanMu` serialises a scan from the loop and from an act; the scan runs outside `s.mu`. The loop reads the consent file before every scan. The refusal sits in `act` beside `notTheBuilders`: `a.RecID == "scan:yes" && !a.Console && (agent || sessionAt(a.From) found)`.
- [ ] **Step 4:** `go test ./internal/service/` → PASS.
- [ ] **Step 5:** commit `feat: the service finds the builder's repos after a yes, and never before (#173)`.

### Task 5: wiring and `matchblox scan`

**Files:**
- Create: `cmd/matchblox/scan.go`, `cmd/matchblox/scan_test.go`
- Modify: `cmd/matchblox/main.go` (`newService` sets `Find`, `ScanRoots`, paths; `commands`, `usageText`, `case "scan"`), docs that list commands (`docs/start.md` or where `docs_test.go` looks)

**Interfaces:**
- Consumes: Tasks 1, 2, 4.
- Produces: `func scanAct(words []string, here string) (proto.Act, error)`; `func cliScan(words []string, w io.Writer, path, reposPath string) error`.

- [ ] **Step 1: Write the failing tests:**
  - `TestScanActYes`: `[]` → `{RecID: "scan:yes", Confirm: "y", Text: ""}`.
  - `TestScanActFolders`: `["~/w", "rel"]` with here `/h` → Text `<home>/w\n/h/rel` (absolute, `~` expanded).
  - `TestScanActOff`: `["off"]` → `{RecID: "scan:no"}`.
  - `TestScanActMissingFolder`: a folder that does not exist → error `scan: no folder <path>`.
  - `TestCliScanSays`: a fake socket service answering the act, `repos.json` with 2 checkouts and `Took` 1.3 s → output `found 2 repositories in 1.3s` and, for `off`, `the scan is off, and its list is gone`.
- [ ] **Step 2:** `go test ./cmd/matchblox/ -run Scan` → FAIL.
- [ ] **Step 3: Implement.** Usage line: `matchblox scan [<folder>…|off]   find the Git repositories here, after asking (off: stop and forget)`. `newService`: `Find` = `gitscan.Find` with `FindOpts{Depth: FindDepth, Remote: cfg.Git.Remote, MaxDirs: 100_000, Budget: 30 * time.Second}`; `ScanRoots` = `gitscan.ScanRoots(home, picked, cfg.Git.Repos, cfg.Git.ScanRoots)`; paths from `state.ScanPath()` / `state.ReposPath()`. Fixtures leave `Find` nil.
- [ ] **Step 4:** `go test ./cmd/...` → PASS (including `docs_test.go`).
- [ ] **Step 5:** commit `feat: matchblox scan finds the repos now, in the folders given (#173)`.

### Task 6: the console asks, and N offers what was found

**Files:**
- Create: `internal/app/scanask.go`, `internal/app/scanask_test.go`
- Modify: `internal/app/model.go` (`found *proto.Found`, `scanLater bool`; `stateMsg` keeps the last non-nil `Found`; `key` routes to the box), `internal/app/consent.go` (`consenting`, `consentBox`, `consentWidth` know the scan question), `internal/app/mouse.go` (a tap closes it as `esc`), `internal/app/lifeform.go` (`gitPaths` appends found paths not already listed, most recent `Active` first; a `folders` form mode)

**Interfaces:**
- Consumes: `proto.Found`, `proto.Checkout`, act ids from Task 4.
- Produces: `func (m Model) asksScan() bool` — `m.found != nil && m.found.Consent == "" && !m.scanLater && m.form == nil && m.pending == nil && m.held == nil && m.peek == nil`.

- [ ] **Step 1: Write the failing tests:**
  - `TestScanBoxShowsOnce`: a snapshot with `Found{Consent: ""}` → `asksScan()`; the view holds the exact title, line and foot.
  - `TestScanBoxYes`: `y` sends `{RecID: "scan:yes", Confirm: "y"}`; the box closes; flash `finding your Git repositories…`.
  - `TestScanBoxNo`: `n` sends `scan:no`.
  - `TestScanBoxLater`: `esc` and a tap each close it and send nothing; a later snapshot with `Consent: ""` keeps it closed.
  - `TestScanBoxPickFolders`: `p` opens a form whose one field is `folders`, holding `~ ~/dev ~/src ~/code ~/projects ~/work`; ⏎ sends `scan:yes` with those, `~` expanded on the service side (Text is what was typed, split on spaces, one per line).
  - `TestScanBoxClosesWhenAnsweredElsewhere`: open; a snapshot with `Found{Consent: "yes"}` → closed.
  - `TestNoBoxWithoutFound`: snapshots with `Found` nil → never `asksScan()`.
  - `TestFoundFeedsWhereNotGit`: Git report with `/w/app`, found `[/w/app, /h/old (Active older), /h/new (Active newer)]` → `gitPaths()` = `[/w/app, /h/new, /h/old]`; the Git tab's rows hold `/w/app` only.
  - `TestFoundKeptWhenNil`: a snapshot with `Found` set, then one with nil → `m.found` unchanged.
- [ ] **Step 2:** `go test ./internal/app/ -run 'ScanBox|NoBox|Found'` → FAIL.
- [ ] **Step 3: Implement.** The N form's `where` hint becomes `a path, or ←→ for a repository` when found paths exist.
- [ ] **Step 4:** `go test ./internal/app/` → PASS.
- [ ] **Step 5:** commit `feat: the console asks once to find the repos, and N offers them (#173)`.

### Task 7: docs and the full suite

**Files:**
- Modify: `DESIGN.md` (Consent entry: the scan question, its keys, `esc later` records nothing), `docs/config.md` (`[git] scan_roots`), `CHANGELOG.md` (Unreleased), `design/0013-host-repos.md` (Status: built)

- [ ] **Step 1:** write the docs.
- [ ] **Step 2:** `go vet ./... && go test ./...` → all PASS.
- [ ] **Step 3:** commit `docs: the repo scan in DESIGN.md, config.md and the changelog (#173)`.
