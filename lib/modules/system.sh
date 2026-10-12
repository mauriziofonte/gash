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
# Usage: disk_usage [--no-color]
# Alias: du2
disk_usage() {
    needs_help "disk_usage" "disk_usage [--no-color]" \
        "Displays disk usage for real filesystem types, formatted for easy reading. Alias: du2" \
        "${1-}" && return

    local no_color=0
    local a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
        esac
    done

    local use_color=1
    __gash_want_colors "$no_color" || use_color=0

    df -hT | awk -v c="$use_color" '
    BEGIN {printf "%-20s %-8s %-8s %-8s %-8s %-6s %-20s\n", "Filesystem", "Type", "Size", "Used", "Avail", "Use%", "Mountpoint"}
    $2 ~ /(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|ntfs)/ {
        if (c == 1) {
            printf "\033[1;33m%-20s\033[0m \033[0;36m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[38;5;214m%-6s\033[0m %-20s\n", $1, $2, $3, $4, $5, $6, $7
        } else {
            printf "%-20s %-8s %-8s %-8s %-8s %-6s %-20s\n", $1, $2, $3, $4, $5, $6, $7
        }
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

    # Honor the runtime color gate (headless, NO_COLOR, non-TTY pipe).
    local use_color=1
    __gash_use_color || use_color=0

    # Extract the relevant parts (ignoring history line numbers), remove duplicates, and avoid self-call.
    # `|| true`: zero matches is a valid outcome, not a pipefail death sentence.
    history | grep -i -- "$@" | grep -v "history_grep" |
        awk '{ $1=""; seen[$0]++; if (seen[$0]==1) print $0 }' |
        awk -v c="$use_color" '{
            if (c == 1)
                printf "\033[1;32m%-5s\033[0m \033[0;36m%-20s\033[0m \033[1;37m%s\033[0m\n", NR, $1" "$2, substr($0, index($0,$3));
            else
                printf "%-5s %-20s %s\n", NR, $1" "$2, substr($0, index($0,$3));
        }' || true

    return 0
}

# Smart history search with timestamps and deduplication.
# Shows only the LAST execution of each unique command.
#
# ARCHITECTURE (v1.5.3+): the function is split in two planes to keep it safe
# for both humans and headless/LLM callers.
#
#   * DATA PLANE  - a single gawk pass extracts, matches, dedups (last-wins,
#     keeping an execution count), sorts and applies reverse/limit, emitting
#     canonical `epoch \t count \t command` records. This is the stable
#     contract consumed by --json, --count, pipes and non-TTY callers. No
#     per-line forks (old code forked `date` and `sed` once PER result line).
#
#   * PRESENTATION PLANE - grouping, centralized colors, execution-count badges
#     and fzf navigation. Enabled ONLY when stdout is a TTY, GASH_HEADLESS is
#     off and neither --json nor --count is set. Invisible to LLM/gash-exec.
#
# Usage: hgrep PATTERN [OPTIONS]
# Options:
#   -n, --limit N       Show only last N results (default: unlimited)
#   -a, --all           Include commands with hgrep/history_grep
#   -r, --reverse       Show oldest first (default: newest last)
#   -j, --json          Output as JSON array
#   -H, --no-highlight  Disable pattern highlighting / force plain output
#   -E, --regex         Use extended regex (default: fixed string)
#   -c, --count         Only show count of matching commands
#   -g, --group         Collapse similar commands by chain prefix (TTY only)
#       --pick          Fuzzy-pick a command via fzf and print it (TTY only)
#       --no-color      Force-disable colors for this invocation
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
  -H, --no-highlight  Disable pattern highlighting / force plain output
  -E, --regex         Use extended regex (default: fixed string match)
  -c, --count         Only show count of unique matching commands
  -g, --group         Collapse similar commands under a shared chain prefix
      --pick          Fuzzy-select a matching command via fzf and print it
      --no-color      Force-disable colored output for this invocation

