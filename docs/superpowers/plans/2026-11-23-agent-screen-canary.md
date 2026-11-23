# Plan: the owner hears that an agent release broke a screen before a builder meets it

Spec: docs/superpowers/specs/2026-11-22-agent-screen-canary-design.md
Spec approved: 2026-11-23
Test command: go vet ./... && go test ./...
Plan approved: 2026-11-23 by the owner
Next: builder:build

## What the code says today

- `internal/said` holds the rules under test: `Screen` (kind `Permits`), `Menu` (a permits prompt whose last line matches `cancelRe`), and the unexported `option(line) (n, label, ok)` and `menuLines(screen)` in `menu.go`.
- The eng review (2026-11-22) drew Claude Code 2.1.330's real Bash approval box against a stand-in Anthropic API in tmux, with `--permission-mode default`, `hasCompletedOnboarding` and `customApiKeyResponses.approved` set to the key's last 20 characters. `said.Screen` read it as `Permits`, and `said.Menu` read it as ok.
- `internal/tmuxtest.Server` skips a test when tmux is missing. The canary must fail there instead: a skip would be a green run that tested nothing.
- `.github/scripts/*.sh` are tested from Go in `internal/release/scripts_test.go` through `script(t, name, args...)`.
- `ci.yml` pins every action by commit hash, sets `permissions: contents: read` at the top, and checks out with `persist-credentials: false`.
- Codex, OpenCode and node are not installed on the build host. Their tasks are proven in CI (Task 7), not locally.

## File map

- `internal/said/canary_check_test.go` (create) — `canaryCheck(screen) (check string, ok bool)` and `scrub(screen, work, home)` with their tests: the verdict on one screen. Runs in the normal suite.
- `testdata/menus/claude-bash-2.1.330.txt` (create) — the live screen from the eng review, scrubbed.
- `internal/said/canary_test.go` (create, `//go:build canary`) — `TestCanary`: for each agent, start it in tmux against its stand-in, wait for the menu, retry once, write one result file.
- `internal/said/canary_agents_test.go` (create, `//go:build canary`) — each agent's install check, version, config files, start command and stand-in model (`net/http/httptest`).
- `.github/scripts/canary-report.sh` (create) — reads the result files and opens, comments on, or leaves the canary issues.
- `internal/release/scripts_test.go` (change) — tests of `canary-report.sh`, with a fake `gh` on PATH.
- `.github/workflows/canary.yml` (create) — the daily run: job `canary` (runs agents, read-only) and job `report` (writes issues, runs no agent).
- `docs/agents.md` (change) — one paragraph under "How an agent tells that it waits": the daily canary and what its issue means.

## Tasks

### Task 1: a screen's verdict

- **Files:** `internal/said/canary_check_test.go`, `testdata/menus/claude-bash-2.1.330.txt`.
- **Test first:** `TestCanaryCheck`, a table:
  | screen | check | ok |
  |---|---|---|
  | `claude-bash-2.1.330.txt` | `""` | true |
  | that screen with the last line removed | `no menu` | false |
  | that screen with option 4 renumbered 5 | `wrong options` | false |
  | options 1–4 with "2. Yes" and "1. Allow" swapped | `wrong options` | false |
  | that screen with the last option "4. Never" | `wrong options` | false |
  | a single "1. Yes" option over the hint | `wrong options` | false |
  | an idle prompt (`❯ ` and a footer) | `no waiting state` | false |
- **Run:** `go test ./internal/said -run TestCanaryCheck`. Before: fails to build, `undefined: canaryCheck`. After: passes.
- **Do:** write `canaryCheck(screen string) (string, bool)`. `Screen(screen)` not `Permits` → `"no waiting state"`. `Menu(screen)` not ok → `"no menu"`. Collect the option rows of `menuLines(screen)` with `option`: the numbers must run 1..n with no gap, n ≥ 2, label 1 starts with `Yes`, label n starts with `No` (case-insensitive) → otherwise `"wrong options"`. Write the fixture from the eng review's screen (4 options, the "Tip: auto mode" line, footer "Esc to cancel · Tab to amend"), with the work folder as `<work>`.
- **Done when:** `TestCanaryCheck` passes, and the full suite passes.
- **Commit:** `test(said): a verdict on one agent screen for the canary (#199)`

### Task 2: the screen leaves the run with no path or name

