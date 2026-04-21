#!/usr/bin/env bash

# Gash Module: File Operations
# Functions for file and directory analysis, extraction, and backup.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   files_largest (flf)     - List largest files with filters (size/json/exclude/...)
#   dirs_largest (dld)      - List largest directories with depth control
#   dirs_find_large (dfl)   - Find directories exceeding size threshold (single-walk)
#   dirs_list_empty (dle)   - List empty directories with filters
#   tree_stats (tls)        - Compact filesystem statistics: totals, extensions, depth
#   archive_extract (axe)   - Extract various archive types
#   file_backup (fbk)       - Create timestamped file backup
#
# Design invariants (v1.5+):
#   - Legacy signatures (PATH-only call) are preserved.
#   - All new flags are additive. --json emits a machine-readable envelope;
#     --null emits NUL-delimited paths; --human is auto-on on TTY.
#   - Single-walk algorithms: dirs_find_large and dirs_largest use `du -k`
#     once and aggregate. Old code re-walked each candidate subtree -> O(N^2).
#   - Scanning "/" requires --allow-root. By default --xdev is NOT on, but
#     --allow-root implies a warning.
#   - Default prune list matches llm_find / llm_tree (node_modules, vendor,
#     .git, __pycache__, .cache) for cross-module consistency.

# =============================================================================
# Private helpers (namespace: __gash_fs_*)
# =============================================================================

# Default directory names pruned from scans unless --no-ignore is passed.
# Kept in sync with llm_find / llm_tree / llm_grep.
__GASH_FS_DEFAULT_PRUNE=(
    node_modules
    vendor
    .git
    __pycache__
    .cache
)

# Parse a human-readable size string ("20M", "1G", "500K", "1024") to bytes.
# Uses numfmt when available, falls back to pure-bash integer arithmetic.
# Accepts IEC suffixes (K KB KiB, M MB MiB, G GB GiB, T TB TiB, P PB PiB),
# case-insensitive. Plain integers are bytes.
#
# Usage: bytes=$(__gash_fs_parse_size "20M") || return 1
# Prints bytes to stdout on success. Returns 1 on invalid input.
__gash_fs_parse_size() {
    local input="${1-}"
    [[ -z "$input" ]] && return 1

    # Try numfmt first (handles "1.5G" decimals that pure-bash can't)
    if command -v numfmt >/dev/null 2>&1; then
        local nf
        nf="$(numfmt --from=iec "$input" 2>/dev/null)" || nf=""
        if [[ "$nf" =~ ^[0-9]+$ ]]; then
            printf '%s' "$nf"
            return 0
        fi
    fi

    # Pure-bash fallback: integer-only, IEC units.
    # Strip spaces, normalize unit.
    local s="${input// /}"
    local num="${s//[^0-9]/}"
    [[ -z "$num" ]] && return 1
    local unit="${s//[0-9]/}"
    # Strip optional trailing "B" and "iB" -> keep letter
    unit="${unit%B}"
    unit="${unit%i}"
    case "${unit^^}" in
        "")         printf '%s' "$num" ;;
        K)          printf '%s' $(( num * 1024 )) ;;
        M)          printf '%s' $(( num * 1024 * 1024 )) ;;
        G)          printf '%s' $(( num * 1024 * 1024 * 1024 )) ;;
        T)          printf '%s' $(( num * 1024 * 1024 * 1024 * 1024 )) ;;
        P)          printf '%s' $(( num * 1024 * 1024 * 1024 * 1024 * 1024 )) ;;
        *)          return 1 ;;
    esac
}

