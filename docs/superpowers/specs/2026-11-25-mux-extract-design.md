# A builder keeps today's native tmux desk, and matchblox reaches every pane only through mux

Issue: #200 step one. Status: in design, 2026-11-25.
Follows: one-way chains mux slice (`internal/mux` for type, keys, paste, screens, mark). Next spec, not this one: native list+bar chrome. Then #205 herdr.

## Who, and the moment

A builder at a desk already runs agents in tmux. The list sits beside a **real** agent pane. Drag, wheel, copy-mode and `ctrl+q` work as tmux. They want matchblox to keep working when they later run agents in herdr, without a second console and without a nested emulator that re-draws the agent's screen.

Evidence: owner, 2026-11-25 — “proper long-term nice architecture we can rely on, extensible and also robust and without degrading the already good enough UX we have currently with the native tmux.” They picked extract-first (way 1). GitHub issue #200 is the parent ticket; this spec copies the acts it needs, so a builder does not need the tracker.

## Why it matters

Today chains, sends, paste and mark go through `internal/mux`. List, split, jump, kill, recover, stoker and beside still name tmux in place: `internal/sample`, `internal/panes`, `internal/service` (lifecycle, recover, peer, stokeragent, copywatch, size), `internal/app` (beside, jump, last client), `internal/remote`, `internal/proto`, `internal/advice`, `cmd/matchblox`. A herdr builder still sees none of their agents. A wrapper that re-parses a nested tmux client would cost fidelity, mouse and wheel — the native pane they already have.

This extract is invisible on purpose. Estimated cost if skipped: a second rewrite when chrome or herdr lands.

## The experience

The builder opens matchblox on a machine that already has agents in tmux.

They see today's console: Sessions beside one agent pane, hosts, stoker, recover, peek, switch, kill. Keys, mouse, wheel and copy-mode behave as they do on `origin/main` at the start of the extract. No new setting, no new bar, no new glyph.

They quit and reopen. Agents are still in their tmux sessions. `⌃o` still jumps into the agent's own pane.

Ways it goes wrong (same words as today, not new copy):

- Sampling finds no tmux or an empty pane list: the console stays as it does on `origin/main` for that case (no silent empty list invented here; no new sentence required).
- Beside cannot open the agent pane: the notice still starts `the agent pane did not open:` and then the error, as today.
- Recover cannot respawn: the row stays “waits for recover” with today's words.

## Approaches

Ways to get a reliable mux, with native tmux as the floor:

1. **Freeze UX, extract mux, then chrome.** Every pane act in the table below goes behind `internal/mux`. Tmux adapter only. Tests and the builder's screen stay as today. A later spec draws list+bar and keeps the agent as a mux pane. Herdr (#205) is a second adapter after the seam is complete.
2. **Bar now on today's splits, mux later.** Faster chrome. Tmux stays in dozens of files. Herdr stays blocked. The bar encodes tmux-isms.
3. **Mux and bar in one slice.** One story. Easy to regress the native pane while the interface is still moving.

The owner picked **1** on 2026-11-25.

This spec is only the extract. Chrome is a later spec. Herdr is #205.

## Pane acts that move

`internal/mux` already has: Type, Keys, Screens, InMode, TypedAt, Load, NoBracketedPaste, Paste, Drop, Mark. Those stay. This extract adds the rest of what matchblox does to panes. Names are left to the builder; the acts are not.

| Act | What the builder needs | Today |
|---|---|---|
| List panes | Every pane with pid, command, directory | `list-panes` |
| Read screen | Visible text, plain or styled | `capture-pane` (already partly mux.Screens) |
| Type and keys | Literal text and named keys | mux Type/Keys |
| Paste | Load a buffer, paste, drop | mux Load/Paste/Drop |
| Start | A session in a directory with an env | `new-session` / `new-window` |
| Rename | Session or window name | `rename-*` |
| Kill | End a pane or session | `kill-pane` / `kill-session` |
| Split | Open the beside agent (or any split matchblox opens) | `split-window` |
| Focus / jump | Bring a pane to the front of a client | `switch-client` / `select-window` / `select-pane` |
| Last client | Which attached client to jump | `list-clients` |
| Restart in the same seat | Kill and start the agent again in that pane (`A`, switch). Pane id stays. | `respawn-pane` |
| Caller's pane | The pane of the process that invoked matchblox | `$TMUX_PANE` |
| Stoker socket | A multiplexer server of matchblox's own | `tmux -L matchblox` |
| Mark | Flip-key mark | mux Mark |

**Peek** and **switch** are console verbs. They use list, read, type, keys, focus and restart. They do not add a mux method of their own.

**Recover** uses list, start, kill and restart. **Beside** uses split and focus. **Hosts** stay a matchblox list; each host has its own mux backend.

## Non-goals

