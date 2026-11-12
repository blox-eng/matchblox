#!/usr/bin/env bash
# Fails a feat pull request that leaves the landing page alone and does not say why.
#
# A feat changes what a builder can do, and www/ is where a builder first reads
# that (owner, 2026-11-07). The rule: a title that starts with feat must change a
# file under www/, or the body must hold a line
#   Landing page: unchanged because <a reason of 10 characters or more>
# Any other title passes.
#
# Usage: landing-guard.sh <title> <body-file> <base-ref>
#        landing-guard.sh <title> <body-file> -   (reads the changed files on stdin)
# It fails closed: an unreadable base ref is a failure, not a pass.
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: landing-guard.sh <title> <body-file> <base-ref|->" >&2
  exit 1
fi
title="$1" body_file="$2" base="$3"

if ! grep -Eq '^feat(\([^)]+\))?!?: ' <<<"$title"; then
  echo "OK: not a feat"
  exit 0
fi

if [ "$base" = "-" ]; then
  files="$(cat)"
else
  if ! git rev-parse --verify --quiet "$base^{commit}" >/dev/null; then
    echo "FAIL: cannot read $base" >&2
    exit 1
  fi
  files="$(git diff --name-only "$base...HEAD")"
fi

if grep -q '^www/' <<<"$files"; then
  echo "OK: the landing page changes"
  exit 0
fi

# GitHub sends bodies with CRLF line ends.
reason="$(tr -d '\r' <"$body_file" | sed -n 's/^Landing page: unchanged because //p' | head -n 1)"
if [ "${#reason}" -ge 10 ]; then
  echo "OK: the landing page is unchanged because $reason"
  exit 0
fi

echo "FAIL: a feat pull request must change the landing page:" >&2
echo "change www/ or add \"Landing page: unchanged because …\" (10 characters or more) to the body." >&2
exit 1
