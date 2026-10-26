# Logs

gash logs: find the log files that fill a disk and reclaim the space safely.

Log files fill disks quietly: a framework that writes megabytes a day, a
pile of rotated `syslog.7.gz` copies, a forgotten `npm-debug.log`.
`gash logs scan` finds them and shows, by category, how much space they hold;
`gash logs clean` gets the space back in the way that is safe on a running
system: it deletes the old copies that nothing writes to any more, and, when
you ask, empties the live logs in place instead of deleting them.

Platforms: Linux, macOS and WSL. With no path given, the scan includes
`/var/log`; reclaiming the files there that belong to root needs `sudo`.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [sudo](../requirements.md#tool-sudo) (feature), for `gash logs` (clean --sudo)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

## Find the logs that fill a disk

`gash logs scan` walks the directories you name and reports the log files by
category. It reads only file metadata, never contents, does not follow
symbolic links, skips `.git/`, `node_modules/` and `vendor/`, and changes
nothing:

<!-- example
mkdir -p shop/storage/logs shop/public
truncate -s 3M shop/storage/logs/laravel.log
truncate -s 2M shop/storage/logs/worker.log.1
truncate -s 1M shop/storage/logs/worker.log.2.gz
truncate -s 40K shop/npm-debug.log
printf '<?php\n' >shop/public/index.php
-->
```console
$ gash logs scan shop

────────────────────────────────────────────────────────────
Log files

category   files  size      reclaimable  
──────────────────────────────────────────────────────────
framework  3      6.0 MiB   6.0 MiB      ━━━━━━━━━━━━━━━━ 
node       1      40.0 KiB  40.0 KiB     ━─────────────── 
╭────────────────────────────────╮
│  Scanned:     4 files in shop  │
│  Total size:  6.0 MiB          │
│  Reclaimable: 6.0 MiB          │
╰────────────────────────────────╯
Hint: gash logs clean deletes rotated logs; add --truncate for live ones, --sudo for /var/log.
```

With no path, it scans the current directory, the user log directories that
exist (`~/.npm/_logs`, `~/.pm2/logs`, `~/.cache/pip/log`, `~/.local/state`)
and `/var/log`. Three options narrow the scan, and work the same for `clean`:

- `--min-size SIZE` leaves out smaller files (`512K`, `10M`, `1G`; a bare
  number is bytes);
- `--older-than AGE` leaves out files modified more recently (`7d`, `48h`,
  `30m`);
- `--depth N` stops N directory levels below each path (`--depth 1`: only
  the files directly in it).

See [`gash logs scan`](reference.md#gash-logs-scan).

## What counts as a log

A file counts by its name, and by the directory it sits in:

| Kind | Examples | What `clean` does |
|---|---|---|
| Live log | `app.log`, `worker.out`, `php_errors.log`, `debug.log`; a file without an extension, or ending in `.txt`, inside a log directory (`log`, `logs`, `nginx`, ...) | Nothing, unless you pass `--truncate`: then it is emptied in place |
| Rotated copy | a numbered or dated copy (`access.log.1`, `app.log.2026-10-24`) and any compressed copy (`error.log.3.gz`, `.xz`, `.bz2`, `.zst`, `.lz4`) | Deleted |
| systemd journal | `*.journal` | Never touched; the report shows their size, to clear with `journalctl --vacuum-size=...` |

A log that carries its date in the middle of its name, as Laravel's daily
logs do (`laravel-2026-10-24.log`), ends in `.log` and so counts as a live
log: `clean` empties it with `--truncate` rather than deleting it.

The categories of the report (`web-server`, `php`, `node`, `python`,
`package-manager`, `database`, `framework`, `systemd-journal`, `other`) come
from the directory names (`nginx`, `apache2`, `storage`, ...) and the file
names (`access`, `npm-debug`, ...).

## Reclaim the space

Preview first: `--dry-run` shows what would be freed and changes nothing.
Then run `clean`, which shows the files it will change and asks; `--yes` (or
`-y`) skips the question:

<!-- example
mkdir -p shop/storage/logs
truncate -s 3M shop/storage/logs/laravel.log
truncate -s 2M shop/storage/logs/worker.log.1
truncate -s 1M shop/storage/logs/worker.log.2.gz
truncate -s 40K shop/npm-debug.log
-->
```console
$ gash logs clean shop --dry-run
OK: Would reclaim 3.0 MiB (2 deleted, 0 truncated)
$ gash logs clean shop --truncate --yes
OK: Reclaimed 6.0 MiB (2 deleted, 2 truncated)
```

Without `--truncate`, only the rotated copies are deleted (the first line
above: 3 MiB). With it, the live logs are emptied as well. Emptying keeps the
file, its owner and its mode, so a program that holds the log open keeps
writing to it: there is no service to restart and no space lost to a deleted
file a process still holds. Empty live logs are left alone.

`clean` without `--yes` needs a terminal to ask on. From a script, a cron
job or an agent it stops with a `confirmation_required` error and changes
nothing; pass `--yes` once you have read the `--dry-run` output.

## Reclaim root-owned logs with sudo

Without `--sudo`, `clean` changes only the files your user may change (a
file you may write is truncated, a file in a directory you may write is
deleted). The others are counted and left, with a warning:

```text
Warning: 14 root-owned files (1.2 GiB) were left: re-run with --sudo to reclaim them.
```

`--sudo` reclaims them too, with at most two commands: one
`sudo rm -f -- FILE...` for the deletions and one `sudo truncate -s 0 -- FILE...`
for the truncations, each with the exact list of files, never through a
shell. `sudo` asks for your password as usual.

```bash
gash logs scan /var/log --min-size 10M
gash logs clean /var/log --sudo --dry-run
gash logs clean /var/log --sudo --older-than 7d
```

With `--dry-run`, `--sudo` counts those files in the total without running
`sudo`.

## The JSON form

Both commands take `--json` and then print one JSON object on stdout:

- `scan --json`: `roots`, `groups` (per category: `files`, `size`,
  `reclaim`, `active`, `rotated` and the `entries`, each with `path`,
  `size`, `modified`, `kind` and `reclaim`), `total_files`, `total_size`,
  `total_reclaim`, `journal_bytes` and `unreadable` (files the scan could not
  read);
- `clean --json`: `dry_run`, `deleted`, `truncated`, `freed`, `failed`,
  `need_root`, `need_root_bytes` and `scanned_files`.

<!-- example
mkdir -p shop/storage/logs
truncate -s 2M shop/storage/logs/worker.log.1
truncate -s 1M shop/storage/logs/worker.log.2.gz
-->
```console
$ gash logs clean shop --dry-run --json
{"dry_run":true,"deleted":2,"truncated":0,"freed":3145728,"failed":0,"need_root":0,"need_root_bytes":0,"scanned_files":2}
```

Sizes are in bytes. Errors are a JSON object on stderr with status 1, the
error envelope of [AGENTS.md](../../AGENTS.md). `scan` is always safe to
run; an agent runs `clean --yes` only once the user has agreed to it.

## When something goes wrong

| Message | What it means and what to do |
|---|---|
| `{"error":"confirmation_required",...}` then `Info: Nothing changed.` | `clean` had no terminal to ask on. Check with `--dry-run`, then pass `--yes`. |
| `Warning: N root-owned files (...) were left: re-run with --sudo to reclaim them.` | Your user cannot change those files. Add `--sudo`. |
| `Error: N files could not be removed.` | A deletion or truncation failed (status 1), for example a `sudo` that was refused. |
| `{"error":"invalid_size",...}` | `--min-size` takes a number with an optional `K`, `M`, `G` or `T` (`512K`, `10M`). |
| `{"error":"invalid_duration",...}` | `--older-than` takes a number with `d`, `h`, `m` or `s` (`7d`, `48h`). |
| `{"error":"unknown_option",...}`, `missing_value`, `unexpected_argument` | A mistyped option; `gash logs clean --help` lists them. |
| `Info: No log files found under: PATH` | Nothing matched: the path holds no log files, or `--min-size` and `--older-than` left them all out. |
| `Info: No reclaimable logs found.` | `clean` found logs but nothing to change: no rotated copies, and live logs only without `--truncate`. |

## See also

- [reference.md](reference.md): `gash logs`, `scan` and `clean` with all their options.
- [Files and archives](../files/README.md): find the largest files and directories of any kind.
- [System](../system/README.md): disk usage per mount.
- [Tools for AI agents](../agents/README.md): `llm_log` reads the end of a log by pattern, time and level.
