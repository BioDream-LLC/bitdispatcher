# Supplemental location: RapidSOS and NG9-1-1 location servers

A wireless caller's ALI location is often a tower sector or a rough fix. A supplemental location source
knows where the phone itself is. bitdispatcher takes one in two ways; both end the same way: the
location becomes the call's newest, and on an incident the call-taker is offered it (**Use it** or
**Keep**). Nothing moves an incident without a person (docs/LOCATION-HOOK.md).

## 1. bitdispatcher asks: HELD

Admin › Integrations › Supplemental location. bitdispatcher sends a HELD location request (RFC 5985,
the NG9-1-1 i3 way to ask a location information server) naming the phone by its number (RFC 6155):

```
POST <HELD address>
Content-Type: application/held+xml;charset=utf-8
Authorization: Bearer <token>        (or Basic with the user and password)

<locationRequest xmlns="urn:ietf:params:xml:ns:geopriv:held" responseTime="emergencyDispatch">
  <locationType exact="false">geodetic civic</locationType>
  <device xmlns="urn:ietf:params:xml:ns:geopriv:held:id"><uri>tel:+15405550100</uri></device>
</locationRequest>
```

The answer's PIDF-LO (point, circle or polygon, civic address, method) is read; a HELD `error`
(`locationUnknown` and the rest) is shown to the call-taker as "no location".

- **Ask for every new 911 call** asks once when a call with a callback arrives (not texts or alerts).
- Otherwise the call-taker presses **Ask <name>** on the ANI / ALI panel; it can be pressed again for a
  newer fix.
- **Try a number** on the settings page asks without a call, to check the address and credentials.

## RapidSOS

RapidSOS describes its NG911 Clearinghouse as answering NENA i3 queries for centres, and gives each
centre its own access credentials (request them from RapidSOS). Its partner API documentation is behind
a partner login, so:

- bitdispatcher has **not been tested against RapidSOS**. The HELD client follows the RFCs and has been
  tested against a HELD server written to the RFCs.
- If RapidSOS gives your centre a HELD address and credentials, put them here and use **Try a number**
  with RapidSOS's test number.
- If RapidSOS's interface for your centre is not HELD, a small adapter can post its locations to the
  location hook (`/api/hooks/location`, docs/LOCATION-HOOK.md) with `"source": "rapidsos"`.
- RapidSOS Portal, its free browser tool, works beside any CAD and needs nothing from bitdispatcher.

## 2. Something else tells bitdispatcher: the location hook

A provider, a CPE, or BitVoice posts newer locations to `/api/hooks/location`. See docs/LOCATION-HOOK.md.

## Additional data

Device additional data (NENA i3 ADR: subscriber, service, device, and providers' medical or telematics
data) arrives today through BitVoice's EIDO and the ACN hook. Dereferencing ADR URIs from a location
server's answer is not done yet.
