# Local web stack (localweb)

`gash localweb` runs local websites on Apache and PHP-FPM as your own user, with
HTTPS, a name for each site in `/etc/hosts`, and its own PHP settings for
each site. It replaces the PHP tool hte-cli.

The command was `gash hte` until Gash 4.0 (HTE stands for *homelab test environment*, the name of hte-cli): `gash hte` still runs it through 4.x, says on stderr that it is deprecated, and goes away in Gash 5.0. Gash 4.1 gave the stack's files their localweb names; see [Files of Gash 4.0](#files-of-gash-40) for how an existing install moves. One `sudo` at install does the few system steps (the hosts helper and its sudoers line, the low ports, autostart); every other command runs without root. Apache and PHP-FPM are the ones your system's packages provide: Gash installs and compiles nothing.

Platforms: Linux (Debian, Ubuntu, Fedora, the RHEL family, Arch and their
derivatives), macOS with Homebrew, and WSL 2 inside one of those
distributions. Other systems (openSUSE, Alpine) get `platform_unsupported`.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [sudo](../requirements.md#tool-sudo) (feature), for `gash localweb`
- [apache](../requirements.md#tool-apache) (feature), for `gash localweb`
- [php-fpm](../requirements.md#tool-php-fpm) (feature), for `gash localweb` (PHP sites)
- [openssl](../requirements.md#tool-openssl) (feature), for `gash localweb`
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
  `~/.config/gash/localweb`, certificates in `~/.local/share/gash/localweb`, the
  rendered configuration and every log in `~/.local/state/gash/localweb`. Logs
  never go inside a document root.

## Install the stack

Install Apache, PHP-FPM and openssl with your package manager first
(`gash localweb doctor` names what is missing), then, in a terminal:

```bash
gash localweb install
```

`install` prints its plan, runs the system steps in one `sudo` (which asks
for your password), then, as you, writes the configuration, starts the
stack and sets up autostart. Each system step is recorded in a receipt
that `uninstall` plays back. Running `install` again changes only what is
missing.

The options that matter:

- `--ports standard` (the default) serves on ports 80 and 443. On Linux
  install sets `net.ipv4.ip_unprivileged_port_start = 80` in
  `/etc/sysctl.d/50-gash-localweb.conf`, which lets every local user bind the
  ports from 80 to 1023. On a shared machine use `--ports high` instead: 8080
  and 8443, and no port step. On macOS, see [macOS](#macos).
- `--autostart` decides how the stack comes back after a reboot. `auto`
  (the default) is a systemd user unit where systemd runs (install turns
  on linger, so it starts at boot), a LaunchAgent on macOS, and nothing
  elsewhere. `shell` starts the stack in the background from the first
  interactive shell that loads Gash; that start prints nothing and never
  slows or fails the shell, and a failure is logged in
  `~/.local/state/gash/localweb/logs/autostart.log`. `none` sets up nothing.
- `--yes` allows install to stop the system Apache when its plan does that.

On the standard ports, install stops the system Apache only when nothing it
serves would be lost. While an enabled system vhost is neither migrated
nor excluded (on a fresh Debian or Ubuntu the stock `000-default.conf`, on
Fedora mod_ssl's `ssl.conf`), install leaves the stack stopped and names
the `--exclude` to add: see [Move sites from hte-cli](#move-sites-from-hte-cli).

See [`gash localweb install`](reference.md#gash-localweb-install).

## Create a site

```bash
cd ~/projects/shop
gash localweb site create --domain shop.test --docroot public
curl -k https://shop.test/
```

`--domain` and `--docroot` are required (in a terminal a missing one is
asked for); a relative document root is resolved against the current
directory. A flag left out gets its default: HTTPS on, forced HTTPS off,
HTTP/2 on, the `dev` profile, a `www.` alias, and the default PHP version
of `localweb.toml` (else the newest installed one). A value that is not valid is
an error: Gash never puts a default in its place. The site's names go in
the managed block of `/etc/hosts` at once.

| Flag | Effect |
|---|---|
| `--php X.Y` | The PHP version (`gash localweb php versions` lists them) |
| `--profile dev\|staging\|hardened` | The pool's limits: `dev` has few restrictions and high memory limits; `staging` adds `open_basedir` and session hardening; `hardened` also disables `exec`, `proc_open` and the like, which can break Laravel, Composer or PHPUnit |
| `--https`, `--no-https` | Serve HTTPS |
| `--force-https`, `--no-force-https` | Redirect HTTP to HTTPS, with status 307 |
| `--force-https-bypass` | Let plain HTTP through for requests from `127.0.0.1` and `::1` |
| `--alias NAME` | One more name for the site (repeatable); `--no-www` drops the `www.` alias |
| `--http2`, `--no-http2` | Serve HTTP/2 |
| `--extra-http-port N`, `--extra-https-port N` | One more loopback port for the site (repeatable) |

With `--ports high`, the URL carries the port: `https://shop.test:8443/`.
See [`gash localweb site create`](reference.md#gash-localweb-site-create).

## Change, disable and remove sites

```bash
gash localweb site list                                   # domain, PHP, HTTPS, enabled, document root
gash localweb site show --domain shop.test                # settings, aliases, ports, php.ini values, certificate
gash localweb site modify --domain shop.test --php 8.3    # only the flags you pass change
gash localweb site disable --domain shop.test             # stop serving it, keep everything
gash localweb site enable --domain shop.test
gash localweb site remove --domain shop.test --yes
```

`site modify` takes the flags of `site create`. Changing the aliases makes
a new certificate; `--no-https` keeps the certificate, so that `--https`
later reuses it. `site remove` deletes the site's state, rendered
configuration, pool, certificate and hosts names; it never touches the
document root, the site's overlay file or its logs.

Hand edits go in overlay files, which Gash includes and never rewrites:
`~/.config/gash/localweb/sites/DOMAIN.apache.conf` for one site, and
`~/.config/gash/localweb/apache.conf` for the whole server
(`gash localweb apache config path` prints it). Run `gash localweb service reload`
after editing them. `gash localweb apache modules list` shows the modules the
stack loads; `enable NAME` and `disable NAME` add or drop one from the
system module directory.

See [`gash localweb site`](reference.md#gash-localweb-site) and
[`gash localweb apache`](reference.md#gash-localweb-apache).

## Set PHP versions and php.ini values

```bash
gash localweb php versions                                              # the PHP-FPM versions found
gash localweb php ini set memory_limit 512M --domain shop.test          # one site
gash localweb php ini get memory_limit --domain shop.test
gash localweb php global set upload_max_filesize 64M --version 8.3      # every site on PHP 8.3
gash localweb php global unset upload_max_filesize --version 8.3
```

`php ini` values belong to one site and are written into its pool.
`php global` values go in an overlay file of one PHP version, read by every
site of that version. Each change reloads what it affects; after a
`php global` change, a PHP-FPM master that does not come back gets the
previous file. Gash finds the versions the
packages install: Debian's `php-fpmX.Y` (also from the sury/ondrej
repositories), Fedora's and Remi's, Arch's (and the AUR ones), and
Homebrew's `php@X.Y`.

See [`gash localweb php`](reference.md#gash-localweb-php).

## Run the stack

```bash
gash localweb service status                  # the masters, and whether each site answers
gash localweb service start
gash localweb service stop
gash localweb service restart
gash localweb service reload                  # render, test, reload; the old configuration comes back on failure
gash localweb service logs --site shop.test   # that site's access, error and PHP logs
```

With the systemd user unit, start, stop and restart go through the unit.
`service logs` prints the last lines (`--lines N`, default 20) and has no
follow mode.

See [`gash localweb service`](reference.md#gash-localweb-service).

## Certificates and site names

```bash
gash localweb cert show --domain shop.test    # names, validity, fingerprint
gash localweb cert renew --domain shop.test   # a new certificate, then a reload
gash localweb hosts list                      # the managed block of /etc/hosts
gash localweb hosts check                     # exit 1 when the block and the sites differ
gash localweb hosts apply                     # write the block again
```

The names live in `/etc/hosts` between `# >>> gash localweb >>>` and `# <<< gash localweb <<<`, as `127.0.0.1` lines; lines outside the block are never changed (a block between the `gash hte` markers of Gash 4.0 is read too, and gets the new markers once `install` has put the current hosts helper in place). Every site change, `service start` and `doctor` write the block again when it drifted. They do it through the hosts helper, the one program install lets your user run as root: it writes only that block of `/etc/hosts`, and only loopback addresses and plain host names.

See [`gash localweb cert`](reference.md#gash-localweb-cert) and
[`gash localweb hosts`](reference.md#gash-localweb-hosts).

## Check everything

```bash
gash localweb doctor
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
gash localweb migrate                      # the plan: what moves, what is skipped and why
gash localweb migrate --apply --yes        # move the sites, stop the system Apache
```

`--site D` migrates only that domain and `--exclude D|PATH` leaves a domain
or a vhost file out (both repeatable). Each old site keeps its forced-HTTPS
behavior. Certificates are copied (`--reissue-certs` makes new ones
instead; `cert renew` replaces a copied pair later), and
`--keep-pool-values` keeps the PHP values of the old pool.

When `install` stopped because of a system vhost you do not need,
`gash localweb migrate --apply --exclude PATH --yes` stops and disables the
system Apache and starts the stack on 80 and 443, also when there is
nothing to import. The excluded file stays as it is, and
`gash localweb uninstall` starts the system Apache again.

There is no `hte-cli` compatibility command. The old commands map as
follows:

| hte-cli 1.1 | gash localweb |
|---|---|
| `sudo hte-cli create --domain=D --docroot=P --phpver=X.Y --ssl=y --forcessl=n --profile=dev` | `gash localweb site create --domain D --docroot P --php X.Y --https --no-force-https --profile dev` |
| `sudo hte-cli modify --domain=D ...` | `gash localweb site modify --domain D ...` (only the flags you pass change) |
| `sudo hte-cli remove --domain=D --force` | `gash localweb site remove --domain D --yes` |
| `sudo hte-cli details [--domain=D]` | `gash localweb site list`, `gash localweb site show --domain D` |
| `sudo hte-cli fix [--domain=D] [--dry-run]` | `gash localweb migrate`, then `gash localweb migrate --apply --yes` (imported sites already have the fixed configuration) |
| `sudo hte-cli hosts list` | `gash localweb hosts list` |
| `sudo hte-cli hosts sync` | `gash localweb hosts apply` |
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

See [`gash localweb migrate`](reference.md#gash-localweb-migrate).

## Files of Gash 4.0

Gash 4.0 and older named the stack's files after hte. Gash 4.1 moves them to the localweb names, in two steps.

- Your files move at the first `gash localweb` command you run after the update (any command but `doctor`): `~/.config/gash/hte` becomes `~/.config/gash/localweb` (with `hte.toml` renamed `localweb.toml`), and the same for `~/.local/state/gash/hte` and `~/.local/share/gash/hte`. A running stack stops, moves and starts again from the new directories; the systemd user unit `gash-hte.service` becomes `gash-localweb.service` (the macOS LaunchAgent likewise). Raw sites imported from hte-cli are rewritten to name the new directories. Your own overlay files are never edited: the command names any that still mention an old directory. When the move cannot finish, everything stays where it was and the stack runs as before.
- The system files move at the next `gash localweb install`: the receipt in `/var/lib/gash-hte` goes to `/var/lib/gash-localweb`, with the backups and archives it names, `/etc/sudoers.d/gash-hte` becomes `/etc/sudoers.d/gash-localweb`, `/etc/sysctl.d/50-gash-hte.conf` becomes `/etc/sysctl.d/50-gash-localweb.conf`, the macOS pf anchor `com.apple/gash-hte` becomes `com.apple/gash-localweb`; install then puts the current hosts helper in place, and the hosts block gets the new markers. The move is the first step of install's plan, shown before `sudo` asks. Until then everything keeps working under the old names, `uninstall` included, and `doctor` reports them. An install interrupted half way leaves a receipt that names every file where it is: running `install` again finishes the move, and `uninstall` works from it too.

The autostart units and scripts of Gash 4.0 call `gash hte __unit` and `gash hte __root`: those keep working through 4.x.

## Uninstall the stack

```bash
gash localweb uninstall --yes
```

`uninstall` stops the stack, removes autostart, and restores what install changed, as its receipt records it: the hosts block, the port setting, linger, the hosts helper and its sudoers line, the macOS port redirect. Your sites, their certificates and settings move to a `localweb-stage-TIMESTAMP` directory, which a later `install` restores (`--restore-stage PATH` names one, `--no-restore` skips it). Document roots are never touched. Run it before `gash uninstall` when you remove Gash.

See [`gash localweb uninstall`](reference.md#gash-localweb-uninstall).

## macOS

Apache and PHP-FPM come from Homebrew:

```bash
brew install httpd php
gash localweb install
```

On the standard ports, install binds 80 and 443 directly when macOS allows
it. When it does not, the stack listens on 8080 and 8443, and install adds
a `pf` anchor that redirects ports 80 and 443 of the loopback to them,
checked with `pfctl -n` first and loaded at boot by a LaunchDaemon
(`/etc/pf.conf` is never edited). `doctor` checks the redirect, and
`uninstall` removes it. Autostart is a LaunchAgent.

## WSL

On WSL 2 `gash localweb` runs inside the Linux distribution as on Linux, with
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
  which Gash never edits: `gash localweb hosts windows` prints the lines to add
  to `C:\Windows\System32\drivers\etc\hosts` for the names Windows cannot
  resolve (asked through `powershell.exe` when interop is available), and
  `doctor` warns about those names.
- **Browsers.** A Windows browser reaches the sites through WSL's
  localhost forwarding, so its requests come from `127.0.0.1`:
  `--force-https-bypass` would let it use plain HTTP too.

A check after install:

1. `gash localweb doctor` reports no failure.
2. `gash localweb site create --domain shop.test --docroot public`, then
   `curl -k https://shop.test/` inside WSL answers.
3. `gash localweb hosts windows`: add the lines it prints to the Windows hosts
   file (as administrator), then open `https://shop.test/` in a Windows
   browser (it warns about the self-signed certificate).
4. Run `wsl --shutdown` in Windows and open WSL again. With systemd the
   stack is up; with `--autostart shell` it starts with the first shell.
   `gash localweb hosts check` exits 0.

## When something goes wrong

Start with `gash localweb doctor`: it names the remedy of each failure. The
errors you are likely to meet:

| Error | Meaning and fix |
|---|---|
| `localweb_not_installed` | Run `gash localweb install` first (`hte_not_installed` before Gash 4.1). |
| `root_not_allowed` | `gash localweb` runs as your own user and asks for `sudo` itself when a step needs root. Run it without `sudo` (only `install`, `uninstall`, `migrate` without `--apply`, and `doctor` accept root). |
| `apache_not_found` | Apache, its module directory or `mime.types` is missing. The hint gives the install command (`sudo apt install apache2`, `sudo dnf install httpd mod_ssl mod_http2`, `sudo pacman -S apache`, `brew install httpd`). |
| `php_version_not_installed` | The site asks for a PHP version that has no PHP-FPM here. `gash localweb php versions` lists what is installed; install it, or `gash localweb site modify --domain D --php X.Y`. |
| `openssl_not_found` | Install openssl with your package manager. |
| `port_in_use` | Another program holds port 80 or 443 (often the system Apache or nginx). `gash localweb doctor` names the process. Stop it, migrate its sites, or reinstall with `--ports high`. |
| `privileged_step_required` | A root step did not run: the hosts helper or its sudoers line is missing (run `gash localweb install` again), or `sudo` was refused. |
| `config_test_failed`, `reload_failed` | The new Apache or PHP configuration did not pass its test, often because of an overlay file; the previous configuration is still in use. `gash localweb service logs` shows why. |
| `confirmation_required` | Add `--yes` to `site remove`, `uninstall`, `migrate --apply` or an `install` that stops the system Apache. |
| `conflicting_options` | Both forms of a flag were given, such as `--https` and `--no-https`. |
| `lock_timeout` | Another `gash localweb` command is running; retry when it ends. |
| `invalid_state` | A site file or `localweb.toml` does not parse or holds a value Gash refuses. The message names the file to fix by hand. |
| `platform_unsupported` | This system is not one of the supported ones, or a step exists only elsewhere (`hosts windows` outside WSL). |

## See also

- [reference.md](reference.md): every `gash localweb` command with all its
  flags.
- [security.md](../security.md#gash-localweb): what install changes as root and
  what the hosts helper accepts.
- [Windows and WSL](../windows/README.md): `gash win` and the other WSL
  helpers.
- [requirements.md](../requirements.md): the packages of Apache, PHP-FPM
  and openssl on each system.
