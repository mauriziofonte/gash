# AGENTS.md: Gash for LLM coding agents

> **Audience:** coding agents that run Bash (Claude Code, Codex, Cursor, Gemini,
> Aider, local models) and the people who configure them.
> **Applies to:** Gash 2.0.0+, including the 3.0 line. Gash 3.0 reimplements
> the toolkit in Go behind the same Bash entry point (`source ~/.gash/gash.sh`),
> so every invocation, function name, flag, JSON shape and error code below is
> unchanged.
> **Stable API:** the public function names and flags below, the JSON shapes
> in section 4, the error contract in section 5 and the variables in section 2.

## 1. Quick start

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status'
```

`GASH_DIR` is derived from the location of `gash.sh`, so a checkout anywhere
works the same way (`source /path/to/gash/gash.sh`). Gash needs Bash 4.4+;
most JSON output also needs `jq`. Help for any function: `llm_tree --help`
or `gash_help llm_tree`.

## 2. Invocation modes

| Variable | Effect |
|---|---|
| `GASH_HEADLESS=1` | Recommended for agents. Loads core and function modules only, prints nothing while loading, no ANSI colors. |
| `NO_COLOR=1` / `GASH_NO_COLOR=1` | No ANSI colors; everything else as in an interactive shell. |
| `GASH_DEBUG=1` | With headless: show the stderr of files that fail to load. |

Headless mode does **not** load the prompt, history settings, banner, the
alias pack, `~/.bash_aliases` or `~/.bash_local`. Module short aliases exist,
but Bash does not expand aliases in non-interactive shells: use full function
names. One exception to "prints nothing": with a Gash 1.x configuration, one
stderr line (at most once a day) asks the user to run the migration.

Without headless mode, `gash.sh` does nothing in a non-interactive shell. Do
not use `bash -i` and do not source `~/.gashrc` (the user's entry point).
`gash_doctor` lists files that failed to load. Without the color variables,
Gash still drops colors for most output when stdout is not a terminal; set
one of them if you parse text.

## 3. Function catalog

Flags (the signatures below are the registered help's, checked by a test):
where a function takes `-n`, it means `--limit`; `--json` is the
machine-output flag. Options are separate words: `--flag=value` and a `--`
end-of-options marker are not supported (both are `unknown_option`, as is
any option a function does not list). A function that takes a PATH refuses
a second one with `unexpected_argument`; `llm_ports` and `llm_procs` take no
PATH (a stray word is `unknown_option`) and `llm_env` reads a bare word as
its filter. A value flag at the end of the line fails with `missing_value`;
integers are plain digits, at most 9. `--help` or `-h` anywhere on an
`llm_*` line prints its help, also where a flag expects its value
(`llm_exec`: as the only argument). A PATH may start with `~` or `~/` (your
home); `~user` is `invalid_path`.

### 3.1 Files and code

| Function | Output |
|---|---|
| `llm_tree [--depth N] [--stats] [--text] [PATH]` | JSON tree (default depth 1); `--stats` adds sizes and child counts; `--text` indented list (symlinks as `name -> target`), 200 lines max |
| `llm_find PATTERN [PATH] [--type f\|d] [--contains REGEX] [-n N]` | Paths, one per line (default 100); PATTERN is a name glob, `--contains` keeps the matching files whose content matches |
| `llm_grep PATTERN [PATH] [--ext a,b] [--context N] [-n N]` | `file:line:content` lines (default 100), absolute paths; a FILE PATH gives `line:content`; context lines use `-`, groups are separated by `--` |
| `llm_config FILE [--max-bytes N]` | File content; YAML converted to JSON by Gash itself (no `yq` needed), the raw text when it does not parse; files over 1 MiB (default) fail with `file_too_large` |

All three search functions skip node_modules, vendor and .git (`llm_tree` also
\_\_pycache\_\_ and .cache, `llm_find` also \_\_pycache\_\_). `llm_grep` and
`llm_find --contains` share one built-in engine, the same whether ripgrep is
installed or not: ripgrep's regex syntax, binary files skipped, files in
sorted path order; `llm_grep` also skips hidden files and what
`.gitignore`/`.ignore`/`.rgignore` exclude, `llm_find --contains` searches
them. Zero matches: exit 0, no output; a PATH that does not exist is
`path_not_found`, an empty PATTERN `missing_pattern`.

### 3.2 Filesystem scanners

| Function | Notes |
|---|---|
| `files_largest [PATH]` | Largest files; `-n N` (default 100), `--min-size SIZE`, `--depth N` |
| `dirs_largest [PATH]` | Largest directories by cumulative size; `--depth N` (default 1), `-n N` |
| `dirs_find_large [PATH]` | Directories over `--size SIZE` (default 20M); `-n N` (default 1000), `--with-mtime` |
| `dirs_list_empty [PATH]` | Empty directories; `--count`, `--min-depth N`, `--ignore-dotfiles` |
| `tree_stats [PATH]` | Totals, top extensions (`--top N`), depth, empty dirs |

Common flags (each `--help` lists its own set): `--json`, `--null`/`-0`
(NUL-separated paths), `--exclude GLOB` (repeatable), `--no-ignore`, `--xdev`,
`--allow-root` (required to scan `/`), `--human`/`--no-human`, `--no-color`.
Sizes take bytes or K/M/G/T/P suffixes. Paths containing a tab or a newline
are left out of the results.

### 3.3 Databases (read-only)

```
llm_db_query   "SQL"  [-c CONN | -f SQLITE_FILE] [-d DB] [-n ROWS]   # default 100 rows
llm_db_tables         [-c CONN | -f SQLITE_FILE] [-d DB]
llm_db_schema  TABLE  [-c CONN | -f SQLITE_FILE] [-d DB]
llm_db_sample  TABLE  [-c CONN | -f SQLITE_FILE] [-d DB] [-n N]       # default 5
llm_db_explain "SQL"  [-c CONN | -f SQLITE_FILE] [-d DB] [--analyze]
```

Connections live in `~/.gash_env` as `DB:name=driver://user:pass@host:port/db`
(mysql, mariadb, pgsql); without `-c` the one named `default` is used.
`gash_db_list` prints the names with masked passwords. SQLite needs only
`-f FILE`. The database is `-d`, else the connection's, else the first word
of a `.target-database` file in the current directory; `-d` and that file
must hold a plain name (`[A-Za-z0-9_][A-Za-z0-9_$.-]{0,63}`, otherwise
`invalid_database_name`). The row limit is `-n` (also `--limit`, and `-r`/`--rows` on
`llm_db_query`). SQLite and PostgreSQL return native JSON types,
MySQL/MariaDB return every value as a string and a SQL NULL as the string
`"NULL"`. `llm_db_schema` keys follow the engine: SQLite
`cid,name,type,notnull,dflt_value,pk`, MySQL/MariaDB
`name,type,nullable,key,default`, PostgreSQL
`column_name,data_type,is_nullable,column_default`. `-h` is always help: the
host comes only from the connection URL. The `details` of `sqlite_error`,
`mysql_error` and `pgsql_error` is the database driver's message on one
line. `llm_db_query` adds a
slow_query_explain field (the plan) when a query takes 100 ms or more
(the query alone: connecting and the session setup do not count).
`--analyze` executes the query to measure it, still read-only.

