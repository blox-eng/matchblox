# Part 2 gaps — phone / remote / CLI

Date: 2026-11-24. Spec: `design/0021-wedge-against-ade.md` (Part 2).  
**Owner picks before any Part 2 build.** This file inventories only. No product
code lands from this list until a ticket names a failing check.

## Method

Walk desk vs phone (`docs/phone.md`), remote (`docs/remote.md`, design/0017),
and CLI (`matchblox -h` vs console acts in `docs/keys.md`). List only where
desk already works and phone/remote/CLI lags. Each row has a repro or
`unknown — needs spike`.

## Non-goals (stay out)

- Bots / Slack / Linear / GitHub as a product surface.
- Electron, WebGL terminal, browser chrome, plugin marketplace.
- Diff review or owning the agent’s own TUI.
- Matching ADE feature walls.

## Inventory

### Phone

| Gap | Desk today | Phone lag | Repro / unknown |
|---|---|---|---|
| Jump fully into the agent’s tmux pane (`ctrl+o` / Enter jump) | Desk jumps into the pane | Phone docs: cannot go into tmux | Repro: open NEEDS YOU on Pocket; there is no desk-equivalent jump into the agent’s own tmux session. Answer-from-card path works; full pane attach does not. |
| Connect another host / pair a device / update from the phone console | Desk Hosts + Machine DEVICES + update doors | Phone console refuses those acts (documented) | Repro: on Pocket, try Hosts connect or DEVICES pair — blocked by design. Not a bug; may stay non-goal for phone. |
| Buzz → open → answer under flaky network | Desk always has the stream | Pair / reconnect copy exists; edge sharpness vs desk unknown | **unknown — needs spike:** pair → buzz → answer while the host sleeps or Tailscale flaps; compare desk recovery. |

### Remote / multi-host

| Gap | Desk (single host) | Remote lag | Repro / unknown |
|---|---|---|---|
| Wait counts while a peer is stale | Single host: waits are live | After loss, header drops peer waits; `stale` after 30s | Repro: disconnect a ticked peer; confirm NEEDS YOU no longer counts that host’s waits (docs/remote.md). Feel vs 0017 “as crisp as the list” — **unknown — needs spike** whether builders miss a “wait unseen on stale host” cue. |
| Jump when local tmux is old | N/A | Jump refuses if laptop tmux &lt; 3.2 | Documented; fix is upgrade tmux, not a product port. |

### CLI vs console

| Gap | Console act | CLI twin | Repro / unknown |
|---|---|---|---|
| Merge / ship actions from the board | Ships rows, `p` + `y` style confirms | `matchblox ships` lists; merge-from-CLI not first-class in `-h` | **unknown — needs spike:** which ship stages the console can drive that CLI cannot. |
| Machine panel navigation (stoker / git / history / panes views) | Keys on Machine | No `matchblox machine …` twin; status JSON covers metrics | Acceptable for agents (they use status/sessions); humans on headless may want a text Machine. Low priority unless requested. |
| Answer a permission from options | Digit on the card | `matchblox send --pick n` | Covered. |
| Hold / release / compact / kill / chain | Console keys | CLI twins exist | Covered. |

## Recommended fix candidates (at most three)

1. **Phone buzz path spike** — timed repro of pair → buzz → answer under one failure mode (sleep or network flap); only then a ticket if desk recovers cleaner.
2. **Stale-host wait cue** — if spike shows builders miss waits on a stale peer, one quiet header or list cue (no new listen port).
3. **Ship act CLI twin** — only if spike shows agents/stoker need a merge/release that console has and `-h` lacks.

Do not implement these in the wedge plan. Owner picks which (if any) become tickets.

## Gate

- [x] Owner picks zero or more candidates. 2026-11-24: candidate 3 (ship act
  CLI twin: `matchblox merge <who>`). Candidates 1 and 2 are deferred: they
  need a spike first.
- [ ] Each pick gets its own failing check or issue before build. Candidate 3:
  `TestMergeIsACommand` and `mergeAct` tests, see
  `docs/superpowers/specs/2026-11-24-merge-cli-design.md`.
