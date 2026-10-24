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
| Where user edits go | Per-site and global overlay files that every generation copies and includes, and gash never rewrites (fixes "modify loses manual edits") |
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
| Site Apache overlay | `$XDG_CONFIG_HOME/gash/hte/sites/<domain>.apache.conf` | Optional, a regular file, never rewritten; each generation holds a copy (`overlays/sites/<domain>.apache.conf`) that every vhost of the site includes (HTE-71) |
| Global Apache overlay | `$XDG_CONFIG_HOME/gash/hte/apache.conf` | Optional, a regular file, never rewritten; each generation holds a copy (`overlays/apache.conf`) included at server level (HTE-71) |
| Hosts names | `$XDG_CONFIG_HOME/gash/hte/hosts.toml` | `names = [...]`: names kept in the managed hosts block beyond the sites' own (the legacy `# hte-cli` names `migrate` carries over); missing means none |
| Global PHP overlay | `$XDG_CONFIG_HOME/gash/hte/php/<X.Y>/` | Per PHP version, a directory of `*.ini` files (`php global set` edits `global.ini` in it) that the version's FPM master reads after the system ones through `PHP_INI_SCAN_DIR` (section 4); never rendered into pools |
| Rendered config | `$XDG_STATE_HOME/gash/hte/render/gen-<n>/` and the symlink `render/current` | Generations; `current` swaps atomically; the previous generation is kept for rollback |
| Certificates | `$XDG_DATA_HOME/gash/hte/certs/<domain>/{cert.pem,key.pem}` | Key 0600, directory 0700 |
| Logs | `$XDG_STATE_HOME/gash/hte/logs/` (`error.log`, `<domain>/access.log`, `<domain>/error.log`, `<domain>/php-error.log`, `php-fpm-<X.Y>.log`) | Never inside a document root. Not rotated; `site remove` keeps `logs/<domain>/`, as it keeps the document root (HTE-48) |
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
extra_http_ports  = []             # more loopback ports for the HTTP vhost (--extra-http-port N)
extra_https_ports = []             # more loopback ports for the HTTPS vhost (--extra-https-port N)
enabled     = true
kind        = "managed"            # managed | raw (imported non-HTE vhost, section 10)
raw_conf    = ""                   # kind = raw: the imported vhost file, kept verbatim after path rewriting
[php_ini]                          # per-site overrides (section 4): a profile key replaces the profile's line(s)
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
  IPv6 loopback is available, then the same for every extra port of an
  enabled site, ascending. Never a wildcard address.
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
* `IncludeOptional` of the generation's copy of the global overlay (a
  comment above it names the user's file), then of every enabled site's
  rendered vhosts.

A site's `extra_http_ports` and `extra_https_ports` (1 to 65535, never 80,
443 or the stack's own ports, never in both lists, the HTTPS ones only on an
HTTPS site, and a port has one protocol across all sites: else
`conflicting_options`) add `Listen` lines and the addresses
`127.0.0.1:<port>` (and `[::1]:<port>`) to that site's HTTP or HTTPS
`<VirtualHost>`. They keep the custom ports of migrated vhosts (section 10).

Per managed site (`render/current/sites/<domain>.conf`). The directives are
hte-cli 1.1.1's (`ApacheManager.php:386-497`, fixture
`templates/v2-vhost-forced.conf`) minus its defects: no Apache 2.2 `order`/
`allow`, no weak `SSLCipherSuite` (server-level TLS defaults instead), quoted
paths, logs outside the document root, 307 instead of 301:

```apache
<VirtualHost 127.0.0.1:80>
    # Managed by gash hte. Edit the site with `gash hte site modify`, or add directives to the overlay named below.
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
    # mod_proxy_fcgi sends SCRIPT_FILENAME percent-encoded: PHP-FPM gets the path Apache resolved.
    ProxyFCGISetEnvIf "true" SCRIPT_FILENAME "%{unescape:%{reqenv:SCRIPT_FILENAME}}"

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

    # Site overlay: "/home/me/.config/gash/hte/sites/shop.test.apache.conf", copied into this generation (run `gash hte service reload` after editing it).
    IncludeOptional "/home/me/.local/state/gash/hte/render/gen-7/overlays/sites/shop.test.apache.conf"
</VirtualHost>
```

