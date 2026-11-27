# The owner answers an agent with one thumb on a match-head joystick, and reaches for no key

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

**At rest.** Above the belt, centred, sits one match head, 96 pt across, a size that does not change. Its lower part overlaps the belt; the rest stands in a row of its own above the belt, so it never covers the terminal's text. The terminal is about 96 pt shorter (estimated from the mockup: a 166 pt dock against about 70 pt today). It looks like the real thing: a dry grit of chlorate and ground glass in glue, so it has a fine, sandy grain and no gloss. The grain only darkens, into small pits; light falls on it from the top left and stays there while the grain turns with the roll. It sits in a shallow well and casts a soft shadow, and it sinks 1 pt when pressed. It is match red at rest and warms to ember and amber as it heats, as today. It is drawn to match the mockup side by side: the same grain size, recess, shadow and colour at 96 pt. No texture or prop is drawn round it. Nothing else is drawn round it. The belt runs behind it and turns without end, as today. The handle row says `roll: options, then the output · hold: sessions · BOX sets them`. In this spec, "the handle row" is that row, above the belt.

**Where it lives.** The joystick is one layout on Pocket's Layouts switch, `Joystick`, beside Steer, tmux and Mine. On it, the dock shows the big head over the belt. On every other layout the dock is today's belt, with its small head key, as before. The owner switches the way they switch layouts today; Pocket keeps the last one.

**A touch.** The moment a finger lands on the head, a thin ring and four marks fade in round it (0.18 s, from 86 % to full size). Each mark shows a glyph and a word: top `↑ ❯ up`, bottom `↓ ❯ down`, left `← cursor`, right `→ cursor`. Nothing is sent on touch. The ring and marks fade 0.9 s after the lift. The heat colours of today stay.

**Push the middle.** The head is a thumbstick. It slides with the thumb, up to 12 pt, and springs back on the lift (0.24 s, a small overshoot). A push of 16 pt toward a mark sends its key once, with one tap.
1. `↑` and `↓` send the arrow keys: the agent's own `❯` moves, as on a keyboard. The handle row shows the option: `2 of 3 · Yes, and open a pull request`.
2. `←` and `→` move the cursor in the input line.
3. Keep pushing, and the key repeats after 0.42 s, every 0.11 s; after 9 repeats `←` `→` move a word at a time (`Esc b`, `Esc f`).
4. Come back toward the middle (under 10 pt), and the next push sends again, so a push in another direction never needs a lift.
5. Past the first option, `↑` reads up the output, a line a push; the handle row says `12 lines up · push ↓ or tap to come back`.

**Tap the middle.** A touch that moves less than 10 pt and lifts in under 0.35 s is `⏎`, at once, with no host round trip: at a question it answers the option the agent's `❯` is on. Up in the output, the tap comes back down and sends nothing.

**Roll the edge.** A finger that lands on the edge and moves round it turns the head and scrolls the output, a tick a line. Keep rolling, and it speeds up: 1 line a notch for the first 10 notches, then 3 to notch 24, then 8 to notch 48, then 20; a pause of 0.35 s starts it again at 1. A flick coasts on after the lift, slowing each frame (×0.93), with a tick a line, and stops at either end with one firm bump. While it moves 3 lines a notch or more, a card names the agent step at the top of the view. The roll never sends a key.

**Hold the middle.** A still touch fills a ring round the head for 0.6 s. Then the sessions show in a card, the current one marked. A push `↑` or `↓` steps through them, and repeats while held. Lift to go there. Lift on the current one, and nothing changes. A lift under 0.35 s is a tap: Enter. A lift between 0.35 s and 0.6 s sends nothing.

**The belt.** Drag the belt, and it comes in front of the head: the head fades, and every key shows. 1.4 s after the belt stops, it slips back behind the head. A tap on a belt key works as today. On `Joystick`, the belt carries `⌨` (the keyboard) and `◂` (back), which used to be marks. After every Enter sent from the keyboard, the line is sent first, then the keyboard folds.

**Motion.** Every reveal and hide is a fade with a short scale, never a jump: marks and ring 0.18 s in, out 0.9 s after the lift; the head's spring-back 0.24 s with a small overshoot; the belt forward 0.25 s, back after 1.4 s; the keyboard 0.25 s; home 0.38 s on `cubic-bezier(.2,.8,.2,1)`; the step card 0.6 s after the last fast notch. Reduce Motion drops the scales and the spring and keeps the fades. The mockup is the reference: the build matches it side by side.

