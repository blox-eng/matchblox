# A readable chat on the phone Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On a 44-column phone, the chat's live screen breaks lines between words, and a question shows once, in the card.

**Architecture:** `fitScreen` in `internal/app/chat.go` wraps at word boundaries in place of a hard cut at the width. When the question card shows, `chatLines` drops the agent's own question block from the screen. That block is the lines from the screen's last rule (`────`) down. If the screen has no rule, nothing is dropped.

**Tech Stack:** Go, `github.com/charmbracelet/x/ansi` (already a dependency: `ansi.Wrap`), Bubble Tea v2 goldens in `testdata/screens`.

**Spec:** `docs/superpowers/specs/2026-10-29-ask-mode-design.md`. The anti-goal this serves: "On a phone held upright, the console is harder to read than it is today." Also its rule "One of everything".

## Global Constraints

- Work in `/home/blox-master/dev/matchblox-261` on `feat/261-ask-mode` (PR #262). Never touch `/home/blox-master/dev/matchblox`.
- Signed Conventional Commits ending in `(#261)`, trailer `Co-Authored-By: <the model you are> <noreply@anthropic.com>`. Never `--no-verify`. Before each commit, run the denylist scan `rg -i -f <(sed '/^$/d' ~/.config/builder/denylist.txt) <(git diff --cached)`. A hit on the repo's own module path is fine.
- Goldens: run `go test ./internal/app -run TestScreens -update`, then read every changed golden before you commit it. If its frames change, regenerate `cmd/matchblox` TestReplay with `-update` too.
- Each task ends with `go test ./...`, `go vet ./...` and `golangci-lint run ./...` passing.
- Docs are in ASD-STE100: short sentences, active voice, present tense.

## Review Focus

1. A word longer than the width (a URL, a path, `example.com/auth/refresh/rotate`) is still cut hard at the width, and no line is wider than `w`. Test `TestFitBreaksALongWordHard` in Task 1.
2. Style codes stay intact across a word wrap: a coloured word that moves to the next line keeps its colour. Test `TestFitWrapKeepsStyles` in Task 1.
3. A card shows, but the screen has no rule (another agent, or a drawn frame). Nothing is dropped, so no line goes missing. Test `TestNoRuleKeepsTheWholeScreen` in Task 2.
4. A permission prompt (no card) keeps the agent's own menu. That menu is the only place its options show. Test `TestPermissionKeepsTheAgentsMenu` in Task 2.
5. Indented tool output (`  ⎿  Read 142 lines`) still fits the width after the wrap. It is covered by the `chatup-44x31` golden in Task 1.

---

### Task 1: The chat wraps between words

**Files:**
- Modify: `internal/app/chat.go:37-58` (`fitScreen`)
- Test: `internal/app/chat_test.go`
- Goldens: `testdata/screens/chat-44x31.txt`, `chatup-44x31.txt`, and the others the update changes

**Interfaces:**
- Consumes: none.
- Produces: `fitScreen(text string, w, h int) []string`. Its signature is unchanged; only its wrap changes.

- [ ] **Step 1: Write the failing tests** in `chat_test.go`:
  - `TestFitWrapsBetweenWords`: `fitScreen("⏺ The refresh endpoint is in and its tests pass.", 44, 10)` returns two lines. `ansi.Strip` of the first is `⏺ The refresh endpoint is in and its tests`, and of the second is `pass.`. No line ends or starts with a part of a word: the second line's first rune follows a space in the original.
  - `TestFitBreaksALongWordHard`: a line of `"see " + strings.Repeat("x", 60)` at w=44 gives lines that are each ≤ 44 wide, and the x's joined back give all 60.
  - `TestFitWrapKeepsStyles`: `"aaaa \x1b[31mredword\x1b[0m"` at w=6 gives a second line that holds `\x1b[31m` before `redword`.
  - `TestFitWrapTrimsBlanks`: no wrapped line ends in a space (`ansi.Strip(l) == strings.TrimRight(ansi.Strip(l), " ")`).
- [ ] **Step 2: Run** `ctx-wire run go test ./internal/app -run 'TestFit'`. Expected: the first and fourth tests FAIL; the second and third may pass.
- [ ] **Step 3: Implement.** In `fitScreen`, replace `ansi.Hardwrap(line, w, true)` with `ansi.Wrap(line, w, "")`, which breaks at spaces and cuts a word longer than `w` hard. Then trim the trailing blanks of each wrapped piece with the existing `trailingBlank` rule, keeping its style codes. Keep `isRule` lines cut as they are now. Update the function's comment: "other text wraps between words".
- [ ] **Step 4: Run** `ctx-wire run go test ./internal/app -run 'TestFit|TestChat'`, then `go test ./internal/app -run TestScreens -update`. Read every changed golden. `chat-44x31` must show `its tests` / `pass.` and `Esc to cancel` whole. Then run `ctx-wire run go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `fix: the chat breaks lines between words, so a phone reads them (#261)`.

### Task 2: A question shows once, in the card

**Files:**
- Modify: `internal/app/chat.go:114-135` (`chatLines`), adding `agentMenuCut`
- Modify: `docs/console.md` (the chat's card section), `docs/superpowers/specs/2026-10-29-ask-mode-design.md` ("The question card")
- Test: `internal/app/chat_test.go`
- Goldens: `chat-*.txt`, `chatup-*.txt`

**Interfaces:**
- Consumes: `fitScreen` (Task 1), `isRule`, `m.cardLines(w)`.
- Produces: `func agentMenuCut(text string) string`. It returns `text` without its last rule line and the lines below it. When `text` has no rule line, it returns `text` unchanged.

- [ ] **Step 1: Write the failing tests:**
  - `TestAgentMenuCut`: on the screen fixture in `screens_test.go` (the one that ends in `Enter to select · ↑/↓ to navigate · Esc to cancel`), the result holds `⏺ The refresh endpoint is in` and does not hold `❯ 1. Rotate it`, `☐ Rotate`, or `Enter to select`.
  - `TestNoRuleKeepsTheWholeScreen`: `agentMenuCut("a\nb\n❯ 1. x")` returns the same text.
  - `TestCardDropsTheAgentsMenu`: at 44x31, a session with an `Ask` and that screen → `chatLines` holds the card's `› 1 Rotate it`, and holds no `❯ 1. Rotate it`.
  - `TestPermissionKeepsTheAgentsMenu`: the same screen, queue state permission, `Ask == nil` (no card) → `chatLines` still holds `❯ 1. Rotate it`.
  - `TestHistoryKeepsTheAgentsMenu`: while scrolled up (`m.screen.Up > 0`), the screen is not cut: history is shown as it was.
- [ ] **Step 2: Run** `ctx-wire run go test ./internal/app -run 'AgentMenu|NoRule|CardDrops|PermissionKeeps|HistoryKeeps'`. Expected: FAIL (undefined `agentMenuCut`).
- [ ] **Step 3: Implement** `agentMenuCut` in `chat.go`. Split on `\n`, find the last index `i` where `isRule(line)` holds, and return the lines before `i`, joined. In `chatLines`, pass `agentMenuCut(m.screen.Text)` to `fitScreen` only when `len(card) > 0` and `m.screen.Up == 0`. This applies on every layout, so a question shows once everywhere (the spec's "One of everything").
- [ ] **Step 4: Docs.** In `docs/console.md`'s card section, add: "While the card shows, the agent's own menu leaves the screen. The card is the one place to answer." In the spec's "The question card", add a line dated 2026-10-30 with the same rule and its reason: two lists of the same options cost a phone a third of its screen.
- [ ] **Step 5: Run** `go test ./internal/app -run TestScreens -update`. Read the chat goldens: each shows the question once, in the card. If its frames changed, run `go test ./cmd/matchblox -run TestReplay -update` and read the question step. Then run `ctx-wire run go test ./...`, `go vet ./...` and `golangci-lint run ./...`. Expected: PASS.
- [ ] **Step 6: Commit** `fix: a question shows once, in the card, not twice (#261)`.
