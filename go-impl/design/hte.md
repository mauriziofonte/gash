<!-- Gash 3.0 design: HTE userland. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# HTE: userland virtual hosts (`gash hte`)

Status: design for stop S1. Replaces the separate PHP tool hte-cli with
`gash hte`. New design: functional parity on the operations, the behaviors
listed in PRD 6.5 kept, the hte-cli defects of PRD 6.7 not carried. Every
intentional difference from hte-cli is listed in `go-impl/PORT/LEDGER.md`,
section "HTE design delta", for triage.

## 1. Model

| Fact | Decision |
|---|---|
| Who runs the stack | The user. One Apache master (`apache2`/`httpd` from the distro or Homebrew) and one `php-fpm` master per PHP version in use, all started by `gash hte service start` as the user. No root process at runtime |
| What is reused | System binaries and modules: Debian/Ubuntu `apache2` + `/usr/lib/apache2/modules`, Fedora `httpd` + `/usr/lib64/httpd/modules`, Arch `httpd` + `/usr/lib/httpd/modules`, Homebrew `httpd` + `$(brew --prefix)/lib/httpd/modules`; versioned `php-fpm` binaries. Nothing is compiled |
| What is rendered | Apache and FPM config files, from declarative per-site state. Rendered files are artifacts, never edited by hand and never the source of truth |
| Where user edits go | Per-site and global overlay files that the renderer includes and never rewrites (fixes "modify loses manual edits") |
| Privilege | Root only in `install`, `uninstall`, `migrate --apply` (interactive sudo) and in the hosts helper (the only NOPASSWD entry). See section 7 |
| Change protocol | Render into a new generation, config test (`httpd -t`, `php-fpm -t`), swap, graceful reload, health check, automatic rollback to the previous generation on any failure |
| Concurrency | Every mutation takes an exclusive lock (`github.com/gofrs/flock`) on `$XDG_STATE_HOME/gash/hte/lock`; reads take none. Lock wait 30 s, then `lock_timeout` (RETRY) |
| Audit | Every mutation appends one JSON line to `$XDG_STATE_HOME/gash/hte/audit.jsonl`: `{"ts","command","site","outcome","details"}` |

The spike in the S1 session proved the model on Ubuntu 24.04: `apache2 -f
<user config> -k start` as an unprivileged user, master and workers owned by
the user, bound to `127.0.0.1:80` and `:443`, HTTP/2 over TLS, `proxy_fcgi` to
a user-owned PHP 8.3 FPM pool, `-k graceful` and FPM `SIGUSR2` reloads, and
stop. Debian's `apache2` builds `mod_log_config`, `mod_logio`, `mod_version`,
`mod_unixd`, `mod_so` and `mod_watchdog` in: the renderer reads `httpd -l`
and emits `LoadModule` only for modules that are not built in.

## 2. Files

| Kind | Path | Notes |
|---|---|---|
| Global settings | `$XDG_CONFIG_HOME/gash/hte/hte.toml` | Ports, listen addresses, default PHP, enabled extra modules, autostart mode |
| Site state | `$XDG_CONFIG_HOME/gash/hte/sites/<domain>.toml` | One file per site; the domain is the identity (no numbering, no identity from socket names) |
| Site Apache overlay | `$XDG_CONFIG_HOME/gash/hte/sites/<domain>.apache.conf` | Optional, included inside every vhost of the site, never rewritten |
| Global Apache overlay | `$XDG_CONFIG_HOME/gash/hte/apache.conf` | Optional, included at server level, never rewritten |
| Global PHP overlay | `$XDG_CONFIG_HOME/gash/hte/php/<X.Y>.ini` | Per PHP version, rendered into every pool of that version |
| Rendered config | `$XDG_STATE_HOME/gash/hte/render/gen-<n>/` and the symlink `render/current` | Generations; `current` swaps atomically; the previous generation is kept for rollback |
| Certificates | `$XDG_DATA_HOME/gash/hte/certs/<domain>/{cert.pem,key.pem}` | Key 0600, directory 0700 |
| Logs | `$XDG_STATE_HOME/gash/hte/logs/` (`error.log`, `<domain>/access.log`, `<domain>/error.log`, `<domain>/php-error.log`, `php-fpm-<X.Y>.log`) | Never inside a document root |
| Runtime | `$XDG_RUNTIME_DIR/gash-hte/`, else `/tmp/gash-hte-<uid>/` (created 0700, owner and non-symlink checked) | PID files, FPM sockets, mutexes, TLS session cache. Socket names are short (`<X.Y>-<10 hex of sha256(domain)>.sock`) to stay under the 104-byte `sun_path` limit of macOS |
| Receipt | `$XDG_STATE_HOME/gash/hte/receipt.json`, root-owned copy `/var/lib/gash-hte/receipt-<user>.json` | Section 8 |
| Audit log | `$XDG_STATE_HOME/gash/hte/audit.jsonl` | |
| Lock | `$XDG_STATE_HOME/gash/hte/lock` | |
| Uninstall stage | `$XDG_STATE_HOME/gash/hte-stage-<UTC timestamp>/` | Section 9 |

