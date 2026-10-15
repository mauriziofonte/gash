#!/usr/bin/env bash

# Gash Module: LLM Utilities: security foundation
# Path/secret/command policy, error envelope, timing, TSV to JSON.
# Sourced by lib/modules/llm.sh.

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

# Paths that are never allowed: the path itself and everything below it.
__LLM_FORBIDDEN_PATHS=(
    '/proc'
    '/sys'
    '/dev'
    '/boot'
    '/root'
    '/etc/shadow'
    '/etc/gshadow'
    '/etc/passwd'
    '/etc/sudoers'
    '/etc/sudoers.d'
)
# Root's home also where it is not /root (macOS: /var/root).
__llm_root_home=~root
if [[ "$__llm_root_home" == /?* && "$__llm_root_home" != /root ]]; then
    __LLM_FORBIDDEN_PATHS+=("$__llm_root_home")
fi
unset __llm_root_home

# Secret files, matched case-insensitively against the base name (of the
# path as given and of its symlink target). Never read, never searched.
__LLM_SECRET_PATTERNS=(
    '.env' '.env.*' '*.env'
    '*.pem' '*.key' '*.p12' '*.pfx' '*.jks' '*.keystore' '*.kdbx' '*.ppk'
    'id_rsa*' 'id_dsa*' 'id_ecdsa*' 'id_ed25519*' '*_rsa' '*_dsa' '*_ecdsa' '*_ed25519'
    'credentials' 'credentials.*' 'secrets' 'secrets.*' '.gash_env'
    '.pgpass' '.netrc' '_netrc' '.git-credentials' '.npmrc' '.pypirc' '.htpasswd'
    '.my.cnf' '.boto' '.s3cfg' '.dockercfg' '.vault-token'
    'kubeconfig' '*.kubeconfig' '*.tfstate' '*.tfstate.*' '*.tfvars'
)

# Templates that share a secret name but hold no secrets.
__LLM_SECRET_EXCEPTIONS=('*.pub' '.env.example' '.env.sample' '.env.dist' '.env.template' '*.env.example')

# Directories whose whole content is secret (matched on the full path).
__LLM_SECRET_DIRS=('.ssh' '.gnupg' '.aws' '.azure' '.kube' '.docker' '.config/gcloud')

# git options that keep a repository's own configuration from running
# programs or paging: fsmonitor hook, hooks (post-index-change), external
# diff, pager. Used by llm_git_* and by the git wrapper inside llm_exec.
__LLM_GIT_SAFE_CONFIG=(-c core.fsmonitor=false -c core.hooksPath=/dev/null -c diff.external= -c core.pager=cat)

# Run git with __LLM_GIT_SAFE_CONFIG (never a shell function named git).
# GIT_OPTIONAL_LOCKS=0: status does not refresh (write) the index.
# Usage: __llm_git ARGS...
__llm_git() {
    GIT_OPTIONAL_LOCKS=0 command git "${__LLM_GIT_SAFE_CONFIG[@]}" "$@"
}

# Validate and sanitize a path.
# Returns sanitized path on stdout, returns 1 if path is forbidden.
# Usage: __llm_validate_path <path>
__llm_validate_path() {
    local path="${1-}"

    # Empty path defaults to current directory
    [[ -z "$path" ]] && {
        echo "."
        return 0
    }

    # Block path traversal attempts: ".." only counts when it is a whole
    # path component ("/tmp/../etc"), not a substring ("file..bak", "v1.2..3")
    if [[ "/$path/" == *"/../"* || "$path" == ".." || "$path" == "../"* || "$path" == *"/.." ]]; then
        __gash_json_error "path_traversal_blocked" "" "FATAL" "Path traversal (..) is not allowed for security reasons"
        return 1
    fi

    # Expand tilde safely
    path="${path/#\~/$HOME}"

    # Normalize path (resolve symlinks, remove double slashes)
    local normalized
    normalized="$(__llm_realpath "$path")" || normalized="$path"

    if __llm_path_forbidden "$path" "$normalized"; then
        __gash_json_error "forbidden_path" "$normalized" "FATAL" "This path is protected and cannot be accessed"
        return 1
    fi

    echo "$normalized"
    return 0
}

