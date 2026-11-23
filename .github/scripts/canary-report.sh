#!/usr/bin/env bash
# Reads the canary's result files (one <agent>.json each) and keeps one open
# issue labelled canary per agent: it opens one for a break, comments on it
# when the version or the kind of break changes, and closes it with a comment
# when the screen reads again. The same break on the next day writes
# nothing. An agent with no result file, or a refused one, could not run,
# and a folder with no results at all opens one issue: the canary tested
# nothing. That issue closes when results arrive again.
#
# The files come from a job that ran agent code nobody reviewed, so every
# field is data: checked against an allow-list, never run. Anyone may write
# on an issue, so an agent's issue is found by its exact title, and markers
# count only as the last line of what the Actions bot wrote.
#
# Usage: canary-report.sh RESULTS-DIR
#   Exits 1 when an agent's screen does not read or has no result, a file or
#   a gh call is refused, or the folder holds no results.
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
    gh label create canary --color FBCA04 --description "an agent release matchblox cannot read" --force >/dev/null || return 1
    labelled=1
  fi
}

# lookup NAME KEY: the open issue of agent NAME ("run" for the
# tested-nothing issue) in $number, and its last KEY marker in $last. A break's title is
# "<Name> <version> draws a screen matchblox cannot read", a could-not-run's
# "the canary could not run <Name>: <reason>".
lookup() {
  local issues search
  if [ "$1" = run ]; then search="$tested_title in:title"; else search="$1 in:title"; fi
  issues="$(gh issue list --label canary --state open --search "$search" --json number,title,author,body,comments)" || return 1
  number="$(jq -r --arg name "$1" --arg tested "$tested_title" '
    def ours: if $name == "run" then . == $tested
      else (startswith($name + " ") and endswith(" draws a screen matchblox cannot read"))
        or startswith("the canary could not run " + $name + ": ") end;
    [.[] | select(.title | ours)] | first | .number // empty' <<<"$issues")" || return 1
  last=
  if [ -n "$number" ]; then
    last="$(jq -r --argjson n "$number" '
      def bot: (.author.login // "") as $l | $l == "github-actions" or $l == "app/github-actions";
      .[] | select(.number == $n) | ., .comments[]? | select(bot) | .body // "" |
        [splits("\r?\n") | select(. != "")] | last // empty' <<<"$issues" |
      grep -xE "<!-- canary $2 [^ ]+ (fail|pass|norun) -->" | tail -n 1)" || true
  fi
}

# post NUMBER TITLE BODY-FILE WHAT: a comment on issue NUMBER, or a new issue
# titled TITLE when NUMBER is empty.
post() {
  if [ -n "$1" ]; then
    gh issue comment "$1" --body-file "$3" >/dev/null || return 1
    echo "$4: commented on #$1"
  else
    label || return 1
    gh issue create --label canary --title "$2" --body-file "$3" || return 1
  fi
}

# close NUMBER TEXT WHAT LAST: closes issue NUMBER with the comment TEXT. When
# LAST, the issue's last marker, already says pass, an earlier close failed
# after its comment: close it with no second comment.
close() {
  if [[ "$4" == *" pass -->" ]]; then
    gh issue close "$1" >/dev/null || return 1
  else
    gh issue close "$1" --comment "$2" >/dev/null || return 1
  fi
  echo "$3: closed #$1"
}

# capped: the last 20000 characters of $1, in $screen, with $cut set when it
# was longer.
capped() {
  screen="$1"
  cut=
  if [ "${#screen}" -gt 20000 ]; then
    screen="${screen: -20000}"
    cut=1
  fi
}

tested_title="the canary tested nothing"

