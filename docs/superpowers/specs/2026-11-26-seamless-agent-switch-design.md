# A builder switches a match to another agent and it carries on in the same worktree, mid-thought

## Who, and the moment

A builder (the person at the console) runs a long match. The match works in a Git worktree and is in a chain with peers. One agent runs out of weekly usage, or the builder wants another agent's strengths. The builder presses A in the console, or runs `matchblox switch`, and picks the new agent. They expect the new agent to pick up the same task, in the same worktree, still in the chain, with nothing to re-explain.

Evidence: on 2026-11-26 the owner switched matchbloxenhanced (Cursor, chain mbcopy) to Claude, and nothing happened (#518, fixed in #519, merged as fcb4e504). Cursor was launched in the main checkout but worked in `../matchblox-494`. Only its task record said so, in prose. The owner's words: "seamless switch, same worktree, context injected as start message, as little context lost as possible".

## Terms

- **Launch folder:** the folder the old agent's process runs in (`Session.Cwd`; for Cursor, its guard's folder since #519).
- **Work folder:** the folder the new agent starts in. It is the `Workspace:` line of the old match's task record when that line passes the check below. Otherwise it is the launch folder.
- **Workspace check:** the path exists, is a directory, and is a worktree of the same repository as the launch folder (the same Git common directory). Any other path is refused as a work folder.

## Why it matters

A failed or lossy switch costs the builder a re-briefing: the goal, the corrections, the worktree and the next step. matchbloxenhanced's record shows the task moving claude → cursor → claude across 20 plan tasks. The cost is estimated at 10–20 minutes of re-briefing per switch, plus the risk of work in the wrong checkout.

## The experience

1. Each match keeps a `Workspace: <path>` line in its task record. The launch brief asks for it, and the record refresh asks the agent to update it.
2. The builder presses A and picks the agent. When the work folder differs from the launch folder, the confirm says `in <work folder> (its worktree)`. `matchblox switch --preview` prints the same folder.
3. When the old agent rests, the console shows `◐ <agent> writes its task record 0:12`. Cursor now gets this step too, from its hook's completed `stop`. Esc switches now.
4. The pane restarts with the new agent in the work folder. Its first message points to the handover. The handover holds the task record, the recent conversation, the pane and the Git state of the work folder. For Cursor, the conversation is the builder's prompts and Cursor's replies from its hooks.
5. The match keeps its name, its chain, its holds and its inbox. Peers get no "left the chain" news.

Ways it goes wrong, and what the builder sees:
- The old agent works or asks: `<name> works: y again switches it mid-turn` (today's words). The capture says `Refresh: not refreshed: <name> works` (today's words).
- The `Workspace:` line fails the workspace check: the confirm says `<path> is not a worktree of this repository: it starts in <launch folder>`. The builder can still confirm. That start is the builder's choice, so it does not count against anti-goal 1.
- Cursor's hooks are not installed: the record wait does not run, and the capture says `not refreshed: cursor-agent gives no turn end: run matchblox setup`.
- matchblox cannot read the launch folder: the refusal from #519 names the fix.
- The new agent does not start: `the pane may have no agent now`. The chain, holds and inbox go back to the old match (today's rollback).

## Non-goals

- No copy of the conversation into the new agent's own session file (way C).
- No conversation log that matchblox keeps for every agent (way B). It is the next step only if this one still loses context.
- No parsing of the record's prose for a folder. Only the `Workspace:` line counts.
- No switch of the stoker's own session (today's rule stays).
- No new setting or key.

## Anti-goals (failure even if it works)

- The new agent starts in a different checkout from the match's worktree, and the builder did not choose that.
- The chain says the match left, or the match stays "moving" in its chain, after a switch that started an agent.
- Captured conversation text passes as the builder's instruction to the new agent. The 0.37.0 rule stands: quoted evidence is never an instruction.
- A `Workspace:` line moves the new agent outside the repository.
- A secret from a Cursor prompt or reply is kept anywhere other than the user's private matchblox state, or kept after its session leaves.
- A switch hangs. Every wait ends at its limit.

## Surfaces

- An agent's tools: the first message, the handover and the `Workspace:` line come from agent-written text. The workspace check bounds the folder (anti-goal 4), and captured text stays quoted evidence (anti-goal 3).
- Secrets: Cursor prompts and replies can hold tokens. They are kept only in the user's private state (0700), within today's capture limits (`turnLimit`, `conversationBudget`), and dropped when the session leaves (anti-goal 5).
- Checked and not touched: authentication, money, uploads, webhooks, CI, personal data beyond the user's own prompts.

## Rulings

- 2026-11-26 — The new agent starts in the same worktree, with the context given as its first message, and as little context lost as possible — because the owner asked for a seamless switch. — the owner.
- 2026-11-26 — Way A: close each gap inside today's switch pipeline. Ways considered: A, close the gaps (moderate, fits the design, per-agent coverage); B, a conversation log matchblox keeps (one format, bigger change, needs reply hooks for each agent); C, move the conversation into the new agent's session file (most context, depends on private formats). — the owner.
- 2026-11-26 — A Cursor session's folder and environment come from the guard above it — because Cursor's process is non-dumpable (#518, #519). — the owner.
- 2026-11-26 — The work folder comes from the record's `Workspace:` line, behind the workspace check — because no other signal names the worktree: Cursor's `workspace_roots` and Claude's transcript `cwd` stay at the launch folder (matchbloxenhanced, 2026-11-26). — the owner (approved with the spec).
- 2026-11-26 — The work folder replaces the launch folder for the whole switch: the respawn's folder, the agent's trust, the capture's Git commands and its `Workspace:` line — because one folder per switch keeps the new agent and its evidence in the same checkout. — the owner (approved with the spec).
- 2026-11-26 — A chain move lands on the new agent by its pid when the agent shows no session id. A move with no agent in the pane 2 minutes after the respawn ends as a leave, with the news `<name> left the chain: the switch started no agent` — because a move that never lands breaks anti-goal 2 and anti-goal 6. — the owner (approved with the spec).
- 2026-11-26 — This ships as a feat PR. The landing page changes if it describes switching, as AGENTS.md requires. — the owner (approved with the spec).

- 2026-11-26 — The `Workspace:` line is one line at the start of a record line, with an absolute path. The first one counts. A new record starts with `Workspace: <launch folder>`. The path resolves symlinks and goes to its worktree's top level. A launch folder outside Git accepts only itself. — because a strict format leaves one reading. — the owner.
- 2026-11-26 — After the record wait, matchblox reads the line again. When the folder differs from the one the confirm showed, the switch refuses: `the workspace changed to <path>: switch again`. — because the builder confirms the folder the agent starts in (anti-goal 1). — the owner.
- 2026-11-26 — `matchblox switch` names in its result the folder it started in and any refused line. — because the CLI has no confirm. — the owner.
- 2026-11-26 — The old refresh request text stays recognised, as old switch prompts are. — because it must never pass as the builder's words (anti-goal 3). — the owner.
- 2026-11-26 — A Cursor session has hooks when it sent a hook event. Otherwise the record wait is skipped, and the capture names why. — because only events prove the hooks run for that session. — the owner.
- 2026-11-26 — Cursor prompts and replies are dropped when that agent no longer runs in its pane: after the switch, after it exits, or when recover forgets it. — because they can hold secrets (anti-goal 5). — the owner.
- 2026-11-26 — The 2-minute leave ends the chain membership only. The hold and the inbox stay on the pane for the next agent. — because the builder's hold and queued messages belong to the seat. — the owner.
- 2026-11-26 — A Cursor capture uses the same selection as a transcript: the first prompt, the latest prompts and the last three replies, each in full up to `turnLimit`. — because one rule for every agent keeps the handover the same. — the owner.
- 2026-11-26 — Automated tests cover the unlanded leave and its news, a worktree of another repository, the drop of stored prompts, and a switch with no task. — because the done-when lines need a live Cursor host. — the owner.
- 2026-11-26 — The feat PR carries `Landing page: unchanged because the landing does not describe switching today`. — because `www/` does not describe switching. — the owner.
- 2026-11-26: spec approved. Decided by: the owner.
- 2026-11-26 — The new match's `capture.md` may hold Cursor prompts after the old session leaves. It is the handover itself, and it stays in the user's 0700 bundle. — because the switch needs it. — the owner (plan approval).
- 2026-11-26 — On the 2-minute leave, the member's undelivered inbox lines go back to their senders with today's "unchained" notice. This replaces "the inbox stays on the pane". — because inbox lines are keyed by member, and a later agent in the pane is a new member. — the owner (plan approval).

## Left to the implementer

- How the service tells the console the work folder: a field on the session in the snapshot, or the preview round-trip.
- Where Cursor's prompts and replies are kept, inside the rules under Surfaces.
- Which Cursor hook state tells "rests" after a completed `stop`, so that the record wait reads it as Claude's `TurnEnded`.

## Calls to check

Settled by the owner on 2026-11-26, with the recommendations below.


- Should a switch with a refused `Workspace:` line start in the launch folder or refuse? Recommendation: start in the launch folder after the confirm names it — because the builder sees the choice, and a refusal blocks a usage-stopped match. Cost if wrong: work in the wrong checkout until the builder notices.
- Is 2 minutes the right limit for an unlanded chain move? Recommendation: yes — because an agent that shows nothing in 2 minutes did not start. Cost if wrong: a false "left the chain" for a slow start.

## Done when

- [ ] A Cursor match launched in the main checkout, with `Workspace: <its worktree>` in its record, switches to Claude. Claude's process folder (`/proc/<pid>/cwd`) is that worktree, and the capture's `Workspace:` and `git rev-parse --show-toplevel` lines name it.
- [ ] The confirm of that switch, and `matchblox switch --preview`, name the worktree.
- [ ] A record with `Workspace: /tmp` (not a worktree of the repository) makes the confirm name the launch folder, and the new agent starts there.
- [ ] An idle Cursor match with a managed record and its hooks installed switches with a handoff. The console shows the record wait, and the capture says `Refresh: record written for this switch`.
- [ ] The capture of a Cursor match holds the builder's latest prompts and Cursor's last replies.
- [ ] After a switch Claude → Cursor, Cursor → Claude and Claude → Codex, `matchblox chain` lists the match in its chain, and no peer gets "left the chain".

## Cold read

Cold read: 2026-11-26, 1 fresh reader. Blocker 0, major 8, nit 8. Fixed 14, contested 0, for the person 2, declined 0. Second read: 16 left (major 11, nit 5), all with the person. Changed after the read: Rulings (the owner's answers and the approval).

For the person: the chain-move limit (finding 7) and the refused-workspace fallback (finding 9) are in Calls to check.