### 3.4 Git, system, project

| Function | Output |
|---|---|
| `llm_git_status [PATH]` | JSON object (4.6) |
| `llm_git_log [-n N] [PATH]` | JSON array of `{hash, subject, author, date}` (default 10) |
| `llm_git_diff [--staged] [PATH]` | `git diff --stat` text |
| `llm_env [--filter REGEX]` | JSON object; secret-looking names left out (PASSWORD, TOKEN, KEY, SECRET, SESSION, JWT, WEBHOOK, `*_PWD`, `*_PAT`, ...), URL passwords (anywhere in a value, up to the last `@` of the URL) and known token formats in values shown as `***` |
| `llm_ports [--listen]` | `[{port, proto, state, address}]`, one record per socket, sorted by port then proto; needs `ss` or `netstat` (`ports_tool_not_found` without) |
| `llm_procs [--name NAME \| --port PORT]` | `[{pid, name, ...}]`; no filter: first 20 of `ps aux`; `--name` matches full command lines; `--port` (0-65535, else `invalid_port`) wins over `--name` |
| `llm_project [PATH]` | `{type, framework, package_manager, has_tests, entry_point, path}`; a missing PATH is `path_not_found` |
| `llm_deps [PATH] [--dev]` | `{manager, dependencies}`, or `{manager, prod, dev}` with `--dev`; names sorted; a `composer.json`/`package.json` that is not one JSON object is `invalid_manifest` |
| `llm_docker_check [PATH]` | Compose image update check as JSON (contacts the registries); an unknown digest is `null`. PATH follows the path rules (6.1); `${VAR}` is resolved from `PATH/.env` and a short allowlist (`HOME`, `USER`, `PATH`, `LANG`, `DOCKER_*`, ...), never from other exported variables |
| `php_detect_version [DIR] [--json]` | `{version, php_raw_binary, php_command, ...}`; JSON by default in headless |
| `node_detect_version [DIR] [--json]` | Same for Node (needs nvm) |

