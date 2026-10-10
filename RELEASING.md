# Releasing

A merge to `main` that CI passed makes a release. Nobody tags by hand.

1. **Release** (`release.yml`) picks the next version from the conventional
   commits (`feat` → minor, `fix` → patch; before 1.0 a breaking change is a
   minor), and makes the release `vX.Y.Z` on the commit CI verified. Its
   notes are the `## [Unreleased]` section of `CHANGELOG.md`, so write that
   section in the pull request.
2. **Publish** (`publish.yml`) builds `matchblox-{linux,darwin}-{amd64,arm64}`
   with a `.sha256`, a CycloneDX SBOM and provenance and SBOM attestations,
   then installs the release with `curl -fsSL https://matchblox.sh | sh` on
   Linux and macOS, amd64 and arm64, and checks `matchblox version`.
3. **Docs** (`docs.yml`) publishes that version's docs to docs.matchblox.com.
4. A pull request `docs: vX.Y.Z is released` moves `[Unreleased]` under the
   version and turns the README, the docs and the site to the released
   install. It merges itself once its checks pass. Until it does, Lint fails
   on other pull requests.

## Turn releases on or off

Releases run while the repository variable `RELEASE_ENABLED` is `true`:

```sh
gh variable set RELEASE_ENABLED --body true -R blox-eng/matchblox
```

Any other value, or no variable, holds them.

## The release token

`RELEASE_TOKEN` is a fine-grained token for this repository only, with
Contents and Pull requests set to read and write. When it expires, the
release job fails with the message to replace it:

1. GitHub → Settings → Developer settings → Fine-grained tokens → Generate
   new token. Resource owner `blox-eng`, only `blox-eng/matchblox`.
2. `gh secret set RELEASE_TOKEN -R blox-eng/matchblox`, and paste it.
3. Re-run the failed Release run.

## When something fails

| What failed | What to do |
|---|---|
| Publish, or its verify job | Fix the cause, then publish the tag again: `gh workflow run publish.yml --ref vX.Y.Z` |
| The changelog pull request did not merge itself | Merge it by hand once its checks pass |
| The docs of a tag | `gh workflow run docs.yml --ref vX.Y.Z` |

Tags are immutable: a bad release is followed by a fixed one, never moved.
