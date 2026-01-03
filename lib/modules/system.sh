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
