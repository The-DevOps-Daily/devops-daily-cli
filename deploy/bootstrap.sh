#!/usr/bin/env bash
# One-time server setup, run as root on a fresh Ubuntu 24.04 droplet that has
# already joined the tailnet (tailscale up). It:
#   1. moves OpenSSH to the tailnet address only, so public port 22 is free
#      for the app (with a 5-minute automatic revert until you confirm);
#   2. writes /etc/ssh-site/ssh-site.env with the public listen address;
#   3. creates an empty sponsor slot config if there is none;
#   4. lets unattended-upgrades update Tailscale too.
# Safe to run again.
set -euo pipefail

TS_IP=$(tailscale ip -4 2>/dev/null | head -1)
case "$TS_IP" in 100.*) ;; *) echo "join the tailnet first (tailscale up)"; exit 1 ;; esac

# 1. Admin SSH on the tailnet only. Ubuntu 24.04 starts sshd from ssh.socket;
# its generator turns ListenAddress into the socket address, and the socket
# uses FreeBind, so it binds even before tailscaled is up at boot.
CONF=/etc/ssh/sshd_config.d/10-tailnet-only.conf
NEW=$(cat <<CONF
# Admin SSH listens only on the tailnet address. Public port 22 is the
# ssh-site app. ssh.socket has FreeBind=yes, so boot order does not matter.
ListenAddress $TS_IP
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
X11Forwarding no
CONF
)
if [ "$(cat "$CONF" 2>/dev/null)" != "$NEW" ]; then
  systemd-run --quiet --on-active=300 --unit=ssh-revert /bin/sh -c \
    "rm -f $CONF; systemctl daemon-reload; systemctl restart ssh.socket"
  printf '%s\n' "$NEW" > "$CONF"
  sshd -t
  systemctl daemon-reload
  systemctl restart ssh.socket
  echo "sshd now listens on $TS_IP:22 only."
  echo "From another terminal, ssh root@$TS_IP, then run: systemctl stop ssh-revert.timer"
  echo "Otherwise the change reverts in 5 minutes."
fi

# 2. The app listens on the public address. Binding 0.0.0.0:22 would clash
# with sshd on the tailnet address.
install -d -m 0755 /etc/ssh-site
if [ ! -f /etc/ssh-site/ssh-site.env ]; then
  PUBLIC_IP=$(curl -fsS --max-time 5 http://169.254.169.254/metadata/v1/interfaces/public/0/ipv4/address)
  printf 'SSH_SITE_LISTEN=%s:22\n' "$PUBLIC_IP" > /etc/ssh-site/ssh-site.env
  chmod 0644 /etc/ssh-site/ssh-site.env
fi

# 3. Sponsor slot: edit this file to change the line; no restart needed.
if [ ! -f /etc/ssh-site/sponsor.conf ]; then
  cat > /etc/ssh-site/sponsor.conf <<'CONF'
# One line on every screen: "Supported by <name>: <text>  <url>".
# Edit and save; the app picks it up within 10 seconds. Empty hides it.
#name=
#text=
#url=
CONF
  chmod 0644 /etc/ssh-site/sponsor.conf
fi

# 4. Security updates for Tailscale as well as Ubuntu.
cat > /etc/apt/apt.conf.d/52unattended-upgrades-tailscale <<'CONF'
Unattended-Upgrade::Origins-Pattern {
        "origin=Tailscale,label=Tailscale";
};
CONF

echo "bootstrap done. Next: make deploy HOST=root@$TS_IP"
