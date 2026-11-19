# The owner drags over an agent's words and pastes exactly those words, on any screen

## Who, and the moment

The owner works at a desk. Two cases: the console runs on this host, or the owner SSHes in with `matchblox <host>`. The list sits at the left and an agent's pane at the right.
The owner wants a command, a path or a reply that the agent wrote. They drag over it with the mouse and paste it elsewhere.
Evidence: the owner, 2026-11-18: "if i copy with my mouse it goes multiline and copies the sidebar as well". "I basically want full copy/paste capability, on every device every harness everywhere, no crappy select from the sidebar, it must automatically trim."

## Why it matters

Today a drag in the agent pane copies nothing. The owner falls back to the terminal's own selection. That selection takes the sidebar's columns, padding spaces and hard line breaks. The owner cleans every paste by hand (estimated: every copy, many times a day).
Measured on this host, 2026-11-18:
- The server's `set-clipboard` is `external`, which drops every application's OSC 52. In a test on a private tmux 3.4 server, an app's OSC 52 left no buffer under `external` and one buffer under `on`.
- Claude Code reads all mouse motion (`mouse_all_flag=1`). So the jump table (`internal/remote/jump.go:128`) gives it the drag, and its copy is then dropped.

## The experience

1. The owner drags over text in the agent pane. tmux highlights only cells of that pane. The highlight stops at the pane's edge.
2. They let go. The selection lands on the clipboard of the device in front of them: tmux makes a buffer, matchblox's service sees it, and the console they look at puts it on their terminal's clipboard. A copy on a host travels over matchblox's own SSH link. The highlight clears. A double-click copies a word, a triple-click a line.
3. They paste. The paste holds the selected words only. It has no sidebar columns and no padding spaces at line ends. A line that wrapped on screen pastes as one line.
4. The same works in every harness (Claude Code, codex, opencode, cursor-agent) and in the list pane's own text.
5. A click and the wheel still reach the agent. A click arrives whole when the button comes up; a drag never clicks anything in the agent.
6. A link the agent prints works where a link works on that screen. On a desktop terminal it opens the way the terminal opens links (for example cmd-click), and a long link that wraps opens whole.

Ways it goes wrong:
- The copy cannot reach a terminal (no console is attached, or the link to the desk is down). tmux's message line says `not copied: <cause> · <fix>`, and the console does not claim `copied`.
- A hop the console cannot read drops the copy. "Check that copy reaches you" proves the whole way once per screen, and the existing words name the hop (docs/console.md, "Copy").

## Non-goals

- The terminal's own Shift-drag or Option-drag selection. It spans both panes, and no program can change that. The docs say: press ctrl+q to show one pane first.
- The phone. matchblox-board owns tap-a-link, the phone's OSC 52 acceptance and `c copy reply` on branch feat/phone-console-controls. This work must not break it.
- A copy view or a pager key.
- Changes to the agents themselves.

## Anti-goals (failure even if it works)

- A paste holds the sidebar's text, the pane border or trailing padding spaces.
- A drag in one agent's pane selects text of another pane.
- The console says `copied` when a hop dropped the copy.
- A click or the wheel no longer reaches the agent: menus, scrolling and the `@` file picker break.
- Turning `set-clipboard on` lets a program read the owner's clipboard. Only writes may pass.

## Surfaces

- An agent's tools: no tool is touched; agent output reaches the owner's clipboard only through a tmux buffer the owner's own drag makes. Harm to avoid: a pane or a host reads or writes the clipboard by escape codes. `set-clipboard` stays as the owner set it (under `on` a pane can read the last copy: measured, see probes), and no pane gets passthrough.
- Checked, not touched: authentication, money, uploads, webhooks, secrets, CI, personal data.

## Rulings

- 2026-11-19 — Superseded by R5 and R7 below (owner, D3 and D7): matchblox changes no `set-clipboard` value; copies travel through tmux buffers and matchblox's own link. — the owner.
- (superseded) 2026-11-18 — The console sets `set-clipboard on` on its own tmux server at every start, with no door to confirm. This replaces the "Let copy through" door of docs/superpowers/specs/2026-11-10-the-board-design.md:251 — because the owner wants copy to work with no setup. — the owner.
- 2026-11-18 — tmux owns every drag in an agent pane, in every harness, even for an agent that reads the mouse. The ways: (a) tmux owns every drag: the same in every harness, wraps join, trailing spaces trim, but Claude Code's own drag-select is bypassed; (b) each agent's own selection: no change of feel, but it differs per harness and Claude Code's breaks stay hard; (c) a copy-view key: an extra step on each copy. Pick (a) — because the owner wants one behaviour everywhere. — the owner.
- 2026-11-18 — Tap-a-link and copy on the phone stay with matchblox-board's branch — because that work is built and waits for the owner's push. — proposed, not confirmed.

- 2026-11-20 — A host's copy reaches the clipboard only while the console shows that host (beside the list, or in a jump to it) and the person's screen had input in the last 2 s; each says `copied: N characters from <host>`. Every copy loses its control codes, and a client in an agent's pane gets none (checked at each copy). — because a program on a host could otherwise put text on the desk clipboard with `tmux set-buffer` (paste-jacking), which a remote host cannot do today; found by the security review of the build. Remaining risk: a host program that repeats set-buffer while the person works in that host's pane. — the owner.
- 2026-11-19: spec approved. Decided by: the owner. (U1 and U2 stay open; the plan keeps them out of scope.)

## Left to the builder

- Where the console sets the option: the start-up code next to `allow-passthrough` in `cmd/matchblox/graphics.go`, or the model.
- How the jump key table and the console's own window bind `MouseDrag1Pane`, `MouseDragEnd1Pane` and the double-click and triple-click word/line selects.
- What happens to the "Let copy through" door: delete it, or keep it only for a tmux that refuses the option.
- How tests prove each hop: a private tmux server per test, as in `internal/tmuxtest`.

## Calls to check

- Does the console put `set-clipboard` back to its old value when it ends? Recommendation: no — because other consoles and sessions share the server, and a value put back would break their copy again. Cost if wrong: one line in the owner's tmux stays changed after matchblox is gone. The docs say so.
- Claude Code in full-screen mode draws its own line breaks, so tmux sees no wrap there. Recommendation: accept this for now, then measure it in the build with a long line from Claude Code. If the breaks stay hard, file an issue upstream and do not rewrite the copied text. Cost if wrong: a pasted paragraph from Claude Code keeps breaks at the pane width.
- A drag on the phone. matchblox-board's phone accepts OSC 52 only within 3 s of the belt's copy-reply tap or a typed key, so a copy from a phone drag or long-press is ignored. Recommendation: leave this to matchblox-board's open call in its PR. Opening that gate on a drag is a phone change. Cost if wrong: on the phone, only `c copy reply` copies, and a drag does not.
- A link that wraps on a desktop terminal. Recommendation: pass OSC 8 hyperlinks on through tmux (`terminal-features ...:hyperlinks`), so the link opens whole wherever the agent marks it. Cost if wrong: a plain, unmarked long link opens cut in two, as today.

## Done when

