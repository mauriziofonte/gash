#!/usr/bin/env bash

# =============================================================================
# Gash Core: Configuration Parser
# =============================================================================
#
# Parses ~/.gash_env for SSH keys, database connections, AI providers and
# named remote hosts.
#
# File format:
#   SSH:keypath=passphrase
#   DB:name=driver://user:password@host:port/database
#   AI:provider=api_token
#   HOST:alias=user@host
#   GASH_SETTING=value          (see __GASH_SETTINGS_KNOWN)
#
# Diagnostics never echo a line's value: a malformed entry may hold a
# token or a password, so warnings name the line number and the key only.
#
# Internal functions:
#   __gash_load_env()            - Load and cache ~/.gash_env
#   __gash_get_ssh_keys()        - Get SSH key entries
#   __gash_get_db_url()          - Get database URL by connection name
#   __gash_parse_db_url()        - Parse URL into components
#   __gash_url_decode()          - Decode %XX sequences
#   __gash_url_encode()          - Encode special characters
#   __gash_check_env_perms()     - Check file permissions
#   __gash_get_ai_token()        - Get AI provider token
#   __gash_get_first_ai_provider() - Get first available AI provider
#   __gash_get_host_target()     - Get user@host by alias
#   __gash_get_host_entries()    - Get all alias/target host entries
#
# Public functions:
#   gash_db_list()             - List available DB connections
#   gash_db_test()             - Test a DB connection
#   gash_env_init()            - Create ~/.gash_env from template
#   gash_ai_list()             - List available AI providers
#
# =============================================================================

# Cache for parsed config
declare -g __GASH_ENV_LOADED=""
declare -g __GASH_ENV_SSH_KEYS=""
declare -g __GASH_ENV_DB_ENTRIES=""
declare -g __GASH_ENV_AI_PROVIDERS=""
declare -g __GASH_ENV_HOST_ENTRIES=""
declare -g __GASH_ENV_SETTINGS=""

# Settings accepted as `NAME=value` lines in ~/.gash_env. A variable already
# set in the environment always wins over the file, so a one-off
# `GASH_BANNER=0 bash` keeps working.
declare -g __GASH_SETTINGS_KNOWN=" GASH_PROMPT GASH_BANNER GASH_HISTORY GASH_HISTORY_SYNC GASH_REPLACE_TOOLS GASH_SAFETY_ALIASES GASH_UMASK GASH_GIT_EXCLUDE GASH_SHOW_INXI GASH_AI_MODEL_CLAUDE GASH_AI_MODEL_GEMINI GASH_AI_CONFIRM GASH_DB_TIMEOUT GASH_LLM_EXEC GASH_LLM_EXEC_ALLOW GASH_LLM_SANDBOX GASH_NVM GASH_PATH_ADD "

# =============================================================================
# URL Encoding/Decoding
# =============================================================================

