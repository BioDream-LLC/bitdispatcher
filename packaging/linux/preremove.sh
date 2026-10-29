#!/bin/sh
# Run by the .deb and .rpm packages before removing files. On an upgrade it does nothing (postinstall
# restarts the service); on removal it stops and disables the service. The data is always kept.
set -e
case "$1" in
  upgrade|1) exit 0 ;; # deb: upgrade; rpm: 1 = an upgrade is replacing this version
esac
if [ -d /run/systemd/system ]; then
  systemctl stop bitdispatcher.service >/dev/null 2>&1 || true
  systemctl disable bitdispatcher.service >/dev/null 2>&1 || true
fi
echo "bitdispatcher: removed. The records are kept in /var/lib/bitdispatcher (or BITDISPATCHER_DATA); delete them only under your retention rules."
exit 0
