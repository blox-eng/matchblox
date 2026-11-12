#!/usr/bin/env bash
# Tests landing-guard.sh and landing-guard-hook.sh in a throwaway git repo.
# Run: bash .github/scripts/landing-guard_test.sh
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
guard="$here/landing-guard.sh"
hook="$here/landing-guard-hook.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

git -C "$tmp" init -q -b main
git -C "$tmp" config user.email t@example.invalid
git -C "$tmp" config user.name t
git -C "$tmp" config commit.gpgsign false
mkdir -p "$tmp/internal" "$tmp/www"
echo a >"$tmp/internal/a.go"
echo a >"$tmp/www/index.html"
git -C "$tmp" add -A && git -C "$tmp" commit -qm base
git -C "$tmp" switch -q -c code
echo b >>"$tmp/internal/a.go"
git -C "$tmp" commit -qam code
git -C "$tmp" switch -q -c page
echo b >>"$tmp/www/index.html"
git -C "$tmp" commit -qam page

fails=0
# check <name> <want-exit> <branch> <title> <body> [stderr-substring]
check() {
  local name="$1" want="$2" branch="$3" title="$4" body="$5" err="${6:-}"
  printf '%s' "$body" >"$tmp/body"
  git -C "$tmp" switch -q "$branch"
  local got=0
  (cd "$tmp" && "$guard" "$title" "$tmp/body" main) >"$tmp/out" 2>"$tmp/err" || got=$?
  if [ "$got" != "$want" ]; then
    echo "FAIL $name: exit $got, want $want"; cat "$tmp/err"; fails=$((fails + 1)); return
  fi
  if [ -n "$err" ] && ! grep -qF "$err" "$tmp/err"; then
    echo "FAIL $name: stderr lacks: $err"; cat "$tmp/err"; fails=$((fails + 1)); return
  fi
  echo "ok $name"
}

reason='Landing page: unchanged because the room has no public face yet'
check "feat without www fails" 1 code "feat: x" "" 'change www/ or add "Landing page: unchanged because …"'
check "feat with www passes" 0 page "feat: x" ""
check "scoped feat with the escape line passes" 0 code "feat(app): x" "$reason"
check "a short reason fails" 1 code "feat: x" "Landing page: unchanged because no"
check "fix without www passes" 0 code "fix: x" ""
check "breaking feat without www fails" 1 code "feat!: x" ""
check "a CRLF body passes" 0 code "feat: x" $'Summary\r\n'"$reason"$'\r\n'

# A missing base ref fails closed (the helper always passes main).
got=0
(cd "$tmp" && "$guard" "feat: x" "$tmp/body" nosuchref) >/dev/null 2>"$tmp/err" || got=$?
if [ "$got" = 1 ] && grep -qF "FAIL: cannot read nosuchref" "$tmp/err"; then echo "ok a missing base ref fails closed"; else echo "FAIL missing base ref: exit $got"; fails=$((fails + 1)); fi

# The file list on stdin replaces the base ref (eng review correction 7).
: >"$tmp/body"
got=0
printf 'internal/a.go\n' | (cd "$tmp" && "$guard" "feat: x" "$tmp/body" -) >/dev/null 2>&1 || got=$?
if [ "$got" = 1 ]; then echo "ok stdin list without www fails"; else echo "FAIL stdin list without www: exit $got"; fails=$((fails + 1)); fi
got=0
printf 'internal/a.go\nwww/index.html\n' | (cd "$tmp" && "$guard" "feat: x" "$tmp/body" -) >/dev/null 2>&1 || got=$?
if [ "$got" = 0 ]; then echo "ok stdin list with www passes"; else echo "FAIL stdin list with www: exit $got"; fails=$((fails + 1)); fi

# The hook: a stub gh answers for PR 7.
mkdir -p "$tmp/bin"
cat >"$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "pr view 7 --json title,body --jq .title") echo "feat: the room" ;;
  "pr view 7 --json title,body --jq .body") echo "no escape here" ;;
  "pr diff 7 --name-only") echo "internal/app/room.go" ;;
  "pr view 8 "*|"pr diff 8 "*) echo "HTTP 502" >&2; exit 1 ;;
  *) echo "unexpected gh $*" >&2; exit 1 ;;
esac
STUB
chmod +x "$tmp/bin/gh"
hookrun() { # hookrun <command> -> exit code
  local rc=0
  printf '{"tool_input":{"command":%s}}' "$(printf '%s' "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')" |
    (cd "$tmp" && PATH="$tmp/bin:$PATH" "$hook") >/dev/null 2>"$tmp/err" || rc=$?
  echo "$rc"
}
expect() { # expect <name> <condition...>
  local name="$1"; shift
  if "$@"; then echo "ok $name"; else echo "FAIL $name"; fails=$((fails + 1)); fi
}
expect "hook blocks a merge without www" test "$(hookrun 'gh pr merge 7 --squash')" = 2
expect "hook blocks ready without www" test "$(hookrun 'gh pr ready 7')" = 2
expect "hook fails closed when gh errors" test "$(hookrun 'gh pr merge 8')" = 2
expect "hook names the PR it could not read" grep -qF "the landing check could not read PR 8" "$tmp/err"
expect "hook passes other commands" test "$(hookrun 'go test ./...')" = 0

if [ "$fails" -gt 0 ]; then echo "$fails failed"; exit 1; fi
