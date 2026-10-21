#!/usr/bin/env bash
# Fails if the site's news line is not the line announce-release.sh writes for
# a release: a hand edit of the line, a headline reworded after the release,
# or an announce that failed. The page is left as it is.
#
# Usage: news-guard.sh VERSION [changelog] [page]
#   VERSION without the v (0.4.0); defaults: CHANGELOG.md, www/index.html
set -euo pipefail

version="${1:?usage: news-guard.sh VERSION [changelog] [page]}"
root="$(dirname "$0")/../.."
changelog="${2:-$root/CHANGELOG.md}"
page="${3:-$root/www/index.html}"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cp "$page" "$tmp"
if ! "$(dirname "$0")/announce-release.sh" "$version" "$changelog" "$tmp" >&2; then
  echo "news-guard: cannot write the news line of v$version (above says why)" >&2
  exit 1
fi
if ! cmp -s "$page" "$tmp"; then
  echo "news-guard: the news line in $page does not name v$version as $changelog does." >&2
  echo "  has:  $(grep 'class="news"' "$page" | sed 's/^[ \t]*//')" >&2
  echo "  want: $(grep 'class="news"' "$tmp" | sed 's/^[ \t]*//')" >&2
  echo "Run .github/scripts/announce-release.sh $version, then commit www/index.html." >&2
  exit 1
fi
echo "OK: the news line names v$version"
