# matchblox Stays Current Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A machine that is behind the latest release says so and offers the update in the console; a machine that opts in updates by itself; a host on another release than the console says so on its row (#390, #389).

**Architecture:** The updater moves out of `cmd/matchblox` into `internal/update` so the service can run it. The service of each machine owns its check, its offer and its auto update, and puts an `update` view in the state every console reads. The console only draws that view and names acts; it makes no request. The offer on the home machine is a door; the notice for a host is a line and one act on its row.

**Tech Stack:** Go 1.27, the existing wire (`internal/proto`), doors (`internal/doors`, `internal/setup`), Bubble Tea console (`internal/app`). No new dependency.

**Spec:** `docs/superpowers/specs/2026-11-12-stay-current-design.md`

## Global Constraints

- One setting: `[update] mode = "offer"` (default), `"auto"` or `"off"`. Any other value is a config error that names the three values.
- `mode = "off"`: no request to find a release, ever, except on a person's key press on that host's row.
- One way to install: the code of `matchblox update` (checksum always, provenance when `gh` can check it, the version the new binary names).
- A build that is not a release tag (`dev`) makes no check and gets no offer.
- A Homebrew install is never replaced; the offer text is `brew upgrade matchblox`.
- Auto installs one day (24h) after this machine first saw the release, and only with no stoker run on and no clone running.
- The console sends acts, never argv. `internal/app` must not import `internal/update` if that reads the machine (`internal/arch_test.go` decides; keep it green).
- An update never ends, stalls or loses a session.
- A failed check or install never reads as "up to date": it shows the reason and the fix.
- Copy rules of the repo: one idea per sentence, active, present tense. No private host or session names in code, tests or docs (use `ws-1`, `api-auth`).
- Commits: signed Conventional Commits that end with `(#390)`; never `--no-verify`.
- Run before each commit: `go test ./... && go vet ./...` (and `golangci-lint run` if the repo's CI runs it; read `.github/workflows`).

## Review Focus

1. GitHub cannot be reached, or answers 403/429, at a check: the view carries the reason; the door does not claim "latest"; the next check is the next day, not a retry loop. (Task 3)
2. The install fails midway (binary not writable, disk full, checksum wrong): the old binary is intact and still serves; the door shows the reason and the fix. (Task 4)
3. Two consoles press update at the same moment, or one console presses twice: one install runs. (Task 4)
4. The latest release is older than or equal to this build, or is a prerelease tag: no offer, no auto install. (Task 3)
5. An auto update lands while a console is open on the old build: that console does not put the old service back, and says how to start again. (Task 5)

---

### Task 1: The updater becomes a package the service can use

**Files:**
- Create: `internal/update/update.go`, `internal/update/update_test.go`
- Modify: `cmd/matchblox/update.go` (thin caller), `cmd/matchblox/update_test.go` (move the tests of moved code)

**Interfaces:**
- Produces:
  - `type Updater struct{ … }` with the fields of today's `updater`, exported where tests in other packages set them: `Base string`, `Client *http.Client`, `Self string`, `Current string`, `GOOS, Arch string`, `LookPath func(string) (string, error)`, `Run func([]string) (string, error)`, `Out io.Writer`.
  - `func New(current string, out io.Writer) (*Updater, error)` (today's `liveUpdater`).
  - `func (u *Updater) Latest() (string, error)`
  - `func (u *Updater) Install(tag string) (Proof, error)`; `type Proof struct{ Checksum, Provenance bool }`
  - `func (u *Updater) Managed() string` — the package manager's command (`"brew upgrade matchblox"`) or `""`.
  - `func (u *Updater) Update() error` — today's `update()`, words unchanged.
  - `func Compare(a, b string) int`, `func IsRelease(tag string) bool`.

- [ ] **Step 1:** Move `updater` and its helpers to `internal/update` with the names above. `deferred()` becomes `Managed()` plus the `say` in `Update()`. `provenance` reports through `Proof.Provenance`.
- [ ] **Step 2:** Move the existing tests with the code; `cmd/matchblox` keeps only the test that `matchblox update` calls `Update()`.
- [ ] **Step 3:** Add `TestInstallReportsItsProof`: with no `gh` on `LookPath`, `Install` returns `Proof{Checksum: true, Provenance: false}` and no error.
- [ ] **Step 4:** Run `go test ./internal/update ./cmd/matchblox ./internal/` — PASS, and `matchblox update` output is byte-for-byte as before (the existing tests assert its words).
- [ ] **Step 5:** Commit `refactor: the updater is a package the service can use (#390)`.

### Task 2: The setting

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `docs/config.md`

**Interfaces:**
- Produces: `type Update struct{ Mode string \`toml:"mode"\` }`, field `Config.Update`; constants `config.UpdateOffer = "offer"`, `UpdateAuto = "auto"`, `UpdateOff = "off"`; `func (u Update) ModeOr() string` returns `"offer"` for `""`.

- [ ] **Step 1:** Tests: a file with no `[update]` gives `ModeOr() == "offer"`; `mode = "auto"` and `"off"` load; `mode = "always"` fails with an error that contains `"offer", "auto" or "off"`.
- [ ] **Step 2:** Implement, validating where the other sections validate.
- [ ] **Step 3:** `docs/config.md`: the `[update]` section, the three values in a table, one line each on what leaves the machine.
- [ ] **Step 4:** `go test ./internal/config` — PASS. Commit `feat: [update] mode picks offer, auto or off (#390)`.

### Task 3: The service checks, and tells every console

**Files:**
- Create: `internal/service/update.go`, `internal/service/update_test.go`
- Modify: `internal/state/state.go` (the view), `internal/service/service.go` (fields, start the loop in `Run`), `cmd/matchblox/main.go` (wire the updater and the mode into `mk()`)

**Interfaces:**
- Consumes: Task 1 `Updater`; Task 2 `Update.ModeOr()`.
- Produces:
  - `state.Doc.Update *UpdateView \`json:"update,omitempty"\``
  - `type UpdateView struct { Mode string \`json:"mode"\`; Current string \`json:"current"\`; Latest string \`json:"latest,omitempty"\`; CheckedAt time.Time \`json:"checked_at,omitzero"\`; FirstSeen time.Time \`json:"first_seen,omitzero"\`; Err string \`json:"err,omitempty"\`; Managed string \`json:"managed,omitempty"\`; Last *UpdateRecord \`json:"last,omitempty"\` }`
  - `type UpdateRecord struct { From, To string; At time.Time; Checksum, Provenance bool; Err string }` (json tags snake_case, omitempty).
  - Service fields: `Update UpdateSource` (nil: no view at all, as a fixture), `UpdateMode string`, `UpdateEvery time.Duration` (0: 24h).
  - `type UpdateSource interface { Latest() (string, error); Install(tag string) (update.Proof, error); Managed() string }` in `internal/service`.
  - `func (s *Service) checkUpdate(now time.Time)` — one look; `func (s *Service) newer() (tag string, ok bool)` — true when `Latest` is a release tag and `update.Compare(current, latest) < 0`.
  - State file `update.json` in the state dir: `{latest, first_seen, closed, last}`. `first_seen` is kept across restarts and reset when `latest` changes.
- A service with the feature **always** sends `update` (with `mode: "off"` and nothing else when off). A console reads the presence of the field as "this host knows the update act" (Task 6).

- [ ] **Step 1: Tests** (a fake `UpdateSource` that counts calls):
  - `TestUpdateOffMakesNoRequest`: mode off, run the loop past two periods: `Latest` calls == 0; the view is `{mode: "off", current: …}`.
  - `TestUpdateCheckAtStartAndDaily`: mode offer: one call at start, one more after 24h of the fake clock, none between.
  - `TestUpdateViewNamesANewerRelease`: current `v0.42.0`, latest `v0.43.0` → `Latest == "v0.43.0"`, `FirstSeen` set once and equal after a second check and after a restart on the same state dir.
  - `TestUpdateCheckFailureIsSaid` (Review Focus 1): `Latest` errors → `Err` holds the error's words, `Latest` keeps the last known value, the next call is a period later.
  - `TestNoOfferForSameOlderOrPrerelease` (Review Focus 4): latest `v0.42.0`, `v0.41.0`, `v0.43.0-rc.1` against current `v0.42.0` → `newer()` false for each.
  - `TestDevBuildMakesNoRequest`: current `dev` → 0 calls, no view.
- [ ] **Step 2:** Run them: FAIL (undefined).
- [ ] **Step 3:** Implement. The loop is a goroutine `Run` starts; it takes `s.Now`, never `time.Now`. It bumps `s.gen` and broadcasts when the view changes, as `refreshDoors` does.
- [ ] **Step 4:** Wire in `cmd/matchblox/main.go`: `update.New(version, io.Discard)`; a failure to build it leaves `s.Update` nil and logs one line.
- [ ] **Step 5:** `go test ./internal/service ./internal/state ./cmd/matchblox` — PASS. Commit `feat: the service looks for a newer release once a day and says what it found (#390)`.

### Task 4: The offer is a door; one act installs

**Files:**
- Modify: `internal/doors/doors.go` (id `Update = "update"`), `internal/service/update.go`, `internal/service/actions.go` (`doors()`, `door()`), `internal/service/update_test.go`, `internal/app/doors.go` only if the door needs a word the generic drawing lacks.

**Interfaces:**
- Consumes: Task 3 view and state file.
- Produces:
  - The door, appended by `s.doors()` (not by `setup.Doors`, whose closing is by id for ever): `ID: "update"`, `Title: "matchblox v0.43.0 is out"`, `Why: "This machine runs v0.42.0."`, `Preview:` the lines `matchblox update` prints for that install, `Sum: "v0.43.0"`. For a managed install: `Preview: "brew upgrade matchblox"`, `Problem: "Homebrew installed matchblox. Update it with Homebrew."`, no primary action.
  - Acts through the existing `door:update` route: primary (typed `y`, `Text` = the tag the person saw) installs that tag; a `Text` that is not the current `Latest` is refused with "a newer release came out: look again". Secondary writes `closed: <tag>` to `update.json`; the door is absent while `closed == Latest`.
  - `func (s *Service) installUpdate(tag string) proto.Result` — single flight: a second call while one runs returns `Skipped: ["already updating"]`.
  - After a good install the existing binary watch stops the service with `ErrReplaced`; nothing new stops it.

- [ ] **Step 1: Tests:**
  - `TestUpdateDoorShowsForANewerRelease` and `…AbsentWhenUpToDate`, `…AbsentWhenOff`.
  - `TestUpdateDoorClosedForThisReleaseOnly`: close at `v0.43.0` → absent; latest becomes `v0.44.0` → present.
  - `TestUpdateActNeedsATypedY`; `TestUpdateActInstallsTheTagTheDoorShowed`; `TestUpdateActRefusesAStaleTag`.
  - `TestUpdateInstallFailureKeepsTheOffer` (Review Focus 2): `Install` errors → `Result.Err` holds the reason, the door stays, `Last.Err` set, the service is not stopped.
  - `TestTwoUpdateActsRunOneInstall` (Review Focus 3): two concurrent acts → `Install` calls == 1.
  - `TestManagedInstallOffersTheManagersCommand`: no primary runs `Install`.
- [ ] **Step 2:** FAIL, implement, PASS.
- [ ] **Step 3:** In `internal/update`, add `TestInstallFailureLeavesTheOldBinary`: a checksum mismatch leaves the bytes at `Self` unchanged (Review Focus 2).
- [ ] **Step 4:** Check the door in a real console frame: add a case to the existing screens test (`internal/app/screens_test.go`, `check(t, …)`) with the door present at 104 and 44 columns.
- [ ] **Step 5:** Commit `feat: the console offers a newer release, and one action installs it (#390)`.

### Task 5: After an update, the right things run again

**Files:**
- Modify: `cmd/matchblox/main.go` (the `errRestart` exit), `internal/app/model.go` (`olderLocal`, the hello branch), `internal/app/layers.go` or where `Options` are built (a `Restart func() error`), tests beside each.

**Interfaces:**
- Consumes: `update.Compare`-like ordering. `internal/app` may not import a package that reads the machine: put `func Compare`/`IsRelease` where `internal/app` may import them (if `internal/arch_test.go` rejects `internal/update`, move the two pure functions to `internal/proto` or a new `internal/release` and have `internal/update` use them).
- Produces:
  - Service: when serve ends with `ErrReplaced` and the new binary exists, a service **not** under the unit (`MATCHBLOX_UNIT` unset) execs the binary at `s.Exe` with its own arguments; under the unit it exits with `errRestart` as today. If the exec fails, it logs the reason and exits with `errRestart`.
  - Console: `Options.Restart func() error` (main sets it to exec `Self` with `os.Args`); nil in tests and on the phone console.
  - `olderLocal()` is true only when the service's wire is older, or both builds are release tags and the service's is the older. A `dev` console keeps today's rule against a service of another build.
  - New `newerLocal()`: the service's release is newer than the console's. The console then sets a notice: `"the service runs v0.43.0, this console v0.42.0 · r: start again"`; `r` calls `Restart`.
  - After a good `door:update` result the console calls `Restart` itself.

- [ ] **Step 1: Tests:**
  - `TestOldConsoleDoesNotReplaceANewerService` (Review Focus 5): hello `Binary: "v0.43.0"`, console `v0.42.0` → no `service:replace` act is sent; the frame holds `start again`.
  - `TestNewConsoleStillReplacesAnOlderService`: the reverse → the act is sent once (the existing test keeps passing).
  - `TestConsoleStartsAgainAfterItsUpdate`: a good result for `door:update` calls `Restart` once.
  - In `cmd/matchblox/main_test.go`: `TestReplacedServiceRunsAgainWithoutAUnit` (exec seam called with `s.Exe`) and `TestReplacedServiceUnderAUnitExits` (`errRestart`, no exec), and `TestFailedExecFallsBackToExit`.
- [ ] **Step 2:** FAIL, implement (an `execFn` seam in main for the tests), PASS.
- [ ] **Step 3:** Commit `fix: a console never puts an older service back, and an updated service runs again by itself (#390)`.

### Task 6: A host on another release says so, and offers its update

**Files:**
- Modify: `internal/app/hostrows.go` (`hostNote`, the row's action), `internal/app/keys.go` (the key), `internal/app/peers.go`, `internal/service/actions.go` (`dispatch`: `"update:host"`), tests: `internal/app/hostrows_test.go`, `internal/app/fuse_test.go`, `internal/service/update_test.go`.

**Interfaces:**
- Consumes: Task 3 view of each peer (`m.stateOf(host).Update`), peer `hello.Binary`, Task 4 `installUpdate`.
- Produces:
  - `func (m Model) releaseNote(host string) string`: `""` when the releases are equal or either is not a release tag; else `"v0.38.1 · this console v0.42.0"`. `hostNote` appends it.
  - The row's action when `releaseNote != ""`:
    - the host's state has `update` and the host is the older → key `u`, typed `y`, sends `proto.Act{RecID: "update:host", Confirm: "y"}` to that peer.
    - the host's state has no `update` (older than this feature) → the row's fix line is `on ws-1: matchblox update`.
    - the console is the older → the fix line points at this machine's offer: `this console is older: update it in SETUP`.
  - Service `"update:host"`: needs `Confirm == "y"`; looks for the latest release now (also under `mode = "off"`: a person asked), then `installUpdate(latest)`; `Skipped: ["v0.43.0 is the latest release"]` when there is nothing newer.
  - A console whose home is a remote host (`!m.opt.Local`): when its own release is older than `Update.Latest` of its home, the notice is `this console runs v0.41.0, the latest is v0.43.0 · run: matchblox update`; no act.
  - Closing: `x` on the notice stores `host + "\x00" + hostRelease + "\x00" + consoleRelease` in the console's kept view prefs (where dismissed recommendations are kept); a closed pair shows no note.

- [ ] **Step 1: Tests:**
  - `TestHostRowNamesBothReleases`, `TestHostRowSilentWhenEqual`, `TestHostRowSilentForADevBuild`.
  - `TestHostUpdateKeySendsTheActToThatHost` (to the peer's conn, not home's; needs the typed y).
  - `TestOldHostGetsTheCommandNotTheKey` (peer state with no `update`).
  - `TestNewerHostPointsAtThisMachine`.
  - `TestRemoteHomeConsoleGetsTheCommand`.
  - `TestClosedReleaseNoteStaysClosedForThatPair`, and returns when either release changes.
  - Service: `TestHostUpdateActLooksEvenWhenOff` (`Latest` calls == 1), `TestHostUpdateActNeedsATypedY`.
  - `TestPeerChainDrawsItsFuse` in `internal/app/fuse_test.go` (#389): `twoHosts` with a peer state that holds two sessions and one chain of both (no `name`, no `mates`, as v0.38.1 sent); the Sessions rows of both carry a fuse glyph (`╮`, `╯` or `╴`) in each of the four groupings.
- [ ] **Step 2:** FAIL, implement, PASS. Add the host row to the screens test at 104 and 44 columns.
- [ ] **Step 3:** Commit `feat: a host on another release says so on its row and offers its update (#390, #389)`.

### Task 7: Auto

**Files:**
- Modify: `internal/service/update.go`, `internal/service/update_test.go`, `internal/app/` where one-time news shows (the flash/toast of `stokerNews`/`joinNews` is the pattern).

**Interfaces:**
- Consumes: Task 3 loop, Task 4 `installUpdate`, the stoker view (`s.cur.Stoker.On` and each `s.cur.Stokers[i].View.On`), the running clones (`s.cur.Clones` with a clone not ended).
- Produces:
  - `func (s *Service) autoDue(now time.Time) bool`: mode auto, `newer()`, not managed, `now - FirstSeen >= 24h`, no stoker run on, no clone running.
  - The loop also wakes every 10 minutes while an install is due but the service is busy; it makes no request on those wakes.
  - On install, `Last` is written to `update.json` before the service stops; the new service sends it in the view. The console shows `updated to v0.43.0 at 03:12` once per record (keyed by `To`+`At` in the console's kept prefs); a failed one shows `the update to v0.43.0 failed: <reason> · run: matchblox update`.

- [ ] **Step 1: Tests:**
  - `TestAutoWaitsADayFromFirstSeen`: 23h → `Install` 0 calls; 24h → 1 call with the tag.
  - `TestAutoWaitsForAStokerRun` and `TestAutoWaitsForAClone`: due, busy → 0 calls and 0 extra `Latest` calls; idle at the next wake → 1.
  - `TestOfferModeNeverInstallsByItself`; `TestAutoLeavesAManagedInstall`.
  - `TestAutoRecordsItsProofAndSurvivesTheRestart`: a new service on the same state dir sends `Last{From, To, At, Checksum: true}`.
  - `TestAutoFailureIsSaidAndTriedTheNextDay`: `Install` errors → `Last.Err`, no second call before the next daily check.
  - Console: `TestAutoUpdateNewsShowsOnce`.
- [ ] **Step 2:** FAIL, implement, PASS.
- [ ] **Step 3:** By hand, in a scratch HOME with a fake release server (`Updater.Base`): start a unit-less service on an old tag with `mode = "auto"` and a first-seen 25h back; see it install, run again on the new binary and answer `matchblox status`. Record the commands and output in the PR.
- [ ] **Step 4:** Commit `feat: mode auto updates a machine by itself, a day after a release and when no work is in flight (#390)`.

### Task 8: The README, the docs, the landing page and the changelog

**Files:**
- Modify: `README.md`, `docs/start.md` (beside the `matchblox update` part), `docs/config.md` (check Task 2's text against the final behavior), `docs/remote.md` (a host on another release), `docs/privacy.md` (the daily request: to GitHub, no data of the builder, `mode = "off"` stops it), `docs/limits.md` (rows: auto does not run for Homebrew installs; auto waits a day; the iOS app is the App Store's; a host older than this release shows the command, not the key), `docs/console.md` (the door and the host note), `www/index.html` (and `www/app.js`/`www/demo` if the landing's demo lists doors), `CHANGELOG.md` (Unreleased).
- Test: `cmd/matchblox/docs_test.go` and `cmd/matchblox/landing_test.go` already check the docs and the landing against the binary; extend them where they list config keys, doors or commands.

- [ ] **Step 1:** Read `docs_test.go` and `landing_test.go` first; add the failing expectations they need (the `[update]` key, the `update` door).
- [ ] **Step 2:** Write the pages. Each states the three modes with the same words as `docs/config.md`; `docs/config.md` owns the detail and the others link to it.
- [ ] **Step 3:** `go test ./cmd/matchblox` — PASS. Open `www/index.html` in a browser at desktop and phone width and look at the changed part.
- [ ] **Step 4:** Grep the whole diff of the branch for the deny-list (`~/.config/builder/denylist.txt`) and for host or session names of the author's machines: none.
- [ ] **Step 5:** Commit `docs: staying current: the offer, auto and off, in the README, the docs and the landing page (#390)`.

### Task 9: Whole-branch check

- [ ] `go build ./... && go test ./... && go vet ./...` — PASS; the linters CI runs — clean.
- [ ] Walk the spec's "Done when" list line by line; for each line name the test or the hand check that proves it, in the PR body.
- [ ] Open the PR with `Closes #390` and `Refs #389` (it closes the notice half; #389 stays open only if its cause is still wanted).
