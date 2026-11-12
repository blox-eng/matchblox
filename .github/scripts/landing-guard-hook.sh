#!/usr/bin/env bash
# A Claude Code PreToolUse hook: it blocks `gh pr merge` and `gh pr ready` on a
# feat pull request that fails landing-guard.sh. It reads the hook JSON on stdin.
# Exit 2 blocks the command and shows stderr to the agent; exit 0 lets it run.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
command="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("command",""))')"

pr="$(sed -nE 's/.*gh pr (merge|ready)( +-[^ ]+)* +([0-9]+).*/\3/p' <<<"$command" | head -n 1)"
if [ -z "$pr" ]; then
  exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if ! title="$(gh pr view "$pr" --json title,body --jq .title 2>"$tmp/err")" ||
  ! gh pr view "$pr" --json title,body --jq .body >"$tmp/body" 2>>"$tmp/err" ||
  ! gh pr diff "$pr" --name-only >"$tmp/files" 2>>"$tmp/err"; then
  echo "the landing check could not read PR $pr: $(head -n 1 "$tmp/err")" >&2
  exit 2
fi

if ! "$here/landing-guard.sh" "$title" "$tmp/body" - <"$tmp/files" >/dev/null 2>"$tmp/why"; then
  cat "$tmp/why" >&2
  exit 2
fi
