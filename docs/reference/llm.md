# llm

Read-only JSON tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`llm_config`](#llm-config) |  | Read a file that is not secret (YAML as JSON) |
| [`llm_db_explain`](#llm-db-explain) |  | Show query execution plan (EXPLAIN) |
| [`llm_db_query`](#llm-db-query) |  | Read-only SQL query (JSON output) |
| [`llm_db_sample`](#llm-db-sample) |  | Sample rows from a table (default 5 rows) |
| [`llm_db_schema`](#llm-db-schema) |  | Show table schema (columns, types, keys) |
| [`llm_db_tables`](#llm-db-tables) |  | List database tables (JSON array) |
| [`llm_deps`](#llm-deps) |  | List project dependencies (JSON) |
| [`llm_docker_exec`](#llm-docker-exec) |  | Run a read-only command in a container, no shell quoting (JSON) |
| [`llm_docker_logs`](#llm-docker-logs) |  | Last log lines of a container, optionally filtered (JSON) |
| [`llm_docker_ps`](#llm-docker-ps) |  | Docker containers with state, health and ports (JSON) |
| [`llm_env`](#llm-env) |  | Filtered environment variables (secrets excluded) |
| [`llm_exec`](#llm-exec) |  | Safe command execution wrapper for AI agents |
| [`llm_find`](#llm-find) |  | Find files by pattern (ignores noise directories) |
| [`llm_git_diff`](#llm-git-diff) |  | Git diff with statistics |
| [`llm_git_log`](#llm-git-log) |  | Recent commits (JSON) |
| [`llm_git_status`](#llm-git-status) |  | Compact git status (JSON) |
| [`llm_grep`](#llm-grep) |  | Search code with structured file:line:content output |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name, port or PID, alive or not, as a tree (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_service`](#llm-service) |  | State and last journal lines of systemd units (JSON) |
| [`llm_sysinfo`](#llm-sysinfo) |  | Load, uptime, CPUs, memory, disks, GPUs and OS in one line (JSON) |
| [`llm_tree`](#llm-tree) |  | Compact directory tree (JSON or text output) |
| [`llm_wait`](#llm-wait) |  | Wait for a log line, a path, a process exit, a URL, a port or a command (JSON) |

## llm_config

Read a file that is not secret (YAML as JSON)

#### Usage

```text
llm_config FILE [--max-bytes N]
```

#### Options

```text
--max-bytes N          Largest file read, in bytes (default 1048576)
```

#### Examples

```bash
llm_config package.json
llm_config docker-compose.yml
llm_config big.json --max-bytes 5000000
```

#### Notes

```text
Reads any file that is not secret: .env files, keys and the other
secret files are refused.
YAML comes out as JSON, converted by Gash itself (no yq needed), or
as the raw text when it does not parse. Files over --max-bytes
(default 1 MiB) fail with file_too_large.
```

## llm_db_explain

Show query execution plan (EXPLAIN)

#### Usage

```text
llm_db_explain "QUERY" [-c CONNECTION | -f SQLITE_FILE] [-d DB] [--analyze]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
--analyze              Run the query to measure it (still read-only)
```

#### Examples

```bash
llm_db_explain "SELECT * FROM users WHERE email LIKE '%@example.com'" -c default
llm_db_explain "SELECT o.* FROM orders o JOIN users u ON o.user_id=u.id" -c legacy --analyze
```

See also: `llm_db_query`

## llm_db_query

Read-only SQL query (JSON output)

#### Usage

```text
llm_db_query "SQL" [-c CONNECTION] [-f SQLITE_FILE] [-n ROWS]
```

#### Examples

```bash
# Query using the default connection
llm_db_query "SELECT id, name FROM users LIMIT 5"

# Query a named connection
llm_db_query "SELECT * FROM orders WHERE status='pending'" -c legacy

# Query a SQLite database (no connection config needed)
llm_db_query "SELECT * FROM users" -f ./data/app.db -n 20
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's;
                       letters, digits, _ $ . - only)
-f, --file PATH        SQLite database file
-n, --limit N          Row cap (default 100; also -r, --rows)
```

#### Notes

```text
Read-only, enforced twice: one statement starting with SELECT, WITH,
VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN or PRAGMA, with no write
keywords or file/program functions; then a read-only client session
(SQLite read-only in defensive mode, MySQL READ ONLY transactions,
PostgreSQL default_transaction_read_only with a GASH_DB_TIMEOUT
statement timeout). Output: {data, rows, query_time}. -h is help: the
host comes from the connection URL. Error details are the database
driver's message.
```

See also: `llm_db_tables`, `llm_db_schema`, `llm_db_sample`, `llm_db_explain`

## llm_db_sample

Sample rows from a table (default 5 rows)

#### Usage

```text
llm_db_sample TABLE [-c CONNECTION] [-f SQLITE_FILE] [-n N]
```

#### Examples

```bash
llm_db_sample users -c default
llm_db_sample orders -c legacy --limit 10
llm_db_sample logs -f ./app.db -n 3
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
-n, --limit N          Rows to return (default 5)
```

See also: `llm_db_query`, `llm_db_schema`

## llm_db_schema

Show table schema (columns, types, keys)

#### Usage

```text
llm_db_schema TABLE [-c CONNECTION | -f SQLITE_FILE] [-d DB]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
```

#### Examples

```bash
llm_db_schema users -c default
llm_db_schema orders -c legacy
llm_db_schema sessions -f ./app.db
```

See also: `llm_db_tables`, `llm_db_query`

## llm_db_tables

List database tables (JSON array)

#### Usage

```text
llm_db_tables [-c CONNECTION | -f SQLITE_FILE] [-d DB]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
```

#### Examples

```bash
llm_db_tables -c default
llm_db_tables -c postgres
llm_db_tables -f ./app.db
```

See also: `llm_db_query`, `llm_db_schema`

## llm_deps

List project dependencies (JSON)

#### Usage

```text
llm_deps [PATH] [--dev]
```

#### Options

```text
--dev                  Split the result into prod and dev
```

#### Examples

```bash
llm_deps
llm_deps /var/www/myapp --dev
```

See also: `llm_project`

## llm_docker_exec

Run a read-only command in a container, no shell quoting (JSON)

#### Usage

```text
llm_docker_exec CTR [--user U] [--width N] [--max-lines N] [--head N] [--tail N] -- CMD...
```

#### Options

```text
--user U, -u U         Run as user U (name or uid[:gid])
--width N              Cut each output line at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N lines of each stream
-- CMD...              The command, one word per argument
```

#### Examples

```bash
# Instead of: docker exec -u node ctr sh -c 'grep -rl "x" /app | head -3'
llm_docker_exec ctr --user node --head 3 -- grep -rl x /app
llm_docker_exec db -- cat /etc/mysql/my.cnf
```

#### Notes

```text
Replaces docker exec CTR sh -c '...' with its quoting levels: CMD's words
reach docker exec as an array, no shell runs. Read-only: CMD passes the
llm_exec rules first (allowlist and option rules, secret files,
forbidden paths, blocklist; GASH_LLM_EXEC_ALLOW extends them); sed and
awk run as sed --sandbox and gawk --sandbox, grep and rg skip secret
files, git is refused. One command (no pipes): shape the output with
--head/--tail. {"rc","out","err"} and "truncated" when lines were
cut; the exit status is 0 when the command ran (read rc). Arbitrary
commands: agent_docker_exec.
```

See also: `llm_exec`

## llm_docker_logs

Last log lines of a container, optionally filtered (JSON)

#### Usage

```text
llm_docker_logs CTR [--tail N] [--since T] [--grep RE] [--width N]
```

#### Options

```text
--tail N               Read the last N lines (default 100)
--since T              Only lines since T (10m, 2h, 2026-10-25T10:00:00)
--grep RE              Keep the lines RE matches (RE2; (?i) for any case)
--width N              Cut each line at N columns (default 300)
```

#### Examples

```bash
llm_docker_logs web --tail 50 --grep '(?i)error'
llm_docker_logs db --since 10m
```

#### Notes

```text
Replaces docker logs --tail 50 ctr 2>&1 | grep -i error | cut -c1-300.
{"ctr","read":N,"log":[...]}: stdout and stderr in docker's order,
read is how many lines docker returned before --grep. Unknown
container: container_not_found (RETRY); docker absent or down:
docker_not_found, docker_unavailable (CONTINUE).
```

See also: `llm_docker_ps`

## llm_docker_ps

Docker containers with state, health and ports (JSON)

#### Usage

```text
llm_docker_ps [--running] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--running              Running containers only (default: all)
--format F             json (default), jsonl, tsv or table
--width N              Cut each value at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N containers
```

#### Examples

```bash
llm_docker_ps
llm_docker_ps --running --format tsv
```

#### Notes

```text
Replaces docker ps --format '{{.Names}} {{.Status}}' and docker ps -a.
[{"name","image","state","health","ports","up"}] by name: state
is docker's (running, exited, ...), health healthy, unhealthy, starting
or "", ports "8080->80/tcp" (the all-address bindings shortened), up
docker's status without the health ("Up 2 hours", "Exited (1) 3
minutes ago"). Without docker: docker_not_found; daemon down:
docker_unavailable (both CONTINUE).
```

See also: `llm_docker_logs`

## llm_env

Filtered environment variables (secrets excluded)

#### Usage

```text
llm_env [--filter REGEX | REGEX]
```

#### Options

```text
-f, --filter REGEX     Only names that match REGEX (Go RE2 syntax, e.g.
                       '^PATH$' or 'NODE|NPM'); a bare REGEX works too
```

#### Examples

```bash
llm_env
llm_env --filter PATH
```

#### Notes

```text
Excludes variables matching secret patterns: PASSWORD, PASSPHRASE,
TOKEN, SECRET, KEY, CREDENTIAL, AUTH, SESSION, JWT, WEBHOOK, *_PWD,
*_PAT, etc. In the others, URL passwords and known token formats
(AKIA..., ghp_..., sk-..., xox?-..., AIza...) are shown as ***.
```

## llm_exec

Safe command execution wrapper for AI agents

#### Usage

```text
llm_exec "command"
```

#### Examples

```bash
llm_exec "ls -la /var/log"
llm_exec "git log --oneline -5 && git status --short"
llm_exec "grep -rn TODO src | head -20"
llm_exec "systemctl status nginx"
```

#### Notes

```text
Runs in a child shell (never the caller's), excluded from history.
Read-only allowlist: ls, cat, head, tail, grep, rg, find, tree, wc,
sort, jq, diff, du, df, ps, ss, git (status, log, diff, show, ...),
systemctl/journalctl/docker (read subcommands) and similar.
Commands joined by |, &&, || and ; only: no $ or backtick expansion,
no subshells, no redirections except >/dev/null, 2>&1 and >&2.
Writing options are refused (find -delete, sort -o, sed -i,
git branch -D, ...); sed and awk run sandboxed (GNU sed, gawk).
Secret files (.env, keys, credentials) are refused, also via globs;
grep -r, rg, diff -r and git grep/log -p/show/diff leave them out.
Recursive commands cannot follow symlinks (grep -R, find -L, ...)
or start at / or above /proc, /sys, /dev, /boot, /root (content
searches: also /etc). The child gets a short environment (PATH,
HOME, USER, locale, TERM, TZ, TMPDIR, ...), never the caller's tokens.

SETTINGS (~/.gash_env only)
GASH_LLM_EXEC_ALLOW="make npm"   extra commands, any options (walk rules kept)
GASH_LLM_EXEC=unrestricted       skip the allowlist
GASH_LLM_SANDBOX=1               run inside bwrap (read-only, no network)
```

## llm_find

Find files by pattern (ignores noise directories)

#### Usage

```text
llm_find PATTERN [PATH] [--type f|d] [--contains REGEX] [-n N]
```

#### Options

```text
--type f|d             Files (default) or directories
--contains REGEX       Keep the files whose content matches REGEX
                       (ripgrep's regex syntax); files only
-n, --limit N          Paths to print (default 100)
```

#### Examples

```bash
# Find all PHP files
llm_find "*.php"

# Find config files containing "database"
llm_find "*.conf" /etc --contains "database"

# Find directories only
llm_find "src*" . --type d
```

#### Notes

```text
PATTERN is a file name glob (find -name). With --contains only the
files it matches are searched, hidden and gitignored ones included,
with or without ripgrep. Skips node_modules, vendor, .git and
__pycache__; --contains also skips secret files. Default limit 100.
A missing PATH is path_not_found, an empty PATTERN missing_pattern.
```

## llm_git_diff

Git diff with statistics

#### Usage

```text
llm_git_diff [--staged] [PATH]
```

#### Options

```text
--staged               Staged changes (also --cached)
```

#### Examples

```bash
llm_git_diff
llm_git_diff --staged
```

See also: `llm_git_status`, `llm_git_log`

## llm_git_log

Recent commits (JSON)

#### Usage

```text
llm_git_log [-n N] [PATH]
```

#### Options

```text
-n, --limit N          Commits to list (default 10)
```

#### Examples

```bash
llm_git_log
llm_git_log --limit 20
```

See also: `llm_git_status`, `llm_git_diff`

## llm_git_status

Compact git status (JSON)

#### Usage

```text
llm_git_status [PATH]
```

#### Examples

```bash
llm_git_status
# {
#   "branch": "main",
#   "ahead": 0,
#   "behind": 0,
#   "staged": [],
#   "modified": ["README.md"],
#   "untracked": [],
#   "conflicted": []
# }
```

See also: `llm_git_diff`, `llm_git_log`

## llm_grep

Search code with structured file:line:content output

#### Usage

```text
llm_grep PATTERN [PATH] [--ext EXT1,EXT2] [--context N] [-n N]
```

#### Options

```text
--ext EXT1,EXT2        Search only files with these extensions
-C, --context N        Lines of context around each match (default 0)
-n, --limit N          Lines to print (default 100)
```

#### Examples

```bash
# Find TODO comments in PHP files
llm_grep "TODO|FIXME" src/ --ext php,js

# Search for a function definition
llm_grep "function processOrder" .

# With context lines
llm_grep "class.*Controller" . --ext php --context 3
```

#### Notes

```text
One built-in engine, the same with or without ripgrep installed:
ripgrep's regex syntax; hidden files, .gitignore/.ignore/.rgignore
matches, binary files, node_modules, vendor and .git are skipped; files
come in sorted path order. Secret files are skipped, templates such as
.env.example searched. Default limit 100.
Output: file:line:content with absolute paths (a FILE PATH gives
line:content); context lines use -, groups are separated by --.
A missing PATH is path_not_found, an empty PATTERN missing_pattern.
```

## llm_ports

List ports in use (JSON)

#### Usage

```text
llm_ports [--listen]
```

#### Options

```text
-l, --listen           Listening sockets only
```

#### Examples

```bash
llm_ports
llm_ports --listen
```

See also: `llm_procs`

## llm_procs

List processes by name, port or PID, alive or not, as a tree (JSON)

#### Usage

```text
llm_procs [--name NAME | --port PORT | --pid PID...] [--alive] [--tree]
          [--format F] [--width N] [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--name NAME            Processes whose command line contains NAME
--port PORT            Processes with a socket on PORT
--pid PID...           These processes, alive or not (one or more PIDs)
--alive                Live processes only (a zombie is not alive)
--tree                 Each process and everything below it, depth first,
                       with ppid and depth
--format F             json (default), jsonl, tsv or table
--width N              Cut each value at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N rows
```

#### Examples

```bash
llm_procs
llm_procs --name nginx
llm_procs --port 3306

# Instead of: kill -0 $PID && echo alive
llm_procs --pid "$pid"

# Instead of: ps -ef | grep vite | grep -v grep
llm_procs --name vite --alive --tree
```

#### Notes

```text
--port takes 0 to 65535 (invalid_port above) and wins over --name,
which is then ignored; --pid wins over both. With --pid, --alive or
--tree the rows are [{"pid","name","cmd","alive"}] (--tree adds
"ppid" and "depth"): --pid rows come in the order given, a gone PID
as alive:false; other rows by PID, never the caller's own shell and its
parents; NAME is then a regular expression (RE2); cmd is cut at 200
columns unless --width is given; without a filter the first 100 rows.
Read-only: llm_procs signals nothing (agent_kill does).
```

See also: `llm_ports`

## llm_project

Detect project type and framework (JSON)

#### Usage

```text
llm_project [PATH]
```

#### Examples

```bash
llm_project
llm_project /var/www/myapp
```

See also: `llm_deps`

## llm_service

State and last journal lines of systemd units (JSON)

#### Usage

```text
llm_service UNIT... [--log N] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--log N                Last N journal lines of each unit (default 10; 0: none)
--format F             json (default), jsonl, tsv or table
--width N              Cut each journal line at N columns (default 300)
--max-lines N, --head N, --tail N
                       Keep the first/last N units
```

#### Examples

```bash
llm_service nginx php8.4-fpm
llm_service mariadb --log 30
```

#### Notes

```text
Replaces systemctl is-active x; journalctl -u x -n 20 --no-pager | cut
-c1-200, for several units in one call. One row per unit, in the order
given: [{"unit","load","active","sub","since","pid","restarts",
"log":[...]}]; an unknown unit is load "not-found". Journal lines are
short-iso without the host name; the journal may need the user in the
systemd-journal group (no lines otherwise). Read-only (systemctl show,
journalctl). Without systemd: systemd_unavailable (CONTINUE).
```

See also: `llm_procs`

## llm_sysinfo

Load, uptime, CPUs, memory, disks, GPUs and OS in one line (JSON)

#### Usage

```text
llm_sysinfo [--mount PATH]...
```

#### Options

```text
--mount PATH           Report the filesystem of PATH (repeatable) instead
                       of / and the other local disks
```

#### Examples

```bash
llm_sysinfo
llm_sysinfo --mount /var/lib/mysql
```

#### Notes

```text
Replaces uptime; free -h; df -h /; nproc; uname -a; nvidia-smi
--query-gpu=... in about 200 bytes: {"host","os","kernel","uptime_h",
"load":[1,5,15],"cpus","cpu","mem_mb":{"total","avail"},
"disks":[{"mount","size_gb","used_pct"}],"gpu":[{"name","mem_mb":
[used,total],"util_pct"}]}; gpu only when nvidia-smi is installed. The
disks are / and the writable local disks (one mount per device; not
/boot, /snap, network or pseudo filesystems). Linux reads /proc and
statfs, no program runs; macOS asks sysctl.
```

See also: `sysinfo`

## llm_tree

Compact directory tree (JSON or text output)

#### Usage

```text
llm_tree [--text] [--depth N] [--stats] [PATH]
```

#### Options

```text
--text                 Indented text list, 200 lines max (default depth 3)
--depth N              Levels below PATH, 1 or more (default 1; also
                       --max-depth)
--stats                Add sizes and child counts to the JSON tree
```

#### Examples

```bash
# JSON output (default)
llm_tree

# Text-indented output
llm_tree --text

# Limited depth
llm_tree --depth 2 /etc/nginx
```

#### Notes

```text
Ignores noise directories below PATH: node_modules, vendor, .git,
__pycache__, .cache. Entries whose name holds a tab or
a newline are left out. --text lists symlinks as name -> target, in
byte order whatever the locale.
```

## llm_wait

Wait for a log line, a path, a process exit, a URL, a port or a command (JSON)

#### Usage

```text
llm_wait --timeout S [--interval S] CONDITION [--tail N] [--width N]
CONDITION is one of:
  --file-match FILE RE   a line of FILE matches RE
  --exists PATH          PATH exists
  --gone PID|NAME        the process ended / no command line matches NAME
  --http URL [--code N] [--json-path P --equals V]
  --port HOST:PORT       a TCP connection opens
  --cmd-ok CMD           CMD (llm_exec rules) exits 0
```

#### Options

```text
--timeout S            Give up after S (seconds, or 500ms, 2m; at most 1h);
                       required
--interval S           Poll interval (default 0.2s for files and paths and a
                       PID, 0.5s for a NAME, a URL and a port, 1s for a command)
--file-match FILE RE   Scan FILE once, then only what is appended (inotify on
                       Linux); a missing FILE is waited for. RE is RE2,
                       matched against each line
--exists PATH          Met when PATH exists
--gone PID|NAME        A PID: met when it ends (a zombie counts as ended). A
                       NAME: a regular expression matched against command
                       lines, as llm_procs --name; the caller is left out
--http URL             GET until the status is 2xx
--code N               ... until it is N (100-599) instead
--json-path P          ... and the JSON body holds --equals V at path P
--equals V             (a.b[0].c; a string compares as is, a number in its
                       shortest form, true, false, null)
--port HOST:PORT       Met when a TCP connection to HOST:PORT opens
--cmd-ok CMD           Re-run the command line CMD (checked once under the
                       llm_exec rules) until it exits 0
--tail N               Add the last N lines of FILE, or of CMD's last output
--width N              Cut the match and tail lines at N columns (default 300)
```

#### Examples

```bash
# Instead of: while ! grep -q DONE out.log; do sleep 10; done
llm_wait --timeout 600 --file-match out.log '^DONE' --tail 5

# A server comes up
llm_wait --timeout 60 --http http://127.0.0.1:8080/health --json-path status --equals ok
llm_wait --timeout 30 --port 127.0.0.1:5432

# A background job ends
llm_wait --timeout 300 --gone "$pid"
```

#### Notes

```text
Replaces sleep-and-check loops (while ! grep -q X f; do sleep N; done,
sleep N; tail f, until curl ...; do sleep N; done) with one call that
returns as soon as the condition holds. Met: {"met":true,"after_s":2.01,
"match":"<line>","tail":[...]} and exit 0 (--http adds "code", --cmd-ok
"rc" and "runs"). Timeout: {"met":false,"after_s":S} and exit 1, with
no error envelope (--http, --port and --cmd-ok add what the last look saw:
"last", "code", "rc"). One condition per call. FILE follows the path
rules and secret files are refused. Reads only: nothing is signaled,
written or started but CMD.
```

See also: `llm_procs`
