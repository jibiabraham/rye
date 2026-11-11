.PHONY: help gate dev build ryectl up down e2e test-vms test-agent-manager test-all tailscale tailscale-policy admin \
	admin-remove

help:             ## list targets
	@grep -E '^[a-z0-9-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*## /\t/'

build:            ## build rye, the GUI and the docs archive into dist/
	@scripts/build.sh

# ryectl builds only dist/ryectl, for the targets that deploy nothing.
ryectl:
	@CGO_ENABLED=0 go build -o dist/ ./cmd/ryectl

up: build         ## build, then create or upgrade the service VM from dist/ (POOL= to choose the storage pool)
	@dist/ryectl up

down: ryectl      ## remove the service VM and its certificate; VMs stay
	@dist/ryectl down

test-vms:         ## test the VM commands against the deployed service VM; creates and removes a real VM
	@scripts/test-vms.sh

tailscale:        ## join the service VM to your tailnet and serve the GUI there
	@scripts/tailscale.sh

tailscale-policy: ## print the tailnet policy for LOGIN, to merge in the Tailscale admin console
	@scripts/tailscale.sh policy

admin: build      ## build, then create, upgrade or start the admin VM
	@dist/ryectl up admin

admin-remove: ryectl ## remove the admin VM, its key and its firewall
	@dist/ryectl down admin

test-agent-manager: ## test agent-manager install against the deployed service VM; creates and removes a real VM
	@scripts/test-agent-manager.sh

test-all:         ## test-vms and test-agent-manager at once, on two VMs
	@$(MAKE) --no-print-directory -j2 -Oline test-vms test-agent-manager

gate:             ## every check CI runs, for the shell scripts, the GUI, the Go code and agent-manager
	@scripts/gate.sh

dev:              ## hot reload on 127.0.0.1:5173
	@cd web && pnpm install --frozen-lockfile --silent && pnpm dev

e2e:              ## browser tests against the production build
	@cd web && pnpm install --frozen-lockfile --silent && pnpm --silent build && pnpm exec playwright install --only-shell chromium && pnpm --silent e2e
