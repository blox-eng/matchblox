# A builder who comes back to a long Cursor session sees its last screen, not a replay of the whole chat

## Who, and the moment

The builder, back at a Cursor session that has run for hours. They open its pane to answer a question.
The pane is now shown by a terminal of another width: the phone, a second terminal, or the same terminal after a split.
Their words (2026-11-13): "it goes through the WHOOOOOOOOOLE session, and that once took 10 minutes ... we need to start AT THE BOTTOM always".
It also starts while the pane is open: "it sometimes starts redrawing again going through the whole thing".

## Why it matters

- Each change of width costs one replay of the whole chat. A visit from a second screen is two changes: in and out.
- One replay took 10 minutes (the builder's report, not measured here).
- The cause is in Cursor's CLI (read in version 2026.10.19, `src/components/alt-screen.tsx`): when the terminal's columns change, it writes `ESC[H ESC[2J ESC[3J`, then prints every message again. A change of rows alone does not do this (measured on its sign-in screen: 9 to 15 bytes for a row, 1,833 bytes for 36 columns).
- matchblox makes it frequent: it moves a phone or a second terminal into the agent's own pane, and tmux gives the window that terminal's size (measured: 2 resizes for one visit).

## The experience

1. The builder starts or switches to a Cursor session with matchblox, as today. Nothing new to learn.
2. A terminal of another width shows the pane. Cursor wraps its text for the new width, as today.
3. The builder does not see the chat scroll by. For as long as Cursor replays, the pane shows one line: `Cursor redraws for the new width…`
4. When the replay ends, the pane shows the end of the chat and Cursor's input box. The scrollback holds the last lines of the chat.
5. Keys typed during the replay reach Cursor.

When it goes wrong:

- The wrapper cannot start: Cursor starts without it, and the pane says `Cursor runs without the redraw guard: <reason>` above Cursor's first screen.
- The wrapper ends while Cursor runs: Cursor ends with it, as a closed terminal does. The pane says `matchblox's redraw guard stopped: <reason>. Start again: matchblox switch <name> --agent cursor-agent`.
- A replay never goes quiet for 60 seconds: the pane shows what it has, and Cursor's output flows as it comes.

## Non-goals

- No fixed window size. The owner ruled it out.
- No change for Claude Code, Codex or OpenCode. Codex draws on the alternate screen by default; a resize costs it one screen.
- No guard for a Cursor that the builder started by hand, outside matchblox.
- No patch of Cursor's own files.
- No full copy of the chat in the scrollback after a width change. Cursor erases the scrollback itself at each one.

## Anti-goals (failure even if it works)

- The pane shows a screen that is not what Cursor shows: a cut line, a wrong colour that stays, a missing input box.
- A key the builder types does not reach Cursor, or reaches it late.
- matchblox no longer knows the session is Cursor: its state, its questions, its context and its account go missing in the console.
- What passes through the wrapper (the chat, a typed token, a sign-in code) lands in a file or a log.
- The pane looks frozen: no line says that Cursor redraws.

## Surfaces

- secrets — everything the builder types into Cursor and everything Cursor prints passes through the wrapper. It keeps it in memory only, and only the tail of a replay. Its anti-goal is above.
- an agent's tools — the wrapper is the terminal of an agent. It must pass keys and output unchanged outside a replay. Its anti-goals are above.

## Rulings

- 2026-11-13 — Resizing stays; no pinned window size — because the builder uses the phone, a second terminal and a resized terminal, and each must get a view that fits. — the owner.
- 2026-11-13 — The fix is for Cursor — "its for cursor". — the owner.
- 2026-11-13 — Way picked: a wrapper that keeps the tail of a replay. The ways:
  1. Tail wrapper: holds Cursor's output from its clear codes until it goes quiet, then prints the clear and the last lines. Small; depends on Cursor's clear codes staying as they are.
  2. Screen wrapper: keeps a full model of the terminal and paints its last state. Exact; a terminal emulator between every key and every byte, for one agent.
  3. Report to Cursor and wait. No code; no date, and the builder pays until then.
  Pick: 1, and 3 beside it — because 1 is the narrowest thing that helps this week and falls back to today's behaviour when Cursor changes. — the owner, with the spec.
- 2026-11-14 — The words of a guard that cannot start go in the pane, not the session's row — because the row needs a second new part, a message from the guard to the service. — the owner, with the plan.
- 2026-11-13: spec approved. Decided by: the owner.

## Left to the builder

- How the wrapper is started and named, and how the sampler still names the session's agent.
- How many lines the tail keeps, within the call below.
- How the end of a replay is told from a pause in it.
- The words for the landing page: this is a `feat`, so `www/` changes with it.

## Calls to check

- How long does a replay take when nothing draws it? Recommendation: measure first, on a signed-in Cursor with a long chat — because the wrapper removes the drawing, not Cursor's own work. Cost if wrong: the pane waits for minutes with one line on it, and the fix is Cursor's to make.
- How much chat does the scrollback keep after a width change? Recommendation: the last 1,000 lines — because the builder comes for the question at the bottom. Cost if wrong: one number.
- Send the report to Cursor? Recommendation: yes, with the two measurements and the component's name — because the real fix is there. Cost if wrong: none; it needs the owner's yes before it leaves this machine.

## Done when

- [ ] In a tmux window with a long Cursor chat, a second terminal 36 columns narrower goes in and out. The pane shows the end of the chat within 5 seconds of each change, and the chat never scrolls by.
- [ ] The same visit without the guard, measured beside it: the report gives both times and both byte counts.
- [ ] A change of rows alone prints nothing new through the guard.
- [ ] A line typed during a replay is in Cursor's input box when the replay ends.
- [ ] The console shows the session as Cursor, with its state and its context, as before.
- [ ] After a session, no file under matchblox's state folder holds a line of the chat.
