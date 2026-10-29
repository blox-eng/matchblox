# The iOS app ships to TestFlight from CI — design

## Intent

Today the matchblox iPhone/iPad app only builds two ways: unsigned on a
simulator in CI (`.github/workflows/ios.yml`), and by hand on a Mac with
Xcode (`ios/README.md`). `docs/phone.md` says plainly that the app "is not
in the App Store or on TestFlight yet." To try the app you need a Mac.

After this work, a maintainer pushes a tag or clicks one button in GitHub
Actions and a signed build lands in TestFlight — no Mac in the loop. The
pipeline lives in the repository, is reproducible, and matches how
comparable open-source apps ship (DuckDuckGo, Mastodon iOS, Wikipedia,
Proton Mail iOS all drive archive + upload through fastlane with an App
Store Connect API key).

## Context the design must respect

- The project has **no committed `.xcodeproj`**. XcodeGen generates it from
  `ios/project.yml` (`make gen`). The workflow must run `xcodegen generate`
  before it archives; `.gitignore` keeps the project and the generated
  `Info.plist` files out of git.
- Three signed targets, all deriving their bundle id from one setting
  `MATCHBLOX_BUNDLE_ID` (`com.blox.matchblox`):
  - `Matchblox` (app) — `com.blox.matchblox`
  - `MatchbloxIsland` (Live Activity widget) — `com.blox.matchblox.island`
  - `MatchbloxBuzz` (notification service) — `com.blox.matchblox.buzz`
- Capabilities in play: App Groups (`group.com.blox.matchblox`), two
  keychain groups, Push Notifications, and App Attest. Only the **app**
  target carries `aps-environment` and the App Attest environment; the two
  extensions carry only App Groups (and the buzz a shared keychain group).
- `RELEASING.md` §"The app's entitlements" already states the rule this
  design enforces: a build from Xcode wants `development` for both
  `aps-environment` and the App Attest environment; a **TestFlight or App
  Store build needs `production` for both**, served by a relay with no
  `APNS_ENV` set. A mismatch makes the relay answer Apple's
  `BadDeviceToken`.
- SwiftTerm, a package dependency, needs a build-plugin trust
  (`-skipPackagePluginValidation`, already used by `make build`) and may
  need the Metal Toolchain on a device archive
  (`xcodebuild -downloadComponent MetalToolchain`).

## Decisions (owner, 2026-10-28)

Confirmed with the owner, and informed by a survey of current open-source
iOS pipelines:

1. **Automation**: a GitHub Actions job builds and uploads. No per-release
   Mac step.
2. **Apple account state**: the paid Developer Program is active, but the
   App Store Connect app record and the three App IDs are **not created
   yet**. The design includes the one-time manual portal steps as a
   checklist; it cannot automate portal work.
3. **Signing**: **Xcode automatic ("cloud") signing**, driven by an App
   Store Connect API key, with no `match` repository. `match`'s value is
   sharing certs across a team; a solo maintainer does not need it, and
   Xcode 13+ cloud-managed certificates removed its other advantage.
4. **Entitlements switch**: **config-driven through `project.yml`** —
   Debug keeps `development`, Release gets `production`, automatically.
5. **Orchestration is fastlane**, not raw `xcodebuild`. Every serious
   public native pipeline surveyed uses fastlane, and bare headless
   `xcodebuild -allowProvisioningUpdates` is documented (Apple's own
   forums) to be flaky on Xcode 16+ for exactly our shape — multiple
   extensions + App Groups + Push + App Attest. fastlane runs the same
   automatic signing far more reliably and is easier to debug. This is the
   Wikipedia pattern (fastlane + automatic signing + API key, no match).
6. **Build number** comes from `latest_testflight_build_number + 1`
   queried from App Store Connect, not the CI run number (fragile across
   workflow renames). Marketing version comes from the `workflow_dispatch`
   input (default `0.1.0`), or the tag for a tag-triggered run.

## Goals

1. A `workflow_dispatch` run, or a push of an `ios-v*` tag, produces a
   signed Release archive of all three targets and uploads it to
   TestFlight, with no local Mac step.
2. A build from Xcode onto a device keeps signing `development`
   entitlements; a CI archive signs `production` entitlements. The repo
   default stays safe (`development`), so nobody ships a sandbox build by
   forgetting a flag.
3. All three targets always carry the same, monotonically increasing build
   number, so TestFlight never rejects the upload for a duplicate or a
   mismatch.
4. The existing simulator test-and-build job (`ios.yml`) keeps running
   unchanged on every `ios/**` change; the release job is separate and
   explicit.
5. The pipeline lives in the repository and a maintainer can read exactly
   what it does. Secrets are the only thing that lives outside git.
6. The one-time Apple-side setup and the per-release steps are written
   down in `ios/README.md` and `RELEASING.md`, including the `APNS_ENV`
   coordination the relay needs.

