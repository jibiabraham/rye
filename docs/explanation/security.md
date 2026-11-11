# Security model

Claude can run a wrong command, or follow instructions hidden in a web page or a repository it reads. This page
lists what a command in a VM can reach.

## What Claude in a VM can reach
- Everything inside its VM. Claude runs as `dev`, who has sudo, and can read every file. That includes the VM's Claude
  login in `~/.claude`, a credential for your Claude account. Deleting the VM doesn't revoke it.
- The internet.
- The integrations attached to its VM, with all the rights of each one's token. The service VM stores the tokens.
- rye's docs and code, on the docs site.

It can't reach private addresses or the host's bridge address, and it can't run `ssh rye` commands. It reaches another
VM only through an integration whose URL host is `<vm>.rye`. Claude on the [admin VM](../how-to/work-from-admin.md)
can run every `ssh rye` command and log in to every VM.

## What to do
- Put in a VM only what Claude may read and send to the internet.
- Give each integration a token with the fewest rights the task needs. [Run a forge with CI](../tutorial/forge.md)
  uses a bot with no right to merge.
- Pick the session's permission mode in the [GUI](../reference/gui.md). The modes are Manual, which asks before
  each command, Accept edits, which changes files without asking, and Auto, which runs most commands without asking.
  A new session starts in Manual.
- Don't rely on an approval to stop an attack. A command you approve runs with sudo and can answer later
  approvals itself, through agent-manager's port in the VM.

## What a VM can reach
- The internet, through NAT on the bridge.
- `ryed` on the service VM's port 80, for the docs site, the integrations attached to that VM and `keys.int.rye`,
  which serves the SSH keys that VMs accept. The docs site serves every file in the deployed commit to every VM.

The VM firewall rejects outbound traffic to 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, 100.64.0.0/10,
127.0.0.0/8 and the host's bridge address. Port isolation on the bridge blocks traffic between VMs.

Inbound, the VM firewall allows SSH from the service VM. It rejects all other inbound traffic.

## What reaches the service VM
- SSH from the host's bridge address and from the admin VM at .11 of the bridge's /24, for the `rye` user with an
  authorized key. Every `ssh rye` command and the GUI tunnel run on the Incus host or the admin VM.
- Port 80 from the bridge's /24, for VMs.
- After `make tailscale`, HTTPS from the owner's tailnet login. `ryed` checks `Tailscale-User-Login` on every request.

`ryed`'s GUI API checks `Host` and `Origin`. A page from another origin in the owner's browser can't call the API
through the tunnel.

## Credentials
- rye's access to Incus is a certificate on the service VM that covers only the `rye` project. In that project, a
  VM's network interface can only be on the `ryebr0` bridge.
- `ryed` removes any `Authorization` header a VM sends before it adds an integration's token.
- A VM attached to an integration whose URL host is `<vm>.rye` can reach one port on that VM through the service
  VM.
- An SSH key that `make up` authorizes logs in to the service VM and to every VM.

## Tailnet
The tailnet policy in [Open the GUI from your phone](../how-to/tailnet.md) grants the owner's devices port 443 on
`tag:rye`, the service VM's tag. No grant has `tag:rye` as its source. The policy's tests fail when an edit opens ports
22 or 80 on `tag:rye`, or lets `tag:rye` connect to the owner's devices. The Tailscale admin console rejects an edit
that fails a test.

## Incus access on the host
`make up`, `make down`, `make tailscale`, `make admin` and `make admin-remove` need membership of the `incus-admin`
group. That membership is equivalent to root on the host.
