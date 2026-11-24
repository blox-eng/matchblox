#!/usr/bin/env bash
# Print matchblox serve (and optional console) RSS in MiB with host, kernel, date.
# Usage: measure-footprint.sh [console-pid]
# Finds serve via pgrep -x matchblox whose cmdline contains "serve", else the
# first non-defunct matchblox. Pass a console PID to include its RSS.
set -euo pipefail

rss_mib() {
  local pid=$1
  local kb
  kb=$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status" 2>/dev/null) || return 1
  if [[ -z "$kb" ]]; then
    return 1
  fi
  echo $((kb / 1024))
}

host=$(hostname)
kernel=$(uname -srm)
day=$(date -u +%Y-%m-%d)

serve_pid=""
while read -r pid; do
  [[ -z "$pid" ]] && continue
  state=$(awk '/^State:/ {print $2}' "/proc/$pid/status" 2>/dev/null || true)
  [[ "$state" == Z* ]] && continue
  cmd=$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null || true)
  if [[ "$cmd" == *" serve"* ]] || [[ "$cmd" == *"serve "* ]]; then
    serve_pid=$pid
    break
  fi
  if [[ -z "$serve_pid" ]]; then
    serve_pid=$pid
  fi
done < <(pgrep -x matchblox 2>/dev/null || true)

if [[ -z "$serve_pid" ]]; then
  echo "measure-footprint: no live matchblox process" >&2
  echo "host=$host kernel=$kernel date=$day"
  exit 1
fi

serve_mib=$(rss_mib "$serve_pid") || {
  echo "measure-footprint: cannot read RSS for pid $serve_pid" >&2
  exit 1
}

echo "host=$host kernel=$kernel date=$day"
echo "serve_pid=$serve_pid serve_rss_mib=$serve_mib"

if [[ "${1:-}" != "" ]]; then
  cons=$1
  cons_mib=$(rss_mib "$cons") || {
    echo "measure-footprint: cannot read RSS for console pid $cons" >&2
    exit 1
  }
  echo "console_pid=$cons console_rss_mib=$cons_mib"
fi
