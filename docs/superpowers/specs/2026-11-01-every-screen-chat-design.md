# The chat on every screen

Part of #261 (PR #262). Owner, 2026-11-01: "fix all of them … properly show
the most important things with regards to the user on each screen … think
like Steve Jobs."

## Intent

The person opens the console to answer the agent that waits for them. On
every screen, that agent's question is the one thing that must be whole and
readable. Everything else — the list, usage, the machine — gives way to it,
in that order. A screen never shows a chat whose question it cannot show,
and never takes keys for a screen the person cannot see.

## What the device survey found

The survey drew #262 at each device's grid (artifact "#262 on every
screen"). The app draws SF Mono 13 pt, about 7.8 × 16 pt a cell.

| Device | Grid | Today | Problem |
|---|---|---|---|
| iPhone 17 Pro on its side, keyboard up | 96 × 8 | Medium | The chat shows no screen and no card; keys still go to the agent. |
| iPhone 17 Pro Max on its side, keyboard up | 106 × 10 | Wide | The card shows its last two lines; the question is cut. |
| A folded phone of 60–89 columns, keyboard up | 60 × 21 | Medium | Flips from one column to a split when the keyboard rises (0006 §4 says it keeps its layout). |
| iPad mini 7 upright | 95 × 65 | Medium | The chat gets 47 columns; the list half has ~20 empty rows; the dock takes the bottom. |
| A Mac terminal tiled to half the screen | 96 × 57, 110 × 65 | Medium | The same: a half-width chat on a tall desk. |
| Any phone, the chat | 44–51 cols | Narrow | The key line is cut ("ctrl+o into…"). |
| Any phone, the chat scrolled up | 44 × 31 | Narrow | The question shows twice: the agent's own menu above the card. |
| A 132 × 32 desk | 132 × 32 | Wide | The left column keeps Machine and cuts the list (recommendations too); the PR says the reverse. |

## Rules

1. **Shape is sticky.** A screen's shape (upright or not) is decided at the
   tallest height seen at its current width. A keyboard that shrinks the
   height never changes the layout; a new width decides afresh.
   (0006 §4 already requires it; the code did not keep it.)
2. **No chat without its question.** Sessions splits into the list and the
   chat only when the chat is at least 44 columns wide and the body has
   room for the chat's title, its account line and the whole question
   card. Short of either, Sessions is one column, and the chat opens
   full-screen, as on a phone. This covers a phone on its side with the
   keyboard up and a landscape screen of 60–82 columns (83 leaves a
   44-column chat). The account line counts as drawn: two lines on a chat
   of 44–59 columns. The card counted is the tallest of the sessions the
   cursor can reach, so the split holds as the cursor moves; at a
   permission prompt, the prompt's whole box counts as the card. Where
   Sessions is one column the selected row opens in place.
3. **One split for Sessions.** Where Sessions splits, the list takes 40% of
   the width (at least 38 columns, at most 78) and the chat takes the
   rest. Usage, then Machine, then the Stoker stack under the list, on
   every split screen, upright or not. The bottom dock is gone from
   Sessions. So the iPad mini upright gets a 56-column chat, full height.
   The other tabs keep their splits (0006 §4).
4. **The list outranks the machine.** Under the list, Machine folds before
   the list is cut. Usage is never cut. The list (with the recommendations)
   is cut only when Usage alone does not leave it room.
5. **The chat's key line keeps the way back.** Its hints go by worth:
   `ctrl+] back`, then `ctrl+o into tmux`, then `keys go to <name>`. A hint
   that does not fit is dropped whole, never cut mid-word. The title
   already names the agent.
6. **One question, once.** While the chat shows the agent's history and a
   card is up, the agent's own menu is cut from the lines above the card,
   as it is on the live screen.
7. **Another host's row is list only.** Since #264, Sessions lists other
   hosts' sessions. The chat watches and sends keys only for a session of
   the console's own host, by pane id. A remote row's right side names its
   host and how to reach it; it never shows the home host's pane of the
   same id, and `keys:`/`watch:` are never sent for it.
8. **List only never leaves Usage behind blank rows.** Since #264, `v`
   (list only) folds the detail pane and the dock, so the list takes the
   screen. On a 107 × 60 phone with a short list that left ~23 blank rows
   and no Usage. In list only the list still comes first and is never cut
   for the stats, but the rows it leaves free take Usage, then Machine,
   under a hairline, each at its fewest lines and only if it fits whole
   (Usage never cut). The key line's hints stay as #264 has them.

## Non-goals

- No font-size or Dynamic Type change (#141).
- No change to the iOS key bar (that is #268).
- The other tabs' layouts stay as they are.

## Done when

- Goldens at the survey's grids: 51×45, 51×31, 96×20, 96×8, 106×10,
  95×65, 145×40, 193×57, 96×57, 104×38 (the open Duo) and 44×31 (folded),
  for Sessions and for the chat.
- A test: a 60-column screen at 34 rows, then 21 rows, keeps one column.
- A test: on every golden size, the chat either shows the whole card or
  is not drawn.
- A test: a remote row sends no `watch:` and no `keys:`.
- A test: a 107×60 list-only Sessions with a short list shows USAGE; one
  whose list fills the screen shows none; a golden of each.
- docs/console.md and docs/keys.md say the rules above.

## Calls made (owner's "fix all"; each is reversible)

- 44 columns is the chat's least width: the question card reads at 44 on
  the folded phone golden.
- Sessions loses its bottom dock on split screens. Usage moves under the
  list, as on Wide today.
- The iPad mini upright and a half-screen Mac now look like a small desk,
  not an opened phone. This narrows the owner's 2026-10-12 ruling (split
  from 90 columns upright) for Sessions only: it still splits, at 40/60.
