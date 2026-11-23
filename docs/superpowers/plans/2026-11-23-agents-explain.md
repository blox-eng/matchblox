Spec: docs/superpowers/specs/2026-11-23-agents-explain-design.md
Spec approved: 2026-11-23
Test command: go test -race ./... (and `go vet ./...`, `golangci-lint run`)
Plan approved: 2026-11-23 by the owner
Next: builder:build

# Plan: a builder sees which screen rule read a waiting session

## What the code holds today

- `internal/said/said.go` `Screen` walks the last `screenWithin` (8) lines with words. It cuts at the working line (`workingRe`), then looks for an approval prompt (`permitsRe`), then a question (`question`), else the last line outside the input box and the hints. `Menu` adds the key hint under the menu (`cancelRe`) on the last line.
- `internal/said/menu.go` `OptionNumber` and `OptionLabel` read a menu's numbered rows. `internal/service/choose.go` sends a digit only when that row reads on the screen. These rows are what a digit picks.
- `internal/said/canary_check_test.go` holds `scrub(screen, work, home, host, user)`, a test-only helper. It skips names under 3 characters, and it replaces them anywhere in a word.
- `cmd/matchblox/cli.go` `cliPeek` reads the service's state doc (`readDoc`) and `resolve`s a session by pane, target or name. It never touches a pane.
- `cmd/matchblox/agents.go` prints the table, and `agentsFix` is its fix line.
- `sample.CaptureScreen(pane)` runs `tmux display-message` and `capture-pane -p -e -J`, and nothing else.
- The state doc carries each session's `UnknownScreen`, `MenuUnread`, `Permits`, `Ask`, `Version`, `Agent`, and the queue item (`State`, `Since`) of its hook.

## File map

- `internal/said/explain.go` (create) — `Explain(screen) Trace`: each rule, the line it matched, the menu rows with their digits, and the lines the rules read. `Screen` and `Menu` read their answer from the same trace.
- `internal/said/explain_test.go` (create) — the tests of `Explain`, and the guard that `Explain`, `Screen` and `Menu` agree.
- `internal/said/said.go` (change) — `Screen` and `Menu` call the trace, so one code path decides.
- `internal/said/scrub.go` (create) — `Scrub(screen string, dirs, names []string) string`, moved from the canary test.
- `internal/said/scrub_test.go` (create) — the tests of `Scrub`, the hostile ones first.
- `internal/said/canary_check_test.go`, `internal/said/canary_test.go` (change) — call `said.Scrub`; the test-only `scrub` goes.
- `cmd/matchblox/explain.go` (create) — `agents explain <who>` and `--file`: reads the doc, captures the pane, prints the verdict, the trace and the scrubbed screen, as text or JSON.
- `cmd/matchblox/explain_test.go` (create) — the tests of `explain.go`.
- `cmd/matchblox/main.go` (change) — dispatch `agents explain`, the `--file` flag, the usage line.
- `cmd/matchblox/agents.go` (change) — `agentsFix` names `matchblox agents explain`.
- `cmd/matchblox/docs_test.go` (change) — `agents` takes `json` and `file`.
- `internal/sample/tmux_test.go` (change) — guard: the capture runs no command that types.
- `docs/agents.md`, `CHANGELOG.md` (change) — the command, and the Unreleased line.

## Task 1: Scrub removes the home path, the host and the user, also short names

Surface: personal data

- **Files:** `internal/said/scrub.go`, `internal/said/scrub_test.go`, `internal/said/canary_check_test.go`, `internal/said/canary_test.go`.
- **Test first (hostile):** `TestScrubRemovesAShortUserAndHost`. Screen `al@box:~/work$ ls /home/al/work/x.go\nAllow this?\n 1. Yes\n`, dirs `["/home/al/work", "/home/al"]`, names `["box", "al"]`. Assert the output holds no `/home/al`, no `al@`, no `@box`, and holds `<user>@<host>`, `<dir>/x.go`, `Allow this?` and `1. Yes` unchanged.
  Then `TestScrubKeepsWordsThatHoldTheName`: name `ye` leaves `Yes` alone; name `o` changes nothing.
