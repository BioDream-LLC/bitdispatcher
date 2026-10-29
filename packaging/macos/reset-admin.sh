#!/bin/sh
# Sets a new generated password for the bitdispatcher user "admin" (Applications › bitdispatcher › Reset
# admin password, or: sudo /usr/local/bitdispatcher/reset-admin.sh). The service stops for a moment, then
# starts again. The password is printed here only; it is not saved anywhere.
set -e
PATH=/usr/bin:/bin:/usr/sbin:/sbin
[ "$(id -u)" = 0 ] || { echo "Run it as an administrator: sudo $0" >&2; exit 1; }
LABEL=com.biodream.bitdispatcher
PLIST=/Library/LaunchDaemons/$LABEL.plist
BITDISPATCHER_DATA="/Library/Application Support/bitdispatcher"
[ -r /usr/local/etc/bitdispatcher.conf ] && . /usr/local/etc/bitdispatcher.conf
launchctl bootout "system/$LABEL" 2>/dev/null || true
st=0
sudo -u _bitdispatcher /usr/local/bitdispatcher/bin/bitdispatcher reset-password -data "$BITDISPATCHER_DATA" admin || st=$?
launchctl bootstrap system "$PLIST"
exit $st
