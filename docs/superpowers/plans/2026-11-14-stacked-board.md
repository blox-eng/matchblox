# The Stacked Board at Medium Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** At Medium (60–99 columns) the board draws `NEEDS YOU`, `WORKING` and `SHIPPING` stacked from top to bottom at full width, so the owner can read every wait and what each match does at 80 columns.

**Architecture:** A new `boardStacked` in `internal/app/boardstack.go` replaces `boardColumns` for `l == Medium` in `boardPanel`. It draws each section's head on its own line (tap span in `d.taps`) and its cards with the existing `boardStack` at x=0, full width. A pure `stackRows` gives each section its rows: `NEEDS YOU` its cards, then `SHIPPING` its ships at the bottom and `WORKING` the rest (owner, 2026-11-14). `WORKING` and `SHIPPING` cards are one line at Medium. Keys reuse the column moves: `←` `→` move between sections; `↓` `↑` cross a section's end.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, the golden screens in `testdata/screens/`.

**Spec:** `docs/superpowers/specs/2026-11-14-stacked-board-design.md` (amends `2026-11-10-the-board-design.md`, Layout and Anti-goals).

## Global Constraints

- Narrow and Wide do not change: every golden of another width stays byte for byte.
- No change to `matchblox sessions --json`, `peek` or `send`.
- No new key, no setting for the layout.
- `NEEDS YOU` takes all its cards' rows first. When `NEEDS YOU` does not fit, it takes all rows but 4; `WORKING` and `SHIPPING` keep 2 each (head and one row). `SHIPPING` sits at the bottom: its head and one row for each ship, 3 rows at most, whatever `WORKING` holds (ruling, 2026-11-14). `WORKING` takes the rest; its blank rows are under its cards (owner, 2026-11-14, in place of a fixed 3:2 split).
- One blank row between sections when rows allow; a blank row goes first when rows are short.
- The key line says `←→ sections` at Medium, `←→ columns` elsewhere.
- Fixtures use the canon's names (DESIGN.md §7). `go test ./...` passes before every commit. Signed Conventional Commits, no `--no-verify`, deny-list check on the staged diff.

## Review Focus

1. A match moving from `WORKING` to `SHIPPING` while `NEEDS YOU` is unchanged: no section head moves.
2. 4 waits at 80×31 with the chosen card holding a 3-line quote and 2 options: every wait whole, no `↓ N more` in `NEEDS YOU`.
3. A hidden section at Medium: its rows go to the others, its name waits on the tab row, `←` `→` skip it.
4. QUIET open (the drawer at 8 rows) at 80×31: the sections still draw their heads.
5. A chain narrowed (`g`) at Medium: the room's talk lines close the sections, over the drawer, as today.

---

### Task 1: The row split

**Files:**
- Create: `internal/app/boardstack.go`
- Test: `internal/app/boardstack_test.go`

**Interfaces:**
- Produces: `func stackRows(need, ships, rows int, work, ship bool) (needH, workH, shipH, gaps int)` — `need` is the rows `NEEDS YOU`'s cards take (0 when hidden), `ships` the count of `SHIPPING` (owner, 2026-11-14), `rows` the body rows for the sections, `work`/`ship` whether each is shown. Each height includes the section's head line. `gaps` is how many blank separator rows to draw (0, 1 or 2).

- [ ] **Step 1: Write the failing test** `TestStackRowsSplitsTheRestThreeToTwo`, table-driven:
  - `stackRows(9, 21, true, true)` → needH 10, workH 6, shipH 3, gaps 2 (rest = rows − needH − gaps = 9; workH = ceil(rest·3/5); shipH = rest − workH).
  - `stackRows(30, 21, true, true)` → needH 17, workH 2, shipH 2, gaps 0 (all rows but 4 to need; the gaps go first).
  - `stackRows(9, 21, true, false)` → shipH 0, workH = rest.
  - `stackRows(0, 21, true, true)` with need hidden → needH 0, gaps 1, workH 12, shipH 8.
  - Same `need` and `rows` with any counts in work/ship give the same result (the function takes no counts).
- [ ] **Step 2:** Run `go test ./internal/app -run TestStackRows` — FAIL (undefined).
- [ ] **Step 3:** Implement `stackRows`.
- [ ] **Step 4:** Run it — PASS.
- [ ] **Step 5:** Commit `feat(board): at Medium the sections split the rows NEEDS YOU leaves 3 to 2 (#406)`.

### Task 2: The stacked body

**Files:**
- Modify: `internal/app/boardstack.go` (add `boardStacked`), `internal/app/board.go:154-163` (dispatch), `internal/app/board.go:710` (`oneLineIn` at Medium)
- Test: `internal/app/boardstack_test.go`, `internal/app/screens_test.go` goldens at 80×31

