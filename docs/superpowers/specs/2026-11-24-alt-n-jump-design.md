# A builder jumps to any of their first nine sessions with one key, from the console or from inside an agent

Issue: #194.

## Who, and the moment

A builder runs six to nine agents on one host, in tmux, with the matchblox console in its home session.
They are inside one agent's pane, and a notice says that another session needs them.
To get there today they press `ctrl+q` to reach the list, then `↑` `↓` down the rows until they reach it.
The list's order changes as sessions work and wait, so the row is not where it was a minute ago.
Evidence: issue #194. `internal/app/keys.go` has no key that goes to a given session. `docs/keys.md` says a plain digit answers the question on screen. `internal/panes/flip.go` binds `⌥s` and `⌥n` in the panes of sampled agents, and hands the key to the console.

## Why it matters

- Each jump costs `ctrl+q`, then 1 to 8 presses of `↓`, then a look at each row: about 3 to 10 keys and a few seconds (estimated, from the list's size).
- A builder who jumps between agents 50 times a day spends minutes on it, and loses their place each time the order moves (estimated).

## The experience

Happy path:

1. Each session row in the console's list shows a number, 1 to 9, in a dim column at the left.
2. A number stays with its session while the session lives. A session that ends frees its number, and the next new session takes the lowest free number. The other sessions keep theirs.
3. In the console, in the list or in the chat's line, `⌥1` … `⌥9` chooses that session, as `↑` `↓` to its row does. On a desk in tmux, the agent beside the list shows it.
4. Inside a sampled agent's pane, `⌥1` … `⌥9` goes straight to that session's pane, the way `⌥s` reaches the room.
5. The phone's console shows the same numbers on the same sessions. A tap on a row opens it, as today.
6. The key line names the key once, as `⌥1-9 jump`. `docs/keys.md` lists it.

When it goes wrong:

- No session has that number. In the console, a notice says `no session 7: the list numbers 1 to 5`. In a pane, tmux shows the same words, and nothing types.
- A session past the ninth has no number. It is reached as today.
- In a pane that is not a sampled agent's (a shell, an editor), `⌥1` … `⌥9` go on as typed, as `⌥s` does there.
- A terminal that does not send Option as Meta types a symbol for `⌥1`. `docs/keys.md` says how to set Option as Meta, as it does for `⌥s`.

## Non-goals

- Numbers past 9, or two-digit jumps.
- A prefix-key fallback for terminals without Meta.
- Numbers in tmux's status line or window names.
- Reordering, pinning or renaming numbers by hand.
- A new phone button. A tap on a row already opens it.

## Anti-goals (failure even if it works)

- A digit typed into an agent jumps away. On the phone, an `Esc` then a quick digit (cancel, then pick option 1) becomes `⌥1`, and the builder lands in another session with their answer lost.
- A number moves to another session while both live, so `⌥3` opens a session the builder did not mean.
- `⌥1` … `⌥9` stop working in a shell or an editor pane, which use them.
- The desk and the phone show different numbers for the same session.

## Surfaces

- An agent's tools: the binding sits in the panes where matchblox types answers. Its anti-goal is "a digit typed into an agent jumps away".
- No other sensitive surface touched. Checked: authentication, money, uploads, webhooks, secrets, CI, personal data.

## Rulings

- 2026-11-24 — Ways shown: (A) `⌥N` in the console and as a tmux key in each sampled agent's pane, through the same hand-off as `⌥s`; (B) the console only, so from a pane it is `ctrl+q` then `⌥N`; (C) the console, plus prefix, a leader and a digit from a pane, for terminals without Meta. Pick: A. — because the builder sits in an agent's pane most of the time, and `⌥s`/`⌥n` already prove the hand-off. — the owner.
- 2026-11-24 — The issue's "plain 1 to 9 switch tabs" is out of date. Plain digits now answer the question on screen, so the jump keeps `⌥`. — the evidence in `docs/keys.md`.

- 2026-11-25: spec approved, with the four calls to check as recommended. Decided by: the owner.

## Left to the builder

- The hand-off message's kind and its format.
- The column's width and style, inside `DESIGN.md`'s row anatomy.
- How the 9 bindings are generated beside `FlipKeysArgv`.

## Calls to check

- Who owns the numbers? Recommend the service, per host, kept in its state while a session lives — because the desk and the phone each run their own console, and only the service sees both. Cost if wrong: two numbers for one session (an anti-goal).
- Sessions of other hosts in the "all hosts" view. Recommend that they show their own host's numbers, and that `⌥N` acts on the chosen session's host — because a jump from a pane goes to that host's console. Cost if wrong: `⌥3` picks a session on another host.
- The phone's `Esc` then a digit. Recommend that the pane binding does not jump for a phone client (the phone has no `⌥` key), the same way the console's line already types the letter for it (`internal/app/sidekeys.go`). Cost if wrong: the anti-goal above.
- A console that restarts. Recommend that numbers survive it, since the service keeps them. Cost if wrong: numbers shuffle after an update.

## Done when

- With 5 sessions, `⌥3` in the console's list chooses the session numbered 3, from the line or the list, with no other key.
- Inside the pane of session 1, `⌥4` shows session 4's pane, and `⌥4` typed in a shell pane types as before.
- Session 2 ends. Sessions 1, 3, 4 and 5 keep their numbers, and the next new session takes 2.
- The phone's list shows the same number for each session as the desk.
- On the phone, `Esc` then `1` in an agent's pane reaches the agent, and no jump happens.
- `TestKeymapsHaveNoCollisions` passes with the new keys, and `docs/keys.md` lists `⌥1-9`.
