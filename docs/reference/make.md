# make targets

`make help` lists the targets.

## Platform

| Target | Does |
|---|---|
| `build` | Builds `rye`, `ryed`, `ryectl`, the GUI and the docs archive into `dist/`. |
| `up` | Runs `build`, then `dist/ryectl up`, which creates or upgrades the service VM from `dist/`. Needs `incus-admin`. |
| `down` | Removes the service VM, its Incus certificate and its entry in `~/.ssh/known_hosts.d/rye`, and keeps the VMs and their disks. Builds only `dist/ryectl`, then runs `dist/ryectl down`. Needs `incus-admin`. |
| `tailscale` | Joins the service VM to your tailnet and serves the GUI there. |
| `tailscale-policy` | Prints the tailnet policy for `LOGIN`, to merge in the Tailscale admin console. |
| `admin` | Runs `build`, then `dist/ryectl up admin`, which creates, upgrades or starts the [admin VM](../how-to/work-from-admin.md). Needs `incus-admin`. |
| `admin-remove` | Builds only `dist/ryectl`, then runs `dist/ryectl down admin`, which removes the admin VM, its key on the service VM, its firewall and its host key. Needs `incus-admin`. |

## Development

| Target | Does |
|---|---|
| `gate` | Runs every check CI runs, for the shell scripts, the Go code, agent-manager and the GUI. |
| `dev` | Serves the GUI with hot reload on `127.0.0.1:5173` and proxies `/api` requests to `localhost:8080`. |
| `e2e` | Runs the Playwright tests against the production build. |
| `test-vms` | Tests the VM commands against the deployed service VM, on a VM named `rye-test-vms` that it creates and removes. |
| `test-agent-manager` | Tests `rye agent-manager install` against the deployed service VM, on a VM named `rye-test-agent-manager` that it creates and removes. |
| `test-all` | Runs `test-vms` and `test-agent-manager` at once, on two VMs. |

## Variables

| Variable | Targets | Default | Meaning |
|---|---|---|---|
| `POOL` | `up` | the current deploy's, else `default` | The storage pool for the service VM and the VMs. |
| `BRIDGE_IP` | `up` | the current deploy's, else `198.18.0.1` | The host's address on `ryebr0`. A later run with another value fails. |
| `SERVICE_IP` | `up` | the current deploy's, else `198.18.0.10` | The service VM's address, in the bridge's /24. |
| `KEY` | `up`, `admin` | `~/.ssh/id_ed25519.pub` | The public key to authorize for `rye`. |
| `LOGIN` | `tailscale`, `tailscale-policy` | the previous run's value; required for `tailscale-policy` | Your tailnet login, such as `you@github`. |
| `SIZE` | `scripts/incus-setup.sh` | `30GiB` | The size of the ZFS pool the script creates. |
