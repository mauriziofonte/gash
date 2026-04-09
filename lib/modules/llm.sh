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

# =============================================================================
# INTERNAL HELPERS (Security Foundation)
# =============================================================================

# Dangerous command patterns - these will be blocked by llm_exec
# shellcheck disable=SC2034
__LLM_DANGEROUS_PATTERNS=(
    # Filesystem destruction - various forms
    'rm -rf /'
    'rm -rf ~'
    'rm -rf \$HOME'
    'rm -rf \*'
    'rm -fr /'
    'rm -fr ~'
    'rm  -rf'
    'rm -rf `'
    'rm -rf \$\('
    'rm -r /'
    'rm -f /'
    'cd / && rm'
    'cd /; rm'
    # Disk operations
    'dd if='
    'mkfs\.'
    '> /dev/sd'
    '> /dev/nvme'
    '> /dev/vd'
    # Permission escalation
    'chmod -R 777 /'
    'chmod 777 /'
    'chown -R .* /'
    'sudo rm -rf'
    'sudo dd '
    'sudo mkfs'
    # System destruction
    ':\(\)\{:\|:&\};:'
    'shutdown'
    'reboot'
    'init 0'
    'init 6'
    'halt'
    'poweroff'
    # Remote code execution patterns
    'curl.*\|.*sh'
    'curl.*\|.*bash'
    'wget.*\|.*sh'
    'wget.*\|.*bash'
    # History manipulation
    'history -c'
    'history -w'
    'HISTFILE='
    # Credential theft
    'cat.*/etc/shadow'
    'cat.*/etc/passwd'
    '\.ssh/id_'
    'AWS_SECRET'
    'PRIVATE_KEY'
    # Dangerous redirections
    '> /dev/sda'
    '> /dev/mem'
    '> /dev/kmem'
)

# Paths that are never allowed
__LLM_FORBIDDEN_PATHS=(
    '/etc/shadow'
    '/etc/passwd'
    '/etc/sudoers'
    '/root'
    '/boot'
    '/dev/sd'
    '/dev/nvme'
    '/dev/vd'
    '/dev/mem'
    '/dev/kmem'
)

# Secret file patterns - never read these
__LLM_SECRET_PATTERNS=(
    '.env'
    '.env.*'
    '*.pem'
    '*.key'
    '*_rsa'
    'id_rsa*'
    'id_ed25519*'
    'credentials*'
    'secrets*'
    '.gash_env'
)

# Validate and sanitize a path.
# Returns sanitized path on stdout, returns 1 if path is forbidden.
# Usage: __llm_validate_path <path>
__llm_validate_path() {
    local path="${1-}"

    # Empty path defaults to current directory
    [[ -z "$path" ]] && { echo "."; return 0; }

    # Block path traversal attempts
    if [[ "$path" == *".."* ]]; then
        echo '{"error":"path_traversal_blocked","action":"FATAL","recoverable":false,"hint":"Path traversal (..) is not allowed for security reasons"}' >&2
        return 1
    fi

    # Expand tilde safely
    path="${path/#\~/$HOME}"

    # Normalize path (resolve symlinks, remove double slashes)
    local normalized
    normalized="$(realpath -m "$path" 2>/dev/null)" || normalized="$path"

    # Check against forbidden paths
    local forbidden
    for forbidden in "${__LLM_FORBIDDEN_PATHS[@]}"; do
        if [[ "$normalized" == "$forbidden"* ]]; then
            echo '{"error":"forbidden_path","details":"'"$normalized"'","action":"FATAL","recoverable":false,"hint":"This path is protected and cannot be accessed"}' >&2
            return 1
        fi
    done

    echo "$normalized"
    return 0
}

# Validate a command against dangerous patterns.
# Returns 0 if safe, 1 if dangerous.
# Usage: __llm_validate_command <command_string>
__llm_validate_command() {
    local cmd="${1-}"

    [[ -z "$cmd" ]] && return 1

    # Normalize whitespace: collapse multiple spaces/tabs to single space, trim
    # This prevents bypassing patterns via extra whitespace (e.g., "rm  -rf  /")
    local normalized_cmd
    normalized_cmd="$(printf '%s' "$cmd" | tr -s '[:space:]' ' ')"
    normalized_cmd="${normalized_cmd# }"
    normalized_cmd="${normalized_cmd% }"

    local pattern
    for pattern in "${__LLM_DANGEROUS_PATTERNS[@]}"; do
        if [[ "$normalized_cmd" =~ $pattern ]]; then
            echo '{"error":"dangerous_command_blocked","details":"Command matches dangerous pattern","action":"FATAL","recoverable":false,"hint":"This command is blocked for safety. Do not attempt to bypass"}' >&2
            return 1
        fi
    done

    return 0
}

# Check if a file matches secret patterns.
# Returns 0 if it's a secret file (should not be read), 1 otherwise.
# Usage: __llm_is_secret_file <filename>
__llm_is_secret_file() {
    local file="${1-}"
    local basename
    basename="$(basename "$file")"

    local pattern
    for pattern in "${__LLM_SECRET_PATTERNS[@]}"; do
        # shellcheck disable=SC2053
        if [[ "$basename" == $pattern ]]; then
            return 0
        fi
    done

    return 1
}

# Output JSON error message to stderr with behavioral directives for LLM clients.
#
# Actions:
#   STOP     - Stop immediately and ask user for guidance
#   RETRY    - Can retry once with corrected input
#   CONTINUE - Warning only, can proceed
#   FATAL    - Unrecoverable, terminate operation
#
# Usage: __llm_error <error_type> [<details>] [<action>] [<recoverable>] [<hint>]
# Example: __llm_error "no_db_config" "Connection 'x' not found" "STOP" "true" "Ask user for connection name"
__llm_error() {
    local error_type="${1-unknown_error}"
    local details="${2-}"
    local action="${3-STOP}"
    local recoverable="${4-false}"
    local hint="${5-}"

    # Build JSON - escape special characters in details and hint
    local escaped_details escaped_hint
    escaped_details=$(__gash_json_escape "$details")
    escaped_hint=$(__gash_json_escape "$hint")

    local json="{\"error\":\"$error_type\""
    [[ -n "$details" ]] && json+=",\"details\":\"$escaped_details\""
    json+=",\"action\":\"$action\""
    json+=",\"recoverable\":$recoverable"
    [[ -n "$hint" ]] && json+=",\"hint\":\"$escaped_hint\""
    json+="}"

    echo "$json" >&2
    return 1
}

# Clean error message for JSON embedding.
# Collapses newlines and runs of whitespace, then trims.
# Does NOT JSON-escape — that is __gash_json_escape's job, invoked by
# __llm_error. Escaping here would double-escape downstream.
# Optional --mysql flag filters known MySQL warnings.
# Usage: cleaned=$(__llm_clean_error_msg "$raw_msg" [--mysql])
__llm_clean_error_msg() {
    local msg="${1-}"
    local filter_mysql=0
    [[ "${2-}" == "--mysql" ]] && filter_mysql=1

    if [[ $filter_mysql -eq 1 ]]; then
        msg=$(printf '%s' "$msg" | grep -v "Using a password on the command line" | grep -v "Deprecated program name")
    fi

    printf '%s' "$msg" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//'
}

# Check if jq is available for JSON formatting.
# Usage: __llm_has_jq
__llm_has_jq() {
    type -P jq >/dev/null 2>&1
}

# Check if ripgrep is available for fast searching.
# Usage: __llm_has_rg
__llm_has_rg() {
    type -P rg >/dev/null 2>&1
}

