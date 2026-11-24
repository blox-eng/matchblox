# Wedge against the ADE: design

Date: 2026-11-24. Status: approved (owner 2026-11-24). Related: Orca probe (mborca),
`DESIGN.md` §1, `design/0001-v0.1.md` §1.1 / §15, `AGENTS.md` (landing).

## Intent

Builders comparing agent “IDEs” meet Electron ADEs that claim terminal /
native while burning ~2 GiB. matchblox already is the light control plane
those pages market: one console, tmux as source of truth, phone and SSH,
one safe action per need.

This work makes that wedge **obvious and felt**, then closes only the
gaps that fit the architecture (phone, remote, CLI) — without becoming an
ADE.

Success: a builder who opens matchblox can **see** that the product stays
small, can **say** why it is not an IDE in one line, and can **finish**
needs-you → answer from desk, phone, or another host without wishing for
Orca’s chrome.

## Goals

1. **Prove lightness in product**, not only in a blog. Machine (and the
   status that already speaks for the machine) names matchblox’s own
   footprint in plain units beside agent load — quiet when normal, clear
   when asked.
2. **One defended category line** on README and in the words we send for
   the landing: matchblox is the tmux control plane for agents, not
   another IDE. No “native TUI” smoke; no ADE feature wall.
3. **Demo evidence** that fits the brand: replay / short clip of
   needs-you → one safe answer (desk + phone), optionally with the
   footprint line visible.
4. **Selective parity that fits**:
   - Phone: needs-you buzz → answer → done stays the hero path; fix
     sharp edges that make remote/phone feel second-class vs desk.
   - Remote / multi-host: status and jump stay as crisp as the single-host
     list (build on `design/0017-every-host.md`).
   - CLI: every console act that matters for agents/stoker keeps or gains
     a CLI twin (`matchblox …`), documented as the agent-facing surface.
5. Landing copy lands in **`www/` in the same feat PR** (user-first brief
   in `design/0021-landing-brief.md`). If the page needs no change, the PR
   body holds `Landing page: unchanged because …`.

## Non-goals

1. Electron, embedded Chromium, WebGL terminal, browser, design mode,
   emulator, plugin marketplace.
2. Slack / Linear / GitHub “bots” as a product surface.
3. Diff review, prompt editor, or owning the agent’s own TUI chrome.
4. Star-count campaigns, commit theater, or download-vanity metrics.
5. Matching Orca feature-for-feature.

## Anti-goals

The work fails, even if it ships, if:

1. matchblox’s idle console+serve grows into Electron-class RAM to “look
   modern.”
2. Copy claims “native” or “GPU terminal” for something that is not.
3. A new surface adds a dead end (a state with no one resolving action).
4. Phone/remote polish needs a web listen port or weakens gate/SSH
   (0005).

## Approaches

| | Path | Trade-off |
|---|---|---|
| A | Messaging only (README + landing words) | Fast; nothing felt in the product |
| B | **Wedge in product + story, then selective phone/remote/CLI** | Real proof; bounded scope |
| C | Chase ADE surface area | Erases the wedge |

**Picked B** (owner, 2026-11-24): “1 and selective 2” — sharpen the
wedge, then only the gaps that fit.

## Shape of the change

### Part 1 — Wedge (first)

- **Machine**: show matchblox’s own RSS (console and/or serve) in the
  machine block already used for host memory — one quiet fact, not a
  dashboard. Prefer existing sample/proc paths; no new daemon.
- **README**: one defended line for builders; measured footprint as a
  quiet footnote (not the lead). Demos stay the visual proof.
- **Landing**: work from the person backwards (Jobs / Apple style). Lead
  with what they feel and can do. No stack, no MiB, no “vs Electron” in
  the hero. Lightness = calm, small, one clear act — shown, not explained.
  Technology and measurements live in Machine, README footnote, docs.
  Edit `www/` in the same feat PR when the story changes.
- **Optional (README / docs only):** measured footnote (desk console
  ~X MiB, serve ~Y MiB on Linux) dated, method named.

### Part 2 — Selective polish (after Part 1 lands or in parallel tasks)

Concrete tickets only where desk already works and phone/remote/CLI lags:

- Phone needs-you path gaps (pair → buzz → answer).
- Multi-host “wait unseen” regressions or dull edges vs 0017’s promise.
- CLI gaps for acts agents/stoker already need from the console.

Each Part 2 item needs its own failing check or issue before build; this
design does not invent a bot platform.

## Constraints

- Language and calm: `DESIGN.md` §1 (silent system, one safe action, no
  dead ends).
- Non-compete: `design/0001-v0.1.md` — not a session manager that edits
  prompts or reviews diffs.
- Landing: edit `www/` in the feat PR when the story changes (`AGENTS.md`).
- Evidence over taste: footprint numbers are measured on a named host/OS
  or not claimed.

## Left to the plan

- Exact Machine row/copy and where serve vs console RSS is sampled.
- Which phone/remote/CLI gaps are in v1 of Part 2 (issue list).
- Apply `design/0021-landing-brief.md` to `www/` in the feat PR.
- Test / replay updates that lock the new Machine line and copy.

## Rulings

- **2026-11-24 — Approach B.** Owner chose wedge first, then selective
  phone/remote/CLI; rejected ADE chase. Reason: keep the measured
  lightness advantage and the existing non-goals.
- **2026-11-24 — Feat PRs edit `www/` when the story changes; otherwise the PR body holds
  `Landing page: unchanged because …`. Reason: landing guard.
- **2026-11-24: spec approved. Decided by: owner.**
- **2026-11-24 — Landing is user-backwards, not tech-forward.** Owner:
  deslopify Apple/Steve Jobs style; do not adopt orca.dev’s feature wall;
  do not put stack or footprint numbers in marketing hero. Reason: the
  wedge is how it feels to keep agents burning, not that it is Go or
  small in RAM. RAM proof stays in product (Machine) and README.
