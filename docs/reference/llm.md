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
| [`llm_lint`](#llm-lint) |  | Syntax-check files by extension, in parallel (php, sh, js, json, yaml, toml, py) |
| [`llm_ports`](#llm-ports) |  | List ports in use (JSON) |
| [`llm_procs`](#llm-procs) |  | List processes by name or port (JSON) |
| [`llm_project`](#llm-project) |  | Detect project type and framework (JSON) |
| [`llm_runtime`](#llm-runtime) |  | Resolve a node major or php version to its binary, without nvm.sh |
| [`llm_test`](#llm-test) |  | Run a test command and summarize it: counts and each failure's file:line |
| [`llm_tools`](#llm-tools) |  | Path and version of each tool, probed in parallel; php and node per version |
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

## llm_lint

Syntax-check files by extension, in parallel (php, sh, js, json, yaml, toml, py)

#### Usage

```text
llm_lint [PATH...] [--ext php,sh,js,json,yaml,toml,py] [--php VER] [--node MAJOR] [-j N] [--timeout S] [--max-lines N] [--width N] [--absolute]
```

#### Options

```text
--ext LIST             Languages to check in directories (default: all):
                       php, sh (also .bash), js (.mjs, .cjs), json, yaml
                       (.yml), toml, py
--php VER              The php that checks .php files: 8.4, 8 (gash
                       runtime's resolution; default: php on PATH)
--node MAJOR           The node that checks .js files: 24 (default: node on
                       PATH)
-j, --jobs N           Checks at a time (default: the CPU count)
--timeout S            Time limit of one checker process (default 60s)
--max-lines N          Failures listed (default 100); --head N and --tail N
                       keep the first and last N, "truncated" counts the rest
--width N              Cut file names and messages at N columns
--relative, --absolute File paths relative to their PATH (default) or
                       absolute
```

#### Examples

```bash
# Every PHP file of src with php 8.4
llm_lint src --ext php --php 8.4

# A whole project, every language
llm_lint .

# Two files
llm_lint deploy.sh config/app.yaml
```

#### Notes

```text
Replaces for f in *.php; do php -l "$f"; done, bash -n and node --check
loops. Output: {"checked":N,"failed":K,"failures":[{"file","line","error"}]}.
php runs once per chunk of files (php -l a b c), bash -n once per file, node
once per chunk (each file compiled as node loads it; a failure is re-checked
with node --check for its message), python compiles in one process (no .pyc
written); JSON, YAML and TOML are parsed by gash (tsconfig.json and .vscode
files may hold comments). Directories skip node_modules, vendor, .git and
__pycache__ and never follow symlinks; secret files are skipped there and
refused as arguments. A FILE argument with another extension is
unsupported_file. A missing checker is runtime_not_installed (STOP).
```

See also: `llm_test`, `llm_runtime`

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

## llm_runtime

Resolve a node major or php version to its binary, without nvm.sh

#### Usage

```text
llm_runtime php|node|composer|npm|npx VERSION
```

#### Examples

```bash
# The node 24 binary
llm_runtime node 24

# composer under php 8.4
llm_runtime composer 8.4
```

#### Notes

```text
Replaces source ~/.nvm/nvm.sh; nvm use 24; which node (and php84 wrappers).
Output: {"runtime":"node","request":"24","path":".../bin/node","version":"24.1.0"};
composer adds "php" (the binary it runs under), npm and npx report their
node's version. VERSION is a prefix: 24, 24.1, 8.4, 8: the newest installed
version that starts with it. node comes from the nvm tree
(${NVM_DIR:-~/.nvm}/versions/node), then the node on PATH; php from
/usr/bin/phpX.Y and /usr/local/bin/phpX.Y, then the php on PATH. Not
installed: runtime_not_installed (STOP) with the installed versions. The
JSON form of gash runtime; it reads gash's runtime cache, never writes it.
```

See also: `llm_tools`, `node_detect_version`, `php_detect_version`

## llm_test

Run a test command and summarize it: counts and each failure's file:line

#### Usage

```text
llm_test [--runner auto|NAME] [--fail-only] [--max-lines N] [--width N] [--timeout S] [--absolute] -- CMD...
```

#### Options

```text
--runner auto|NAME     The output format: pest, phpunit, phpstan, psalm,
                       vitest, jest, mocha, pytest, go, cargo, playwright
                       (default auto: from CMD and the output)
--fail-only            Print nothing when CMD exits 0 with no failure
--max-lines N          Failures listed (default 50; --head N, --tail N);
                       unparsed output: the last N lines (default 20)
--width N              Cut messages at N columns (default 200)
--timeout S            End CMD after S (default: no limit)
--relative, --absolute Paths below the working directory relative (default)
                       or as the runner printed them
```

#### Examples

```bash
# Pest with a filter
llm_test -- vendor/bin/pest --filter UserTest

# A package script; silent when green
llm_test --fail-only -- npm test

# Go with the JSON reporter
llm_test -- go test -json ./...
```

#### Notes

```text
Replaces pest | grep -E 'Tests:|FAIL' and npm test | tail -20. CMD runs as
given (bash -c 'cd app && vendor/bin/pest' works), stdout and stderr
merged, stdin /dev/null; the exit status is CMD's. Output:
{"runner","passed","failed","skipped","duration_s","failures":[{"name",
"file","line","message"}],"parsed":true,"exit_code"}; analyzer findings
(phpstan, psalm) are failures. Output no parser reads:
{"runner":null,"duration_s","parsed":false,"exit_code","tail":[...]}.
Options come before --; a program not on PATH is command_not_found. CMD is
the project's own code: llm_test runs it without llm_exec's rails.
```

See also: `llm_lint`

## llm_tools

Path and version of each tool, probed in parallel; php and node per version

#### Usage

```text
llm_tools [NAME...] [-j N] [--timeout S]
```

#### Options

```text
-j, --jobs N           Probes at a time (default 8)
--timeout S            Time limit of one probe (default 10s)
```

#### Examples

```bash
# The usual toolchain
llm_tools

# A few tools
llm_tools git python3 rg php
```

#### Notes

```text
Replaces git --version; php -v | head -1; command -v rg. Output:
{"git":{"version":"2.43.0","path":"/usr/bin/git"},"rg":null,
"php":[{"version":"8.3.6","path":"/usr/bin/php8.3"}]}, keys in the order
asked. php and node list every installed version (/usr/bin/phpX.Y, the nvm
tree, the one on PATH), oldest first; a tool not on PATH is null, one whose
probe prints no version has "version":null. Each NAME is a plain program
name, probed with --version (go version, ssh -V, java -version; npm and npx
read npm's package.json). Without NAME: git php composer node npm python3 go
rg jq docker.
```

See also: `llm_runtime`, `llm_env`

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
