#!/usr/bin/env bash

# Gash Module: System Operations
# Functions for system monitoring, process management, and service control.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   disk_usage (du2)        - Display disk usage for specific filesystem types
#   history_grep (hg)       - Search command history with colored output
#   ip_public (myip)        - Get your public IP address
#   process_find (pf)       - Search for a process by name
#   process_kill (pk)       - Kill all processes by name
#   port_kill (ptk)         - Kill all processes by port
#   services_stop (svs)     - Stop well-known services
#   sudo_last (plz)         - Run last command with sudo
#   mkdir_cd (mkcd)         - Create directory and cd into it

# -----------------------------------------------------------------------------
# Disk Usage
# -----------------------------------------------------------------------------

# Display disk usage for specific filesystem types.
# Usage: disk_usage
# Alias: du2
disk_usage() {
    needs_help "disk_usage" "disk_usage" \
        "Displays disk usage for specific filesystem types, formatted for easy reading. Alias: du2" \
        "${1-}" && return

    df -hT | awk '
    BEGIN {printf "%-20s %-8s %-8s %-8s %-8s %-6s %-20s\n", "Filesystem", "Type", "Size", "Used", "Avail", "Use%", "Mountpoint"}
    $2 ~ /(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|ntfs)/ {
        printf "\033[1;33m%-20s\033[0m \033[0;36m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[38;5;214m%-6s\033[0m %-20s\n", $1, $2, $3, $4, $5, $6, $7
    }'

    return 0
}

# -----------------------------------------------------------------------------
# History Search
# -----------------------------------------------------------------------------

# Search command history with colored output, removing duplicates.
# Usage: history_grep PATTERN
# Alias: hg
history_grep() {
    needs_help "history_grep" "history_grep PATTERN" \
        "Searches the bash history for commands matching PATTERN. Alias: hg" \
        "${1-}" && return

    __gash_require_arg "${1-}" "pattern" "history_grep <pattern>" || return 1

    # Extract the relevant parts (ignoring history line numbers), remove duplicates, and avoid self-call
    history | grep -i -- "$@" | grep -v "history_grep" | \
        awk '{ $1=""; seen[$0]++; if (seen[$0]==1) print $0 }' | \
        awk '{
            printf "\033[1;32m%-5s\033[0m \033[0;36m%-20s\033[0m \033[1;37m%s\033[0m\n", NR, $1" "$2, substr($0, index($0,$3));
        }'

    return 0
}

