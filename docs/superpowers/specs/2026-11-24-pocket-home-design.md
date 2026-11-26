# Pocket home: the box

Status: approved (2026-11-25).

## Intent

A live console in Pocket fills the screen, and nothing in it leads out.
Hosts and Island show only on the cover of a dropped or ended console
(`ConsoleView.swift:49`). A first-time user never finds the hosts, the demo
or the text size (owner, 2026-11-24: "I don't see the setting at all").
Pocket gets one home that a person can reach from any console, and that stays
out of the way.

## Goals

1. A person reaches home from a live console in two taps, with no gesture to
   learn.
2. Home lists the paired hosts. A tap opens a host's console. A host can be
   removed, with a question first.
3. The text size, the Live Activity (the Island today), the demo, the privacy
   policy and the sponsor page are on home, quietly.
4. The canon (`DESIGN.md`) gets a Pocket section for all of it.

## Non-goals

- New settings beyond text size. Theme, font family and key repeat stay out.
- A change to the Island's behaviour. Only its name and entry point move.
- A change to the console's own home (`◂`).

## Anti-goals

- Home competes with the matches: the way in must not look like a layout.
- Text size reads as the main thing on home (owner, 2026-11-24).
- An iOS settings list look. Home is drawn as the console: Plex Mono,
  regions with a small uppercase `faint` label and a hairline, no boxes.

## Rulings

- 2026-11-25: spec approved. Decided by: the owner.

## Decided (owner, 2026-11-24)

- **The way in:** a stamp on the matchbox drawer's tray floor, bottom left,
  in the ink and edge of `EDIT MINE` (`BeltLook.stampInk`, `stampEdge`). It
  reads `BOX`, with a small front-on box of §3 (sleeve, striker, tray). The
  tray slides out of the sleeve as the drawer opens (§5 open, 380 ms). The
  agent tag moves one place to the right of it.
- **The idea:** a session is a match; home is the box all the matches lie in.
- **Home's order:** hosts first (`● live` on the connected one, `+ pair a
  host`), then one `Live Activity` row and its `show` line, then one quiet line `text − 13 pt +`
  with a short sample at that size, then the footer.
- **Footer:** `demo · privacy ↗ · ♥ sponsor ↗`, in `faint`. Privacy opens
  docs.matchblox.com/privacy; sponsor opens github.com/sponsors/blox-eng
  (live, 2026-11-24).
- **First run** (no host): home opens instead of the scanner. The hosts
  region says one sentence, then `pair a host` and `try the demo`.
- **Text size** leaves the Island screen. The pinch on the terminal stays.

## Decided in build (owner, 2026-11-27)

- **Show on home:** what the island and the lock screen show is one tap on
  home. Under the `Live Activity` row, one quiet line:
  `show  sessions · usage · auto`. The choice in force is in the accent;
  the others are `faint`. A tap saves `IslandPrefs.show` and refreshes the
  activities, as the Live Activity screen does. The row then reads
  `work-1 · buzz on    ›`: the show word is on its own line, once. The
  Live Activity screen keeps its `Show` picker for the long `auto` text.
  Reason: the owner wants the choice one tap away (relayed by
  matchbloxisland, confirmed by the owner in this session).

## Decided in design review (owner, 2026-11-24)

- **1.1 Order:** hosts lead with full rows; `Live Activity` gets one row; the
  text line and the footer are `faint`. Home has one job: the hosts.
- **1.2 Name:** `Island` becomes `Live Activity` in the app's copy and §9.
  The activity shows on the lock screen, in the Dynamic Island, in StandBy
  and on the Watch, so a place name is wrong. Apple uses the same word in iOS
  Settings. The buzz shows under it as a sub-line. Code names stay.
- **1.3 Version:** in the footer, `faint`, last: `demo · privacy ↗ · ♥
  sponsor ↗  0.58.0`.
- **1.4 Back:** a visible `‹ work-1` at the top left names the console you
  came from and returns to it as it was. On first run, or with no console
  behind home, it does not show.

```
‹ work-1
[box] matchblox
HOSTS · 2 ──────────────────────
▸ work-1  10.0.0.4        ● live
  laptop  laptop.local    2h ago
+ pair a host
LIVE ACTIVITY ──────────────────
  work-1 · sessions  buzz on    ›