- [ ] On this host, in the console beside view, a drag over three lines of Claude Code's reply pastes in another app as those three lines: no sidebar text, no `│`, no trailing spaces.
- [ ] The same drag works in a codex pane and a cursor-agent pane.
- [ ] Over SSH from the desktop (`matchblox <host>`), the same drag pastes on the desktop.
- [ ] After a console start and a host jump, `tmux show -sv set-clipboard` prints the same value as before on both servers, and a pane's OSC 52 read query gets no reply.
- [ ] A drag in an opencode pane pastes the selected text (opencode's own drawn borders are a known limit, see Unresolved decisions).
- [ ] A click on a Claude Code menu item and the wheel in its pane still work.
- [ ] A long `https://` link that the agent prints opens whole on a cmd-click in the desktop terminal.
- [ ] "Check that copy reaches you" passes on the desktop, both local and over SSH.

---

# Eng review (/plan-eng-review, 2026-11-19)

Target: this spec, docs/superpowers/specs/2026-11-18-copy-one-pane-design.md, as written on 2026-11-18 (the text above this line is the unchanged plan).

## Probes run (bounded, current behaviour, tmux 3.4, private servers)

| Probe | external | on |
|---|---|---|
| App OSC 52 in a pane makes a tmux buffer | no buffer | buffer |
| App OSC 52 in the agent pane, through a nested view client, reaches the outer terminal | 0 OSC 52 seen | 1 OSC 52 seen, 2 buffers |
| tmux copy-mode copy in the agent pane, through the nested view, reaches the outer terminal | buffer, 0 OSC 52 seen | 2 buffers, 1 OSC 52 seen |
| App OSC 52 read query (`52;c;?`) gets a reply | no reply | no reply (3.4 has no get-clipboard option) |
| tmux copy of `abc     ` and of a soft-wrapped line | — | `abc` (trailing spaces gone); the wrapped line is one line; a hard break stays |

## Scope Challenge

- What exists: `paneMouse` (internal/remote/jump.go:131) is the one place for the jump and view mouse binds. `ViewArgv` (internal/remote/view.go:63) and `Jump.Remote` (jump.go:165) and the host jump list (jump.go:258) all use it. `tmuxPassesClipboard` (internal/app/copy.go:46) reads the option. The door `CopyThrough` (internal/doors/doors.go:162) sets it. The list's own drag copy is `clip` (copy.go:35).
- Complexity: about 6 non-test files (jump.go, copy.go, doors.go, the cmd start-up, docs/console.md, CHANGELOG.md), 0 new types. Under the 8-file gate: no structure question.
- TODOS.md: none in the repo.
- Search: no new pattern; tmux options only. Search skipped (Aside not probed; known tmux behaviour was probed directly instead).
- Learnings: unavailable (bun not on PATH).

Findings:
1. [P1] (confidence 9) Factual correction: under `external` the nested view drops even tmux's own copy-mode copy (probe row 3). The ruling to set `on` fixes both. No question; an integration test through a nested client becomes required proof.
2. [P1] (confidence 8) internal/remote/jump.go:155-175 `Remote()` sets `mouse on` and key tables on the host's tmux but no `set-clipboard`. A `matchblox <host>` jump copies inside the host's tmux, which the console on the desk does not own. Spec ruling 1 says "its own tmux server" only. → R1.
3. [P2] (confidence 5) tmux 3.5 adds `get-clipboard`. The spec's surface line ("tmux answers no OSC 52 read query") is proven only on 3.4; a desk running brew tmux 3.5+ is unproven. → R2.
4. [P2] (confidence 6) jump.go:134 `MouseDown1Pane ... select-pane -t = ; send-keys -M` gives the agent the press. If the drag then goes to copy mode, the agent never gets the release. Claude Code reads all motion (`mouse_all_flag=1`) and may keep a held button. Unverified. → R3 (regression contract).
5. [P3] (confidence 8) jump.go:131 binds no `DoubleClick1Pane` / `TripleClick1Pane`. A session key-table hides root, so word and line select do nothing in a jump or a view. → R4.

## Decision ledger

### R1: the host's tmux also gets set-clipboard on
Finding: 2, P1, confidence 8, internal/remote/jump.go:155, reviewer: Claude (native)
Plan baseline: ruling 1 sets `on` on the console's own tmux server only (spec, Rulings).
Runtime evidence: Remote() sets mouse and key tables on the host, no clipboard option (jump.go:155-175, :248-262).
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R1 host set-clipboard | not set by matchblox | jump list sets `on` on the host server | not set; docs tell the owner to set it |
| Console own server `on` | approved (ruling 1) | unchanged | unchanged |
| R2, R3, R4 | pending | pending | pending |
Question D1:
D1 — Set set-clipboard on on the host too?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: When you open a host with `matchblox <host>`, the agent runs inside the host's own tmux. The console on your desk sets the clipboard option only on its own tmux. So a copy on the host is still dropped there, before it ever reaches SSH.
Stakes if we pick wrong: copy over SSH stays broken, the exact case you named.
Recommendation: A because the jump already sets `mouse on` on the host the same way; one more option in the same list.
Completeness: A=10/10, B=6/10
Pros / cons:
A) Jump sets it on the host (recommended)
  ✅ Copy works on every host you open, with no setup on each host
  ✅ One more command in the list the jump already sends (jump.go:248)
  ❌ Changes a server-wide option on each host you open, and it stays on
B) Docs only
  ✅ matchblox changes nothing on a host beyond what it does today
  ❌ Every host needs a hand edit, and copy fails silently until then
Net: one line on each host buys working SSH copy everywhere.
Header: Host clipboard
Options:
A) Jump sets it on host (recommended)
The jump's tmux list sets `set -s set-clipboard on` on the host, as it already sets mouse on. Effort human ~30 min / CC ~5 min, plus a jump_test row. Risk: a server-wide option on each host stays on.
B) Docs only
The docs tell the owner to put `set -s set-clipboard on` in each host's tmux.conf. Effort ~10 min. Risk: copy over SSH fails until each host is edited.

State: superseded by R5 (D3)
Actual answer: A) Jump sets it on host (recommended), owner, D1 2026-11-19
Accepted scope: the jump's tmux list (Jump.Remote, the host jump list at jump.go:248, and so ViewArgv) sets `set -s set-clipboard on` on the server it runs on; a jump_test row asserts it.
History: approved D1 2026-11-19 (host `on`); replaced by R5 B, D3 2026-11-19: the jump sets the matchblox copy command on the host instead of `on`.

### R2: pin get-clipboard off where tmux has it
Finding: 3, P2, confidence 5, spec Surfaces line, reviewer: Claude (native)
Plan baseline: spec Surfaces says tmux 3.4 answers no OSC 52 read query and the console asks for none.
Runtime evidence: tmux 3.4 here has no `get-clipboard` option (`invalid option`), and a read query got no reply under external or on (probe row 4). tmux 3.5+ behaviour: unknown, not probed.
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R2 get-clipboard | not set | `set -sq get-clipboard off` on the console server and the host; `-q` makes tmux 3.4 skip the unknown name | not set; the surface line names 3.4 only |
| R1 host set-clipboard on | approved D1 | unchanged | unchanged |
| R3, R4 | pending | pending | pending |
Question D2:
D2 — Turn off clipboard reads on newer tmux?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: Turning the clipboard on lets programs write to it. On this host's tmux (3.4) no program can read it back: I tested that. Newer tmux (3.5, common from brew on a Mac) has a separate `get-clipboard` setting, which I could not test here. If that setting answers reads, any program in any pane could read what you last copied.
Stakes if we pick wrong: a password you copied could be read by a program running in some other pane.
Recommendation: A because one more option, ignored where it does not exist, makes the no-read promise true on every tmux.
Completeness: A=10/10, B=6/10
Pros / cons:
A) Pin get-clipboard off (recommended)
  ✅ The no-read promise holds on every tmux version, not only 3.4
  ✅ Same command list as set-clipboard; `-q` makes tmux 3.4 skip the unknown name
  ❌ Overrides a setting the owner might have chosen on purpose on a newer tmux
