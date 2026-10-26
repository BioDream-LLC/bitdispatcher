# Security scans

Free scanners run against bitdispatcher. This is not a penetration test or an audit; it is what
automated tools find. Repeat it before each release.

## 26 October 2026 (commit after 2fe7fba)

| Scanner | What it checks | Run against | Result |
|---|---|---|---|
| govulncheck | Known vulnerabilities in Go code that is actually called | source | none |
| OSV-Scanner | Known vulnerabilities in Go and web dependencies (go.mod, pnpm-lock.yaml) | source | none |
| Trivy (fs) | Dependency vulnerabilities, committed secrets, misconfiguration | source | none |
| gosec | Risky Go patterns | source | 584 hints, reviewed below |
| Mozilla HTTP Observatory | Security headers | bitdispatcher.com, demo.bitdispatcher.com | A+ (110), 12 of 12 |
| Qualys SSL Labs | TLS setup | bitdispatcher.com, demo.bitdispatcher.com | A+ on every address |
| OWASP ZAP baseline | Passive web checks | bitdispatcher.com, demo.bitdispatcher.com | 0 failures, 8 warnings |
| OWASP ZAP API scan (active) | Injection, errors, disclosure, on all 475 API operations, signed in as admin | a throwaway local server with demo data | 0 failures; two real findings, fixed |
| Signed-out sweep | Every API route without a session | the same local server | 446 refused (401); the rest are public by design |

**Fixed.** A missing critical review answered 500 instead of 404. An API path that is not served
(for example the training routes on the live server) answered 200 with the web page instead of a
JSON 404; scanners took those for open routes. Both are covered in `TestAPIFlow`.

**Reviewed and left.**
- gosec: unhandled errors (386), `math/rand` for non-secret choices such as simulator timing (44),
  file paths built from checked names (the GIS upload name is limited to `[A-Za-z0-9._-]` and a known
  extension), `InsecureSkipVerify` only where a SHA-256 certificate pin replaces it or where a
  certificate is read to show its fingerprint, MD5 only for change-file and SIP digest checks that
  the formats require, and SQL built only from constants in the map store.
- ZAP "SQL injection" on `/api/roster` and others: bitdispatcher's records are an event ledger, not
  SQL, and those routes ignore the parameter; the differing answers were the scan's own changes.
- ZAP warnings: `style-src 'unsafe-inline'` (inline styles from the map and charts; scripts stay
  `'self'`), no Permissions-Policy or Cross-Origin-Embedder-Policy header, the `10.0.0.5:7001`
  placeholder in the NCIC adapter field, and client addresses shown to admins in sign-in records.

**Not covered.** Access control between roles beyond the signed-out sweep and the Go tests, business
logic, the neighbour and BitVoice links under attack, denial of service, and the host. A human
penetration test is still needed.

## How to repeat

    make vuln
    go run github.com/securego/gosec/v2/cmd/gosec@latest ./...
    go run github.com/google/osv-scanner/v2/cmd/osv-scanner@latest scan source -r .
    docker run --rm -v "$PWD":/src:ro aquasec/trivy fs --scanners vuln,secret,misconfig /src
    docker run --rm ghcr.io/zaproxy/zaproxy:stable zap-baseline.py -t https://<your server>/

The Observatory (developer.mozilla.org/en-US/observatory) and SSL Labs (ssllabs.com/ssltest) are
web pages: give them your server's public name.