## Non-goals

- App Store *release* (public submission, `deliver`/screenshots/metadata).
  This design stops at TestFlight. Public submission is a later spec.
- `match`, a certs repository, or manual `.p12` handling — unless the
  automatic-signing fallback below is triggered.
- Automatic upload on every push to `main`. Releases are deliberate: a
  manual trigger or a tag only.
- Changing the app's features, entitlements set, or capabilities. The only
  entitlement change is `development` → `production` values for Release.

## Components

### 1. Versioning in `project.yml`

All three targets hardcode `CFBundleShortVersionString "0.1.0"` and
`CFBundleVersion "1"`. Change each target's `info.properties` to read the
version from build settings, and add the settings with the current values
as defaults:

- `CFBundleShortVersionString = $(MARKETING_VERSION)`
- `CFBundleVersion = $(CURRENT_PROJECT_VERSION)`
- in each target's `settings.base`: `MARKETING_VERSION: "0.1.0"`,
  `CURRENT_PROJECT_VERSION: "1"`

A local Xcode build then behaves exactly as today. The fastlane lane
overrides both on the archive command so every target gets the same pair.

### 2. The production entitlements switch

Add one file, `ios/Matchblox/Matchblox.production.entitlements`, identical
to `Matchblox/Matchblox.entitlements` except:

- `aps-environment` → `production`
- `com.apple.developer.devicecheck.appattest-environment` → `production`

In `project.yml`, make `CODE_SIGN_ENTITLEMENTS` on the `Matchblox` target
config-specific:

```yaml
settings:
  base:
    CODE_SIGN_ENTITLEMENTS: Matchblox/Matchblox.entitlements
  configs:
    Release: { CODE_SIGN_ENTITLEMENTS: Matchblox/Matchblox.production.entitlements }
```

The scheme runs Debug and archives Release, so an Xcode device build (Debug)
signs `development` and a CI archive (Release) signs `production`
automatically. The two extensions are untouched — they carry no push or App
Attest entitlement. The two entitlements files differ only in those two
values; both are committed and the difference is documented inline.

### 3. fastlane

New directory `ios/fastlane/`:

- `Appfile` — `app_identifier "com.blox.matchblox"`, the team id read from
  the `FASTLANE_TEAM_ID` / `DEVELOPMENT_TEAM` environment, never a literal.
- `Gymfile` — scheme `Matchblox`, `export_method "app-store"`,
  `clean true`.
- `Fastfile` — one `beta` lane:
  1. `app_store_connect_api_key(key_id:, issuer_id:, key_content:,
     is_key_content_base64: true, in_house: false)` from env.
  2. The build number, computed to survive the **first ever upload**.
     The app record is brand new (decision 2), so it has zero builds and
     may not yet have the `MARKETING_VERSION` on App Store Connect. The
     lane must therefore call `latest_testflight_build_number` with
     `initial_build_number: 0` **and** rescue the "no build / version not
     found" error to `0`, then add `1`. The first build is number `1`; no
     run ever aborts for a missing prior build. (Writing it as a bare
     `latest_testflight_build_number + 1` is the trap: on a fresh app it
     either silently starts at `2` or raises, exactly on the one release
     that matters most.)
  3. `gym` with `xcargs:` containing
     `-allowProvisioningUpdates -skipPackagePluginValidation`,
     `MARKETING_VERSION=...`, `CURRENT_PROJECT_VERSION=<build>`,
     `DEVELOPMENT_TEAM=...`, `MATCHBLOX_BUNDLE_ID=com.blox.matchblox`.
     Automatic signing plus the API key provisions all three targets on the
     fly — the single biggest friction-saver for the extensions.
  4. `upload_to_testflight(api_key:, skip_waiting_for_build_processing:
     true)`.

fastlane generates the export options itself from the lane; no committed
`ExportOptions.plist` is needed.

### 4. The GitHub Actions workflow

New file `.github/workflows/ios-release.yml`, separate from `ios.yml`:

- Triggers: `workflow_dispatch` (input `marketing_version`, default
  `0.1.0`) and `push` on tags matching `ios-v*`.
- **Resolving the marketing version — both triggers must supply one.** A
  first step computes a single `MARKETING_VERSION`: on a tag run, strip the
  prefix (`${GITHUB_REF_NAME#ios-v}`, so `ios-v0.2.0` → `0.2.0`); on a
  dispatch run, take the `marketing_version` input. That one resolved value
  is passed to fastlane. Without this the tag path has no version and ships
  a build mislabelled `0.1.0` (or fails export on an empty version) — the
  `ios-v*` tag is the documented release gesture, so it cannot be the
  unversioned path.
- `runs-on: macos-15`, `timeout-minutes: 45`, `permissions: contents:
  read`.