Paths are resolved by `internal/platform` from the environment snapshot
with the XDG rules (`github.com/adrg/xdg` only cross-checks it in a Linux
test: on macOS the library would answer `~/Library`); macOS uses the same
XDG layout (`~/.config`, `~/.local/state`, `~/.local/share`), so the tree is
identical on every platform and in tests.

Site state (TOML, `github.com/BurntSushi/toml`):

```toml
domain      = "shop.test"          # identity, immutable (rename = create + remove)
aliases     = ["www.shop.test"]    # www. alias by default; --no-www drops it
docroot     = "/home/me/src/shop/public"   # absolute, must exist, never created or modified
php         = "8.3"                # an installed version (section 4)
profile     = "dev"                # dev | staging | hardened (values of hte-cli PhpProfiles.php)
https       = true
force_https = false
force_https_bypass_loopback = false   # true: no redirect for 127.0.0.1 and ::1 (hte-cli 1.1.x behavior, D-2)
http2       = true
enabled     = true
kind        = "managed"            # managed | raw (imported non-HTE vhost, section 10)
raw_conf    = ""                   # kind = raw: the imported vhost file, kept verbatim after path rewriting
[php_ini]                          # per-site overrides, rendered as php_value / php_admin_value
memory_limit = "512M"
```

Invalid values never fall back to defaults (no "current directory", "newest
PHP" or "dev profile" fallback): they fail with the error contract.

## 3. Apache rendering

Server level (`render/current/httpd.conf`), from `text/template` with golden
files:

* `ServerRoot`, `PidFile`, `DefaultRuntimeDir`, `Mutex file:` in the runtime
  directory; global `ErrorLog` in the logs directory; `LogLevel warn`.
* `LoadModule` lines for the userland module list minus the built-in ones:
  `mpm_event`, `authn_core`, `authn_file`, `authz_core`, `authz_host`,
  `authz_user`, `auth_basic`, `access_compat` (legacy `order`/`allow`),
  `alias`, `dir`, `env`, `filter`, `deflate`, `brotli`, `headers`, `mime`,
  `negotiation`, `reqtimeout`, `setenvif`, `rewrite`, `proxy`, `proxy_fcgi`,
  `proxy_http` (reverse-proxy vhosts imported as raw), `socache_shmcb`,
  `ssl`, `http2`, `log_config`/`logio` where not built in. `gash hte apache
  modules enable NAME` adds a module from the system module directory only;
  the list is validated against the files that exist there.
* `Listen 127.0.0.1:<http>` and `Listen 127.0.0.1:<https>`, plus `[::1]` when
  IPv6 loopback is available. Never a wildcard address.
* TLS defaults equivalent to Debian `ssl.conf` (`SSLProtocol all -SSLv3
  -TLSv1 -TLSv1.1`, `SSLCipherSuite HIGH:!aNULL`, `SSLHonorCipherOrder off`,
  `SSLSessionCache shmcb:<run>/ssl_scache(512000)`, `SSLSessionCacheTimeout
  300`) and `security.conf` (`ServerTokens`, `ServerSignature`, `TraceEnable
  Off`), `Protocols h2 h2c http/1.1`, `TypesConfig` pointing at the
  platform's `mime.types`, `AddDefaultCharset` as Debian ships it.
* The Debian `apache2.conf` and `dir.conf` defaults the sites rely on, on
  every platform: `<Directory "/"> AllowOverride None` + `Require all
  denied`, `<FilesMatch "^\.ht"> Require all denied` (so `.htaccess` and
  `.htpasswd` are never served), and `DirectoryIndex index.html index.cgi
  index.pl index.php index.xhtml index.htm` (LEDGER HTE-59).
* Debian's five `LogFormat` lines (`vhost_combined`, `combined`, `common`,
  `referer`, `agent`; the vhosts use `combined`), and `unixd` in the module
  list (built into Debian's `apache2`, a loadable module elsewhere). The TLS
  directives are not wrapped in `<IfModule mod_ssl.c>`: `ssl` is in the
  list, so a missing module fails the start instead of silently serving no
  HTTPS. A `Listen` on a TLS port other than 443 names the protocol
  (`Listen 127.0.0.1:8443 https`).
* `IncludeOptional` of the global overlay, then of every enabled site's
  rendered vhosts.

Per managed site (`render/current/sites/<domain>.conf`). The directives are
hte-cli 1.1.1's (`ApacheManager.php:386-497`, fixture
`templates/v2-vhost-forced.conf`) minus its defects: no Apache 2.2 `order`/
`allow`, no weak `SSLCipherSuite` (server-level TLS defaults instead), quoted
paths, logs outside the document root, 307 instead of 301:

```apache
<VirtualHost 127.0.0.1:80>
    # Managed by gash hte. Edit the site with `gash hte site modify`, or add directives to the overlay below.
    ServerName shop.test
    ServerAlias www.shop.test
    DocumentRoot "/home/me/src/shop/public"
    ServerAdmin admin@localhost.net
    UseCanonicalName Off
    Options -ExecCGI -Includes
    RemoveHandler cgi-script .cgi .pl .plx .ppl .perl
    ErrorLog "/home/me/.local/state/gash/hte/logs/shop.test/error.log"
    CustomLog "/home/me/.local/state/gash/hte/logs/shop.test/access.log" combined
    CustomLog "/home/me/.local/state/gash/hte/logs/shop.test/bytes.log" "%{%s}t %I .\n%{%s}t %O ."

    Protocols h2c http/1.1

    <FilesMatch \.php$>
        SetHandler "proxy:unix:/run/user/1000/gash-hte/8.3-1a2b3c4d5e.sock|fcgi://localhost/"
    </FilesMatch>

    <IfModule mod_brotli.c>
        AddOutputFilterByType BROTLI_COMPRESS text/html text/plain text/xml text/css text/javascript application/x-javascript application/javascript application/json application/x-font-ttf application/vnd.ms-fontobject image/x-icon
    </IfModule>

    <Directory "/home/me/src/shop/public/">
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    # force_https = true:
    #   RewriteEngine On
    #   RewriteCond %{HTTPS} off
    # force_https_bypass_loopback = true adds (requests from 127.0.0.1 and ::1 are not redirected, as in hte-cli 1.1.x):
    #   RewriteCond %{REMOTE_ADDR} !^127\.0\.0\.1$
    #   RewriteCond %{REMOTE_ADDR} !^::1$
    #   RewriteRule ^/(.*) https://%{HTTP_HOST}%{REQUEST_URI} [R=307,L]

    IncludeOptional "/home/me/.config/gash/hte/sites/shop.test.apache.conf"
</VirtualHost>
```

The redirect target is `https://%{HTTP_HOST}%{REQUEST_URI}` when clients
reach HTTPS on 443 (standard ports, and the macOS `pf` redirect); with any
other HTTPS port (`install --ports high`) it is
`https://%{SERVER_NAME}:<https port>%{REQUEST_URI}`, since `HTTP_HOST`
carries the HTTP port.

