# Files and archives

These functions answer the questions a full disk raises (which files and
which directories take the space, what a tree is made of, which directories
are empty) and handle two everyday file jobs: extracting an archive of any
common format without letting it write outside its folder, and keeping a
dated copy of a file before you edit it.

Platforms: Linux, macOS and WSL.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [unzip](../requirements.md#tool-unzip) (feature), for `archive_extract` (.zip)
- [bzip2](../requirements.md#tool-bzip2) (feature), for `archive_extract` (.bz2)
- [xz](../requirements.md#tool-xz) (feature), for `archive_extract` (.xz)
- [zstd](../requirements.md#tool-zstd) (feature), for `archive_extract` (.zst)
- [7z](../requirements.md#tool-7z) (feature), for `archive_extract` (.7z)
- [unrar](../requirements.md#tool-unrar) (feature), for `archive_extract` (.rar)
- [uncompress](../requirements.md#tool-uncompress) (feature), for `archive_extract` (.Z)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Find what fills a disk

Four functions look at the same tree from different sides:

| To see | Use | Size it reports |
|---|---|---|
| the biggest files | [`files_largest`](reference.md#files_largest) (`flf`) | the file's size |
| the biggest directories at one level | [`dirs_largest`](reference.md#dirs_largest) (`dld`) | disk usage, everything below included |
| every directory over a size, at any depth | [`dirs_find_large`](reference.md#dirs_find_large) (`dfl`) | disk usage, everything below included |
| totals and the file types that dominate | [`tree_stats`](reference.md#tree_stats) (`tls`) | the files' sizes |

All four start from the current directory when you give no path, and print
absolute paths (the examples on this page show your home as `~`). They skip
the folders that hold dependencies and caches, such as `node_modules/` and
`.git/`, unless you pass `--no-ignore`. To see which filesystems are full in
the first place, use [`disk_usage`](../system/README.md#check-disk-space); for
log files, [`gash logs`](../logs/README.md).

The examples below use this tree:

```text
proj/
├── assets/          video.mp4 (3 MiB), logo.png (40 KiB)
├── logs/            app.log (1.5 MiB)
│   └── old/         app-2025.log.gz (2.5 MiB)
├── node_modules/    lib/bundle.js (5 MiB)
└── src/             main.go
```

### The biggest files

`files_largest` lists files from the largest down, with their modification
time:

<!-- example
mkdir -p proj/assets proj/logs/old proj/src proj/node_modules/lib
head -c 3145728 /dev/zero >proj/assets/video.mp4
head -c 40960 /dev/zero >proj/assets/logo.png
head -c 1572864 /dev/zero >proj/logs/app.log
head -c 2621440 /dev/zero >proj/logs/old/app-2025.log.gz
head -c 5242880 /dev/zero >proj/node_modules/lib/bundle.js
printf 'package main\n' >proj/src/main.go
find proj -type f -exec touch -d '2026-01-15 09:30' {} +
touch -d '2026-02-20 18:05' proj/logs/app.log
-->
```console
$ files_largest proj
3.0 MiB       ~/proj/assets/video.mp4  2026-01-15 09:30
2.5 MiB       ~/proj/logs/old/app-2025.log.gz  2026-01-15 09:30
1.5 MiB       ~/proj/logs/app.log  2026-02-20 18:05
40.0 KiB      ~/proj/assets/logo.png  2026-01-15 09:30
13 B          ~/proj/src/main.go  2026-01-15 09:30
$ files_largest proj --min-size 2M
3.0 MiB       ~/proj/assets/video.mp4  2026-01-15 09:30
2.5 MiB       ~/proj/logs/old/app-2025.log.gz  2026-01-15 09:30
$ files_largest proj --no-ignore --limit 2
5.0 MiB       ~/proj/node_modules/lib/bundle.js  2026-01-15 09:30
3.0 MiB       ~/proj/assets/video.mp4  2026-01-15 09:30
```

`node_modules/lib/bundle.js` is the largest file, but only `--no-ignore`
shows it. The options that matter most:

- `--min-size SIZE` hides smaller files, and `--limit N` (default 100) keeps
  the N largest.
- `--exclude GLOB` skips names or paths below the start that match, and can
  be given more than once: `--exclude '*.iso' --exclude backups`.
- `--depth N` limits how deep the scan goes, and `--xdev` stays on the
  filesystem of the start directory (useful from `/home` when other disks are
  mounted below it).

### The biggest directories

`dirs_largest` shows the directories of one level, by disk usage (as `du`
counts it), each with everything below it. `--depth 2` adds the level below,
so a parent shows next to its children:

<!-- example
mkdir -p proj/assets proj/logs/old proj/src proj/node_modules/lib
head -c 3145728 /dev/zero >proj/assets/video.mp4
head -c 40960 /dev/zero >proj/assets/logo.png
head -c 1572864 /dev/zero >proj/logs/app.log
head -c 2621440 /dev/zero >proj/logs/old/app-2025.log.gz
head -c 5242880 /dev/zero >proj/node_modules/lib/bundle.js
printf 'package main\n' >proj/src/main.go
-->
```console
$ dirs_largest proj --min-size 1M
4.0 MiB       ~/proj/logs
3.0 MiB       ~/proj/assets
$ dirs_largest proj --depth 2 --min-size 1M
4.0 MiB       ~/proj/logs
3.0 MiB       ~/proj/assets
2.5 MiB       ~/proj/logs/old
```

`dirs_find_large` walks the whole tree and lists every directory over a
threshold, 20 MiB unless you pass `--size`. `--with-mtime` adds the time of
the newest file below each one, which tells an active directory from a
forgotten one:

<!-- example
mkdir -p proj/assets proj/logs/old proj/src proj/node_modules/lib
head -c 3145728 /dev/zero >proj/assets/video.mp4
head -c 40960 /dev/zero >proj/assets/logo.png
head -c 1572864 /dev/zero >proj/logs/app.log
head -c 2621440 /dev/zero >proj/logs/old/app-2025.log.gz
head -c 5242880 /dev/zero >proj/node_modules/lib/bundle.js
printf 'package main\n' >proj/src/main.go
find proj -type f -exec touch -d '2026-01-15 09:30' {} +
touch -d '2026-02-20 18:05' proj/logs/app.log
-->
```console
$ dirs_find_large proj --size 2M --with-mtime
4.0 MiB       ~/proj/logs  2026-02-20 18:05
3.0 MiB       ~/proj/assets  2026-01-15 09:30
2.5 MiB       ~/proj/logs/old  2026-01-15 09:30
```

Sizes count in powers of 1024 in both input and output: `--size 500M` means
500 MiB, and `1.5G`, `750K`, `20MiB` or a plain number of bytes work too.

### Use the results in a script

`--json` prints one JSON object with the list under `data`, the count, the
total and the number of paths that could not be read. `--null` prints only
the paths, each ended by a NUL byte, for `xargs -0`; paths that hold a tab or
a newline are left out of every output, so each record is a whole path.

<!-- example
mkdir -p proj/assets
head -c 3145728 /dev/zero >proj/assets/video.mp4
touch -d '2026-01-15 09:30' proj/assets/video.mp4
mask: "scan_time":"[0-9]+ms"
-->
```console
$ files_largest proj --json
{"data":[{"size":3145728,"size_human":"3.0 MiB","mtime":"2026-01-15 09:30","path":"~/proj/assets/video.mp4"}],"count":1,"total_size":3145728,"total_size_human":"3.0 MiB","kind":"files","scan_time":"0ms","errors":0}
```

## Summarize a tree

`tree_stats` gives the counts, the total and average size, how deep the files
go, how many directories are empty, and the file extensions that take the most
files and the most space (`--top N` for more than five):

<!-- example
mkdir -p proj/assets proj/logs/old proj/src proj/node_modules/lib
head -c 3145728 /dev/zero >proj/assets/video.mp4
head -c 40960 /dev/zero >proj/assets/logo.png
head -c 1572864 /dev/zero >proj/logs/app.log
head -c 2621440 /dev/zero >proj/logs/old/app-2025.log.gz
head -c 5242880 /dev/zero >proj/node_modules/lib/bundle.js
printf 'package main\n' >proj/src/main.go
mask: Scan time: +[0-9]+ms
-->
```console
$ tree_stats proj
Tree statistics for ~/proj
  Files:       5
  Directories: 5
  Total size:  7.0 MiB (7381005 bytes)
  Avg file:    1.4 MiB
  Max depth:   2
  Empty dirs:  0
  Scan time:   0ms

  Top 5 extensions by count:
    mp4               1 files  3.0 MiB
    gz                1 files  2.5 MiB
    log               1 files  1.5 MiB
    png               1 files  40.0 KiB
    go                1 files  13 B

  Top 5 extensions by size:
    mp4          3.0 MiB
    gz           2.5 MiB
    log          1.5 MiB
    png          40.0 KiB
    go           13 B
```

The extension is what follows the last dot of the name, in lower case; files
without one, and names such as `.env`, count as `(no ext)`.
`Max depth` counts the directory levels below the root: `logs/old/app-2025.log.gz`
is at depth 2 (one less than `find -maxdepth` counts, which also counts the
file itself).

## Find and remove empty directories

[`dirs_list_empty`](reference.md#dirs_list_empty) (`dle`) lists the
directories that hold nothing at all, not even hidden files. Removing them
can leave their parent empty, so list again after a removal:

<!-- example
mkdir -p uploads/2025/03 uploads/2026
printf 'jpeg\n' >uploads/2026/cover.jpg
-->
```console
$ dirs_list_empty uploads
~/uploads/2025/03
$ dirs_list_empty uploads --null | xargs -0 rmdir
$ dirs_list_empty uploads
~/uploads/2025
$ dirs_list_empty uploads --count
1
```

`--ignore-dotfiles` leaves out directories whose name starts with a dot, and
`--min-depth N` and `--depth N` set the levels it looks at.

## Extract an archive

[`archive_extract`](reference.md#archive_extract) (`axe`) picks the tool from
the file name: tar archives with any of the usual compressions, zip, rar, 7z,
and single compressed files such as `.gz` or `.zst` (the
[reference](reference.md#archive_extract) lists every suffix). It extracts
into the current directory, or into the directory you name, which it creates
when missing. tar, unzip, unrar and 7z print the names they extract:

<!-- example
mkdir -p site/css
printf '<h1>Hello</h1>\n' >site/index.html
printf 'body { margin: 0 }\n' >site/css/main.css
tar --sort=name -czf site.tar.gz site
rm -r site
printf 'listen 80;\n' >nginx.conf
gzip nginx.conf
-->
```console
$ axe site.tar.gz preview
site/
site/css/
site/css/main.css
site/index.html
$ ls preview/site
css
index.html
$ axe nginx.conf.gz
$ cat nginx.conf
listen 80;
```

A single compressed file becomes the file name without its suffix, next to
where you extract, and the archive stays. An existing file of that name is
replaced only when decompression succeeds, so a damaged archive never
destroys it.

Files from a tar archive belong to you and get your umask, even when you
extract as root. When nobody can answer (headless mode, or no terminal),
unzip, unrar and 7z never ask and never overwrite an existing file.

### What it refuses

Before it writes anything, `archive_extract` reads the list of members of a
tar, zip, 7z or rar archive and refuses the whole archive when one member
would land outside the destination: an absolute path, a `..` component, or a
link that points outside:

<!-- example
mkdir work
printf 'x\n' >escape.txt
cd work && tar -P -cf ../evil.tar ../escape.txt && cd ..
rm -r work escape.txt
ln -s /etc/passwd passwd
tar -cf links.tar passwd
rm passwd
-->
```console
$ axe evil.tar
Error: Archive 'evil.tar' has an unsafe member '../escape.txt' (absolute path, .. component or link outside the destination); nothing extracted.
$ axe links.tar
Error: Archive 'links.tar' has an unsafe member 'passwd' (absolute path, .. component or link outside the destination); nothing extracted.
```

Nothing is extracted in either case and the status is 1. When the member list
cannot be read (a damaged header), the check is skipped and the tool's own
handling applies. It also refuses, before writing anything:

- a format it does not know, or a missing tool, which it names;
- a `.zip` or `.rar` whose name contains `*`, `?` or `[`, which unzip and
  unrar would read as a wildcard: rename the file;
- a single compressed file whose output would overwrite the archive itself.

## Back up a file before editing it

[`file_backup`](reference.md#file_backup) (`fbk`) copies a file next to
itself, with the date and time in the name (local time,
`_backup_YYYYMMDDHHMMSS`):

<!-- example
printf 'listen 80;\n' >nginx.conf
mask: _backup_[0-9]{14}
-->
```console
$ file_backup nginx.conf
'nginx.conf' -> 'nginx.conf_backup_20261026084731'
```

A backup never replaces an existing file: a second one in the same second
gets `_1`, then `_2`. For a symbolic link, the copy is of the file it points
to. The copy keeps the permission bits (without setuid, setgid and sticky,
and with your umask applied) and gets the current time. Use it before an
edit: `fbk ~/.bashrc && nano ~/.bashrc`.

## When something goes wrong

The scanners print an error code and a hint, as a JSON envelope on stderr
with `--json`:

| Message | Meaning and what to do |
|---|---|
| `Error: root_scan_blocked: Scanning '/' is blocked` | A scan of `/` walks virtual filesystems and is slow. Scan a directory, or pass `--allow-root` (with `--xdev` to stay on the root filesystem) |
| `Error: forbidden_path: PATH` | System trees such as `/proc`, `/sys` and `/dev`, and the root user's home, are never scanned, not even through a symbolic link |
| `Error: path_not_found: Path 'PATH' does not exist` | Check the path; `~` and `~/...` are expanded, other forms are taken as written |
| `Error: not_a_directory: ...` | The scanners start from a directory, not a file |
| `Error: invalid_size: VALUE` | Use a number with an optional unit: `500K`, `20M`, `1.5G` |
| `Error: unknown_option: OPTION` | `gash_help NAME` lists the options |
| `Warning: Scan completed with N unreadable path(s) (permission denied).` | The results leave out the N paths you may not read; the status is still 0 |

`archive_extract` and `file_backup`:

| Message | Meaning and what to do |
|---|---|
| `Error: Command 'unzip' is not installed or not in PATH.` | Install the tool the format needs ([requirements](../requirements.md#tool-unzip)); for `.zst` the message is `Extracting .zst needs zstd.` Nothing was written |
| `Error: Cannot extract 'FILE', unsupported file type.` | The suffix is not one the [reference](reference.md#archive_extract) lists; rename the file if its suffix is wrong |
| `Error: Archive 'FILE' has an unsafe member 'NAME' ...` | See [What it refuses](#what-it-refuses). Inspect the archive with `tar -tvf`, `unzip -l` or `7z l` before you trust it |
| `Error: Archive name 'FILE' contains wildcard characters ...` | Rename the archive without `*`, `?` and `[` |
| A tool's own error, then status 1 | The tool ran and failed, for example on a damaged archive; its message says why |
| `Error: File 'FILE' does not exist.` or `Error: 'FILE' is not a regular file.` | Both functions take one regular file |
| `Error: Failed to create backup file.` | The directory of the file is not writable for you, or the disk is full; nothing partial is left |

## See also

- [reference.md](reference.md): every function of this area, with all options.
- [System](../system/README.md): `disk_usage` and the other system checks.
- [Logs](../logs/README.md): find the log files that fill a disk and reclaim the space.
- [Tools for AI agents](../agents/README.md): `llm_find`, `llm_tree` and the other read-only JSON tools.
