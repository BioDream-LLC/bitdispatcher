#!/bin/sh
# Pins the simulated BitVoice's certificate, keeps the public demo account, and on an empty board creates a
# morning's incidents so a visitor finds work in every state.
set -eu
B=https://127.0.0.1:8443/api
PW=$(cat /etc/bitdispatcher/admin-password)
. /etc/bitdispatcher/push.env
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
api PUT /admin/integrations "{\"bitvoiceUrl\":\"https://127.0.0.1:9444\",\"bitvoiceFingerprint\":\"$FP\",\"aliListen\":\"127.0.0.1:5001\",\"eidoToken\":\"$EIDO_TOKEN\",\"radioSecret\":\"$RADIO_SECRET\"}" >/dev/null
echo "simulated BitVoice pinned at $FP"
# The hooks the simulator sends crash data and drone reports to, its drone system, and the demo's
# non-emergency line and support number.
# The BitVoice demo's radio crews, matched to the demonstration's fire units by their radio names
# (the simulated radio uses the medics and police units, by radio ID).
for f in 'E1|Engine 1' 'E2|Engine 2' 'L1|Ladder 1' 'BC1|Battalion 1' 'A5|Ambulance 5'; do
  IFS='|' read -r id alias <<EOF2
$f
EOF2
  u=$(api GET /state | jq -c --arg id "$id" --arg a "$alias" '.units | (if type == "object" then .[] else .[] end) | select(.id == $id)
    | {id, name, type, station, capabilities, radioId, agency, area, radioAlias: $a}')
  [ -n "$u" ] && api PUT "/admin/units/$id" "$u" >/dev/null || true
done
# Views of a place from the side: open street photos (Panoramax) and the Street View link. EagleView needs an
# agency's own licence, so the demo has none.
api PUT /admin/views '{"panoramaxOn":true,"googleLink":true}' >/dev/null
api PUT /admin/connections "{\"hookToken\":\"$HOOK_TOKEN\",\"droneUrl\":\"http://127.0.0.1:9445/launch\",\"droneToken\":\"bd_drone_demo\",\"droneAuto\":true,\"stationAlerts\":true}" >/dev/null
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
# Call types and settings the roadmap features use, for a board started before they existed (a new board
# has them from the defaults): further alarms and roll calls on structure fires, the scheduled, tow and
# paper types, and minimum staffing on unit types.
CT=$(api GET /state | jq -c '.callTypes')
if [ "$(printf '%s' "$CT" | jq '[.[] | select(.code == "SF" and ((.alarms // []) | length) > 0)] | length')" = "0" ]; then
  api PUT /admin/calltypes/SF "$(printf '%s' "$CT" | jq -c '.[] | select(.code == "SF") | .par = true | .alarms = [{"name":"2nd alarm","runCard":[{"capability":"engine","count":2},{"capability":"ladder","count":1},{"capability":"command","count":1},{"capability":"als","count":1}]},{"name":"3rd alarm","runCard":[{"capability":"engine","count":2},{"capability":"ladder","count":1},{"capability":"water","count":1}]}]')" >/dev/null || true
  api PUT /admin/calltypes/HAZ "$(printf '%s' "$CT" | jq -c '.[] | select(.code == "HAZ") | .par = true')" >/dev/null || true
fi
for t in 'EVENT|Special event / standby|5|other|[]' 'TRANSFER|Scheduled transport|4|ems|[{"capability":"transport","count":1}]' 'TOW|Tow request / disabled vehicle|4|police|[{"capability":"police","count":1}]' 'PAPER|Paper service / warrant service|4|police|[{"capability":"police","count":1}]'; do
  IFS='|' read -r code name pri disc rc <<EOF3
$t
EOF3
  if [ "$(printf '%s' "$CT" | jq --arg c "$code" '[.[] | select(.code == $c)] | length')" = "0" ]; then
    api PUT "/admin/calltypes/$code" "{\"name\":\"$name\",\"priority\":$pri,\"discipline\":\"$disc\",\"runCard\":$rc}" >/dev/null || true
  fi
done
S=$(api GET /state | jq -c '.settings')
if [ "$(printf '%s' "$S" | jq '[.unitTypes[] | select(.minStaff)] | length')" = "0" ]; then
  api PUT /admin/settings "$(printf '%s' "$S" | jq -c '.unitTypes |= map(if .code == "engine" or .code == "ladder" then .minStaff = 3 elif .code == "medic" or .code == "ambulance" then .minStaff = 2 else . end)')" >/dev/null || true
fi

# The roadmap features, seeded once per board (the nightly reset empties it): a roster with crews, a
# simulated pager (bvsim's, on this computer: its responders answer pages), tow companies, the contacts
# directory and an on-call list, two areas, papers to serve and the schedule. Every name is invented.
if [ "$(api GET /roster | jq '.members | length')" = "0" ]; then
  m() { api PUT /admin/members "$1" >/dev/null || true; }
  m '{"id":"1101","name":"Avery Brooks","rank":"Captain","quals":["officer","emt"],"station":"ST1"}'
  m '{"id":"1102","name":"Jordan Ellis","rank":"Driver","quals":["driver","emt"],"station":"ST1"}'
  m '{"id":"1103","name":"Casey Morgan","rank":"Firefighter","quals":["firefighter","emt"],"station":"ST1"}'
  m '{"id":"1104","name":"Riley Shah","rank":"Paramedic","quals":["paramedic","driver"],"station":"ST1"}'
  m '{"id":"1105","name":"Quinn Park","rank":"EMT","quals":["emt","driver"],"station":"ST1"}'
  m '{"id":"1106","name":"Drew Patel","rank":"Lieutenant","quals":["officer","emt","technical-rescue"],"station":"ST1"}'
  m '{"id":"1107","name":"Sam Rivera","rank":"Driver","quals":["driver","emt"],"station":"ST1"}'
  m '{"id":"1108","name":"Morgan Lee","rank":"Firefighter","quals":["firefighter"],"station":"ST1"}'
  m '{"id":"3101","name":"Taylor Grant","rank":"Volunteer","quals":["firefighter","driver"],"station":"ST3","volunteer":true,"topic":"demo-3101"}'
  m '{"id":"3102","name":"Jamie Fox","rank":"Volunteer","quals":["emt"],"station":"ST3","volunteer":true,"topic":"demo-3102"}'
  m '{"id":"3103","name":"Robin Hale","rank":"Volunteer","quals":["firefighter","emt"],"station":"ST3","volunteer":true,"topic":"demo-3103"}'
  m '{"id":"3104","name":"Kendall Ruiz","rank":"Volunteer","quals":["paramedic"],"station":"ST3","volunteer":true,"topic":"demo-3104"}'
  m '{"id":"3105","name":"Parker Cole","rank":"Volunteer","quals":["firefighter"],"station":"ST3","volunteer":true,"topic":"demo-3105"}'
  api PUT /units/E1/crew '{"crew":[{"member":"1101","role":"officer"},{"member":"1102","role":"driver"},{"member":"1103","role":"firefighter"}]}' >/dev/null || true
  api PUT /units/M1/crew '{"crew":[{"member":"1104","role":"paramedic"},{"member":"1105","role":"emt"}]}' >/dev/null || true
  api PUT /units/L1/crew '{"crew":[{"member":"1106","role":"officer"},{"member":"1107","role":"driver"}]}' >/dev/null || true
  api PUT /admin/paging '{"publicUrl":"https://demo.bitdispatcher.com","gateways":[{"id":"sim","name":"Demo pager (simulated)","kind":"ntfy","url":"http://127.0.0.1:9446/demo"}]}' >/dev/null || true
  api PUT /admin/pagegroups '{"name":"Station 3 volunteers (simulated)","members":["3101","3102","3103","3104","3105"],"gateways":["sim"],"stations":["ST3"],"note":"Demo: a simulated pager; its simulated responders answer each page"}' >/dev/null || true
  t() { api PUT /admin/tows "$1" >/dev/null || true; }
  t '{"name":"Star City Towing (demo)","phone":"540-555-0111","classes":["light","medium"]}'
  t '{"name":"Blue Ridge Wrecker (demo)","phone":"540-555-0112","classes":["light","medium","heavy"]}'
  t '{"name":"Valley Recovery (demo)","phone":"540-555-0113","classes":["light","heavy"]}'
  d() { api PUT /directory "$1" | jq -r .id; }
  d '{"name":"Water department after hours (demo)","org":"City water (demo)","category":"utility","phones":[{"label":"24 h","number":"540-555-0150"}],"tags":["water main"]}' >/dev/null
  d '{"name":"Gas company emergency line (demo)","org":"Gas utility (demo)","category":"utility","phones":[{"label":"24 h","number":"540-555-0151"}],"types":["GAS"]}' >/dev/null
  d '{"name":"Power company outages (demo)","org":"Electric utility (demo)","category":"utility","phones":[{"label":"dispatch line","number":"540-555-0152"}],"types":["SF"]}' >/dev/null
  d '{"name":"Medical examiner (demo)","category":"medical","phones":[{"number":"540-555-0153"}],"tags":["ME"]}' >/dev/null
  d '{"name":"Poison control","category":"medical","phones":[{"label":"national","number":"1-800-222-1222"}],"types":["OD"]}' >/dev/null
  d '{"name":"Interpreter line (demo)","category":"interpreter","phones":[{"number":"540-555-0155"}]}' >/dev/null
  P1=$(d '{"name":"Alex Kim, public works (demo)","org":"Public works (demo)","category":"public works","phones":[{"label":"cell","number":"540-555-0161"}]}')
  P2=$(d '{"name":"Bailey Tran, public works (demo)","org":"Public works (demo)","category":"public works","phones":[{"label":"cell","number":"540-555-0162"}]}')
  NOW=$(date -u +%s)
  iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
  api PUT /oncall "{\"name\":\"Public works on call (demo)\",\"types\":[\"GAS\",\"OTHER\"],\"backup\":\"$P1\",\"shifts\":[{\"entry\":\"$P1\",\"from\":\"$(iso $((NOW-43200)))\",\"to\":\"$(iso $((NOW+43200)))\"},{\"entry\":\"$P2\",\"from\":\"$(iso $((NOW+43200)))\",\"to\":\"$(iso $((NOW+129600)))\"}]}" >/dev/null || true
  api PUT /admin/geofences '{"name":"School zone (demo)","kind":"caution","note":"School zone: children crossing 07:30-08:30 and 14:30-15:30; no sirens near the school (demo area)","ring":[[-79.9545,37.2650],[-79.9465,37.2650],[-79.9465,37.2700],[-79.9545,37.2700]]}' >/dev/null || true
  api PUT /admin/geofences '{"name":"Hospital ambulance bay (demo)","kind":"alert","enter":true,"disciplines":["ems"],"ring":[[-79.9440,37.2490],[-79.9395,37.2490],[-79.9395,37.2515],[-79.9440,37.2515]]}' >/dev/null || true
  p() { api PUT /papers "$1" >/dev/null || true; }
  p '{"kind":"arrest","number":"GD26-0042 (demo)","court":"General District Court (demo)","subject":{"name":"Casey Q Example","dob":"1991-03-14","sex":"M","description":"Fictional person for the demonstration"},"location":{"address":"4333 Old Cave Spring Rd"},"charge":"Failure to appear (demo)","caution":"known to resist (demo)"}'
  p '{"kind":"protective-order","number":"JD26-0107 (demo)","court":"Juvenile and Domestic Relations Court (demo)","subject":{"name":"Robin Z Sample","sex":"F"},"location":{"address":"526 Orange Ave NE"},"charge":"Serve the respondent (demo)"}'
  p '{"kind":"summons","number":"CV26-0310 (demo)","court":"Circuit Court (demo)","subject":{"name":"Taylor X Placeholder"},"location":{"address":"1906 Belleview Ave SE"},"charge":"Civil summons (demo)"}'
  echo "seeded the roster, paging, tows, directory, areas, papers and schedule"
fi
if [ "$(api GET /schedule | jq '.items | length')" = "0" ]; then
  TOMORROW=$(date -u -d '+2 hour' +%Y-%m-%dT%H:00:00Z)
  TODAY10=$(date -u -d 'today 14:00' +%Y-%m-%dT%H:%M:%SZ)
  api PUT /schedule "{\"kind\":\"call\",\"title\":\"Dialysis transport (demo)\",\"type\":\"TRANSFER\",\"location\":{\"address\":\"1906 Belleview Ave SE\"},\"start\":\"$TOMORROW\",\"lead\":15,\"repeat\":\"daily\",\"notes\":\"Wheelchair; to the dialysis centre (demo)\"}" >/dev/null || true
  api PUT /schedule "{\"kind\":\"event\",\"title\":\"City market festival (demo)\",\"type\":\"EVENT\",\"location\":{\"address\":\"Market Square, Roanoke\"},\"start\":\"$TODAY10\",\"minutes\":480,\"repeat\":\"daily\",\"notes\":\"Street closures on Market St and Campbell Ave (demo)\"}" >/dev/null || true
  echo "seeded the schedule"
fi
# Public alerts, seeded once: the settings (the public feed stays off on the demo, so nothing visitors write
# is published), a simulated siren controller, and residents reached by the simulated pager.
if [ "$(api GET /admin/alerting | jq -r '.sender')" = "" ]; then
  api PUT /admin/alerting '{"sender":"alerts@demo.bitdispatcher.com","senderName":"Roanoke Valley Demo 911 (demo data)","jurisdiction":"Roanoke Valley (demo)","geocodes":["051770","051161","051775"],"timeZone":"America/New_York","feed":false,"twoPerson":false,"receivers":[{"id":"siren","name":"Siren controller (simulated)","url":"http://127.0.0.1:9446/cap/sirens"}]}' >/dev/null || true
  api PUT /admin/alertlists '{"id":"","name":"Residents (simulated)","gateways":["sim"],"note":"Demo: fictional people reached by the simulated pager","people":[
    {"name":"Resident downtown (demo)","topic":"res-downtown","lat":37.2715,"lon":-79.9410},
    {"name":"Resident Old Southwest (demo)","topic":"res-osw","lat":37.2640,"lon":-79.9500},
    {"name":"Resident Williamson Rd (demo)","topic":"res-wmsn","lat":37.2990,"lon":-79.9330},
    {"name":"Residente en el centro (demo)","topic":"res-es","lang":"es","lat":37.2725,"lon":-79.9395},
    {"name":"Resident Salem (demo)","topic":"res-salem","lat":37.2935,"lon":-80.0548},
    {"name":"Resident anywhere (demo)","topic":"res-all"}]}' >/dev/null || true
  echo "seeded public alerts"