# Compute elapsed microseconds between two $EPOCHREALTIME values (bash 5.0+).
# Pure integer arithmetic — zero forks. Returns 0 if either timestamp is empty.
# Usage: local us=$(__llm_elapsed_us "$start" "$end")
__llm_elapsed_us() {
    local t0="${1:-}" t1="${2:-}"
    [[ -z "$t0" || -z "$t1" ]] && { echo 0; return; }

    local s0="${t0%%.*}" f0="${t0#*.}" s1="${t1%%.*}" f1="${t1#*.}"
    # Pad fractional part to 6 digits (EPOCHREALTIME gives 6)
    while [[ ${#f0} -lt 6 ]]; do f0="${f0}0"; done
    while [[ ${#f1} -lt 6 ]]; do f1="${f1}0"; done
    # Force base-10 (leading zeros would trigger octal)
    local us=$(( (10#$s1 - 10#$s0) * 1000000 + 10#$f1 - 10#$f0 ))
    [[ $us -lt 0 ]] && us=0
    echo "$us"
}

# Format microseconds to a human-readable query time string.
# <1ms → "842µs", <1s → "12.3ms", ≥1s → "1.23s".
# Pure bash arithmetic — zero forks.
# Usage: local qt=$(__llm_format_query_time "$us")
__llm_format_query_time() {
    local us="${1:-0}"
    if [[ $us -lt 1000 ]]; then
        printf '%dµs' "$us"
    elif [[ $us -lt 1000000 ]]; then
        printf '%d.%dms' "$(( us / 1000 ))" "$(( (us % 1000) / 100 ))"
    else
        printf '%d.%02ds' "$(( us / 1000000 ))" "$(( (us % 1000000) / 10000 ))"
    fi
}

# Execute a DB _impl function, measure execution time, and wrap the result
# in a JSON envelope with timing metadata.
#
# Success → stdout:  {"data": <result>, "rows": N, "query_time": "12.3ms"}
# Error   → stderr:  original error JSON + injected "query_time" field
#
# Slow-query auto-explain (llm_db_query only):
#   Set __GASH_DB_SLOW_QUERY=1 and __GASH_DB_EXPLAIN_ARGS=(filtered args)
#   in the caller's scope. When query_time ≥ 100ms, the envelope includes
#   a "slow_query_explain" field with the EXPLAIN output.
#
# Usage: __llm_db_envelope <impl_func> [args...]
__llm_db_envelope() {
    local _dbenv_func="$1"; shift
    local _dbenv_ts=${EPOCHREALTIME:-}

    local _dbenv_out _dbenv_rc _dbenv_stderr
    _dbenv_stderr=$(mktemp)
    _dbenv_out=$("$_dbenv_func" "$@" 2>"$_dbenv_stderr")
    _dbenv_rc=$?

    local _dbenv_t1=${EPOCHREALTIME:-}
    local _dbenv_us _dbenv_qt
    _dbenv_us=$(__llm_elapsed_us "$_dbenv_ts" "$_dbenv_t1")
    _dbenv_qt=$(__llm_format_query_time "$_dbenv_us")

    if [[ $_dbenv_rc -eq 0 ]]; then
        rm -f "$_dbenv_stderr"
        if __llm_has_jq; then
            if [[ "${__GASH_DB_SLOW_QUERY-}" == "1" ]] && [[ $_dbenv_us -ge 100000 ]]; then
                # >100ms: auto-explain for slow query
                local _dbenv_xpl
                _dbenv_xpl=$(__llm_db_explain_impl "${__GASH_DB_EXPLAIN_ARGS[@]}" 2>/dev/null) || true
                [[ -z "$_dbenv_xpl" ]] && _dbenv_xpl='[]'
                printf '%s' "$_dbenv_out" | jq -c \
                    --arg qt "$_dbenv_qt" \
                    --argjson xpl "$_dbenv_xpl" \
                    '{data: ., rows: length, query_time: $qt, slow_query_explain: $xpl}'
            else
                printf '%s' "$_dbenv_out" | jq -c \
                    --arg qt "$_dbenv_qt" \
                    '{data: ., rows: length, query_time: $qt}'
            fi
        else
            printf '%s\n' "$_dbenv_out"
        fi
    else
        # Error path: inject query_time into error JSON, preserve stderr channel
        if __llm_has_jq && [[ -s "$_dbenv_stderr" ]]; then
            jq -c --arg qt "$_dbenv_qt" '. + {query_time: $qt}' < "$_dbenv_stderr" >&2
        elif [[ -s "$_dbenv_stderr" ]]; then
            cat "$_dbenv_stderr" >&2
        fi
        rm -f "$_dbenv_stderr"
        return $_dbenv_rc
    fi
}

# Convert tab-separated input (first row = headers, subsequent rows = data)
# into a JSON array of objects. Emits "[]" for empty input or header-only input.
#
# Modes:
#   raw   — default. Assumes field values contain literal bytes (no escape
#           sequences). Escapes backslashes and quotes for JSON. Used for
#           psql -A output and anywhere else where the stream is byte-faithful.
#   mysql — Assumes field values come from `mysql --batch` WITHOUT `--raw`,
#           which pre-escapes `\\`, `\t`, `\n`, `\r`, `\0`, `\Z`. These happen
#           to coincide 1:1 with JSON string escape sequences (except `\0`/`\Z`
#           which we remap to \u escapes), so the backslash pass-through is
#           correct and tabs/newlines inside cell values stay safe.
#
# Limitation (raw mode): assumes no literal TAB or NEWLINE characters inside
# cell values — they would corrupt row/field parsing. Use mysql mode or the
# jsonb_agg wrapper (pgsql branch) when data may contain control chars.
#
# Usage: printf '%s\n' "$tsv" | __llm_tsv_to_json [raw|mysql]
__llm_tsv_to_json() {
    local mode="${1-raw}"
    awk -F'\t' -v mode="$mode" '
        function esc_raw(s,   r) {
            r = s
            gsub(/\\/, "\\\\", r)
            gsub(/"/, "\\\"", r)
            return r
        }
        function esc_mysql(s,   r) {
            # mysql --batch (no --raw) emits \\, \t, \n, \r as 2-char
            # escape sequences — identical to JSON string escapes, so
            # pass through. Only remap \0 and \Z (not valid JSON) and
            # escape any unescaped double quotes.
            r = s
            gsub(/\\0/, "\\\\u0000", r)
            gsub(/\\Z/, "\\\\u001a", r)
            gsub(/"/, "\\\"", r)
            return r
        }
        function esc(s) {
            if (mode == "mysql") return esc_mysql(s)
            return esc_raw(s)
        }
        BEGIN { started = 0; n = 0 }
        NR == 1 {
            n = NF
            for (i = 1; i <= NF; i++) cols[i] = esc($i)
            next
        }
        NF == 0 { next }
        {
            if (started == 0) {
                printf "["
                started = 1
            } else {
                printf ","
            }
            printf "{"
            for (i = 1; i <= n; i++) {
                if (i > 1) printf ","
                printf "\"%s\":\"%s\"", cols[i], esc($i)
            }
            printf "}"
        }
        END {
            if (started == 0) print "[]"
            else print "]"
        }
    '
}

# =============================================================================
# PUBLIC FUNCTIONS
# =============================================================================

# -----------------------------------------------------------------------------
# Core Helper
# -----------------------------------------------------------------------------

# Execute a command safely without recording in bash history.
# Validates the command against dangerous patterns before execution.
# Usage: llm_exec <command>
# Example: llm_exec "ls -la /tmp"
llm_exec() {
    if needs_help "llm_exec" "llm_exec <command>" "Execute command safely (no history, validated against dangerous patterns)" "${1-}"; then
        return 0
    fi

    local cmd="${1-}"

    if [[ -z "$cmd" ]]; then
        __llm_error "empty_command" "" "RETRY" "true" "Provide a command to execute"
        return 1
    fi

    # Validate command is safe
    if ! __llm_validate_command "$cmd"; then
        return 1
    fi

    # Execute without history
    __gash_no_history eval "$cmd"
}

# -----------------------------------------------------------------------------
# Directory & File Navigation
# -----------------------------------------------------------------------------

# Output a compact directory tree in JSON or text format.
# Usage: llm_tree [--text] [--depth N] [PATH]
# Example: llm_tree src/
# Example: llm_tree --text --depth 2 .
llm_tree() {
    if needs_help "llm_tree" "llm_tree [--text] [--depth N] [PATH]" "Compact directory tree (JSON default, --text for indented)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_tree_impl "$@"
}

__llm_tree_impl() {
    local text_mode=0
    local max_depth=3
    local target_path="."

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --text) text_mode=1; shift ;;
            --depth)
                max_depth="${2-3}"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --text or --depth N"
                return 1
                ;;
            *)
                target_path="$1"
                shift
                ;;
        esac
    done

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if [[ ! -d "$safe_path" ]]; then
        __llm_error "not_a_directory" "$safe_path" "RETRY" "true" "Provide a valid directory path"
        return 1
    fi

    if [[ $text_mode -eq 1 ]]; then
        # Text mode: use find with indentation
        find "$safe_path" -maxdepth "$max_depth" \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -name '__pycache__' -prune -o \
            -name '.cache' -prune -o \
            -type f -print -o -type d -print 2>/dev/null | \
            sort | \
            head -n 200 | \
            while IFS= read -r item; do
                # Calculate depth for indentation
                local rel="${item#$safe_path}"
                rel="${rel#/}"
                local depth
                depth=$(echo "$rel" | tr -cd '/' | wc -c)
                local indent=""
                for ((i=0; i<depth; i++)); do indent+="  "; done
                local name
                name="$(basename "$item")"
                if [[ -d "$item" ]]; then
                    echo "${indent}${name}/"
                else
                    echo "${indent}${name}"
                fi
            done
    else
        # JSON mode
        echo "{"
        echo "  \"path\": \"$safe_path\","
        echo "  \"type\": \"directory\","
        echo "  \"children\": ["

        local first=1
        find "$safe_path" -maxdepth 1 -mindepth 1 \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -print 2>/dev/null | \
            sort | \
            head -n 100 | \
            while IFS= read -r item; do
                local name
                name="$(basename "$item")"
                local type="file"
                [[ -d "$item" ]] && type="directory"

                if [[ $first -eq 1 ]]; then
                    first=0
                else
                    echo ","
                fi
                printf '    {"name": "%s", "type": "%s"}' "$name" "$type"
            done

        echo ""
        echo "  ]"
        echo "}"
    fi
}

# Find files by pattern, optimized for common use cases.
# Usage: llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]
# Example: llm_find "*.php" src/
# Example: llm_find --contains "class.*Controller" .
llm_find() {
    if needs_help "llm_find" "llm_find <pattern> [PATH] [--type f|d] [--contains REGEX]" "Find files by pattern (ignores node_modules, vendor, .git)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_find_impl "$@"
}

__llm_find_impl() {
    local pattern=""
    local target_path="."
    local file_type="f"
    local contains_regex=""
    local limit=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --type)
                file_type="${2-f}"
                shift 2
                ;;
            --contains)
                contains_regex="${2-}"
                shift 2
                ;;
            --limit)
                limit="${2-100}"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --type, --contains, or --limit"
                return 1
                ;;
            *)
                if [[ -z "$pattern" ]]; then
                    pattern="$1"
                else
                    target_path="$1"
                fi
                shift
                ;;
        esac
    done

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if [[ -n "$contains_regex" ]]; then
        # Search by content
        if __llm_has_rg; then
            rg -l --no-heading --color=never \
                --glob '!node_modules' --glob '!vendor' --glob '!.git' \
                "$contains_regex" "$safe_path" 2>/dev/null | head -n "$limit"
        else
            grep -rl --include='*.php' --include='*.js' --include='*.ts' \
                --include='*.py' --include='*.go' --include='*.java' \
                --exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=.git \
                "$contains_regex" "$safe_path" 2>/dev/null | head -n "$limit"
        fi
    else
        # Search by filename pattern
        find "$safe_path" \
            -name 'node_modules' -prune -o \
            -name 'vendor' -prune -o \
            -name '.git' -prune -o \
            -name '__pycache__' -prune -o \
            -type "$file_type" -name "$pattern" -print 2>/dev/null | \
            head -n "$limit"
    fi
}

# Grep with structured output, optimized for code search.
# Usage: llm_grep <pattern> [PATH] [--ext EXT1,EXT2] [--context N]
# Example: llm_grep "TODO|FIXME" src/ --ext php,js
llm_grep() {
    if needs_help "llm_grep" "llm_grep <pattern> [PATH] [--ext EXT1,EXT2] [--context N]" "Search code with file:line:content output" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_grep_impl "$@"
}

__llm_grep_impl() {
    local pattern=""
    local target_path="."
    local extensions=""
    local context=0
    local limit=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ext)
                extensions="${2-}"
                shift 2
                ;;
            --context|-C)
                context="${2-0}"
                shift 2
                ;;
            --limit)
                limit="${2-100}"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --ext, --context, or --limit"
                return 1
                ;;
            *)
                if [[ -z "$pattern" ]]; then
                    pattern="$1"
                else
                    target_path="$1"
                fi
                shift
                ;;
        esac
    done

    if [[ -z "$pattern" ]]; then
        __llm_error "missing_pattern" "" "RETRY" "true" "Provide a search pattern as first argument"
        return 1
    fi

    # Validate path
    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if __llm_has_rg; then
        local rg_args=(-n --no-heading --color=never)
        rg_args+=(--glob '!node_modules' --glob '!vendor' --glob '!.git')

        if [[ -n "$extensions" ]]; then
            IFS=',' read -ra exts <<< "$extensions"
            for ext in "${exts[@]}"; do
                rg_args+=(--glob "*.$ext")
            done
        fi

        [[ $context -gt 0 ]] && rg_args+=(-C "$context")

        rg "${rg_args[@]}" "$pattern" "$safe_path" 2>/dev/null | head -n "$limit"
    else
        local grep_args=(-rn --color=never)
        grep_args+=(--exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=.git)

        if [[ -n "$extensions" ]]; then
            IFS=',' read -ra exts <<< "$extensions"
            for ext in "${exts[@]}"; do
                grep_args+=(--include="*.$ext")
            done
        fi

        [[ $context -gt 0 ]] && grep_args+=(-C "$context")

        grep "${grep_args[@]}" "$pattern" "$safe_path" 2>/dev/null | head -n "$limit"
    fi
}

