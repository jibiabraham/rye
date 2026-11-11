# rye commands

The service VM runs `rye` as the sshd `ForceCommand` of its `rye` user. Run a command with `ssh rye <command>`. [SSH
config](ssh-config.md) defines the `rye` host alias.

`ssh rye` with no command prints the usage and exits 0.

Errors print `rye: <message>` and exit 1.

## VMs

| Command | Does |
|---|---|
| `new <vm> [--cpu n] [--memory m] [--disk s] [--cloud-init] [--integration a,b] [--verbose]` | Creates and starts a VM, waits for first boot, and prints `Ready: <vm> in <duration>`. |
| `ls [--json]` | Lists VMs with status, CPU, memory and disk size. `--json` adds an `agentManager` field, true after `agent-manager install`. |
| `show <vm> [--json]` | Shows one VM. |
| `start <vm>`, `restart <vm>` | Starts or restarts a VM and runs rye's setup in it. It also installs agent-manager in a VM that has it. |
| `stop <vm>` | Stops a VM. |
| `rm <vm> --yes` | Deletes a VM and its disk, and prints `Removed <vm>`. It also removes each integration whose URL host is `<vm>.rye`. For a name with no VM, it removes the host key and those integrations and prints `No VM <vm>; removed its host key and integrations`. A new VM with the same name gets a new host key, as [Manage VMs](../how-to/manage-vms.md#delete-a-vm) describes. |
| `resize <vm> [--cpu n] [--memory m] [--disk s]` | Changes a stopped VM's sizes. The disk only grows. |

Flags can come before or after the positional arguments.

`new` flags:

| Flag | Default | Meaning |
|---|---|---|
| `--cpu` | 2 | CPU count. |
| `--memory` | 4GiB | Memory. |
| `--disk` | 30GiB | Disk size. |
| `--cloud-init` | off | Reads cloud-init user-data from stdin. |
| `--integration` | none | Integrations to attach, comma-separated. |
| `--verbose` | off | Prints every line of the VM's first-boot output. |

VMs run Ubuntu 26.04 from `images.linuxcontainers.org`.

## Integrations

| Command | Does |
|---|---|
| `integration add <name> --url u [--header h]` | Adds or replaces an integration. Reads the credential from stdin. `--header` defaults to `Authorization`. |
| `integration ls` | Lists integrations with URL, header and attached VMs. |
| `integration rm <name>` | Removes an integration and its attachments. |
| `integration attach <name> <vm>` | Gives a VM access to an integration. |
| `integration detach <name> <vm>` | Removes a VM's access to an integration. |

Names are 1 to 63 lowercase letters, digits and hyphens, and start and end with a letter or digit. `keys` and `docs` are
reserved for `keys.int.rye`, which serves the SSH keys that VMs accept, and the [docs site](docs-site.md),
`docs.int.rye`.

## Other

| Command | Does |
|---|---|
| `agent-manager install <vm>` | Installs Node.js and agent-manager in a running VM, starts it, and prints `agent-manager is listening on 127.0.0.1:7420`. On a VM that isn't running it prints `rye: <vm> is <status>; ssh rye start <vm>`. |
| `agent-manager install --all` | Runs `agent-manager install` in every running VM that has agent-manager, with each VM's name before its line, and prints `Skipped <vm>, which is stopped` for each stopped one. It installs in the others when one fails, and reports each failure. |
| `ssh-config` | Prints the SSH aliases for this service VM. [SSH config](ssh-config.md) describes them. |
| `help`, `-h`, `--help` | Prints the usage. |

A command that names a missing VM prints `rye: no VM <name>; ssh rye ls lists them`.
