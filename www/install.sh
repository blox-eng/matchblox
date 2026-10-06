#!/bin/sh
# matchblox installer — https://matchblox.sh
#
# Installs the matchblox binary for this machine, then starts it.
#
# You are reading this before you pipe it into a shell, which is the right
# instinct. What it does, in order:
#
#   1. refuses anything but Linux or macOS on amd64 or arm64
#   2. resolves the release (latest, or $MATCHBLOX_VERSION)
#   3. downloads matchblox-<os>-<arch> and its .sha256
#   4. VERIFIES THE CHECKSUM, and installs nothing if it does not match
#   5. verifies the build provenance too, when `gh` is available
#   6. installs to /usr/local/bin, or ~/.local/bin when that is not writable
#   7. starts matchblox in this terminal, when there is one
#
# Before the first release (v0.1.0), there is no binary to download. Then it
# builds from source with `go install` when Go is on this machine, and tells
# you what to do when it is not.
#
# It writes one file. matchblox itself changes nothing outside
# ~/.config/matchblox and ~/.local/state/matchblox without asking you first.
#
# Environment:
#   MATCHBLOX_VERSION   tag to install (default: the latest release)
#   MATCHBLOX_BIN_DIR   directory to install into (default: as described above)
#   MATCHBLOX_NO_START  set to 1 to install without starting matchblox
#
# The whole script is one function, invoked on the last line. A truncated
# download therefore does nothing at all, rather than executing half of it.

set -eu

REPO="blox-eng/matchblox"

main() {
  say() { printf '%s\n' "$*"; }
  die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }
  need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }
  need uname
  need curl
  need mktemp

  # 1. The platform. matchblox reads /proc on Linux and the system's own
  #    APIs on macOS; Windows runs it inside WSL, which is Linux.
  case $(uname -s) in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "matchblox runs on Linux and macOS (this is $(uname -s)). On Windows, run this inside WSL." ;;
  esac
  case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m); matchblox is published for amd64 and arm64" ;;
  esac

  if [ -n "${MATCHBLOX_BIN_DIR:-}" ]; then
    bindir=$MATCHBLOX_BIN_DIR
  elif [ -w /usr/local/bin ] 2>/dev/null; then
    bindir=/usr/local/bin
  else
    bindir="$HOME/.local/bin"
  fi

  # 2. Resolve the version. No release yet: build from source.
  version=${MATCHBLOX_VERSION:-}
  if [ -z "$version" ]; then
    version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1) || true
  fi
  if [ -z "$version" ]; then
    from_source
  else
    from_release
  fi

  case ":$PATH:" in
    *":$bindir:"*) ;;
    *) say ""; say "  $bindir is not on your PATH. Add it:"; say "    export PATH=\"$bindir:\$PATH\"" ;;
  esac

  # 7. Start it. The script reads from the pipe, so the console gets the
  #    terminal itself.
  if [ "${MATCHBLOX_NO_START:-}" != 1 ] && [ -r /dev/tty ] && [ -w /dev/tty ] && (: </dev/tty) 2>/dev/null; then
    say ""
    exec "$bindir/matchblox" </dev/tty >/dev/tty
  fi
  say ""
  say "  run: matchblox"
}

from_release() {
  if command -v sha256sum >/dev/null 2>&1; then
    checksum() { sha256sum "$1" | cut -d' ' -f1; }
  elif command -v shasum >/dev/null 2>&1; then
    checksum() { shasum -a 256 "$1" | cut -d' ' -f1; }
  else
    die "no sha256sum or shasum; refusing to install a binary it cannot verify"
  fi

  asset="matchblox-$os-$arch"
  base="https://github.com/$REPO/releases/download/$version"
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT INT TERM

  say "matchblox $version ($os/$arch)"

  # 3. Download the binary and the checksum published beside it.
  curl -fsSL -o "$work/$asset" "$base/$asset" ||
    die "could not download $asset from $version — check that the release exists"
  curl -fsSL -o "$work/$asset.sha256" "$base/$asset.sha256" ||
    die "could not download the checksum for $asset; refusing to install unverified"

  # 4. Verify, and stop here if it does not match.
  want=$(cut -d' ' -f1 <"$work/$asset.sha256")
  got=$(checksum "$work/$asset")
  [ -n "$want" ] || die "the published checksum is empty; refusing to install"
  if [ "$want" != "$got" ]; then
    die "checksum mismatch for $asset
  published $want
  download  $got
Nothing was installed. Do not use the downloaded file."
  fi
  say "  checksum  ok"

  # 5. The checksum comes from the same release as the binary: it proves the
  #    bytes arrived intact, not that CI built them. The provenance
  #    attestation proves that, and only `gh` can check it.
  if command -v gh >/dev/null 2>&1; then
    gh attestation verify "$work/$asset" --repo "$REPO" >/dev/null 2>&1 ||
      die "provenance verification FAILED for $asset.
The checksum matched, but the build attestation did not verify against $REPO.
Nothing was installed. Please report this: https://github.com/$REPO/security"
    say "  provenance ok (built by CI in $REPO)"
  else
    say "  provenance not checked (install the 'gh' CLI to verify the build attestation)"
  fi

  # 6. Install.
  mkdir -p "$bindir" || die "could not create $bindir"
  install -m 0755 "$work/$asset" "$bindir/matchblox" 2>/dev/null ||
    { cp "$work/$asset" "$bindir/matchblox" && chmod 0755 "$bindir/matchblox"; } ||
    die "could not write to $bindir — set MATCHBLOX_BIN_DIR, or re-run with sudo"
  say "  installed $bindir/matchblox"
}

from_source() {
  command -v go >/dev/null 2>&1 || die "matchblox has no release yet (v0.1.0 is close), so it builds from source, and that needs Go 1.26 or later:
  https://go.dev/dl/
Then run this again, or: go install github.com/$REPO/cmd/matchblox@latest"
  say "matchblox (no release yet: building from source with $(go version | cut -d' ' -f3))"
  mkdir -p "$bindir" || die "could not create $bindir"
  GOBIN=$bindir go install "github.com/$REPO/cmd/matchblox@latest" ||
    die "go install failed; the output above says why"
  say "  installed $bindir/matchblox"
}

main "$@"
