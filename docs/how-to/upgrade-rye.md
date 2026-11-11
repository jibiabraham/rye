# Upgrade rye

An upgrade builds `rye` and `ryed`, with the GUI inside `ryed`, installs them on the service VM and restarts `ryed`. You
then reinstall agent-manager in each VM that has it. The upgrade keeps your VMs, their disks and the integrations. Run
the steps on the Incus host, from the root of your clone.

## 1. Update the clone

    git pull

## 2. Build and upgrade the service VM

    make up

`make up` keeps the addresses, the pool and the authorized keys of the existing deploy.

## 3. Install the new agent-manager in each VM
Install it in every running VM that has agent-manager:

    ssh rye agent-manager install --all

It prints `Skipped <vm>, which is stopped` for each stopped VM with agent-manager. A stopped VM keeps its old
agent-manager until you start it with `ssh rye start <vm>`, which installs the new agent-manager. The install restarts
agent-manager and closes the VM's live sessions. A closed session resumes on its next message.

## 4. Upgrade the admin VM
If you have an [admin VM](work-from-admin.md), upgrade its agent-manager, GUI and `ryed`:

    make admin