A generation holds a copy of every overlay it includes, made when it is
rendered, and includes the copies, never the user's files (HTE-71): an
overlay edit is a change of the rendering, tested, swapped and rolled back
like any other, and a rollback brings back the overlays the previous
generation ran with. The copy exists only when the user's file does
(`IncludeOptional` skips the missing path); a path that is not a regular
file is `invalid_state`.

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
existing directory, and none of `$ * ? [ ] ; : " \` or control characters,
which Apache (`${NAME}` expansion, `<Directory>` wildcards) or the FPM INI
parser (`;` comments, `${NAME}`, quoting) would interpret, or that would split
the `open_basedir` list (`:`, its separator) (`invalid_docroot`, the details
naming the character, hint: "rename the directory or use a symlink without
these characters"; HTE-08). `migrate` reports a legacy vhost with such a root
as a skip with that code and leaves it in the system. Pool values are
double-quoted. Apache 2.4.58 `mod_proxy_fcgi` sends `SCRIPT_FILENAME`
percent-encoded (`my%20shop`), and PHP-FPM answers "File not found." for it:
every vhost with a PHP handler sets `ProxyFCGISetEnvIf "true" SCRIPT_FILENAME
"%{unescape:%{reqenv:SCRIPT_FILENAME}}"`, which gives FPM the path Apache
resolved. Raw vhosts are rendered as they are.

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

One self-signed certificate per site, SAN `<domain>`, `*.<domain>` (which
covers `www.`) and every alias (HTE-47), CN `<domain>`, made by the `openssl`
command line (`genpkey` EC P-256, then `req -x509` with a generated request
configuration and a random serial; hte-cli: RSA 2048 through a generated
script run as root from `/tmp`), validity 10 years (hte-cli: 30),
`basicConstraints CA:FALSE`, `keyUsage digitalSignature`, `extKeyUsage
serverAuth`. Private keys go only through `openssl`: gash never reads,
parses or holds one (PRD MUST NOT 6), and parses only certificates
(`crypto/x509`, for `cert show` and `doctor`). `openssl` writes the key into a
0700 temporary directory inside the site's 0700 directory; gash sets it to
0600 and renames it into place, then the certificate (a crash in between
leaves a key the certificate does not match, which `doctor` reports;
temporary directories of an interrupted run are removed by the next one).
The pair is checked by comparing the public key `openssl x509 -pubkey`
prints with the one `openssl pkey -pubout` derives from the key. No
passphrase (a userland master cannot prompt; hte-cli's passphrase was fake
anyway, `-nodes`). `cert renew` replaces both files and reloads; `doctor`
warns 30 days before expiry. No local CA, no mkcert (PRD 2). Each site's
files live in their own directory, so no operation can touch another site's
certificate (hte-cli's `rm -rf <dir>/<domain>.*` deleted `shop.test.it`'s
files when creating `shop.test`). A legacy key that `migrate` copies is
read by the root step, which hands the open file to `openssl pkey` running
as the user (section 7).

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
  ; ...then the site's [php_ini] keys the profile does not set
  ```

  The site's `[php_ini]` (lead decision, 2026-10-18): a key the profile also
  sets (or the per-version `session.save_path` below) is written in place of
  the profile's line, with the profile directive's admin kind
  (`memory_limit = "512M"` replaces both `php_admin_value[memory_limit]` and
  `php_value[memory_limit]`); any other key is `php_value`/`php_flag`, so
  `ini_set` can still change it. A boolean word (`on`, `off`, `yes`, `no`,
  `true`, `false`, any case) becomes a bare `*_flag` (FPM refuses a quoted
  flag: "invalid boolean value"); any other value is a `*_value` written in
  double quotes, so `;` is text and INI constants are not evaluated
  (`error_reporting` takes a number). A value with a control character, `"`,
  `\` or `${` is refused (`invalid_state`).

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
  then `include=` of that version's pools. It runs with `-c` the platform's
  `php.ini` for that version and `PHP_INI_SCAN_DIR=<system scan
  dir>:<overlay dir>` (lead decision, 2026-10-18): the system directory of
  additional `.ini` files the binary was built with (discovery reads it from
  `php-fpm -i`, "Scan this dir for additional .ini files", cached with the
  version; the layout's default when `-i` does not name one), then
  `$XDG_CONFIG_HOME/gash/hte/php/<X.Y>/`, both written out in full (an empty
  element meaning "the compiled-in directory" needs PHP 7.x; the full list
  works on 5.6 to 8.5), so the system files are read, never written, and the
  overlay's values win. A directory containing `:` cannot be passed and is
  refused (`platform_unsupported`). Each generation carries a
  `manifest.json` with the binaries, `php.ini`, `PHP_INI_SCAN_DIR`, PID
  files, sockets and site addresses it was tested with; supervision runs the
  generation from it.

## 5. Lifecycle and supervision

The masters daemonize themselves; gash starts, signals and checks them through
PID files and never runs a supervisor process of its own.

| Command | Does |
|---|---|
| `service start` | Render if stale, config test, start each FPM master in use, then Apache; wait until the sockets and ports answer |
| `service stop` | `SIGTERM` (Apache `-k stop`), wait, `SIGKILL` after a timeout. A PID is acted on only when `/proc/<pid>/cmdline` (Linux) or `ps -o command= -p` (macOS) names our config path, so a recycled PID is never signalled |
| `service restart` | stop + start |
| `service reload` | Render, test, swap, FPM `SIGUSR2` first (wait until every socket of the new generation accepts), then Apache `-k graceful`, health check; rollback on failure. On a site removal the order is reversed: Apache first, then FPM (HTE-27). Per master: one whose pools are added or changed reloads before Apache, one whose pools only went away reloads after it, a version no longer used stops after it, a version whose binary, `php.ini` or `PHP_INI_SCAN_DIR` changed restarts. An Apache overlay edit is a change of the rendering (the generation holds copies of the overlays, section 3), so a failure at run time that the config test does not see, such as a `Listen` on a busy port in the global overlay, rolls back to the previous generation and its own copies (HTE-71). When the rendering is unchanged, the current generation is config-tested and the running masters reload in the same order (they read the global PHP overlay directories themselves); a failed test changes nothing. `php global set\|unset` reloads that version's master, and puts the previous overlay back when it does not come back |
| `service status` | Per master: running, PID, uptime, listening addresses; per site: answers on HTTP and HTTPS. `--json` |
| `service logs` | Tail of the global, per-site Apache and PHP logs (`--site`, `--lines N`; no follow mode, so agents never hang) |

A master counts as ready when its log gained its readiness line after the
signal or start (Apache `AH00489 ... resuming normal operations`, which it
logs whatever `LogLevel` says; FPM `ready to handle connections`), its PID
file names it, and every address (Apache) or socket (FPM) accepts a
connection; a failure line (`AH00016`, `no listening sockets available`,
`FPM initialization failed`) ends the wait at once. FPM re-executes and
daemonizes again on `SIGUSR2`, so its PID changes; Apache keeps its PID on a
graceful restart but exits when a new `Listen` port is taken, which the
rollback then repairs by starting it on the previous generation. A PID file
whose process is not ours is removed before a start (Apache refuses to
start while the PID it names is alive). FPM's command line becomes
`php-fpm: master process (<config>)`, which still names the configuration.
Apache's config test does not check that a key matches its certificate (it
fails only at start), so certificates are checked by the pair check of 3.1
before a generation is made.

Autostart, chosen by `install` and recorded in the receipt:

| Platform | Mechanism |
|---|---|
| Linux with systemd | User unit `gash-hte.service` (`Type=oneshot`, `RemainAfterExit=yes`, `ExecStart=<gash> hte __unit start`, `ExecStop=<gash> hte __unit stop`: the hidden command starts and stops the masters directly, because every start, stop and restart of the stack goes through `systemctl --user` while the unit is in use: `service start\|stop\|restart`, the stack start of `install` and of the `migrate --apply` switch, the stop of `uninstall` (HTE-73)), `systemctl --user enable`, plus `loginctl enable-linger <user>` (root step) so it starts at boot |
| Linux without systemd (containers, WSL without systemd) | No boot hook (`/etc/wsl.conf` is never touched). Optional `autostart = "shell"`: the first interactive shell that loads `gash init bash` starts the stack when it is down (one detached background `gash hte __autostart` per shell, never blocking or failing the shell load; no output, the error of a failed start in `$XDG_STATE_HOME/gash/hte/logs/autostart.log`) |
| macOS | User LaunchAgent `~/Library/LaunchAgents/io.github.mauriziofonte.gash.hte.plist` with `RunAtLoad`, running `<gash> hte __unit start` with the XDG directories install resolved as its `EnvironmentVariables` (launchd starts it without them); install runs `launchctl enable gui/<uid>/io.github.mauriziofonte.gash.hte` (clearing an earlier disable) and starts the stack itself, launchd loads the agent at the next login; uninstall runs `launchctl bootout` on it (not loaded is fine) and removes the file |

With the systemd unit, a `systemctl --user start` that leaves a master
down (the unit stayed active while its masters ended) is followed by a
`restart`. A command that holds the state lock and runs a unit job writes
`$XDG_STATE_HOME/gash/hte/unit.lease` (its PID) for the job's duration;
the hidden command, which systemd runs meanwhile, then works under that
command's lock instead of waiting for it (a lease whose process is gone,
or with the lock free, does not count). The masters of `migrate`'s staging
generation (two run-time loopback ports, while the stack is down) are
still started and stopped directly: the unit always starts the stored
settings. Masters a reload starts (a PHP version that comes into use, the
rollback's restart) are started directly too.

## 6. Ports 80/443 without root at runtime

| Platform | Mechanism (recorded in the receipt, reverted by uninstall) |
|---|---|
| Linux, WSL2 | `net.ipv4.ip_unprivileged_port_start=80` via `/etc/sysctl.d/50-gash-hte.conf` plus `sysctl -w`. The setting applies to IPv4 and IPv6. When the current value is already 80 or lower (Docker containers default to 0) nothing changes and the receipt says so. Trade-off, stated by `install` before asking: every local user may then bind ports 80 to 1023 |
| macOS | First choice: a direct bind, if the running kernel lets an unprivileged process bind `127.0.0.1:80` (checked by `install` with a real bind of `127.0.0.1:80` and `:443`; only `EACCES` or `EPERM` counts as a refusal, a port in use is the stack's start to report). Otherwise a `pf` anchor redirecting `127.0.0.1:80`/`:443` on `lo0` to the userland ports (`8080`/`8443` by default), loaded at boot by a root LaunchDaemon that only runs `pfctl -a <anchor> -f <root-owned file>` (details below) |
| Any, when declined or impossible | High ports (`8080`/`8443`, configurable), reported by `doctor`; site URLs carry the port |

Rejected: `setcap cap_net_bind_service` on the distro binary (modifies a
package-owned file, lost on upgrade, also grants the system Apache), authbind
(Debian only), `APACHE_RUN_USER` on the system Apache (the master still starts
and parses its config as root; PRD 6.1).

The macOS mechanism is to be verified on the GitHub macOS runner during the
HTE phase; the design supports both outcomes.

The `pf` redirect is the root operation `pf_redirect` (ports: the stack's
HTTP and HTTPS ports). `/etc/pf.conf` is never edited: the anchor is
`com.apple/gash-hte`, a child of the `com.apple` anchor that the stock
`pf.conf` evaluates (`rdr-anchor "com.apple/*"`); a `pf.conf` without that
line is `platform_unsupported` (hint: restore it, or `--ports high`). Root
writes the rules file `/etc/pf.anchors/gash-hte` (0644, checked with
`pfctl -n` before it is put in place):

```
rdr on lo0 inet proto tcp from any to 127.0.0.1 port 80 -> 127.0.0.1 port 8080
rdr on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port 8443
rdr on lo0 inet6 proto tcp from any to ::1 port 80 -> ::1 port 8080
rdr on lo0 inet6 proto tcp from any to ::1 port 443 -> ::1 port 8443
pass in quick on lo0 inet proto tcp from any to 127.0.0.1 port { 8080 8443 } keep state
pass in quick on lo0 inet6 proto tcp from any to ::1 port { 8080 8443 } keep state
pass out quick on lo0 proto tcp from any to any port { 80 443 8080 8443 } no state
pass out quick on lo0 proto tcp from any port { 80 443 8080 8443 } to any no state
```

A packet on lo0 passes pf twice, out and then in. With `rdr pass` the
state of a redirected connection makes pf refuse the state of a later
direct connection to 8080 (the `state-insert` counter), so that SYN is
dropped and the stack looks dead (seen on the macOS CI runner). The rules
redirect without `pass`, keep state for the inbound half only, and pass the
outbound half of these ports without state. Uninstall flushes the whole
anchor (`pfctl -a com.apple/gash-hte -F all`).

then the LaunchDaemon `/Library/LaunchDaemons/io.github.mauriziofonte.gash.hte.pf.plist`
(0644, checked with `plutil -lint`; `RunAtLoad`, program
`/sbin/pfctl -E -a com.apple/gash-hte -f /etc/pf.anchors/gash-hte`: `-E`
takes a reference on pf being enabled, which it is not by default, without
turning it off for anyone else), and loads the anchor now with that same
command. The receipt records both files, then the redirect. The settings
keep the high ports (Apache listens there) and `install` reports
`redirect: "pf"`; while the root receipt holds the redirect to the
settings' ports, the HTTP to HTTPS redirect targets 443 (section 3), and
`doctor` checks both files and that the redirect's targets are the
settings' ports. Uninstall unloads the daemon (`launchctl bootout
system/io.github.mauriziofonte.gash.hte.pf`), empties the anchor
(`pfctl -a com.apple/gash-hte -F all`) and removes both files; pf itself
stays enabled with the anchor empty until the next restart (the boot-time
reference has no token to release). To verify on the runner: that the
anchor loaded at boot is evaluated (the daemon may run before macOS loads
`/etc/pf.conf`).

## 7. Privilege boundary

Root is used only by:

1. `install`, `uninstall`, `migrate --apply`: one interactive `sudo
   <gash binary> hte __root` session per command (`sudo -n` in headless or
   without a terminal, so it fails with `privileged_step_required` instead of
   prompting). The user side writes the plan the user saw to the session's
   stdin as one JSON line, then asks for its operations by id; root answers
   each with one JSON line carrying the changes and its receipt copy, which
   the user side stores as its own. The operations are a closed, typed set
   (`internal/hte/rootops`: install helper, write sudoers drop-in, set
   sysctl, write pf anchor (`pf_redirect`, section 6), enable linger, stop and disable a named system
   unit, start it again when this session stopped it and nothing changed
   since (`undo_unit_stop`, the migrate rollback), reload one (for the
   system Apache first `apachectl configtest`, and on a failure the vhosts
   archived earlier in the same session put back), record the
   hosts block, archive a legacy vhost or pool, hand a legacy key file to
   `openssl pkey` running as the user, record the autostart, undo the last
   change of the receipt, remove the receipt), each run at most once, only
   when the plan holds it, and checked against root's
   own reading of the system: a vhost or pool to archive must be one root's
   `legacy.Scan` finds, a key to copy the `SSLCertificateKeyFile` of such a
   vhost, the helper's content the SHA-256 the plan names. The user is the
   one sudo names (`SUDO_USER`, `SUDO_UID`), never the plan. Each change is
   recorded in the root receipt (pending) before it is made and marked done
   after; undoing a pending change tolerates one that never happened. A
   second install finds its own changes and records nothing new. The plan
   names conditional steps too (the rollback above): the user side asks for
   one only when its condition holds. Nothing generated is executed as root
   from `/tmp` (hte-cli defect).
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
  When the rename fails with EBUSY or EXDEV (a container bind-mounts
  `/etc/hosts`), the temporary file is removed and the same validated content
  is written in place: opened without truncation and without following a
  symlink, checked to be the file that was read, written, truncated to the
  new length, fsynced (HTE-69).
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
start` and on `doctor`. The block is one `127.0.0.1` line per enabled site
with every name it serves (domain order), then one line with the names of
`hosts.toml` that no site serves; never a `::1` line (lead decision,
2026-10-18, LEDGER HTE-37: Apache still answers on `[::1]` for a client that
resolves `::1` another way). The drift check reads `/etc/hosts` as the user
(it is world-readable) and compares the region byte for byte with the
desired block, naming the missing and the unexpected names.

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

Besides the kinds above: `dir_created` (a directory install made, such as
`/usr/local/libexec/gash`, removed on uninstall when empty), `pf_redirect`
(macOS: `name` the anchor, `path` its rules file, `applied` the targets,
`{"http":8080,"https":8443}`; undone by emptying the anchor and unloading
its LaunchDaemon, whose files are the `file_created` changes before it), `legacy_pool`
(an archived pool), `autostart` (the mode and the user file, made and undone
by the user side; root only records it so both copies stay equal) and
`pending: true` on a change recorded but not yet completed. Both copies are
compact JSON plus a newline (the canonical encoding), compared byte for
byte; root keeps `/var/lib/gash-hte` 0755 and its copy 0644, so `doctor`
can read it as the user, and the backup and archive directories 0700.

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
| `install [--yes] [--ports standard\|high] [--autostart auto\|systemd\|launchd\|shell\|none] [--restore-stage PATH\|--no-restore]` | Prerequisite check per platform; plan; one interactive sudo for the bootstrap (helper, drop-in, ports, linger, freeing 80/443); receipt; userland tree; stack started; autostart. Idempotent. Refuses to stop the system Apache while an enabled vhost is neither migrated nor excluded: it then exits 0 without starting the stack, with `started: false`, `next: "gash hte migrate"` and a warning naming those vhosts and the command (HTE-75). On macOS `--ports standard` binds directly or sets up the `pf` redirect (`redirect: "pf"`, section 6). Detects an uninstall stage and offers to restore it |
| `doctor [--json]` | Binaries and modules, ports and who holds them, supervision and autostart, sockets, config test, per-site health (HTTP/HTTPS answer), name resolution (with the Windows lines on WSL), certificate validity and SAN, receipt integrity, available PHP versions; the exact remedy for each failure it cannot fix. It repairs a drifted hosts block itself (through the helper). `--json`: `{"ok": BOOL, "checks": [{"name", "status", "details", "remedy"}]}` with status `ok`, `warn`, `fail`, `fixed` or `skip`; any `fail` makes it exit non-zero. Port holders are read from `/proc/net/tcp{,6}` (Linux): the process when it is the user's own, else its uid |
| `migrate [--apply] [--site D]... [--exclude D\|PATH]... [--keep-pool-values] [--reissue-certs] [--yes]` | Section 10. Dry-run plan by default; the plan names every excluded enabled vhost and every enabled Debian `conf-enabled/` snippet as "no longer served" (HTE-64). `--exclude` takes a domain (every vhost serving it) or an absolute vhost file path as the plan lists it (that file only, whatever its class); nothing is excluded implicitly, not even Debian's `000-default.conf`, and each enabled vhost that blocks the stop is listed with the exact `--exclude PATH` to add (HTE-31). With every enabled vhost excluded and nothing to import (a stock `000-default.conf` or `ssl.conf` only), `--apply --yes` still switches: it stops and disables the system Apache and starts the stack on the standard ports, the excluded files untouched; uninstall starts the system Apache again |
| `uninstall [--yes]` | Stop the stack, remove autostart, move all userland HTE config and state into `hte-stage-<ts>/`, restore the receipt's pre-install state (units, port setting, linger, hosts block, helper and drop-in removed, legacy vhosts re-enabled if they were) |
| `site create --domain D --docroot P [--php X.Y] [--profile P] [--https\|--no-https] [--force-https\|--no-force-https] [--force-https-bypass\|--no-force-https-bypass] [--alias N]... [--no-www] [--no-http2] [--extra-http-port N]... [--extra-https-port N]...` | New managed site; fails on any invalid value |
| `site modify --domain D [same flags]` | Changes the state; overlays untouched. `--no-https` keeps the site's certificate directory, which `--https` later reuses and `doctor` lists while the site serves HTTP only (HTE-25) |
| `site remove --domain D --yes` | Removes state, rendered files, pool, certificate and hosts entries of exactly that site; never the document root, the site's overlay or its logs |
| `site list`, `site show --domain D`, `site enable`, `site disable` | |
| `php versions`, `php ini get\|set\|unset --domain D KEY [VALUE]`, `php global get\|set\|unset --version X.Y KEY [VALUE]` | |
| `apache config path`, `apache modules list\|enable\|disable NAME` | The global overlay the user edits (generations include a copy of it). `modules list`: every module of the userland list (`base`), the extra modules of `hte.toml` (`enabled`), then the other `mod_<name>.so` files of the system module directory (`available`), each with `builtin` (compiled in, no `LoadModule`), its path, and `missing` for a base or enabled module without a file. `modules enable NAME` adds a module of the system module directory to `hte.toml` (`apache_module_missing` when the directory has no `mod_NAME.so`; a base or already enabled module changes nothing) and `disable NAME` removes an extra one (`conflicting_options` for a base module); either moves the stack to a generation that loads exactly those modules, and a failure puts `hte.toml` back |
| `service start\|stop\|restart\|reload\|status\|logs` | Section 5 |
| `cert show\|renew --domain D` | |
| `hosts list\|apply\|check\|windows` | |

Every command needs `install` first except `install`, `doctor`, `migrate`
(dry-run) and `php versions`; others fail with `hte_not_installed` (STOP).

Output (`internal/hte/htecmd`): with `--json` a command prints one compact
JSON object on stdout, the engine's result; the commands whose result is a
single value wrap it: `php ini get` and `php global get` print `{"domain"`
or `"version", "key", "value", "set"}`, `apache config path` `{"path"}`,
`service logs` `{"logs": [{"path", "lines"}]}`, `hosts list`
`{"entries": [{"address", "names"}]}`, `hosts windows` `{"lines"}`. Without
`--json` the data goes to stdout as text (key-value blocks and tables) and
the messages (OK, warnings, hints) to stderr. An error is one envelope on
stderr and exit 1; a command that did part of its work (root steps that
ran, sites imported) prints that part first. `doctor` with a `fail`, and
`hosts check` or `hosts apply` with a block that still differs, exit 1
without an envelope. Before sudo starts, the root steps are listed on
stderr, numbered, so that they stand above sudo's password prompt (not
with `--json` without a terminal: the result carries them). Command-line
errors map as for the other 3.x commands: an unknown flag is
`unknown_option`, a flag or positional argument missing its value
`missing_value`, an extra word or unknown command `unexpected_argument`,
each with the hint `Run: gash hte <command> --help`.

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
validation refuses; a human fixes the file), `openssl_not_found` (FATAL, the
hint names the platform's install command, like `apache_not_found`),
`invalid_ports` and `invalid_autostart` (RETRY: as `invalid_profile`, every
enum flag has its `invalid_<flag>`, whose details name the value and the
accepted ones), `php_not_enabled` (RETRY: a PHP command on a site without
PHP, hint `site modify --php VERSION`) (HTE-72). Partial failure never exits 0
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
listeners, addresses on other ports the loopback addresses of that port
(an extra port of the site); a PHP-FPM socket becomes the site's own pool (profile `dev`);
`SSLCertificateFile`/`SSLCertificateKeyFile` become the site's userland pair
(copied). A file with `Listen`, several `ServerName` values, half a
certificate pair, a section without a port, a port of both protocols or a
socket without a PHP version stays in the system and is reported. Raw vhosts get no `IncludeOptional` of the site overlay: the
imported text is the site's configuration, kept in `raw_conf`.

Parsing follows what hte-cli itself read back (discovery code of 1.1.0 and
1.1.1, `ApacheManager.php:50-143`), made strict:

1. Candidates: every `*.conf` under the platform's vhost directories
   (recursive, as hte-cli; an entry the user cannot read, or a symlink
   loop, is listed in the plan as unreadable and the scan goes on, HTE-28),
   classified by marker; a file with a marker but no
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

Also handled: duplicate domains (both reported; the user excludes one by its
file path, `--exclude PATH`), a vhost using another site's pool socket
(both sites get their own pool; reported), custom ports (every
`<VirtualHost>` port other than 80, 443 and the stack's own becomes an extra
port of the site, HTTP or HTTPS by the section's `SSLEngine on`, raw vhosts
included; reported, with a note below 1024), unmarked vhosts using HTE
certificates (imported as raw, certificate
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

How `--apply` does it (`internal/hte/migrate.go`):

* It needs `install` first and `--yes` when its root steps stop a unit or
  archive a file (`confirmation_required` otherwise, before anything runs);
  as euid 0 it is `root_not_allowed` (the sites go into the invoking user's
  own stack). One sudo session holds every root step: the key copies, the
  stop of the system Apache with its conditional rollback
  (`undo_unit_stop`), the archive steps, the FPM unit steps.
* Staging: when the stack is not running, the state of every site (the new
  ones included) is rendered on two free loopback ports picked at run time
  (IPv4 only, no extra ports, which the system Apache may hold), started,
  probed, stopped. When the stack already runs, the new sites are applied
  to it and probed there. A site answers when every address gives an HTTP
  status other than 502, 503 and 504 (redirects are not followed). A
  failure puts the previous state files back and removes the new sites'
  certificate directories.
* While an enabled vhost blocks the stop (the plan's `blocking_stop`), the
  sites are imported and verified, nothing is stopped or archived, and a
  later run finds them `already_managed`: their vhosts and pools are
  archived then. A failure after the switch starts the system Apache again
  (`undo_unit_stop`, which root runs only when the stop is the last change
  of its receipt and was made in the same session); the imported sites
  stay, and the next run completes the move.
* High ports (`install --ports high`, neither port 80 nor 443, HTE-70): the
  system Apache keeps running and keeps the ports it listens on. The plan
  (`system_apache`) names those ports (its `Listen` directives and the
  addresses of the enabled vhosts that are not imported) and the vhosts and
  snippets it still serves, which then neither block nor count as "no longer
  served". `--apply` stops nothing: it imports and verifies the sites,
  archives their vhosts, then runs one `unit_reload` of the system Apache,
  which tests the system configuration first. A failed test puts the
  archived vhosts back, reloads nothing and fails with `config_test_failed`;
  the imported sites stay. System FPM units left without pools stop as with
  standard ports. `uninstall` re-enables the archived vhosts and reloads the
  system Apache when the receipt never stopped it.
* Archived: every imported or already managed vhost, and the exact hte-cli
  pool of a marked one (parsing step 5 above); the pools of raw vhosts are never
  archived (the default `www.conf` may serve them). A system FPM unit left
  with no pool file stops and is disabled; one that keeps pools is
  reloaded.

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
| Debian, Ubuntu | E2E in containers here (Ubuntu; deb.debian.org is blocked in this session) and in CI (Debian 12 and Ubuntu 24.04): real Apache, real PHP-FPM, without systemd and with systemd as PID 1. Multi-PHP on Ubuntu: the ondrej/sury repositories are blocked in this session, so PHP 7.4, 8.2 and 8.4 come from the official `php:X.Y-fpm` images copied side by side into one image (`/opt/php/X.Y`), here and in CI |
| Fedora | E2E container in CI (the session cannot reach Fedora mirrors): distro PHP plus Remi side-by-side |
| Arch | E2E container in CI (the session cannot reach Arch mirrors): `php` and `php-legacy` |
| macOS | GitHub Actions macOS runner (the `hte-macos` job, `testdata/hte/e2e/macos.sh` running `TestE2EDistro` natively): Homebrew `httpd`, `php`, `php@8.3`; the port mechanism install picks there, the LaunchAgent, the hosts helper, and the system paths compared before install and after uninstall |
| WSL2 | Manual checklist for the operator (`docs/hte.md`, section "WSL") |

Unit tests use a fixture system root (`internal/hte/sysroot` maps every system
path under a temporary directory) and an in-process command runner fake, so
install followed by uninstall is proven to restore the fixture byte for byte.

The container E2E is `testdata/hte/e2e/run.sh`: it builds the test binary of
`internal/hte/e2e` (build tag `hte_e2e`; it skips outside the container) and
the hosts helper, and runs them in a disposable container (`--network none`,
no systemd) whose system Apache and PHP-FPM serve the hte-cli V2 tree. The
binary runs as an unprivileged user and is also the root step, through one
sudoers rule for `<binary> hte __root`; afterwards `setup.sh` compares `/etc`,
`/usr/local` and `/var/lib/gash-hte` with their state before install.
`/etc/hosts` stays Docker's bind mount, so the helper writes it in place
(`setup.sh` checks that first). `run.sh DISTRO` picks the image:
`ubuntu` (the default, Ubuntu 24.04, `Dockerfile.base`) and `debian`
(Debian 12, the same apt recipe) run `TestE2E` and `TestE2EReinstall` on
the hte-cli tree, moved to the distro PHP (8.3, 8.2) that `setup.sh` reads
from the `php-fpm` package; `fedora` (Fedora 44, `Dockerfile.fedora`:
`httpd`, `mod_ssl`, `mod_http2`, the distro `php-fpm` and Remi's
`php83-php-fpm`) and `arch` (`Dockerfile.arch`: `apache`, `php-fpm`,
`php-legacy-fpm`) have no hte-cli and run `TestE2EDistro` on the packages
as installed, never started: discovery of both packagings, install (on
Fedora, mod_ssl's `_default_:443` vhost in `ssl.conf` blocks the standard
ports, HTE-75, and `migrate --apply --exclude` of that file switches
onto them with nothing to import; the high ports beside a running system
Apache are `TestE2EReinstall`'s), a site on each
version with HTTPS, HTTP/2 and sessions, `php global set`, `site modify
--php`, the service commands, doctor and uninstall. CI runs every distro,
plus the multi-PHP story on Ubuntu and the systemd story on Debian and
Ubuntu. `GASH_HTE_E2E_MULTIPHP=1 run.sh` runs `TestE2EMultiPHP`
in an image that adds PHP-FPM 7.4, 8.2 and 8.4 from the official
`php:X.Y-fpm` images (`Dockerfile.multiphp`): each binary is copied into
`/opt/php/X.Y` with every library it loads, its own loader included
(`patchelf` points it there), and installed in the Debian layout
(`/usr/sbin/php-fpmX.Y`, `/etc/php/X.Y/fpm/`, a `www` pool, a SysV
service `phpX.Y-fpm`); the hte-cli tree then keeps `https.test` on the
system PHP 7.4, and the story checks that each site runs on a userland
master of its version, a site on every other version, `site modify --php`
(the unused master stops), `php global set` on one version, and uninstall
giving the 7.4 site back to the system PHP 7.4. `GASH_HTE_E2E_SYSTEMD=1 run.sh` runs `TestE2ESystemd` in a
container with systemd as PID 1 (`Dockerfile.systemd`: logind and the user
D-Bus added, the units that touch the kernel or devices masked; started
`--privileged` with a private cgroup namespace: systemd mounts its cgroup
hierarchy, which the docker-default AppArmor profile of the GitHub runners
denies to an unprivileged container even with `CAP_SYS_ADMIN`): in a login session of the user, install with
`--autostart systemd`, the service commands, the migrate switch and
uninstall, each master checked to run in the unit's cgroup (HTE-73).
