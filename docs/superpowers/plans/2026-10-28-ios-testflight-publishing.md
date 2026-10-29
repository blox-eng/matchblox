# iOS TestFlight Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A GitHub Actions run (tag `ios-v*` or manual dispatch) builds a signed Release of the matchblox app plus its two extensions and uploads it to TestFlight, with no local Mac in the loop.

**Architecture:** Keep XcodeGen as the source of truth (`ios/project.yml`); drive the archive + upload through fastlane with Xcode automatic ("cloud") signing and an App Store Connect API key (no `match`, no stored certs). The dev→production entitlements switch rides on the Xcode build configuration (Debug = development, Release = production) so the repo default stays safe. A separate workflow leaves the existing simulator test job untouched.

**Tech Stack:** XcodeGen, Xcode 16 / `xcodebuild`, fastlane (`gym`, `upload_to_testflight`, `app_store_connect_api_key`), GitHub Actions (`macos-15`), Bundler/Ruby.

**Spec:** `docs/superpowers/specs/2026-10-28-ios-testflight-publishing-design.md`

## Global Constraints

- Bundle ids derive from one setting `MATCHBLOX_BUNDLE_ID = com.blox.matchblox`; extensions are `com.blox.matchblox.island` and `com.blox.matchblox.buzz`. App Group `group.com.blox.matchblox`. Never hardcode a second literal — override the one setting.
- Deployment target iOS 18.0; `SWIFT_VERSION 5.0`; Xcode 16.
- Only the **app** target carries `aps-environment` and `com.apple.developer.devicecheck.appattest-environment`. Both are `development` in the repo default and `production` only for the Release config. The two extensions carry no push/App Attest entitlement and are not duplicated.
- App + both extensions must ship the **same** `CFBundleVersion`, or TestFlight rejects the upload.
- fastlane `-skipPackagePluginValidation` is required (SwiftTerm build plugin), matching `make build`.
- Secrets (exact names): `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, `APP_STORE_CONNECT_KEY_P8` (base64 `.p8`). Repo variable: `APPLE_TEAM_ID`. The API key must hold the **Admin** role (automatic signing creates profiles).
- Do not auto-publish on push to `main`. Releases are a manual dispatch or an `ios-v*` tag only.
- `docs/phone.md`'s "not on TestFlight yet" wording changes only after a build is actually accepted — not in this work.

## Review Focus

- **First-ever upload (zero builds on App Store Connect).** The app record is brand new; the build-number computation must return `1`, not raise or silently start at `2`. → Task 2 tests the rescue path.
- **Tag trigger supplies no version.** `ios-v0.2.0` must yield `MARKETING_VERSION=0.2.0`; a dispatch must use its input. An unversioned tag build is a mislabelled or failed release. → Task 3 tests both trigger paths.
- **Build-number / version parity across the three targets.** If the extensions keep a hardcoded `CFBundleVersion` while the app reads `$(CURRENT_PROJECT_VERSION)`, TestFlight rejects the upload for a bundle-version mismatch. → Task 1 tests all three targets read the version build settings.
- **Entitlements environment per build config.** A Release archive must sign `production` for `aps-environment` and App Attest, while a Debug/Xcode build stays `development`; getting this backwards ships a sandbox build that the production relay answers with `BadDeviceToken`. → Task 1 tests the per-config split in the generated project.
- **Xcode 16 absent on the runner.** The "newest Xcode 16" selection yields an empty path if a future `macos-15` image drops it, failing the archive. Shared with `ios.yml`; Task 3 adds a guard that fails loudly with a clear message rather than proceeding with no toolchain.

---

### Task 1: Versioned project with a config-driven production-entitlements switch

**Files:**
- Create: `ios/Matchblox/Matchblox.production.entitlements`
- Modify: `ios/project.yml` (all three targets' `info.properties`; the `Matchblox` target's `settings`)
- Verify against: the generated `ios/Matchblox.xcodeproj/project.pbxproj` (git-ignored; a build artifact, not committed)

**Interfaces:**
- Produces: build settings `MARKETING_VERSION` and `CURRENT_PROJECT_VERSION` on every signed target, overridable from the command line; the scheme `Matchblox` (Debug run / Release archive) signs development entitlements in Debug and production in Release. Task 2's `gym` call and Task 3's workflow rely on both.

- [ ] **Step 1: Write the failing checks**

A shell check script (throwaway, run from `ios/`) that generates the project and asserts the four facts this task must produce:

```bash
xcodegen generate >/dev/null
P=Matchblox.xcodeproj/project.pbxproj
# a) Release config of the app → production entitlements; Debug → development
awk '/CODE_SIGN_ENTITLEMENTS = Matchblox\/Matchblox.production.entitlements/{p=1} /name = Release/{r=1}' "$P"
grep -q "CODE_SIGN_ENTITLEMENTS = Matchblox/Matchblox.production.entitlements" "$P"   # exists
grep -q "CODE_SIGN_ENTITLEMENTS = Matchblox/Matchblox.entitlements" "$P"             # still exists (Debug)
# b) all three signed targets carry the version build settings
test "$(grep -c 'MARKETING_VERSION = ' "$P")" -ge 2
test "$(grep -c 'CURRENT_PROJECT_VERSION = ' "$P")" -ge 2
# c) a Release-config command-line override of CURRENT_PROJECT_VERSION reaches every target's Info.plist
```

The authoritative assertion for the entitlements split (proven during review): in `project.pbxproj` the `XCBuildConfiguration` named `Release` for the app target has `CODE_SIGN_ENTITLEMENTS = Matchblox/Matchblox.production.entitlements`, and the one named `Debug` has `CODE_SIGN_ENTITLEMENTS = Matchblox/Matchblox.entitlements`.

- [ ] **Step 2: Run the checks to verify they fail**

Run: `cd ios && xcodegen generate && grep -c Matchblox.production.entitlements Matchblox.xcodeproj/project.pbxproj`
Expected: `0` (the production file and the config override do not exist yet).

- [ ] **Step 3: Create `ios/Matchblox/Matchblox.production.entitlements`**

A copy of `ios/Matchblox/Matchblox.entitlements` with exactly two values changed from `development` to `production`: `aps-environment` and `com.apple.developer.devicecheck.appattest-environment`. Every other key (`application-groups`, `keychain-access-groups`) is byte-identical. Leave a one-line comment at the top noting it is the Release/TestFlight twin of the development file.

- [ ] **Step 4: Modify `ios/project.yml`**

  - In **each** of the three targets' `info.properties`, replace the literal version strings with build-setting references: `CFBundleShortVersionString: $(MARKETING_VERSION)` and `CFBundleVersion: $(CURRENT_PROJECT_VERSION)`.
  - In **each** of the three targets' `settings.base`, add `MARKETING_VERSION: "0.1.0"` and `CURRENT_PROJECT_VERSION: "1"` (preserving today's defaults so a plain Xcode build is unchanged).
  - On the **`Matchblox`** target only, add a `configs.Release` block overriding `CODE_SIGN_ENTITLEMENTS: Matchblox/Matchblox.production.entitlements`, leaving `settings.base.CODE_SIGN_ENTITLEMENTS: Matchblox/Matchblox.entitlements` as the default (Debug).

- [ ] **Step 5: Run the checks to verify they pass**

Run: `cd ios && xcodegen generate && grep -n "CODE_SIGN_ENTITLEMENTS\|MARKETING_VERSION\|CURRENT_PROJECT_VERSION" Matchblox.xcodeproj/project.pbxproj`
Expected: the app's Release config shows `Matchblox.production.entitlements`, its Debug config shows `Matchblox.entitlements`, and `MARKETING_VERSION`/`CURRENT_PROJECT_VERSION` appear for the signed targets.

- [ ] **Step 6: Confirm the normal build still works**

Run: `make -C ios build`
Expected: PASS (unsigned simulator build, Debug config → development entitlements). Needs a Mac with Xcode 16; if the only available toolchain is newer, record that the structural checks in Step 5 passed and defer the build to the CI runner.

- [ ] **Step 7: Commit**

```bash
git add ios/project.yml ios/Matchblox/Matchblox.production.entitlements
git commit -m "feat(ios): version from build settings and a production-entitlements Release config"
```

---

### Task 2: The fastlane `beta` lane

**Files:**
- Create: `ios/Gemfile`
- Create: `ios/fastlane/Appfile`
- Create: `ios/fastlane/Gymfile`
- Create: `ios/fastlane/Fastfile`

**Interfaces:**
- Consumes: the `Matchblox` scheme and the `MARKETING_VERSION`/`CURRENT_PROJECT_VERSION` build settings from Task 1.
- Produces: a `beta` lane invocable as `bundle exec fastlane beta`, reading `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_KEY_P8_BASE64`, `MARKETING_VERSION`, and `APPLE_TEAM_ID` from the environment. Task 3's workflow calls it.

- [ ] **Step 1: Write the failing check for the build-number rescue (first-release case)**

Isolate the build-number decision into a lane helper so it is testable without App Store Connect:

```ruby
# the behavior under test: next_build_number { ... } returns 1 when the
# block raises (no app/version/build on ASC yet) or yields nil/-1/0,
# and (latest + 1) otherwise.
def next_build_number
  latest = (yield rescue nil)
  latest = -1 if latest.nil? || latest < 0
  latest + 1
