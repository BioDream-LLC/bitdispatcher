"""Reads an IPAWS-style alert feed (or one CAP message) with EAS Station's own CAP poller parser and prints
what it makes of each alert: identifier, status, message type, event, SAME event code, severity, area,
county codes and whether it found the polygon. Exits 1 when it finds no alert. Run by
scripts/eas-station.sh; EAS Station (AGPL-3.0) stays outside bitdispatcher."""
import json
import logging
import os
import sys

sys.path.insert(0, os.environ["EAS_STATION"])
logging.basicConfig(level=logging.ERROR)
from poller.cap_geometry import _parse_ipaws_xml_feed, parse_cap_alert  # noqa: E402

log = logging.getLogger("eas")
text = open(sys.argv[1], encoding="utf-8").read()
if "gov.fema.ipaws.services/feed" not in text:  # a single message: wrap it as the feed does
    text = '<alerts xmlns="http://gov.fema.ipaws.services/feed">' + text.split("?>", 1)[-1] + "</alerts>"
found = 0
for feature in _parse_ipaws_xml_feed(text, log):
    a = parse_cap_alert(feature, log)
    if not a:
        print("NOT PARSED", json.dumps(feature.get("properties", {}).get("identifier")))
        continue
    found += 1
    p = feature.get("properties", {})
    print(json.dumps({
        "identifier": a.get("identifier"), "status": a.get("status"), "msgType": a.get("message_type"),
        "event": a.get("event"), "same": (p.get("eventCode") or {}).get("SAME"), "severity": a.get("severity"),
        "urgency": a.get("urgency"), "area": a.get("area_desc"), "geocode": p.get("geocode"),
        "polygon": bool(feature.get("geometry")), "expires": str(a.get("expires")), "headline": a.get("headline"),
    }, default=str, ensure_ascii=False))
sys.exit(0 if found else 1)
