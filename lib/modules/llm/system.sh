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
    if __llm_needs_help "llm_ports" "llm_ports [--listen]" "List TCP/UDP ports (JSON array of {port, proto, state, address}); --listen keeps listening sockets only" "$@"; then
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
    else
        __llm_error "ports_tool_not_found" "Neither ss nor netstat is installed" "CONTINUE" "false" "Install iproute2 (ss) or net-tools (netstat) to list ports. Skip this check"
        return 1
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
    if __llm_needs_help "llm_procs" "llm_procs [--name NAME] [--port PORT]" "List processes (JSON)" "$@"; then
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
            *)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --name NAME, --port PORT or no option"
                return 1
                ;;
        esac
    done

    if [[ -n "$port" ]]; then
        __llm_validate_uint "port" "$port" || return 1
    fi

    # One compact JSON array, like every other llm_* function. No matches
    # (pgrep/lsof exit 1) is a valid outcome: || true.
    local -a items=()
    local pid cmd line
    if [[ -n "$port" ]]; then
        if type -P lsof >/dev/null 2>&1; then
            while read -r pid; do
                [[ "$pid" =~ ^[0-9]+$ ]] || continue
                cmd="$(ps -p "$pid" -o comm= 2>/dev/null)" || true
                items+=("{\"pid\":$pid,\"name\":\"$(__gash_json_escape "$cmd")\",\"port\":$port}")
            done < <(lsof -i :"$port" -t 2>/dev/null || true)
        fi
    elif [[ -n "$name" ]]; then
        while read -r pid; do
            [[ "$pid" =~ ^[0-9]+$ ]] || continue
            cmd="$(ps -p "$pid" -o comm= 2>/dev/null)" || true
            items+=("{\"pid\":$pid,\"name\":\"$(__gash_json_escape "$cmd")\"}")
        done < <(pgrep -f "$name" 2>/dev/null || true)
    else
        # List all (limited). LC_ALL=C forces dot-decimal %CPU/%MEM (an
        # it_IT locale would emit "0,5" and invalidate the JSON); the name
        # field is escaped for quotes/backslashes.
        while IFS= read -r line; do
            items+=("$line")
        done < <(LC_ALL=C ps aux --no-headers 2>/dev/null | head -n 20 |
            awk '{
                name = $11
                gsub(/\\/, "\\\\", name); gsub(/"/, "\\\"", name)
                user = $1
                gsub(/\\/, "\\\\", user); gsub(/"/, "\\\"", user)
                printf "{\"pid\":%s,\"user\":\"%s\",\"cpu\":%s,\"mem\":%s,\"name\":\"%s\"}\n", $2, user, $3, $4, name
            }' || true)
    fi

    local IFS=,
    printf '[%s]\n' "${items[*]}"
}

# List environment variables (filtered, no secrets).
# Usage: llm_env [--filter PATTERN]
llm_env() {
    if __llm_needs_help "llm_env" "llm_env [--filter PATTERN]" "List env vars (filtered, no secrets)" "$@"; then
        return 0
    fi

    __gash_no_history __llm_env_impl "$@"
}

__llm_env_impl() {
    local filter=""
    while (($# > 0)); do
        case "$1" in
            --filter | -f)
                if (($# < 2)); then
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a regular expression"
                    return 1
                fi
                filter="$2"
                shift 2
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --filter PATTERN"
                return 1
                ;;
            *)
                # Bare pattern: accepted for compatibility with v1.
                filter="$1"
                shift
                ;;
        esac
    done

    if [[ -n "$filter" ]]; then
        local rc=0
        # shellcheck disable=SC2319 # the [[ ]] status itself: 2 = invalid regex (bash 5.3 also prints why)
        [[ "" =~ $filter ]] 2>/dev/null || rc=$?
        if ((rc == 2)); then
            __llm_error "invalid_filter" "$filter" "RETRY" "true" "--filter takes an extended regular expression, e.g. '^PATH$' or 'NODE|NPM'"
            return 1
        fi
    fi

    local name value first=1
    printf '{'
    # Exported variables of this shell, which is what env would print:
    # compgen and ${!name} need no `env -0` (BSD env has no -0) and keep
    # multi-line values whole.
    while IFS= read -r name; do
        [[ "$name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
        if [[ -n "$filter" ]] && ! [[ "$name" =~ $filter ]]; then
            continue
        fi
        __llm_env_secret_name "$name" && continue
        value="${!name-}"
        __llm_env_redact value
        ((first)) || printf ','
        first=0
        printf '\n  "%s": "%s"' "$(__gash_json_escape "$name")" "$(__gash_json_escape "$value")"
    done < <(compgen -e | LC_ALL=C sort -u)
    printf '\n}\n'
}

# Whether a variable name looks like it holds a secret (case-insensitive
# substring match; PWD and OLDPWD are directories).
# Usage: __llm_env_secret_name NAME
__llm_env_secret_name() {
    local upper="${1^^}"
    local re='(PASSWORD|PASSWD|PASSPHRASE|PASS$|_PWD$|SECRET|TOKEN|KEY|CREDENTIAL|AUTH|PRIVATE|COOKIE|SIGNATURE|SESSION|BEARER|JWT|WEBHOOK|SALT|ENCRYPT|(^|_)PAT$|AWS_|DATABASE_URL|DSN|CONN(ECTION)?_?STRING)'
    [[ "$upper" =~ $re ]]
}

# Mask secrets inside a value whose name looked harmless: the password of
# a URL (scheme://user:***@host) and known token formats (the ones the AI
# module redacts: AWS keys, GitHub, OpenAI/Anthropic, Slack, Google).
# Usage: __llm_env_redact VAR_NAME
__llm_env_redact() {
    local -n __er_value="$1"
    if [[ "$__er_value" =~ ^([A-Za-z][A-Za-z0-9+.-]*://[^/:@]*):[^/@]*@(.*)$ ]]; then
        __er_value="${BASH_REMATCH[1]}:***@${BASH_REMATCH[2]}"
    fi
    local re='-----BEGIN [[:upper:][:digit:] ]*PRIVATE KEY'
    if [[ "$__er_value" =~ $re ]]; then
        __er_value="***"
        return 0
    fi
    for re in 'AKIA[[:upper:][:digit:]]{16}' '(gh[pousr]_|github_pat_)[[:alnum:]_]{20,}' \
        'sk-(ant-)?[[:alnum:]_-]{20,}' 'xox[abposr]-[[:alnum:]-]{10,}' 'AIza[[:alnum:]_-]{35}'; do
        while [[ "$__er_value" =~ $re ]]; do
            __er_value="${__er_value//"${BASH_REMATCH[0]}"/***}"
        done
    done
}