fi
# The operational updates, seeded once: the agencies the centre dispatches for (units given theirs), the
# demo user's usual coverage, qualifications and credentials, a staffing minimum, the classic keys for the
# police desk, and a BOLO in the inbox. All fictional demo data.
if [ "$(api GET /agencies | jq '.agencies | length')" = "0" ]; then
  ag() { api PUT "/admin/agencies/$1" "$2" >/dev/null || true; }
  ag DISP '{"name":"Emergency communications centre (demo)","disciplines":["dispatch"]}'
  ag RPD '{"name":"City police (demo)","disciplines":["police"]}'
  ag RCPD '{"name":"County police (demo)","disciplines":["police"]}'
  ag SPD '{"name":"Salem police (demo)","disciplines":["police"]}'
  ag RFD '{"name":"City fire and EMS (demo)","disciplines":["fire","ems"]}'
  ag RCFR '{"name":"County fire and rescue (demo)","disciplines":["fire","ems"]}'
  ag SFD '{"name":"Salem fire and EMS (demo)","disciplines":["fire","ems"]}'
  ag RPW '{"name":"Public works (demo)","disciplines":["other"]}'
  api GET /state | jq -c '.settings.unitTypes as $t | .units[] | . as $u | ($t[] | select(.code == $u.type) | .discipline) as $d
    | {id, name, type, station, capabilities, radioId, radioAlias, area, minStaff, agency: (if $d == "police" then "RPD" elif $d == "fire" or $d == "ems" then "RFD" else "" end)}' |
  while read -r u; do api PUT "/admin/units/$(printf '%s' "$u" | jq -r .id)" "$u" >/dev/null || true; done
  api PUT /admin/staff/demo '{"home":"DISP","covered":["RPD","RFD"],"monitor":["RCPD","RCFR"]}' >/dev/null || true
  q() { api PUT "/admin/quals/$1" "$2" >/dev/null || true; }
  q hazmat-ops '{"name":"HazMat operations","category":"credential","months":36}'
  q hazmat-tech '{"name":"HazMat technician","category":"team","months":24,"renewDays":60,"prereqs":["hazmat-ops"],"verify":true,"levels":["Technician","Team Leader"]}'
  q swift-water '{"name":"Swift water rescue","category":"team","months":12,"verify":true}'
  q fto '{"name":"Field training officer","category":"training"}'
  q evidence-tech '{"name":"Evidence technician","category":"specialty"}'
  NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ); YAGO=$(date -u -d '-1 year' +%Y-%m-%dT%H:%M:%SZ); SOON=$(date -u -d '-23 months' +%Y-%m-%dT%H:%M:%SZ)
  cr() { api PUT "/members/$1/credentials/$2" "$3" >/dev/null || true; }
  cr 1106 hazmat-ops "{\"issued\":\"$YAGO\",\"verify\":true}"
  cr 1106 hazmat-tech "{\"level\":\"Technician\",\"issued\":\"$YAGO\",\"issuer\":\"State fire programs (demo)\",\"verify\":true}"
  cr 1103 hazmat-ops "{\"issued\":\"$YAGO\",\"verify\":true}"
  cr 1103 hazmat-tech "{\"level\":\"Technician\",\"issued\":\"$SOON\",\"issuer\":\"State fire programs (demo)\",\"verify\":true}"
  cr 1107 swift-water "{\"issued\":\"$YAGO\",\"verify\":false}"
  S=$(api GET /state | jq -c '.settings | .staffing = {"short":"limited","dispatch":"warn","minimums":[{"qual":"hazmat-tech","count":2,"note":"Demo: two HazMat technicians on duty"}]}
    | .commands = ((.commands // {}) | .keys = {"police":"classic"})')
  api PUT /admin/settings "$S" >/dev/null || true
  for m in 1101 1102 1103 1104 1105 1106 1107; do api POST "/members/$m/on" '{"kind":"regular","shift":"A"}' >/dev/null || true; done
  api POST /comms '{"kind":"bolo","priority":"medium","subject":"(demo) Stolen vehicle: silver Honda Civic","body":"Fictional demo BOLO: silver Honda Civic, VA plate DEMO123, taken from Market St this morning. Do not stop on this alone; confirm first.","plate":"DEMO123","plateState":"VA","vehicle":"silver Honda Civic","agency":"RPD"}' >/dev/null || true
  echo "seeded agencies, qualifications, staffing and a BOLO"
fi
# Mark-outs with a longest time, so a long one is shown OVER and told to supervisors (once).
if [ "$(api GET /state | jq '[.settings.markOuts[] | select(.maxMinutes)] | length')" = "0" ]; then
  S=$(api GET /state | jq -c '.settings | .markOuts = [.markOuts[] | if .code == "MEAL" then .maxMinutes = 45 elif .code == "FUEL" then .maxMinutes = 20 else . end]')
  api PUT /admin/settings "$S" >/dev/null || true
fi
# The final round, seeded once: operational periods in the demo's time zone, critical-incident rules and
# alert phrases (live, so visitors see an alert), and two flags at a demo address (one for the whole
# complex, one for a single apartment).
if [ "$(api GET /state | jq -r '.settings.shift.timeZone // ""')" = "" ]; then
  EX=$(api GET /admin/phrases/examples)
  S=$(api GET /state | jq -c --argjson ex "$EX" '.settings
    | .shift = {"timeZone":"America/New_York","periods":[{"name":"Day","start":"07:00"},{"name":"Night","start":"19:00"}]}
    | .critical = {"version":1,"phrases":[$ex.phrases[] | .testing = false],"rules":[
        {"id":"r-sf","name":"Structure fire","kind":"calltype","match":"SF"},
        {"id":"r-p1","name":"Priority 1","kind":"priority","match":"1"},
        {"id":"r-em","name":"Unit emergency","kind":"event","match":"unit-emergency"},
        {"id":"r-shots","name":"Shots fired heard","kind":"phrase","match":"shots-fired"},
        {"id":"r-down","name":"Officer down heard","kind":"phrase","match":"officer-down"}]}')
  api PUT /admin/settings "$S" >/dev/null || true
  api PUT /flags '{"category":"hazard","title":"Oxygen in use in the leasing office","warning":"Demo: home oxygen tanks stored in the leasing office","instruction":"No open flame; ventilate before entry","severity":"high","source":"demo data","scope":{"address":"1210 WILLIAMSON RD NE","lat":37.2885,"lon":-79.9395,"verified":true,"complex":"Williamson Court (demo)"},"conflictOk":true}' >/dev/null || true
  api PUT /flags '{"category":"dangerous-animal","title":"Two large dogs in apartment 204","warning":"Demo: two large dogs, aggressive with strangers","instruction":"Ask the caller to secure the dogs before entry","severity":"high","source":"demo data","scope":{"address":"1210 WILLIAMSON RD NE","lat":37.2885,"lon":-79.9395,"verified":true,"complex":"Williamson Court (demo)","building":"B","unit":"204"},"conflictOk":true}' >/dev/null || true
  echo "seeded shift periods, critical rules and phrases, and demo flags"
fi
# Continuity, seeded once: two internet checks, a relocation checklist and contacts, the continuity pack
# every 15 minutes. Map updates stay off on the demo (a rebuild of the 100-mile map is heavy for it).
if [ "$(api GET /state | jq '.settings.continuity.checks // [] | length')" = "0" ]; then
  S=$(api GET /state | jq -c '.settings | .continuity = {"checks":[{"name":"OpenStreetMap downloads","url":"https://download.geofabrik.de/"},{"name":"BitVoice demo","url":"https://demo.bitvoice.org/"}],
    "steps":["Call the 911 authority to reroute 911 to the backup site","Sign in at the backup site consoles","Check the radio control stations at the backup site","Tell field units by radio: dispatch is at the backup site","Post a supervisor at the main centre door"],
    "contacts":["Backup site (demo): 540-555-0199","911 authority (demo): 540-555-0142","Neighbouring centre (demo): 540-555-0188"],"packMinutes":15}')
  api PUT /admin/settings "$S" >/dev/null || true
  echo "seeded continuity checks, checklist and contacts"
fi
# No full-width banners that must be acknowledged: a shared demo has nobody to acknowledge them, and they
# covered the screen. Urgent and high messages still sound once and show in the inbox. (Radio emergencies are
# acknowledged by demo-tidy.timer.) Every start, so it also applies to a board seeded before this was added.
S=$(api GET /state | jq -c '.settings | .priorityRules = [
  {"level":"urgent","tone":"alarm","silenceable":true},{"level":"high","tone":"alert","silenceable":true},
  {"level":"medium","tone":"chime","silenceable":true},{"level":"low","tone":"tick","silenceable":true}]')
api PUT /admin/settings "$S" >/dev/null || true
api POST /auth/logout '{}' >/dev/null || true
echo "demo ready"