`llm_project` types: `php`, `javascript`, `python`, `go`, `rust`, `unknown`.
The framework comes from the dependency keys of `package.json`
(`dependencies`/`devDependencies`: next, react, vue, express) and
`composer.json` (`require`/`require-dev`: `symfony/*`; laravel by `artisan`);
flask from a `requirements.txt` entry; `package_manager` is `poetry` only
when `pyproject.toml` has a `[tool.poetry]` table.
Example: `{"type":"javascript","framework":"express","package_manager":"npm","has_tests":true,"entry_point":"","path":"/tmp/web.tbjb"}`.

### 3.5 Command execution

`llm_exec "COMMAND LINE"` runs one quoted command line read-only in a child
Bash and passes through its stdout, stderr and exit status (rules in 6.3):

```bash
llm_exec "git log --oneline -5 && git status --short"
llm_exec "grep -rn TODO src | head -20"
```

## 4. Output shapes

Real output; only whitespace was changed to fit the page.

### 4.1 Size list (`files_largest`, `dirs_largest`, `dirs_find_large` with `--json`)

```json
{
  "data": [
    {"size": 524288, "size_human": "512.0 KiB", "mtime": "2026-10-14 12:20", "path": "/tmp/web.tbjb/logs/app.log"},
    {"size": 70000, "size_human": "68.3 KiB", "mtime": "2026-10-14 12:20", "path": "/tmp/web.tbjb/src/logo.png"}
  ],
  "count": 2, "total_size": 594288, "total_size_human": "580.3 KiB",
  "kind": "files", "scan_time": "7ms", "errors": 0
}
```

kind is "files" or "directories"; directories have no mtime unless
`dirs_find_large --with-mtime`. errors counts unreadable entries (usually
permission denied). No results: `"data": []` and `"count": 0`.

### 4.2 Empty dirs (`dirs_list_empty --json`)

```json
{"data":["/tmp/web.tbjb/empty","/tmp/web.tbjb/tests"],"count":2,"path":"/tmp/web.tbjb","errors":0}
```

### 4.3 Tree stats (`tree_stats --json --top 2`)

```json
{"data":{"path":"/tmp/web.tbjb","files_total":4,"dirs_total":5,"size_total":594370,
  "size_total_human":"580.4 KiB","avg_file_size":148592,"max_depth":1,"empty_dirs":2,
  "top_by_count":[{"ext":"log","count":1,"size":524288},{"ext":"js","count":1,"size":2}],
  "top_by_size":[{"ext":"log","size":524288},{"ext":"png","size":70000}]},
 "scan_time":"6ms","errors":0}
```

### 4.4 Database (`llm_db_*`)

```json
{"data":[{"id":1,"name":"alice"},{"id":2,"name":"bob"}],"rows":2,"query_time":"32.5ms"}
{"data":[],"rows":0,"query_time":"29.1ms"}
{"data":[{"plan":"QUERY PLAN\n`--SEARCH users USING INTEGER PRIMARY KEY (rowid=?)"}],"rows":1,"query_time":"38.2ms"}
```

`llm_db_tables` returns `{"data":["users"],...}`. `llm_db_explain` always
returns an array: JSON plans as the engine gives them, SQLite text plans as
`[{"plan": TEXT}]`; `llm_db_query "EXPLAIN QUERY PLAN ..."` on SQLite gives
the same `[{"plan": TEXT}]`, and a SQLite bytecode `EXPLAIN` its rows
(`addr`, `opcode`, `p1`..`p5`, `comment`). query_time is in µs, ms or s, and counts the time the
query ran on the server (sent to last row read), not the connection.

### 4.5 Tree (`llm_tree --depth 2 --stats`)

```json
{"path":"/tmp/web.tbjb","type":"directory","size":594370,"children_count":5,"children":[
  {"name":"src","type":"directory","size":70002,"children_count":2,"children":[
    {"name":"app.js","type":"file","size":2},{"name":"logo.png","type":"file","size":70000}]},
  {"name":"logs","type":"directory","size":524288,"children_count":1,"children":[
    {"name":"app.log","type":"file","size":524288}]},
  {"name":"empty","type":"directory","size":0,"children_count":0},
  {"name":"tests","type":"directory","size":0,"children_count":0},
  {"name":"package.json","type":"file","size":80}]}
