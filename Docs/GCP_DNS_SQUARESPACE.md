# Squarespace DNS for Diana TEST

Status: **PUBLISHED AND VERIFIED**. Global static IP `diana-test-ip` is
`136.69.63.17`. Both hostnames resolve to this address and managed certificate
`diana-test-cert-v3` is `ACTIVE`.

Final intended records:

| TYPE | HOST | VALUE | TTL | PURPOSE |
|---|---|---|---|---|
| A | `test.platform` | `136.69.63.17` | 1 hour | Diana TEST frontend |
| A | `api-test` | `136.69.63.17` | 1 hour | Diana TEST API and ANAF callback |

To publish the records:

1. Open Squarespace → Domains.
2. Select `accountingtechco.com`.
3. Open DNS → DNS Settings → Custom Records.
4. Search existing records for hosts exactly `test.platform` and `api-test`.
5. If either exists with a different value, stop and report the conflict; do
   not delete it silently.
6. Add only the exact A records above.

Do not touch root/apex records, `www`, Squarespace website records, MX, SPF,
DKIM, DMARC, or unrelated TXT/CNAME records. If Google later emits a domain
verification record, add only that exact generated token.

Verification after the user saves DNS:

```bash
dig +short test.platform.accountingtechco.com A
dig +short api-test.accountingtechco.com A
nslookup test.platform.accountingtechco.com
nslookup api-test.accountingtechco.com
curl -I https://test.platform.accountingtechco.com/
curl https://api-test.accountingtechco.com/healthz
./scripts/gcp/verify-test.sh
```

Deployment is not ready until both names resolve to the reserved global IP and
the Google-managed certificate reports `ACTIVE` for both domains.
