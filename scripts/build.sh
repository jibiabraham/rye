#!/bin/bash
# Builds rye, ryed with the GUI, ryectl, a git archive of HEAD for docs.int.rye, and VERSION into dist/, or into the
# directory in $1. rye holds agent-manager's files. ryectl up deploys what it builds. Needs Go, Node, pnpm and git.
set -euo pipefail
cd "$(dirname "$0")/.."
exec </dev/null

DIST=$(realpath -m "${1:-dist}")

log() { echo "==> $*"; }

rm -rf "$DIST"
mkdir -p "$DIST"

# ryed embeds the GUI from web/dist. The Go build needs it first.
log "GUI"
(cd web && pnpm install --frozen-lockfile --silent && pnpm --silent build)

log "rye, ryed and ryectl"
CGO_ENABLED=0 go build -o "$DIST/" ./cmd/rye ./cmd/ryed ./cmd/ryectl

log "docs and code"
version=$(git rev-parse --short HEAD)
if [ -n "$(git status --porcelain)" ]; then
  echo "build: the docs archive holds HEAD, without uncommitted changes" >&2
  version=$version-dirty
fi
git archive --format=tar -o "$DIST/source.tar" HEAD
echo "$version" >"$DIST/VERSION"
log "built $version in $DIST"
