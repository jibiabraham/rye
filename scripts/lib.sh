# shellcheck shell=bash
# Helpers for scripts/test-*.sh, which source this file. They reach the deployed service VM through the rye alias that
# ryectl ssh-config writes, and its VMs through the <vm>.rye aliases.

# Runs a rye command on the service VM.
rye() { ssh -o BatchMode=yes "rye" "$@"; }

# Runs ssh to a VM without host key checks, because a reused VM name has a new host key.
ssh_vm() { ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR "$@"; }

# Waits until ssh to the VM named $1 works.
wait_for_vm() {
  for _ in $(seq 1 90); do
    ssh_vm "$1.rye" true 2>/dev/null && return
    sleep 2
  done
  return 1
}

# Prints a step with the seconds since the start.
step() { echo "==> $* (${SECONDS}s)"; }
