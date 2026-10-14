# The iOS window Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A builder pairs the matchblox iPhone app with one scan and opens their own console on their host, with no Termius, no account and no password.

**Architecture:** Two parts, two PRs. Part A (Go, #87, plus the host half of #88): the gate also lets the console and a one-time pairing through, `matchblox pair` shows a QR code with a one-time ed25519 key that sshd expires after 5 minutes, and `matchblox devices` lists and removes keys. Part B (Swift, #88): a SwiftUI app in `ios/` whose logic lives in a Swift package `ios/MatchbloxKit`. The app holds a Secure Enclave P-256 key, speaks SSH with SwiftNIO SSH and draws the console with SwiftTerm. The only contract between the parts is one fixture, `testdata/pair/v1.json`, which the Go tests and the Swift tests both read.

**Tech Stack:** Go 1.26 (existing), `rsc.io/qr` (new, BSD-3); Swift 5.10, SwiftUI, CryptoKit, VisionKit, XcodeGen, SwiftTerm (MIT), swift-nio-ssh (Apache-2.0); GitHub Actions `macos-15` for the Swift side.

**Spec:** design/0006-anywhere.md (§6.1, §6.2, §6.3, §6.5). Issues #87 and #88.

## Global Constraints

- No server of ours in the path. Reaching the host is the builder's job (0006 §2, non-goal 2).
- No account, no password, no login screen (0006 anti-goal 3).
- The console is the only UI. The app draws no screen that the console already has (0006 anti-goal 1).
- The gate lets through exactly: `matchblox serve --stdio`, `matchblox console`, and `gate pair` for a one-time key. Everything else exits 126. (`matchblox open` and `matchblox devices token` come with #89, not here.)
- Device key line: `restrict,pty,command="'<exe>' gate phone" <key> matchblox:phone:<name>`. A laptop key (`'<exe>' gate`) keeps the stream alone: only `gate phone` also opens the console.
- One-time key line: `restrict,expiry-time="<system-zone YYYYMMDDHHMM>",command="'<exe>' gate pair <id>" <key> matchblox:pair`. It expires 5 minutes after `matchblox pair` starts (rounded up to the next whole minute). The time is in the system zone, read from `/etc/localtime`, with no `Z`: sshd reads it so from OpenSSH 7.7, and a `Z` (UTC) needs OpenSSH 9.1, where older sshd rejects the whole line. It works once.
- Pairing URL: `matchblox://pair?p=<base64url, no padding, of the payload JSON>`.
- iOS deployment target 17.2 (the Live Activity of #90 needs it). Swift 5.10, `SWIFT_STRICT_CONCURRENCY: complete`, iPhone only, as in the blox app (`~/dev/blox/ios/project.yml`).
- Bundle id `com.blox.matchblox`. The team is not set in `project.yml`; each builder sets it in Xcode until the owner names the team.
- Docs and console copy follow ASD-STE100 and DESIGN.md (one meaning for each sign, no dead ends: each error names its fix).
- Commits follow Conventional Commits (the PR title check enforces it).

## Review Focus

1. **The address in the QR does not work from the phone** (the host's own hostname on a LAN, for example). The builder must be able to change the address on the pairing screen before the app connects. → Task B4 test `testDraftAddressOverride`.
2. **A QR scanned a second time, or after Ctrl-C.** The one-time key is gone, so sshd refuses it. The app must say "This code was used or expired. Run matchblox pair on ws-1 again." and not a raw SSH error. → Task A7 (second login fails), Task B4 test `testUsedCodeMessage`.
3. **An authorized_keys with other people's lines, no final newline or a symlink.** Every line that is not ours keeps its bytes. → Task A1 test `TestAuthorizeDeviceKeepsOtherLines`.
4. **A phone name with quotes, a newline or an emoji.** iOS 16+ reports every device as "iPhone", so the builder types the name. The host refuses a bad name; the app cleans the name before it sends it. → Task A1 test `TestValidDeviceName`, Task B4 test `testCleanName` (the same cases, from the fixture).
5. **The phone rotates or shows the keyboard while a laptop is attached.** The pty gets a window-change, and the shared session takes the size of the device used last (`window-size latest`). → Task A3 test `TestSizeArgv`, Task B6 manual check on a device.

---

## Part A: the host (Go). PR 1: `feat: a paired phone opens the console (#87)`

### Task A0: Branch and commit the design

**Files:**
- Modify: `design/0001-v0.1.md:70`. Remove "starting agents" from "Not planned". Keep "a prompt editor, diff review".
- Add: `design/0006-anywhere.md`, `design/0007-ios-window-plan.md` (both already written).

- [ ] **Step 1:** `git switch -c feat/87-ios-window`
- [ ] **Step 2:** Edit design/0001 line 70 as stated above.
- [ ] **Step 3:** Run `go test ./cmd/matchblox/ -run Docs`. Expected: PASS (the design docs are not checked, but the run proves the tree builds).
- [ ] **Step 4: Commit**

```bash
git add design/0001-v0.1.md design/0006-anywhere.md design/0007-ios-window-plan.md
git commit -m "docs: matchblox anywhere: the design and the plan of the iOS window (#87, #88)"
```

### Task A1: Device keys in authorized_keys

**Files:**
- Create: `internal/setup/devices.go`, `internal/setup/devices_test.go`
- Modify: `internal/remote/remote.go` (add `ValidDeviceKey`, `ConsoleCommand`)
- Test: `internal/remote/remote_test.go`

**Interfaces:**
- Produces:
  - `remote.ConsoleCommand = "matchblox console"`
  - `remote.ValidDeviceKey(k string) bool`: true only for `ecdsa-sha2-nistp256 <base64> [comment]` whose blob decodes to the three SSH strings `ecdsa-sha2-nistp256`, `nistp256` and a 65-byte uncompressed point (first byte 0x04).
  - `setup.ValidDeviceName(s string) bool`: 1 to 40 runes of letters, digits, space, `.`, `_`, `-`, `'` and `’`. No leading or trailing space.
  - `setup.AuthorizeDevice(home, exe, pubKey, name string) (backup string, err error)`: same rules as `Authorize` (exePath, backup, a line with the same key body is replaced), and writes the device key line of Global Constraints. The comment is `matchblox:phone:<name>`.
  - `type Device struct { Kind, Name, Fingerprint string }`. Kind is `phone` for a `matchblox:phone:` comment, else `laptop`. Fingerprint from `setup.Fingerprint`.
  - `setup.Fingerprint(authorizedKey string) (string, error)`: `SHA256:` + unpadded standard base64 of the SHA-256 of the key blob, the same as `ssh-keygen -lf`.
  - `setup.Devices(home string) ([]Device, error)`: every line whose command is `'<any>' gate` (laptop) or `'<any>' gate phone`, in file order; never a `gate pair <id>` line.
  - `setup.RemoveDevice(home, name string) (backup string, err error)`: removes the one line of that phone name, or the line whose fingerprint is `name`. Error `no device named <name>` or `<n> devices named <name>; give the fingerprint`.

- [ ] **Step 1: Write the failing tests**

```go
func TestValidDeviceKey(t *testing.T)            // a key from `ssh-keygen -t ecdsa -b 256` passes; ssh-ed25519, ecdsa-sha2-nistp384, a cut blob, "x y" fail
func TestValidDeviceName(t *testing.T)           // read the cases from testdata/pair/v1.json "names": {"good": [...], "bad": [...]}
func TestAuthorizeDeviceWritesTheGateLine(t *testing.T)
// want line: `restrict,pty,command="'/usr/local/bin/matchblox' gate phone" ecdsa-sha2-nistp256 AAAA… matchblox:phone:Jordan’s iPhone`
func TestAuthorizeDeviceKeepsOtherLines(t *testing.T)
// old file: "ssh-rsa AAAA other@box" with no final newline, plus a laptop gate line; both keep their bytes; a backup is made
func TestDevicesAndRemoveDevice(t *testing.T)
// a laptop line and two phones; Devices returns 3 in file order with kinds; RemoveDevice("Jordan’s iPhone") leaves 2; an unknown name errors
func TestFingerprintMatchesSSHKeygen(t *testing.T) // setup.Fingerprint(fixture.hostkey_pub) == fixture.hostkey_fp
```

- [ ] **Step 2:** Run `go test ./internal/setup/ ./internal/remote/ -run 'Device'`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement the interfaces above. Reuse `read`, `keep` and `replace` from `setup.go`. Write the `names`, `hostkey_pub` and `hostkey_fp` blocks of `testdata/pair/v1.json` now (the fingerprint from `ssh-keygen -lf`) (good: `iPhone`, `Jordan’s iPhone`, `Desk-2`; bad: empty, 41 runes, `a"b`, `a\nb`, `🔥`, ` lead`).
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(setup): authorize a phone key for the gate, and list and remove devices`

### Task A2: The one-time pairing key

**Files:**
- Modify: `internal/setup/devices.go`, `internal/setup/devices_test.go`

**Interfaces:**
- Produces:
  - `setup.AddPairKey(home, exe, pubKey, id string, until time.Time) error`: writes the one-time key line of Global Constraints. `until` is written in UTC as `200601021504` plus `Z`. `pubKey` must pass `remote.ValidPubKey` (ed25519). `id` is 12 lowercase hex characters.
  - `setup.RemovePairKey(home, id string) (removed bool, err error)`: removes the line whose command is `'<exe>' gate pair <id>`.
  - `setup.PairKeyPresent(home, id string) (bool, error)`

- [ ] **Step 1: Write the failing tests**

```go
func TestAddPairKeyLine(t *testing.T)
// until 2026-10-14 14:37 UTC → `restrict,expiry-time="202610141437Z",command="'/usr/local/bin/matchblox' gate pair 0123456789ab" ssh-ed25519 AAAA… matchblox:pair`
func TestRemovePairKey(t *testing.T) // present → removed=true and gone; a second call → removed=false, nil error; other lines keep their bytes
```

- [ ] **Step 2:** Run `go test ./internal/setup/ -run PairKey`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run again. Expected: PASS.
- [ ] **Step 5: Commit** `feat(setup): a one-time pairing key that sshd expires after five minutes`

### Task A3: The gate opens the console

**Files:**
- Modify: `cmd/matchblox/gate.go`, `cmd/matchblox/gate_test.go`, `cmd/matchblox/main.go` (pass `args[1:]` to `gateMain`; call the size step), `internal/panes/home.go`, `internal/panes/home_test.go`

**Interfaces:**
- Consumes: `remote.ConsoleCommand` (A1).
- Produces:
  - `gateDecision(mode, orig string) (argv []string, err error)`: mode "" (laptop key) allows `remote.ServeCommand` → `["serve", "--stdio"]` only; mode `phone` also allows `remote.ConsoleCommand` → `[]string{}` (the console); an unknown mode allows nothing.
  - `gateMain(args []string) error`: `args == ["pair", <id>]` → `pairMain` (Task A5); `[]` → mode ""; `["phone"]` → mode `phone`; other args exit 126; else the decision on `SSH_ORIGINAL_COMMAND`.
  - `panes.SizeArgv() []string` = `["tmux", "set-option", "-w", "-t", "=matchblox:", "window-size", "latest"]`. The console runs it once at start when `MATCHBLOX_HOME=1`, and ignores its error.

- [ ] **Step 1: Write the failing tests.** Rename `TestGateRunsOnlyTheServiceStream` to `TestGateRunsOnlyItsCommands`. Add `remote.ConsoleCommand` to the allowed list (want argv `[]string{}`). Add to the refused list: `"matchblox console; id"`, `"matchblox console "`, `"matchblox  console"`, `"matchblox gate pair 0123456789ab"` (pair comes only from the forced command, never from the requested command). Add `TestSizeArgv` in `internal/panes/home_test.go`, asserting the exact argv above.
- [ ] **Step 2:** Run `go test ./cmd/matchblox/ ./internal/panes/ -run 'Gate|SizeArgv'`. Expected: FAIL.
- [ ] **Step 3:** Implement. Keep the comment on why a tmux client is never a gate command, and add one line: the console command attaches the shared home session, where `goHome` already decides tmux or in place.
- [ ] **Step 4:** Run again. Expected: PASS.
- [ ] **Step 5: Commit** `feat(gate): a paired key opens the console, and the console takes the size of the last device (#87)`

### Task A4: The pairing payload and the QR

**Files:**
- Create: `internal/pair/pair.go`, `internal/pair/qr.go`, `internal/pair/pair_test.go`, `testdata/pair/v1.json` (complete it)
- Modify: `go.mod` (`rsc.io/qr`)

**Interfaces:**
- Produces:
  - `type Payload struct { V int "json:\"v\""; Host string "json:\"host\""; Port int "json:\"port\""; User string "json:\"user\""; Name string "json:\"name\""; HostKey string "json:\"hostkey\""; Key string "json:\"key\"" }` (the field order is the JSON order). `HostKey` is a `SHA256:` fingerprint. `Key` is the 32-byte ed25519 seed in standard base64.
  - `(p Payload) URL() string`, `pair.Parse(url string) (Payload, error)`. Parse refuses `v != 1`, a missing field, a port outside 1..65535 and a seed that is not 32 bytes.
  - `type Request struct { Name string "json:\"name\""; Key string "json:\"key\"" }`, `type Reply struct { OK bool "json:\"ok\""; Host string "json:\"host,omitempty\""; Error string "json:\"error,omitempty\"" }`. Each is one JSON line.
  - `pair.NewKey() (seed []byte, authorized, id string, err error)`: `authorized` is `ssh-ed25519 <b64>`; `id` is the first 12 hex characters of the SHA-256 of the public key blob.
  - `pair.QR(text string) ([]string, error)`: medium error correction, a quiet zone of 2 modules, two modules in each cell with `▀ ▄ █` and space. Dark modules are spaces on a light block, so it scans on a dark terminal.
- `testdata/pair/v1.json` gains: `payload`, `url`, `request`, `request_line`, `replies` (`ok`, `bad name`, `bad key`), `p256_x963_hex` with `p256_openssh` (one fixed key from `ssh-keygen -t ecdsa -b 256`), and `names`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPayloadURLMatchesTheFixture(t *testing.T)  // fixture.payload.URL() == fixture.url; Parse(fixture.url) == fixture.payload
func TestParseRefuses(t *testing.T)                // v 2, no host, port 0, a 31-byte seed, not base64url, another scheme
func TestRequestAndReplyLines(t *testing.T)        // json.Marshal(fixture.request)+"\n" == fixture.request_line; each reply line round-trips
func TestNewKeyID(t *testing.T)                    // len(id)==12, hex; authorized passes remote.ValidPubKey; ed25519.NewKeyFromSeed(seed) gives the same public key
func TestQRSize(t *testing.T)                      // for the fixture URL: len(lines) == (size+4+1)/2, every line has size+4 runes
```

- [ ] **Step 2:** Run `go test ./internal/pair/`. Expected: FAIL.
- [ ] **Step 3:** Implement. The Go tests and the Swift tests read the fixture at `testdata/pair/v1.json` relative to the repo root.
- [ ] **Step 4:** Run again. Expected: PASS. Run `go test ./internal/ -run Arch`. Expected: PASS (`internal/app` does not import `pair`).
- [ ] **Step 5: Commit** `feat(pair): the pairing payload, its URL and its QR code`

### Task A5: `matchblox pair` and `gate pair`

**Files:**
- Create: `cmd/matchblox/pair.go`, `cmd/matchblox/pair_test.go`
- Modify: `cmd/matchblox/main.go` (add `pair` to `commands`; dispatch)

**Interfaces:**
- Consumes: A1, A2, A4.
- Produces:
  - `pairMain(home, exe string, in io.Reader, out io.Writer, id string) error` (gate pair): reads one `pair.Request` line, checks `remote.ValidDeviceKey` and `setup.ValidDeviceName`, then `setup.AuthorizeDevice` and `setup.RemovePairKey(home, id)`. It writes one `pair.Reply` line. Errors: `bad name`, `bad key`, and the error text of a failed write. The reply on success holds the host name.
  - `pairCmd(ctx context.Context, args []string, out io.Writer, now func() time.Time) error`. Flags: `--address host` (default: the `Self.DNSName` of `tailscale status --json` without its final dot, else `os.Hostname()`), `--port 22`, `--name` (default: the short hostname). It does `pair.NewKey`, `setup.AddPairKey(until = now()+5m)` and prints the QR, then the URL, then:
    `Scan with the iPhone camera. The code works once, until 14:37.`
    It checks `PairKeyPresent` each second. When the line is gone, it prints `Paired. matchblox devices lists it.` On timeout or ctx cancel, it calls `RemovePairKey` and prints `The code expired. Run matchblox pair again.` The host key comes from `/etc/ssh/ssh_host_ed25519_key.pub`; if that file is missing, it says so and names the fix.

- [ ] **Step 1: Write the failing tests**

```go
func TestPairMainAuthorizesAndRemovesTheCode(t *testing.T) // fake home with a pair line for id; fixture.request_line in → fixture.replies.ok out; Devices has the phone; PairKeyPresent false
func TestPairMainRefusesABadName(t *testing.T)             // reply "bad name"; authorized_keys unchanged (the pair line stays: sshd expires it)
func TestPairCmdExpires(t *testing.T)                      // ctx cancelled at once → the pair line is gone; output ends with the expired line
```

- [ ] **Step 2:** Run `go test ./cmd/matchblox/ -run Pair`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run again. Expected: PASS.
- [ ] **Step 5: Commit** `feat: matchblox pair shows a one-time QR code, and a scan adds the phone (#88)`

### Task A6: `matchblox devices`, and the docs

**Files:**
- Create: `cmd/matchblox/devices.go`
- Modify: `cmd/matchblox/main.go` (add `devices` to `commands`), `cmd/matchblox/docs_test.go` (`commandFlags` for `pair`: `address`, `port`, `name`), `docs/phone.md`, `docs/remote.md`, `docs/safety.md`, `CHANGELOG.md` (Unreleased)

**Interfaces:**
- Consumes: `setup.Devices`, `setup.RemoveDevice`.
- Produces: `devicesCmd(args []string, home string, out io.Writer) error`. `matchblox devices` prints one row for each key: kind, name and fingerprint, in aligned columns, `phone` rows first. `matchblox devices rm <name|fingerprint>` removes one and prints `Removed <name>. Backup: <path>`.

- [ ] **Step 1: Write the failing test** `TestDevicesListAndRemove` (golden output for one laptop and two phones; `rm` of an unknown name returns the error of A1).
- [ ] **Step 2:** Run `go test ./cmd/matchblox/ -run Devices`. Expected: FAIL.
- [ ] **Step 3:** Implement. Docs, in ASD-STE100:
  - docs/phone.md: a new first section, "The matchblox app" (pair, open, a new address, remove a phone). Termius moves to "Without the app".
  - docs/remote.md: `matchblox devices`.
  - docs/safety.md: "A phone key can go to every agent pane, so it has your power. The Secure Enclave and Face ID protect it. Remove a lost phone with `matchblox devices rm`."
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS (the docs test checks the new commands and flags).
- [ ] **Step 5: Commit** `feat: matchblox devices lists the phones and laptops that open this host, and removes one`

### Task A7: The whole pairing on a real sshd

**Files:**
- Modify: `cmd/matchblox/sshd_test.go`

**Interfaces:**
- Consumes: `startSSHD` (existing), A1 to A5.

- [ ] **Step 1: Write the test** `TestPairOnARealSSHD`:
  1. Make a one-time key with `ssh-keygen -t ed25519`. Call `setup.AddPairKey(far home, bin, pub, id, now+5m)`.
  2. `ssh -i onetime host` with `fixture.request_line` on stdin → stdout is the `ok` reply.
  3. The same command again → ssh fails (the code works once).
  4. `ssh -i phone host id` (the phone key from `ssh-keygen -t ecdsa -b 256`) → exit 126.
  5. `ssh -i phone host matchblox serve --stdio` → the hello of the service stream, as in `TestConnectAndGateOnARealSSHD`.
  6. A second pair key with `until = now-1m` → ssh fails (sshd expires it).
- [ ] **Step 2:** Run `go test ./cmd/matchblox/ -run TestPairOnARealSSHD -v`. Expected: PASS, or SKIP with "no sshd here" on a machine without sshd. CI has sshd.
- [ ] **Step 3: Commit** `test: pair a phone key and open the gate on a real sshd`
- [ ] **Step 4:** Push, open PR 1 (body: `Closes #87`, `Refs #88`), and wait for green CI.

---

## Part B: the app (Swift). PR 2: `feat: the matchblox iPhone app opens your console (#88)`

Part B needs macOS with Xcode 16 or later. This plan does not assume a Mac at hand: CI on `macos-15` builds and tests every push, so an agent on Linux can work through `gh run watch`.

### Task B0: Skeleton, licenses and CI

**Files:**
- Create: `ios/project.yml`, `ios/Makefile` (from `~/dev/blox/ios/Makefile`: `install`, `gen`, `build`, `test`), `ios/README.md`, `ios/.gitignore` (`*.xcodeproj`, `build/`), `ios/Matchblox/MatchbloxApp.swift`, `ios/Matchblox/Info.plist`, `ios/Matchblox/Matchblox.entitlements` (empty for now), `ios/MatchbloxKit/Package.swift`, `ios/MatchbloxKit/Sources/MatchbloxKit/Fixture.swift`, `ios/MatchbloxKit/Tests/MatchbloxKitTests/FixtureTests.swift`, `.github/workflows/ios.yml`, `ios/THIRD_PARTY.md`

**Interfaces:**
- Produces: the targets `Matchblox` (app), `MatchbloxKit` (a package: iOS 17.2, macOS 14, so `swift test` runs on the macOS runner), and the package dependencies SwiftTerm and swift-nio-ssh, pinned with `from:` to the latest release that is at least two weeks old. Info.plist: `CFBundleURLTypes` with the scheme `matchblox`, `NSCameraUsageDescription` = "matchblox uses the camera to scan the pairing code of your host.", `NSFaceIDUsageDescription` = "Face ID unlocks the key that opens your console.", portrait and landscape.
- `ios.yml`: on `push` and `pull_request` with paths `ios/**`, `testdata/pair/**`. Steps: `swift test --package-path ios/MatchbloxKit`, `brew install xcodegen`, `make -C ios build`.

- [ ] **Step 1:** Verify the two licenses in their repositories (SwiftTerm: MIT; swift-nio-ssh: Apache-2.0) and record them in `ios/THIRD_PARTY.md`. If one is not as stated, stop and report.
- [ ] **Step 2: Write the failing test** `FixtureTests.testFixtureLoads`: it reads `../../../testdata/pair/v1.json` from `#filePath` and asserts that `payload.name == "ws-1"`.
- [ ] **Step 3:** Push the branch `feat/88-ios-app`. Run `gh run watch`. Expected: the test job FAILS (no `Fixture` type).
- [ ] **Step 4:** Implement `Fixture.load() throws -> Fixture` (a Codable mirror of the JSON, test support only).
- [ ] **Step 5:** Push and watch. Expected: PASS for both `swift test` and `make build`.
- [ ] **Step 6: Commit** `build(ios): the matchblox app skeleton, MatchbloxKit and macOS CI`

### Task B1: Read the pairing URL

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/PairPayload.swift`, `Tests/MatchbloxKitTests/PairPayloadTests.swift`

**Interfaces:**
- Produces: `public struct PairPayload: Codable, Equatable, Sendable { v, host, port, user, name, hostkey, key }` and `public init(url: URL) throws(PairError)`. `PairError` cases: `.notMatchblox`, `.version(Int)`, `.malformed`.

- [ ] **Step 1: Write the failing tests:** `testFixtureURL` (`PairPayload(url: fixture.url) == fixture.payload`) and `testRefuses` (the same cases as the Go `TestParseRefuses`).
- [ ] **Step 2:** Push and watch. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Push and watch. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ios): read the pairing URL of matchblox pair`

### Task B2: Keys and fingerprints

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/SSHKeys.swift`, `Tests/MatchbloxKitTests/SSHKeysTests.swift`, `ios/Matchblox/DeviceKey.swift`

**Interfaces:**
- Produces (Kit):
  - `SSHKeys.openSSH(p256X963: Data, comment: String) -> String`
  - `SSHKeys.fingerprint(openSSH: String) throws -> String` (as `setup.Fingerprint`)
  - `SSHKeys.fingerprint(hostKey: NIOSSHPublicKey) -> String`
  - `SSHKeys.oneTimeKey(seedBase64: String) throws -> NIOSSHPrivateKey`
- Produces (app): `DeviceKey.loadOrCreate() throws -> SecureEnclave.P256.Signing.PrivateKey`. Access control is `.privateKeyUsage` and `.biometryAny`, with the device passcode as fallback. The `dataRepresentation` is kept in the Keychain (`kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, service `com.blox.matchblox.device`). It also gives `DeviceKey.openSSH(name:) -> String` and `DeviceKey.nioKey(_:) -> NIOSSHPrivateKey` (`NIOSSHPrivateKey(secureEnclaveP256Key:)`).

- [ ] **Step 1: Write the failing tests:** `testOpenSSHMatchesTheFixture` (from `p256_x963_hex` it gives `p256_openssh`, without its comment) and `testFingerprintMatchesSSHKeygen` (`hostkey_pub` gives `hostkey_fp`).
- [ ] **Step 2:** Push and watch. Expected: FAIL.
- [ ] **Step 3:** Implement. `DeviceKey` has no CI test (CI has no Secure Enclave). Check it on a device in B6.
- [ ] **Step 4:** Push and watch. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ios): the phone key in the Secure Enclave, and SSH key fingerprints`

### Task B3: The SSH session

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/SSHSession.swift`, `ConnectFailure.swift`, `Tests/MatchbloxKitTests/ConnectFailureTests.swift`

**Interfaces:**
- Produces:
  - `public actor SSHSession`, with `init(host: String, port: Int, user: String, key: NIOSSHPrivateKey, hostKeyFingerprint: String)` and the members:
    - `open(command: String, pty: (cols: Int, rows: Int)?) async throws(ConnectFailure) -> AsyncStream<Data>`
    - `write(_ data: Data) async`
    - `resize(cols: Int, rows: Int) async`
    - `close() async`
  - The host key delegate accepts only a key with `SSHKeys.fingerprint(hostKey:) == hostKeyFingerprint`.
  - A pty asks for `xterm-256color`.
  - The exec request carries `command` (the gate ignores it for a one-time key).
  - `public enum ConnectFailure: Error, Equatable { case unreachable, hostKeyChanged, refused, closed(String) }`, and `ConnectFailure.classify(_ error: Error) -> ConnectFailure`: DNS failures, connection refused and timeouts map to `.unreachable`; a fingerprint mismatch maps to `.hostKeyChanged`; an auth failure maps to `.refused`.

- [ ] **Step 1: Write the failing test** `testClassify`. Build the NIO and POSIX errors directly: `ECONNREFUSED`, `ETIMEDOUT` and a resolver failure each give `.unreachable`; the delegate's mismatch error gives `.hostKeyChanged`; `NIOSSHError` for auth gives `.refused`.
- [ ] **Step 2:** Push and watch. Expected: FAIL.
- [ ] **Step 3:** Implement on `NIOSSHHandler` over `ClientBootstrap` (NIOTransportServices on iOS).
- [ ] **Step 4:** Push and watch. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ios): an SSH session that trusts only the paired host key`

### Task B4: Pairing

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/Pairing.swift`, `HostStore.swift`, `Tests/MatchbloxKitTests/PairingTests.swift`, `HostStoreTests.swift`, `ios/Matchblox/PairView.swift`, `ios/Matchblox/ScanView.swift`

**Interfaces:**
- Produces (Kit):
  - `public struct PairDraft { payload, address: String, port: Int, phoneName: String }`. Its `init(payload:)` sets `address = payload.host`, and the builder can change it.
  - `Pairing.cleanName(_ s: String) -> String` drops every rune that `setup.ValidDeviceName` refuses, trims, cuts to 40, and gives "iPhone" when the result is empty. Its test cases come from `fixture.names`.
  - `Pairing.requestLine(name: String, key: String) -> Data` (equals `fixture.request_line`).
  - `Pairing.reply(_ line: Data) throws -> PairReply`.
  - `Pairing.message(for: ConnectFailure, host: String) -> String`:
    - `.refused` → "This code was used or expired. Run matchblox pair on \(host) again."
    - `.unreachable` → "Can't reach \(host). Check the address, or that the phone is on your network."
    - `.hostKeyChanged` → "\(host) has a new host key. Run matchblox pair on \(host) again."
  - `public struct Host: Codable, Equatable, Identifiable { label, address, port, user, fingerprint; id = fingerprint }`.
  - `public final class HostStore` (a JSON file in Application Support) has `all() -> [Host]`, `upsert(_ h: Host)`, `updateAddress(fingerprint: String, address: String)` and `remove(fingerprint: String)`.
- Produces (app):
  - The `matchblox://pair` URL (from the Camera) and `ScanView` (VisionKit `DataScannerViewController`, QR only) both open `PairView`.
  - `PairView` shows the host name, an address field, a phone name field (default `UIDevice.current.name`), and `Pair`.
  - The reply words are those of `testdata/pair/v1.json` `replies`: `bad name`, `bad key`, `code used`, `key in use`, `write failed`, `too slow`. The login shell of the host can print text (a `.bashrc` echo) before the reply, so the reader skips each line that is not a JSON object and takes the first one that is.
  - `Pair` runs `DeviceKey.loadOrCreate` (Face ID), opens `SSHSession` with the one-time key and no pty, writes `requestLine`, and reads one reply line. On `ok`, it calls `HostStore.upsert` and opens the window (B6). On an error, it shows `Pairing.message` in place, with `Scan again`.

- [ ] **Step 1: Write the failing tests:**
  - `testDraftAddressOverride`
  - `testCleanName`
  - `testRequestLineMatchesTheFixture`
  - `testRepliesFromTheFixture`
  - `testUsedCodeMessage` (`.refused`, host "ws-1" → the exact text)
  - `testReplySkipsShellNoise` (`"welcome\n{\"ok\":true,\"host\":\"ws-1\"}\n"` → ok)
  - `testHostStoreKeysByFingerprint` (upsert twice with a new address gives one host with the new address)
- [ ] **Step 2:** Push and watch. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Push and watch. Expected: PASS for the tests and for `make build`.
- [ ] **Step 5: Commit** `feat(ios): pair with one scan of matchblox pair`

### Task B5: Hosts and reconnect

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/Opening.swift`, `Tests/MatchbloxKitTests/OpeningTests.swift`, `ios/Matchblox/HostsView.swift`, `ios/Matchblox/ReachView.swift`

**Interfaces:**
- Produces (Kit): `Opening.first(hosts: [Host]) -> Route` with `enum Route { case scan, console(Host), hosts }`:
  - no host → `.scan`;
  - one host → `.console(host)` (the app opens straight into the console);
  - more → `.hosts`.
- Produces (app):
  - `HostsView` lists each host's label and address, has `+ Pair a host`, and swipe to remove (it removes the host from the phone and says "Remove its key on the host with matchblox devices rm").
  - `ReachView` appears on `.unreachable` with the title `Can't reach ws-1.` and the subtitle `New address?`. It has an address field filled with the last address, `Try again` (which calls `HostStore.updateAddress`, then reconnects; the pairing stays), and `Scan again`.
  - On `.hostKeyChanged`, it shows `ws-1 has a new host key.` with only `Scan again`. It never connects.

- [ ] **Step 1: Write the failing test** `testFirstRoute`, covering the three cases.
- [ ] **Step 2:** Push and watch. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Push and watch. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ios): hosts, and a new address without a new pairing`

### Task B6: The console window

**Files:**
- Create: `ios/Matchblox/ConsoleView.swift` (SwiftTerm `TerminalView` in a `UIViewRepresentable`)

**Interfaces:**
- Consumes: `SSHSession` (B3), `DeviceKey` (B2), `Host` (B4).
- Produces:
  - `ConsoleView(host: Host)`. It opens `SSHSession.open(command: "matchblox console", pty: terminal size)`, feeds the stream to the terminal and sends the terminal's input with `write`.
  - On a size change (rotation, keyboard) it calls `resize`. It uses SwiftTerm's accessory key row (Esc, Ctrl, Tab, arrows).
  - Background `#0F0D0A` (DESIGN.md `bg`), the system monospaced font, no other chrome.
  - Face ID runs once for each connect: an `LAContext` with `touchIDAuthenticationAllowableReuseDuration = 10`.
  - When the stream ends, it reconnects through B5.

- [ ] **Step 1:** Implement (UI with no unit test).
- [ ] **Step 2:** Push and watch. Expected: `make build` PASS.
- [ ] **Step 3: Manual check on a device** (the owner, or anyone with the team set in Xcode). Each item must pass:
  1. On the host, run `matchblox pair`, then scan with the Camera → `PairView` → `Pair` → Face ID → the console. The host prints `Paired.`
  2. `matchblox devices` on the host lists the phone.
  3. Rotate the phone → the console takes the landscape layout. Open the keyboard → the console fits above it.
  4. Attach a laptop to the same session at the same time → the size follows the device you used last.
  5. Quit and reopen the app → one Face ID, then the console. There is no hosts screen when there is one host.
  6. Change the host's address in the store to a wrong one → `Can't reach ws-1.` → type the right address → the console.
  7. Scan the same QR again → "This code was used or expired. Run matchblox pair on ws-1 again."
- [ ] **Step 4: Commit** `feat(ios): the console in the app, with the key row of a terminal`
- [ ] **Step 5:** Push, open PR 2 (body: `Closes #88`), and wait for green CI.

### Task B7: TestFlight (blocked on the Apple Developer team)

- [ ] **Step 1:** The owner names the Apple Developer team. Set `DEVELOPMENT_TEAM` in `ios/project.yml`.
- [ ] **Step 2:** Archive and upload to TestFlight (manual in Xcode, or `xcodebuild archive` + `xcrun altool`). Write the steps in `RELEASING.md` under "The iOS app".
- [ ] **Step 3: Commit** `build(ios): sign with the blox team and ship to TestFlight`