```

type is "directory", "file" or "link"; children are not sorted. Without
`--stats` nodes carry only name and type. Directory sizes are cumulative and
children counts are real, also below the depth limit. The listing stops at
500 entries; the root then carries `"truncated": true`.

### 4.6 Git status (`llm_git_status`)

```json
{"branch":"main","ahead":0,"behind":0,"staged":["notes.txt"],"modified":["src/app.js"],"untracked":["TODO.md"],"conflicted":[]}
```

branch is "" on a detached HEAD. A file can be both staged and modified.

## 5. Error contract

`llm_*` functions and the filesystem scanners (in `--json` mode) report
errors on **stderr** as one JSON line, with a non-zero exit status:

```json
{"error":"not_a_git_repo","details":"/tmp is not a git repository","action":"CONTINUE","recoverable":true,"hint":"Not inside a git repository. Skip git operations"}
```

`error`, `action` and `recoverable` are always present; `details` and `hint`
only when non-empty. `recoverable` is `false` exactly when `action` is
`FATAL`. `llm_db_*` errors also carry query_time.

| Action | Meaning | What to do |
|---|---|---|
| `RETRY` | Bad input: flag, value, syntax | Fix the arguments, retry once |
| `CONTINUE` | Not applicable here (no repository, no package file) | Skip this step, go on |
| `STOP` | Needs a human decision (which connection, allow a command) | Ask the user; show `hint` |
| `FATAL` | Refused by policy, or impossible (secret file, write query, missing client) | Do not retry or rephrase; show `hint`, stop |

Each code always carries the action shown:

| Area | Codes |
|---|---|
| Arguments (RETRY) | `unknown_option` `missing_value` `unexpected_argument` `invalid_limit` `invalid_depth` `invalid_size` `invalid_top` `invalid_port` `invalid_context` `invalid_regex` `invalid_filter` `invalid_type` `invalid_database_name` `missing_pattern` `missing_query` `missing_table` `invalid_table_name` `missing_file` `empty_command` |
| Paths | `path_traversal_blocked` `forbidden_path` `root_scan_blocked` `secret_file_blocked` (FATAL); `path_not_found` `not_a_directory` `path_unreadable` `file_not_found` `file_too_large` `invalid_path` (RETRY) |
| `llm_exec` | `dangerous_command_blocked` `option_not_allowed` (FATAL); `command_not_allowed` `sandbox_unavailable` (STOP); `construct_not_allowed` `unterminated_quote` `glob_matches_option` (RETRY) |
| SQL guard | `write_operation_blocked` `multiple_statements_blocked` `client_command_blocked` `dot_command_blocked` (FATAL); `executable_comment_blocked` `backslash_in_literal` `unterminated_literal` (RETRY) |
| Database | `no_db_config` `no_database` `unknown_db_type` (STOP); `sqlite_error` `mysql_error` `pgsql_error` `sqlite_file_not_found` `missing_sqlite_file` (RETRY); `sqlite_not_found` `mysql_not_found` `psql_not_found` `invalid_sqlite_file` (FATAL) |
| Context | `not_a_git_repo` `git_not_found` `no_package_file` `ports_tool_not_found` (CONTINUE); `invalid_manifest` (RETRY); `jq_not_found` (FATAL) |
| `llm_docker_check` | `no_services` `docker_not_found` `compose_not_installed` (CONTINUE); `compose_not_found` (RETRY) |

Integer options take 0 to 999999999 (`invalid_<name>`: "Value 'X' is not a
non-negative integer below 1000000000"); `llm_tree --depth` takes 1 to
999999999 (`invalid_depth`: "Value 'X' is not an integer from 1 to
999999999").

The one exception: scanners without `--json` print human `Error:` /
`Hint:` lines.

Branch on `action`, not on the exit status alone. Never retry the same call
more than once; the same error twice means stop and ask.

## 6. Safety model

The guards are **defense in depth, not a sandbox**. They stop common mistakes
and keep an agent that only has these functions read-only. They do not
contain an agent that can already run arbitrary shell commands: it can bypass
every check below.

### 6.1 Paths

* `llm_*` path arguments: a `..` component is refused; `/proc`, `/sys`,
  `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/gshadow`, `/etc/passwd`,
  `/etc/sudoers`, `/etc/sudoers.d`, `/etc/shadow-`, `/etc/gshadow-`,
  `/etc/security/opasswd` and everything below them are refused, checked as
  written and after resolving symlinks. Root's home is refused also where it
  is not `/root` (macOS: `/var/root`).
* `llm_tree`, `llm_find` and `llm_grep` walks may not start at `/` or at a
  directory above a protected tree (`llm_grep` and `llm_find --contains`:
  also above an `/etc` entry, so not at `/etc`): `forbidden_path`, hint
  `Search a subdirectory`. Below their root they leave out every entry at or
  below a protected path, and they never follow symlinks.
* Scanners refuse `/proc`, `/sys`, `/dev`, `/boot`, `/root` as scan roots
  (the PATH argument only: a walk that follows `-L` or starts at `/` with
  `--allow-root` can still enter them), need `--allow-root` for `/`, and do
  not refuse `..`.
* `llm_exec` applies the same forbidden-path rules to every argument of
  every command (option values, `key=value` words, `~`, glob matches, and
  relative paths after a `cd` in the line), except `/dev/null`, the
  arguments of `echo`/`printf`/`basename`/`dirname` and the pattern operand
  of `grep`/`rg`/`sed`/`awk`/`jq`/`yq`. Not in `unrestricted` mode.
* `llm_exec` walks (`grep -r`, `rg`, `find`, `fd`, `tree`, `du`, `ls -R`,
  `diff -r`) may not start at `/` or at a directory above a protected tree
  (content searches: also above `/etc`), and may not follow symlinks
  (`grep -R`, `rg -L`, `find -L`/`-follow`/`-files0-from`, `fd -L`,
  `tree -l`, `du -L`, `ls -RL`, `--dereference-recursive`).
* Paths resolve as GNU `realpath -m` resolves them, on every platform.

### 6.2 Secret files

Matched case-insensitively on the base name, and on the target of a symlink
(the list is `internal/pathguard`'s table, checked by a test):

* Environment: `.env` `.env.*` `*.env` `.envrc`
* Keys and keystores: `*.pem` `*.key` `*.p12` `*.pfx` `*.jks` `*.keystore`
  `*.kdbx` `*.ppk` `id_rsa*` `id_dsa*` `id_ecdsa*` `id_ed25519*` `*_rsa`
  `*_dsa` `*_ecdsa` `*_ed25519` `ssh_host_*_key`
* Credentials: `credentials` `credentials.*` `credentials_*` `credentials-*`
  `secrets` `secrets.*` `.gash_env` `.pgpass` `.netrc` `_netrc`
  `.git-credentials` `.npmrc` `.pypirc` `.htpasswd` `.my.cnf` `.boto`
  `.s3cfg` `.dockercfg` `.vault-token` `kubeconfig` `*.kubeconfig`
  `*.tfstate` `*.tfstate.*` `*.tfvars` `*.tfvars.json`
* Anything under a directory `.ssh` `.gnupg` `.aws` `.azure` `.kube`
  `.docker` `.config/gcloud`
* Not secret (templates, which win over the above): `*.pub` `.env.example`
  `.env.sample` `.env.dist` `.env.template` `*.env.example`; `llm_grep`
  and `llm_find --contains` search them.

`llm_config` and `llm_exec` refuse them as arguments; `llm_grep`,
`llm_find --contains` and, inside `llm_exec`, `grep`, `rg`, `diff` and
`git diff`/`show`/`grep`/`log -p` skip them (`git log` with `--follow` or
`-L` does not; nor do commands named in `GASH_LLM_EXEC_ALLOW` or
`unrestricted` mode). `llm_exec` also refuses `git show REV:PATH` for a
secret PATH and git blobs named by id (`git show SHA`, `git cat-file -p
SHA`, `--batch`). Their names still appear in `llm_tree` and `llm_find`
name searches. Detection is by name only: a password inside `config.php` is
not.

### 6.3 `llm_exec`

1. **Blocklist** of dangerous patterns (`rm -rf /`, `dd if=`, `mkfs.`,
   shutdown, `curl ... | sh`, `history -c`, `.ssh/id_`, ...). A regex
   tripwire over the whole line, quoted text included (`echo 'shutdown'`
   is refused), easy to evade by rephrasing; it applies in every mode.
2. **Parser**: plain words joined by `|`, `&&`, `||`, `;` or newlines. No `$`
   or backtick expansion, no subshells or braces, no background jobs, no
   `VAR=value` prefixes, no redirections except `>/dev/null`, `2>&1`, `>&2`.
3. **Allowlist** of read-only commands (ls, cat, head, grep, rg, find, jq,
   diff, du, ps, ss, git, systemctl, journalctl, docker, ...) with rules for
   their writing options (`find -delete`/`-exec`, `sort -o`, `sed -i`,
   `git branch -D`, `git -c`, `docker rm`, `systemctl stop`, ...). php, node,
   python and similar run only with `--version`. sed and awk run as GNU
   `sed --sandbox` / `gawk --sandbox`, refused where those are missing.
   Arguments and glob matches that are secret files are refused, pattern
   operands included (`grep id_rsa` is refused like a file). Also
   refused: `ps e`, `getent shadow`/`gshadow`, `yq` `load()` and `-s`,
   `diff -r` without GNU `--no-dereference`, git `--literal-pathspecs`,
   `--no-index`, `cat-file --batch`, `stash show -p`, patch options on
   `stash list` (`-p`, `-U`, `--word-diff`, ...) and `remote show`
   without `-n` (it contacts the network).

The child Bash runs with `--noprofile --norc` and a short environment:
PATH, HOME, USER, LOGNAME, LANG, LANGUAGE, `LC_*`, TERM, TZ, TMPDIR,
NO_COLOR, XDG_RUNTIME_DIR, DOCKER_HOST, DOCKER_CONTEXT, PWD, plus
`GIT_OPTIONAL_LOCKS=0` and `GIT_TERMINAL_PROMPT=0` (in `unrestricted` mode:
the caller's environment, with BASH_ENV and ENV empty). No exported
functions or other variables reach it; it cannot change the
caller's directory or variables. There is **no timeout**: avoid `tail -f`,
`journalctl -f`, `docker logs -f`. Allowed commands can still expose data
(`docker inspect` shows container environments, `ps` shows command lines).

Git, in `llm_exec` and in `llm_git_*`, runs with `core.fsmonitor=false`,
`core.hooksPath=/dev/null`, no external diff and `core.pager=cat`, and diffs
with `--no-ext-diff --no-textconv`, so a cloned repository's configuration
cannot name a program to run. Clean and smudge filters named in
`.gitattributes` and configured in the repository can still run.

Settings, read from `~/.gash_env` only (or the file named by `GASH_ENV_FILE`),
never from same-named environment variables:

| Setting | Effect |
|---|---|
| `GASH_LLM_EXEC_ALLOW="make npm"` | Extra commands, any options; the secret-file, forbidden-path and walk rules (no symlink-following walk, no walk at `/` or above a protected tree) still apply |
| `GASH_LLM_EXEC=unrestricted` | Skips parser, allowlist and secret-file check; only the blocklist remains |
| `GASH_LLM_SANDBOX=1` | Runs the child in `bwrap`: read-only filesystem, no network (`sandbox_unavailable` if bwrap cannot start) |

Whoever controls the agent's environment (`HOME`, `GASH_ENV_FILE`) controls
which file is read. On `command_not_allowed`, ask the user; never edit
`~/.gash_env` yourself.

### 6.4 Databases

1. **SQL guard**, a lexer per engine (quotes, comments, PostgreSQL dollar
   quotes): one statement (a trailing `;` is fine); first keyword SELECT, WITH,
   VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN or PRAGMA; no write keywords or
   file, program or sleep functions outside literals and comments (INSERT,
   INTO, FOR UPDATE, ATTACH, LOAD_FILE, pg_read_file, pg_sleep, ...); no client
   commands (`\!`, sqlite dot-commands, MySQL `/*! */` comments).
2. **Read-only session**, through the built-in drivers (no `sqlite3`,
   `mysql` or `psql` client runs): SQLite opened read-only in defensive,
   query-only mode with untrusted schemas; MySQL/MariaDB
   `SET SESSION TRANSACTION READ ONLY` plus a sql_select_limit row cap;
   PostgreSQL with default_transaction_read_only=on and a statement timeout
   of `GASH_DB_TIMEOUT` seconds (default 30). MySQL and SQLite have no
   timeout.

No program runs, so neither passwords nor the SQL appear on a command line.
Table names for `llm_db_schema` and `llm_db_sample` must match
`[A-Za-z_][A-Za-z0-9_]*`; database names from `-d` or `.target-database`
must match `[A-Za-z0-9_][A-Za-z0-9_$.-]*`, 64 characters at most
(`invalid_database_name`). Read-only is not harmless: a heavy SELECT still
loads the server, and the database user decides what is readable. Prefer a
read-only database user in `~/.gash_env`.

### 6.5 AI functions

`ai_query` and `ai_sysinfo` send text to Claude or Gemini after redacting
private key blocks, URL passwords, `password=`/`token:`-style values and known
token formats (sk-, ghp_, AKIA, xox, AIza). Redaction is pattern-based and
can miss secrets. `GASH_AI_CONFIRM`: auto (default) asks before sending data
Gash collected itself (`ai_sysinfo`), 1 asks before every request, 0 never
asks. In headless mode or without a terminal, a request that needs
confirmation is refused. Call
AI functions only when the user asks for them.

Headless mode reads `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`,
`GASH_AI_MODEL_CLAUDE` and `GASH_AI_MODEL_GEMINI` from `~/.gash_env` (the
environment wins) and prints no configuration warnings unless `GASH_DEBUG=1`.

## 7. Best practices

* Use `--json` and parse with `jq`; text output is for people.
* Use `--null` with `xargs -0` for paths; never split paths on newlines.
* Prefer the dedicated function to `llm_exec`: `llm_grep` over grep,
  `llm_git_status` over git status.
* Bound big scans with `--depth N`, `--xdev`, `-n N`, `--exclude GLOB`.
* Pass the SQL statement as one quoted argument; keep user input out of
  string literals.
* Empty results are success (`[]`, `"data": []`, or no lines, exit 0), also
  under `set -euo pipefail`. Read stderr only when the exit status is non-zero.

```bash
out=$(llm_db_query "SELECT id FROM users" -c default 2>err.json) || {
  case "$(jq -r .action err.json)" in
    RETRY)    : fix the arguments, retry once ;;
    CONTINUE) : skip this step ;;
    *)        jq -r '.hint // .error' err.json ;;   # STOP or FATAL: tell the user
  esac
}
```

## 8. Drop-in snippets

### 8.1 Claude Code (`CLAUDE.md`)

````markdown
## Gash (2.0+)

Run: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'`

