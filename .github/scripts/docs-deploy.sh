#!/usr/bin/env bash
# Says whether this docs build deploys: "true" or "false".
# A tag deploys its docs. Main deploys only before the first release and
# while releases are off: GitHub Pages keeps the first deployment of a
# commit, so a main deploy of the commit a release tags next would drop
# the tag's docs.
#
# Usage: docs-deploy.sh <event> <ref> <RELEASE_ENABLED> <a v* tag, or "">
set -euo pipefail
event="$1" ref="$2" releases="$3" tag="$4"
deploy=false
if [ "$event" != pull_request ]; then
  case "$ref" in
    refs/tags/v*) deploy=true ;;
    refs/heads/main) [ -z "$tag" ] && [ "$releases" != true ] && deploy=true ;;
  esac
fi
echo "$deploy"
