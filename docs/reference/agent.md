# agent

Write tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`agent_replace`](#agent-replace) |  | Replace text in explicit files, all or nothing |

## agent_replace

Replace text in explicit files, all or nothing

#### Usage

```text
agent_replace FILE... (--old A --new B)... [--count N | --min N] [--regex] [--dry-run] [--force] [--format F] [--width N]
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
