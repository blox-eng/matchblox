# Releasing

A merge to `main` that CI passed makes a release. Nobody tags by hand.

1. **Release** (`release.yml`) picks the next version from the conventional
   commits (`feat` → minor, `fix` → patch; before 1.0 a breaking change is a
   minor), and makes the pre-release `vX.Y.Z` on the commit CI verified. Its
   notes are the `## [Unreleased]` section of `CHANGELOG.md`, so write that
   section in the pull request.
2. **Publish** (`publish.yml`) builds `matchblox-{linux,darwin}-{amd64,arm64}`
   with a `.sha256`, a CycloneDX SBOM and provenance and SBOM attestations,
   then installs the release with `curl -fsSL https://matchblox.sh | sh` on
   Linux and macOS, amd64 and arm64, and checks `matchblox version`. Then it
   makes it the latest release, checks the install with no version named,
   and publishes its docs to docs.matchblox.com. Until then the install
   script keeps installing the release before it.
3. A pull request `docs: vX.Y.Z is released` moves `[Unreleased]` under the
   version and turns the README, the docs and the site to the released
   install. It merges itself once its checks pass, and the release job keeps
   it up to date with `main` for 40 minutes. Until it merges, Lint fails on
   other pull requests.

## Turn releases on or off

Releases run while the repository variable `RELEASE_ENABLED` is `true`:

```sh
gh variable set RELEASE_ENABLED --body true -R blox-eng/matchblox
```

Any other value, or no variable, holds them.

## The release token

`RELEASE_TOKEN` is a fine-grained token for this repository only, with
Contents and Pull requests set to read and write. When it is missing or
expired, the Create Release job fails and says so. To replace it:

1. GitHub → Settings → Developer settings → Fine-grained tokens → Generate
   new token. Resource owner `blox-eng`, only `blox-eng/matchblox`.
2. `gh secret set RELEASE_TOKEN -R blox-eng/matchblox`, and paste it.
3. Re-run the failed Release run.

## When something fails

| What failed | What to do |
|---|---|
| Publish, from a runner or network hiccup | Publish the tag again: `gh workflow run publish.yml --ref vX.Y.Z` |
| Publish, from a bug in the code or the workflow | Merge the fix: the next release is published and becomes the latest. The broken one stays a pre-release |
| The changelog pull request did not merge itself | Update it with `main` (`gh pr update-branch <branch>`), then merge it once its checks pass |
| No changelog pull request after a release | Run `.github/scripts/promote-changelog.sh X.Y.Z <date>` and `VERSION=vX.Y.Z .github/scripts/flip-release.sh`, and open the pull request yourself |
| The docs of a tag | Publish the tag again: the docs deploy after its verify job |

Tags are immutable: a bad release is followed by a fixed one, never moved.
