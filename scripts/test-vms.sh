#!/bin/bash
# Runs the VM commands against a deployed service VM. Creates and removes a real VM.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"
VM=rye-test-vms
status() { rye show "$VM" --json | jq -r .status; }
expect() { [ "$(status)" = "$1" ] || { echo "status is $(status), want $1" >&2; exit 1; }; }

trap 'rye rm "$VM" --yes || true' EXIT
vm() { ssh_vm "$VM.rye" "$@"; }

step new
printf '#cloud-config\nruncmd:\n  - sleep 5\n  - touch /cloud-init-ran\n' | rye new "$VM" --disk 12GiB --cloud-init
expect running
rye ls --json | jq -e --arg vm "$VM" 'any(.[]; .name == $vm)' >/dev/null

[ "$(vm whoami)" = dev ]

step isolation
service=$(ssh -G rye | awk '$1 == "hostname" { print $2 }')
# shellcheck disable=SC2016  # expands in the VM
bridge=$(vm 'ip route show default | awk "{ print \$3 }"')
reaches() { vm "timeout 5 bash -c '</dev/tcp/$1/$2'" 2>/dev/null; }
for target in "$bridge 22" "$bridge 8443" "$service 22"; do
  # shellcheck disable=SC2086  # two words, an address and a port
  if reaches $target; then
    echo "the VM reaches $target" >&2
    exit 1
  fi
done
reaches "$service" 80

step "stop, resize and start"
vm 'touch marker; sudo touch /root-marker'
host_key=$(vm 'cat /etc/ssh/ssh_host_ed25519_key.pub')
# rye start runs the setup. The setup writes the key lookup.
vm 'sudo rm /usr/local/sbin/rye-keys'
rye stop "$VM"
expect stopped
rye resize "$VM"
rye resize "$VM" --cpu 1 --memory 2GiB --disk 13GiB
if rye resize "$VM" --disk 11GiB 2>/dev/null; then
  echo "disk shrank" >&2
  exit 1
fi
rye start "$VM"
wait_for_vm "$VM"
rye show "$VM" --json | jq -e '.cpu == "1" and .memory == "2GiB" and .disk == "13GiB"' >/dev/null
# shellcheck disable=SC2016  # expands in the VM
vm '[ "$(nproc)" = 1 ] && [ "$(df -BG --output=size / | tail -1 | tr -dc 0-9)" -ge 12 ]'
if rye resize "$VM" --cpu 2 2>/dev/null; then
  echo "resized a running VM" >&2
  exit 1
fi

vm '[ -e marker ] && [ -e /root-marker ] && [ -e /cloud-init-ran ]'
[ "$(vm 'cat /etc/ssh/ssh_host_ed25519_key.pub')" = "$host_key" ]

step integrations
printf 'test-value' | rye integration add rye-test --url https://example.com --header X-Rye-Test
code() { vm "curl -s -o /dev/null -m 10 -w '%{http_code}' http://rye-test.int.rye/"; }
[ "$(code)" = 403 ]
rye integration attach rye-test "$VM"
[ "$(code)" = 200 ]
rye integration detach rye-test "$VM"
[ "$(code)" = 403 ]
rye integration attach rye-test "$VM"
rye integration rm rye-test
[ "$(code)" = 403 ]

step restart
# A graceful restart needs a booted guest.
wait_for_vm "$VM"
rye restart "$VM"
expect running
echo "test-vms passed in ${SECONDS}s"
