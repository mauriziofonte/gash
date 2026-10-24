<!-- Gash 3.0 user guide: gash hte. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Local sites with `gash hte`

`gash hte` runs local Apache and PHP-FPM sites as your own user. It replaces
the separate PHP tool hte-cli. One `sudo` at install does the few system
steps (the hosts helper, its sudoers line, the low ports, autostart), and
every other command runs without root. The Apache and PHP-FPM binaries are
the ones your system already has. gash does not compile anything.

```bash
gash hte install                       # one sudo, then the stack runs as you
gash hte migrate                       # what would move from hte-cli (a dry run)
gash hte migrate --apply --yes         # move the hte-cli sites, stop the system Apache
gash hte site create --domain shop.test --docroot public
gash hte doctor                        # checks everything, names the fix for each failure
```

Every command takes `--json` (one JSON object on stdout) and `--help`. Errors
are the Gash error envelope on stderr with exit status 1 (see the
[agent guide](../AGENTS.md)). Without a terminal nothing prompts: a missing
required flag fails with `missing_value`, and a destructive step without
`--yes` fails with `confirmation_required`.

## Commands

| Command | Does |
|---|---|
| `install [--yes] [--ports standard\|high] [--autostart auto\|systemd\|launchd\|shell\|none]` | The system steps (one sudo), the userland tree, the stack, autostart |
| `uninstall --yes` | Stops the stack, restores what install changed, keeps your sites in a stage for a later install |
| `doctor` | Binaries, modules, ports, the stack, every site, certificates, names, the receipt |
| `migrate [--apply] [--site D]... [--exclude D\|PATH]... [--keep-pool-values] [--reissue-certs] [--yes]` | Imports hte-cli and other system vhosts; a dry-run plan without `--apply` |
| `site create\|modify\|remove\|list\|show\|enable\|disable` | Sites (below) |
| `php versions`, `php ini get\|set\|unset`, `php global get\|set\|unset` | PHP versions, per-site and per-version PHP settings |
| `apache config path`, `apache modules list\|enable\|disable` | The Apache overlay you edit, extra modules |
| `service start\|stop\|restart\|reload\|status\|logs` | The masters |
| `cert show\|renew --domain D` | The site's self-signed certificate |
| `hosts list\|apply\|check\|windows` | The managed block in `/etc/hosts`; `windows` prints the lines for the Windows hosts file under WSL |

`--autostart` picks how the stack comes back after a reboot. The default
`auto` is a systemd user unit where systemd runs, a LaunchAgent on macOS,
and nothing elsewhere. Where there is no systemd (containers, WSL without
systemd), `--autostart shell` makes every interactive shell that loads Gash
start the stack in the background when it is down. That start prints
nothing and never slows or fails the shell; when it fails, the reason is in
`~/.local/state/gash/hte/logs/autostart.log`.

`site create` needs `--domain` and `--docroot`. A relative document root is
resolved against the current directory. When you leave a flag out, the site
gets HTTPS on, forced HTTPS off, HTTP/2 on, the `dev` profile, a `www.` alias,
and the PHP version set as default in `hte.toml` (else the newest installed
one). A value that is not valid is an error: gash never puts a default in
its place. The other flags are `--php X.Y`,
`--profile dev|staging|hardened`, `--https`/`--no-https`,
`--force-https`/`--no-force-https`,
`--force-https-bypass`/`--no-force-https-bypass`, `--alias NAME` (repeatable),
`--no-www`, `--no-http2`, `--extra-http-port N` and `--extra-https-port N`.
`site modify` takes the same flags. `site remove --domain D --yes` removes
the site's state, configuration, pool, certificate and hosts names. It never
touches the document root, the site's overlay or its logs.

## Coming from hte-cli

There is no `hte-cli` compatibility command. The old commands map as follows:

| hte-cli 1.1 | gash hte |
|---|---|
| `sudo hte-cli create --domain=D --docroot=P --phpver=X.Y --ssl=y --forcessl=n --profile=dev` | `gash hte site create --domain D --docroot P --php X.Y --https --no-force-https --profile dev` |
| `sudo hte-cli modify --domain=D ...` | `gash hte site modify --domain D ...` (only the flags you pass change) |
| `sudo hte-cli remove --domain=D --force` | `gash hte site remove --domain D --yes` |
| `sudo hte-cli details [--domain=D]` | `gash hte site list`, `gash hte site show --domain D` |
| `sudo hte-cli fix [--domain=D] [--dry-run]` | `gash hte migrate` (dry run), then `gash hte migrate --apply --yes`. Imported sites already have the fixed configuration |
| `sudo hte-cli hosts list` | `gash hte hosts list` |
| `sudo hte-cli hosts sync` | `gash hte hosts apply` (also run by every site change, `service start` and `doctor`) |
| `sudo hte-cli hosts add\|remove --domain=D` | No direct counterpart: the block always holds the names of the enabled sites |
| `--hosts=no` | No counterpart: every site's names go in the managed block |
| `--interactive`, `--no-normalize`, `-n` | Not needed: a terminal is asked only for a missing required flag; headless calls never prompt |

What else changes when you move:

* Sites run under your own user, with no root process. Configuration,
  certificates and logs live in your XDG directories (`~/.config/gash/hte`,
  `~/.local/share/gash/hte`, `~/.local/state/gash/hte`). Logs are never put
  inside a document root.
* Apache listens on loopback only (`127.0.0.1`, and `[::1]` when the machine
  has IPv6 loopback). The hosts lines are `127.0.0.1` lines in a block
  between `# >>> gash hte >>>` and `# <<< gash hte <<<`. Your old `# hte-cli`
  lines are left as they are.
* Forced HTTPS redirects with 307, not 301, and also redirects requests from
  `127.0.0.1` and `::1`. `--force-https-bypass` turns the hte-cli 1.1
  exception for loopback clients back on. `migrate` keeps each old site's
  behavior.
* Certificates are ECDSA P-256 and valid for 10 years. Their names are the
  domain, `*.domain` and every alias. Changing a site's aliases makes a new
  certificate. `migrate` copies the old RSA pair, or makes a new one with
  `--reissue-certs`. `doctor` notes a copied pair, which `cert renew`
  replaces. `site modify --no-https` keeps the certificate, so that
  `--https` later reuses it.
* Hand edits go in overlay files, which gash never rewrites:
  `~/.config/gash/hte/sites/<domain>.apache.conf` for a site,
  `~/.config/gash/hte/apache.conf` for the server
  (`gash hte apache config path`). Run `gash hte service reload` after
  editing them.
