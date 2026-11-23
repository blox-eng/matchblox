# The owner hears that an agent release broke a screen before a builder meets it

Issue: #199 (second slice). Slice 1: `2026-11-21-unknown-screen-design.md`.

## Who, and the moment

The owner maintains matchblox. Claude Code, Codex and OpenCode each ship a new version most days.
One of them rewords its approval menu.
Today the first person to learn of it is a builder, in the morning, when a waiting session shows `unknown screen` (slice 1) and they answer it by hand.
Then the builder has to report it, the owner has to reproduce it, and a fix ships as a release.
Evidence: issue #199 ("a scheduled CI job that installs each agent's latest release ... a break opens an issue before builders hit it").
`internal/said` holds the screen rules as Go regexes. `release.yml` ships a release on each merge. `internal/update` installs a release only after its checksum and build attestation check.

## Why it matters

- Each day that a break stays unknown, every builder on that agent's new version answers its menus by hand (estimated: one to three days today, from report to fix).
- A break that slice 1 cannot see (a reworded menu that a rule still reads, but with the wrong options) picks a wrong choice. Only a test against the real screen catches it (from the parser's design).
- The fix path is already short: a merge is a published release in about 20 minutes (measured: 18–22 min on v0.55.0–v0.57.0). The slow part is finding out.

## The experience

Happy path:

1. Each day, CI installs the latest release of Claude Code, Codex and OpenCode.
2. CI starts each one in a terminal against a stand-in model, in the mode where it asks before a tool runs (Claude Code now defaults to auto mode, which asks nothing). The stand-in asks for one tool run, so the agent draws its real approval menu.
3. matchblox's own rules read that screen: the menu, the waiting state, and the options. The options read well when they are numbered 1 to n with no gap, n is two or more, the first starts with Yes and the last with No.
4. All agents read well. Nothing is said. The run is green.

When it goes wrong:

- A rule does not read an agent's screen. CI opens one issue: `Claude Code 2.1.331 draws a screen matchblox cannot read`. The issue holds the version, the check that failed (no menu, wrong options, no waiting state) and the screen as text. The fix it names: add the screen to `testdata/menus` and change the rule in `internal/said`.
- The same break happens on the next day. CI adds a comment to the open issue with the newer version, and opens no second issue.
- The agent will not start, or never draws a menu, for example because its install or its stand-in model changed. CI opens an issue that says so (`the canary could not run Codex 0.99: <reason>`), and it is not an issue about a screen. A canary that tests nothing must not be green.
- A fix merges and the next run reads the screen. The issue gets a comment that the screen reads again. The owner closes it.

## Non-goals

- Rules as data files, or rules that arrive without a release (way A). A later slice, if breaks come faster than releases.
- Local rule overrides (way C).
- `matchblox agents explain`.
- Cursor: its agent needs a signed-in account.
- A change to what a builder sees in the console.
- A change to how `[update]` installs a release.
- Closing #199. It stays open for rules without a release, the rules version in `matchblox agents`, `explain` and overrides.

## Anti-goals (failure even if it works)

- The job that runs an agent holds a token that can write to the repository, or any secret.
- A real model or a paid API key is used.
- One break opens an issue each day.
- A run that tested no agent shows as green.
- A flaky run files an issue for a screen that reads well.
- An issue body carries a home path, a host name or a user name.

## Surfaces

- CI: the job installs and runs the latest third-party agent releases, which are code the owner did not review. Anti-goal: that job holds no write token and no secret. Its token is read-only, and the checkout does not keep it in the git config, where agent code can read it. Only a second job, which checks out only matchblox's own script, without the token saved, and runs no agent code, may open or comment on an issue, from the screens that the first job uploads.
- Secrets: none. The stand-in model needs no key. Anti-goal: a real key is used.
- Checked and not touched: authentication, money, uploads, webhooks, personal data, an agent's tools on a builder's host (CI only).

## Rulings

- 2026-11-23 — The options read well when they are numbered 1 to n with no gap, n ≥ 2, the first is a Yes and the last a No. The ways were: (A) this shape with the Yes and No ends: catches a reordered menu, and a reworded end files an issue too; (B) the numbers only: fewer false issues, but misses a moved Yes. Picked A — because a moved Yes is the harm that a digit picks the wrong choice. — the owner, in the eng review.
- 2026-11-22 — The way is B: find the break early, and let a release deliver the fix. The ways were: (A) rules as data files fetched on their own: a fix with no release, but a second trust channel into the code that decides the keys; (B) a daily canary in CI that opens an issue: no new trust path, but a fix still needs a release, a gap of hours; (C) local override files: works offline, but the builder writes a regex and a bad one misreads a menu. Picked B — because the release path already ships in an hour with a provenance check, and the missing part is to know. — the owner.
- 2026-11-23 — The report job checks out matchblox's own script, without the token saved, instead of nothing — because a script in the repository has tests, and one inline in the workflow has none. — the owner, with the plan.
- 2026-11-23 — The agents install and run as an unprivileged user with no sudo, not as the runner's user. The ways were: (A) an unprivileged user: the runner's memory and its Actions token are out of the agents' reach, at the cost of a more complex job; (B) accept the risk: smallest, but a hostile agent release could write caches that main's jobs restore, the iOS release build among them; (C) a separate repository: strongest, but a new repository whose results must reach issues here. Picked A — because main's caches feed the release builds. — the owner, in the build review.
- 2026-11-23 — The canary job proves on each run that the user canary cannot read the runner's home, and blocks canary's traffic to the cloud metadata services (168.63.129.16, 169.254.169.254). The ways were: (A) a self-check and the block: each run proves it, at two more steps; (B) the self-check only: the metadata services stay open; (C) accept the image as it is. Picked A — because only a run on GitHub shows the image's permissions, so each run must show them. — the owner, in review round 2.
- 2026-11-23: spec approved. Decided by: the owner.

## Left to the builder

- The stand-in model: how each agent is pointed at it, and what it answers.
- How the screen is captured (tmux, as matchblox runs agents).
- The schedule's hour, and how a run is started by hand.
- The issue's labels and its exact body.
- Recommended: the canary is a Go test behind a build tag, with `net/http/httptest` as the stand-in and `internal/said` called directly, so it tests the shipped rules and adds no binary.

## Calls to check

- Retry before an issue. Recommend: one failed check runs again once, and only two failures in a row file an issue — because a slow runner can catch the menu mid-draw. Cost if wrong: one false issue, or a break known one run later.
- Codex and OpenCode in the first version. Recommend yes, all three — because the stand-in model costs the same for each, and each has an approval menu that `internal/said` reads. Cost if wrong: if one cannot use a stand-in model, it ships later, and the issue for it says why. Not yet proven: the eng review drew Claude Code 2.1.330's menu against a stand-in, but Codex and OpenCode are not installed on the review host.
- The fixture. Recommend that the issue names the exact file name for `testdata/menus` and holds the screen ready to paste — because the fix then starts with a failing test. Cost if wrong: the owner copies the screen by hand.

## Done when

- A run by hand on the current agent releases is green, and its log names each agent, its version and each check that passed.
- With a rule broken on purpose on a test branch (for example `cancelRe` set to match nothing), a run opens one issue that names the agent, its version, the failed check and the screen.
- A second run with the same break comments on that issue, and opens no new one.
- With an agent's install broken on purpose, the run opens a "could not run" issue and is not green.
- The job that runs the agents has read-only repository rights and no secrets, as its workflow file shows.