────────────────────────────────
text  − 13 pt +     ▰ flint-auth ╿
demo · privacy ↗ · ♥ sponsor ↗  0.58.0
```

### States

| Thing | State | What the person sees |
|---|---|---|
| Host row | live | `● live` in the accent; the row under `‹ <host>` |
| Host row | reachable, not open | when it was last seen, `2h ago`, in `faint` |
| Host row | not reachable (2.1) | `○ not reachable · since 14:02` in `faint`; a tap still opens it, and the console's cover says why and the fix |
| Host row | checking | the last known line stays; no spinner |
| Remove | question (2.2) | inline under the row: `remove work-1? the phone forgets its key; matchblox pair brings it back.` For the live host it adds `its console closes`. `[remove]` in `neg`, `[keep]` |
| Remove | done | the existing notice `removed work-1`; a live console closes and `‹ work-1` goes |
| Remove | last host (2.3) | home stays and shows the first-run state, under the notice |
| Text size | at 9 or 24 pt | `−` or `+` dims (`TextSize.isSmallest`/`isBiggest`) |
| Live Activity row | demo host only | no row (today's rule: only a real host) |
| Footer `demo` | demo host paired (2.4) | `demo` leaves the footer; the host row is the way in |
| Footer `demo` | no host (first run) | `demo` leaves the footer; `try the demo` in the hosts region is the one way in (2026-11-26: two `Try the demo` controls on one screen broke VoiceOver and the UI test) |

### Journey (Pass 3)

- **3.1 First run:** the first time Pocket opens with no host, the canon
  mark plays once: it falls, opens, the match strikes and stands
  (`DESIGN.md` §3: 300/380/480/560 ms). Then home shows the first-run
  sentence with `pair a host` and `try the demo`. Later visits with no
  host skip the play and open on that same quiet first-run home.
- **3.2 Teaching BOX:** after the first successful pair lands in a
  console, the drawer opens once with the tray out and `BOX` briefly
  accented (same ink language as a strike flare), then settles. Never
  again. A later pair or a reinstall that already knows a host does not
  replay it.
- **3.3 Remove:** home has no belt. A tap opens a host. A swipe left on
  a host row asks to remove, with the Pass 2 question line under the
  row. No `−` chrome on the rows.
- **3.4 Text size:** the home line is a convenience. Pinching the
  terminal is the main control. Home never leads with type size.

| Step | Person does | Person feels | Spec |
|---|---|---|---|
| 1 | Opens Pocket with no host | brand, then calm | 3.1 box plays once → first-run home |
| 2 | Pairs a host (or tries demo) | oriented | lands in that console |
| 3 | Sees the drawer teach | “ah, BOX” | 3.2 tray out, `BOX` accents once |
| 4 | Works in the console | in the match | drawer closed; matches lead |
| 5 | Needs hosts / size / Live Activity | two taps out | pull drawer → tap `BOX` |
| 6 | On home, taps a host or `‹ host` | back to work | 1.4; live row stays |
| 7 | Swipes a host left | sure, not trapped | 3.3 → Pass 2 remove question |
| 8 | Later visits | quiet | no replay of 3.1 or 3.2 |

### Canon (Pass 5)

- **5.1 Pocket section in `DESIGN.md`:** one new Pocket section that
  names the decided surface: the `BOX` stamp (`BeltLook.stampInk` /
  `stampEdge`, tray floor bottom left, agent tag one place to the right);
  home order (hosts → Live Activity → text line → footer); no belt (tap
  opens, swipe left asks to remove); `‹ host` back; first-run mark play
  once; BOX teach-once after first pair; text size on home as a
  convenience (pinch stays primary). §9 copy: `Island` becomes `Live
  Activity` in the app's words; code names stay. Island behaviour does
  not change.

### Access (Pass 6)

- **6.1 BOX tap:** the stamp stays small (~10–11 pt ink, edge as
  `EDIT MINE`). Its hit area is at least 44×44 pt via an invisible
  `contentShape` (same pattern as `EditStamp`). VoiceOver: `Home, the
  box`.
- **6.2 Reduce Motion:** when Reduce Motion is on, skip the first-run
  mark play (3.1) and the BOX teach (3.2). Land on the quiet end state:
  first-run home, or the drawer closed with `BOX` already on the tray.
  The “once” flags still set so neither replays later.
- **6.3 VoiceOver:** each control has one spoken line:

  | Control | VoiceOver |
  |---|---|
  | `‹ work-1` | `Back to work-1` |
  | Host row | `work-1, 10.0.0.4, live` (or `2 hours ago` / `not reachable since 14:02`) |
  | Host row actions | Open (default); Remove (custom / swipe) |
  | `+ pair a host` | `Pair a host` |
  | Live Activity row | `Live Activity, work-1, buzz on` |
  | Show line | `Show sessions, selected` · `Show usage` · `Show auto, usage while the stoker runs` |
  | Text `−` / `+` | `Smaller text` / `Bigger text` |
  | Footer | `Try the demo` · `Privacy policy` · `Sponsor Matchblox` |
- **6.4 Tap height:** host rows, the Live Activity row, each `show` word, text `−` / `+`,
  and each footer link have a hit area at least 44 pt tall (paint may stay
  tighter; the shape expands as with `BOX`).

## Approaches considered

- **A stamp in the drawer (picked).** Always one pull away, costs no
  terminal cell, and reads as part of the matchbox.
- A corner glyph over the terminal: easy to find, but it covers text.
- A swipe from the left edge: invisible to a first-time user, and it fights
  selection and the scroll wheel.

## Left to the builder

- The route enum and how `HostsView` grows into home.
- The exact first-run sentence on the hosts region (one line, calm).
- Where the “once” flags for 3.1 and 3.2 are stored (UserDefaults is enough).

## NOT in scope

- Sidebar briefly showing a match twice (standalone + in chain). Separate
  desk bug; not home. Filed as #506 from review 7.1.
- Island / Live Activity behaviour beyond rename, moving its entry to
  home, and the one-tap `show` line (non-goal). What each choice draws
  stays with the island work.
- Theme, font family, key repeat (non-goal).
- The console’s own home (`◂`).
- Regenerating the Claude artifact mockup in this review (`$D` not used;
  v4 artifact remains the visual reference).

## What already exists

- `DESIGN.md` §3 mark timings, §5 open (380 ms), §9 Island; console
  regions (`faint` label + hairline); tokens `faint` / `accent` / `neg`.
- `BeltLook.stampInk` / `stampEdge`; `EditStamp` hit-area pattern
  (`contentShape` inset); `MatchTray` tray floor.
- `HostsView` (list, pair, demo, remove, Island entry); `IslandView`
  (Follow / Show / Buzz / text size); `TextSize` + terminal pinch;
  `ConsoleView` cover for hosts/Island today.
- Footer destinations live: privacy docs, GitHub Sponsors.

## Implementation Tasks

Synthesized from this review. For the plan after approval.

- [ ] **T1 (P1)** — Tray — Add `BOX` stamp (EDIT MINE pattern, ≥44×44 hit,
  VO `Home, the box`); shift agent tag right; teach-once after first pair;
  honor Reduce Motion.
- [ ] **T2 (P1)** — Home — Grow `HostsView` into home: order, states table,
  `‹ host` back, swipe-left remove, Live Activity row, text line, footer
  (demo/privacy/sponsor/version), first-run + no-host path, VO labels,
  ≥44 pt taps.
- [ ] **T3 (P1)** — Live Activity — Rename Island copy to Live Activity in
  app strings and `DESIGN.md` §9; keep code names; move entry from cover
  to home row; text size leaves Island screen.
- [ ] **T4 (P2)** — Canon — Write the Pocket section in `DESIGN.md` per 5.1.
- [ ] **T5 (P2)** — First run — Canon mark play once (or skip under Reduce
  Motion); then first-run home.

## Design Review Completion Summary

```
  +====================================================================+
  |         DESIGN PLAN REVIEW — COMPLETION SUMMARY                    |
  +====================================================================+
  | System Audit         | DESIGN.md present; UI scope = Pocket home   |
  | Step 0               | 5/10; gaps: states, a11y, lens calls        |
  | Pass 1  (Info Arch)  | 5/10 → 9/10 after 1.1–1.4                   |
  | Pass 2  (States)     | 3/10 → 9/10 after 2.1–2.4 + states table    |
  | Pass 3  (Journey)    | 5/10 → 9/10 after 3.1–3.4 + storyboard      |
  | Pass 4  (AI Slop)    | 10/10 → 10/10 (Operate; no issues)          |
  | Pass 5  (Design Sys) | 6/10 → 9/10 after 5.1 Pocket canon section  |
  | Pass 6  (Responsive) | 4/10 → 9/10 after 6.1–6.4                   |
  | Pass 7  (Decisions)  | 1 resolved (7.1 → backlog issue)           |
  +--------------------------------------------------------------------+
  | NOT in scope         | written (5 items)                           |
  | What already exists  | written                                     |
  | TODOS.md updates     | 0 (repo uses GitHub issues; 7.1 filed)      |
  | Approved Mockups     | 0 via $D; Claude artifact v4 is reference   |
  | Decisions made       | 18 (… + 7.1)                                |
  | Decisions deferred   | first-run sentence; route enum (builder)    |
  | Overall design score | 5/10 → 9/10                                 |
  +====================================================================+
```

### Pass 7

- **7.1 Sidebar duplicate:** filed as #506; do not block Pocket home.
  (Owner, 2026-11-25.)

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | clean | score: 5/10 → 9/10, 18 decisions |

- **VERDICT:** DESIGN CLEARED for implementation after owner approves this spec. Eng review required before/with the plan.

NO UNRESOLVED DECISIONS
