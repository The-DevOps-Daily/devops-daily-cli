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
- `--timeout 5s`: how long to wait for each network reply. For `check
  headers` it covers each request, including its connect and TLS handshake.

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

International names work in any check: `bücher.de` is looked up, sent as
the TLS server name and matched against the certificate as
`xn--bcher-kva.de`, and the report shows both forms.

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
gone (a dangling CNAME, which someone may be able to take over), a CNAME
loop, or a chain of 16 or more aliases. The chain is followed to its end even
when the resolver stops early, as it does for `--type CNAME`. With the
default types, a name with no A or AAAA records is a `warn`; with `--type`,
a type with no records is a `warn`.

## check headers

```sh
ddy check headers devops-daily.com
ddy check headers http://example.com/login
```

`https://` is added when there is no scheme. Redirects (301, 302, 303, 307
and 308; not 300 or 304) are followed one at a time, at most 10, and each
hop is shown with its status and target. A
loop, too many redirects or an untrusted certificate is a `fail`; a
redirect from HTTPS to plain HTTP is a `warn`.

The final response's headers:

| Header | `pass` when |
| --- | --- |
| `Strict-Transport-Security` | `max-age` is 180 days or more and no directive appears twice |
| `Content-Security-Policy` | `script-src`, `script-src-elem` and `script-src-attr` (each falling back to `script-src`, then `default-src`) allow no `'unsafe-inline'` without a nonce or hash, and no `'unsafe-eval'`, `*`, `http:`, `https:`, `https://*` or `data:` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` or CSP `frame-ancestors` | `DENY`/`SAMEORIGIN`, or a `frame-ancestors` with no wildcard source in at least one policy |
| `Referrer-Policy` | the last valid value, over all the headers, is not `unsafe-url` or `no-referrer-when-downgrade` |
| `Permissions-Policy` | present |

A missing HSTS (on HTTPS), CSP, `X-Content-Type-Options` or frame protection
is `missing`. A missing `Referrer-Policy` or `Permissions-Policy` is only
`info`: browsers default to `strict-origin-when-cross-origin`, and
`Permissions-Policy` is optional. With several CSP headers the browser
enforces all of them, so the strictest one counts.

`Cache-Control`, `Age`, `ETag` and `Server` are shown as `info`, with what
they mean for caches. There are no grades.

## check email

```sh
ddy check email gmail.com
ddy check email example.com --selector mailjet --server 8.8.8.8
```

An email address or a URL works too; only the domain is used.

- **SPF**: there must be one `v=spf1` record. Like a receiver, the check
  stops at the first `all` and uses `redirect=` only when there is no `all`.
  It counts the DNS lookups that needs (`include`, `a`, `mx`, `ptr`,
  `exists` and an evaluated `redirect`, nested includes too) against the
  limit of 10. These are a `fail` (receivers return permerror): over 10
  lookups, an `mx` with more than 10 MX hosts, a bad `ip4`/`ip6` address or
  prefix length, an unknown mechanism, and an include with no SPF record.
  `+all` is a `fail`, `?all` or no `all` a `warn`, `ptr` a `warn`
  (deprecated). An `include` or `redirect` with macros (`%{d}`) is a `warn`:
  it is filled in per message, so the count cannot be checked.
- **DMARC**: one record at `_dmarc.<domain>` whose first tag is
  `v=DMARC1`, exactly in that case (spaces around `=` are fine). `p=none` is
  a `warn` (monitoring only), and so is a missing `rua` (no reports). A
  `pct` under 100 is a `warn`: the updated standard (DMARCbis) drops `pct`,
  and `t=y` is the new testing switch. With `t=y`, receivers apply one step
  less: `reject` becomes `quarantine`, `quarantine` becomes `none`.
- **DKIM**: keys cannot be listed, so the check tries common selectors:
  `default`, `google`, `selector1`, `selector2`, `k1`, `s1`, `mail` and
  `dkim`. Finding none is only `info`: many providers (Amazon SES, for
  example) use their own selector names. Add yours with `--selector` (find
  it as `s=` in the `DKIM-Signature` header of a mail you received). RSA keys
  under 2048 bits are a `warn`, under 1024 a `fail`. A malformed record is a
  `fail`: `v=` that is not first or not `DKIM1`, no `p=`, a key that does not
  decode, an unknown `k=` or a tag given twice. An empty `p=` means the key
  was revoked on purpose (`info`).
- **MTA-STS** (`_mta-sts.<domain>`) and **TLS-RPT** (`_smtp._tls.<domain>`)
  are optional. When present there must be one record each, MTA-STS with an
  `id=` (1 to 32 letters and digits) and TLS-RPT with a `rua=`; otherwise
  senders ignore them, which is a `fail`. The MTA-STS policy file itself is
  not downloaded.

## Not in the SSH app

The check commands exist only in the local CLI. In the SSH app
(`ssh ssh.devops-daily.com`) they would make our server connect to any host
a visitor typed. `cmd/ssh-site/imports_test.go` fails if the server ever
imports `internal/check` or `internal/cli`, directly or through another
package.
