# Set up a workspace VM

A workspace VM is a VM for working on this repository. `deployments/workspace-vm/cloud-init.yaml` installs the toolchain
that `make gate` needs, at the versions the repository pins. The cloud-init also installs Incus for `make up` and the
`test-*` targets. [Develop rye](developing.md) describes the gate and the targets.

Incus VMs inside the workspace VM need nested virtualization on the Incus host. Check it on the host:

    cat /sys/module/kvm_*/parameters/nested

It prints `Y` or `1` when nested virtualization is on.

## Create it
From the root of a clone of this repository on the Incus host:

    ssh rye new ws --cpu 6 --memory 12GiB --disk 60GiB \
      --cloud-init < deployments/workspace-vm/cloud-init.yaml

## Clone the repository inside it

    ssh ws.rye
    git clone <repository URL> rye
    cd rye

## Deploy rye inside it
`make up` inside the workspace VM needs another /24, because the VM's own address is in 198.18.0.0/24:

    BRIDGE_IP=198.19.0.1 SERVICE_IP=198.19.0.10 make up

The cloud-init adds `dev` to `incus-admin` inside the VM.
