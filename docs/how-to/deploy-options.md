# Set the storage pool, subnet and SSH keys

`make up` reads the variables below from the environment. A rerun upgrades the service VM in place and keeps your
VMs.

## Join incus-admin when make up can't reach Incus
`make up`, `make down`, `make admin` and `make admin-remove` reach Incus through its socket. For a user outside the
`incus-admin` group, they fail with:

    ryectl: cannot reach Incus at /var/lib/incus/unix.socket (...); join incus-admin, then log in again

`make tailscale` needs the same group. Join it as in [step 3 of First run](../tutorial/first-run.md#3-install-incus).

## Use another storage pool
`make up` uses the pool named `default` for the service VM and your VMs. Name another one with `POOL`:

    POOL=fast make up

If the pool doesn't exist, `make up` fails with `ryectl: no storage pool <pool>; set POOL=`. `incus storage list`
lists the pools.

## Use another subnet
`make up` creates the bridge on 198.18.0.0/24, with the service VM at 198.18.0.10 and 198.18.0.11 reserved for the
admin VM.

On a host that already routes that /24, such as a workspace VM, `make up` fails with
`ryectl: this host already routes 198.18.0.0/24 through <interface>`.

Pick another /24 from 198.18.0.0/15:

    BRIDGE_IP=198.19.0.1 SERVICE_IP=198.19.0.10 make up

Later runs keep these addresses. The admin VM takes .11 of the new /24.

## Move a deploy to another subnet
Incus can't delete `ryebr0` while the `rye` project's default profile or a VM's network interface uses it. These
commands delete the VMs and their disks, then deploy on the new subnet:

    ssh rye rm <vm> --yes                                  # for each VM
    make down
    incus profile device remove default eth0 --project rye
    incus network delete ryebr0
    BRIDGE_IP=198.19.0.1 SERVICE_IP=198.19.0.10 make up

`make up` adds the interface back to the profile on the new bridge.

## Authorize another SSH key
`make up` authorizes `~/.ssh/id_ed25519.pub`. Name another public key with `KEY`:

    KEY=~/.ssh/other.pub make up

Each run adds its key and keeps the keys earlier runs added. Each authorized key logs in to the service VM and to
every VM.

## Remove an authorized key
The keys are in `/home/rye/.ssh/authorized_keys` on the service VM. VMs read the same file through the service VM on
each login. Delete the key's line. This command matches it by its comment, `old@laptop`:

    incus exec rye-service -- sed -i '/ old@laptop$/d' /home/rye/.ssh/authorized_keys

Keep the line that ends in `rye-service`. It is the service VM's own key. `rye` and `ryed` use it to log in to
VMs.

## Remove the service VM

    make down

It deletes the service VM, its Incus certificate and its entry in `~/.ssh/known_hosts.d/rye`. It keeps your VMs and
their disks, the bridge, the firewalls and the `rye` project. A later `make up` creates a new service VM that manages
the existing VMs.
