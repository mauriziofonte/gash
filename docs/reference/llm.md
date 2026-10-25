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
| [`llm_env`](#llm-env) |  | Filtered environment variables (secrets excluded) |
| [`llm_exec`](#llm-exec) |  | Safe command execution wrapper for AI agents |
| [`llm_find`](#llm-find) |  | Find files by pattern (ignores noise directories) |
| [`llm_git_diff`](#llm-git-diff) |  | Git diff with statistics |
| [`llm_git_log`](#llm-git-log) |  | Recent commits (JSON) |
| [`llm_git_status`](#llm-git-status) |  | Compact git status (JSON) |
| [`llm_grep`](#llm-grep) |  | Search code with structured file:line:content output |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_remote_run`](#llm-remote-run) |  | Run a read-only command on registered hosts, in parallel or in steps |
| [`llm_same`](#llm-same) |  | Is the deployed copy the local one? Hash both sides, list the differences |
| [`llm_tree`](#llm-tree) |  | Compact directory tree (JSON or text output) |

## llm_config

Read a file that is not secret (YAML as JSON)

#### Usage

```text
llm_config FILE [--max-bytes N] [--host ALIAS]
```

#### Options

```text
--max-bytes N          Largest file read, in bytes (default 1048576)
--host ALIAS           Run on a registered HOST: alias over ssh, one round
                       trip (repeatable; see AGENTS.md 3.12)
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
         [--host ALIAS]
```

#### Options

```text
--type f|d             Files (default) or directories
--contains REGEX       Keep the files whose content matches REGEX
                       (ripgrep's regex syntax); files only
-n, --limit N          Paths to print (default 100)
--host ALIAS           Run on a registered HOST: alias over ssh, one round
                       trip (repeatable; see AGENTS.md 3.12)
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
         [--host ALIAS]
```

#### Options

```text
--ext EXT1,EXT2        Search only files with these extensions
-C, --context N        Lines of context around each match (default 0)
-n, --limit N          Lines to print (default 100)
--host ALIAS           Run on a registered HOST: alias over ssh, one round
                       trip (repeatable; see AGENTS.md 3.12)
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

List processes by name or port (JSON)

#### Usage

```text
llm_procs [--name NAME | --port PORT]
```

#### Options

```text
--name NAME            Processes whose command line contains NAME
--port PORT            Processes with a socket on PORT
```

#### Examples

```bash
llm_procs
llm_procs --name nginx
llm_procs --port 3306
```

#### Notes

```text
--port takes 0 to 65535 (invalid_port above) and wins over --name,
which is then ignored.
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

## llm_remote_run

Run a read-only command on registered hosts, in parallel or in steps

#### Usage

```text
llm_remote_run HOST... [-j N] [--timeout S] [--format F] -- CMD
llm_remote_run HOST --step CMD... [--stop-on-error] [--timeout S] [--format F]
(budget: [--width N] [--max-lines N] [--head N] [--tail N])
```

#### Options

```text
-- CMD                 The command line, run on every HOST in parallel: one
                       word is the whole line (quote pipes), several words
                       are one command
--step CMD             One step of a session on one HOST; repeat it: the
                       steps run in order in ONE ssh round trip
--stop-on-error        End the session at the first step that fails
-j, --jobs N           Hosts at a time (default 8)
--timeout S            Per host (with --step: the session; default 120)
--format F             json (default), jsonl, tsv or table
--width N              Cut output lines at N columns
--max-lines N          Keep the first N lines of each out/err (--head N)
--head N, --tail N     Keep the first / last N lines of each out/err
                       (default: 100 and 100, an omitted marker between)
```

#### Examples

```bash
# The same check on three hosts at once
llm_remote_run web1 web2 db -- 'df -h / | tail -n 1'

# Three steps, one round trip
llm_remote_run web --step 'systemctl is-active nginx' --step 'tail -n 20 /var/log/nginx/error.log' --step 'uptime'
```

#### Notes

```text
Replaces for h in ...; do ssh $h CMD; done and ssh h a; ssh h b; ssh h c.
HOST is a registered HOST: alias of ~/.gash_env (unknown_host, STOP,
otherwise). CMD passes the llm_exec rules here before anything connects
(read-only commands, secret files and protected paths refused); a glob or
~ is refused (remote_glob_unsupported): it would expand on this machine.
Each command runs in bash on the host, stdin closed, from the remote home.
Steps do not share a shell (no cd or variables between steps).
Output: [{"host","rc","ms","out","err","error"}] in HOST order, error
null or timeout, host_unreachable, remote_protocol_error; with --step
[{"step","rc","ms","out"}] (out holds stdout and stderr; ms null when
the host's date has no %N), and {"stopped":true,"skipped":K} or
{"timed_out":true} on stderr. A failing command is data (rc), not an
error of the call.
```

## llm_same

Is the deployed copy the local one? Hash both sides, list the differences

#### Usage

```text
llm_same LOCAL ALIAS:REMOTE... [--container CTR] [--exclude NAME]... [-j N] [--timeout S]
(budget: [--width N] [--max-lines N] [--head N] [--tail N])
```

#### Options

```text
--container CTR        REMOTE is a path inside container CTR on the host
                       (docker exec)
--exclude NAME         Leave out files and directories with this name glob
                       (repeatable; .git is always left out)
-j, --jobs N           Targets at a time (default 8)
--timeout S            Per target (default 120)
--width N              Cut each p at N columns
--max-lines N          Keep the first N diff entries (--head N; default 100)
--head N, --tail N     Keep the first / last N diff entries
```

#### Examples

```bash
# One file on two hosts
llm_same config/app.php web1:/srv/app/config/app.php web2:/srv/app/config/app.php

# A whole tree, without node_modules
llm_same dist web:/var/www/app/dist --exclude node_modules

# Inside a container
llm_same src/handler.py web:/app/handler.py --container api
```

#### Notes

```text
Replaces sha256sum f | cut -c1-12; ssh h "docker exec c sha256sum f" and
the eyeball comparison. One ssh round trip per target, however many files.
Output: {"same":false,"files":N,"diff":[{"host","p","local","remote"}]}
with the first 12 hex digits of each sha256, null where a side lacks the
path; p is relative to the directory (LOCAL as given for a file, "." when
a side is missing or is a directory against a file); files counts the
local files. Directories are compared recursively over regular files
(symlinks not followed); names holding a newline or a backslash are left
out. Secret files are not compared (secrets_skipped: K); a secret LOCAL
file is secret_file_blocked. ALIAS is a registered HOST: alias; REMOTE
follows the remote path rules (AGENTS.md 3.12). The host needs
sha256sum, shasum or openssl (hash_tool_not_found).
```

## llm_tree

Compact directory tree (JSON or text output)

#### Usage

```text
llm_tree [--text] [--depth N] [--stats] [PATH] [--host ALIAS]
```

#### Options

```text
--text                 Indented text list, 200 lines max (default depth 3)
--depth N              Levels below PATH, 1 or more (default 1; also
                       --max-depth)
--stats                Add sizes and child counts to the JSON tree
--host ALIAS           Run on a registered HOST: alias over ssh, one round
                       trip (repeatable; see AGENTS.md 3.12)
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
