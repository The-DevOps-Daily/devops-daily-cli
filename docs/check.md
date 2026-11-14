# Check commands

`devops-daily check` (or `ddy check`) looks at a host from your own machine
and says, in plain words, what it found:

| Command | What it checks |
| --- | --- |
| `check tls <host[:port]>` | TLS version, cipher, the certificate (names, issuer, dates, days left), the chain, trust, host name match, OCSP stapling |
| `check dns <name>` | A, AAAA, MX, NS, TXT and CAA records with TTLs, and the CNAME chain |
| `check headers <url>` | Each redirect, then the security headers and cache headers of the final response |
| `check email <domain>` | MX, SPF, DMARC, DKIM, MTA-STS and TLS-RPT records |

Every line has a status:

| Status | Meaning |
| --- | --- |
| `pass` | Looks right |
| `info` | Just information |
| `warn` | Works, but worth fixing |
| `missing` | Not set, and it should be |
| `fail` | Broken, or the check could not run |

## Flags and exit codes

All four commands take:

- `--json`: print the report as JSON for scripts. `details` has the raw
  data (for example `details.daysLeft` for TLS) and `sections` has every
  finding with a stable `id`.
- `--timeout 5s`: how long to wait for each network reply.

Exit codes: `0` all good, `1` warnings (`warn` or `missing`), `2` a
problem (`fail`) or an error, such as a host that does not resolve or a bad
argument.

```sh
ddy check tls example.com --json | jq .details.daysLeft
ddy check tls example.com >/dev/null || echo "fix the certificate"
```

## What leaves your machine

Only what the check needs: a connection to the host you name, and DNS
questions to your system resolver (the first `nameserver` in
`/etc/resolv.conf`) or the server you pick with `--server`. No telemetry and
no request to devops-daily.com. Where `/etc/resolv.conf` cannot be read (on
Windows, for example), DNS questions go to `1.1.1.1` and the report says so.
`check headers` sends `User-Agent: devops-daily-cli/<version> (check
headers)`.

On macOS and Windows, Go asks the operating system to verify certificate
chains, and the operating system may fetch a missing intermediate
certificate itself.

## check tls

```sh
ddy check tls github.com
ddy check tls smtp.gmail.com:465
```

The port defaults to 443, and a pasted URL works too. The check connects
without trusting the certificate first, so that a bad one is reported
instead of stopping the check; then it verifies the chain against your
system's trusted roots and the host name separately. It warns when the
certificate expires in 21 days or less: lifetimes are getting shorter (Let's
Encrypt moves to 64 days in February 2027, later 45), so renewal must be
automatic. TLS 1.0 and 1.1 are a `fail`. A missing OCSP staple is only
`info`, since Let's Encrypt ended OCSP in 2025.

## check dns

```sh
ddy check dns devops-daily.com
ddy check dns www.github.com --type CNAME,A --server 1.1.1.1
```

`--type` takes `A`, `AAAA`, `CNAME`, `MX`, `NS`, `TXT`, `CAA` or `SOA`
(repeat it or separate with commas). Without it you get A, AAAA, MX, NS,
TXT and CAA. `--server` takes an IP address, with an optional port. TTLs
from a caching resolver count down; ask the zone's own name server (see the
NS records) for the full value.

A name that does not exist is a `fail`, and so is an alias whose target is
gone (a dangling CNAME, which someone may be able to take over). With the
default types, a name with no A or AAAA records is a `warn`; with `--type`,
a type with no records is a `warn`.

## check headers

```sh
ddy check headers devops-daily.com
ddy check headers http://example.com/login
```

`https://` is added when there is no scheme. Redirects are followed one at
a time (at most 10), and each hop is shown with its status and target. A
loop, too many redirects or an untrusted certificate is a `fail`; a
redirect from HTTPS to plain HTTP is a `warn`.

The final response's headers:

| Header | `pass` when |
| --- | --- |
| `Strict-Transport-Security` | `max-age` is 180 days or more |
| `Content-Security-Policy` | `script-src` (or `default-src`) has no `'unsafe-inline'` without a nonce or hash, no `'unsafe-eval'`, `*`, `http:`, `https:` or `data:` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` or CSP `frame-ancestors` | `DENY`/`SAMEORIGIN`, or any `frame-ancestors` but `*` |
| `Referrer-Policy` | anything but `unsafe-url` and `no-referrer-when-downgrade` |
| `Permissions-Policy` | present |

`Cache-Control`, `Age`, `ETag` and `Server` are shown as `info`, with what
they mean for caches. There are no grades.

## check email

```sh
ddy check email gmail.com
ddy check email example.com --selector mailjet --server 8.8.8.8
```

An email address or a URL works too; only the domain is used.

- **SPF**: there must be one `v=spf1` record. The check counts the DNS
  lookups it needs (`include`, `a`, `mx`, `ptr`, `exists` and `redirect`,
  nested includes too) against the limit of 10. `+all` is a `fail`, `?all`
  or no `all` a `warn`.
- **DMARC**: one `v=DMARC1` record at `_dmarc.<domain>`. `p=none` is a
  `warn` (monitoring only), and so is a missing `rua` (no reports). A `pct`
  under 100 is a `warn`: the updated standard (DMARCbis) drops `pct`, and
  `t=y` is the new testing switch.
- **DKIM**: keys cannot be listed, so the check tries common selectors:
  `default`, `google`, `selector1`, `selector2`, `k1`, `s1`, `mail` and
  `dkim`. Add yours with `--selector` (find it as `s=` in the
  `DKIM-Signature` header of a mail you received). RSA keys under 2048 bits
  are a `warn`, under 1024 a `fail`.
- **MTA-STS** (`_mta-sts.<domain>`) and **TLS-RPT** (`_smtp._tls.<domain>`)
  are optional and shown when present. The MTA-STS policy file itself is not
  downloaded.

## Not in the SSH app

The check commands exist only in the local CLI. In the SSH app
(`ssh ssh.devops-daily.com`) they would make our server connect to any host
a visitor typed. `cmd/ssh-site/imports_test.go` fails if the server ever
imports `internal/check` or `internal/cli`, directly or through another
package.
