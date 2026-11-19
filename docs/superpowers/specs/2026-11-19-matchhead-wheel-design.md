# The owner scrolls an agent's output by feel, one notch at a time, with a match head on the belt

## Who, and the moment

The owner runs agents from an iPhone with Matchblox Pocket. One hand holds the phone, often while walking. An agent wrote 300 lines, and the owner wants to read back to where it started. Today the owner swipes the terminal: the text follows the finger at one speed, and there is nothing to feel in tmux. In the owner's words (2026-11-18/19 session): "I want a vertical, tactile (triggering vibration) scroller, analog … based on the speed at which I spin the wheel (like a mouse wheel)", and "I want to feel how we scroll and feel the steps". The owner also wants "all the shortcuts matchblox asks for to be available as buttons to add within the menu".

## Why it matters

- A long read-back takes many swipes, about 20 for 300 lines in tmux at 5 lines a step with today's one-speed swipe (estimated).
- In a tmux pane a swipe gives no haptic, because ticks play only while the console shows (`ConsoleTerminalView.swift`, measured by reading the code).
- To put a console key such as `g` or `ctrl+o` on a layout, the owner types it by hand in the belt editor (measured: the editor has no list of keys).

## The experience

**The head.** In the belt editor, the owner adds the match head to a layout, like any key. It is one cell wide and sits where the owner puts it. On the belt it is a round red match head.

1. The owner rolls the head up or down with the thumb, as on a trackball. The head turns under the thumb, and its speckles move with it.
2. Each notch scrolls the terminal and plays one short, sharp tap. A roll down shows older lines, as a swipe down does today.
3. A slow roll moves 1 line a notch. A faster roll moves more lines a notch, up to 10. Taps come closer together as the roll gets faster, up to about 30 a second. Above that, the taps stay at that rate and each notch moves more lines.
4. The head shows its speed. Cold red, then ember, then amber. Rolled too fast, it **strikes**: it lights, and one rigid tap marks the strike. It cools when the roll slows.
5. A flick keeps the head turning with slower taps, and it cools out. A touch stops it.
6. A sideways roll turns the head and scrolls nothing (ruling of 2026-11-20).
7. A tap on the head without a roll sends nothing.

**With no head on the layout.** A swipe on the terminal scrolls as today, now with a tap per step in tmux too. A hold on the terminal, then a swipe, is the fast scroll: the same speed-up as the head.

**The keys.** The belt editor shows every key the console uses, grouped by where it works: in the list, in the line, in tmux. Each key shows its word (`g chain`, `m menu`, `ctrl+o expand`). A tap adds it to the layout. The match head is the first entry.

**When it goes wrong.**
- At the top or bottom of the scrollback: one heavy bump, and the head stops turning. In a tmux pane the app cannot know where the top is, so the head turns on, and tmux stays at its top line.
- A screen that has no scrollback and does not read the mouse (vim with the mouse off): the head turns, plays no tap, and the notice says `nothing scrolls here · this program has no scrollback`.
- A tap on a belt key after a scroll in tmux leaves copy mode first, then sends the key. The view goes back to the bottom.

## Non-goals

- No head in the handle row, at the screen edge, or anywhere off the belt.
- No change to the Board layout's own keys. The head can be added to Board like any layout.
- No new landscape layout. The belt stays at the bottom in landscape.
- No sound.
- No setting for the speed-up. It ships at up to 10 lines a notch, and the owner tunes it on a phone.

## Anti-goals (failure even if it works)

- A tap that the owner feels without a scroll that the owner sees, or a scroll with no tap.
- Taps that blur into a buzz at speed.
- A key that the owner did not tap reaches the agent (for example, tmux copy mode reads a belt key as a command).
- A strike that never cools, so the head stays lit.
- A hold on the terminal that was meant for the row menu or text selection starts the fast scroll instead.

## Surfaces

