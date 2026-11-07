# Landing page: one matchbox, one story

Date: 2026-11-07. Owner: matchbloxrelease (www/index.html). Status: draft for Martin.

## Why

Martin, 2026-11-07: "rethink each section against SEO UX and minimalistic
websites design practices". Two defects started it:

- #327 added the line "Tap a match to light it…" under the chain. The matches
  must invite the tap themselves.
- The chain lede opens with "Two agents in one repo". Since v0.34.0, chains
  across repos are the news.

## Who it is for

People who live in the terminal, like the matchbox, match and fire metaphor,
and enjoy small, well-made software (Martin, 2026-11-07). Most of them arrive
by a shared link (Hacker News, lobste.rs, Discord, GitHub), not by search.

## Goals

- Every section is one beat of the metaphor: a heading, one line, and its live
  visual. No card grids.
- Visible words drop from 862 (measured on the live page, excluding the
  terminal replays) to about 430.
- A shared link shows a preview card: Open Graph and Twitter tags, plus a
  1200×630 image.
- One H1. Each beat has an H2 that names what it does. The `<title>` carries
  the words "terminal" and "coding agents".

## Non-goals

- No change to the visual system (fonts, colours, the fold, the matchbox
  canvas).
- No feature that has not shipped. One-way chains and the chain room (#326)
  join the page when they merge.
- No fetched counts such as GitHub stars, no testimonials, no analytics.

## Anti-goals

- The metaphor turns into decoration that adds words.
- Instructions placed next to an interactive element instead of affordance.

## Approaches (Martin picked B, 2026-11-07)

- A. Trim in place: cut duplicates, keep the structure. Small, but the page
  still reads as a list of features.
- **B. One story: Strike → The box → The chain → The night → The pocket →
  Strike one.** The metaphor carries the structure and halves the page.
- C. One screen: hero, console, install. Most minimal, but it hides chains
  and the phone app, which set matchblox apart.

## The page, beat by beat

| Beat | Keeps | Changes |
|---|---|---|
| **Head** | description meta | `<title>`: "matchblox · a terminal console for every coding agent". Add `og:*` and `twitter:card` tags and `/assets/og.png` (the mark plus the H1, rendered from HTML at 1200×630). |
| **Strike** (hero) | H1 "Light an idea. Keep it burning.", news eyebrow, works-with row, install | Sub becomes one line: "One terminal console for every coding agent you run. See who waits for you and answer in a line." The carried-matchbox sentence goes. |
| **The box** | console replay, its caption | Add a visible H2: "Every agent, one box". One note line takes the one fact worth keeping from the cut agents section: context and account show for Claude Code, Codex, OpenCode and Cursor, and any agent is added by its command. |
| **The chain** | the match row, `matchblox chain <who>` | The tip text goes now. Affordance: the first match flickers until a visitor touches the row, and the hover lifts the match under the pointer. The words (lede and 3 beats) come from matchblox-chain. The Option-s flip comes from matchblox-ux after #326. |
| **The night** | the 6-step stoker stage | One section instead of two. H2: "Keep it burning while you sleep". Lede: "Press n, say what to keep going, and sleep. The stoker works your sessions through the night and leaves a card for the morning. Merges, releases and deploys wait for your word." One link: "The stoker →". The second stoker section and the ways note go. |
| **The pocket** | H2, the 3 blocks | Each block is one sentence. |
| **Strike one** (new) | — | H2 "Strike one". The install command again, with copy, plus Docs and GitHub links. |
| Footer | as is | — |

Cut: the "Every agent you run, in one list" section and the four "What it does"
cards. The console replay already shows the list, the PR track and usage; the
docs carry the keys.

## Done when

- Martin approves a mockup first (his "Mockup first" ruling).
- The page has the six beats, one H1 and an H2 for each beat. No text tells a
  visitor how to use an interactive element.
- The words count about 430, by the same measure as the 862.
- The OG tags validate, and `og.png` loads at 1200×630.
- At 390 and 1440 pixels wide: no horizontal scroll, and the chain lights from
  a tap and from the keyboard (Enter).
- The repo's site, docs and release tests pass.

## Left to the builder

- The exact flicker timing and the hover lift.
- How to render `og.png` (a Playwright screenshot of a small HTML card,
  committed).
- The one-sentence wording of the pocket blocks, within the voice above.
