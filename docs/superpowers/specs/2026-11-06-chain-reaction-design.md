# Chain reaction

Issue: #299. Status: for the owner's approval. Date: 2026-11-06.
Mockup: the "Chain reaction design" page (owner's artifact, 2026-11-05).

## Why

A person runs four agents. Two of them work in one repo and do not know about
each other. One rewrites a file the other is halfway through. The person finds
out at merge time.

Chain reaction fixes that moment. Matches in one repo learn about each other.
A match chooses to join another's chain, and chain-mates then hear each other's
news between their steps. Only chain-mates can message each other.

## Goals

1. Two matches in one repo each learn about the other at the start of a turn,
   with no setup by the person.
2. Chain-mates hear each other's commits, branch changes, finishes and waits,
   and are never interrupted mid-thought.
3. `matchblox send` reaches only chain-mates. A stranger cannot type into a
   session.
4. The person sees every chain in Sessions, in every grouping, and on a phone.
5. The landing page shows the feature as a fire domino.

## Non-goals

- Matching "similar work" by topic, from what agents say.
- A chat-room screen or a message history view.
- Chains across hosts. A chain lives on one host.
- Hooks for Codex and OpenCode. They get the at-rest wake only.

## Anti-goals

This fails, even if it runs, when:

- A chain-mate's news breaks into an agent's turn mid-thought, or a typed line
  lands in a half-written prompt and gets sent.
- A session that is not in the chain can type into one that is.
- A chain exists that Sessions does not show.
- The session that waits for the person is cut, hidden or moved below the
  fold by a fuse.
- A hook makes the agent wait. Today's budget is 30 ms (`hookTimeout`).

## Owner's decisions

- Sessions should be aware of other sessions working on similar things, and
  this is configurable with sensible defaults. (2026-11-05)
- Agents chain themselves, as matches would, and burn together. (2026-11-05)
- The name is chain reaction. A row of matches that light each other is a "fire
  domino". (2026-11-05)
- The landing page shows it: the matches catch one by one. When the last
  lights, the first goes out, then the rest. Each smokes a little, and they
  light again at random. (2026-11-05)
- In Sessions, a fuse (option B of the mockup) joins a chain's matches.
  (2026-11-05)
- A chain moves as one into the group of its neediest match. (2026-11-06)
- Approach A below. (2026-11-06)
- A chain needs both sides: the target, or a member of its chain, must chain
  back. (2026-11-06)

## Approaches

| | What | Gain | Cost |
|---|---|---|---|
| **A. Hooks deliver, typing only at rest (chosen)** | The service keeps chains and an inbox per match. The turn-start hook, and on Claude Code the after-tool hook, print what is new. A message to a chain-mate at rest is typed into its pane to wake it. | Never interrupts. The risky typing path runs only for an idle prompt. | Full only on Claude Code. Cursor gets news at turn start only if its hook can carry text. Codex and OpenCode get the wake only. |
| B. Type everything | Every note is typed into the pane, as #297 does | Same for every agent | Interrupts working matches. Every #297 typing bug applies. |
| C. The agent asks | `matchblox chain news`, and the brief says to run it | Cheapest, works anywhere | Nothing is pushed. Agents rarely ask. |

## The experience

1. **Strike.** The person starts sessions as today.
2. **Notice.** When two matches work in one repo, each gets one line at the
   start of its next turn:

   ```
   [matchblox] peer-send also works in matchblox, on feat/peer-send.
     To hear from it, run: matchblox chain peer-send
   ```

   The line shows once per pair. If neither chains, matchblox does not ask again for that pair
   until one of them changes branch.
3. **Chain.** Both sides consent. `matchblox chain <who>` asks to join the
   chain of `<who>`. The request waits until `<who>`, or any match already in
   its chain, runs `matchblox chain` back toward the asker. Then the two chains
   become one chain. Until then nothing is shared, and the target hears the
   request at its next turn:

   ```
   [matchblox] docs-release asks to chain with you. To accept, run:
     matchblox chain docs-release
   ```

   A request lapses after one hour, or when either match leaves the repo. `<who>` is a session name or pane, as in `matchblox peek`.
   Both matches must be in one repo on one host.
4. **Burn together.** At the start of a turn, and after each tool call on Claude
   Code, the hook prints what is new since the match last heard:

   ```
   [matchblox] since your last turn:
     sessions-list committed 2 files in internal/app/sessions.go
     www-chain waits for the owner
     [from www-chain %14] is the chain flag still called [chains] in config?
   ```

   The news covers: a commit (its file count and up to three paths), a branch
   change, a finish (Stop), a wait for the person, and a message.
5. **Talk.** `matchblox send <who> "<text>"` reaches a chain-mate only. A busy
   one gets it in its inbox. One at rest gets it typed into its pane, which wakes
   it. The tag `[from <name> <pane>]` names the sender. The pane id is part of the tag, so a renamed
   window cannot forge it.
6. **See.** Sessions draws the fuse. See [Sessions](#sessions).
7. **Burn out.** A chain ends when its matches end. `matchblox unchain` takes
   the match out of its chain. A chain of one is no chain. Nothing is left in the
   state dir.

`matchblox chain` with no argument lists the match's chain, with `--json` for
agents.

## Sessions

The rule: a chain stays together in the group of its neediest match. A view
that splits it leaves each match a stub, and selecting one lights the rest.

- **The fuse column.** After the match glyph: two columns from 70 columns of
  list, one column below. Every row keeps the column while any chain is in the
  view, so names stay aligned. With no chain in the view there is no column,
  and the list is today's, unchanged.
- **Need (default).** The chain moves into the group of its neediest match.
  Inside it, members keep need order: the one that waits first. The label keeps
  its count of the group's own and adds the chain: `WAITS FOR YOU · 1 · chain
  of 3`. Glyphs: `╮` first, `┤` middle, `╯` last, with `─` before each from 70
  columns.
- **Status, and none with a sort.** Members stay where the view puts them. Each
  has a stub `─╴`. The stubs of the selected row's chain light in the accent;
  others are faint. The selected row's own stub is `━╸`.
- **Repo.** A chain is in one repo, so its members are always adjacent there:
  the full fuse.
- **Two chains** next to each other each open with `╮` and close with `╯`.
- **A filter** that hides members joins the visible ones. The label adds
  `· <n> hidden`.
- **Child rows and the phone's opened block** carry `│` in the fuse column. The
  hairlines around the opened block start after it.
- **A held match** keeps its place in the chain.
- **The spark.** When a chain-mate commits or sends, a bright cell runs down the
  fuse once, as other changes play once today (`noteChanges`). No spark with
  `NoMotion`.
- **A pair that could chain.** A session in the same repo as another, not
  chained, shows a faint `· same repo · chain?` after its name, while the name
  has room.
- **The context bar and the place column** stay as today. The owner's "activity
  line" is not identified yet. It is out of this design until the owner points
  at it.

## The landing page

A section "Chain reaction." in `www/index.html`, after "Every agent in tmux".
It holds a row of standing matches drawn in SVG, a lede, and the three beats
(they notice, they chain, they burn together). The motion follows the
owner's decision above. With `prefers-reduced-motion`, every match burns, still.
It uses the site's tokens and its match colours.

## How it fits together

- **Who can chain.** Any agent in Sessions. What each one gets differs:
  - Claude Code gets the news at turn start and after each tool call, and the wake at rest.
  - Cursor gets the news at turn start if its hook can carry text. If it cannot, it gets nothing pushed.
  - Codex gets the wake at rest. A message sent while it is busy is typed once
    it comes to rest.
  - OpenCode can be in a chain and shows on the fuse, but cannot receive a send,
    because a typed line can submit its draft. `send` to it fails with "<who> runs
    OpenCode, which matchblox cannot type into safely. It sees the chain in
    `matchblox chain`."
- **A match that leaves the repo** leaves its chain at the next sample. Its
  chain-mates hear "<who> left the chain: it moved to <repo>".
- **Chains** live in the service, keyed by the session's identity
  (`SessionID`, else pane and process start). They are saved in the state dir
  as `chains.json`, so a service restart keeps them. Members that no longer
  exist are pruned on each sample.
- **The repo** of a session is the git checkout its `Cwd` is in, worktrees
  included, as `repoOf` reads it today. Two worktrees of one repo are one repo.
- **The inbox** holds each match's news since its last delivery: at most 20
  items, the oldest dropped first, and a count of the dropped ones ("and 3
  more").
- **News sources:** Stop and the wait from the hook queue. Messages from
  `send`. Commits and branch changes come from two places, because the git scan
  (`gitscan.Report`, every 5 minutes) has no HEAD and no file list:
  - On Claude Code, `PostToolUse` of Bash spots a `git commit`, `git switch` or
    `git checkout`, and names the match that ran it at once.
  - For every other agent, each sample reads `HEAD` and the branch of each worktree
    that holds a chained match (`git rev-parse HEAD`, chained worktrees only). When it moves, the
    news names the match if it is the only one in that worktree, else "someone
    in <worktree>". Files come from `git diff --name-only <old> <new>`.
- **Delivery.** The hook asks the service for the match's news with a reply,
  inside the 30 ms budget. On a timeout it prints nothing, and the news waits
  in the inbox for the next delivery. On Claude Code, `UserPromptSubmit` prints
  it as plain text. `PostToolUse` returns it as `additionalContext` in
  `hookSpecificOutput`. `SessionStart` prints the notice for a match that is
  already in one repo with another.
- **Typing at rest** reuses #297's path, with its review's fixes:
  - It types only into agents that read a prompt line (claude, codex) and only
    at rest.
  - It refuses a pane in copy mode (`#{pane_in_mode}`).
  - It waits up to 2 s for the line to show, matching `[from `. If the line does not show, it clears the line and reports
    "not delivered".
  - It holds the send lock only while it reserves the rate slot.
  - It refuses `unicode.Cf` and the other hidden characters.
  - The tag carries the pane id.
- **Send's gate.** The service refuses a send from a sender it cannot place
  in a pane (an orphaned process) when the client is not the console. The
  person's own sends from the console are unchanged.
- **The limits** of #297 stay: one message per pair each 30 s, 20 an hour per
  sender.
- **The brief** ("A match knows its surroundings", docs/agents.md) gains the
  chain commands. It says plainly that a chain gives a match the right to message
  its chain-mates, and nothing more.
- **Permissions.** `Bash(matchblox chain:*)` and `Bash(matchblox send:*)` join
  `HandRules`. A send outside the chain fails in the service, whatever the
  permission says.

### Config

```toml
[chains]
notice = true        # the same-repo line at the start of a turn
send = "chain"       # who send reaches: "chain" or "off"
```

A session opts out with `matchblox chain --off`. It gets no notice, cannot be
chained, and leaves its chain. `[peers] send` from draft #297 never shipped, so it is
dropped with no alias.

### Rules that guard the anti-goals

Each has a test:

- A send to a session outside the sender's chain fails, and nothing is typed.
- `chain <who>` from one side alone shares nothing: no news, no send, no fuse.
- News is never typed into a busy pane: a test with a busy pane and a message
  sees the inbox grow and no `send-keys`.
- A typed line that does not show within 2 s is cleared, and the send reports
  it as not delivered.
- The hook returns within its budget with the service stuck: the existing
  test of "less than 50 ms with the service up, down or stuck" covers the reply.
- Every chain in the snapshot draws in Sessions, in each grouping, at 44 and at
  150 columns. The waiting session's row is whole at 44 columns.

## Done when

- Two Claude Code sessions in one repo on a throwaway tmux server: each sees
  the notice, one chains, and the other hears its commit at its next tool call.
- A send from a non-member fails with "<who> is not in your chain. Run
  `matchblox chain <who>` first."
- Sessions shows the fuse in need, status, repo and none, at 44 and 150
  columns, and the opened block on a phone.
- The landing section plays as decided, and is still with reduced motion.
- The docs (agents.md, config.md, the CLI help) say what chains do and do not
  grant.
- #297 is closed or folded into this branch.

## Calls made in writing this

- Chains are one host only, because one repo on two machines is two checkouts
  that cannot collide in a file.
- The notice repeats for a pair only after a branch change, because a notice on
  every turn is noise.
- Commit news comes from the hook on Claude Code and from a HEAD read of chained
  worktrees for the rest, because the 5-minute git scan is too slow for the
  next tool call.
- A match that moves to another repo leaves its chain, because the fuse and the
  Repo grouping assume one repo.
- OpenCode is in chains but receives no send, because a typed line can submit
  its draft.
- The inbox keeps 20 items, because a chain-mate that returns from a long tool
  call needs a summary, not a log.
- The activity line is left out until the owner names it.
- The notice says "on a detached HEAD." for a match with no branch. It is
  skipped while either side has asked the other, and while either side is off.

## Left to the builder

- The exact wording of the notice and the news, in the tone above.
- The commit summary's limits beyond three paths.
- The reply message's shape between the hook and the service.
- How the spark is timed, within one pass per event.
- The landing section's SVG, from the mockup.

## To check while building

- Whether Claude Code's `PostToolUse` reads `additionalContext` from
  `hookSpecificOutput` in the installed version. If it does not, news arrives at
  turn start only.
- Whether Cursor's `beforeSubmitPrompt` can add text to the turn. If it cannot,
  Cursor gets the at-rest wake only, and the docs say so.

Checked 2026-11-06. The live Claude Code run could not start: the CLI had no
valid API key. Claude Code's hook docs say `additionalContext` from
`PostToolUse` goes into the context next to the tool result, so the print
stays. Cursor's hook docs give `beforeSubmitPrompt` only `continue` and a
`user_message` shown to the person, so it stays silent. The same docs give
`sessionStart` and `postToolUse` an `additional_context` that goes into the
conversation, so Cursor hears the news there. matchblox types nothing into
Cursor: a message to it waits for its next tool call.

## As built

What changed from this spec while building it:

- **The sender gate goes by terminal.** The service tells an orphaned agent from
  the person by the sender's controlling tty: a tty that matches an agent's pane
  is that agent, and no match is the person. Where the system gives no peer
  credentials (Windows) a CLI send is refused and the console sends. The gate is
  not a sandbox: a process that takes a fresh pty can pass as the person.
- **The gate covers `answer:` and `pick:` too.** `pick:` can approve another
  session's permission menu, so it passes the same gate as a send.
- **Cursor hears the news through `sessionStart` and `postToolUse`**, as
  checked above; matchblox types nothing into it.
- **Chains stay off under `--fixtures`.** The demo and fixtures runs keep no
  state beyond their temp dir, and the demo shows no chain.
- **Asks are told once each.** A request is not repeated after every tool call;
  `matchblox chain` still lists the ones waiting.
- **News is given back until it is acked.** The hook acks after it prints; the
  service returns an un-acked item to the front of the inbox after 200 ms, so a
  hook that gave up loses nothing, at the cost of one more message per delivery.
- **A queued Codex line is tried three times**, then dropped with a note to the
  sender's inbox: "queued" does not end in silence.
- **The same-repo hint compares full repo paths**, not base names: two repos both
  named `api` are not one repo.
- **A chain needs two matches here.** One with a single match left in Sessions
  draws no fuse and no count.
- **`Chain.Spark` is not saved on its own.** A spark is a moment for the
  console, not state.
- **The landing section's second beat** says "until both sides chain".
