#!/usr/bin/env bash
# Reads the canary's result files (one <agent>.json each) and keeps one open
# issue labelled canary per agent: it opens one for a break, comments on it
# when the version or the kind of break changes, and comments once when the
# screen reads again. The same break on the next day writes nothing. An agent
# with no result file could not run, and a folder with no results at all
# opens one issue: the canary tested nothing. That issue hears once when
# results arrive again, and once when a later outage starts.
#
# The files come from a job that ran agent code nobody reviewed, so every
# field is data: checked against an allow-list, never run.
#
# Usage: canary-report.sh RESULTS-DIR
#   Exits 1 when an agent's screen does not read or has no result, a file is
#   refused, or the folder holds no results.
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: canary-report.sh RESULTS-DIR" >&2
  exit 2
fi
dir="$1"

status=0
labelled=

# shown: a field as one printable line, safe in a log.
shown() {
  local s="${1//[^[:print:]]/?}"
  printf '%s' "${s:0:200}"
}

refuse() {
  echo "refused: $*" >&2
  status=1
}

# fence: a run of backticks longer than any in $1, three at least.
fence() {
  local n
  n="$(printf '%s\n' "$1" | awk '{ while (match($0, /`+/)) { if (RLENGTH > m) m = RLENGTH; $0 = substr($0, RSTART + RLENGTH) } } END { print m + 0 }')"
  if [ "$n" -lt 3 ]; then n=2; fi
  printf '%*s' "$((n + 1))" '' | tr ' ' '`'
}

# titled: the first line of $1, plain characters only, at most 80 of them.
titled() {
  local s
  s="$(printf '%s\n' "$1" | head -n 1 | LC_ALL=C tr -cd "A-Za-z0-9 .,:'()_/-")"
  s="${s:0:80}"
  s="${s%"${s##*[! ]}"}"
  printf '%s' "${s:-no reason given}"
}

label() {
  if [ -z "$labelled" ]; then
    gh label create canary --color FBCA04 --description "an agent release matchblox cannot read" --force >/dev/null
    labelled=1
  fi
}

# find_tested: the open "the canary tested nothing" issue in $tested, and the
# last marker on it in $tested_last.
tested_title="the canary tested nothing"
find_tested() {
  local issues
  issues="$(gh issue list --label canary --state open --search "$tested_title in:title" --json number,title,body,comments)"
  tested="$(jq -r --arg title "$tested_title" '[.[] | select(.title | contains($title))] | first | .number // empty' <<<"$issues")"
  tested_last=
  if [ -n "$tested" ]; then
    tested_last="$(jq -r --argjson n "$tested" '.[] | select(.number == $n) | .body, (.comments[]?.body // empty)' <<<"$issues" |
      grep -oE "<!-- canary run [^ ]+ (pass|norun) -->" | tail -n 1 || true)"
  fi
}