- An agent's tools: belt keys and wheel steps type into a live agent's pane. The harm is a key or command the owner did not mean (anti-goal: a key that the owner did not tap reaches the agent).

## Rulings

- 2026-11-19 — The scroll control is a visible, felt wheel, not a gesture laid over the belt — because the owner wants to "feel the steps", and called the belt swipe "a hacky workaround". — the owner.
- 2026-11-19 — The wheel is a match head that strikes when rolled too fast — because the owner asked for "a matchhead that we rotate and it lights up if you scroll it too fast". — the owner.
- 2026-11-19 — The head rolls like a ball (up and down, and sideways), not a dial — because the owner chose roll after trying both in the mockup. — the owner.
- 2026-11-19 — Ways for the head's place: (a) one fixed cell at the belt's end: always there, but 6 cells of keys instead of 7; (b) taller head out of the box: 7 cells, but keys under 44 pt on a 375 pt phone; (c) a key the owner places: no cost to layouts without it, but it can spin off screen. Pick: (c). — the owner.
- 2026-11-19 — With no head: swipe on the screen scrolls, and hold then swipe is the fast scroll — because the owner said so in the same answer. — the owner.
- 2026-11-19 — Keys stay 44 pt or larger — because 7 cells and a head go below 44 pt on 375 and 393 pt phones (measured from `BeltGeometry`). — proposed, not confirmed.
- 2026-11-19: spec approved. Decided by: the owner.
- 2026-11-20 — A sideways roll scrolls nothing — because SwiftTerm 1.20.0 sends wheel right (67) in SGR mode as `CSI <64;…m`, a wheel-up release, so the console scrolled up on a sideways roll (found in review). — proposed, not confirmed.

## Left to the builder

- The haptic engine (Core Haptics or UIKit), and the tap's sharpness by heat.
- How the head's turn and heat draw. The colours are the match tokens of DESIGN.md §2 (`match-head`, `match-ember`, `flame-amber`, `flame-core`).
- How the head piece is stored in a layout, and how old layouts read it.
- The notch length: about 10 pt of roll a notch.

## Calls to check

- Does a hold for the fast scroll take 0.25 s? Recommendation: yes, with movement before 0.48 s — because the console's row menu opens on a still hold at 0.48 s (`BoardTouch.longPress`), and the two must not race. Cost if wrong: the menu opens when the owner meant to scroll, or the reverse.
- Does a scroll in tmux leave copy mode on the next key? Recommendation: yes, and the view goes back to the bottom — because otherwise `q` or `/` acts on copy mode. Cost if wrong: a key goes to tmux, not to the agent.
- Is the strike at about 2400 pt/s? Recommendation: start there and tune it on the owner's phone — because the browser mockup cannot measure a thumb. Cost if wrong: it strikes too often or never.
- Do the keys in the editor's list come from the console's own key table? Recommendation: yes, one source — because a second copy of the keys drifts. Cost if wrong: the list shows keys the console no longer has.

## Done when

- [ ] On an iPhone, the owner adds the head to a layout in the editor and sees it on the belt.
- [ ] In a tmux pane with 300 lines of output, the owner rolls the head up and reaches the first line, feeling a tap per notch.
- [ ] A slow roll moves one line a notch. A hard roll moves many lines, and the taps stay distinct.
- [ ] A hard roll lights the head with one rigid tap. It goes dark within about 1 s after the roll stops.
- [ ] In a plain shell, at the top of the scrollback, the owner feels one heavy bump and the head stops.
- [ ] A tap on `g` after a tmux scroll sends `g` to the agent's line, not to tmux copy mode.
- [ ] On a layout with no head, a hold then a swipe on the terminal scrolls fast, and a still hold still opens the row menu.
- [ ] The editor lists `g chain`, `m menu` and `ctrl+o expand`, and a tap adds each one to the layout.
- [ ] With VoiceOver on, the head reads "Scroll wheel", and a swipe up or down on it scrolls one page.
