#!/usr/bin/env bash

# Gash Core: Output Functions
# Centralized output formatting for consistent messaging across Gash.
#
# All functions are errexit-safe and can be used with `set -e`.
#
# COLOR POLICY (v1.5+):
#   Gash follows a hybrid color gating model:
#
#   1. Load-time: if any of GASH_HEADLESS=1, NO_COLOR, GASH_NO_COLOR is set,
#      all color variables are zeroed at source time. This disables ANSI
#      across every call site (core emitters, inline printfs, help system).
#
#   2. Runtime: core emitters (__gash_info/error/warning/success/...) re-check
#      on every call whether colors should be emitted. Colors are suppressed
#      when stdout is not a TTY (pipe/redirect) even if the env allows them.
#
#   3. Explicit flag: functions that accept --no-color (files_largest,
#      dirs_largest, sysinfo, gash_doctor, docker_compose_check, etc.) can
#      force-disable colors for a single invocation via __gash_want_colors.
#
#   Standards respected: https://no-color.org (NO_COLOR env var).
#   Gash-specific: GASH_NO_COLOR, GASH_HEADLESS.
#
# STREAM POLICY (v1.5.1+):
#   Structured data goes to stdout. ALL diagnostics (info/success/warning/
#   error/step/ssh/debug/prompts) go to stderr. This keeps values captured
#   via $(...) clean: a permission warning emitted during a config load must
#   never end up prefixed to an API token or a DB URL.

# -----------------------------------------------------------------------------
# Color Definitions
# -----------------------------------------------------------------------------

# Reset
__GASH_COLOR_OFF='\033[0m'

# Regular Colors
__GASH_RED='\033[0;31m'
__GASH_GREEN='\033[0;32m'
__GASH_YELLOW='\033[0;33m'
__GASH_BLUE='\033[0;34m'
__GASH_CYAN='\033[0;36m'
__GASH_WHITE='\033[0;37m'

# Bold Colors
__GASH_BOLD_RED='\033[1;31m'
__GASH_BOLD_GREEN='\033[1;32m'
__GASH_BOLD_YELLOW='\033[1;33m'
__GASH_BOLD_BLUE='\033[1;34m'
__GASH_BOLD_CYAN='\033[1;36m'
__GASH_BOLD_WHITE='\033[1;37m'

# -----------------------------------------------------------------------------
# Extended Color Palette (256-color support)
# -----------------------------------------------------------------------------

# Semantic colors for consistent theming
__GASH_COLOR_PRIMARY='\033[38;5;39m'      # Bright blue
__GASH_COLOR_SECONDARY='\033[38;5;243m'   # Gray
__GASH_COLOR_ACCENT='\033[38;5;214m'      # Orange
__GASH_COLOR_MUTED='\033[38;5;245m'       # Dim gray

# Status colors (256-color variants)
__GASH_COLOR_SUCCESS='\033[38;5;82m'      # Bright green
__GASH_COLOR_WARNING='\033[38;5;220m'     # Yellow/gold
__GASH_COLOR_ERROR='\033[38;5;196m'       # Bright red
__GASH_COLOR_INFO='\033[38;5;75m'         # Sky blue

# Unicode symbols with ASCII fallback
__GASH_SYMBOL_OK='OK'
__GASH_SYMBOL_ERR='ERR'
__GASH_SYMBOL_WARN='WARN'
__GASH_SYMBOL_INFO='INFO'
__GASH_SYMBOL_ARROW='->'
__GASH_SYMBOL_BULLET='-'

# Detect unicode support and upgrade symbols
if [[ "${LANG-}" == *UTF-8* ]] || [[ "${LC_ALL-}" == *UTF-8* ]]; then
    __GASH_SYMBOL_OK='[OK]'
    __GASH_SYMBOL_ERR='[X]'
    __GASH_SYMBOL_WARN='[!]'
    __GASH_SYMBOL_INFO='[i]'
    __GASH_SYMBOL_ARROW='->'
    __GASH_SYMBOL_BULLET='*'
fi

# -----------------------------------------------------------------------------
# Color Gating (Load-time + Runtime)
# -----------------------------------------------------------------------------

# Environment-level check: returns 0 if colors are permitted by env, 1 if any
# disable signal is set (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR).
# This is evaluated both at load time and at runtime.
__gash_color_env_ok() {
    [[ "${GASH_HEADLESS-}" == "1" ]] && return 1
    [[ -n "${NO_COLOR-}" ]] && return 1
    [[ -n "${GASH_NO_COLOR-}" ]] && return 1
    return 0
}

