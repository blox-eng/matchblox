#!/usr/bin/env bash
# Writes the site's news line from a release's changelog section: its version
# and the first headline (the first "- **...**" line), linking to the
# release. The page keeps loading nothing from another origin: the line is
# written here, at release time. A section without a headline, or a page
# without its one news line, leaves the page as it is and fails.
#
# Usage: announce-release.sh VERSION [changelog] [page]
#   VERSION without the v (0.4.0); defaults: CHANGELOG.md, www/index.html
set -euo pipefail

version="${1:?usage: announce-release.sh VERSION [changelog] [page]}"
root="$(dirname "$0")/../.."
changelog="${2:-$root/CHANGELOG.md}"
page="${3:-$root/www/index.html}"

headline="$(awk -v head="## [$version]" '
  index($0, head) == 1 { on = 1; next }
  on && /^## \[/ { exit }
  on && /^- \*\*/ {
    line = $0
    sub(/^- \*\*/, "", line)
    end = index(line, "**")
    if (end) { print substr(line, 1, end - 1) }
    exit
  }
' "$changelog")"
headline="${headline%.}"
if [ -z "$headline" ]; then
  echo "announce-release: no headline (a \"- **...**\" line) under [$version] in $changelog; the news line is left as it is" >&2
  exit 1
fi

if [ "$(grep -c 'class="news"' "$page" || true)" != 1 ]; then
  echo "announce-release: $page needs exactly one news line (class=\"news\"); left as it is" >&2
  exit 1
fi

escaped="$(printf '%s' "$headline" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&#39;/g")"
tmp="$(mktemp)"
NEWS="<a class=\"news\" href=\"https://github.com/blox-eng/matchblox/releases/tag/v$version\"><span class=\"tag\">New</span> <span>v$version · $escaped</span> <span class=\"go\">Changelog →</span></a>" \
  awk '
    index($0, "class=\"news\"") {
      match($0, /^[ \t]*/)
      print substr($0, 1, RLENGTH) ENVIRON["NEWS"]
      next
    }
    { print }
  ' "$page" >"$tmp"
cat "$tmp" >"$page"
rm -f "$tmp"
