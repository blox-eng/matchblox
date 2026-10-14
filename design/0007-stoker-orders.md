# #69 The stoker takes your orders, in your ways

Mock (approved 2026-10-13): https://claude.ai/artifact/1cbN3jcXRZFc1aCMUJKddp

## Who, and the moment

Martin runs a dozen agent sessions at once and hands the machine to the stoker for the night or while he is away. Today the stoker only compacts. On those nights he wants more: pause some builds so the ones that matter get the 5-hour and weekly windows; when a session opens a PR, compact it and send it the review skill; set the ready sessions up in merge order and have each one watch the PR before it and merge on green. In the morning he wants to see why each idle session is idle, which ones he parked, and what happened to each thing he asked for.

Those flows are Martin's own. The product must not hard-code them. What matchblox gives every user is a stoker that **takes orders in plain words and follows the user's own ways**, plus the sight and the hands it needs to carry them out.

## What is missing today

- The stoker has one job (compact at the right moment) and takes no instruction.
- A quiet session only says `idle`, `cold` or `stale`. It never says why: waiting on a merge, PR merged, limit hit, stuck, out of work.
- You can't park a session. A session you know is on hold sits in the quiet pile like any other.
- Sessions groups by need only. `s` sorts by a column into a flat list, and there is no way to group by status or repo.
- Nothing acts on the sessions as a whole. Merging a stack in order, pausing the non-priority work and reviewing every new PR are all done by hand, session by session.

## What you would see and do

### 1. Sessions: why it is idle, park, group by

- **Every quiet row says why it is idle**, in the column where the state word is today (mock step 1). The reasons, roughly in order of precedence:
  - `#4556 waits your merge`: its ship is green and waits for a person.
  - `CI red · <job>`: its ship's CI failed.
  - `#4536 merged · done`: its ship is merged or live.
  - `5h limit · back 16:00`: today's `out`, now with the time.
  - `stuck: <short error>`: the last turn ended on a tool error or an API error.
  - `out of work`: plan at 100%, or a turn that ended with no question and nothing in flight.
  - Otherwise plain `idle`.
  These are read from data matchblox already has (the transcript, ships, limits). No model is involved, and each reason is a deterministic rule with a test.
- **`z` parks the selected session**, and `z` again lets it go. A parked session moves to a **HELD** group at the bottom of Sessions, reading `parked by you`. The stoker never messages or compacts it, and `matchblox send` refuses it. Parks live in the service state and survive restarts.
  - The stoker's own holds (below) land in the same HELD group, reading `held by stoker · <why>`. `z` on one of those releases it: matchblox tells the session to carry on.
- **`g` cycles the grouping**: by need (today's, the default), by status, by repo, none. `s` / `S` keep sorting within groups (today's flat-list sort becomes "group: none"), and gain newest-first and oldest-first. Both choices are remembered per console.
  - The mock showed sort on `o`. In the build, sort stays on the existing `s`/`S`, and `o` is the new order (§3).

### 2. The stoker is an agent you choose, in a pane you start

- (Revised 2026-10-14, see the end: matchblox now starts the stoker on its own tmux server.) matchblox does not start agents (design/0001 §1). You start your stoker the way you start any session: `tmux new -s stoker claude` (or `codex`, or `opencode`). The config names it: `[stoker] pane = "stoker"` (a tmux session name). Claude Code is what the docs show.
- Without one, the stoker works exactly as today (compacts only). The launch sheet then shows the door: `Start a stoker agent: tmux new -s stoker claude`, with the copyable command.
- The stoker pane is a session like any other in Sessions (marked `stoker`), and its context gets compacted like any other. Everything that matters lives in matchblox's run log, not in its context.
- **The agent's hands are matchblox commands**, the same for every agent. Each one goes through the service and its guards:
  - `matchblox sessions --json`: every session's state, idle reason, ship, context, plan %, account, parked/held.
  - `matchblox ships --json` and `matchblox usage --json`.
  - `matchblox send <session> "<text>"` and `--pick <n>` for a menu: the peek's path, with its pane lock, its read-before-type and its refusals (a prompt up, a typed line, parked).
  - `matchblox compact <session>`: the stoker's existing compact step.
  - `matchblox hold <session> --why "<text>"`: marks it held and queues one message, "finish this step, then stop and wait". It never interrupts: a session in the middle of a turn uses the window until that step ends (Martin, 2026-10-13: "queue it is"). The stoker still "never ends or kills anything". `matchblox release <session>` lets it go with "carry on".
  - `matchblox stoker plan`, `matchblox stoker step`: write the plan and log steps to the card (below).