# Load-time gate: if any env disable is in effect, zero all color variables.
# This covers inline printf/echo call sites that don't consult a runtime
# helper. Modules using `${__GASH_BOLD_CYAN}...${__GASH_COLOR_OFF}` patterns
# will emit empty strings instead of ANSI sequences.
if ! __gash_color_env_ok; then
    __GASH_COLOR_OFF=''
    __GASH_RED=''      ; __GASH_GREEN=''       ; __GASH_YELLOW=''
    __GASH_BLUE=''     ; __GASH_CYAN=''        ; __GASH_WHITE=''
    __GASH_BOLD_RED='' ; __GASH_BOLD_GREEN=''  ; __GASH_BOLD_YELLOW=''
    __GASH_BOLD_BLUE=''; __GASH_BOLD_CYAN=''   ; __GASH_BOLD_WHITE=''
    __GASH_COLOR_PRIMARY=''   ; __GASH_COLOR_SECONDARY=''
    __GASH_COLOR_ACCENT=''    ; __GASH_COLOR_MUTED=''
    __GASH_COLOR_SUCCESS=''   ; __GASH_COLOR_WARNING=''
    __GASH_COLOR_ERROR=''     ; __GASH_COLOR_INFO=''
fi

# Runtime gate for core emitters and color-aware helpers.
# Returns 0 when colors should be emitted, 1 otherwise.
#
# Uniform policy: if stdout is not a TTY, we suppress colors everywhere
# (including messages going to stderr). This keeps output consistent when
# callers capture stdout, pipe through awk/jq/cat, or feed the output to
# another process such as an LLM.
__gash_use_color() {
    __gash_color_env_ok || return 1
    [[ -t 1 ]] || return 1
    return 0
}

# Helper for functions that accept a --no-color flag.
# Usage: __gash_want_colors "$no_color_flag_value"  (0 = auto, 1 = force off)
# Returns 0 when colors should be emitted, 1 otherwise.
__gash_want_colors() {
    local flag="${1-0}"
    [[ "$flag" -eq 1 ]] && return 1
    __gash_use_color
}

# Emit a block of `local VAR=''` declarations that, when eval'd inside a
# caller function, locally shadow all Gash color vars to empty. Used by
# functions that want to conditionally disable colors for a single invocation.
#
# Usage pattern (inside a function body):
#   eval "$(__gash_color_scope "$no_color")"
# When colors should be ON, returns nothing -> eval is a no-op.
# When colors should be OFF, returns the `local ...` block -> shadows the vars
# inside the caller's scope, propagating via bash dynamic scoping to any
# child function called thereafter.
__gash_color_scope() {
    local no_color="${1-0}"
    __gash_want_colors "$no_color" && return 0
    cat <<'EOS'
local __GASH_COLOR_OFF=''
local __GASH_RED='' __GASH_GREEN='' __GASH_YELLOW=''
local __GASH_BLUE='' __GASH_CYAN='' __GASH_WHITE=''
local __GASH_BOLD_RED='' __GASH_BOLD_GREEN='' __GASH_BOLD_YELLOW=''
local __GASH_BOLD_BLUE='' __GASH_BOLD_CYAN='' __GASH_BOLD_WHITE=''
local __GASH_COLOR_PRIMARY='' __GASH_COLOR_SECONDARY=''
local __GASH_COLOR_ACCENT='' __GASH_COLOR_MUTED=''
local __GASH_COLOR_SUCCESS='' __GASH_COLOR_WARNING=''
local __GASH_COLOR_ERROR='' __GASH_COLOR_INFO=''
EOS
}

# -----------------------------------------------------------------------------
# Core Output Functions
# -----------------------------------------------------------------------------

# Print an informational message.
# Usage: __gash_info "message"
__gash_info() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_CYAN}Info:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf 'Info: %s\n' "$message" >&2
    fi
}

# Print an error message to stderr.
# Usage: __gash_error "message"
__gash_error() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_BOLD_RED}Error:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf 'Error: %s\n' "$message" >&2
    fi
}

# Print a success message.
# Usage: __gash_success "message"
__gash_success() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_BOLD_GREEN}OK:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf 'OK: %s\n' "$message" >&2
    fi
}

# Print a warning message.
# Usage: __gash_warning "message"
__gash_warning() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_BOLD_YELLOW}Warning:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf 'Warning: %s\n' "$message" >&2
    fi
}

