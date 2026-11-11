# Manage VMs

[rye commands](../reference/rye.md) lists every flag.

## Create a VM with other sizes

    ssh rye new build --cpu 6 --memory 12GiB --disk 60GiB

To run your own cloud-init user-data on first boot, pass `--cloud-init` and send the file on stdin:

    ssh rye new build --cloud-init < user-data.yaml

[How a VM is set up](../explanation/vm-setup.md#user-data) describes when your user-data runs and what a `users` key
in it changes.

## Resize a VM
`resize` works on a stopped VM:

    ssh rye stop build
    ssh rye resize build --memory 16GiB --disk 80GiB
    ssh rye start build

## Delete a VM

    ssh rye rm build --yes

`rm` also removes each integration whose URL points at `build.rye`, with its token, and prints a line for each.

A new VM with the same name gets a new host key. On the Incus host, forget the old one before you log in to the new
VM:

    ssh-keygen -R build.rye -f ~/.ssh/known_hosts.d/rye