EXAMPLES:
  hgrep git                    Search for "git" commands
  hgrep "composer.*install"    Search with pattern
  hgrep docker -n 10           Last 10 docker commands
  hgrep -E "^git (push|pull)"  Regex: git push or pull at start
  hgrep npm -j                 JSON output for scripting
  hgrep make -c                Count unique make commands
  hgrep cd -g                  Group "cd X && ..." variants under one prefix
  hgrep git --pick             Fuzzy-pick a past git command (needs fzf)

OUTPUT FORMAT:
  [2025-01-15 14:32:01] git push origin main
  [2025-01-15 15:10:22] git commit -m "fix"

  With --json:
  [{"timestamp":"2025-01-15 14:32:01","epoch":1705330321,"count":1,"command":"git push origin main"},...]
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
    local group=0
    local pick=0
    local no_highlight=0
    local no_color_flag=0

    # -H forces the plain, uncolored data-plane rendering.
    [[ "${GASH_HEADLESS-}" == "1" ]] && no_highlight=1

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -n | --limit)
                if [[ -z "${2-}" || ! "${2-}" =~ ^[0-9]+$ ]]; then
                    __gash_error "Option -n requires a numeric argument"
                    return 1
                fi
                limit="$2"
                shift 2
                ;;
            -a | --all)
                include_self=1
                shift
                ;;
            -r | --reverse)
                reverse=1
                shift
                ;;
            -j | --json)
                json_output=1
                shift
                ;;
            -H | --no-highlight)
                no_highlight=1
                shift
                ;;
            -E | --regex)
                use_regex=1
                shift
                ;;
            -c | --count)
                count_only=1
                shift
                ;;
            -g | --group)
                group=1
                shift
                ;;
            --pick)
                pick=1
                shift
                ;;
            --no-color)
                no_color_flag=1
                shift
                ;;
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

    # -------------------------------------------------------------------------
    # DATA PLANE: one awk pass (POSIX awk: works with gawk, mawk, BSD awk)
    # extracts, matches and dedups (last-wins) -> "epoch \t count \t command";
    # sort orders by last epoch (honoring --reverse), tail applies --limit.
    # No per-line subprocesses.
    # -------------------------------------------------------------------------
    local sort_order="-k1,1n"
    [[ "$reverse" -eq 1 ]] && sort_order="-k1,1nr"
    local records
    records=$(awk -v pattern="$pattern" \
        -v include_self="$include_self" \
        -v use_regex="$use_regex" '
    BEGIN { lpattern = tolower(pattern) }
    /^#[0-9]+$/ {
        timestamp = substr($0, 2)
        if ((getline cmd) > 0) {
            gsub(/^[ \t]+|[ \t]+$/, "", cmd)
            if (cmd == "") next
            if (include_self == 0 && (cmd ~ /hgrep/ || cmd ~ /history_grep/)) next
            lcmd = tolower(cmd)
            matched = 0
            if (use_regex == 1) {
                if (lcmd ~ lpattern) matched = 1
            } else {
                if (index(lcmd, lpattern) > 0) matched = 1
            }
            if (matched) {
                last[cmd] = timestamp   # last occurrence wins (dedup)
                cnt[cmd]++              # keep execution count
            }
        }
    }
    END { for (c in last) printf "%s\t%s\t%s\n", last[c], cnt[c], c }
    ' "$histfile" | LC_ALL=C sort -t "$(printf '\t')" -s $sort_order |
        if [[ "$limit" -gt 0 ]]; then tail -n "$limit"; else cat; fi) || return 1

    # Handle empty results
    if [[ -z "$records" ]]; then
        if [[ "$count_only" -eq 1 ]]; then
            echo "0"
        elif [[ "$json_output" -eq 1 ]]; then
            echo "[]"
        fi
        return 0
    fi

    # Count mode: one unique command per record.
    if [[ "$count_only" -eq 1 ]]; then
        printf '%s\n' "$records" | wc -l | tr -d ' '
        return 0
    fi

    # JSON output mode: single gawk pass, strftime instead of per-line `date`.
    if [[ "$json_output" -eq 1 ]]; then
        printf '%s\n' "$records" | awk '
        BEGIN { printf "["; first = 1 }
        function jesc(s) {
            gsub(/\\/, "\\\\", s); gsub(/"/, "\\\"", s)
            gsub(/\n/, "\\n", s);  gsub(/\t/, "\\t", s)
            return s
        }
        {
            i1 = index($0, "\t"); ep = substr($0, 1, i1 - 1)
            rest = substr($0, i1 + 1)
            i2 = index(rest, "\t"); cn = substr(rest, 1, i2 - 1)
            cmd = substr(rest, i2 + 1)
            ds = strftime("%Y-%m-%d %H:%M:%S", ep)
            if (first) first = 0; else printf ","
            printf "{\"timestamp\":\"%s\",\"epoch\":%d,\"count\":%d,\"command\":\"%s\"}", \
                   ds, ep, cn, jesc(cmd)
        }
        END { print "]" }'
        return 0
    fi

    # -------------------------------------------------------------------------
    # PRESENTATION PLANE (TTY + interactive only). Non-TTY / headless callers
    # fall through to the plain data-plane rendering below.
    # -------------------------------------------------------------------------
    local interactive=0
    if [[ -t 1 && "${GASH_HEADLESS-}" != "1" && "$no_highlight" -eq 0 ]]; then
        interactive=1
    fi

    # Centralized color gating: __gash_want_colors honors env, TTY and --no-color.
    local want_color=0
    if [[ "$interactive" -eq 1 ]] && __gash_want_colors "$no_color_flag"; then
        want_color=1
    fi

    # Resolve color values from the centralized palette (byte-real ANSI).
    local c_off='' c_ts='' c_hl='' c_grp='' c_cnt='' c_cmd=''
    if [[ "$want_color" -eq 1 ]]; then
        printf -v c_off '%b' "$__GASH_COLOR_OFF"
        printf -v c_ts '%b' "$__GASH_CYAN"
        printf -v c_hl '%b' "$__GASH_BOLD_YELLOW"
        printf -v c_grp '%b' "$__GASH_BOLD_GREEN"
        printf -v c_cnt '%b' "$__GASH_COLOR_MUTED"
        printf -v c_cmd '%b' "$__GASH_BOLD_WHITE"
    fi

    # Highlight regex: raw pattern in regex mode, escaped in fixed-string mode.
    local hlpat="$pattern"
    if [[ "$use_regex" -eq 0 ]]; then
        hlpat=$(printf '%s' "$pattern" | sed 's/[][\.*^$()+?{}|]/\\&/g')
    fi

    # --pick: fuzzy navigation via fzf. Requires a TTY on both ends + fzf.
    if [[ "$pick" -eq 1 ]]; then
        if [[ "$interactive" -eq 0 || ! -t 0 ]]; then
            __gash_error "--pick requires an interactive terminal"
            return 1
        fi
        if ! command -v fzf >/dev/null 2>&1; then
            __gash_error "--pick requires fzf; install it or drop --pick"
            return 1
        fi
        local sel
        sel=$(printf '%s\n' "$records" | awk '
            {
                i1 = index($0, "\t"); ep = substr($0, 1, i1 - 1)
                rest = substr($0, i1 + 1)
                i2 = index(rest, "\t"); cmd = substr(rest, i2 + 1)
                printf "[%s] %s\n", strftime("%Y-%m-%d %H:%M:%S", ep), cmd
            }' | fzf --height=40% --reverse --no-sort \
            --prompt="hgrep> " --tiebreak=index) || return 0
        [[ -z "$sel" ]] && return 0
        # Strip the "[timestamp] " prefix, emit the bare command.
        printf '%s\n' "${sel#*] }"
        return 0
    fi

    # --group: collapse commands sharing a chain prefix (before the first
    # &&/||/;/| operator) into one header with ranked sub-rows. TTY only:
    # non-interactive callers get the flat data-plane rendering instead.
    if [[ "$group" -eq 1 && "$interactive" -eq 1 ]]; then
        printf '%s\n' "$records" | awk \
            -v c_off="$c_off" -v c_ts="$c_ts" -v c_hl="$c_hl" \
            -v c_grp="$c_grp" -v c_cnt="$c_cnt" -v c_cmd="$c_cmd" \
            -v hlpat="$hlpat" -v want_color="$want_color" '
        BEGIN { IGNORECASE = 1; nr = 0 }
        # Position (1-based) of the first chain operator, or 0 if none.
        function chainpos(s,   best, p, i, toks, nt) {
            nt = split("&& || ; |", toks, " ")
            best = 0
            for (i = 1; i <= nt; i++) {
                p = index(s, toks[i])
                if (p > 0 && (best == 0 || p < best)) best = p
            }
            return best
        }
        function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
        {
            i1 = index($0, "\t"); ep = substr($0, 1, i1 - 1)
            rest = substr($0, i1 + 1)
            i2 = index(rest, "\t"); cn = substr(rest, 1, i2 - 1)
            cmd = substr(rest, i2 + 1)
            nr++
            r_ep[nr] = ep; r_cn[nr] = cn; r_cmd[nr] = cmd
            p = chainpos(cmd)
            if (p > 1) key = trim(substr(cmd, 1, p - 1))
            else       key = cmd            # no operator -> singleton group
            r_key[nr] = key
            g_n[key]++
            members[key] = members[key] (members[key] == "" ? "" : SUBSEP) nr
            if (ep > g_max[key]) g_max[key] = ep
        }
        END {
            # Order groups by most-recent activity (max epoch) descending.
            ng = 0
            for (k in g_n) { ng++; gk[ng] = k }
            for (a = 1; a <= ng; a++)
                for (b = a + 1; b <= ng; b++)
                    if (g_max[gk[b]] > g_max[gk[a]]) { t = gk[a]; gk[a] = gk[b]; gk[b] = t }

            for (a = 1; a <= ng; a++) {
                k = gk[a]
                nm = split(members[k], mi, SUBSEP)
                if (nm <= 1) {
                    # Singleton: render like a normal flat row.
                    idx = mi[1]
                    render_row(r_ep[idx], r_cn[idx], r_cmd[idx])
                    continue
                }
                # Group header: shared prefix + variant count.
                printf "%s%s%s %s(%d variants)%s\n", c_grp, k, c_off, c_cnt, nm, c_off
                # Rank members: exec-count desc, then recency (epoch) desc.
                for (x = 1; x <= nm; x++)
                    for (y = x + 1; y <= nm; y++) {
                        ix = mi[x]; iy = mi[y]
                        if (r_cn[iy] + 0 > r_cn[ix] + 0 ||
                            (r_cn[iy] + 0 == r_cn[ix] + 0 && r_ep[iy] + 0 > r_ep[ix] + 0)) {
                            t = mi[x]; mi[x] = mi[y]; mi[y] = t
                        }
                    }
                for (x = 1; x <= nm; x++) {
                    idx = mi[x]
                    glyph = (x == nm) ? "  \xe2\x94\x94\xe2\x94\x80 " : "  \xe2\x94\x9c\xe2\x94\x80 "
                    suffix = substr(r_cmd[idx], length(k) + 1)
                    gsub(/^[ \t]+/, "", suffix)
                    badge = (r_cn[idx] + 0 > 1) ? sprintf(" %s(x%d)%s", c_cnt, r_cn[idx], c_off) : ""
                    ts = strftime("%H:%M:%S", r_ep[idx])
                    printf "%s%s%s%s%s%s%s%s\n", glyph, c_ts, ts, c_off, c_cmd, hl(suffix), c_off, badge
                }
            }
        }
        # Highlight the pattern inside a string (in-place gsub, case-insensitive).
        function hl(s) {
            if (want_color == 1 && hlpat != "") gsub(hlpat, c_hl "&" c_off c_cmd, s)
            return s
        }
        # Flat single-row renderer, shared by singleton groups.
        function render_row(ep, cn, cmd,   ts, badge) {
            ts = strftime("%Y-%m-%d %H:%M:%S", ep)
            badge = (cn + 0 > 1) ? sprintf(" %s(x%d)%s", c_cnt, cn, c_off) : ""
            printf "%s[%s]%s %s%s%s%s\n", c_ts, ts, c_off, c_cmd, hl(cmd), c_off, badge
        }'
        return 0
    fi

    # -------------------------------------------------------------------------
    # FLAT rendering: strftime + optional highlight/colors/count in a single
    # gawk pass. When non-interactive this collapses to the classic plain
    # "[date] command" format (byte-identical to the previous behavior).
    # -------------------------------------------------------------------------
    printf '%s\n' "$records" | awk \
        -v c_off="$c_off" -v c_ts="$c_ts" -v c_hl="$c_hl" \
        -v c_cnt="$c_cnt" -v c_cmd="$c_cmd" \
        -v hlpat="$hlpat" -v want_color="$want_color" -v show_count="$interactive" '
    BEGIN { IGNORECASE = 1 }
    function hl(s) {
        if (want_color == 1 && hlpat != "") gsub(hlpat, c_hl "&" c_off c_cmd, s)
        return s
    }
    {
        i1 = index($0, "\t"); ep = substr($0, 1, i1 - 1)
        rest = substr($0, i1 + 1)
        i2 = index(rest, "\t"); cn = substr(rest, 1, i2 - 1)
        cmd = substr(rest, i2 + 1)
        ts = strftime("%Y-%m-%d %H:%M:%S", ep)
        badge = (show_count == 1 && cn + 0 > 1) ? sprintf(" %s(x%d)%s", c_cnt, cn, c_off) : ""
        printf "%s[%s]%s %s%s%s%s\n", c_ts, ts, c_off, c_cmd, hl(cmd), c_off, badge
    }'
}

