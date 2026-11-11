# rye

Service that runs durable VMs on Incus as remote coding environments for agents, managed over SSH and a web GUI.

## Commands
- `make gate`: every check CI runs. Run it before every push.
- `make up`: build into `dist/` with `scripts/build.sh`, then create or upgrade the service VM with `dist/ryectl up`.
  Needs `incus-admin`. Set `POOL` for a non-default pool. `make build` builds only.
- `make test-vms`: run VM commands against the deployed service VM. Run it before each PR that touches Incus code.
- `make up`, then `make test-agent-manager`: test the agent-manager install. Run them before each PR that touches
  `agent-manager/` or `rye agent-manager`.
- `ssh rye-gui` forwards the GUI that `make up` installed to `localhost:8080`.
- `make dev`: Vite dev server on port 5173, with `/api` sent to `localhost:8080`. `make e2e`: browser tests against
  the production build.
- Add shadcn components to `web/src/components/ui/` with `cd web && pnpm dlx shadcn@latest add <component>`.

## Layout
- The Go module is `rye`.
- `cmd/rye`: platform commands, run by `ForceCommand` on the service VM.
- `cmd/ryed`: the key endpoint, docs site and integration proxy for VMs, in `internal/proxy` and `internal/docs`, and
  the GUI API on localhost, in `internal/api`. With `-local`, in `local.go` and `idle.go`, it serves the admin VM's
  own agent-manager.
- `internal/integration`: the integration store. `internal/hostkeys`: known_hosts edits. `internal/atomicfile`: files
  replaced in one rename.
- `internal/agentmanager`: the GUI API's connection to each VM's agent-manager, and `rye agent-manager install`,
  through `vmexec`.
- `internal/compute`: the VM lifecycle. `internal/compute/incus`: its Incus implementation, using the REST API.
- `internal/vmexec`: commands and files in a VM through the Incus guest agent.
- `internal/vmconfig`: what the VMs share and what ryectl, rye and ryed agree on, such as the service VM's file layout.
- `agent-manager`: the TypeScript Agent SDK service in VMs. Node runs the `.ts` files directly. `rye` embeds them
  through `agent-manager/files.go`, and `internal/agentmanager` installs them.
- `cmd/ryectl` and `internal/platform`: `make up` and `make down` on the Incus host, through the Incus admin client in
  `internal/compute/incus/admin.go`. `internal/compute/incus/guest`: VM vendor-data.
- `web/src`: the GUI. `ryed` embeds its build, `web/dist`, through `web/web.go`. Build the GUI before Go code with
  `cd web && pnpm build`. `@/` is `web/src`. Unit tests next to the code as `*.test.ts(x)`.
  - `router.ts`: the route tree. `routes/`: the root layout and one component per page.
  - `session/`: one session's stream, transcript and approvals.
  - `components/`: shared components. `components/ui/`: shadcn components.
  - `hooks/`: shared hooks, with the API queries in `hooks/queries.ts`. `lib/`: the API client and pure functions.
- `web/e2e`: Playwright tests.
- `scripts/`: shell scripts. Keep them passing shellcheck. `scripts/lib.sh` holds the test scripts' shared helpers.
- `docs/`: user docs in the Diátaxis quadrants `tutorial/`, `how-to/`, `reference/` and `explanation/`. `README.md`
  contains the intro, glossary, quick start and an index of the docs.

## Git
- Work on a branch, push it to `origin`, open a pull request against `main`. Only the owner pushes to or
  merges `main`.
- Ask the owner before adding a third-party Go module, and list the modules it adds.
- Commit messages: a short summary line, then what changed and why.
