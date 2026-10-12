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
__gash_json_escape() {
    local str="${1-}"
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
    printf '%s' "$str"
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
# NUL-delimited transport: values may contain newlines, tabs, quotes.
# Usage: json=$(__gash_json_arr "${files[@]}")
__gash_json_arr() {
    if (($# == 0)); then
        printf '[]'
        return 0
    fi
    printf '%s\0' "$@" | jq -cRs 'split("\u0000")[:-1]'
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

fi # end help registration guard
