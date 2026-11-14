# The stacked board at Medium

Issue: #406. Builds on: the board (#372, spec `2026-11-10-the-board-design.md`).
Mockup: the owner's artifact "Board at 80 Columns" (today next to the stacked board, two fleets).

## Why

At 80 columns the board puts `WORKING` and `SHIPPING` side by side at 20 cells each. A card has about 10 cells to say what the match does: `busy Writing the…`. In the fixed fleet (4 waits) the fourth `NEEDS YOU` card falls under the column (`↓ 1 more`). Most terminals open at 80 columns. The owner cannot read the board there.

## Owner's decisions

All dated 2026-11-14.

- At Medium the board stacks its states from top to bottom. Narrow and Wide do not change.
- One layout. No setting switches back to columns at this width. If the stack is wrong, a revert brings the columns back.
- The anti-goal "a column moves or changes width by itself while the owner reads" changes for Medium: a section may move, and only when the count of matches in `NEEDS YOU` changes.

## My calls

- `←` `→` go to the section above and below. The owner's rule "the left and right arrows move between columns" stays: at Medium the sections are the columns. `]` `[` keep their job: the next and the previous chain. (The issue draft said `]` `[`; that clashed with chains.)
- `↑` `↓` stay in the open section, as they stay in a column today. From the last row of a section, `↓` goes to the first row of the next section; from the first, `↑` goes to the last row of the section above. So `↑` `↓` reach every card in order, as the mockup showed.
- `QUIET` stays the drawer under the sections, as today.
- The chosen card in `NEEDS YOU` opens its peek, so a move of the selection there moves the sections under it. That move is the owner's own act, not one "by itself". The mockup did the same.
- `WORKING` and `SHIPPING` split their rows 3 to 2, fixed. The mockup gave `SHIPPING` only its own rows; then a new working match would push it down, against the owner's ruling.

## Non-goals

- No change to Narrow or Wide, to the card at Narrow and Wide, or to the live view.
- No new key and no setting for the layout.
- No change to `matchblox sessions --json`, `peek` or `send`.
- Pocket's belt and touch do not change. A tap on a section's name hides it, as a tap on a column's name does.

## Anti-goals

The work fails, even if it runs, when:

- A wait is not on the screen at 80×31 with up to 4 waits and the chosen card open.
- A section moves while the count of `NEEDS YOU` and the chosen card stay the same: a match that starts or stops work, a new ship, a scroll.
- A section's name is not on the screen while it has cards and is not hidden.

## The design

### The body

Top to bottom, in the rows that the columns take today (`boardRows`):

1. `NEEDS YOU · N  −`, then each card. A card has its verbose lines as today (name with harness and wait; the state word and its text; the track, `ctx` and chain stub), drawn the full width. Only the chosen card opens its peek: the quote, the options and its keys. In one-line density each card is one row.
2. A blank row, then `WORKING · N  −`, then one row a match: glyph, name, harness, the state word and its text, and at the right `ctx` and the chain stub. This is the row that QUIET draws on Wide (`boardBlockOf` with `quietRow`), with the state text.
3. A blank row, then `SHIPPING · N  −`, then one row a match: glyph, name, harness, the state word and its text, and at the right the track and the wait.
4. The `QUIET` drawer, as today.

A section's head is its own line, with its tap span over the name and the `−`. The open section's name is fg bold, as a column's head is.

### Who gets the rows

`NEEDS YOU` gets the rows of all its cards first, with the chosen card open. The rest goes to `WORKING` and `SHIPPING` in a fixed split, 3 to 2 (`WORKING` takes the odd row), whatever their counts: as a column keeps its width today. A section that does not fit ends in `↓ N more`, and scrolls so that the chosen card is whole, as a column does today. A section with fewer cards than rows leaves them blank. When `NEEDS YOU` alone does not fit, it takes the rows but 4 and scrolls; `WORKING` and `SHIPPING` keep 2 rows each: the head and one row.

So a section's place depends only on the count of matches in `NEEDS YOU` and on the chosen card: a change the owner makes, or the news the owner wants. A match that starts work, stops it or ships moves nothing.

A hidden section gives its rows to the others. A blank row goes first when rows are short, then the gap under the heads.

### Taps and keys

- A tap on a section's name hides it, as on a column's name. The hidden name waits on the tab row as today.
- A tap on a card chooses it; a tap on the chosen card goes in, as today.
- `←` `→`: the section above and below. They skip a hidden or an empty section. `↑` `↓` as in "My calls".
- The key line says `←→ sections` in place of `←→ columns`.

## Tests

- Goldens at 80×31 of the fixed fleet, in both densities, with each section hidden, narrowed to the chain, and narrowed to a host. They replace the 80×31 board goldens.
- A golden at 80×31 of a fleet with 2 waits and 13 working matches: every `NEEDS YOU` card whole, and `WORKING` ending in `↓ N more`.
- `NEEDS YOU` never ends in `↓ N more` at 80×31 with 1 to 4 waits, each with the chosen card open.
- No section's head moves when a working match's text changes, when a match moves between `WORKING` and `SHIPPING`, when `WORKING` gains or loses a match, or when the selection moves in `WORKING` or `SHIPPING`.
- `←` `→` move between sections and skip a hidden one; `↓` from a section's last row reaches the next section's first.
- A tap on each section's name hides it.
- Narrow and Wide goldens do not change.

## Not verified

- How many waits fit at 60 columns × 24 rows. The test pins 80×31.
