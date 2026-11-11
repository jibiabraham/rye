#!/bin/bash
# Joins the service VM to the owner's tailnet as tag:rye and serves the GUI there over HTTPS to the owner's login.
# LOGIN= names that login on the first run. Rerun to print the address and check the tailnet policy. With the argument
# policy, it only prints the tailnet policy for LOGIN.
set -euo pipefail
cd "$(dirname "$0")/.."
SERVICE=rye-service
DROPIN=/etc/systemd/system/ryed.service.d/tailnet.conf
on() { incus --project default exec "$SERVICE" -- "$@"; }
die() { echo "tailscale: $*" >&2; exit 1; }

# The tailnet policy the GUI needs, for the Tailscale admin console's Access controls page. The tests make the console
# refuse a later edit that opens other ports or lets the service VM reach the owner's devices.
policy() {
  cat <<P
{
  "tagOwners": {"tag:rye": ["$login"]},
  "grants": [
    {"src": ["autogroup:member"], "dst": ["autogroup:self"], "ip": ["*"]},
    {"src": ["$login"], "dst": ["tag:rye"], "ip": ["tcp:443"]},
  ],
  "tests": [
    {"src": "$login", "accept": ["tag:rye:443"], "deny": ["tag:rye:22", "tag:rye:80"]},
    {"src": "tag:rye", "deny": ["$login:22", "$login:443"]},
  ],
}
P
}
fix() {
  echo "tailscale: $*" >&2
  echo "Tailnet policy for https://login.tailscale.com/admin/acls, to merge with your own rules." >&2
  policy >&2
}

if [ "${1:-}" = policy ]; then
  login=${LOGIN:-}
  [ -n "$login" ] || die "set LOGIN= to your tailnet login, as the admin console shows it, e.g. LOGIN=you@github"
  policy
  exit
fi
login=${LOGIN:-$(on sed -n 's/^Environment=RYE_TAILNET_LOGIN=//p' "$DROPIN" 2>/dev/null || true)}
[ -n "$login" ] || die "set LOGIN= to your tailnet login, as the admin console shows it, e.g. LOGIN=you@github"

# Prints a login link and QR code, then waits for the login. --accept-dns=false keeps .rye lookups on the bridge's DNS.
on tailscale up --qr --accept-dns=false --advertise-tags=tag:rye --hostname="$SERVICE" || { fix "the tailnet refused tag:rye"; exit 1; }
on tailscale serve reset
on tailscale serve --bg http://127.0.0.1:8081
status=$(on tailscale status --json)
name=$(jq -r '.Self.DNSName | rtrimstr(".")' <<<"$status")
on sh -c "mkdir -p ${DROPIN%/*} && printf '[Service]\nEnvironment=RYE_TAILNET_HOST=%s\nEnvironment=RYE_TAILNET_LOGIN=%s\n' '$name' '$login' >$DROPIN"
on systemctl daemon-reload
on systemctl restart ryed
echo "GUI: https://$name, for $login"

# Reads which sources the tailnet lets reach this node, and on which ports. The verdict is unknown when the
# tailscale debug output changes format.
verdict=$(on tailscale debug netmap | jq -r --argjson s "$status" --arg login "$login" -f scripts/tailnet-access.jq) || verdict=unknown
case $verdict in
  ok) echo "policy: only $login reaches this node, on port 443" ;;
  none) fix "no device can reach the GUI on port 443" ;;
  open*) fix "the tailnet lets more reach this node than $login on port 443 (${verdict#open })" ;;
  *) echo "tailscale: could not read the node's packet filter; check the policy against scripts/tailscale.sh" >&2 ;;
esac