The HTTPS vhost is the same block on `127.0.0.1:443` with hte-cli's
`<FilesMatch "\.(cgi|shtml|phtml|php)$"> SSLOptions +StdEnvVars`, its
`ssl-` log names and HTTPS bytes-log format, `SSLEngine on` and the site's
`SSLCertificateFile`/`SSLCertificateKeyFile`. When IPv6 loopback exists both
vhosts also answer on `[::1]`, so the `::1` bypass condition, when enabled,
is live (in hte-cli it was dead code). Every path written into a config file is quoted
and escaped by one function (`quoteApacheArg`: `"` and `\` escaped; any
control character, newline included, is rejected at validation), so a
document root with spaces works and a newline cannot inject configuration
(hte-cli defect). `${APACHE_LOG_DIR}` is never used: the userland master has
no Debian `envvars`.

`Protocols` per vhost: `h2c http/1.1` on the HTTP vhost, `h2 http/1.1` on the
HTTPS vhost, `http/1.1` in both when `http2 = false` (LEDGER HTE-38).
Document roots are validated before anything is rendered: absolute after
resolving a relative argument against the current directory (HTE-29), an
existing directory, and none of `$ * ? [ ; : " \` or control characters, which
Apache (`${NAME}` expansion, `<Directory>` wildcards) or the FPM INI parser
(`;` comments, `${NAME}`) would interpret, or that would split the
`open_basedir` list (`:`, its separator) (`invalid_docroot`, hint: "rename
the directory or use a symlink without these characters"; HTE-08). Pool
values are double-quoted.

Force-HTTPS and the loopback bypass (decision D-2, option C; LEDGER HTE-52):
with loopback-only binding every client address is `127.0.0.1` or `::1`,
also for a Windows browser reaching WSL2 through localhost forwarding, so the
hte-cli bypass would make force-HTTPS redirect nothing. The bypass is
therefore a per-site setting, `force_https_bypass_loopback`, off by default:
force-HTTPS redirects every plain-HTTP request, and a site that needs plain
HTTP for local tools turns the bypass on (the PRD 6.5 behavior). `migrate`
keeps each legacy site's behavior: V1 sites import with the bypass off (they
redirected everything), V1-fixed, V2 and Broken-bypass sites with it on.

`site create`, `site modify`, `site show`, `doctor` and `migrate` warn
(stderr, and a `warnings` array in `--json`) when the document root's
`.htaccess` has its own HTTPS redirect: a `RewriteRule` or `Redirect` to
`https://`, including the `%{HTTP:X-Forwarded-Proto}` form that hte-cli's
regex missed (hte-cli warned only in `fix`).

### 3.1 Certificates

One self-signed certificate per site, SAN `<domain>` and `*.<domain>` (which
covers `www.`), CN `<domain>`, generated in-process with `crypto/x509` and
`crypto/ecdsa` (P-256; hte-cli: RSA 2048 through a generated script run as
root from `/tmp`), validity 10 years (hte-cli: 30), `basicConstraints
CA:FALSE`, `keyUsage digitalSignature`, `extKeyUsage serverAuth`. The key is
written once (0600, in a 0700 directory, atomic rename) and never read back
or parsed by gash; the pair is validated by Apache's config test. No
passphrase (a userland master cannot prompt; hte-cli's passphrase was fake
anyway, `-nodes`). `cert renew` replaces both files and reloads; `doctor`
warns 30 days before expiry. No local CA, no mkcert (PRD 2). Each site's
files live in their own directory, so no operation can touch another site's
certificate (hte-cli's `rm -rf <dir>/<domain>.*` deleted `shop.test.it`'s
files when creating `shop.test`).

## 4. PHP-FPM

* Discovery of installed versions 5.6 to 8.5 (the hte-cli list): a version
  counts only when its FPM binary runs (`<bin> -v` answers that version) and
  its php.ini exists. Probed layouts: Debian/Ubuntu `/usr/sbin/php-fpmX.Y` +
  `/etc/php/X.Y/fpm/php.ini` (distro and sury/ondrej); Fedora distro
  `/usr/sbin/php-fpm` + `/etc/php.ini`, Remi side-by-side
  `/opt/remi/phpXY/root/usr/sbin/php-fpm` + `/etc/opt/remi/phpXY/php.ini`;
  Arch `/usr/bin/php-fpm` (+ `/etc/php/php.ini`) and `/usr/bin/php-fpm-legacy`
  (+ `/etc/php-legacy/php.ini`), AUR `php-fpmXY` names; Homebrew
  `<prefix>/opt/php@X.Y/sbin/php-fpm` and `<prefix>/opt/php/sbin/php-fpm` +
  `<prefix>/etc/php/X.Y/php.ini`. Results are cached in the state directory,
  keyed by binary path, size and mtime. hte-cli counted a bare `/etc/php/X.Y`
  directory as installed, so a CLI-only version became the default and its
  FPM restart failed (design delta).
* Installing PHP versions or extensions is out of scope: `doctor` reports what
  is missing and prints the platform's install command.
* One pool per site, `render/current/fpm/<X.Y>/pools/<domain>.conf`:

  ```ini
  [shop.test]
  ; Managed by gash hte. Profile: dev
  listen = /run/user/1000/gash-hte/8.3-1a2b3c4d5e.sock
  listen.mode = 0660
  security.limit_extensions = .php
  ; ...the profile block, verbatim from hte-cli (below)...
  slowlog = /home/me/.local/state/gash/hte/logs/shop.test/php-slow.log
  php_admin_value[error_log] = /home/me/.local/state/gash/hte/logs/shop.test/php-error.log
  ; ...then the global overlay of 8.3, then the site's [php_ini]
  ```

  No `user`, `group`, `listen.owner` or `listen.group`: the master runs as the
  user, so the socket and the workers are the user's (php-fpm ignores those
  directives when it is not root). hte-cli wrote the invoking user there,
  `root` when `modify` ran as root, which php-fpm refused (hte-cli defect).
* Profiles `dev`, `staging`, `hardened` carry the exact values of hte-cli
  `PhpProfiles.php` (unchanged since 1.1.0), copied into
  `internal/hte/profiles` with a golden test against the rendered hte-cli
  pools (`testdata/hte/templates/v2-pool-*.conf`):

  | Setting | dev | staging | hardened |
  |---|---|---|---|
  | `php_admin_value[disable_functions]` | `apache_child_terminate,apache_get_modules,apache_getenv,apache_note,apache_setenv` | dev + `dl,highlight_file,show_source,phpinfo` | staging + `exec,shell_exec,system,passthru,popen,proc_open,proc_close,proc_get_status,proc_terminate,pcntl_exec,pcntl_fork,pcntl_signal,symlink` |
  | `php_admin_flag[allow_url_fopen]` | off | off | off |
  | `pm` / `max_children` / `start_servers` / `min_spare_servers` / `max_spare_servers` | dynamic / 5 / 2 / 1 / 3 | same | same |
  | `pm.max_requests` | (none) | 500 | 500 |
  | `chdir`, `catch_workers_output`, `request_terminate_timeout` | `/`, yes, 180s | same | same |
  | `php_flag[display_errors]`, `php_admin_flag[log_errors]` | off, on | same | same |
  | `post_max_size`, `upload_max_filesize` (admin) | 128M | 128M | 32M |
  | `memory_limit` (admin and plain value) | 1024M | 1024M | 256M |
  | `php_value[short_open_tag]` | On | On | On |
  | `php_admin_value[open_basedir]` | (none) | `<docroot>:/var/lib/php:/usr/lib/php:/usr/share/php:/tmp:/dev/urandom` | same |
  | `allow_url_include`, `session.cookie_httponly`, `session.use_strict_mode`, `session.use_only_cookies` (admin flags) | (none) | off, on, on, on | same |
  | `auto_prepend_file`, `auto_append_file` (admin values) | (none) | (none) | `""`, `""` |
  | `php_admin_flag[expose_php]`, `max_execution_time`, `max_input_time` | (none) | (none) | off, 30, 60 |

  Rendering order and the `php_admin_flag`/`php_admin_value` choice follow
  `PhpProfiles.php:233-246`. Two changes, both in the LEDGER design delta:
  the `slowlog` and `error_log` paths move to the logs directory (hte-cli put
  them in the document root, where they could be served), and when the
  platform's default `session.save_path` is not writable by the user (Fedora's
  `/var/lib/php/session` is group `apache`), the pool sets a per-version
  directory under `$XDG_STATE_HOME/gash/hte/php/<X.Y>/sessions` and appends it
  to `open_basedir`. `slowlog` stays without `request_slowlog_timeout`, as in
  hte-cli (an inert setting, listed as a decision item).
