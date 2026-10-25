# llm

Read-only JSON tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`llm_assert`](#llm-assert) |  | Check that files hold, lack or count patterns; exit 0 only if all pass |
| [`llm_config`](#llm-config) |  | Read a file that is not secret (YAML as JSON) |
| [`llm_count`](#llm-count) |  | Count several patterns in several files in one pass per file |
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
| [`llm_hash`](#llm-hash) |  | Hash files in parallel (sha256 or md5) |
| [`llm_log`](#llm-log) |  | Last lines of a log, read from the end, by pattern, time and level |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_read`](#llm-read) |  | Read files, line ranges or the lines around a match, numbered |
| [`llm_stat`](#llm-stat) |  | Metadata of several paths: type, size, mode, owner, mtime, lines |
| [`llm_textcheck`](#llm-textcheck) |  | Text hygiene checks: non-ASCII, CRLF, BOM, trailing blanks, tabs, long lines |
| [`llm_textstats`](#llm-textstats) |  | Lines, words, characters, bytes, gzip size and longest line of files |
| [`llm_tree`](#llm-tree) |  | Compact directory tree (JSON or text output) |

## llm_assert

Check that files hold, lack or count patterns; exit 0 only if all pass

#### Usage

```text
llm_assert FILE... (--has P | --lacks P | --count P=N)... [--regex] [--lines | --occurrences] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--has P                Every FILE holds P (repeatable, as the two below)
--lacks P              No FILE holds P
--count P=N            Every FILE holds P exactly N times (the last = splits)
--regex                Patterns are regular expressions (ripgrep's syntax);
                       literal text by default
--lines                --count counts the lines that hold P (default)
--occurrences          --count counts every occurrence of P
-j N, --jobs N         Files read at a time (default: the CPU count)
--timeout S            Time limit per file (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
# After an edit: the new name is in, the old one is gone, one route
llm_assert app/A.php app/B.php --has 'NewName' --lacks 'OldName'
llm_assert routes/web.php --count "Route::get('/health'=1"
```

#### Notes

```text
Replaces chains of grep -q / grep -c used as checks, read back by eye.
Every assertion is checked on every FILE, with the llm_count scanner (one
read per file). All pass: {"pass":true,"checked":N}, exit 0. Otherwise
exit 1 and {"pass":false,"checked":N,"failed":[{"file","check","found"}]},
check as has:P, lacks:P or count:P=N; a FILE that cannot be read fails as
{"file","error"} (file_not_found, not_a_file, timeout). Secret files are
refused (secret_file_blocked, FATAL); a bad --count is invalid_assertion.
```

See also: `llm_count`

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

## llm_count

Count several patterns in several files in one pass per file

#### Usage

```text
llm_count FILE... --pat P... [--regex] [--lines | --occurrences] [--format F] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--pat P                A pattern to count (repeatable)
--regex                Patterns are regular expressions (ripgrep's syntax);
                       literal text by default
--lines                Count the lines that hold the pattern, as grep -c
                       (default)
--occurrences          Count every occurrence, as grep -o | wc -l
--format F             tsv, jsonl or table: one row per file and pattern
                       (file, pat, count); json is the default object
-j N, --jobs N         Files read at a time (default: the CPU count)
--timeout S            Time limit per file (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
llm_count storage/logs/laravel.log --pat ERROR --pat WARNING --pat 'Timeout'
llm_count a.md b.md --pat '## ' --format tsv
```

#### Notes

```text
Replaces grep -c A f; grep -c B f; grep -c C g (one pass per pattern).
Every pattern is counted in one read of each file (a large file in
parallel parts). Output: {"FILE":{"P":N,...},...}, files and patterns in
the order given (repeats dropped). A FILE that cannot be read is
{"FILE":{"error":"file_not_found"}} (also not_a_file, timeout). A pattern
never matches across lines; an empty one is missing_pattern, one with a
line break invalid_filter. Secret files are refused (secret_file_blocked,
FATAL). For a case-insensitive count use --regex with (?i).
```

See also: `llm_assert`, `llm_grep`

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

## llm_hash

Hash files in parallel (sha256 or md5)

#### Usage

```text
llm_hash PATH... [--algo sha256|md5] [--short N] [--format F] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--algo sha256|md5      The digest (default sha256)
--short N              Keep the first N hex digits
--format F             json (default), jsonl, tsv or table
--head N, --tail N     Keep the first / last N rows (--max-lines N is --head)
--width N              Cut values at N columns
-j N, --jobs N         Files hashed at a time (default: the CPU count)
--timeout S            Time limit per file (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
llm_hash dist/app.js dist/app.css --short 12
llm_hash backup.tar --algo md5
```

#### Notes

```text
Replaces sha256sum f | cut -c1-12 and md5sum f, file by file.
Output: [{"p","sha256","size"}] in input order (the key is "md5" with
--algo md5). A PATH that cannot be hashed keeps its row with null digest
and size and an "error" (file_not_found, not_a_file, path_unreadable,
timeout); the error column appears only then. Secret files are hashed: a
digest shows no content.
```

See also: `llm_stat`

## llm_log

Last lines of a log, read from the end, by pattern, time and level

#### Usage

```text
llm_log FILE [--tail N] [--last-match RE -n N] [--grep RE] [--since T] [--level L] [--width N] [--format F]
```

#### Options

```text
--tail N, -n N         The last N selected lines (default 50)
--last-match RE        Lines that match RE (ripgrep's syntax); with -n N, the
                       last N matches. Same as --grep RE
--grep RE              Keep the lines that match RE (repeatable: any one)
--since T              Entries at or after T: a duration back from now (30s,
                       10m, 2h, 1d, 1w) or an ISO 8601 date or time
--level L              Entries at level L or more severe: trace, debug,
                       info, notice, warning, error, critical, alert,
                       emergency (warn, err, fatal, ... are read too)
--width N              Cut each line at N columns
--format F             json, jsonl, tsv or table: rows {"line","ts",
                       "level","text"} (line numbers cost a count pass)
```

#### Examples

```bash
llm_log storage/logs/laravel.log --last-match ERROR -n 20
llm_log /var/log/nginx/error.log --since 1h --level error --width 300
```

#### Notes

```text
Replaces tail -n N log | cut -c1-W and grep X log | tail -N.
The file is read backwards in 64 KiB blocks, so the cost is the size of
the answer, not of the log. Lines print in file order, as they are. An
entry is a line with a timestamp or a level and the lines below it with
neither (a stack trace): --since and --level keep or drop whole entries,
--grep single lines. Timestamps read: ISO 8601 (also [bracketed], as
Laravel and Monolog write it), syslog (Oct 25 14:02:01), nginx and Apache
access ([25/Oct/2026:14:02:01 +0000]) and error logs, and an ISO time in
the first 64 bytes (JSON lines). A time without a zone is local (TZ).
With --since the walk stops at the first older entry and a cut by --tail
is marked "[... K lines omitted]". Secret files are refused (FATAL),
binary ones binary_file; a bad T is invalid_duration.
```

See also: `llm_read`

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

## llm_read

Read files, line ranges or the lines around a match, numbered

#### Usage

```text
llm_read PATH[:A-B]... [--lines A:B] [--around RE -C N] [--max-bytes N] [--head N] [--tail N] [--max-lines N] [--width N] [--format F] [--absolute]
```

#### Options

```text
PATH:A-B               Lines A to B of that file (also PATH:A:B, PATH:A-)
--lines A:B            Lines A to B of every PATH without its own range
                       (repeatable; A-B and A,B too, A: and :B leave an end
                       open)
--around RE            The lines that match RE (ripgrep's syntax) and their
                       context (repeatable)
-C N, --context N      Context lines around each match (default 3)
--max-bytes N          Content bytes printed per file (default 262144)
--head N, --tail N     The first / last N selected lines of each file
                       (--max-lines N is --head N)
--width N              Cut each line at N columns
--format F             json, jsonl, tsv or table: rows {"file","line","text"}
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
# Locate and read in one call
llm_read app/Http/Kernel.php --around 'protected \$middleware' -C 10

# Two ranges of two files
llm_read src/a.php:120-180 src/b.php:1-40

# The end of a file
llm_read CHANGELOG.md --tail 30
```

#### Notes

```text
Replaces grep -n X f, then sed -n A,Bp f (two calls), and cat a b.
Each run of consecutive lines is a block "==> FILE:A-B <==", each line N:
then its text (N- for a context line of --around); a cut is marked
"[... K lines omitted]". A file with nothing selected prints its header
and why ([no match in N lines], [empty file]). PATH:A-B is read as a range
only when PATH:A-B itself does not exist. Secret files are refused
(secret_file_blocked, FATAL, as llm_config), binary files binary_file, a
missing file file_not_found, a bad range invalid_range, before anything is
printed.
```

See also: `llm_grep`, `llm_log`

## llm_stat

Metadata of several paths: type, size, mode, owner, mtime, lines

#### Usage

```text
llm_stat PATH... [--lines] [--follow] [--format F] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--lines                Count the lines of regular files under 64 MiB
--follow               Describe what symlinks point to; target is the
                       resolved path
--format F             tsv (default), json, jsonl or table
--head N, --tail N     Keep the first / last N rows (--max-lines N is --head)
--width N              Cut output lines at N columns
-j N, --jobs N         Paths read at a time (default: the CPU count)
--timeout S            Time limit per path (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
llm_stat composer.json vendor storage/logs/laravel.log --lines
llm_stat public/storage --follow --format json
```

#### Notes

```text
Replaces ls -la a b; [ -f c ] && echo yes; wc -l d; readlink -f e.
One row per PATH in input order: p, t (f d l p s c b, or missing), size,
mode (octal, as chmod reads it), owner (user:group), mtime (UTC, ISO
8601), lines with --lines (null for a directory, a link, a file of 64 MiB
or more). target is the link text of a symlink, or with --follow the
resolved path (t "l" then means it could not be followed). A missing path
is a row, not an error. Secret files: metadata yes, lines no (error
secret_file_blocked in that row). Columns lines, target and error appear
only when they apply. TSV by default: the JSON objects of --format json
repeat every key on every row.
```

See also: `llm_hash`, `llm_textstats`

## llm_textcheck

Text hygiene checks: non-ASCII, CRLF, BOM, trailing blanks, tabs, long lines

#### Usage

```text
llm_textcheck FILE... [--rules non-ascii,crlf,bom,trailing-ws,tabs,long-lines=N,final-newline,mixed-indent] [--max-lines N] [--format F] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--rules LIST           The rules to run, comma-separated (default:
                       non-ascii,crlf,bom,trailing-ws,final-newline,
                       mixed-indent)
--max-lines N          Issues listed (default 100; --head N is the same,
                       --tail N keeps the last N)
--width N              Cut samples at N columns
--format F             jsonl, tsv or table: one row per issue; json is the
                       default object
-j N, --jobs N         Files read at a time (default: the CPU count)
--timeout S            Time limit per file (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
llm_textcheck README.md docs/*.md
llm_textcheck src/*.php --rules trailing-ws,tabs,long-lines=120
```

#### Notes

```text
Replaces grep -nP '[^\x00-\x7F]' f, file -i f and python unicodedata
scripts. One read per file. Output: {"files":N,"issues":[{"file","line",
"rule","sample"}]}, plus "counts" by rule when there are issues,
"truncated" when the list was cut and "skipped" ([{"file","reason"}]:
binary_file, file_not_found, not_a_file) when files were skipped. No
issue: {"files":N,"issues":[]}. non-ascii names the code points
(U+200B, U+00E9 e-acute); crlf is one issue per file ("K of N lines");
bom, mixed-indent (the first line indented unlike the first indented
line) and final-newline are once per file. Secret files are refused
(FATAL); an unknown rule is invalid_rule.
```

See also: `llm_textstats`

## llm_textstats

Lines, words, characters, bytes, gzip size and longest line of files

#### Usage

```text
llm_textstats FILE... [--format F] [-j N] [--timeout S] [--absolute]
```

#### Options

```text
--format F             json (default), jsonl, tsv or table
--head N, --tail N     Keep the first / last N rows (--max-lines N is --head)
--width N              Cut values at N columns
-j N, --jobs N         Files read at a time (default: the CPU count)
--timeout S            Time limit per file (seconds, 500ms, 2s, 1m)
--absolute, --relative Absolute paths, or as given (the default)
```

#### Examples

```bash
llm_textstats README.md docs/guide.md
```

#### Notes

```text
Replaces wc -w f; wc -c f; gzip -c f | wc -c (three processes per file).
One read per file. Output: [{"p","lines","words","chars","bytes","gzip",
"max_line"}]: lines counts a last line without a newline, words as wc -w
(in a Markdown file without fenced code blocks and front matter), chars
are UTF-8 characters, gzip is the gzip -c size at level 6 (null from 64
MiB), max_line the longest line in characters. A file that is binary,
secret, missing or not a file keeps its row with null values and an
"error" (binary_file, secret_file_blocked, file_not_found, not_a_file).
```

See also: `llm_stat`, `llm_textcheck`

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