- **Run:** `go test ./internal/said -run Scrub`. Before: fails to build, `undefined: Scrub`. After: passes.
- **Do:**
  1. `Scrub(screen string, dirs, names []string) string`. Replace each dir, longest first, with `<dir>` (the canary's `<work>`/`<home>` labels go; its tests change to `<dir>`).
  2. Replace each name of 2 or more characters at word boundaries only (`regexp.QuoteMeta`, `\b`), the first as `<host>`, the second as `<user>`, by position in `names`. Callers pass host, then user.
  3. Move the canary's callers to `said.Scrub(screen, []string{work, home}, []string{host, user})`. Delete the test-only `scrub`. Keep `TestScrubKeepsThePrompt` and `TestScrubKeepsShortNames`, now on `Scrub`.
- **Done when:** `go test ./internal/said -run Scrub` and `go test -tags canary ./internal/said -skip '^TestCanary$'` pass.
- **Commit:** `refactor(said): one Scrub for the canary and explain, short names too (#199)`

## Task 2: Explain names the rule that decided, on the same lines Screen reads

Surface: secrets

- **Files:** `internal/said/explain.go`, `internal/said/explain_test.go`, `internal/said/said.go`.
- **Test first (hostile):** `TestExplainShowsOnlyTheLinesTheRulesRead`. A screen with `export TOKEN=sk-live-123` 20 lines above a Claude Code menu. Assert no line of `Trace.Lines` holds `sk-live-123`, and the menu's rows are there.
  Then:
  - `TestExplainAgreesWithScreenAndMenu`: for each file in `testdata/menus` and each screen in `said_test.go`'s Screen and Menu tables, `Explain(s).Kind`, `.Line` equal `Screen(s)`, and `.MenuRead` equals `Menu(s)`'s ok. This is the guard that explain never names a rule that did not decide.
  - `TestExplainReadsTheClaudeMenu`: on `testdata/menus/claude-bash-2.1.330.txt`: the hit for `approval prompt` is the `1. Yes` line, `menu key hint` is the last line, `working line` has no hit, and `Options` is `1 Yes`, `2 …`, `3 No` as `OptionNumber`/`OptionLabel` read them.
  - `TestExplainNamesTheFallback`: a screen with no prompt and no question gives `Kind` none, and the `fallback` hit names the last line with words.
- **Run:** `go test ./internal/said -run Explain`. Before: `undefined: Explain`. After: passes, and the whole package passes.
- **Do:**
  1. `type Hit struct{ Rule string; Line int; Text string }` (Line is 1-based in `Lines`, 0 for no match). `type Option struct{ N int; Label string }`. `type Trace struct{ Kind Kind; Line string; Hits []Hit; MenuRead bool; Options []Option; Lines []string }`.
  2. Rules, in this order and with these names: `working line`, `approval prompt`, `question`, `menu key hint`, `fallback`.
  3. Move the body of `Screen` into `Explain`, recording each rule's hit. `Screen` returns `t.Line, t.Kind`. `Menu` returns `t.Line, t.MenuRead`. No regex is evaluated in two places.
  4. `Options`: the rows of `Lines` that `option` reads, only when `Kind` is `Permits`.
  5. `Lines`: the window the rules read (from `from` to the end), as plain text.
- **Done when:** `go test ./internal/said` passes, and `Screen`'s and `Menu`'s existing tests are unchanged.
- **Commit:** `feat(said): Explain names the rule that read a screen (#199)`

## Task 3: `matchblox agents explain <who>` prints the verdict, the rules and the screen

Surface: an agent's tools, personal data

- **Files:** `cmd/matchblox/explain.go`, `cmd/matchblox/explain_test.go`, `cmd/matchblox/main.go`, `internal/sample/tmux_test.go`.
- **Test first (hostile):**
  - `TestCaptureScreenOnlyReads` (in `internal/sample`): call `captureScreen` with a fake `run` that records argv. Assert the tmux subcommands are only `display-message` and `capture-pane`, with no `send-keys`, `paste-buffer` or `load-buffer`.
  - `TestExplainLeavesNoNameOrPath`: a doc with session `api`, `UnknownScreen` true, a queue item with `State` permission. `screenOf` returns a screen holding the test's home dir, host and user (all set through the injected `who` facts). Assert the output holds none of the three.
- **Then:**
  - `TestExplainUnknownScreen`: the output holds `Verdict  unknown screen: no rule reads a menu`, `Hook     waits for a permission, for 14s` (the clock is injected), `Rules    built into matchblox v<version>`, each rule name, and `Fix: report it at github.com/blox-eng/matchblox/issues with this output`.
  - `TestExplainReadMenuHasNoFix`: on a read menu, a `Menu  1 Yes · 2 … · 3 No` line, and no `Fix:` line.
  - `TestExplainHooklessAgent`: a Codex session gives `Hook     no hooks: screen only`.
  - `TestExplainUnknownSession`: `no session api: matchblox sessions lists them`, error returned.
  - `TestExplainJSON`: `--json` decodes to the same verdict, hits, options and screen lines.
- **Run:** `go test ./cmd/matchblox -run Explain` and `go test ./internal/sample -run CaptureScreenOnlyReads`. Before: `undefined: writeExplain`; the sample test passes at once, as a guard. After: all pass.
- **Do:**
  1. `writeExplain(w io.Writer, doc state.Doc, who string, x explainEnv, asJSON bool) error`. `explainEnv` holds `screenOf func(string) (string, bool, bool)`, `now time.Time`, `home`, `host`, `user` and the session's dir. Production fills them with `sample.CaptureScreen`, `time.Now()`, `os.UserHomeDir`, `os.Hostname`, `user.Current`.
  2. `resolve` the session. Capture its pane. Run `said.Explain` on `said.Plain` of the screen.
  3. The verdict, from the doc: `UnknownScreen` → `unknown screen: no rule reads a menu`; `MenuUnread` → `menu not read yet`; `Ask` → `asks: <question>`; `Permits` → `waits for an approval: <line>`; else `none: <line>`.
  4. The hook line: `no hooks: screen only` for an agent in `hookless`; `waits for a permission, for <d>` from the queue item; `no wait` otherwise.
  5. Scrub `Lines` and every hit's text with `said.Scrub(…, []string{dir, home}, []string{host, user})`.
  6. In `main.go`: `agents explain` goes to `cliExplain`. It reads the doc like `cliPeek`. Add the usage line `matchblox agents explain <who>  which screen rule read a session, to report it (--file, --json)`.
- **Done when:** the tests above pass, and `go test ./cmd/matchblox` passes.
- **Commit:** `feat: matchblox agents explain shows which rule read a screen (#199)`

## Task 4: explain says when the screen changed, and reads a saved screen

Surface: personal data

- **Files:** `cmd/matchblox/explain.go`, `cmd/matchblox/explain_test.go`, `cmd/matchblox/main.go`, `cmd/matchblox/docs_test.go`.
- **Test first:**
  - `TestExplainSaysTheScreenChanged`: the doc says `UnknownScreen`, but the captured screen is a read menu. The output holds `the screen changed since the note: run it again`. The reverse case (a doc with `Permits`, a screen no rule reads) holds the same line.
  - `TestExplainFile`: `--file testdata/menus/claude-bash-2.1.330.txt` prints the menu with its digits, and no `Session` and no `Hook` line.
  - `TestExplainFileUnreadable`: a missing path gives an error that names the path. The error passes through `Scrub`, so a home path in it becomes `<dir>`.
  - `TestExplainNeedsOneOf`: both `<who>` and `--file` given, or neither, give `explain takes a session or --file, not both`, or `explain needs a session: matchblox agents explain <who>`.
- **Run:** `go test ./cmd/matchblox -run 'Explain|Docs'`. Before: the changed-screen line and `--file` fail. After: pass.
- **Do:**
  1. Disagreement: the note says unknown or unread while `Trace.MenuRead` is true; or the note says `Permits` while `Trace.Kind` is not `Permits`.
  2. `--file <path>`: read the file (cap 1 MiB), run `Explain`, print with no session and hook lines.
  3. `docs_test.go`: `"agents": {"json", "file"}`.
- **Done when:** the tests pass.
- **Commit:** `feat: explain reads a saved screen and says when the screen changed (#199)`

## Task 5: the agents table points to explain, and the docs say how to report

- **Files:** `cmd/matchblox/agents.go`, `cmd/matchblox/agents_test.go`, `docs/agents.md`, `CHANGELOG.md`.
- **Test first:** in `agents_test.go`, the test that checks the fix line expects `a screen matchblox cannot read: run matchblox agents explain <session>, and report its output`.
- **Run:** `go test ./cmd/matchblox -run Agents`. Before: fails on the old line. After: passes.
- **Do:** change `agentsFix`. In `docs/agents.md`, after the `agents` table, add how to run `explain`, what it shows, that it shows only the lines the rules read, and "check it before you post it". Add the changelog line under Unreleased: `You can see which rule read an agent's screen.` (7 words or fewer as the headline).
- **Done when:** the test passes, and `.github/scripts/changelog-guard.sh` passes.
- **Commit:** `docs: the agents table points to explain (#199)`

## Task 6: review the whole change

- Load `builder:review` on the branch, with this plan, the spec and the test command.
- Check by hand on this host, as the spec's done-when lines say: run `matchblox agents explain` on a waiting Claude Code session. Capture the pane with `tmux capture-pane -p` before and after, and compare them.

## Lens findings

- Engineering, one owner: `Screen`, `Menu` and `Explain` share one walk of the rules (Task 2, step 3). A second walk could drift, and explain would then name a rule that did not decide. `TestExplainAgreesWithScreenAndMenu` guards it.
- Engineering, one owner: the canary's `scrub` moves into product code (Task 1). It is not copied.
- Engineering: the canary scrub replaced a name anywhere in a word, so names under 3 characters were skipped. A user named `al` would leak. Changed to word boundaries, for 2 characters or more (Task 1). `TestScrubKeepsShortNames` still holds: `ye` does not match inside `Yes`.
- Architecture, boundary: explain needs no new message to the service. It reads the state doc, as `peek` does, and captures the pane on this host, as the service does. The spec's ruling says "answered by the service from what it saw". The verdict comes from the service's doc. The rule trace comes from a new capture, as the spec's Calls recommend. The disagreement line covers the gap (Task 4). No new proto kind, so no version bump for phone or console.
- Architecture, an agent's tools: explain must only read. `sample.CaptureScreen` runs only `display-message` and `capture-pane`, and `TestCaptureScreenOnlyReads` pins it (Task 3). explain builds no `proto.Act`.
- Design: the person meets the text output and its errors. Each error names its fix (Tasks 3 and 4). The `Fix:` line shows only when the screen is unknown, so a read screen says nothing extra (Task 3).
- Design: the spec's done-when said the digits match `matchblox send <who> --pick n`. `--pick` answers the question tool only, not an approval menu. An approval menu's digit goes through the console's `choose`, which reads the row with `OptionNumber`. Task 2 reads `Options` with the same function. The spec's line is read as "the digits match what the console's digit sends".

## Spec check

| Spec line | Task |
|---|---|
| `unknown screen` session: verdict, hook wait, each rule, rules version, screen with no home path, host or user | Task 3: agents explain prints the verdict, the rules and the screen |
| A read menu: the menu with its digits, matching what a digit sends | Task 2: Explain names the rule (Options via OptionNumber); Task 3 (`TestExplainReadMenuHasNoFix`) |
| `--file testdata/menus/claude-bash-2.1.330.txt`: the menu with digits, no hook line | Task 4: explain reads a saved screen |
| The pane holds the same text, no key reached it | Task 3 (`TestCaptureScreenOnlyReads`); Task 6: the manual before/after capture |
| The agents fix line names `matchblox agents explain` | Task 5: the agents table points to explain |
| Anti-goal: explain types a key or sends text into the pane | Task 3 (`TestCaptureScreenOnlyReads`, no `proto.Act`) |
| Anti-goal: the output carries a home path, host or user name | Task 1 (hostile short names); Task 3 (`TestExplainLeavesNoNameOrPath`); Task 4 (scrubbed errors) |
| Anti-goal: the verdict disagrees with the note, and nothing says so | Task 4 (`TestExplainSaysTheScreenChanged`) |
| Anti-goal: explain names a rule that did not decide, or invents a match | Task 2 (`TestExplainAgreesWithScreenAndMenu`, one walk) |
| Anti-goal: the screen shows more than the lines the rules read | Task 2 (`TestExplainShowsOnlyTheLinesTheRulesRead`) |

## Changes during build
