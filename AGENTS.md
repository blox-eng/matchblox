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
- matchbloxrelease owns `www/`. Send it the words and the visual. Do not edit
  the page yourself.