# Print an SSH-related message with optional color variant.
# Usage: __gash_ssh "message" [color]
# Colors: info (default), success, warning, error
__gash_ssh() {
    local message="${1-}"
    local color="${2:-info}"

    if __gash_use_color; then
        local msg_color
        case "$color" in
            success) msg_color="${__GASH_BOLD_GREEN}" ;;
            warning) msg_color="${__GASH_BOLD_YELLOW}" ;;
            error)   msg_color="${__GASH_BOLD_RED}" ;;
            *)       msg_color="${__GASH_BOLD_WHITE}" ;;
        esac
        echo -e "${__GASH_CYAN}SSH:${__GASH_COLOR_OFF} ${msg_color}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf 'SSH: %s\n' "$message" >&2
    fi
}

# Print a step indicator for multi-step operations.
# Usage: __gash_step current total "message"
__gash_step() {
    local current="${1-}"
    local total="${2-}"
    local message="${3-}"
    if __gash_use_color; then
        echo -e "${__GASH_CYAN}[${current}/${total}]${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
    else
        printf '[%s/%s] %s\n' "$current" "$total" "$message" >&2
    fi
}

# Print a debug message (only if GASH_DEBUG=1).
# Usage: __gash_debug "message"
__gash_debug() {
    if [[ "${GASH_DEBUG:-0}" == "1" ]]; then
        local message="${1-}"
        if __gash_use_color; then
            echo -e "${__GASH_BLUE}Debug:${__GASH_COLOR_OFF} ${message}" >&2
        else
            printf 'Debug: %s\n' "$message" >&2
        fi
    fi
}

# Print raw colored text without prefix.
# Usage: __gash_print color "message"
# Colors: red, green, yellow, blue, cyan, white, bold_* variants
__gash_print() {
    local color="${1-}"
    local message="${2-}"

    if ! __gash_use_color; then
        printf '%s\n' "$message"
        return 0
    fi

    local color_code
    case "$color" in
        red)    color_code="${__GASH_RED}" ;;
        green)  color_code="${__GASH_GREEN}" ;;
        yellow) color_code="${__GASH_YELLOW}" ;;
        blue)   color_code="${__GASH_BLUE}" ;;
        cyan)   color_code="${__GASH_CYAN}" ;;
        white)  color_code="${__GASH_WHITE}" ;;
        bold_red)    color_code="${__GASH_BOLD_RED}" ;;
        bold_green)  color_code="${__GASH_BOLD_GREEN}" ;;
        bold_yellow) color_code="${__GASH_BOLD_YELLOW}" ;;
        bold_blue)   color_code="${__GASH_BOLD_BLUE}" ;;
        bold_cyan)   color_code="${__GASH_BOLD_CYAN}" ;;
        bold_white)  color_code="${__GASH_BOLD_WHITE}" ;;
        *)      color_code="${__GASH_COLOR_OFF}" ;;
    esac

    echo -e "${color_code}${message}${__GASH_COLOR_OFF}"
}

# -----------------------------------------------------------------------------
# Enhanced Output Functions
# -----------------------------------------------------------------------------

# Enhanced success with symbol
# Usage: __gash_success_v2 "message"
__gash_success_v2() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_COLOR_SUCCESS}${__GASH_SYMBOL_OK}${__GASH_COLOR_OFF} ${message}" >&2
    else
        printf '%s %s\n' "$__GASH_SYMBOL_OK" "$message" >&2
    fi
}

# Enhanced warning with symbol
# Usage: __gash_warning_v2 "message"
__gash_warning_v2() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_COLOR_WARNING}${__GASH_SYMBOL_WARN}${__GASH_COLOR_OFF} ${message}" >&2
    else
        printf '%s %s\n' "$__GASH_SYMBOL_WARN" "$message" >&2
    fi
}

# Enhanced error with symbol
# Usage: __gash_error_v2 "message"
__gash_error_v2() {
    local message="${1-}"
    if __gash_use_color; then
        echo -e "${__GASH_COLOR_ERROR}${__GASH_SYMBOL_ERR}${__GASH_COLOR_OFF} ${message}" >&2
    else
        printf '%s %s\n' "$__GASH_SYMBOL_ERR" "$message" >&2
    fi
}

# -----------------------------------------------------------------------------
# Spinner for Long Operations
# -----------------------------------------------------------------------------

# Global spinner state
declare -g __GASH_SPINNER_PID=""
declare -g __GASH_SPINNER_MSG=""
declare -g __GASH_SPINNER_PREV_TRAP=""
declare -g __GASH_SPINNER_TRAP_SET=""

