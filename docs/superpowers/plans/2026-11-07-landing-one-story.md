# Landing page: one story — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** matchblox.com tells one matchbox story in six beats and about half the words, with a preview card for shared links.

**Architecture:** A static page: `www/index.html`, `www/styles.css` and `www/app.js`, with no build step. A Go test in `cmd/matchblox` pins the page's structure, next to the existing landing tests. The OG image is a committed PNG, rendered once from a small HTML card.

**Tech Stack:** HTML, CSS, vanilla JS. Go tests (`go test`). Playwright (MCP) for the browser checks and the PNG.

**Spec:** `docs/superpowers/specs/2026-11-07-landing-one-story-design.md`

## Global Constraints

- Beats in order, with H2 ids: Strike (hero, the H1), `box-h` "Every agent, one box", `chain-h` (the chain), `stoker-h` "Keep it burning while you sleep", `pocket-h` "Carry it in your pocket", `strike-h` "Strike one".
- `<title>`: `matchblox · a terminal console for every coding agent`.
- Hero sub, exactly: `One terminal console for every coding agent you run. See who waits for you and answer in a line.`
- Night lede, exactly: `Press n, say what to keep going, and sleep. The stoker works your sessions through the night and leaves a card for the morning. Merges, releases and deploys wait for your word.`
- Visible words ≤ 480: the body text with `<pre>`, `<script>`, `<style>` and `<svg>` removed. The baseline is 862.
- No text tells the visitor how to use an interactive element. The words "Tap a match" and "Try g and s" do not appear.
- The chain section's lede and beats stay as they are. matchblox-chain owns those words. `Matches outside the chain cannot.` stays (pinned by `TestTheLandingSaysMatchesOutsideTheChainCannot`).
- Keep the markers `<!-- replay:still -->`, `<!-- /replay:still -->`, `<!-- stoker:still -->` and `<!-- /stoker:still -->`, the `class="news"` line (`news-guard.sh`), and the install commands (`internal/release` tests).
- No change to fonts, colours, the fold or the matchbox canvas.
- Commits are signed Conventional Commits. The PR is `feat(www): …` with a `### Changed` entry in `CHANGELOG.md`.

## Review Focus

- Reduced motion: the first-match flicker must not run under `prefers-reduced-motion: reduce`. The row stays lit, as `run()` already does → Task 4 test.
- Keyboard: once the tip text and `aria-describedby` are gone, the row still has `role="button"` and `tabindex="0"`, and Enter lights it → Task 4 test.
- Phone width 390 px: the new closing install line wraps with no horizontal scroll → Task 6 check.
- A second install block (Strike one) duplicates element ids (`cmdline`, `copy`). The ids must stay unique → Task 2 test.
- The OG image path is absolute (`https://matchblox.com/assets/og.png`). Scrapers ignore relative `og:image` → Task 2 test.

---

### Task 1: Mockup (gate)

**Files:** Create the scratchpad file `landing-story-mockup.html` (not in the repo).

- [ ] **Step 1:** Load the `artifact-design` skill. Build the mockup from the live `www/index.html` with every change in the spec's beat table applied. The replays may be static stills.
- [ ] **Step 2:** Publish it as an artifact and give Martin the link.
- [ ] **Step 3:** Stop until Martin approves it. Fold his changes into the Global Constraints before Task 2.

### Task 2: The structure test

**Files:** Create `cmd/matchblox/landing_test.go`.

**Interfaces:** Produces `landingPage(t) []byte` and `visibleWords(page []byte) int` for Tasks 3–5.

- [ ] **Step 1: Write the failing tests**

```go
func TestTheLandingTellsOneStory(t *testing.T)       // exactly one "<h1"; h2 ids in order: box-h, chain-h, stoker-h, pocket-h, strike-h
func TestTheLandingIsShort(t *testing.T)             // visibleWords(page) <= 480
func TestTheLandingNeedsNoInstructions(t *testing.T) // !Contains "Tap a match", "Try g and s", "Every agent you run, in one list"
func TestTheLandingHasAPreviewCard(t *testing.T)     // <title> as Global Constraints; og:title, og:description, og:url, twitter:card=summary_large_image;
                                                     // og:image == "https://matchblox.com/assets/og.png"; image.DecodeConfig(www/assets/og.png) == 1200x630
func TestTheLandingIdsAreUnique(t *testing.T)        // every id="…" occurs once
```

- [ ] **Step 2:** Run `ctx-wire run go test ./cmd/matchblox -run 'TestTheLanding' -count=1`. Expected: FAIL in all five, except possibly `IdsAreUnique`.
- [ ] **Step 3:** Commit as `test(www): the landing page's one-story structure is pinned`.

### Task 3: Head, Strike and The box

**Files:** Modify `www/index.html` (the head, the hero and `#console`). Create `www/assets/og.png`.

