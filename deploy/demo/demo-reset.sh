#!/bin/sh
# Every night: back to the demonstration data. The recorded map, county GIS, aerial photos, users and
# integrations stay; the ledger (every incident, call and status) starts again.
set -eu
systemctl stop bvsim bitdispatcher
rm -f /var/lib/bitdispatcher/ledger.jsonl
systemctl start bitdispatcher bvsim
