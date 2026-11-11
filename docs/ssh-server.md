# SSH server

`ssh ssh.devops-daily.com` opens a small DevOps Daily app in your terminal.
It is an app, never a shell: any key or no key works, there is no password
prompt, and nothing else is reachable.

The menu:

1. **Latest posts**: the newest 15 items from the site's RSS feed, with date,
   summary and link. The feed is cached and refreshed every 30 minutes in the
   background, so a slow feed never blocks a session.
2. **Daily quiz**: one question a day from the site's quizzes. Everyone gets
   the same question on the same UTC day. Answer with a to d, then see the
   explanation and a link to the full quiz.
3. **Incident of the day**: a real-world problem from the *30 Days of
   Real-World DevOps Problems* series, the fix, and a link to the matching
   post or simulator.
4. **Newsletter**: a link to the subscribe page. The app never asks for an
   email address.
5. **Links**: the site, simulators, Opsville, the roadmap and GitHub.

A one-line sponsor slot sits above the key help on every screen. It works in
80x24 and in larger terminals; `q` or `ctrl+c` quits.

Built with [Wish](https://github.com/charmbracelet/wish) and
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Security model

- Only `session` channels are accepted, one per connection. `direct-tcpip`
  (`ssh -L`, `ssh -D`) and every other channel type are rejected; there are no
  global request handlers, so `tcpip-forward` (`ssh -R`) is refused.
- Channel requests other than `pty-req`, `window-change`, `shell` and `exec`
  get a "no": `env`, `x11-req`, `auth-agent-req@openssh.com`, `subsystem`
  (so no SFTP or modern scp) and `signal`.
- `exec` (`ssh host cat /etc/passwd`, legacy `scp -O`) prints a short note and
  exits with status 1. Nothing is ever run.
- A session without a terminal is refused with a note.
- Public key and keyboard-interactive auth always succeed with no prompt.
  Keys are not stored or logged. Password auth is off.
- The SSH banner is `SSH-2.0-DevOpsDaily`, not a fake OpenSSH version.
- The app never queries the client terminal (no OSC or DA requests); colors
  come from `TERM` alone. All feed, quiz and config text is stripped of
  control codes before it is drawn.

Limits (flags on the binary, defaults shown):

| Limit | Default |
| --- | --- |
| New connections per address | 10 per minute (`-rate-per-ip`) |
| Open connections per address | 5 (`-max-per-ip`) |
| Open connections in total | 200 (`-max-sessions`) |
| Idle timeout (no key press) | 10 minutes (`-idle-timeout`) |
| Longest session | 30 minutes (`-max-session`) |
| Shell must start within | 30 seconds of connecting |
| Window size | clamped to 500x200 |

IPv6 addresses are grouped by /64. Limits are checked before the SSH
handshake, so a refused connection costs one accept and one short write. The
app shows the idle and session limits; the SSH layer has a backstop one minute
later. Refused connections are logged at most once a minute per address;
normal sessions are logged without addresses.

## Server layout

The service runs as a hardened systemd unit (`deploy/ssh-site.service`):
`DynamicUser`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`,
`NoNewPrivileges`, only `CAP_NET_BIND_SERVICE`, `AF_INET`/`AF_INET6`/`AF_UNIX`
only, a system call filter, `MemoryMax=256M` and `TasksMax=64`.

| Path | What |
| --- | --- |
| `/usr/local/bin/ssh-site` | the binary |
| `/etc/ssh-site/ssh-site.env` | `SSH_SITE_LISTEN=<public address>:22` |
| `/etc/ssh-site/sponsor.conf` | the sponsor line |
| `/var/lib/ssh-site/host_ed25519` | the host key, created on first start |
| `/var/lib/ssh-site/feed.json` | the feed cache |

Admin SSH (OpenSSH) listens only on the server's tailnet address, so public
port 22 belongs to the app. The app binds the public address explicitly,
because a wildcard bind would clash with sshd on the tailnet address.

## Build and test

Needs Go 1.26 or newer.

```sh
make test     # unit tests: date picks, feed cache, limits, server hardening, UI layout
make build    # static linux/amd64 binary in bin/ssh-site
make run      # local copy on 127.0.0.1:2222; then: ssh -p 2222 localhost
```

## Deploy

Once per server, as root, after `tailscale up`:

```sh
bash deploy/bootstrap.sh
```

It moves sshd to the tailnet address (with a 5-minute automatic revert until
you confirm a new login works), writes the env file and an empty sponsor
config, and adds Tailscale to unattended-upgrades.

Then, for every release:

```sh
make deploy HOST=root@<tailnet address>
```

`deploy/deploy.sh` copies the binary and unit, restarts the service, rolls
back to the previous binary if the new one does not stay up, and prints the
host key fingerprint.

To check a running server from outside (rate limit, no shell, no scp or
sftp, no forwarding, session cap; about four minutes):

```sh
scripts/check-live.sh ssh.devops-daily.com
```

Everyone behind one NAT address shares the per-address limits, so run it
when nothing else from that address is connecting.

## Change the sponsor line

Edit `/etc/ssh-site/sponsor.conf` on the server:

```
name=Example
text=What they do, in a few words
url=https://example.com
```

The app re-reads it within 10 seconds; no rebuild or restart. An empty file
hides the line. On narrow terminals the text is dropped first, then the URL.

## Refresh the quiz and incident content

Questions and incidents are baked into the binary from
`internal/content/snapshot.json`. To refresh it:

```sh
make snapshot QUIZZES=<site checkout>/content/quizzes PROBLEMS=<devops-problems.json>
make test && make deploy HOST=...
```

Only problems marked `answered` are included, so no answer appears before it
was published. Picks walk a fixed shuffle one step per UTC day, so the order
changes when the snapshot does.

## Host key

```sh
ssh root@<tailnet address> /usr/local/bin/ssh-site -fingerprint -state-dir /var/lib/ssh-site
```

The key is generated on the server on first start and never leaves it. Keep
the state directory when rebuilding, or visitors will see a host key warning.

