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
| [`llm_dns`](#llm-dns) |  | DNS records of several names at once (JSON) |
| [`llm_env`](#llm-env) |  | Filtered environment variables (secrets excluded) |
| [`llm_exec`](#llm-exec) |  | Safe command execution wrapper for AI agents |
| [`llm_find`](#llm-find) |  | Find files by pattern (ignores noise directories) |
| [`llm_git_diff`](#llm-git-diff) |  | Git diff with statistics |
| [`llm_git_log`](#llm-git-log) |  | Recent commits (JSON) |
| [`llm_git_overview`](#llm-git-overview) |  | Repository state in one small object (JSON) |
| [`llm_git_show`](#llm-git-show) |  | A commit's metadata and files, its patch, or a file at a revision |
| [`llm_git_status`](#llm-git-status) |  | Compact git status (JSON) |
| [`llm_grep`](#llm-grep) |  | Search code with structured file:line:content output |
| [`llm_hash`](#llm-hash) |  | Hash files in parallel (sha256 or md5) |
| [`llm_http`](#llm-http) |  | Probe several URLs at once: status, timings, headers, JSON value (JSON) |
| [`llm_log`](#llm-log) |  | Last lines of a log, read from the end, by pattern, time and level |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_read`](#llm-read) |  | Read files, line ranges or the lines around a match, numbered |
| [`llm_stat`](#llm-stat) |  | Metadata of several paths: type, size, mode, owner, mtime, lines |
| [`llm_tcp`](#llm-tcp) |  | Can a TCP connection be opened to each HOST:PORT (JSON) |
| [`llm_textcheck`](#llm-textcheck) |  | Text hygiene checks: non-ASCII, CRLF, BOM, trailing blanks, tabs, long lines |
| [`llm_textstats`](#llm-textstats) |  | Lines, words, characters, bytes, gzip size and longest line of files |
| [`llm_tls`](#llm-tls) |  | TLS certificate of several hosts at once: expiry, SANs, issuer (JSON) |
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

## llm_dns

DNS records of several names at once (JSON)

#### Usage

```text
llm_dns NAME... [--type A,AAAA,MX,TXT,NS,CAA,CNAME,SOA] [--server IP]
        [-j N] [--timeout S] [--format F] [--width N] [--max-lines N]
        [--head N] [--tail N]
```

#### Options

```text
--type T,T             Record types to ask for (default A,AAAA): A, AAAA,
                       MX, TXT, NS, CAA, CNAME, SOA
--server IP            Resolver to ask, IP[:PORT] (default: the nameservers
                       of /etc/resolv.conf)
-j, --jobs N           Names at a time (default 32)
--timeout S            Time limit per name, all its types (default 5s)
--format F             json (default), jsonl, tsv or table
--width N              Cut string values at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N rows (cut rows are reported)
```

#### Examples

```bash
llm_dns example.com
llm_dns example.com example.org --type A,MX,TXT
llm_dns _dmarc.example.com --type TXT --server 1.1.1.1
```

#### Notes

```text
Replaces for d in ...; do for t in A MX TXT; do dig +short $t $d; done;
done. One row per NAME, in input order: {"name", one key per type, "ms",
"error"}. A and AAAA (in numeric order), NS, CNAME and TXT are sorted
string lists (a TXT record's strings joined); MX is [{"host","pref"}], CAA
[{"flag","tag","value"}], SOA [{"mname","rname","serial","refresh",
"retry","expire","minimum"}]. A type with no records is []; a type whose
lookup failed is null. error is null, or nxdomain (the name does not
exist), servfail, refused, timeout, connection_refused, bad_response.
Names are asked as given (no search domains).
```

See also: `llm_tcp`, `llm_tls`, `llm_http`

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
         [--sort name|mtime|size] [--newer REF] [--since T] [--tracked]
         [--all] [--count [--by ext|dir|type]] [--relative|--absolute]
         [--format json|jsonl|tsv|table] [--width N] [--max-lines N]
         [--head N] [--tail N]
```

#### Options

```text
--type f|d             Files (default) or directories
--contains REGEX       Keep the files whose content matches REGEX
                       (ripgrep's regex syntax); files only
-n, --limit N          Paths to print (default 100)
--sort name|mtime|size Path order (byte order), newest first, or
                       largest first
--newer REF            Modified after REF (a file or directory)
--since T              Modified at T or later: an age (30s, 15m, 2h,
                       3d, 1w), a date (2026-10-25), a local date and
                       time (2026-10-25T14:30) or @EPOCH
--tracked              Only what git tracks (git ls-files)
--all                  Do not skip node_modules, vendor, .git,
                       __pycache__
--count                Print {"count":N} instead of the paths
--by ext|dir|type      Counts per extension, per first directory below
                       PATH, or per type f, d, l (every type unless
                       --type); implies --count
--relative             Paths relative to PATH (default: absolute)
--absolute             Absolute paths (the default)
--format FORMAT        Rows {"p","size","mtime"}: json, jsonl, tsv
                       or table
--width N              Cut each line (each value in rows) at N columns
--max-lines N          Keep the first N paths (also --head N), or the
                       last N with --tail N; they replace -n and mark
                       what they cut
```

#### Examples

```bash
# Find all PHP files
llm_find "*.php"

# Find config files containing "database"
llm_find "*.conf" /etc --contains "database"

# Find directories only
llm_find "src*" . --type d

# Count PHP files per first directory (find | wc -l)
llm_find "*.php" src --count --by dir

# The 5 newest logs, relative paths (find -newer | sort | head)
llm_find "*.log" . --sort mtime -n 5 --relative
```

#### Notes

```text
Replaces find | sort | head, find | wc -l, find -newer and
git ls-files | grep. PATTERN is a file name glob (find -name). With
--contains only the files it matches are searched, hidden and
gitignored ones included, with or without ripgrep. Skips
node_modules, vendor, .git and __pycache__; --contains also skips
secret files. Default limit 100.
--count, --sort, --format and --head/--tail see every match first:
a cut JSON array or TSV gets {"truncated":K} on stderr, a cut text
output a [... K lines omitted] line. --count counts the paths the
listing prints without -n. mtime is RFC 3339 in UTC.
A missing PATH is path_not_found, an empty PATTERN missing_pattern;
--tracked outside a repository is not_a_git_repo (CONTINUE).
```

## llm_git_diff

Git diff with statistics

#### Usage

```text
llm_git_diff [--staged] [REV..REV] [--max-lines N] [--width N] [PATH]
llm_git_diff [--staged] [REV..REV] (--files | --numstat) [--format F] [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
llm_git_diff [--staged] [REV..REV] --summary [--format F] [PATH]
llm_git_diff [--staged] [REV..REV] --patch [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
```

#### Options

```text
--staged               Staged changes (also --cached)
REV..REV               Compare two commits (A..B, A...B for the merge base)
--files                One row per file: {"p","add","del","status"}
--numstat              One row per file: {"p","add","del"}
--summary              The totals only: {"files","insertions","deletions"}
--patch                The patch as text, capped per file (default 200 lines)
--format F             json (default), jsonl, tsv or table (--files, --numstat,
                       --summary)
--max-lines N          Lines (or rows) to keep: of the --stat text, of the rows,
                       of each file with --patch; also --head N and --tail N
--width N              Cut each line (JSON: each path) at N columns
```

#### Examples

```bash
llm_git_diff
llm_git_diff --staged

# Totals of the work tree, then the changed files of the last three commits
llm_git_diff --summary
llm_git_diff HEAD~3..HEAD --files --format tsv

# One file's patch, 80 lines at most
llm_git_diff --patch --max-lines 80 src/app.js
```

#### Notes

```text
Without an option: git diff --stat text, as git prints it. --files,
--numstat and --summary replace git diff --stat | tail -1, --name-only,
--name-status and --numstat; --patch replaces git diff -- file | head -80.
Paths are relative to the repository root; add and del are null for a
binary file. With a mode option a FILE as PATH limits the diff to it.
--patch keeps each file's header and puts [... K lines omitted] where it
cuts; a secret file's patch is replaced by a marker line (a secret FILE
as PATH is secret_file_blocked). An unknown revision is unknown_revision
(RETRY). Rows that the budget cuts are counted in the stderr metadata line
({"truncated":K}).
```

See also: `llm_git_overview`, `llm_git_status`, `llm_git_log`

## llm_git_log

Recent commits (JSON)

#### Usage

```text
llm_git_log [-n N] [--since D] [--until D] [--grep RE] [--author RE] [--path P]...
            [--fields LIST] [--format F] [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
```

#### Options

```text
-n, --limit N          Commits to list (default 10)
--since D, --until D   Commits after / before a date (git's dates: 2026-01-31,
                       "2 weeks ago", yesterday)
--grep RE              Commits whose message matches RE (extended regex)
--author RE            Commits whose author matches RE (extended regex)
--path P               Commits that touch P (repeat for more; git pathspecs)
--fields LIST          Comma list of hash, subject, date, author, files
                       (default hash,subject,author,date); files is the
                       array of the paths each commit changed
--format F             json (default), jsonl, tsv or table
--max-lines N          Rows to keep (also --head N, --tail N); the cut is
                       counted on stderr
--width N              Cut each string value at N columns
```

#### Examples

```bash
llm_git_log
llm_git_log --limit 20

# The last three subjects, compact
llm_git_log -n 3 --fields hash,subject

# Fixes of the last week that touched src/, with their files
llm_git_log --since "1 week ago" --grep '^fix' --path src --fields hash,subject,files
```

#### Notes

```text
Replaces git log --oneline -N [-- path] and git log -1 --format='%h %s'.
Without a 3.4 option the output is the 2.x array of {hash, subject,
author, date}; any 3.4 option writes the rows through the shared output
layer (same keys, --fields to choose). A bad --grep or --author is
invalid_regex (RETRY); a repository with no commit gives [].
```

See also: `llm_git_overview`, `llm_git_status`, `llm_git_diff`

## llm_git_overview

Repository state in one small object (JSON)

#### Usage

```text
llm_git_overview [PATH] [--log N] [--files N]
```

#### Options

```text
--log N                Last N commit subjects (default 3; 0 for none)
--files N              Also list the paths of each state, at most N each
```

#### Examples

```bash
llm_git_overview
# {"branch":"main","upstream":"origin/main","ahead":1,"behind":0,
#  "counts":{"staged":0,"modified":2,"untracked":1,"conflicted":0},
#  "log":[{"hash":"a1b2c3d","subject":"fix: parser"}],
#  "diffstat":{"files":2,"insertions":10,"deletions":3}}

# With up to 20 paths per state
llm_git_overview . --files 20
```

#### Notes

```text
Replaces git status --short | wc -l; git log --oneline -3; git diff --stat
| tail -1. branch is "" on a detached HEAD, upstream "" without one (ahead
and behind are then 0). counts follow llm_git_status: a path both staged
and modified counts in both. diffstat counts the tracked changes against
HEAD, staged or not (untracked files are only in counts). With --files,
"files" holds the four lists (paths relative to the repository root) and
"truncated" the number of paths left out. Not a repository:
not_a_git_repo (CONTINUE).
```

See also: `llm_git_status`, `llm_git_log`, `llm_git_diff`

## llm_git_show

A commit's metadata and files, its patch, or a file at a revision

#### Usage

```text
llm_git_show [REV] [--stat] [--files] [PATH]
llm_git_show [REV] --patch [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
llm_git_show REV:FILE [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
```

#### Options

```text
--stat                 Add the totals: {"files","insertions","deletions"}
--files                Add each file's status letter (A, M, D, R, C, T)
--patch                The commit's patch as text, capped per file
--max-lines N          Lines kept per file with --patch (default 200), of the
                       content with REV:FILE (default 2000), files in the
                       JSON otherwise; also --head N and --tail N
--width N              Cut each line (JSON: each path and the subject) at N
                       columns
```

#### Examples

```bash
llm_git_show HEAD~1 --stat
# {"hash":"6595b2c","subject":"docs: change 118","author":"Ann",
#  "date":"2026-10-25 22:44:46 +0000","files":[{"p":"CHANGES.md","add":1,
#  "del":0}],"stat":{"files":1,"insertions":1,"deletions":0}}

# The patch, 80 lines per file
llm_git_show a1b2c3d --patch --max-lines 80

# A file as it was two commits ago, first 100 lines
llm_git_show HEAD~2:src/app.js --max-lines 100
```

#### Notes

```text
Replaces git show REV --stat, git show REV --name-status and git show
REV:path | sed -n 1,80p. REV defaults to HEAD; a tag shows its commit; a
merge is shown against its first parent. File paths are relative to the
repository root; add and del are null for a binary file. A FILE as PATH
limits the files and the patch to it. --patch keeps each file's header and
puts [... K lines omitted] where it cuts; the patch of a secret file is
replaced by a marker line. REV:FILE paths are relative to the repository
root (REV:./FILE to PATH); a secret FILE is secret_file_blocked (FATAL), a
binary one binary_file. An unknown REV is unknown_revision (RETRY), a FILE
missing at REV file_not_found, not a repository not_a_git_repo (CONTINUE).
```

See also: `llm_git_log`, `llm_git_diff`

## llm_git_status

Compact git status (JSON)

#### Usage

```text
llm_git_status [PATH]
llm_git_status [PATH] --summary [--fetch] [--timeout S] [--format F]
llm_git_status [PATH] --files [--format F] [--max-lines N] [--head N] [--tail N] [--width N]
llm_git_status [PATH] --fetch [--timeout S]
llm_git_status --repos PATH... [--fetch] [-j N] [--timeout S] [--format F] [--max-lines N] [--width N]
```

#### Options

```text
--summary              The number of paths in each state instead of the lists
--files                One row per changed path: {"p","xy"} (the two letters
                       of git status --short: index, work tree; ?? untracked)
--fetch                Fetch the upstream branch first (no tags, no prune), then
                       print the llm_git_overview object with "fetched"
--repos PATH...        The summary of each repository, in parallel: one row per
                       PATH, in the order given
--timeout S            Bound the fetch (default 20s), with --repos each
                       repository
-j N                   Repositories at a time (default: the CPU count, 8 with
                       --fetch)
--format F             json (default), jsonl, tsv or table (--summary, --files,
                       --repos)
--max-lines N          Rows to keep (also --head N, --tail N)
--width N              Cut each string value at N columns
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

# Counts only; the changed paths as rows
llm_git_status --summary
llm_git_status --files --format tsv

# Fresh ahead/behind, and three repositories at once
llm_git_status --fetch
llm_git_status --repos ~/src/api ~/src/web ~/src/docs --format tsv
```

#### Notes

```text
Without an option: the pretty 2.x object above. --summary replaces git
status --short | wc -l, --files git status --short, --repos a loop of
git -C R status, --fetch git fetch && git status -sb. --summary prints
{"branch","upstream","ahead","behind","staged","modified",
"untracked","conflicted"}; --repos rows add "repo" first and "error"
last (null, or the code of a PATH that is refused, missing or not a
repository). --fetch never prompts: a failure (network, credentials,
timeout) is "fetched":false with "error", and the local state is still
printed; a repository whose own configuration names a program for the
fetch (core.sshCommand, a credential helper, uploadpack, url.insteadOf,
protocol.*) is not fetched. Paths are relative to the repository root.
```

See also: `llm_git_overview`, `llm_git_diff`, `llm_git_log`

## llm_grep

Search code with structured file:line:content output

#### Usage

```text
llm_grep PATTERN [PATH] [--ext EXT1,EXT2] [--context N] [-n N]
         [--exclude GLOB]... [--only-matching] [--distinct]
         [--files-only] [--count [--by file|ext|dir]]
         [--relative|--absolute] [--format json|jsonl|tsv|table]
         [--width N] [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--ext EXT1,EXT2        Search only files with these extensions
-C, --context N        Lines of context around each match (default 0)
-n, --limit N          Lines to print (default 100)
--exclude GLOB         Skip the files and directories whose name or
                       path below PATH matches GLOB (repeatable)
-o, --only-matching    Print each match instead of its line
--distinct             Each distinct match with its count, most
                       frequent first: [{"match","count"}]
                       (implies --only-matching)
-l, --files-only       Print the matching files
-c, --count            Print {"count":N}: matching lines (matches
                       with -o, files with -l)
--by file|ext|dir      Counts per file, extension or first directory
                       below PATH; implies --count
--relative             Paths relative to PATH (default: absolute)
--absolute             Absolute paths (the default)
--format FORMAT        Rows {"file","line","text"} ({"file"} with
                       -l): json, jsonl, tsv or table
--width N              Cut each line (each value in rows) at N columns
--max-lines N          Keep the first N lines (also --head N), or the
                       last N with --tail N; they replace -n and mark
                       what they cut
```

#### Examples

```bash
# Find TODO comments in PHP files
llm_grep "TODO|FIXME" src/ --ext php,js

# Search for a function definition
llm_grep "function processOrder" .

# With context lines
llm_grep "class.*Controller" . --ext php --context 3

# Most frequent matches (grep -o | sort | uniq -c | sort -rn)
llm_grep "Log::[a-z]+" src --distinct

# Without tests, relative paths
llm_grep TODO . --exclude tests --relative -n 20
```

#### Notes

```text
Replaces grep -rn | grep -v | head, grep -o | sort | uniq -c and
grep -rl / grep -c.
One built-in engine, the same with or without ripgrep installed:
ripgrep's regex syntax; hidden files, .gitignore/.ignore/.rgignore
matches, binary files, node_modules, vendor and .git are skipped; files
come in sorted path order. Secret files are skipped, templates such as
.env.example searched. Default limit 100.
Output: file:line:content with absolute paths (a FILE PATH gives
line:content); context lines use -, groups are separated by --.
--count, --distinct, --format and --head/--tail see every match
first: a cut JSON array or TSV gets {"truncated":K} on stderr, a cut
text output a [... K lines omitted] line. --context goes with the
text output only (conflicting_options otherwise).
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

## llm_http

Probe several URLs at once: status, timings, headers, JSON value (JSON)

#### Usage

```text
llm_http URL... [--method M] [--header H]... [--auth-env VAR]...
         [--resolve HOST:IP]... [--follow] [--status-only] [--headers h1,h2]
         [--json-path P] [--grep RE] [--body-max N] [--expect CODE]
         [--insecure] [-j N] [--timeout S] [--format F] [--width N]
         [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--method M             GET (default), HEAD or OPTIONS; any other method is
                       a write and is refused (http_method_blocked, FATAL)
--header H             A request header "Name: value" (repeatable); "Host:
                       NAME" sets the Host. A credential header
                       (Authorization, Cookie, a name with TOKEN, KEY,
                       SECRET, AUTH, ...) is refused: use --auth-env
--auth-env VAR         Send the header held by the exported variable VAR:
                       "Name: value", or a bare value meaning
                       "Authorization: value" (repeatable). Never printed,
                       never sent on to another host by a redirect
--resolve HOST:IP      Connect to IP for HOST (also HOST:PORT:IP); TLS and
                       the Host header keep HOST; no proxy for HOST
--follow               Follow redirects (at most 10)
--status-only          Do not read the body: url, status, ms, ok, error
--headers h1,h2        Add these response headers (credentials masked)
--json-path P          Add the value at P of the JSON body (a.b[0].c)
--grep RE              Add the first body line matching RE (RE2) and the
                       number of matching lines
--body-max N           Body bytes kept for --grep and --json-path
                       (default 8388608)
--expect CODE          Statuses that are ok: codes or classes, e.g. 200,
                       2xx, 200,301 (default 200 to 399)
-k, --insecure         Do not verify the certificate (not with --auth-env)
-j, --jobs N           URLs at a time (default 32)
--timeout S            Time limit per URL, redirects and body included
                       (default 10s)
--format F             json (default), jsonl, tsv or table
--width N              Cut string values at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N rows (cut rows are reported)
```

#### Examples

```bash
# Status of many URLs at once
llm_http https://a.example/health https://b.example/health --status-only

# Redirect chain and two headers
llm_http http://example.com --follow --headers location,content-type

# An API field, the credential from the environment
export API_AUTH='Authorization: Bearer ...'
llm_http https://api.example/jobs/42 --auth-env API_AUTH --json-path status

# A site on a new server before DNS points at it
llm_http https://example.com/ --resolve example.com:203.0.113.7 --grep '<title>'
```

#### Notes

```text
Replaces curl -w %{http_code} loops, curl -I | grep, curl | jq and
curl | grep. Read-only: GET, HEAD and OPTIONS, no request body. One row per
URL, in input order: {"url","status","ms","timing","bytes","redirects",
"ok","error"}, then "headers", "value", "match" and "matches" when asked
(--status-only: url, status, ms, ok, error). timing is {"dns","connect",
"tls","ttfb"} in ms; redirects lists each 3xx as {"status","location"}.
ok: no error, the status expected, --grep matched, --json-path found.
error is null or one of dns_not_found dns_error connection_refused
connection_reset unreachable timeout tls_error certificate_error
too_many_redirects transport_error invalid_json json_path_not_found
body_max_exceeded. A URL with a password is refused (secret_in_argv).
Proxies from HTTPS_PROXY, HTTP_PROXY and NO_PROXY; CAs from the system or
SSL_CERT_FILE / SSL_CERT_DIR.
```

See also: `llm_tls`, `llm_tcp`, `llm_dns`

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
llm_ports [--listen] [--port N]
```

#### Options

```text
-l, --listen           Listening sockets only
--port N               Sockets on local port N (0 to 65535), each with the
                       processes that hold it
```

#### Examples

```bash
llm_ports
llm_ports --listen

# Who listens on 8080
llm_ports --listen --port 8080
```

#### Notes

```text
--port replaces ss -tlnp | grep :8080: it adds "procs":[{"pid","name"}]
to each record (from ss -p or netstat -p; empty for another user's process
unless you are root).
```

See also: `llm_procs`, `llm_tcp`

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

## llm_tcp

Can a TCP connection be opened to each HOST:PORT (JSON)

#### Usage

```text
llm_tcp HOST:PORT... [-j N] [--timeout S] [--format F] [--width N]
        [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
-j, --jobs N           Targets at a time (default 32)
--timeout S            Connect time limit per target (default 5s)
--format F             json (default), jsonl, tsv or table
--width N              Cut string values at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N rows (cut rows are reported)
```

#### Examples

```bash
llm_tcp db.internal:5432 cache.internal:6379
llm_tcp [::1]:22 127.0.0.1:8080 --timeout 1
```

#### Notes

```text
Replaces nc -z h p and timeout 2 bash -c '</dev/tcp/h/p'. Nothing is sent:
the connection is closed once open. One row per target, in input order:
{"target","open","ms","error"}; error is null, or connection_refused,
timeout (filtered), unreachable, dns_not_found, dns_error.
```

See also: `llm_ports`, `llm_tls`, `llm_http`

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

## llm_tls

TLS certificate of several hosts at once: expiry, SANs, issuer (JSON)

#### Usage

```text
llm_tls HOST[:PORT]... [--sni NAME] [-j N] [--timeout S] [--format F]
        [--width N] [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
--sni NAME             Server name to send and verify (default: HOST)
-j, --jobs N           Hosts at a time (default 32)
--timeout S            Time limit per host, connect and handshake
                       (default 10s)
--format F             json (default), jsonl, tsv or table
--width N              Cut string values at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N rows (cut rows are reported)
```

#### Examples

```bash
llm_tls example.com example.org
llm_tls mail.example.com:465 203.0.113.7 --sni www.example.com
```

#### Notes

```text
Replaces echo | openssl s_client -connect h:443 -servername h | openssl
x509 -noout -dates -subject. PORT defaults to 443; an https:// URL is
taken too. One row per host, in input order: {"host","protocol",
"subject","issuer","san","not_after","days_left","verified",
"verify_error","chain","ms","error"}. The handshake accepts any
certificate, so an expired or self-signed one is still described; verified
says whether the chain checks out for the server name against the system
CAs (or SSL_CERT_FILE / SSL_CERT_DIR), verify_error why not: expired,
not_yet_valid, hostname_mismatch, unknown_authority, invalid. chain lists
the certificates after the leaf as {"subject","issuer","not_after"}.
error is null, or connection_refused, timeout, dns_not_found, tls_error
(the handshake failed), no_certificate. No STARTTLS.
```

See also: `llm_http`, `llm_tcp`

## llm_tree

Compact directory tree (JSON or text output)

#### Usage

```text
llm_tree [--text] [--depth N] [--stats] [--all] [--relative|--absolute]
         [--width N] [--max-lines N] [--head N] [--tail N] [PATH]
llm_tree --count [--by ext|dir|type] [--type f|d] [--name GLOB]
         [--newer REF] [--since T] [--depth N] [--all]
         [--format json|jsonl|tsv|table] [PATH]
llm_tree --top N [--by dir|ext] [--depth N] [--name GLOB]
         [--newer REF] [--since T] [--format json|jsonl|tsv|table] [PATH]
```

#### Options

```text
--text                 Indented text list, 200 lines max (default depth 3)
--depth N              Levels below PATH, 1 or more (default 1; also
                       --max-depth); with --count, no limit unless given
--stats                Add sizes and child counts to the JSON tree
--all                  Do not skip node_modules, vendor, .git,
                       __pycache__, .cache
--relative             The root's path as "." (--top: relative is the
                       default)
--absolute             Absolute paths (--top rows too)
--count                Print {"count":N}: the entries below PATH
--by ext|dir|type      --count per extension, first directory below
                       PATH, or type f, d, l (implies --count); --top
                       per directory (default) or extension
--type f|d             --count files or directories only
--name GLOB            --count/--top: names matching GLOB only
--newer REF            --count/--top: modified after REF only
--since T              --count/--top: modified at T or later (30s, 2h,
                       3d, 1w, 2026-10-25, 2026-10-25T14:30, @EPOCH)
--top N                Disk usage: {"total","files","top":[{"p",
                       "size","files"}],"skipped"}, largest first
--format FORMAT        --count/--top as json, jsonl, tsv or table rows
--width N              Cut each line (each value) at N columns
--max-lines N          --text/--top: keep the first N lines or rows
                       (also --head N), or the last N with --tail N
```

#### Examples

```bash
# JSON output (default)
llm_tree

# Text-indented output
llm_tree --text

# Limited depth
llm_tree --depth 2 /etc/nginx

# Files per extension (find -type f | wc -l)
llm_tree --count --type f --by ext src

# The 10 largest directories (du -sh */ | sort -rh | head)
llm_tree --top 10 .
```

#### Notes

```text
Replaces du -sh */ | sort -rh | head and find | wc -l.
Ignores noise directories below PATH: node_modules, vendor, .git,
__pycache__, .cache. Entries whose name holds a tab or
a newline are left out. --text lists symlinks as name -> target, in
byte order whatever the locale.
--top sums the apparent size (st_size) of every file below PATH,
noise directories included (disk usage hides nothing); --by dir
lists each directory down to --depth (default 1), as du -d does.
Directories that cannot be read count in skipped; a cut list gets
"truncated":K.
```
