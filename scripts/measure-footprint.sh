#!/usr/bin/env bash
# Print matchblox serve (and optional console) RSS in MiB with host, kernel, date.
# Usage: measure-footprint.sh [console-pid]
# Prefers the process that owns $XDG_RUNTIME_DIR/matchblox/matchblox.sock
# (the .lock file pid, same as transport.Owner). Standby "serve" processes
# that lost the socket are ignored.
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

alive() {
  local pid=$1 state
  [[ -d "/proc/$pid" ]] || return 1
  state=$(awk '/^State:/ {print $2}' "/proc/$pid/status" 2>/dev/null || true)
  [[ "$state" != Z* ]]
}

socket_path() {
  if [[ -n "${XDG_RUNTIME_DIR:-}" ]]; then
    echo "$XDG_RUNTIME_DIR/matchblox/matchblox.sock"
  else
    echo "/tmp/matchblox-$(id -u)/matchblox.sock"
  fi
}

# serve_pid_from_sock: lock file first, else any process holding the socket open.
serve_pid_from_sock() {
  local sock lock pid
  sock=$(socket_path)
  lock="$sock.lock"
  if [[ -f "$lock" ]]; then
    pid=$(tr -d ' \n' <"$lock" || true)
    if [[ -n "$pid" ]] && alive "$pid"; then
      echo "$pid"
      return 0
    fi
  fi
  [[ -S "$sock" ]] || return 1
  # Fallback: who has the socket open (Linux).
  if command -v fuser >/dev/null 2>&1; then
    pid=$(fuser "$sock" 2>/dev/null | awk '{print $1; exit}')
    if [[ -n "${pid:-}" ]] && alive "$pid"; then
      echo "$pid"
      return 0
    fi
  fi
  return 1
}

host=$(hostname)
kernel=$(uname -srm)
day=$(date -u +%Y-%m-%d)

serve_pid=""
serve_pid=$(serve_pid_from_sock || true)

if [[ -z "$serve_pid" ]]; then
  # Last resort: a live matchblox whose cmdline names serve (may be standby).
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    alive "$pid" || continue
    cmd=$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null || true)
    if [[ "$cmd" == *" serve"* ]] || [[ "$cmd" == *"/matchblox serve"* ]]; then
      serve_pid=$pid
      break
    fi
  done < <(pgrep -x matchblox 2>/dev/null || true)
fi

if [[ -z "$serve_pid" ]]; then
  echo "measure-footprint: no live matchblox serve (no socket owner)" >&2
  echo "host=$host kernel=$kernel date=$day"
  exit 1
fi

serve_mib=$(rss_mib "$serve_pid") || {
  echo "measure-footprint: cannot read RSS for pid $serve_pid" >&2
  exit 1
}

sock=$(socket_path)
echo "host=$host kernel=$kernel date=$day"
echo "socket=$sock"
echo "serve_pid=$serve_pid"
echo "serve_rss_mib=$serve_mib"

if [[ "${1:-}" != "" ]]; then
  cons=$1
  cons_mib=$(rss_mib "$cons") || {
    echo "measure-footprint: cannot read RSS for console pid $cons" >&2
    exit 1
  }
  echo "console_pid=$cons"
  echo "console_rss_mib=$cons_mib"
fi
