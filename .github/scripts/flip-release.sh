#!/usr/bin/env bash
# Turns the docs, the README and the site to the released text: drops each
# block between "<!-- until-release -->" and "<!-- /until-release -->", and
# shows each block between "<!-- after-release" and "after-release -->".
# Each marker is a line of its own. A file with an open or nested marker is
# left as it is, and the script fails.
#
# Usage: flip-release.sh [file...]   (default: the README, docs and site)
set -euo pipefail

cd "$(dirname "$0")/../.."
if [ $# -eq 0 ]; then
  set -- README.md docs/*.md www/index.html
fi

status=0
for f in "$@"; do
  tmp="$(mktemp)"
  if awk '
    { line = $0; gsub(/^[ \t]+|[ \t]+$/, "", line) }
    line == "<!-- until-release -->" { if (blk) exit 1; blk = "until"; next }
    line == "<!-- /until-release -->" { if (blk != "until") exit 1; blk = ""; next }
    line == "<!-- after-release" { if (blk) exit 1; blk = "after"; next }
    line == "after-release -->" { if (blk != "after") exit 1; blk = ""; next }
    blk == "until" { next }
    { print }
    END { if (blk) exit 1 }
  ' "$f" >"$tmp"; then
    cat "$tmp" >"$f"
  else
    echo "flip-release: $f has an open or nested release marker; left as it is" >&2
    status=1
  fi
  rm -f "$tmp"
done
exit "$status"
