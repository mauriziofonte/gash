# dev

PHP and Node.js versions, headless Chrome.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

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
node_detect_version [project_dir] [--json]
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
installed. JSON output is also emitted automatically under
GASH_HEADLESS=1, --json is not required there.
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
Needs nvm loaded (GASH_NVM=lazy or on does it) and network access to
nodejs.org. Without a terminal the question is refused: pass -y.
```

See also: `nvm_prune_versions`, `node_detect_version`

## nvm_prune_versions

Remove duplicate/older NVM patch versions

Alias: `nvmp`

#### Usage

```text
nvm_prune_versions [--force]
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
even if it is not the latest patch for its major.
```

## php_detect_version

Detect the PHP version a project wants

#### Usage

```text
php_detect_version [project_dir] [--json]
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
Priority: .target-php-version (walk-up) -> composer.json .require.php ->
.github/copilot-instructions.md -> fallback 8.4. JSON output is also
emitted automatically under GASH_HEADLESS=1, --json is not required there.
```

## Aliases

| Alias | Runs |
|---|---|
| `php` | `phpX.Y` |
| `composer-packages-update` | `composer global update` |
| `composer-self-update` | `sudo composer self-update` |
| `hte` | `sudo phpX.Y hte-cli create` |
| `hte-create` | `sudo phpX.Y hte-cli create` |
| `hte-remove` | `sudo phpX.Y hte-cli remove` |
| `hte-details` | `sudo phpX.Y hte-cli details` |

