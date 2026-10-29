#!/bin/sh
# Sets bitdispatcher up as a service after its files are in place: run by the .deb and .rpm packages
# after installing, and by install.sh. Safe to run again: an upgrade keeps the data, the settings and
# the users, and only restarts the service.
set -e

DATA=/var/lib/bitdispatcher
[ -r /etc/default/bitdispatcher ] && . /etc/default/bitdispatcher
DATA=${BITDISPATCHER_DATA:-$DATA}
ADDR=${BITDISPATCHER_ADDR:-:8443}
PORT=${ADDR##*:}

# A system user with no shell and no home of its own owns the data; the service runs as it.
if ! id bitdispatcher >/dev/null 2>&1; then
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --home-dir "$DATA" --no-create-home --shell /usr/sbin/nologin --user-group bitdispatcher
  else
    adduser --system --home "$DATA" --no-create-home --shell /usr/sbin/nologin --group bitdispatcher
  fi
fi
# Serial radios for drones (MAVLink) are in the dialout group.
getent group dialout >/dev/null 2>&1 && usermod -a -G dialout bitdispatcher 2>/dev/null || true

# The data holds criminal justice and personal information: the service user only.
install -d -o bitdispatcher -g bitdispatcher -m 0700 "$DATA"

# The first administrator is made here so its password is shown once, here, instead of in the log.
PW=$(runuser -u bitdispatcher -- /usr/bin/bitdispatcher init-admin -data "$DATA" 2>/dev/null || true)

if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable bitdispatcher.service >/dev/null 2>&1
  if systemctl is-active --quiet bitdispatcher.service; then
    systemctl restart bitdispatcher.service
  else
    if command -v ss >/dev/null 2>&1 && ss -Hltn "sport = :$PORT" 2>/dev/null | grep -q .; then
      echo "bitdispatcher: another program is using port $PORT. Set BITDISPATCHER_ADDR in /etc/default/bitdispatcher," >&2
      echo "  then: sudo systemctl start bitdispatcher" >&2
    else
      systemctl start bitdispatcher.service || echo "bitdispatcher: the service did not start; see: journalctl -u bitdispatcher" >&2
    fi
  fi
else
  echo "bitdispatcher: systemd is not running here; start it with: /usr/bin/bitdispatcher -data $DATA -addr $ADDR"
fi

HOST=$(hostname -f 2>/dev/null || hostname 2>/dev/null || cat /etc/hostname 2>/dev/null || echo localhost)
echo
echo "  bitdispatcher is installed. Consoles open https://$HOST:$PORT/"
if [ -n "$PW" ]; then
  echo
  echo "  First sign-in:  user  admin   password  $PW"
  echo "  It is shown only now. Change it in Admin > Users. If lost:"
  echo "    sudo systemctl stop bitdispatcher"
  echo "    sudo -u bitdispatcher bitdispatcher reset-password -data $DATA admin"
  echo "    sudo systemctl start bitdispatcher"
fi
echo
echo "  Settings: /etc/default/bitdispatcher. Log: journalctl -u bitdispatcher"
echo "  Other computers need port $PORT/tcp open in this server's firewall."
echo
exit 0
