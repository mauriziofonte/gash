# Local web stack (HTE)

`gash hte` runs local websites on Apache and PHP-FPM as your own user, with
HTTPS, a name for each site in `/etc/hosts`, and its own PHP settings for
each site. It replaces the PHP tool hte-cli. One `sudo` at install does the
few system steps (the hosts helper and its sudoers line, the low ports,
autostart); every other command runs without root. Apache and PHP-FPM are
the ones your system's packages provide: Gash installs and compiles
nothing.

Platforms: Linux (Debian, Ubuntu, Fedora, the RHEL family, Arch and their
derivatives), macOS with Homebrew, and WSL 2 inside one of those
distributions. Other systems (openSUSE, Alpine) get `platform_unsupported`.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [sudo](../requirements.md#tool-sudo) (feature), for `gash hte`
- [apache](../requirements.md#tool-apache) (feature), for `gash hte`
- [php-fpm](../requirements.md#tool-php-fpm) (feature), for `gash hte` (PHP sites)
- [openssl](../requirements.md#tool-openssl) (feature), for `gash hte`
<!-- END generated: prerequisites -->

Every command takes `--json` (one JSON object on stdout) and `--help`.
Errors are the Gash error envelope on stderr with exit status 1. Without a
terminal nothing prompts: a missing required flag fails with
`missing_value`, and a destructive step without `--yes` fails with
`confirmation_required`.

## How the stack is laid out

- One Apache, run as you, listening on the loopback addresses only
  (`127.0.0.1`, and `[::1]` where the machine has IPv6 loopback): the sites
  are not reachable from the network.
- One PHP-FPM master per PHP version, and in it one pool per site, with the
  site's profile and its own php.ini values.
- One self-signed certificate per site (ECDSA P-256, valid for 10 years),
  for the domain, `*.domain` and every alias. Browsers warn about it.
- Your files live in your XDG directories: settings and site state in
  `~/.config/gash/hte`, certificates in `~/.local/share/gash/hte`, the
  rendered configuration and every log in `~/.local/state/gash/hte`. Logs
  never go inside a document root.

## Install the stack

Install Apache, PHP-FPM and openssl with your package manager first
(`gash hte doctor` names what is missing), then, in a terminal:

```bash
gash hte install
```

`install` prints its plan, runs the system steps in one `sudo` (which asks
for your password), then, as you, writes the configuration, starts the
stack and sets up autostart. Each system step is recorded in a receipt
that `uninstall` plays back. Running `install` again changes only what is
missing.

The options that matter:

- `--ports standard` (the default) serves on ports 80 and 443. On Linux
  install sets `net.ipv4.ip_unprivileged_port_start = 80` in
  `/etc/sysctl.d/50-gash-hte.conf`, which lets every local user bind the
  ports from 80 to 1023. On a shared machine use `--ports high` instead: 8080
  and 8443, and no port step. On macOS, see [macOS](#macos).
- `--autostart` decides how the stack comes back after a reboot. `auto`
  (the default) is a systemd user unit where systemd runs (install turns
  on linger, so it starts at boot), a LaunchAgent on macOS, and nothing
  elsewhere. `shell` starts the stack in the background from the first
  interactive shell that loads Gash; that start prints nothing and never
  slows or fails the shell, and a failure is logged in
  `~/.local/state/gash/hte/logs/autostart.log`. `none` sets up nothing.
- `--yes` allows install to stop the system Apache when its plan does that.

On the standard ports, install stops the system Apache only when nothing it
serves would be lost. While an enabled system vhost is neither migrated
nor excluded (on a fresh Debian or Ubuntu the stock `000-default.conf`, on
Fedora mod_ssl's `ssl.conf`), install leaves the stack stopped and names
the `--exclude` to add: see [Move sites from hte-cli](#move-sites-from-hte-cli).

See [`gash hte install`](reference.md#gash-hte-install).

## Create a site

```bash
cd ~/projects/shop
gash hte site create --domain shop.test --docroot public
curl -k https://shop.test/
```

`--domain` and `--docroot` are required (in a terminal a missing one is
asked for); a relative document root is resolved against the current
directory. A flag left out gets its default: HTTPS on, forced HTTPS off,
HTTP/2 on, the `dev` profile, a `www.` alias, and the default PHP version
of `hte.toml` (else the newest installed one). A value that is not valid is
an error: Gash never puts a default in its place. The site's names go in
the managed block of `/etc/hosts` at once.

| Flag | Effect |
|---|---|
| `--php X.Y` | The PHP version (`gash hte php versions` lists them) |
| `--profile dev\|staging\|hardened` | The pool's limits: `dev` has few restrictions and high memory limits; `staging` adds `open_basedir` and session hardening; `hardened` also disables `exec`, `proc_open` and the like, which can break Laravel, Composer or PHPUnit |
| `--https`, `--no-https` | Serve HTTPS |
| `--force-https`, `--no-force-https` | Redirect HTTP to HTTPS, with status 307 |
| `--force-https-bypass` | Let plain HTTP through for requests from `127.0.0.1` and `::1` |
| `--alias NAME` | One more name for the site (repeatable); `--no-www` drops the `www.` alias |
| `--http2`, `--no-http2` | Serve HTTP/2 |
| `--extra-http-port N`, `--extra-https-port N` | One more loopback port for the site (repeatable) |

With `--ports high`, the URL carries the port: `https://shop.test:8443/`.
See [`gash hte site create`](reference.md#gash-hte-site-create).

## Change, disable and remove sites

```bash
gash hte site list                                   # domain, PHP, HTTPS, enabled, document root
gash hte site show --domain shop.test                # settings, aliases, ports, php.ini values, certificate
gash hte site modify --domain shop.test --php 8.3    # only the flags you pass change
gash hte site disable --domain shop.test             # stop serving it, keep everything
gash hte site enable --domain shop.test
gash hte site remove --domain shop.test --yes
```

`site modify` takes the flags of `site create`. Changing the aliases makes
a new certificate; `--no-https` keeps the certificate, so that `--https`
later reuses it. `site remove` deletes the site's state, rendered
configuration, pool, certificate and hosts names; it never touches the
document root, the site's overlay file or its logs.

Hand edits go in overlay files, which Gash includes and never rewrites:
`~/.config/gash/hte/sites/DOMAIN.apache.conf` for one site, and
`~/.config/gash/hte/apache.conf` for the whole server
(`gash hte apache config path` prints it). Run `gash hte service reload`
after editing them. `gash hte apache modules list` shows the modules the
stack loads; `enable NAME` and `disable NAME` add or drop one from the
system module directory.

See [`gash hte site`](reference.md#gash-hte-site) and
[`gash hte apache`](reference.md#gash-hte-apache).

## Set PHP versions and php.ini values

```bash
gash hte php versions                                              # the PHP-FPM versions found
gash hte php ini set memory_limit 512M --domain shop.test          # one site
gash hte php ini get memory_limit --domain shop.test
gash hte php global set upload_max_filesize 64M --version 8.3      # every site on PHP 8.3
gash hte php global unset upload_max_filesize --version 8.3
```

`php ini` values belong to one site and are written into its pool.
`php global` values go in an overlay file of one PHP version, read by every
site of that version. Each change reloads what it affects; after a
`php global` change, a PHP-FPM master that does not come back gets the
previous file. Gash finds the versions the
packages install: Debian's `php-fpmX.Y` (also from the sury/ondrej
repositories), Fedora's and Remi's, Arch's (and the AUR ones), and
Homebrew's `php@X.Y`.

See [`gash hte php`](reference.md#gash-hte-php).

## Run the stack

```bash
gash hte service status                  # the masters, and whether each site answers
gash hte service start
gash hte service stop
gash hte service restart
gash hte service reload                  # render, test, reload; the old configuration comes back on failure
gash hte service logs --site shop.test   # that site's access, error and PHP logs
```

With the systemd user unit, start, stop and restart go through the unit.
`service logs` prints the last lines (`--lines N`, default 20) and has no
follow mode.

See [`gash hte service`](reference.md#gash-hte-service).

## Certificates and site names

```bash
gash hte cert show --domain shop.test    # names, validity, fingerprint
gash hte cert renew --domain shop.test   # a new certificate, then a reload
gash hte hosts list                      # the managed block of /etc/hosts
gash hte hosts check                     # exit 1 when the block and the sites differ
gash hte hosts apply                     # write the block again
```

The names live in `/etc/hosts` between `# >>> gash hte >>>` and
`# <<< gash hte <<<`, as `127.0.0.1` lines; lines outside the block are
never changed. Every site change, `service start` and `doctor` write the
block again when it drifted. They do it through the hosts helper, the one
program install lets your user run as root: it writes only that block of
`/etc/hosts`, and only loopback addresses and plain host names.

See [`gash hte cert`](reference.md#gash-hte-cert) and
[`gash hte hosts`](reference.md#gash-hte-hosts).

## Check everything

```bash
gash hte doctor
```

`doctor` checks the platform, Apache and its modules, the PHP versions,
openssl, the receipt, the hosts helper and its sudoers line, the port
setting and who holds the ports, autostart, the configuration test, every
site (HTTP and HTTPS answers, certificate validity and names) and name
resolution. Each failure comes with its remedy, and a drifted hosts block
is written again. It exits 1 when a check fails.

## Move sites from hte-cli

`migrate` imports the sites of hte-cli, and the other vhosts of the system
Apache. Without `--apply` it only prints the plan:

```bash
gash hte migrate                      # the plan: what moves, what is skipped and why
gash hte migrate --apply --yes        # move the sites, stop the system Apache
```

`--site D` migrates only that domain and `--exclude D|PATH` leaves a domain
or a vhost file out (both repeatable). Each old site keeps its forced-HTTPS
behavior. Certificates are copied (`--reissue-certs` makes new ones
instead; `cert renew` replaces a copied pair later), and
`--keep-pool-values` keeps the PHP values of the old pool.

When `install` stopped because of a system vhost you do not need,
`gash hte migrate --apply --exclude PATH --yes` stops and disables the
system Apache and starts the stack on 80 and 443, also when there is
nothing to import. The excluded file stays as it is, and
`gash hte uninstall` starts the system Apache again.

There is no `hte-cli` compatibility command. The old commands map as
follows:

| hte-cli 1.1 | gash hte |
|---|---|
| `sudo hte-cli create --domain=D --docroot=P --phpver=X.Y --ssl=y --forcessl=n --profile=dev` | `gash hte site create --domain D --docroot P --php X.Y --https --no-force-https --profile dev` |
| `sudo hte-cli modify --domain=D ...` | `gash hte site modify --domain D ...` (only the flags you pass change) |
| `sudo hte-cli remove --domain=D --force` | `gash hte site remove --domain D --yes` |
| `sudo hte-cli details [--domain=D]` | `gash hte site list`, `gash hte site show --domain D` |
| `sudo hte-cli fix [--domain=D] [--dry-run]` | `gash hte migrate`, then `gash hte migrate --apply --yes` (imported sites already have the fixed configuration) |
| `sudo hte-cli hosts list` | `gash hte hosts list` |
| `sudo hte-cli hosts sync` | `gash hte hosts apply` |
| `sudo hte-cli hosts add\|remove --domain=D` | None: the block always holds the names of the enabled sites |
| `--hosts=no` | None: every site's names go in the managed block |
| `--interactive`, `--no-normalize`, `-n` | Not needed: a terminal is asked only for a missing required flag |

What changes after the move:

- Sites run under your own user, with no root process, and their files
  live in your XDG directories.
- Forced HTTPS redirects with 307, not 301, and also redirects requests
  from `127.0.0.1` and `::1`. `--force-https-bypass` turns the hte-cli 1.1
  exception for loopback clients back on.
- Certificates are ECDSA P-256 for 10 years, for the domain, `*.domain`
  and every alias. `doctor` notes a pair copied from hte-cli.
- Your old `# hte-cli` lines in `/etc/hosts` stay as they are.

See [`gash hte migrate`](reference.md#gash-hte-migrate).

## Uninstall the stack

```bash
gash hte uninstall --yes
```

`uninstall` stops the stack, removes autostart, and restores what install
changed, as its receipt records it: the hosts block, the port setting,
linger, the hosts helper and its sudoers line, the macOS port redirect.
Your sites, their certificates and settings move to an
`hte-stage-TIMESTAMP` directory, which a later `install` restores
(`--restore-stage PATH` names one, `--no-restore` skips it). Document roots
are never touched. Run it before `gash_uninstall` when you remove Gash.

See [`gash hte uninstall`](reference.md#gash-hte-uninstall).

## macOS

Apache and PHP-FPM come from Homebrew:

```bash
brew install httpd php
gash hte install
```

On the standard ports, install binds 80 and 443 directly when macOS allows
it. When it does not, the stack listens on 8080 and 8443, and install adds
a `pf` anchor that redirects ports 80 and 443 of the loopback to them,
checked with `pfctl -n` first and loaded at boot by a LaunchDaemon
(`/etc/pf.conf` is never edited). `doctor` checks the redirect, and
`uninstall` removes it. Autostart is a LaunchAgent.

## WSL

On WSL 2 `gash hte` runs inside the Linux distribution as on Linux, with
these differences:

- **Autostart.** When WSL boots with systemd (`[boot] systemd=true` in
  `/etc/wsl.conf`), `install` uses the systemd user unit. Without systemd,
  `--autostart auto` sets up none; `--autostart shell` starts the stack
  from the first interactive shell that loads Gash. Gash never edits
  `/etc/wsl.conf`.
- **Ports.** The same `net.ipv4.ip_unprivileged_port_start` step as on
  Linux.
- **Names.** WSL writes `/etc/hosts` again at every boot; `service start`
  and `doctor` put the Gash block back. Windows has its own hosts file,
  which Gash never edits: `gash hte hosts windows` prints the lines to add
  to `C:\Windows\System32\drivers\etc\hosts` for the names Windows cannot
  resolve (asked through `powershell.exe` when interop is available), and
  `doctor` warns about those names.
- **Browsers.** A Windows browser reaches the sites through WSL's
  localhost forwarding, so its requests come from `127.0.0.1`:
  `--force-https-bypass` would let it use plain HTTP too.

A check after install:

1. `gash hte doctor` reports no failure.
2. `gash hte site create --domain shop.test --docroot public`, then
   `curl -k https://shop.test/` inside WSL answers.
3. `gash hte hosts windows`: add the lines it prints to the Windows hosts
   file (as administrator), then open `https://shop.test/` in a Windows
   browser (it warns about the self-signed certificate).
4. Run `wsl --shutdown` in Windows and open WSL again. With systemd the
   stack is up; with `--autostart shell` it starts with the first shell.
   `gash hte hosts check` exits 0.

## When something goes wrong

Start with `gash hte doctor`: it names the remedy of each failure. The
errors you are likely to meet:

| Error | Meaning and fix |
|---|---|
| `hte_not_installed` | Run `gash hte install` first. |
| `root_not_allowed` | `gash hte` runs as your own user and asks for `sudo` itself when a step needs root. Run it without `sudo` (only `install`, `uninstall`, `migrate` without `--apply`, and `doctor` accept root). |
| `apache_not_found` | Apache, its module directory or `mime.types` is missing. The hint gives the install command (`sudo apt install apache2`, `sudo dnf install httpd mod_ssl mod_http2`, `sudo pacman -S apache`, `brew install httpd`). |
| `php_version_not_installed` | The site asks for a PHP version that has no PHP-FPM here. `gash hte php versions` lists what is installed; install it, or `gash hte site modify --domain D --php X.Y`. |
| `openssl_not_found` | Install openssl with your package manager. |
| `port_in_use` | Another program holds port 80 or 443 (often the system Apache or nginx). `gash hte doctor` names the process. Stop it, migrate its sites, or reinstall with `--ports high`. |
| `privileged_step_required` | A root step did not run: the hosts helper or its sudoers line is missing (run `gash hte install` again), or `sudo` was refused. |
| `config_test_failed`, `reload_failed` | The new Apache or PHP configuration did not pass its test, often because of an overlay file; the previous configuration is still in use. `gash hte service logs` shows why. |
| `confirmation_required` | Add `--yes` to `site remove`, `uninstall`, `migrate --apply` or an `install` that stops the system Apache. |
| `conflicting_options` | Both forms of a flag were given, such as `--https` and `--no-https`. |
| `lock_timeout` | Another `gash hte` command is running; retry when it ends. |
| `invalid_state` | A site file or `hte.toml` does not parse or holds a value Gash refuses. The message names the file to fix by hand. |
| `platform_unsupported` | This system is not one of the supported ones, or a step exists only elsewhere (`hosts windows` outside WSL). |

## See also

- [reference.md](reference.md): every `gash hte` command with all its
  flags.
- [security.md](../security.md#gash-hte): what install changes as root and
  what the hosts helper accepts.
- [Windows and WSL](../windows/README.md): `gash win` and the other WSL
  helpers.
- [requirements.md](../requirements.md): the packages of Apache, PHP-FPM
  and openssl on each system.
