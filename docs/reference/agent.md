# agent

Write tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`agent_put`](#agent-put) |  | Upload a file to a registered host: atomic, optional backup and hash check |
| [`agent_replace`](#agent-replace) |  | Replace text in explicit files, all or nothing |

## agent_put

Upload a file to a registered host: atomic, optional backup and hash check

#### Usage

```text
agent_put ALIAS:PATH --from FILE|- [--mode M] [--backup] [--verify] [--force] [--dry-run] [--timeout S]
```

#### Options

```text
--from FILE|-          The content: a local file, or - for stdin (64 MiB max)
--mode M               Octal mode of the written file (default: the old
                       file's, or the host's umask for a new one)
--backup               Copy the old file to PATH.bak-<UTC time> first
--verify               Check the sha256 on the host before the rename
--force                Allow an ASK path (/etc, /var, /usr/local, /root, any
                       .ssh): only after the user agreed
--dry-run              Check the target on the host and report; write nothing
--timeout S            Bound the call (default 300)
```

#### Examples

```bash
# Deploy one file, keep the old one, check what arrived
agent_put web:/var/www/app/config.php --from config.php --backup --verify --force

# Generated content from stdin, into the remote home
printf '%s\n' "$TOKEN_LINE" | agent_put web:app/.version --from -
```

#### Notes

```text
Replaces scp f h:/tmp/x; ssh h "install -m 644 /tmp/x /dest"; sha256sum on
both sides. One ssh round trip: the content goes on stdin to a temporary
file next to PATH (the old file's mode kept), its size is checked (and its
sha256 with --verify), then it is renamed over PATH; a failed check
replaces nothing (write_failed, put_verify_failed: STOP). PATH's directory
must exist; a relative PATH is below the remote home. ALIAS is a
registered HOST: alias. The transfer path policy applies before connecting
and again on the host on the resolved path: DENY paths are
write_path_denied (FATAL, even with --force), ASK paths sensitive_path
(STOP) without --force; secret files are never written or uploaded
(secret_file_blocked). Output: {"host","path","bytes","sha256",
"replaced","backup","verified","written":true}; --dry-run gives
"exists" and "dry_run":true,"written":false.
```

## agent_replace

Replace text in explicit files, all or nothing

#### Usage

```text
agent_replace FILE... (--old A --new B)... [--count N | --min N] [--regex] [--dry-run] [--force] [--format F] [--width N] [--host ALIAS]
```

#### Options

```text
--old A --new B        One replacement; repeat the pair for more, applied in
                       order (each to the result of the one before)
--count N              Every pair must match exactly N times in every file
                       (default: exactly 1)
--min N                Every pair must match at least N times (replaces all)
--regex                --old is a Go (RE2) regular expression; $1 in --new is
                       a group
--dry-run              Report the counts and the lines each file would gain
                       and lose; write nothing
--force                Allow a sensitive path (/etc, /var, /usr/local, /root,
                       .ssh) outside your HOME and working directory: only
                       after the user agreed
--format F             json (default), jsonl, tsv or table: one row per file
                       and pair
--width N              Cut output lines at N columns
--host ALIAS           Edit the files on a registered HOST: alias (the gash
                       binary runs there; one ssh round trip; AGENTS.md 3.12)
```

#### Examples

```bash
# Rename a class in two files; fails, writing nothing, unless each has it once
agent_replace app/A.php app/B.php --old 'OldName' --new 'NewName'

# Every occurrence, at least one per file, preview first
agent_replace src/x.js src/y.js --old 'var ' --new 'let ' --min 1 --dry-run

# Two pairs, a regular expression with a group
agent_replace conf.ini --old 'debug=1' --new 'debug=0' --old 'v([0-9]+)' --new 'v$1-lts' --regex
```

#### Notes

```text
Replaces python3 open().read().replace() + write scripts and sed -i, with
the count assertion built in. All or nothing: every pair's count is checked
in every file in memory first; one mismatch is replace_count_mismatch
(RETRY) and no file changes. Writes go to a temporary file renamed over the
target (mode kept); a file that changed meanwhile is file_changed and
nothing is written. Targets are explicit: no glob is expanded, each file is
named once. Secret files, protected paths and system paths are refused
(FATAL); a sensitive path is sensitive_path (STOP) without --force; binary
files and files over 64 MiB are refused. Output:
{"files":N,"replaced":{"FILE":[n1,n2]},"written":true}.
```
