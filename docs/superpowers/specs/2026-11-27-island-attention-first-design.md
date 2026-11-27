# The island puts what needs you first

Owner ask (2026-11-27): "my active or idle sessions that want my attention on
the front, and if all sessions are out of fire only the match we have now; the
right part can be the limits". Owner ruling on the open choices: "whatever
Steve Jobs would pick".

## Why

The island answers one question at a glance: does anything need me? Today it
leads with one session and can show a working session while another waits.
The person then has to open Pocket to find out.

## Goals

- A session that needs you always leads the island, over any work.
- With several that need you, the island says how many.
- With nothing that needs you, the island is the match it is now.
- The right of the island is one number: the limit that matters.
- The lock screen lists what needs you, then the limits.

## Non-goals

- One Live Activity per session. Delivery apps do it per order, but the
  Dynamic Island shows at most two activities, the cards come and go, and
  each card costs its own pushes. One island per host stays.
- Rotating limits. A Live Activity draws again only on a push. A rotation
  each few seconds would spend the push budget on noise.
- A new setting. The face adapts. The Show choice (auto, sessions, usage)
  stays as it is; its place on Pocket home is #508's.

## Anti-goals

- A working session hides a question.
- The island changes when nothing changed for the person.

## Design

**Needs you** is a session with a queue row that waits for the person: it
asks (a permission or a question) or it finished its turn. Order: asks before
finished; within each, the oldest first. This is the existing `waits` and
`done` order of `push.Build`; out and burns come after, as now.

**Compact island.**

| State | Left | Right |
|---|---|---|
| 1 needs you | the match, `╿` lit | the session's name |
| 2+ need you | the match, `╿` lit | `╿ N` |
| nothing needs you, work runs | the match of the busiest session | the tightest working limit, `wk 30%` |
| all quiet | the host mark `▰` | the tightest working limit, or nothing |

The tightest working limit is today's `tightest`: idle accounts and spent
windows left out (#513).

**Lock screen.** Up to 3 rows of sessions that need you, each the name and
what it wants: `asks to run a command`, `asks you a question`,
`finished its turn`. Past 3: `+ N more`. Then one line of limits, the fullest
window of each working account. With nothing that needs you: the match and
the limits, as now.

**Tap.** Opens the top session that needs you; else the console, as now.

## Owner rulings, 2026-11-27 (after the #513 phone check)

**The match burns while any session lives.** The flame is lit while a
session works, waits for you, has finished its turn, or sits idle. It goes
out only when no session runs on the host (quiet), or a limit stops the top
session (out). A wait shows smoke over the lit flame. Reason: the owner
reads a lit match as "this host is alive"; a cold match while sessions are
open says the wrong thing.

- The host sends a new top state `idle`: a session with no queue row that
  does not work. It ranks after `burns`; of the idle ones, the one prompted
  last leads. An old phone reads `idle` as quiet, as today.
- Burn counts stay the working sessions only.

**The buzz hisses: one sound, `pinch-ember`.** A short pinch hiss and one
quiet ember pop, 0.85 s, for a wait and for a finished turn alike. The
owner heard both sounds of design 0009 and picked this one ("pinch ember",
2026-11-27); `dying-ember` is dropped. The phone buzzes on a finished turn
too (0009 §3.3), which the shipped buzz left out.

- The push names the sound (`aps.sound`). The island already shows the
  state to Apple, so the name says nothing new. A phone without the file
  plays the default sound.

## Constraints

- The payload grows by the sessions that need you, at most 3, each its
  number and its kind. A missing list means today's single top session, so
  an old host and a new phone, or the reverse, work as before.
- The push rates of design 0010 §3.6 stay: a new wait goes at once.

## Approaches

1. **One island, attention first (picked).** One activity, a count, one number
   on the right. Fewest pushes; one place to look.
2. One activity per session that needs you. Clear cards on the lock screen,
   but the Dynamic Island shows two, and the pushes multiply.
3. Rotating limits on the right. Rejected: a push per rotation.

## Left to the builder

The exact field names, the widget layout within these rules, and the copy of
`+ N more`.
