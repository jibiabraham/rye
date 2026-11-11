#!/bin/sh
# Setup for every VM: dev with sudo, sshd, the key lookup and automatic upgrades. rye runs it as root through the Incus
# guest agent after the first boot and on each rye start or restart. incus.go fills in its placeholders from
# internal/vmconfig.
set -eu
export DEBIAN_FRONTEND=noninteractive
command -v sshd >/dev/null || { apt-get update -qq; apt-get install -y -qq openssh-server unattended-upgrades; }
id dev >/dev/null 2>&1 || useradd -m -s /bin/bash dev
echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-rye-dev
chmod 440 /etc/sudoers.d/90-rye-dev
visudo -cqf /etc/sudoers.d/90-rye-dev
cat > /etc/ssh/sshd_config.d/10-rye.conf <<'C'
@SSHD@
C
cat > /usr/local/sbin/rye-keys <<C
#!/bin/sh
exec curl -fsS -m 5 http://keys.int.rye/
C
chmod 755 /usr/local/sbin/rye-keys
cat > @AUTO_UPGRADES_PATH@ <<'C'
@AUTO_UPGRADES@
C
systemctl restart ssh