# -----------------------------------------------------------------------------
# Database Functions (Read-Only)
# -----------------------------------------------------------------------------

# Resolve a named database connection to its components via namerefs.
# Fetches the URL from config, parses it, decodes password, resolves database name.
# Usage: __llm_resolve_db <connection> <database> <out_drv> <out_usr> <out_pw> <out_hst> <out_prt> <out_dbn>
__llm_resolve_db() {
    local conn="$1" db_name="$2"
    local -n _rd_drv="$3" _rd_usr="$4" _rd_pw="$5" _rd_hst="$6" _rd_prt="$7" _rd_dbn="$8"

    local db_url
    db_url=$(__gash_get_db_url "$conn") || {
        __llm_error "no_db_config" "Connection '$conn' not found" "STOP" "true" "Ask user which database connection to use. Connections are configured in ~/.gash_env"
        return 1
    }

    __gash_parse_db_url "$db_url" _rd_drv _rd_usr _rd_pw _rd_hst _rd_prt _rd_dbn
    _rd_pw=$(__gash_url_decode "$_rd_pw")

    # Use explicit database name if provided, otherwise fall back to URL or .target-database
    if [[ -n "$db_name" ]]; then
        _rd_dbn="$db_name"
    elif [[ -z "$_rd_dbn" ]]; then
        if [[ -f ".target-database" ]]; then
            _rd_dbn="$(<.target-database)"
            _rd_dbn="${_rd_dbn%%[[:space:]]*}"
        fi
    fi

    if [[ -z "$_rd_dbn" ]]; then
        __llm_error "no_database" "Database name not specified" "STOP" "true" "Ask user for database name. Use -d option or specify in connection URL"
        return 1
    fi
}