- Code: `llm_tree --depth 2 --stats`, `llm_find '*.php' --contains REGEX`, `llm_grep PATTERN --ext php,js`
- Disk: `files_largest --json`, `dirs_largest --json`, `tree_stats --json`
- DB (read-only): `llm_db_tables -c CONN`, `llm_db_schema TABLE -c CONN`, `llm_db_query "SELECT ..." -c CONN`
- Git: `llm_git_status`, `llm_git_log -n 10`, `llm_git_diff --staged`
- Commands: `llm_exec "CMD"` runs allowlisted read-only commands only

Errors: stderr JSON `{error, action, recoverable, details?, hint?}`. RETRY: fix, retry once.
CONTINUE: skip. STOP: ask the user. FATAL: show the hint, stop; never rephrase to get around it.
Never edit ~/.gash_env. Reference: ~/.gash/AGENTS.md
````

### 8.2 Codex, Cursor and other `AGENTS.md` readers

````markdown
## Tooling: Gash 2.0+

Invoke: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <fn> [args]'`
- Filesystem (`--json`): files_largest, dirs_largest, dirs_find_large, dirs_list_empty, tree_stats
- Code: llm_tree, llm_find, llm_grep, llm_config
- Database, read-only (`-c CONN` or `-f SQLITE_FILE`): llm_db_query, llm_db_tables, llm_db_schema, llm_db_sample, llm_db_explain
- Git: llm_git_status, llm_git_diff, llm_git_log
- System and project: llm_env, llm_ports, llm_procs, llm_project, llm_deps
- Command lines: llm_exec (read-only allowlist; ask the user when a command is refused)