# Smart history search with timestamps and deduplication.
# Shows only the LAST execution of each unique command.
# Usage: hgrep PATTERN [OPTIONS]
# Options:
#   -n, --limit N       Show only last N results (default: unlimited)
#   -a, --all           Include commands with hgrep/history_grep
#   -r, --reverse       Show oldest first (default: newest last)
#   -j, --json          Output as JSON array
#   -H, --no-highlight  Disable pattern highlighting
#   -E, --regex         Use extended regex (default: fixed string)
#   -c, --count         Only show count of matching commands
hgrep() {
    # Custom help (more detailed than needs_help)
    if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
        cat <<'EOF'
hgrep - Smart history search with timestamps and deduplication

USAGE:
  hgrep PATTERN [OPTIONS]

OPTIONS:
  -n, --limit N       Show only last N results
  -a, --all           Include hgrep/history_grep in results
  -r, --reverse       Show oldest first (default: newest last)
  -j, --json          Output as JSON array
  -H, --no-highlight  Disable pattern highlighting in output
  -E, --regex         Use extended regex (default: fixed string match)
  -c, --count         Only show count of unique matching commands

EXAMPLES:
  hgrep git                    Search for "git" commands
  hgrep "composer.*install"    Search with pattern
  hgrep docker -n 10           Last 10 docker commands
  hgrep -E "^git (push|pull)"  Regex: git push or pull at start
  hgrep npm -j                 JSON output for scripting
  hgrep make -c                Count unique make commands

OUTPUT FORMAT:
  [2025-01-15 14:32:01] git push origin main
  [2025-01-15 15:10:22] git commit -m "fix"

  With --json:
  [{"timestamp":"2025-01-15 14:32:01","command":"git push origin main"},...]
EOF
        return 0
    fi

    local pattern=""
    local limit=0
    local include_self=0
    local reverse=0
    local json_output=0
    local use_regex=0
    local count_only=0

    # Disable colors automatically in HEADLESS mode (for LLM agents)
    local no_highlight=0
    [[ "${GASH_HEADLESS-}" == "1" ]] && no_highlight=1

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -n|--limit)
                if [[ -z "${2-}" || ! "${2-}" =~ ^[0-9]+$ ]]; then
                    __gash_error "Option -n requires a numeric argument"
                    return 1
                fi
                limit="$2"; shift 2
                ;;
            -a|--all) include_self=1; shift ;;
            -r|--reverse) reverse=1; shift ;;
            -j|--json) json_output=1; shift ;;
            -H|--no-highlight) no_highlight=1; shift ;;
            -E|--regex) use_regex=1; shift ;;
            -c|--count) count_only=1; shift ;;
            -*)
                __gash_error "Unknown option: $1. Use -h for help."
                return 1
                ;;
            *)
                if [[ -z "$pattern" ]]; then
                    pattern="$1"
                else
                    __gash_error "Multiple patterns not supported: $1"
                    return 1
                fi
                shift
                ;;
        esac
    done

    if [[ -z "$pattern" ]]; then
        __gash_error "Missing required argument: pattern"
        echo "Usage: hgrep <pattern> [options]. Use -h for help." >&2
        return 1
    fi

    local histfile="${HISTFILE:-$HOME/.bash_history}"
    if [[ ! -r "$histfile" ]]; then
        __gash_error "Cannot read history file: $histfile"
        return 1
    fi

    # Flush current session history to file
    history -a 2>/dev/null || true

    # Process history file with awk
    local awk_result
    awk_result=$(awk -v pattern="$pattern" \
                     -v include_self="$include_self" \
                     -v use_regex="$use_regex" '
    BEGIN {
        IGNORECASE = 1
    }
    /^#[0-9]+$/ {
        timestamp = substr($0, 2)
        if (getline cmd > 0) {
            # Skip empty commands
            if (cmd == "") next
            # Skip self-references unless -a flag
            if (include_self == 0 && (cmd ~ /hgrep/ || cmd ~ /history_grep/)) next
            # Match pattern (regex or fixed string)
            matched = 0
            if (use_regex == 1) {
                if (cmd ~ pattern) matched = 1
            } else {
                if (index(tolower(cmd), tolower(pattern)) > 0) matched = 1
            }
            if (matched) {
                # Store: last occurrence wins (dedup)
                commands[cmd] = timestamp
            }
        }
    }
    END {
        # Build array for sorting by timestamp
        n = 0
        for (cmd in commands) {
            n++
            ts[n] = commands[cmd]
            cm[n] = cmd
        }
        # Sort by timestamp ascending (bubble sort)
        for (i = 1; i <= n; i++) {
            for (j = i + 1; j <= n; j++) {
                if (ts[i] > ts[j]) {
                    tmp = ts[i]; ts[i] = ts[j]; ts[j] = tmp
                    tmp = cm[i]; cm[i] = cm[j]; cm[j] = tmp
                }
            }
        }
        # Output: timestamp<TAB>command
        for (i = 1; i <= n; i++) {
            print ts[i] "\t" cm[i]
        }
    }
    ' "$histfile") || return 1

    # Handle empty results
    if [[ -z "$awk_result" ]]; then
        if [[ "$count_only" -eq 1 ]]; then
            echo "0"
        elif [[ "$json_output" -eq 1 ]]; then
            echo "[]"
        fi
        return 0
    fi

    # Apply reverse if requested
    if [[ "$reverse" -eq 1 ]]; then
        awk_result=$(echo "$awk_result" | tac)
    fi

    # Apply limit if requested
    if [[ "$limit" -gt 0 ]]; then
        awk_result=$(echo "$awk_result" | tail -n "$limit")
    fi

    # Count mode
    if [[ "$count_only" -eq 1 ]]; then
        echo "$awk_result" | wc -l | tr -d ' '
        return 0
    fi

    # JSON output mode
    if [[ "$json_output" -eq 1 ]]; then
        echo -n '['
        local first=1
        while IFS=$'\t' read -r ts cmd; do
            local date_str
            date_str=$(date -d "@$ts" '+%Y-%m-%d %H:%M:%S' 2>/dev/null) || date_str="unknown"
            # Escape JSON special chars in command
            cmd="${cmd//\\/\\\\}"
            cmd="${cmd//\"/\\\"}"
            cmd="${cmd//$'\n'/\\n}"
            cmd="${cmd//$'\t'/\\t}"
            if [[ "$first" -eq 1 ]]; then
                first=0
            else
                echo -n ','
            fi
            printf '{"timestamp":"%s","epoch":%s,"command":"%s"}' "$date_str" "$ts" "$cmd"
        done <<< "$awk_result"
        echo ']'
        return 0
    fi

    # Standard output with optional highlighting
    while IFS=$'\t' read -r ts cmd; do
        local date_str
        date_str=$(date -d "@$ts" '+%Y-%m-%d %H:%M:%S' 2>/dev/null) || date_str="unknown"

        if [[ "$no_highlight" -eq 0 ]]; then
            # Highlight pattern in command (case-insensitive)
            local highlighted_cmd
            if [[ "$use_regex" -eq 1 ]]; then
                # For regex, use sed with case-insensitive flag
                highlighted_cmd=$(echo "$cmd" | sed -E "s/($pattern)/\\\\033[1;33m\\1\\\\033[1;37m/gi" 2>/dev/null) || highlighted_cmd="$cmd"
            else
                # For fixed string, escape regex special chars and highlight
                local escaped_pattern
                escaped_pattern=$(printf '%s' "$pattern" | sed 's/[[\.*^$()+?{|]/\\&/g')
                highlighted_cmd=$(echo "$cmd" | sed -E "s/($escaped_pattern)/\\\\033[1;33m\\1\\\\033[1;37m/gi" 2>/dev/null) || highlighted_cmd="$cmd"
            fi
            printf '\033[0;36m[%s]\033[0m \033[1;37m%b\033[0m\n' "$date_str" "$highlighted_cmd"
        else
            # Plain output without colors (for -H flag or GASH_HEADLESS mode)
            printf '[%s] %s\n' "$date_str" "$cmd"
        fi
    done <<< "$awk_result"
}