- **Surface:** personal data (spec anti-goal: an issue body carries a home path, a host name or a user name).
- **Files:** `internal/said/canary_check_test.go`.
- **Test first (hostile input first):** `TestScrubRemovesTheRunsFolders`. Input: a screen holding `/home/runner/work/_temp/canary/work`, `/home/runner/.claude`, `runner@fv-az123`, Assert that none of `work`, `home`, `runner`, `fv-az123` is in `scrub(screen, work, home, "fv-az123", "runner")`, and that `<work>`, `<home>` and `<user>@<host>` are. `TestScrubKeepsShortNames`: an empty, 1- or 2-character host and user leave the screen unchanged.
- **Run:** `go test ./internal/said -run TestScrub`. Before: `undefined: scrub`. After: passes.
- **Do:** `scrub(screen, work, home, host, user string) string` replaces `work` with `<work>` and `home` with `<home>` (longest first), then `host` with `<host>` and `user` with `<user>` when each is 3 characters or more. Task 3 passes `os.Hostname()` and the `os/user` name.
- **Done when:** `TestScrubRemovesTheRunsFolders` and `TestScrubKeepsShortNames` pass.
- **Commit:** `test(said): the canary's screens carry no path, host or user (#199)`

### Task 3: Claude Code's canary run

- **Files:** `internal/said/canary_test.go`, `internal/said/canary_agents_test.go`.
- **Test first:** `TestCanary/claude` under `-tags canary`, with `MATCHBLOX_CANARY_OUT` set to a temp folder. It asserts that `claude.json` there holds `{"agent":"Claude Code","version":"<x.y.z>","ok":true,"check":"","screen":"…"}`, and that the stand-in received exactly one request with a `Bash` tool.
- **Run:** `MATCHBLOX_CANARY_OUT=$(mktemp -d) go test -tags canary ./internal/said -run TestCanary/claude -v`. Before: no test runs (`testing: warning: no tests to run`). After: passes in under 30 s on the build host, which has Claude Code 2.1.330.
- **Do:**
  1. `canary_agents_test.go`: type `agent struct { key, name, bin string; version func() (string, error); setup func(home, work, url string) error; argv func() []string; stand http.Handler }`. Claude Code: `bin` "claude"; version from `claude --version` (first field, must match `^\d+(\.\d+){1,3}$`); setup writes `<home>/.claude.json` with `hasCompletedOnboarding`, `customApiKeyResponses.approved` = the last 20 characters of the fake key, and the work folder trusted; argv `claude --permission-mode default "make the canary file"`; env `HOME`, `PATH`, `TERM=xterm-256color`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=sk-ant-canary-…`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `DISABLE_AUTOUPDATER=1`. The stand-in answers `POST /v1/messages` as SSE with one `tool_use` Bash `{"command":"touch canary-ran"}` when the request has a `Bash` tool and no `tool_result`, else a text "Done."; it answers `count_tokens` and any GET with a small JSON body.
  2. `canary_test.go`: `TestCanary` takes the agents named in `MATCHBLOX_CANARY_AGENTS` (comma list, default all). For each: a missing tmux or a missing binary is `t.Errorf` and a `could not run` result, never `t.Skip`. Start tmux on its own `-L` socket, 120×40, in `work`, with `env -i` and the agent's env only. Poll `capture-pane -p` each 500 ms for 30 s until `canaryCheck` is ok; on a timeout, kill the server and run it once more from a clean home. Write `<out>/<key>.json` with the last screen through `scrub`. A run where no agent was tried fails.
- **Done when:** the run above passes locally, and `claude.json` holds `ok: true` and a version.
- **Commit:** `test(said): a canary run draws Claude Code's real approval menu (#199)`

### Task 4: Codex's canary run

- **Files:** `internal/said/canary_agents_test.go`.
- **Test first:** `TestCanary/codex` under `-tags canary`, as in Task 3, expecting `codex.json`.
- **Run:** in CI only (Task 7), since Codex is not on the build host. Locally, `MATCHBLOX_CANARY_AGENTS=codex go test -tags canary ./internal/said -run TestCanary` fails with `could not run Codex: codex is not installed`. That failure is the expected local result, and it proves that a missing agent is not green.
- **Do:** Codex: `bin` "codex", version from `codex --version`. setup writes `<home>/.codex/config.toml` with a `model_providers.canary` (`base_url` = the stand-in, `wire_api = "responses"`, `env_key = "CANARY_KEY"`), `model_provider = "canary"`, `approval_policy = "untrusted"`, and the work folder trusted. The stand-in answers `POST /v1/responses` as SSE with one `function_call` to `shell` running `touch canary-ran`.
- **Done when:** the local run fails with the "could not run" message, and Task 7's CI run writes `codex.json` with `ok: true`.
- **Commit:** `test(said): a canary run draws Codex's approval menu (#199)`