shopt -s nullglob
files=("$dir"/*.json)
find_tested
if [ ${#files[@]} -eq 0 ]; then
  echo "no canary results: the canary tested nothing" >&2
  marker="<!-- canary run none norun -->"
  if [ -n "$tested" ] && [ "$tested_last" = "$marker" ]; then
    echo "$tested_title: still #$tested, nothing new"
    exit 1
  fi
  body="$(mktemp)"
  {
    printf 'The canary wrote no result for any agent, so it tested nothing. The run is not green.\n\n'
    printf "The canary job's log says why: an install, the run, or the artifact upload failed.\n\n"
    printf '%s\n' "$marker"
  } >"$body"
  if [ -n "$tested" ]; then
    gh issue comment "$tested" --body-file "$body" >/dev/null
    echo "$tested_title: commented on #$tested"
  else
    label
    gh issue create --label canary --title "$tested_title" --body-file "$body"
  fi
  rm -f "$body"
  exit 1
fi
if [ -n "$tested" ] && [ "$tested_last" != "<!-- canary run none pass -->" ]; then
  body="$(mktemp)"
  printf 'The canary runs again.\n\n<!-- canary run none pass -->\n' >"$body"
  gh issue comment "$tested" --body-file "$body" >/dev/null
  echo "the canary runs again: commented on #$tested"
  rm -f "$body"
fi

for f in "${files[@]}"; do
  file="$(basename "$f")"
  case "$file" in
    claude.json | codex.json | opencode.json) ;;
    *) refuse "$(shown "$file") is not claude.json, codex.json or opencode.json" ;;
  esac
done

missing="$(mktemp)"
trap 'rm -f "$missing"' EXIT
for key in claude codex opencode; do
  case "$key" in
    claude) name="Claude Code" ;;
    codex) name="Codex" ;;
    opencode) name="OpenCode" ;;
  esac
  file="$key.json"
  f="$dir/$file"
  if [ ! -e "$f" ]; then
    f="$missing"
    jq -n --arg agent "$name" \
      '{agent: $agent, version: "", ok: false, check: "could not run", screen: "the canary wrote no result for \($agent)"}' >"$f"
  fi
  if ! jq -e 'type == "object" and (.agent | type) == "string" and (.version | type) == "string"
      and (.ok | type) == "boolean" and (.check | type) == "string" and (.screen | type) == "string"' \
      "$f" >/dev/null 2>&1; then
    refuse "$file is not a canary result"
    continue
  fi
  agent="$(jq -r '.agent' "$f")"
  version="$(jq -r '.version' "$f")"
  ok="$(jq -r '.ok' "$f")"
  check="$(jq -r '.check' "$f")"
  screen="$(jq -r '.screen' "$f")"

  if [ "$agent" != "$name" ]; then
    refuse "$file names agent \"$(shown "$agent")\", not $name"
    continue
  fi
  case "$ok/$check" in
    true/) kind=pass ;;
    "false/no waiting state" | "false/no menu" | "false/wrong options") kind=fail ;;
    "false/could not run") kind=norun ;;
    *)
      refuse "$name check \"$(shown "$check")\" is not a check"
      continue
      ;;
  esac
  if [ "$kind" = norun ] && [ -z "$version" ]; then
    mark=none
  elif [[ "$version" =~ ^[0-9]+(\.[0-9]+){1,3}$ ]]; then
    mark="$version"
  else
    refuse "$name version \"$(shown "$version")\" is not a version"
    continue
  fi
  [ "$kind" = pass ] || status=1
  marker="<!-- canary $key $mark $kind -->"

  issues="$(gh issue list --label canary --state open --search "$name in:title" --json number,title,body,comments)"
  number="$(jq -r --arg name "$name" '[.[] | select(.title | contains($name))] | first | .number // empty' <<<"$issues")"
  last=
  if [ -n "$number" ]; then
    last="$(jq -r --argjson n "$number" '.[] | select(.number == $n) | .body, (.comments[]?.body // empty)' <<<"$issues" |
      grep -oE "<!-- canary $key [^ ]+ (fail|pass|norun) -->" | tail -n 1 || true)"
  fi

  body="$(mktemp)"
  if [ "$kind" = pass ]; then
    if [ -z "$number" ] || [[ "$last" == *" pass -->" ]]; then
      echo "$name $version: reads well"
      rm -f "$body"
      continue
    fi
    printf 'matchblox reads %s %s again.\n\n%s\n' "$name" "$version" "$marker" >"$body"
  else
    if [ -n "$number" ] && [ "$last" = "$marker" ]; then
      echo "$name $mark: still #$number, nothing new"
      rm -f "$body"
      continue
    fi
    fixture="testdata/menus/$key-bash-$version.txt"
    tick="$(fence "$screen")"
    if [ "$kind" = norun ]; then
      title="the canary could not run $name: $(titled "$screen")"
      {
        printf 'The canary could not run %s %s, so it tested nothing for it. The run is not green.\n\n' "$name" "${version:-(no version)}"
        printf 'Why:\n\n%s\n%s\n%s\n\n' "$tick" "$screen" "$tick"
        # shellcheck disable=SC2016 # Markdown code, not shell
        printf 'Fix the install or the stand-in model in `internal/said/canary_agents_test.go`.\n\n'
        printf '%s\n' "$marker"
      } >"$body"
    else
      title="$name $version draws a screen matchblox cannot read"
      {
        printf '%s.\n\n' "$title"
        printf -- '- Agent: %s\n- Version: %s\n- Check that failed: %s\n\n' "$name" "$version" "$check"
        printf 'The screen:\n\n%s\n%s\n%s\n\n' "$tick" "$screen" "$tick"
        # shellcheck disable=SC2016 # Markdown code, not shell
        printf 'Fix: add the screen as `%s`, see a test in `internal/said` fail on it, then change the rule.\n\n' "$fixture"
        printf '%s\n' "$marker"
      } >"$body"
    fi
  fi

  if [ -n "$number" ]; then
    gh issue comment "$number" --body-file "$body" >/dev/null
    echo "$name $mark: commented on #$number"
  else
    label
    gh issue create --label canary --title "$title" --body-file "$body"
  fi
  rm -f "$body"
done
exit "$status"
