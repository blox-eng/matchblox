# Accounts on N Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** N (and `matchblox new`) starts claude, codex or cursor-agent on a chosen account or a new one, the new one signs in inside its session, and USAGE shows every account side by side.

**Architecture:**
- A new package `internal/accounts` owns the folders: names, paths, the links or copy from the default, and the env of each agent. It has no tmux and no network.
- The service's `newSession` asks it for the folder and the env, puts `env VAR=… argv` on the `new-session` step, and for a new account runs a sign-in driver that waits for the agent's first screen and sends its keys.
- The sampler tags each session with `AccountDir`; limits are keyed by it; Cursor usage becomes one poll per account. The console form gets `account` and `setup` fields; USAGE names a block by its account.

**Tech Stack:** Go 1.26, the standard library, tmux ≥ 3.0. No new dependencies.

**Spec:** `design/0013-accounts.md`. Read it first, including Facts. This plan settles its "Left to the builder" items in the tasks below.

## Global Constraints

- Agents with accounts: `claude`, `codex`, `cursor-agent` (`cursor` accepted as an alias). Not `opencode`.
- Root: `~/.matchblox/accounts/<agent>/<name>/`, mode 0700. Name: 1–10 characters, `[a-z0-9-]`. `default` is not a folder.
- Codex socket check: `len(folder) + 43 <= 103` on darwin, `<= 107` elsewhere; refuse with the reason.
- Env per agent, exact:
  - claude: `CLAUDE_CONFIG_DIR=<folder>` (no trailing slash); new account adds `BROWSER=/usr/bin/true`.
  - codex: `CODEX_HOME=<folder>`; argv adds `-c cli_auth_credentials_store="file"` after the binary.
  - cursor-agent: `AGENT_CLI_CREDENTIAL_STORE=file`, `HOME=<folder>/home`, `CURSOR_CONFIG_DIR=<folder>/cfg`.
- Own entries (never linked or copied):
  - claude: `.claude.json`, `.credentials.json`, `projects`, `sessions`, `history.jsonl`.
  - codex: `auth.json`, `sessions`, `archived_sessions`, `history.jsonl`, `*.sqlite*`, `app-server-control`, `packages`, `.tmp`.
  - cursor-agent, under `home/.cursor`: `auth.json`, `cli-config.json`, `chats`, `projects`. `home` itself mirrors every top-level entry of `$HOME` except `.cursor`.
- Setup modes: `like-default` (links; default), `copy` (one-time copy), `empty`.
- matchblox never reads a sign-in code from a pane, never types a password, never copies a token into its own files or logs, never writes the default sign-in.
- The env goes on the command (`env VAR=… argv`), never `new-session -e`.
- Switch agent (A) and the stoker start on `default` only.
- Fixtures stay anonymous: `you@example.com`, `/work/app`, `/home/u`.
- Match the surrounding code: sentence-style comments, `Test<Sentence>` names, helpers `lifeTest`, `lifeSnap`, `consoleAct`, `newAct`, `loadedWith`, `key`, `typeText`, `cliDoc`.
- Each user-facing change adds a line under `## [Unreleased]` in `CHANGELOG.md` in its own task.
- `go test ./...` and `golangci-lint run` before each commit. Conventional, signed commits ending with the session's attribution line.

## Review Focus

1. **A link target that is a real file in the account** (a tool rewrote `settings.json` in place): the next start leaves it alone and does not fail. Test in Task 1 (`TestPrepareLeavesARealFileAlone`).
2. **The default sign-in is never touched**: no step that `newSession` runs for an account names `~/.claude`, `~/.codex`, `~/.cursor/auth.json` or the keychain. Test in Task 2 (`TestNewAccountStepsNeverNameTheDefault`).
3. **Two new accounts before sign-in** show as two USAGE blocks, not one. Test in Task 4 (`TestTwoUnsignedAccountsStayApart`).
4. **The agent updates and the first screen changes**: the driver gives up after 20 s, says "sign in by hand in <name>", and the session keeps running. Test in Task 3 (`TestSignInGivesUpAndSaysSo`).
5. **A split pane in an account's tmux session** gets the builder's own `HOME`. Test in Task 2 (`TestAccountEnvIsOnTheCommandNotTheSession`).