# Check if sqlite3 is available.
# Usage: __llm_has_sqlite
__llm_has_sqlite() {
    type -P sqlite3 >/dev/null 2>&1
}

# Execute a SQLite query with JSON output.
# Usage: __llm_sqlite_query <query> <sqlite_file> [max_rows]
__llm_sqlite_query() {
    local query="${1-}"
    local sqlite_file="${2-}"
    local max_rows="${3-100}"

    # Check sqlite3 is installed
    if ! __llm_has_sqlite; then
        __llm_error "sqlite_not_found" "SQLite3 CLI not installed" "FATAL" "false" "sqlite3 binary not found. Install with: sudo apt install sqlite3"
        return 1
    fi

    # Validate file path
    if [[ -z "$sqlite_file" ]]; then
        __llm_error "missing_sqlite_file" "" "RETRY" "true" "Provide SQLite file path with -f option"
        return 1
    fi

    local safe_path
    safe_path="$(__llm_validate_path "$sqlite_file")" || return 1

    if [[ ! -f "$safe_path" ]]; then
        __llm_error "sqlite_file_not_found" "$safe_path" "RETRY" "true" "SQLite file does not exist. Check the path"
        return 1
    fi

    # Verify it's a valid SQLite file
    if ! file "$safe_path" 2>/dev/null | grep -q "SQLite"; then
        __llm_error "invalid_sqlite_file" "$safe_path is not a valid SQLite database" "RETRY" "true" "Provide a valid SQLite database file"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # Add LIMIT if not present in SELECT queries
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"
    if [[ "$upper_query" =~ ^[[:space:]]*SELECT ]] && [[ ! "$upper_query" =~ LIMIT ]]; then
        query="$query LIMIT $max_rows"
    fi

    # Execute query with JSON output
    local sqlite_output sqlite_stderr sqlite_exit
    sqlite_stderr=$(mktemp)
    sqlite_output=$(sqlite3 -json -readonly "$safe_path" "$query" 2>"$sqlite_stderr")
    sqlite_exit=$?

    # Check for errors
    if [[ $sqlite_exit -ne 0 ]]; then
        local err_msg=""
        if [[ -s "$sqlite_stderr" ]]; then
            err_msg=$(<"$sqlite_stderr")
        fi
        rm -f "$sqlite_stderr"
        err_msg=$(__llm_clean_error_msg "$err_msg")
        if [[ -n "$err_msg" ]]; then
            __llm_error "sqlite_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
        else
            __llm_error "sqlite_error" "Query failed with exit code $sqlite_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
        fi
        return 1
    fi
    rm -f "$sqlite_stderr"

    echo "$sqlite_output"
}

# Execute a read-only database query with JSON output.
# Usage: llm_db_query <query> [-d DATABASE] [-c CONNECTION] [-f SQLITE_FILE]
# Example: llm_db_query "SELECT * FROM users LIMIT 5" -c myconn
# Example: llm_db_query "SELECT * FROM users" -f /path/to/db.sqlite
llm_db_query() {
    if needs_help "llm_db_query" "llm_db_query <query> [-c CONNECTION] [-f SQLITE_FILE] [-r ROWS]" "Execute read-only database query (JSON envelope with timing). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    # Build explain args: copy "$@" but strip -r/--rows (not accepted by explain)
    local __GASH_DB_EXPLAIN_ARGS=()
    local _qw_args=("$@") _qw_i=0
    while [[ $_qw_i -lt ${#_qw_args[@]} ]]; do
        case "${_qw_args[$_qw_i]}" in
            -r|--rows) (( _qw_i += 2 )) ;;
            *) __GASH_DB_EXPLAIN_ARGS+=("${_qw_args[$_qw_i]}"); (( _qw_i += 1 )) ;;
        esac
    done
    local __GASH_DB_SLOW_QUERY=1

    __gash_no_history __llm_db_envelope __llm_db_query_impl "$@"
}

__llm_db_query_impl() {
    local query=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local max_rows=100

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--database)
                database="${2-}"
                shift 2
                ;;
            -c|--connection)
                connection="${2-default}"
                shift 2
                ;;
            -f|--file)
                sqlite_file="${2-}"
                shift 2
                ;;
            -r|--rows)
                max_rows="${2-100}"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use -d DATABASE, -c CONNECTION, -f SQLITE_FILE, or -r ROWS"
                return 1
                ;;
            *)
                query="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$query" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query as first argument"
        return 1
    fi

    # Security: Only allow read-only queries (validate BEFORE loading config)
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"

    # Block write operations
    if [[ "$upper_query" =~ ^[[:space:]]*(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE) ]]; then
        __llm_error "write_operation_blocked" "Only SELECT, SHOW, DESCRIBE, EXPLAIN allowed" "FATAL" "false" "Write operations are blocked for safety"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # SQLite mode: use -f option
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "$query" "$sqlite_file" "$max_rows"
        return $?
    fi

    # Resolve database connection
    local db_driver db_user db_pass db_host db_port db_database
    __llm_resolve_db "$connection" "$database" db_driver db_user db_pass db_host db_port db_database || return 1
    database="$db_database"

    # Add LIMIT if not present in SELECT queries
    if [[ "$upper_query" =~ ^[[:space:]]*SELECT ]] && [[ ! "$upper_query" =~ LIMIT ]]; then
        query="$query LIMIT $max_rows"
    fi

    case "$db_driver" in
        mysql|mariadb)
            local mysql_bin
            mysql_bin=$(type -P mariadb 2>/dev/null) || mysql_bin=$(type -P mysql 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __llm_error "mysql_not_found" "MySQL/MariaDB client not installed" "FATAL" "false" "mysql/mariadb binary not found. User must install: sudo apt install mariadb-client"
                return 1
            fi

            # Execute query with TSV output, capturing stderr for errors.
            # Intentionally NOT --raw: mysql --batch without --raw escapes
            # tab/newline/backslash/NUL as \t \n \\ \0 — these are 1:1
            # JSON-compatible (handled by __llm_tsv_to_json mysql mode),
            # so cell values with embedded control chars survive round-trip.
            local mysql_output mysql_stderr mysql_exit
            mysql_stderr=$(mktemp)
            mysql_output=$("$mysql_bin" -u"$db_user" -p"$db_pass" -h"$db_host" -P"$db_port" "$database" \
                --default-character-set=utf8mb4 \
                -e "$query" \
                --batch 2>"$mysql_stderr")
            mysql_exit=$?

            # Read stderr and clean up temp file
            local err_msg=""
            if [[ -s "$mysql_stderr" ]]; then
                err_msg=$(<"$mysql_stderr")
            fi
            rm -f "$mysql_stderr"

            # Filter out warnings (not errors)
            local filtered_err
            filtered_err=$(__llm_clean_error_msg "$err_msg" --mysql)

            # If mysql exited with error, always return structured error
            if [[ $mysql_exit -ne 0 ]]; then
                if [[ -n "$filtered_err" ]]; then
                    __llm_error "mysql_error" "$filtered_err" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "mysql_error" "Query failed with exit code $mysql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi

            # If there's a real error message (not just warnings), report it
            if [[ -n "$filtered_err" ]]; then
                __llm_error "mysql_error" "$filtered_err" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                return 1
            fi

            # Convert TSV output to JSON array (shared helper, mysql mode
            # preserves \t/\n/\\ pre-escaped sequences from --batch)
            printf '%s\n' "$mysql_output" | __llm_tsv_to_json mysql
            ;;
        pgsql)
            if ! type -P psql >/dev/null 2>&1; then
                __llm_error "psql_not_found" "PostgreSQL client not installed" "FATAL" "false" "psql binary not found. User must install: sudo apt install postgresql-client"
                return 1
            fi

            # Strategy: for SELECT/WITH/VALUES/TABLE (wrappable) queries, wrap
            # with `jsonb_agg(to_jsonb(...))` so PostgreSQL itself serializes
            # the result as native JSON. This preserves PG types (int, bool,
            # null, arrays, nested jsonb) and — crucially — handles tabs,
            # newlines and backslashes inside cell values without any TSV
            # parsing fragility. Non-wrappable statements (SHOW, etc.) fall
            # back to the shared TSV helper.
            local psql_output psql_stderr psql_exit use_jsonb=0 exec_query
            psql_stderr=$(mktemp)

            if [[ "$upper_query" =~ ^[[:space:]]*(SELECT|WITH|VALUES|TABLE)[[:space:]] ]]; then
                use_jsonb=1
                exec_query="SELECT COALESCE(jsonb_agg(to_jsonb(__gash_row)), '[]'::jsonb) FROM (${query}) __gash_row"
                psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                    -At -c "$exec_query" 2>"$psql_stderr")
            else
                # Fallback: header row + no footer, tab-separated.
                psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                    -P footer=off -A -F $'\t' -c "$query" 2>"$psql_stderr")
            fi
            psql_exit=$?

            # Check for PostgreSQL errors. Only exit code failures count as
            # errors — NOTICE/WARNING lines also land on stderr but do not
            # fail the query (e.g. identifier truncation, deprecated feature
            # warnings), so ignore stderr when psql_exit == 0.
            if [[ $psql_exit -ne 0 ]]; then
                local err_msg=""
                [[ -s "$psql_stderr" ]] && err_msg=$(<"$psql_stderr")
                rm -f "$psql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg")
                if [[ -n "$err_msg" ]]; then
                    __llm_error "pgsql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "pgsql_error" "Query failed with exit code $psql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi
            rm -f "$psql_stderr"

            if [[ $use_jsonb -eq 1 ]]; then
                # psql -At emitted the jsonb_agg result as raw JSON text
                printf '%s\n' "$psql_output"
            else
                # TSV fallback (shared helper, symmetric with mysql)
                printf '%s\n' "$psql_output" | __llm_tsv_to_json raw
            fi
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver. Only mysql, mariadb, pgsql are supported"
            return 1
            ;;
    esac
}

