# Agents in this repository

## Before a feat PR merges

A pull request whose title starts with `feat` changes the landing page: it
changes a file under `www/`. If the page needs no change, the body holds the
line `Landing page: unchanged because <reason>`, with a reason of 10
characters or more.

- CI's `Landing page` step (job `Conventional Commits`) blocks the merge
  until one of the two is true.
- Check it before you ask for a merge:
  `.github/scripts/landing-guard.sh "<title>" <body-file> origin/main`.
- Edit `www/` in the same PR when the feat changes what a builder can do.
  Keep the page calm and user-first (`DESIGN.md`); do not leave the landing
  for someone else.