- A herdr adapter (#205).
- Desk chrome: matchblox-drawn bar, `?` / `,` / `t` / `D` sheets, DONE/spaces/ages as a visual redesign. Those wait for the chrome spec.
- Shipping the desk **wrapper** (nested tmux client → pty → vt). PR #504's wrapper does not land as the lasting agent view.
- tmux control mode (`tmux -CC`) as the transport. The extract keeps today's `exec` of tmux.
- Changing Pocket's upright screen, or how `⌃o` jumps.
- Pulling SSH into mux. `internal/remote` keeps the hop; each host has its own mux backend. Remote may still contain the word `tmux` in hop scripts until a later spec.
- Moving `internal/setup`'s “install tmux” door into mux. That door talks to the OS, not to panes.
- Side-by-side tmux and herdr panes in one list (after two adapters exist).

## Anti-goals (failure even if it works)

- The builder loses native select, wheel, copy-mode or cell fidelity on the agent pane.
- A session that was in the list before the change is missing after it, with no reason on screen.
- A key or a send reaches the wrong pane.
- Capture, logs or argv leak a secret from an agent's screen or from the env of start.
- An agent's pane is killed or respawned as a side effect of the extract, when the builder did not ask.

## Surfaces

- an agent's tools — mux starts, types, pastes, focuses, kills and respawns agent panes. Harm: a send or a kill hits the wrong pane, or a spawn runs with the wrong env.
- secrets — spawn env, paste buffers and screen capture can hold tokens. Harm: they reach a log, an error, or another pane's buffer.

Checked, not touched: authentication, money, uploads, webhooks, CI, personal data.

## Rulings

- 2026-11-07 — chains and sends go through a small `internal/mux` (type, keys, read, copy mode); tmux plus a fake. — the owner (one-way chains spec).
- 2026-10-23 — #200: step one is `internal/mux` with tmux behind it and no behaviour change; step two is herdr. — the owner (issue #200).
- 2026-11-25 — freeze today's native tmux UX, extract every remaining pane act behind mux, then a later chrome spec. Wrapper is not the lasting architecture. — the owner (picked way 1).
- 2026-11-25 — one mux backend is one multiplexer server on one host (one tmux socket, later one herdr session). The stoker uses its own socket (`tmux -L matchblox`) beside the builder's. Remote stays outside mux. — the owner (approved the grain).
- 2026-11-25 — the extract keeps today's tmux argv transport (`exec` of tmux). It does not switch to control mode. — the owner (non-goal; closed the open call).
- 2026-11-25: spec approved. Decided by: the owner.

## Left to the builder

- Method names and whether a call returns argv steps (as Type/Keys/Paste do today) or runs immediately.
- Order of moving packages, as long as each step keeps the suite green and the screen unchanged.
- How the service holds one backend per host socket.
- How `internal/arch_test.go` grows: production packages that perform the acts in the table must not name `tmux`, `sample.Tmux` or `TmuxOn`, except `internal/mux`. Exceptions the probe must allow: `internal/setup` (install tmux), `internal/remote` hop scripts (SSH still launches tmux on the peer until a later spec), tests, `internal/demo`, `internal/tmuxtest`. `internal/sample` may keep helpers that **only** `internal/mux` calls.
- Whether `muxtest.Fake` grows in place or splits. Tests that do not need a real tmux keep the fake.

## Calls to check

All closed. Grain: one backend per socket (2026-11-25).

## Done when

- [ ] A builder with agents already in tmux opens this matchblox: the list, the beside agent, `ctrl+q`, peek, switch, `x` kill, recover and the stoker behave as on `origin/main` at the start of the extract.
- [ ] Drag, wheel and copy-mode in the agent pane still belong to tmux, not to a matchblox vt.
- [ ] `go test ./internal -run TestPaneActsNameNoTmux` fails if a production file that performs a pane act still names `tmux` / `sample.Tmux` / `TmuxOn` outside `internal/mux` and the exceptions above. (The test name is a suggestion; the builder may name it. The denylist is not.)
- [ ] `go test ./internal/service ./internal/mux` still passes with `muxtest.Fake` for chain, send, paste and mark tests.
- [ ] With no herdr installed, the console still starts. No setting is required.

## Cold read

Cold read: 2026-11-25, 1 fresh reader. Blocker 2, major 4, nit 3. Fixed 8, contested 0, for the person 1, declined 0. Second read: 12 left.

First read: unified the pane-act table, named the denylist exceptions, froze failure copy, closed control-mode. The open call remains one backend per host socket.

Second read (not fixed): beside/home extra tmux (bind, zoom, options) vs the table; list fields beyond pid/command/dir; peek is CLI not a mux verb; recover uses new-session not respawn; stoker is a second socket on the same host; copywatch/size unnamed; panes helpers vs denylist; remote ViewArgv vs hop exception; kill also TERM; attach-session; grep of bare `tmux` vs quoted argv.