# -----------------------------------------------------------------------------
# Network
# -----------------------------------------------------------------------------

# Get your public IP address.
# Usage: ip_public [--no-color]
# Alias: myip
ip_public() {
    local no_color=0
    local a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
            -h | --help)
                needs_help "ip_public" "ip_public [--no-color]" \
                    "Displays your public IP address (via ipinfo.io). Alias: myip" \
                    "--help" && return 0
                ;;
        esac
    done

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

    # Local color scope (respects env, TTY, --no-color)
    eval "$(__gash_color_scope "$no_color")"
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
        # Honor the runtime color gate (headless, NO_COLOR, non-TTY pipe).
        local use_color=1
        __gash_use_color || use_color=0
        echo "$result" | awk -v c="$use_color" '{
            if (c == 1)
                printf "   \033[1;33m%-8s\033[0m \033[0;36m%-12s\033[0m %-4s \033[1;37m%-40s\033[0m\n", $2, $1, $3, $11
            else
                printf "   %-8s %-12s %-4s %-40s\n", $2, $1, $3, $11
        }'
    else
        __gash_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by name.
# Usage: process_kill PROCESS_NAME
# Alias: pk
process_kill() {
    needs_help "process_kill" "process_kill PROCESS_NAME [--force]" \
        "Kill processes whose executable name matches NAME exactly (SIGTERM first, SIGKILL for survivors). Alias: pk" \
        "${1-}" && return

    local process_name="${1-}"
    local force_flag="${2-}"
    __gash_require_arg "$process_name" "process name" "process_kill <process_name>" || return 1

    # Exact, case-insensitive match on the executable name: a loose
    # `ps | grep -i` would also hit redis-sentinel when asked for redis,
    # or an editor with the name anywhere in its argv.
    local pids
    pids=$(pgrep -ix "$process_name" 2>/dev/null || true)

    if [[ -z "$pids" ]]; then
        __gash_error "No process found with exact name '$process_name'. Use process_find (pf) to inspect candidates."
        return 1
    fi

    local count
    count=$(wc -w <<<"$pids")
    if [[ "$count" -gt 1 && "$force_flag" != "--force" ]]; then
        __gash_warning "$count processes match '$process_name': $(tr '\n' ' ' <<<"$pids")"
        if ! needs_confirm_prompt "Kill all of them?"; then
            __gash_info "Aborted."
            return 0
        fi
    fi

    # Graceful shutdown first: SIGTERM, brief wait, SIGKILL only survivors
    # (a straight kill -9 gives databases no chance to flush to disk).
    local pid
    for pid in $pids; do
        kill "$pid" 2>/dev/null || true
    done

    local waited=0
    while ((waited < 30)); do
        local alive=0
        for pid in $pids; do
            kill -0 "$pid" 2>/dev/null && alive=1
        done
        [[ $alive -eq 0 ]] && break
        sleep 0.1
        waited=$((waited + 1))
    done

    for pid in $pids; do
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null || true
            __gash_info "Process $pid ignored SIGTERM; killed with SIGKILL."
        else
            __gash_info "Process $pid terminated."
        fi
    done
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
        # SIGTERM first, SIGKILL only for survivors
        local pid
        for pid in $pids; do
            kill "$pid" 2>/dev/null || true
        done
        local waited=0
        while ((waited < 30)); do
            local alive=0
            for pid in $pids; do
                kill -0 "$pid" 2>/dev/null && alive=1
            done
            [[ $alive -eq 0 ]] && break
            sleep 0.1
            waited=$((waited + 1))
        done
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
                __gash_info "Process $pid on port $port ignored SIGTERM; killed with SIGKILL."
            else
                __gash_info "Process $pid on port $port terminated."
            fi
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
        # Re-run the last command VERBATIM via `bash -c` (an unquoted
        # `sudo $(fc -ln -1)` would re-split words and re-expand globs,
        # potentially running something different as root). Show it first.
        local last_cmd
        last_cmd=$(fc -ln -1)
        last_cmd="${last_cmd#"${last_cmd%%[![:space:]]*}"}"
        __gash_warning "About to run as root: $last_cmd"
        if needs_confirm_prompt "Proceed?"; then
            sudo bash -c "$last_cmd"
        else
            __gash_info "Aborted."
            return 0
        fi
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
# WSL (Windows Subsystem for Linux)
# -----------------------------------------------------------------------------

# Restart WSL by saving bash history and triggering wsl --shutdown.
# WSL restarts automatically when a new terminal is opened.
# Alias: wr
wsl_restart() {
    needs_help "wsl_restart" "wsl_restart" \
        "Restart WSL (saves bash history first). Alias: wr" \
        "${1-}" && return

    __gash_require_wsl || return 1

    __gash_info "Saving bash history..."
    history -a
    __gash_info "Shutting down WSL (will restart on next launch)..."
    cmd.exe /C wsl --shutdown
}

# Shutdown WSL by saving bash history and triggering wsl --shutdown.
# Alias: wsd
wsl_shutdown() {
    needs_help "wsl_shutdown" "wsl_shutdown" \
        "Shutdown WSL (saves bash history first). Alias: wsd" \
        "${1-}" && return

    __gash_require_wsl || return 1

    __gash_info "Saving bash history..."
    history -a
    __gash_info "Shutting down WSL..."
    cmd.exe /C wsl --shutdown
}

# Open Windows Explorer in the current or specified directory.
# Alias: wex
wsl_explorer() {
    needs_help "wsl_explorer" "wsl_explorer [PATH]" \
        "Open Windows Explorer in the current or specified directory. Alias: wex" \
        "${1-}" && return

    __gash_require_wsl || return 1

    local target="${1:-.}"
    explorer.exe "$target"
}

# Open Windows Task Manager from WSL.
# Alias: wtm
wsl_taskmanager() {
    needs_help "wsl_taskmanager" "wsl_taskmanager" \
        "Open Windows Task Manager from WSL. Alias: wtm" \
        "${1-}" && return

    __gash_require_wsl || return 1

    (cd /mnt/c && cmd.exe /C taskmgr &)
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
alias wr='wsl_restart'
alias wsd='wsl_shutdown'
alias wex='wsl_explorer'
alias wtm='wsl_taskmanager'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "disk_usage" \
        --aliases "du2" \
        --module "system" \
        --short "Display disk usage for real filesystem types" \
        --see-also "dirs_largest dirs_find_large" \
        <<'HELP'
USAGE
  disk_usage

EXAMPLES
  # Show disk usage for ext4, xfs, btrfs, zfs, etc.
  du2

NOTES
  Filters out virtual filesystems (tmpfs, devtmpfs, overlay).
  Shows only real disk partitions with human-readable sizes.
HELP

    __gash_register_help "history_grep" \
        --aliases "hg" \
        --module "system" \
        --short "Search command history with colored output" \
        --see-also "hgrep" \
        <<'HELP'
USAGE
  history_grep PATTERN

EXAMPLES
  # Find all git commands in history
  hg git

  # Find docker-compose commands
  hg "docker compose"

  # Find SSH connections
  hg "ssh "

NOTES
  Simple and fast. For advanced features (timestamps,
  deduplication, JSON output), use hgrep instead.
HELP

    __gash_register_help "hgrep" \
        --module "system" \
        --short "Smart history search with timestamps and deduplication" \
        --see-also "history_grep" \
        <<'HELP'
USAGE
  hgrep PATTERN [OPTIONS]

OPTIONS
  -n LIMIT        Show only last N results (default: all)
  -a, --all       Include the hgrep command itself in results
  -r, --reverse   Reverse order (oldest first)
  -j, --json      Output as JSON array
  -H, --no-highlight  Disable color highlighting of matches
  -E, --regex     Treat PATTERN as extended regex
  -c, --count     Only show count of matching commands

EXAMPLES
  # Search for git push commands
  hgrep "git push"

  # Last 10 docker commands
  hgrep docker -n 10

  # Export history as JSON
  hgrep deploy -j

  # Count how many times you ran a command
  hgrep "npm install" -c

  # Regex: find git push or pull
  hgrep -E "git (push|pull)"

  # Reverse order (oldest first)
  hgrep kubectl -r
HELP

    __gash_register_help "ip_public" \
        --aliases "myip" \
        --module "system" \
        --short "Get your public IP address" \
        <<'HELP'
USAGE
  ip_public

EXAMPLES
  # Show your public IP
  myip

  # Use in a script
  MY_IP=$(myip) && echo "Server IP: $MY_IP"
HELP

    __gash_register_help "process_find" \
        --aliases "pf" \
        --module "system" \
        --short "Search for a process by name" \
        --see-also "process_kill port_kill" \
        <<'HELP'
USAGE
  process_find PROCESS_NAME

EXAMPLES
  # Find nginx processes
  pf nginx

  # Find node processes
  pf node

  # Check if a service is running
  pf mysql
HELP

    __gash_register_help "process_kill" \
        --aliases "pk" \
        --module "system" \
        --short "Kill all processes matching a name" \
        --see-also "process_find port_kill" \
        <<'HELP'
USAGE
  process_kill PROCESS_NAME

EXAMPLES
  # Kill all node processes
  pk node

  # Kill a stuck service
  pk webpack-dev-server
HELP

    __gash_register_help "port_kill" \
        --aliases "ptk" \
        --module "system" \
        --short "Kill processes listening on a specific port" \
        --see-also "process_find process_kill" \
        <<'HELP'
USAGE
  port_kill PORT

EXAMPLES
  # Kill whatever is on port 3000
  ptk 3000

  # Free up port 8080 before starting a dev server
  ptk 8080

  # Clear port 5432 (PostgreSQL default)
  ptk 5432

NOTES
  Requires lsof. Uses kill -9 (SIGKILL).
HELP

    __gash_register_help "services_stop" \
        --aliases "svs" \
        --module "system" \
        --short "Stop well-known services (Apache, MySQL, Redis, Docker, etc.)" \
        --see-also "process_kill" \
        <<'HELP'
USAGE
  services_stop [--force]

EXAMPLES
  # Stop services with confirmation prompt
  svs

  # Skip confirmation
  svs --force

SERVICES STOPPED
  apache2, nginx, mysql, mariadb, postgresql,
  mongodb, redis, memcached, docker

NOTES
  Only stops services that are currently active.
  Uses systemctl, so it requires systemd.
HELP

    __gash_register_help "sudo_last" \
        --aliases "plz" \
        --module "system" \
        --short "Re-run last command with sudo" \
        <<'HELP'
USAGE
  sudo_last [command]

EXAMPLES
  # Oops, forgot sudo? Re-run with sudo
  apt update        # -> Permission denied
  plz               # -> sudo apt update

  # Run a specific command with sudo
  plz systemctl restart nginx
HELP

    __gash_register_help "mkdir_cd" \
        --aliases "mkcd" \
        --module "system" \
        --short "Create a directory and cd into it" \
        <<'HELP'
USAGE
  mkdir_cd DIRECTORY

EXAMPLES
  # Create and enter a new project directory
  mkcd my-new-project

  # Create nested directories and enter
  mkcd ~/projects/client/frontend
HELP

    __gash_register_help "wsl_restart" \
        --aliases "wr" \
        --module "system" \
        --short "Restart WSL (saves history, runs wsl --shutdown)" \
        --see-also "wsl_shutdown" \
        <<'HELP'
USAGE
  wsl_restart

EXAMPLES
  # Restart WSL (saves bash history first)
  wsl_restart

NOTES
  Saves bash history before shutdown so no commands are lost.
  WSL will restart automatically when you open a new terminal.
  Only works inside WSL (Windows Subsystem for Linux).
HELP

    __gash_register_help "wsl_shutdown" \
        --aliases "wsd" \
        --module "system" \
        --short "Shutdown WSL (saves history, runs wsl --shutdown)" \
        --see-also "wsl_restart" \
        <<'HELP'
USAGE
  wsl_shutdown

EXAMPLES
  # Shutdown WSL (saves bash history first)
  wsd

NOTES
  Saves bash history before shutdown so no commands are lost.
  Only works inside WSL (Windows Subsystem for Linux).
HELP

    __gash_register_help "wsl_explorer" \
        --aliases "wex" \
        --module "system" \
        --short "Open Windows Explorer from WSL" \
        --see-also "wsl_taskmanager" \
        <<'HELP'
USAGE
  wsl_explorer [PATH]

EXAMPLES
  # Open Explorer in current directory
  wex

  # Open Explorer in a specific path
  wex /home/user/projects

  # Open Explorer in Windows user folder
  wex /mnt/c/Users

NOTES
  Defaults to current directory if no path specified.
  Only works inside WSL (Windows Subsystem for Linux).
HELP

    __gash_register_help "wsl_taskmanager" \
        --aliases "wtm" \
        --module "system" \
        --short "Open Windows Task Manager from WSL" \
        --see-also "wsl_explorer process_find" \
        <<'HELP'
USAGE
  wsl_taskmanager

EXAMPLES
  # Open Windows Task Manager
  wtm

NOTES
  Launches Task Manager in the background.
  Does not change your current working directory.
  Only works inside WSL (Windows Subsystem for Linux).
HELP

fi # end help registration guard