B) Leave it
  ✅ No extra option and no extra error to ignore on 3.4
  ❌ Reads on tmux 3.5+ stay unproven, and the Surfaces line must say so
Net: a one-line guard against a clipboard leak we cannot rule out.
Header: Clipboard reads
Options:
A) Pin get-clipboard off (recommended)
The console and the jump also run `set -sq get-clipboard off`; `-q` keeps tmux 3.4 quiet about the unknown name, so the rest of the jump's command list still runs. Effort human ~30 min / CC ~5 min, tests in the same rows as R1. Risk: overrides an owner's own get-clipboard choice.
B) Leave it
Set nothing; the Surfaces line says the no-read proof covers tmux 3.4 only. Effort ~5 min. Risk: on tmux 3.5+ a pane may read the last copy.

State: superseded (no remedy)
Actual answer: owner, D2 2026-11-19: "isnt there a better way to solve this" (no option picked)
Accepted scope: none. R2's premise is false: tmux 3.4, 3.5a and 3.6 have no get-clipboard option. In input.c input_osc_52 the only gate is `set-clipboard` == on; under on a `52;c;?` query gets the top paste buffer. Re-probe (python raw reader, tmux 3.4): external -> no reply; on -> `ESC]52;;c2VjcmV0LW9u BEL` ("secret-on"). The first read probe was wrong (the shell's cat missed the reply). Replaced by R5.
History: D2 asked "Pin get-clipboard off" vs "Leave it" on a false premise.

### R5: how a copy reaches the clipboard (reopens spec ruling 1)
Finding: 6 (new), P1, confidence 10, tmux input.c input_osc_52 (3.4/3.5a/3.6) + probe, reviewer: Claude (native)
Plan baseline: spec ruling 1 (owner, 2026-11-18): the console sets `set-clipboard on` at every start. R1 (owner, D1): the jump sets it on each host too.
Runtime evidence: under `on`, any program in any pane writes the clipboard and reads tmux's last copy by OSC 52 (probe above). Under `external`, no pane can do either; tmux's own copy still reaches a directly attached terminal, but a nested view client or an SSH pane in a desk tmux drops it (probe rows 2-3).
Reason to reopen: a concrete new risk the owner did not have on 2026-11-18. A host you SSH into from a pane, or a file you `cat` that holds the query, gets your last copy typed back into that pane. A program running as you can already run `tmux show-buffer`, so the new exposure is remote hosts and untrusted text.
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R5 clipboard path | ruling 1 + R1: `on` on console server and hosts | keep `on` (as ruled), docs name the read exposure | keep each tmux's own value; matchblox carries its own copies: tmux's drag copy runs a matchblox copy command that writes to the person's real terminal (`load-buffer -w` to the real client; over SSH, OSC 52 wrapped for the desk tmux's passthrough, on the console's own panes only) |
| An agent's own copy (Claude Code /copy, codex) | dropped | reaches the clipboard | still dropped |
| The console list's `c` | dropped | reaches the clipboard | reaches it (through tmux, not OSC 52) |
| Remote host / untrusted text reads the last copy | no | yes | no |
| R1 host `on` | approved D1 | unchanged | replaced: the jump sets the copy command, not `on` |
| R3, R4 | pending | pending | pending |
Question D3:
D3 — Turn set-clipboard on, or have matchblox carry its own copies?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: I tested it. With tmux's clipboard switch `on`, any program in any pane can write your clipboard and also read back the last thing you copied. That includes a server you SSH into from a pane, and a file you print that contains the right escape code. tmux has no separate switch for reads. The other way is to leave tmux's switch alone and have matchblox hand each mouse copy to your real screen itself. That is safer, but copy commands inside an agent (Claude Code's /copy) keep failing.
Stakes if we pick wrong: either a remote host can read your last copy, or agent copy commands stay broken and the build takes about 3x longer.
Recommendation: B because your mouse-drag copy is the thing you asked for, and B gives it to you without letting any pane read your clipboard.
Completeness: A=8/10, B=9/10
Pros / cons:
A) Turn set-clipboard on, as ruled
  ✅ Every copy works, including an agent's own copy command
  ✅ Smallest build: two option lines plus the drag binds
  ❌ Any pane, including a remote host you SSH into, can read your last copy
B) matchblox carries its copies (recommended)
  ✅ No pane can read or write your clipboard; your tmux setting stays yours
  ✅ Mouse drags and the console's `c` reach the clipboard on desk and over SSH
  ❌ Agent copy commands (Claude Code /copy) stay dropped; about 3x the build
Net: convenience for agent copy commands versus a clipboard no pane can read.
Header: Copy path
Options:
A) Turn set-clipboard on, as ruled
Keep ruling 1 and R1: `set-clipboard on` on the console server and each host. The docs name the read exposure. Effort human ~0.5 day / CC ~30 min. Risk: any pane, including a remote host, reads the last copy.
B) matchblox carries its copies (recommended)
Leave set-clipboard as the owner has it. tmux's drag copy pipes to a matchblox copy command that writes to the person's real terminal: load-buffer -w to the real client, or OSC 52 through passthrough on the console's own SSH pane. Effort human ~2 days / CC ~1.5 h. Risk: agent copy commands stay dropped; more moving parts.

State: approved
Actual answer: B) matchblox carries its copies (recommended), owner, D3 2026-11-19
Accepted scope: matchblox sets no `set-clipboard` value on any server; each tmux keeps the owner's value. tmux's drag copy (copy-pipe-and-cancel, which pipes to tmux's `copy-command` when it has no command) runs a matchblox copy command. That command writes the selection to the person's real terminal: `load-buffer -w` to the real client (a client whose tty is no pane's tty), or, inside an SSH jump, OSC 52 wrapped for passthrough, which the desk console allows on its own panes only. The console list's `c` goes through the same path, not tea.SetClipboard. Agent copy commands stay dropped, and the docs say so. R1 is replaced: the jump sets the copy command on the host, not `on`. Spec ruling 1 is superseded by this answer.
History: none

### R3: an agent never keeps a held button after a drag (regression contract)
Finding: 4, P2, confidence 6, internal/remote/jump.go:134, reviewer: Claude (native)
Plan baseline: spec ruling 2 (owner): tmux owns every drag in every harness; spec anti-goal: a click or the wheel no longer reaching the agent is a failure. No contract for the press the agent already got.
Runtime evidence: jump.go:134 `{"bind-key", "-T", table, "MouseDown1Pane", "if", "-F", "1", "select-pane -t = ; send-keys -M", ""}` sends the press to the agent. A drag then enters copy mode (owner ruling), and the copy-mode table takes the release, so the agent never sees it. What Claude Code (mouse_all_flag=1), codex, opencode and cursor-agent do with a press and no release: unknown, not probed.
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R3 held-press contract | none | the agent pane gets a release before copy mode starts; a test app in a private tmux records a press then a release for each drag; live check in the four agents | no synthetic release; a live check in the four agents only, a finding if one sticks |
| Press and wheel go to the agent | yes (jump.go:134, :133) | unchanged | unchanged |
| R5 copy path | approved D3 (B) | unchanged | unchanged |
| R4 | pending | pending | pending |
Question D4:
D4 — Send the agent a release when a drag turns into a tmux copy?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: When you press the mouse in an agent's pane, the agent gets the press, so clicks keep working. If you then drag, tmux takes over to select text, and the agent never hears you let go. Some programs then act as if the button is still held: they select or drag things on their own the next time the mouse moves.
Stakes if we pick wrong: after a copy, Claude Code's screen could misbehave until you click again, which breaks "clicks still work".
Recommendation: A because a recorded test makes "the agent always gets a release" a rule that holds for every agent, not a hope.
Completeness: A=10/10, B=7/10
Pros / cons:
A) Send a release, test it (recommended)
  ✅ No agent can be left thinking the button is down, whatever agent it is
  ✅ A test app in a private tmux proves press and release pair up on every drag
  ❌ A hand-made mouse sequence in the drag binding is one more thing to keep right
B) Live check only
  ✅ No extra mouse sequence; the binds stay tmux's defaults plus copy mode
  ❌ A new agent or a new agent version can break it, and no test sees it
