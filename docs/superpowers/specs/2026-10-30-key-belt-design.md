# The key belt: a key bar you arrange, turning round the phone (#122)

Mockup, the reference for every visual and motion below:
<https://claude.ai/artifact/VezKp1djbb3mpk6Lm1NNBF> (version `1793359559-edd5`).
When this text and the mockup disagree on a look, the mockup wins. When they
disagree on a behaviour, this text wins.

## Intent

The builder steers agents from the iPhone (#261). The nine fixed keys of #262
work, but one row cannot hold everything the builder reaches for: digits to
pick options, Ctrl keys for tmux, and agent commands such as `/compact`. The
owner wants a key bar that is theirs to shape, that feels like a physical part
of the matchbox, and that clicks under the thumb.

The owner's words (2026-10-30): "pick from layouts, or select the keys you want
to show and which ones to hide, when more keys are added than the width
allows, scroll horizontally infinitely … like a rotating bezel going behind
the phone"; "a belt like on an engine rotating, with the two bezels of the
iPhone the two ends"; "the rotations must have a click … using the motor of
the iPhone, so it feels super tactile"; "the drawer on open be like the inside
of a matchbox"; "the previous one become dark as if it burned off"; "this match
design must be exactly the same as the one we use in the island".

## Rulings (2026-10-30, the owner)

- The key bar is arranged by the builder. This reverses the #261 ruling "seven
  keys and no more" and the call "close #122". #122 stays open and this work
  closes it.
- Command buttons are in scope: they send an agent command in one tap. This
  reverses #122's "no text macros" for commands only (a label and the command
  it sends). Free text macros stay out.
- Each agent's commands carry that agent's mark. The Commands layout follows
  the agent of the session on screen. It never shows every agent's buttons at
  once.
- The belt is built as a hand-made UIKit view (approach A below).

## What the builder sees

### The belt

1. The belt is one row of keys across the full width of the screen, from the
   phone's left edge to its right edge. There are no posts, frames or fades at
   the ends: the phone's own two sides are the belt's two pulleys.
2. Between the edges the belt is flat. A key that reaches an edge folds round
   it: it turns on its inner side, from 0° to 90°, as it crosses the edge, and
   it goes on behind the phone. Its face darkens as it turns, up to 75% black at
   90°. The perspective is short (the mockup uses 180 px), so the fold reads as
   a sharp corner, not a curve.
3. The belt never ends. After its last key comes its first key again.
4. Under the keys is the striker: the grit of the matchbox, strips 1.4 pt wide
   every 3 pt at 45°, in `markStriker` on a dark card ground, between a dark top
   line and a light bottom line. It is the same grit that `MatchBox.swift` draws.
5. Every key is one or two cells wide. A cell is sized so that the home row
   (below) fills the width: cell = (screen width − 8 × gap) / 7, at least
   44 pt, with gap 4 pt. On a 390 pt iPhone a cell is about 48 pt. Keys are
   44 pt tall on a 60 pt belt. A key's label is an icon or a key name, never a
   word sentence.
6. Key look: a dark bevel (`#2c261d` to `#1d1913`), a 1 pt `#3a3328` edge,
   radius 6, a light top inner line and a dark drop line. A pressed key is
   `hair` with its label in `accent` for 140 ms.

### Detents, the click, and home

1. The belt moves in detents: it always comes to rest with a key's left side on
   the screen's left edge (plus one gap).
2. Each key side that passes the left edge, while dragged or while coasting,
   gives one haptic tick: `UISelectionFeedbackGenerator.selectionChanged()`,
   the tick of a picker wheel.
   Ruling (2026-10-30): the tick comes when the nearest detent changes, halfway
   between two detents. Reason: the belt rests on a detent, so a tick at the
   edge itself fires on touch jitter at rest.
3. The home stop is the detent with `◂` at the left edge. Every layout has
   `◂ ↑ ↓ ⏎` as its first four keys, with `↑ ↓ ⏎` two cells wide, so at home
   `◂` sits at the left and `↑ ↓ ⏎` fill the right. Reaching home gives a firmer
   click: `UIImpactFeedbackGenerator(style: .medium)`.
