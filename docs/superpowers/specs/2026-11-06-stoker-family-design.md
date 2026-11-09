# A family of stokers

Issue: #304. Status: for the owner's approval. Date: 2026-11-06.
Builds on: #302 (chain reaction), the stoker with orders (#69, #75).

## Why

A person runs many matches. Today they read Sessions, select a row and
answer each match one by one. Chains (#302) group the matches that work on
one goal, but nobody owns the plan of a chain. The one stoker on the host
works for every session at once, so the goals of two chains mix in one
context.

The person wants to talk to a chain as one thing. They give the chain an
order in their own words, and a stoker that owns that chain drives its
matches. At night, the stoker that runs the night drives each chain's
stoker. That is a family of stokers.

## Goals

1. Each chain can have its own stoker: its own order, plan, card and gos.
2. When the person talks to a chain, they talk to the stoker of that chain.
3. The night stoker heads the family. It gives orders to each chain's
   stoker. A chain stoker drives the matches of its chain only.
4. Sessions, the STOKER tab, the dock and a phone show every stoker of the
   family and the chain it owns.
5. The landing page shows the family: a stoker at the head of each chain,
   and the night stoker that lights them.

## Non-goals

- A stoker for a match that is in no chain. The host stoker keeps those.
- Messages between two chain stokers. Orders go down only: from the person
  or the host stoker to a chain stoker.
- A sandbox. The guard stays a seatbelt (see `docs/limits.md`).
- A family across hosts. A family lives on one host, as a chain does.
- Chains across repos. That work is separate and in progress. This design
  does not depend on a chain's repo.

## Anti-goals

This fails, even if it runs, when:

- A chain stoker acts on a match outside its chain.
- A stoker gets a go that the person did not give. A chain stoker's gos are
  never more than the gos of whoever gave it its order.
- A stoker agent starts and the person cannot see it, or cannot tell which
  chain it owns. Each stoker is one more agent that uses tokens.
- Two stokers drive the same match. Each match has one owner.
- With no chains, the console, the night and the STOKER tab change in any
  way. Without a chain stoker, today's screens stay byte for byte.

## Owner's decisions

- A stoker for each chain. (2026-11-06)
- The landing page must show this. (2026-11-06)
- When the person talks to a chain, they talk to the stoker of that chain.
  (2026-11-06)
- The night stoker runs the stoker of each chain: a family of stokers.
  (2026-11-06)
- Approach A below: a separate stoker agent for each chain. (2026-11-06)
- A night seals the whole family. Each chain stoker with a run on joins the
  night, is sealed and ends at `night_ends`. At night, the person can answer
  a chain stoker's prompts, but gives it no new order. (2026-11-07)

## Approaches

| | What | Gain | Cost |
|---|---|---|---|
| **A. A stoker agent per chain (chosen)** | Each chain stoker is its own agent, in tmux session `stoker-<chain>` on matchblox's own server, with its own run, card and plan. The host stoker gives it orders with `matchblox stoker order`. | Each chain has one owner with its own context. Goals never mix. | One more agent per chain, and its tokens. |
| B. A match of the chain is its stoker | One member also directs the others. | No extra agent. | That member both builds and directs. The guard stops merges in stoker panes, so it cannot merge its own pull request. |
| C. One stoker, one plan per chain | The host stoker keeps one card per chain. | Cheapest. | Every goal shares one context. It is one stoker that juggles, not a family. |

## Words

- **The host stoker**: today's stoker, tmux session `stoker` on
  `tmux -L matchblox`. At night it is the night stoker.
- **A chain stoker**: the stoker of one chain, tmux session
  `stoker-<stoker-id>` on the same server.
- **The family**: the host stoker and every chain stoker.
- **A chain's name**: the name of its first member, for example `api-auth`.
  Where a command takes `<chain>`, it takes that name, the name of any
  member, or the chain id.
- **A stoker id**: each chain stoker has an id of its own (`s1`, `s2`, …),
  not its chain's id. Its tmux session is `stoker-<stoker-id>` and its store
  is `stokers/<stoker-id>/`. It records the id of the chain it owns, and the
  service updates that id when chains join.

## The experience

### Talk to a chain

In Sessions, a chain shows with its fuse (#302). With a chain stoker, the
fuse gets a head row: the stoker.

```text
WAITS FOR YOU
  ╿ ✦ stoker   api · plan 2/5 · asks: merge #812 now?
  ├ ✦ api-auth     busy  41%
  └ ╿ api-docs     idle  12%
```

- `a` on the head row opens the conversation with the chain stoker, in the
  same chat window as any session (#289). The person types an order in
  their own words. The first order of a run comes from the start sheet. A
  later order by day goes in as `New order: <words>`, as `o` does today, and
  goes onto the card. At night the person answers prompts only.
- `a` on a member row still talks to that member. The person can answer a
  member's question directly.
- `a` on the head row shows the stoker's pane, as the chat does today
  (#289). The person can answer the stoker's prompts there.
- A chain with no stoker shows no head row. `a` on the fuse of such a chain
  opens the start sheet (below). So does `a` on the head row of a stoker
  with no run on: a new run needs an order and its gos.
- The chain moves into the group of its neediest row, the stoker row
  included. A stoker that asks the person puts the chain in WAITS FOR YOU.

### Start a chain stoker

The start sheet is today's launch sheet, scoped to one chain:

```text
Stoke the chain api (2 matches)?
How should I run this chain?  ____________________________
gos ✓ merges  ○ releases  ○ deploys  ○ kills
Enter starts · Esc closes
```

- `Enter` starts the agent in `stoker-<stoker-id>`, writes its brief and
  types `Read <path> and follow it.` into its pane.
- From the shell: `matchblox stoker new --chain <who> ["<order>"]`, where
  `<who>` is any member of the chain. Its gos come from `--go merges,…`.
  With no `--go`, it has none. The same holds for
  `matchblox stoker order` from the person's shell.
- A chain stoker starts only after an act: the person's `Enter`, their
  command, or an order from the host stoker. It never starts by itself
  when a chain forms.

### Runs

- Each chain stoker has a run of its own (its own `stoker.Mode`), with its
  order, gos, `max_steps`, triggers, holds and driven sessions.
- By day, a chain stoker's run is apart from the host stoker's: `f` turns
  the host stoker on or off only.
- `n` starts the night. Each chain stoker with a run on joins it: its run
  is sealed and ends at `night_ends`. `n` again ends the night for the
  family. A chain stoker that the host stoker starts at night is part of
  the night.
- `max_steps` counts for each stoker. With `max_chains = 4`, a family
  takes at most five times today's steps.

### The night heads the family

- `n` starts the night as today. The brief of the host stoker lists each
  chain, its stoker (or none) and that stoker's plan.
- The host stoker gives a chain stoker an order with
  `matchblox stoker order <chain> "<text>"`. If the chain has no stoker,
  this starts one. Its gos are the gos of the order's giver, or fewer with
  `--go`. The order goes onto both cards.
- A night is sealed for the person, as today. The host stoker can still
  give orders to chain stokers during the night: that is its job.
- The host stoker does not act on a match whose chain has a stoker with a
  run on. It acts through that stoker. One owner for each match. A chain
  stoker with no run on owns nothing: the host stoker acts on its chain.
- When a chain stoker's run starts, the host stoker's holds on its chain's
  matches pass to it. A kill or a go of the host stoker that waits on its
  card for one of those matches is dropped, with a line on the card that
  says so. Triggers that name those matches pass to the chain stoker.
- When the night ends, each chain stoker that the night started rests, and
  the morning card lists each chain with its result.

### What a chain stoker can do

All of today's stoker rules apply (see `docs/stoker.md`, "What the stoker
agent cannot do"), and these:

- Its `send`, `compact`, `hold`, `release` and `kill` reach the members of
  its chain only. Any other target fails with
  `<who> is not in the chain of this stoker.` and nothing happens.
- Its `peek`, `sessions`, `ships` and `usage` read the whole host. Reading
  is not acting.
- A session it starts with `matchblox new` joins its chain with no ask: the
  stoker asks for the chain. A start in a folder outside the chain's repos
  is refused while chains are one repo each.
- It cannot give an order to another stoker, start one, or give a go.
- During its run, the guard stops merges, releases and deploys in its panes,
  as in the host stoker's panes today. Its sessions merge with the go the
  chain stoker has.

### The STOKER tab, the dock and the header

- The STOKER tab lists the family: the host stoker's card first, then one
  card for each chain stoker, under the chain's name. `Enter` on a card
  opens that stoker's pane. `Q` and a typed `y` quit that stoker only.
- The dock and the header count the family: `stoker · 2 chains`. With no
  chain stoker, they read as today.
- `matchblox sessions --json` gives each stoker row `"stoker": true` and
  `"chain": "<chain-id>"` and `"stoker_id"` (both empty for the host stoker).

### When a chain ends

- A chain with fewer than two members ends (#302). Its stoker writes its
  result on its card and quits.
- When two chains join and one has a stoker, that stoker owns the joined
  chain.
- When both have one, the stoker of the chain whose id survives stays. The
  other writes on its card that its chain joined `<name>`, and quits. Its
  latest order is typed into the surviving stoker as
  `New order (from the joined chain <name>): <words>`, with only the gos
  that both stokers had.

## The landing page

The chain section (`www/index.html`, "Chain reaction") gets a stoker at the
head of each row of matches. A new beat follows it: the night stoker lights
the head of each chain, and each chain's stoker lights its matches. The text
says what the person does: talk to a chain, and its stoker answers. The
section names the commands `matchblox stoker new --chain <who>` and
`matchblox stoker order`. It names no other product. It renders in dark,
light and at 390 px, and it honors `prefers-reduced-motion`.

## How it fits together

- **State.** Each chain stoker has a store of its own:
  `stokers/<stoker-id>/` in the state directory, with today's files
  (`stoker.json`, `stoker.jsonl`, `stoker-runs.jsonl`). The host stoker
  keeps today's paths. `internal/stoker`'s `Store` opens a directory, so it
  serves both.
- **Who acts.** Today every pane of matchblox's own server is the
  stoker's. Now the tmux session of a pane on that server names its stoker:
  `stoker` is the host stoker, and `stoker-<stoker-id>` is a chain
  stoker. The service resolves the pane from the command's pid, as today.
- **Scope.** The service checks each act of a chain stoker against the
  members of its chain (`chains.Store.Mates`) at the time of the act.
- **Gos.** An order records its gos and its giver. A chain stoker's gos at
  each act are its own gos and its giver's gos now, together: when the
  host stoker loses a go, or the night ends, its chain stokers lose it too.
  An order with a go that its giver does not have is refused.
- **Commands.** New: `matchblox stoker new --chain`, `matchblox stoker order`.
  `matchblox stoker quit` takes `--chain <who>`.
- **Docs.** `docs/stoker.md` gets "A family of stokers". `docs/agents.md`
  (chains), `docs/limits.md`, `docs/config.md`, `docs/keys.md` and
  `CHANGELOG.md` follow.

### Rules that guard the anti-goals

Each has a test:

- A chain stoker's act on a non-member is refused.
- An order with a go that its giver does not have is refused.
- A chain stoker's act needs the go both in its own gos and in its giver's
  gos at the time of the act.
- The host stoker's act on a match whose chain has a stoker is refused, with
  `<who> has the stoker of chain <id>: give it an order.`
- With no chain stoker, the Sessions, STOKER tab and dock screens match
  today's golden files.
- Each stoker start writes a line to the host card and the run log.

## Config

```toml
[stoker]
max_chains = 4   # chain stokers alive at once; a fifth start is refused and says why
```

A chain stoker runs the agent of `[stoker] agent`, on the default sign-in,
as the host stoker does today.

## Done when

- From the head row of a chain, the person gives one order and takes one
  task to a merged pull request. They type only the order and the `y`.
- At night, the host stoker gives orders to two chain stokers, and each
  drives its own chain. A test shows that neither can act on the other
  chain's matches.
- With no chains, every golden screen file is unchanged.
- The landing page shows the family in dark, light and at 390 px.
- `go test ./...` and `go vet ./...` pass, and the docs check passes.

## Calls made in writing this

- `a` on a member row still reaches that member, so the person can answer
  one match directly.
- The seven calls that answer the builder check, each marked in the text:
  live gos, ownership while a run is on, the chat as the pane, a chain's
  name, a stoker id apart from the chain id, `max_steps` per stoker, and a
  new session that joins its chain with no ask.
- A chain stoker never starts when a chain forms. Each start is an act of
  the person or the host stoker, because each stoker uses tokens.
- `max_chains = 4` by default, to cap the tokens of a family.
- A chain stoker runs on the agent and the sign-in of the host stoker.
- Reading the host (`peek`, `sessions`) is open to a chain stoker. Acting
  is closed outside its chain.
- When chains join, the stoker of the surviving id stays.

## Left to the builder

- The exact words of the head row and the start sheet, within the rules of
  `DESIGN.md`.
- The animation of the landing page's new beat.
- The format of a stoker id, within what tmux takes in a session name.
