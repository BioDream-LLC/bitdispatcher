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
  -d "{\"username\":\"admin\",\"password\":\"$PW\",\"console\":\"SETUP\",\"duty\":\"admin\"}" "$B/auth/login" >/dev/null
api() { curl -skf -b "$J" -X "$1" -H 'X-BitDispatcher: 1' -H 'Content-Type: application/json' ${3:+-d "$3"} "$B$2"; }
api PUT /admin/users/demo '{"name":"Demo user","role":"supervisor","password":"try-bitdispatcher-demo","disabled":false,"demo":true}' >/dev/null
# The demo user may work every duty but administration, so a visitor can try each workspace.
api PUT /admin/users/demo/duties '{"duties":["dispatcher","calltaker","police","fire","ems","supervisor","records"]}' >/dev/null
FP=$(api POST /admin/integrations/bitvoice/fingerprint '{"url":"https://127.0.0.1:9444"}' | jq -r .fingerprint)
api PUT /admin/integrations "{\"bitvoiceUrl\":\"https://127.0.0.1:9444\",\"bitvoiceFingerprint\":\"$FP\",\"aliListen\":\"127.0.0.1:5001\"}" >/dev/null
echo "simulated BitVoice pinned at $FP"
# The hooks the simulator sends crash data and drone reports to, its drone system, and the demo's
# non-emergency line and support number.
# Views of a place from the side: open street photos (Panoramax) and the Street View link. EagleView needs an
# agency's own licence, so the demo has none.
api PUT /admin/views '{"panoramaxOn":true,"googleLink":true}' >/dev/null
api PUT /admin/connections '{"hookToken":"bd_hook_demo","droneUrl":"http://127.0.0.1:9445/launch","droneToken":"bd_drone_demo","droneAuto":true,"stationAlerts":true}' >/dev/null
S=$(api GET /state | jq -c '.settings | .nonEmergencyLines = ["+15408532121"] | .supportLine = "Employee assistance: 540-555-0199 (demo number)"
  | .talkgroups = ["FIRE-DISP","FIRE-OPS2","EMS-1","PD-MAIN","PD-TAC3"]')
api PUT /admin/settings "$S" >/dev/null
# Example configuration for the newer workspaces: a response plan, receiving facilities, the example
# guidance (approved here, for the demo only), and the sample disclosure rules (enabled here, for the
# demo only: an agency's counsel reviews its own).
api PUT /admin/plans/RP1 '{"name":"Station 1 structure fire (example)","callTypes":["SF"],"area":"ST1","order":["E1","L1","E2","BC1"],"note":"Example plan for the demonstration"}' >/dev/null || true
for f in 'CARILION|Carilion Roanoke Memorial Hospital|trauma|37.2502|-79.9417' 'LEWISGALE|LewisGale Medical Center|hospital|37.2867|-80.0467' 'VAMC|Salem VA Medical Center|hospital|37.2731|-80.0265'; do
  IFS='|' read -r id name kind lat lon <<EOF2
$f
EOF2
  api PUT "/admin/facilities/$id" "{\"name\":\"$name (demo)\",\"kind\":\"$kind\",\"lat\":$lat,\"lon\":$lon,\"status\":\"open\"}" >/dev/null || true
done
api GET /admin/guidance | jq -c '.examples[] | .approved = true' | while read -r d; do
  id=$(printf '%s' "$d" | jq -r .id); api PUT "/admin/guidance/$id" "$d" >/dev/null || true
done
api GET /admin/foia/rules | jq -c '.samples[] | .enabled = true' | while read -r r; do
  id=$(printf '%s' "$r" | jq -r .id); api PUT "/admin/foia/rules/$id" "$r" >/dev/null || true
done

if [ "$(api GET /state | jq '[.incidents[] | select(.status != "closed")] | length')" = "0" ]; then
  cmd() { api POST /command "$(jq -nc --arg l "$1" --arg i "${2:-}" '{line: $l, incident: $i}')"; }
  num() { jq -r '.incident // empty'; }
  SF=$(cmd 'NEW SF 2001 Crystal Spring Ave SW' | num)
  cmd 'REC' "$SF" >/dev/null
  cmd 'PN Caller reports smoke from the second floor; occupant may be inside' "$SF" >/dev/null
  SICK=$(cmd 'NEW SICK 526 Orange Ave NE' | num)
  cmd 'D 2A11 / first unit on scene' "$SICK" >/dev/null
  cmd 'OS 2A11' "$SICK" >/dev/null
  cmd 'NEW FA 2102 Grandin Rd SW' >/dev/null
  cmd 'NEW FALL 1906 Belleview Ave SE' >/dev/null
  cmd 'NEW DOM 4333 Old Cave Spring Rd #B' >/dev/null
  echo "seeded the demonstration incidents"
fi
api POST /auth/logout '{}' >/dev/null || true
echo "demo ready"
