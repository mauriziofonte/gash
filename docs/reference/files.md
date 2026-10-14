# files

Largest files and folders, tree stats, archives, backups.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`archive_extract`](#archive-extract) | `axe` | Extract archives (tar, zip, gz, bz2, rar, 7z) |
| [`dirs_find_large`](#dirs-find-large) | `dfl` | Find directories exceeding a size threshold (single-walk aggregation) |
| [`dirs_largest`](#dirs-largest) | `dld` | List the largest directories with depth and filter control |
| [`dirs_list_empty`](#dirs-list-empty) | `dle` | List empty directories with depth, prune and count options |
| [`file_backup`](#file-backup) | `fbk` | Create a timestamped backup of a file |
| [`files_largest`](#files-largest) | `flf` | List the largest files under a path with size/exclude/json filters |
| [`tree_stats`](#tree-stats) | `tls` | Compact filesystem stats: totals, top extensions, depth, empty dirs |

## archive_extract

Extract archives (tar, zip, gz, bz2, rar, 7z)

Alias: `axe`

#### Usage

```text
archive_extract ARCHIVE_FILE [OUTPUT_DIR]
```

#### Examples

```bash
# Extract a tar.gz in the current directory
axe backup.tar.gz

# Extract a zip to a specific folder
axe release.zip /opt/app

# Extract a 7z archive
axe data.7z /tmp/extracted
```

#### Supported formats

```text
.tar.gz, .tar.bz2, .tar, .gz, .bz2,
.tgz, .tbz2, .zip, .rar, .7z, .z
(case-insensitive)
```

See also: `file_backup`

## dirs_find_large

Find directories exceeding a size threshold (single-walk aggregation)

Alias: `dfl`

#### Usage

```text
dirs_find_large [DIRECTORY] [--size SIZE] [--depth N] [--limit N]
                [--exclude GLOB] [--no-ignore] [--xdev]
                [--allow-root] [--with-mtime] [--json] [--null]
                [--human] [--no-color]
```

#### Examples

```bash
# Find directories larger than 20MB (default) in current path
dirs_find_large

# Find directories larger than 1GB with newest-file timestamp
dirs_find_large --size 1G --with-mtime /var

# JSON envelope for scripting
dirs_find_large --size 500M --json /home | jq '.data'

# Constrain depth to avoid long walks
dirs_find_large --size 100M --depth 6 /opt
```

#### Options

```text
--size SIZE            Size threshold (default 20M; formats: 20M, 1G, 500K, bytes)
--depth N              Depth limit (default unbounded; use to cap very deep trees)
-n, --limit N          Cap results (default 1000)
--exclude GLOB         Prune matching paths (repeatable)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--allow-root           Required to scan '/'
--with-mtime           Append newest-file mtime per directory (extra cost per match)
--json                 Emit JSON envelope
--null, -0             NUL-delimited paths
--human / --no-human   Size formatting toggle
--no-color             Disable ANSI colors
```

#### Performance

```text
Uses a single `du -k` pass plus awk filtering: O(N) in filesystem entries,
versus the legacy per-directory re-walk which was O(N^2). Expect 10x-100x
speedup on large trees.
```

#### Notes

```text
numfmt is optional: a pure-bash IEC-suffix parser provides fallback.
```

See also: `dirs_largest`, `files_largest`, `tree_stats`, `disk_usage`

## dirs_largest

List the largest directories with depth and filter control

Alias: `dld`

#### Usage

```text
dirs_largest [PATH] [--limit N] [--depth N] [--min-size SIZE]
             [--exclude GLOB] [--no-ignore] [--xdev]
             [--allow-root] [--json] [--null] [--human] [--no-color]
```

#### Examples

```bash
# Biggest top-level dirs in the current path (default depth 1)
dirs_largest

# Drill two levels deep, smallest 100MB
dirs_largest --depth 2 --min-size 100M /home

# Find which user's home is eating disk space (JSON envelope)
dirs_largest /home --json | jq '.data[:5]'

# NUL-delimited paths, suitable for xargs -0 du -sh
dirs_largest --null --depth 3 ~/projects | xargs -0 du -sh
```

#### Options

```text
-n, --limit N          Cap results (default 100)
--depth N              Depth limit (default 1, top-level only). Use higher values
                       to reveal nested offenders (node_modules, vendor/, target/).
--min-size SIZE        Ignore directories smaller than SIZE
--exclude GLOB         Prune matching paths (repeatable, substring match)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--allow-root           Required to scan '/'
--json                 Emit JSON envelope
--null, -0             NUL-delimited paths
--human / --no-human   Force human-readable / raw-bytes sizes
--no-color             Disable ANSI colors
```

#### Notes

```text
Sizes are cumulative (du semantics). Deeper scans will show parent
directories alongside their subdirs; filter with --min-size and --depth
to narrow the view.
```

See also: `files_largest`, `dirs_find_large`, `tree_stats`, `disk_usage`

## dirs_list_empty

List empty directories with depth, prune and count options

Alias: `dle`

#### Usage

```text
dirs_list_empty [PATH] [--min-depth N] [--depth N] [--exclude GLOB]
                [--ignore-dotfiles] [--no-ignore] [--xdev]
                [--allow-root] [--null] [--count] [--json]
```

#### Examples

```bash
# Find empty dirs in current path
dirs_list_empty

# Count only
dirs_list_empty /var/www --count

# Remove safely with xargs -0 (NUL-delimited)
dirs_list_empty --null /opt/app/uploads | xargs -0 rmdir

# Skip hidden (dot-prefixed) dirs
dirs_list_empty --ignore-dotfiles ~/projects
```

#### Options

```text
--min-depth N      Minimum depth (find -mindepth)
--depth N          Maximum depth (find -maxdepth)
--exclude GLOB     Prune matching paths
--ignore-dotfiles  Skip directories whose name starts with '.'
--no-ignore        Disable default pruning
--xdev             Stay on a single filesystem
--allow-root       Required to scan '/'
--null, -0         NUL-delimited output for xargs -0
--count, -c        Print count only (to stdout)
--json             Emit JSON envelope: {data, count, path, errors}
```

#### Notes

```text
"Empty" means no regular entries (not even hidden files). A directory
containing only a prune-excluded subdirectory is NOT empty.
```

See also: `files_largest`, `dirs_largest`, `tree_stats`

## file_backup

Create a timestamped backup of a file

Alias: `fbk`

#### Usage

```text
file_backup FILE
```

#### Examples

```bash
# Backup a config before editing
fbk /etc/nginx/nginx.conf
# Creates: /etc/nginx/nginx.conf_backup_20240115143022

# Backup-then-edit workflow
fbk /etc/apache2/apache2.conf && vim /etc/apache2/apache2.conf

# Backup a script before refactoring
fbk my-deploy-script.sh
```

See also: `archive_extract`

## files_largest

List the largest files under a path with size/exclude/json filters

Alias: `flf`

#### Usage

```text
files_largest [PATH] [--limit N] [--min-size SIZE] [--exclude GLOB]
              [--depth N] [--no-ignore] [--xdev] [--follow-symlinks]
              [--allow-root] [--json] [--null] [--human] [--no-color]
```

#### Examples

```bash
# Find the biggest files in the current directory
files_largest

# Top 20 files larger than 50MB, stay on same filesystem
files_largest --limit 20 --min-size 50M --xdev /var/log

# JSON output for scripting (envelope: data/count/total_size/scan_time)
files_largest /home --json | jq '.data[] | select(.size > 1073741824)'

# NUL-delimited paths for safe xargs pipelines
files_largest --min-size 100M --null /tmp | xargs -0 rm -i

# Include vendored dirs (disable default noise pruning)
files_largest --no-ignore ~/projects
```

#### Options

```text
-n, --limit N          Cap results (default 100)
--min-size SIZE        Ignore files smaller than SIZE (e.g. 10M, 1G, 500K)
--exclude GLOB         Prune matching paths (repeatable, substring match)
--depth N              Limit scan depth (default: unbounded)
--no-ignore            Disable default pruning (node_modules/.git/vendor/__pycache__/.cache)
--xdev                 Stay on a single filesystem
--follow-symlinks, -L  Follow symbolic links
--allow-root           Required to scan '/' (safety guard)
--json                 Emit JSON envelope: {data, count, total_size, scan_time, errors}
--null, -0             NUL-delimited paths for xargs -0
--human                Force human-readable sizes (auto-on on TTY)
--no-color             Disable ANSI colors (also honors NO_COLOR)
```

#### Notes

```text
Paths containing literal tab or newline characters are excluded from
results (they cannot be represented safely in TSV/line-oriented output).
--null records are therefore always intact, complete paths.
```

See also: `dirs_largest`, `dirs_find_large`, `tree_stats`

## tree_stats

Compact filesystem stats: totals, top extensions, depth, empty dirs

Alias: `tls`

#### Usage

```text
tree_stats [PATH] [--depth N] [--top N] [--exclude GLOB]
           [--no-ignore] [--xdev] [--follow-symlinks]
           [--allow-root] [--json] [--no-color]
```

#### Examples

```bash
# Audit current directory
tree_stats

# Drill into a projects tree, top 10 extensions
tree_stats --top 10 ~/projects

# JSON envelope (useful for dashboards / CI)
tree_stats /var/log --json | jq '.data.top_by_size'

# Stay on one filesystem, cap scan depth
tree_stats --xdev --depth 8 /
```

#### Options

```text
--depth N              Scan depth limit (default unbounded)
--top N                Number of extensions in top lists (default 5)
--exclude GLOB         Prune matching paths (repeatable)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--follow-symlinks, -L  Follow symbolic links
--allow-root           Required to scan '/'
--json                 Emit JSON envelope
--no-color             Disable ANSI colors
```

#### Notes

```text
Single-walk aggregation via `find -printf` + awk. Extensions are derived
from the basename (lowercased, after the last dot). Files with no dot
or hidden-only dot (.env) are grouped as "(no ext)".
```

See also: `files_largest`, `dirs_largest`, `dirs_find_large`

