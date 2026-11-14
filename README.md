# DevOps Daily CLI

Learn DevOps from your terminal.

## Install

Homebrew:

```sh
brew install The-DevOps-Daily/tap/devops-daily
```

The install script:

```sh
curl -fsSL https://raw.githubusercontent.com/The-DevOps-Daily/devops-daily-cli/main/install.sh | sh
```

npm:

```sh
npm install -g @devopsdaily/devops-daily-cli
npx @devopsdaily/devops-daily-cli
```

Go:

```sh
go install github.com/The-DevOps-Daily/devops-daily-cli/cmd/devops-daily@latest
```

Release archives are available from the GitHub releases page.

## Verify a release

Check the archive against `checksums.txt`. If you have GitHub CLI, verify
build provenance too:

```sh
gh attestation verify <file> -R The-DevOps-Daily/devops-daily-cli
```

## Commands

Run `devops-daily` (or the short alias `ddy`) with no command for the home
menu: the same one `ssh ssh.devops-daily.com` shows, with latest posts you
can read in full, the daily quiz question and the incident of the day.

| Command | What it does |
| --- | --- |
| `posts [--tag k8s] [--limit N] [--all] [--json]` | List posts, newest first |
| `read <slug>` | Read a post in the terminal, with its links at the end |
| `quiz [topic]` | Play one of the site's quizzes; your best score is kept |
| `flashcards [deck]` | Study a deck with simple spaced repetition |
| `checklist [name]` | Tick off a checklist; progress is saved |
| `incident [--fix]` | Today's real-world incident, then the fix |
| `play [simulator]` | Open a simulator, a game or Opsville in your browser |
| `check tls\|dns\|headers\|email <target>` | Check a certificate, DNS records, security headers or email records |
| `subscribe` | Open the newsletter sign-up page |
| `sync` | Download the latest content now |
| `update` | Show how to upgrade the way you installed it |
| `version` | Print the version |
| `completion bash\|zsh\|fish` | Shell completions |

Names are forgiving: `devops-daily quiz docker`, `devops-daily play dns` and
`devops-daily read terraform-state` find the closest match or list the
candidates.

## Check a host

Four everyday checks, run from your machine. Each line says what was found
and what it means, and the last line links to a DevOps Daily page on the
topic.

```sh
ddy check tls github.com                        # certificate, days left, chain, trust
ddy check dns www.github.com --server 1.1.1.1   # records with TTLs, CNAME chain
ddy check headers devops-daily.com              # redirects, security and cache headers
ddy check email gmail.com                       # MX, SPF, DMARC, DKIM, MTA-STS, TLS-RPT
```

```text
$ ddy check tls expired.badssl.com
...
  fail     Expires in     expired 4234 days ago (2015-04-12 23:59 UTC)
           Browsers block the site with a warning page. Renew the certificate now.
  fail     Trusted        no, a certificate in the chain has expired or is not valid yet
...
Result: 2 problems.
```

Add `--json` for scripts and `--timeout 10s` for slow hosts. Exit codes: 0
all good, 1 warnings, 2 a problem or an error. The checks are not part of
the SSH app. See [docs/check.md](docs/check.md) for what each one looks at.

## Offline use and files

After content has been synced, the CLI works offline. On Linux, the cache is
`~/.cache/devops-daily` and progress is in
`~/.local/state/devops-daily`. macOS and Windows use their system cache and
config directories.

Set `DEVOPS_DAILY_CACHE_DIR` or `DEVOPS_DAILY_STATE_DIR` to override those
locations. Set `DEVOPS_DAILY_OFFLINE=1` for offline mode, and `NO_COLOR`
to disable color.

## Privacy

No telemetry. The CLI downloads the public content files from
`devops-daily.com/cli/v1/` and, only when you run `update`, asks GitHub for
the latest version. The `check` commands talk only to the host you name and
to your DNS resolver (or the one you pick with `--server`). Nothing about
you is sent.

## SSH app

```sh
ssh ssh.devops-daily.com
```

See the [SSH server documentation](docs/ssh-server.md).

## Data format

The public content format is documented in [docs/data-format.md](docs/data-format.md).

## Build from source

```sh
make test
make cli
```

## License

[MIT](LICENSE).
