# The sidebar console

Issue: #411. Replaces the layout of the board (#372, spec `2026-11-10-the-board-design.md`). Holds #406 (#410).
Mockup: `design/0021-sidebar-console.html`, a copy of the owner's artifact "Sidebar console", version 10. Open it in a browser: it has the prototype, every branch (the buttons load each state), and the landing-page section. Its rows, lines and keys are the design; where this spec and the mockup differ, this spec wins.

## Why

The owner tested the board in v0.45.0 on 2026-11-14 and judged it worse than the old session list, "by a lot". To answer an agent they chose a card, pressed `⏎` and worked inside the pane. Chains were a small stub. `SHIPPING` made no sense to them. A held session looked idle. The setup steps, the new-session row and the tips were cramped. `s` did nothing because one host was hidden in Machine › Hosts, and nothing said why. An empty `NEEDS YOU` took space. On the phone the states were pages to swipe.

## Owner's decisions (2026-11-14)

- Sessions are grouped by state. Chains stay visible.
- The console is a list of sessions beside one chat, "like the Claude Code app, but in the terminal". Going into the pane is an extra step, not the main path.
- Typing in the chat is a line: `⏎` sends one message (owner chose it over keys straight to the pane).
- `s` for hosts stays. A hidden host must not block it, and `s` is never silent.
- A chain is one unit in the list. It sits in the group of its most urgent member, so the whole chain moves when one member changes state. Its members are joined by links (`├` `│` `└`), not by colour.
- The state icon is the match. The agent's mark sits at the end of the row.
- No rail. The list is open or hidden.
- An empty section takes no row.
- The Jobs pass of the mockup (version 10) is the design: "lgtm".
- #410 (the stacked board) is on hold.
- What the board added and this console replaces goes: the columns, hiding a column, `←` `→` between columns, the peek and the live view, the strip (`u`), and `v`. `⏎` into the pane becomes `⌃o` expand. The chain room lives on as `g`. Code, docs and tests of the removed parts go with them, each removal its own commit.
- The chat is the agent's real pane, cut to its conversation, with matchblox's line under it. No parser of an agent's chat.
- Hidden means "left out of all hosts", and nothing more (owner: "hidden means hidden… Steve Jobs this"). See "Hosts".

## Non-goals

- No change to `matchblox sessions --json`, `peek` or `send`.
- No new agent integration, and no parser of an agent's chat.
- No edit of `www/` by this work: matchbloxrelease owns it and gets a brief (see "What else changes").

## Anti-goals

The work fails, even if it runs, when:

- a wait is not on the screen, or not counted, while the owner looks at the console;
- a message the owner typed reaches a session other than the one the line names;
- a key or a tap does nothing, with no word why;
- the owner must tick or set anything before the console shows the hosts they did not hide;
- an empty section takes a row.

## The design

### The screen (desk)

Top to bottom: the header, the body, the key line.

- **The header**: `matchblox`, then `● N need you` (every host, every chain; `✓ nothing needs you` at zero), then the hosts at the right (`ws-1 · gpu-1  s ▸`), then any notice. With the list hidden, the header also names each session that needs you; a tap on a name opens its chat.
- **The body**: the list at the left (40 columns), the chat at the right.
- **The key line**: where the keys go first (`writing to flint-auth` or `in the list`), then at most five keys for that place.

### The list

Groups, in this order, each with a head `NEEDS YOU · 4  −`, and a thin line between groups: `NEEDS YOU`, `WORKING`, `HELD`, `IDLE`. A group with no session has no head and no row.