# Start a spinner for long-running operations
# Usage: __gash_spinner_start "Loading..."
__gash_spinner_start() {
    local message="${1-Working...}"
    __GASH_SPINNER_MSG="$message"

    # Don't start spinner if not in a terminal or in headless mode
    [[ ! -t 1 ]] && return 0
    [[ "${GASH_HEADLESS-}" == "1" ]] && return 0

    # Spinner characters (ASCII compatible)
    local frames='|/-\'

    # Honor the runtime color gate (NO_COLOR exported after source, etc.)
    local frame_on="" frame_off=""
    if __gash_use_color; then
        frame_on="${__GASH_COLOR_PRIMARY}"
        frame_off="${__GASH_COLOR_OFF}"
    fi

    # Start spinner in background, suppressing job notifications
    # Method: disable job control, start process, disown it, re-enable
    {
        set +m  # Disable job control to suppress "[1] PID" notification
        (
            local i=0
            local len=${#frames}
            while true; do
                printf "\r${frame_on}%s${frame_off} %s " "${frames:i:1}" "$message"
                i=$(( (i + 1) % len ))
                sleep 0.1
            done
        ) &
        __GASH_SPINNER_PID=$!
        disown "$__GASH_SPINNER_PID" 2>/dev/null  # Remove from job table
        set -m 2>/dev/null  # Re-enable job control (may fail in non-interactive)
    } 2>/dev/null

    # Ensure cleanup on script exit, preserving any pre-existing EXIT trap
    # so a caller's cleanup (temp files, locks) is restored on stop.
    __GASH_SPINNER_PREV_TRAP="$(trap -p EXIT)"
    __GASH_SPINNER_TRAP_SET="1"
    trap '__gash_spinner_stop' EXIT

    return 0
}

# Stop the spinner
# Usage: __gash_spinner_stop [success|error|warning]
__gash_spinner_stop() {
    local status="${1-success}"

    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""

        # Clear the spinner line
        printf "\r\033[K"

        # Print final status
        case "$status" in
            success) __gash_success_v2 "${__GASH_SPINNER_MSG}" ;;
            error)   __gash_error_v2 "${__GASH_SPINNER_MSG}" ;;
            warning) __gash_warning_v2 "${__GASH_SPINNER_MSG}" ;;
        esac
    fi

    # Only touch the EXIT trap if we set it; restore what was there before.
    if [[ -n "${__GASH_SPINNER_TRAP_SET-}" ]]; then
        if [[ -n "${__GASH_SPINNER_PREV_TRAP-}" ]]; then
            eval "$__GASH_SPINNER_PREV_TRAP"
        else
            trap - EXIT
        fi
        __GASH_SPINNER_PREV_TRAP=""
        __GASH_SPINNER_TRAP_SET=""
    fi

    return 0
}

# -----------------------------------------------------------------------------
# Progress Bar for Multi-Step Operations
# -----------------------------------------------------------------------------

# Display a progress bar
# Usage: __gash_progress current total "message"
__gash_progress() {
    local current="${1-0}"
    local total="${2-100}"
    local message="${3-}"

    # Don't show progress bar if not in a terminal or in headless mode
    [[ ! -t 1 ]] && return 0
    [[ "${GASH_HEADLESS-}" == "1" ]] && return 0

    # Avoid division by zero
    [[ "$total" -eq 0 ]] && total=1

    # Calculate percentage and bar width
    local percent=$(( current * 100 / total ))
    local width=30
    local filled=$(( percent * width / 100 ))
    local empty=$(( width - filled ))

    # Build the bar
    local bar=""
    local i
    for ((i=0; i<filled; i++)); do bar+="="; done
    [[ $filled -lt $width ]] && bar+=">"
    for ((i=0; i<empty-1 && i>=0; i++)); do bar+=" "; done

    # Honor the runtime color gate
    local bar_on="" bar_off=""
    if __gash_use_color; then
        bar_on="${__GASH_COLOR_PRIMARY}"
        bar_off="${__GASH_COLOR_OFF}"
    fi

    # Print progress
    printf "\r${bar_on}[%-${width}s]${bar_off} %3d%% %s" "$bar" "$percent" "$message"

    # Print newline when complete
    [[ $current -ge $total ]] && echo

    return 0
}

# -----------------------------------------------------------------------------
# Section Headers
# -----------------------------------------------------------------------------

# Print a section header
# Usage: __gash_section "Section Title"
__gash_section() {
    local title="${1-}"
    local width=60

    # Build separator line
    local line=""
    local i
    for ((i=0; i<width; i++)); do line+="-"; done

    echo
    if __gash_use_color; then
        echo -e "${__GASH_COLOR_PRIMARY}${line}${__GASH_COLOR_OFF}"
        echo -e "${__GASH_COLOR_PRIMARY}--${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${title}${__GASH_COLOR_OFF}"
    else
        printf '%s\n' "$line"
        printf -- '-- %s\n' "$title"
    fi
    echo
}