# List all tables in the database.
# Usage: llm_db_tables [-c CONNECTION] [-f SQLITE_FILE]
llm_db_tables() {
    if needs_help "llm_db_tables" "llm_db_tables [-c CONNECTION] [-f SQLITE_FILE]" "List database tables (JSON array). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_tables_impl "$@"
}

__llm_db_tables_impl() {
    local database=""
    local connection="default"
    local sqlite_file=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--database) database="${2-}"; shift 2 ;;
            -c|--connection) connection="${2-default}"; shift 2 ;;
            -f|--file) sqlite_file="${2-}"; shift 2 ;;
            *) shift ;;
        esac
    done

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        local result
        result=$(__llm_sqlite_query "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name" "$sqlite_file" 1000) || return 1
        if __llm_has_jq; then
            printf '%s' "$result" | jq -c '[.[].name]'
        else
            printf '%s\n' "$result"
        fi
        return 0
    fi

    # Get driver from connection to determine query type
    local db_url
    db_url=$(__gash_get_db_url "$connection") || {
        __llm_error "no_db_config" "Connection '$connection' not found" "STOP" "true" "Ask user which database connection to use"
        return 1
    }

    local db_driver _u _p _h _port _db
    __gash_parse_db_url "$db_url" db_driver _u _p _h _port _db

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    local result
    case "$db_driver" in
        mysql|mariadb)
            result=$(__llm_db_query_impl "SHOW TABLES" "${args[@]}" 2>/dev/null) || return 1
            if __llm_has_jq; then
                printf '%s' "$result" | jq -c '[.[].[] | values]'
            else
                # Fallback: extract table names without jq
                printf '%s' "$result" | sed 's/^\[{[^}]*:"\([^"]*\)"}\(,\|]\)/"\1"\2/g'
            fi
            ;;
        pgsql)
            result=$(__llm_db_query_impl "SELECT tablename FROM pg_tables WHERE schemaname='public'" "${args[@]}" 2>/dev/null) || return 1
            if __llm_has_jq; then
                printf '%s' "$result" | jq -c '[.[].tablename]'
            else
                printf '%s\n' "$result"
            fi
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver"
            return 1
            ;;
    esac
}

# Show schema for a table.
# Usage: llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]
llm_db_schema() {
    if needs_help "llm_db_schema" "llm_db_schema <table> [-c CONNECTION] [-f SQLITE_FILE]" "Show table schema (JSON). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_schema_impl "$@"
}

__llm_db_schema_impl() {
    local table=""
    local database=""
    local connection="default"
    local sqlite_file=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--database) database="${2-}"; shift 2 ;;
            -c|--connection) connection="${2-default}"; shift 2 ;;
            -f|--file) sqlite_file="${2-}"; shift 2 ;;
            -*) shift ;;
            *) table="$1"; shift ;;
        esac
    done

    if [[ -z "$table" ]]; then
        __llm_error "missing_table" "" "RETRY" "true" "Provide table name as first argument"
        return 1
    fi

    # Sanitize table name (alphanumeric and underscore only)
    if [[ ! "$table" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
        __llm_error "invalid_table_name" "Table name must be alphanumeric with underscores" "RETRY" "true" "Use only letters, numbers, and underscores"
        return 1
    fi

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "SELECT cid, name, type, \"notnull\", dflt_value, pk FROM pragma_table_info('$table')" "$sqlite_file" 1000
        return $?
    fi

    # Get driver from connection
    local db_url
    db_url=$(__gash_get_db_url "$connection") || {
        __llm_error "no_db_config" "Connection '$connection' not found" "STOP" "true" "Ask user which database connection to use"
        return 1
    }

    local db_driver _u _p _h _port _db
    __gash_parse_db_url "$db_url" db_driver _u _p _h _port _db

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    case "$db_driver" in
        mysql|mariadb)
            __llm_db_query_impl "DESCRIBE $table" "${args[@]}"
            ;;
        pgsql)
            __llm_db_query_impl "SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name='$table' AND table_schema='public' ORDER BY ordinal_position" "${args[@]}"
            ;;
        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver"
            return 1
            ;;
    esac
}

# Get sample rows from a table.
# Usage: llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [--limit N]
llm_db_sample() {
    if needs_help "llm_db_sample" "llm_db_sample <table> [-c CONNECTION] [-f SQLITE_FILE] [--limit N]" "Get sample rows from table (default 5 rows). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_sample_impl "$@"
}