---

### Task 0: Spikes (gate; no product code)

Run the five spikes in `design/0013-accounts.md` "Spikes before the plan" by hand, with real second accounts, and write each result as one bullet under a new `## Revised while building` heading in the design. If spike 1 (Cursor mirror) fails, stop and bring the result back before Task 1: the Cursor parts of Tasks 1, 2 and 5 change. If spike 3 shows the daemon holds the rollout, Task 4 adds the newest-file fallback; otherwise it does not.

- [ ] **Step 1:** Spike 1 to 5, each result one bullet in the design.
- [ ] **Step 2:** Commit `docs: spikes for accounts on N (#170)`.

### Task 1: `internal/accounts`

**Files:**
- Create: `internal/accounts/accounts.go`, `internal/accounts/prepare.go`
- Test: `internal/accounts/accounts_test.go`, `internal/accounts/prepare_test.go`

**Interfaces:**
- Produces:
  - `type Mode string` with `LikeDefault Mode = "like-default"`, `Copy Mode = "copy"`, `Empty Mode = "empty"`.
  - `func Root(home string) string` → `<home>/.matchblox/accounts`.
  - `func CheckName(agent, name, home, goos string) error`.
  - `func Folder(home, agent, name string) string`.
  - `func List(home, agent string) ([]string, error)` → account names, sorted.
  - `func Env(agent, folder string, fresh bool) []string` → `KEY=value` pairs.
  - `func Args(agent string) []string` → extra argv after the binary (codex only).
  - `func Prepare(home, agent, folder string, mode Mode) error` → makes the folder (0700) and its links or copy; idempotent.
  - `func DefaultDir(home, agent string) string` → `~/.claude`, `~/.codex`, `~/.cursor`.

- [ ] **Step 1: Failing tests** in `accounts_test.go`:
  - `TestNameRules`: `CheckName("claude","work","/home/u","linux")` is nil; `"Work"`, `""`, `"default"`, `"abcdefghijk"` (11), `"a/b"` each error.
  - `TestCodexSocketLimit`: with `home` of 60 characters, `CheckName("codex","abcdefghij",home,"darwin")` errors and the message contains `"socket"`; with `"/home/u"` it is nil.
  - `TestEnvOfEachAgent`: `Env("claude","/home/u/.matchblox/accounts/claude/work",true)` equals `[CLAUDE_CONFIG_DIR=/home/u/.matchblox/accounts/claude/work BROWSER=/usr/bin/true]`; with `fresh=false` only the first; codex → `[CODEX_HOME=<f>]`; cursor-agent → `[AGENT_CLI_CREDENTIAL_STORE=file HOME=<f>/home CURSOR_CONFIG_DIR=<f>/cfg]`.
  - `TestArgsForcesCodexFileStore`: `Args("codex")` equals `[-c cli_auth_credentials_store="file"]`; claude and cursor-agent give nil.
- [ ] **Step 2: Failing tests** in `prepare_test.go` (temp `home` with a fake default):
  - `TestLikeDefaultLinksAllButOwn`: default `~/.claude` has `settings.json`, `skills/`, `.claude.json`, `projects/`; after `Prepare(..., LikeDefault)` the account has `settings.json` and `skills` as symlinks to the default, and no `.claude.json` or `projects`.
  - `TestPrepareLeavesARealFileAlone`: account already has a real `settings.json`; `Prepare` returns nil and the file is unchanged.
  - `TestPrepareAddsNewEntries`: an entry added to the default after the first `Prepare` is linked by the second.
  - `TestCopyIsIndependent`: `Copy` makes real files; editing the copy leaves the default unchanged; own entries are absent.
  - `TestEmptySharesNothing`: `Empty` gives an empty folder (for cursor-agent: `home/` mirrored, `home/.cursor` empty, `cfg/` empty).
  - `TestCursorHomeMirrorsHome`: `$HOME` has `.gitconfig`, `.ssh/`, `Library/`, `.cursor/hooks.json`, `.cursor/auth.json`; after `LikeDefault` the account `home/` links `.gitconfig`, `.ssh`, `Library`; `home/.cursor/hooks.json` is a link; `home/.cursor/auth.json` is absent.
  - `TestCursorStatusLineSeeded`: default `~/.cursor/cli-config.json` has `statusLine` and `authInfo`; the account `cfg/cli-config.json` has `statusLine` and no `authInfo`.
  - `TestEmptyHomeWorks`: a `$HOME` with no dotfiles at all; `Prepare` for each agent and mode returns nil.
  - `TestFolderIsPrivate`: the account folder's mode is 0700.