### Task 5: OpenCode's canary run

- **Files:** `internal/said/canary_agents_test.go`.
- **Test first:** `TestCanary/opencode`, as in Task 4, expecting `opencode.json`.
- **Run:** as in Task 4: locally `could not run OpenCode: opencode is not installed`.
- **Do:** OpenCode: `bin` "opencode", version from `opencode --version`. setup writes `<work>/opencode.json` with a provider `canary` (`npm: "@ai-sdk/openai-compatible"`, `options.baseURL` = the stand-in + `/v1`), `model` "canary/stand-in", and `permission.bash = "ask"`. The stand-in answers `POST /v1/chat/completions` as SSE with one `tool_calls` entry `bash {"command":"touch canary-ran"}`.
- **Done when:** the local run fails with the "could not run" message, and Task 7's CI run writes `opencode.json` with `ok: true`.
- **Commit:** `test(said): a canary run draws OpenCode's approval menu (#199)`

### Task 6: the report opens one issue per break

- **Surface:** CI (spec anti-goals: one break opens an issue each day; a token reaches agent code). The result files are written by a job that ran untrusted agent code, so the script treats each field as hostile data.
- **Files:** `.github/scripts/canary-report.sh`, `internal/release/scripts_test.go`.
- **Test first (hostile input first):** `TestCanaryReportTreatsResultsAsData`. A result with `version` `1.0$(touch pwned)` and a `screen` with `` `touch pwned2` `` and `$(touch pwned3)`. Assert: no `pwned*` file exists after the run, the fake `gh` got no `issue create`, and the output says `refused: Claude Code version "1.0$(touch pwned)" is not a version`.
  Then these, each with a fake `gh` on PATH that logs its arguments and serves a canned `issue list`:
  - `TestCanaryReportOpensOneIssue`: one failed result, no open issue → one `issue create --label canary --title "Claude Code 2.1.331 draws a screen matchblox cannot read" --body-file …`. The body holds the version, the check, the screen in a code block, the fixture name `testdata/menus/claude-bash-2.1.331.txt`, and the marker `<!-- canary claude 2.1.331 fail -->`.
  - `TestCanaryReportCommentsOnANewerVersion`: the open issue's last marker is `claude 2.1.331 fail`; the result is 2.1.332 failed → one `issue comment`, no `issue create`.
  - `TestCanaryReportStaysQuietOnTheSameVersion`: the last marker is `claude 2.1.331 fail`; the result is 2.1.331 failed → no `gh` write.
  - `TestCanaryReportSaysItReadsAgain`: an open issue, the result ok → one comment `matchblox reads Claude Code 2.1.332 again.`, with marker `pass`; a second run with that marker writes nothing.
  - `TestCanaryReportCouldNotRun`: a result with check `could not run` → an issue titled `the canary could not run Codex: codex is not installed`, and exit 1.
  - `TestCanaryReportNeedsResults`: an empty folder → exit 1, `no canary results: the canary tested nothing`.
- **Run:** `go test ./internal/release -run TestCanaryReport`. Before: fails, the script does not exist. After: passes.
- **Do:** `canary-report.sh <results-dir>`: `set -euo pipefail`. For each `*.json`, read fields with `jq -r`. Refuse a version that does not match `^[0-9]+(\.[0-9]+){1,3}$` and an agent key outside `claude|codex|opencode`. Never pass a field through `eval` or an unquoted expansion. Write the body to a temp file and pass `--body-file`. Find the open issue with `gh issue list --label canary --state open --search "<name> in:title" --json number,body,comments`, and take the last `<!-- canary … -->` marker from the body and its comments. Exit 1 when any result is not ok.
- **Done when:** all seven tests pass.
- **Commit:** `ci: the canary report opens one issue per screen break (#199)`

### Task 7: the daily workflow