### 3. Orders at launch

- **`n` (night) and `f` (stoker on) open the launch sheet** (mock step 2): "How should I run tonight?", a text line, and the recipe names from your ways as chips to tap into the order. Blank is fine: the stoker then runs your standing rules only.
- **`⏎` sends the order** to the stoker pane, wrapped in a short brief, and the run starts at once. The brief covers the role, a snapshot of the machine, the path of the ways file, the commands above, the run id, the gos, and "start now, keep your plan current with `matchblox stoker plan`". Revised 2026-10-14 (below): there is no plan gate.
- **The gos are the builder's chips** on the sheet: `merges`, `releases`, `deploys`, each off (it asks you) until toggled. The agent writes and rewrites its plan on the card whenever it likes; nobody approves it.
- `Esc` closes the sheet and sends nothing.
- **A night is sealed.** Once it runs it takes no new order. `n` stops it, and a new night takes new orders.
- **By day, `o` gives a new order** (mock step 5): one line, `New order: <words>`, to the stoker that runs. No restart and no new brief; done steps stay done, and the run's gos become the sheet's chips.

### 4. The run: through the sessions, woken by matchblox

- **The stoker acts only through sessions**, by messaging the session that owns the work. It never merges, pushes or edits code itself.
- **matchblox wakes the stoker** when something it may care about happens, and only when the stoker pane is at rest. The triggers:
  - a session's turn ends;
  - a ship moves a stage or appears (a new PR);
  - a usage window crosses a line (each 10% of the 5h and the week);
  - the night is about to end.
  Wakes are coalesced to at most one a minute, as one short message listing what changed. The agent decides what that means for its plan and its standing rules. This is how "PR up → compact + /review" works without matchblox knowing that rule.
- **Each step logs to the card** with `matchblox stoker step <n> --state doing|done|failed|waits "<text>"`. A step in flight says what it waits for. A failed step says why, and when a person can resolve it, it names the session to answer.
- **Merges, releases and deploys keep their confirms.** During a run, the PreToolUse hook denies `gh pr merge`, `gh release …` and `gh workflow run` in a session whose last input came from the stoker, unless the run carries that grant. The denial tells the session "the stoker's order did not give the go for this". The grant comes only from the go chips the builder set on the launch sheet. What waits for a person shows on the card, and `y` gives the go for that one step.
- Each step on the card opens (`⏎`) to the exact command matchblox ran for it: `matchblox send Weather "…"` (design/0001 §2.5, no action without its exact command on the screen).
- A run sends at most a set number of messages (`max_steps`, 60 by default) and ends at `night_ends` like today.

### 5. The card