- [ ] **Step 3: Run** `go test ./internal/accounts/` → FAIL (package missing).
- [ ] **Step 4: Implement.** Links are `os.Symlink(abs target, entry)`; a mode switch never removes an existing entry. `Copy` uses a recursive copy that skips own entries and symlinks it meets (copies the link). The cursor-agent `home/` mirror applies in every mode (it is the builder's dotfiles, not the agent's setup); `Empty` only skips the `.cursor` entries and the `cfg` seed.
- [ ] **Step 5: Run** `go test ./internal/accounts/` → PASS.
- [ ] **Step 6: Commit** `feat: accounts keep their folders beside the default (#170)`.

### Task 2: Start on an account

**Files:**
- Modify: `internal/proto/proto.go:186-192`, `internal/service/lifecycle.go` (`launchArgv`, `newSession`, `newSteps`)
- Test: `internal/service/lifecycle_test.go`

**Interfaces:**
- Consumes: Task 1's `CheckName`, `Folder`, `List`, `Env`, `Args`, `Prepare`.
- Produces:
  - `proto.NewSession` gains `Account string \`json:"account,omitempty"\``, `NewAccount bool \`json:"new_account,omitempty"\``, `Setup string \`json:"setup,omitempty"\``.
  - `func launchArgv(bin, agent, name, first string, extra []string) []string` (extra goes right after `bin`).
  - `Service.Home string` (defaults to `os.UserHomeDir()`), so tests point it at a temp dir.
  - A new-account start's `advice.Action.Label` is `new <agent> account "<name>", <setup>: sign in in the session` (the console's confirm shows it).

- [ ] **Step 1: Failing tests:**
  - `TestStartOnAnAccount`: `newAct(t, proto.NewSession{Where: dir, Agent: "claude", Account: "work"})` with `work` prepared; the `new-session` step ends with `env CLAUDE_CONFIG_DIR=<home>/.matchblox/accounts/claude/work <bin> --name <name>`.
  - `TestUnknownAccountIsRefused`: `Account: "nope"` without `NewAccount` → `Err` contains `no claude account nope`.
  - `TestNewAccountPreparesAndStartsWithoutFirst`: `NewAccount: true, Account: "work2", Setup: "copy", First: "x"` → folder exists, `First` dropped, `BROWSER=/usr/bin/true` in the env.
  - `TestCodexAccountForcesFileStore`: the argv has `-c cli_auth_credentials_store="file"` right after the binary.
  - `TestNewAccountStepsNeverNameTheDefault`: for each agent, no step contains `<home>/.claude`, `<home>/.codex`, `<home>/.cursor/auth.json` or `security`.
  - `TestAccountEnvIsOnTheCommandNotTheSession`: no step has `-e`, `set-environment` or `setenv`.
  - `TestDefaultStartsAsToday`: `Account: ""` and `Account: "default"` give today's exact steps.
  - `TestOpencodeHasNoAccounts`: `Agent: "opencode", Account: "work"` → `Err` contains `opencode has no accounts`.