- [ ] **Step 1:** Set the `<title>`. Add the OG and Twitter meta tags with the values from Task 2.
- [ ] **Step 2:** Render `og.png`: a 1200×630 card with the mark, the H1 and the title line, in dark theme colours. Take a Playwright screenshot of a scratchpad HTML file. Commit the PNG only.
- [ ] **Step 3:** Replace the hero `.sub` with the exact sub line.
- [ ] **Step 4:** In `#console`, add `<h2 id="box-h">Every agent, one box</h2>` and point the section's `aria-labelledby` at it. Below the caption, add one `.note`: `Claude Code, Codex, OpenCode and Cursor also show their context and account. Add any other agent by its command in <code>agents</code>.`
- [ ] **Step 5:** Run `TestTheLandingHasAPreviewCard`. Expected: PASS. Commit as `feat(www): the page names its job, and a shared link shows a card`.

### Task 4: The chain invites the tap

**Files:** Modify `www/index.html` (`.chain`), `www/styles.css:317` and the chain block in `www/app.js` (≈ lines 500–600).

- [ ] **Step 1:** Delete the `#chain-tip` span, its CSS rule and the svg's `aria-describedby`. Keep the svg's `aria-label`, `role="button"`, `tabindex="0"` and the `<code>`.
- [ ] **Step 2:** In `app.js`, add a beckon: until the first `pointerenter`, `pointerdown` or `keydown` on the svg, match 0 gets class `beckon` every 2.4 s for 600 ms. Skip it when `still()` is true. In CSS, `.beckon .glow` pulses opacity 0 → .5 → 0. `g:hover .head` lifts with `translateY(-3px)`, only under `@media (hover: hover)`.
- [ ] **Step 3:** In Playwright on `www/` served locally (`python3 -m http.server -d www`):
  1. Focus the svg and press Enter: match 0 gets `lit` first.
  2. Click the middle match: `lit` spreads both ways.
  3. With `emulateMedia({reducedMotion:'reduce'})`: no `beckon` class appears in 5 s.
- [ ] **Step 4:** Run `TestTheLandingNeedsNoInstructions`. It still fails until Task 5. Commit as `feat(www): the first match beckons instead of a line telling you to tap`.

### Task 5: The night, the pocket, the cuts, Strike one

**Files:** Modify `www/index.html`, `www/app.js` (the copy handler) and `CHANGELOG.md`.

- [ ] **Step 1:** In `#orders`, set the H2 id to `stoker-h` and the text to "Keep it burning while you sleep". Use the exact night lede. Replace the stage's `.say` hint "Try g and s, or z on a row." with nothing (keep its `<b>` line). Delete the ways `.note`. Add `<p class="note"><a href="https://docs.matchblox.com/stoker/">The stoker →</a></p>`.
- [ ] **Step 2:** Delete the second stoker section (`stoker-h` "Keep it burning while you sleep" with its two `.run` figures), the agents section (`agents-h`) and `section.blocks[aria-label="What it does"]`.
- [ ] **Step 3:** Pocket. Each `<p>` in its three blocks becomes one sentence:
  - "No account and no password: the scan is the consent."
  - "A sealed push the moment a session needs you, on the Lock Screen and in the Dynamic Island."
  - "Tap a session, answer with a tap, and swipe back through what it did."
- [ ] **Step 4:** Before `<footer>`, add `<section class="install end" aria-labelledby="strike-h">`. It holds `<h2 id="strike-h">Strike one</h2>`, a `.cmd` with `<code id="cmdline-end">` holding the script command, `<button class="copy" id="copy-end">` and a `.meta` line with the Docs and GitHub links. In `app.js`, make the copy handler copy the text of the `.cmd code` beside the clicked `.copy`, not `#cmdtext`.
- [ ] **Step 5:** Add `### Changed` under `[Unreleased]` in `CHANGELOG.md`: "**The page tells one story.** matchblox.com reads as one matchbox, in half the words, and a shared link shows a card."
- [ ] **Step 6:** Run `ctx-wire run go test ./cmd/matchblox ./internal/release -count=1`. Expected: PASS, including all five `TestTheLanding*` tests. Commit as `feat(www): the page tells one matchbox story, in half the words`.

### Task 6: Verify and ship

- [ ] **Step 1:** In Playwright at 390×844 and 1440×900: `document.documentElement.scrollWidth <= innerWidth`. Screenshot each beat. Both copy buttons put their command on the clipboard.
- [ ] **Step 2:** Run `.github/scripts/news-guard.sh 0.35.0` (or the newest released version). Expected: `OK`.
- [ ] **Step 3:** Grep the diff for private names. Push. Open a PR that links the spec and attaches the screenshots. Turn on squash auto-merge.
- [ ] **Step 4:** After it merges and Release runs, load matchblox.com and check the six beats and the preview card. One way to check the card: `curl -s https://matchblox.com | rg og:image`, then load the image.
