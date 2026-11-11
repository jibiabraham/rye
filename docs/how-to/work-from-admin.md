# Let Claude manage your VMs

The admin VM, `rye-admin`, runs Claude sessions that can run `ssh rye` commands and log in to every VM, as you do
from the Incus host. Claude in other VMs can't. For example, ask it to create a VM or to set one up from a
tutorial.

The admin VM has a GUI of its own, where you approve those commands. Keep its sessions in Manual mode. Only the
Incus host can reach the admin VM, over SSH. The admin VM reaches the internet and the service VM.

## Create or start it
On the Incus host, after `make up`:

    make admin

It builds into `dist/`, then creates the VM at `198.18.0.11` by default and installs the
build's agent-manager, GUI and `ryed`. It prints the two commands below, which use the `rye-admin` alias from
[the SSH config](../reference/ssh-config.md). Log in to Claude on the VM once, as in
[step 6 of First run](../tutorial/first-run.md#6-create-a-vm-and-log-in-to-claude):

    ssh -t rye-admin claude

Then forward its GUI and open `http://localhost:8081`:

    ssh -N -L 8081:127.0.0.1:8080 rye-admin

The VM's `ryed` powers it off after 30 minutes with no running turn, pending approval, background task or session
change. An open SSH connection doesn't count. `make admin` starts it again and upgrades its agent-manager, GUI and
`ryed`.

`make admin-remove` removes the VM, its key on the service VM, its firewall and its host key in
`~/.ssh/known_hosts.d/rye`.

## What a session on it can do
- `ssh rye <command>` runs one [rye command](../reference/rye.md) on the service VM, such as `ssh rye ls`.
- `ssh <vm>.rye` logs in to a VM as `dev`, who has sudo, and `ssh <vm>.rye <command>` runs one command there.
- To reach a service that listens on a VM's `127.0.0.1`, run `ssh <vm>.rye curl …`.
- The [docs site](../reference/docs-site.md) serves rye's files, and a command that reads stdin takes one through a
  pipe, as in

      curl -s http://docs.int.rye/deployments/forge-vm/cloud-init.yaml | ssh rye new forge --cloud-init

The admin VM has no `incus` command or Incus credentials. `ssh rye` commands act on VMs in project `rye`.
