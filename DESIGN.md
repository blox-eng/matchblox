# matchblox design

Status: binding (2026-10-07). Language: ASD-STE100. Code names stay as they
are.

This file is the one source for how matchblox looks, moves and speaks: in the
console, on matchblox.com and in the README. Where a design document
(`design/0001` §10, `design/0003` §4) gives other values, this file applies.
A change to a token, a glyph or a motion changes this file in the same pull
request.

## 1. Idea

matchblox keeps the fire burning. Each coding agent is a **match**. A match
burns while its agent works, rests while it waits for the person, and is
spent when its context is full. The builder's work is to keep the matches
burning: answer what waits, compact what is spent.

Four rules follow from the idea. Every screen is judged against them.

1. **The work of the system is silent. The act of the person has weight.**
   Only fire and a session that needs the person move. A confirmed action
   has a clear end.
2. **One meaning for each sign.** A glyph, a colour or a motion means one
   thing everywhere: in the console, on the site, in the README.
3. **No dead ends.** Each state that needs the person shows the one action
   that resolves it, in the same place. An error tells the fix.
4. **Calm by default.** A normal row is quiet. Colour is an event: only an
   exception gets a colour wash, and the accent appears once in each view.

## 2. Colour

Dark first. The light set applies when the terminal background is light (the
console asks the terminal) or the visitor picks light (the site).

### 2.1 Console tokens

The code is `internal/app/theme.go`. `app.Tokens(dark)` names each colour;
the replay maps every colour of a frame to its token, so a colour that is
not a token fails the replay test.

| Token | Dark | Light | Use |
|---|---|---|---|
| `bg` | `#0F0D0A` | `#F5F1E7` | ground |
| `fg` | `#EBE6DC` | `#1C1813` | text |
| `muted` | `#A69D8D` | `#5F5A4C` | secondary text |
| `faint` | `#776F60` | `#8A8170` | labels, units, a spent match |
| `hair` | `#2E2921` | `#DDD5C3` | hairlines |
| `accent` | `#C3A56A` | `#9A7B3F` | the active tab, the selection, the one primary action, a session that asks |
| `wash` | `#221D14` | `#ECE3CF` | the background of the selected row |
| `warn` | `#B98F44` | `#8A6420` | a warning |
| `neg` | `#B0563F` | `#9E4A34` | a failure, a destructive action, context at the compact limit |

The match and the mark (§3, §4):

| Token | Dark | Light | Use |
|---|---|---|---|
| `flame` | `#E8822E` | `#E8822E` | the flame |
| `flame-amber` | `#F5A54A` | `#DF8E2C` | the flame, between its orange and its core |
| `flame-core` | `#FFD27A` | `#D69A2A` | the core of the flame, the flash of a strike |
| `match-head` | `#C2452D` | `#B23A24` | the head of a match at rest |
| `match-breath-1..3` | `#9E3A26` `#7A2F1F` `#572318` | `#BF5F4B` `#CD8372` `#DAA899` | the head as it breathes, toward the ground |
| `match-ember` | `#9C5A46` | `#9E5E4A` | a flame that dies, a head that burns away |
| `mark-top` `mark-left` `mark-right` | `#3A3328` `#1C1914` `#2A251D` | `#EFE9DC` `#CFC6AF` `#DED6C3` | the faces of the box |
| `mark-tray` `mark-striker` `mark-stick` | `#15120E` `#5A4128` `#C9A877` | `#C4BAA2` `#8C6A43` `#B8925B` | the drawer, the striker, the wood |

### 2.2 Site tokens

The site (`www/styles.css`) has its own text tokens for a page (`--bg`,
`--fg`, `--muted`, `--faint`, `--hair*`, `--primary`) and the console tokens
as `--t-<token>` for the replay. A replay span gets the class `fg-<token>` or
`bg-<token>`. To add a token: add it to `Tokens()`, to both `--t-` sets
(dark and light) and the class list in `styles.css`, and to the tables here.

## 3. The mark

The mark is an isometric matchbox: three faces, a drawer, a striker on the
side, and a match that stands on the drawer.

- **Site:** `www/assets/matchblox-mark*.svg`, and the strike canvas in the
  hero (`app.js`): blocks fall into the box, the drawer opens, the match
  strikes and burns. In light mode it is daylight: the match lights only
  while the pointer is on it.
- **Console start screen:** the same mark, drawn with half-blocks (two pixels
  in each cell), 40 × 24 pixels. Fall to 300 ms, open to 380 ms, strike to
  480 ms, stand to 560 ms. In a dark terminal the match burns; in a light
  terminal it stays unlit. Any key stops it. `--no-motion` or `NO_MOTION=1`
  shows the last frame only. The start screen stays while the console waits
  for the service.
- **Console header:** `▰ matchblox · <host>`. The `▰` is the accent.

## 4. The match

The match is the element of matchblox. One cell, and the character is the
state. The code is `internal/app/match.go`.

| State | Glyph | Colour | Motion | Means |
|---|---|---|---|---|
| burning | `✦` | `flame`, `flame-amber`, `flame-core` | flickers: a new colour each 200 ms; `✧` for one step in six | the agent works |
| at rest, asks | `╿` | `match-head` → `match-breath-1..3` | breathes: one slow breath each 2.4 s | the agent waits for an answer or a permission |
| at rest | `╿` | `match-head` | still | the agent finished its turn and waits |
| spent | `│` | `faint` | still | the context is at or above the compact limit (`sessions.compact_at`, default 85 %) |

Order: spent comes first. A session that works with a full context is spent.

A change of state plays once:

