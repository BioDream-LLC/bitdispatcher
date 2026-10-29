#!/bin/sh
# Installs bitdispatcher from this folder (the Linux download) as a systemd service.
#   sudo ./install.sh [--port 8443] [--open-firewall]
# Run again to upgrade: the data, settings and users are kept. Remove with: sudo ./uninstall.sh
# The .deb and .rpm packages do the same through the package manager.
set -e
cd "$(dirname "$0")"
PORT=""
FIREWALL=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --port=*) PORT="${1#*=}"; shift ;;
    --open-firewall) FIREWALL=1; shift ;;
    -h|--help) sed -n '2,5p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
[ "$(id -u)" = 0 ] || { echo "Run as root: sudo $0 $*" >&2; exit 1; }
[ -d /run/systemd/system ] || { echo "This installer needs systemd. Without it, run ./bitdispatcher directly." >&2; exit 1; }
case "$PORT" in
  ""|*[!0-9]*) [ -z "$PORT" ] || { echo "--port must be a number" >&2; exit 2; } ;;
  *) [ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] || { echo "--port must be 1 to 65535" >&2; exit 2; } ;;
esac

install -m 0755 bitdispatcher /usr/bin/bitdispatcher
install -m 0644 bitdispatcher.service /etc/systemd/system/bitdispatcher.service
# The settings file is kept on an upgrade; only --port changes it.
[ -f /etc/default/bitdispatcher ] || install -m 0644 bitdispatcher.default /etc/default/bitdispatcher
if [ -n "$PORT" ]; then
  sed -i "s/^BITDISPATCHER_ADDR=.*/BITDISPATCHER_ADDR=:$PORT/" /etc/default/bitdispatcher
fi
install -d -m 0755 /usr/share/doc/bitdispatcher
install -m 0644 LICENSE /usr/share/doc/bitdispatcher/LICENSE
[ -f bitdispatcher-manual.pdf ] && install -m 0644 bitdispatcher-manual.pdf /usr/share/doc/bitdispatcher/

sh ./postinstall.sh

. /etc/default/bitdispatcher
P=${BITDISPATCHER_ADDR##*:}
if [ "$FIREWALL" = 1 ]; then
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    ufw allow "$P/tcp" comment bitdispatcher >/dev/null && echo "Opened $P/tcp in ufw."
  elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd -q --permanent --add-port="$P/tcp" && firewall-cmd -q --reload && echo "Opened $P/tcp in firewalld."
  else
    echo "No active ufw or firewalld found; open $P/tcp in your firewall."
  fi
fi
