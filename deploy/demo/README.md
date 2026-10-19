# The public demonstration

[demo.bitdispatcher.com](https://demo.bitdispatcher.com) (bitdispatcher.perfuse.health, where it started, still works): sign in as `demo` with password
`try-bitdispatcher-demo` (a dispatcher: everything on the console, no administration). Demonstration data
around Roanoke, Virginia; every 911 call, ALI record and radio event comes from the BitVoice simulator and is
marked SIMULATED. It goes back to the demonstration data every night at 09:00 UTC.

It runs on a Lightsail instance with 1 GB of memory (Debian 12): bitdispatcher on 127.0.0.1:8443, bvsim
beside it, and Caddy in front for the Let's Encrypt certificate. The 100-mile recorded map, Virginia's
address points and the aerial photos are recorded on a workstation and copied into `/var/lib/bitdispatcher`;
recording them on the instance would need more memory than it has. `install.sh` sets up the rest.