- [ ] **Step 2: Run** `go test ./internal/service/ -run 'Account|Default'` → FAIL.
- [ ] **Step 3: Implement.** `newSteps` checks the account (refuse before any step), `newSession` runs `Prepare` before `Actions.Do` only when `a.Confirm` is true (a dry plan writes nothing), then builds `env K=V… argv`. Update every `launchArgv` caller (switch passes nil).
- [ ] **Step 4: Run** `go test ./internal/service/` → PASS.
- [ ] **Step 5: Commit** `feat: a new session starts on the account the builder picks (#170)`.

### Task 3: The sign-in driver

**Files:**
- Create: `internal/service/signin.go`
- Modify: `internal/service/lifecycle.go` (`newSession` calls it after a new-account start)
- Test: `internal/service/signin_test.go`

**Interfaces:**
- Consumes: `Service.Capture`, `panes.keys` via a new exported `panes.Keys(target string, keys ...string) []string`, `Service.Actions`.
- Produces: `func signInKeys(agent, screen string) (keys []string, ok bool)` and `func (s *Service) driveSignIn(ctx context.Context, pane, agent, name string)`, run in a goroutine; polls every 500 ms, gives up at 20 s.

- [ ] **Step 1: Failing tests:**
  - `TestSignInKeysOfEachScreen` (table): claude screen containing `Choose the text style` → `[Enter]`, then a screen containing `Select login method` → `[Enter]`; codex `Sign in with ChatGPT` → `[Down Enter]`; cursor-agent `Press any key to log in` → `[Enter]`; any other screen → `ok=false`.
  - `TestSignInSendsKeysOnce`: a fake `Capture` that shows the codex picker three times; exactly one `send-keys … Down` and one `… Enter`.
  - `TestSignInGivesUpAndSaysSo`: a screen that never matches; after the timeout (inject `s.signInWait = 50*time.Millisecond`) an alert or flash says `sign in by hand in <name>`; no keys sent; the pane is not killed.
  - `TestSignInNeverReadsTheCode`: the driver's log and result contain no text from after the matched screen line (feed a screen with `ABCD-12345`; assert it appears nowhere in recorded output).
