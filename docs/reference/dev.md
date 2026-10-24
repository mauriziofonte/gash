# dev

PHP and Node.js versions, headless Chrome.

_Written from the help registry (`gash_help NAME`); `go test ./internal/registry` checks it against the help text._

| Function | Alias | Description |
|---|---|---|
| [`chrome_headless_update`](#chrome-headless-update) | `chri` | Install or upgrade chrome-headless-shell (Chrome for Testing) |
| [`node_detect_version`](#node-detect-version) |  | Detect the Node.js version a project wants |
| [`node_upgrade`](#node-upgrade) | `nvmc` | Upgrade each installed Node.js major to its newest patch |
| [`nvm_prune_versions`](#nvm-prune-versions) | `nvmp` | Remove duplicate/older NVM patch versions |
| [`php_detect_version`](#php-detect-version) |  | Detect the PHP version a project wants |

## chrome_headless_update

Install or upgrade chrome-headless-shell (Chrome for Testing)

Alias: `chri`

#### Usage

```text
chrome_headless_update [-y] [--dry-run]
```

#### Examples

```bash
# Show what would change, touch nothing
chri --dry-run

# After upgrading Playwright: bring the shared browser to the same build
chri

# Unattended (CI, provisioning scripts)
chri -y
```

#### Notes

```text
The version is the chromium-headless-shell build listed in browsers.json
of the latest playwright-core release, downloaded from Chrome for Testing
(exact version, never "latest stable"). Linux x86_64 only.
Layout: /opt/chrome-headless-shell/<version>/ with the symlink
/usr/local/bin/chrome-headless-shell (GASH_CHROME_ROOT and
GASH_CHROME_LINK change them). The outgoing version is kept for
rollback, older ones are removed. Installing uses sudo only when ROOT
or the link directory is not writable; headless or without a terminal
it runs sudo -n and stops early when sudo wants a password. The
binary's --version must name the version exactly, also for a copy
already under ROOT (one that does not is replaced).
Missing shared libraries are reported with the apt packages to install.
Point your tools at the shared binary and stop npm from downloading
browsers of its own:
  export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 PUPPETEER_SKIP_DOWNLOAD=1
  Playwright: launchOptions: { executablePath: '/usr/local/bin/chrome-headless-shell' }
  Puppeteer:  { headless: 'shell', executablePath: '/usr/local/bin/chrome-headless-shell' }
Without a terminal the question is refused: pass -y.
```

See also: `node_upgrade`

## node_detect_version

Detect the Node.js version a project wants

#### Usage

```text
node_detect_version [project_dir] [--json|--llm]
```

#### Examples

```bash
# Human-readable, current directory
node_detect_version

# JSON, specific project
node_detect_version /path/to/project --json
```

#### Notes

```text
Priority: .nvmrc -> .node-version -> package.json engines.node -> latest
installed, read in project_dir only (no walk-up). Every source is
reduced to its major (lts/NAME through nvm's alias files, then the
codename table), and full_version is the newest installed patch of
that major: an exact pin such as v20.9.0 resolves to the newest
installed v20 patch. Installed versions are the vX.Y.Z directories of
$NVM_DIR/versions/node (prereleases and other names do not count).
When the major is not installed, a warning names it and the newest
installed version is used. A project_dir that is not a directory gives
"Warning: Directory not found: DIR" with the same answer and status 0.
Unknown options are refused before the NVM directory is checked.
JSON output is also emitted automatically under GASH_HEADLESS=1;
--json (or --llm) is not required there.
```

## node_upgrade

Upgrade each installed Node.js major to its newest patch

Alias: `nvmc`

#### Usage

```text
node_upgrade [-y] [--dry-run]
```

#### Examples

```bash
# See the plan, change nothing
nvmc --dry-run

# Upgrade: plan, one question, then install and clean up
nvmc

# Unattended
nvmc -y
```

#### Notes

```text
For each major already installed (v18, v20, v22...) installs the newest
published patch with the global packages of the patch it replaces
(nvm install --reinstall-packages-from), then removes the older patches
of that major. Majors that are not installed are never added and a
major is never removed. When an install fails, that major keeps its
patches. The active version moves to the new patch of its major.
Exits 1 when an install or a removal failed.
Needs nvm loaded (GASH_NVM=lazy or on does it) and network access to
nodejs.org. Without a terminal the question is refused: pass -y.
```

See also: `nvm_prune_versions`, `node_detect_version`

## nvm_prune_versions

Remove duplicate/older NVM patch versions

Alias: `nvmp`

#### Usage

```text
nvm_prune_versions [-y|--yes|--force]
```

#### Examples

```bash
# Preview what would be removed (default, no changes made)
nvmp

# Actually uninstall the duplicate patch versions
nvmp --force
```

#### Notes

```text
For each installed major version, keeps only the latest patch.
The currently active version (nvm current) is never uninstalled,
even if it is not the latest patch for its major. -y and --yes are
the same as --force. Exits 1 when a removal failed.
```

## php_detect_version

Detect the PHP version a project wants

#### Usage

```text
php_detect_version [project_dir] [--json|--llm]
```

#### Examples

```bash
# Human-readable, current directory
php_detect_version

# JSON, specific project
php_detect_version /path/to/project --json
```

#### Notes

```text
Priority: .target-php-version (the first X.Y of its first line, looked
up in project_dir, then in every parent directory except /) ->
composer.json .require.php -> .github/copilot-instructions.md (the
first "php X.Y") -> fallback 8.4. composer.json and
copilot-instructions.md are read in project_dir only.
A composer constraint is split on | and ||; each alternative counts by
its lower bound (^7.4 is 7.4; a bare major such as ^8, >=8 or 8.* is
8.0; * alone counts for nothing). The highest alternative whose
/usr/bin/phpX.Y is installed wins, else the highest alternative.
When /usr/bin/phpX.Y is not installed, a warning on stderr says so and
names the binary used instead (/usr/bin/phpX, then php on PATH); the
JSON is unchanged. A project_dir that is not a directory gives
"Warning: Directory not found: DIR" with the same answer and status 0.
JSON output is also emitted automatically under GASH_HEADLESS=1;
--json (or --llm) is not required there.
```

## Aliases

The php pack is loaded in interactive shells when `php` is on PATH.
`phpX.Y` below is the absolute path of that binary followed by
`-d allow_url_fopen=1 -d memory_limit=2048M`.

| Alias | Runs | Defined |
|---|---|---|
| `phpXY` (`php85` ... `php56`) | `phpX.Y` | for each installed phpX.Y |
| `composerXY` | `phpX.Y composer` | for each installed phpX.Y, when composer is installed |
| `1composerXY` (`1composer72`, `1composer71`, `1composer70`, `1composer56`) | `phpX.Y composer1` | for each installed phpX.Y of 7.2, 7.1, 7.0 and 5.6, when composer1 is installed |
| `php` | newest `phpX.Y` | with GASH_REPLACE_TOOLS=1 |
| `composer` | newest `phpX.Y composer` | with GASH_REPLACE_TOOLS=1, when composer is installed |
| `composer-packages-update` | `composer global update` | when composer is installed |
| `composer-self-update` | `sudo composer self-update` | when composer is installed |
| `composer1-self-update` | `sudo composer1 self-update` | when composer1 is installed |

The 2.x `hte`, `hte-create`, `hte-remove` and `hte-details` aliases are
gone: use `gash hte`.