4. A drag moves the belt with the finger. On release it coasts, slows
   (the mockup decays velocity by 0.93 per 16 ms frame), and settles on the
   nearest detent with an ease-out.
5. A layout opens on its home stop.

### Tap versus drag

1. A touch that moves less than 8 pt and lifts is a tap: it presses the key
   under it. The key sends at touch-up (not at touch-down as in #262), because
   at touch-down the belt cannot yet tell a tap from a drag.
2. A touch that moves 8 pt or more is a drag. It presses no key, ever.
3. A drag is horizontal only. A vertical move on the belt does nothing.

### The handle row and the matchbox drawer

1. Over the belt is the handle row, 24 pt tall, on the sleeve (`markRight`):
   - left: the layout's name in faint mono caps (`STEER`), with the agent's mark
     before it on the Commands layout;
   - centre: a grip, 36 × 4 pt;
   - right: a ghost keyboard icon, always shown, in the same faint colour as the
     name. A tap on it shows or hides the system keyboard. While the keyboard
     is up the icon is `fg`.
2. A pull up on the handle row, or a tap on its grip or name, opens the drawer;
   a pull down closes it. A tap on the keyboard icon never opens the drawer.
3. The drawer is the matchbox tray, sliding up out of the sleeve. It has the
   tray floor (`markTray`) with a faint card grain, walls (`markLeft`) with an
   inner shadow, and it slides 30 pt as it opens.
4. In the tray lie four matches in four equal columns, one per layout: Steer,
   Commands, tmux, Mine, each with its name under it in faint mono caps.
5. The matches are the island's match: the same geometry and tokens as
   `MatchView` and `MatchGeometry` in `ios/MatchbloxIsland/SessionsFace.swift`.
   The code is shared, not copied (see Approaches).
   - A layout not in use is a ready match: the full stick and the red head
     (`matchHead`), as on the island's closed box.
   - The layout in use burns: the island's `burns` state at a short burnt
     length (front at 0.12).
   - Picking another layout strikes it: a flare (the flame 1.6× its size and a
     brighter glow) for 260 ms, then the steady flame. The match it leaves goes
     out: the island's burnt wood and head, and the island's smoke puffs, which
     rise and fade over 1.4 s. The burnt match stays burnt while the tray is
     open.
   - Closing the tray gives a fresh box: every match is ready again except the
     one in use, which burns.
6. Picking a match runs the belt to the new layout (the old keys run off and
   the new set settles on its home stop) and closes the tray 0.9 s later, so
   the strike shows.
7. On the tray floor: the session's agent with its mark, and a stamped
   `EDIT MINE` label. A tap on it opens the editor.

### The keyboard

1. A tap on the terminal shows the system keyboard, as it does today.
2. The keyboard icon on the handle row shows and hides it.
3. The belt and the handle row stay above the keyboard while it is up, and at
   the bottom of the screen while it is down (as the #262 bar does).

### The layouts

| Layout | Keys, in order |
|---|---|
| Steer | `◂ ↑ ↓ ⏎ Esc ^C ⇞ ⇟` |
| Commands | `◂ ↑ ↓ ⏎`, then the agent's commands (each two cells, with the agent's mark) |
| tmux | `◂ ↑ ↓ ⏎ ⇞ ⇟ Tab ⇧Tab ^B` |
| Mine | the builder's own, from the catalog; `◂ ↑ ↓ ⏎` first by default |

Commands follows the agent of the session the console shows. When the console
shows no session, or the agent is unknown, Commands shows `◂ ↑ ↓ ⏎` only, and
the handle row says `COMMANDS` with no mark.

### The catalog and the editor

1. The editor is a sheet: a grip, the title `Mine`, one line of help, then the
   catalog in groups, each a strict 6-column grid of equal cells; a two-cell key
   spans two columns.
2. Groups: Steer (`◂ ↑ ↓ ⏎ Esc ^C ⇞ ⇟`), Edit (`Tab ⇧Tab ← → ⌫`), Digits
   (`1`–`9`), Ctrl (`^A ^B ^D ^L ^R`), and one group per agent with its
   commands and marks.
3. A tap adds a key to the end of Mine or takes it off. `◂` always stays: a tap
   on it says why. The builder reorders Mine by dragging a key in the grid (my
   call: #122 asks for reorder, and the mockup did not show it).
4. Mine is one set per device, shared by all hosts, kept in the app (#122).

### What each key does

| Key | Sends |
|---|---|
| `◂` | `HomeKey.bytes`: back to the console, or back to Sessions in the chat |
| `↑ ↓ ← →` | the arrow sequences |
| `⏎` `Tab` `⇧Tab` `Esc` `⌫` | CR, HT, `ESC [ Z`, ESC, DEL |
| `^C` `^A` `^B` `^D` `^L` `^R` | the control byte |
| `1`–`9` | the digit |
| `⇞` `⇟` | a page up or down of the history: the console's history scroll (#263) in the chat; tmux copy-mode page in a jumped pane |
| an agent command | the console runs it: see below |

### Commands go through the console

The app is a terminal: it does not know which agent the console shows, and it
must never type a command blind. So the console and the app talk in the same
private signal the home key uses (`ESC _ matchblox … ESC \`):

- **Console → app.** When the session on screen changes, the console sends
  `ESC _ matchblox;agent=<id> ESC \`, where `<id>` is `claude`, `codex`,
  `opencode`, `cursor` or empty. The app reads it from the terminal stream,
  switches Commands, and does not draw it.
- **App → console.** A command key sends `ESC _ matchblox;cmd=<name> ESC \`.
  The console runs it on the session on screen, behind the chat's guards
  (#261: right agent, seen screen, console only):
  - `compact` and `clear` run matchblox's own guarded actions (`compact:<pane>`
    and its clear), which wait until the agent rests;
  - any other command types its text and Enter through the `keys:` act;
  - a command the session's agent does not have sends nothing, and the key line
    says `/<name> is <Agent>'s: <session> runs <other agent>`.
- The per-agent command list lives in the console (the host), one table, with
  the mark id for each. The app shows what the console names for the agent.
  Command names for Codex, OpenCode and Cursor are checked against each
  agent's current documentation before they go in the table.

### Agent marks

The marks are the PNGs of `internal/logos/png/dark` (`claude-color`, `openai`,
`opencode`, `cursor`), bundled into the app. On a key the mark is 10 pt at the
top-right corner; on the handle row 10 pt; on the tray floor 11 pt.

## Goals

- Every look and motion in the mockup, on a device, with real haptics.
- #122's asks: add, remove and reorder; a home key that moves but never goes;
  one set per device; sensible defaults.
- One match drawing for the island and the tray.

## Non-goals

- Free text macros (a key that types any text). Commands only.
- A key's label in words longer than a command name.
- Layouts per host. One set per device.
- A belt on the iPad's hardware keyboard bar.
- Gestures on the belt other than a horizontal drag and a tap (no long-press
  menus, no vertical swipes).

## Anti-goals (failure even if it works)

- A drag presses a key. A tap starts a turn.
- The belt rests between two keys.
- A command reaches an agent that is not the one on screen, or an agent that
  lacks it.
- The tray's match looks different from the island's match.
- The belt drops frames while dragging on an iPhone 13 or newer (60 fps
  minimum, 120 on ProMotion).
- A key is under 44 pt tall, or a one-cell key under 44 pt wide.

## Approaches

**A. A hand-built UIKit view (picked).** The belt is a `UIView` whose keys are
layers under a parent with a perspective `sublayerTransform`. A pan recognizer
and a `CADisplayLink` drive one offset. The fold, the detent set, the coast and
the settle are pure functions in MatchbloxKit (like `WheelScroll`), tested on
CI. Haptics are UIKit's feedback generators. It replaces `KeyBarView` in the
accessory slot. Picked because the feel lives in exact control of the
tap-versus-drag line and of the tick timing.

**B. SwiftUI in the accessory slot.** Fast to draw (`rotation3DEffect`,
`DragGesture`), but hosting SwiftUI in `inputAccessoryView` beside SwiftTerm
has sizing and gesture quirks, and per-detent haptics are harder to time.

**C. UICollectionView with a custom layout.** Free scrolling and snapping, but
"never ends" needs a huge item count, and the fold on top of its scrolling is
harder to control.

**Sharing the match.** `MatchView`, `MatchGeometry`, `FlameView` and
`SmokeView` move from `SessionsFace.swift` into one file that both the app and
the island extension compile, as `project.yml` already does for
`IslandAttributes.swift`. MatchbloxKit has no SwiftUI today; this keeps it so.

## Constraints

- iOS 18+, SwiftTerm 1.20.0, xcodegen (`ios/project.yml`). No Swift toolchain
  on the Linux host: CI (`.github/workflows/ios.yml`) compiles and runs the
  MatchbloxKit tests; the device proves the feel.
- Tokens only from `Tokens.swift` and DESIGN.md. The mockup's literal colours
  that are not tokens (the key bevel, the card ground) become named constants
  in one place, listed in the commit.
- The home key and the signals share one parser on each side: the console
  already reads `HomeKey`; the app adds a reader for `matchblox;agent=`.
- The console side is Go in `internal/app` (chat, keys) and the command table;
  it lands with its own tests and the docs (`docs/phone.md`, `docs/keys.md`).

## Left to the builder

- Spring and coast constants beyond the mockup's, if a device shows they feel
  wrong.
- How the app intercepts the console's APC in SwiftTerm (a parser hook or a
  stream filter before the terminal).
- Where the clear action lives in the console today, and its name in the
  command table.
- The editor's drag-to-reorder interaction details.

## Calls made after the builder check (2026-10-30, mine; the owner checks them)

1. **How a command reaches the console.** tmux passes only the private
   sequences it knows as user keys, and the home key already holds slot 900.
   Each command in the command table gets its own user key, slots 901 and up,
   registered at console start as `HomeKeyArgv` does. The command table is
   finite (no free text), so a fixed set of slots covers it. A command key
   sends its own sequence, `ESC _ matchblox;cmd=<id> ESC \`.
2. **One command table, two readers.** The table (id, label, agent, mark,
   what it runs) is a fixture in `testdata/keys/commands.json`. The Go console
   and the app's MatchbloxKit both read it in their tests, as both read
   `testdata/home` for the home key, so the two cannot drift. The app ships its
   copy; the console refuses an id it does not know.
3. **The session a command acts on is the chat's session only.** A command
   runs only while the chat shows a session, behind the #261 guards. On the
   Sessions list, and in a jumped tmux pane, the command keys are dimmed, and a
   tap says `commands run from the chat`. The console sends `agent=` when the
   chat opens on a session or its session changes, and `agent=` (empty) when it
   closes.
4. **Console → app through tmux.** The console wraps the signal for tmux
   passthrough (`ESC P tmux; … ESC \`), as `cmd/matchblox/graphics.go` already
   relies on `allow-passthrough`. While the phone is in a jumped pane the
   command keys are dimmed anyway (call 3), so a signal that arrives there does
   no harm. Where passthrough is off (older tmux), Commands shows `◂ ↑ ↓ ⏎`
   and the handle row says `COMMANDS · needs tmux 3.3`.
5. **`⇞` and `⇟` send PageUp and PageDown** (`ESC [ 5 ~`, `ESC [ 6 ~`)
   everywhere. In the chat the console takes them as a page of the history
   scroll (#263). In a jumped pane matchblox's jump key table binds them to
   tmux copy-mode page up and down. The app never needs to know where it is.
6. **The app knows it is in a jumped pane** from the `◂ matchblox` label in the
   status line, as the swipe-scroll speed already does (`HomeKey.labelColumns`).