- Steps: checkout → select newest Xcode 16 (same step as `ios.yml`; note
  this is a shared dependency with `ios.yml` — if a future `macos-15` image
  drops Xcode 16 the selection yields an empty path and the archive fails,
  so revisit the pin once the app is confirmed to build on the runner's
  Xcode) → 
  `xcodebuild -downloadComponent MetalToolchain` (SwiftTerm) →
  `brew install xcodegen` → Ruby + `bundle install` (a small
  `ios/Gemfile` pins fastlane) → write the API key `.p8` from the secret →
  `make -C ios gen` → `bundle exec fastlane beta` with the version/team
  env set.
- The API key file and any keychain are removed in a final always-run
  cleanup step.

### 5. Secrets and variables (GitHub repo settings)

- Secret `APP_STORE_CONNECT_KEY_ID`
- Secret `APP_STORE_CONNECT_ISSUER_ID`
- Secret `APP_STORE_CONNECT_KEY_P8` — the `.p8` contents, base64
- Variable `APPLE_TEAM_ID` — not secret

The API key needs the **Admin** role: automatic signing *creates*
provisioning profiles, which the App Manager role cannot do.

### 6. One-time Apple-side setup (manual checklist, documented)

Written into `RELEASING.md` as an iOS subsection:

1. Register three App IDs: `com.blox.matchblox` (with App Groups, Push
   Notifications, App Attest), `com.blox.matchblox.island` (App Groups),
   `com.blox.matchblox.buzz` (App Groups). Enable the matching capabilities
   on each.
2. Create App Group `group.com.blox.matchblox` and attach it to all three.
3. Create the App Store Connect app record for `com.blox.matchblox`.
4. Create the Admin App Store Connect API key; download the `.p8` once.
5. Add the three secrets and the one variable to the GitHub repository.
6. Deploy the push relay with **no `APNS_ENV`** so it talks to Apple's
   production service — a TestFlight build is production and a
   `development` relay answers `BadDeviceToken` (RELEASING.md already
   describes this; the iOS section cross-references it).

### 7. Documentation

- `ios/README.md`: a "Publish to TestFlight" section — the one-time setup
  (point to RELEASING.md), how to cut a release (tag or button), and the
  automatic-vs-production entitlements rule.
- `RELEASING.md`: the checklist in §6 above, plus where the secrets live.
- `docs/phone.md`: update the "not on TestFlight yet" wording **only once a
  build is actually accepted** — not as part of the code change.

## Risks and fallbacks

- **Automatic signing flakiness.** If CI automatic signing misbehaves for
  the extensions (the documented Xcode 16+ risk), the fallback is the
  Proton Mail pattern on the *same* fastlane lane: import a distribution
  `.p12` from a secret into a temporary keychain and fetch the three
  App Store profiles `readonly`. This is a one-lane change, no
  re-architecture; the spec names it so the plan can carry a ready
  contingency rather than discovering it mid-release.
- **Metal Toolchain / plugin trust.** The archive must pass
  `-skipPackagePluginValidation` (as `make build` does) and the workflow
  runs `-downloadComponent MetalToolchain` before archiving, because
  SwiftTerm needs it on a device build and the existing simulator job does
  not exercise this.
- **`APNS_ENV` mismatch.** Covered by the §6 checklist; called out because
  it is invisible at build time and only shows up as broken push on a real
  TestFlight device.

## Verification

- `make -C ios gen` succeeds with the new `project.yml`, and `make -C ios
  build` (the unsigned simulator build) still passes — proving the
  versioning and per-config entitlements changes did not break the normal
  build. This is runnable without any Apple credentials.
- The config-driven entitlements split is proven: in a generated project,
  the `Matchblox` target's Release build config resolves
  `CODE_SIGN_ENTITLEMENTS` to `Matchblox.production.entitlements` and its
  Debug config to `Matchblox.entitlements`. (Confirmed during design review
  by running `xcodegen generate` and reading the resulting `project.pbxproj`.)
- The build-number logic returns `1`, not an error, against an app record
  with zero builds — the first-release case. The plan verifies this before
  the first real upload (a fastlane dry-run or a unit check of the lane's
  rescue path), because it cannot be discovered safely on the live run.
- A tag `ios-v0.2.0` produces an archive whose `CFBundleShortVersionString`
  is `0.2.0`, and a dispatch with input `0.3.0` produces `0.3.0` — proving
  both triggers carry a version.
- `bundle exec fastlane beta` is exercised for real by the first
  `workflow_dispatch` run once the Apple-side setup and secrets are in
  place; a build accepted by TestFlight is the acceptance test. CI cannot
  prove this without the credentials, so the plan treats the first real
  upload as a manual gate, as `RELEASING.md` already does for the push
  relay's live check.
- The device checklist in `ios/README.md` (Secure Enclave, camera, Face
  ID, push, island) stays the human gate before a release is handed to
  testers; this design does not replace it.