end
```

A plain Ruby check (throwaway, `ruby`-run) asserting:
`next_build_number { raise "not found" } == 1`, `next_build_number { 0 } == 1`, `next_build_number { 7 } == 8`.

- [ ] **Step 2: Run it to verify it fails**

Run: `test -f ios/fastlane/Fastfile && echo present || echo absent`
Expected: `absent` — no lane or helper exists yet. (Keep the `next_build_number` helper copied into a tiny `ruby` snippet or a `*_spec` file so Step 1's assertions run in plain Ruby, independent of the fastlane DSL.)

- [ ] **Step 3: Create the Gemfile and fastlane config**

  - `ios/Gemfile`: `source "https://rubygems.org"` and `gem "fastlane"` (optionally pinned to a recent major).
  - `ios/fastlane/Appfile`: `app_identifier "com.blox.matchblox"`; `team_id ENV["APPLE_TEAM_ID"]` (never a literal team).
  - `ios/fastlane/Gymfile`: `scheme "Matchblox"`, `export_method "app-store"`, `clean true`.

- [ ] **Step 4: Implement the `beta` lane in `ios/fastlane/Fastfile`**

The lane, in order:
  1. `app_store_connect_api_key(key_id: ENV["ASC_KEY_ID"], issuer_id: ENV["ASC_ISSUER_ID"], key_content: ENV["ASC_KEY_P8_BASE64"], is_key_content_base64: true)`.
  2. `build = next_build_number { latest_testflight_build_number(initial_build_number: 0, version: ENV["MARKETING_VERSION"], api_key: api_key) }` — using the helper from Step 1 so the first release returns `1`.
  3. `build_app` (gym) with `xcargs:` containing `-allowProvisioningUpdates -skipPackagePluginValidation MARKETING_VERSION=#{ENV["MARKETING_VERSION"]} CURRENT_PROJECT_VERSION=#{build} DEVELOPMENT_TEAM=#{ENV["APPLE_TEAM_ID"]}`.
  4. `upload_to_testflight(api_key: api_key, skip_waiting_for_build_processing: true)`.

