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
| [`llm_http`](#llm-http) |  | Probe several URLs at once: status, timings, headers, JSON value (JSON) |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_tcp`](#llm-tcp) |  | Can a TCP connection be opened to each HOST:PORT (JSON) |
| [`llm_tls`](#llm-tls) |  | TLS certificate of several hosts at once: expiry, SANs, issuer (JSON) |
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
-j, --jobs N           Names at a time (default 8)
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
-j, --jobs N           URLs at a time (default 8)
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

## llm_tcp

Can a TCP connection be opened to each HOST:PORT (JSON)

#### Usage

```text
llm_tcp HOST:PORT... [-j N] [--timeout S] [--format F] [--width N]
        [--max-lines N] [--head N] [--tail N]
```

#### Options

```text
-j, --jobs N           Targets at a time (default 8)
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
-j, --jobs N           Hosts at a time (default 8)
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
