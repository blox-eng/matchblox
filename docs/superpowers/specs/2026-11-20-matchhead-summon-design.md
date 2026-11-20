# You call up the match head anywhere on the console, and tap it for Enter

## Who, and the moment

The owner reads an agent's long reply on the phone, one thumb on the screen.
The match head sits at the end of the belt, far from the thumb.
After a scroll, the owner wants to answer, and reaches down for ⏎.
Evidence: the owner, 2026-11-20, after using v0.50.0: "I should be able to click the ball for Enter",
and "hold on the screen should make the ball appear and I can then scroll up and down for a few seconds wherever I want".

## Why it matters

Each answer after a scroll is two reaches: down to the head, then to ⏎ (estimated).
A phone with no head on its belt has only the fast swipe, which gives no notches to feel.
Apple also reports the TestFlight build 0.4.0 with "one or more issues": the app ships no privacy manifest for UserDefaults.

## The experience

**A tap on the head.**
1. The head is still. The owner taps it, and lifts with no roll.
2. The head flashes, the thumb feels one firm tap, and the agent receives Enter.

**The head under the thumb.**
1. On the terminal, the owner rests a finger for a moment, then moves it.
2. The match head appears under the finger. The finger now rolls it, with the same notches, heat and strike as the head on the belt.
3. The owner lifts. The head stays where it is for about 3 seconds.
4. The owner rolls it again to scroll more, or taps it for Enter. After about 3 seconds with no touch, it fades.
5. A finger that rests and does not move still opens the right-click menu at the hold, as today.

**Ways it goes wrong.**
- The head coasts after a flick, and the owner touches it to stop it. The touch stops the coast and sends nothing.
- The program on the pane reads no scroll, and has no scrollback. The head says "nothing scrolls here · this program has no scrollback", as the belt head does. A tap still sends Enter.
- The keyboard is up. The head appears above the keyboard, never under it.

**The App Store.** Apple's email on the next build reports no missing API declaration.

## Non-goals

- No new place for the head on the belt, and no change to its look.
- No setting for the 3 seconds.
- No other key than Enter on a click: no double-click, no long-press menu on the head.

## Anti-goals (failure even if it works)

- A touch that stops a coasting head sends Enter to the agent.
- A roll that ends in a short lift sends Enter.
- The called-up head hides the line the owner reads, and stays there.
- A finger that rests to open the right-click menu also calls up the head, so the menu and the head both appear.
- The privacy manifest declares a reason that the code does not have, or misses one the code uses.

## Surfaces

- An agent's tools — a stray Enter can answer the agent's question or approve its tool call. Its anti-goals are the first two above.
- Personal data — the privacy manifest tells Apple and the owner what the app reads. Its anti-goal is the last one above.

## Rulings

- 2026-11-20 — A rest, then a move, calls up the head under the finger. It replaces the fast swipe. A rest with no move keeps the right-click and the copy menu. — because a hold already opens the agent's menu, and the fast swipe was already "rest, then move". Ways shown: rest then roll; any hold replaces the right-click; a two-finger hold. — the owner.
- 2026-11-20 — A tap sends Enter only when the head was still: no coast and no roll just before the touch, and no roll during it. — because a touch also stops a coast. — proposed, not confirmed.
- 2026-11-20 — The privacy manifest ships in the same PR as the head changes. — the owner.
- 2026-11-20 — The landing page's Pocket section and the "The match head" section of `docs/phone.md` show the head as a small drawing, and say the tap and the call-up in words. — because the head is the Pocket's most physical part, and the page names it only in words. — the owner.
- 2026-11-20: spec approved. Decided by: the owner.

## Left to the builder

- The exact "still" window before a tap counts as a click, and the most finger movement a tap may have.
- How long the called-up head waits, near 3 seconds, and how it fades.
- Where the head goes when the finger is near an edge.
- The required-reason codes for UserDefaults in the app and in the widget, from Apple's list.

## Calls to check

- Does the email name issues other than the API declaration? Recommendation: fix in this PR only those that the owner pastes from the email — because the Gmail connector cannot read it. Cost if wrong: one more build with the same email.
- Does a tap on the belt head also send Enter when no agent pane is open? Recommendation: yes, the same as the ⏎ key — because the head is a key. Cost if wrong: one stray Return in a shell.

## Done when

- On the owner's iPhone, a tap on the still head sends Enter: the agent receives the answer.
- A flick of the head, then a touch to stop it, sends nothing.
- A rest, then a move, on the terminal shows the head under the finger, and a roll scrolls with notches the thumb feels.
- After a lift, the head waits, takes a second roll or a tap, and fades about 3 seconds after the last touch.
- A still hold opens the right-click menu, and no head appears.
- The TestFlight email for the next build names no missing API declaration.
- matchblox.com's Pocket section and docs.matchblox.com's phone page show the head drawing, and their words name the tap and the call-up.