Net: a small synthetic event buys a rule a test can enforce.
Header: Held press
Options:
A) Send a release, test it (recommended)
On a drag in the agent pane, the bind sends the pane an SGR release at the press position, then starts copy mode. A tmuxtest app records the events: each drag gives one press and one release. A live check runs in Claude Code, codex, opencode and cursor-agent. Effort human ~0.5 day / CC ~30 min. Risk: one hand-made escape sequence to keep right.
B) Live check only
No synthetic release. The build checks the four agents by hand and files a finding if one sticks. Effort human ~1 h / CC ~10 min. Risk: a future agent version breaks it unseen.

State: superseded by R3 (reopened), D8
Actual answer: A) Send a release, test it (recommended), owner, D4 2026-11-19
Accepted scope: on a drag in a pane whose program reads the mouse, the bind sends that pane an SGR release at the press position, then starts copy mode. A tmuxtest app records mouse events: each drag gives exactly one press and one release, and a plain click still gives press + release. Live check in Claude Code, codex, opencode and cursor-agent.
History: none

### R4: double-click and triple-click select a word and a line in a jump and a view
Finding: 5, P3, confidence 8, internal/remote/jump.go:131, reviewer: Claude (native)
Plan baseline: spec "Left to the builder" names the double-click and triple-click binds; no behaviour approved.
Runtime evidence: paneMouse (jump.go:131-139) binds WheelUpPane, MouseDown1Pane, MouseDrag1Pane, MouseDrag1Border only. A session key-table hides root, so tmux's default DoubleClick1Pane / TripleClick1Pane do nothing in a jump or a view.
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R4 word/line select | nothing in a jump or a view | double-click selects and copies a word, triple-click a line, through the R5 copy path, in every harness | nothing; drags only |
| R3 release contract | approved D4 | also covers these clicks | unchanged |
| R5 copy path | approved D3 | unchanged | unchanged |
Question D5:
D5 — Double-click copies a word, triple-click a line?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: In a normal tmux, double-click picks a word and triple-click a line, and both copy. Inside a matchblox jump or the agent view these do nothing today, because matchblox's key table hides tmux's defaults. A path or a URL is usually one word, so double-click is the fastest way to copy one.
Stakes if we pick wrong: copying a path keeps needing a careful drag.
Recommendation: A because it is two more bind lines on the same table and the same copy path, and it matches what every terminal does.
Completeness: A=10/10, B=7/10
Pros / cons:
A) Add word and line select (recommended)
  ✅ Double-click copies a path, a URL or a hash in one move, in every harness
  ✅ Same binds as tmux's own defaults, in the table that hides them
  ❌ A double-click no longer reaches an agent that wanted it
B) Drags only
  ✅ Agents keep getting double-clicks as they do now
  ❌ Copying a single word needs a precise drag, the slow way
Net: two bind lines turn the commonest copy into one gesture.
Header: Word select
Options:
A) Add word and line select (recommended)
paneMouse adds DoubleClick1Pane (select-word) and TripleClick1Pane (select-line), both copying through the R5 path and sending the R3 release first. Tests: a jump_test row for the binds, and the tmuxtest copy checks a word. Effort human ~1 h / CC ~10 min. Risk: an agent no longer receives double-clicks.
B) Drags only
No new binds. Effort none. Risk: copying one word stays a precise drag.

State: approved
Actual answer: A) Add word and line select (recommended), owner, D5 2026-11-19
Accepted scope: paneMouse adds DoubleClick1Pane (select-word) and TripleClick1Pane (select-line), both copying through the R5 path and sending the R3 release first. A jump_test row asserts the binds; the tmuxtest copy test checks a word.
History: none

### R6: an owner's own tmux copy-command
Finding: 7 (Section 1), P2, confidence 7, R5 accepted scope (copy-command), reviewer: Claude (native)
Plan baseline: R5 B (D3): tmux's drag copy pipes to a matchblox copy command through tmux's `copy-command` option.
Runtime evidence: `copy-command` is a server option, empty on this host (`tmux show -sv copy-command` prints nothing). The default MouseDragEnd1Pane is `send-keys -X copy-pipe-and-cancel` with no command, so it runs copy-command (tmux 3.4, -f /dev/null). An owner who set copy-command (for example to xclip or pbcopy) would lose it if matchblox overwrites it.
Comparison grid:
| Choice | Current | A | B |
|---|---|---|---|
| R6 owner's copy-command | not touched | matchblox keeps the owner's command and runs it after its own, with the same text | matchblox replaces it |
| R5 copy path | approved D3 | unchanged | unchanged |
Question D6:
D6 — Keep an owner's own tmux copy command running?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: tmux lets you name one command that every mouse copy runs. matchblox will use that slot to carry the copy to your screen. Your tmux leaves it empty today, but on another machine someone may already have xclip or pbcopy there.
Stakes if we pick wrong: on a machine with its own copy command, that command silently stops running.
Recommendation: A because nothing the owner set stops working, at the cost of a few lines.
Completeness: A=10/10, B=7/10
Pros / cons:
A) Keep theirs, run after ours (recommended)
  ✅ An existing xclip or pbcopy setup keeps working beside matchblox
  ✅ matchblox stores the old value, so removing matchblox can put it back
  ❌ Two commands per copy, and the owner's command can fail on its own
B) Replace it
  ✅ One command per copy, nothing to chain
  ❌ An owner's own copy setup silently stops, the kind of failure the spec forbids
Net: a few lines keep someone else's setup alive.
Header: Copy command
Options:
A) Keep theirs, run after ours (recommended)
When copy-command holds a command that is not matchblox's, matchblox saves it in a server user option and its copy command pipes the same text to it after its own write. Test: a tmuxtest row with a fake owner command. Effort human ~1 h / CC ~10 min. Risk: the owner's command fails on its own; matchblox reports only its own failure.
B) Replace it
matchblox sets copy-command to its own command, whatever was there. Effort none. Risk: an owner's xclip or pbcopy setup stops silently.

State: superseded by R7 (D7)
Actual answer: A) Keep theirs, run after ours (recommended), owner, D6 2026-11-19
Accepted scope: when copy-command holds a command that is not matchblox's, matchblox saves it in a server user option, and its copy command pipes the same text to it after its own write. A tmuxtest row uses a fake owner command.
History: approved D6 2026-11-19; superseded by R7 A (no copy-command is installed).

