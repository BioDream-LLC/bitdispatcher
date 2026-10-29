#!/bin/sh
# Removes bitdispatcher installed by install.sh: the service, the program and its documentation.
#   sudo ./uninstall.sh            keeps the records and /etc/default/bitdispatcher
#   sudo ./uninstall.sh --purge    also deletes them, after asking twice
set -e
[ "$(id -u)" = 0 ] || { echo "Run as root: sudo $0 $*" >&2; exit 1; }
DATA=/var/lib/bitdispatcher
[ -r /etc/default/bitdispatcher ] && . /etc/default/bitdispatcher
DATA=${BITDISPATCHER_DATA:-$DATA}

systemctl disable --now bitdispatcher.service >/dev/null 2>&1 || true
rm -f /etc/systemd/system/bitdispatcher.service /usr/bin/bitdispatcher
rm -rf /usr/share/doc/bitdispatcher
systemctl daemon-reload

if [ "$1" = "--purge" ] && [ -d "$DATA" ]; then
  printf 'Delete the records in %s, including the ledger of every call and dispatch? Type DELETE: ' "$DATA"
  read -r a || a=""
  printf 'This cannot be undone. Type DELETE again: '
  read -r b || b=""
  if [ "$a" = DELETE ] && [ "$b" = DELETE ]; then
    rm -rf "$DATA" /etc/default/bitdispatcher
    userdel bitdispatcher 2>/dev/null || true
    echo "Deleted the records and the bitdispatcher user."
    exit 0
  fi
  echo "Kept the records."
fi
echo "bitdispatcher removed. The records are kept in $DATA; delete them only under your retention rules."