**Set it on home.** Open the drawer, tap `BOX`, and home opens (the home of `2026-11-24-pocket-home-design.md`, PR #508). A region `THE HEAD` sits under `LIVE ACTIVITY`. It draws the head with its four marks and its hold. Tap one, and a list of keys shows, each with its glyph and word: `↑ ❯ up`, `↓ ❯ down`, `◂ back`, `⌨ type`, `← cursor`, `→ cursor`, `Esc cancel`, `Tab tab`, `⇧Tab mode`, `⇞ page up`, `⇟ page down`, `/c compact` (types `/compact` with no Return), `· nothing`, and for the hold only `⇄ sessions`. Tap a key to set it. `undo` takes back the last change. `as it came` brings back the first marks. `^C` is in the list, dim, with the reason: `stays on the belt: a roll can brush the edge`. The region also holds `its colour`: five small heads, `match red` (as it came), `strike-anywhere brown`, `blue tip`, `green tip` and `charcoal`. A tap on one colours the dock's head at once. The colour changes only the cold head; the heat colours stay.

**When it goes wrong.**
- No menu on screen: the roll goes straight into the output.
- A program with no scrollback that does not read the mouse: the head turns, and the handle row says `nothing scrolls here · this program has no scrollback`, as today.
- In a tmux pane the app knows the bottom, not the top: the head turns on at the top, as today.
- The keyboard is up: the head is under the keyboard, and the belt stays above it, as today.

**Heuristics check** (owner, 2026-11-27: "nielsen heuristics apply here").
- Status: the handle row always says what a push and a roll do now. The hold ring fills before the sessions show.
- Real world: a thumbstick you push, an edge you roll, a centre you press, a home where settings live. Plain words: back, cursor, type, sessions.
- Control: a short tap is Enter, a lift in the hold ring sends nothing, lift on your own session is nothing, home has undo and as it came.
- Consistency: one head in all of Pocket. The glyphs match the belt's; the words are new and short. Settings live only on home.
- Error prevention: where the finger lands decides the meaning: the middle is the stick, the edge is the wheel. A roll never sends a key; a push sends one key per push.
- Recognition: the marks show on touch, before a press sends. Home draws the head with every mark.
- Flexibility: `Joystick` is a layout to pick, and the old belt stays. The first marks work on day one; the owner's own marks and hold come later.
- Minimal: at rest, one head. No texture, no counter, no drawn flame.
- Errors: a bump and a reason at each end.
- Help: the handle row at rest says what a roll and a hold do, and where to set them.

## Non-goals

- No tap-then-roll, no double tap, no circle gestures as commands. A push is a direction, nothing more.
- No cardboard, striker paper, wooden stick, flame or counter on the dock. The head's own grain is the only texture.
- No setting for the size, the grain or the depth. The colour is the head's only look setting.
- No gloss, sheen or highlight spot on the head.
- No second switch: `Joystick` is a layout, and the Layouts switch is the only way in and out.
- No setting for the speed-up, the notch size or the fade times.
- No change to the console on the desk.

## Anti-goals (failure even if it works)

- A roll on the edge sends a key, or a tap after a push sends Enter.
- A push sends a second key without the thumb coming back toward the middle or repeating on purpose.
- The owner loses today's belt and its small head: any layout but `Joystick` looks and works as before.
- A lift after 0.35 s, or a lift after a roll, sends Enter.
- A hold, or any touch on the head, changes the owner's settings.
- The head hides the agent's question or the owner's input line, and stays there.
- The sessions card goes to a session other than the one marked when the finger lifts.
- `^C` reaches the agent from the head.
- The keyboard folds before the line is sent, and the typed text is lost.

## Surfaces

- An agent's tools: rolls, taps and marks type into a live agent's pane. A stray Enter or arrow can approve a tool call: a push sends one key per push, a tap sends Enter only under 10 pt and 0.35 s, and a roll sends no key. Their anti-goals are the first, second, fifth and sixth above.
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
- 2026-11-27 (after TestFlight build 9) — The head is a joystick: push `↑` `↓` to move the agent's own `❯`, `←` `→` for the cursor, a tap is `⏎`, a roll of the edge scrolls with momentum, a hold opens the sessions. The edge-roll walk through Pocket's own highlight and the host-checked pick leave the head — because the owner said "i need movements for these like a joystick, tap to click and enter", and found the roll "not natural". Supersedes the push-to-steer drop and the host-checked pick rulings above. — the owner (mockup v15: "that looks about right").
- 2026-11-27 — `Joystick` is an option, a layout on the Layouts switch; every other layout keeps today's belt and its small head key — because the owner said "i want the new view to be an option not the only way to navigate". Supersedes way A's "one head on every layout". — the owner.
- 2026-11-27 — Reveal, hide and spring timings are the mockup's, written under Motion — because the owner said the build's animations must be "more seamless like in the artifact". — the owner.
- 2026-11-27 — With the tap as plain Enter, Pocket no longer reads the menu, and the phone's key stops running `menu` and `choose`; `sessions --json` stays for the hold. The two commands stay for scripts. Proposed, not confirmed: the smaller key surface; cost if wrong, a later pick by number needs the gate rule back.
- 2026-11-27 — The head is 96 pt, fixed, with no size setting — because the owner said "the size is the smallest available, no need to even give me the range". — the owner.
- 2026-11-27 — The head is drawn as its real material: medium grain, no gloss, a touch of depth (a shallow well, a soft shadow, a 1 pt sink). Ways shown in the mockup: soft matte, satin, glazed ceramic, each at a touch and a lot of depth; then fine, medium and coarse grain. Pick: medium grain, a touch of depth — because a real match head is chlorate and ground glass in glue (RSC; US737672), which scatters light and grips, and the owner said gloss "is not a feature this material would have". The grain only darkens, because a grain that lightens read as skin. — the owner.
- 2026-11-27 — The colour is set on home, under `THE HEAD`: match red (as it came), strike-anywhere brown, blue tip, green tip, charcoal. Match red is a deeper red than `Theme.matchHead` today — because the owner asked for "a different pigmentation" and then "the color change selector to the home screen". — the owner.

## Depends on

- Pocket home (PR #508, `build/pocket-home`). This work starts after #508 merges, because `BOX` and home's regions exist only there.

## Left to the builder

- The haptic for each notch, crossing, bump, hold and strike, from today's `Haptics.swift`.
- The size of the middle zone (the mockup used 48% of the radius) and the notch angle (the mockup used 15°).
- How the agent's current option is known, from the menu that `internal/menu` reads.
- How old layouts with the head key load: the key drops out, and the layout keeps its other keys. The Scroll group leaves the belt editor.
- How the head's model moves into MatchbloxKit as plain functions that tests reach, and the views only draw.

## Calls to check

- Where does the sessions card get its list? Recommendation: a new call to the connected host for its sessions (name, agent, state), this host only — because the Live Activity state (`IslandState`) carries counts and one top session, not a list. Cost if wrong: if the list should span every paired host, the card needs a call per host.
- Does the roll round the edge end the up-and-down roll of 2026-11-19 and the sideways ruling of 2026-11-20 everywhere? Recommendation: yes; no up-and-down roll stays in Pocket — because one head has one grammar. Cost if wrong: two roll grammars to learn.
- What does a rest, then a move, on the terminal do once the called-up head goes? Recommendation: a plain scroll, as a swipe — because the hold for the right-click menu must stay. Cost if wrong: the owner loses a gesture they use.
- Does the step card read the step from the visible screen? Recommendation: yes, the nearest `✦` or `⏺` line at the top of the view, and no card when there is none — because the app reads the screen and not the agent. Cost if wrong: a card that names the wrong step.
- Is about 96 pt less terminal right, where the choice of A estimated 40 pt? Recommendation: keep it, and measure on the owner's phone that a Claude Code question with three options and the input line still fit above the head at 13 pt text — because the head must never cover the text. Cost if wrong: the question scrolls away under the head, or the head shrinks.

## Done when

- [ ] On the owner's iPhone, the Layouts switch offers `Joystick`. On it the dock shows the big head over the belt; on Steer, tmux and Mine the belt and its small head key work as before.
- [ ] At a Claude Code question, the owner pushes `↓` once and the agent's `❯` moves to option 2, with a tap; a tap on the middle answers it.
- [ ] A push held `↓` repeats after about half a second; coming back toward the middle and pushing `↑` moves the `❯` back up without a lift.
- [ ] Past the first option, `↑` reads up the output; a tap comes back down and sends nothing.
- [ ] A roll of the edge scrolls the output; a flick coasts and stops at the bottom with one bump; the roll sends no key.
- [ ] `←` `→` move the input line's cursor; `⌨` on the belt raises the keyboard; Enter sends the line, then the keyboard folds.
- [ ] A hold fills the ring and shows the sessions; a push `↓` and a lift opens the chosen session; a lift on the current one changes nothing.
- [ ] Next to the mockup on the owner's phone, the head, the marks' fade in and out, the spring-back and the belt coming forward look and move the same.
- [ ] On home, the owner sets a mark and the colour (`blue tip`); the dock's head changes at once and keeps it after a restart.
- [ ] With VoiceOver on, the head reads "Match head"; a swipe up or down sends `↑` or `↓`; a double-tap sends Enter; the four marks and "sessions" are its custom actions.
- [ ] `docs/phone.md` and the landing page describe `Joystick` as a layout you can pick.

## Cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 1, major 7, nit 5. Fixed 11, contested 1, for the person 0, declined 1. Second read: 15 left. Changed after the read: The experience, Anti-goals, Rulings, Left to the builder (the owner's answers to the second read). Changed after approval: The experience, Non-goals, Anti-goals, Surfaces, Rulings, Done when (the joystick, an option, its motion; owner, 2026-11-27, after TestFlight build 9); The experience, Rulings, Done when (the host-checked pick, owner, 2026-11-27); The experience, Non-goals, Rulings, Calls to check, Done when (96 pt fixed, the head's material and depth, its colour on home, owner, 2026-11-27).

- Contested: the step card and the rest-then-move fallback are "only recommendations". They sit under Calls to check, which is where an open question belongs (builder spec rules).
- Declined (nit): done-when lines run on the owner's iPhone. A done-when line is a check the person sees; tests belong to the plan.
- Left from the second read, for the owner: lift-early Enter vs the anti-goal wording; the agent's menu selection is not read today (`internal/menu` trims the mark); where speed-up notches count from; what `◂ back` sends; the word-move bytes; a touch that moves less than a notch; when the keyboard folds; how the sessions card is rolled from the middle; plus 7 nits.
- Not cold-read after the joystick change (2026-11-27): the owner tried the behaviour in the mockup (v15) first; the plan change that follows gets its cold read.