* The global FPM master per version, `render/current/fpm/<X.Y>/php-fpm.conf`:
  `[global]` with `pid`, `error_log` in the logs directory, `daemonize = yes`,
  then `include=` of that version's pools. It uses the platform's `php.ini`
  for that version (discovered above) and the global overlay
  `$XDG_CONFIG_HOME/gash/hte/php/<X.Y>.ini`, passed with `-c`/`PHP_INI_SCAN_DIR`
  so the system files are read, never written.

## 5. Lifecycle and supervision

The masters daemonize themselves; gash starts, signals and checks them through
PID files and never runs a supervisor process of its own.

| Command | Does |
|---|---|
| `service start` | Render if stale, config test, start each FPM master in use, then Apache; wait until the sockets and ports answer |
| `service stop` | `SIGTERM` (Apache `-k stop`), wait, `SIGKILL` after a timeout. A PID is acted on only when `/proc/<pid>/cmdline` (Linux) or `ps -o command= -p` (macOS) names our config path, so a recycled PID is never signalled |
| `service restart` | stop + start |
| `service reload` | Render, test, swap, FPM `SIGUSR2` first (wait until every socket of the new generation accepts), then Apache `-k graceful`, health check; rollback on failure. On a site removal the order is reversed: Apache first, then FPM (HTE-27) |
| `service status` | Per master: running, PID, uptime, listening addresses; per site: answers on HTTP and HTTPS. `--json` |
| `service logs` | Tail of the global, per-site Apache and PHP logs (`--site`, `--lines N`; no follow mode, so agents never hang) |

Autostart, chosen by `install` and recorded in the receipt:

| Platform | Mechanism |
|---|---|
| Linux with systemd | User unit `gash-hte.service` (`Type=oneshot`, `RemainAfterExit=yes`, `ExecStart=gash hte service start`, `ExecStop=gash hte service stop`), `systemctl --user enable`, plus `loginctl enable-linger <user>` (root step) so it starts at boot |
| Linux without systemd (containers, WSL without systemd) | No boot hook (`/etc/wsl.conf` is never touched). Optional `autostart = "shell"`: the first interactive shell that loads `gash init bash` starts the stack when it is down (one background `gash hte service start`, no output on success) |
| macOS | User LaunchAgent `~/Library/LaunchAgents/io.github.mauriziofonte.gash.hte.plist` with `RunAtLoad` |

## 6. Ports 80/443 without root at runtime

| Platform | Mechanism (recorded in the receipt, reverted by uninstall) |
|---|---|
| Linux, WSL2 | `net.ipv4.ip_unprivileged_port_start=80` via `/etc/sysctl.d/50-gash-hte.conf` plus `sysctl -w`. The setting applies to IPv4 and IPv6. When the current value is already 80 or lower (Docker containers default to 0) nothing changes and the receipt says so. Trade-off, stated by `install` before asking: every local user may then bind ports 80 to 1023 |
| macOS | First choice: a direct bind, if the running kernel lets an unprivileged process bind `127.0.0.1:80` (checked by `install` with a real bind). Otherwise a `pf` anchor redirecting `127.0.0.1:80`/`:443` on `lo0` to the userland ports (`8080`/`8443` by default), loaded at boot by a root LaunchDaemon that only runs `pfctl -a <anchor> -f <root-owned file>` |
| Any, when declined or impossible | High ports (`8080`/`8443`, configurable), reported by `doctor`; site URLs carry the port |

Rejected: `setcap cap_net_bind_service` on the distro binary (modifies a
package-owned file, lost on upgrade, also grants the system Apache), authbind
(Debian only), `APACHE_RUN_USER` on the system Apache (the master still starts
and parses its config as root; PRD 6.1).

The macOS mechanism is to be verified on the GitHub macOS runner during the
HTE phase; the design supports both outcomes.

## 7. Privilege boundary

Root is used only by:

1. `install`, `uninstall`, `migrate --apply`: one interactive `sudo
   <gash binary> hte __root <op>` per command. The operations are a closed,
   typed set (install helper, write sudoers drop-in, set sysctl, write pf
   anchor, enable linger, stop/disable/enable/start a named system unit,
   disable/enable/archive a legacy vhost or pool, read a legacy key file into
   the user's certificate directory, write the root receipt), each validated
   against the plan the user saw. Nothing generated is executed as root from
   `/tmp` (hte-cli defect).
2. The hosts helper `gash-hosts-helper` (section 7.1).

### 7.1 Hosts helper and sudoers drop-in

* Binary: `/usr/local/libexec/gash/gash-hosts-helper`, root:root 0755, built
  as its own small `main` package (no cobra, no network, no DB drivers):
  `cmd/gash-hosts-helper`.
* Interface: `gash-hosts-helper apply` reads the desired block from stdin;
  `gash-hosts-helper check` prints the current block. Nothing else.
* Input grammar: at most 4096 lines, each `ADDRESS NAME...` where ADDRESS is
  exactly `127.0.0.1` or `::1` and each NAME is a lowercase hostname (labels
  `[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?`, at least two labels, 253 bytes at
  most, no wildcard, no trailing dot). Any other byte, line or address: exit
  2, nothing written.
* Effect: replaces only the region between `# >>> gash hte >>>` and `# <<<
  gash hte <<<` in `/etc/hosts` (appended when absent), written to a temporary
  file in `/etc`, fsynced, renamed over the original with its owner and mode.
  On macOS it then flushes the resolver cache with two fixed commands.
* Sudoers drop-in `/etc/sudoers.d/gash-hte`, 0440 root:root, one line:
  `<user> ALL=(root) NOPASSWD: /usr/local/libexec/gash/gash-hosts-helper apply`
  (and the same for `check`). It names the user, not a group, and exact
  arguments. It is written to a temporary file, validated with `visudo -c -f`,
  then renamed into place; `install` refuses to continue if the system sudoers
  does not include `/etc/sudoers.d` and prints the remedy (it never edits
  `/etc/sudoers`).
* Refusal paths are tested (unit tests on the parser, E2E calls with bad
  input, wrong address, wildcard, injected newline, oversize input).

### 7.2 Name resolution

Any TLD is allowed for local sites. gash keeps the desired hosts block in its
state and applies it through the helper on every site change, on `service
start` and on `doctor`.

| Platform | Mechanism |
|---|---|
| Linux, macOS | Hosts block through the helper |
| WSL2 | Same inside WSL. WSL regenerates `/etc/hosts` at boot: `service start` and `doctor` re-apply the block and report drift. The Windows hosts file is out of scope: `doctor` resolves each site name the Windows way (`powershell.exe -NoProfile -Command [System.Net.Dns]::GetHostAddresses(...)` through interop when available) and prints the exact lines to add to `C:\Windows\System32\drivers\etc\hosts` for the names Windows cannot resolve |

## 8. Receipt

`install` writes the receipt before the first system change and updates it
after each one, so an interrupted install can still be undone:

```json
{"schema":1,"user":"me","uid":1000,"platform":"ubuntu-24.04","created":"2026-10-18T10:00:00Z",
 "changes":[
  {"id":1,"kind":"file_created","path":"/usr/local/libexec/gash/gash-hosts-helper","sha256":"..."},
  {"id":2,"kind":"file_created","path":"/etc/sudoers.d/gash-hte","sha256":"..."},
  {"id":3,"kind":"sysctl","key":"net.ipv4.ip_unprivileged_port_start","prior":"1024","applied":"80",
   "file":{"path":"/etc/sysctl.d/50-gash-hte.conf","prior":null}},
  {"id":4,"kind":"linger","prior":false,"applied":true},
  {"id":5,"kind":"unit","name":"apache2.service","prior":{"enabled":true,"active":true},"applied":{"enabled":false,"active":false}},
  {"id":6,"kind":"hosts_block","prior":null},
  {"id":7,"kind":"legacy_vhost","path":"/etc/apache2/sites-available/001-shop.test.conf",
   "prior":{"enabled":true},"archive":"/var/lib/gash-hte/archive/20261018T100000Z/..."}
 ]}
```

`file_modified` changes keep the prior content in
`/var/lib/gash-hte/backup/` (root-owned), so restoring is byte-exact.
`uninstall` acts on the root copy and refuses when the user copy differs from
it (`doctor` reports the mismatch as a receipt integrity failure): a process
running as the user cannot steer what root restores.

## 9. Commands

`gash hte <command>` with `--json` on every command, the Gash error contract,
no prompts in headless or without a TTY (a missing required flag fails with
`missing_value`), prompts on a TTY only for a missing required flag, `--yes`
for destructive steps (else `confirmation_required`, RETRY).

| Command | Outcome |
|---|---|
| `install [--yes] [--ports standard\|high] [--autostart auto\|systemd\|launchd\|shell\|none] [--restore-stage PATH\|--no-restore]` | Prerequisite check per platform; plan; one interactive sudo for the bootstrap (helper, drop-in, ports, linger, freeing 80/443); receipt; userland tree; stack started; autostart. Idempotent. Refuses to stop the system Apache while an enabled vhost is neither migrated nor excluded (it points at `migrate`). Detects an uninstall stage and offers to restore it |
| `doctor [--json]` | Binaries and modules, ports and who holds them, supervision and autostart, sockets, config test, per-site health (HTTP/HTTPS answer), name resolution (with the Windows lines on WSL), certificate validity and SAN, receipt integrity, available PHP versions; the exact remedy for each failure it cannot fix |
| `migrate [--apply] [--site D]... [--exclude D]... [--keep-pool-values] [--reissue-certs] [--yes]` | Section 10. Dry-run plan by default; the plan names every excluded enabled vhost and every enabled Debian `conf-enabled/` snippet as "no longer served" (HTE-64) |
| `uninstall [--yes]` | Stop the stack, remove autostart, move all userland HTE config and state into `hte-stage-<ts>/`, restore the receipt's pre-install state (units, port setting, linger, hosts block, helper and drop-in removed, legacy vhosts re-enabled if they were) |
| `site create --domain D --docroot P [--php X.Y] [--profile P] [--https\|--no-https] [--force-https\|--no-force-https] [--force-https-bypass\|--no-force-https-bypass] [--alias N]... [--no-www] [--no-http2]` | New managed site; fails on any invalid value |
| `site modify --domain D [same flags]` | Changes the state; overlays untouched |
| `site remove --domain D --yes` | Removes state, rendered files, pool, certificate and hosts entries of exactly that site; never the document root |
| `site list`, `site show --domain D`, `site enable`, `site disable` | |
| `php versions`, `php ini get\|set\|unset --domain D KEY [VALUE]`, `php global get\|set\|unset --version X.Y KEY [VALUE]` | |
| `apache config path`, `apache modules list\|enable\|disable NAME` | Global overlay; modules within the userland list |
| `service start\|stop\|restart\|reload\|status\|logs` | Section 5 |
| `cert show\|renew --domain D` | |
| `hosts list\|apply\|check\|windows` | |

Every command needs `install` first except `install`, `doctor`, `migrate`
(dry-run) and `php versions`; others fail with `hte_not_installed` (STOP).

New error codes (registered in the contract catalog with fixed actions):
`hte_not_installed` (STOP), `site_not_found` (RETRY), `site_exists` (RETRY),
`invalid_domain` (RETRY), `invalid_docroot` (RETRY), `docroot_not_found`
(RETRY), `php_version_not_installed` (RETRY), `invalid_profile` (RETRY),
`confirmation_required` (RETRY), `lock_timeout` (RETRY), `port_in_use` (STOP),
`privileged_step_required` (STOP), `config_test_failed` (STOP), `reload_failed`
(STOP; details say whether the rollback succeeded), `apache_not_found` (FATAL),
`apache_module_missing` (STOP: a module of the userland list or an enabled
extra module is not in the system module directory; the hint names the
platform's install command), `platform_unsupported` (FATAL),
`receipt_mismatch` (FATAL), `conflicting_options` (RETRY: a flag pair that
contradicts itself, such as `--force-https` with `--no-https`),
`invalid_state` (STOP: a state file that does not parse or that the strict
validation refuses; a human fixes the file). Partial failure never exits 0
(hte-cli defect).

Rules where the operator may choose otherwise (LEDGER items marked
"Decision for the operator"; the design follows the recommended option):

* euid 0: every `gash hte` command except `install`, `uninstall`, `migrate`
  and `doctor` fails with `root_not_allowed` (FATAL, hint "Run gash hte as
  your own user; it asks for sudo itself when a step needs root") (HTE-19).
* A domain that starts with `www.` is kept as given and gets no `www.www.`
  alias; every alias goes into the certificate SAN, and an alias change
  reissues the certificate (HTE-47).
* Absent flags: `https` on, `force_https` off, `force_https_bypass_loopback`
  off, `http2` on, profile `dev`,
  PHP = the `hte.toml` default, else the highest discovered version; a
  default is only used when the flag is absent, never instead of an invalid
  value (HTE-55).

## 10. Migration from hte-cli and system vhosts

`gash hte migrate` reads the system Apache and FPM configuration
(`/etc/apache2/sites-{available,enabled}`, `/etc/httpd/conf.d`,
`/etc/httpd/conf/extra`, `/etc/php/*/fpm/pool.d`, the hte-cli certificate
directory and `.info` files, the hosts file) and classifies every vhost:

| Class | Recognized by | Plan |
|---|---|---|
| V1 (hte-cli 1.0.x) | Marker `# Auto-generated by the test environment creator`; force block with the directives at column 0, no localhost bypass; pool without `; Profile:` | Import as managed; force-HTTPS normalized, bypass off (V1 redirected every request); profile by the rule of step 4 |
| V1-fixed | V1 marker plus the bypass block of a later `fix` | Import as managed, bypass on |
| V2 (1.1.x) | Marker `# Auto-generated by HTE-Cli`; bypass block; pool with `; Profile:` | Import as managed, bypass on |
| Broken-bypass | What hte-cli `fix` recognizes (blank lines between the directives) | Import as managed, normalized, bypass on |
| Malformed | Marker present, domain or socket missing | Not imported; reported |
| Raw | No marker (reverse proxies, hand-written vhosts) | Import as `kind = raw`: system paths and `${APACHE_LOG_DIR}` rewritten, certificates copied, validated by the config test; failures stay in the system and are reported |

A raw import rewrites only what cannot work in userland and refuses the
rest: `${APACHE_LOG_DIR}` and the system log directory become the site's log
directory; `<VirtualHost>` addresses on 80 and 443 become the loopback
listeners; a PHP-FPM socket becomes the site's own pool (profile `dev`);
`SSLCertificateFile`/`SSLCertificateKeyFile` become the site's userland pair
(copied). A file with `Listen`, several `ServerName` values, half a
certificate pair or a socket without a PHP version stays in the system and
is reported. Raw vhosts get no `IncludeOptional` of the site overlay: the
imported text is the site's configuration, kept in `raw_conf`.

Parsing follows what hte-cli itself read back (discovery code of 1.1.0 and
1.1.1, `ApacheManager.php:50-143`), made strict:

1. Candidates: every `*.conf` under the platform's vhost directories
   (recursive, as hte-cli), classified by marker; a file with a marker but no
   parseable `SetHandler "proxy:unix:.../php<X.Y>-fpm-<domain>.sock|..."` is
   Malformed (hte-cli 1.1.0 crashed on those, 1.1.1 skipped them).
2. Identity: the socket's domain, cross-checked against `ServerName`
   (different: reported, not imported). Domains are compared
   case-insensitively; hte-cli kept `--domain` verbatim, so `App.test` and
   `app.test` can both exist (reported as duplicates).
3. Document root: quoted or unquoted `DocumentRoot`, recovered from
   `<Directory ...>` (always `<docroot>/`) when an unquoted path with spaces
   was truncated.
4. HTTPS: a `:443` vhost plus both `SSLCertificate*File` lines (the 1.1.1
   rule); forced: either force comment; profile: `; Profile:` lowercased when
   present, else the profile whose values match the pool exactly (whitespace
   normalized), else `dev` with the differences reported; an unknown
   `; Profile:` gives `dev` with a warning (HTE-66). Certificates: exactly
   `<certdir>/<domain>.crt` and `.key`, copied by the root step, or a new
   pair with `--reissue-certs`; `doctor` notes copied legacy (RSA)
   certificates and suggests `cert renew` (HTE-67).
5. Pool: the exact path `/etc/php/<X.Y>/fpm/pool.d/<domain>.conf` (or the
   platform equivalent) whose first section is `[<domain>]`. Never hte-cli's
   substring search, which took any pool mentioning `[<domain>]`, even in a
   comment. The pool's own values are compared with the profile and the
   differences reported (they go to the site's `[php_ini]` with `--keep-pool-values`).
6. Certificates: exactly `<certdir>/<domain>.crt` and `.key` (`.info`
   ignored); never a `<domain>.*` glob. The key is root 0400: the root step
   copies it into the user's certificate directory, or `--reissue-certs`
   makes a new one.
7. Hosts: lines marked `# hte-cli` give the names to put in the managed
   block. The legacy lines themselves are left untouched (they resolve the
   same names to the same address): migrate never deletes or rewrites a hosts
   line outside its own block.

Also handled: duplicate domains (both reported; the user picks one with
`--site` or excludes the other), a vhost using another site's pool socket
(both sites get their own pool; reported), custom `Listen` ports (kept on the
site as extra listen ports in the high range, reported when below 1024 and not
80/443), unmarked vhosts using HTE certificates (imported as raw, certificate
copied), orphan pools, certificates and `.info` files and index gaps
(reported, never deleted), IP addresses used as domains by hte-cli 1.0.8+
(reported, not imported: a site name must be a hostname), a system Apache whose
workers already run as the user through `APACHE_RUN_USER` in `envvars`
(reported; it changes nothing, the master is still root).

`--apply`, per site, idempotent:

1. Create the userland site (state, certificate, pool, vhost) and verify that
   it answers, first on a staging listener (`127.0.0.1:<high port>`, with the
   site's Host header and SNI) while the system Apache still holds 80/443.
2. When every enabled system vhost is migrated or excluded: stop and disable
   the system Apache (and system FPM services left without pools), start the
   userland stack on 80/443, verify every site again.
3. Only then disable and archive the legacy files (`a2dissite` or the platform
   equivalent; the files are moved to `/var/lib/gash-hte/archive/<ts>/`, never
   deleted), each change recorded in the receipt.

The exact historical templates of every variant were recovered at S1 by
running the real hte-cli 1.0.12 and 1.1.1 template code (sources from git
history and the shipped phars), with what hte-cli 1.1.0 and 1.1.1 read back
from each tree. They move into `testdata/hte/legacy/<variant>/` (V1,
V1-fixed, V2, Broken-bypass, Malformed, plus templates and the certificate
script), and migrate is proven on all of them (PRD 11.1). No tagged hte-cli
release wrote the Broken-bypass format (it matches hte-cli's own `fix`
regression test, probably an unreleased 1.0.13): the fixture is built from
that test.

## 11. Platforms and verification

| Platform | Verification |
|---|---|
| Debian, Ubuntu | E2E in containers (no systemd) here and in CI: real Apache, real PHP-FPM, multi-PHP. In this cloud session the ondrej/sury repositories are blocked; multi-PHP is built from official `php:X.Y-fpm` images copied side by side into one image (`/opt/php/X.Y`), CI uses the distro repositories |
| Fedora | E2E container in CI (the session cannot reach Fedora mirrors): distro PHP plus Remi side-by-side |
| Arch | E2E container in CI (the session cannot reach Arch mirrors): `php` and `php-legacy` |
| macOS | GitHub Actions macOS runner: Homebrew `httpd`, `php`, `php@8.3`; port mechanism verified there |
| WSL2 | Manual checklist for the operator (`docs/hte/wsl-checklist.md`) |

Unit tests use a fixture system root (`internal/hte/sysroot` maps every system
path under a temporary directory) and an in-process command runner fake, so
install followed by uninstall is proven to restore the fixture byte for byte.