# Format a byte count into human-readable IEC with one decimal.
# Pure-bash integer arithmetic (no floating-point).
# Examples: 0 -> "0 B", 1536 -> "1.5 KiB", 1572864 -> "1.5 MiB"
#
# Usage: __gash_fs_human_size 1048576
__gash_fs_human_size() {
    local bytes="${1-0}"
    [[ "$bytes" =~ ^[0-9]+$ ]] || { printf '0 B'; return 0; }

    local -a units=(B KiB MiB GiB TiB PiB EiB)
    local i=0
    local size="$bytes"
    local rem=0
    while (( size >= 1024 && i < ${#units[@]} - 1 )); do
        rem=$(( (size * 10 / 1024) % 10 ))
        size=$(( size / 1024 ))
        i=$(( i + 1 ))
    done

    if (( i == 0 )); then
        printf '%d %s' "$size" "${units[$i]}"
    else
        printf '%d.%d %s' "$size" "$rem" "${units[$i]}"
    fi
}

# Decide whether ANSI colors should be emitted.
# Honors: --no-color flag (passed as $1), NO_COLOR env, TTY check on stdout,
# and --json / --null modes (forced off).
#
# Usage: if __gash_fs_color_ok "$no_color" "$is_machine"; then ...
__gash_fs_color_ok() {
    local no_color="${1-0}"
    local is_machine="${2-0}"
    [[ "$no_color" -eq 1 ]] && return 1
    [[ "$is_machine" -eq 1 ]] && return 1
    [[ -n "${NO_COLOR-}" ]] && return 1
    [[ -t 1 ]] || return 1
    return 0
}

# Check whether GNU du supports --exclude (BSD du does not).
# Result cached in __GASH_FS_DU_HAS_EXCLUDE.
__gash_fs_du_has_exclude() {
    if [[ -z "${__GASH_FS_DU_HAS_EXCLUDE-}" ]]; then
        if du --help 2>&1 | grep -q -- '--exclude'; then
            __GASH_FS_DU_HAS_EXCLUDE=1
        else
            __GASH_FS_DU_HAS_EXCLUDE=0
        fi
    fi
    [[ "$__GASH_FS_DU_HAS_EXCLUDE" == "1" ]]
}

# Resolve a user-supplied path and reject dangerous targets.
# Rejects:
#   - "/" unless --allow-root was passed
#   - paths under /proc, /sys, /dev, /boot, /root, /etc/shadow-like
#   - non-directories
#
# Prints the resolved absolute path on stdout. Returns 1 on rejection.
# Usage: resolved=$(__gash_fs_safe_path "$input" "$allow_root" "$is_machine") || return 1
__gash_fs_safe_path() {
    local input="${1-}"
    local allow_root="${2-0}"
    local is_machine="${3-0}"

    [[ -z "$input" ]] && input="."

    # Expand tilde
    input="${input/#\~/$HOME}"

    # Distinguish "does not exist" from "exists but is not a directory" to give
    # users accurate feedback. cd only succeeds on real directories, so a cd
    # failure can mean either case.
    if [[ ! -e "$input" ]]; then
        __gash_fs_error "path_not_found" "Path '$input' does not exist" "RETRY" \
            "Provide an existing directory path" "$is_machine"
        return 1
    fi
    if [[ ! -d "$input" ]]; then
        __gash_fs_error "not_a_directory" "Path '$input' is not a directory" "RETRY" \
            "Provide a directory path (not a file or special node)" "$is_machine"
        return 1
    fi

    local resolved
    resolved="$(cd -- "$input" 2>/dev/null && pwd -P)" || resolved=""

    if [[ -z "$resolved" ]]; then
        __gash_fs_error "path_unreadable" "Path '$input' is not readable" "RETRY" \
            "Check directory permissions" "$is_machine"
        return 1
    fi

    # Root-of-fs guard
    if [[ "$resolved" == "/" && "$allow_root" -ne 1 ]]; then
        __gash_fs_error "root_scan_blocked" "Scanning '/' is blocked" "FATAL" \
            "Use --allow-root to override (slow, traverses virtual filesystems)" "$is_machine"
        return 1
    fi

    # Forbidden prefixes (match llm.sh forbidden paths)
    local forbidden
    for forbidden in /proc /sys /dev /boot /root; do
        if [[ "$resolved" == "$forbidden" || "$resolved" == "$forbidden"/* ]]; then
            __gash_fs_error "forbidden_path" "$resolved" "FATAL" \
                "This path is protected and cannot be scanned" "$is_machine"
            return 1
        fi
    done

    printf '%s' "$resolved"
    return 0
}

# Build find(1) prune arguments for the default noise dirs, plus user-supplied
# --exclude globs. The caller must follow the returned args with the terminator
# `-o <primary> -print` (or similar) to make prune actually skip entries.
#
# Usage: mapfile -t prune_args < <(__gash_fs_find_prune_args no_ignore extra_excludes...)
# Emits one argument per line (null-safe if the caller uses mapfile -d '').
__gash_fs_find_prune_args() {
    local no_ignore="${1-0}"
    shift
    local -a user_excludes=("$@")

    local -a parts=()
    if [[ "$no_ignore" -ne 1 ]]; then
        local name
        for name in "${__GASH_FS_DEFAULT_PRUNE[@]}"; do
            parts+=('(' '-type' 'd' '-name' "$name" '-prune' ')' '-o')
        done
    fi
    local pat
    for pat in "${user_excludes[@]}"; do
        [[ -z "$pat" ]] && continue
        parts+=('(' '-path' "$pat" '-prune' ')' '-o')
    done

    local p
    for p in "${parts[@]}"; do
        printf '%s\n' "$p"
    done
}

# Emit a structured error. In machine mode emits JSON to stderr using the
# same contract as llm_* functions. In human mode emits a colored "Error:"
# line via __gash_error.
#
# Usage: __gash_fs_error <type> <details> <action> <hint> <is_machine>
# Returns 1 always (caller: `__gash_fs_error ... || return 1`).
__gash_fs_error() {
    local err_type="${1-unknown_error}"
    local details="${2-}"
    local action="${3-STOP}"
    local hint="${4-}"
    local is_machine="${5-0}"

    if [[ "$is_machine" -eq 1 ]]; then
        local esc_details esc_hint
        esc_details=$(__gash_json_escape "$details")
        esc_hint=$(__gash_json_escape "$hint")
        local json="{\"error\":\"$err_type\""
        [[ -n "$details" ]] && json+=",\"details\":\"$esc_details\""
        json+=",\"action\":\"$action\",\"recoverable\":false"
        [[ -n "$hint" ]] && json+=",\"hint\":\"$esc_hint\""
        json+="}"
        printf '%s\n' "$json" >&2
    else
        local msg="$err_type"
        [[ -n "$details" ]] && msg="$msg: $details"
        __gash_error "$msg"
        # Hint line: honor the same gate as __gash_error (stdout TTY + env).
        # Use __gash_use_color to keep styling consistent across the two lines.
        if [[ -n "$hint" ]]; then
            if __gash_use_color; then
                echo -e "${__GASH_COLOR_MUTED}Hint: ${hint}${__GASH_COLOR_OFF}" >&2
            else
                printf 'Hint: %s\n' "$hint" >&2
            fi
        fi
    fi
    return 1
}

# Emit a scan-time warning (e.g. permission-denied count) to stderr.
# Silent in machine mode unless count > 0; then emits a compact JSON line.
__gash_fs_scan_warn() {
    local err_count="${1-0}"
    local is_machine="${2-0}"
    [[ "$err_count" -le 0 ]] && return 0
    if [[ "$is_machine" -eq 1 ]]; then
        printf '{"warning":"partial_results","skipped_entries":%d,"hint":"Some paths were unreadable (permissions)"}\n' \
            "$err_count" >&2
    else
        __gash_warning "Scan completed with ${err_count} unreadable path(s) (permission denied)."
    fi
}

# Current monotonic milliseconds (best-effort). Uses EPOCHREALTIME if available
# (bash 5.0+), falls back to `date +%s%N`.
__gash_fs_now_ms() {
    if [[ -n "${EPOCHREALTIME-}" ]]; then
        local s="${EPOCHREALTIME%%.*}"
        local f="${EPOCHREALTIME#*.}"
        while [[ ${#f} -lt 6 ]]; do f="${f}0"; done
        printf '%s' $(( 10#$s * 1000 + 10#${f:0:3} ))
        return 0
    fi
    date +%s%3N 2>/dev/null || echo 0
}

# Format elapsed milliseconds to a compact string: "42ms", "1.23s", "2m3s".
__gash_fs_format_elapsed() {
    local ms="${1-0}"
    if (( ms < 1000 )); then
        printf '%dms' "$ms"
    elif (( ms < 60000 )); then
        printf '%d.%02ds' $(( ms / 1000 )) $(( (ms % 1000) / 10 ))
    else
        printf '%dm%ds' $(( ms / 60000 )) $(( (ms % 60000) / 1000 ))
    fi
}

# =============================================================================
# Public functions
# =============================================================================

# List the largest regular files in a directory tree, sorted by size.
# Usage: files_largest [PATH] [flags]
# Alias: flf
files_largest() {
    if needs_help "files_largest" \
        "files_largest [PATH] [--limit N] [--min-size SIZE] [--exclude GLOB] [--depth N] [--no-ignore] [--xdev] [--follow-symlinks] [--allow-root] [--json] [--null] [--human] [--no-color]" \
        "Lists the largest files under PATH (default 100), sorted by size. Alias: flf" \
        "${1-}"; then
        return 0
    fi

    # Defaults
    local dir="."
    local limit=100
    local min_size_input=""
    local min_size_bytes=0
    local max_depth=""
    local no_ignore=0
    local xdev=0
    local follow=0
    local allow_root=0
    local json_mode=0
    local null_mode=0
    local human_mode=""
    local no_color=0
    local -a excludes=()

    # Pre-scan for --json so that errors emitted INSIDE the arg-parse loop
    # (e.g. unknown flag) still produce the JSON envelope expected by callers
    # that pass --json along with a bad flag.
    local __a
    for __a in "$@"; do [[ "$__a" == "--json" ]] && { json_mode=1; break; }; done

    # Parse arguments (defer semantic validation until after the loop so
    # $json_mode and friends are fully known).
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --limit)            limit="${2-100}"; shift 2 ;;
            --min-size)         min_size_input="${2-}"; shift 2 ;;
            --exclude)          excludes+=("${2-}"); shift 2 ;;
            --depth|--max-depth) max_depth="${2-}"; shift 2 ;;
            --no-ignore)        no_ignore=1; shift ;;
            --xdev|--one-filesystem) xdev=1; shift ;;
            --follow-symlinks|-L) follow=1; shift ;;
            --allow-root)       allow_root=1; shift ;;
            --json)             json_mode=1; shift ;;
            --null|-0)          null_mode=1; shift ;;
            --human)            human_mode=1; shift ;;
            --no-human)         human_mode=0; shift ;;
            --no-color)         no_color=1; shift ;;
            -h|--help)
                needs_help "files_largest" "see usage" "" "--help" && return 0 ;;
            --) shift; break ;;
            -*)
                __gash_fs_error "unknown_option" "$1" "RETRY" "Use --help to see valid flags" "$json_mode"
                return 1 ;;
            *)
                dir="$1"; shift ;;
        esac
    done

    # Semantic validation (after parse, with $json_mode known)
    [[ "$limit" =~ ^[0-9]+$ ]] || { __gash_fs_error "invalid_limit" "$limit" "RETRY" "Provide a positive integer" "$json_mode"; return 1; }
    if [[ -n "$max_depth" && ! "$max_depth" =~ ^[0-9]+$ ]]; then
        __gash_fs_error "invalid_depth" "$max_depth" "RETRY" "Provide a non-negative integer" "$json_mode"
        return 1
    fi
    if [[ -n "$min_size_input" ]]; then
        if ! min_size_bytes=$(__gash_fs_parse_size "$min_size_input"); then
            __gash_fs_error "invalid_size" "$min_size_input" "RETRY" "Use a size like 10M, 1G, 500K" "$json_mode"
            return 1
        fi
    fi

    # Resolve path
    local safe_path
    safe_path="$(__gash_fs_safe_path "$dir" "$allow_root" "$json_mode")" || return 1

    # Auto-decide human mode
    if [[ -z "$human_mode" ]]; then
        if [[ "$json_mode" -eq 1 || "$null_mode" -eq 1 ]]; then human_mode=0; else human_mode=1; fi
    fi

    # Build find command
    local -a find_cmd=(find)
    [[ "$follow" -eq 1 ]] && find_cmd+=(-L)
    find_cmd+=("$safe_path")
    [[ "$xdev" -eq 1 ]] && find_cmd+=(-xdev)
    [[ -n "$max_depth" ]] && find_cmd+=(-maxdepth "$max_depth")

    # Prunes
    local prune_line
    while IFS= read -r prune_line; do
        [[ -n "$prune_line" ]] && find_cmd+=("$prune_line")
    done < <(__gash_fs_find_prune_args "$no_ignore" "${excludes[@]}")

    # Action: match only regular files. Apply min-size filter post-hoc in awk
    # to avoid fragile `-size +<N-1>c` arithmetic (N=0 -> invalid predicate).
    find_cmd+=(-type f -printf '%s\t%TY-%Tm-%Td %TH:%TM\t%p\n')

    local t0 t1 elapsed
    t0=$(__gash_fs_now_ms)

    # Capture find stderr (permission denials) to count
    local err_tmp
    err_tmp="$(mktemp)" || err_tmp=""

    local sorted
    if [[ -n "$err_tmp" ]]; then
        sorted=$("${find_cmd[@]}" 2>"$err_tmp" \
            | awk -F'\t' -v min_b="$min_size_bytes" 'NF >= 3 && ($1 + 0) >= min_b' \
            | sort -nr -k1,1 -t$'\t' \
            | head -n "$limit")
    else
        sorted=$("${find_cmd[@]}" 2>/dev/null \
            | awk -F'\t' -v min_b="$min_size_bytes" 'NF >= 3 && ($1 + 0) >= min_b' \
            | sort -nr -k1,1 -t$'\t' \
            | head -n "$limit")
    fi

    t1=$(__gash_fs_now_ms)
    elapsed=$(__gash_fs_format_elapsed $(( t1 - t0 )))

    local err_count=0
    if [[ -n "$err_tmp" && -s "$err_tmp" ]]; then
        err_count=$(wc -l < "$err_tmp")
        rm -f "$err_tmp"
    else
        [[ -n "$err_tmp" ]] && rm -f "$err_tmp"
    fi

    # Render
    if [[ "$json_mode" -eq 1 ]]; then
        __gash_fs_render_size_list_json "$sorted" "$elapsed" "$err_count" "files"
    elif [[ "$null_mode" -eq 1 ]]; then
        # NUL-delimited paths only (for xargs -0)
        printf '%s' "$sorted" | awk -F'\t' 'NF>=3 { printf "%s\0", $3 }'
    else
        __gash_fs_render_size_list_text "$sorted" "$human_mode" "$no_color" "files"
        __gash_fs_scan_warn "$err_count" 0
    fi

    return 0
}

# List the largest directories under a path.
# Usage: dirs_largest [PATH] [flags]
# Alias: dld
dirs_largest() {
    if needs_help "dirs_largest" \
        "dirs_largest [PATH] [--limit N] [--depth N] [--min-size SIZE] [--exclude GLOB] [--no-ignore] [--xdev] [--allow-root] [--json] [--null] [--human] [--no-color]" \
        "Lists the largest directories under PATH, sorted by cumulative size. Default depth 1 (top-level). Alias: dld" \
        "${1-}"; then
        return 0
    fi

    local dir="."
    local limit=100
    local max_depth=1
    local min_size_input=""
    local min_size_bytes=0
    local no_ignore=0
    local xdev=0
    local allow_root=0
    local json_mode=0
    local null_mode=0
    local human_mode=""
    local no_color=0
    local -a excludes=()

    local __a
    for __a in "$@"; do [[ "$__a" == "--json" ]] && { json_mode=1; break; }; done

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --limit)            limit="${2-100}"; shift 2 ;;
            --depth|--max-depth) max_depth="${2-1}"; shift 2 ;;
            --min-size)         min_size_input="${2-}"; shift 2 ;;
            --exclude)          excludes+=("${2-}"); shift 2 ;;
            --no-ignore)        no_ignore=1; shift ;;
            --xdev|--one-filesystem) xdev=1; shift ;;
            --allow-root)       allow_root=1; shift ;;
            --json)             json_mode=1; shift ;;
            --null|-0)          null_mode=1; shift ;;
            --human)            human_mode=1; shift ;;
            --no-human)         human_mode=0; shift ;;
            --no-color)         no_color=1; shift ;;
            -h|--help)
                needs_help "dirs_largest" "see usage" "" "--help" && return 0 ;;
            --) shift; break ;;
            -*)
                __gash_fs_error "unknown_option" "$1" "RETRY" "Use --help to see valid flags" "$json_mode"
                return 1 ;;
            *)
                dir="$1"; shift ;;
        esac
    done

    [[ "$limit" =~ ^[0-9]+$ ]] || { __gash_fs_error "invalid_limit" "$limit" "RETRY" "Provide a positive integer" "$json_mode"; return 1; }
    [[ "$max_depth" =~ ^[0-9]+$ ]] || { __gash_fs_error "invalid_depth" "$max_depth" "RETRY" "Provide a positive integer" "$json_mode"; return 1; }
    [[ "$max_depth" -lt 1 ]] && max_depth=1
    if [[ -n "$min_size_input" ]]; then
        if ! min_size_bytes=$(__gash_fs_parse_size "$min_size_input"); then
            __gash_fs_error "invalid_size" "$min_size_input" "RETRY" "Use a size like 10M, 1G" "$json_mode"
            return 1
        fi
    fi

    local safe_path
    safe_path="$(__gash_fs_safe_path "$dir" "$allow_root" "$json_mode")" || return 1

    if [[ -z "$human_mode" ]]; then
        if [[ "$json_mode" -eq 1 || "$null_mode" -eq 1 ]]; then human_mode=0; else human_mode=1; fi
    fi

    # Build du command (single walk).
    local -a du_cmd=(du -k -d "$max_depth")
    [[ "$xdev" -eq 1 ]] && du_cmd+=(-x)

    # Add --exclude for default prunes if GNU du supports it (fast path).
    local gnu_exclude=0
    if __gash_fs_du_has_exclude; then
        gnu_exclude=1
        if [[ "$no_ignore" -ne 1 ]]; then
            local name
            for name in "${__GASH_FS_DEFAULT_PRUNE[@]}"; do
                du_cmd+=(--exclude="$name")
            done
        fi
        local pat
        for pat in "${excludes[@]}"; do
            [[ -n "$pat" ]] && du_cmd+=(--exclude="$pat")
        done
    fi

    du_cmd+=(-- "$safe_path")

    local t0 t1 elapsed
    t0=$(__gash_fs_now_ms)

    local err_tmp
    err_tmp="$(mktemp)" || err_tmp=""

    # du output: <kb>\t<path>\n (assumption: no tab/newline in path).
    # Convert to bytes and filter excluded patterns in awk when GNU exclude unavailable.
    local raw
    if [[ -n "$err_tmp" ]]; then
        raw=$("${du_cmd[@]}" 2>"$err_tmp")
    else
        raw=$("${du_cmd[@]}" 2>/dev/null)
    fi

    # Post-filter in awk:
    #   - drop the top-level entry (== safe_path) to avoid dwarfing the list
    #   - apply default prunes and user excludes when GNU --exclude unavailable
    #   - apply --min-size (bytes)
    #   - convert kb -> bytes, format mtime via stat(1) would be too slow;
    #     we skip mtime for dirs (mtime-of-dir is not what users want; use
    #     dirs_find_large --with-mtime for that).
    local filtered
    filtered=$(
        printf '%s\n' "$raw" | awk -F'\t' \
            -v self="$safe_path" \
            -v min_b="$min_size_bytes" \
            -v fallback_prune="$([[ "$gnu_exclude" -eq 0 && "$no_ignore" -ne 1 ]] && printf 1 || printf 0)" \
            -v fallback_prune_list="$(IFS='|'; echo "${__GASH_FS_DEFAULT_PRUNE[*]}")" \
            -v user_excludes="$(IFS='|'; echo "${excludes[*]}")" \
            '
            BEGIN {
                if (fallback_prune) {
                    n_prune = split(fallback_prune_list, prune_arr, "|")
                }
                n_user = split(user_excludes, user_arr, "|")
            }
            function path_blocked(p,   i, seg) {
                if (fallback_prune) {
                    for (i = 1; i <= n_prune; i++) {
                        if (prune_arr[i] == "") continue
                        seg = "/" prune_arr[i]
                        if (p ~ seg "/" || p ~ seg "$") return 1
                    }
                }
                for (i = 1; i <= n_user; i++) {
                    if (user_arr[i] == "") continue
                    # user excludes are interpreted as substring for simplicity
                    if (index(p, user_arr[i]) > 0) return 1
                }
                return 0
            }
            NF < 2 { next }
            {
                kb = $1 + 0
                path = $2
                for (i = 3; i <= NF; i++) path = path "\t" $i
                if (path == self) next
                if (path_blocked(path)) next
                bytes = kb * 1024
                if (bytes < min_b) next
                printf "%d\t\t%s\n", bytes, path
            }
            '
    )

    t1=$(__gash_fs_now_ms)
    elapsed=$(__gash_fs_format_elapsed $(( t1 - t0 )))

    local err_count=0
    if [[ -n "$err_tmp" && -s "$err_tmp" ]]; then
        err_count=$(wc -l < "$err_tmp")
        rm -f "$err_tmp"
    else
        [[ -n "$err_tmp" ]] && rm -f "$err_tmp"
    fi

    local sorted
    sorted=$(printf '%s\n' "$filtered" | sort -nr -k1,1 -t$'\t' | head -n "$limit")

    if [[ "$json_mode" -eq 1 ]]; then
        __gash_fs_render_size_list_json "$sorted" "$elapsed" "$err_count" "directories"
    elif [[ "$null_mode" -eq 1 ]]; then
        printf '%s' "$sorted" | awk -F'\t' 'NF>=3 { printf "%s\0", $3 }'
    else
        __gash_fs_render_size_list_text "$sorted" "$human_mode" "$no_color" "directories"
        __gash_fs_scan_warn "$err_count" 0
    fi

    return 0
}

# Find directories exceeding a size threshold (single-walk: du aggregates once).
# Usage: dirs_find_large [--size SIZE] [DIRECTORY] [flags]
# Alias: dfl
dirs_find_large() {
    if needs_help "dirs_find_large" \
        "dirs_find_large [DIRECTORY] [--size SIZE] [--depth N] [--limit N] [--exclude GLOB] [--no-ignore] [--xdev] [--allow-root] [--with-mtime] [--json] [--null] [--human] [--no-color]" \
        "Finds directories larger than SIZE (default 20M), single-walk aggregation. Alias: dfl" \
        "${1-}"; then
        return 0
    fi

    local dir="."
    local size_threshold="20M"
    local max_depth=""
    local limit=1000
    local no_ignore=0
    local xdev=0
    local allow_root=0
    local json_mode=0
    local null_mode=0
    local human_mode=""
    local no_color=0
    local with_mtime=0
    local -a excludes=()

    local __a
    for __a in "$@"; do [[ "$__a" == "--json" ]] && { json_mode=1; break; }; done

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --size)             size_threshold="${2-20M}"; shift 2 ;;
            --depth|--max-depth) max_depth="${2-}"; shift 2 ;;
            --limit)            limit="${2-1000}"; shift 2 ;;
            --exclude)          excludes+=("${2-}"); shift 2 ;;
            --no-ignore)        no_ignore=1; shift ;;
            --xdev|--one-filesystem) xdev=1; shift ;;
            --allow-root)       allow_root=1; shift ;;
            --with-mtime)       with_mtime=1; shift ;;
            --json)             json_mode=1; shift ;;
            --null|-0)          null_mode=1; shift ;;
            --human)            human_mode=1; shift ;;
            --no-human)         human_mode=0; shift ;;
            --no-color)         no_color=1; shift ;;
            -h|--help)
                needs_help "dirs_find_large" "see usage" "" "--help" && return 0 ;;
            --) shift; break ;;
            -*)
                __gash_fs_error "unknown_option" "$1" "RETRY" "Use --help to see valid flags" "$json_mode"
                return 1 ;;
            *)
                dir="$1"; shift ;;
        esac
    done

    local threshold_bytes
    if ! threshold_bytes=$(__gash_fs_parse_size "$size_threshold"); then
        __gash_fs_error "invalid_size" "$size_threshold" "RETRY" "Use a size like 20M, 1G, 500K" "$json_mode"
        return 1
    fi
    [[ "$limit" =~ ^[0-9]+$ ]] || { __gash_fs_error "invalid_limit" "$limit" "RETRY" "Provide a positive integer" "$json_mode"; return 1; }
    if [[ -n "$max_depth" && ! "$max_depth" =~ ^[0-9]+$ ]]; then
        __gash_fs_error "invalid_depth" "$max_depth" "RETRY" "Provide a non-negative integer" "$json_mode"
        return 1
    fi

    local safe_path
    safe_path="$(__gash_fs_safe_path "$dir" "$allow_root" "$json_mode")" || return 1

    if [[ -z "$human_mode" ]]; then
        if [[ "$json_mode" -eq 1 || "$null_mode" -eq 1 ]]; then human_mode=0; else human_mode=1; fi
    fi

    local threshold_kb=$(( (threshold_bytes + 1023) / 1024 ))

    # Build du command. Default depth = unbounded (matches legacy).
    local -a du_cmd=(du -k)
    [[ "$xdev" -eq 1 ]] && du_cmd+=(-x)
    [[ -n "$max_depth" && "$max_depth" -gt 0 ]] && du_cmd+=(-d "$max_depth")

    local gnu_exclude=0
    if __gash_fs_du_has_exclude; then
        gnu_exclude=1
        if [[ "$no_ignore" -ne 1 ]]; then
            local name
            for name in "${__GASH_FS_DEFAULT_PRUNE[@]}"; do
                du_cmd+=(--exclude="$name")
            done
        fi
        local pat
        for pat in "${excludes[@]}"; do
            [[ -n "$pat" ]] && du_cmd+=(--exclude="$pat")
        done
    fi

    du_cmd+=(-- "$safe_path")

    local t0 t1 elapsed
    t0=$(__gash_fs_now_ms)

    local err_tmp
    err_tmp="$(mktemp)" || err_tmp=""

    local raw
    if [[ -n "$err_tmp" ]]; then
        raw=$("${du_cmd[@]}" 2>"$err_tmp")
    else
        raw=$("${du_cmd[@]}" 2>/dev/null)
    fi

    # Filter by threshold and exclude patterns. Emit bytes\t(optional mtime)\tpath.
    # mtime is added AFTER filtering, only for entries that pass (keeps it cheap).
    local filtered
    filtered=$(
        printf '%s\n' "$raw" | awk -F'\t' \
            -v th_kb="$threshold_kb" \
            -v self="$safe_path" \
            -v fallback_prune="$([[ "$gnu_exclude" -eq 0 && "$no_ignore" -ne 1 ]] && printf 1 || printf 0)" \
            -v fallback_prune_list="$(IFS='|'; echo "${__GASH_FS_DEFAULT_PRUNE[*]}")" \
            -v user_excludes="$(IFS='|'; echo "${excludes[*]}")" \
            '
            BEGIN {
                if (fallback_prune) n_prune = split(fallback_prune_list, prune_arr, "|")
                n_user = split(user_excludes, user_arr, "|")
            }
            function path_blocked(p,   i, seg) {
                if (fallback_prune) {
                    for (i = 1; i <= n_prune; i++) {
                        if (prune_arr[i] == "") continue
                        seg = "/" prune_arr[i]
                        if (p ~ seg "/" || p ~ seg "$") return 1
                    }
                }
                for (i = 1; i <= n_user; i++) {
                    if (user_arr[i] == "") continue
                    if (index(p, user_arr[i]) > 0) return 1
                }
                return 0
            }
            NF < 2 { next }
            {
                kb = $1 + 0
                path = $2
                for (i = 3; i <= NF; i++) path = path "\t" $i
                if (kb < th_kb) next
                if (path == self && NR == 1) next   # skip self-root on first line when descending
                if (path_blocked(path)) next
                printf "%d\t%s\n", kb * 1024, path
            }
            ' | sort -nr -k1,1 -t$'\t' | head -n "$limit"
    )

    # Enrich with mtime if requested (post-filter, only for matching rows).
    local enriched="$filtered"
    if [[ "$with_mtime" -eq 1 && -n "$filtered" ]]; then
        enriched=$(
            printf '%s\n' "$filtered" | while IFS=$'\t' read -r bytes path; do
                [[ -z "$path" ]] && continue
                local mtime
                mtime=$(find "$path" -maxdepth 10 -type f -printf '%T@\t%TY-%Tm-%Td %TH:%TM\n' 2>/dev/null \
                        | sort -nr -k1,1 -t$'\t' | head -n1 | cut -f2)
                printf '%s\t%s\t%s\n' "$bytes" "${mtime:-N/A}" "$path"
            done
        )
    else
        enriched=$(printf '%s\n' "$filtered" | awk -F'\t' 'NF>=2 { printf "%s\t\t%s\n", $1, $2 }')
    fi

    t1=$(__gash_fs_now_ms)
    elapsed=$(__gash_fs_format_elapsed $(( t1 - t0 )))

    local err_count=0
    if [[ -n "$err_tmp" && -s "$err_tmp" ]]; then
        err_count=$(wc -l < "$err_tmp")
        rm -f "$err_tmp"
    else
        [[ -n "$err_tmp" ]] && rm -f "$err_tmp"
    fi

    if [[ "$json_mode" -eq 1 ]]; then
        __gash_fs_render_size_list_json "$enriched" "$elapsed" "$err_count" "directories"
    elif [[ "$null_mode" -eq 1 ]]; then
        printf '%s' "$enriched" | awk -F'\t' 'NF>=3 { printf "%s\0", $3 }'
    else
        __gash_fs_render_size_list_text "$enriched" "$human_mode" "$no_color" "directories"
        __gash_fs_scan_warn "$err_count" 0
    fi

    return 0
}

# List empty directories under PATH.
# Usage: dirs_list_empty [PATH] [flags]
# Alias: dle
dirs_list_empty() {
    if needs_help "dirs_list_empty" \
        "dirs_list_empty [PATH] [--min-depth N] [--depth N] [--exclude GLOB] [--ignore-dotfiles] [--no-ignore] [--xdev] [--allow-root] [--null] [--count] [--json]" \
        "Lists all empty directories under PATH. Alias: dle" \
        "${1-}"; then
        return 0
    fi

    local dir="."
    local min_depth=""
    local max_depth=""
    local ignore_dotfiles=0
    local no_ignore=0
    local xdev=0
    local allow_root=0
    local null_mode=0
    local count_only=0
    local json_mode=0
    local -a excludes=()

    local __a
    for __a in "$@"; do [[ "$__a" == "--json" ]] && { json_mode=1; break; }; done

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --min-depth)        min_depth="${2-}"; shift 2 ;;
            --depth|--max-depth) max_depth="${2-}"; shift 2 ;;
            --exclude)          excludes+=("${2-}"); shift 2 ;;
            --ignore-dotfiles)  ignore_dotfiles=1; shift ;;
            --no-ignore)        no_ignore=1; shift ;;
            --xdev|--one-filesystem) xdev=1; shift ;;
            --allow-root)       allow_root=1; shift ;;
            --null|-0)          null_mode=1; shift ;;
            --count|-c)         count_only=1; shift ;;
            --json)             json_mode=1; shift ;;
            -h|--help)
                needs_help "dirs_list_empty" "see usage" "" "--help" && return 0 ;;
            --) shift; break ;;
            -*)
                __gash_fs_error "unknown_option" "$1" "RETRY" "Use --help to see valid flags" "$json_mode"
                return 1 ;;
            *)
                dir="$1"; shift ;;
        esac
    done

    if [[ -n "$min_depth" && ! "$min_depth" =~ ^[0-9]+$ ]]; then
        __gash_fs_error "invalid_depth" "$min_depth" "RETRY" "Provide a non-negative integer" "$json_mode"
        return 1
    fi
    if [[ -n "$max_depth" && ! "$max_depth" =~ ^[0-9]+$ ]]; then
        __gash_fs_error "invalid_depth" "$max_depth" "RETRY" "Provide a non-negative integer" "$json_mode"
        return 1
    fi

    local safe_path
    safe_path="$(__gash_fs_safe_path "$dir" "$allow_root" "$json_mode")" || return 1

    # find -type d -empty, with prunes.
    local -a find_cmd=(find "$safe_path")
    [[ "$xdev" -eq 1 ]] && find_cmd+=(-xdev)
    [[ -n "$min_depth" ]] && find_cmd+=(-mindepth "$min_depth")
    [[ -n "$max_depth" ]] && find_cmd+=(-maxdepth "$max_depth")

    local prune_line
    while IFS= read -r prune_line; do
        [[ -n "$prune_line" ]] && find_cmd+=("$prune_line")
    done < <(__gash_fs_find_prune_args "$no_ignore" "${excludes[@]}")

    find_cmd+=(-type d -empty)
    [[ "$ignore_dotfiles" -eq 1 ]] && find_cmd+=(! -name '.*')
    find_cmd+=(-print)

    local err_tmp
    err_tmp="$(mktemp)" || err_tmp=""

    local output
    if [[ -n "$err_tmp" ]]; then
        output=$("${find_cmd[@]}" 2>"$err_tmp")
    else
        output=$("${find_cmd[@]}" 2>/dev/null)
    fi

    local err_count=0
    if [[ -n "$err_tmp" && -s "$err_tmp" ]]; then
        err_count=$(wc -l < "$err_tmp")
        rm -f "$err_tmp"
    else
        [[ -n "$err_tmp" ]] && rm -f "$err_tmp"
    fi

    local count
    count=$(printf '%s' "$output" | awk 'NF { n++ } END { print n+0 }')

    if [[ "$json_mode" -eq 1 ]]; then
        local paths_json
        paths_json=$(
            printf '%s\n' "$output" | awk '
                NF { lines[++n] = $0 }
                END {
                    printf "["
                    for (i = 1; i <= n; i++) {
                        s = lines[i]
                        gsub(/\\/, "\\\\", s)
                        gsub(/"/,  "\\\"", s)
                        gsub(/\t/, "\\t",  s)
                        gsub(/\r/, "\\r",  s)
                        if (i > 1) printf ","
                        printf "\"%s\"", s
                    }
                    printf "]"
                }
            '
        )
        local esc_path
        esc_path=$(__gash_json_escape "$safe_path")
        printf '{"data":%s,"count":%d,"path":"%s","errors":%d}\n' \
            "$paths_json" "$count" "$esc_path" "$err_count"
        return 0
    fi

    if [[ "$count_only" -eq 1 ]]; then
        printf '%d\n' "$count"
        __gash_fs_scan_warn "$err_count" 0
        return 0
    fi

    if [[ "$null_mode" -eq 1 ]]; then
        printf '%s' "$output" | awk 'NF { printf "%s\0", $0 }'
    else
        printf '%s\n' "$output"
    fi
    __gash_fs_scan_warn "$err_count" 0
    return 0
}

# Compact filesystem statistics: totals, top extensions, depth, empty dirs.
# Single-walk aggregation via find(1) + awk.
# Usage: tree_stats [PATH] [flags]
# Alias: tls
tree_stats() {
    if needs_help "tree_stats" \
        "tree_stats [PATH] [--depth N] [--top N] [--exclude GLOB] [--no-ignore] [--xdev] [--follow-symlinks] [--allow-root] [--json] [--no-color]" \
        "Compact filesystem stats: totals, top extensions (by count & size), depth, empty dirs. Alias: tls" \
        "${1-}"; then
        return 0
    fi

    local dir="."
    local max_depth=""
    local top_n=5
    local no_ignore=0
    local xdev=0
    local follow=0
    local allow_root=0
    local json_mode=0
    local no_color=0
    local -a excludes=()

    local __a
    for __a in "$@"; do [[ "$__a" == "--json" ]] && { json_mode=1; break; }; done

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --depth|--max-depth) max_depth="${2-}"; shift 2 ;;
            --top)              top_n="${2-5}"; shift 2 ;;
            --exclude)          excludes+=("${2-}"); shift 2 ;;
            --no-ignore)        no_ignore=1; shift ;;
            --xdev|--one-filesystem) xdev=1; shift ;;
            --follow-symlinks|-L) follow=1; shift ;;
            --allow-root)       allow_root=1; shift ;;
            --json)             json_mode=1; shift ;;
            --no-color)         no_color=1; shift ;;
            -h|--help)
                needs_help "tree_stats" "see usage" "" "--help" && return 0 ;;
            --) shift; break ;;
            -*)
                __gash_fs_error "unknown_option" "$1" "RETRY" "Use --help to see valid flags" "$json_mode"
                return 1 ;;
            *)
                dir="$1"; shift ;;
        esac
    done

    if [[ -n "$max_depth" && ! "$max_depth" =~ ^[0-9]+$ ]]; then
        __gash_fs_error "invalid_depth" "$max_depth" "RETRY" "Provide a non-negative integer" "$json_mode"
        return 1
    fi
    [[ "$top_n" =~ ^[0-9]+$ ]] || { __gash_fs_error "invalid_top" "$top_n" "RETRY" "Provide a positive integer" "$json_mode"; return 1; }

    local safe_path
    safe_path="$(__gash_fs_safe_path "$dir" "$allow_root" "$json_mode")" || return 1

    local -a find_cmd=(find)
    [[ "$follow" -eq 1 ]] && find_cmd+=(-L)
    find_cmd+=("$safe_path")
    [[ "$xdev" -eq 1 ]] && find_cmd+=(-xdev)
    [[ -n "$max_depth" ]] && find_cmd+=(-maxdepth "$max_depth")

    local prune_line
    while IFS= read -r prune_line; do
        [[ -n "$prune_line" ]] && find_cmd+=("$prune_line")
    done < <(__gash_fs_find_prune_args "$no_ignore" "${excludes[@]}")

    # Emit per-entry: type\tsize\tdepth\tpath_relative
    # %y = single-char type, %s = size, %P = relative path
    find_cmd+=(\( -type f -o -type d \) -printf '%y\t%s\t%P\n')

    local t0 t1 elapsed
    t0=$(__gash_fs_now_ms)

    local err_tmp
    err_tmp="$(mktemp)" || err_tmp=""

    local raw
    if [[ -n "$err_tmp" ]]; then
        raw=$("${find_cmd[@]}" 2>"$err_tmp")
    else
        raw=$("${find_cmd[@]}" 2>/dev/null)
    fi

    t1=$(__gash_fs_now_ms)
    elapsed=$(__gash_fs_format_elapsed $(( t1 - t0 )))

    local err_count=0
    if [[ -n "$err_tmp" && -s "$err_tmp" ]]; then
        err_count=$(wc -l < "$err_tmp")
        rm -f "$err_tmp"
    else
        [[ -n "$err_tmp" ]] && rm -f "$err_tmp"
    fi

    # Awk aggregation: totals, per-extension count/size, max depth.
    # Extension extraction: everything after the last "." in the basename, if any,
    # lowercased. Files with no dot (or leading-dot-only like ".env") are classified
    # as "(no ext)".
    local stats_ndjson
    stats_ndjson=$(
        printf '%s\n' "$raw" | awk -F'\t' -v topN="$top_n" '
            function basename(p,   n, parts) {
                n = split(p, parts, "/")
                return parts[n]
            }
            function ext_of(name,   dot) {
                dot = 0
                # ignore leading dot (hidden files)
                if (substr(name, 1, 1) == ".") name = substr(name, 2)
                if (name == "") return "(no ext)"
                for (i = length(name); i > 0; i--) {
                    if (substr(name, i, 1) == ".") { dot = i; break }
                }
                if (dot == 0) return "(no ext)"
                return tolower(substr(name, dot + 1))
            }
            NF < 3 { next }
            {
                t = $1; sz = $2 + 0; path = $3
                for (i = 4; i <= NF; i++) path = path "\t" $i
                # depth = number of / in relative path; 0 for root itself
                depth = 0
                for (k = 1; k <= length(path); k++) {
                    if (substr(path, k, 1) == "/") depth++
                }
                if (depth > max_depth) max_depth = depth
                if (t == "f") {
                    files_total++
                    size_total += sz
                    e = ext_of(basename(path))
                    ext_cnt[e]++
                    ext_sz[e] += sz
                } else if (t == "d") {
                    dirs_total++
                }
            }
            END {
                # Build top by count and top by size (simple O(k*N) selection).
                n_ext = 0
                for (e in ext_cnt) { n_ext++; ext_list[n_ext] = e }

                # Top by count
                printf "files_total\t%d\n", files_total + 0
                printf "dirs_total\t%d\n", dirs_total + 0
                printf "size_total\t%d\n", size_total + 0
                printf "max_depth\t%d\n", max_depth + 0

                # Average file size
                avg = (files_total > 0) ? int(size_total / files_total) : 0
                printf "avg_file_size\t%d\n", avg

                # Top by count: repeated selection of max
                printf "top_by_count_begin\t\n"
                k = (topN < n_ext) ? topN : n_ext
                for (r = 0; r < k; r++) {
                    best = ""; best_cnt = -1
                    for (i = 1; i <= n_ext; i++) {
                        e = ext_list[i]
                        if (e == "") continue
                        if (ext_cnt[e] > best_cnt) { best_cnt = ext_cnt[e]; best = e }
                    }
                    if (best == "") break
                    printf "top_count\t%s\t%d\t%d\n", best, best_cnt, ext_sz[best] + 0
                    ext_cnt[best] = 0   # remove from pool
                    for (i = 1; i <= n_ext; i++) if (ext_list[i] == best) { ext_list[i] = ""; break }
                }
                printf "top_by_count_end\t\n"

                # Refresh counts from size_arr? We zeroed ext_cnt — rebuild via ext_sz.
                # Copy ext_sz to work_sz so we can zero out selected.
                for (e in ext_sz) work_sz[e] = ext_sz[e]
                # Need original count for display (we nuked ext_cnt): rescan stats
                # from ext_sz isn'\''t possible, so we store aux. Actually, let'\''s
                # record count in parallel and expose via a cached array.
                # -> Re-read: we already emitted top_count so this is for display only.
                printf "top_by_size_begin\t\n"
                n_ext2 = 0
                for (e in work_sz) { n_ext2++; list2[n_ext2] = e }
                k2 = (topN < n_ext2) ? topN : n_ext2
                for (r = 0; r < k2; r++) {
                    best = ""; best_sz = -1
                    for (i = 1; i <= n_ext2; i++) {
                        e = list2[i]
                        if (e == "") continue
                        if (work_sz[e] > best_sz) { best_sz = work_sz[e]; best = e }
                    }
                    if (best == "" || best_sz <= 0) break
                    # count is gone; emit 0 placeholder — rendering uses -- for count
                    printf "top_size\t%s\t%d\n", best, best_sz
                    work_sz[best] = -1
                    for (i = 1; i <= n_ext2; i++) if (list2[i] == best) { list2[i] = ""; break }
                }
                printf "top_by_size_end\t\n"
            }
        '
    )

    # Parse the NDJSON-ish output into shell vars / arrays.
    local files_total=0 dirs_total=0 size_total=0 max_depth_val=0 avg_file_size=0
    local -a top_count_rows=()
    local -a top_size_rows=()
    local key rest
    while IFS=$'\t' read -r key rest; do
        case "$key" in
            files_total)    files_total="$rest" ;;
            dirs_total)     dirs_total="$rest" ;;
            size_total)     size_total="$rest" ;;
            max_depth)      max_depth_val="$rest" ;;
            avg_file_size)  avg_file_size="$rest" ;;
            top_count)
                # rest still tab-separated: ext\tcount\tsize
                top_count_rows+=("$rest")
                ;;
            top_size)
                # ext\tsize
                top_size_rows+=("$rest")
                ;;
        esac
    done <<< "$stats_ndjson"

    # Count empty dirs (separate pass — cheap enough and stays accurate).
    local empty_count=0
    empty_count=$(find "$safe_path" ${xdev:+-xdev} -type d -empty 2>/dev/null | wc -l)

    if [[ "$json_mode" -eq 1 ]]; then
        local esc_path; esc_path=$(__gash_json_escape "$safe_path")
        # Build top_by_count array
        local tc_json="["
        local first=1 row ext cnt sz
        for row in "${top_count_rows[@]}"; do
            IFS=$'\t' read -r ext cnt sz <<< "$row"
            local esc_ext; esc_ext=$(__gash_json_escape "$ext")
            [[ $first -eq 0 ]] && tc_json+=","
            first=0
            tc_json+="{\"ext\":\"$esc_ext\",\"count\":${cnt:-0},\"size\":${sz:-0}}"
        done
        tc_json+="]"

        local ts_json="["
        first=1
        for row in "${top_size_rows[@]}"; do
            IFS=$'\t' read -r ext sz <<< "$row"
            local esc_ext; esc_ext=$(__gash_json_escape "$ext")
            [[ $first -eq 0 ]] && ts_json+=","
            first=0
            ts_json+="{\"ext\":\"$esc_ext\",\"size\":${sz:-0}}"
        done
        ts_json+="]"

        printf '{"data":{"path":"%s","files_total":%d,"dirs_total":%d,"size_total":%d,"size_total_human":"%s","avg_file_size":%d,"max_depth":%d,"empty_dirs":%d,"top_by_count":%s,"top_by_size":%s},"scan_time":"%s","errors":%d}\n' \
            "$esc_path" \
            "$files_total" "$dirs_total" "$size_total" \
            "$(__gash_fs_human_size "$size_total")" \
            "$avg_file_size" "$max_depth_val" "$empty_count" \
            "$tc_json" "$ts_json" \
            "$elapsed" "$err_count"
        return 0
    fi

    # Human-readable rendering
    local W="" G="" Y="" C="" M="" R=""
    if __gash_fs_color_ok "$no_color" 0; then
        W="${__GASH_BOLD_WHITE-}"; G="${__GASH_GREEN-}"; Y="${__GASH_BOLD_YELLOW-}"
        C="${__GASH_CYAN-}"; M="${__GASH_COLOR_MUTED-\033[38;5;245m}"; R="${__GASH_COLOR_OFF-}"
    fi

    echo -e "${W}Tree statistics for${R} ${C}${safe_path}${R}"
    printf '  %sFiles:%s       %s%d%s\n' "$W" "$R" "$Y" "$files_total" "$R"
    printf '  %sDirectories:%s %s%d%s\n' "$W" "$R" "$Y" "$dirs_total" "$R"
    printf '  %sTotal size:%s  %s%s%s (%d bytes)\n' "$W" "$R" "$G" "$(__gash_fs_human_size "$size_total")" "$R" "$size_total"
    printf '  %sAvg file:%s    %s%s%s\n' "$W" "$R" "$G" "$(__gash_fs_human_size "$avg_file_size")" "$R"
    printf '  %sMax depth:%s   %s%d%s\n' "$W" "$R" "$Y" "$max_depth_val" "$R"
    printf '  %sEmpty dirs:%s  %s%d%s\n' "$W" "$R" "$Y" "$empty_count" "$R"
    printf '  %sScan time:%s   %s%s%s\n' "$W" "$R" "$M" "$elapsed" "$R"

    if [[ ${#top_count_rows[@]} -gt 0 ]]; then
        echo
        echo -e "  ${W}Top ${top_n} extensions by count:${R}"
        local ext cnt sz
        for row in "${top_count_rows[@]}"; do
            IFS=$'\t' read -r ext cnt sz <<< "$row"
            printf '    %s%-12s%s %s%6d%s files  %s%s%s\n' \
                "$C" "$ext" "$R" "$Y" "$cnt" "$R" "$G" "$(__gash_fs_human_size "${sz:-0}")" "$R"
        done
    fi

    if [[ ${#top_size_rows[@]} -gt 0 ]]; then
        echo
        echo -e "  ${W}Top ${top_n} extensions by size:${R}"
        for row in "${top_size_rows[@]}"; do
            IFS=$'\t' read -r ext sz <<< "$row"
            printf '    %s%-12s%s %s%s%s\n' \
                "$C" "$ext" "$R" "$G" "$(__gash_fs_human_size "${sz:-0}")" "$R"
        done
    fi

    __gash_fs_scan_warn "$err_count" 0
    return 0
}

# =============================================================================
# Rendering helpers (shared between files_largest / dirs_largest / dirs_find_large)
# =============================================================================

# Render a size-list in machine-readable JSON envelope.
# Input format on stdin: "<bytes>\t<mtime_or_empty>\t<path>\n" lines.
# Usage: __gash_fs_render_size_list_json <data> <scan_time> <err_count> <kind>
__gash_fs_render_size_list_json() {
    local data="$1"
    local scan_time="$2"
    local err_count="$3"
    local kind="$4"

    local items total count
    # Build JSON array via awk (handles escapes).
    items=$(
        printf '%s\n' "$data" | awk -F'\t' '
            function esc(s,   r) {
                r = s
                gsub(/\\/, "\\\\", r)
                gsub(/"/,  "\\\"", r)
                gsub(/\n/, "\\n",  r)
                gsub(/\r/, "\\r",  r)
                gsub(/\t/, "\\t",  r)
                return r
            }
            function hbytes(b,   units, i, sz, rem) {
                units[0]="B"; units[1]="KiB"; units[2]="MiB"; units[3]="GiB"
                units[4]="TiB"; units[5]="PiB"
                i=0; sz=b; rem=0
                while (sz >= 1024 && i < 5) {
                    rem = int((sz * 10 / 1024)) % 10
                    sz = int(sz / 1024)
                    i++
                }
                if (i == 0) return sprintf("%d %s", sz, units[i])
                return sprintf("%d.%d %s", sz, rem, units[i])
            }
            NF < 3 { next }
            {
                bytes = $1 + 0
                mtime = $2
                path = $3
                for (i = 4; i <= NF; i++) path = path "\t" $i
                if (started) printf ","
                started = 1
                printf "{\"size\":%d,\"size_human\":\"%s\"", bytes, hbytes(bytes)
                if (length(mtime) > 0) printf ",\"mtime\":\"%s\"", esc(mtime)
                printf ",\"path\":\"%s\"}", esc(path)
                n++
                total += bytes
            }
            END {
                if (!started) printf ""
                printf "\n%d\n%d\n", n+0, total+0
            }
        '
    )

    # Split: the awk output ends with "\n<count>\n<total>\n".
    local tail2 arr_body
    tail2="$(printf '%s' "$items" | tail -n 2)"
    count="$(printf '%s' "$tail2" | sed -n '1p')"
    total="$(printf '%s' "$tail2" | sed -n '2p')"
    arr_body="$(printf '%s' "$items" | head -n -2 2>/dev/null || printf '%s' "$items" | awk 'NR > 0 { a[NR] = $0 } END { for (i = 1; i <= NR - 2; i++) printf "%s%s", a[i], (i < NR - 2 ? "\n" : "") }')"
    [[ -z "$arr_body" ]] && arr_body=""
    [[ -z "$count" ]] && count=0
    [[ -z "$total" ]] && total=0

    printf '{"data":[%s],"count":%d,"total_size":%d,"total_size_human":"%s","kind":"%s","scan_time":"%s","errors":%d}\n' \
        "$arr_body" "$count" "$total" "$(__gash_fs_human_size "$total")" "$kind" "$scan_time" "$err_count"
}

# Render a size-list in human-readable text (with optional colors).
# Input format on stdin: "<bytes>\t<mtime_or_empty>\t<path>\n" lines.
# Usage: __gash_fs_render_size_list_text <data> <human_mode> <no_color> <kind>
__gash_fs_render_size_list_text() {
    local data="$1"
    local human_mode="$2"
    local no_color="$3"
    # shellcheck disable=SC2034
    local kind="$4"

    local color_on=0
    if __gash_fs_color_ok "$no_color" 0; then color_on=1; fi

    printf '%s\n' "$data" | awk -F'\t' -v human="$human_mode" -v color="$color_on" '
        function hbytes(b,   units, i, sz, rem) {
            units[0]="B"; units[1]="KiB"; units[2]="MiB"; units[3]="GiB"
            units[4]="TiB"; units[5]="PiB"
            i=0; sz=b; rem=0
            while (sz >= 1024 && i < 5) {
                rem = int((sz * 10 / 1024)) % 10
                sz = int(sz / 1024)
                i++
            }
            if (i == 0) return sprintf("%d %s", sz, units[i])
            return sprintf("%d.%d %s", sz, rem, units[i])
        }
        function pad(s, n,   l) {
            l = length(s)
            if (l >= n) return s
            return s sprintf("%*s", n - l, "")
        }
        NF < 3 { next }
        {
            bytes = $1 + 0
            mtime = $2
            path = $3
            for (i = 4; i <= NF; i++) path = path "\t" $i

            if (human) sz_str = hbytes(bytes)
            else sz_str = bytes " B"

            # Variable-width size column to avoid truncating long paths
            sz_col = pad(sz_str, 12)

            if (color) {
                # Yellow size, cyan path, muted mtime
                printf "\033[1;33m%s\033[0m  \033[0;36m%s\033[0m", sz_col, path
                if (length(mtime) > 0) printf "  \033[38;5;245m%s\033[0m", mtime
                printf "\n"
            } else {
                printf "%s  %s", sz_col, path
                if (length(mtime) > 0) printf "  %s", mtime
                printf "\n"
            }
        }
    '
}

# -----------------------------------------------------------------------------
# Archive Extraction
# -----------------------------------------------------------------------------

# Extract various archive types (case-insensitive) with optional output directory.
# Usage: archive_extract ARCHIVE_FILE [OUTPUT_DIR]
# Alias: axe
archive_extract() {
    needs_help "archive_extract" "archive_extract ARCHIVE_FILE [OUTPUT_DIR]" \
        "Extracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR. Alias: axe" \
        "${1-}" && return

    local archive_file="${1-}"
    local output_dir="${2:-.}"

    __gash_require_arg "$archive_file" "archive file" "archive_extract <archive_file> [output_dir]" || return 1
    __gash_require_file "$archive_file" || return 1

    if [ ! -d "$output_dir" ]; then
        mkdir -p "$output_dir" || {
            __gash_error "Failed to create output directory '$output_dir'."
            return 1
        }
    fi

    local rc=0
    shopt -s nocasematch
    case "$archive_file" in
        *.tar.bz2)   tar xvjf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tar.gz)    tar xvzf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.bz2)       bunzip2 -c "$archive_file" > "$output_dir/$(basename "$archive_file" .bz2)" || rc=1 ;;
        *.rar)       unrar x "$archive_file" "$output_dir" || rc=1 ;;
        *.gz)        gunzip -c "$archive_file" > "$output_dir/$(basename "$archive_file" .gz)" || rc=1 ;;
        *.tar)       tar xvf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tbz2)      tar xvjf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.tgz)       tar xvzf "$archive_file" -C "$output_dir" --no-same-owner --no-same-permissions || rc=1 ;;
        *.zip)       unzip "$archive_file" -d "$output_dir" || rc=1 ;;
        *.z)         uncompress "$archive_file" -c > "$output_dir/$(basename "$archive_file" .z)" || rc=1 ;;
        *.7z)        7z x "$archive_file" -o"$output_dir" || rc=1 ;;
        *)           __gash_error "Cannot extract '$archive_file', unsupported file type."; rc=1 ;;
    esac
    shopt -u nocasematch

    return $rc
}

# -----------------------------------------------------------------------------
# File Backup
# -----------------------------------------------------------------------------

# Create a backup of a file with a timestamp suffix.
# Usage: file_backup FILE
# Alias: fbk
file_backup() {
    local file="${1-}"

    __gash_require_arg "$file" "file" "file_backup <file>" || return 1
    __gash_require_file "$file" || return 1

    local backup="${file}_backup_$(date +%Y%m%d%H%M%S)"
    cp -v "$file" "$backup"

    if [ ! -f "$backup" ]; then
        __gash_error "Failed to create backup file."
        return 1
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias flf='files_largest'
alias dld='dirs_largest'
alias dfl='dirs_find_large'
alias dle='dirs_list_empty'
alias tls='tree_stats'
alias axe='archive_extract'
alias fbk='file_backup'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

__gash_register_help "files_largest" \
    --aliases "flf" \
    --module "files" \
    --short "List the largest files under a path with size/exclude/json filters" \
    --see-also "dirs_largest dirs_find_large tree_stats" \
    <<'HELP'
USAGE
  files_largest [PATH] [--limit N] [--min-size SIZE] [--exclude GLOB]
                [--depth N] [--no-ignore] [--xdev] [--follow-symlinks]
                [--allow-root] [--json] [--null] [--human] [--no-color]

EXAMPLES
  # Find the biggest files in the current directory
  files_largest

  # Top 20 files larger than 50MB, stay on same filesystem
  files_largest --limit 20 --min-size 50M --xdev /var/log

  # JSON output for scripting (envelope: data/count/total_size/scan_time)
  files_largest /home --json | jq '.data[] | select(.size > 1073741824)'

  # NUL-delimited paths for safe xargs pipelines
  files_largest --min-size 100M --null /tmp | xargs -0 rm -i

  # Include vendored dirs (disable default noise pruning)
  files_largest --no-ignore ~/projects

OPTIONS
  --limit N              Cap results (default 100)
  --min-size SIZE        Ignore files smaller than SIZE (e.g. 10M, 1G, 500K)
  --exclude GLOB         Prune matching paths (repeatable, substring match)
  --depth N              Limit scan depth (default: unbounded)
  --no-ignore            Disable default pruning (node_modules/.git/vendor/__pycache__/.cache)
  --xdev                 Stay on a single filesystem
  --follow-symlinks, -L  Follow symbolic links
  --allow-root           Required to scan '/' (safety guard)
  --json                 Emit JSON envelope: {data, count, total_size, scan_time, errors}
  --null, -0             NUL-delimited paths for xargs -0
  --human                Force human-readable sizes (auto-on on TTY)
  --no-color             Disable ANSI colors (also honors NO_COLOR)

NOTES
  Paths containing literal tab or newline characters are emitted verbatim
  but may confuse downstream awk/cut consumers. Use --null for safety.
HELP

__gash_register_help "dirs_largest" \
    --aliases "dld" \
    --module "files" \
    --short "List the largest directories with depth and filter control" \
    --see-also "files_largest dirs_find_large tree_stats disk_usage" \
    <<'HELP'
USAGE
  dirs_largest [PATH] [--limit N] [--depth N] [--min-size SIZE]
               [--exclude GLOB] [--no-ignore] [--xdev]
               [--allow-root] [--json] [--null] [--human] [--no-color]

EXAMPLES
  # Biggest top-level dirs in the current path (default depth 1)
  dirs_largest

  # Drill two levels deep, smallest 100MB
  dirs_largest --depth 2 --min-size 100M /home

  # Find which user's home is eating disk space (JSON envelope)
  dirs_largest /home --json | jq '.data[:5]'

  # NUL-delimited paths, suitable for xargs -0 du -sh
  dirs_largest --null --depth 3 ~/projects | xargs -0 du -sh

OPTIONS
  --limit N              Cap results (default 100)
  --depth N              Depth limit (default 1, top-level only). Use higher values
                         to reveal nested offenders (node_modules, vendor/, target/).
  --min-size SIZE        Ignore directories smaller than SIZE
  --exclude GLOB         Prune matching paths (repeatable, substring match)
  --no-ignore            Disable default pruning
  --xdev                 Stay on a single filesystem
  --allow-root           Required to scan '/'
  --json                 Emit JSON envelope
  --null, -0             NUL-delimited paths
  --human / --no-human   Force human-readable / raw-bytes sizes
  --no-color             Disable ANSI colors

NOTES
  Sizes are cumulative (du semantics). Deeper scans will show parent
  directories alongside their subdirs; filter with --min-size and --depth
  to narrow the view.
HELP

__gash_register_help "dirs_find_large" \
    --aliases "dfl" \
    --module "files" \
    --short "Find directories exceeding a size threshold (single-walk aggregation)" \
    --see-also "dirs_largest files_largest tree_stats disk_usage" \
    <<'HELP'
USAGE
  dirs_find_large [DIRECTORY] [--size SIZE] [--depth N] [--limit N]
                  [--exclude GLOB] [--no-ignore] [--xdev]
                  [--allow-root] [--with-mtime] [--json] [--null]
                  [--human] [--no-color]

EXAMPLES
  # Find directories larger than 20MB (default) in current path
  dirs_find_large

  # Find directories larger than 1GB with newest-file timestamp
  dirs_find_large --size 1G --with-mtime /var

  # JSON envelope for scripting
  dirs_find_large --size 500M --json /home | jq '.data'

  # Constrain depth to avoid long walks
  dirs_find_large --size 100M --depth 6 /opt

OPTIONS
  --size SIZE            Size threshold (default 20M; formats: 20M, 1G, 500K, bytes)
  --depth N              Depth limit (default unbounded; use to cap very deep trees)
  --limit N              Cap results (default 1000)
  --exclude GLOB         Prune matching paths (repeatable)
  --no-ignore            Disable default pruning
  --xdev                 Stay on a single filesystem
  --allow-root           Required to scan '/'
  --with-mtime           Append newest-file mtime per directory (extra cost per match)
  --json                 Emit JSON envelope
  --null, -0             NUL-delimited paths
  --human / --no-human   Size formatting toggle
  --no-color             Disable ANSI colors

PERFORMANCE
  Uses a single `du -k` pass plus awk filtering: O(N) in filesystem entries,
  versus the legacy per-directory re-walk which was O(N^2). Expect 10x-100x
  speedup on large trees.

NOTES
  numfmt is optional: a pure-bash IEC-suffix parser provides fallback.
HELP

__gash_register_help "dirs_list_empty" \
    --aliases "dle" \
    --module "files" \
    --short "List empty directories with depth, prune and count options" \
    --see-also "files_largest dirs_largest tree_stats" \
    <<'HELP'
USAGE
  dirs_list_empty [PATH] [--min-depth N] [--depth N] [--exclude GLOB]
                  [--ignore-dotfiles] [--no-ignore] [--xdev]
                  [--allow-root] [--null] [--count] [--json]

EXAMPLES
  # Find empty dirs in current path
  dirs_list_empty

  # Count only
  dirs_list_empty /var/www --count

  # Remove safely with xargs -0 (NUL-delimited)
  dirs_list_empty --null /opt/app/uploads | xargs -0 rmdir

  # Skip hidden (dot-prefixed) dirs
  dirs_list_empty --ignore-dotfiles ~/projects

OPTIONS
  --min-depth N      Minimum depth (find -mindepth)
  --depth N          Maximum depth (find -maxdepth)
  --exclude GLOB     Prune matching paths
  --ignore-dotfiles  Skip directories whose name starts with '.'
  --no-ignore        Disable default pruning
  --xdev             Stay on a single filesystem
  --allow-root       Required to scan '/'
  --null, -0         NUL-delimited output for xargs -0
  --count, -c        Print count only (to stdout)
  --json             Emit JSON envelope: {data, count, path, errors}

NOTES
  "Empty" means no regular entries (not even hidden files). A directory
  containing only a prune-excluded subdirectory is NOT empty.
HELP

__gash_register_help "tree_stats" \
    --aliases "tls" \
    --module "files" \
    --short "Compact filesystem stats: totals, top extensions, depth, empty dirs" \
    --see-also "files_largest dirs_largest dirs_find_large" \
    <<'HELP'
USAGE
  tree_stats [PATH] [--depth N] [--top N] [--exclude GLOB]
             [--no-ignore] [--xdev] [--follow-symlinks]
             [--allow-root] [--json] [--no-color]

EXAMPLES
  # Audit current directory
  tree_stats

  # Drill into a projects tree, top 10 extensions
  tree_stats --top 10 ~/projects

  # JSON envelope (useful for dashboards / CI)
  tree_stats /var/log --json | jq '.data.top_by_size'

  # Stay on one filesystem, cap scan depth
  tree_stats --xdev --depth 8 /

OPTIONS
  --depth N              Scan depth limit (default unbounded)
  --top N                Number of extensions in top lists (default 5)
  --exclude GLOB         Prune matching paths (repeatable)
  --no-ignore            Disable default pruning
  --xdev                 Stay on a single filesystem
  --follow-symlinks, -L  Follow symbolic links
  --allow-root           Required to scan '/'
  --json                 Emit JSON envelope
  --no-color             Disable ANSI colors

NOTES
  Single-walk aggregation via `find -printf` + awk. Extensions are derived
  from the basename (lowercased, after the last dot). Files with no dot
  or hidden-only dot (.env) are grouped as "(no ext)".
HELP

__gash_register_help "archive_extract" \
    --aliases "axe" \
    --module "files" \
    --short "Extract archives (tar, zip, gz, bz2, rar, 7z)" \
    --see-also "file_backup" \
    <<'HELP'
USAGE
  archive_extract ARCHIVE_FILE [OUTPUT_DIR]

EXAMPLES
  # Extract a tar.gz in the current directory
  axe backup.tar.gz

  # Extract a zip to a specific folder
  axe release.zip /opt/app

  # Extract a 7z archive
  axe data.7z /tmp/extracted

SUPPORTED FORMATS
  .tar.gz, .tar.bz2, .tar, .gz, .bz2,
  .tgz, .tbz2, .zip, .rar, .7z, .z
  (case-insensitive)
HELP

__gash_register_help "file_backup" \
    --aliases "fbk" \
    --module "files" \
    --short "Create a timestamped backup of a file" \
    --see-also "archive_extract" \
    <<'HELP'
USAGE
  file_backup FILE

EXAMPLES
  # Backup a config before editing
  fbk /etc/nginx/nginx.conf
  # Creates: /etc/nginx/nginx.conf_backup_20240115143022

  # Backup-then-edit workflow
  fbk /etc/apache2/apache2.conf && vim /etc/apache2/apache2.conf

  # Backup a script before refactoring
  fbk my-deploy-script.sh
HELP

fi  # end help registration guard
