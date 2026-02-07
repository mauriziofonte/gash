#!/usr/bin/env bash

# =============================================================================
# Gash Core: Configuration Parser
# =============================================================================
#
# Parses ~/.gash_env for SSH keys, database connections and AI providers.
#
# File format:
#   SSH:keypath=passphrase
#   DB:name=driver://user:password@host:port/database
#   AI:provider=api_token
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

# =============================================================================
# URL Encoding/Decoding
# =============================================================================

# Decode URL-encoded string (%XX -> char)
__gash_url_decode() {
    local encoded="${1-}"
    local decoded=""
    local i=0
    local len=${#encoded}

    while ((i < len)); do
        local char="${encoded:i:1}"
        if [[ "$char" == "%" ]] && ((i + 2 < len)); then
            local hex="${encoded:i+1:2}"
            if [[ "$hex" =~ ^[0-9A-Fa-f]{2}$ ]]; then
                decoded+=$(printf "\\x$hex")
                ((i += 3))
                continue
            fi
        fi
        decoded+="$char"
        ((i++))
    done

    printf '%s' "$decoded"
}

# Encode special characters for URL (char -> %XX)
__gash_url_encode() {
    local string="${1-}"
    local encoded=""
    local i

    for ((i = 0; i < ${#string}; i++)); do
        local char="${string:i:1}"
        case "$char" in
            [a-zA-Z0-9._~-])
                encoded+="$char"
                ;;
            *)
                encoded+=$(printf '%%%02X' "'$char")
                ;;
        esac
    done

    printf '%s' "$encoded"
}

# =============================================================================
# Config File Parsing
# =============================================================================

# Check file permissions (should be 600)
__gash_check_env_perms() {
    local file="${1-}"
    [[ ! -f "$file" ]] && return 0

    local perms
    if [[ "$(uname)" == "Darwin" ]]; then
        perms=$(stat -f %Lp "$file" 2>/dev/null) || true
    else
        perms=$(stat -c %a "$file" 2>/dev/null) || true
    fi

    if [[ -n "$perms" && "$perms" != "600" ]]; then
        __gash_warning "~/.gash_env has insecure permissions ($perms). Run: chmod 600 ~/.gash_env"
    fi
}

# Load and parse ~/.gash_env
__gash_load_env() {
    # Return cached if already loaded
    [[ -n "$__GASH_ENV_LOADED" ]] && return 0

    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"

    # File doesn't exist - not an error, just empty config
    if [[ ! -f "$env_file" ]]; then
        __GASH_ENV_LOADED="1"
        __GASH_ENV_SSH_KEYS=""
        __GASH_ENV_DB_ENTRIES=""
        __GASH_ENV_AI_PROVIDERS=""
        return 0
    fi

    # Check permissions
    __gash_check_env_perms "$env_file"

    local ssh_keys=""
    local db_entries=""
    local ai_providers=""
    local line

    while IFS= read -r line || [[ -n "$line" ]]; do
        # Remove CRLF
        line="${line%$'\r'}"

        # Trim whitespace
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"

        # Skip empty lines and comments
        [[ -z "$line" || "$line" == \#* ]] && continue

        # Parse SSH entries: SSH:keypath=passphrase
        if [[ "$line" == SSH:* ]]; then
            local ssh_rest="${line#SSH:}"
            local keypath="${ssh_rest%%=*}"
            local passphrase="${ssh_rest#*=}"

            # Expand tilde
            keypath="${keypath/#\~/$HOME}"

            # Validate key file exists
            if [[ ! -f "$keypath" ]]; then
                __gash_warning "SSH key not found: $keypath (skipping)"
                continue
            fi

            # Store as TAB-separated: keypath\tpassphrase
            [[ -n "$ssh_keys" ]] && ssh_keys+=$'\n'
            ssh_keys+="${keypath}"$'\t'"${passphrase}"
            continue
        fi

        # Parse DB entries: DB:name=driver://user:pass@host:port/database
        if [[ "$line" == DB:* ]]; then
            local db_rest="${line#DB:}"
            local db_name="${db_rest%%=*}"
            local db_url="${db_rest#*=}"

            # Validate URL format
            if [[ ! "$db_url" =~ ^(mysql|mariadb|pgsql):// ]]; then
                __gash_warning "Invalid DB URL format for '$db_name': $db_url (skipping)"
                continue
            fi

            # Store as TAB-separated: name\turl
            [[ -n "$db_entries" ]] && db_entries+=$'\n'
            db_entries+="${db_name}"$'\t'"${db_url}"
            continue
        fi

        # Parse AI entries: AI:provider=api_token
        if [[ "$line" == AI:* ]]; then
            local ai_rest="${line#AI:}"
            local provider="${ai_rest%%=*}"
            local token="${ai_rest#*=}"

            # Validate provider name
            if [[ ! "$provider" =~ ^(claude|gemini)$ ]]; then
                __gash_warning "Unknown AI provider '$provider' (only claude/gemini supported)"
                continue
            fi

            # Store as TAB-separated: provider\ttoken
            [[ -n "$ai_providers" ]] && ai_providers+=$'\n'
            ai_providers+="${provider}"$'\t'"${token}"
            continue
        fi

        # Unknown line format
        __gash_warning "Unknown config line format: $line (skipping)"

    done < "$env_file"

    __GASH_ENV_LOADED="1"
    __GASH_ENV_SSH_KEYS="$ssh_keys"
    __GASH_ENV_DB_ENTRIES="$db_entries"
    __GASH_ENV_AI_PROVIDERS="$ai_providers"
}

# Force reload of config
__gash_reload_env() {
    __GASH_ENV_LOADED=""
    __GASH_ENV_SSH_KEYS=""
    __GASH_ENV_DB_ENTRIES=""
    __GASH_ENV_AI_PROVIDERS=""
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
    done <<< "$__GASH_ENV_DB_ENTRIES"

    return 1
}

# Parse database URL into components
# Usage: __gash_parse_db_url "mysql://user:pass@host:port/db" VAR_DRIVER VAR_USER VAR_PASS VAR_HOST VAR_PORT VAR_DB
__gash_parse_db_url() {
    local url="${1-}"
    local -n out_driver="${2-_gash_dummy}"
    local -n out_user="${3-_gash_dummy}"
    local -n out_pass="${4-_gash_dummy}"
    local -n out_host="${5-_gash_dummy}"
    local -n out_port="${6-_gash_dummy}"
    local -n out_db="${7-_gash_dummy}"

    # Extract driver: mysql://... -> mysql
    local driver="${url%%://*}"
    out_driver="$driver"

    # Remove driver prefix
    local rest="${url#*://}"

    # Extract user:pass@host:port/database
    local userinfo=""
    local hostinfo=""
    local database=""

    # Split by LAST @ to get userinfo and hostinfo
    # Using %@* (shortest suffix match) instead of %%@* to handle @ in passwords
    if [[ "$rest" == *@* ]]; then
        userinfo="${rest%@*}"
        rest="${rest##*@}"
    fi

    # Split remaining by / to get hostinfo and database
    if [[ "$rest" == */* ]]; then
        hostinfo="${rest%%/*}"
        database="${rest#*/}"
    else
        hostinfo="$rest"
        database=""
    fi

    # Parse userinfo: user:pass
    local user=""
    local pass=""
    if [[ -n "$userinfo" ]]; then
        if [[ "$userinfo" == *:* ]]; then
            user="${userinfo%%:*}"
            pass="${userinfo#*:}"
        else
            user="$userinfo"
        fi
    fi

    # Parse hostinfo: host:port
    local host=""
    local port=""
    if [[ -n "$hostinfo" ]]; then
        if [[ "$hostinfo" == *:* ]]; then
            host="${hostinfo%%:*}"
            port="${hostinfo#*:}"
        else
            host="$hostinfo"
            # Default ports
            case "$driver" in
                mysql|mariadb) port="3306" ;;
                pgsql) port="5432" ;;
                *) port="" ;;
            esac
        fi
    fi

    out_user="$user"
    out_pass="$pass"
    out_host="$host"
    out_port="$port"
    out_db="$database"

    return 0
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
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local name="${line%%$'\t'*}"
        local url="${line#*$'\t'}"

        # Mask password in URL for display
        local display_url
        if [[ "$url" =~ ^([^:]+://[^:]+:)[^@]+(@.*)$ ]]; then
            display_url="${BASH_REMATCH[1]}****${BASH_REMATCH[2]}"
        else
            display_url="$url"
        fi

        printf "  %-15s %s\n" "$name" "$display_url"
    done <<< "$__GASH_ENV_DB_ENTRIES"
}

# Test a database connection
gash_db_test() {
    needs_help "gash_db_test" "gash_db_test [CONNECTION_NAME]" \
        "Test a database connection from ~/.gash_env. Default: 'default'" \
        "${1-}" && return

    local name="${1:-default}"

    local url
    url=$(__gash_get_db_url "$name") || {
        __gash_error "Connection '$name' not found in ~/.gash_env"
        return 1
    }

    local driver user pass host port db
    __gash_parse_db_url "$url" driver user pass host port db

    # Decode password
    pass=$(__gash_url_decode "$pass")

    __gash_info "Testing connection '$name' ($driver://$user@$host:$port/$db)..."

    case "$driver" in
        mysql|mariadb)
            local mysql_bin
            mysql_bin=$(type -P mysql 2>/dev/null) || mysql_bin=$(type -P mariadb 2>/dev/null) || true
            if [[ -z "$mysql_bin" ]]; then
                __gash_error "MySQL/MariaDB client not found"
                return 1
            fi

            if "$mysql_bin" -u"$user" -p"$pass" -h"$host" -P"$port" ${db:+-D "$db"} -e "SELECT 1" >/dev/null 2>&1; then
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

            if PGPASSWORD="$pass" psql -U "$user" -h "$host" -p "$port" ${db:+-d "$db"} -c "SELECT 1" >/dev/null 2>&1; then
                __gash_success "Connection successful!"
                return 0
            else
                __gash_error "Connection failed"
                return 1
            fi
            ;;
        *)
            __gash_error "Unknown driver: $driver"
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

    local target="$HOME/.gash_env"
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
    done <<< "$__GASH_ENV_AI_PROVIDERS"

    return 1
}

# Get first available AI provider name
# Returns provider name or empty if none configured
__gash_get_first_ai_provider() {
    __gash_load_env

    [[ -z "$__GASH_ENV_AI_PROVIDERS" ]] && return 1

    local first_line
    first_line=$(head -n1 <<< "$__GASH_ENV_AI_PROVIDERS")
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

    local line
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        local provider="${line%%$'\t'*}"
        local token="${line#*$'\t'}"

        # Mask token for display (show first 8 chars)
        local masked_token="${token:0:8}..."

        printf "  %-10s %s\n" "$provider" "$masked_token"
    done <<< "$__GASH_ENV_AI_PROVIDERS"
}