Scope Challenge result: scope accepted as-is (no feature cut; R5 changed the mechanism, not the scope).

## 1. Architecture review

The copy path after R5 (no `set-clipboard` change anywhere):

```
 drag / double / triple click in an agent pane (jump or view key table, paneMouse)
   │  pane reads the mouse? ── yes ─▶ send SGR release at the press cell (R3)
   ▼
 tmux copy mode -M ─▶ select ─▶ release ─▶ copy-pipe-and-cancel (no command)
   │                                     └─▶ tmux copy-command = matchblox copy cmd (absolute path)
   ▼
 matchblox copy cmd (stdin = selection, already trimmed and unwrapped by tmux)
   ├─ real clients of this server (client tty is no pane tty): load-buffer -w -t <client>
   │     └─ desk terminal / Pocket gets OSC 52 from tmux itself (external is enough)
   ├─ this server is a host reached by a jump (@matchblox wrap mark set by the jump):
   │     write OSC 52 wrapped in tmux passthrough to the jump client's tty
   │     └─ desk tmux: the jump pane has allow-passthrough (desk console sets it) ─▶ terminal
   ├─ owner's saved copy-command (R6): pipe the same text to it
   └─ failure: tmux display-message "not copied: <cause> · <fix>" (no silent drop)
 nested view client (desk console window) is a client whose tty IS a pane tty: skipped,
   so no copy goes through the outer pane's OSC 52 parser.
 console list `c` / its own drag ─▶ same command (load-buffer -w), not tea.SetClipboard
```

Findings:
7. [P2] (confidence 7) R6: owner's copy-command. Resolved D6 (A).
8. [P2] (confidence 8) internal/remote/jump.go:248-262 the host jump runs ssh in a desk session with no `allow-passthrough`; cmd/matchblox/graphics.go:40-57 sets it only on the console's own pane. The wrapped OSC 52 from a host is dropped without it. Necessary code for approved R5 B: the jump pane gets `allow-passthrough on` (pane option, that pane only). No question.
9. [P2] (confidence 7) tmux runs copy-command through its own shell with the server's PATH. A bare `matchblox` may not resolve on a host. Necessary code for R5 B: set the absolute path of the running binary (os.Executable), shell-quoted. No question.
10. [P2] (confidence 8) internal/doors/doors.go:162 `copyThroughArgv = {"tmux","set","-s","set-clipboard","on"}` and the door "Let copy through" would turn on the read exposure R5 rejected; internal/app/copy.go:19 `tmuxDrops` tells the owner to run it. Necessary code for R5 B: delete the door, its allowlist entry (doors.go:181) and the notice; docs/console.md "Copy" table rewritten. No question.
Failure per new path: the copy command exits non-zero (no real client, passthrough off, binary moved). The owner sees `not copied: <cause> · <fix>` in tmux's message line. Covered by tests in Section 3.
Dispositions: 7 accepted (D6); 8, 9, 10 accepted as necessary work of D3.

## 2. Code quality review

