#!/bin/bash
# Runs every check that CI runs, for the shell scripts, the GUI, the Go code and agent-manager. make gate runs it.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0
step() { echo "==> $*"; }

step "shell: shellcheck $(shellcheck --version | awk '/^version:/{print $2}')"
shellcheck -x scripts/*.sh internal/compute/incus/guest/*.sh agent-manager/install.sh || fail=1

# ryed embeds web/dist. The web steps build it before the Go steps.
cd web
step "web: node $(node --version), pnpm $(pnpm --version)"
step "web: install from the lockfile"
pnpm install --frozen-lockfile --silent || fail=1
step "web: vulnerabilities"
pnpm audit --audit-level high || fail=1
step "web: format"
pnpm --silent format-check || fail=1
step "web: lint"
pnpm --silent lint || fail=1
step "web: tests"
pnpm --silent test || fail=1
step "web: typecheck and build"
pnpm --silent build || fail=1
step "web: browser tests (Playwright, headless Chromium)"
pnpm exec playwright install --only-shell chromium >/dev/null || fail=1
pnpm --silent e2e || fail=1

cd ..
step "go: $(go env GOVERSION)"
step "go: format"
golangci-lint fmt --diff || fail=1
step "go: lint"
golangci-lint run || fail=1
step "go: vulnerabilities"
govulncheck ./... || fail=1
step "go: tests"
go test -race -count=1 ./... || fail=1

cd agent-manager
step "agent-manager: install from the lockfile"
pnpm install --frozen-lockfile --silent || fail=1
step "agent-manager: vulnerabilities"
pnpm audit --audit-level high || fail=1
step "agent-manager: format"
pnpm --silent format-check || fail=1
step "agent-manager: lint"
pnpm --silent lint || fail=1
step "agent-manager: typecheck"
pnpm --silent typecheck || fail=1
step "agent-manager: tests"
pnpm --silent test || fail=1

if [ $fail = 0 ]; then step "gate passed"; else step "gate FAILED"; fi
exit $fail
