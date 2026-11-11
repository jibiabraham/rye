# Develop rye

The Go code is in `cmd/` and `internal/`, agent-manager in `agent-manager/`, and the GUI in `web/` (React, Tailwind and
shadcn components). [make targets](../reference/make.md) lists every target.

`ryed` embeds the GUI's build from `web/dist`. Build the GUI once before you build or test the Go code:

    cd web && pnpm install && pnpm build

## Work on the GUI

    make dev

It serves the GUI with hot reload on port 5173 and proxies `/api` requests to `localhost:8080`. Keep `ssh rye-gui`
running for the API.

Add a shadcn component to `web/src/components/ui/` with:

    cd web && pnpm dlx shadcn@latest add <component>

The components use the variants and utilities in `web/src/shadcn.css`, a copy of the stylesheet in the shadcn package.
If a new component uses a utility that file lacks, copy the stylesheet again from the release the CLI used.

## Test
- `make gate` runs every check CI runs, for the shell scripts, the Go code, agent-manager and the GUI. Run it before
  every push.
- The GUI's unit tests are in `*.test.ts(x)` files next to the code and run with vitest.
- `make e2e` runs the Playwright tests in `web/e2e/` against the production build, and the gate runs them too. They
  need Chromium's system libraries. `cd web && pnpm exec playwright install-deps chromium` installs them on Debian
  and Ubuntu.
- `make test-vms` runs the VM commands against the deployed service VM, on a VM named `rye-test-vms` that it creates
  and removes. Run it before a PR that touches Incus code.
- `make test-agent-manager` runs `rye agent-manager install` against the deployed service VM, on a VM named
  `rye-test-agent-manager` that it creates and removes. Run it after `make up`, before a PR that touches
  `agent-manager/` or `rye agent-manager`.
- `make test-all` runs both at once, on two VMs.

## Handle a pnpm audit failure
`pnpm audit` in the gate fails on high and critical advisories, in `agent-manager/` and in `web/`. For an advisory with
no fixed release, add its GHSA ID to `auditConfig.ignoreGhsas` in that directory's `pnpm-workspace.yaml`. Above the ID,
add a comment with the reason and the expected fixed release. Remove the entry when that release is published.
