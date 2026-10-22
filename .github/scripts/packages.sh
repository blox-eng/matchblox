#!/usr/bin/env bash
# Writes the Homebrew formula and the AUR package of a release from the
# .sha256 files that publish.yml attached to it:
#
#   <out>/Formula/matchblox.rb       for the tap blox-eng/homebrew-tap
#   <out>/aur/PKGBUILD, .SRCINFO     for the AUR package matchblox-bin
#
# Both install the release binaries that publish.yml built and attested;
# nothing builds again. A pre-release tag, or a missing or malformed
# checksum, writes nothing, and the script fails.
#
# Usage: packages.sh vX.Y.Z <dir with matchblox-*.sha256> <out>
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "usage: packages.sh vX.Y.Z <dir with matchblox-*.sha256> <out>" >&2
  exit 2
fi
tag=$1 sums=$2 out=$3
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "packages: '$tag' is not a release tag (like v0.1.0); a pre-release gets no package" >&2
  exit 1
fi
version=${tag#v}
repo="blox-eng/matchblox"
base="https://github.com/$repo/releases/download/$tag"
desc="One terminal console for every AI coding agent you run"

sum() {
  local f="$sums/matchblox-$1.sha256" s
  [ -f "$f" ] || { echo "packages: no $f" >&2; exit 1; }
  s=$(cut -d' ' -f1 <"$f")
  [[ "$s" =~ ^[0-9a-f]{64}$ ]] || { echo "packages: $f holds no sha256" >&2; exit 1; }
  printf '%s' "$s"
}
darwin_arm64=$(sum darwin-arm64)
darwin_amd64=$(sum darwin-amd64)
linux_arm64=$(sum linux-arm64)
linux_amd64=$(sum linux-amd64)

mkdir -p "$out/Formula" "$out/aur"

cat >"$out/Formula/matchblox.rb" <<EOF
# Written by .github/scripts/packages.sh in $repo for $tag. Do not edit:
# the next release writes it again.
class Matchblox < Formula
  desc "$desc"
  homepage "https://matchblox.com"
  license "MIT"

  depends_on "tmux"

  on_macos do
    on_arm do
      url "$base/matchblox-darwin-arm64"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$base/matchblox-darwin-amd64"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$base/matchblox-linux-arm64"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$base/matchblox-linux-amd64"
      sha256 "$linux_amd64"
    end
  end

  def install
    bin.install Dir["matchblox-*"].first => "matchblox"
  end

  test do
    assert_equal "v#{version}", shell_output("#{bin}/matchblox version").strip
  end
end
EOF

cat >"$out/aur/PKGBUILD" <<EOF
# Written by .github/scripts/packages.sh in $repo for $tag. Do not edit:
# the next release writes it again.
pkgname=matchblox-bin
pkgver=$version
pkgrel=1
pkgdesc="$desc"
arch=('x86_64' 'aarch64')
url="https://github.com/$repo"
license=('MIT')
depends=('tmux')
provides=('matchblox')
conflicts=('matchblox')
options=('!strip')
source_x86_64=("matchblox-\${pkgver}-x86_64::$base/matchblox-linux-amd64")
source_aarch64=("matchblox-\${pkgver}-aarch64::$base/matchblox-linux-arm64")
sha256sums_x86_64=('$linux_amd64')
sha256sums_aarch64=('$linux_arm64')

package() {
  install -Dm755 "matchblox-\${pkgver}-\${CARCH}" "\${pkgdir}/usr/bin/matchblox"
}
EOF

# What makepkg --printsrcinfo prints for the PKGBUILD above: the AUR reads
# this file, not the PKGBUILD.
cat >"$out/aur/.SRCINFO" <<EOF
pkgbase = matchblox-bin
	pkgdesc = $desc
	pkgver = $version
	pkgrel = 1
	url = https://github.com/$repo
	arch = x86_64
	arch = aarch64
	license = MIT
	depends = tmux
	provides = matchblox
	conflicts = matchblox
	options = !strip
	source_x86_64 = matchblox-$version-x86_64::$base/matchblox-linux-amd64
	sha256sums_x86_64 = $linux_amd64
	source_aarch64 = matchblox-$version-aarch64::$base/matchblox-linux-arm64
	sha256sums_aarch64 = $linux_arm64

pkgname = matchblox-bin
EOF
