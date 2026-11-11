# rye

rye is a thin, opinionated service that helps orchestrate durable virtual machines as remote coding environments for
agents (like Claude). VMs are managed through `ssh rye` commands, modelled after [exe.dev](https://exe.dev/). It
currently works on Linux machines, using [Incus](https://linuxcontainers.org/incus/) as the virtual machine manager.

A rye VM is a place to run a coding agent with little supervision. The VM has only the data you put in it and can't
reach your local network, which leaves an agent little to leak or break.

The [integrations](docs/how-to/integrations.md) feature keeps third-party API credentials out of the VM. The agent
sends a request to `http://<name>.int.rye/` with no token, and rye adds the token before forwarding it.

Install agent-manager with `ssh rye agent-manager install <vm>` to spin up agents on demand in a VM. rye also serves a
browser GUI that shows the sessions of all your VMs in one place. Start work in a VM's terminal or in the GUI, and
switch between them at any point.

![The quick start in 30 seconds: make up, ssh rye new dev1, agent-manager install, then a first message in the GUI on desktop and at phone width](docs/media/first-run.webp)

## Quick start
You need an x86_64 Debian or Ubuntu machine and a Claude account. `scripts/incus-setup.sh` installs Incus. The build
needs Go 1.27.4, Node 24 or later, pnpm 11.8.0, make, git, curl and gpg. Run these commands on that machine, from the
root of a clone of this repository:

    scripts/incus-setup.sh                                 # skip on a machine that already runs Incus
    sudo usermod -aG incus-admin "$USER"                   # then log out and in again
    ssh-keygen -t ed25519                                  # skip when ~/.ssh/id_ed25519 exists
    make up                                                # builds rye and starts it
    dist/ryectl ssh-config                                 # once; add the Include line it prints to ~/.ssh/config
    ssh rye new dev1                                       # your first VM
    ssh rye agent-manager install dev1                     # install agent-manager on dev1
    ssh -t dev1.rye claude                                 # run claude, then /login and /exit (once per new VM)
    ssh rye-gui                                            # leave running, then open http://localhost:8080

The [first-run tutorial](docs/tutorial/first-run.md) has the same steps with the output of each.

## The parts
- **Incus host**: the machine where you type every `ssh rye` command.
- **VMs**: the VMs you create with `ssh rye new`. You log in to one with `ssh <vm>.rye`.
- **GUI**: the web page that shows the agent sessions of every VM.
- **Service VM**: `rye-service`, the VM that `make up` creates. It runs rye and serves the GUI.
- **agent-manager**: the program in a VM that runs agent sessions. `ssh rye agent-manager install <vm>` installs it.
- **Integration**: a way for a VM to call an API with a token that rye holds. The VM calls
  `http://<name>.int.rye/`, and the service VM adds the token.

## Documentation
Read these in order the first time:

1. [First run](docs/tutorial/first-run.md) sets up rye and Claude in a VM.
2. [Open the GUI from your phone](docs/how-to/tailnet.md) sets up Tailscale for it.
3. [Give a VM access to an API](docs/how-to/integrations.md) stores an API token on the service VM for a VM to use
   through rye.
4. [Run a forge with CI](docs/tutorial/forge.md) sets up a Forgejo git server with CI and a bot user for Claude.

Day to day:

- [Manage VMs](docs/how-to/manage-vms.md)
- [Use a VM in the GUI](docs/how-to/use-vm-in-gui.md)
- [Let Claude manage your VMs](docs/how-to/work-from-admin.md)
- [Upgrade rye](docs/how-to/upgrade-rye.md)
- [Set the storage pool, subnet and SSH keys](docs/how-to/deploy-options.md)
- [Upgrade the forge to a new Forgejo release](docs/how-to/forge-upgrade.md)

How rye works:

- [Security model](docs/explanation/security.md)
- [Architecture](docs/explanation/architecture.md)
- [How a VM is set up](docs/explanation/vm-setup.md)

Reference:

- [rye commands](docs/reference/rye.md)
- [GUI](docs/reference/gui.md)
- [SSH config](docs/reference/ssh-config.md)
- [make targets](docs/reference/make.md)
- [Docs served to VMs](docs/reference/docs-site.md)

Working on rye's code:

- [Develop rye](docs/how-to/developing.md)
- [Set up a workspace VM](docs/how-to/workspace-vm.md)
- [GUI API](docs/reference/gui-api.md)
- [agent-manager protocol](docs/reference/agent-manager-protocol.md)
