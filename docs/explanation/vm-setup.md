# How a VM is set up

## Vendor-data and setup
rye passes cloud-init vendor-data to each VM, a cloud-config that creates `dev` on the first boot.

rye then runs its setup script as root through the Incus guest agent, after the first boot and each time
`ssh rye start` or `ssh rye restart` boots the VM:

- It creates `dev` when that user does not exist, and gives `dev` passwordless sudo.
- It sets up sshd and the authorized-keys lookup from `keys.int.rye`.
- It turns on automatic upgrades.

`ssh rye agent-manager install` installs Node.js, links `/usr/local/bin/claude` and turns on systemd linger for `dev`.
[Architecture](architecture.md#the-steps-of-an-agent-manager-install) lists its steps. `ssh rye start` and `ssh rye
restart` install agent-manager in a VM that has it.

## User-data
`--cloud-init` passes user-data, and cloud-init runs it on the first boot. A `users` key in it replaces the
vendor-data's `users` entry, and rye's setup creates `dev` with sudo if your entry left it out.

## Disk size
`ssh rye resize --disk` grows a stopped VM's disk. The filesystem grows to the new size on the next boot.

## OS updates
unattended-upgrades installs updates in place. To move to a new Ubuntu release, run `sudo do-release-upgrade` in the VM.