# Whether a path is in __LLM_FORBIDDEN_PATHS (whole components: /root, not
# /rootfs), both as written and as resolved. macOS resolves /etc, /tmp and
# /var to /private/...: that form is checked without the prefix too.
# NORMALIZED is the __llm_realpath form when the caller already has it.
# Usage: __llm_path_forbidden PATH [NORMALIZED]
__llm_path_forbidden() {
    local path="${1-}" normalized="${2-}" lexical forbidden candidate
    [[ -n "$normalized" ]] || normalized="$(__llm_realpath "$path")" || normalized="$path"
    lexical="$(__llm_realpath -s "$path")" || lexical="$path"
    for candidate in "$normalized" "${normalized#/private}" "$lexical"; do
        for forbidden in "${__LLM_FORBIDDEN_PATHS[@]}"; do
            if [[ "$candidate" == "$forbidden" || "$candidate" == "$forbidden"/* ]]; then
                return 0
            fi
        done
    done
    return 1
}

# Whether a walk starting at PATH would enter a protected tree: PATH is /
# or a directory above one of them. MODE "content" counts every forbidden
# path (grep -r /etc would read /etc/shadow), MODE "names" only the trees
# a listing could enter (/proc, /sys, /dev, /boot, /root, root's home).
# Usage: __llm_path_above_forbidden PATH content|names
__llm_path_above_forbidden() {
    local path="${1-}" mode="${2-content}" normalized forbidden candidate
    normalized="$(__llm_realpath "$path")" || normalized="$path"
    for candidate in "$normalized" "${normalized#/private}"; do
        candidate="${candidate%/}"
        [[ -z "$candidate" ]] && return 0
        for forbidden in "${__LLM_FORBIDDEN_PATHS[@]}"; do
            [[ "$mode" == names && "$forbidden" == /etc/* ]] && continue
            [[ "$forbidden" == "$candidate"/* ]] && return 0
        done
    done
    return 1
}

# Canonical absolute form of PATH, missing components allowed: what
# `realpath -m` prints (with -s: `realpath -m -s`, symlinks kept). Where
# realpath has no -m (BSD, older macOS, busybox) and fails, a pure-bash
# walk resolves the symlinks itself, so the path guards still see their
# targets. __LLM_REALPATH_M=0 skips realpath (tests).
# Usage: __llm_realpath [-s] PATH
__llm_realpath() {
    local lexical=0
    if [[ "${1-}" == -s ]]; then
        lexical=1
        shift
    fi
    local path="${1-}"
    [[ -z "$path" ]] && return 1
    if [[ "${__LLM_REALPATH_M-}" != 0 ]]; then
        if ((lexical)); then
            realpath -m -s -- "$path" 2>/dev/null && return 0
        else
            realpath -m -- "$path" 2>/dev/null && return 0
        fi
    fi
    __llm_realpath_walk "$lexical" "$path"
}

# Pure-bash part of __llm_realpath: one component at a time, following
# symlinks (plain readlink exists everywhere) unless LEXICAL is 1. Fails
# after 40 links (a loop).
# Usage: __llm_realpath_walk LEXICAL PATH
__llm_realpath_walk() {
    local lexical="$1" path="$2" out="" comp link hops=0 i=0
    [[ "$path" == /* ]] || path="${PWD%/}/$path"
    local -a todo=() more=()
    __llm_split_path todo "$path"
    while ((i < ${#todo[@]})); do
        comp="${todo[i]}"
        ((i += 1))
        case "$comp" in
            "" | .) continue ;;
            ..)
                out="${out%/*}"
                continue
                ;;
        esac
        if ((! lexical)) && [[ -L "$out/$comp" ]]; then
            ((++hops > 40)) && return 1
            link="$(readlink -- "$out/$comp")" || return 1
            [[ "$link" == /* ]] && out=""
            __llm_split_path more "$link"
            todo=("${more[@]}" "${todo[@]:i}")
            i=0
            continue
        fi
        out+="/$comp"
    done
    printf '%s\n' "${out:-/}"
}

# Split PATH on "/" into OUT (no word splitting, newlines kept).
# Usage: __llm_split_path OUT PATH
__llm_split_path() {
    local -n __sp_out="$1"
    local p="$2"
    __sp_out=()
    while [[ "$p" == */* ]]; do
        __sp_out+=("${p%%/*}")
        p="${p#*/}"
    done
    __sp_out+=("$p")
}