**Interfaces:**
- Consumes: `stackRows`; `boardStack(d, b, c, y, x, w, h, quietRow)`; `boardHead(c, n, w, open)`; `boardDrawer`.
- Produces: `func (m Model) boardStacked(d *boardDrawn, b *boardDraw, shown []boardCol, w, rows int, talk []roomDrawn)`. Sets `d.headAt` to the first section head's line and `d.colsTo` to the line under the last section. Each section head line gets `d.taps[line] = []span{{0, w, c.short()}}` so a tap on it hides the section (the same press `d.heads` carries for columns).
- `oneLineIn(c)` is true for `colWork` and `colShip` when `m.layoutAt(m.width) == Medium`.

- [ ] **Step 1: Write failing tests:**
  - `TestMediumStacksTheSections`: the fixed fleet at 80×31; the plain body has `NEEDS YOU · 4` on a line of its own, then all four wait names, then `WORKING · 3`, then `SHIPPING · 3`, in that order, each head starting at column 0.
  - `TestMediumShowsEveryWait`: for 1 to 4 waits, each chosen in turn (the chosen card open), no `↓` line inside `NEEDS YOU` at 80×31.
  - `TestMediumHeadsHoldStill`: take the head lines' indexes; then (a) change a working match's text, (b) move a match from `WORKING` to `SHIPPING`, (c) add a working match, (d) move the selection within `WORKING`; the indexes of all three heads are unchanged.
  - `TestMediumBusyFleet`: 2 waits and 13 working matches (canon names) at 80×31; both waits whole, `WORKING` ends in `↓ N more`, golden `board-busy-80x31`.
- [ ] **Step 2:** Run them — FAIL.
- [ ] **Step 3:** Implement `boardStacked` and route `l == Medium` in `boardPanel` to it (the room's talk lines and the drawer as `boardColumns` places them on Medium). `NEEDS YOU`'s height: the sum of its blocks' lines (`boardBlockOf`), capped by `stackRows`.
- [ ] **Step 4:** Run `go test ./internal/app -run 'TestMedium|TestStackRows'` — PASS. Regenerate the 80×31 goldens with the project's update flag; check by eye that only Medium goldens changed (`git diff --stat testdata/`).
- [ ] **Step 5:** Fix the existing Medium tests that pin columns (`board_test.go:32-34`, `TestQuietIsADrawerOnMedium`, `boardshort_test.go`, `boardhosts_test.go:59`): each keeps its intent on the stack.
- [ ] **Step 6:** `go test ./...` — PASS. Commit `feat(board): at Medium the board stacks its states, each the full width (#406)`.

### Task 3: Keys, taps and the key line

**Files:**
- Modify: `internal/app/boardkeys.go` (the `↑` `↓` move at a section's end on Medium), `internal/app/keys.go:165-166` (help word at Medium), the key-line builder.
- Test: `internal/app/boardkeys_test.go`

**Interfaces:**
- Consumes: the column move that `bk.Left`/`bk.Right` run today (unchanged: it already skips hidden and empty columns).

- [ ] **Step 1: Write failing tests:**
  - `TestMediumDownCrossesIntoTheNextSection`: choose the last wait, `↓` chooses the first working match; `↑` comes back to the last wait.
  - `TestMediumArrowsMoveBetweenSections`: `→` from `NEEDS YOU` opens `WORKING`; with `WORKING` hidden, `→` opens `SHIPPING`.
  - `TestMediumTapOnASectionHidesIt`: a tap on each section's head line hides it; `+ work 3` shows on the tab row.
  - `TestMediumKeyLineSaysSections`: the key line at 80×31 has `←→ sections`; at 104×38 it has `←→ columns`.
- [ ] **Step 2:** Run — FAIL.
- [ ] **Step 3:** Implement. Wide and Narrow keep their `↑` `↓` (stop at a column's end).
- [ ] **Step 4:** `go test ./...` — PASS.
- [ ] **Step 5:** Commit `feat(board): at Medium the arrows move between sections, and ↓ goes on into the next (#406)`.

### Task 4: Docs

**Files:**
- Modify: `docs/console.md` (the board's layout at Medium), `docs/keys.md` (`←` `→`), `CHANGELOG.md` (Unreleased, Changed).

- [ ] **Step 1:** Write the three changes in ASD-STE100, from the spec. CHANGELOG: `**At 60 to 99 columns the board stacks its states.**` with what the owner now sees, and `(#406)`.
- [ ] **Step 2:** `go test ./...` (docs tests, if any) — PASS. Commit `docs: the board at Medium is a stack (#406)`.
- [ ] **Step 3:** Landing page: brief matchbloxrelease with the words and the 80×31 golden, or put `Landing page: unchanged because <reason>` in the PR body. Check with `.github/scripts/landing-guard.sh`.