Define `next_build_number` as a private helper in the Fastfile (the code in Step 1).

- [ ] **Step 5: Verify the lane parses, lists, and the rescue logic holds**

Run: `cd ios && ruby -c fastlane/Fastfile && bundle install --quiet && bundle exec fastlane lanes`
Expected: `Syntax OK`; `beta` appears in the lane list.
Run the Step 1 Ruby assertions against the extracted helper.
Expected: all three return as asserted (1, 1, 8).

- [ ] **Step 6: Commit**

```bash
git add ios/Gemfile ios/fastlane/Appfile ios/fastlane/Gymfile ios/fastlane/Fastfile
git commit -m "feat(ios): fastlane beta lane for TestFlight with API-key cloud signing"
```

---

### Task 3: The release workflow

**Files:**
- Create: `.github/workflows/ios-release.yml`

**Interfaces:**
- Consumes: `make -C ios gen` (Task 1) and `bundle exec fastlane beta` (Task 2).
- Produces: a workflow triggered by `workflow_dispatch` (input `marketing_version`, default `0.1.0`) and by `push` tags `ios-v*`, that resolves one `MARKETING_VERSION` and runs the lane.

- [ ] **Step 1: Write the failing check for version resolution**

A shell check of the exact resolution snippet the workflow will use, for both triggers:

```bash
resolve() { # $1=event, $2=ref_name, $3=input
  if [ "$1" = push ]; then printf '%s' "${2#ios-v}"; else printf '%s' "$3"; fi
}
test "$(resolve push ios-v0.2.0 '')" = 0.2.0
test "$(resolve workflow_dispatch '' 0.3.0)" = 0.3.0
```

- [ ] **Step 2: Run it to verify it fails**

Run: `test -f .github/workflows/ios-release.yml`
Expected: FAIL (file absent).

