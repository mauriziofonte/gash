#!/usr/bin/env bash

# Gash Module: LLM Utilities
# Optimized commands for LLM (Large Language Model) interaction with minimal token usage.
# All output is machine-readable (JSON or newline-separated).
#
# SECURITY NOTES:
# - All functions exclude themselves from bash history
# - llm_exec validates commands against dangerous patterns
# - Read-only operations by default
# - No .env file access (secrets protection)
# - Input sanitization on all user-provided paths/queries
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (NO short aliases - LLM use only):
#   llm_exec        - Safe command wrapper (no history, validated)
#   llm_tree        - Compact directory tree
#   llm_find        - Optimized file finder
#   llm_grep        - Grep with structured output
#   llm_db_query    - Read-only database queries
#   llm_db_tables   - List database tables
#   llm_db_schema   - Show table schema
#   llm_db_sample   - Sample rows from table
#   llm_project     - Project info detection
#   llm_deps        - List dependencies
#   llm_config      - Read config files (no .env)
#   llm_git_status  - Compact git status
#   llm_git_diff    - Diff with stats
#   llm_git_log     - Recent log
#   llm_ports       - Ports in use
#   llm_procs       - Processes by name/port
#   llm_env         - Filtered env vars (no secrets)

# Implementation lives in lib/modules/llm/, loaded in dependency order
# (core first). Help registration for every llm_* function follows.
for __gash_llm_part in core exec fs db project git system; do
    # shellcheck source=/dev/null
    source "${BASH_SOURCE[0]%/*}/llm/${__gash_llm_part}.sh"
done
unset __gash_llm_part

# -----------------------------------------------------------------------------
# NO SHORT ALIASES (by design)
# These functions are for LLM use only, not for human typing
# -----------------------------------------------------------------------------

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "llm_exec" \
        --module "llm" \
        --short "Safe command execution wrapper for AI agents" \
        <<'HELP'
USAGE
  llm_exec "command"

EXAMPLES
  llm_exec "ls -la /var/log"
  llm_exec "git log --oneline -5 && git status --short"
  llm_exec "grep -rn TODO src | head -20"
  llm_exec "systemctl status nginx"

NOTES
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
  GASH_LLM_EXEC_ALLOW="make npm"   extra commands, any arguments
  GASH_LLM_EXEC=unrestricted       skip the allowlist
  GASH_LLM_SANDBOX=1               run inside bwrap (read-only, no network)
HELP

    __gash_register_help "llm_tree" \
        --module "llm" \
        --short "Compact directory tree (JSON or text output)" \
        <<'HELP'
USAGE
  llm_tree [--text] [--depth N] [PATH]

EXAMPLES
  # JSON output (default)
  llm_tree

  # Text-indented output
  llm_tree --text

  # Limited depth
  llm_tree --depth 2 /etc/nginx

NOTES
  Ignores noise directories: node_modules, vendor, .git, __pycache__
HELP

    __gash_register_help "llm_find" \
        --module "llm" \
        --short "Find files by pattern (ignores noise directories)" \
        <<'HELP'
USAGE
  llm_find PATTERN [PATH] [--type f|d] [--contains REGEX] [-n N]

EXAMPLES
  # Find all PHP files
  llm_find "*.php"

  # Find config files containing "database"
  llm_find "*.conf" /etc --contains "database"

  # Find directories only
  llm_find "src*" . --type d

NOTES
  PATTERN is a file name glob (find -name). With --contains only the
  files it matches are searched, hidden and gitignored ones included,
  with or without ripgrep. Skips node_modules, vendor, .git and
  __pycache__; --contains also skips secret files. Default limit 100.
HELP

    __gash_register_help "llm_grep" \
        --module "llm" \
        --short "Search code with structured file:line:content output" \
        <<'HELP'
USAGE
  llm_grep PATTERN [PATH] [--ext EXT1,EXT2] [--context N]

EXAMPLES
  # Find TODO comments in PHP files
  llm_grep "TODO|FIXME" src/ --ext php,js

  # Search for a function definition
  llm_grep "function processOrder" .

  # With context lines
  llm_grep "class.*Controller" . --ext php --context 3
HELP

    __gash_register_help "llm_db_query" \
        --module "llm" \
        --short "Read-only SQL query (JSON output)" \
        --see-also "llm_db_tables llm_db_schema llm_db_sample llm_db_explain" \
        <<'HELP'
USAGE
  llm_db_query "SQL" [-c CONNECTION] [-f SQLITE_FILE] [-n ROWS]

EXAMPLES
  # Query using the default connection
  llm_db_query "SELECT id, name FROM users LIMIT 5"

  # Query a named connection
  llm_db_query "SELECT * FROM orders WHERE status='pending'" -c legacy

  # Query a SQLite database (no connection config needed)
  llm_db_query "SELECT * FROM users" -f ./data/app.db -n 20

OPTIONS
  -c, --connection NAME  Connection from ~/.gash_env (default: default)
  -d, --database NAME    Database name (overrides the connection's;
                         letters, digits, _ $ . - only)
  -f, --file PATH        SQLite database file
  -n, --limit N          Row cap (default 100; also -r, --rows)

NOTES
  Read-only, enforced twice: one statement starting with SELECT, WITH,
  VALUES, TABLE, SHOW, DESCRIBE, EXPLAIN or PRAGMA, with no write
  keywords or file/program functions; then a read-only client session
  (sqlite3 -readonly -safe, MySQL READ ONLY transactions, PostgreSQL
  default_transaction_read_only with a GASH_DB_TIMEOUT statement timeout).
  Output: {data, rows, query_time}.
