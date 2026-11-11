#!/usr/bin/env bash
# Install or update ssh-site on a server prepared with deploy/bootstrap.sh.
#
#   deploy/deploy.sh <binary> <ssh target>
#
# <ssh target> is the admin address, for example root@<tailnet address>.
# The old binary is kept as ssh-site.prev and restored if the new one does
# not stay up.
set -euo pipefail

BIN=${1:?usage: deploy.sh <binary> <ssh target>}
HOST=${2:?usage: deploy.sh <binary> <ssh target>}
DIR=$(cd "$(dirname "$0")" && pwd)

[ -f "$BIN" ] || { echo "no binary at $BIN (run make build)"; exit 1; }
file "$BIN" | grep -q 'ELF 64-bit LSB executable, x86-64' || { echo "$BIN is not a linux/amd64 binary"; exit 1; }

scp -q "$BIN" "$HOST:/usr/local/bin/ssh-site.new"
scp -q "$DIR/ssh-site.service" "$HOST:/etc/systemd/system/ssh-site.service.new"

ssh "$HOST" 'bash -s' <<'EOF'
set -euo pipefail
[ -f /etc/ssh-site/ssh-site.env ] || { echo "missing /etc/ssh-site/ssh-site.env; run deploy/bootstrap.sh first"; exit 1; }
chmod 0755 /usr/local/bin/ssh-site.new
/usr/local/bin/ssh-site.new -version
[ -f /usr/local/bin/ssh-site ] && cp -p /usr/local/bin/ssh-site /usr/local/bin/ssh-site.prev
mv /usr/local/bin/ssh-site.new /usr/local/bin/ssh-site
mv /etc/systemd/system/ssh-site.service.new /etc/systemd/system/ssh-site.service
systemctl daemon-reload
systemctl enable --quiet ssh-site
systemctl restart ssh-site
sleep 3
if ! systemctl is-active --quiet ssh-site; then
  journalctl -u ssh-site -n 30 --no-pager
  if [ -f /usr/local/bin/ssh-site.prev ]; then
    echo "new build did not stay up; restoring the previous one"
    mv /usr/local/bin/ssh-site.prev /usr/local/bin/ssh-site
    systemctl restart ssh-site
  fi
  exit 1
fi
systemctl --no-pager --lines=0 status ssh-site | head -5
echo "host key: $(/usr/local/bin/ssh-site -fingerprint -state-dir /var/lib/ssh-site)"
EOF
