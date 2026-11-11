# First run

This tutorial sets up rye on a bare Debian or Ubuntu machine. It creates a VM called `dev1` and ends in the GUI, a
web page where Claude installs a package in `dev1`.

rye's docs call this machine the Incus host, because [Incus](https://linuxcontainers.org/incus/) runs the VMs on it.
Run every command on the Incus host, as a user with sudo. If the host is a server, log in to it over SSH first.

The GUI opens in a browser on the host. For a host without a desktop, follow this tutorial to step 6, then
[open the GUI from your phone](../how-to/tailnet.md).

## 1. Check the host
The host must be x86_64 Debian or Ubuntu with these tools on `PATH`:

| Tool | Version | Used by |
|---|---|---|
| [Go](https://go.dev/doc/install) | 1.27.4, as in `go.mod` | `make build` |
| [Node](https://nodejs.org/en/download) | 24 or later | `make build` |
| [pnpm](https://pnpm.io/installation) | 11.8.0 | `make build` |
| git, curl | any | `make build`, `scripts/incus-setup.sh` |
| ssh-keygen | any | creating your SSH key |
| gpg | any | `scripts/incus-setup.sh` |

You also need a Claude account. You log in to it inside the VM in step 6.

## 2. Clone the repository

    git clone <repository URL> rye
    cd rye

Every later command runs from this directory.

## 3. Install Incus
On a host without Incus, run:

    scripts/incus-setup.sh

It installs the Incus packages from Zabbly's stable apt repository. On a host with no storage pool, it creates a 30GiB
ZFS pool named `default`. The pool stores the disks of every VM. Set `SIZE` for another size, such as
`SIZE=100GiB scripts/incus-setup.sh`.

The next step needs the Incus socket, which only members of the `incus-admin` group can use. Join the group first,
then log out and in again:

    sudo usermod -aG incus-admin "$USER"

Members of `incus-admin` have full control of Incus. That equals root on the host.

## 4. Build rye and bring up the service VM

`make up` authorizes `~/.ssh/id_ed25519.pub` to log in to the service VM. Create that key first when it's missing:

    ssh-keygen -t ed25519

Then build and deploy:

    make up

`make up` builds rye into `dist/`. It then creates the service VM, `rye-service`, which runs rye, and a private
network for your VMs.

Two errors are specific to a host that already runs Incus:

- `ryectl: no storage pool default; set POOL=`. Run `POOL=<pool> make up` with a pool from `incus storage list`.
- `ryectl: core.https_address is <address>; rye needs 198.18.0.1:8443`. Incus listens on another address.
  Run `incus config unset core.https_address`, then `make up` again, which sets the address rye needs. To keep the
  other address in use, set `:8443`. That opens the Incus API on every interface of the host.

[Set the storage pool, subnet and SSH keys](../how-to/deploy-options.md) describes these settings.

`make up` ends by printing the next commands. Steps 5 and 6 run them:

    ==> ready. Next:
        dist/ryectl ssh-config               once, for the SSH aliases
        ssh rye new <vm>
        ssh rye agent-manager install <vm>   to install agent-manager in a VM
        ssh rye agent-manager install --all  after an upgrade, in every VM with agent-manager

## 5. Add the SSH aliases

    dist/ryectl ssh-config

It writes four aliases to `~/.ssh/config.d/rye`. The first time, it prints a line to add at the top of
`~/.ssh/config`, before any `Host` block:

    Include config.d/rye

The tutorial uses `rye` for commands, `<vm>.rye` to log in to a VM and `rye-gui` for the GUI. The fourth is in
[SSH config](../reference/ssh-config.md).

Check the alias:

    ssh rye ls

It prints the column headings `NAME STATUS CPU MEMORY DISK` and no rows.

## 6. Create a VM and log in to Claude

    ssh rye new dev1

rye creates `dev1` with 2 CPUs, 4GiB of memory and a 30GiB disk. It waits for first boot to finish,
then prints `Ready: dev1 in <duration>`.

Install agent-manager, the program that runs Claude for the GUI:

    ssh rye agent-manager install dev1

It prints `agent-manager is listening on 127.0.0.1:7420`. The install also adds the `claude` command to `dev1`'s `PATH`.

agent-manager uses the Claude login in `dev1`'s `~/.claude`. The login is a credential for your Claude account, and
every command in `dev1` can read it. Create it once per new VM:

    ssh -t dev1.rye claude

SSH asks you to accept `dev1`'s host key on the first connection. In Claude, type `/login`, open the printed link and
finish the login in your browser. Then type `/exit`.

## 7. Open the GUI
In a second terminal, start the tunnel:

    ssh rye-gui

It prints nothing and keeps running. Open <http://localhost:8080> in a browser on the host.

1. Pick `dev1`.
2. Send `Install the tree package, then show me /etc/apt as a tree` from the message box. The first message starts
   a new session.
3. Claude asks to run a command that installs the package. Approve it in the page. Claude asks before each later
   command unless you choose to always allow it.

`dev1` stores the session. It is in the session list when you open the page again.

Without the Claude login from step 6, the reply is `Not logged in · Please run /login`.

## Next steps
- [Open the GUI from your phone](../how-to/tailnet.md)
- [Use another VM in the GUI](../how-to/use-vm-in-gui.md)
- [Give a VM access to an API](../how-to/integrations.md)
- [Run a forge with CI](forge.md)
- [How a VM is set up](../explanation/vm-setup.md)
