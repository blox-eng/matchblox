# The owner answers an agent with one thumb on one match head, and reaches for no key

## Who, and the moment

The owner runs agents from an iPhone with Matchblox Pocket, one hand on the phone, often while walking.
An agent asks a question. The owner reads what it wants, picks an option or types a line, and sends it.
Then the owner waits for the next question.
Evidence: the owner, 2026-11-27: "every single time i need to answer a session i usually review what is wanted from me then i type stuff click enter then honestly the pocket can switch to close the keyboard in most cases and leave the joystick".

## Why it matters

- Today the match head is one belt cell, and only on a layout where the owner added it (`docs/phone.md`, "The match head"). An answer is a reach to `↑` `↓` `⏎` at the belt's right end (measured by reading the layouts).
- After a typed answer the keyboard stays up, and the owner closes it by hand to read the next reply (owner, 2026-11-27).
- Pocket has two heads with two behaviours: the belt key, and the head that comes under the finger after a rest on the terminal (`docs/superpowers/specs/2026-11-20-matchhead-summon-design.md`). The owner learns both (measured: two specs, one `HeadDriver`).

## The experience

**At rest.** Above the belt, centred, sits one match head, about 132 pt across. Its lower part overlaps the belt; the rest stands in a row of its own above the belt, so it never covers the terminal's text. The terminal is about 150 pt shorter (estimated from the mockup: a 224 pt dock against about 70 pt today). It is the head of today, with its shading and heat colours, and no texture or prop round it. Nothing else is drawn round it. The belt runs behind it and turns without end, as today. The handle row says `roll: options, then the output · hold: sessions · BOX sets them`. In this spec, "the handle row" is that row, above the belt.

**A touch.** The moment a finger lands on the head, a thin ring and four marks fade in round it. Each mark shows a glyph and a word: top `◂ back`, left `← cursor`, right `→ cursor`, bottom `⌨ type`. The heat colours of today stay: cold red, ember, amber. Nothing is sent on touch. The ring and marks fade 0.9 s after the lift.

**Roll the edge.** The finger moves round the head, and the head turns under it.
1. When the agent shows a menu, each notch moves Pocket's highlight one option, and plays one tap. Anticlockwise goes up, clockwise goes down. The highlight starts on the option the agent marks with `❯` or `›`. The agent's own mark does not move. The handle row shows the highlighted option: `2 of 3 · Yes, and open a pull request`. The host reads the menu, its mark and its signature, as `service/choose.go` does; Pocket asks for them over its SSH link, so it works in the console and in an agent's pane.
2. Past the first option, the roll goes on up into the output, one line a notch. A firm tap marks the crossing. The handle row says `12 lines up · tap the head to come back`.
3. Keep rolling, and it speeds up with time spent rolling: 1 line a notch for the first 10 notches, then 3 to notch 24, then 8 to notch 48, then 20. The count starts at the crossing into the output. A pause of 0.35 s starts it again at 1. Menu options always move one a notch. At 20 the head strikes, as today, once a roll: it flashes and plays one rigid tap.
4. While it moves 3 lines a notch or more, a card names the agent step at the top of the view (`✦ Running go test ./internal/...`) and how far up the view is.
5. At the last option, or the top of the scrollback, one firm bump, and the handle row says why and which way to roll.
6. Rolling clockwise from up in the output stops at the bottom with one firm tap. The same roll never goes on into the menu, so it never moves the agent's choice. A new roll does.

**Tap the middle.** At a menu, the tap picks the highlighted option: Pocket sends its number, its text and the menu's signature, and the host types the number only if the screen still matches. If the screen changed, nothing is sent, and the handle row says `the question changed · roll again`. With no menu, the tap is Enter, at once. Up in the output, the tap only brings the view back to the bottom.

**Press the edge.** A touch on the edge that moves less than 10 pt and lifts sends the key of the mark beside it. A touch that moves more sends no mark's key. `◂ back` sends what the belt's `◂` sends: back to matchblox. A hold on `←` or `→` repeats after 0.42 s, every 0.11 s, and after 9 repeats moves a word at a time (`Esc b`, `Esc f`). A touch that starts in the middle and then moves turns the head and sends nothing. `⌨` raises the keyboard. After every Enter sent from the keyboard, the line is sent, then the keyboard folds, and the head is back.

**Hold the middle.** A ring fills round the head for 0.6 s. Then the sessions show in a card, the current one marked. Any move round the head now steps through them: the finger needs not reach the edge. Lift to go there. Lift on the current one, and nothing changes. A lift under 0.35 s is a tap: Enter. A lift between 0.35 s and 0.6 s sends nothing.

**The belt.** Drag the belt, and it comes in front of the head: the head fades, and every key shows. 1.4 s after the belt stops, it slips back behind the head. A tap on a belt key works as today.