11. [P3] (confidence 7) internal/app/copy.go:35 `clip` writes OSC 52 through tea.SetClipboard when no Clip is set. Under R5 B the console must route `c` through the same matchblox copy path (load-buffer -w), or a desk console under `external` still drops it. Necessary code of D3. One shared path (the copy command's Go function) serves the copy-command entry and `clip`: two real callers, same contract (text in, real clients out). No new abstraction beyond that function.
12. [P3] (confidence 6) The `copied … · not checked here` / pasteChecked flow (copy.go:20-22) stays; its words must stop naming set-clipboard. Part of finding 10.
Dispositions: 11, 12 accepted as necessary work of D3.

## 3. Test review

Framework: Go (`go test ./...`), private tmux servers through internal/tmuxtest; Swift on macOS CI (not touched).

```
CODE PATHS                                              USER FLOWS
[+] internal/remote/jump.go paneMouse                   [+] Drag-copy in the beside view (desk)
  ├── [★★ TESTED] binds exist — jump_test.go:10 mouseIn   ├── [GAP] [→E2E] drag in agent pane → real client gets OSC 52, external server
  ├── [GAP] drag → copy-mode for a mouse-reading pane     ├── [GAP] [→E2E] nothing reaches a nested view client's pane parser
  ├── [GAP] SGR release before copy mode (R3)             └── [GAP] paste holds no sidebar text (manual, done-when 1)
  └── [GAP] double/triple-click binds (R4)              [+] Over SSH (matchblox <host>)
[+] matchblox copy command (new)                          ├── [GAP] [→E2E] host copy → wrapped OSC 52 → desk tmux passthrough → terminal
  ├── [GAP] real-client pick (skips pane-tty clients)     └── [GAP] passthrough on the jump pane only
  ├── [GAP] wrap mark → passthrough form                [+] Error states
  ├── [GAP] owner's saved copy-command runs after (R6)    ├── [GAP] no real client → message with fix
  └── [GAP] failure → display-message, exit non-zero      └── [GAP] owner command fails → matchblox still copied, says so
[+] install of copy-command (console start, jump)       [+] Regression
  ├── [GAP] sets absolute path; keeps owner's value       ├── [GAP] CRITICAL click still = press+release (R3)
  └── [GAP] never sets set-clipboard (R5 contract)        └── [GAP] CRITICAL wheel still reaches the agent
[+] internal/app/copy.go clip → copy path
  └── [★★ TESTED] copy_test.go menu/drag text — extend: goes through the copy command, not OSC 52
[−] doors CopyThrough + tmuxDrops: retire

COVERAGE: 2/19 paths tested (11%) | GAPS: 17 (4 E2E)
QUALITY: ★★:2
```

Tests to add (value cards):
- tmuxtest: a view copy reaches the real client under `external`. Value: protects=drag copy in the beside view reaches the desk terminal; fails_when=the copy goes through the nested pane's OSC 52 parser; why_new=no test crosses the nested client; seam=none; no_claim=a real desk terminal's clipboard (script(1) stands in)
- tmuxtest: a copy never sets set-clipboard. Value: protects=no pane can read the last copy (R5); fails_when=any start-up or jump sets set-clipboard on; why_new=no test reads the option after start; seam=none; no_claim=none beyond protects
- tmuxtest: mouse recorder app. Value: protects=each drag gives one press and one release, a click gives press+release, the wheel arrives; fails_when=the R3 release is missing or doubled; why_new=no test sees the agent's mouse events; seam=none; no_claim=real agents' reaction (live check)
- jump_test rows: drag, double-click, triple-click binds; allow-passthrough on the jump pane; copy-command install with -q free list. Value: protects=the jump's command list; fails_when=a bind or option drops; why_new=mouseIn covers four binds only; seam=none; no_claim=tmux runs them (covered by tmuxtest)
- copy command unit table: real-client pick, wrap form, owner command after, failure message. Value: protects=routing and errors; fails_when=a pane-tty client is picked or a failure is silent; why_new=new code; seam=none; no_claim=terminal behaviour
- copy_test extension: `c` goes through the copy path. Value: protects=console copy under external; fails_when=clip falls back to OSC 52; why_new=copy_test asserts text, not route; seam=none (Options.Clip exists); no_claim=none beyond protects
Tests to retire: doors_test rows for CopyThrough; copy_test rows asserting `tmuxDrops`.
Live checks (done-when): Claude Code, codex, opencode, cursor-agent: click a menu item after a drag; paste a three-line drag on the desk, local and over SSH.

## 4. Performance review

One short process per copy (the copy command), plus a `list-clients` call: milliseconds, on a person's action only (estimated). No loops, no polling. No issues found.

## Outside Voice

Provider: codex not installed (CODEX_MODE: not_installed). Native fallback: a Claude Plan subagent, read-only, finished inside the five-minute timer. Its report, in full:

OUTSIDE VOICE (Claude subagent):
1. High: the owner's copy-mode MouseDragEnd1Pane binding (global copy-mode tables) decides whether copy-command runs; a rebind or unbind makes every drag copy nothing, silently.
2. High: the beside view of a peer's agent (PeerViewArgv, internal/app/beside.go:346: ssh in a console pane) is missed; passthrough only on the host jump pane leaves remote agents beside the list dropped.
3. High: allow-passthrough on the jump and peer panes lets anything printed on the host (any process, host panes with passthrough, banners) write raw escapes to the desk terminal, including OSC 52 writes and reads on terminals that answer: a wider channel than set-clipboard on, which R5 rejected.
4. High: os.Executable() is the desk's path; the host needs its own matchblox at a version with the copy command; copy-command stays set after the jump, and a moved or removed binary breaks every copy on that server; no restore path.
5. Medium: the copy command writing to the client tty races tmux's redraw after copy-pipe-and-cancel; a split sequence garbles the screen or loses the copy.
6. Medium: "the real client" is undefined; sending to every real client puts a desk copy on the phone and breaks the phone's copy gate; the copy job has no client context.
7. Medium: R3's synthetic release needs the pane's mouse encoding; tmux has no press-position format; press+release at one cell is a click on a button or link; for a double-click the release has no press. Simpler: send nothing on MouseDown1Pane, send press+release on MouseUp1Pane.
8. Medium: copy-command is server-wide; every keyboard copy in every session on the desk and on each host runs matchblox.
9. Medium: the spec's top half (ruling 1, "Ways it goes wrong" 1, Surfaces, done-when `set-clipboard` `on`) contradicts R5; "Check that copy reaches you" proves only the console `c` path.
10. Medium: full-screen agents such as opencode draw their own sidebars and borders; a tmux line selection takes them, and their wraps stay hard; done-when never drags in opencode.
11. Low: two buffers per drag; an owner already on set-clipboard on gets two OSC 52 writes; use a named buffer.
12. Low: deleting the "Let copy through" door removes the only opt-in for agents' own copy commands.
13. Low: a plain `ssh host; tmux attach` outside matchblox gets no wrap mark and still drops copies.
Recommendation (reviewer): do not build R5 B as written; probe load-buffer -w under external, a rebound drag-end, a tty write race, and passthrough from a host process; then fix 1-4 and redo the R5 comparison.

Parent probes after the report (tmux 3.4, private servers):
| Probe | Result |
|---|---|
| `load-buffer -w -t <client>` under set-clipboard external | terminal got 1 OSC 52 |
| wrapped OSC 52 written to a nested client's tty, outer pane allow-passthrough off / on | 0 / 1 OSC 52 at the terminal |
| tmux -C attach while a buffer is set | `%paste-buffer-changed buffer0` |
| PeerViewArgv | internal/app/beside.go:346, confirmed |

Cross-model tension: the reviewer and the parent agree that R5 B needs passthrough over SSH and that passthrough opens the desk terminal to the host (finding 3). Findings 1, 2, 4, 6 and 7 hold on the code read. Finding 5 was not probed. This reopens R5 (R7 below) and R3 (finding 7, after R7).

### R7: carry copies over matchblox's own link, not the terminal (reopens R5)
Finding: outside voice 1-6, 8, 13 (High/Medium), confidence 8, reviewer: Claude Plan subagent + parent probes
Plan baseline: R5 B (owner, D3): copy-command runs a matchblox copy command; over SSH it writes OSC 52 wrapped for passthrough.
Runtime evidence: probes above. matchblox already links the desk console to each host's service over SSH stdio (internal/transport/stdio.go, proto kinds in internal/proto/proto.go:35-54). tmux control mode reports every new buffer (`%paste-buffer-changed`).
Comparison grid:
| Choice | Current (R5 B) | A apply | B keep | C investigate | D defer |
|---|---|---|---|---|---|
| How a copy is noticed | copy-command on every server | each matchblox service watches its own tmux's buffers (control mode) | copy-command | unchanged until the spike reports | R7 stays open; R5 B stands |
| How it reaches the screen | load-buffer -w; over SSH OSC 52 through passthrough | the console the person looks at loads it to its own client with load-buffer -w; a host's copy travels to the desk console as a proto message | as current | — | as current |
| Passthrough opened on jump / peer panes | yes | no | yes | — | yes |
| Owner's copy binds / copy-command | depends on them | any copy that makes a buffer works; nothing installed | depends on them | — | depends |
| Host needs a matchblox copy subcommand on PATH | yes | no (its service already runs) | yes | — | yes |
| set-clipboard | untouched | untouched | untouched | untouched | untouched |
| R3, R4, R6 | approved | R6 drops (no copy-command); R3 reopened separately; R4 unchanged | unchanged | unchanged | unchanged |
Question D7:
D7 — Carry copies over matchblox's own link instead of through the terminal?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: An independent review found that the design you picked (D3) has a hole over SSH. To get a host's copy to your desk, it must open a "passthrough" door in your desk tmux, and then anything on that host can send raw escape codes to your terminal. That is the same kind of risk D3 was meant to avoid. It also breaks if your tmux has its own mouse copy binding, misses agents shown beside the list from another host, and needs matchblox installed and up to date on every host. There is a cleaner way. matchblox already keeps its own connection between your desk and each host. Each host's matchblox notices every tmux copy (tmux announces each one) and sends it over that connection. Your desk console then puts it on your clipboard. No escape code crosses your terminal, and nothing changes in anyone's tmux settings.
Stakes if we pick wrong: either the desk terminal is open to anything on a host, or copies over SSH stay unreliable.
Recommendation: A because it closes the passthrough hole and fixes findings 1, 2, 4, 6 and 13 at once, using a connection matchblox already has.
Note: options differ in kind, not coverage — no completeness score.
Pros / cons:
A) Apply: copies go over matchblox's link (recommended)
  ✅ No passthrough and no tmux setting changes on any desk or host
  ✅ Works with any owner binding that makes a tmux buffer, and for peer views
  ❌ A new piece: a tmux control-mode watcher in each service, plus one message kind
B) Keep R5 B, fix it in place
  ✅ Builds on the decision already made, with no new watcher
  ❌ Still needs passthrough over SSH, the hole the review found
C) Investigate first
  ✅ A bounded spike proves control mode over the existing link before you choose
  ❌ One more round trip before any build starts
D) Defer this change
  ✅ Nothing changes now; R5 B stays as decided
  ❌ The build starts on a design the review says not to build
Net: one new watcher buys a copy path no host can abuse.
Header: Copy carrier
Options:
A) Apply: copies go over matchblox's link (recommended)
Each matchblox service watches its own tmux's buffers through a control-mode client; a new copy goes to the console the person is looking at, which loads it to its own tmux client with load-buffer -w. A host's copy reaches the desk console as a proto message over the existing SSH link. No copy-command, no passthrough, set-clipboard untouched. R6 drops. Effort human ~3 days / CC ~2-3 h. Risk: a long-lived control-mode client per server to keep healthy.
B) Keep R5 B, fix it in place
Keep copy-command plus passthrough; the spec adds the peer view panes, the host binary path, and a check of the owner's drag-end bind. Effort human ~2 days / CC ~1.5 h. Risk: the host can write raw escapes to the desk terminal.
C) Investigate first
A spike of at most 1 h: a control-mode watcher on this host plus a copy sent over the existing link, then R7 is asked again with the result. No build until then; R5 B stays approved but unbuilt.
D) Defer this change
Leave R7 open and R5 B as decided; the plan records the review's objection as unresolved.

