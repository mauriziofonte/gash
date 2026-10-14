# dev

PHP and Node.js versions per project.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`node_detect_version`](#node-detect-version) |  | Detect the Node.js version a project wants |
| [`nvm_prune_versions`](#nvm-prune-versions) | `nvmp` | Remove duplicate/older NVM patch versions |
| [`php_detect_version`](#php-detect-version) |  | Detect the PHP version a project wants |

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
| `php` | `` |
| `composer-packages-update` | ` global update` |
| `composer-self-update` | `sudo self-update` |
| `hte` | ` create` |
| `hte-create` | ` create` |
| `hte-remove` | ` remove` |
| `hte-details` | ` details` |

