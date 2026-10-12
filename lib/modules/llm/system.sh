#!/usr/bin/env bash

# Gash Module: LLM Utilities: system
# llm_ports, llm_procs, llm_env.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# System Functions
# -----------------------------------------------------------------------------

# List ports in use.
# Usage: llm_ports [--listen]
llm_ports() {
    if needs_help "llm_ports" "llm_ports [--listen]" "List TCP/UDP ports (JSON array of {port, proto, state, address}); --listen keeps listening sockets only" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_ports_impl "$@"
}

__llm_ports_impl() {
    local scope="a"
    while (($# > 0)); do
        case "$1" in
            --listen | -l) scope="l" ;;
            *)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --listen or no option"
                return 1
                ;;
        esac
        shift
    done

    # One record per proto/address/port/state, sorted by port.
    local rows=""
    if type -P ss >/dev/null 2>&1; then
        rows="$(ss -tun"$scope" 2>/dev/null | "${__GASH_AWK:-awk}" '
            # ss prints a Netid column only when several protocols are shown.
            NR == 1 { netid = ($1 == "Netid"); next }
            {
                if (netid) { proto = $1; state = $2; local_addr = $5 }
                else { proto = "tcp"; state = $1; local_addr = $4 }
                n = split(local_addr, a, ":"); port = a[n]
                addr = substr(local_addr, 1, length(local_addr) - length(port) - 1)
                if (state == "UNCONN") state = "LISTEN"
                if (port ~ /^[0-9]+$/) printf "%s\t%s\t%s\t%s\n", port, proto, state, addr
            }')"
    elif type -P netstat >/dev/null 2>&1; then
        rows="$(netstat -tun"$scope" 2>/dev/null | "${__GASH_AWK:-awk}" '$1 ~ /^(tcp|udp)/ {
            local_addr = $4; n = split(local_addr, a, ":"); port = a[n]
            addr = substr(local_addr, 1, length(local_addr) - length(port) - 1)
            proto = $1; sub(/6$/, "", proto)
            state = (proto == "udp") ? "LISTEN" : $6
            if (port ~ /^[0-9]+$/) printf "%s\t%s\t%s\t%s\n", port, proto, state, addr
        }')"
    fi

    printf '%s\n' "$rows" | sort -t $'\t' -k1,1n -k2,2 -u | "${__GASH_AWK:-awk}" -F '\t' '
        function esc(s) { gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s); return s }
        BEGIN { printf "[" }
        NF == 4 {
            printf "%s{\"port\":%d,\"proto\":\"%s\",\"state\":\"%s\",\"address\":\"%s\"}", sep, $1, esc($2), esc($3), esc($4)
            sep = ","
        }
        END { print "]" }'
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
            --name)
                name="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            --port)
                port="${2-}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            *) shift ;;
        esac
    done

    echo "["

    if [[ -n "$port" ]]; then
        __llm_validate_uint "port" "$port" || {
            echo "]"
            return 1
        }
        # Find process by port (no matches is a valid outcome: || true)
        if type -P lsof >/dev/null 2>&1; then
            lsof -i :"$port" -t 2>/dev/null | while read -r pid; do
                local cmd
                cmd="$(ps -p "$pid" -o comm= 2>/dev/null)"
                echo "{\"pid\":$pid,\"name\":\"$(__gash_json_escape "$cmd")\",\"port\":$port},"
            done | sed '$ s/,$//' || true
        fi
    elif [[ -n "$name" ]]; then
        # Find process by name (pgrep returns 1 on zero matches: || true)
        pgrep -f "$name" 2>/dev/null | while read -r pid; do
            local cmd
            cmd="$(ps -p "$pid" -o comm= 2>/dev/null)"
            echo "{\"pid\":$pid,\"name\":\"$(__gash_json_escape "$cmd")\"},"
        done | sed '$ s/,$//' || true
    else
        # List all (limited). LC_ALL=C forces dot-decimal %CPU/%MEM (an
        # it_IT locale would emit "0,5" and invalidate the JSON); the name
        # field is escaped for quotes/backslashes.
        LC_ALL=C ps aux --no-headers 2>/dev/null | head -n 20 |
            awk '{
                name = $11
                gsub(/\\/, "\\\\", name); gsub(/"/, "\\\"", name)
                user = $1
                gsub(/\\/, "\\\\", user); gsub(/"/, "\\\"", user)
                printf "{\"pid\":%s,\"user\":\"%s\",\"cpu\":%s,\"mem\":%s,\"name\":\"%s\"},\n", $2, user, $3, $4, name
            }' | sed '$ s/,$//' || true
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

    # env -0: NUL-delimited entries so multi-line values stay attached to
    # their own variable instead of being parsed as bogus extra entries.
    local first=1
    while IFS='=' read -r -d '' name value; do
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

        [[ $first -eq 0 ]] && echo ","
        first=0
        printf '  "%s": "%s"' "$(__gash_json_escape "$name")" "$(__gash_json_escape "$value")"
    done < <(env -0 2>/dev/null | sort -z)

    echo ""
    echo "}"
}
