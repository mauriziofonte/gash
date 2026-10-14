#!/usr/bin/env bash

# Gash Core: Utility Functions
# General-purpose helpers used across Gash modules.
#
# All functions are errexit-safe and can be used with `set -e`.

# awk used where the implementation matters (hgrep: strftime, IGNORECASE):
# GASH_AWK when set, else gawk when installed, else the system awk.
if [[ -n "${GASH_AWK-}" ]]; then
    __GASH_AWK="$GASH_AWK"
elif type -P gawk >/dev/null 2>&1; then
    __GASH_AWK="gawk"
else
    __GASH_AWK="awk"
fi

# -----------------------------------------------------------------------------
# Help System
# -----------------------------------------------------------------------------

# Print help message for a function if requested.
# Usage: needs_help "program" "usage" "description" "$1" && return
# Returns: 0 if help was requested (and printed), 1 otherwise
#
# Colors are gated via __gash_use_color when available (load+runtime), so
# the help blob is plain text under GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR,
# or when stdout is not a TTY.
needs_help() {
    local program="${1-}"
    local usage="${2-}"
    local help="${3-}"
    local user_input="${4-}"

    if [[ "$user_input" == "--help" || "$user_input" == "-h" ]]; then
        # The full registered help (examples, notes) when there is one.
        if declare -f __gash_help_display >/dev/null 2>&1 && __gash_help_display "$program"; then
            return 0
        fi
        if declare -f __gash_use_color >/dev/null 2>&1 && __gash_use_color; then
            printf '\e[38;5;214m%s\e[0m\n' "$program"
            printf '\e[1;97mUsage:\e[0m \e[1;96m%s\e[0m\n' "$usage"
            printf '%s\n' "$help"
        else
            printf '%s\n' "$program"
            printf 'Usage: %s\n' "$usage"
            printf '%s\n' "$help"
        fi
        return 0
    fi

    return 1
}

# -----------------------------------------------------------------------------
# User Prompts
# -----------------------------------------------------------------------------

# Prompt user for yes/no confirmation.
# Usage: needs_confirm_prompt "Are you sure?" && do_thing
# Returns: 0 if user confirms (y/Y), 1 otherwise
#
# Headless contract: when no controlling terminal is available (or
# GASH_HEADLESS=1), the prompt always refuses. REPLY is local so a stale
# value inherited from the environment can never ghost-confirm a
# destructive operation. The prompt goes to stderr so callers captured via
# $(...) never see the prompt text in the value.
needs_confirm_prompt() {
    local prompt="${1-}"
    local REPLY=""

    [[ "${GASH_HEADLESS-}" == "1" ]] && return 1

    if declare -f __gash_use_color >/dev/null 2>&1 && __gash_use_color; then
        printf '%b' "$prompt ${__GASH_BOLD_WHITE}(y/N):${__GASH_COLOR_OFF} " >&2
    else
        printf '%s (y/N): ' "$prompt" >&2
    fi

    # A failed read (no tty, EOF) must refuse, never reuse a previous REPLY.
    read -r REPLY </dev/tty 2>/dev/null || return 1

    [[ "$REPLY" =~ ^[Yy]$ ]]
}

# -----------------------------------------------------------------------------
# Terminal Utilities
# -----------------------------------------------------------------------------

