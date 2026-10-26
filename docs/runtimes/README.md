# PHP, Node.js and Chrome

Pick PHP and Node.js versions per project, run commands under a version, and keep headless Chrome current.

Many machines carry several PHP versions (`php8.1`, `php8.3`, ...) and several
Node.js versions (through nvm). Gash tells you which version a project asks
for, gives you short commands for each installed PHP, runs any command under
a chosen version without loading nvm, keeps the Node.js versions current, and
installs the headless Chrome that Playwright and Puppeteer use.

Platforms: Linux, macOS and WSL. PHP versions are the `phpX.Y` programs that
Debian and Ubuntu packages (and the ondrej PPA) install; elsewhere the `php`
on `PATH` is used. Node.js versions are the ones nvm installed in
`${NVM_DIR:-~/.nvm}`. `chrome_headless_update` works on Linux x86_64 only.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [unzip](../requirements.md#tool-unzip) (feature), for `chrome_headless_update`
- [php](../requirements.md#tool-php) (feature), for `php_detect_version`, `gash runtime` (php) and `gash run` (--runtime php)
- [composer](../requirements.md#tool-composer) (optional), for `gash runtime` (composer) and `gash run` (--runtime composer)
- [node](../requirements.md#tool-node) (feature), for `node_upgrade`, `nvm_prune_versions`, `gash runtime` (node) and `gash run` (--runtime node)
- [ldd](../requirements.md#tool-ldd) (feature), for `chrome_headless_update`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Find the PHP version a project wants

[`php_detect_version`](reference.md#php_detect_version) reads, in this order,
and stops at the first that names a version:

1. `.target-php-version` (its first `X.Y`), in the project directory or any
   parent directory;
2. the `require.php` constraint of `composer.json`, in the project directory;
3. the first "php X.Y" of `.github/copilot-instructions.md`;
4. otherwise, 8.4.

```bash
php_detect_version              # the current directory
php_detect_version ~/src/shop --json
```

It prints the version and the commands that serve it: the binary
(`/usr/bin/php8.3`), the alias of the php pack below (`php83`), and the same
for Composer. When that version is not installed, a warning names the binary
used instead (`/usr/bin/phpX`, then `php` on `PATH`), and the alias it names
does not exist in your shell. A `composer.json` constraint with several
alternatives (`^8.1 || ^8.3`) picks the highest one that is installed.
`--json` prints one JSON object; in headless mode JSON is the default.

## Run PHP and Composer at a given version

When `php` is installed, every interactive shell gets an alias for each
`phpX.Y` program on `PATH`, and one for Composer under it:

```bash
php83 -v                     # /usr/bin/php8.3 -d allow_url_fopen=1 -d memory_limit=2048M
composer83 install           # Composer run by PHP 8.3
php81 artisan migrate:status
```

`phpXY` and `composerXY` exist for each installed version from 5.6 to 8.5,
`1composerXY` runs `composer1` under PHP 7.2 and older, and with
`GASH_REPLACE_TOOLS=1` in `~/.gash_env` plain `php` and `composer` point at
the newest version. The pack also has `composer-packages-update`
(`composer global update`), `composer-self-update` and
`composer1-self-update` (both through `sudo`). The table is in
[the php pack](reference.md#the-php-pack).

## Find the Node.js version a project wants

[`node_detect_version`](reference.md#node_detect_version) reads `.nvmrc`,
then `.node-version`, then `engines.node` of `package.json`, in the project
directory only, and reduces what it finds to a major version: `20.9.0`,
`v20` and `>=20` all mean 20, and `lts/iron` goes through nvm's alias files
(then a built-in table of codenames). With none of them, it takes the newest
installed version. It then names the newest installed patch of that major:

<!-- example
mkdir -p .nvm/versions/node/v20.11.1 .nvm/versions/node/v22.12.0 web
printf '{"name":"web","engines":{"node":">=22"}}\n' >web/package.json
-->
```console
$ cd web
$ node_detect_version
Info: Node.js version for '.': 22.12.0 (major 22)
  node_command    node22
  npm_command     npm22
  npx_command     npx22
$ echo 20 >.nvmrc
$ node_detect_version --json
{"version":"20","full_version":"20.11.1","node_command":"node20","npm_command":"npm20","npx_command":"npx20"}
```

The three command names (`node20`, `npm20`, `npx20`) are a naming
convention for wrappers you may define yourself; Gash does not define them.
To run a command under that version, use `gash run --runtime` (next
section). When the wanted major is not installed, a warning says so and
names the newest installed version instead, with the `nvm install` command
to fix it.

## Run a command under a version, without nvm

`gash runtime` resolves a version to its binary, and `gash run --runtime`
runs a command with that binary first on `PATH`. Neither loads `nvm.sh`,
and both work the same in scripts and in headless mode:

```bash
gash runtime node 22          # {"runtime":"node","request":"22","path":".../v22.12.0/bin/node","version":"22.12.0"}
gash runtime php 8.3 --path   # /usr/bin/php8.3
gash runtime composer 8.3     # composer, with "php" naming the PHP that runs it
```

The version is a prefix: `22` is the newest installed 22.x, `8.3` the newest
8.3. Node.js comes from the nvm tree first, then from the `node` on `PATH`;
PHP from `/usr/bin/phpX.Y` and `/usr/local/bin/phpX.Y`, then from the `php`
on `PATH`. `npm`, `npx` and `composer` are accepted too. The answers are
cached in `~/.cache/gash/runtimes.json` until a version is installed or
removed.

`gash run` runs a command and returns its exit status unchanged. Its
options come before `--`:

<!-- example
for v in 20.11.1 22.12.0; do mkdir -p .nvm/versions/node/v$v/bin; printf '#!/bin/sh\necho v%s\n' $v >.nvm/versions/node/v$v/bin/node; chmod +x .nvm/versions/node/v$v/bin/node; done
-->
```console
$ gash run --runtime node:20 -- node --version
v20.11.1
$ gash run --max-lines 3 -- seq 10
1
2
3
[... 7 lines omitted]
```

- `--runtime NAME:VERSION` (repeatable): `node:20`, `php:8.3`. A version
  that is not installed stops the call with a `runtime_not_installed` error
  that lists the installed ones.
- `--max-lines N` (or `--head N`), `--tail N`, `--width N`: keep the output
  short; what is cut becomes one `[... K lines omitted]` line.
- `--test-summary`: print a summary of a test run (counts, and each
  failure's file and line) instead of its output.
- `--safe`: run the command under the read-only rules of `llm_exec`: a
  command that could write is refused.

The command may be a Gash function: `gash run --max-lines 20 -- files_largest .`.
See [`gash runtime`](reference.md#gash-runtime) and
[`gash run`](reference.md#gash-run). Agents use the JSON form,
[`llm_runtime`](../agents/reference.md#llm_runtime).

## Load nvm in every shell

nvm adds 0.2 to 0.5 seconds to each shell that sources `nvm.sh`. Set
`GASH_NVM` in `~/.gash_env` to let Gash handle it:

| Value | Effect |
|---|---|
| `off` (default) | Gash leaves nvm to your own shell files. |
| `lazy` | `node`, `npm`, `npx` and the global packages of nvm's default version are on `PATH` at once; `nvm.sh` loads the first time you run `nvm`. |
| `on` | `nvm.sh` and its completion load when the shell starts. |

`NVM_DIR` defaults to `~/.nvm`. `node_upgrade` and `nvm_prune_versions`
need the `nvm` command, which `lazy` and `on` both provide.

## Keep the Node.js versions current

[`node_upgrade`](reference.md#node_upgrade) (alias `nvmc`) brings every
installed major to its newest patch: it installs the new patch with the
global packages of the one it replaces, removes the older patches of that
major, and moves the active version along. It never adds a major you do not
have and never removes one.

```bash
nvmc --dry-run   # the plan: what is installed, removed, already current
nvmc             # the plan, one question, then the work
nvmc -y          # no question
```

It reads the version list from nodejs.org, so it needs the network. A
failed install leaves that major as it was, and the status is then 1.

[`nvm_prune_versions`](reference.md#nvm_prune_versions) (alias `nvmp`) only
removes: for each major it keeps the newest patch and uninstalls the older
ones, and it never removes the active version. Without an option it shows
what it would remove; `--force` (or `-y`) removes it.

```bash
nvmp           # preview
nvmp --force   # uninstall the duplicates
```

## Keep headless Chrome current

[`chrome_headless_update`](reference.md#chrome_headless_update) (alias
`chri`) installs chrome-headless-shell, the build of Chrome that Playwright
and Puppeteer drive. It takes the exact build that the latest Playwright
release pins, downloads it from Chrome for Testing, checks that the binary
starts and names that version, and installs it into
`/opt/chrome-headless-shell/<version>/` with the link
`/usr/local/bin/chrome-headless-shell`:

```bash
chri --dry-run   # the plan and the download size, nothing changed
chri             # the plan, one question, then the install
chri -y          # unattended
```

The version it replaces is kept for a rollback, older ones are removed.
`sudo` is used only when the install directory or the link directory is not
writable. `GASH_CHROME_ROOT` and `GASH_CHROME_LINK` (environment variables)
move them. To make your tools use this one browser instead of downloading
their own:

```bash
export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 PUPPETEER_SKIP_DOWNLOAD=1
```

and point Playwright's `executablePath` (Puppeteer: `executablePath` with
`headless: 'shell'`) at `/usr/local/bin/chrome-headless-shell`.

## When something goes wrong

| Message | What it means and what to do |
|---|---|
| `Warning: PHP 8.2 is not installed; php_raw_binary falls back to /usr/bin/php` | The project wants a PHP you do not have. Install `php8.2`, or change `.target-php-version`. |
| `Error: NVM directory not found (...)` | `node_detect_version` needs nvm. Install it, or set `NVM_DIR` if it lives elsewhere. |
| `Warning: Node 18 not installed. Using 22.12.0. Run: nvm install 18` | The project wants a major you do not have; the answer names the newest one instead. |
| `Error: No Node.js versions installed in NVM. Install: nvm install 20` | nvm has no version yet. |
| `{"error":"runtime_not_installed",...}` | `gash runtime` or `gash run --runtime`: the version is not installed; `details` lists the installed ones. |
| `Error: nvm is not loaded in this shell. Source $NVM_DIR/nvm.sh first.` | `node_upgrade` and `nvm_prune_versions` need `nvm`: set `GASH_NVM=lazy` and open a new shell. |
| `Error: Unable to read the remote index (network?). Nothing changed.` | `node_upgrade` could not reach nodejs.org. |
| `Error: Chrome for Testing publishes chrome-headless-shell for Linux x86_64 only (...)` | There is no build for this system. |
| `Error: chrome_headless_update needs 'unzip'.` | Install [unzip](../requirements.md#tool-unzip). |
| `Error: Missing shared libraries: ...` | The next line names the `apt-get install` command that adds them; run it, then `chri` again. |
| `Error: sudo needs a password: run interactively or configure NOPASSWD.` | An unattended run (`-y` without a terminal) cannot type a sudo password. Run `chri` in a terminal, or make the install directory writable. |
| `Error: No terminal to ask for confirmation: pass -y to go ahead.` | `chri` or `nvmc` ran without a terminal: add `-y` once you have read the plan (`--dry-run`). |

## See also

- [reference.md](reference.md): every function, alias and command of this area.
- [configuration](../configuration.md#settings-reference): `GASH_NVM`, `GASH_REPLACE_TOOLS`, `GASH_PATH_ADD`, and the [environment variables](../configuration.md#environment-only-variables) `GASH_CHROME_ROOT` and `GASH_CHROME_LINK`.
- [Local web stack (HTE)](../hte/README.md): local Apache and PHP-FPM sites, one pool per PHP version.
- [Tools for AI agents](../agents/README.md): `llm_runtime`, and `llm_tools` for the versions installed.