shopt -s nullglob
files=("$dir"/*.json)
if ! lookup run run; then
  refuse "gh failed for $tested_title"
  number='' last=''
fi
tested="$number" tested_last="$last"
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
  post "$tested" "$tested_title" "$body" "$tested_title" || refuse "gh failed for $tested_title"
  rm -f "$body"
  exit 1
fi
if [ -n "$tested" ]; then
  close "$tested" "$(printf 'The canary runs again.\n\n<!-- canary run none pass -->')" "the canary runs again" "$tested_last" ||
    refuse "gh failed for $tested_title"
fi

for f in "${files[@]}"; do
  file="$(basename "$f")"
  case "$file" in
    claude.json | codex.json | opencode.json) ;;
    *) refuse "$(shown "$file") is not claude.json, codex.json or opencode.json" ;;
  esac
done

# read_result FILE NAME: the result's fields, or 1 with the reason in $why.
read_result() {
  why=
  if ! jq -e 'type == "object" and (.agent | type) == "string" and (.version | type) == "string"
      and (.ok | type) == "boolean" and (.check | type) == "string" and (.screen | type) == "string"' \
      "$1" >/dev/null 2>&1; then
    why="$(basename "$1") is not a canary result"
    return 1
  fi
  agent="$(jq -r '.agent' "$1")"
  version="$(jq -r '.version' "$1")"
  ok="$(jq -r '.ok' "$1")"
  check="$(jq -r '.check' "$1")"
  screen="$(jq -r '.screen' "$1")"
  if [ "$agent" != "$2" ]; then
    why="$(basename "$1") names agent \"$(shown "$agent")\", not $2"
    return 1
  fi
  case "$ok/$check" in
    true/) kind=pass ;;
    "false/no waiting state" | "false/no menu" | "false/wrong options") kind=fail ;;
    "false/could not run") kind=norun ;;
    *)
      why="$2 check \"$(shown "$check")\" is not a check"
      return 1
      ;;
  esac
  if [ "$kind" = norun ] && [ -z "$version" ]; then
    mark=none
  elif [[ "$version" =~ ^[0-9]+(\.[0-9]+){1,3}$ ]]; then
    mark="$version"
  else
    why="$2 version \"$(shown "$version")\" is not a version"
    return 1
  fi
}

for key in claude codex opencode; do
  case "$key" in
    claude) name="Claude Code" ;;
    codex) name="Codex" ;;
    opencode) name="OpenCode" ;;
  esac
  f="$dir/$key.json"
  if [ ! -e "$f" ]; then
    version='' kind=norun mark=none screen="the canary wrote no result for $name"
  elif ! read_result "$f" "$name"; then
    refuse "$why"
    version='' kind=norun mark=none screen="its result was refused: $why"
  fi
  [ "$kind" = pass ] || status=1
  marker="<!-- canary $key $mark $kind -->"

  if ! lookup "$name" "$key"; then
    refuse "gh failed for $name"
    continue
  fi

  if [ "$kind" = pass ]; then
    if [ -z "$number" ]; then
      echo "$name $version: reads well"
    else
      close "$number" "$(printf 'matchblox reads %s %s again.\n\n%s' "$name" "$version" "$marker")" "$name $version" "$last" ||
        refuse "gh failed for $name"
    fi
    continue
  fi
  if [ -n "$number" ] && [ "$last" = "$marker" ]; then
    echo "$name $mark: still #$number, nothing new"
    continue
  fi
  capped "$screen"
  tick="$(fence "$screen")"
  body="$(mktemp)"
  if [ "$kind" = norun ]; then
    title="the canary could not run $name: $(titled "$screen")"
    {
      printf 'The canary could not run %s %s, so it tested nothing for it. The run is not green.\n\n' "$name" "${version:-(no version)}"
      printf 'Why:\n\n%s\n%s\n%s\n\n' "$tick" "$screen" "$tick"
      # shellcheck disable=SC2016 # Markdown code, not shell
      printf 'Fix the install or the stand-in model in `internal/said/canary_agents_test.go`.\n\n'
      if [ -n "$cut" ]; then printf 'The text above is cut to its last 20000 characters.\n\n'; fi
      printf '%s\n' "$marker"
    } >"$body"
  else
    title="$name $version draws a screen matchblox cannot read"
    {
      printf '%s.\n\n' "$title"
      printf -- '- Agent: %s\n- Version: %s\n- Check that failed: %s\n\n' "$name" "$version" "$check"
      printf 'The screen:\n\n%s\n%s\n%s\n\n' "$tick" "$screen" "$tick"
      if [ -n "$cut" ]; then printf 'The screen is cut to its last 20000 characters.\n\n'; fi
      # shellcheck disable=SC2016 # Markdown code, not shell
      printf 'Fix: add the screen as `testdata/menus/%s-bash-%s.txt`, see a test in `internal/said` fail on it, then change the rule.\n\n' "$key" "$version"
      printf '%s\n' "$marker"
    } >"$body"
  fi
  post "$number" "$title" "$body" "$name $mark" || refuse "gh failed for $name"
  rm -f "$body"
done
exit "$status"