Errors: stderr JSON with `action` RETRY | CONTINUE | STOP | FATAL; retry at most once.
Reference: ~/.gash/AGENTS.md
````

### 8.3 Generic system prompt

```text
You can use Gash, a Bash toolkit with JSON output, through:
  GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'
Functions: llm_tree, llm_find, llm_grep, llm_config, llm_git_status, llm_git_log,
llm_git_diff, llm_db_query, llm_db_tables, llm_db_schema, llm_db_sample,
llm_db_explain, llm_env, llm_ports, llm_procs, llm_project, llm_deps, llm_exec,
files_largest, dirs_largest, dirs_find_large, dirs_list_empty, tree_stats.
Use --json where offered. Errors arrive on stderr as JSON with "action":
RETRY (fix input, retry once), CONTINUE (skip), STOP (ask the user),
FATAL (report the hint, stop). Do not try to get around refusals.
```

## 9. Version compatibility

Version: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; echo "$GASH_VERSION"'`.
Changes in 2.1 that affect agents:

* **New error codes** (RETRY): `invalid_database_name`, `invalid_type`,
  `file_too_large`.
* **Stricter arguments**: a second PATH fails with `unexpected_argument`;
  `llm_git_status`, `llm_project` and `llm_config` reject unknown options;
  `llm_find` needs a pattern; `--help` works anywhere on the line.
