# Reclaiming log-file space (`gash logs`)

Log files are the quiet way a disk fills up: a forgotten `debug.log`, a
rotated `syslog.7.gz` pile, a framework that writes megabytes a day and never
cleans up. `gash logs` finds them and reclaims the space, with eye-candy for
a human and `--json` for an agent.

```bash
gash logs scan             # what is eating disk, grouped by category
gash logs clean --dry-run  # what would be freed, nothing changed
gash logs clean --yes      # reclaim it
```

## What counts as a log

`gash logs` recognises log files by name and by the patterns the usual
runtimes write, not only `*.log`:

- plain `*.log` and log-shaped `*.txt`;
- rotated copies (`access.log.1`, `error.log.2`), including compressed ones
  (`.gz`, `.bz2`, `.xz`, `.zst`) and dated rotations (`app-2026-10-24.log`);
- framework and language patterns — Apache/nginx/caddy access and error
  logs, PHP and PHP-FPM, Node (`npm-debug.log`, pm2), Python (pip, Django,
  gunicorn), package managers (apt, dnf, pacman), databases, framework logs
  (Laravel `storage/logs`, Rails), and systemd-journal fragments.

Each file is sorted into a category so the report shows where the space went,
not just a flat list.

## `scan`

```bash
gash logs scan [PATH...] [--min-size SIZE] [--older-than DUR] [--depth N] [--json]
```

`scan` walks each PATH (the current directory, or common log roots when none
is given) and reports the log files it finds, grouped by category, with the
space each group holds. It changes nothing.

- `--min-size 1M` ignores files below a size (accepts `K`, `M`, `G`).
- `--older-than 7d` ignores files touched more recently than that (`d`, `h`).
- `--depth N` limits how deep each root is walked.
- `--json` emits one object: the groups, the files, and the total reclaimable.

## `clean`

```bash
gash logs clean [PATH...] [--truncate] [--older-than DUR] [--min-size SIZE] \
               [--sudo] [--dry-run] [--yes] [--json]
```

`clean` reclaims space in the way that is safe for a running system:

- **rotated and compressed logs are deleted** — they are already closed, and
  nothing is writing to them;
- **live logs are left alone** unless you pass `--truncate`, which empties
  them in place (keeping the inode, so a daemon holding the file open keeps
  writing to the same handle — no restart, no `logrotate` surprise).

It asks before it acts. `--dry-run` shows what would be freed and changes
nothing; otherwise `clean` needs `--yes`. In headless mode a `clean` without
`--yes` returns `confirmation_required` rather than prompting.

`--sudo` extends the reclaim to root-owned logs (`/var/log/...`): the delete
and truncate then run through an explicit `sudo rm` / `sudo truncate` on the
exact files, never a shell string. Without `--sudo`, `gash logs` only touches
files you own.

`--older-than` and `--min-size` narrow what `clean` touches, exactly as for
`scan`.

### For agents

Every `gash logs` command takes `--json`. `scan --json` is always safe to
run. To reclaim space without a human, pass `--yes` (once the user has agreed
to it); the JSON then reports what was freed. Errors are the envelope of
[AGENTS.md section 5](../AGENTS.md#5-error-contract) on stderr with exit
status 1 — `invalid_duration` for a bad `--older-than`, `confirmation_required`
for a `clean` that needs `--yes`.