HELP

    __gash_register_help "llm_db_tables" \
        --module "llm" \
        --short "List database tables (JSON array)" \
        --see-also "llm_db_query llm_db_schema" \
        <<'HELP'
USAGE
  llm_db_tables [-c CONNECTION] [-f SQLITE_FILE]

EXAMPLES
  llm_db_tables -c default
  llm_db_tables -c postgres
  llm_db_tables -f ./app.db
HELP

    __gash_register_help "llm_db_schema" \
        --module "llm" \
        --short "Show table schema (columns, types, keys)" \
        --see-also "llm_db_tables llm_db_query" \
        <<'HELP'
USAGE
  llm_db_schema TABLE [-c CONNECTION] [-f SQLITE_FILE]

EXAMPLES
  llm_db_schema users -c default
  llm_db_schema orders -c legacy
  llm_db_schema sessions -f ./app.db
HELP

    __gash_register_help "llm_db_sample" \
        --module "llm" \
        --short "Sample rows from a table (default 5 rows)" \
        --see-also "llm_db_query llm_db_schema" \
        <<'HELP'
USAGE
  llm_db_sample TABLE [-c CONNECTION] [-f SQLITE_FILE] [-n N]

EXAMPLES
  llm_db_sample users -c default
  llm_db_sample orders -c legacy --limit 10
  llm_db_sample logs -f ./app.db -n 3

OPTIONS
  -c, --connection NAME  Connection from ~/.gash_env (default: default)
  -d, --database NAME    Database name (overrides the connection's)
  -f, --file PATH        SQLite database file
  -n, --limit N          Rows to return (default 5)
HELP

    __gash_register_help "llm_db_explain" \
        --module "llm" \
        --short "Show query execution plan (EXPLAIN)" \
        --see-also "llm_db_query" \
        <<'HELP'
USAGE
  llm_db_explain "QUERY" [-c CONNECTION] [-f SQLITE_FILE] [--analyze]

EXAMPLES
  llm_db_explain "SELECT * FROM users WHERE email LIKE '%@example.com'" -c default
  llm_db_explain "SELECT o.* FROM orders o JOIN users u ON o.user_id=u.id" -c legacy --analyze
HELP

    __gash_register_help "llm_project" \
        --module "llm" \
        --short "Detect project type and framework (JSON)" \
        --see-also "llm_deps" \
        <<'HELP'
USAGE
  llm_project [PATH]

EXAMPLES
  llm_project
  llm_project /var/www/myapp
HELP

    __gash_register_help "llm_deps" \
        --module "llm" \
        --short "List project dependencies (JSON)" \
        --see-also "llm_project" \
        <<'HELP'
USAGE
  llm_deps [PATH] [--dev]

EXAMPLES
  llm_deps
  llm_deps /var/www/myapp --dev
HELP

    __gash_register_help "llm_config" \
        --module "llm" \
        --short "Read config files (JSON/YAML, excludes .env)" \
        <<'HELP'
USAGE
  llm_config FILE [--max-bytes N]

EXAMPLES
  llm_config package.json
  llm_config docker-compose.yml
  llm_config big.json --max-bytes 5000000

NOTES
  Refuses to read .env files and other secret files for security.
  YAML comes out as JSON when yq (mikefarah's or the Python yq) is
  installed, as the raw text otherwise. Files over --max-bytes
  (default 1 MiB) fail with file_too_large.
HELP

    __gash_register_help "llm_git_status" \
        --module "llm" \
        --short "Compact git status (JSON)" \
        --see-also "llm_git_diff llm_git_log" \
        <<'HELP'
USAGE
  llm_git_status [PATH]

EXAMPLES
  llm_git_status
  # {"branch":"main","ahead":0,"behind":0,"staged":[],"modified":["README.md"],"untracked":[],"conflicted":[]}
HELP

    __gash_register_help "llm_git_diff" \
        --module "llm" \
        --short "Git diff with statistics" \
        --see-also "llm_git_status llm_git_log" \
        <<'HELP'
USAGE
  llm_git_diff [--staged] [PATH]

EXAMPLES
  llm_git_diff
  llm_git_diff --staged
HELP

    __gash_register_help "llm_git_log" \
        --module "llm" \
        --short "Recent commits (JSON)" \
        --see-also "llm_git_status llm_git_diff" \
        <<'HELP'
USAGE
  llm_git_log [--limit N] [PATH]

EXAMPLES
  llm_git_log
  llm_git_log --limit 20
HELP

    __gash_register_help "llm_ports" \
        --module "llm" \
        --short "List ports in use (JSON)" \
        --see-also "llm_procs" \
        <<'HELP'
USAGE
  llm_ports [--listen]

EXAMPLES
  llm_ports
  llm_ports --listen
HELP

    __gash_register_help "llm_procs" \
        --module "llm" \
        --short "List processes by name or port (JSON)" \
        --see-also "llm_ports" \
        <<'HELP'
USAGE
  llm_procs [--name NAME] [--port PORT]

EXAMPLES
  llm_procs
  llm_procs --name nginx
  llm_procs --port 3306
HELP

    __gash_register_help "llm_env" \
        --module "llm" \
        --short "Filtered environment variables (secrets excluded)" \
        <<'HELP'
USAGE
  llm_env [--filter PATTERN]

EXAMPLES
  llm_env
  llm_env --filter PATH

NOTES
  Excludes variables matching secret patterns: PASSWORD, PASSPHRASE,
  TOKEN, SECRET, KEY, CREDENTIAL, AUTH, SESSION, JWT, WEBHOOK, *_PWD,
  *_PAT, etc. In the others, URL passwords and known token formats
  (AKIA..., ghp_..., sk-..., xox?-..., AIza...) are shown as ***.
HELP

fi # end help registration guard