- [ ] **Step 2: Run** `go test ./internal/service/ -run SignIn` → FAIL.
- [ ] **Step 3: Implement.** Match strings live in one table in `signin.go`, with a comment naming the probed versions (Claude Code 2.1.308, codex-cli 0.163.0, Cursor 2026.10.19); a looser match is not needed.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: a new account reaches its sign-in by itself (#170)`.

### Task 4: The sampler knows the account

**Files:**
- Modify: `internal/sample/sampler.go` (`Session`, `limitSources`, the claude, codex and cursor branches), `internal/sample/account.go` (`envOf`), `internal/limits/limits.go` (`Account`)
- Test: `internal/sample/limits_test.go`, `internal/sample/context_test.go` (or a new `internal/sample/accounts_test.go`)

**Interfaces:**
- Consumes: `accounts.Root`.
- Produces: `Session.AccountDir string \`json:"account_dir,omitempty"\``, `limits.Account.Dir string \`json:"dir,omitempty"\``, `Session.SigningIn bool \`json:"signing_in,omitempty"\`` (true when `AccountDir` is set and `Account` is empty).

- [ ] **Step 1: Failing tests:**
  - `TestSessionUnderAnAccountFolder`: a claude process with `CLAUDE_CONFIG_DIR=/home/u/.matchblox/accounts/claude/work` → `AccountDir` is that path; with `~/.claude-work` (made by hand) → `AccountDir` is that path too; default → `""`.
  - `TestTwoUnsignedAccountsStayApart`: two codex sessions in two account folders with no `auth.json` → two `Limits` entries with different `Dir`.
  - `TestDefaultKeepsItsKey`: default sessions build the same `Limits` as before (compare against today's fixture).
  - `TestCursorAccountReadsItsOwnHome`: a cursor-agent process with `HOME=/home/u/.matchblox/accounts/cursor-agent/work/home`, `CURSOR_CONFIG_DIR=…/work/cfg`, and `authInfo` in that `cli-config.json` → `Account` is that email; a process with a foreign `HOME` outside the root still reads nothing.
  - `TestSigningInWhileNoName`: an account folder with no sign-in → `SigningIn` true.
- [ ] **Step 2: Run** `go test ./internal/sample/` → FAIL.
- [ ] **Step 3: Implement.** Key `limitSources` by `[3]string{provider, account, dir}` where `dir` is set only for a non-default folder. `envOf` trusts a `HOME` under `accounts.Root(s.Home)` for cursor-agent only. If Task 0 spike 3 found the daemon holds the rollout, add `TestCodexRolloutFromTheNewestFile` and the fallback in `codexRollout`.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: each account has its own limits, signed in or not (#170)`.

### Task 5: Cursor usage per account

**Files:**
- Modify: `internal/limits/cursorusage.go`, `internal/sample/sampler.go` (cursor branch), `cmd/matchblox/main.go:772-795`
- Test: `internal/limits/cursorusage_test.go`, `internal/sample/limits_test.go`

**Interfaces:**
- Produces: `type CursorUsages struct { Every time.Duration; Keychain func(context.Context) (string, error); File func(path string) (string, error); Fetch func(ctx context.Context, token string) ([]byte, error) }` with `func (u *CursorUsages) Reading(dir string, now time.Time) *Reading` — `dir == ""` uses `Keychain`, else `File(<dir>/home/.cursor/auth.json)`. `Sampler.CursorUsage` becomes `*limits.CursorUsages`. `func FileToken(path string) (string, error)` reads `accessToken` only.

- [ ] **Step 1: Failing tests:**
  - `TestEachCursorAccountHasItsOwnReading`: two dirs, a fake `Fetch` that answers by token; `Reading("a")` and `Reading("b")` differ; `Reading("")` uses the keychain token.
  - `TestFileTokenReadsOnlyTheToken`: an `auth.json` with `accessToken` and `refreshToken` → the access token; a missing file → error.
  - `TestCursorAccountIsNotGivenTheDefaultsMonth`: in the sampler, a cursor session with `AccountDir` set never gets the `""` reading.
  - `TestAccountUsageRunsOffMac`: with `runtime.GOOS` faked to linux (pass `goos` into the wiring function), account dirs still poll; `default` does not.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** Keep one `CursorUsage` per key inside `CursorUsages` (a map under a mutex); the token is read when a call is due and dropped after, as today.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: Cursor usage for each Cursor account (#170)`.

### Task 6: Known accounts reach the console

**Files:**
- Modify: `internal/state/state.go:44-46` (`Doc`), `internal/service/service.go:347`
- Test: `internal/service/service_test.go`

**Interfaces:**
- Consumes: `accounts.List`, `Session.AccountDir`.
- Produces: `state.Doc.Accounts map[string][]string \`json:"accounts,omitempty"\`` — per agent, `default` first, then the folder names, then hand-made folders of live sessions by their path (`~/.claude-work`), deduplicated.

- [ ] **Step 1: Failing test** `TestKnownAccountsPerAgent`: with folders `claude/{personal,work}`, `codex/work` and a live claude session on `/home/u/.claude-old` → `{"claude": ["default","personal","work","~/.claude-old"], "codex": ["default","work"], "cursor-agent": ["default"]}`; opencode absent.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: the console knows each agent's accounts (#170)`.

### Task 7: The N form picks the account

**Files:**
- Modify: `internal/app/lifeform.go` (fields, keys, confirm), `internal/app/model.go:449`, the form's view code (`internal/app/view.go` spans)
- Test: `internal/app/lifeform_test.go`

**Interfaces:**
- Consumes: `state.Doc.Accounts`, `proto.NewSession` fields from Task 2.
- Produces: `fieldAccount` after `fieldAgent`, and `fieldSetup` shown only for a new account. `form.account string`, `form.newAccount bool`, `form.setup string`.

UX rules (from the design and common form practice): the field cycles with ←→ like `agent`; `+ new account` is the last entry; picking it turns the field into a name box with the cursor in it; `setup` appears under it with `like default` preselected and one grey line of help per choice (`like default: shares skills, plugins and settings with default`; `copy of default: a copy that can change on its own`; `empty: nothing shared, the agent's first run`); `first` is cleared and dimmed with the hint `after sign-in`; an invalid name shows its reason under the field at once, not only on Enter; switching `agent` resets `account` to `default`. Only `claude`, `codex`, `cursor-agent` show the field.

- [ ] **Step 1: Failing tests:**
  - `TestAccountFieldCycles`: `N`, tab to account, `right` → `personal`, `right` → `work`, `right` → `+ new account`.
  - `TestNewAccountShowsSetupLikeDefault`: on `+ new account`, typing `work2` → the view has `setup` and `‹ like default ›`; Enter sends `NewSession{Account:"work2", NewAccount:true, Setup:"like-default"}`.
  - `TestBadNameSaysWhyAtOnce`: typing `Work` shows `a–z, 0–9 and -` under the field before Enter.
  - `TestFirstIsDimmedForANewAccount`: the first message is not sent and the view shows `after sign-in`.
  - `TestAgentChangeResetsAccount` and `TestOpencodeShowsNoAccount`.
  - `TestConfirmNamesTheAccount`: the confirm line reads `start claude in /work/app on work` and, for new, `new claude account "work2", like default: sign in in the session`.
- [ ] **Step 2: Run** `go test ./internal/app/ -run 'Account|Setup|First|Confirm'` → FAIL.
- [ ] **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: N picks the account, or makes one (#170)`.

### Task 8: USAGE and rows name the account

**Files:**
- Modify: `internal/app/dock.go:151-262` (`usageLines`, `accountNames`), the session row code in `internal/app/sessions.go`
- Test: `internal/app/dock_test.go`, `internal/app/sessions_test.go`

**Interfaces:**
- Consumes: `limits.Account.Dir`, `Session.AccountDir`, `Session.SigningIn`.

- [ ] **Step 1: Failing tests:**
  - `TestUsageNamesAccountBlocks`: a default claude block and a `work` claude block → lines start `✻ Max` (as today) and `✻ work`.
  - `TestHandMadeFolderIsNamedByItsBase`: `Dir` `/home/u/.claude-old` → `✻ .claude-old`.
  - `TestRowSaysSigningIn`: a session with `SigningIn` shows `signing in` in its row.
  - `TestDefaultOnlyLooksAsToday`: a snapshot with no `Dir` anywhere renders byte-for-byte as before (golden).
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: USAGE names each account (#170)`.

### Task 9: CLI, docs, changelog

**Files:**
- Modify: `cmd/matchblox/life.go` (`new`), `docs/agents.md`, `docs/limits.md`, `docs/keys.md`, `config.example.toml` (comment only), `CHANGELOG.md`, `design/0013-accounts.md` (`## Revised while building`)
- Test: `cmd/matchblox/cli_test.go`

**Interfaces:**
- Produces: `matchblox new … --account <name>`, `--new-account` (with `--account`), `--setup like-default|copy|empty` (default `like-default`; only with `--new-account`).

- [ ] **Step 1: Failing tests:**
  - `TestNewTakesAnAccount`: `new --agent codex --account work /work/app` sends `Account:"work"`.
  - `TestNewAccountNeedsAName`: `--new-account` without `--account` → usage error.
  - `TestSetupNeedsNewAccount`: `--setup copy` without `--new-account` → usage error.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Docs.** `docs/agents.md`: the accounts section (where folders live, what is shared, what is not, removing an account = deleting its folder, Cursor's `HOME` mirror, user-scope MCP servers not shared). `docs/limits.md`: blocks per account. `docs/keys.md`: the account and setup fields. CHANGELOG one line per user-facing change under `## [Unreleased]`.
- [ ] **Step 6: Run** `go test ./... && golangci-lint run` → PASS.
- [ ] **Step 7: Commit** `docs: accounts on N (#170)`.

---

## Addendum (2026-10-22, after PR #181): last-used account and pre-merge fixes

The owner asked: N must not make the builder re-pick (or re-sign-in) an account on every start. Rulings: the service remembers the last account started per agent and the forms open on it; `matchblox new` without `--account` stays on `default` (scripts stay predictable); `+ new account` is never preselected.

### Task 10: The service remembers the last account per agent

**Files:** `internal/service/lastaccount.go` (new), `internal/service/lifecycle.go`, `internal/service/service.go`, `internal/state/state.go`; tests `internal/service/lastaccount_test.go`.

**Interfaces:**
- Produces: `state.Doc.LastAccount map[string]string \`json:"last_account,omitempty"\`` — per agent (`claude`, `codex`, `cursor-agent`), the label of the account last started (as listed in `Doc.Accounts`; `default` included). Only labels present in that agent's `Doc.Accounts` at publish are sent.
- Kept in `<state dir>/last-accounts.json` (mode 0600, written atomically: temp file + rename), loaded when the service starts; a missing or unreadable file means none.
- Set after a start that ran in full (`len(res.Ran) == len(steps)` and no error), for existing and new accounts alike (a new account is remembered by its name), never for opencode, never for a stoker start.

- [ ] Tests: `TestLastAccountIsRememberedAfterAStart`; `TestLastAccountSurvivesARestart` (new Service on the same state dir reads it); `TestFailedStartIsNotRemembered`; `TestGoneAccountIsNotSent` (label no longer in Accounts → absent from Doc); `TestStokerStartIsNotRemembered`; `TestLastAccountFileIsPrivate` (0600).
- [ ] Commit `feat: matchblox remembers the last account of each agent (#170)`.

### Task 11: N opens on the last account

**Files:** `internal/app/lifeform.go`, `internal/app/model.go`; tests `internal/app/lifeform_test.go`.

- Consumes `state.Doc.LastAccount`. On open and on every agent change, the account field takes `LastAccount[agent]` when it is in the agent's list, else `default`. `+ new account` is never preselected. The flash on start already names the account.
- [ ] Tests: `TestNOpensOnTheLastAccount`; `TestAgentChangeTakesThatAgentsLastAccount`; `TestGoneLastAccountFallsBackToDefault`; `TestNewAccountIsNeverPreselected` (LastAccount names an account not in the list → default, not new).
- [ ] Docs: docs/keys.md and docs/agents.md (the field opens on the last account started for the agent; `matchblox new` without `--account` uses default). CHANGELOG line under Unreleased: "N opens on the account you last started for each agent; matchblox remembers it across restarts."
- [ ] Commit `feat: N opens on the account you last started (#170)`.

### Task 12: Pre-merge fixes from the final review

**Files:** `internal/sample/codex.go`, `internal/sample/sampler.go`, `internal/service/lifecycle.go`, `internal/accounts/prepare.go`, docs (`docs/agents.md`, `docs/limits.md`, `design/0013-accounts.md`), CHANGELOG; tests beside each.

- [ ] Held rollouts are claimed before the greedy loop (`taken` seeded with every held path). Test `TestHeldRolloutIsNotGivenToAnotherSession`.
- [ ] Wording: docs and design say each Codex session in one directory gets a different log, matched by start order, and that the match is right when each session's first turn comes in start order (not tested for other orders). docs/limits.md: Codex context and limits "tested with Codex 0.163".
- [ ] Cursor "signing in" limits hint only when that login's usage is read (`reads`). Test.
- [ ] A per-folder lock in the service held across Prepare and the start, so two confirms of one new account cannot both "make" it; the second is refused with "is there already". Test with two concurrent acts.
- [ ] A failed re-prepare of an existing like-default account does not block the start: the start runs, and the action's result says "some shared entries did not link: <first error>". Test.
- [ ] Codex `installation_id` is an own entry (accounts are not linked in Codex's telemetry). Update the own-entry fixture test and docs/agents.md.
- [ ] Commit(s) `fix: …(#170)` per group.