__llm_db_sample_impl() {
    local table=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local limit=5

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--database) database="${2-}"; shift 2 ;;
            -c|--connection) connection="${2-default}"; shift 2 ;;
            -f|--file) sqlite_file="${2-}"; shift 2 ;;
            --limit) limit="${2-5}"; shift 2 ;;
            -*) shift ;;
            *) table="$1"; shift ;;
        esac
    done

    if [[ -z "$table" ]]; then
        __llm_error "missing_table" "" "RETRY" "true" "Provide table name as first argument"
        return 1
    fi

    # Sanitize table name
    if [[ ! "$table" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
        __llm_error "invalid_table_name" "Table name must be alphanumeric with underscores" "RETRY" "true" "Use only letters, numbers, and underscores"
        return 1
    fi

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        __llm_sqlite_query "SELECT * FROM $table LIMIT $limit" "$sqlite_file" "$limit"
        return $?
    fi

    local args=()
    [[ -n "$database" ]] && args+=(-d "$database")
    args+=(-c "$connection")

    __llm_db_query_impl "SELECT * FROM $table LIMIT $limit" "${args[@]}"
}

# Explain a query execution plan.
# Usage: llm_db_explain <QUERY> [-c CONNECTION] [-f SQLITE_FILE] [--analyze]
llm_db_explain() {
    if needs_help "llm_db_explain" "llm_db_explain <QUERY> [-c CONNECTION] [-f SQLITE_FILE] [--analyze]" "Analyze query execution plan (EXPLAIN). Use -c for MySQL/PostgreSQL, -f for SQLite" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_db_envelope __llm_db_explain_impl "$@"
}

__llm_db_explain_impl() {
    local query=""
    local database=""
    local connection="default"
    local sqlite_file=""
    local analyze=0

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--database)
                database="${2-}"
                shift 2
                ;;
            -c|--connection)
                connection="${2-default}"
                shift 2
                ;;
            -f|--file)
                sqlite_file="${2-}"
                shift 2
                ;;
            --analyze)
                analyze=1
                shift
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use -c CONNECTION, -f SQLITE_FILE, or --analyze"
                return 1
                ;;
            *)
                query="$1"
                shift
                ;;
        esac
    done

    if [[ -z "$query" ]]; then
        __llm_error "missing_query" "" "RETRY" "true" "Provide a SQL query to analyze as first argument"
        return 1
    fi

    # Security: Only allow read-only queries
    local upper_query
    upper_query="$(echo "$query" | tr '[:lower:]' '[:upper:]')"

    # Block write operations
    if [[ "$upper_query" =~ ^[[:space:]]*(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE) ]]; then
        __llm_error "write_operation_blocked" "Only SELECT queries can be explained" "FATAL" "false" "EXPLAIN only works with SELECT queries for safety"
        return 1
    fi

    # Block semicolon (multiple statements)
    if [[ "$query" == *";"* ]]; then
        __llm_error "multiple_statements_blocked" "Only single SQL statements allowed" "FATAL" "false" "Remove semicolons - only one statement per query"
        return 1
    fi

    # SQLite mode
    if [[ -n "$sqlite_file" ]]; then
        # Check sqlite3 is installed
        if ! __llm_has_sqlite; then
            __llm_error "sqlite_not_found" "SQLite3 CLI not installed" "FATAL" "false" "sqlite3 binary not found. Install with: sudo apt install sqlite3"
            return 1
        fi

        # Validate file path
        local safe_path
        safe_path="$(__llm_validate_path "$sqlite_file")" || return 1

        if [[ ! -f "$safe_path" ]]; then
            __llm_error "sqlite_file_not_found" "$safe_path" "RETRY" "true" "SQLite file does not exist. Check the path"
            return 1
        fi

        # SQLite EXPLAIN QUERY PLAN is always available
        local explain_cmd="EXPLAIN QUERY PLAN"

        local sqlite_output sqlite_stderr sqlite_exit
        sqlite_stderr=$(mktemp)
        sqlite_output=$(sqlite3 -json -readonly "$safe_path" "$explain_cmd $query" 2>"$sqlite_stderr")
        sqlite_exit=$?

        if [[ $sqlite_exit -ne 0 ]]; then
            local err_msg=""
            if [[ -s "$sqlite_stderr" ]]; then
                err_msg=$(<"$sqlite_stderr")
            fi
            rm -f "$sqlite_stderr"
            err_msg=$(__llm_clean_error_msg "$err_msg")
            if [[ -n "$err_msg" ]]; then
                __llm_error "sqlite_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
            else
                __llm_error "sqlite_error" "Query failed with exit code $sqlite_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
            fi
            return 1
        fi
        rm -f "$sqlite_stderr"

        echo "$sqlite_output"
        return 0
    fi

    # Resolve database connection
    local db_driver db_user db_pass db_host db_port db_database
    __llm_resolve_db "$connection" "$database" db_driver db_user db_pass db_host db_port db_database || return 1
    database="$db_database"

    # Build EXPLAIN prefix based on driver and options
    local explain_prefix

    case "$db_driver" in
        mysql|mariadb)
            local mysql_bin
            mysql_bin=$(type -P mariadb 2>/dev/null) || mysql_bin=$(type -P mysql 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __llm_error "mysql_not_found" "MySQL/MariaDB client not installed" "FATAL" "false" "mysql/mariadb binary not found. User must install: sudo apt install mariadb-client"
                return 1
            fi

            # Detect if it's MariaDB or MySQL (MariaDB uses ANALYZE, MySQL uses EXPLAIN ANALYZE)
            local is_mariadb=0
            local version_output
            version_output=$("$mysql_bin" -u"$db_user" -p"$db_pass" -h"$db_host" -P"$db_port" "$database" \
                --default-character-set=utf8mb4 -N -e "SELECT VERSION()" 2>/dev/null)
            [[ "$version_output" == *"MariaDB"* ]] && is_mariadb=1

            if [[ $analyze -eq 1 ]]; then
                if [[ $is_mariadb -eq 1 ]]; then
                    # MariaDB: ANALYZE FORMAT=JSON <query>
                    explain_prefix="ANALYZE FORMAT=JSON"
                else
                    # MySQL 8.0.18+: EXPLAIN ANALYZE <query>
                    explain_prefix="EXPLAIN ANALYZE"
                fi
            else
                explain_prefix="EXPLAIN FORMAT=JSON"
            fi

            # Execute EXPLAIN query
            local mysql_output mysql_stderr mysql_exit
            mysql_stderr=$(mktemp)
            mysql_output=$("$mysql_bin" -u"$db_user" -p"$db_pass" -h"$db_host" -P"$db_port" "$database" \
                --default-character-set=utf8mb4 \
                -e "$explain_prefix $query" 2>"$mysql_stderr")
            mysql_exit=$?

            # Check for errors (filter out password warning)
            if [[ $mysql_exit -ne 0 ]]; then
                local err_msg
                err_msg=$(<"$mysql_stderr")
                rm -f "$mysql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg" --mysql)
                if [[ -n "$err_msg" ]]; then
                    __llm_error "mysql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                    return 1
                fi
            fi
            rm -f "$mysql_stderr"

            # Output result (skip header line for JSON format)
            echo "$mysql_output" | tail -n +2
            ;;

        pgsql)
            if ! type -P psql >/dev/null 2>&1; then
                __llm_error "psql_not_found" "PostgreSQL client not installed" "FATAL" "false" "psql binary not found. User must install: sudo apt install postgresql-client"
                return 1
            fi

            if [[ $analyze -eq 1 ]]; then
                explain_prefix="EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)"
            else
                explain_prefix="EXPLAIN (FORMAT JSON)"
            fi

            # Execute EXPLAIN query.
            # -A unaligned + -t tuples-only: emit raw JSON text with no
            # column wrapping, no "+" continuations and no trailing footer —
            # required for the output to parse as valid JSON.
            local psql_output psql_stderr psql_exit
            psql_stderr=$(mktemp)
            psql_output=$(PGPASSWORD="$db_pass" psql -U "$db_user" -h "$db_host" -p "$db_port" -d "$database" \
                -At -c "$explain_prefix $query" 2>"$psql_stderr")
            psql_exit=$?

            # Check for errors. Only exit code failures count — NOTICE/WARNING
            # on stderr (identifier truncation, etc.) are ignored when exit=0.
            if [[ $psql_exit -ne 0 ]]; then
                local err_msg=""
                [[ -s "$psql_stderr" ]] && err_msg=$(<"$psql_stderr")
                rm -f "$psql_stderr"
                err_msg=$(__llm_clean_error_msg "$err_msg")
                if [[ -n "$err_msg" ]]; then
                    __llm_error "pgsql_error" "$err_msg" "RETRY" "true" "Query syntax error. Fix the SQL and retry"
                else
                    __llm_error "pgsql_error" "EXPLAIN failed with exit code $psql_exit" "RETRY" "true" "Query failed. Check SQL syntax and retry"
                fi
                return 1
            fi
            rm -f "$psql_stderr"

            echo "$psql_output"
            ;;

        *)
            __llm_error "unknown_db_type" "$db_driver" "STOP" "false" "Unsupported database driver. Only mysql, mariadb, pgsql are supported"
            return 1
            ;;
    esac
}

# -----------------------------------------------------------------------------
# Project Analysis
# -----------------------------------------------------------------------------

# Detect project type and return info as JSON.
# Usage: llm_project [PATH]
llm_project() {
    if needs_help "llm_project" "llm_project [PATH]" "Detect project type and info (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_project_impl "$@"
}