# Print a [client] INI block for mysql --defaults-extra-file consumption.
# Keeps credentials off argv: /proc/<pid>/cmdline is world-readable, so
# -p"$pass" would expose the password to any local process for the whole
# duration of the query.
# Usage: "$mysql_bin" --defaults-extra-file=<(__llm_mysql_client_ini U P H PORT) ...
__llm_mysql_client_ini() {
    local user="${1-}" pass="${2-}" host="${3-}" port="${4-}"
    pass="${pass//\\/\\\\}"
    pass="${pass//\"/\\\"}"
    printf '[client]\nuser=%s\npassword="%s"\nhost=%s\nport=%s\n' "$user" "$pass" "$host" "$port"
}

# Validate that a flag value is a plain positive integer.
# Values interpolated into SQL LIMIT clauses or `head -n` MUST pass this
# guard: a crafted "-r '5; DROP TABLE x'" would otherwise reach the engine.
# At most 9 digits: a longer value overflows bash or find arithmetic.
# Usage: __llm_validate_uint <flag_name> <value> || return 1
__llm_validate_uint() {
    local name="${1-}"
    local value="${2-}"
    if [[ ! "$value" =~ ^[0-9]{1,9}$ ]]; then
        __llm_error "invalid_${name}" "Value '$value' is not a positive integer below 1000000000" "RETRY" "true" "Provide a plain integer for ${name}"
        return 1
    fi
    return 0
}

# Database names reach the client as --database=NAME (MySQL) or PGDATABASE
# (PostgreSQL), never as a bare word; still, only plain names are accepted,
# so a value can never read as an option or a connection string.
# Usage: __llm_validate_db_name NAME SOURCE || return 1
__llm_validate_db_name() {
    local re='^[A-Za-z0-9_][A-Za-z0-9_$.-]{0,63}$'
    if [[ ! "${1-}" =~ $re ]]; then
        __llm_error "invalid_database_name" "${2-}: '${1-}'" "RETRY" "true" "A database name holds letters, digits, _ \$ . and - (64 at most, not starting with -); pass a valid -d NAME"
        return 1
    fi
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
    local -a words=()
    IFS=$' \t\n' read -r -d '' -a words <<<"$cmd" || true
    local normalized_cmd="${words[*]}"

    local pattern
    for pattern in "${__LLM_DANGEROUS_PATTERNS[@]}"; do
        if [[ "$normalized_cmd" =~ $pattern ]]; then
            __gash_json_error "dangerous_command_blocked" "Command matches dangerous pattern" "FATAL" "This command is blocked for safety. Do not attempt to bypass"
            return 1
        fi
    done

    return 0
}

# Check if a file is secret: its base name matches __LLM_SECRET_PATTERNS
# (case-insensitively, templates excepted) or it lives in one of
# __LLM_SECRET_DIRS. A symlink is judged by its target as well.
# Returns 0 if it's a secret file (should not be read), 1 otherwise.
# Usage: __llm_is_secret_file <filename>
__llm_is_secret_file() {
    local file="${1-}" resolved=""
    [[ -z "$file" ]] && return 1
    __llm_secret_name "$file" && return 0
    if [[ -e "$file" || -L "$file" ]]; then
        resolved="$(__llm_realpath "$file")" || resolved=""
        [[ -n "$resolved" ]] && __llm_secret_name "$resolved" && return 0
    fi
    return 1
}