- [ ] **Step 3: Create `.github/workflows/ios-release.yml`**

  - `on`: `workflow_dispatch` with input `marketing_version` (default `0.1.0`); `push` with `tags: ["ios-v*"]`.
  - `permissions: { contents: read }`; job `runs-on: macos-15`, `timeout-minutes: 45`.
  - Steps, in order:
    1. `actions/checkout`.
    2. **Resolve the version** into `$GITHUB_ENV` as `MARKETING_VERSION` using the Step 1 logic (tag → `${GITHUB_REF_NAME#ios-v}`, dispatch → the input).
    3. **Select Xcode 16 with a guard:** `xcode=$(ls -d /Applications/Xcode_16*.app 2>/dev/null | sort -V | tail -1); [ -n "$xcode" ] || { echo "::error::No Xcode 16 on this runner"; exit 1; }; sudo xcode-select -s "$xcode"`.
    4. `xcodebuild -downloadComponent MetalToolchain` (SwiftTerm).
    5. `brew install xcodegen`.
    6. `ruby/setup-ruby` with `bundler-cache: true` working-dir `ios` (or `cd ios && bundle install`).
    7. Write `$ASC_KEY_P8_BASE64` to a file / pass through env (the lane already base64-decodes via `is_key_content_base64`).
    8. `make -C ios gen`.
    9. `cd ios && bundle exec fastlane beta`, with `env:` mapping the three `APP_STORE_CONNECT_*` secrets to `ASC_KEY_ID`/`ASC_ISSUER_ID`/`ASC_KEY_P8_BASE64`, `APPLE_TEAM_ID` from the repo variable, and `MARKETING_VERSION` from `$GITHUB_ENV`.
    10. A final `if: always()` cleanup step removing any written key material and temporary keychain.

- [ ] **Step 4: Verify the workflow is well-formed and the resolution logic holds**

Run: the Step 1 shell assertions (both pass).
Run: `actionlint .github/workflows/ios-release.yml` if available, else `python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' .github/workflows/ios-release.yml`.
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ios-release.yml
git commit -m "ci(ios): release workflow archives and uploads to TestFlight"
```

---

### Task 4: Documentation — one-time setup and the release gesture

**Files:**
- Modify: `ios/README.md` (add a "Publish to TestFlight" section)
- Modify: `RELEASING.md` (add an iOS TestFlight subsection)

**Interfaces:**
- Consumes: the secret/variable names and the Apple-side requirements from the whole spec. No code depends on this task.

- [ ] **Step 1: Add the "Publish to TestFlight" section to `ios/README.md`**

Cover: how to cut a release (push an `ios-v<version>` tag, or run the workflow with a `marketing_version`); that it runs on GitHub Actions with no Mac needed; the automatic-vs-production entitlements rule (Debug = development for Xcode builds, Release = production for TestFlight); and a pointer to the `RELEASING.md` one-time setup.

- [ ] **Step 2: Add the iOS TestFlight subsection to `RELEASING.md`**

Write the one-time checklist, verbatim values:
  1. Register App IDs `com.blox.matchblox` (App Groups, Push Notifications, App Attest), `com.blox.matchblox.island` (App Groups), `com.blox.matchblox.buzz` (App Groups); enable the matching capabilities.
  2. Create App Group `group.com.blox.matchblox` and attach it to all three.
  3. Create the App Store Connect app record for `com.blox.matchblox`.
  4. Create the **Admin** App Store Connect API key; download the `.p8` once.
  5. Add GitHub secrets `APP_STORE_CONNECT_KEY_ID`, `APP_STORE_CONNECT_ISSUER_ID`, `APP_STORE_CONNECT_KEY_P8` (base64) and variable `APPLE_TEAM_ID`.
  6. Deploy the push relay with **no `APNS_ENV`** so it talks to Apple production — a TestFlight build is production and a `development` relay answers `BadDeviceToken`. Cross-reference the existing entitlements paragraph in `RELEASING.md`.

- [ ] **Step 3: Verify the docs name the real artifacts**

Run: `grep -n "ios-v\|APP_STORE_CONNECT_KEY_P8\|group.com.blox.matchblox\|APNS_ENV\|Admin" RELEASING.md ios/README.md`
Expected: each term present where the checklist/section describes it.

- [ ] **Step 4: Commit**

```bash
git add ios/README.md RELEASING.md
git commit -m "docs(ios): one-time TestFlight setup and the release gesture"
```

---

## Notes on verification reach

Tasks 1–4 are fully verifiable without Apple credentials via the structural checks above (project generation + `grep`/`awk`, Ruby syntax + the rescue-logic assertions, the version-resolution shell assertions, YAML/actionlint). The archive, cloud signing, and TestFlight upload can only be exercised end-to-end on a `macos-15` runner with the Admin API key in place; per the spec, the **first accepted TestFlight build is a manual gate**, mirroring how `RELEASING.md` already treats the push relay's live check. If that first run surfaces automatic-signing flakiness for the extensions, the spec's documented fallback (Proton-style manual `.p12` + readonly profiles on the same lane) is a Task-2-local change, not a re-plan.
