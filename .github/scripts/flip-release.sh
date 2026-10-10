#!/usr/bin/env bash
# Turns the docs, the README and the site to the released text: drops each
# block between "<!-- until-release -->" and "<!-- /until-release -->", and
# shows each block between "<!-- after-release" and "after-release -->".
# Each marker is a line of its own. "@VERSION@" in a shown block becomes
# $VERSION, the release tag. A file with an open or nested marker, or a
# @VERSION@ without $VERSION, is left as it is, and the script fails.
#
# Usage: VERSION=v0.1.0 flip-release.sh [file...]
#   (default files: the README, docs and site)
set -euo pipefail

version="${VERSION:-}"
if [ -n "$version" ] && [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "flip-release: VERSION '$version' is not a release tag (like v0.1.0)" >&2
  exit 1
fi

cd "$(dirname "$0")/../.."
if [ $# -eq 0 ]; then
  set -- README.md docs/*.md www/index.html
fi

status=0
for f in "$@"; do
  tmp="$(mktemp)"
  if awk -v version="$version" '
    { line = $0; gsub(/^[ \t]+|[ \t]+$/, "", line) }
    line == "<!-- until-release -->" { if (blk) exit 1; blk = "until"; next }
    line == "<!-- /until-release -->" { if (blk != "until") exit 1; blk = ""; next }
    line == "<!-- after-release" { if (blk) exit 1; blk = "after"; next }
    line == "after-release -->" { if (blk != "after") exit 1; blk = ""; next }
    blk == "until" { next }
    blk == "after" && index($0, "@VERSION@") { if (version == "") exit 2; gsub(/@VERSION@/, version) }
    { print }
    END { if (blk) exit 1 }
  ' "$f" >"$tmp"; then
    cat "$tmp" >"$f"
  else
    echo "flip-release: $f has an open or nested release marker, or @VERSION@ without VERSION; left as it is" >&2
    status=1
  fi
  rm -f "$tmp"
done
exit "$status"