State: approved
Actual answer: A) Apply: copies go over matchblox's link (recommended), owner, D7 2026-11-19
Accepted scope: each matchblox service watches its own tmux server's buffers through a tmux control-mode client (`%paste-buffer-changed`). A new copy goes to the console the person is looking at, which loads it to its own tmux client with `load-buffer -w`. A host's copy reaches the desk console as a proto message over the existing SSH link. No copy-command, no passthrough, no set-clipboard change on any server. R5 B's mechanism and R6 are superseded; R5's goal (no pane reads or writes the clipboard by OSC 52) stands.
History: none

### R3 (reopened): how a click and a drag reach a mouse-reading agent
Finding: outside voice 7, Medium, confidence 7, internal/remote/jump.go:134, reviewer: Claude Plan subagent
Plan baseline: R3 approved D4 (A): send the pane an SGR release at the press position, then start copy mode.
Runtime evidence: tmux 3.4 gives the drag binding the current cell (`#{mouse_x}`, `#{mouse_y}`), not the press cell. A release on the pressed cell completes a click, so a drag that starts on a menu item or a link would click it. A pane may use a non-SGR mouse encoding (`#{mouse_sgr_flag}` 0); SGR bytes there arrive as typed text. For a double-click the agent already had press and release.
Reason to reopen: these facts make the approved remedy cause a new failure (an accidental click), a concrete new risk.
Comparison grid:
| Choice | Current (R3 A, D4) | A | B |
|---|---|---|---|
| R3 press timing | press sent at once; synthetic release before copy mode | press held back: MouseDown1Pane selects the pane only; on a release with no drag, the bind sends the agent press+release at that cell in the pane's own encoding | keep D4, but only for SGR panes, at the drag's first cell; no release for double-click |
| A drag that starts on a button | clicks it | clicks nothing | clicks it |
| Test | press/release recorder | recorder: a click gives press+release, a drag gives nothing, the wheel arrives | recorder as in D4 |
| R4, R7 | approved | unchanged | unchanged |
Question D8:
D8 — Hold back the agent's mouse press until the button comes up?
Project/branch/task: matchblox main, the copy spec (docs/superpowers/specs/2026-11-18-copy-one-pane-design.md).
ELI10: The review found a flaw in D4's fix. The "release" that matchblox sends when a drag starts lands where the press was, so to the agent it looks like a click. If your drag starts on a menu item or a link in Claude Code, that item fires. The cleaner way: matchblox holds the press back. If you let go without moving, the agent gets a whole click (press and release) at that spot. If you drag, the agent gets nothing and tmux selects text.
Stakes if we pick wrong: either a drag sometimes clicks a button, or clicks reach the agent a moment later (on release, not on press).
Recommendation: A because a drag can never click anything, and the agent never sees half a click.
Completeness: A=10/10, B=7/10
Pros / cons:
A) Hold the press, send a whole click (recommended)
  ✅ A drag never clicks a button, a menu item or a link in any agent
  ✅ The agent only ever sees whole clicks, so it cannot get stuck
  ❌ A click reaches the agent on release, not on press: a drag inside the agent is gone
B) Keep D4, narrowed
  ✅ Clicks reach the agent on press, as today
  ❌ A drag that starts on a button still clicks it
Net: click-on-release costs nothing visible and removes accidental clicks.
Header: Click timing
Options:
A) Hold the press, send a whole click (recommended)
MouseDown1Pane only selects the pane. MouseUp1Pane with no drag sends the agent press+release at that cell, in SGR or in the normal mouse encoding by `#{mouse_sgr_flag}`; a pane that does not read the mouse gets nothing. A drag goes to tmux copy mode and the agent gets nothing. Test: the recorder sees press+release per click, nothing per drag, and wheel events. Effort human ~0.5 day / CC ~30 min. Risk: an agent that wants its own drags loses them (none of the four uses them for copy).
B) Keep D4, narrowed
Keep the press at once; before copy mode send a release at the drag's first cell, only to SGR panes; no release for a double-click. Effort human ~0.5 day / CC ~30 min. Risk: a drag starting on a button clicks it.

State: approved
Actual answer: A) Hold the press, send a whole click (recommended), owner, D8 2026-11-19
Accepted scope: MouseDown1Pane only selects the pane. MouseUp1Pane with no drag sends a mouse-reading agent press+release at that cell, in SGR or the normal encoding by `#{mouse_sgr_flag}`; a pane that does not read the mouse gets nothing. A drag goes to tmux copy mode and the agent gets nothing. The wheel is unchanged. A tmuxtest recorder app proves: a click gives press+release, a drag gives nothing, the wheel arrives. Live check in Claude Code, codex, opencode and cursor-agent.
History: D4 2026-11-19 approved "Send a release, test it"; reopened by outside voice finding 7.

Outside voice dispositions: 1, 2, 4, 6, 8, 13 accepted through R7 (D7); 3 accepted through R7 (no passthrough); 7 accepted through R3 reopened (D8); 9 accepted as a factual correction (spec top half aligned to R5/R7/R3/R4); 11 accepted as build detail (named buffer, no duplicate copies); 5 moot under R7 (no tty write); 10 and 12 unresolved (below).

Approval readiness: PASS. Checked: R3 reopened (D8), R4 (D5), R5 goal (D3), R7 (D7). Superseded: R1 (D1 → D3), R2 (D2, false premise), R3 first (D4 → D8), R6 (D6 → D7).

## Test plan after R7 (replaces the copy-command rows of Section 3)

```
CODE PATHS                                              USER FLOWS
[+] service buffer watcher (new, tmux -C per server)    [+] Desk, beside view
  ├── [GAP] %paste-buffer-changed → copy event           ├── [GAP] [→E2E] drag in agent pane → console's client gets OSC 52 (external server)
  ├── [GAP] watcher dies → restarts, says so             └── [GAP] double-click word / triple-click line
  └── [GAP] own load-buffer does not re-trigger a loop  [+] Over SSH (host jump and peer view)
[+] proto copy message host → desk console                ├── [GAP] [→E2E] host buffer → proto message → desk client OSC 52
  ├── [GAP] carries text; size cap                       └── [GAP] no passthrough set on any pane
  └── [GAP] desk console not attached → kept? dropped + said
[+] console: which client gets it                       [+] Regression (CRITICAL, R3 D8)
  ├── [GAP] the client showing this console               ├── [GAP] click = press+release on release, right cell, both encodings
  └── [GAP] phone console: board's copy gate untouched    ├── [GAP] drag = nothing to the agent
[+] paneMouse binds (jump.go)                             └── [GAP] wheel still reaches the agent
  ├── [★★ TESTED] mouseIn — jump_test.go:10 (extend)
  └── [GAP] Down selects only; Up sends click; Double/Triple select
[+] copy.go clip → buffer path (not OSC 52)  [★★ copy_test, extend]
[−] doors CopyThrough, tmuxDrops: retire
COVERAGE: 2/18 paths (11%) | GAPS: 16 (3 E2E)
```

