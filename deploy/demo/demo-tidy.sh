#!/bin/sh
# Every 20 seconds on the public demo: acknowledge, in bitdispatcher only, the radio emergencies the simulator
# and the BitVoice demo's crews raise, so their red bar does not cover the screen of a demo nobody staffs.
# The command line's ACK is used, which does not acknowledge on the BitVoice demo's own console.
set -eu
B=https://127.0.0.1:8443/api
PW=$(cat /etc/bitdispatcher/admin-password)
J=$(mktemp)
trap 'rm -f "$J"' EXIT
curl -skf -c "$J" -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\",\"console\":\"TIDY\",\"duty\":\"admin\"}" "$B/auth/login" >/dev/null || exit 0
for i in 1 2 3 4 5 6 7 8 9 10; do
  r=$(curl -skf -b "$J" -X POST -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' -d '{"line":"ACK"}' "$B/command" || true)
  echo "$r" | grep -q '"ok":true' || break
done
curl -skf -b "$J" -X POST -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' -d '{}' "$B/auth/logout" >/dev/null || true