# Path of the program NAME runs as, like `type -P NAME` (first executable
# file on PATH), into REPLY, without a subshell. Returns 1 when none.
# Usage: __gash_path_of NAME
__gash_path_of() {
    # No `local -` (set -f): Bash 4.4 and 5.0 crash at exit after it runs
    # in an interactive shell; read -a splits without globbing.
    local dir
    local -a dirs=()
    REPLY=""
    [[ -n "${1-}" && "$1" != */* ]] || return 1
    # Bash's own search answers "none" faster than the loop below.
    type -P "$1" >/dev/null 2>&1 || return 1
    # An empty entry, trailing ones included, is the current directory.
    IFS=: read -r -a dirs <<<"${PATH-}"
    [[ "${PATH-}" == *: ]] && dirs+=("")
    for dir in ${dirs[@]+"${dirs[@]}"}; do
        [[ -z "$dir" ]] && dir="."
        if [[ -f "$dir/$1" && -x "$dir/$1" ]]; then
            REPLY="$dir/$1"
            return 0
        fi
    done
    return 1
}

# Get terminal width (best-effort, errexit-safe).
# Usage: width=$(__gash_tty_width)
# Returns: Terminal width in columns (default 80)
__gash_tty_width() {
    local w=""

    # Try $COLUMNS first
    if [[ "${COLUMNS-}" =~ ^[0-9]+$ ]] && ((COLUMNS > 0)); then
        printf '%s' "$COLUMNS"
        return 0
    fi

    # No terminal to ask: skip the tput and stty forks.
    if [[ ! -t 0 && ! -t 1 && ! -t 2 ]]; then
        printf '80'
        return 0
    fi

    # Try tput
    if command -v tput >/dev/null 2>&1; then
        w="$(tput cols 2>/dev/null || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && ((w > 0)); then
            printf '%s' "$w"
            return 0
        fi
    fi

    # Try stty
    if command -v stty >/dev/null 2>&1; then
        w="$(stty size 2>/dev/null | awk '{print $2}' || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && ((w > 0)); then
            printf '%s' "$w"
            return 0
        fi
    fi

    # Fallback
    printf '80'
}

# -----------------------------------------------------------------------------
# String Utilities
# -----------------------------------------------------------------------------

# Trim leading and trailing whitespace (spaces and tabs).
# Usage: trimmed=$(__gash_trim_ws "  text  ")
__gash_trim_ws() {
    local s="${1-}"
    # trim leading spaces/tabs
    s="${s#"${s%%[!$' \t']*}"}"
    # trim trailing spaces/tabs
    s="${s%"${s##*[!$' \t']}"}"
    printf '%s' "$s"
}

# Escape string for safe embedding in JSON values (RFC 8259): backslash,
# double quote and every control character U+0000..U+001F. Pure bash; the
# per-character loop only runs for the rare strings that carry other
# control characters than \n \r \t.
# Usage: escaped=$(__gash_json_escape "string with \"quotes\"")
#        __gash_json_escape_to VAR "string"   (no subshell; VAR must not be
#        named str, out, ch, code or i)
__gash_json_escape_to() {
    local str="${2-}"
    str="${str//\\/\\\\}"
    str="${str//\"/\\\"}"
    str="${str//$'\n'/\\n}"
    str="${str//$'\r'/\\r}"
    str="${str//$'\t'/\\t}"
    str="${str//$'\b'/\\b}"
    str="${str//$'\f'/\\f}"
    if [[ "$str" == *[$'\001'-$'\037']* ]]; then
        local LC_ALL=C out="" ch code i
        for ((i = 0; i < ${#str}; i++)); do
            ch="${str:i:1}"
            if [[ "$ch" == [$'\001'-$'\037'] ]]; then
                printf -v code '%d' "'$ch"
                printf -v ch '\\u%04x' "$((code & 255))"
            fi
            out+="$ch"
        done
        str="$out"
    fi
    printf -v "$1" '%s' "$str"
}

__gash_json_escape() {
    local __gash_esc_out
    __gash_json_escape_to __gash_esc_out "${1-}"
    printf '%s' "$__gash_esc_out"
}

# Print the machine error envelope shared by llm_* and the filesystem
# scanners to stderr (see AGENTS.md, "Error contract"):
#   {"error":TYPE,"details":..,"action":ACTION,"recoverable":BOOL,"hint":..}
# `recoverable` follows from the action: only FATAL is unrecoverable.
# details and hint are omitted when empty. Always returns 1.
# Usage: __gash_json_error TYPE [DETAILS] [ACTION] [HINT]
__gash_json_error() {
    local type="${1:-unknown_error}" details="${2-}" action="${3:-STOP}" hint="${4-}"
    case "$action" in
        RETRY | CONTINUE | STOP | FATAL) ;;
        *) action="STOP" ;;
    esac
    local recoverable=true
    [[ "$action" == "FATAL" ]] && recoverable=false

    local json
    json="{\"error\":\"$(__gash_json_escape "$type")\""
    [[ -n "$details" ]] && json+=",\"details\":\"$(__gash_json_escape "$details")\""
    json+=",\"action\":\"$action\",\"recoverable\":$recoverable"
    [[ -n "$hint" ]] && json+=",\"hint\":\"$(__gash_json_escape "$hint")\""
    printf '%s}\n' "$json" >&2
    return 1
}

# Render a single string as a quoted, fully escaped JSON string (via jq).
# Usage: json=$(__gash_json_str "raw value")
__gash_json_str() {
    jq -cn --arg s "${1-}" '$s'
}

# Render an argument list as a JSON array of strings ([] when empty).
# Values may contain newlines, tabs, quotes and control characters: each
# is escaped in bash, then jq normalizes the array (invalid UTF-8 becomes
# U+FFFD). No NUL transport: jq 1.6 cuts raw input at the first NUL.
# Usage: json=$(__gash_json_arr "${files[@]}")
__gash_json_arr() {
    if (($# == 0)); then
        printf '[]'
        return 0
    fi
    local out="" v e sep=""
    for v in "$@"; do
        __gash_json_escape_to e "$v"
        out+="${sep}\"${e}\""
        sep=","
    done
    jq -c . <<<"[$out]"
}

# -----------------------------------------------------------------------------
# Shell Utilities
# -----------------------------------------------------------------------------

# Run curl with a sensitive header passed via --config on a file descriptor,
# never in argv (argv is world-readable through /proc/<pid>/cmdline).
# Usage: __gash_curl_auth "x-api-key: $token" [curl args...]
__gash_curl_auth() {
    local hdr="${1-}"
    shift
    curl --config <(printf 'header = "%s"\n' "$hdr") "$@"
}

# Execute a command with shell history temporarily disabled.
# Saves and restores the history state around the wrapped command.
# Usage: __gash_no_history <function_name> [args...]
__gash_no_history() {
    local hist_was_enabled=0

    # Check if history is currently enabled
    if [[ -o history ]]; then
        hist_was_enabled=1
        set +o history
    fi

    # Execute the passed function/command
    "$@"
    local rc=$?

    # Restore history if it was enabled
    if [[ $hist_was_enabled -eq 1 ]]; then
        set -o history
    fi

    return $rc
}

# -----------------------------------------------------------------------------
# Path Utilities
# -----------------------------------------------------------------------------

# Expand tilde (~) in path to $HOME.
# Usage: expanded=$(__gash_expand_tilde_path "~/file")
__gash_expand_tilde_path() {
    local p="${1-}"
    if [[ "$p" == \~/* ]]; then
        printf '%s' "$HOME/${p:2}"
        return 0
    fi
    if [[ "$p" == "~" ]]; then
        printf '%s' "$HOME"
        return 0
    fi
    printf '%s' "$p"
}

# Put directories at the front of PATH (or the end with --append), once
# each and in the order given. Directories that do not exist are skipped (a
# toolchain installed later appears in the next shell); one already in PATH
# keeps its place.
# Usage: path_add [--append] DIR...
path_add() {
    needs_help "path_add" "path_add [--append] DIR..." \
        "Add directories to PATH once each, skipping missing ones." "${1-}" && return

    local append=0 dir i
    local -a dirs=()
    if [[ "${1-}" == "--append" ]]; then
        append=1
        shift
    fi
    # Prepending in reverse keeps the order given: path_add a b -> a:b:PATH.
    if ((append)); then
        dirs=("$@")
    else
        for ((i = $#; i >= 1; i--)); do dirs+=("${!i}"); done
    fi
    for dir in "${dirs[@]}"; do
        dir="$(__gash_expand_tilde_path "$dir")"
        dir="${dir%/}"
        [[ -n "$dir" && -d "$dir" ]] || continue
        [[ ":$PATH:" == *":$dir:"* ]] && continue
        if ((append)); then
            PATH="${PATH:+$PATH:}$dir"
        else
            PATH="$dir${PATH:+:$PATH}"
        fi
    done
    export PATH
    return 0
}

# -----------------------------------------------------------------------------
# Plans, steps and downloads (apt-style)
# -----------------------------------------------------------------------------
#
# Commands that change the system (node_upgrade, chrome_headless_update)
# print a plan, ask once and then report each step on one line, the way apt
# does. Everything here writes to stderr: stdout stays for data.

# Bytes to IEC units with one decimal: 0 B, 1.5 KiB, 142.3 MiB.
# Usage: __gash_human_size BYTES
__gash_human_size() {
    local size="${1-0}" rem=0 i=0
    local -a units=(B KiB MiB GiB TiB PiB EiB)
    [[ "$size" =~ ^[0-9]+$ ]] || size=0
    while ((size >= 1024 && i < ${#units[@]} - 1)); do
        rem=$(((size * 10 / 1024) % 10))
        size=$((size / 1024))
        i=$((i + 1))
    done
    if ((i == 0)); then
        printf '%d %s' "$size" "${units[0]}"
    else
        printf '%d.%d %s' "$size" "$rem" "${units[$i]}"
    fi
}

# Ask "Do you want to continue? [Y/n]" after a plan has been printed (Enter
# means yes). ASSUME_YES=1, from a -y flag, skips the question. Without a
# terminal, or in headless mode, it refuses instead of waiting for input.
# Usage: __gash_confirm_plan ASSUME_YES
__gash_confirm_plan() {
    local assume_yes="${1-0}" answer=""
    [[ "$assume_yes" == "1" ]] && return 0
    if [[ "${GASH_HEADLESS-}" == "1" ]] || ! { : </dev/tty; } 2>/dev/null; then
        __gash_error "No terminal to ask for confirmation: pass -y to go ahead."
        return 1
    fi
    printf 'Do you want to continue? [Y/n] ' >&2
    if ! read -r answer </dev/tty 2>/dev/null; then
        printf '\n' >&2
        return 1
    fi
    [[ -z "$answer" || "$answer" =~ ^[Yy]([Ee][Ss])?$ ]] && return 0
    __gash_info "Aborted."
    return 1
}

# "Message... done" around a command; its stdout passes through, and the
# line ends in "failed" with the command's status when it fails.
# Usage: __gash_task "Reading the remote index" command [args...]
__gash_task() {
    local msg="${1-}" rc=0
    shift
    printf '%s... ' "$msg" >&2
    "$@" || rc=$?
    if ((rc == 0)); then
        if __gash_color_env_ok 2>/dev/null && [[ -t 2 ]]; then
            printf '%sdone%s\n' "$__GASH_COLOR_SUCCESS" "$__GASH_COLOR_OFF" >&2
        else
            printf 'done\n' >&2
        fi
    else
        if __gash_color_env_ok 2>/dev/null && [[ -t 2 ]]; then
            printf '%sfailed%s\n' "$__GASH_COLOR_ERROR" "$__GASH_COLOR_OFF" >&2
        else
            printf 'failed\n' >&2
        fi
    fi
    return "$rc"
}

# Content-Length of URL after redirects, 0 when the server does not say.
# Usage: __gash_remote_size URL
__gash_remote_size() {
    curl -fsIL "${1-}" 2>/dev/null | tr -d '\r' |
        awk 'tolower($1) == "content-length:" { n = $2 } END { print n + 0 }'
}

# Download URL to DEST with a live progress line on a terminal (percent,
# size, speed, time left) and a summary at the end. Ctrl-C stops curl too
# and removes the partial file.
# Usage: __gash_download URL DEST
__gash_download() {
    local url="${1-}" dest="${2-}"
    __gash_require_command curl "Downloads need curl." || return 1
    [[ -n "$url" && -n "$dest" ]] || {
        __gash_error "Usage: __gash_download URL DEST"
        return 1
    }

    local total
    total="$(__gash_remote_size "$url")"
    if ((total > 0)); then
        printf 'Get: %s [%s]\n' "$url" "$(__gash_human_size "$total")" >&2
    else
        printf 'Get: %s\n' "$url" >&2
    fi

    local errf t0=$SECONDS pid rc=0 prev_int
    errf="$(mktemp "${TMPDIR:-/tmp}/gash-download.XXXXXX")" || return 1
    prev_int="$(trap -p INT)"
    # The group's stderr swallows an interactive shell's "[1] PID" notice.
    { curl -fsSL -o "$dest" "$url" 2>"$errf" & } 2>/dev/null
    pid=$!
    # shellcheck disable=SC2064 # expand now: the trap must name this pid
    trap "kill $pid 2>/dev/null; wait $pid 2>/dev/null; command rm -f '$errf' '$dest'; printf '\nAborted.\n' >&2; ${prev_int:-trap - INT}; return 130" INT

    if [[ -t 2 && "${GASH_HEADLESS-}" != "1" ]]; then
        local done_b=0 prev_b=0 speed=0 tick=0 pct filled bar eta i
        while kill -0 "$pid" 2>/dev/null; do
            done_b="$(wc -c <"$dest" 2>/dev/null || echo 0)"
            done_b="${done_b//[[:space:]]/}"
            # Speed over the last second (4 ticks of 250 ms).
            if ((++tick % 4 == 0)); then
                speed=$((done_b - prev_b))
                prev_b=$done_b
            fi
            if ((total > 0)); then
                pct=$((done_b * 100 / total))
                ((pct > 100)) && pct=100
                filled=$((pct * 30 / 100))
                bar=""
                for ((i = 0; i < 30; i++)); do
                    if ((i < filled)); then bar+="$__GASH_GLYPH_BAR_FULL"; else bar+="$__GASH_GLYPH_BAR_EMPTY"; fi
                done
                eta="--"
                ((speed > 0)) && eta="$(((total - done_b) / speed))s"
                printf '\r%3d%% %s %10s / %-10s %10s/s %6s\e[K' "$pct" "$bar" \
                    "$(__gash_human_size "$done_b")" "$(__gash_human_size "$total")" \
                    "$(__gash_human_size "$speed")" "$eta" >&2
            else
                printf '\r%10s %10s/s\e[K' "$(__gash_human_size "$done_b")" "$(__gash_human_size "$speed")" >&2
            fi
            sleep 0.25
        done
        printf '\r\e[K' >&2
    fi

    wait "$pid" 2>/dev/null || rc=$?
    if [[ -n "$prev_int" ]]; then eval "$prev_int"; else trap - INT; fi
    if ((rc != 0)); then
        __gash_error "Download failed ($url): $(tr -d '\n' <"$errf")"
        command rm -f "$errf" "$dest"
        return "$rc"
    fi
    command rm -f "$errf"

    local elapsed=$((SECONDS - t0)) size
    ((elapsed > 0)) || elapsed=1
    size="$(wc -c <"$dest")"
    size="${size//[[:space:]]/}"
    printf 'Fetched %s in %ss (%s/s)\n' "$(__gash_human_size "$size")" "$elapsed" \
        "$(__gash_human_size $((size / elapsed)))" >&2
}

# -----------------------------------------------------------------------------
# Miscellaneous
# -----------------------------------------------------------------------------

# Display all available terminal colors.
# Useful for testing color support.
all_colors() {
    needs_help "all_colors" "all_colors" "Displays all available terminal colors (256 colors)." "${1-}" && return

    local i
    for i in {0..255}; do
        printf "\x1b[38;5;%smcolour%s\x1b[0m\n" "$i" "$i"
    done
}

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "all_colors" \
        --module "utils" \
        --short "Display all 256 terminal colors" \
        <<'HELP'
USAGE
  all_colors

EXAMPLES
  # Test your terminal's color support
  all_colors

  # Pipe to less for scrolling
  all_colors | less -R
HELP

    __gash_register_help "path_add" \
        --module "utils" \
        --short "Add directories to PATH once each" \
        <<'HELP'
USAGE
  path_add [--append] DIR...

EXAMPLES
  # Toolchains first, in this order, only those that exist
  path_add ~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin

  # At the end of PATH instead
  path_add --append /opt/tools/bin

NOTES
  Directories that do not exist are skipped and one already in PATH stays
  where it is, so running it again (or re-sourcing ~/.gashrc) never makes
  PATH grow. GASH_PATH_ADD in ~/.gash_env does this for every new shell.
HELP

fi # end help registration guard