__llm_project_impl() {
    local target_path="${1-.}"

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    local project_type="unknown"
    local language=""
    local framework=""
    local has_tests="false"
    local entry_point=""
    local package_manager=""

    # Detect by config files
    if [[ -f "$safe_path/composer.json" ]]; then
        project_type="php"
        package_manager="composer"

        if [[ -f "$safe_path/artisan" ]]; then
            framework="laravel"
            entry_point="public/index.php"
        elif [[ -d "$safe_path/symfony" ]] || grep -q '"symfony/' "$safe_path/composer.json" 2>/dev/null; then
            framework="symfony"
            entry_point="public/index.php"
        fi

        [[ -d "$safe_path/tests" ]] && has_tests="true"

    elif [[ -f "$safe_path/package.json" ]]; then
        project_type="javascript"
        package_manager="npm"
        [[ -f "$safe_path/yarn.lock" ]] && package_manager="yarn"
        [[ -f "$safe_path/pnpm-lock.yaml" ]] && package_manager="pnpm"

        if grep -q '"next"' "$safe_path/package.json" 2>/dev/null; then
            framework="nextjs"
        elif grep -q '"react"' "$safe_path/package.json" 2>/dev/null; then
            framework="react"
        elif grep -q '"vue"' "$safe_path/package.json" 2>/dev/null; then
            framework="vue"
        elif grep -q '"express"' "$safe_path/package.json" 2>/dev/null; then
            framework="express"
        fi

        [[ -d "$safe_path/tests" ]] || [[ -d "$safe_path/__tests__" ]] && has_tests="true"

    elif [[ -f "$safe_path/requirements.txt" ]] || [[ -f "$safe_path/pyproject.toml" ]]; then
        project_type="python"
        package_manager="pip"
        [[ -f "$safe_path/pyproject.toml" ]] && package_manager="poetry"

        if [[ -f "$safe_path/manage.py" ]]; then
            framework="django"
        elif grep -q 'flask' "$safe_path/requirements.txt" 2>/dev/null; then
            framework="flask"
        fi

        [[ -d "$safe_path/tests" ]] && has_tests="true"

    elif [[ -f "$safe_path/go.mod" ]]; then
        project_type="go"
        package_manager="go"
        [[ -d "$safe_path/tests" ]] || find "$safe_path" -name "*_test.go" -quit 2>/dev/null && has_tests="true"

    elif [[ -f "$safe_path/Cargo.toml" ]]; then
        project_type="rust"
        package_manager="cargo"
        [[ -d "$safe_path/tests" ]] && has_tests="true"
    fi

    # Output JSON
    cat <<EOF
{
  "type": "$project_type",
  "framework": "$framework",
  "package_manager": "$package_manager",
  "has_tests": $has_tests,
  "entry_point": "$entry_point",
  "path": "$safe_path"
}
EOF
}

# List project dependencies.
# Usage: llm_deps [PATH] [--dev]
llm_deps() {
    if needs_help "llm_deps" "llm_deps [PATH] [--dev]" "List project dependencies (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_deps_impl "$@"
}

__llm_deps_impl() {
    local target_path="."
    local include_dev=0

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dev) include_dev=1; shift ;;
            *) target_path="$1"; shift ;;
        esac
    done

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if [[ -f "$safe_path/composer.json" ]]; then
        if __llm_has_jq; then
            if [[ $include_dev -eq 1 ]]; then
                jq '{manager:"composer",prod:.require|keys,dev:."require-dev"|keys}' "$safe_path/composer.json"
            else
                jq '{manager:"composer",dependencies:.require|keys}' "$safe_path/composer.json"
            fi
        else
            echo '{"manager":"composer","note":"install jq for detailed output"}'
        fi

    elif [[ -f "$safe_path/package.json" ]]; then
        if __llm_has_jq; then
            if [[ $include_dev -eq 1 ]]; then
                jq '{manager:"npm",prod:.dependencies|keys,dev:.devDependencies|keys}' "$safe_path/package.json"
            else
                jq '{manager:"npm",dependencies:.dependencies|keys}' "$safe_path/package.json"
            fi
        else
            echo '{"manager":"npm","note":"install jq for detailed output"}'
        fi

    elif [[ -f "$safe_path/requirements.txt" ]]; then
        echo '{"manager":"pip","dependencies":['
        grep -v '^#' "$safe_path/requirements.txt" | grep -v '^$' | \
            sed 's/[<>=].*//' | \
            awk '{printf "\"%s\",", $1}' | sed 's/,$//'
        echo ']}'
    else
        __llm_error "no_package_file" "No composer.json, package.json, or requirements.txt found" "CONTINUE" "false" "Not a package-managed project"
        return 1
    fi
}

# Read a config file (NOT .env for security).
# Usage: llm_config <file>
# Supports: .json, .yaml, .yml
llm_config() {
    if needs_help "llm_config" "llm_config <file>" "Read config file (JSON/YAML, NOT .env)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_config_impl "$@"
}

__llm_config_impl() {
    local file="${1-}"

    if [[ -z "$file" ]]; then
        __llm_error "missing_file" "" "RETRY" "true" "Provide a file path as argument"
        return 1
    fi

    # Security: Block .env files
    if __llm_is_secret_file "$file"; then
        __llm_error "secret_file_blocked" "Cannot read .env or credential files" "FATAL" "false" "This file contains secrets and cannot be read"
        return 1
    fi

    local safe_path
    safe_path="$(__llm_validate_path "$file")" || return 1

    if [[ ! -f "$safe_path" ]]; then
        __llm_error "file_not_found" "$safe_path" "RETRY" "true" "Check the file path and try again"
        return 1
    fi

    case "$safe_path" in
        *.json)
            cat "$safe_path"
            ;;
        *.yaml|*.yml)
            if __llm_has_jq && type -P yq >/dev/null 2>&1; then
                yq -o=json "$safe_path"
            else
                cat "$safe_path"
            fi
            ;;
        *)
            cat "$safe_path"
            ;;
    esac
}

# -----------------------------------------------------------------------------
# Git Functions
# -----------------------------------------------------------------------------

# Compact git status as JSON.
# Usage: llm_git_status [PATH]
llm_git_status() {
    if needs_help "llm_git_status" "llm_git_status [PATH]" "Compact git status (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_status_impl "$@"
}

__llm_git_status_impl() {
    local target_path="${1-.}"

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    if ! git -C "$safe_path" rev-parse --git-dir >/dev/null 2>&1; then
        __llm_error "not_a_git_repo" "$safe_path is not a git repository" "CONTINUE" "false" "Not inside a git repository. Skip git operations"
        return 1
    fi

    local branch
    branch="$(git -C "$safe_path" branch --show-current 2>/dev/null)"

    local ahead=0 behind=0
    local tracking
    tracking="$(git -C "$safe_path" rev-parse --abbrev-ref '@{upstream}' 2>/dev/null)"
    if [[ -n "$tracking" ]]; then
        ahead="$(git -C "$safe_path" rev-list --count '@{upstream}..HEAD' 2>/dev/null || echo 0)"
        behind="$(git -C "$safe_path" rev-list --count 'HEAD..@{upstream}' 2>/dev/null || echo 0)"
    fi

    local staged=() modified=() untracked=()

    while IFS= read -r line; do
        local status="${line:0:2}"
        local file="${line:3}"

        case "$status" in
            "A "*|"M "*|"D "*|"R "*) staged+=("$file") ;;
            " M"|" D") modified+=("$file") ;;
            "??") untracked+=("$file") ;;
            "AM"|"MM") staged+=("$file"); modified+=("$file") ;;
        esac
    done < <(git -C "$safe_path" status --porcelain 2>/dev/null)

    # Build JSON
    echo "{"
    echo "  \"branch\": \"$branch\","
    echo "  \"ahead\": $ahead,"
    echo "  \"behind\": $behind,"
    printf '  "staged": [%s],\n' "$(printf '"%s",' "${staged[@]}" | sed 's/,$//')"
    printf '  "modified": [%s],\n' "$(printf '"%s",' "${modified[@]}" | sed 's/,$//')"
    printf '  "untracked": [%s]\n' "$(printf '"%s",' "${untracked[@]}" | sed 's/,$//')"
    echo "}"
}

