# AGENTS.md: Gash for LLM coding agents

> **Audience:** coding agents that run Bash (Claude Code, Codex, Cursor, Gemini,
> Aider, local models) and the people who configure them.
> **Applies to:** Gash 3.x. Gash 3.0 runs the toolkit in one Go binary
> behind the same Bash entry point (`source ~/.gash/gash.sh`): invocations,
> function names, flags, JSON shapes and error codes are those of 2.x, with
> the 3.0 changes listed in [section 13](#13-version-compatibility).
> **Stable API:** the public function names and flags below, the JSON shapes
> in section 4, the error contract in section 5 and the variables in section 2.

## 1. Quick start

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status'
```

`GASH_DIR` is derived from the location of `gash.sh`, so a checkout anywhere
works the same way (`source /path/to/gash/gash.sh`). Gash needs Bash 4.4+.
The JSON comes from the `gash` binary itself: the `llm_*`, database and
scanner functions need no `jq`, `sqlite3`, `mysql`, `psql` or `rg` (use `jq`
on your side to read the output, if you like). Help for any function:
`llm_tree --help` or `gash_help llm_tree`. `gash version --json` prints
`{"version", "os", "arch", "path"}` of the binary in use.

## 2. Invocation modes

| Variable | Effect |
|---|---|
| `GASH_HEADLESS=1` | Recommended for agents. Loads core and function modules only, prints nothing while loading, no ANSI colors. |
| `NO_COLOR=1` / `GASH_NO_COLOR=1` | No ANSI colors; everything else as in an interactive shell. |
| `GASH_DEBUG=1` | With headless: show the stderr of files that fail to load. |

Headless mode does **not** load the prompt, history settings, banner, the
alias pack, `~/.bash_aliases` or `~/.bash_local`. Module short aliases exist,
but Bash does not expand aliases in non-interactive shells: use full function
names. Sourcing in headless mode never prints, whatever the configuration
(a Gash 1.x one included: its reminder is for interactive shells only).

Without headless mode, `gash.sh` does nothing in a non-interactive shell. Do
not use `bash -i` and do not source `~/.gashrc` (the user's entry point).
`gash_doctor` lists files that failed to load. Without the color variables,
Gash still drops colors for most output when stdout is not a terminal; set
one of them if you parse text.

**Direct invocation, no sourcing.** Every `llm_*` and `agent_*` function
also runs as `gash <fn> args` (the binary on PATH, no `gash.sh`, no Bash:
a few ms instead of ~100 ms per call). Given the same working directory
and the same exported environment, stdout, stderr (error envelopes
included) and the exit status are those of the sourced function, byte for
byte (a test compares them); only a shell variable that is not exported
reaches the sourced function alone. `gash agent-functions --json` lists
these functions for a wrapper that routes calls directly (3.11).

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
| `llm_tree [--depth N] [--stats] [--text] [--all] [--count [--by ext\|dir\|type]] [--top N] [--type f\|d] [--name GLOB] [--newer REF] [--since T] [--relative\|--absolute] [--format F] [--width N] [--max-lines N] [--head N] [--tail N] [PATH]` | JSON tree (default depth 1); `--stats` adds sizes and child counts; `--text` indented list (symlinks as `name -> target`), 200 lines max; `--count` `{"count":N}`; `--top N` disk usage `{"total","files","top":[{"p","size","files"}],"skipped"}` |
| `llm_find PATTERN [PATH] [--type f\|d] [--contains REGEX] [-n N] [--sort name\|mtime\|size] [--newer REF] [--since T] [--tracked] [--all] [--count [--by ext\|dir\|type]] [--relative\|--absolute] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | Paths, one per line (default 100); PATTERN is a name glob, `--contains` keeps the matching files whose content matches; `--format` rows `{"p","size","mtime"}` |
| `llm_grep PATTERN [PATH] [--ext a,b] [--context N] [-n N] [--exclude GLOB]... [--only-matching] [--distinct] [--files-only] [--count [--by file\|ext\|dir]] [--relative\|--absolute] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `file:line:content` lines (default 100), absolute paths; a FILE PATH gives `line:content`; context lines use `-`, groups are separated by `--`; `--distinct` `[{"match","count"}]`; `--format` rows `{"file","line","text"}` |
| `llm_config FILE [--max-bytes N] [--path P]` | File content; YAML converted to JSON by Gash itself (no `yq` needed), the raw text when it does not parse; files over 1 MiB (default) fail with `file_too_large`; `--path P` (3.4) parses YAML, TOML, INI or JSON and prints the value at P as compact JSON (see 3.8) |

All three search functions skip node_modules, vendor and .git (`llm_tree` also
\_\_pycache\_\_ and .cache, `llm_find` also \_\_pycache\_\_). `llm_grep` and
`llm_find --contains` share one built-in engine, the same whether ripgrep is
installed or not: ripgrep's regex syntax, binary files skipped, files in
sorted path order; `llm_grep` also skips hidden files and what
`.gitignore`/`.ignore`/`.rgignore` exclude, `llm_find --contains` searches
them. `llm_find` names and `llm_tree` entries come in the same byte order
(C order: each directory's entries sorted by name), so `-n N` and the JSON
tree's 500-entry cap cut at the same place on every run. Zero matches: exit
0, no output; a PATH that does not exist is `path_not_found`, an empty
PATTERN `missing_pattern`.

3.4 modes (each opt-in; without its flag the output is unchanged). Paths
stay absolute unless `--relative` (paths below PATH; `.` for PATH itself),
except the new `--top` rows, relative unless `--absolute`. `--count` prints
`{"count":N}`, with `--by` (which implies it) `{"count":N,"by":{KEY:N}}`:
`llm_find --count` counts the paths its listing prints without `-n`;
`llm_tree --count` the entries below PATH at any depth (to `--depth N` when
given); `llm_grep --count` matching lines (matches with `--only-matching`,
`-o`; files with `--files-only`, `-l`). `--by ext` keys are tree_stats'
extensions (`(no ext)`), `--by dir` the first directory below PATH (`.` for
entries directly in it), `--by type` the letters `f`, `d`, `l` (every type
unless `--type`), `--by file` the file. `llm_find --sort mtime|size` puts the
newest or largest first, `--sort name` the paths in byte order;
`--newer REF` keeps what was modified after REF, `--since T` what was
modified at T or later (an age `30s` `15m` `2h` `3d` `1w`, a date, a local
`2026-10-25T14:30`, RFC 3339 or `@EPOCH`; else `invalid_since`); `--tracked`
keeps what `git ls-files` lists (`not_a_git_repo`, CONTINUE, outside a
repository); `--all` drops the noise-directory skip. `llm_grep --exclude
GLOB` (repeatable) skips the entries whose name or path below PATH matches,
as the scanners' `--exclude`; `--distinct` (implies `--only-matching`) lists
each distinct match with its count, most frequent first, at most `-n` rows.
`llm_tree --top N` sums the apparent size (`st_size`) of every file below
PATH, noise directories included; `--by dir` (default) lists every
directory down to `--depth` (default 1) with its cumulative size as `du -d`
does, `--by ext` the extensions; unreadable directories count in `skipped`,
a cut list adds `"truncated":K`. `--count`, `--sort`, `--distinct`,
`--format` and `--head`/`--tail`/`--max-lines` see every match before they
print: the budget flags then replace `-n`, and what they cut is reported
(section 3.6). Row and count `mtime` values are RFC 3339 in UTC. `--context`
with a row, count or match mode, `--files-only` with `--only-matching`, and
the `llm_tree` filters without `--count`/`--top` are `conflicting_options`;
a bad `--sort` or `--by` value is `invalid_sort` or `invalid_by` (RETRY).

### 3.2 Filesystem scanners

| Function | Notes |
|---|---|
| `files_largest [PATH]` | Largest files; `-n N` (default 100), `--min-size SIZE`, `--depth N` |
| `dirs_largest [PATH]` | Largest directories by cumulative size; `--depth N` (default 1), `-n N` |
| `dirs_find_large [PATH]` | Directories over `--size SIZE` (default 20M); `-n N` (default 1000), `--with-mtime` |
| `dirs_list_empty [PATH]` | Empty directories; `--count`, `--min-depth N`, `--ignore-dotfiles` |
| `tree_stats [PATH]` | Totals, top extensions (`--top N`), depth, empty dirs |

Common flags (each `--help` lists its own set): `--json`, `--null`/`-0`
(NUL-separated paths), `--exclude GLOB` (repeatable; skips entries whose name
or path below PATH matches; a newline in GLOB is `invalid_filter`),
`--no-ignore`, `--xdev`, `--allow-root` (required to scan `/`),
`--human`/`--no-human`, `--no-color`. Sizes are bytes, or a number with up to
6 decimals and K, M, G, T, P or E, optionally followed by B, i or iB (powers
of 1024). Paths containing a tab or a newline are left out of the results.

### 3.3 Databases (read-only)

```
llm_db_query   "SQL"  [-c CONN | -f SQLITE_FILE] [-d DB] [-n ROWS] [--format F] [--scalar]  # default 100 rows
llm_db_tables         [-c CONN | -f SQLITE_FILE] [-d DB] [--counts] [--format F] [--scalar]
llm_db_schema  TABLE|--all [-c CONN | -f SQLITE_FILE] [-d DB] [--format F] [--scalar]
llm_db_sample  TABLE  [-c CONN | -f SQLITE_FILE] [-d DB] [-n N] [--format F] [--scalar]      # default 5
llm_db_explain "SQL"  [-c CONN | -f SQLITE_FILE] [-d DB] [--analyze] [--format F] [--scalar]
llm_db_databases      [-c CONN | -f SQLITE_FILE] [-d DB] [--format F] [--scalar]
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

Output options (3.4): `--format json` is the envelope above, byte for byte
(the default). `--format jsonl`, `tsv` or `table` print the rows themselves
(3.6), with SQL NULL as `null`, `\N` or an empty cell on every engine
(MySQL/MariaDB included; the envelope keeps its `"NULL"` string), REALs in
their shortest form and PostgreSQL columns in the query's order; the
metadata `{"query_time","rows"}` is one JSON line on stderr, plus
`"truncated":true` when the `-n` cap cut rows (the count beyond it is
unknown) and `slow_query_explain` for a slow query. `--scalar` prints the
value of a 1x1 result as a TSV cell (`\N` for NULL), nothing on stderr;
any other shape is `not_a_scalar` (RETRY) with the shape in `details`
(`3 rows x 2 columns`, `100+ rows x 1 columns`). `llm_db_tables --counts`
returns `[{name, rows}]`: exact on SQLite (`count(*)`), the engine's
estimate on MySQL/MariaDB (`information_schema.tables`) and PostgreSQL
(`pg_class.reltuples`), `null` where there is none (a view, a table
PostgreSQL has not analyzed). `llm_db_schema --all` returns the schema rows
of every table `llm_db_tables` lists in one call, each led by a `table` key,
sorted by table and column position. `llm_db_databases` lists the
databases (`{"data":[names]}`, sorted; system schemas included; `main` for
SQLite) and needs no database in the connection.

### 3.4 Git, system, project

| Function | Output |
|---|---|
| `llm_git_status [PATH] [--summary] [--files] [--fetch] [--repos PATH...] [-j N] [--timeout S] [--format F] [--max-lines N] [--head N] [--tail N] [--width N]` | JSON object (4.6); `--summary` the counts per state; `--files` rows `{p, xy}` (the `git status --short` letters); `--repos` one summary row per repository (`repo` first, `error` last), in parallel; `--fetch` fetches the upstream branch only and prints the `llm_git_overview` object plus `fetched` |
| `llm_git_log [-n N] [--since D] [--until D] [--grep RE] [--author RE] [--path P] [--fields LIST] [--format F] [--max-lines N] [--head N] [--tail N] [--width N] [PATH]` | JSON array of `{hash, subject, author, date}` (default 10); `--fields` picks among hash, subject, date, author, files (the changed paths) |
| `llm_git_diff [--staged] [REV..REV] [--files] [--numstat] [--summary] [--patch] [--format F] [--max-lines N] [--head N] [--tail N] [--width N] [PATH]` | `git diff --stat` text; `--files` rows `{p, add, del, status}`, `--numstat` rows `{p, add, del}`, `--summary` `{files, insertions, deletions}`, `--patch` the patch text capped per file |
| `llm_git_overview [PATH] [--log N] [--files N]` | `{branch, upstream, ahead, behind, counts:{staged, modified, untracked, conflicted}, log:[{hash, subject}], diffstat:{files, insertions, deletions}}` (3 subjects by default); `--files N` adds the path lists, N per state |
| `llm_git_show [REV] [--stat] [--files] [--patch] [--max-lines N] [--head N] [--tail N] [--width N] [PATH]` | `{hash, subject, author, date, files:[{p, add, del}]}` of REV (default HEAD); `--stat` adds `stat` totals, `--files` each file's `status`; `--patch` the patch text capped per file; `REV:FILE` the file's content at REV |
| `llm_env [--filter REGEX]` | JSON object; secret-looking names left out (PASSWORD, TOKEN, KEY, SECRET, SESSION, JWT, WEBHOOK, `*_PWD`, `*_PAT`, ...), URL passwords (anywhere in a value, up to the last `@` of the URL) and known token formats in values shown as `***` |
| `llm_ports [--listen] [--port N]` | `[{port, proto, state, address}]`, one record per socket, sorted by port then proto; needs `ss` or `netstat` (`ports_tool_not_found` without); `--port N` (0-65535, else `invalid_port`) keeps local port N and adds `procs: [{pid, name}]` to each record (empty for another user's process unless root) |
| `llm_procs [--name NAME \| --port PORT \| --pid PID...] [--alive] [--tree] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{pid, name, ...}]`; no filter: first 20 of `ps aux`; `--name` matches full command lines; `--port` (0-65535, else `invalid_port`) wins over `--name`, `--pid` over both. With `--pid`, `--alive` or `--tree` (3.4): `[{pid, name, cmd, alive}]` (`--tree` adds `ppid`, `depth`), never the caller's own chain (3.10) |
| `llm_project [PATH]` | `{type, framework, package_manager, has_tests, entry_point, path}`; a missing PATH is `path_not_found` |
| `llm_deps [PATH] [--dev]` | `{manager, dependencies}`, or `{manager, prod, dev}` with `--dev`; names sorted; a `composer.json`/`package.json` that is not one JSON object is `invalid_manifest` |
| `llm_docker_check [PATH]` | Compose image update check as JSON (contacts the registries; Compose applies override files and `COMPOSE_FILE`); an unknown digest is `null`; an `ERROR` service's `error` is one of `dns_error` `connection_refused` `timeout` `ssl_error` `transport_error` `unauthorized` `forbidden` `not_found` `rate_limited` `server_error` `http_NNN` `malformed_token` `no_digest_in_response` `registry_unreachable` `invalid_reference`. PATH follows the path rules (6.1); `${VAR}` is resolved from `PATH/.env` and a short allowlist (`HOME`, `USER`, `PATH`, `LANG`, `DOCKER_*`, ...), never from other exported variables |
| `php_detect_version [DIR] [--json]` | `{version, php_raw_binary, php_command, ...}`; JSON by default in headless |
| `node_detect_version [DIR] [--json]` | Same for Node (needs nvm) |

Git (3.4 options): one call answers the usual combination: `llm_git_overview`
for `git status --short | wc -l; git log --oneline -3; git diff --stat | tail -1`,
`llm_git_status --summary`/`--files`/`--repos`/`--fetch`, `llm_git_diff
--summary`/`--files`/`--patch`, `llm_git_show REV --stat` and `REV:FILE`. Without
a 3.4 option every existing function prints what it printed before. Paths
are relative to the repository root, as git prints them; `add`/`del` are null
for a binary file. `--patch` keeps each file's header lines and cuts its body
at `--max-lines` (default 200; `REV:FILE` content 2000) with
`[... K lines omitted]`; the patch of a secret file (6.2) is replaced by one
marker line and `REV:FILE` of a secret file is `secret_file_blocked`. A FILE
as PATH limits `llm_git_diff` (with a 3.4 option) and `llm_git_show` to it.
`--fetch` is the one network read: the upstream branch only (no tags, no
prune, no submodules), bounded by `--timeout` (default 20 s), never
prompting; a failure is `"fetched":false` with `error` and the local state is
still printed, and a repository whose own configuration names a program for
the fetch (`core.sshCommand`, a credential helper, `uploadpack`,
`url.*.insteadOf`, `protocol.*`) is not fetched. An unknown revision is
`unknown_revision`, a bad `--grep`/`--author` regex `invalid_regex`, options
that do not go together `conflicting_options` (all RETRY).

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

### 3.6 Shared output options (3.4)

The 3.4 agent functions share one output layer; each function's `--help`
lists the options it takes. The defaults never change: a function prints
what it printed before unless you pass one of these.

| Option | Effect |
|---|---|
| `--format json\|jsonl\|tsv\|table` | Row output as one compact JSON array (`json`), one JSON object per line (`jsonl`), a header line plus tab-separated rows (`tsv`: tab, newline, carriage return and backslash in a value are written `\t`, `\n`, `\r`, `\\`; a null is `\N`), or aligned columns for humans (`table`). An unknown format is `invalid_format` |
| `--width N` | Cuts each output line (in JSON, each string value) at N display columns, ending it with `…` |
| `--max-lines N`, `--head N`, `--tail N` | Keeps the first N (`--max-lines` is `--head`) and/or the last N lines or rows; what is cut becomes one `[... K lines omitted]` line in text and `table` output, `"truncated": K` in a JSON object, and the metadata line below for a JSON array, `jsonl` and `tsv` |
| `-j N`, `--timeout S` | Multi-target functions: N targets at a time (default: the CPU count, 32 for network probes) and a per-target time limit (seconds, `0.5`, or `500ms`, `2s`, `1m`; at most 1h). A target that times out is a field of its result, not a failed call; results keep the input order |
| `--relative`, `--absolute` | Paths relative to the PATH argument (or the working directory), or absolute (today's default) |

Values are positive integers (1 to 999999999): 0 or a word is
`invalid_width`, `invalid_max_lines`, `invalid_head`, `invalid_tail` or
`invalid_jobs`, a bad duration `invalid_timeout` (all RETRY). Stdout carries
data only: the metadata of a row output (`rows`, `truncated`, timings) is
one JSON line on stderr with no `error` key, so it is never an error
envelope. Nothing is ever cut without a marker.

`GASH_TELEMETRY_FILE=/path` (exported, off by default) appends one JSON line
per `llm_*`/`agent_*` call to that local file:
`{"ts","fn","ms","out_bytes","rc","truncated"}`. Never the arguments or any
value, never the network; a write failure never fails the call.

### 3.7 Text reads (3.4)

| Function | Output |
|---|---|
| `llm_stat PATH... [--lines] [--follow] [--format F] [-j N] [--timeout S] [--absolute]` | TSV by default (`--format json`: `[{p, t, size, mode, owner, mtime}]`), one row per PATH in input order; `t` is `f` `d` `l` `p` `s` `c` `b` or `missing`; `lines` with `--lines`, `target` for symlinks and with `--follow`, `error` when a row has one |
| `llm_count FILE... --pat P... [--regex] [--lines \| --occurrences] [--format F] [-j N] [--timeout S] [--absolute]` | `{"FILE":{"P":N}}`, files and patterns in input order; lines that hold P (as `grep -c`), or every occurrence; `--format tsv`: `file pat count` rows |
| `llm_assert FILE... (--has P \| --lacks P \| --count P=N)... [--regex] [--lines \| --occurrences] [-j N] [--timeout S] [--absolute]` | `{"pass":true,"checked":N}` and exit 0, or `{"pass":false,"checked":N,"failed":[{file, check, found}]}` and exit 1 |
| `llm_read PATH[:A-B]... [--lines A:B] [--around RE -C N] [--max-bytes N] [--head N] [--tail N] [--max-lines N] [--width N] [--format F] [--absolute]` | Blocks `==> FILE:A-B <==` of numbered lines (`N:text`, `N-text` for context), or rows `{file, line, text}` |
| `llm_log FILE [--tail N] [--last-match RE -n N] [--grep RE] [--since T] [--level L] [--width N] [--format F]` | The last N (default 50) selected lines, as they are, in file order; rows `{line, ts, level, text}` with `--format` |
| `llm_hash PATH... [--algo sha256\|md5] [--short N] [--format F] [-j N] [--timeout S] [--absolute]` | `[{p, sha256, size}]` (`md5` with `--algo md5`), computed in parallel, input order |
| `llm_textstats FILE... [--format F] [-j N] [--timeout S] [--absolute]` | `[{p, lines, words, chars, bytes, gzip, max_line}]` |
| `llm_textcheck FILE... [--rules non-ascii,crlf,bom,trailing-ws,tabs,long-lines=N,final-newline,mixed-indent] [--max-lines N] [--format F] [-j N] [--timeout S] [--absolute]` | `{"files":N,"issues":[{file, line, rule, sample}]}`; `counts` by rule when there are issues, `truncated` and `skipped` when they apply |

Paths print as given (`--absolute` for the absolute form). Every PATH goes
through the path rules of 6.1 before anything is read: a refusal stops the
whole call. A file that is missing, not a regular file or out of
`--timeout` is a value of its row, not a failed call: `t: "missing"`
(`llm_stat`), `{"error":"file_not_found"}` (`llm_count`, `llm_hash`,
`llm_textstats`, `llm_assert`'s failed list), `skipped` (`llm_textcheck`);
`llm_read` and `llm_log` fail with `file_not_found`. Secret files (6.2):
`llm_read`, `llm_log`, `llm_count`, `llm_assert` and `llm_textcheck` refuse
them (`secret_file_blocked`, FATAL, as `llm_config`); `llm_stat` gives their
metadata but not their lines, `llm_textstats` an `error` row; `llm_hash`
hashes them (a digest shows no content). Binary files (a NUL byte in the
first 8000 bytes) are `binary_file` for `llm_read` and `llm_log`, an
`error` row for `llm_textstats`, `skipped` for `llm_textcheck`. Lines count
as an editor counts them: a last line without a newline is a line.
Patterns of `--regex`, `--around`, `--grep` and `--last-match` use
ripgrep's syntax, as `llm_grep`; a pattern never matches across lines.
`llm_count` and `llm_assert` read each file once for all patterns (a large
file in parallel parts); `llm_log` reads the file backwards in 64 KiB
blocks, so its cost is the size of the answer. `llm_log --since` takes a
duration back from now (`30s`, `10m`, `2h`, `1d`, `1w`) or an ISO 8601 date
or time, and reads the timestamps of ISO 8601 (also `[bracketed]`, Laravel
and Monolog), syslog, nginx and Apache access and error logs and JSON lines;
`--since` and `--level` keep whole entries (a line with a timestamp or a
level and the lines below it with neither, such as a stack trace). New
codes, all RETRY: `invalid_range`, `invalid_assertion`, `invalid_algo`,
`invalid_short`, `invalid_level`, `invalid_rule`.

### 3.8 Data files and values (3.4)

| Function | Output |
|---|---|
| `llm_json FILE\|- [--path P] [--keys \| --shape] [--format json\|jsonl\|tsv\|table] [--width N] [--head N] [--tail N]` | The value at P (default: the whole document) as compact JSON, numbers and escapes as written; `--keys` the keys of an object (or of the objects of an array); `--shape` types and lengths two levels deep; an array of objects as rows with `--format`; `-` reads stdin |
| `llm_csv FILE [--head N] [--cols] [--count] [--delim C] [--where COL=V] [--format json\|jsonl\|tsv\|table]` | `{"rows","matches","cols","header","sample","delimiter","errors","error_count"}` (`rows` and `error_count` with `--count`, `matches` with `--where`); `--head` sample rows (default 5, 0 for none); the sample as rows with `--format`, the rest on the stderr metadata line |
| `llm_dotenv FILE [KEY...] [--present]` | `{"KEY":"value","DB_PASSWORD":"***(set)","MISSING":null}` in argument order; `--present` gives `true`/`false`; without KEY the names the file sets (never a value) |
| `llm_date [--diff A B \| --add DATE DURATION \| --epoch X] [--tz ZONE] [--fmt FORMAT]` | `{"result":"...","epoch":N}` (now without a mode); `--diff` gives `{"result":"67d","days":67,"seconds":N}` (B minus A) |

Paths use one grammar (`internal/jsonpath`): `a.b[0].c`, `[-1]` from the
end, `paths["/api/x"]` for a key with dots or slashes, an optional leading
`$`. A path that does not exist is `json_path_not_found` (RETRY) with the
nearest existing prefix and what is there in the hint; invalid JSON is
`invalid_json` (RETRY) with line and column, checked over the whole text
before any value prints; a repeated key keeps the last value. `--shape`
lengths are `N` or `MIN..MAX`, mixed types join with `|`, members missing
from some objects are listed in `optional`. `llm_csv` reads quoted fields
with delimiters, `""` and newlines (a record is not a line), CRLF or LF,
skips blank lines and a UTF-8 BOM, sniffs the delimiter among `,` `;` tab
`|`, and lists the first 10 malformed records (field count differing from
the header, a stray or unterminated quote) by their first line; without
`--count` the read stops once the header and the sample are complete.
`--where` matches a column (header name or 1-based number) exactly; an
unknown one is `unknown_column` (RETRY). Secret files are refused as by
`llm_config`.

`llm_config FILE --path P` (row in 3.1) takes the same path grammar and
parses the file by its extension (`.yaml`/`.yml`, `.toml`, `.ini`/`.cfg`,
`.json`; any other: the first of JSON, TOML, INI and YAML that parses):
TOML keeps its key order and gives dates as RFC 3339 text; INI is read as
Python's `configparser.ConfigParser()` reads it (`{section: {key: value}}`,
keys lowercased, `DEFAULT` first when it has keys and inherited by every
section, `%(key)s` and `%%` expanded, continuation lines joined with
newlines; a repeated section or key merges instead of failing); a YAML
file with several documents is the array of its documents. `--path .` is
the whole file. A file that does not parse is `invalid_config` (RETRY, the
format and line in `details`). Without `--path` the output is unchanged.

`llm_dotenv` is the one reader of dotenv files (`.env`, `.env.*`, `*.env`,
`.envrc`; key and credential files and anything under `.ssh`, `.aws`, ...
stay `secret_file_blocked`). A key whose name `llm_env` hides (PASSWORD,
TOKEN, KEY, SECRET, AUTH, ...) shows `"***(set)"`, or `""` when empty; any
other value passes `llm_env`'s redaction (URL passwords, private key
blocks, known token formats). Values are read as python-dotenv reads them
(`export`, quotes, multi-line quoted values, `#` comments; the last of a
repeated key) except that `${VAR}` is never expanded. A KEY that is not a
variable name is `invalid_env_name`.

`llm_date` dates: `now`, `today`, `yesterday`, `tomorrow`, `YYYY-MM-DD`,
`YYYY-MM-DD[T ]HH:MM[:SS][.frac][Z|+HH:MM]`, RFC 2822, `@EPOCH`; one
without an offset is in `--tz` (an IANA name, `UTC` or `+HH:MM`; default
`TZ`, else the system zone). DURATION is number+unit pairs (`y`, `mo`, `w`,
`d`, `h`, `m`, `s`, `ms`; `-600s`, `1h30m`, `1y2mo`): months clamp to the
month's last day, days follow the calendar across DST, a date plus
calendar units stays a date. `--epoch` reads a number (13 digits:
milliseconds) as a Unix time and anything else as a date. `--fmt` is
`rfc3339` (default), `date`, `time`, `epoch`, `rfc2822` or a date(1)
format (`%Y-%m-%d %H:%M:%S`, also `%-d`, `%N`, `%:z`). The result keeps the
input's offset unless `--tz` is given. Bad input is `invalid_date`,
`invalid_duration`, `invalid_timezone` or `invalid_date_format` (RETRY).

### 3.9 Network probes (3.4)

| Function | Output |
|---|---|
| `llm_http URL... [--method M] [--header H]... [--auth-env VAR]... [--resolve HOST:IP]... [--follow] [--status-only] [--headers h1,h2] [--json-path P] [--grep RE] [--body-max N] [--expect CODE] [--insecure] [-j N] [--timeout S] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{url, status, ms, timing, bytes, redirects, ok, error}]`, plus `headers` (`--headers`), `value` (`--json-path`), `match` and `matches` (`--grep`); `--status-only`: `[{url, status, ms, ok, error}]` |
| `llm_dns NAME... [--type A,AAAA,MX,TXT,NS,CAA,CNAME,SOA] [--server IP] [-j N] [--timeout S] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{name, <one key per type>, ms, error}]` (default types A,AAAA) |
| `llm_tls HOST[:PORT]... [--sni NAME] [-j N] [--timeout S] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{host, protocol, subject, issuer, san, not_after, days_left, verified, verify_error, chain, ms, error}]` |
| `llm_tcp HOST:PORT... [-j N] [--timeout S] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{target, open, ms, error}]` |

The four probes only read (plan D4) and share the rules of 3.6: targets
are probed in parallel (`-j`, default 32) with a per-target `--timeout`
(llm_http and llm_tls 10s, llm_dns and llm_tcp 5s), one row per target in
input order, exit 0 once the targets ran. What goes wrong with one target
is its `error` field, never a failed call: `dns_not_found`, `dns_error`,
`connection_refused`, `connection_reset`, `unreachable`, `timeout`,
`tls_error`, `certificate_error`, `transport_error`; llm_http adds
`too_many_redirects`, `invalid_json`, `json_path_not_found` and
`body_max_exceeded`, llm_dns `nxdomain`, `servfail`, `refused` and
`bad_response`, llm_tls `no_certificate`. A call with no target is
`missing_target`, a target that does not parse (a URL that is not http(s),
a HOST:PORT without a valid port, a bad name) `invalid_target` (RETRY).

`llm_http` sends GET (default), HEAD or OPTIONS: any other method can
change the server's state, is a write, and is refused with
`http_method_blocked` (FATAL); writes belong to `agent_*`. No request body.
`ok` is true when there is no error, the status is expected (`--expect
200`, `2xx` or `200,301`; default 200 to 399), `--grep` matched and
`--json-path` was found. `timing` is `{dns, connect, tls, ttfb}` in ms
(null for a phase that did not happen; summed over redirect hops);
`redirects` lists each 3xx as `{status, location}` (one without
`--follow`, every hop with it, at most 10). Credentials never go on the
command line: a `--header` whose name carries one (`Authorization`,
`Cookie`, a name with TOKEN, KEY, SECRET, AUTH, ...) and a URL with a
password are `secret_in_argv` (RETRY). `--auth-env VAR` sends the header
held by the exported variable VAR, either `Name: value` or a bare value
meaning `Authorization: value`; the value is never printed, not even in an
error, and is not sent on when a redirect leaves the host (`auth_env_unset`,
STOP, when VAR is empty; `invalid_auth_env`, RETRY, for a bad name or a
value with a line break; `--insecure` with `--auth-env` is
`conflicting_options`). Response headers selected with `--headers` keep
credentials masked (`set-cookie` keeps the cookie name and attributes),
and known token formats are masked in headers, `match` and `value` as
`llm_env` masks them. `--body-max N` (default 8 MiB) bounds the body kept
for `--grep` and `--json-path`; `bytes` counts the whole body. Proxies come
from `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, CAs from the system or
`SSL_CERT_FILE`/`SSL_CERT_DIR`. Other codes: `invalid_header`,
`invalid_resolve`, `invalid_expect`, `invalid_body_max`,
`invalid_json_path`, `invalid_regex` (RETRY).

`llm_dns` asks `--server IP[:PORT]` or the nameservers of
`/etc/resolv.conf` itself (UDP, TCP when truncated); names are asked as
given, without search domains. Answers are sorted (addresses in numeric
order): A/AAAA, NS, CNAME and TXT are string lists (a TXT record's strings joined), MX `[{host, pref}]`,
CAA `[{flag, tag, value}]`, SOA `[{mname, rname, serial, refresh, retry,
expire, minimum}]`; `[]` means no records, `null` that the lookup failed.
An unknown type is `invalid_type`, a server that is not an IP
`invalid_server` (RETRY). `llm_tls` accepts any certificate in the
handshake so that an expired or self-signed one is still described, then
verifies the chain for the server name (`--sni`, default HOST):
`verified` and `verify_error` (`expired`, `not_yet_valid`,
`hostname_mismatch`, `unknown_authority`, `invalid`); `days_left` is
negative once expired.

### 3.10 Processes, waiting and services (3.4)

| Function | Output |
|---|---|
| `llm_wait --timeout S [--interval S] (--file-match FILE RE \| --exists PATH \| --gone PID\|NAME \| --http URL [--code N] [--json-path P --equals V] \| --port HOST:PORT \| --cmd-ok CMD) [--tail N] [--width N]` | `{"met":true,"after_s":2.01,"match":"<line>","tail":[...]}`, exit 0; timeout `{"met":false,"after_s":S}`, exit 1, no error envelope |
| `llm_service UNIT... [--log N] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{unit, load, active, sub, since, pid, restarts, log:[...]}]`, in the order given |
| `llm_docker_ps [--running] [--format F] [--width N] [--max-lines N] [--head N] [--tail N]` | `[{name, image, state, health, ports, up}]` by name |
| `llm_docker_logs CTR [--tail N] [--since T] [--grep RE] [--width N]` | `{ctr, read, log:[...]}` |
| `llm_docker_exec CTR [--user U] [--width N] [--max-lines N] [--head N] [--tail N] -- CMD...` | `{rc, out, err}` (+ `truncated`); exit 0 when the command ran |
| `llm_sysinfo [--mount PATH]...` | `{host, os, kernel, uptime_h, load, cpus, cpu, mem_mb, disks, gpu}` in about 200 bytes |

Waiting is one call: `llm_wait` looks at its one condition at once and
then every `--interval` (0.2 s for files, paths and a PID; 0.5 s for a
NAME, a URL and a port; 1 s for a command), and on Linux a file or path
change wakes it at once (inotify). `--file-match` scans the file once, then
only the bytes appended (a truncated or replaced file is scanned again; a
missing one is waited for); RE is RE2, matched per line. `--gone` takes a
PID (a zombie counts as gone) or a NAME matched as `llm_procs --name`.
`--http` waits for a 2xx (or `--code N`), with `--json-path P --equals V`
for a value of the JSON body; `--cmd-ok` re-runs a command line checked
once under the `llm_exec` rules (6.3). The timeout is an answer, not an
error: `met:false` with exit 1, plus what the last look saw (`last`,
`code`, `rc`). `--timeout` is required (at most 1h); `missing_condition`
without a condition, `unexpected_argument` for a second one.

The process functions read, never signal (`agent_kill` does). Listings
leave out the caller's own chain (the gash process, its parent shells and
their same-command-line subshells), so `llm_procs --name X` and
`llm_wait --gone X` never match the shell that asked; `--pid` targets
are reported as given. `llm_service` runs `systemctl show` and
`journalctl -u`; without systemd it is `systemd_unavailable` (CONTINUE).
The docker functions run the `docker` CLI with `llm_docker_check`'s short
environment; no docker is `docker_not_found`, a daemon that cannot be
reached `docker_unavailable` (both CONTINUE), an unknown container
`container_not_found` (RETRY). `llm_docker_exec` passes CMD's words to
`docker exec` as an array (no shell, no quoting) after the `llm_exec` rules:
one command, `sed`/`awk` under `--sandbox`, `grep`/`rg` skipping secret
files, `git` refused. Journal, log and match lines are cut at 300 columns
unless `--width` says otherwise.

### 3.11 Runtimes, lint and tests (3.4)

| Function | Output |
|---|---|
| `llm_tools [NAME...] [-j N] [--timeout S]` | JSON object, keys in the order asked: `{"version","path"}` per program, `null` when it is not on PATH, `"version":null` when its probe prints none; `php` and `node` are arrays of every installed version, oldest first. Without NAME: git php composer node npm python3 go rg jq docker |
| `llm_runtime php\|node\|composer\|npm\|npx VERSION` | `{"runtime","request","path","version"}` (composer adds `"php"`, the binary it runs under); VERSION is a prefix (`24`, `24.1`, `8.4`, `8`) and the newest match wins |
| `llm_lint [PATH...] [--ext php,sh,js,json,yaml,toml,py] [--php VER] [--node MAJOR] [-j N] [--timeout S] [--max-lines N] [--width N] [--absolute]` | `{"checked","failed","failures":[{"file","line","error"}]}`, 100 failures at most by default; exit 0 also when files fail |
| `llm_test [--runner auto\|NAME] [--fail-only] [--max-lines N] [--width N] [--timeout S] [--absolute] -- CMD...` | `{"runner","passed","failed","skipped","duration_s","failures":[{"name","file","line","message"}],"parsed":true,"exit_code"}`; output no parser reads: `{"runner":null,"duration_s","parsed":false,"exit_code","tail":[LINES]}`. The exit status is CMD's |

`llm_test` and `gash run` take a command after `--`: the one exception to
the no-`--` rule of this section. Their options come before `--` (or before
the first word that is not an option), every word after it belongs to CMD,
`--help` included. `llm_test` runs a test suite and nothing else: a runner
(pest, phpunit, paratest, phpstan, psalm, vitest, jest, mocha, playwright,
pytest; by path such as `vendor/bin/pest`, or through `php`, `node`, `npx`,
`pnpm exec`, `python -m`), `php artisan test`, `go test`, `cargo test`, the
test script of npm, pnpm, yarn, bun or composer, `make test`; any other
command is `not_a_test_command` (RETRY), and `gash run` runs it. The suite
runs the project's own code, without `llm_exec`'s rails; stdout and stderr
are merged, stdin is `/dev/null`, a program not on PATH
is `command_not_found` (status 127). Runners: pest, phpunit, phpstan and
psalm (analyzer findings count as failures), vitest, jest (text or
`--json`), mocha, pytest, go (text or `-json`), cargo, playwright; `auto`
picks one from CMD and the output.

`llm_lint` runs `php -l` on batches of files, `bash -n` per shell script,
node per batch of js files (each compiled as node loads it, CommonJS or ES
module; a failure is re-checked with `node --check` for its message),
python `compile()` per batch (no `.pyc` written), and parses JSON (comments
allowed in `tsconfig.json`, `jsconfig.json` and `.vscode/`), YAML and TOML
itself. Directories are walked like `llm_find` (no node_modules, vendor,
.git, \_\_pycache\_\_, no symlinks followed); a secret file is skipped
there and `secret_file_blocked` as an argument, another extension as an
argument is `unsupported_file`. A missing checker is
`runtime_not_installed` (STOP).

`llm_runtime`, `llm_tools` and `llm_lint --php/--node` find node in the
nvm tree (`${NVM_DIR:-~/.nvm}/versions/node`, without loading nvm.sh), php
as `/usr/bin/phpX.Y` or `/usr/local/bin/phpX.Y`, then the one on PATH. A
version that is not installed is `runtime_not_installed` (STOP) naming the
installed ones: ask the user. They read gash's runtime cache, never write
it.

Three commands of the binary (run them as `gash WORD`; they are not shell
functions):

| Command | Effect |
|---|---|
| `gash runtime php\|node\|composer\|npm\|npx VERSION [--path]` | The `llm_runtime` object, or with `--path` the path alone; the resolution is cached in `${XDG_CACHE_HOME:-~/.cache}/gash/runtimes.json` until the nvm tree or `/usr/bin/php*` change |
| `gash run [--width N] [--max-lines N] [--head N] [--tail N] [--test-summary] [--runtime node:24\|php:8.4] [--safe] -- CMD...` | Runs CMD and returns its exit status unchanged. The budget flags shape its output (3.6, `[... K lines omitted]` markers); `--runtime` (repeatable) puts that runtime's directory first on PATH; `--test-summary` prints the `llm_test` object instead of the output; `--safe` runs CMD under `llm_exec`'s rules (6.3). CMD may be a gash function (`gash run --max-lines 20 -- files_largest .`). gash's own errors are envelopes |
| `gash agent-functions [--json]` | Every `llm_*` and `agent_*` function: `[{"name","family","short","usage","flags","replaces"}]`, family `read` or `write`; a table without `--json` |

### 3.12 Remote hosts (3.4)

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
permission denied). No results: `"data": []` and `"count": 0`. Equal sizes
are ordered by mtime, newer first (`files_largest`), then by path in
descending byte order.

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

top_by_count is ordered by count, then size (both descending), then
extension in ascending byte order; top_by_size by size, then count, then
extension. Extensions are lowercased in ASCII only.

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

`llm_*` and `agent_*` functions and the filesystem scanners (in `--json`
mode) report errors on **stderr** as one JSON line, with a non-zero exit
status:

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
| Database | `no_db_config` `no_database` `unknown_db_type` `invalid_db_url` (STOP); `sqlite_error` `mysql_error` `pgsql_error` `sqlite_file_not_found` `missing_sqlite_file` (RETRY); `sqlite_not_found` `mysql_not_found` `psql_not_found` `invalid_sqlite_file` (FATAL) |
| Context | `not_a_git_repo` `git_not_found` `no_package_file` `ports_tool_not_found` (CONTINUE); `invalid_manifest` (RETRY); `jq_not_found` (FATAL) |
| `llm_docker_check` | `no_services` `docker_not_found` `compose_not_installed` `invalid_service_name` (CONTINUE); `compose_not_found` `invalid_env_file` (RETRY) |
| WSL & Windows (`gash wsl`, `gash win`, `gash logs`) | `interop_unavailable` `windows_access_denied` `elevation_required` `windows_not_found` (STOP); `interop_timeout` `invalid_windows_argument` `invalid_duration` `confirmation_required` (RETRY) |
| Shared output options (3.4) | `invalid_format` `invalid_width` `invalid_max_lines` `invalid_head` `invalid_tail` `invalid_jobs` `invalid_timeout` (RETRY) |
| Files and search (3.4) | `invalid_sort` `invalid_by` `invalid_since` (RETRY) |
| Text reads (3.4) | `invalid_range` `invalid_assertion` `invalid_algo` `invalid_short` `invalid_level` `invalid_rule` (RETRY) |
| Data files and values (3.4) | `not_a_scalar` `invalid_json` `invalid_json_path` `json_path_not_found` `not_an_object` `invalid_delimiter` `invalid_where` `unknown_column` `invalid_config` `invalid_env_name` `invalid_date` `invalid_timezone` `invalid_date_format` (RETRY) |
| Git (3.4) | `git_error` `detached_head` `merge_conflict` `push_failed` (STOP); `invalid_log` `invalid_files` `invalid_fields` `unknown_revision` `missing_message` `invalid_author` `nothing_to_commit` `git_add_failed` `commit_failed` `no_upstream` `unknown_remote` `push_rejected` (RETRY) |
| Network probes (3.4) | `http_method_blocked` (FATAL); `auth_env_unset` (STOP); `missing_target` `invalid_target` `invalid_server` `invalid_header` `invalid_resolve` `invalid_expect` `invalid_body_max` `invalid_auth_env` `secret_in_argv` (RETRY) |
| Processes, waiting, services (3.4) | `kill_refused` (FATAL); `kill_failed` (STOP); `ps_not_found` `systemd_unavailable` `container_not_running` `docker_unavailable` (CONTINUE); `invalid_pid` `missing_condition` `invalid_interval` `invalid_code` `invalid_url` `missing_unit` `missing_container` `missing_command` `invalid_user` `container_not_found` `docker_error` `invalid_signal` `pattern_too_broad` (RETRY) |
| Runtimes, lint, tests (3.4) | `runtime_not_installed` `runtime_link_failed` (STOP); `unknown_runtime` `invalid_version` `command_not_found` `unsupported_file` `invalid_ext` `unknown_runner` `not_a_test_command` (RETRY) |
| Write tools (`agent_*`) | `write_path_denied` (FATAL); `sensitive_path` `write_failed` (STOP); `invalid_replacement` `invalid_count` `replace_count_mismatch` `binary_file` `file_changed` (RETRY) |

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
* Scanners refuse `/proc`, `/sys`, `/dev`, `/boot`, `/root` (and root's
  home where it is not `/root`), also when reached through a symlink or
  from `/`: as the PATH argument they fail with `forbidden_path`, and a walk
  that follows `-L` or starts at `/` with `--allow-root` skips them. They
  need `--allow-root` for `/` and do not refuse `..`.
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
* Windows (WSL), reachable over /mnt: `ntuser.dat` `ntuser.dat.*`
  `usrclass.dat` `usrclass.dat.*` `*.rdp` `unattend.xml` `autounattend.xml`
  `login data` `logins.json` `key4.db` `key3.db`
* Anything under a directory `.ssh` `.gnupg` `.aws` `.azure` `.kube`
  `.docker` `.config/gcloud`
* Not secret (templates, which win over the above): `*.pub` `.env.example`
  `.env.sample` `.env.dist` `.env.template` `*.env.example`; `llm_grep`
  and `llm_find --contains` search them.

`llm_config` and `llm_exec` refuse them as arguments; `llm_grep`,
`llm_find --contains` and, inside `llm_exec`, `grep`, `rg`, `diff` and
`git diff`/`show`/`grep`/`log -p` skip them (`git log` with `--follow` or
`-L` does not; nor do commands named in `GASH_LLM_EXEC_ALLOW` or
`unrestricted` mode). Inside `llm_exec` those tools search the `.env`
templates and the `id_*` public keys (`id_rsa.pub`); an exception name that
also matches another pattern (`credentials.pub`, `secrets.env.example`)
stays skipped there. `llm_exec` also refuses `git show REV:PATH` for a
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
| `GASH_LLM_SANDBOX=1` | Runs the child in `bwrap`: read-only filesystem, no network, and under WSL no Windows interop (`sandbox_unavailable` if bwrap cannot start) |

Whoever controls the agent's environment (`HOME`, `GASH_ENV_FILE`) controls
which file is read. On `command_not_allowed`, ask the user; never edit
`~/.gash_env` yourself.

### 6.4 Databases

1. **SQL guard**, a lexer per engine (quotes, comments, PostgreSQL dollar
   quotes): one statement (a trailing `;` is fine); first keyword SELECT, WITH,
   VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN or PRAGMA (a read only: no value,
   no acting pragma such as optimize); no write keywords or
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
private key blocks, URL passwords, `password=`/`token:`-style values,
Authorization headers, `SSH:` passphrase lines, crypt password hashes and
known token formats (sk-, ghp_, AKIA, xox, AIza). Redaction is pattern-based
and can miss secrets (free text such as "the password is ..." is sent). `GASH_AI_CONFIRM`: auto (default) asks before sending data
Gash collected itself (`ai_sysinfo`), 1 asks before every request, 0 never
asks; any other value asks before every request and warns. In headless mode or without a terminal, a request that needs
confirmation is refused. `ai_ask` needs a terminal and refuses headless:
use `ai_query "question"`. Piped input must start within 2 seconds; an open
but silent stdin is ignored with a warning (redirect `</dev/null`). Call
AI functions only when the user asks for them.

Headless mode reads `GASH_DB_TIMEOUT`, `GASH_AI_CONFIRM`,
`GASH_AI_MODEL_CLAUDE` and `GASH_AI_MODEL_GEMINI` from `~/.gash_env` (the
environment wins) and prints no configuration warnings unless `GASH_DEBUG=1`.

## 7. Routing: the raw shape and the Gash function

The command an agent would type, and the Gash call that answers it in one
round trip with a capped, structured answer. `gash agents-hint` prints this
table with a short preamble, ready to paste into an agent's instructions
(`--format json` for one object per row). The table is generated from the
registry, where each function's entry names the shape it replaces.

| Instead of | Use |
| --- | --- |
| `grep -q P f && echo ok; grep -c P f (as checks)` | `llm_assert` |
| `cat config/app.yaml (a config file that is not secret)` | `llm_config` |
| `python3 -c "import tomllib, yaml ..."; yq .a.b f` | `llm_config --path a.b` |
| `grep -c A f; grep -c B f; grep -c C g` | `llm_count` |
| `python3 -c "import csv ..."; wc -l f; head -3 f` | `llm_csv` |
| `date -d '-600 seconds'; python3 datetime arithmetic` | `llm_date` |
| `mysql -e 'SHOW DATABASES'; psql -l` | `llm_db_databases` |
| `mysql -e "EXPLAIN SELECT ..."` | `llm_db_explain` |
| `mysql -N -B -e "SELECT ..."; sqlite3 f "SELECT ..."` | `llm_db_query --format tsv` |
| `llm_db_query ... \| jq -r '.rows[0][0]'` | `llm_db_query --scalar` |
| `mysql -e "SELECT * FROM t LIMIT 5"` | `llm_db_sample` |
| `mysql -e "SHOW CREATE TABLE t"; sqlite3 f .schema` | `llm_db_schema` |
| `mysqldump --no-data` | `llm_db_schema --all` |
| `mysql -e "SHOW TABLES"; sqlite3 f .tables` | `llm_db_tables` |
| `SELECT count(*) FROM t, per table` | `llm_db_tables --counts` |
| `cat composer.json package.json \| jq .require` | `llm_deps` |
| `for d; do dig +short MX $d; done` | `llm_dns --type MX` |
| `docker compose pull --dry-run; docker images` | `llm_docker_check` |
| `docker exec -u USER ctr sh -c '...'` | `llm_docker_exec` |
| `docker logs --tail 50 ctr 2>&1 \| grep -i error` | `llm_docker_logs` |
| `docker ps --format '{{.Names}} {{.Status}}'` | `llm_docker_ps` |
| `grep -E '^(DB_HOST\|APP_URL)=' .env` | `llm_dotenv` |
| `env \| grep -i PATH; printenv NODE_ENV` | `llm_env` |
| `bash -c "..." for an allowlisted command` | `llm_exec` |
| `find D -name '*.php' -not -path '*/vendor/*'` | `llm_find` |
| `find D -type f \| wc -l` | `llm_find --count` |
| `find D -newer F \| sort \| head` | `llm_find --newer F --sort mtime` |
| `git diff --stat \| tail -1` | `llm_git_diff --summary` |
| `git diff --name-status; git diff --numstat` | `llm_git_diff --files` |
| `git log --oneline -20 -- path` | `llm_git_log --path P` |
| `git status --short \| wc -l; git log --oneline -3; git diff --stat \| tail -1` | `llm_git_overview` |
| `git show REV --stat` | `llm_git_show --stat` |
| `git show REV:path \| sed -n 1,80p` | `llm_git_show --max-lines 80` |
| `git status --short; git branch -vv` | `llm_git_status` |
| `for r; do git -C $r status -sb; done` | `llm_git_status --repos` |
| `git fetch && git status -sb` | `llm_git_status --fetch` |
| `grep -rn --include='*.php' X . \| grep -v vendor` | `llm_grep --ext php` |
| `grep -o RE f \| sort \| uniq -c` | `llm_grep --distinct` |
| `sha256sum f \| cut -c1-12; md5sum f` | `llm_hash` |
| `for u; do curl -s -o /dev/null -w '%{http_code}' $u; done` | `llm_http --status-only` |
| `curl -s URL \| jq .status` | `llm_http --json-path status` |
| `python3 -c "import json ..."; jq .a.b f` | `llm_json --path a.b` |
| `for f; do php -l $f; done; bash -n x.sh; node --check f.js` | `llm_lint` |
| `tail -n 50 app.log \| cut -c1-200` | `llm_log` |
| `grep ERROR app.log \| tail -20` | `llm_log --last-match ERROR -n 20` |
| `ss -tuln` | `llm_ports` |
| `ss -tlnp \| grep :8080; lsof -i :8080` | `llm_ports --port 8080` |
| `ps aux \| grep X \| grep -v grep` | `llm_procs --name X` |
| `kill -0 PID` | `llm_procs --pid PID` |
| `ls; cat composer.json package.json (what kind of project is this)` | `llm_project` |
| `grep -n X f, then sed -n A,Bp f` | `llm_read --around X -C 5` |
| `cat -n f \| sed -n A,Bp; cat a b` | `llm_read` |
| `source ~/.nvm/nvm.sh; nvm use 24; node ...; php84 ...` | `llm_runtime` |
| `systemctl is-active x; journalctl -u x -n 20` | `llm_service --log 20` |
| `ls -la a b; [ -f c ] && echo yes; wc -l d; readlink -f e` | `llm_stat` |
| `uptime; free -h; df -h /; nproc; nvidia-smi` | `llm_sysinfo` |
| `nc -z h p; timeout 2 bash -c '</dev/tcp/h/p'` | `llm_tcp` |
| `vendor/bin/pest \| grep -E 'Tests:\|FAIL'; npm run test \| tail -20` | `llm_test` |
| `grep -nP '[^\x00-\x7F]' f; file -i f` | `llm_textcheck` |
| `wc -w f; wc -c f; gzip -c f \| wc -c` | `llm_textstats` |
| `openssl s_client -connect h:443 \| openssl x509 -noout -dates` | `llm_tls` |
| `php84 -v \| head -1; node24 --version; git --version; command -v rg` | `llm_tools` |
| `tree -L 2 -I node_modules` | `llm_tree` |
| `du -sh */ \| sort -rh \| head` | `llm_tree --top 10` |
| `while ! grep -q X f; do sleep N; done; tail f` | `llm_wait --file-match f X` |
| `until curl -sf URL; do sleep 1; done` | `llm_wait --http URL` |
| `docker exec ctr sh -c '...' (writes)` | `agent_docker_exec` |
| `git add A B && git commit -m S && git push && git log -1` | `agent_git_commit --add A B --push` |
| `pkill -f x; kill PID` | `agent_kill` |
| `python3 - <<EOF ... s.replace(...) ... EOF; sed -i` | `agent_replace` |


## 8. Write tools (`agent_*`)

Every `llm_*` function is read-only, with no exception. Every change an agent
makes through Gash is an `agent_*` call, a separate family with its own card
topic (`gash agent`) and reference page, so an agent given only the `llm_*`
functions knows exactly what stays out. The family keeps the `llm_*`
contract: the JSON envelope, the error contract of section 5, the shared
output options of 3.6, the telemetry line, `gash <fn>` without sourcing.

| Rule | |
|---|---|
| `--dry-run` | Every `agent_*` function takes it: the same checks, a report of what would change (`"dry_run":true,"written":false`), nothing written |
| Explicit targets | Each target is named: no glob is expanded, no implicit "all" (`agent_git_commit` has no default `-A`) |
| All or nothing | Checks run on every target before the first write; one failure writes nothing |
| Secret files and protected paths | Refused as for `llm_*` (`secret_file_blocked`, `forbidden_path`, `path_traversal_blocked`: FATAL) |
| System paths | The transfer policy of the remote functions applies to local targets too: `/bin`, `/sbin`, `/lib*`, `/usr/bin`, `/usr/sbin`, `/usr/lib*`, `/boot`, `/sys`, `/proc`, `/dev`, `/etc/ssh`, `/etc/sudoers*`, `/etc/passwd`, `/etc/shadow*`, `/etc/gshadow*`, `/etc/pam.d` are `write_path_denied` (FATAL, even with `--force`); `/etc`, `/var`, `/usr/local`, `/root` and any `.ssh` are `sensitive_path` (STOP: ask the user, then `--force`), except below your HOME or below the working directory of the project you run in (never `.ssh`, never a working directory below `/etc`) |
| Git | Never a force push |

| Function | Output |
|---|---|
| `agent_replace FILE... (--old A --new B)... [--count N\|--min N] [--regex] [--dry-run] [--force] [--format F] [--width N]` | `{"files":N,"replaced":{"FILE":[n1,n2]},"written":true}`; each pair must match exactly once per file (`--count N`: exactly N, `--min N`: at least N, replacing all); pairs apply in order; a mismatch anywhere is `replace_count_mismatch` (RETRY) and no file changes; `--dry-run` adds `"diff":{"FILE":{"add":A,"del":D}}` (lines gained and lost) |

`agent_replace` writes each file through a temporary file in the same
directory, renamed over the target with its mode (and owner, as root); a
target that changed while it ran is `file_changed` and nothing is written.
Binary files (`binary_file`) and files over 64 MiB (`file_too_large`) are
refused.

## 9. Best practices

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

## 10. Local sites (`gash hte`)

`gash hte` manages the user's local Apache and PHP-FPM sites
([docs/hte.md](docs/hte.md)). It is not read-only: call it when the user asks
for a site change, and read with `site list`, `site show` and `doctor` the
rest of the time.

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; gash hte site list --json'
```

* Every `gash hte` command takes `--json` (one JSON object on stdout) and
  `--help`. Errors are the envelope of section 5 on stderr, exit status 1.
* Headless calls never prompt: a missing required flag is `missing_value`
  (RETRY with the flag), a destructive step without `--yes` is
  `confirmation_required` (RETRY with `--yes`, once the user agreed).
* `gash hte install` needs `sudo` once and changes system files: leave it
  to the user. Without an install every other command fails with
  `hte_not_installed` (STOP).

| Action | Codes |
|---|---|
| RETRY | `missing_value` `conflicting_options` `confirmation_required` `site_not_found` `site_exists` `invalid_domain` `invalid_docroot` `docroot_not_found` `php_version_not_installed` `php_not_enabled` `invalid_profile` `invalid_ports` `invalid_autostart` `lock_timeout` |
| STOP | `hte_not_installed` `port_in_use` `privileged_step_required` `config_test_failed` `reload_failed` `apache_module_missing` `invalid_state` |
| FATAL | `apache_not_found` `openssl_not_found` `platform_unsupported` `receipt_mismatch` `root_not_allowed` |

## 11. WSL, Windows and log reclaim (`gash wsl`, `gash win`, `gash logs`)

On a WSL distribution Gash reaches the Windows host and converts paths; on
any host it reclaims disk space from log files. Each command takes `--json`
(one JSON object on stdout) and `--help`; errors are the envelope of
section 5 on stderr, exit status 1.

`gash wsl` reports and repairs the WSL environment:

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; gash wsl info --json'
```

* `info` summarises the detection (version, distro, interop, drives,
  networking); it exits 0 off WSL with `{"wsl":false}`.
* `doctor` checks interop, the mounts and the configuration; it exits 1 when
  something is wrong, and off WSL.
* `path PATH [--windows|--mixed|--linux]` converts a path between its Linux
  and Windows forms.
* `config [FILE]` shows the effective `wsl.conf` and `.wslconfig` settings.
* `interop [--export]` reports this shell's interop; `--export` prints the
  `export WSL_INTEROP=…` line to `eval` when a reattached tmux or screen
  left it stale.

`gash win` inspects the Windows host over interop, **read-only**:

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; gash win info --json'
```

The subcommands are `info`, `disk`, `net`, `updates`, `defender`, `policy`,
`hosts`, `doctor`, `services [FILTER]`, `procs [FILTER]`, `apps [FILTER]`,
`reg KEY [--value NAME]`, `events [--log L] [--level LVL] [--since DUR]
[-n N]`, `ports [--port N]` and `file PATH`. Nothing here writes to Windows.
Each call is bounded by `GASH_WIN_TIMEOUT` (default 10s): a disabled or
wedged interop is `interop_unavailable` or `interop_timeout` (do not work
around it), and data that needs an administrator reads as
`windows_access_denied`.

`gash win fix` is the one path that changes Windows, and it is **human-only,
behind a UAC prompt**. A headless caller is refused with
`elevation_required`; an agent uses `--dry-run` to read the exact elevated
PowerShell command and hands it to a human to run:

```bash
gash win fix flush-dns --dry-run --json
```

The actions are a fixed allowlist (`flush-dns`, `restart-service NAME`,
`hosts-add IP HOST`, `hosts-remove HOST`); every argument is validated and
embedded as data, never as code.

`gash logs` finds and reclaims disk space from log files (user-owned and,
with `--sudo`, root-owned):

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; gash logs scan --json'
```

* `scan [PATH...]` reports the log files eating disk, grouped by category
  (`*.log`, `*.txt`, rotated and compressed logs, and framework patterns);
  nothing is changed. `--min-size`, `--older-than` and `--depth` narrow it.
* `clean [PATH...]` reclaims space: it deletes rotated and compressed logs,
  and with `--truncate` also truncates live logs in place (a running daemon
  keeps its open file). `--dry-run` shows what would be freed; otherwise it
  needs `--yes`. A headless `clean` without `--yes` is
  `confirmation_required` (RETRY with `--yes`, once the user agreed).

## 12. Drop-in snippets

### 12.1 Claude Code (`CLAUDE.md`)

````markdown
## Gash (3.x)

Run: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'`

- Code: `llm_tree --depth 2 --stats`, `llm_find '*.php' --contains REGEX`, `llm_grep PATTERN --ext php,js`
- Disk: `files_largest --json`, `dirs_largest --json`, `tree_stats --json`
- DB (read-only): `llm_db_tables -c CONN`, `llm_db_schema TABLE -c CONN`, `llm_db_query "SELECT ..." -c CONN`
- Git: `llm_git_status`, `llm_git_log -n 10`, `llm_git_diff --staged`
- Commands: `llm_exec "CMD"` runs allowlisted read-only commands only
- Local sites: `gash hte site list --json`, `gash hte doctor --json`; change sites only when asked

Errors: stderr JSON `{error, action, recoverable, details?, hint?}`. RETRY: fix, retry once.
CONTINUE: skip. STOP: ask the user. FATAL: show the hint, stop; never rephrase to get around it.
Never edit ~/.gash_env. Reference: ~/.gash/AGENTS.md
````

### 12.2 Codex, Cursor and other `AGENTS.md` readers

````markdown
## Tooling: Gash 3.x

Invoke: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <fn> [args]'`
- Filesystem (`--json`): files_largest, dirs_largest, dirs_find_large, dirs_list_empty, tree_stats
- Code: llm_tree, llm_find, llm_grep, llm_config
- Database, read-only (`-c CONN` or `-f SQLITE_FILE`): llm_db_query, llm_db_tables, llm_db_schema, llm_db_sample, llm_db_explain
- Git: llm_git_status, llm_git_diff, llm_git_log
- System and project: llm_env, llm_ports, llm_procs, llm_project, llm_deps
- Command lines: llm_exec (read-only allowlist; ask the user when a command is refused)
- Local sites (`--json`): gash hte site list, gash hte site show --domain D, gash hte doctor

Errors: stderr JSON with `action` RETRY | CONTINUE | STOP | FATAL; retry at most once.
Reference: ~/.gash/AGENTS.md
````

### 12.3 Generic system prompt

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

## 13. Version compatibility

Version: `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; echo "$GASH_VERSION"'`,
or `gash version --json`. Changes in 3.0 that affect agents (the full list
is the 3.0.0 entry of `CHANGELOG.md`):

* **No client programs**: the `llm_*`, `llm_db_*` and scanner functions run
  inside the binary, so `jq`, `sqlite3`, `mysql`, `psql`, `rg` and `find`
  are no longer needed. `~/.my.cnf` and `~/.pgpass` are not read: put the
  credentials in the `DB:` connection.
* **Missing paths fail**: a PATH that does not exist is `path_not_found`
  (2.x: exit 0 with no output). An empty pattern is `missing_pattern`.
* **`~` expands**: `~` and `~/...` are the home directory; `~user` is
  `invalid_path`.
* **Refused queries** report the guard verdict of the connection's engine
  (MySQL, MariaDB, PostgreSQL or SQLite), still as `write_operation_blocked`
  and the other codes of 6.4.
* **`llm_exec`**: allowlisted commands keep the walk-root and symlink checks
  of 6.1.
* **`llm_procs --name NAME`** no longer lists the shells that run it.
* **New error codes**: `invalid_db_url`, `invalid_manifest`, `invalid_path`,
  `invalid_service_name`, and the `gash hte` codes of section 8.
* **Commands that change things ask**: `git_sync`, `git_add_tag`,
  `git_delete_tag`, `git_apply_patch`, `process_kill`, `docker_stop_all`,
  `docker_start_all` and `wsl_compact` show a plan and ask; without a
  terminal they refuse unless given `-y`/`--yes`.

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