# Decode URL-encoded string (%XX -> char)
__gash_url_decode() {
    local encoded="${1-}"
    local decoded=""
    local i=0
    local len=${#encoded}
    local char hex

    while ((i < len)); do
        char="${encoded:i:1}"
        if [[ "$char" == "%" ]] && ((i + 2 < len)); then
            hex="${encoded:i+1:2}"
            if [[ "$hex" =~ ^[0-9A-Fa-f]{2}$ ]]; then
                # printf -v (not $(...)) so decoded newlines are preserved
                printf -v char "\\x$hex"
                decoded+="$char"
                i=$((i + 3))
                continue
            fi
        fi
        decoded+="$char"
        i=$((i + 1))
    done

    printf '%s' "$decoded"
}

# Encode special characters for URL (char -> %XX)
__gash_url_encode() {
    local string="${1-}"
    local encoded=""
    local i char code
    # Iterate BYTES, not codepoints: a multi-byte UTF-8 char must produce
    # one %XX per byte (e.g. "è" -> %C3%A8, never %E8).
    local LC_ALL=C

    for ((i = 0; i < ${#string}; i++)); do
        char="${string:i:1}"
        case "$char" in
            [a-zA-Z0-9._~-])
                encoded+="$char"
                ;;
            *)
                # Mask to one byte: some libcs (musl) map high bytes in the
                # C locale to U+DF80+byte, so "'x" can exceed 255.
                printf -v code '%d' "'$char"
                printf -v char '%%%02X' "$((code & 255))"
                encoded+="$char"
                ;;
        esac
    done

    printf '%s' "$encoded"
}

# =============================================================================
# Config File Parsing
# =============================================================================

# Check that the config file is private: owned by the current user and with
# no group or other permission bits (600, 400, 700 are all fine). Symlinks are followed (dotfile
# managers commonly symlink ~/.gash_env).
__gash_check_env_perms() {
    local file="${1-}"
    [[ ! -f "$file" ]] && return 0
    # Headless output is for programs: a warning line would break the JSON
    # on stderr, and nobody reads it (GASH_DEBUG=1 still checks).
    [[ "${GASH_HEADLESS-}" == "1" && "${GASH_DEBUG-}" != "1" ]] && return 0

    # GNU stat first, BSD stat second, whatever the OS: macOS users often
    # have GNU coreutils first on PATH, where `stat -f` means something else.
    local perms="" owner="" out
    if out="$(stat -L -c '%a %u' "$file" 2>/dev/null)" && [[ "$out" =~ ^[0-7]+\ [0-9]+$ ]]; then
        read -r perms owner <<<"$out"
    elif out="$(stat -L -f '%Lp %u' "$file" 2>/dev/null)" && [[ "$out" =~ ^[0-7]+\ [0-9]+$ ]]; then
        read -r perms owner <<<"$out"
    fi

    if [[ "$perms" =~ ^[0-7]{1,4}$ ]] && (((8#$perms) & 8#077)); then
        __gash_warning "${file/#$HOME/\~} is accessible by other users (mode $perms). Run: chmod 600 ${file/#$HOME/\~}"
    fi
    if [[ -n "$owner" && "$owner" != "${EUID-}" ]]; then
        __gash_warning "${file/#$HOME/\~} is owned by uid $owner, not by you (uid ${EUID-}). Gash will still read it."
    fi
    return 0
}

# Load and parse ~/.gash_env (cached in globals for the life of the shell).
# Callers that read values through $(...) should call __gash_load_env first,
# so the cache is filled in the parent shell and not rebuilt per subshell.
__gash_load_env() {
    # Return cached if already loaded
    [[ -n "$__GASH_ENV_LOADED" ]] && return 0

    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"

    __GASH_ENV_SSH_KEYS=""
    __GASH_ENV_DB_ENTRIES=""
    __GASH_ENV_AI_PROVIDERS=""
    __GASH_ENV_HOST_ENTRIES=""
    __GASH_ENV_SETTINGS=""

    # File doesn't exist - not an error, just empty config
    if [[ ! -f "$env_file" ]]; then
        __GASH_ENV_LOADED="1"
        return 0
    fi

    __gash_check_env_perms "$env_file"

    # Warnings name the file and the line number, never the line's content:
    # a malformed line may be a pasted token or password.
    local shown="${env_file/#$HOME/\~}"
    local ssh_keys="" db_entries="" ai_providers="" host_entries="" settings=""
    local line lineno=0 key value rest q inner

    while IFS= read -r line || [[ -n "$line" ]]; do
        lineno=$((lineno + 1))
        line="${line%$'\r'}"

        # Trim whitespace
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"

        # Skip empty lines and comments
        [[ -z "$line" || "$line" == \#* ]] && continue

        case "$line" in
            SSH:* | DB:* | AI:* | HOST:*)
                rest="${line#*:}"
                if [[ "$rest" != *=* ]]; then
                    __gash_warning "Line $lineno of $shown: ${line%%:*}: entry has no '=' (expected ${line%%:*}:name=value), skipping"
                    continue
                fi
                key="${rest%%=*}"
                value="${rest#*=}"
                if [[ -z "$key" ]]; then
                    __gash_warning "Line $lineno of $shown: ${line%%:*}: entry has an empty name, skipping"
                    continue
                fi
                ;;
            GASH_*=*)
                key="${line%%=*}"
                value="${line#*=}"
                if [[ "$value" == [\"\']* ]]; then
                    # Quoted: GASH_X="a b" (# inside the quotes is kept),
                    # optionally followed by a "# comment".
                    q="${value:0:1}"
                    inner="${value:1}"
                    if [[ "$inner" == *"$q"* ]]; then
                        rest="${inner#*"$q"}"
                        rest="${rest#"${rest%%[![:space:]]*}"}"
                        [[ -z "$rest" || "$rest" == \#* ]] && value="${inner%%"$q"*}"
                    fi
                else
                    # Unquoted: a "# comment" after whitespace is not part
                    # of the value (GASH_BANNER=1   # 0 = no banner).
                    value="${value%%[[:space:]]#*}"
                    value="${value%"${value##*[![:space:]]}"}"
                fi
                if [[ ! "$key" =~ ^GASH_[A-Z0-9_]+$ || "$__GASH_SETTINGS_KNOWN" != *" $key "* ]]; then
                    __gash_warning "Line $lineno of $shown: unknown GASH_ setting, skipping"
                    continue
                fi
                [[ -n "$settings" ]] && settings+=$'\n'
                settings+="${key}"$'\t'"${value}"
                continue
                ;;
            *)
                __gash_warning "Line $lineno of $shown: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping"
                continue
                ;;
        esac

        case "$line" in
            SSH:*)
                # SSH:keypath=passphrase (empty passphrase is allowed)
                key="${key/#\~/$HOME}"
                if [[ ! -f "$key" ]]; then
                    __gash_warning "Line $lineno of $shown: SSH key file not found (skipping)"
                    continue
                fi
                [[ -n "$ssh_keys" ]] && ssh_keys+=$'\n'
                ssh_keys+="${key}"$'\t'"${value}"
                ;;
            DB:*)
                # DB:name=driver://user:pass@host:port/database
                if [[ ! "$value" =~ ^(mysql|mariadb|pgsql):// ]]; then
                    __gash_warning "Line $lineno of $shown: DB URL must start with mysql://, mariadb:// or pgsql:// (skipping)"
                    continue
                fi
                [[ -n "$db_entries" ]] && db_entries+=$'\n'
                db_entries+="${key}"$'\t'"${value}"
                ;;
            AI:*)
                # AI:provider=api_token
                if [[ ! "$key" =~ ^(claude|gemini)$ ]]; then
                    __gash_warning "Line $lineno of $shown: unknown AI provider (supported: claude, gemini), skipping"
                    continue
                fi
                if [[ -z "$value" ]]; then
                    __gash_warning "Line $lineno of $shown: AI provider $key has an empty token (skipping)"
                    continue
                fi
                [[ -n "$ai_providers" ]] && ai_providers+=$'\n'
                ai_providers+="${key}"$'\t'"${value}"
                ;;
            HOST:*)
                # HOST:alias=user@host
                if [[ ! "$value" =~ ^[^@[:space:]]+@[^@[:space:]]+$ ]]; then
                    __gash_warning "Line $lineno of $shown: HOST target must be user@host (skipping)"
                    continue
                fi
                [[ -n "$host_entries" ]] && host_entries+=$'\n'
                host_entries+="${key}"$'\t'"${value}"
                ;;
        esac
    done <"$env_file"

    __GASH_ENV_LOADED="1"
    __GASH_ENV_SSH_KEYS="$ssh_keys"
    __GASH_ENV_DB_ENTRIES="$db_entries"
    __GASH_ENV_AI_PROVIDERS="$ai_providers"
    __GASH_ENV_HOST_ENTRIES="$host_entries"
    __GASH_ENV_SETTINGS="$settings"
}

# Apply settings read from ~/.gash_env as shell variables. A variable that
# is already set (environment, ~/.bashrc before Gash) is left untouched.
# With NAMEs, only those settings are applied (headless mode skips the
# ones that only shape the interactive shell).
# Usage: __gash_apply_settings [NAME...]
__gash_apply_settings() {
    __gash_load_env
    [[ -z "$__GASH_ENV_SETTINGS" ]] && return 0
    local line key value only=" $* "
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        key="${line%%$'\t'*}"
        value="${line#*$'\t'}"
        [[ $# -eq 0 || "$only" == *" $key "* ]] || continue
        [[ -n "${!key+x}" ]] && continue
        printf -v "$key" '%s' "$value"
    done <<<"$__GASH_ENV_SETTINGS"
    return 0
}

# Read a setting from ~/.gash_env without applying it.
# Usage: __gash_get_setting NAME  (prints value, rc 1 when absent)
__gash_get_setting() {
    local name="${1-}" line
    __gash_load_env
    while IFS= read -r line; do
        [[ "${line%%$'\t'*}" == "$name" ]] || continue
        printf '%s' "${line#*$'\t'}"
        return 0
    done <<<"$__GASH_ENV_SETTINGS"
    return 1
}

# Force reload of config
__gash_reload_env() {
    __GASH_ENV_LOADED=""
    __gash_load_env
}

# =============================================================================
# SSH Key Functions
# =============================================================================

# Get SSH keys as array of "keypath\tpassphrase" lines
__gash_get_ssh_keys() {
    __gash_load_env
    printf '%s' "$__GASH_ENV_SSH_KEYS"
}

# =============================================================================
# Database Functions
# =============================================================================

# Get database URL by connection name
# Returns URL or empty string if not found
__gash_get_db_url() {
    local name="${1:-default}"

    __gash_load_env

    [[ -z "$__GASH_ENV_DB_ENTRIES" ]] && return 1

    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local entry_name="${line%%$'\t'*}"
        local entry_url="${line#*$'\t'}"

        if [[ "$entry_name" == "$name" ]]; then
            printf '%s' "$entry_url"
            return 0
        fi
    done <<<"$__GASH_ENV_DB_ENTRIES"

    return 1
}

# Parse database URL into components
# Usage: __gash_parse_db_url "mysql://user:pass@host:port/db" VAR_DRIVER VAR_USER VAR_PASS VAR_HOST VAR_PORT VAR_DB
#
# All locals carry the __pdu_ prefix so callers can pass any reasonable
# variable name without hitting bash nameref shadowing (a caller var named
# like one of our locals would silently lose the assignment).
__gash_parse_db_url() {
    local __pdu_url="${1-}"
    local -n __pdu_out_driver="${2-__gash_pdu_dummy}"
    local -n __pdu_out_user="${3-__gash_pdu_dummy}"
    local -n __pdu_out_pass="${4-__gash_pdu_dummy}"
    local -n __pdu_out_host="${5-__gash_pdu_dummy}"
    local -n __pdu_out_port="${6-__gash_pdu_dummy}"
    local -n __pdu_out_db="${7-__gash_pdu_dummy}"

    # Extract driver: mysql://... -> mysql
    local __pdu_driver="${__pdu_url%%://*}"
    __pdu_out_driver="$__pdu_driver"

    # Remove driver prefix
    local __pdu_rest="${__pdu_url#*://}"

    # Extract user:pass@host:port/database
    local __pdu_userinfo=""
    local __pdu_hostinfo=""
    local __pdu_database=""

    # Split by LAST @ to get userinfo and hostinfo
    # Using %@* (shortest suffix match) instead of %%@* to handle @ in passwords
    if [[ "$__pdu_rest" == *@* ]]; then
        __pdu_userinfo="${__pdu_rest%@*}"
        __pdu_rest="${__pdu_rest##*@}"
    fi

    # Split remaining by / to get hostinfo and database
    if [[ "$__pdu_rest" == */* ]]; then
        __pdu_hostinfo="${__pdu_rest%%/*}"
        __pdu_database="${__pdu_rest#*/}"
    else
        __pdu_hostinfo="$__pdu_rest"
        __pdu_database=""
    fi

    # Parse userinfo: user:pass
    local __pdu_user=""
    local __pdu_pass=""
    if [[ -n "$__pdu_userinfo" ]]; then
        if [[ "$__pdu_userinfo" == *:* ]]; then
            __pdu_user="${__pdu_userinfo%%:*}"
            __pdu_pass="${__pdu_userinfo#*:}"
        else
            __pdu_user="$__pdu_userinfo"
        fi
    fi

    # Parse hostinfo: host:port
    local __pdu_host=""
    local __pdu_port=""
    if [[ -n "$__pdu_hostinfo" ]]; then
        if [[ "$__pdu_hostinfo" == *:* ]]; then
            __pdu_host="${__pdu_hostinfo%%:*}"
            __pdu_port="${__pdu_hostinfo#*:}"
        else
            __pdu_host="$__pdu_hostinfo"
            # Default ports
            case "$__pdu_driver" in
                mysql | mariadb) __pdu_port="3306" ;;
                pgsql) __pdu_port="5432" ;;
                *) __pdu_port="" ;;
            esac
        fi
    fi

    __pdu_out_user="$__pdu_user"
    __pdu_out_pass="$__pdu_pass"
    __pdu_out_host="$__pdu_host"
    __pdu_out_port="$__pdu_port"
    __pdu_out_db="$__pdu_database"

    return 0
}

# =============================================================================
# Remote Host Functions
# =============================================================================

# Get remote target (user@host) by alias
# Returns target or empty string (rc 1) if not found
__gash_get_host_target() {
    local alias_name="${1-}"

    [[ -z "$alias_name" ]] && return 1

    __gash_load_env

    [[ -z "$__GASH_ENV_HOST_ENTRIES" ]] && return 1

    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local entry_alias="${line%%$'\t'*}"
        local entry_target="${line#*$'\t'}"

        if [[ "$entry_alias" == "$alias_name" ]]; then
            printf '%s' "$entry_target"
            return 0
        fi
    done <<<"$__GASH_ENV_HOST_ENTRIES"

    return 1
}

# Get all host entries as "alias\ttarget" lines
__gash_get_host_entries() {
    __gash_load_env
    printf '%s' "$__GASH_ENV_HOST_ENTRIES"
}

# =============================================================================
# Public Helper Functions
# =============================================================================

# List available database connections
gash_db_list() {
    needs_help "gash_db_list" "gash_db_list" \
        "List available database connections from ~/.gash_env" \
        "${1-}" && return

    __gash_load_env

    if [[ -z "$__GASH_ENV_DB_ENTRIES" ]]; then
        __gash_info "No database connections configured in ~/.gash_env"
        __gash_info "Run 'gash_env_init' to create a template configuration"
        return 0
    fi

    __gash_info "Available database connections:"
    echo

    local line
    {
        while IFS= read -r line; do
            [[ -z "$line" ]] && continue
            local name="${line%%$'\t'*}"
            local url="${line#*$'\t'}"

            # Mask password in URL for display (greedy .+ extends to the LAST @,
            # so passwords containing @ are fully masked)
            local display_url
            if [[ "$url" =~ ^([^:]+://[^:]+:).+(@[^@]*)$ ]]; then
                display_url="${BASH_REMATCH[1]}****${BASH_REMATCH[2]}"
            else
                display_url="$url"
            fi

            printf '%s\t%s\n' "$name" "$display_url"
        done <<<"$__GASH_ENV_DB_ENTRIES"
    } | __gash_table --no-header --indent 2 --gap 1 --min 1:15 --color 1:accent
}

# Test a database connection
gash_db_test() {
    needs_help "gash_db_test" "gash_db_test [CONNECTION_NAME]" \
        "Test a database connection from ~/.gash_env. Default: 'default'" \
        "${1-}" && return

    local name="${1:-default}"

    local url
    __gash_load_env
    url=$(__gash_get_db_url "$name") || {
        __gash_error "Connection '$name' not found in ~/.gash_env"
        return 1
    }

    # NOTE: use prefixed names to avoid bash nameref shadowing inside
    # __gash_parse_db_url — local vars with unprefixed names (driver, user...)
    # would collide with the function's own locals and assignments would be
    # lost to the shadow scope.
    local _db_driver _db_user _db_pass _db_host _db_port _db_name
    __gash_parse_db_url "$url" _db_driver _db_user _db_pass _db_host _db_port _db_name

    # Decode password
    _db_pass=$(__gash_url_decode "$_db_pass")

    __gash_info "Testing connection '$name' ($_db_driver://$_db_user@$_db_host:$_db_port/$_db_name)..."

    case "$_db_driver" in
        mysql | mariadb)
            local mysql_bin
            mysql_bin=$(type -P mysql 2>/dev/null) || mysql_bin=$(type -P mariadb 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __gash_error "MySQL/MariaDB client not found"
                return 1
            fi

            # Credentials via --defaults-extra-file on a process-substitution
            # fd, never in argv (/proc/<pid>/cmdline is world-readable).
            local _db_pass_ini="${_db_pass//\\/\\\\}"
            _db_pass_ini="${_db_pass_ini//\"/\\\"}"
            if "$mysql_bin" --defaults-extra-file=<(printf '[client]\nuser=%s\npassword="%s"\nhost=%s\nport=%s\n' "$_db_user" "$_db_pass_ini" "$_db_host" "$_db_port") ${_db_name:+-D "$_db_name"} -e "SELECT 1" >/dev/null 2>&1; then
                __gash_success "Connection successful!"
                return 0
            else
                __gash_error "Connection failed"
                return 1
            fi
            ;;
        pgsql)
            if ! type -P psql >/dev/null 2>&1; then
                __gash_error "PostgreSQL client (psql) not found"
                return 1
            fi

            if PGPASSWORD="$_db_pass" psql -U "$_db_user" -h "$_db_host" -p "$_db_port" ${_db_name:+-d "$_db_name"} -c "SELECT 1" >/dev/null 2>&1; then
                __gash_success "Connection successful!"
                return 0
            else
                __gash_error "Connection failed"
                return 1
            fi
            ;;
        *)
            __gash_error "Unknown driver: $_db_driver"
            return 1
            ;;
    esac
}

# Create ~/.gash_env from template
gash_env_init() {
    needs_help "gash_env_init" "gash_env_init [--force]" \
        "Create ~/.gash_env from template. Use --force to overwrite." \
        "${1-}" && return

    local force=""
    [[ "${1-}" == "--force" ]] && force="1"

    local target="${GASH_ENV_FILE:-$HOME/.gash_env}"
    local template="$GASH_DIR/.gash_env.template"

    if [[ -f "$target" && -z "$force" ]]; then
        __gash_warning "$target already exists. Use --force to overwrite."
        return 1
    fi

    if [[ ! -f "$template" ]]; then
        __gash_error "Template not found: $template"
        return 1
    fi

    cp "$template" "$target" || {
        __gash_error "Failed to copy template"
        return 1
    }

    chmod 600 "$target" || {
        __gash_error "Failed to set permissions"
        return 1
    }

    __gash_success "Created $target"
    __gash_info "Edit with your SSH keys and database credentials"
}

# =============================================================================
# AI Provider Functions
# =============================================================================

# Get AI provider token by provider name
# Returns token or empty string if not found
__gash_get_ai_token() {
    local provider="${1-}"

    [[ -z "$provider" ]] && return 1

    __gash_load_env

    [[ -z "$__GASH_ENV_AI_PROVIDERS" ]] && return 1

    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local entry_provider="${line%%$'\t'*}"
        local entry_token="${line#*$'\t'}"

        if [[ "$entry_provider" == "$provider" ]]; then
            printf '%s' "$entry_token"
            return 0
        fi
    done <<<"$__GASH_ENV_AI_PROVIDERS"

    return 1
}

# Get first available AI provider name
# Returns provider name or empty if none configured
__gash_get_first_ai_provider() {
    __gash_load_env

    [[ -z "$__GASH_ENV_AI_PROVIDERS" ]] && return 1

    local first_line="${__GASH_ENV_AI_PROVIDERS%%$'\n'*}"
    [[ -z "$first_line" ]] && return 1

    printf '%s' "${first_line%%$'\t'*}"
    return 0
}

# List available AI providers
gash_ai_list() {
    needs_help "gash_ai_list" "gash_ai_list" \
        "List available AI providers from ~/.gash_env" \
        "${1-}" && return

    __gash_load_env

    if [[ -z "$__GASH_ENV_AI_PROVIDERS" ]]; then
        __gash_info "No AI providers configured in ~/.gash_env"
        __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
        return 0
    fi

    __gash_info "Available AI providers:"
    echo

    # Tokens masked: the first 8 characters only.
    local line token
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        token="${line#*$'\t'}"
        printf '%s\t%s...\n' "${line%%$'\t'*}" "${token:0:8}"
    done <<<"$__GASH_ENV_AI_PROVIDERS" | __gash_table --no-header --indent 2 --gap 1 --min 1:10 --color 1:accent
}