# -----------------------------------------------------------------------------
# Network
# -----------------------------------------------------------------------------

# Get your public IP address.
# Usage: ip_public
# Alias: myip
ip_public() {
    local ip=""

    if command -v wget >/dev/null 2>&1; then
        ip=$(wget -qO- https://ipinfo.io/ip 2>/dev/null) || true
    elif command -v curl >/dev/null 2>&1; then
        ip=$(curl -s https://ipinfo.io/ip 2>/dev/null) || true
    else
        __gash_error "This function requires either 'wget' or 'curl' to be installed."
        return 1
    fi

    if [[ -z "$ip" ]]; then
        __gash_error "Failed to retrieve public IP address."
        return 1
    fi

    echo -e "${__GASH_BOLD_WHITE}Public IP:${__GASH_COLOR_OFF} ${__GASH_CYAN}${ip}${__GASH_COLOR_OFF}"
}

# -----------------------------------------------------------------------------
# Process Management
# -----------------------------------------------------------------------------

# Search for a process by name.
# Usage: process_find PROCESS_NAME
# Alias: pf
process_find() {
    needs_help "process_find" "process_find PROCESS_NAME" \
        "Search for a process by name. Alias: pf" \
        "${1-}" && return

    local process_name="${1-}"
    __gash_require_arg "$process_name" "process name" "process_find <process_name>" || return 1

    local result
    result=$(ps aux | grep -i -- "$process_name" | grep -v grep)

    if [ -n "$result" ]; then
        __gash_info "Processes matching $process_name:"
        echo "$result" | awk '{ printf "   \033[1;33m%-8s\033[0m \033[0;36m%-12s\033[0m %-4s \033[1;37m%-40s\033[0m\n", $2, $1, $3, $11 }'
    else
        __gash_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by name.
# Usage: process_kill PROCESS_NAME
# Alias: pk
process_kill() {
    needs_help "process_kill" "process_kill PROCESS_NAME" \
        "Kill all processes by name. Alias: pk" \
        "${1-}" && return

    local process_name="${1-}"
    __gash_require_arg "$process_name" "process name" "process_kill <process_name>" || return 1

    local pids
    pids=$(ps aux | grep -i -- "$process_name" | grep -v grep | awk '{print $2}')

    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            __gash_info "Process with PID $pid killed."
        done
    else
        __gash_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by port.
# Usage: port_kill PORT
# Alias: ptk
port_kill() {
    local port="${1-}"

    __gash_require_arg "$port" "port" "port_kill <port>" || return 1
    __gash_require_command "lsof" "This function requires 'lsof', which is not available." || return 1

    local pids
    pids=$(lsof -t -i:"$port" 2>/dev/null || true)

    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            __gash_info "Process on port $port with PID $pid killed."
        done
    else
        __gash_error "No process found on port $port."
    fi
}

# -----------------------------------------------------------------------------
# Service Management
# -----------------------------------------------------------------------------

# Stop well-known services like Apache, Nginx, MySQL, MariaDB, PostgreSQL, Redis, etc.
# Usage: services_stop [--force]
# Alias: svs
services_stop() {
    local services=("apache2" "nginx" "mysql" "mariadb" "postgresql" "mongodb" "redis" "memcached" "docker")
    local force_flag="${1-}"

    if [[ "$force_flag" != "--force" ]]; then
        if ! needs_confirm_prompt "${__GASH_BOLD_YELLOW}Warning:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}Stop all well-known services?${__GASH_COLOR_OFF}"; then
            return 0
        fi
    fi

    for service in "${services[@]}"; do
        if systemctl is-active --quiet "$service"; then
            __gash_info "Stopping $service service..."
            sudo systemctl stop "$service"
        fi
    done
}

# -----------------------------------------------------------------------------
# Miscellaneous
# -----------------------------------------------------------------------------

# Run last command with sudo.
# Usage: sudo_last [command]
# Alias: plz
sudo_last() {
    if [[ -n "${1-}" ]]; then
        sudo "$@"
    else
        # shellcheck disable=SC2046
        sudo $(fc -ln -1)
    fi
}

# Create directory and cd into it.
# Usage: mkdir_cd DIRECTORY
# Alias: mkcd
mkdir_cd() {
    local dir="${1-}"

    __gash_require_arg "$dir" "directory" "mkdir_cd <directory>" || return 1

    mkdir -p "$dir" && cd "$dir" || return 1
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias du2='disk_usage'
alias hg='history_grep'
alias myip='ip_public'
alias pf='process_find'
alias pk='process_kill'
alias ptk='port_kill'
alias svs='services_stop'
alias plz='sudo_last'
alias mkcd='mkdir_cd'
