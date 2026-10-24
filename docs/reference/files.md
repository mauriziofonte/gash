# files

Largest files and folders, tree stats, archives, backups.

_Written from the help registry (`gash_help NAME`); `go test ./internal/registry` checks it against the help text._

| Function | Alias | Description |
|---|---|---|
| [`archive_extract`](#archive-extract) | `axe` | Extract archives (tar, zip, gz, bz2, xz, zst, Z, rar, 7z) |
| [`dirs_find_large`](#dirs-find-large) | `dfl` | Find directories exceeding a size threshold (single-walk aggregation) |
| [`dirs_largest`](#dirs-largest) | `dld` | List the largest directories with depth and filter control |
| [`dirs_list_empty`](#dirs-list-empty) | `dle` | List empty directories with depth, prune and count options |
| [`file_backup`](#file-backup) | `fbk` | Create a timestamped backup of a file |
| [`files_largest`](#files-largest) | `flf` | List the largest files under a path with size/exclude/json filters |
| [`tree_stats`](#tree-stats) | `tls` | Compact filesystem stats: totals, top extensions, depth, empty dirs |

## archive_extract

Extract archives (tar, zip, gz, bz2, xz, zst, Z, rar, 7z)

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
.tar.gz, .tar.bz2, .tar.xz, .txz, .tar.zst, .tzst, .tar.Z, .taz, .tar,
.gz, .bz2, .xz, .zst, .Z (compress), .z (pack),
.tgz, .tbz2, .zip, .rar, .7z
(case-insensitive)
```

#### Notes

```text
Tools: tar (with zstd for .tar.zst/.tzst, uncompress for .tar.Z/.taz),
gunzip, bunzip2, xz, zstd and uncompress (.z needs gzip's uncompress)
for the single-file formats, unzip, unrar and 7z. A missing tool is
reported before anything is written. OUTPUT_DIR (default: the current
directory) is created when missing.
Single-file formats write the archive name without its suffix into
OUTPUT_DIR; an existing file of that name is replaced only when the
tool succeeds.
Members with an absolute path, a .. component or a link outside
OUTPUT_DIR are refused before anything is extracted.
The tools' listings go to stdout. Exit status: 0 on success, 1 on any
failure.
With GASH_HEADLESS=1 or without a terminal on stdin, unzip, unrar and
7z never prompt and never overwrite existing files; an encrypted 7z
archive prints its password prompt and stops on end of input.
.zip and .rar names containing *, ? or [ are refused (unzip and unrar
would expand them as wildcards): rename the file.
The tools' own configuration applies: TAR_OPTIONS, UNZIP, RAR and
~/.rarrc, GZIP, BZIP2, XZ_OPT and similar variables.
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
--size SIZE            Size threshold (default 20M): bytes, or a number with
                       up to 6 decimals and K, M, G, T, P or E, optionally
                       followed by B, i or iB
--depth N              Depth limit (default unbounded; 0 also means unbounded)
-n, --limit N          Cap results (default 1000)
--exclude GLOB         Skip entries whose name or path below PATH matches GLOB
                       (repeatable; no newlines)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--allow-root           Required to scan '/'
--with-mtime           Append newest-file mtime per directory (extra cost per match)
--json                 Emit JSON envelope
--null, -0             NUL-delimited paths
--human / --no-human   Size formatting toggle
--no-color             Disable ANSI colors (layout unchanged; NO_COLOR also honored)
```

#### Performance

```text
One walk adds up disk usage per directory (as `du -k` counts it): O(N)
in filesystem entries, versus the legacy per-directory re-walk which was
O(N^2).
```

#### Notes

```text
Paths containing literal tab or newline characters are excluded from
results (they cannot be represented safely in TSV/line-oriented output).
--null records are therefore always intact, complete paths.
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
--depth N              Depth limit (default 1, top-level only; 0 means 1).
                       Use higher values to reveal nested offenders
                       (node_modules, vendor/, target/).
--min-size SIZE        Ignore directories smaller than SIZE: bytes,
                       or a number with up to 6 decimals and K, M, G, T,
                       P or E, optionally followed by B, i or iB
--exclude GLOB         Skip entries whose name or path below PATH matches GLOB
                       (repeatable; no newlines)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--allow-root           Required to scan '/'
--json                 Emit JSON envelope
--null, -0             NUL-delimited paths
--human / --no-human   Force human-readable / raw-bytes sizes
--no-color             Disable ANSI colors (layout unchanged; NO_COLOR also honored)
```

#### Notes

```text
Sizes are cumulative (du semantics). Deeper scans will show parent
directories alongside their subdirs; filter with --min-size and --depth
to narrow the view.
Paths containing literal tab or newline characters are excluded from
results (they cannot be represented safely in TSV/line-oriented output).
--null records are therefore always intact, complete paths.
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
--exclude GLOB     Skip entries whose name or path below PATH matches GLOB
                   (repeatable; no newlines)
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
Paths containing literal tab or newline characters are excluded from
results (they cannot be represented safely in TSV/line-oriented output).
--null records are therefore always intact, complete paths.
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

#### Notes

```text
The stamp is local time (YYYYMMDDHHMMSS, no zone). A backup never
replaces an existing file or link: a second backup in the same second
gets _1, then _2, and so on.
A symlinked FILE is followed: the backup is a copy of its target, next
to FILE. On success cp -v prints one line on stdout:
'SRC' -> 'DST'. A failed copy removes the partial backup (exit 1).
Mode: FILE's permission bits without setuid, setgid and sticky, with
the umask applied; the modification time is the time of the copy.
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

# JSON output for scripting
# (envelope: data/count/total_size/total_size_human/kind/scan_time/errors)
files_largest /home --json | jq '.data[] | select(.size > 1073741824)'

# NUL-delimited paths for safe xargs pipelines
files_largest --min-size 100M --null /tmp | xargs -0 rm -i

# Include vendored dirs (disable default noise pruning)
files_largest --no-ignore ~/projects
```

#### Options

```text
-n, --limit N          Cap results (default 100)
--min-size SIZE        Ignore files smaller than SIZE: bytes, or a
                       number with up to 6 decimals and K, M, G, T, P or
                       E, optionally followed by B, i or iB
--exclude GLOB         Skip entries whose name or path below PATH matches GLOB
                       (repeatable; no newlines)
--depth N              Limit scan depth (default: unbounded)
--no-ignore            Disable default pruning (node_modules/.git/vendor/__pycache__/.cache)
--xdev                 Stay on a single filesystem
--follow-symlinks, -L  Follow symbolic links (never into protected paths)
--allow-root           Required to scan '/' (safety guard)
--json                 Emit JSON envelope: {data, count, total_size,
                       total_size_human, kind, scan_time, errors}
--null, -0             NUL-delimited paths for xargs -0
--human                Human-readable sizes (default except with --json or --null)
--no-color             Disable ANSI colors (layout unchanged; NO_COLOR also honored)
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
tree_stats --xdev --depth 8 --allow-root /
```

#### Options

```text
--depth N              Scan depth limit (default unbounded)
--top N                Number of extensions in top lists (default 5)
--exclude GLOB         Skip entries whose name or path below PATH matches GLOB
                       (repeatable; no newlines)
--no-ignore            Disable default pruning
--xdev                 Stay on a single filesystem
--follow-symlinks, -L  Follow symbolic links (never into protected paths)
--allow-root           Required to scan '/'
--json                 Emit JSON envelope
--no-color             Disable ANSI colors (layout unchanged; NO_COLOR also honored)
```

#### Notes

```text
Single-walk aggregation. Extensions are derived from the basename
(ASCII-lowercased, after the last dot). Files with no dot or
hidden-only dot (.env) are grouped as "(no ext)".
Paths containing literal tab or newline characters are excluded from
every count and total (they cannot be represented safely in
line-oriented output).
```

See also: `files_largest`, `dirs_largest`, `dirs_find_large`