A STOKER tab (it replaces `c`'s whole-card view; the dock panel stays the glance), mock steps 3 and 4:
- Your words, quoted.
- `PLAN n of m`, each step with its time, glyph (✓ done, ◐ doing, ✕ failed, ⏸ waits for you, ○ next) and a one-line outcome.
- STANDING RULES: what the rules did, logged the same way.
- At the end of a night: `DONE 4 of 5 · 1 WAITS FOR YOU`, and every failed or waiting step carries its one action (`a` answers that session, `y` gives the go).
- Today's compact steps and RESUME lines stay on the card as steps.
- The run log goes into the existing `stoker-runs.jsonl`.

### 6. Your ways

- One markdown file, `ways.md`, next to the config (`~/.config/matchblox/ways.md`). It is free-form, and matchblox only reads two conventions: `# Recipe: <name>` headings become the chips in the launch sheet, and the whole file goes to the stoker by path in every brief.
- First run: setup offers a door, **Write your ways**, which writes a starter file with three example standing rules: compact + `/review` when a PR goes up, hold non-priority work past 80% of the week, never touch a parked session. It also writes one example recipe (merge order). They're examples, and the user edits them.
- Martin's merge-order flow, his priority list and his window rules live in his own file, not in matchblox.

### 7. matchblox.com and the README replay

- Standing rule from Martin (2026-10-13): **every feature PR updates the landing page and the README replay to match.** Here, the replay world gains:
  - a quiet session with its idle reason, a parked row in HELD, and a `g` regroup;
  - an `n` launch sheet with an order and its plan;
  - the night card at work;
  - the morning card.
  Re-record desk 132x32, duo 104x38 and phone 44x31 (www/demo/frames-*.json, replay-*.svg, docs/assets/replay.svg).
- **The Duo fold must be visible.** Today the whole device is a tap target with no cue, and Martin found it only by chance. Add a visible two-state switch under the device, `Folded · Open`, that shows the current state and does the same as a tap. The tap on the device keeps working. The switch is keyboard-reachable and has a pressed state for screen readers.

### 8. Calls made in the spec (the builder check's questions)

- **Compaction stays the service's.** The deterministic compactor (`decide.go`) keeps running in every run, with or without an agent stoker. `matchblox compact` is a request to that same compactor, under its guards (30-minute rule, at rest, not waiting on a person). One owner, so the two never act on the same pane.
- **Tests.** The acceptance is automated: a scripted fake stoker agent (a shell script that runs `matchblox` commands), a fake `gh` on PATH and a fixture repository with 3 dependent PRs, run under `go test -race` on a private tmux socket. One manual live check with a real Claude Code stoker runs on a private socket before merge, never on Martin's sessions.
- **The merge guard.** PreToolUse gains a Bash matcher, and `matchblox hook` makes a synchronous call to the service for it (with a short timeout). It denies only in a pane marked as last driven by the stoker. That mark is also written to the state dir, so the hook fails closed for those panes when the service doesn't answer, and stays open everywhere else. Setup re-installs the hook on existing installs, as an update door. Codex and OpenCode have no such hook. In a run without the grant, the stoker never asks those sessions to merge, release or deploy, and the plan marks them `unguarded`.
- **`y` on a waiting step** grants one command of the kind the guard denied: one `gh pr merge <n>` (or the bare merge of that session, or one release or deploy) in that one pane, used once. A line with two guarded commands needs a go for each. matchblox puts that step on the card by itself and then wakes the stoker with "go given for <session>: <command>".
- **Take over.** While the stoker rests, matchblox wakes it with nothing and refuses its `send`, `hold` and `compact`. It gets one message when it rests and one when it is back on, listing what changed meanwhile.

## Not in this

- matchblox starts no agent and runs no model of its own. It needs no API key.
- No built-in flows: merge order, priorities and window sharing are ways, not code.
- #40 (hold at a usage-limit reset) stays its own issue. Its "hold" should reuse §1's HELD state, and nothing here blocks it.

## Done when

- `z`, `g` and the idle reasons work at all three widths, with tests for each idle-reason rule.
- A night launched from the phone with an order produces a plan in the sheet. Once started, it is carried out through the sessions, woken by matchblox, and the card shows each step and its outcome.
- On a test repository with a stack of 3 dependent PRs and a ways file holding a merge-order recipe, "set the ready ones up in merge order and merge them on green" lands all three without a person, or stops on the first red with why.
- Without the merge grant, the same run stops at each merge with `⏸ waits for you`, and the hook denies a session's `gh pr merge`.
- With no stoker pane configured, `n`/`f` behave as today and the sheet shows the door.
- The landing page and the README replay show the new screens, and the Duo switch is visible.
- docs: stoker.md, keys.md, config.md, a new ways.md. CHANGELOG entry.

## Left to the builder

- Exact idle-reason detection for "stuck" (which transcript signals count).
- The brief's wording and the wake message format.
- Whether `matchblox stoker plan` takes stdin or flags.
- How `e` on the stoker card opens ways.md (a tmux popup with $EDITOR is fine).
- How the group/sort choice is persisted.

## Revised 2026-10-14: no plan gate

Martin, after the first real run: the previous plan still showed, the stoker took far too long to rewrite its plan, and managing the stoker looked like work. "The plan can happen async, and it can maintain it itself, also we could benefit from some determinism." Rulings AY and AZ.

- **An order starts at once.** `n` and `f` open the launch sheet: the order's text line, the recipe chips and the go chips. `⏎` sends the order and the run is on from its first second: an order is `running`, then `done`. There is no `planning`, `planned` or go, no plan in the sheet, no "plan written · waits for your go", and no typed `go` or `drop the order`. `Esc` sends nothing.
- **The stoker keeps its own plan.** `matchblox stoker plan` replaces the plan on the card, with no approval and no restart. The card shows the latest plan, each step with its last logged state; a step keeps its number across plans. Steps logged under an older plan stay in the run log.
- **The gos are the builder's chips.** The sheet shows `merges`, `releases` and `deploys`, each off (ask me) by default, reached with `Tab` from the order line (`Tab` again or `Esc` goes back), where `←` `→` move and `Space` toggles one (Ruling BA: macOS types µ®∂ for Alt keys), or toggled with a tap. These alone are the run's goes; the agent reads no go from the order's words, and `asks:` and `grant:` lines in a plan are read and ignored. A night's chips are sealed with the night.
- **A denied command asks by itself.** With a go off, the guard denies a guarded command in a driven pane as before, and matchblox adds `⏸ waits for you: <session> wants to run <command>` to the card. `y` grants one command of that kind once in that pane (`merge:#<n>`, `merge:@<pane>` for a bare merge, or the kind), and the stoker hears "go given for <session>: <command>". No agent is involved. `merge:#<n>` and `merge:@<session>` remain only as these internal forms. The stoker can still ask for a go with `matchblox stoker step <n> --state waits --needs <go>` (Ruling BB): it only asks, and only the builder's `y` gives the go.
- **A new order by day is one line.** `o` opens the sheet with the run's chips and an empty line; `⏎` types `New order: <words>` into the stoker pane (a longer one goes to a file the stoker is told to read). No restart, no new brief, done steps stay done; the run's goes become the chips. The card quotes the latest order, with the earlier ones folded under it.
- **The brief starts fast.** It carries a snapshot: a line per session (name, word, idle reason, ship and stage, context %, plan %, parked or held), a line per ship no session drives, and each account's 5h and week, cut at forty session lines.
- **Triggers.** `matchblox stoker when --ship <n> --stage ci|green|merged|live|red <hand>` or `--rest <session> <hand>`, the hand being `send <session> "<text>"`, `compact <session>`, `release <session>` or `hold <session> --why "<text>"`. The service fires each once, as an act from the stoker's pane under the same guards, logs it to the card (`when #4573 merged → sent billed: …`, or failed with why) and tells the stoker at its next wake. Triggers wait while the builder has taken over, end with the run, and count toward `max_steps`. `--list` and `--drop <id>` manage them; the card shows those that wait under WAITING ON. A rest trigger needs a turn's end in the transcript, so it fires for Claude Code sessions only.
- **A new order shows only its own plan.** A run that ended is folded when a new order starts a new run: its card stays in `stoker-runs.jsonl`, and nothing of it mixes into the new order's sheet or card.

## Revised 2026-10-14: matchblox starts the stoker on its own tmux server

The builder asked that the stoker not sit among their own sessions ("a 'hidden' tmux, like agent manager does already, so we dont conflate them"). This replaces §2's "You start your stoker the way you start any session" and the `[stoker] pane` key.

- **matchblox starts the agent.** `f` or `n` with no stoker agent running starts it: `tmux -L matchblox new-session -d -s stoker -c ~ <agent>`, where `<agent>` is `[stoker] agent` (default `claude`). The sheet says `starting the stoker…`, and the order is typed once the agent is ready. It is started only by an act from the builder's console.
- **Out of sight.** The server is matchblox's own, so the agent is not in the builder's `tmux ls` or session switcher, and Sessions leaves it out (#81).
- **Looking at it.** `Enter` on the STOKER tab's agent row, or a tap on the header's stoker word, opens it in a `tmux display-popup` attached to that server; prefix `d` closes it. Outside tmux, the console names `tmux -L matchblox attach -t stoker`.
- **Off and quit.** `f` stops the run and leaves the agent resting, so its context carries to the next order. `Q` on the STOKER tab, with a typed `y` (or `matchblox stoker quit`), ends it.
- **Pane ids name their server.** `%0` on matchblox's server is not `%0` on the builder's: driven.json, the hook (from `TMUX` and `TMUX_PANE`) and the caller identity all carry the server, so no pane of the builder's is taken for the stoker's.
- **`[stoker] pane` is gone.** The feature was not released with it; a config that still has it gets a warning in `matchblox status` and is otherwise ignored.