| Change | Time | Frames |
|---|---|---|
| **The strike**: at rest → burning | 800 ms | `╿` `flame-core` (the head flashes, 200 ms) → `·` `flame-core` (a spark, 300 ms) → `✧` `flame-amber` (it grows, 300 ms) → burning |
| **Goes out**: burning → at rest | 400 ms | `✧` `flame-amber` → `✧` `match-ember` → at rest |
| **Burns away**: → spent | 600 ms | `╿` `match-ember` → `│` |

Rules:

1. The glyph never stands alone: the state word stays next to it
   (`✦ busy`, `╿ asks`, `│ burnt`).
2. A match means one agent session. Do not use it for anything else.
3. The console wakes only while something moves: a burning match, a match
   that asks, or a change that still plays. Then it draws 10 times a second;
   else it draws only on a new state or a key.
4. Motion comes from the clock, not from a frame count, so the same moment
   always draws the same frame (the replay depends on it).
5. With `--no-motion` or `NO_MOTION=1`: `✦` in `flame`, `╿` in
   `match-head`, `│` in `faint`, no change plays.
6. Every place that shows an agent uses this match: the queue, the sessions,
   the replay, the site, and later the tmux status line, the overnight view
   (night mode) and the docs. A new place uses the same glyphs and motions.
   A new state is a change to this section first.

## 5. Motion

| Motion | Time | Use |
|---|---|---|
| Flicker | 200 ms a step | a burning match (§4) |
| Breath | 2.4 s | a match that asks (§4) |
| Strike | 800 ms | a session starts to work (§4) |
| Out | 400 ms | a session stops (§4) |
| Burn | 600 ms | a session reaches the compact limit (§4) |
| Fade | 140 ms | new values, menus |
| Rise | 180 ms | the confirm bar, toasts |
| Fall | 560 ms | the start screen (§3) |
| Stamp | 240 ms | a confirmed destructive action |
| Heal | 240 ms | a corrected figure shows "was X" |
| Pulse | 1.6 s | a value that is not measured yet |

There are no spinners. `--no-motion` and `NO_MOTION=1` make every motion an
instant change; on the site, `prefers-reduced-motion` does the same.

## 6. Console layout

- **Header:** the mark, the name and the host, then each metric with its
  trend. A narrow terminal drops the trends, then metrics from the right.
- **Tabs:** one row, `1 QUEUE` to `8 PANES`. The active tab has an accent
  rule under it. A tab that needs the person shows it (`4 PROCS !` in `neg`,
  `1 QUEUE 3` in the accent). At 80 columns the names shorten so all eight
  tabs and the alert marker fit.
- **Regions:** a small uppercase label in `faint`, a hairline and space.
  Panels have no boxes.
- **Rows:** the match and the state word first, then the wait, the name, the
  tmux place and the last line. Figures are aligned; units are faint. The
  selected row has a `▌` and the `wash` background.
- **Footer:** the keys of the tab on the left, "sampled N ago" on the right.
  A pending action replaces the keys with `RUN <command>` and its confirm.
- **Widths:** 100 columns or more: all columns. 60 to 99: fewer columns, the
  detail under the list. Less than 60 (phone): one column, the queue first,
  each row on two lines.
- **Keys:** each key is on a phone SSH key bar: digits, arrows, `Enter`,
  `Esc` and letters. No `Ctrl` combinations. `Enter` never runs a
  destructive step; that needs `x` and then a typed `y`.

## 7. Copy

- Plans, specs, docs and on-screen text use ASD-STE100: short sentences,
  one instruction each, active voice.
- A status is a glyph and a word: `✦ busy`, `╿ asks`, `! compact`,
  `▲ 92 °C`.
- An empty state is one sentence (and the action, when there is one):
  "nothing waits for you".
- An error tells the fix: "the service (pid 4242) does not answer: kill
  4242, then start matchblox again".
- Every command the console runs is shown before it runs, as the exact argv.
- Public text names no real machine, person or company. The demo host is
  `ws-1`.

## 8. The site

- **The story:** once, everyone carried a matchbox to light a fire wherever
  they went. matchblox is the one you carry to every terminal: the fire is
  an idea, and keeping it burning is to keep building it. **One fire for
  each view:** only the hero's match burns; other motifs are the box, the
  drawer and the striker.
- **The first screen:** the header; the hero, with the news line ("New ·
  <what is new> · Changelog →", to `CHANGELOG.md`), the line ("Light an
  idea. Keep it burning."), one sentence and the install block on the left
  and the strike canvas on the right; and at the bottom centre the invite
  to scroll: a closed matchbox whose drawer slides out, over "Open the box".
  On a phone the canvas comes first and the invite follows the install.
- **Tags:** a feature that is not released yet has the tag "Soon".
- **Then:** the replay of the real console, which starts when it scrolls
  into view; three short blocks; the motto "Light the match. Keep it
  burning."; the links; the footer (From Blox Engineering · GitHub ·
  Discord · bloxng.com).
- **README:** the same recording, as `www/demo/replay.svg`.
- **Type:** IBM Plex Sans (variable) for text, IBM Plex Mono 400 and 500 for
  commands, labels and the replay. Both are served from the site.
- **Install block:** one framed block with tabs (Script, Go), a `$` prompt,
  the command, and Copy joined to it; under it the platform and "read the
  script first ↗". On a phone the command wraps; it is never cut.
- **Replay:** recorded frames of the real console (`TestReplay`), drawn in
  the site's tokens, so it follows light and dark. A still frame is in the
  HTML for no script and for reduced motion.
- **Rules:** everything loads from the site itself: `script-src 'self'`,
  `connect-src 'self'`, no inline script or style, no third-party resource.
  No horizontal scroll at 390 px. Check light and dark at 390 and 1280 px.