* **`llm_exec`** runs with a minimal environment and refuses more (6.3).
* **`llm_find --contains`** honours PATTERN; `llm_procs` prints one line.

Changes from 1.x that affect agents (the full list for people, with upgrade
steps, is in `docs/migration.md`):

* **`llm_exec`**: 1.x evaluated the line in the caller's shell behind a
  blocklist; 2.0 uses a child Bash, the parser and the allowlist of 6.3.
  Other commands fail with `command_not_allowed` (STOP).
* **`recoverable` follows `action`**: `false` only for FATAL (in 1.x some
  CONTINUE errors, such as `not_a_git_repo`, said `false`).
* **`llm_git_status`** has a conflicted array (unmerged paths).
* **`llm_tree`** sets `"truncated": true` at 500 entries; `--stats` sizes and
  counts are real totals past the depth limit.
* **`-n` means `--limit`** (`files_largest`, `dirs_largest`, `dirs_find_large`,
  `llm_find`, `llm_grep`, `llm_git_log`, `llm_db_query`, `llm_db_sample`);
  `docker_compose_upgrade -n` is
  refused (use `--dry-run`). `--json` replaces `--llm` on `php_detect_version`
  and `node_detect_version` (`--llm` still works).
* **Empty envelopes**: `llm_db_*` return `{"data": [], "rows": 0, ...}` for no
  rows (1.x printed nothing).
