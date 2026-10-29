#!/bin/sh
# Removes bitdispatcher from this Mac:  sudo /usr/local/bitdispatcher/uninstall.sh [--purge]
# The records (ledger, users, settings, recorded map), the settings file and the logs are kept unless
# --purge is given and DELETE is typed twice. The Uninstall app asks the same twice in dialogs, then
# runs this with --purge-confirmed.
set -e
PATH=/usr/bin:/bin:/usr/sbin:/sbin
[ "$(id -u)" = 0 ] || { echo "Run it as an administrator: sudo $0 $*" >&2; exit 1; }
LABEL=com.biodream.bitdispatcher
CONF=/usr/local/etc/bitdispatcher.conf
BITDISPATCHER_DATA="/Library/Application Support/bitdispatcher"
[ -r "$CONF" ] && . "$CONF"
DATA=$BITDISPATCHER_DATA

PURGE=no
case "${1:-}" in
  --purge)
    printf 'Delete the records in %s too? This includes the ledger of every call and dispatch.\nType DELETE to delete them, anything else to keep them: ' "$DATA"
    read -r a || a=""
    if [ "$a" = DELETE ]; then
      printf 'They cannot be recovered. Type DELETE again: '
      read -r a || a=""
      [ "$a" = DELETE ] && PURGE=yes
    fi ;;
  --purge-confirmed) PURGE=yes ;;
  ""|--keep) ;;
  *) echo "usage: $0 [--purge]" >&2; exit 2 ;;
esac

launchctl bootout "system/$LABEL" 2>/dev/null || true
FW=/usr/libexec/ApplicationFirewall/socketfilterfw
[ -x "$FW" ] && "$FW" --remove /usr/local/bitdispatcher/bin/bitdispatcher >/dev/null 2>&1 || true
rm -f "/Library/LaunchDaemons/$LABEL.plist"
[ "$(readlink /usr/local/bin/bitdispatcher 2>/dev/null)" = /usr/local/bitdispatcher/bin/bitdispatcher ] && rm -f /usr/local/bin/bitdispatcher
rm -rf /Applications/bitdispatcher /usr/local/bitdispatcher
for p in com.biodream.bitdispatcher com.biodream.bitdispatcher.launchd com.biodream.bitdispatcher.apps com.biodream.bitdispatcher.setup; do
  pkgutil --forget "$p" >/dev/null 2>&1 || true
done

if [ "$PURGE" = yes ]; then
  rm -rf "$DATA" /Library/Logs/bitdispatcher
  rm -f "$CONF" /usr/local/etc/bitdispatcher.first-sign-in
  dscl . -delete /Users/_bitdispatcher 2>/dev/null || true
  dscl . -delete /Groups/_bitdispatcher 2>/dev/null || true
  echo "bitdispatcher removed, with its records."
  # macOS 26 can refuse to delete an account even for an administrator with Full Disk Access.
  if dscl . -read /Users/_bitdispatcher >/dev/null 2>&1; then
    echo "macOS did not allow removing the hidden service account _bitdispatcher. It has no password and no"
    echo "shell and owns nothing now; installing bitdispatcher again reuses it."
  fi
else
  echo "bitdispatcher removed. The records are kept in $DATA; delete them only under your retention rules."
fi