**Set it on home.** Open the drawer, tap `BOX`, and home opens (the home of `2026-11-24-pocket-home-design.md`, PR #508). A region `THE HEAD` sits under `LIVE ACTIVITY`. It draws the head with its four marks and its hold. Tap one, and a list of keys shows, each with its glyph and word: `◂ back`, `⌨ type`, `← cursor`, `→ cursor`, `Esc cancel`, `Tab tab`, `⇧Tab mode`, `⇞ page up`, `⇟ page down`, `/c compact` (types `/compact` with no Return), `· nothing`, and for the hold only `⇄ sessions`. Tap a key to set it. `undo` takes back the last change. `as it came` brings back the first marks. `^C` is in the list, dim, with the reason: `stays on the belt: a roll can brush the edge`. The region also holds `size − 132 pt +`, from 96 to 160 pt in steps of 4. The terminal gives up the head's height: about 150 pt at 132 pt.

**When it goes wrong.**
- No menu on screen: the roll goes straight into the output.
- A program with no scrollback that does not read the mouse: the head turns, and the handle row says `nothing scrolls here · this program has no scrollback`, as today.
- In a tmux pane the app knows the bottom, not the top: the head turns on at the top, as today.
- The keyboard is up: the head is under the keyboard, and the belt stays above it, as today.

**Heuristics check** (owner, 2026-11-27: "nielsen heuristics apply here").
- Status: the handle row always says what a roll does now. The hold ring fills before the sessions show.
- Real world: a ball you roll, a centre you press, a home where settings live. Plain words: back, cursor, type, sessions.
- Control: a short tap is Enter, a lift in the hold ring sends nothing, lift on your own session is nothing, home has undo and as it came.
- Consistency: one head in all of Pocket. The glyphs match the belt's; the words are new and short. Settings live only on home.
- Error prevention: where the finger lands decides the meaning, not the shape of a gesture. A roll never presses a mark.
- Recognition: the marks show on touch, before a press sends. Home draws the head with every mark.
- Flexibility: the first marks work on day one. The owner's own marks and hold come later.
- Minimal: at rest, one head. No texture, no counter, no drawn flame.
- Errors: a bump and a reason at each end.
- Help: the handle row at rest says what a roll and a hold do, and where to set them.

## Non-goals

- No push to steer, no tap-then-roll, no double tap, no circle gestures as commands.
- No cardboard, grain, striker paper, wooden stick, flame or counter on the dock.
- No new layout in the tray, and no toggle to hide the head.
- No setting for the speed-up, the notch size or the fade times.
- No change to the console on the desk.

## Anti-goals (failure even if it works)

- A roll that brushes the edge sends a mark's key.
- A lift after 0.35 s, or a lift after a roll, sends Enter.
- A hold, or any touch on the head, changes the owner's settings.
- The head hides the agent's question or the owner's input line, and stays there.
- The sessions card goes to a session other than the one marked when the finger lifts.
- `^C` reaches the agent from the head.
- The keyboard folds before the line is sent, and the typed text is lost.

## Surfaces

- An agent's tools: rolls, taps and marks type into a live agent's pane. A stray Enter can approve a tool call. A stale pick could answer a question the owner did not see; the host refuses it. Their anti-goals are the first, second, fifth and sixth above.
- Checked and not touched: authentication, money, uploads, webhooks, secrets, CI, personal data (the marks are stored on the device with the layouts, and the privacy manifest already declares that storage).

## Rulings

- 2026-11-27 — The head is a control you roll round its edge, with a middle to press and a rim of four marks, at the level of a classic scroll wheel — because the owner asked for "that level of sophistication". — the owner.
- 2026-11-27 — Where the finger lands decides the meaning. Push to steer and the tap-then grammar are dropped — because few controls, each one obvious, beat a gesture grammar to learn. — the owner (Steve Jobs pass).
- 2026-11-27 — At rest only the head shows. The ring and marks show on touch — because the owner asked for "just the match head without the rims, which can show on touch". — the owner.
- 2026-11-27 — The hold never edits. Editing lives on home, under `BOX`. The hold switches session — because the owner said "edit can be the opened box … hold must be a different action", and picked "switch session" in the mockup. — the owner.
- 2026-11-27 — The dock keeps the console's flat look: no matchbox textures or props — because the owner said "i hate the latest matchbox inspired enhancement". — the owner.
- 2026-11-27 — A roll walks the options, then on up into the output. Marks show on touch. The belt comes in front while dragged. The keyboard folds after a send — because the owner approved these picks in the mockup ("lgtm"). — the owner.
- 2026-11-27 — At a menu, the roll moves a highlight of Pocket's own, and a tap sends a pick the host checks, never arrow keys — because a lost or late arrow moves the agent's choice to a wrong option, and the host already refuses a stale pick (`service/choose.go`). A peer session proposed it; the owner picked it over Pocket reading its own screen. — the owner.
- 2026-11-27 — The second cold read's eight gaps take the answers now written in The experience: a tap under 0.35 s, the host reads the menu mark (now: the host-checked pick, ruling above), the speed-up counts from the crossing, `◂` is the belt's `◂`, `Esc b`/`Esc f`, 10 pt for a press, the fold after every keyboard send, any move steps the sessions. — the owner.
- 2026-11-27 — Where the head lives. Ways: (A) one head, always on the dock, over every layout; the belt head key and the rest-then-move head go: one of everything, but two shipped features leave and the terminal loses about 40 pt. (B) a new `Head` layout: low risk, but three heads with three behaviours. (C) the dock head with a toggle: an escape hatch, but a mode to learn and the old heads stay. Pick: (A). — the owner.
- 2026-11-27: spec approved. Decided by: the owner.

## Depends on

- Pocket home (PR #508, `build/pocket-home`). This work starts after #508 merges, because `BOX` and home's regions exist only there.

## Left to the builder

- The haptic for each notch, crossing, bump, hold and strike, from today's `Haptics.swift`.
- The size of the middle zone (the mockup used 48% of the radius) and the notch angle (the mockup used 15°).
- How the agent's current option is known, from the menu that `internal/menu` reads.
- How old layouts with the head key load: the key drops out, and the layout keeps its other keys. The Scroll group leaves the belt editor.
- How the head's model moves into MatchbloxKit as plain functions that tests reach, and the views only draw.

## Calls to check

- Does the output get a line highlight as in the mockup? Recommendation: no; the view scrolls and the handle row counts the lines up — because in a tmux pane the app sends wheel steps and draws no line of its own. Cost if wrong: the owner loses the "you are here" mark in a long read.
- Where does the sessions card get its list? Recommendation: a new call to the connected host for its sessions (name, agent, state), this host only — because the Live Activity state (`IslandState`) carries counts and one top session, not a list. Cost if wrong: if the list should span every paired host, the card needs a call per host.
- Does the roll round the edge end the up-and-down roll of 2026-11-19 and the sideways ruling of 2026-11-20 everywhere? Recommendation: yes; no up-and-down roll stays in Pocket — because one head has one grammar. Cost if wrong: two roll grammars to learn.
- What does a rest, then a move, on the terminal do once the called-up head goes? Recommendation: a plain scroll, as a swipe — because the hold for the right-click menu must stay. Cost if wrong: the owner loses a gesture they use.
- Does the step card read the step from the visible screen? Recommendation: yes, the nearest `✦` or `⏺` line at the top of the view, and no card when there is none — because the app reads the screen and not the agent. Cost if wrong: a card that names the wrong step.
- Is about 150 pt less terminal right, where the choice of A estimated 40 pt? Recommendation: keep it, and measure on the owner's phone that a Claude Code question with three options and the input line still fit above the head at 13 pt text — because the head must never cover the text. Cost if wrong: the question scrolls away under the head, or the head shrinks.

## Done when

- [ ] On the owner's iPhone, a console shows one head above the belt, on every layout, and no head key on the belt.
- [ ] At a Claude Code question, in the console and in the agent's pane, the owner rolls the head anticlockwise and the highlight moves up one option a notch, with a tap each; a tap answers with that option.
- [ ] If the agent's screen changes between the roll and the tap, the tap sends nothing and the handle row says `the question changed · roll again`.
- [ ] Rolling on past the first option scrolls up into the output, with one firm tap at the crossing, and a tap on the middle brings the view back down.
- [ ] After 48 notches of steady rolling, each notch moves 20 lines, the head strikes, and the step card names the `✦` or `⏺` step nearest the top of the view.
- [ ] Rolling clockwise from up in the output stops at the bottom, and the agent's choice does not move.
- [ ] A tap on the middle answers the question, with no wait.
- [ ] A roll that ends on the edge sends no key. A press on the `⌨` mark raises the keyboard. Enter sends the line, and the keyboard folds.
- [ ] A hold fills the ring, shows the sessions, and a roll and lift opens the chosen session. A lift on the current session changes nothing.
- [ ] On home, the owner sets the left mark to `Esc`. The console's left mark then reads `Esc cancel`. `undo` brings back `← cursor`.
- [ ] Dragging the belt brings it in front of the head, and it goes back behind after it stops.
- [ ] With VoiceOver on, the head reads "Match head". With a menu on screen a swipe up or down moves one option; with none it scrolls one page. A double-tap sends Enter. The four marks and "sessions" are its custom actions.
- [ ] `docs/phone.md` and the landing page's Pocket section describe one head, and name no belt head key.

## Cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 1, major 7, nit 5. Fixed 11, contested 1, for the person 0, declined 1. Second read: 15 left. Changed after the read: The experience, Anti-goals, Rulings, Left to the builder (the owner's answers to the second read). Changed after approval: The experience, Rulings, Done when (the host-checked pick, owner, 2026-11-27).

- Contested: the step card and the rest-then-move fallback are "only recommendations". They sit under Calls to check, which is where an open question belongs (builder spec rules).
- Declined (nit): done-when lines run on the owner's iPhone. A done-when line is a check the person sees; tests belong to the plan.
- Left from the second read, for the owner: lift-early Enter vs the anti-goal wording; the agent's menu selection is not read today (`internal/menu` trims the mark); where speed-up notches count from; what `◂ back` sends; the word-move bytes; a touch that moves less than a notch; when the keyboard folds; how the sessions card is rolled from the middle; plus 7 nits.
