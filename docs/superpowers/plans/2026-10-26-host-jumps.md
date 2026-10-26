# Host Jumps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A jump from a console to a session on a host shows one tmux, returns to the console it came from, moves no other client, and asks the builder's login at most once — in the TUI, secure way first, Tailscale SSH included.

**Architecture:** The laptop runs each jump in a private local tmux session; the host attaches it through a private grouped session with no status, no prefix and no mouse (package `remote`, file `jump.go`). The builder's own ssh login runs on one matchblox-owned control master per host, checked in the background when a host's console opens (`remote/login.go`). Every prompt ssh, ssh-keygen and ssh-add raise reaches the TUI through `matchblox askpass` over a per-console socket (new package `askpass`); the console offers the key ladder (new package `login`).

**Tech Stack:** Go, Bubble Tea v2, tmux ≥ 3.2 (laptop) / host minimum set in Task 2, OpenSSH ≥ 8.4.

**Spec:** `docs/superpowers/specs/2026-10-26-host-jumps-design.md`

## Global Constraints

- The stream's ssh (`remote.Target.ssh`) does not change: own key, `ControlMaster=no`, `ControlPath=none`, `BatchMode=yes`.
- Every remote command is argv, quoted by `remote.Quote`, wrapped by `onHost`, checked by an allowlist before ssh wraps it.
- Names: local jump session `matchblox-<host>-<8 hex>` (host `.`/`:` → `_`); host jump session `matchblox-jump-<8 hex>`, pattern `^matchblox-jump-[0-9a-f]{8}$`.
- Master: `ControlMaster=auto`, `ControlPath=<cache>/ssh/%C`, `ControlPersist=10m`, directory 0700. `<cache>` = `os.UserCacheDir()/matchblox`.
- Askpass: `SSH_ASKPASS=<this matchblox binary>`, `SSH_ASKPASS_REQUIRE=force`, `MATCHBLOX_ASKPASS=<socket>`. ssh runs the binary with the prompt as its one argument; `run` enters askpass mode only when `MATCHBLOX_ASKPASS` is set, there is exactly one argument, and it is not a command name in `commands`.
- No password, passphrase or key material on argv, in env, on disk or in logs. matchblox never creates a key without a passphrase (an empty answer to ssh-keygen's prompt is refused in the TUI).
- tmux target rules (verified 2026-10-26, tmux 3.4): a session target in `set-option` is `=<name>:`; a failing command does not stop a command list; `--` keeps the ssh argv as argv.
- Comments and copy: ASD-STE100 style, as the surrounding code. Public repo: no private host or person names in code, tests, docs or commits (`ws-1` in examples).
- Commits: signed Conventional Commits (`git commit -S`), each ending with the Co-Authored-By line; never `--no-verify`.

## Review Focus

1. **A builder whose tmux prefix is not C-b** (C-a, or a two-key prefix like `M-a`): the way back must work with that key. → Task 2 test `TestJumpWayBackWithPrefixCa`, Task 1 table rows for `C-a` and `M-a`.
2. **A host name with `.`, `:`, `@` or `[v6]`** (`me@build.example.com`, `[fe80::1]`): S must be a valid exact target and the allowlist must accept it. → Task 1 `TestHostSessionName` rows.
3. **The ssh drops mid-jump** (laptop sleeps): no S or G left behind, and the builder lands on the console. → Task 2 `TestJumpDropLeavesNothing`.
4. **A keyboard-interactive prompt with escape sequences** from a hostile server: it must not draw raw. → Task 5 `TestCleanPrompt`.
5. **Two consoles on one laptop open the same host at once:** they share the master socket; the second must not prompt again or break the first. → Task 5 `TestMasterReusedOnRealSSHD`.

---

### Task 1: Jump argv builders and allowlist

**Files:**
- Create: `internal/remote/jump.go`
- Test: `internal/remote/jump_test.go`
- (`NavArgv` stays until Task 3 deletes it with its callers, so every commit builds.)

**Interfaces:**
- Produces:
  - `type Jump struct { Host, Pane, Dir, Prefix, Home, ID string; Outer bool }` — exactly one of Pane (a pane id) or Dir (absolute, no `#`/`;`); `Prefix` the laptop's tmux prefix key name (`C-b`); `ID` 8 lowercase hex; `Outer` the console runs inside tmux; `Home` the laptop session the console runs in ("" → `matchblox`), the target of the way back.
  - `func NewJumpID() string` (crypto/rand, 8 hex)
  - `func HostSessionName(host, id string) string` → `matchblox-<host with . and : as _>-<id>`
  - `func JumpSessionName(id string) string` → `matchblox-jump-<id>`
  - `func JumpSession(name string) bool` — true for both name shapes (used by the sampler)
  - `func (j Jump) Remote() []string` — the host's one tmux command list
  - `func (j Jump) Local(ssh []string) []string` — the laptop's one tmux command list
  - `func JumpAllowed(j Jump) bool`
  - `func PrefixKey(s string) bool` — `^(C-|M-|C-M-)?[A-Za-z0-9\\]^_@[]$` or `None`-free tmux key names

- [ ] **Step 1: Write the failing tests**

`TestJumpRemoteToPane`: `Jump{Host:"ws-1", Pane:"%12", Prefix:"C-b", ID:"0a1b2c3d", Outer:true}.Remote()`, joined by `Line`, equals exactly:
`tmux bind-key -T matchblox-jump-C-b C-b switch-client -T matchblox-jump-C-b-prefix ';' bind-key -T matchblox-jump-C-b-prefix d detach-client ';' bind-key -T matchblox-jump-C-b-prefix m detach-client ';' new-session -d -t %12 -s matchblox-jump-0a1b2c3d ';' set-option -t =matchblox-jump-0a1b2c3d: destroy-unattached on ';' set-option -t =matchblox-jump-0a1b2c3d: status off ';' set-option -t =matchblox-jump-0a1b2c3d: prefix None ';' set-option -t =matchblox-jump-0a1b2c3d: key-table matchblox-jump-C-b ';' set-option -t =matchblox-jump-0a1b2c3d: mouse off ';' select-window -t <window of %12 in G> ';' select-pane -t %12 ';' attach-session -t =matchblox-jump-0a1b2c3d`
(the select-window target is fixed by Task 2's finding; write the test with the form Task 2 proves).
With `Outer:false`: `status on` and `status-left '◂ console: C-b m '` instead of `status off`.

`TestJumpRemoteToDir`: `Dir:"/w/a"` gives `new-session -d -s matchblox-jump-<id> -c /w/a` in place of the grouped `new-session`, and no select-window/select-pane.

`TestJumpLocal`: `Jump{Host:"ws-1",…}.Local([]string{"ssh","x"})` equals
`tmux new-session -d -s matchblox-ws-1-0a1b2c3d -- ssh x ';' set-option -t =matchblox-ws-1-0a1b2c3d: detach-on-destroy off ';' set-option -t =matchblox-ws-1-0a1b2c3d: key-table matchblox-host ';' set-option -t =matchblox-ws-1-0a1b2c3d: mouse on ';' set-option -t =matchblox-ws-1-0a1b2c3d: status-left '#[range=user|matchblox] ◂ console #[norange]' ';' bind-key -T matchblox-host User900 switch-client -t =matchblox ';' bind-key -T matchblox-host MouseDown1Status if -F '#{==:#{mouse_status_range},matchblox}' 'switch-client -t =matchblox' '' ';' switch-client -t =matchblox-ws-1-0a1b2c3d`
where `ssh x` is wrapped (pre-flight ruling R3): the session's command is `/bin/sh -c 'ssh "$@"; s=$?; [ $s = 0 ] || { printf "\nssh exited %s. Press Enter to go back.\n" "$s"; read _; }; exit 0' sh ssh x` — the ssh argv stays argv, no user value in the script text. No remain-on-exit: the session always ends when the builder leaves, and a failure waits for Enter so it can be read.

`TestHostSessionName` table: `ws-1`→`matchblox-ws-1-<id>`; `me@build.example.com`→`matchblox-me@build_example_com-<id>`; `[fe80::1]`→`matchblox-[fe80__1]-<id>`.

`TestJumpAllowed` table: valid pane jump true; pane `12` false; Dir relative false; Dir with `#` or `;` false; both Pane and Dir false; neither false; ID `XYZ` false; Host failing `ValidHost` false; Prefix `C-b`, `C-a`, `M-a` true; Prefix `;`, `#{x}`, `` false.

`TestJumpSession`: `matchblox-jump-0a1b2c3d` true, `matchblox-ws-1-0a1b2c3d` true, `matchblox` false, `matchblox-notes` false, `work` false.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/remote/ -run 'Jump|HostSession'` → undefined: Jump.
- [ ] **Step 3: Implement** `jump.go` per the Interfaces; `Remote` builds argv slices only (no string concatenation of user values except the fixed name formats).
- [ ] **Step 4: Run, expect PASS** — `go test ./...`.
- [ ] **Step 5: Commit** `feat(remote): jump argv for a private session on each side (#214)`.

### Task 2: The two-tmux-server guard

**Files:**
- Create: `internal/remote/jump_tmux_test.go`
- Modify: `internal/remote/jump.go` (fix forms the test proves wrong)

**Interfaces:**
- Consumes: Task 1 `Jump`, `Remote`, `Local`; `internal/tmuxtest.Server`.
- Produces: the host's minimum tmux version as `const MinHostTmux = "<x.y>"` in `jump.go`, and `const MinLocalTmux = "3.2"`.

The fake ssh: a script in `t.TempDir()` named `ssh` that ignores its options and runs its last argument with `/bin/sh -c` after replacing the leading `tmux` with `tmux -L <inner>` (use `Split` on the onHost line, then exec). Outer clients are real tmux clients started with `script -qfc 'tmux -L <outer> attach -t =matchblox' /dev/null` (Linux) — skip on macOS CI if `script` differs.

- [ ] **Step 1: Write the failing tests** (each starts outer server with session `matchblox` running `sleep 600`, inner server with session `work` of two windows):
  - `TestJumpOneStatusLine`: after Local+Remote, the outer client's session is S; inner G has `status off`; `capture-pane` of the outer S pane shows no `◂ matchblox`.
  - `TestJumpKeepsOtherClientsWindow`: a second inner client attached to `work` on window 0; jump to a pane in window 1; that client's `#{window_index}` stays 0.
  - `TestJumpWayBackHomeKey` and `…Tap`: send User900 / the status click to S; the outer client's session is `matchblox`.
  - `TestJumpWayBackWithPrefixCa`: outer prefix set to C-a, `Prefix:"C-a"`; send `C-a C-a m`; G is gone, S is gone, the outer client is on `matchblox`.
  - `TestJumpDropLeavesNothing`: kill the fake ssh process; within 2 s no S on outer, no G on inner, outer client on `matchblox`.
  - `TestJumpFailedSSHIsReadable`: fake ssh prints `ssh: connect to host ws-1 port 22: Connection refused` and exits 255; S stays and `capture-pane` contains `Connection refused` and `ssh exited 255`. The pane is alive and waiting; Enter ends S and the client is back on the console.
  - `TestTwoJumpsSameHost`: two outer clients jump to the same host; both S exist and neither client moved.
- [ ] **Step 2: Run, expect FAIL** where the Task 1 forms are wrong; fix `jump.go` and the Task 1 expectations together. Record the window target form that selects in G only.
- [ ] **Step 3: Find the minimum host tmux** from the features used (grouped sessions, `prefix None`, `key-table`, `destroy-unattached`); set `MinHostTmux` with a one-line reason.
- [ ] **Step 4: Run, expect PASS** — `go test ./internal/remote/ -run TestJump -count=1`.
- [ ] **Step 5: Commit** `test(remote): two tmux servers guard host jumps (#214)`.

### Task 3: The console jumps through Jump

**Files:**
- Modify: `internal/app/remote.go` (`forHost`), `internal/app/model.go` (Options, `confirm`), `internal/app/tabs.go:237` (shell on a host), `cmd/matchblox/main.go:489-491` (options), `cmd/matchblox/gate_test.go:69`
- Modify: `internal/sample/tmux.go` (`ParsePanes` skips `remote.JumpSession` sessions)
- Test: `internal/app/remote_test.go`, `internal/sample/tmux_test.go`

**Interfaces:**
- Consumes: Task 1 `Jump`, `NewJumpID`, `JumpAllowed`, `Jump.Local`, `Jump.Remote`; `remote.TermArgv`.
- Produces:
  - `Options.Prefix func() string` — the laptop tmux prefix (`tmux show-options -gv prefix`); nil or "" → `C-b`.
  - `Options.Home func() string` — the console's own tmux session (`tmux display-message -p '#{session_name}'`); nil or "" → `matchblox`.
  - `func (m Model) jumpOnHost(a *action) *action` — replaces the `a.nav` branch of `forHost`.
  - `func remote.JumpSSH(host string, j Jump, master []string) []string` — `ssh -t <master…> -o ForwardAgent=no -o ClearAllForwardings=yes -- host onHost(j.Remote())` (master is `nil` until Task 4).

- [ ] **Step 1: Write the failing tests**
  - `TestRemoteJumpInsideTmuxOpensItsOwnSession` (replaces `TestRemoteJumpAttachesOverSSH`): `onHost` model with `OutsideTmux:false`, a fixed ID source; enter, enter: `opt.Run` (not Exec) got exactly `j.Local(JumpSSH(...))`; Exec was not called; the confirm line shows the ssh line.
  - `TestRemoteJumpOutsideTmuxTakesTheTerminal`: `OutsideTmux:true`: Exec got `JumpSSH(...)` with `Outer:false`.
  - `TestRemoteShellOnHostIsPrivate`: the tabs shell action on a host gives a `Jump` with `Dir`.
  - `TestRemoteNavStillRefusesUnknownSteps`: unchanged, still passes.
  - `TestParsePanesSkipsJumpSessions`: lines from `matchblox-jump-0a1b2c3d` and `matchblox-ws-1-0a1b2c3d` are dropped; `work` stays first even when the jump session sorts first.
- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/app/ ./internal/sample/ -run 'Remote|ParsePanes'`.
- [ ] **Step 3: Implement.** `forHost`'s nav branch maps a `switch-client -t %N` or `attach-session` step to `Jump{Pane}`, a `new-window -c D` step to `Jump{Dir}`, anything else → return `a` unchanged (refused on confirm, as today). Inside tmux the action runs its single step with `opt.Run` (a tmux command list returns at once); outside, `exec`. Model takes the ID from `opt.JumpID func() string` (nil → `remote.NewJumpID`) so tests are fixed. Wire `Prefix` and `Home` in `main.go`. Delete `remote.NavArgv` and its test; fix `gate_test.go` to use `JumpSSH`. Inside tmux older than `MinLocalTmux`, the jump is refused with the flash `tmux <v> is older than 3.2: update tmux, or run the console outside tmux` (test `TestRemoteJumpOldLocalTmuxRefused`, with `Options.LocalTmux string`).
- [ ] **Step 4: Run, expect PASS** — `go test ./...`.
- [ ] **Step 5: Commit** `fix(app): a jump to a host opens in its own session, never in the console's pane (#214)`.

### Task 4: The host's tmux in Hello, and the login check

**Files:**
- Modify: `internal/proto/proto.go` (`Hello.Tmux string \`json:"tmux,omitempty"\``), `internal/service/service.go:766` (fill it from `tmux -V`, "" without tmux)
- Create: `internal/remote/login.go`, `internal/remote/login_test.go`, `internal/remote/testdata/login/*.txt`
- Create: `internal/app/login.go`, `internal/app/login_test.go`
- Modify: `internal/app/remote.go` (jump waits on the check; uses master opts), `cmd/matchblox/main.go` (cache dir)

**Interfaces:**
- Consumes: Task 3 `JumpSSH(host, j, master)`.
- Produces:
  - `func MasterOpts(dir string) []string` → `-o ControlMaster=auto -o ControlPath=<dir>/%C -o ControlPersist=10m`
  - `func EnsureMasterDir(cache string) (string, error)` — `<cache>/ssh`, created 0700, refused if it exists with wider mode or another owner.
  - `func CheckArgv(host, dir string) []string` → `ssh -v -o BatchMode=yes <MasterOpts> -o ConnectTimeout=10 -- host true`
  - `type Login int` with `LoginOK, LoginPassword, LoginTailscaleCheck, LoginTailscaleDenied, LoginHostKeyUnknown, LoginHostKeyChanged, LoginUnreachable`
  - `func ClassifyLogin(stderr string, exit int) (Login, string)` — the string is the Tailscale URL or reason.
  - `func Tailscale(stderr string) bool` — from the `-v` line `Remote protocol version 2.0, remote software version <v>`.
  - `func TmuxTooOld(v, min string) bool` (in `remote`); app shows the tmux door when the host's `Hello.Tmux` is "none" or too old; "" (older host) tries.
  - app: `loginMsg{host string; login remote.Login; detail string}`; Model field `login remote.Login` and `checking bool`.

- [ ] **Step 1: Record the OpenSSH fixtures.** From this machine: `ssh -v -o BatchMode=yes -o PreferredAuthentications=password <a host with no key> true` → `openssh-password.stderr`; host key unknown and changed from `sshd_test.go`'s sshd. Sanitize as the Tailscale ones are (no real IPs, paths or names). The Tailscale fixtures exist (recorded live 2026-10-26 from tailscale/tailscale:v1.104.2, default policy, `internal/remote/testdata/login/`, exit codes in `exits.txt`):
  - banner: `debug1: Remote protocol version 2.0, remote software version Tailscale` (`ssh-keyscan`: `SSH-2.0-Tailscale`);
  - check: `# Tailscale SSH requires an additional check.` then `# To authenticate, visit: <url>`; after approval `# Authentication checked with Tailscale SSH.`, exit 0;
  - denied: `tailscale: tailnet policy does not permit you to SSH as user "<user>"`, exit 255;
  - ok: authenticated `using "none"`, exit 0.
- [ ] **Step 2: Write the failing tests** — `TestClassifyLogin` over each fixture (expect the Login and, for check, the exact URL); `TestMasterDirMode` (0700 made; 0755 existing refused); `TestHelloCarriesTmux` (service fills a non-empty version when `tmux` is on PATH); app `TestHostOpenRunsLoginCheck` (opening a host's console issues exactly `CheckArgv`, through `opt.Run`-like `opt.Check func(argv []string) (stderr string, exit int)`), `TestJumpWaitsForLoginCheck`, `TestJumpUsesMaster` (jump ssh contains `MasterOpts`), `TestOldHostTmuxShowsDoor`.
- [ ] **Step 3: Run, expect FAIL.**
- [ ] **Step 4: Implement.** `ClassifyLogin` reads the Tailscale lines above verbatim; the check URL is the token after `visit: ` and must match `^https://login\.tailscale\.com/[A-Za-z0-9/]+$` before it is shown. The check runs once per host console open, in a tea.Cmd, never blocking draw. LoginOK → nothing shown. Others → Task 6/7 surfaces (until then a one-line flash naming the result and its fix).
- [ ] **Step 5: Run, expect PASS** — `go test ./...`.
- [ ] **Step 6: Commit** `feat: one ssh master per host and a login check before a jump (#214)`.

### Task 5: `matchblox askpass` and the prompt socket

**Files:**
- Create: `internal/askpass/askpass.go`, `internal/askpass/askpass_test.go`
- Modify: `cmd/matchblox/main.go` (`run`: when `MATCHBLOX_ASKPASS` is set, `run` serves the prompt before any other dispatch)
- Test: `cmd/matchblox/sshd_test.go` (new test)

**Interfaces:**
- Produces:
  - `type Prompt struct { Host, Text string; Confirm bool }` — `Confirm` for yes/no prompts (`(yes/no` in Text); `Text` already cleaned.
  - `type Server struct{…}`; `func Listen(dir string) (*Server, error)` — socket in a fresh 0700 dir under `dir`, mode 0600, 32-byte random token.
  - `func (s *Server) Env(host string) []string` → `SSH_ASKPASS=<self>`, `SSH_ASKPASS_REQUIRE=force`, `MATCHBLOX_ASKPASS=<socket>`, `MATCHBLOX_ASKPASS_TOKEN=<token>`, `MATCHBLOX_ASKPASS_HOST=<host>` (token in env is the helper's key to the socket, not a secret of the builder).
  - `func (s *Server) Next(ctx) (Prompt, func(answer string, ok bool), error)` — one prompt at a time; `ok=false` makes the helper exit 1 (ssh cancels).
  - `func (s *Server) Close() error`
  - `func Ask(env func(string) string, prompt string, out io.Writer) error` — the helper side.
  - `func Clean(s string) string` — drops C0/C1 controls and ESC sequences, keeps printable text, caps at 200 runes.

- [ ] **Step 1: Write the failing tests**
  - `TestAskRoundTrip`: Ask with the right token gets the answer on `out` with a trailing newline.
  - `TestAskWrongTokenRefused`: Ask exits with error; `Next` never yields.
  - `TestCleanPrompt`: `"\x1b]0;pwn\x07Password: \x1b[31m"` → `"Password: "`; a 500-rune prompt → 200 runes.
  - `TestConfirmPrompt`: `"Are you sure you want to continue connecting (yes/no/[fingerprint])? "` → `Confirm:true`.
  - `TestNothingLogged`: run a round trip with a `slog` handler capturing everything; the answer string appears nowhere.
  - `TestMasterReusedOnRealSSHD` (sshd_test.go): builder key encrypted with passphrase `p4ss`; ssh with `Env()` and `MasterOpts`; the server answers one prompt (`Enter passphrase for key`) with `p4ss`; a second ssh to the same host with the same ControlPath gets no prompt and exits 0.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Protocol: one JSON line each way (`{"token","host","text"}` → `{"answer","ok"}`); the server compares tokens with `subtle.ConstantTimeCompare`; answers are written once and the byte slice zeroed.
- [ ] **Step 4: Run, expect PASS** — `go test ./internal/askpass/ ./cmd/matchblox/ -run 'Ask|Clean|Confirm|Logged|MasterReused'`.
- [ ] **Step 5: Commit** `feat(askpass): ssh prompts go to the console, never the raw terminal (#214)`.

### Task 6: Prompts in the TUI

**Files:**
- Create: `internal/app/askfield.go`, `internal/app/askfield_test.go`
- Modify: `internal/app/model.go` (Options `Ask AskSource`; Update routes keys to the field while one is open), `cmd/matchblox/main.go` (one `askpass.Server` per console run; its `Env` added to every builder-login ssh, ssh-keygen and ssh-add the console starts)

**Interfaces:**
- Consumes: Task 5 `Prompt`, `Server.Next`, `Server.Env`.
- Produces: `type AskSource interface { Next(context.Context) (askpass.Prompt, func(string, bool), error); Env(host string) []string }`; the field's render shows host, cleaned text, masked input (`•` per rune), and the keys line.

- [ ] **Step 1: Write the failing tests**
  - `TestAskFieldMasks`: type `abc`; the render has `•••` and never `abc`; enter answers `abc`, ok.
  - `TestAskFieldEscCancels`: esc answers ok=false; the field closes; the flash says the ssh was cancelled.
  - `TestAskFieldConfirmNeedsTypedYes`: a Confirm prompt: `y` + enter does not answer; `yes` + enter answers `yes`.
  - `TestAskFieldRefusesEmptyNewPassphrase`: prompt text `Enter passphrase (empty for no passphrase): ` with empty input: enter keeps the field open and says a passphrase is needed.
  - Golden screens at 44×31 and 104×38 (`testdata/screens/ask-*.txt`) with the replay harness.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** as an overlay layer like the existing confirm (`focus.go` layer order: the ask field is above everything).
- [ ] **Step 4: Run, expect PASS** — `go test ./internal/app/ -run AskField` and the golden screen test.
- [ ] **Step 5: Commit** `feat(app): ssh asks in the console, masked, with a typed yes for host keys (#214)`.

### Task 7: The key ladder

**Files:**
- Create: `internal/login/ladder.go`, `internal/login/ladder_test.go`, `internal/login/install.go`, `internal/login/install_test.go`
- Create: `internal/app/ladder.go`, `internal/app/ladder_test.go`
- Modify: `internal/remote/remote.go` (`ValidUserKey`)

**Interfaces:**
- Consumes: Task 4 `LoginPassword`; Task 6 askpass env on every command here.
- Produces:
  - `type Source int` (`FromAgent, FromFile, NewKey, NotNow`); `type Step struct { Source Source; Path, Line, Fingerprint string; NoPassphrase bool }`
  - `func Ladder(agent []string, sshDir string) Step` — agent lines from `ssh-add -L`; order: agent `sk-ssh-ed25519@openssh.com` > agent `ssh-ed25519` > files `id_ed25519_sk`, `id_ed25519`, `id_ecdsa_sk`, `id_ecdsa`, `id_rsa` (first with a `.pub`) > `NewKey`.
  - `func HasNoPassphrase(path string) bool` — `ssh-keygen -y -P "" -f path` exits 0.
  - `func InstallArgv(host string, line string, master []string) ([]string, error)` (the askpass env goes on the `exec.Cmd`, never in argv) — refuses a line failing `ValidUserKey`; remote script fixed text: `umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; grep -qxF "$1" ~/.ssh/authorized_keys || printf '%s\n' "$1" >> ~/.ssh/authorized_keys` with the key as `$1`.
  - `func NewKeyArgv(path string) []string` → `ssh-keygen -q -t ed25519 -C matchblox-login -f <path>` (passphrase via askpass; verified 2026-10-26).
  - `func AddArgv(path, goos string) []string` → `ssh-add [--apple-use-keychain] <path>`.
  - The macOS `~/.ssh/config` lines (`Host <host>\n  AddKeysToAgent yes\n  UseKeychain yes\n`) are a setup door `doors.SSHKeychain` in `internal/setup/sshconfig.go`, built like the way-back door (`internal/setup/tmux.go`): preview diff, append once, keep the file's bytes. Test `TestSSHKeychainDoorAppendsOnce`.
  - `remote.ValidUserKey(line string) bool` — `ssh-ed25519`, `sk-ssh-ed25519@openssh.com`, `ecdsa-sha2-nistp256/384/521`, `sk-ecdsa-sha2-nistp256@openssh.com`, `ssh-rsa`; base64 body; optional comment `[A-Za-z0-9@._-]+`.

- [ ] **Step 1: Write the failing tests**
  - `TestLadderOrder` table over fake agent lines and a temp `~/.ssh`: each row expects the Source and Path.
  - `TestHasNoPassphrase`: a `-N ""` key true; a passphrase key false.
  - `TestInstallArgvRefusesBadLine`: `ssh-ed25519 AAAA; rm -rf ~` → error.
  - `TestInstallOnRealSSHD` (sshd_test.go): install twice; `authorized_keys` has the line once, mode 0600, `.ssh` 0700.
  - `TestNewKeyHasPassphrase`: NewKeyArgv with a test askpass answering `p4ss` twice; `HasNoPassphrase` false.
  - app `TestPasswordLoginOffersLadder`: `LoginPassword` shows the top step, its exact command and fingerprint; `y` runs it; `n` picks NotNow and the jump proceeds (prompting in the field).
  - app `TestNoPassphraseKeyWarns`: a FromFile step with NoPassphrase shows `ssh-keygen -p -f <path>`.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** After a successful install, the console re-runs the login check; LoginOK clears the offer.
- [ ] **Step 4: Run, expect PASS** — `go test ./internal/login/ ./internal/app/ ./cmd/matchblox/`.
- [ ] **Step 5: Commit** `feat: the console sets up your own key once, secure way first (#214)`.

### Task 8: Tailscale SSH

**Files:**
- Modify: `internal/app/login.go`, `internal/app/layers.go` (host row warning), `internal/setup/paircheck.go` (export the #185 message as `TailscaleGateMessage`)
- Test: `internal/app/login_test.go`, `internal/app/layers_test.go`

**Interfaces:**
- Consumes: Task 4 `ClassifyLogin`, `Tailscale`; fixtures from Task 4 Step 1.
- Produces: `Shell` host rows carry `tailscale bool` from the last check.

- [ ] **Step 1: Write the failing tests**
  - `TestTailscaleCheckShowsURL`: `LoginTailscaleCheck` with URL U: the render has U as an OSC 8 link; `c` copies U through `opt.Clip`; the ladder is never offered; when the check exits 0 the jump proceeds; esc cancels.
  - `TestTailscaleDeniedSaysWhy`: the reason line and a pointer to docs/remote.md#tailscale-ssh.
  - `TestTailscaleHostRowWarns`: the Hosts row shows the #185 message, `tailscale set --ssh=false` first.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** The check runs with a 5-minute limit while a Tailscale URL is shown (the 10 s ConnectTimeout stays for the TCP connect).
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** `feat: jumps work on Tailscale SSH hosts, and the console says the gate does not hold there (#214)`.

### Task 9: Docs and changelog

**Files:**
- Modify: `docs/remote.md` (sections: "Jump into a session", "Log in once", "Tailscale SSH"), `docs/tmux.md` (the way back without config), `CHANGELOG.md` `## [Unreleased]` (### Fixed: the nested matchblox loop; ### Added: login once in the console), `design/0005-remote-mode.md` §4 A07 (one line pointing to the new spec)
- Test: existing `cmd/matchblox/docs_test.go` and `internal/docscheck` pass.

- [ ] **Step 1:** Write the docs in the voice of `docs/remote.md` (ASD-STE100, one idea per sentence).
- [ ] **Step 2:** `go test ./cmd/matchblox/ -run Docs ./internal/docscheck/` → PASS; `.github/scripts/changelog-guard.sh` → PASS.
- [ ] **Step 3: Commit** `docs: jumps, the login asked once, and Tailscale SSH (#214)`.

### Task 10: Done-when on real machines

- [ ] **Step 1:** `go test ./... -count=1` and `golangci-lint run` → clean.
- [ ] **Step 2 (needs the owner):** from the laptop console in tmux, with this machine added as a host: open a session → one status line; first open asks once in the TUI and offers the key; after `y`, a second open asks nothing; prefix m, the `◂` tap and prefix prefix m return to the laptop's console; the phone on this machine's shared console did not move; a second laptop driving at once did not move.
- [ ] **Step 3:** Open the PR closing #214; Project status In review.
