#!/usr/bin/env bash
# Fails if the newest released tag has no section in CHANGELOG.md.
#
# Releases are cut automatically from Conventional Commits, but the changelog is
# written by hand, so nothing ties the two together: four releases once shipped
# while their entries still sat under [Unreleased], and the file told readers
# that released work was unreleased. This gate catches that on the first release
# it happens to, rather than the fourth.
#
# It is deliberately fail-closed. A missing tag means the checkout has no tags
# (`fetch-depth: 1` without `fetch-tags`), not that there is nothing to check —
# passing there would be the same silent no-op the gate exists to prevent.
#
# Run it the same way locally: .github/scripts/changelog-guard.sh
set -euo pipefail

cd "$(dirname "$0")/../.."
CHANGELOG="CHANGELOG.md"

tag="$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n 1)"
if [ -z "$tag" ]; then
  # No local tag: either nothing is released yet, or the checkout has no tags.
  # Ask the remote, so a shallow checkout still fails closed.
  if git ls-remote --exit-code --tags origin 'v[0-9]*' >/dev/null 2>&1; then
    echo "FAIL: the remote has version tags but the checkout has none (fetch-tags: true)" >&2
    exit 1
  fi
  echo "OK: nothing released yet"
  exit 0
fi

version="${tag#v}"

if ! grep -qF "## [$version] - " "$CHANGELOG"; then
  echo "FAIL: $tag is released but $CHANGELOG has no '## [$version]' section." >&2
  echo "Move its entries out of [Unreleased] and date them." >&2
  exit 1
fi

if ! grep -qF "[$version]: https://" "$CHANGELOG"; then
  echo "FAIL: $CHANGELOG has a section for $version but no '[$version]:' link definition." >&2
  exit 1
fi

echo "OK: $tag is documented in $CHANGELOG"