- **Surface:** CI (spec anti-goals: the agent job holds a write token or a secret; a real key is used).
- **Files:** `.github/workflows/canary.yml`, `internal/release/scripts_test.go`.
- **Test first (hostile input first):** `TestCanaryWorkflowKeepsTheTokenFromAgents`. It parses `canary.yml` (YAML via the module's existing YAML package, or line rules if there is none) and asserts:
  - The job `canary` has `permissions: contents: read` and no `issues`.
  - Each of its `actions/checkout` steps has `persist-credentials: false`.
  - No step in it names `secrets.`.
  - Every `uses:` is pinned to a 40-character hash.
  - The job `report` has `issues: write`, runs only `canary-report.sh`, and has `if: github.event_name != 'pull_request'`.
- **Run:** `go test ./internal/release -run TestCanaryWorkflow`. Before: fails, no `canary.yml`. After: passes.
- **Do:** `on: schedule: cron "17 5 * * *"`, `workflow_dispatch`, and `pull_request` with paths `internal/said/**`, `.github/workflows/canary.yml`, `.github/scripts/canary-report.sh`. Job `canary` (ubuntu-latest):
  - checkout (pinned, `persist-credentials: false`) and setup-go and setup-node (pinned).
  - `sudo apt-get install -y tmux`.
  - `npm install -g --ignore-scripts=false @anthropic-ai/claude-code@latest @openai/codex@latest opencode-ai@latest`.
  - `MATCHBLOX_CANARY_OUT=canary go test -tags canary -run TestCanary -v ./internal/said` with `continue-on-error: true`, then upload the `canary` folder with `if: always()`.

  Job `report` (`needs: canary`, `if: always() && github.event_name != 'pull_request'`, `permissions: issues: write, contents: read`):
  - checkout of the workflow's own commit with `persist-credentials: false`, used only to read `canary-report.sh`.
  - download the artifact.
  - `canary-report.sh canary` with `GH_TOKEN: ${{ github.token }}`.

  On a pull request, the `canary` job's test step fails the job when a result is not ok.
- **Done when:** `TestCanaryWorkflowKeepsTheTokenFromAgents` passes. On the pull request, the `canary` job is green, and its artifact holds `claude.json`, `codex.json` and `opencode.json` with `ok: true`.
- **Commit:** `ci: a daily canary runs each agent's latest release (#199)`

### Task 8: the doc names the canary

- **Files:** `docs/agents.md`.
- **Check (no test can see prose):** before, `rg -n "canary" docs/agents.md` finds nothing. After, it finds the new paragraph. It says that each day CI runs the latest Claude Code, Codex and OpenCode against a stand-in model. It says that an issue labelled `canary` means matchblox cannot read that version's menu yet, and that a builder on that version sees `unknown screen` until the fix release.
- **Done when:** the check finds the paragraph, and the full suite passes.
- **Commit:** `docs: the daily canary and what its issue means (#199)`

### Task 9: review

- Load `builder:review` on branch `feat/199-screen-canary` against `origin/main`, with the spec, this plan and the test command `go vet ./... && go test ./...`, plus `go test -tags canary -run TestCanary/claude ./internal/said`.

## Lens findings

- Engineering: a skipped agent would be a green canary that tested nothing. Changed Task 3: a missing tmux or a missing binary is an error, never `t.Skip`. Task 6 also exits 1 with no results.
- Engineering: a slow runner can catch the menu mid-draw. Changed Task 3: it retries once from a clean home before it calls a break (the spec's Calls recommendation).
- Engineering: the canary code lives in `_test.go` files of `internal/said`. It uses the unexported `option` and `menuLines` and adds no product code or binary, so builders get nothing new (recommended in the spec's Left to the builder).
- Security: the result files come from a job that ran unreviewed code, so they can be forged. Added Task 6's hostile test (shell in the version and the screen), and a version and agent allow-list.
- Security: `report` checks out the repository to read `canary-report.sh`. The spec says it "checks out nothing". The checkout is of matchblox's own commit, without the token saved, and runs no agent code, so the anti-goal still holds. This is a change to a spec line: see Calls.
- Design: the person meets an issue, not a screen. The title names the agent and version. The body names the failed check, the screen and the fixture file to add, so the fix starts with a failing test (spec Calls). Comments come only on a newer version or on a recovery, to keep one break from becoming daily noise.
- Architecture: no API, data store or service boundary changes. The `report` job's boundary with the `canary` job is covered by Tasks 6 and 7. No change beyond those.

## Check against the spec

| Spec line | Task |
|---|---|
| A run by hand is green, its log names each agent, version and check | Task 3, Task 7 |
| A rule broken on purpose opens one issue with agent, version, check, screen | Task 6 (`OpensOneIssue`), and the check after merge (Calls) |
| A second run with the same break comments, opens no new one | Task 6 (`CommentsOnANewerVersion`, `StaysQuietOnTheSameVersion`) |
| A broken install opens a "could not run" issue and is not green | Task 4/5 local run, Task 6 (`CouldNotRun`) |
| The agent job is read-only and has no secrets | Task 7 |
| Anti-goal: the agent job holds a write token or a secret | Task 7 |
| Anti-goal: a real model or a paid key is used | Task 3 (stand-in, fake key), Task 7 (no `secrets.`) |
| Anti-goal: one break opens an issue each day | Task 6 |
| Anti-goal: a run that tested no agent is green | Task 3, Task 6 (`NeedsResults`) |
| Anti-goal: a flaky run files an issue for a screen that reads well | Task 3 (one retry) |
| Anti-goal: an issue body carries a home path, host or user name | Task 2 |
| Surface: CI | Tasks 6, 7 |
| Options read well: 1..n, n ≥ 2, Yes first, No last | Task 1 |

## Changes during build

- 2026-11-23: the spec line on the `report` job now reads "checks out only matchblox's own script" (plan Call 1, approved with the plan by the owner).
- 2026-11-23: Task 2's `scrub` takes `host` and `user` as arguments. As planned it read this machine's names, so a runner's `runner@fv-az123` in the hostile input could never be removed, and no test could feed it a short name. The caller (Task 3) passes the real names. The hostile input is unchanged. Proposed by the Task 2 builder, taken by this session.
- 2026-11-23: Task 3 adds `internal/said/main_test.go` (`//go:build canary`): the repo guard `TestEveryTestTmuxNamesItsServer` needs a TestMain that calls `testhome.Run` in any package whose tests start tmux.
- 2026-11-23: Task 3's `agent` has `env(home, url, binDir)`, `version` runs with that env (never the real home), and `stand` makes a fresh stand-in per attempt that counts tool requests.
- 2026-11-23: a `could not run` result carries its reason in `screen`; a run whose stand-in got no tool request is `could not run`, not a screen break (spec: a canary that tests nothing must not be green).
- 2026-11-23: the could-not-run issue title has no version (`the canary could not run Codex: <reason>`); the version is in the body and the marker. The spec's example `Codex 0.99:` is only an example.
- 2026-11-23: Task 7's run step makes the `MATCHBLOX_CANARY_OUT` folder first (`mkdir -p`); `TestCanary` writes into it and does not create it. The results folder is `$RUNNER_TEMP/canary` in both jobs.
- 2026-11-23: Task 7 imports `go.yaml.in/yaml/v3`, already in `go.sum` through gopsutil and testify; `go.mod` gains one direct `require` line and nothing new is downloaded.
- 2026-11-23: closes review finding 1 (a hung or missing agent left the run green). `canary-report.sh` knows the three agents: one with no result file could not run (`the canary wrote no result for <Name>`), and an empty or missing folder opens one `the canary tested nothing` issue (marker key `run`). Each agent's version command gets 20 s (`commandTimeout`) and its process group is killed after; `setup` runs no command, so it needs none.
- 2026-11-23: closes review finding 2 (a failed agent install opened no issue). Each agent installs in its own `continue-on-error` step, the run step runs `if: always()`, and report's download is `continue-on-error`, so a missing artifact reaches the script, which opens `the canary tested nothing`.
- 2026-11-23: closes the review's open check (`supply-chain/CWE-829/.github/workflows/canary.yml/canary`), as the owner ruled in the spec: the canary job makes a user `canary` with no sudo (checked: in neither sudo nor admin, `sudo -n` fails). Each agent installs as `canary` into `/home/canary/.npm-global`, and `go test` runs as `canary` on a copy of the source it owns, all through `sudo -u canary -H env -i`, so no runner variable reaches an agent. The run waits for that user step; only canary's regular `*.json` files reach the upload. `cache: false` stays.
- 2026-11-23: closes the automated security review's TOCTOU finding on the hand-off (root's `find … -exec cp` over canary's folder could copy a swapped-in link to a root-only file into the artifact). Root no longer reads or writes canary's files: the source copy goes runner `tar` → `sudo -u canary tar`, every canary process is killed (and checked gone) before the hand-off, and the hand-off reads only `claude`, `codex` and `opencode.json` through `sudo -u canary cat`, cut at 1 MiB, written by the runner.
- 2026-11-23: closes review finding 3 (an agent that quit lost its last screen). The canary's tmux server keeps a dead pane (`remain-on-exit on`), and the result holds the reason, then the scrubbed last screen with up to 40 lines of its history, where tmux moves what a dead pane drew.
- 2026-11-23: closes round 2's open check (`supply-chain/CWE-829/.github/workflows/canary.yml/canary`), as the owner ruled in the spec: before any install, the user step fails the job when canary can read `/home/runner`, `$RUNNER_TEMP`, `$GITHUB_WORKSPACE` or the folder of the `Runner.Worker` binary (each `test -r` runs as canary; with no `Runner.Worker` found, the log says that check is skipped). It rejects canary's traffic to 168.63.129.16 and 169.254.169.254 with `iptables -m owner --uid-owner canary`, and fails when a `curl` as canary reaches either. Guarded by `TestCanaryWorkflowProvesTheUserIsBoxedIn`.
- 2026-11-23: closes round 2's finding 1 (a hung install cost all three agents their test): each install step has `timeout-minutes: 5`, so a hang fails that step, `continue-on-error` moves on, and the run says `<bin> is not installed`. Also removes the slop `GOFLAGS=-mod=mod` from the run step; `go` keeps `-mod=readonly` and fails rather than rewrite `go.mod`. Both guarded in `TestCanaryWorkflowKeepsTheTokenFromAgents`.
- 2026-11-23: closes round 2's finding 2 (a cancelled run could file false could-not-run or tested-nothing issues): `cancel-in-progress` is `${{ github.event_name == 'pull_request' }}`, so a second run on `main` waits for the first. Guarded in `TestCanaryWorkflowKeepsTheTokenFromAgents`.
- 2026-11-23: closes round 2's finding 3 (the tested-nothing issue never heard that the canary ran again): when results arrive and `the canary tested nothing` is open, the report comments once `The canary runs again.` with `<!-- canary run none pass -->`. While the outage goes on it writes nothing; a new outage after that pass comments the `norun` marker again on the same issue. Guarded by `TestCanaryReportSaysItRunsAgain`.
- 2026-11-23: closes the PR's first CI run (OpenCode 1.19.6 failed `no menu` on a screen matchblox reads as `asks`), review round 4 finding 1, and fix list R1, M5 and I7, as the owner ruled in the spec: the check is per agent. Claude Code keeps the full check (`canaryCheck`), whose question must also end in `?` and not be the stand-in's command; Codex and OpenCode pass `waitingCheck` (`Screen` is `Permits`). `testdata/menus/opencode-bash-1.19.6.txt` is the CI screen with `canary` restored. docs/agents.md says what each agent's break shows a builder.
- 2026-11-23: closes review round 4 finding 2 and fix list M1, I4, I5 and I6. Every agent's PATH adds node's folder (`agentPath`, from the test's own PATH, which is `CANARY_PATH` in CI), so an npm `#!/usr/bin/env node` launcher starts (`TestNodeLauncherFindsNode`); a failed version command adds the first lines of its stderr (`TestVersionKeepsStderr`). Version and setup reasons are scrubbed like screens (`TestFailReasonsAreScrubbed`). A stand-in with no tool request keeps the screen in its reason (`TestNoToolRequestKeepsTheScreen`); an agent that got the tool call but never shows `touch canary-ran` could not run (`TestUntakenReplyCouldNotRun`); a real break on one attempt stays the verdict when the retry errors (`TestRetryKeepsARealBreak`). `drawMenu` waits `menuTimeout` (30 s), which those tests shorten.

## Calls

- The `report` job checks out matchblox's own commit to read `canary-report.sh`. The spec line says it "checks out nothing". Recommend: change that line to "checks out only matchblox's own script, without the token saved, and runs no agent code". The anti-goal stays the same. The other way is to inline the script in the workflow, where no test can reach it.
- The done-when line "a rule broken on purpose opens one issue" can only run on GitHub after merge, because a pull request run writes no issues (Task 7). Recommend: after merge, run the workflow once by hand on a branch with `cancelRe` broken, as part of `ship`.
- Codex and OpenCode configs (Tasks 4, 5) follow their documented provider settings, unverified on this host. If one of them cannot draw its menu against a stand-in, its CI result says `could not run`, and that agent moves to a follow-up issue.
