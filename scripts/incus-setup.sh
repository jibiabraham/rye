#!/bin/bash
# Installs Incus from the Zabbly stable channel for development and for hosts without Incus. On a host with no storage
# pool it preseeds a ZFS pool named default on a sparse loop file of SIZE (30GiB unless set). make up creates the
# network. With any storage pool present, the init step is skipped.
set -euo pipefail
# Incus create commands read a YAML config from stdin when it isn't a terminal.
exec </dev/null

FINGERPRINT=4EFC590696CB15B87C73A3AD82CC8797C838DCFD
CHANNEL=stable
SIZE=${SIZE:-30GiB}

log() { echo "==> $*"; }

log "apt repository: Zabbly $CHANNEL"
key=$(mktemp)
curl -fsSL https://pkgs.zabbly.com/key.asc -o "$key"
got=$(gpg --show-keys --with-colons "$key" | awk -F: '/^fpr:/{print $10; exit}')
[ "$got" = "$FINGERPRINT" ] || { echo "incus-setup: Zabbly key fingerprint is $got, expected $FINGERPRINT" >&2; exit 1; }
sudo install -D -m 0644 "$key" /etc/apt/keyrings/zabbly.asc
rm -f "$key"
sudo tee /etc/apt/sources.list.d/zabbly-incus.sources >/dev/null <<EOF
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/incus/$CHANNEL
Suites: $(. /etc/os-release && echo "$VERSION_CODENAME")
Components: main
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/zabbly.asc
EOF

log "packages"
sudo apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq incus zfsutils-linux nftables

log "automatic updates from the Zabbly origin"
sudo tee /etc/apt/apt.conf.d/52unattended-zabbly >/dev/null <<EOF
Unattended-Upgrade::Origins-Pattern {
  "origin=pkgs.zabbly.com,label=incus-$CHANNEL";
};
EOF

log "init"
# The first client call prints a first-run hint about incus admin init, which this step replaces.
pools=$(sudo incus storage list -f csv -c n 2>/dev/null | paste -sd ' ')
if [ -n "$pools" ]; then
  log "storage pools exist ($pools), init skipped; make up takes one with POOL="
else
  # The same preseed as deployments/workspace-vm/cloud-init.yaml.
  printf 'storage_pools:\n- name: default\n  driver: zfs\n  config:\n    size: %s\n' "$SIZE" | sudo incus admin init --preseed
  log "storage pool default: zfs on a $SIZE loop file"
fi

log "done; join incus-admin (sudo usermod -aG incus-admin \"\$USER\"), log in again, then run make up"
