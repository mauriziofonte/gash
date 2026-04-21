# AGENTS.md — Gash Integration Guide for LLM Agents

> **Audience:** LLM coding agents — Claude Code, OpenAI Codex, Kimi, Gemini,
> Cursor, Windsurf, Aider, local LLMs via Ollama, and anything else that
> shells out to Bash. Drop the relevant sections of this file into your
> agent's system prompt / `CLAUDE.md` / `AGENTS.md` / `.cursorrules` to give
> it safe, structured access to Gash.
>
> **Applies to:** Gash `v1.5.0+`.
> **Stability:** The `llm_*` contract, the `--json` envelope, and the error
> shape are considered stable API. Breaking changes require a minor bump.

---

## Table of contents

1. [Quick auto-configuration](#1-quick-auto-configuration)
2. [Invocation modes](#2-invocation-modes)
3. [Function catalog](#3-function-catalog)
4. [JSON envelope schemas](#4-json-envelope-schemas)
5. [Error contract](#5-error-contract)
6. [Safety guardrails](#6-safety-guardrails)
7. [Best practices](#7-best-practices)
8. [Agent-specific drop-ins](#8-agent-specific-drop-ins)
9. [Version compatibility](#9-version-compatibility)

---

## 1. Quick auto-configuration

### One-liner (any agent, any Bash)

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'
```

That single command gives the agent:

* All `llm_*` functions (safe command exec, file/code search, DB queries, git status, ports, processes, project info, dependencies, docker compose check, …)
* All v1.5 filesystem scanners with JSON envelopes (`files_largest`, `dirs_largest`, `dirs_find_large`, `dirs_list_empty`, `tree_stats`)
* **Zero ANSI color pollution** on stdout and stderr — essential for parseable output
* **No Bash history pollution** — all call paths exit history via `__gash_no_history`
* **No user-profile mutation** — skips `~/.bash_aliases`, `~/.bash_local`, PS1 prompt, aliases

### Environment variables the agent can set

| Variable | Effect | When to use |
|---|---|---|
| `GASH_HEADLESS=1` | Full LLM mode: no prompt, no aliases, no profile loading, zero ANSI | **Recommended for agents.** Default choice. |
| `GASH_NO_COLOR=1` | Only disable ANSI, keep the rest of the interactive shell | Agent running inside a user's existing shell |
| `NO_COLOR=1` | Same as `GASH_NO_COLOR` but honors the [no-color.org](https://no-color.org) standard | Portability; recognized by many other CLIs |

Any of the three disables colors at load time. In addition, Gash auto-disables colors whenever `stdout` is not a TTY (pipe/redirect), so agents that capture output via `$(...)` or pipes don't need to set anything to get clean text.

---

## 2. Invocation modes

### (A) Headless — recommended for agents

```bash
export GASH_HEADLESS=1
source ~/.gash/gash.sh
# all llm_* + fs scanners now available, plain output
```

Or one-shot:

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status'
```

### (B) No-color only — preserves shell experience

```bash
export GASH_NO_COLOR=1
# normal interactive shell, but no ANSI from Gash
```

### (C) Ambient pipe — zero configuration

```bash
# Gash auto-disables colors when stdout is not a TTY.
files_largest --json /var/log | jq '.data[0]'
```

### What NOT to do

* ❌ `bash -i -c '...'` — forces interactive mode, loads aliases, loads user profile. Slow and pollutes the shell.
* ❌ Parsing colored output — even without the gate, don't rely on ANSI sequences as data; use `--json`.
* ❌ `source ~/.gashrc` directly — `~/.gashrc` is the user's entry point, may load extra files. Source `~/.gash/gash.sh` with `GASH_HEADLESS=1` instead.

---

## 3. Function catalog

All `llm_*` functions are designed for agent consumption: machine-readable output on stdout, JSON errors on stderr, no bash history, input validation.

### 3.1 Filesystem scanners (`v1.5+`)

| Function | JSON mode | Notes |
|----------|-----------|-------|
| `files_largest [PATH]` | `--json` | Top N files by size, with size/mtime/path |
| `dirs_largest [PATH]` | `--json` | Top N dirs by cumulative size, `--depth N` to drill |
| `dirs_find_large [PATH]` | `--json` | Dirs over `--size SIZE` threshold, single-walk O(N) |
| `dirs_list_empty [PATH]` | `--json` | Empty dirs, with `--count` and `--null` |
| `tree_stats [PATH]` | `--json` | Totals + top extensions + depth + empty-dir count |

Common flags (accepted by all five):

```
--limit N                # cap results (default 100)
--min-size SIZE          # bytes / K / M / G / T / IEC suffixes
--depth N                # max-depth cap
--exclude GLOB           # prune matching paths (repeatable)
--no-ignore              # disable default prune (node_modules/.git/vendor/__pycache__/.cache)
--xdev                   # stay on a single filesystem
--allow-root             # required to scan "/" (safety guard)
--json                   # machine-readable envelope output
--null, -0               # NUL-delimited paths (for xargs -0)
--human / --no-human     # size formatting (auto-on on TTY)
--no-color               # force plain output
```

### 3.2 File & code search (zero-ANSI, prune-aware)

```
llm_exec "<command>"                      # validated command exec, no history
llm_tree [--depth N] [--stats] [PATH]     # nested JSON tree (--stats adds size/children_count)
llm_find <pattern> [PATH] [--type f|d] [--contains REGEX] [--limit N]
llm_grep <pattern> [PATH] [--ext a,b,c] [--context N] [--limit N]
llm_config <FILE>                         # read config files (blocks secrets)
```

### 3.3 Database (read-only, enveloped)

All 5 functions return `{"data": ..., "rows": N, "query_time": "Xms"}` on success, JSON error with `query_time` on failure.

```
llm_db_query   "<SQL>"   [-c CONN | -f SQLITE] [-d DB] [-r N]
llm_db_tables            [-c CONN | -f SQLITE] [-d DB]
llm_db_schema  <TABLE>   [-c CONN | -f SQLITE] [-d DB]
llm_db_sample  <TABLE>   [-c CONN | -f SQLITE] [-d DB] [--limit N]
llm_db_explain "<SQL>"   [-c CONN | -f SQLITE] [-d DB] [--analyze]
```

Supported drivers: `mysql`, `mariadb`, `pgsql` (via `-c CONN` mapped in `~/.gash_env`). SQLite via `-f PATH`. PostgreSQL preserves native JSON types (int, bool, array, jsonb) — they come back as JSON, not strings.

**Slow-query auto-EXPLAIN:** `llm_db_query` only — when `query_time ≥ 100ms`, the envelope auto-adds `"slow_query_explain": [...]`.

### 3.4 Git (compact)

```
llm_git_status [PATH]                 # {branch, ahead, behind, staged, modified, untracked}
llm_git_diff [--staged] [PATH]        # stat-mode diff
llm_git_log [--limit N] [PATH]        # hash, subject, author, date
```

### 3.5 System

```
llm_env [--filter PATTERN]            # env vars, secrets redacted
llm_ports [--listen]                  # [{port, proto, state}]
llm_procs [--name NAME | --port PORT] # process list, top 20 if no filter
```

### 3.6 Project analysis

```
llm_project [PATH]                    # detect: node | php | python | go | rust | ... | unknown
llm_deps    [PATH] [--dev]            # deps from package.json / composer.json / requirements.txt / ...
llm_docker_check [PATH]               # Compose image-update check (JSON)
```

### 3.7 Discovery functions (all)

All functions live in `~/.gash/lib/modules/llm.sh` and `~/.gash/lib/modules/files.sh`. To list them at runtime from a headless shell:

```bash
GASH_HEADLESS=1 bash -c '
  source ~/.gash/gash.sh
  declare -F | awk "{print \$3}" | grep -E "^(llm_|files_largest|dirs_|tree_stats)"
'
```

---

## 4. JSON envelope schemas

### 4.1 Size-list envelope (`files_largest`, `dirs_largest`, `dirs_find_large`)

```json
{
  "data": [
    {
      "size": 524288,
      "size_human": "512.0 KiB",
      "mtime": "2026-04-21 10:26",
      "path": "/absolute/path/to/file"
    }
  ],
  "count": 1,
  "total_size": 524288,
  "total_size_human": "512.0 KiB",
  "kind": "files",
  "scan_time": "7ms",
  "errors": 0
}
```

Notes:

* `mtime` absent for directories unless `--with-mtime` is passed on `dirs_find_large`.
* `kind` is `"files"` or `"directories"`.
* `errors` counts stderr lines from the underlying `find`/`du` (typically permission denied).

### 4.2 Empty-dirs envelope (`dirs_list_empty --json`)

```json
{
  "data": ["/path/empty_a", "/path/empty_b"],
  "count": 2,
  "path": "/absolute/scanned/root",
  "errors": 0
}
```

### 4.3 Tree stats envelope (`tree_stats --json`)

```json
{
  "data": {
    "path": "/absolute/path",
    "files_total": 1139,
    "dirs_total": 388,
    "size_total": 7824061,
    "size_total_human": "7.4 MiB",
    "avg_file_size": 6869,
    "max_depth": 6,
    "empty_dirs": 11,
    "top_by_count": [
      {"ext": "md",  "count": 362, "size": 706048},
      {"ext": "zsh", "count": 351, "size": 1038643}
    ],
    "top_by_size": [
      {"ext": "gif", "size": 2725478},
      {"ext": "zsh", "size": 1038643}
    ]
  },
  "scan_time": "19ms",
  "errors": 0
}
```

### 4.4 Database envelope (`llm_db_*`)

```json
{
  "data": [{"id": 1, "name": "alice"}, {"id": 2, "name": "bob"}],
  "rows": 2,
  "query_time": "12.3ms"
}
```

With slow-query auto-EXPLAIN (`llm_db_query` only, ≥100ms):

```json
{
  "data": [...],
  "rows": 1000,
  "query_time": "423ms",
  "slow_query_explain": [/* EXPLAIN plan */]
}
```

### 4.5 `llm_tree` envelope (nested tree)

```json
{
  "path": "/absolute/root",
  "type": "directory",
  "size": 65540,                    /* only with --stats */
  "children_count": 3,              /* only with --stats */
  "children": [
    {
      "name": "a",
      "type": "directory",
      "size": 65538,                /* cumulative, with --stats */
      "children_count": 2,
      "children": [
        {"name": "file.txt", "type": "file", "size": 2},
        {"name": "b", "type": "directory", "size": 65536, "children_count": 1, "children": [
          {"name": "big.bin", "type": "file", "size": 65536}
        ]}
      ]
    },
    {"name": "root.txt", "type": "file", "size": 2}
  ]
}
```

Default `--depth 1` emits a flat top-level list (backward-compatible). `--depth N` (N > 1) nests.

### 4.6 `llm_git_status` envelope

```json
{
  "branch": "main",
  "ahead": 0,
  "behind": 0,
  "staged": [],
  "modified": ["README.md"],
  "untracked": []
}
```

---

## 5. Error contract

All `llm_*` and all v1.5 filesystem scanners emit errors to **stderr** as JSON with this shape:

```json
{
  "error": "<error_type>",
  "action": "STOP|RETRY|CONTINUE|FATAL",
  "recoverable": true,
  "details": "<optional human context>",
  "hint": "<optional remediation tip>"
}
```

`error` and `action` are **always** present. `recoverable` is always a boolean. `details` and `hint` are present only when non-empty.

### Action semantics

| Action | Meaning | What the agent should do |
|---|---|---|
| `RETRY` | Fixable: bad flag, syntax error, invalid input | Fix the argument, retry **once**. |
| `CONTINUE` | Warning only: not a git repo, binary absent, partial read | Proceed; skip the operation; do not retry. |
| `STOP` | Ambiguous: multiple connections, unclear intent | Ask the user for clarification. Do not guess. |
| `FATAL` | Unrecoverable: write blocked, forbidden path, SQLite bad magic | Surface the `hint` to the user, stop. |

### Stop conditions (from CLAUDE.md convention)

* Any `{"error":...}` from a Gash call → follow `action`.
* DB connection failure → `STOP`, ask the user which connection to use.
* Multiple connections possible (ambiguity) → `STOP`.
* **Same error twice → do not retry a third time.** Ask.

### Example: parse and branch in a shell pipeline

```bash
out="$(files_largest --json /proc 2>err.json || true)"
if [[ -s err.json ]]; then
  action=$(jq -r '.action // "STOP"' < err.json)
  case "$action" in
    STOP|FATAL)  echo "Ask user: $(jq -r '.hint // .error' < err.json)"; exit 1 ;;
    RETRY)       echo "Retry with corrected input" ;;
    CONTINUE)    : ;;                              # proceed silently
  esac
fi
```

---

## 6. Safety guardrails

Agents get these guarantees automatically:

### Path validation

* `..` traversal rejected.
* Forbidden prefixes rejected: `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/etc/shadow`, `/etc/passwd`, `/etc/sudoers`.
* Scanning `/` requires explicit `--allow-root`.
* `realpath -m` normalization before predicate checks.

### Command validation (`llm_exec`)

These patterns are blocked — no way to bypass:

* `rm -rf /`, `rm -rf ~`, `rm -rf $HOME`, any equivalent whitespace-normalized form.
* `dd if=`, `mkfs.*`, `> /dev/sd*`, `> /dev/nvme*`, `> /dev/vd*`.
* Permission escalation: `chmod 777 /`, `chown -R .* /`, `sudo rm -rf`, `sudo dd`, `sudo mkfs`.
* System destruction: shutdown / reboot / init 0 / halt / poweroff.
* Fork bombs: `:(){:|:&};:`.
* Remote code execution: `curl|sh`, `wget|bash`, …
* History manipulation: `history -c`, `history -w`, `HISTFILE=`.
* Credential theft: `cat /etc/shadow`, `cat /etc/passwd`, any `.ssh/id_*` access.

### Secret file protection

Blocked extensions/patterns: `.env`, `.env.*`, `*.pem`, `*.key`, `*_rsa`, `id_rsa*`, `id_ed25519*`, `credentials*`, `secrets*`, `.gash_env`.

### SQL injection protection

* Write keywords blocked at statement start: `INSERT | UPDATE | DELETE | DROP | CREATE | ALTER | TRUNCATE | GRANT | REVOKE`.
* Only read-only: `SELECT | SHOW | DESCRIBE | EXPLAIN | WITH` allowed.
* Multi-statement via `;` → always blocked (remove trailing semicolons).
* Table names for `llm_db_schema` / `llm_db_sample` must match `[A-Za-z0-9_]+`.
* SQLite file must pass magic-number check (`SQLite format 3\0`).

### No-history policy

All `llm_*` functions wrap execution through `__gash_no_history`: temporarily disables `set -o history`, runs the command, restores. Agent calls do not pollute the user's bash history.

---

## 7. Best practices

### Output handling

* **Always prefer `--json`** for structured consumption. Text output is for humans.
* Use `jq -e` for validation/filtering in pipelines; trap non-zero exit as "selector found nothing".
* Use `--null` (NUL-delim) before piping paths to `xargs -0`, `cpio --null`, etc. Never rely on newline delimiters for paths.
* Parse `query_time` / `scan_time` numerically for SLO/latency monitoring.

### Error handling

* Branch on `.action`, not on exit code alone. Exit code tells you "something went wrong"; `.action` tells you what to do about it.
* On `STOP`, surface `hint` (or `error`) verbatim to the user. Do not invent a recovery path.
* Never retry the same call more than once. Two failures → stop, ask.

### Performance

* `dirs_find_large` uses a single `du -k` walk (O(N)). Pass `--depth N` to bound it on very deep trees (default unbounded).
* `tree_stats` uses a single `find -printf` walk; cheap even on 10k+ entries.
* For giant trees, combine `--xdev` (stay on one filesystem) with `--depth N` to keep scan time bounded.

### Composition

* `files_largest --null | xargs -0 …`  — safe path pipeline
* `llm_db_query | jq '.data[0]'`        — DB → structured filter
* `llm_git_status | jq -e '.modified | length > 0'` — conditional logic
* `tree_stats --json | jq '.data.top_by_size[0].ext'` — dominant file type

### Anti-patterns

* ❌ Grepping human output of `files_largest` (no flag) instead of parsing `--json`.
* ❌ Passing user-supplied SQL directly to `llm_db_query` without validation; the security guard rejects writes, but **quoted** user input is still dangerous (e.g., SQL in a `WHERE` clause).
* ❌ Assuming `stdout` is a TTY when writing to it. Gash's auto-gate handles that, but don't inject ANSI yourself elsewhere in the pipeline.

---

## 8. Agent-specific drop-ins

### 8.1 Claude Code (`CLAUDE.md`)

Add this block to the project's or user's `CLAUDE.md`:

````markdown
## Gash integration (v1.5+)

The environment has Gash, an LLM-friendly Bash enhancer. Invoke it via:

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function> [args]'
```

Prefer these Gash functions over raw shell commands:

**Filesystem audit (JSON):**
- `files_largest --json [PATH]` — top files by size
- `dirs_largest --json --depth 2 [PATH]` — top dirs, drill two levels
- `dirs_find_large --size 1G --json [PATH]` — dirs over a threshold
- `tree_stats --json [PATH]` — one-shot totals + top extensions + depth

**Code / project exploration:**
- `llm_tree --depth 3 --stats` — nested JSON with size + children_count
- `llm_find <pattern> --contains <regex>` — content search (uses ripgrep if available)
- `llm_grep <pattern> --ext php,js` — code search with file:line:content

**Database (read-only, enveloped):**
- `llm_db_query "<SQL>" -c <connection>` — SELECT only, auto-EXPLAIN on slow queries
- `llm_db_schema <table> -c <connection>` — column info
- `llm_db_sample <table> --limit 5 -c <connection>` — sample rows

**Git context:**
- `llm_git_status` — JSON branch/ahead/behind/staged/modified/untracked
- `llm_git_diff --staged` — stat-mode diff
- `llm_git_log --limit 10` — recent commits

**Error handling:** errors arrive on stderr as `{error, action, recoverable, details?, hint?}`.
Branch on `.action`: `STOP` → ask user; `RETRY` → fix and retry once; `CONTINUE` → proceed;
`FATAL` → surface `hint`, stop.

**Safety:** `rm -rf /`, `dd`, `mkfs`, fork bombs, secret files (`.env`, `*.pem`, `*_rsa`),
DDL/DML on databases, and paths under `/proc`, `/sys`, `/dev`, `/boot`, `/root` are all
blocked at the function level. No need to re-validate.

Full reference: `~/.gash/AGENTS.md`.
````

### 8.2 OpenAI Codex / Cursor / `AGENTS.md`-compatible agents

Add to the project's `AGENTS.md`:

````markdown
## Tooling: Gash v1.5+

**Invocation:** `GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <fn>'`

**Structured functions with JSON envelopes:**
- Filesystem: `files_largest|dirs_largest|dirs_find_large|dirs_list_empty|tree_stats` with `--json`
- Code search: `llm_tree|llm_find|llm_grep`
- Database (read-only): `llm_db_query|tables|schema|sample|explain` with `-c CONN|-f FILE`
- Git: `llm_git_status|diff|log`
- System: `llm_ports|procs|env`
- Project: `llm_project|deps|docker_check`

**Errors:** stderr JSON `{error, action, recoverable, details?, hint?}`; branch on `.action`.

**Constraints:** read-only DB; blocked: path traversal, `/proc`, `/sys`, `/dev`, `/boot`, `/root`,
secret files (`.env`, `*.pem`, `*_rsa`), `rm -rf`, `dd`, `mkfs`, fork bombs.

Reference: `~/.gash/AGENTS.md`.
````

### 8.3 Kimi / Gemini / generic LLM (system-prompt snippet)

```text
You have access to Gash, an LLM-friendly Bash enhancer at ~/.gash/. To use it:

  GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; <function>'

Prefer these functions (they emit JSON on stdout, JSON errors on stderr):

  File system: files_largest, dirs_largest, dirs_find_large, dirs_list_empty, tree_stats (all with --json)
  Code:        llm_tree (--depth N --stats), llm_find, llm_grep
  Database:    llm_db_query, llm_db_tables, llm_db_schema, llm_db_sample, llm_db_explain (read-only)
  Git:         llm_git_status, llm_git_diff, llm_git_log
  System:      llm_ports, llm_procs, llm_env (secrets redacted)
  Project:     llm_project, llm_deps, llm_docker_check

Error envelope: {"error":"<type>","action":"STOP|RETRY|CONTINUE|FATAL","recoverable":<bool>,"details":"..","hint":".."}

Rules: branch on .action; do not retry more than once; on STOP, ask the user; never try to bypass
the safety guards (they reject destructive commands, secret files, system paths).
```

### 8.4 Aider, Windsurf, Cline, and other custom agents

Most agents accept either a `CLAUDE.md`-like or `AGENTS.md`-like file. Use the Codex block above (§8.2) as a starting point; it is intentionally agent-agnostic.

---

## 9. Version compatibility

This document tracks Gash **v1.5.0+**.

### Pre-1.5 differences (avoid if possible)

* No JSON envelope on filesystem scanners — `files_largest` etc. emitted colored, truncated human text.
* No `tree_stats` — one-shot audit not available.
* `llm_tree` JSON was hardcoded to `-maxdepth 1`; `--depth N` only affected `--text` mode.
* ANSI colors leaked in non-TTY contexts — captured output contained `\033[...]`.
* `dirs_find_large` was O(N²) — a per-directory `du -sk` re-walk for each candidate.
* `dirs_find_large` required `numfmt`; now has a pure-bash IEC fallback.

### Runtime version check

```bash
GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; echo "$GASH_VERSION"'
# -> 1.5.0
```

### Future compatibility

The following are considered public, stable API:

* All `llm_*` function names and their argument shapes
* All filesystem scanner function names and their common flag set
* The JSON envelope shapes in §4 (size-list, empty-dirs, tree-stats, database, llm_tree, git-status)
* The error contract in §5 (`error`, `action`, `recoverable`, `details`, `hint`)
* The environment variables in §1 (`GASH_HEADLESS`, `GASH_NO_COLOR`, `NO_COLOR`)

Internal helpers (names starting with `__gash_*` or `__llm_*`) are **not** part of the public contract and may change without notice.

---

*Questions, issues, or contributions: <https://github.com/mauriziofonte/gash>.*