# Name-only part of __llm_is_secret_file (no filesystem access).
__llm_secret_name() {
    local path="${1%/}" base pattern dir
    base="${path##*/}"
    base="${base,,}"
    for pattern in "${__LLM_SECRET_EXCEPTIONS[@]}"; do
        # shellcheck disable=SC2053
        [[ "$base" == $pattern ]] && return 1
    done
    for pattern in "${__LLM_SECRET_PATTERNS[@]}"; do
        # shellcheck disable=SC2053
        [[ "$base" == $pattern ]] && return 0
    done
    for dir in "${__LLM_SECRET_DIRS[@]}"; do
        [[ "/${path,,}/" == *"/$dir/"* ]] && return 0
    done
    return 1
}

# Turn a glob into a case-insensitive one for tools without -iname
# (grep --exclude): ".env" -> ".[eE][nN][vV]".
__llm_icase_glob() {
    local in="$1" out="" c i
    for ((i = 0; i < ${#in}; i++)); do
        c="${in:i:1}"
        if [[ "$c" == [a-zA-Z] ]]; then
            out+="[${c,,}${c^^}]"
        else
            out+="$c"
        fi
    done
    printf '%s' "$out"
}

# Exclusion arguments that keep secret files out of content searches.
# Usage: __llm_secret_excludes rg|grep OUT_ARRAY
__llm_secret_excludes() {
    local -n __se_out="$2"
    local pattern dir
    __se_out=()
    for pattern in "${__LLM_SECRET_PATTERNS[@]}"; do
        if [[ "$1" == rg ]]; then
            __se_out+=(--iglob "!$pattern")
        else
            __se_out+=("--exclude=$(__llm_icase_glob "$pattern")")
        fi
    done
    for dir in "${__LLM_SECRET_DIRS[@]}"; do
        if [[ "$1" == rg ]]; then
            __se_out+=(--iglob "!**/$dir/**")
        elif [[ "$dir" != */* ]]; then
            __se_out+=("--exclude-dir=$(__llm_icase_glob "$dir")")
        fi
    done
}

# --help or -h anywhere among ARGS: print the help (the full registered
# help when there is one) and return 0; otherwise return 1.
# Usage: if __llm_needs_help NAME USAGE DESCRIPTION "$@"; then return 0; fi
__llm_needs_help() {
    local program="$1" usage="$2" help="$3" arg
    shift 3
    for arg in "$@"; do
        if [[ "$arg" == --help || "$arg" == -h ]]; then
            needs_help "$program" "$usage" "$help" --help
            return 0
        fi
    done
    return 1
}

# Store a positional PATH in OUT; SEEN (a caller variable, empty at first)
# remembers one was given, so a second is unexpected_argument.
# Usage: __llm_take_path OUT SEEN VALUE || return 1
__llm_take_path() {
    local -n __tp_out="$1" __tp_seen="$2"
    if [[ -n "$__tp_seen" ]]; then
        __llm_error "unexpected_argument" "$3" "RETRY" "true" "Pass at most one PATH"
        return 1
    fi
    __tp_out="$3"
    __tp_seen=1
}

# Argument parsing for functions that take at most one PATH and no
# options: stores it in OUT ("." when absent); an option is
# unknown_option, a second path unexpected_argument.
# Usage: __llm_path_arg OUT HINT "$@" || return 1
__llm_path_arg() {
    local -n __pa_out="$1"
    local hint="$2"
    shift 2
    __pa_out="."
    local seen=0
    while (($# > 0)); do
        if [[ "$1" == -* ]]; then
            __llm_error "unknown_option" "$1" "RETRY" "true" "$hint"
            return 1
        fi
        if ((seen)); then
            __llm_error "unexpected_argument" "$1" "RETRY" "true" "$hint"
            return 1
        fi
        __pa_out="$1"
        seen=1
        shift
    done
    return 0
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
    # The 4th argument (recoverable) is kept for call-site compatibility;
    # __gash_json_error derives it from the action.
    __gash_json_error "${1-}" "${2-}" "${3-STOP}" "${5-}"
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

# Compute elapsed microseconds between two $EPOCHREALTIME values.
# Pure integer arithmetic — zero forks. Returns 0 if either timestamp is empty.
# Usage: local us=$(__llm_elapsed_us "$start" "$end")
__llm_elapsed_us() {
    local t0="${1:-}" t1="${2:-}"
    [[ -z "$t0" || -z "$t1" ]] && {
        echo 0
        return
    }

    # The separator is the locale's decimal point: "." or "," (it_IT, de_DE).
    local s0="${t0%%[.,]*}" f0="${t0#*[.,]}" s1="${t1%%[.,]*}" f1="${t1#*[.,]}"
    # Pad fractional part to 6 digits (EPOCHREALTIME gives 6)
    while [[ ${#f0} -lt 6 ]]; do f0="${f0}0"; done
    while [[ ${#f1} -lt 6 ]]; do f1="${f1}0"; done
    # Force base-10 (leading zeros would trigger octal)
    local us=$(((10#$s1 - 10#$s0) * 1000000 + 10#$f1 - 10#$f0))
    [[ $us -lt 0 ]] && us=0
    echo "$us"
}

# $EPOCHREALTIME stand-in for bash < 5.0: GNU date with microseconds,
# empty when unavailable (the elapsed time then reads 0).
__llm_epoch_fallback() {
    local t
    t=$(date +%s.%6N 2>/dev/null) || return 0
    [[ "$t" =~ ^[0-9]+\.[0-9]{6}$ ]] && printf '%s' "$t"
    return 0
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
        printf '%d.%dms' "$((us / 1000))" "$(((us % 1000) / 100))"
    else
        printf '%d.%02ds' "$((us / 1000000))" "$(((us % 1000000) / 10000))"
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
    local _dbenv_func="$1"
    shift
    local _dbenv_ts=${EPOCHREALTIME:-}
    [[ -n "$_dbenv_ts" ]] || _dbenv_ts=$(__llm_epoch_fallback)

    local _dbenv_out _dbenv_rc _dbenv_stderr
    _dbenv_stderr=$(mktemp)
    _dbenv_out=$("$_dbenv_func" "$@" 2>"$_dbenv_stderr")
    _dbenv_rc=$?

    local _dbenv_t1=${EPOCHREALTIME:-}
    [[ -n "$_dbenv_t1" ]] || _dbenv_t1=$(__llm_epoch_fallback)
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
                jq -e . >/dev/null 2>&1 <<<"$_dbenv_xpl" || _dbenv_xpl='[]'
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
            jq -c --arg qt "$_dbenv_qt" '. + {query_time: $qt}' <"$_dbenv_stderr" >&2
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
        function esc_mysql(s,   r, i, n, c, d) {
            # mysql --batch (no --raw) emits \\, \t, \n, \r as 2-char
            # escape sequences, identical to JSON string escapes, so they
            # pass through. \0 and \Z (not valid JSON) become \u escapes,
            # any other backslash a literal one. Scanned left to right: the
            # "\0" inside "\\0" is an escaped backslash and a digit.
            if (index(s, "\\") == 0) {
                r = s
                gsub(/"/, "\\\"", r)
                return r
            }
            r = ""
            n = length(s)
            for (i = 1; i <= n; i++) {
                c = substr(s, i, 1)
                if (c == "\"") {
                    r = r "\\\""
                } else if (c != "\\") {
                    r = r c
                } else if (i == n) {
                    r = r "\\\\"
                } else {
                    d = substr(s, ++i, 1)
                    if (d == "0") r = r "\\u0000"
                    else if (d == "Z") r = r "\\u001a"
                    else if (d == "n" || d == "t" || d == "r" || d == "\\") r = r "\\" d
                    else r = r "\\\\" d
                }
            }
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
