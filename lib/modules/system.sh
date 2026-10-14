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

# Display disk usage for real (disk-backed) filesystems.
# Usage: disk_usage [--no-color]
# Alias: du2
disk_usage() {
    needs_help "disk_usage" "disk_usage [--no-color]" \
        "Displays disk usage for real filesystem types, formatted for easy reading. Alias: du2" \
        "${1-}" && return

    local no_color=0 a
    for a in "$@"; do
        case "$a" in
            --no-color) no_color=1 ;;
            *)
                __gash_error "disk_usage: unknown option '$a'"
                return 1
                ;;
        esac
    done
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"

    # POSIX output in KiB: one line per filesystem, mount point last. GNU df
    # adds the type with -T; BSD/macOS df has no -T, so keep /dev devices.
    local raw typed=1
    raw="$(df -PTk 2>/dev/null)" || raw=""
    if [[ -z "$raw" ]]; then
        typed=0
        raw="$(df -Pk 2>/dev/null)" || raw=""
    fi
    [[ -n "$raw" ]] || {
        __gash_error "disk_usage: df failed"
        return 1
    }

    local rows
    rows="$(printf '%s\n' "$raw" | "${__GASH_AWK:-awk}" -v typed="$typed" '
        function h(k,   u, i) {
            split("KiB MiB GiB TiB PiB", u, " ")
            i = 1
            while (k >= 1024 && i < 5) { k /= 1024; i++ }
            return sprintf(k < 10 ? "%.1f %s" : "%.0f %s", k, u[i])
        }
        NR == 1 { next }
        {
            if (typed) { type = $2; f = 3 } else { type = "-"; f = 2 }
            size = $f; used = $(f + 1); avail = $(f + 2); pct = $(f + 3)
            mnt = $(f + 4); for (i = f + 5; i <= NF; i++) mnt = mnt " " $i
            if (typed) {
                if (type !~ /^(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|exfat|msdos|ntfs|ntfs3|fuseblk|apfs|hfs|reiserfs|jfs)$/ &&
                    !(type ~ /^(drvfs|9p)$/ && mnt ~ /^\/mnt\/[a-z]$/)) next
            } else if ($1 !~ /^\/dev\//) next
            if (seen[mnt]++) next
            sub(/%$/, "", pct)
            printf "%s\t%s\t%s\t%s\t%s\t%s\t%d\n", mnt, $1, type, h(size), h(used), h(avail), pct + 0
        }
    ')"
    if [[ -z "$rows" ]]; then
        __gash_info "No disk-backed filesystems found (df lists only virtual ones)."
        return 0
    fi

    local B="${__GASH_BOLD}" M="${__GASH_COLOR_MUTED}" C="${__GASH_CYAN}" R="${__GASH_COLOR_OFF}"
    __gash_use_color || { B="" M="" C="" R=""; }

    # Column widths from the data.
    local mnt dev type size used avail pct
    local wm=5 wd=6 wt=4 ws=4 wu=4 wa=4
    while IFS=$'\t' read -r mnt dev type size used avail pct; do
        ((${#mnt} > wm)) && wm=${#mnt}
        ((${#dev} > wd)) && wd=${#dev}
        ((${#type} > wt)) && wt=${#type}
        ((${#size} > ws)) && ws=${#size}
        ((${#used} > wu)) && wu=${#used}
        ((${#avail} > wa)) && wa=${#avail}
    done <<<"$rows"
    ((wd > 28)) && wd=28

    echo
    printf '  %s%-*s  %-*s  %-*s  %*s  %*s  %*s  %s%s\n' "$B" \
        "$wm" "MOUNT" "$wd" "DEVICE" "$wt" "TYPE" "$ws" "SIZE" "$wu" "USED" "$wa" "FREE" "USE" "$R"
    local bar pc
    while IFS=$'\t' read -r mnt dev type size used avail pct; do
        ((${#dev} > wd)) && dev="${__GASH_GLYPH_ELLIPSIS}${dev: -$((wd - 1))}"
        pc="${__GASH_GREEN}"
        ((pct >= 75)) && pc="${__GASH_YELLOW}"
        ((pct >= 90)) && pc="${__GASH_BOLD_RED}"
        __gash_use_color || pc=""
        __gash_bar bar "$pct" 100 10
        printf '  %s%-*s%s  %s%-*s%s  %-*s  %*s  %*s  %*s  %s%s%s%s%s%s %s%3d%%%s\n' \
            "$C" "$wm" "$mnt" "$R" "$M" "$wd" "$dev" "$R" "$wt" "$type" \
            "$ws" "$size" "$wu" "$used" "$wa" "$avail" \
            "$pc" "${bar%%$'\t'*}" "$R" "$M" "${bar#*$'\t'}" "$R" "$pc" "$pct" "$R"
    done <<<"$rows"
    echo
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
    records=$("${__GASH_AWK:-awk}" -v pattern="$pattern" \
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
        printf '%s\n' "$records" | "${__GASH_AWK:-awk}" '
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
        sel=$(printf '%s\n' "$records" | "${__GASH_AWK:-awk}" '
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
        printf '%s\n' "$records" | "${__GASH_AWK:-awk}" \
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
    printf '%s\n' "$records" | "${__GASH_AWK:-awk}" \
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
    __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"
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

# Kill all processes by name, after listing them and asking (-y skips the
# question). Without a terminal to ask (GASH_HEADLESS=1, no /dev/tty)
# nothing is killed unless -y.
# Usage: process_kill [-y] PROCESS_NAME
# Alias: pk
process_kill() {
    needs_help "process_kill" "process_kill [-y] PROCESS_NAME" \
        "Kill processes whose executable name matches NAME exactly (SIGTERM first, SIGKILL for survivors), after asking. Alias: pk" \
        "${1-}" && return

    local process_name="" force=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -y | --yes | -f | --force) force=1 ;;
            -*)
                __gash_error "Unknown option: $1"
                return 1
                ;;
            *) process_name="$1" ;;
        esac
        shift
    done
    __gash_require_arg "$process_name" "process name" "process_kill [-y] <process_name>" || return 1

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
    count=$(($(wc -w <<<"$pids")))
    if [[ $force -eq 0 ]]; then
        # Same rule as port_kill: nobody to ask means no kill without -y.
        if [[ "${GASH_HEADLESS-}" == "1" ]] || ! { : </dev/tty; } 2>/dev/null; then
            local pid_list
            pid_list="$(tr '\n' ' ' <<<"$pids")"
            __gash_error "$count process(es) match '$process_name' (PID ${pid_list% }). Refusing to kill without a terminal to confirm; pass -y."
            return 1
        fi
        local pid cmd
        __gash_info "Matching '$process_name':"
        for pid in $pids; do
            cmd=$(ps -o args= -p "$pid" 2>/dev/null) || cmd="?"
            printf '  %s  %s\n' "$pid" "${cmd:0:100}" >&2
        done
        if ! needs_confirm_prompt "Kill $( ((count > 1)) && echo "all $count" || echo it)?"; then
            __gash_info "Aborted."
            return 1
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

# Terminate the processes listening on a port (TCP listeners and UDP
# sockets bound to it; clients connected to that port are left alone).
# Lists them and asks first unless --force/-f.
# Usage: port_kill [--force] PORT
# Alias: ptk
port_kill() {
    needs_help "port_kill" "port_kill [-y] PORT" "Terminate the processes listening on PORT. Alias: ptk" "${1-}" && return

    local force=0 port=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -y | --yes | -f | --force) force=1 ;;
            -*)
                __gash_error "Unknown option: $1"
                return 1
                ;;
            *) port="$1" ;;
        esac
        shift
    done

    __gash_require_arg "$port" "port" "port_kill [-y] <port>" || return 1
    if ! [[ "$port" =~ ^[0-9]+$ ]] || ((10#$port < 1 || 10#$port > 65535)); then
        __gash_error "Invalid port: $port (expected 1-65535)."
        return 1
    fi
    port=$((10#$port))
    __gash_require_command "lsof" "This function requires 'lsof', which is not available." || return 1

    local pids
    pids=$({
        lsof -t -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null
        lsof -t -nP -iUDP:"$port" 2>/dev/null
    } | sort -un)

    if [[ -z "$pids" ]]; then
        __gash_error "Nothing is listening on port $port."
        return 1
    fi

    local pid cmd
    __gash_info "Listening on port $port:"
    for pid in $pids; do
        cmd=$(ps -o comm= -p "$pid" 2>/dev/null) || cmd="?"
        printf '  %s  %s\n' "$pid" "${cmd:-?}" >&2
    done
    if [[ $force -eq 0 ]] && ! needs_confirm_prompt "Terminate these processes?"; then
        __gash_info "Aborted."
        return 1
    fi

    # SIGTERM first, SIGKILL only for survivors after ~3s
    for pid in $pids; do
        kill "$pid" 2>/dev/null || true
    done
    local waited=0 alive
    while ((waited < 30)); do
        alive=0
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
}

# -----------------------------------------------------------------------------
# Service Management
# -----------------------------------------------------------------------------

# Stop well-known services like Apache, Nginx, MySQL, MariaDB, PostgreSQL, Redis, etc.
# Usage: services_stop [--force]
# Alias: svs
services_stop() {
    needs_help "services_stop" "services_stop [-y] [--dry-run]" "Stop well-known services (web servers, databases, docker). Alias: svs" "${1-}" && return

    local assume_yes=0 dry_run=0 arg
    for arg in "$@"; do
        case "$arg" in
            -y | --yes | -f | --force) assume_yes=1 ;;
            --dry-run) dry_run=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: services_stop [-y] [--dry-run])"
                return 1
                ;;
        esac
    done

    # systemd when it is PID 1; the SysV `service` wrapper otherwise (WSL
    # without systemd, containers).
    local manager=""
    if [[ -d "${__GASH_SYSTEMD_RUNDIR:-/run/systemd/system}" ]] && command -v systemctl >/dev/null 2>&1; then
        manager=systemd
    elif command -v service >/dev/null 2>&1; then
        manager=service
    else
        __gash_error "No service manager found (systemd or service)."
        return 1
    fi

    local services=("apache2" "nginx" "mysql" "mariadb" "postgresql" "mongodb" "redis" "redis-server" "memcached" "docker")
    local -a running=()
    local service
    for service in "${services[@]}"; do
        if [[ "$manager" == systemd ]]; then
            systemctl is-active --quiet "$service" 2>/dev/null && running+=("$service")
        else
            service "$service" status >/dev/null 2>&1 && running+=("$service")
        fi
    done

    if ((${#running[@]} == 0)); then
        __gash_info "None of the known services is running."
        return 0
    fi
    __gash_info "Running: ${running[*]}"
    ((dry_run)) && return 0
    if ((assume_yes == 0)) && ! needs_confirm_prompt "Stop these services?"; then
        __gash_info "Aborted."
        return 1
    fi

    local rc=0
    for service in "${running[@]}"; do
        __gash_info "Stopping $service..."
        if [[ "$manager" == systemd ]]; then
            sudo systemctl stop "$service" || rc=1
        else
            sudo service "$service" stop || rc=1
        fi
    done
    return "$rc"
}

# -----------------------------------------------------------------------------
# Miscellaneous
# -----------------------------------------------------------------------------

# Run last command with sudo.
# Usage: sudo_last [command]
# Alias: plz
sudo_last() {
    needs_help "sudo_last" "sudo_last [COMMAND...]" "Run COMMAND, or the previous command after confirmation, with sudo. Alias: plz" "${1-}" && return
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
    needs_help "mkdir_cd" "mkdir_cd DIRECTORY" "Create DIRECTORY and cd into it. Alias: mkcd" "${1-}" && return
    local dir="${1-}"

    __gash_require_arg "$dir" "directory" "mkdir_cd <directory>" || return 1

    mkdir -p "$dir" && cd "$dir" || return 1
}

# -----------------------------------------------------------------------------
# WSL (Windows Subsystem for Linux)
# -----------------------------------------------------------------------------

# Run a Windows command from a Windows directory, so cmd.exe does not
# complain about a \\wsl$ UNC working directory.
__gash_wsl_cmd() {
    (
        cd /mnt/c 2>/dev/null || cd /
        cmd.exe /C "$@"
    )
}

# Stop the WSL VM (every distro, Docker Desktop's included) after saving
# this shell's history; WSL starts again with the next terminal. Asks
# first unless -y.
__gash_wsl_stop_vm() {
    local name="$1" assume_yes=0 arg
    shift
    for arg in "$@"; do
        case "$arg" in
            -y | --yes) assume_yes=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: $name [-y])"
                return 1
                ;;
        esac
    done

    __gash_require_wsl || return 1

    if ((assume_yes == 0)) && ! needs_confirm_prompt "Shut down WSL: every distro, Docker Desktop and running servers stop?"; then
        __gash_info "Aborted."
        return 1
    fi
    history -a 2>/dev/null || true
    __gash_info "History saved. Shutting down WSL (it starts again with the next terminal)..."
    __gash_wsl_cmd wsl --shutdown
}

# Alias: wr
wsl_restart() {
    needs_help "wsl_restart" "wsl_restart [-y]" \
        "Shut down WSL so that the next terminal starts it fresh (saves bash history first). Alias: wr" \
        "${1-}" && return
    __gash_wsl_stop_vm wsl_restart "$@"
}

# Alias: wsd
wsl_shutdown() {
    needs_help "wsl_shutdown" "wsl_shutdown [-y]" \
        "Shut down WSL (saves bash history first). Alias: wsd" \
        "${1-}" && return
    __gash_wsl_stop_vm wsl_shutdown "$@"
}

# Open Windows Explorer in the current or specified directory.
# Alias: wex
wsl_explorer() {
    needs_help "wsl_explorer" "wsl_explorer [PATH]" \
        "Open Windows Explorer in the current or specified directory. Alias: wex" \
        "${1-}" && return

    __gash_require_wsl || return 1

    local target="${1:-.}" win
    if [[ ! -e "$target" ]]; then
        __gash_error "No such file or directory: $target"
        return 1
    fi
    # Explorer understands Windows paths only (\\wsl.localhost\... for
    # Linux files); a Linux path makes it open Documents instead.
    win="$(wslpath -w "$target" 2>/dev/null)" || win="$target"
    # explorer.exe exits 1 even when the window opened.
    explorer.exe "$win" || true
}

# Open Windows Task Manager from WSL.
# Alias: wtm
wsl_taskmanager() {
    needs_help "wsl_taskmanager" "wsl_taskmanager" \
        "Open Windows Task Manager from WSL. Alias: wtm" \
        "${1-}" && return

    __gash_require_wsl || return 1

    (__gash_wsl_cmd taskmgr &)
}

# One line "MemFree|SReclaimable|free blocks of 512 KiB or more per zone",
# from /proc/meminfo and /proc/buddyinfo (orders 7 and up).
__gash_wsl_mem_snapshot() {
    local free slab blocks
    free="$(awk '/^MemFree:/ { printf "%d MiB", $2 / 1024 }' /proc/meminfo)"
    slab="$(awk '/^SReclaimable:/ { printf "%d MiB", $2 / 1024 }' /proc/meminfo)"
    blocks="$(awk '{ n = 0; for (i = 12; i <= NF; i++) n += $i * 2 ^ (i - 12); printf "%s%s=%d", sep, $4, n; sep = " " }' /proc/buddyinfo 2>/dev/null)"
    printf '%s|%s|%s\n' "$free" "$slab" "$blocks"
}

# Unblock Windows interop on WSL2 when memory is fragmented: drop the page
# cache and slab, compact memory, then check that a Windows program starts.
#
# Symptom: code ., explorer.exe or cmd.exe fail with
#   ERROR: UtilAcceptVsock:...: accept4 failed 110
# Each interop launch opens an hv_sock channel whose ring buffer needs a
# contiguous order-7 (512 KiB) block; on fragmented RAM the allocation fails
# (dmesg: "page allocation failure: order:7" under vmbus_alloc_ring) and the
# Windows side never connects back (110 = ETIMEDOUT).
# Usage: wsl_compact [--status]
# Alias: wslc
wsl_compact() {
    needs_help "wsl_compact" "wsl_compact [--status]" \
        "Drop caches and compact memory so Windows interop works again on a fragmented WSL2 VM. Alias: wslc" \
        "${1-}" && return

    local status_only=0 arg
    for arg in "$@"; do
        case "$arg" in
            --status) status_only=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: wsl_compact [--status])"
                return 1
                ;;
        esac
    done

    __gash_require_wsl || return 1

    local fails last
    fails="$(dmesg 2>/dev/null | grep -c 'page allocation failure: order:7' || true)"
    last="$(dmesg -T 2>/dev/null | grep 'page allocation failure: order:7' | tail -n 1 | sed -E 's/^\[([^]]+)\].*/\1/')"
    printf 'order-7 allocation failures since the VM started: %s%s\n' "${fails:-0}" "${last:+ (last: $last)}"

    local before after b_free b_slab b_blocks a_free a_slab a_blocks
    before="$(__gash_wsl_mem_snapshot)"
    if ((status_only)); then
        IFS='|' read -r b_free b_slab b_blocks <<<"$before"
        printf '%-22s %s\n' "MemFree" "$b_free" "SReclaimable (slab)" "$b_slab" "free >=512KiB blocks" "$b_blocks"
        return 0
    fi

    __gash_info "Dropping the page cache and slab, then compacting memory (sudo)..."
    if ! sudo sh -c 'sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory'; then
        __gash_error "drop_caches/compact_memory failed."
        return 1
    fi
    after="$(__gash_wsl_mem_snapshot)"

    IFS='|' read -r b_free b_slab b_blocks <<<"$before"
    IFS='|' read -r a_free a_slab a_blocks <<<"$after"
    printf '%-22s %-26s %s\n' "" "before" "after"
    printf '%-22s %-26s %s\n' "MemFree" "$b_free" "$a_free"
    printf '%-22s %-26s %s\n' "SReclaimable (slab)" "$b_slab" "$a_slab"
    printf '%-22s %-26s %s\n' "free >=512KiB blocks" "$b_blocks" "$a_blocks"

    local cmd=/mnt/c/Windows/System32/cmd.exe
    if [[ -x "$cmd" ]]; then
        if timeout 20 "$cmd" /c exit 0 >/dev/null 2>&1; then
            __gash_success "Windows interop works."
        else
            __gash_error "Windows interop still fails. Last resort, from Windows: wsl --shutdown"
            return 1
        fi
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias du2='disk_usage'
# Mercurial is hg too: keep the alias off when it is installed.
type -P hg >/dev/null 2>&1 || alias hg='history_grep'
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
alias wslc='wsl_compact'

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
  disk_usage [--no-color]

EXAMPLES
  # Show disk usage for ext4, xfs, btrfs, zfs, etc.
  du2

NOTES
  Filters out virtual filesystems (tmpfs, devtmpfs, overlay, fuse).
  Windows drives under WSL (/mnt/c, ...) are shown.
  The bar turns yellow at 75% and red at 90%.
  On macOS (no df -T) every /dev device is listed, without a type.
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
  process_kill [-y] PROCESS_NAME

EXAMPLES
  # Kill all node processes (lists them and asks first)
  pk node

  # No confirmation (scripts, agents)
  pk -y webpack-dev-server

NOTES
  Matches the executable name exactly, ignoring case, lists the matches
  with their command lines and asks. Without a terminal to ask
  (GASH_HEADLESS=1, no /dev/tty) nothing is killed unless -y is given.
  --force is the same as -y. SIGTERM first, SIGKILL for processes still
  alive after about 3 seconds. Declining returns 1.
HELP

    __gash_register_help "port_kill" \
        --aliases "ptk" \
        --module "system" \
        --short "Kill processes listening on a specific port" \
        --see-also "process_find process_kill" \
        <<'HELP'
USAGE
  port_kill [-y] PORT

EXAMPLES
  # Stop whatever listens on port 3000 (lists it and asks first)
  ptk 3000

  # No confirmation (scripts, agents)
  ptk -y 3000

NOTES
  --force is the same as -y. Requires lsof. Only listeners are targeted: a browser or ssh session
  connected to that port elsewhere is left alone. SIGTERM first,
  SIGKILL for processes still alive after about 3 seconds.
HELP

    __gash_register_help "services_stop" \
        --aliases "svs" \
        --module "system" \
        --short "Stop well-known services (Apache, MySQL, Redis, Docker, etc.)" \
        --see-also "process_kill" \
        <<'HELP'
USAGE
  services_stop [-y] [--dry-run]

EXAMPLES
  # List the running ones and ask before stopping them
  svs

  # Only list them
  svs --dry-run

  # Skip confirmation
  svs -y

SERVICES STOPPED
  apache2, nginx, mysql, mariadb, postgresql,
  mongodb, redis, redis-server, memcached, docker

NOTES
  Only stops services that are currently active, through systemd when it
  runs, else through `service` (WSL without systemd, containers).
  --force is the same as -y. Returns 1 when declined or when a stop
  fails.
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
  wsl_restart [-y]

EXAMPLES
  # Restart WSL (asks, saves bash history first)
  wr

  # Without the question
  wr -y

NOTES
  Runs `wsl --shutdown`: every distro stops, Docker Desktop's included,
  with the servers running in them. WSL starts again with the next
  terminal. Asks first; -y skips the question. Same as wsl_shutdown.
HELP

    __gash_register_help "wsl_shutdown" \
        --aliases "wsd" \
        --module "system" \
        --short "Shutdown WSL (saves history, runs wsl --shutdown)" \
        --see-also "wsl_restart" \
        <<'HELP'
USAGE
  wsl_shutdown [-y]

EXAMPLES
  # Shut down WSL (asks, saves bash history first)
  wsd

NOTES
  Runs `wsl --shutdown`: every distro stops, Docker Desktop's included.
  Asks first; -y skips the question.
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

    __gash_register_help "wsl_compact" \
        --aliases "wslc" \
        --module "system" \
        --short "Fix Windows interop on a WSL2 VM with fragmented memory" \
        --see-also "wsl_restart wsl_shutdown" \
        <<'HELP'
USAGE
  wsl_compact [--status]

EXAMPLES
  # code . or explorer.exe fail with "UtilAcceptVsock ... accept4 failed 110"
  wslc

  # Only show the failures and the free large blocks, change nothing
  wslc --status

NOTES
  Every Windows program started from WSL needs a contiguous 512 KiB block
  of guest memory for its hv_sock channel. When memory is fragmented the
  allocation fails (dmesg: "page allocation failure: order:7") and the
  launch times out. wsl_compact drops the page cache and slab, compacts
  memory (sudo), prints the free blocks before and after, and checks that
  cmd.exe starts. If it still fails, run `wsl --shutdown` from Windows.
HELP

fi # end help registration guard
