#!/bin/sh
# Run as root on the demo instance (Debian 12) from a directory holding these files and the two binaries.
# The recorded map, county GIS and photos go in /var/lib/bitdispatcher first (copied, not recorded there).
set -e
export DEBIAN_FRONTEND=noninteractive
if [ ! -f /swapfile ]; then
  fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
sysctl -q vm.swappiness=10 && echo 'vm.swappiness=10' > /etc/sysctl.d/90-swap.conf
apt-get update -qq && apt-get install -y -qq caddy curl jq >/dev/null
id bitdispatcher >/dev/null 2>&1 || useradd --system --home /var/lib/bitdispatcher --shell /usr/sbin/nologin bitdispatcher
install -d -o bitdispatcher -g bitdispatcher -m 700 /var/lib/bitdispatcher
install -d -m 755 /opt/bitdispatcher/bin
install -m 755 bitdispatcher bvsim /opt/bitdispatcher/bin/
install -m 755 demo-setup.sh demo-reset.sh /opt/bitdispatcher/
install -m 644 bitdispatcher.service bvsim.service demo-reset.service demo-reset.timer /etc/systemd/system/
install -d -m 700 /etc/bitdispatcher
install -m 644 Caddyfile /etc/caddy/Caddyfile
systemctl daemon-reload
systemctl enable -q bitdispatcher
systemctl restart bitdispatcher
sleep 6
# The administrator's password is printed once, at the first start: keep it for demo-setup.sh.
if [ ! -s /etc/bitdispatcher/admin-password ]; then
  journalctl -u bitdispatcher --no-pager | grep -o 'with password  [^ ]*' | tail -1 | awk '{print $3}' > /etc/bitdispatcher/admin-password
  chmod 600 /etc/bitdispatcher/admin-password
fi
# The secrets for what is pushed to the demo (EIDOs, radio events, hooks): random, kept here, set in
# bitdispatcher by demo-setup.sh and sent by bvsim. A real BitVoice pushing here is given the same.
if [ ! -s /etc/bitdispatcher/push.env ]; then
  (umask 077; printf 'EIDO_TOKEN=bd_eido_%s\nRADIO_SECRET=bd_radio_%s\nHOOK_TOKEN=bd_hook_%s\n' \
    "$(openssl rand -hex 24)" "$(openssl rand -hex 24)" "$(openssl rand -hex 24)" > /etc/bitdispatcher/push.env)
fi
systemctl enable -q demo-reset.timer bvsim
systemctl start demo-reset.timer
systemctl restart bvsim caddy
systemctl is-active bitdispatcher bvsim caddy
