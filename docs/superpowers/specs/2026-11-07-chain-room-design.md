# Chain room

Issue: #326 (a follow-up to #304). Status: approved by the owner, 2026-11-07. Date: 2026-11-07.
Mockup: the "Chain Room Mockups" page (owner's artifact, 2026-11-07), option A.
Builds on: #304 (a stoker per chain, PR #312) and #299 (chains). Work starts after #312 merges.

## Why

The owner runs chains of matches, and each chain has a stoker. Today, to work
with one chain, the owner hops between tabs. Sessions shows the matches, the
STOKER tab shows the stoker's cards, and Ships and Git show the whole machine.
To start a chain's stoker, the owner fills a sheet. To go from a session to
its chain's stoker and back, there is no key.

The owner said: starting a stoker per chain must be seamless, "a la Steve
Jobs, without clutter". Switching between a session and its stoker must be
quick. "Maybe chains can get their own view." "I also don't see myself using
the Ships and other views, maybe those can become chain specific, and once on a
chain we can interact."

Success: the owner opens the console, sees the chains, opens one, and does
everything for that chain in one place. From inside any session, one key goes
to that chain's stoker, and the same key comes back.

## Goals

1. The console opens on a list of chains. Each chain opens into its room: the
   stoker, its plan, its matches, its ships and its worktrees, on one screen.
2. One key flips between a session and its chain's stoker, both ways, from
   anywhere in tmux. A second key walks the chain's matches.
3. A chain with no stoker gets one with no sheet and no form. The first line
   the owner types to it becomes its order.
4. Ships and Git show only the chain in the room. Their global tabs go.
5. The tab row is `chains · machine`. Machine holds usage, CPU, history and
   panes for the rare look.
6. The phone (the iOS app runs this console) gets the same room at 40 columns.
   The Live Activity and the buzz name the chain that needs the owner.
7. The docs and the landing page show the room and the flip.

## Non-goals

- New Sessions groupings. The need, status and repo groupings go away, and
  nothing replaces them: the chain is the only grouping.
- A chat history view, or a log of what the stoker said.
- Changes to how a stoker decides, drives or guards (`DenyFamily`, plans,
  gos). The service changes only where a screen needs new data (the push
  payload, the per-client "came from" session).
- Chains across hosts. A chain lives on one host (#299). #309 (chains across
  repos on one machine) works inside this design and is not part of it.

## Anti-goals

This fails, even if it runs, when:

- A session that waits for the owner is hidden: not in the room, or below the
  fold of the chain list.
- A chain exists that the chain list does not show.
- The flip key reaches an agent as typing, or steals a key an agent CLI uses.
- The flip loses where the owner came from: the way back lands on another
  session.
- Starting a stoker shows a form, a sheet or a question before the first line.
- The room shows another chain's ships or worktrees.

## Owner's decisions

- Redesign the stoker-per-chain experience across every surface: desk, phone,
  iOS, landing page and the Sessions groupings. (2026-11-07)
- Starting a stoker per chain is seamless, with no clutter. The session ⇄
  stoker switch is quick. Chains get their own view. Ships and the other views
  become chain specific. (2026-11-07)
- "Choose whatever Steve Jobs would pick." (2026-11-07) The design takes
  option A of the mockup, with no groupings, on this delegation (see
  Approaches).
- "Figure things out … reach completeness 10/10, Apple grade design, native
  iOS components as needed: a vibration when a match changes state, a
  vibration like in Termius when scrolling up and down." (2026-11-07) The
  design review's decisions from issue 4 on are made on this delegation and
  marked "review call". Issues 1A, 2A and 3A the owner picked.

## Approaches

The mockup showed three. The owner delegated the pick.

- **A. Chains are the home (picked).** The console opens on chains. A chain
  opens into its room. The tabs fold to `chains · machine`. It is the only
  option that does all three things the owner asked: an own view per chain,
  Ships made chain specific, and interaction once on a chain. Cost: the largest
  change. Five tabs fold into two, the docs and goldens turn over, and the
  groupings go.
- **B. Zoom into a chain.** Sessions stays home. ⏎ on a chain zooms the list to
  that chain. Small change, but two homes in one tab, and Ships and Git stay as
  global tabs the owner does not use.
- **C. The stoker is the chain.** Opening a chain attaches to the stoker's chat
  with a rail of matches. Purest, but the chain's state lives only in the
  stoker's words. A stuck stoker hides its chain, and gos are not on a card the
  owner can scan.

Why A and no groupings: one idea, the chain, carries the whole console.
Groupings are a second way to see the same sessions, so they are clutter by
the owner's rule. B keeps both worlds; C hides the state. A removes the most.

## The design

### The chain list (home)

- One row per chain, peer hosts' chains included (no chain stays hidden). A
  session in no chain is a chain of one, named for its
  session. Order: the chain that needs the owner most comes first (a go or an
  ask, then a wait, then work, then idle), so the old "need" grouping becomes
  the list's only order.
- A row shows `≋` when the chain has a stoker (a new glyph: heat over the
  fire; `✦` stays the busy match, DESIGN.md rule 2; review call), the chain's
  name, its count of matches, the one thing that needs the owner (or the
  chain's state), and the stoker's plan progress (`plan 1/3`). `≋` takes
  `muted`, and the `night` token while that chain's stoker is in charge.
- The order holds still while the owner looks (rule 1, review call): a chain
  moves only when its need tier changes (asks or go → waits → works → idle),
  never within a tier, and the cursor stays on its chain when rows move. No
  row animates.
- Coming back to the console (home key, `‹`) selects the chain the owner was
  last in (review call).
- The fuse and chain head of #299/#312 stay inside the room, not in the list.
- The list keeps everything Sessions holds besides sessions, in today's order
  (review 2026-11-07, issue 2A): the doors on top (`SET UP`, Pair a device,
  the night door; they fold as today), then the chains, then the ghost row
  `+ start a new session here` with its folder and branch, then
  `RECOMMENDATIONS · N · h hides`. The ghost row and the doors keep their
  §6 keys and rules; the chain count leaves the ghost row out.

### The room

**Layout** (issue 1A, reopened as design review D7-A, owner 2026-11-08,
to fit #334 / PR #346, where ⏎ goes into the agent's own pane): on Wide and
Medium the room takes the left 40%, and the selected match's live view
(read-only; the wheel scrolls it back, as Sessions after #346) takes the
rest. ⏎, `a` or a tap on a match goes into that match's own tmux pane,
moving only the screen that asked. From there ⌥s or prefix s comes back to
the room. On Narrow the room fills the screen and ⏎ goes into the pane. With
the cursor on the stoker's card, the right side shows the stoker's recent
steps. The room has no chat that takes keys.

**Building on #346:** this design needs #346 merged first. The plan rebases
onto it.

The room's regions, from top to bottom (see the mockup's option A):

0. **The need line** (review 2026-11-07, issue 3A). When a go or an ask
   waits, one line leads the room, above the card: `╿ ember-billing asks:
   may I force-push the rebase?` or `▸ merge #4552 waits for your go · y`.
   The oldest need wins; a second shows as `+1`. With no need, no line.
1. **The stoker's card.** It shows the active step and one step on each
   side, then `↓ N more` (⏎ on it unfolds the plan), so the matches always
   fit above the fold. Its order in quotes, then its plan steps with ✓, ▸
   and plain marks. A step that waits for a go says so, with `y gives it`.
   `y` gives the go to the selected step, as today's STOKER tab does.
2. **The matches.** One line each: the fuse column (#299), the match, the
   agent's mark, the name, the repo when the team spans repos (#309), its
   state or its ask, and `✉ N` when its inbox holds N notices (matchblox-chain,
   2026-11-07). A mate the sampler cannot see stays in `faint` with `not seen ·
   Nm`; the team never hides. A match in another team too adds `· also in
   <team>`. ⏎ or `a` on a match opens its live chat (see Layout).
3. **SHIPS** (an inline label, D11-A). The chain's PRs only, one line: live, checks, waits for merge.
4. **GIT** (an inline label). The chain's worktrees only, one line: count, merged, safe to
   remove.
5. **The talk line.** `› tell <chain>'s stoker…`. ⏎ sends the line to the
   chain's stoker. Then the card quotes those newest words at once, in
   `muted`, until the stoker's next step takes them, then in `fg` (design
   review D6-A, owner 2026-11-08). The room keeps no history.

**The cursor and the keys** (review call). The cursor moves over the room's
rows: the need line, the card's steps, the matches, ships, git and the talk
line. ⏎ acts on the selected row: on the need line or a match it opens that
match's chat; on `↓ N more` it unfolds the plan; on ships it opens the ship's
detail (DESIGN.md §6 Ships, with `x` to merge through the consent box); on git
it lists the worktrees (`space`, `X`, `x` as on today's Git tab); on the talk
line it starts typing. `y` gives a go on the need line or on a step that
waits. `]` and `[` go to the next and the previous chain (the phone: a swipe
left or right in the room). esc goes back one level: typing → the room (the
draft stays) → the chain list. Every key of the selected row shows in the key
line; at 40 columns the key line shows the three keys of that row and `?
keys`.

**Typing.** While the talk line types, every key is text until ⏎ sends it or
esc stops; the key line says `typing to hearth's stoker · ⏎ sends · esc
stops`, and `›` takes the accent. After ⏎: the toast `sent to hearth's
stoker`, and the line clears.

**A go that acts outside the session** (review call): a go whose step
merges, force-pushes, kills or removes runs through the consent box, with the
exact argv and a typed `y` (DESIGN.md §6). Other gos give on `y` at once,
with the toast `go given: rebase ember-billing`.

**One accent** (rule 4, review call): the accent marks the need line (or,
without one, the selected row and the active tab). An ask on a match row, a
ship that waits for merge and the talk line's `›` use `warn` or `muted` while
the need line holds the accent.

**A chain of one** (review call): its card is titled `≋ the stoker · every
session in no chain`, and its talk line says `› tell the stoker…`. The words
make plain that this stoker is shared.

### The flip

- ⌥s (a tmux key matchblox binds, like the home key) goes from any session of
  a chain to that chain's stoker. On the stoker, ⌥s goes back to the session
  the owner came from. matchblox stores that session per client.
- ⌥n walks the chain's matches in order, then the stoker, then the first match
  again: the stoker and its matches form one loop.
- From a chain of one, ⌥s goes to the host's stoker (my call: #304's host unit
  stays the stoker of sessions in no chain, so the chain-of-one rows share
  it). That room's card shows only the host plan's steps for that session.
- The flip works inside one tmux server. A peer host's session flips on that
  host's own console. In this console, ⌥s on a peer's chain opens its room
  (my call).
- There is no ⌥b (review call): the home key and prefix `m` already go to
  the console. The phone has no Option key: its key bar gets a flip and a next
  button that send user-key sequences, as the ◂ button sends the home key
  (my call).
- In the console: ⌥s on a chain's row opens its room. In a room, ⌥s goes
  back to the session the owner came from; with none stored, it starts typing
  on the talk line (review call: never a dead end). No chat in the room takes
  keys (D7), so ⌥s has this one meaning there (D4, owner 2026-11-08).
- **The way to find it** (design review D5-A, owner 2026-11-08): on a pane in
  a team, tmux's status line shows `≋ hearth` after `◂ matchblox`. A click or
  a tap on it flips, as the home target goes home.
- Where ⌥s lands (review call): on the need line when one waits, else on the
  talk line, typing.
- Key check, first task of the build (review call): read the key maps of
  Claude Code, Codex and OpenCode, and readline's defaults, for M-s and M-n.
  A key any of them uses is replaced by the nearest free Option key, on desk
  and phone alike, and the spec and DESIGN.md say which.

### The keys on a Mac, and the way back

- **Fallback keys** (owner, 2026-11-08, design review D3-A): prefix `s` flips
  and prefix `n` walks the matches, everywhere, the way prefix `m` goes home.
  ⌥s and ⌥n work where the terminal sends Option as Meta (Ghostty, kitty,
  iTerm2 set so). Terminal.app and iTerm2's default profile type `ß` and a
  dead `˜` instead; `docs/keys.md` says how to set Option as Meta. tmux's
  own prefix `s` (choose-tree) moves to prefix `S`.
- **The back ladder** (owner, 2026-11-08): ◂, ⌘M, ctrl+M (the home APC) and
  prefix `m` go back one step per press:
  1. an agent's pane → the console, on the room of that pane's team (else
     the chain list);
  2. a chat that has the keys → the keys go back to the room;
  3. a room → the chain list;
  4. the chain list of a host you jumped to → back to the console you came
     from, on its host list; on the phone, the app's Hosts screen.
  The console tells tmux and the app it is at the top through its terminal
  title, `matchblox · top` (tmux reads it as `#{pane_title}`; the app's
  terminal view gets it as the title). Only at the top does the next press
  leave the host. Today a press leaves at once.

### Waking a stoker

- The stoker's chat is the room's talk line, in the console. ⌥s lands on the
  chain's room with the talk line focused, not on the agent's tmux pane (my
  call: matchblox cannot read what the owner types into an agent's pane).
- On a chain with no stoker, the room shows an empty card and the line
  `› what should hearth's stoker do?`. The first ⏎ starts the stoker with that
  line as its order, through today's chain order path (`giveChainOrder`
  already refuses an empty order, so nothing starts before words exist).
- The new stoker takes the defaults today's sheet would give it. The sheet
  stays reachable from the stoker's card for the rare change, but no flow
  requires it.
- A chain of one with the host stoker off works the same way: its first line
  starts the host stoker with that order and the sheet's defaults (my call).

### The tabs

- The tab row is `chains · machine`. Keys 1 and 2 pick them.
- Machine keeps DESIGN.md §6's order and adds what the folded tabs leave
  (review call): usage, the machine, the detached busy loops (with `!` in
  `neg` on the tab while one burns), the top processes, then the host
  stoker's controls (`f` launch or stop, `n` night runs, the sealed night,
  the family dock), DEVICES, history and panes. Each is a region with its
  label (§6 Regions). Peer hosts show as they do today (my call).
- The header (review call): `● N wait for you` stays. The stoker word becomes
  `≋ N` while N stokers run (`stoker off` in `faint` when none,
  `stoker rests` in `muted`). The night stays a host matter: the whole
  console wears it only while the host stoker's run is in charge; a chain's
  stoker in charge paints only its `≋` in the `night` token.
- The Sessions, Ships, Git, History, Panes and STOKER tabs go. Their content
  lives in the room (per chain) or in Machine (per machine).
- The need, status and repo groupings go, with their keys and config.

### The phone

- At 40 columns the room fills the screen. The chain list is one step back
  (‹ or esc). These frames are the target (review call):

```
 ▰ ws-1                  ● 1  ≋ 2
 ‹ chains · ≋ hearth
 ──────────────────────────────────
 ╿ ember-billing asks: force-push?
 ≋ "merge on green, rebase behind"
   ✓ merge #4551 when green
   ▸ rebase ember-billing  y gives
   ↓ 1 more
 ● hearth-api      done
 ╿ ember-billing   asks
 ✦ kindle-web      busy · 41%
 SHIPS #4551 live · #4552 waits
 GIT   2 worktrees · 1 safe
 › tell hearth's stoker…
 ⏎ go in  y give  ] next  ? keys
```

(Design review 2026-11-08: the §6 header stays in the room (D12-A); a
one-line strip takes its label inline, uppercase in `faint` (D11-A); the key
line has no duplicate (D10-A).)

```
 ▰ matchblox            ● 1  ≋ 2
 CHAINS  machine
 ──────────────────────────────────
▌≋ hearth       3  asks    plan 1/3
 ≋ kindle       2  busy    plan 2/4
   ember-tools  1  idle
 + start a new session here
 ⏎ room  ] next  ? keys
```

- The key bar (design review D9-A, owner 2026-11-08): two catalog keys,
  `⇄` flip and `⤳` next, sized as every belt key. They are in the steer and
  commands layouts and in the editor. A saved custom bar gets `⇄` appended
  once, with the one-time note `⇄ flips to the stoker`.
- Touch targets (review call): every tappable row is a full terminal row on
  a cell of at least 44 pt on the phone's font size; the key bar's flip and
  next buttons are 44 × 44 pt with VoiceOver labels `Flip to the stoker`
  and `Next match`.
- The Live Activity (MatchbloxIsland) shows the chain that needs the owner
  most (review call; DESIGN.md §9 changes in step 1). The Sessions face
  becomes the Chains face:
  - lock screen: `▰ ws-1  ≋ 2 ╿ 1` over `hearth · ember-billing asks`;
  - compact: the match of that chain's neediest session and `hearth`;
  - minimal: the front of the match, `╿` over it while anything waits;
  - expanded: the need, `plan 1/3`, and `Answer` (or `Open`).
  A chain of one shows its session as today. Unknown names say `chain 3`.
  `Show` offers `Chains`, `Usage` and `Auto`; a wait still breaks through
  for 60 s on the Usage face.
- The buzz (MatchbloxBuzz) names the chain and the need: `hearth: merge
  #4552? needs your go`. Tapping it opens that chain's room.
- **Haptics** (owner, 2026-11-07; details review call). Native
  `UIFeedbackGenerator`s, one of each kind kept prepared, as `Haptics.swift`
  does for pairing:
  - **A match changes state**, in the foreground, on the host the app shows:
    a match that starts to ask or wait for a go plays `.warning`
    (UINotificationFeedbackGenerator); a match that finishes plays a soft
    impact (`.soft`, intensity 0.6). Busy ↔ idle churn plays nothing (rule
    1: the system's work is silent). At most one haptic each 2 s; a burst
    plays once. In the background the buzz's own notification plays.
  - **Scrolling**, like a ratchet: in the terminal view, each row the wheel
    pan or the scrollback moves plays a selection tick
    (UISelectionFeedbackGenerator), at most 30 a second; reaching the top or
    the bottom of the scrollback plays one `.rigid` impact. The cursor moving
    over rows on a swipe ticks the same way.
  - **Acts:** the flip button plays a `.light` impact; a go given plays
    `.success`; a refusal plays `.error`.
  - A setting `Haptics` (on by default) in the app's settings turns them all
    off; the system's own haptics switch also wins. They play with Reduce
    Motion on, as pairing's do.
- The push payload grows by new fields only. An old app ignores them and opens
  the console as today. A new app on an old host shows the host stoker as
  today. No versioned break, so iOS ships in its own release step (my call).

### Docs and landing page

- `docs/console.md`, `docs/stoker.md`, `docs/keys.md`, `docs/phone.md` and
  `docs/ships.md` describe the room, the flip and the two tabs. `docs/ships.md`
  folds into the room's page or goes.
- The landing page shows the flip: a session, ⌥s, the stoker, ⌥s back.
- The CHANGELOG names the change under [Unreleased] as a change of the
  console, with the removed tabs and groupings listed.

### States

What the owner sees in each state (review call). Copy follows DESIGN.md §7:
one sentence, and the action.

| Element | Loading | Empty | Error | Partial |
|---|---|---|---|---|
| Chain list | before the first state: names only, no zero figures (§6) | no session: the ghost row leads with the cursor, with the two quick starts (§6) | the service does not answer: the §7 error with its fix, in the body | a peer out of reach: its chains stay, dimmed, with `ws-1 is not reachable · <why>` as the region's label |
| Room: card | `≋ waking…` in `muted` after the first ⏎ | no stoker: `› what should hearth's stoker do?`, typing | the stoker failed to start: `the stoker did not start: <why>` in `neg`; the typed order stays in the line for ⏎ again | a stoker that does not answer for 60 s: `≋ hearth's stoker is quiet since 14:02 · ⏎ asks it` in `warn` |
| Room: matches | — | the last match ended: `the chain has no matches · + start one here` and the chain leaves the list when its stoker stops | a match whose pane is gone: `That session ended.` (§6) | — |
| ships, git | `reading…` in `faint` | the line says `no ships yet` / `no worktrees` in `faint`, one line | GitHub limited or unreadable: the §6 strip text on the ships line, in `warn` | — |
| Talk line | — | the placeholder `› tell hearth's stoker…` | not sent: `not sent: <why>`, the draft stays | — |
| Live Activity | the app's last state | `nothing waits for you` | `island: open matchblox on the phone to renew` (§9) | — |

### The journey

| Step | The owner does | The owner feels | The design gives |
|---|---|---|---|
| 1 | Opens the console | Calm: what needs me? | The chain list, the neediest first, one accent |
| 2 | ⏎ on hearth | Oriented | The need line first, the card, the matches |
| 3 | `y` on the go | In control | The consent box for a merge; the toast; `.success` on the phone |
| 4 | ⏎ on ember-billing | Close to the work | Its live chat beside the room |
| 5 | ⌥s from inside a session | Quick | The room's talk line, typing; ⌥s again goes back |
| 6 | Starts a new chain's stoker | Effortless | No sheet: the first line is the order |
| 7 | Phone in the pocket | Trust | One buzz names the chain and the need; tap opens the room |

5 seconds: one list, one accent, the need first. 5 minutes: two keys (⏎, ⌥s)
carry the whole day. Five years: the chain is the only idea to learn.

### DESIGN.md changes

DESIGN.md changes in the same pull request as each step (its own rule):

- Step 1: §4 adds `≋` (has a stoker); §6 adds the room, the flip keys, the
  need line, the header's `≋ N`, and the go consent rule; §9 replaces the
  Sessions face with the Chains face and adds the haptics.
- Step 2: §6 Tabs becomes `CHAINS · machine` (digits 1 and 2, no `p`, the
  console opens on chains); §6 Sessions' groups and sorts go; §6 Ships and
  Git move under the room; the Machine tab's order is the one above.

### What a chain is

A chain is a team: every match in it hears every other (owner, 2026-11-07:
"teams for sure"). One team gets one room and one stoker. A match may join
more than one team: it shows in each team's room, with one chat, and two
teams that share a match do not hear each other. Links (A–B and B–C without
A–C) are out.

### Names

The room shows a team's name as `internal/chains` keeps it. Naming and
renaming a team (`matchblox chain <team>`) belong to the chains work
(matchblox-chain); this design adds no naming of its own.

## Constraints

- No sibling product names in the spec, the code, the docs or the landing page.
- A hook keeps its 30 ms budget (`hookTimeout`). The flip is a tmux binding,
  not a hook.
- The flip key must not reach an agent as typing. It is bound in tmux, like the
  home key, and passes nothing to the pane.
- Two steps, both in this scope: step 1 is the room and the flip on today's
  console (the room opens from a chain's head in Sessions). Step 2 folds the
  tabs and removes the groupings. Each step ships green on its own.
- Signed Conventional Commits. Goldens update in the commit that changes the
  screen.

## Left to the builder

- Column widths, truncation and colors, inside the mockup's layout.
- Where the per-client "came from" session lives (tmux option or service
  state).
- How Machine's sections scroll and which one opens first.
- Whether the old tab number keys print a one-time hint about the fold.
- The exact intensity curve of the scroll ticks, inside the limits above.

## Design Review Completion Summary

Review: /plan-design-review, 2026-11-07. Outside voice: a Claude subagent
(19 findings); Codex unavailable (its sandbox cannot start here).

| Pass | Before | After | What remains |
|---|---|---|---|
| Step 0 | 5/10 | — | — |
| 1 Information architecture | 4 | 10 | none |
| 2 States | 3 | 9 | the states table needs goldens to prove it |
| 3 Journey | 5 | 10 | none |
| 4 Specific, not generic | 6 | 9 | none |
| 5 Design system | 4 | 9 | DESIGN.md edits land with each step |
| 6 Phone and access | 4 | 9 | the M-s / M-n key check, the build's first task |

Overall: 3 → 9 (the lowest pass). The owner picked 1A, 2A and 3A; the rest
are review calls on the owner's delegation of 2026-11-07.

Not in scope: new groupings, a chat history view, chains across hosts, a
haptic for busy ↔ idle.

What already exists and is reused: #261's chat, the consent box, the §6
regions, the Ships track, the night palette, §9's faces, `Haptics.swift`
and the terminal view's `UIImpactFeedbackGenerator`.

## GSTACK REVIEW REPORT

| Run | Status | Findings |
|---|---|---|
| plan-design-review (in-host + Claude subagent) | issues_found, fixed | 22 decisions: 3 by the owner, 19 review calls |
| Codex outside voice | unavailable | missing coverage |

VERDICT: design complete. The two 9s close with proof in the build: goldens for the states, and the key check.
OUTSIDE COVERAGE: single-model.

NO UNRESOLVED DECISIONS
