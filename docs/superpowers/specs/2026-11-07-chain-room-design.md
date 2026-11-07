# Chain room

Issue: to be filed (a follow-up to #304). Status: for the owner's approval. Date: 2026-11-07.
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
- A row shows ✦ when the chain has a stoker, the chain's name, its count of
  matches, the one thing that needs the owner (or the chain's state), and the
  stoker's plan progress (`plan 1/3`).
- The fuse and chain head of #299/#312 stay inside the room, not in the list.

### The room

From top to bottom (see the mockup's option A):

1. **The stoker's card.** Its order in quotes, then its plan steps with ✓, ▸
   and plain marks. A step that waits for a go says so, with `y gives it`.
   `y` gives the go to the selected step, as today's STOKER tab does.
2. **The matches.** One line each: the state dot, the name, and its state or
   its ask. `a` opens the selected match (attaches to its session).
3. **ships.** The chain's PRs only, one line: live, checks, waits for merge.
4. **git.** The chain's worktrees only, one line: count, merged, safe to
   remove.
5. **The talk line.** `› tell <chain>'s stoker…`. ⏎ sends the line to the
   chain's stoker.

Keys in the room: ⏎ talk, ⌥s flip, `y` give go, `a` open match, tab next
chain, esc back to the list. Every key shows in the hint line.

### The flip

- ⌥s (a tmux key matchblox binds, like the home key) goes from any session of
  a chain to that chain's stoker. On the stoker, ⌥s goes back to the session
  the owner came from. matchblox stores that session per client.
- ⌥n walks the chain's matches in order, then the stoker, then the first match
  again: the stoker and its matches form one loop.
- ⌥b goes to the console, the same as the home key today.
- From a chain of one, ⌥s goes to the host's stoker (my call: #304's host unit
  stays the stoker of sessions in no chain, so the chain-of-one rows share
  it). That room's card shows only the host plan's steps for that session.
- The flip works inside one tmux server. A peer host's session flips on that
  host's own console. In this console, ⌥s on a peer's chain opens its room
  (my call).
- ⌥b is new on the desk, next to the home key and prefix m, which stay. The
  phone has no Option key: its key bar gets flip and next buttons that send
  user-key sequences, as the ◂ button sends the home key (my call).
- In the console, on a chain's row or in its room, ⌥s goes to its stoker too.

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
- Machine holds, in this order: the host stoker's controls (`f` launch or
  stop, `n` night runs, the sealed night, the family dock), usage and limits,
  CPU, history, panes. Each is a section of one scrollable screen. Peer hosts
  show as they do today (my call).
- The Sessions, Ships, Git, History, Panes and STOKER tabs go. Their content
  lives in the room (per chain) or in Machine (per machine).
- The need, status and repo groupings go, with their keys and config.

### The phone

- At 40 columns the room fills the screen. The chain list is one step back
  (‹ or esc). The mockup's phone frames are the target.
- The Live Activity (MatchbloxIsland) shows the chain that needs the owner
  most: its name, its stoker's plan progress and its one need. Today it knows
  only the host stoker.
- The buzz (MatchbloxBuzz) names the chain and the need: `hearth: merge
  #4552? needs your go`. Tapping it opens that chain's room.
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

- If ⌥s, ⌥n or ⌥b collides with a key an agent CLI or tmux default uses,
  pick the nearest free key and keep the same key on desk and phone.
- Column widths, truncation and colors, inside the mockup's layout.
- Where the per-client "came from" session lives (tmux option or service
  state).
- How Machine's sections scroll and which one opens first.
- Whether the old tab number keys print a one-time hint about the fold.