- **A row** is one line: the selection mark `▌`, the match, the name (`@host` for another host when all hosts show), the wait or the pull request's badge (`#4551 CI 6/9`) at the right, then the agent's mark (`✻` `❋` `⬢`). A row has a second line only when its session needs the owner (what it asks) or it is the chosen row.
- **The match** is the state: struck and red when it needs the owner, burning while it works, unlit (`╽`) when held, burnt out (`╵`) when idle.
- **A chain** is a head row `≋ login   step 2 of 3 · 3`, then its members in step order, each a row with a link (`├`, the last `└`, and `│` under a two-line row). The chain sits in the group of its most urgent member (needs you, then works, then held, then idle). All its members draw under its head there, and nowhere else. Each group's count counts only the sessions in that state. In its group, the chain takes the place its most urgent member would take by the group's order (my call). The links light when the chosen session is in that chain. A tap on the head chooses its most urgent member.
- **Held**: `held by the stoker · not the priority`, or `parked by you · z lets it go`.
- **Pull requests**: no `SHIPPING` group. A PR is a badge on its row. A PR that waits for the owner's merge puts its row in `NEEDS YOU`.
- **The foot**: one line, `+ new · ✧ 2 tips · ! 1 setup`. The setup part shows only while a step is open. Each opens in the chat's place.
- Order inside a group does not change while the states do not (the board's rule).

### The chat

- A head line: the name, `@host`, the agent, its state, the PR badge, `ctx N%`, and `⌃o expand · m menu`.
- For a session in a chain, one line: `chain login · step 2 of 3   kindle-web done → flint-auth needs you → flint-ui works`. No line for a session in no chain.
- The conversation, then the question with its options when it asks.
- The line at the bottom: `› …   ⏎ send to flint-auth`.

### Where the keys go

Two places. The key line names the one that has them.

- **The line** (the default): choosing a session puts the keys here. Letters type; `⏎` sends; a paste goes in as one piece. With the line empty: a digit marks an option and the same digit sends it; `↑` `↓` choose the session above or below in the list's order. `esc` clears a mark, then moves the keys to the list. The board's guards against a stray digit carry over (my call). A digit with no question on screen types into the line. Options past 9 are answered by typing (my call).
- **The list**: `↑` `↓` choose; `⏎` goes back to the line; `s` next host; `g` only this chain (with the stoker's steps over the chat); `/` search; `m` the menu; `c` copy the last message; `z` park or let go; `n` a new session; `⇥` Machine. Any other letter goes back to the line and is typed there.
- **Always**: one key hides or shows the list (`⌃b` in the mockup; the builder picks a key that is not tmux's prefix, my call). `⌃o` expands into the agent's own pane, in place of today's `⏎` into the pane, and `⌃o` again comes back, so `esc` reaches the agent inside.

### Hosts

One rule: a hidden host is left out of "all hosts", and nothing more.

- **All hosts** shows every host the owner did not hide, with no ticking. `● N need you` counts the hosts in view.
- **`s`** steps through every host, hidden ones too, then all hosts. On a hidden host the list shows its sessions, and the header says `gpu-1 · hidden from all hosts · h shows it`. `h` there unhides it. Machine › Hosts still ticks and unticks, as today.
- **One host unhidden**: "all hosts" is that host, so `s` does not stop on it twice: it steps from host to host.
- **A host that does not answer** keeps its rows, dimmed, and the header says `gpu-1 not reached · since 2m`. A message to it stays in the line, `not sent: gpu-1 is not reached`, until the owner sends it again or clears it. Nothing retries by itself.

### Messages and their outcomes

- To a waiting session: sent; the chat shows it.
- To a working session: queued for its next turn, and the chat says so. Where an agent takes typed input while busy, matchblox sends it at once; where it does not, matchblox holds it and sends it when the turn ends. The builder finds which agent does which (my call).
- To a parked session: the line is off and says `parked: z lets it go`.
- To a session the stoker holds: `1` releases it, from its question.
- The service lost: rows dim, the header says `no service · it redials`, and nothing is sent.

### Copy and paste

- Paste: the terminal's paste reaches the line as one piece, and the line sends it to the session through its host. This works when the console runs over ssh and the session is on a peer.
- Copy: `c` and the menu copy through the console's `clip` (OSC 52 when the host gives no clip), so the text reaches the terminal the owner holds, through ssh and tmux. The setup step "Let copy through" covers tmux. A drag in the chat selects chat text only.

### States

- Loading: the list says `reading your matches…`; the chat waits.
- No sessions: the chat says `No sessions yet. + new session starts one here (n). matchblox setup finds the agents you already run.`
- A search with no match: `Nothing matches "x". esc clears the search.`

### The phone

The list is the first screen, full width, the same rows. A tap opens the chat; `◂` comes back. No sideways pages. Pocket's belt and long-press keep their jobs.

### Matchblox Pocket

The app draws the console, so it gets the sidebar console. It changes where the board taught it pages:

- The belt's Board layout follows the new keys: `◂` (back to the list), the chosen session's answers (`1` `2` `3`), `↑ ↓ ⏎`, then `⌃o` expand, `s` host, `g` chain, `/`, `m`, `c`, `z`, `n`. The column keys (`←` `→` between pages) go.
- A sideways swipe no longer turns pages. A tap opens a chat; `◂` or the edge swipe comes back. A long-press still opens the menu.
- The app's text box under the console types into the chat line, and a paste from iOS reaches the line as one piece.
- The App Store text and its screenshots follow the new console (with the landing page, below).

### What else changes

- **The landing page.** After the owner approves the built console, matchbloxrelease gets the brief: the words "Every agent. One chat." and "What needs you is on top. Choose it, and you are already in its chat.", the stills in the mockup's landing section, and a new desk and phone replay in `www/demo/`. Until then the feat PR carries `Landing page: unchanged because …`, with the reason that the page waits for the owner's approval of the built console.
- **The README and the docs.** `README.md`, `docs/console.md`, `docs/keys.md`, `docs/start.md`, `docs/phone.md`, `docs/remote.md` (hidden hosts), `docs/index.md`, and every other doc that names the board's columns, `SHIPPING`, the peek or the live view, say the new console in ASD-STE100. `CHANGELOG.md` names it under Unreleased.
- **The replay and the demo.** The replay tests (`TestReplay`) and their frames follow the new screen at every size they draw.

### What stays from the board

`● N need you`; answering in two presses; the fixed order inside a group; `s` `g` `/` `z` `m` `c`; the menu; the stoker's card and talk line in `g`; Machine as its own tab.

## Left to the builder

- Where each agent's pane is cut to its conversation, and how much history the chat shows.
- The list's width at sizes other than the 16-inch desk, and when the list starts hidden.
- The key that hides the list, and the key for expand if `⌃o` clashes with an agent.

## Calls to check

- **Where a wait inside a chain counts.** I recommend that `● N` and the group count only the sessions in that state, as drawn. If wrong, a chain's count looks smaller than its rows.

## Done when

- At 160 columns, the owner chooses `flint-auth`, presses `2` twice, and the answer reaches it, with no `⏎` into the pane.
- A chain with members in three states sits in `NEEDS YOU` as one unit with links, and moves to `WORKING` when its wait is answered.
- With gpu-1 hidden, all hosts shows ws-1 only; `s` reaches gpu-1, the header says it is hidden, and `h` shows it in all hosts again.
- With nothing waiting, no `NEEDS YOU` head shows and the header says `✓ nothing needs you`.
- `⌃b` hides the list, and the header names each wait.
- On a phone, a tap opens a chat and `◂` returns.
- Over ssh to a host whose console reaches a peer, a paste reaches the peer's session, and `c` puts its last message on the laptop's clipboard.
- In Pocket, the belt has the new keys, a tap opens a chat, and the text box types into its line.
- No doc and no README line names the board's columns, `SHIPPING`, the peek or the live view.