* **Stricter arguments**: unknown options fail (`unknown_option`), flags
  without values fail (`missing_value`), and `llm_db_*` and `llm_exec` reject
  extra arguments (`unexpected_argument`).
* **New error codes**: `command_not_allowed`, `option_not_allowed`,
  `construct_not_allowed`, `unterminated_quote`, `glob_matches_option`,
  `sandbox_unavailable`, `client_command_blocked`, `dot_command_blocked`,
  `executable_comment_blocked`, `backslash_in_literal`, `unterminated_literal`,
  `invalid_filter`, `missing_value`, `unexpected_argument`,
  `ports_tool_not_found`, `docker_not_found`, `compose_not_installed`.
* **Consistent errors**: `llm_git_diff` and `llm_git_log` return
  `not_a_git_repo` outside a repository (1.x: no output, or `[]`);
  `llm_ports` returns `ports_tool_not_found` instead of `[]`;
  `llm_docker_check` errors moved from stdout to the standard stderr
  envelope (the path is in `details`); `llm_procs` rejects unknown options.
* **`llm_exec` path rules**: arguments under `/proc`, `/sys`, `/dev`, `/root`
  and the other forbidden paths fail with `forbidden_path`.
* **Stricter SQL**: queries 1.x accepted (`SELECT ... INTO`, `FOR UPDATE`,
  sleep functions) now fail with `write_operation_blocked`.

Internal helpers (`__gash_*`, `__llm_*`) are not part of the API and change
without notice.
