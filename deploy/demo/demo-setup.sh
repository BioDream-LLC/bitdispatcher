#!/bin/sh
# Pins the simulated BitVoice's certificate, keeps the public demo account, and on an empty board creates a
# morning's incidents so a visitor finds work in every state.
set -eu
B=https://127.0.0.1:8443/api
PW=$(cat /etc/bitdispatcher/admin-password)
J=$(mktemp)
trap 'rm -f "$J"' EXIT
for i in $(seq 1 60); do curl -skf "$B/health" >/dev/null 2>&1 && break; sleep 1; done
for i in $(seq 1 30); do curl -sk https://127.0.0.1:9444/ -o /dev/null 2>&1 && break; sleep 1; done
curl -skf -c "$J" -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\",\"console\":\"SETUP\"}" "$B/auth/login" >/dev/null
api() { curl -skf -b "$J" -X "$1" -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' ${3:+-d "$3"} "$B$2"; }
api PUT /admin/users/demo '{"name":"Demo dispatcher","role":"dispatcher","password":"try-bitdispatcher-demo","disabled":false,"demo":true}' >/dev/null
FP=$(api POST /admin/integrations/bitvoice/fingerprint '{"url":"https://127.0.0.1:9444"}' | jq -r .fingerprint)
api PUT /admin/integrations "{\"bitvoiceUrl\":\"https://127.0.0.1:9444\",\"bitvoiceFingerprint\":\"$FP\",\"aliListen\":\"127.0.0.1:5001\"}" >/dev/null
echo "simulated BitVoice pinned at $FP"

if [ "$(api GET /state | jq '[.incidents[] | select(.status != "closed")] | length')" = "0" ]; then
  cmd() { api POST /command "$(jq -nc --arg l "$1" --arg i "${2:-}" '{line: $l, incident: $i}')"; }
  num() { jq -r '.incident // empty'; }
  SF=$(cmd 'NEW SF 2001 Crystal Spring Ave SW' | num)
  cmd 'REC' "$SF" >/dev/null
  cmd 'PN Caller reports smoke from the second floor; occupant may be inside' "$SF" >/dev/null
  SICK=$(cmd 'NEW SICK 526 Orange Ave NE' | num)
  cmd 'D 2A11' "$SICK" >/dev/null
  cmd 'OS 2A11' "$SICK" >/dev/null
  cmd 'NEW FA 2102 Grandin Rd SW' >/dev/null
  cmd 'NEW FALL 1906 Belleview Ave SE' >/dev/null
  cmd 'NEW DOM 4333 Old Cave Spring Rd #B' >/dev/null
  echo "seeded the demonstration incidents"
fi
api POST /auth/logout '{}' >/dev/null || true
echo "demo ready"