# Git diff with stats.
# Usage: llm_git_diff [--staged] [PATH]
llm_git_diff() {
    if needs_help "llm_git_diff" "llm_git_diff [--staged] [PATH]" "Git diff with stats" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_diff_impl "$@"
}

__llm_git_diff_impl() {
    local staged=0
    local target_path="."

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --staged) staged=1; shift ;;
            *) target_path="$1"; shift ;;
        esac
    done

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    local diff_args=(--stat --no-color)
    [[ $staged -eq 1 ]] && diff_args+=(--staged)

    git -C "$safe_path" diff "${diff_args[@]}" 2>/dev/null
}

# Recent git log as JSON.
# Usage: llm_git_log [--limit N] [PATH]
llm_git_log() {
    if needs_help "llm_git_log" "llm_git_log [--limit N] [PATH]" "Recent git log (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_log_impl "$@"
}

__llm_git_log_impl() {
    local limit=10
    local target_path="."

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --limit) limit="${2-10}"; shift 2 ;;
            *) target_path="$1"; shift ;;
        esac
    done

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    echo "["
    git -C "$safe_path" log --oneline -n "$limit" --format='{"hash":"%h","subject":"%s","author":"%an","date":"%ci"},' 2>/dev/null | \
        sed '$ s/,$//'
    echo "]"
}

# -----------------------------------------------------------------------------
# System Functions
# -----------------------------------------------------------------------------

# List ports in use.
# Usage: llm_ports [--listen]
llm_ports() {
    if needs_help "llm_ports" "llm_ports [--listen]" "List ports in use (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_ports_impl "$@"
}

__llm_ports_impl() {
    local listen_only=0

    [[ "${1-}" == "--listen" ]] && listen_only=1

    echo "["

    if type -P ss >/dev/null 2>&1; then
        local ss_args=(-tuln)
        ss "${ss_args[@]}" 2>/dev/null | tail -n +2 | \
            awk '{
                split($5, a, ":");
                port = a[length(a)];
                proto = $1;
                if (port ~ /^[0-9]+$/) {
                    printf "{\"port\":%s,\"proto\":\"%s\",\"state\":\"%s\"},\n", port, proto, $2
                }
            }' | sort -t: -k2 -n -u | sed '$ s/,$//'
    elif type -P netstat >/dev/null 2>&1; then
        netstat -tuln 2>/dev/null | tail -n +3 | \
            awk '{
                split($4, a, ":");
                port = a[length(a)];
                proto = $1;
                if (port ~ /^[0-9]+$/) {
                    printf "{\"port\":%s,\"proto\":\"%s\"},\n", port, proto
                }
            }' | sort -t: -k2 -n -u | sed '$ s/,$//'
    fi

    echo "]"
}

# List processes by name or port.
# Usage: llm_procs [--name NAME] [--port PORT]
llm_procs() {
    if needs_help "llm_procs" "llm_procs [--name NAME] [--port PORT]" "List processes (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_procs_impl "$@"
}

__llm_procs_impl() {
    local name=""
    local port=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --name) name="${2-}"; shift 2 ;;
            --port) port="${2-}"; shift 2 ;;
            *) shift ;;
        esac
    done

    echo "["

    if [[ -n "$port" ]]; then
        # Find process by port
        if type -P lsof >/dev/null 2>&1; then
            lsof -i :"$port" -t 2>/dev/null | while read -r pid; do
                local cmd
                cmd="$(ps -p "$pid" -o comm= 2>/dev/null)"
                echo "{\"pid\":$pid,\"name\":\"$cmd\",\"port\":$port},"
            done | sed '$ s/,$//'
        fi
    elif [[ -n "$name" ]]; then
        # Find process by name
        pgrep -f "$name" 2>/dev/null | while read -r pid; do
            local cmd
            cmd="$(ps -p "$pid" -o comm= 2>/dev/null)"
            echo "{\"pid\":$pid,\"name\":\"$cmd\"},"
        done | sed '$ s/,$//'
    else
        # List all (limited)
        ps aux --no-headers 2>/dev/null | head -n 20 | \
            awk '{printf "{\"pid\":%s,\"user\":\"%s\",\"cpu\":%s,\"mem\":%s,\"name\":\"%s\"},\n", $2, $1, $3, $4, $11}' | \
            sed '$ s/,$//'
    fi

    echo "]"
}

# List environment variables (filtered, no secrets).
# Usage: llm_env [--filter PATTERN]
llm_env() {
    if needs_help "llm_env" "llm_env [--filter PATTERN]" "List env vars (filtered, no secrets)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_env_impl "$@"
}

__llm_env_impl() {
    local filter="${1-}"

    # Secret patterns to exclude
    local -a secret_patterns=(
        'PASSWORD'
        'SECRET'
        'TOKEN'
        'KEY'
        'CREDENTIAL'
        'AUTH'
        'PRIVATE'
        'AWS_'
        'API_KEY'
        'DATABASE_URL'
        'DB_PASS'
    )

    echo "{"

    local first=1
    while IFS='=' read -r name value; do
        # Skip empty names
        [[ -z "$name" ]] && continue

        # Apply filter if specified
        if [[ -n "$filter" ]] && [[ ! "$name" =~ $filter ]]; then
            continue
        fi

        # Skip secrets
        local is_secret=0
        for pattern in "${secret_patterns[@]}"; do
            if [[ "$name" =~ $pattern ]]; then
                is_secret=1
                break
            fi
        done
        [[ $is_secret -eq 1 ]] && continue

        # Escape value for JSON
        value="${value//\\/\\\\}"
        value="${value//\"/\\\"}"
        value="${value//$'\n'/\\n}"

        [[ $first -eq 0 ]] && echo ","
        first=0
        printf '  "%s": "%s"' "$name" "$value"
    done < <(env | sort)

    echo ""
    echo "}"
}

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
  llm_exec "df -h"
  llm_exec "systemctl status nginx"

NOTES
  Validates commands against a dangerous-pattern blacklist.
  Excluded from bash history. Blocks: rm -rf /, dd, mkfs,
  fork bombs, and other destructive operations.
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
  llm_find PATTERN [PATH] [--type f|d] [--contains REGEX]

EXAMPLES
  # Find all PHP files
  llm_find "*.php"

  # Find config files containing "database"
  llm_find "*.conf" /etc --contains "database"

  # Find directories only
  llm_find "src*" . --type d
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
  llm_db_query "SQL" [-c CONNECTION] [-f SQLITE_FILE]

EXAMPLES
  # Query using the default connection
  llm_db_query "SELECT id, name FROM users LIMIT 5"

  # Query a named connection
  llm_db_query "SELECT * FROM orders WHERE status='pending'" -c legacy

  # Query a SQLite database (no connection config needed)
  llm_db_query "SELECT * FROM users" -f ./data/app.db

NOTES
  Read-only: only SELECT, SHOW, DESCRIBE, EXPLAIN are allowed.
  INSERT, UPDATE, DELETE, DROP, TRUNCATE, ALTER are blocked.
  Output format: JSON array of objects.
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
  llm_db_sample TABLE [-c CONNECTION] [-f SQLITE_FILE] [--limit N]

EXAMPLES
  llm_db_sample users -c default
  llm_db_sample orders -c legacy --limit 10
  llm_db_sample logs -f ./app.db --limit 3
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
  llm_config FILE

EXAMPLES
  llm_config package.json
  llm_config docker-compose.yml

NOTES
  Refuses to read .env files and other secret files for security.
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
  # {"branch":"main","ahead":0,"behind":0,"staged":[],"modified":["README.md"],"untracked":[]}
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
  Excludes variables matching secret patterns: PASSWORD, TOKEN,
  SECRET, KEY, API_KEY, CREDENTIAL, etc.
HELP

fi  # end help registration guard