Value cards:
- E2E desk drag: protects=a drag in the beside view reaches the console's terminal under external; fails_when=the copy relies on OSC 52 through the nested pane; why_new=no test crosses the view; seam=none; no_claim=a real terminal clipboard (script(1) log stands in)
- E2E host copy: protects=a host buffer reaches the desk client over the proto link; fails_when=the message kind or the desk's load-buffer drops; why_new=new path; seam=none; no_claim=real SSH (stdio pipe stands in)
- Recorder: protects=click arrives whole on release, drag sends nothing, wheel arrives; fails_when=a bind forwards the press or loses the up; why_new=no test sees agent mouse bytes; seam=none; no_claim=each agent's reaction (live check)
- set-clipboard unchanged + OSC 52 read gets no reply: protects=R5 goal; fails_when=any code sets set-clipboard or passthrough; why_new=none reads it after start; seam=none; no_claim=none beyond protects
- Watcher loop guard: protects=one copy makes one clipboard write; fails_when=the console's own load-buffer re-fires the watcher; why_new=new; seam=none; no_claim=none beyond protects
Tests to retire: doors_test CopyThrough rows; copy_test tmuxDrops rows.

## NOT in scope
- Agents' own copy commands (Claude Code /copy): they write OSC 52, which stays dropped under external. Unresolved decision 2.
- A plain `ssh host; tmux attach` outside matchblox: no matchblox link, no copy carrier.
- The phone's drag copy: matchblox-board's copy gate (feat/phone-console-controls).
- Rectangle selection for agents that draw their own sidebars.

## What already exists
- tmux copy mode: trims trailing spaces, joins soft wraps (probe), reused as the selector.
- `load-buffer -w`: reaches the client's terminal under external (probe), reused as the writer.
- The desk-to-host proto link (internal/transport, internal/proto): reused as the carrier; one new message kind.
- paneMouse (jump.go:131): one place for jump, view and peer-view binds.
- copy.go `clip` and the "Check that copy reaches you" door: kept; `clip` routes through a buffer.

## Failure modes
| Path | Failure | Test | Handling | Seen by the person |
|---|---|---|---|---|
| watcher | control-mode client dies | watcher restart test | restart; status says it | yes (status line) |
| proto copy | desk link down when a host copies | E2E host copy, link-down row | drop, tmux message on the host pane | yes |
| console client pick | no client shows the console | unit | tmux message `not copied: no screen shows the console · open the console` | yes |
| load-buffer -w | client terminal lacks Ms | unit (feature check) | message names the hop | yes |
| loop | console's own buffer re-fires watcher | loop guard test | named buffer ignored | n/a |
Critical gaps (no test, no handling, silent): 0 after the planned tests.

## Worktree parallelization strategy
| Step | Modules touched | Depends on |
|---|---|---|
| Mouse binds (R3, R4) | internal/remote | — |
| Buffer watcher + proto message | internal/service, internal/proto, internal/transport | — |
| Console delivery + clip route + door removal | internal/app, internal/doors, docs | watcher |
Lane A: mouse binds. Lane B: watcher → console delivery. Launch A + B; merge A; finish B. Conflict flag: internal/app/beside.go is also touched by matchblox-board's preview branch; keep app edits in copy.go.

## Implementation Tasks
- [ ] **T1 (P1, human: ~4h / CC: ~30min)** — remote — Hold the press; send a whole click on release; drag to copy mode; double/triple-click select
  - Surfaced by: R3 reopened (D8), R4 (D5)
  - Files: internal/remote/jump.go, internal/remote/jump_test.go, internal/tmuxtest (recorder app)
  - Verify: go test ./internal/remote/... ./internal/tmuxtest/...; live click after a drag in the four agents
- [ ] **T2 (P1, human: ~1d / CC: ~1h)** — service — Watch each server's buffers through tmux control mode; emit a copy event; loop guard; restart
  - Surfaced by: R7 (D7)
  - Files: internal/service (new watcher file), internal/proto/proto.go
  - Verify: go test ./internal/service/... with a private tmux server
- [ ] **T3 (P1, human: ~1d / CC: ~1h)** — app — The console the person looks at loads a copy to its own client (load-buffer -w); host copies arrive over the proto link; clip routes through a buffer
  - Surfaced by: R7 (D7), Section 2 finding 11
  - Files: internal/app/copy.go, internal/app (event handling), internal/transport
  - Verify: E2E desk drag and host copy tests; manual paste on the desk, local and over SSH
- [ ] **T4 (P2, human: ~2h / CC: ~15min)** — doors/docs — Remove "Let copy through" and the tmuxDrops notice; rewrite docs/console.md "Copy"; CHANGELOG; landing page line
  - Surfaced by: Section 1 finding 10, R5 goal
  - Files: internal/doors/doors.go, internal/app/copy.go, docs/console.md, CHANGELOG.md, www/
  - Verify: go test ./internal/doors/... ./internal/app/...; .github/scripts/landing-guard.sh
- [ ] **T5 (P1, human: ~1h / CC: ~10min)** — guard — set-clipboard unchanged and no passthrough after start and jump; an OSC 52 read gets no reply
  - Surfaced by: R5 goal, outside voice 3
  - Files: internal/tmuxtest or cmd/matchblox tests
  - Verify: go test ./...

## Unresolved decisions that may bite you later
1. Outside voice 10: agents that draw their own sidebars or borders (opencode) put them into a tmux line selection. No remedy chosen (rectangle select, or accept).
2. Outside voice 12: an opt-in for agents' own copy commands (the removed "Let copy through" door, with the read risk stated). No remedy chosen.

## Completion summary
- Step 0: Scope Challenge — scope accepted as-is
- Reality rows: 7/7 emitted by hand (helper needs bun): premise-table 6 claims, 1 FALSE (no-read under on), 0 unverified; already-done pass (git log: nothing since #430); surface-check finding (get-clipboard absent 3.4-3.6); binding-decisions pass (ruling 1 reopened on record); numbers pass (all measured or labelled estimated); deferred-asks pass (no issue); pr-count pass (1 PR)
- Architecture Review: 4 issues found
- Code Quality Review: 2 issues found
- Test Review: diagram produced, 16 gaps identified
- Performance Review: 0 issues found
- NOT in scope: written
- What already exists: written
- TODOS.md updates: 0 items proposed (no TODOS.md; 2 unresolved decisions instead)
- Failure modes: 0 critical gaps flagged
- Unresolved decisions: 2 in this review
- Outside voice: codex not installed; native Claude Plan subagent completed (13 findings); outside coverage unavailable
- Parallelization: 2 lanes, 2 parallel / 1 sequential
- Lake Score: 7/7 answers picked the recommended 10/10 option (D7 had no score)

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Outside Review | codex (not installed) → native Plan subagent | Independent 2nd opinion | 1 | unavailable (native completed) | 13 findings, 11 resolved, 2 open |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | issues_open | 22 issues, 0 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | — |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **OUTSIDE COVERAGE:** codex, plan-review phase, unavailable (CLI not installed); native Claude Plan subagent completed inside the timer with 13 findings. No external coverage.
- **VERDICT:** no review CLEAR; eng review required (issues are mapped to tasks T1-T5; 2 decisions open).

**UNRESOLVED DECISIONS:**
- U1: opencode and other self-drawn sidebars in a tmux line selection (outside voice 10)
- U2: an opt-in for agents' own copy commands (outside voice 12)
