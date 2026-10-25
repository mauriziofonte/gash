# llm

Read-only JSON tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`llm_config`](#llm-config) |  | Read a file that is not secret (YAML as JSON) |
| [`llm_csv`](#llm-csv) |  | Header, sample and row count of a CSV file (replaces python3 csv.reader, wc -l) |
| [`llm_db_databases`](#llm-db-databases) |  | List the databases of a connection (replaces SHOW DATABASES, psql -l) |
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
| [`llm_json`](#llm-json) |  | One value, the keys or the shape of a JSON file (replaces python3 json.load, jq) |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_tree`](#llm-tree) |  | Compact directory tree (JSON or text output) |

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

## llm_csv

Header, sample and row count of a CSV file (replaces python3 csv.reader, wc -l)

#### Usage

```text
llm_csv FILE [--head N] [--cols] [--count] [--delim C] [--where COL=V]
        [--format json|jsonl|tsv|table]
```

#### Options

```text
--head N               Sample rows (default 5; 0 for none)
--cols                 The header only, no sample
--count                Count every row (reads the whole file)
--delim C              The delimiter (default: sniffed among , ; tab |);
                       tab or \t for a tab
--where COL=V          Only rows whose column COL (a header name or a
                       1-based number) is exactly V
--format F             json (default), or the sample as jsonl, tsv or table
```

#### Examples

```bash
llm_csv export.csv
llm_csv export.csv --head 2 --count
llm_csv data.csv --where status=failed --count --head 10
llm_csv report.txt --delim ';' --cols
```

#### Notes

```text
Replaces python3 -c "import csv; r=list(csv.reader(open(f))); ..." and
wc -l f; head -3 f (which miscounts quoted newlines). Output:
{"rows":N,"matches":M,"cols":K,"header":[...],"sample":[[...]],
"delimiter":",","errors":[{"line":L,"error":"..."}],"error_count":E}
(rows, matches and error_count with --count; matches with --where).
A streaming RFC 4180 parser: quoted fields hold delimiters, "" and
newlines; a record is not a line; blank lines are skipped. errors lists
the first 10 malformed records (field count differing from the header,
a stray quote, an unterminated quote) by their first line. Without
--count the read stops once the header and the sample are complete.
With --format jsonl, tsv or table the sample rows print under the
header and the rest is one JSON line on stderr.
```

See also: `llm_json`, `llm_config`

## llm_db_databases

List the databases of a connection (replaces SHOW DATABASES, psql -l)

#### Usage

```text
llm_db_databases [-c CONNECTION | -f SQLITE_FILE] [-d DB] [--format json|jsonl|tsv|table] [--scalar]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database to connect to (any one works)
-f, --file PATH        SQLite database file
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
```

#### Examples

```bash
llm_db_databases -c default
llm_db_databases -c postgres --format tsv
```

#### Notes

```text
Replaces mysql -e 'SHOW DATABASES' and psql -l. Output:
{data: [names], rows, query_time}, sorted: every schema MySQL/MariaDB
shows (system ones included), every PostgreSQL database that is not a
template, "main" for an SQLite file. The connection needs no
database here (PostgreSQL then connects to "postgres"). With
--format jsonl, tsv or table the names are rows of a "name" column
and the metadata is one JSON line on stderr.
```

See also: `llm_db_tables`, `llm_db_query`

## llm_db_explain

Show query execution plan (EXPLAIN)

#### Usage

```text
llm_db_explain "QUERY" [-c CONNECTION | -f SQLITE_FILE] [-d DB] [--analyze]
               [--format json|jsonl|tsv|table] [--scalar]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
--analyze              Run the query to measure it (still read-only)
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
```

#### Examples

```bash
llm_db_explain "SELECT * FROM users WHERE email LIKE '%@example.com'" -c default
llm_db_explain "SELECT o.* FROM orders o JOIN users u ON o.user_id=u.id" -c legacy --analyze
```

#### Notes

```text
--format jsonl, tsv or table print the plan rows (a JSON plan as
compact JSON in its cell) and the metadata as one JSON line on stderr.
```

See also: `llm_db_query`

## llm_db_query

Read-only SQL query (JSON output)

#### Usage

```text
llm_db_query "SQL" [-c CONNECTION | -f SQLITE_FILE] [-d DB] [-n ROWS]
             [--format json|jsonl|tsv|table] [--scalar]
```

#### Examples

```bash
# Query using the default connection
llm_db_query "SELECT id, name FROM users LIMIT 5"

# Query a named connection
llm_db_query "SELECT * FROM orders WHERE status='pending'" -c legacy

# Query a SQLite database (no connection config needed)
llm_db_query "SELECT * FROM users" -f ./data/app.db -n 20

# Rows as TSV, one value alone
llm_db_query "SELECT id, email FROM users" -c default --format tsv
llm_db_query "SELECT count(*) FROM orders" -c default --scalar
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's;
                       letters, digits, _ $ . - only)
-f, --file PATH        SQLite database file
-n, --limit N          Row cap (default 100; also -r, --rows)
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
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
--format jsonl, tsv or table print the rows themselves (replaces
llm_db_query ... | jq -c '.data[]' and python3 reshaping): SQL NULL is
null, \N in TSV and an empty table cell on every engine; the metadata
{rows, query_time, truncated} is one JSON line on stderr, with
"truncated": true when -n cut rows. --scalar prints the value of a
1x1 result as a TSV cell (not_a_scalar, RETRY, with the actual shape
otherwise). --format json is the envelope, unchanged.
```

See also: `llm_db_tables`, `llm_db_schema`, `llm_db_sample`, `llm_db_explain`

## llm_db_sample

Sample rows from a table (default 5 rows)

#### Usage

```text
llm_db_sample TABLE [-c CONNECTION | -f SQLITE_FILE] [-d DB] [-n N]
              [--format json|jsonl|tsv|table] [--scalar]
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
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
```

See also: `llm_db_query`, `llm_db_schema`

## llm_db_schema

Show table schema (columns, types, keys)

#### Usage

```text
llm_db_schema TABLE [-c CONNECTION | -f SQLITE_FILE] [-d DB]
llm_db_schema --all [-c CONNECTION | -f SQLITE_FILE] [-d DB]
              [--format json|jsonl|tsv|table] [--scalar]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
--all                  Every table in one call, each row led by "table"
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
```

#### Examples

```bash
llm_db_schema users -c default
llm_db_schema orders -c legacy
llm_db_schema sessions -f ./app.db
llm_db_schema --all -c default --format tsv
```

#### Notes

```text
--all replaces one llm_db_schema call per table: the rows of every
table llm_db_tables lists, sorted by table and column position.
```

See also: `llm_db_tables`, `llm_db_query`

## llm_db_tables

List database tables (JSON array)

#### Usage

```text
llm_db_tables [-c CONNECTION | -f SQLITE_FILE] [-d DB] [--counts]
              [--format json|jsonl|tsv|table] [--scalar]
```

#### Options

```text
-c, --connection NAME  Connection from ~/.gash_env (default: default)
-d, --database NAME    Database name (overrides the connection's)
-f, --file PATH        SQLite database file
--counts               [{name, rows}]: the row count of each table
--format F             json (the envelope, default), jsonl, tsv or table
--scalar               Print the one value of a 1x1 result
```

#### Examples

```bash
llm_db_tables -c default
llm_db_tables -c postgres
llm_db_tables -f ./app.db
llm_db_tables -c default --counts --format tsv
```

#### Notes

```text
--counts: rows is exact on SQLite (count(*)), the engine's estimate on
MySQL/MariaDB (information_schema) and PostgreSQL (pg_class), null
where the engine has none (a view, a table not analyzed yet).
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

## llm_json

One value, the keys or the shape of a JSON file (replaces python3 json.load, jq)

#### Usage

```text
llm_json FILE|- [--path P] [--keys | --shape] [--format json|jsonl|tsv|table]
         [--width N] [--head N] [--tail N]
```

#### Options

```text
--path P               The value at P: a.b[0].c, paths["/api/x"].post, [-1]
--keys                 The keys of an object (or of the objects of an array)
--shape                Types and lengths, two levels deep
--format F             json (default), jsonl (one element per line), tsv or
                       table (an array of objects as rows)
--width N              Cut each string value at N columns (ends with …)
--head N, --tail N     Keep the first / last N elements of an array
```

#### Examples

```bash
llm_json package.json --path scripts
llm_json openapi.json --path 'paths["/users"].get.responses' --keys
llm_json data.json --shape
llm_json users.json --format tsv --head 20
curl -s https://api.example/x | llm_json - --path items[0]
```

#### Notes

```text
Replaces python3 -c "import json; print(json.load(open(f))[...])" and
jq '.a', jq 'length, (.[0]|keys)'. The whole text is validated first
(invalid_json, RETRY, with line and column); the value prints as compact
JSON with its numbers and escapes as written; a repeated key: the last
one wins. A path that does not exist is json_path_not_found (RETRY) with
the nearest existing prefix in the hint; --keys on a value without
objects is not_an_object. --shape: {"type":"array[100]","items":
{"type":"object{3}","keys":{"id":"number",...},"optional":[...]}};
lengths are N or MIN..MAX, mixed types are joined with |. A cut array
adds {"truncated":K} on stderr. Secret files (.env, keys, ...) are
refused; - reads the standard input.
```

See also: `llm_config`, `llm_csv`

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
