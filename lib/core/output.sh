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

# Colors hold real escape bytes, so they work the same with printf '%s',
# printf '%b', echo -e and awk -v.

# Reset
__GASH_COLOR_OFF=$'\e[0m'

# Regular Colors
__GASH_RED=$'\e[0;31m'
__GASH_GREEN=$'\e[0;32m'
__GASH_YELLOW=$'\e[0;33m'
__GASH_BLUE=$'\e[0;34m'
__GASH_CYAN=$'\e[0;36m'
__GASH_WHITE=$'\e[0;37m'

# Bold Colors
__GASH_BOLD_RED=$'\e[1;31m'
__GASH_BOLD_GREEN=$'\e[1;32m'
__GASH_BOLD_YELLOW=$'\e[1;33m'
__GASH_BOLD_BLUE=$'\e[1;34m'
__GASH_BOLD_CYAN=$'\e[1;36m'
__GASH_BOLD_WHITE=$'\e[1;37m'

# Text attributes
__GASH_BOLD=$'\e[1m'
__GASH_DIM=$'\e[2m'
__GASH_UNDERLINE=$'\e[4m'

# -----------------------------------------------------------------------------
# Extended Color Palette (256-color support)
# -----------------------------------------------------------------------------

# Semantic colors for consistent theming
__GASH_COLOR_PRIMARY=$'\e[38;5;39m'    # Bright blue
__GASH_COLOR_SECONDARY=$'\e[38;5;243m' # Gray
__GASH_COLOR_ACCENT=$'\e[38;5;214m'    # Orange
__GASH_COLOR_MUTED=$'\e[38;5;245m'     # Dim gray

# Status colors (256-color variants)
__GASH_COLOR_SUCCESS=$'\e[38;5;82m'  # Bright green
__GASH_COLOR_WARNING=$'\e[38;5;220m' # Yellow/gold
__GASH_COLOR_ERROR=$'\e[38;5;196m'   # Bright red
__GASH_COLOR_INFO=$'\e[38;5;75m'     # Sky blue

# -----------------------------------------------------------------------------
# Glyphs (Unicode when the locale is UTF-8, ASCII otherwise)
# -----------------------------------------------------------------------------

__gash_locale_utf8() {
    local loc="${LC_ALL:-${LC_CTYPE:-${LANG-}}}"
    [[ "$loc" == *[Uu][Tt][Ff]-8* || "$loc" == *[Uu][Tt][Ff]8* ]]
}

if __gash_locale_utf8; then
    __GASH_GLYPH_OK='✓'
    __GASH_GLYPH_ERR='✗'
    __GASH_GLYPH_WARN='!'
    __GASH_GLYPH_INFO='•'
    __GASH_GLYPH_ARROW='→'
    __GASH_GLYPH_BULLET='•'
    __GASH_GLYPH_HLINE='─'
    __GASH_GLYPH_VLINE='│'
    __GASH_GLYPH_CORNERS='╭╮╰╯'
    __GASH_GLYPH_BAR_FULL='█'
    __GASH_GLYPH_BAR_EMPTY='░'
    __GASH_GLYPH_DOT='·'
    __GASH_GLYPH_ELLIPSIS='…'
    __GASH_GLYPH_POINTER='❯'
    __GASH_SPINNER_FRAMES='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
else
    __GASH_GLYPH_OK='+'
    __GASH_GLYPH_ERR='x'
    __GASH_GLYPH_WARN='!'
    __GASH_GLYPH_INFO='*'
    __GASH_GLYPH_ARROW='->'
    __GASH_GLYPH_BULLET='*'
    __GASH_GLYPH_HLINE='-'
    __GASH_GLYPH_VLINE='|'
    __GASH_GLYPH_CORNERS='++++'
    __GASH_GLYPH_BAR_FULL='#'
    __GASH_GLYPH_BAR_EMPTY='.'
    __GASH_GLYPH_DOT='-'
    __GASH_GLYPH_ELLIPSIS='~'
    __GASH_GLYPH_POINTER='>'
    __GASH_SPINNER_FRAMES='|/-\'
fi

# Older names, kept for modules and user scripts that use them.
__GASH_SYMBOL_OK="$__GASH_GLYPH_OK"
__GASH_SYMBOL_ERR="$__GASH_GLYPH_ERR"
__GASH_SYMBOL_WARN="$__GASH_GLYPH_WARN"
__GASH_SYMBOL_INFO="$__GASH_GLYPH_INFO"
__GASH_SYMBOL_ARROW="$__GASH_GLYPH_ARROW"
__GASH_SYMBOL_BULLET="$__GASH_GLYPH_BULLET"

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
    __GASH_RED=''
    __GASH_GREEN=''
    __GASH_YELLOW=''
    __GASH_BLUE=''
    __GASH_CYAN=''
    __GASH_WHITE=''
    __GASH_BOLD_RED=''
    __GASH_BOLD_GREEN=''
    __GASH_BOLD_YELLOW=''
    __GASH_BOLD_BLUE=''
    __GASH_BOLD_CYAN=''
    __GASH_BOLD_WHITE=''
    __GASH_BOLD=''
    __GASH_DIM=''
    __GASH_UNDERLINE=''
    __GASH_COLOR_PRIMARY=''
    __GASH_COLOR_SECONDARY=''
    __GASH_COLOR_ACCENT=''
    __GASH_COLOR_MUTED=''
    __GASH_COLOR_SUCCESS=''
    __GASH_COLOR_WARNING=''
    __GASH_COLOR_ERROR=''
    __GASH_COLOR_INFO=''
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
local __GASH_BOLD='' __GASH_DIM='' __GASH_UNDERLINE=''
local __GASH_COLOR_PRIMARY='' __GASH_COLOR_SECONDARY=''
local __GASH_COLOR_ACCENT='' __GASH_COLOR_MUTED=''
local __GASH_COLOR_SUCCESS='' __GASH_COLOR_WARNING=''
local __GASH_COLOR_ERROR='' __GASH_COLOR_INFO=''
EOS
}

# -----------------------------------------------------------------------------
# Core Output Functions
# -----------------------------------------------------------------------------
#
# Messages are printed with printf '%s', never echo -e: a path such as
# C:\new or a message containing "\c" is shown exactly as given.
#
# Plain output (pipes, NO_COLOR, headless) keeps the stable word prefixes
# "Info:", "OK:", "Warning:", "Error:" that scripts may grep for. A color
# terminal gets a colored glyph instead.

# Usage: __gash_emit COLOR GLYPH PLAIN_PREFIX MESSAGE   (to stderr)
# Color terminal: "<COLOR>GLYPH<OFF> MESSAGE". Otherwise: "PLAIN_PREFIX MESSAGE".
__gash_emit() {
    if __gash_use_color; then
        printf '%s%s%s %s\n' "${1-}" "${2-}" "$__GASH_COLOR_OFF" "${4-}" >&2
    else
        printf '%s %s\n' "${3-}" "${4-}" >&2
    fi
}

# Print an informational message.
# Usage: __gash_info "message"
__gash_info() {
    __gash_emit "$__GASH_COLOR_INFO" "$__GASH_GLYPH_INFO" "Info:" "${1-}"
}

# Print an error message to stderr.
# Usage: __gash_error "message"
__gash_error() {
    __gash_emit "$__GASH_COLOR_ERROR" "$__GASH_GLYPH_ERR Error:" "Error:" "${1-}"
}

# Print a success message.
# Usage: __gash_success "message"
__gash_success() {
    __gash_emit "$__GASH_COLOR_SUCCESS" "$__GASH_GLYPH_OK" "OK:" "${1-}"
}

# Print a warning message.
# Usage: __gash_warning "message"
__gash_warning() {
    __gash_emit "$__GASH_COLOR_WARNING" "$__GASH_GLYPH_WARN Warning:" "Warning:" "${1-}"
}

# Print an SSH-related message with optional color variant.
# Usage: __gash_ssh "message" [color]
# Colors: info (default), success, warning, error
__gash_ssh() {
    local message="${1-}" color="${2:-info}"
    if __gash_use_color; then
        local msg_color=""
        case "$color" in
            success) msg_color="$__GASH_COLOR_SUCCESS" ;;
            warning) msg_color="$__GASH_COLOR_WARNING" ;;
            error) msg_color="$__GASH_COLOR_ERROR" ;;
        esac
        printf '%sSSH:%s %s%s%s\n' "$__GASH_CYAN" "$__GASH_COLOR_OFF" "$msg_color" "$message" "$__GASH_COLOR_OFF" >&2
    else
        printf 'SSH: %s\n' "$message" >&2
    fi
}

# Print a step indicator for multi-step operations.
# Usage: __gash_step current total "message"
__gash_step() {
    if __gash_use_color; then
        printf '%s[%s/%s]%s %s\n' "$__GASH_COLOR_PRIMARY" "${1-}" "${2-}" "$__GASH_COLOR_OFF" "${3-}" >&2
    else
        printf '[%s/%s] %s\n' "${1-}" "${2-}" "${3-}" >&2
    fi
}

# Print a debug message (only if GASH_DEBUG=1).
# Usage: __gash_debug "message"
__gash_debug() {
    [[ "${GASH_DEBUG:-0}" == "1" ]] || return 0
    if __gash_use_color; then
        printf '%sDebug:%s %s\n' "$__GASH_COLOR_MUTED" "$__GASH_COLOR_OFF" "${1-}" >&2
    else
        printf 'Debug: %s\n' "${1-}" >&2
    fi
}

# Print colored text without prefix (stdout).
# Usage: __gash_print color "message"
# Colors: red, green, yellow, blue, cyan, white, bold_* variants, muted, dim
__gash_print() {
    local color="${1-}" message="${2-}"
    if ! __gash_use_color; then
        printf '%s\n' "$message"
        return 0
    fi
    local color_code=""
    case "$color" in
        red) color_code="${__GASH_RED}" ;;
        green) color_code="${__GASH_GREEN}" ;;
        yellow) color_code="${__GASH_YELLOW}" ;;
        blue) color_code="${__GASH_BLUE}" ;;
        cyan) color_code="${__GASH_CYAN}" ;;
        white) color_code="${__GASH_WHITE}" ;;
        bold_red) color_code="${__GASH_BOLD_RED}" ;;
        bold_green) color_code="${__GASH_BOLD_GREEN}" ;;
        bold_yellow) color_code="${__GASH_BOLD_YELLOW}" ;;
        bold_blue) color_code="${__GASH_BOLD_BLUE}" ;;
        bold_cyan) color_code="${__GASH_BOLD_CYAN}" ;;
        bold_white) color_code="${__GASH_BOLD_WHITE}" ;;
        muted) color_code="${__GASH_COLOR_MUTED}" ;;
        dim) color_code="${__GASH_DIM}" ;;
    esac
    printf '%s%s%s\n' "$color_code" "$message" "$__GASH_COLOR_OFF"
}

# -----------------------------------------------------------------------------
# Glyph-only status lines (no word prefix, also in plain mode)
# -----------------------------------------------------------------------------

# Usage: __gash_success_v2 "message"
__gash_success_v2() {
    __gash_emit "$__GASH_COLOR_SUCCESS" "$__GASH_GLYPH_OK" "$__GASH_GLYPH_OK" "${1-}"
}

# Usage: __gash_warning_v2 "message"
__gash_warning_v2() {
    __gash_emit "$__GASH_COLOR_WARNING" "$__GASH_GLYPH_WARN" "$__GASH_GLYPH_WARN" "${1-}"
}

# Usage: __gash_error_v2 "message"
__gash_error_v2() {
    __gash_emit "$__GASH_COLOR_ERROR" "$__GASH_GLYPH_ERR" "$__GASH_GLYPH_ERR" "${1-}"
}

# -----------------------------------------------------------------------------
# Spinner for Long Operations
# -----------------------------------------------------------------------------
#
# Drawn on stderr, only when stderr is a terminal and not in headless mode,
# so captured stdout never contains spinner frames. Job control is left as
# the caller had it.

declare -g __GASH_SPINNER_PID=""
declare -g __GASH_SPINNER_MSG=""
declare -g __GASH_SPINNER_PREV_TRAP=""
declare -g __GASH_SPINNER_TRAP_SET=""

# Usage: __gash_spinner_start "Loading..."
__gash_spinner_start() {
    local message="${1-Working...}"
    __GASH_SPINNER_MSG="$message"

    [[ -t 2 && "${GASH_HEADLESS-}" != "1" ]] || return 0
    [[ -n "$__GASH_SPINNER_PID" ]] && __gash_spinner_stop none

    local on="" off="" frames="$__GASH_SPINNER_FRAMES"
    if __gash_color_env_ok; then
        on="$__GASH_COLOR_PRIMARY" off="$__GASH_COLOR_OFF"
    fi

    # Without monitor mode bash prints no "[1] PID" line for the background
    # job; restore whatever the caller had right after.
    local had_monitor=0
    [[ $- == *m* ]] && had_monitor=1
    set +m
    (
        n=${#frames} i=0
        printf '\e[?25l' >&2
        while :; do
            printf '\r%s%s%s %s' "$on" "${frames:i:1}" "$off" "$message" >&2
            i=$(((i + 1) % n))
            sleep 0.1
        done
    ) &
    __GASH_SPINNER_PID=$!
    disown "$__GASH_SPINNER_PID" 2>/dev/null || true
    ((had_monitor)) && set -m

    # Stop the spinner if the script exits, keeping the caller's EXIT trap.
    __GASH_SPINNER_PREV_TRAP="$(trap -p EXIT)"
    __GASH_SPINNER_TRAP_SET="1"
    trap '__gash_spinner_stop' EXIT
    return 0
}

# Usage: __gash_spinner_stop [success|error|warning|none]
__gash_spinner_stop() {
    local status="${1-success}"

    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf '\r\e[K\e[?25h' >&2
        case "$status" in
            success) __gash_success_v2 "${__GASH_SPINNER_MSG}" ;;
            error) __gash_error_v2 "${__GASH_SPINNER_MSG}" ;;
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
# Progress Bar
# -----------------------------------------------------------------------------

# Usage: __gash_progress current total "message"   (stderr, terminals only)
__gash_progress() {
    local current="${1-0}" total="${2-100}" message="${3-}"
    [[ -t 2 && "${GASH_HEADLESS-}" != "1" ]] || return 0
    ((total > 0)) || total=1
    ((current > total)) && current=$total

    local width=30 percent filled bar="" i full="#" empty="."
    if __gash_locale_utf8; then
        full="█" empty="░"
    fi
    percent=$((current * 100 / total))
    filled=$((percent * width / 100))
    for ((i = 0; i < width; i++)); do
        if ((i < filled)); then bar+="$full"; else bar+="$empty"; fi
    done

    local on="" off=""
    if __gash_color_env_ok; then
        on="$__GASH_COLOR_PRIMARY" off="$__GASH_COLOR_OFF"
    fi
    printf '\r%s%s%s %3d%% %s\e[K' "$on" "$bar" "$off" "$percent" "$message" >&2
    ((current >= total)) && printf '\n' >&2
    return 0
}

# -----------------------------------------------------------------------------
# Section Headers
# -----------------------------------------------------------------------------

# Usage: __gash_section "Section Title"   (stdout)
__gash_section() {
    local title="${1-}" width="${COLUMNS:-80}" line="" i
    ((width > 60)) && width=60
    for ((i = 0; i < width; i++)); do line+="$__GASH_GLYPH_HLINE"; done

    printf '\n'
    if __gash_use_color; then
        printf '%s%s%s\n' "$__GASH_COLOR_PRIMARY" "$line" "$__GASH_COLOR_OFF"
        printf '%s%s%s\n' "$__GASH_BOLD" "$title" "$__GASH_COLOR_OFF"
    else
        printf '%s\n%s\n' "$line" "$title"
    fi
    printf '\n'
}

# Visible length of a string: ANSI color sequences removed.
# Usage: __gash_visible_len VAR "string"
__gash_visible_len() {
    local -n __gvl_out="$1"
    local __gvl_s="${2-}"
    while [[ "$__gvl_s" =~ $'\e'\[[0-9\;]*m ]]; do
        __gvl_s="${__gvl_s//"${BASH_REMATCH[0]}"/}"
    done
    __gvl_out=${#__gvl_s}
}

# Group the digits of an integer by thousands: 1234567 -> 1,234,567.
# Usage: __gash_group_digits VAR NUMBER
__gash_group_digits() {
    local -n __ggd_out="$1"
    local __ggd_n="${2-0}" __ggd_sign="" __ggd_res=""
    [[ "$__ggd_n" == -* ]] && __ggd_sign="-" __ggd_n="${__ggd_n#-}"
    while ((${#__ggd_n} > 3)); do
        __ggd_res=",${__ggd_n: -3}$__ggd_res"
        __ggd_n="${__ggd_n:0:${#__ggd_n}-3}"
    done
    __ggd_out="$__ggd_sign$__ggd_n$__ggd_res"
}

# A bar of WIDTH cells, VALUE/MAX of them full (at least one when VALUE > 0).
# The full and empty parts are split by a tab so callers can color each.
# Usage: __gash_bar VAR VALUE MAX WIDTH
__gash_bar() {
    local -n __gb_out="$1"
    local __gb_v="${2-0}" __gb_max="${3-0}" __gb_w="${4-10}" __gb_f=0 __gb_i __gb_full="" __gb_empty=""
    ((__gb_max > 0)) && __gb_f=$(((__gb_v * __gb_w + __gb_max / 2) / __gb_max))
    ((__gb_f < 1 && __gb_v > 0 && __gb_max > 0)) && __gb_f=1
    ((__gb_f > __gb_w)) && __gb_f=$__gb_w
    for ((__gb_i = 0; __gb_i < __gb_f; __gb_i++)); do __gb_full+="$__GASH_GLYPH_BAR_FULL"; done
    for ((__gb_i = __gb_f; __gb_i < __gb_w; __gb_i++)); do __gb_empty+="$__GASH_GLYPH_BAR_EMPTY"; done
    __gb_out="$__gb_full"$'\t'"$__gb_empty"
}

# Word-wrap TEXT to WIDTH columns into the array VAR. Newlines in TEXT start
# new lines (an empty line stays empty); a line starting with "- " wraps
# with a two-space hanging indent.
# Usage: __gash_wrap VAR WIDTH TEXT
__gash_wrap() {
    local -n __gw_out="$1"
    local __gw_w="${2-72}" __gw_text="${3-}" __gw_para __gw_word __gw_line __gw_ind __gw_lead
    local -a __gw_words
    __gw_out=()
    while IFS= read -r __gw_para || [[ -n "$__gw_para" ]]; do
        if [[ -z "$__gw_para" ]]; then
            __gw_out+=("")
            continue
        fi
        # Keep the paragraph's indent; a "- " bullet hangs two more.
        __gw_lead="${__gw_para%%[! ]*}"
        __gw_ind="$__gw_lead"
        [[ "${__gw_para#"$__gw_lead"}" == "- "* ]] && __gw_ind+="  "
        __gw_line=""
        read -ra __gw_words <<<"$__gw_para"
        for __gw_word in "${__gw_words[@]}"; do
            if [[ -z "$__gw_line" ]]; then
                __gw_line="$__gw_lead$__gw_word"
            elif ((${#__gw_line} + 1 + ${#__gw_word} > __gw_w)); then
                __gw_out+=("$__gw_line")
                __gw_line="$__gw_ind$__gw_word"
            else
                __gw_line+=" $__gw_word"
            fi
        done
        __gw_out+=("$__gw_line")
    done <<<"$__gw_text"
}

# Let the user pick one of OPTIONS. On a terminal: arrow keys (or j/k, or
# the option's number) move, Enter confirms, q or Esc cancels. Otherwise
# (GASH_TUI=plain, TERM=dumb, or input/output not a terminal) a numbered
# prompt that reads a line; an empty answer takes DEFAULT.
# Usage: __gash_choose VAR DEFAULT OPTION...   (VAR gets the 1-based index)
# Returns 1 on cancel or end of input.
__gash_choose() {
    local -n __gc_out="$1"
    local __gc_sel="${2-1}"
    shift 2
    local -a __gc_opts=("$@")
    local __gc_n=${#__gc_opts[@]} __gc_i __gc_key __gc_rest __gc_line __gc_try
    ((__gc_n > 0)) || return 1
    ((__gc_sel >= 1 && __gc_sel <= __gc_n)) || __gc_sel=1
    local __gc_on="" __gc_off=""
    if __gash_use_color; then
        __gc_on="${__GASH_COLOR_PRIMARY}${__GASH_BOLD}" __gc_off="$__GASH_COLOR_OFF"
    fi

    if [[ "${GASH_TUI-}" == plain || "${TERM-}" == dumb || ! -t 0 || ! -t 1 ]]; then
        for ((__gc_i = 0; __gc_i < __gc_n; __gc_i++)); do
            printf '  %d) %s\n' $((__gc_i + 1)) "${__gc_opts[__gc_i]}"
        done
        for __gc_try in 1 2 3; do
            printf '  Choice [%d]: ' "$__gc_sel"
            IFS= read -r __gc_line || {
                echo
                return 1
            }
            __gc_line="${__gc_line//[[:space:]]/}"
            [[ -z "$__gc_line" ]] && __gc_line="$__gc_sel"
            [[ "$__gc_line" == q ]] && return 1
            if [[ "$__gc_line" =~ ^[0-9]+$ ]] && ((10#$__gc_line >= 1 && 10#$__gc_line <= __gc_n)); then
                __gc_out=$((10#$__gc_line))
                return 0
            fi
            printf '  Type a number from 1 to %d (attempt %d of 3).\n' "$__gc_n" "$__gc_try"
        done
        return 1
    fi

    local __gc_first=1
    while :; do
        ((__gc_first)) || printf '\e[%dA' "$__gc_n"
        __gc_first=0
        for ((__gc_i = 0; __gc_i < __gc_n; __gc_i++)); do
            if ((__gc_i + 1 == __gc_sel)); then
                printf '\r\e[K  %s%s %s%s\n' "$__gc_on" "$__GASH_GLYPH_POINTER" "${__gc_opts[__gc_i]}" "$__gc_off"
            else
                printf '\r\e[K    %s\n' "${__gc_opts[__gc_i]}"
            fi
        done
        IFS= read -rsn1 __gc_key || return 1
        case "$__gc_key" in
            $'\e')
                __gc_rest=""
                IFS= read -rsn2 -t 0.05 __gc_rest || true
                case "$__gc_rest" in
                    '[A' | 'OA') ((__gc_sel = __gc_sel > 1 ? __gc_sel - 1 : __gc_n)) ;;
                    '[B' | 'OB') ((__gc_sel = __gc_sel < __gc_n ? __gc_sel + 1 : 1)) ;;
                    '') return 1 ;;
                esac
                ;;
            k) ((__gc_sel = __gc_sel > 1 ? __gc_sel - 1 : __gc_n)) ;;
            j) ((__gc_sel = __gc_sel < __gc_n ? __gc_sel + 1 : 1)) ;;
            [1-9]) ((__gc_key <= __gc_n)) && __gc_sel=$__gc_key ;;
            q) return 1 ;;
            '') break ;;
        esac
    done
    __gc_out=$__gc_sel
    return 0
}

# Draw a rounded box around lines (stdout), sized to the longest line and
# capped at the terminal width. Lines may carry color codes. The border
# takes BORDER_COLOR when colors are on.
# Usage: __gash_box [--color ESC] LINE...
__gash_box() {
    local bcolor=""
    if [[ "${1-}" == "--color" ]]; then
        bcolor="${2-}"
        shift 2
    fi
    local on="" off=""
    if __gash_use_color && [[ -n "$bcolor" ]]; then
        on="$bcolor" off="$__GASH_COLOR_OFF"
    fi
    local line len inner=0
    for line in "$@"; do
        __gash_visible_len len "$line"
        ((len > inner)) && inner=$len
    done
    local max=$(($(__gash_tty_width) - 6))
    ((max < 20)) && max=20
    ((inner > max)) && inner=$max

    local corners="$__GASH_GLYPH_CORNERS" v="$__GASH_GLYPH_VLINE" rule="" i
    for ((i = 0; i < inner + 4; i++)); do rule+="$__GASH_GLYPH_HLINE"; done
    printf '%s%s%s%s%s\n' "$on" "${corners:0:1}" "$rule" "${corners:1:1}" "$off"
    for line in "$@"; do
        __gash_visible_len len "$line"
        if ((len > inner)); then
            # Too wide for the terminal: drop the colors, cut, mark the cut.
            while [[ "$line" =~ $'\e'\[[0-9\;]*m ]]; do
                line="${line//"${BASH_REMATCH[0]}"/}"
            done
            line="${line:0:inner-1}${__GASH_GLYPH_ELLIPSIS}"
            len=$inner
        fi
        printf '%s%s%s  %s%*s  %s%s%s\n' "$on" "$v" "$off" "$line" $((inner - len)) '' "$on" "$v" "$off"
    done
    printf '%s%s%s%s%s\n' "$on" "${corners:2:1}" "$rule" "${corners:3:1}" "$off"
}

# -----------------------------------------------------------------------------
# Tables
# -----------------------------------------------------------------------------

# Split LINE on every tab into the array named ARRAY, keeping empty fields
# (read -a would merge consecutive tabs).
# Usage: __gash_split_tabs ARRAY LINE
__gash_split_tabs() {
    local -n __gst_out="$1"
    local rest="${2-}"
    __gst_out=()
    while [[ "$rest" == *$'\t'* ]]; do
        __gst_out+=("${rest%%$'\t'*}")
        rest="${rest#*$'\t'}"
    done
    __gst_out+=("$rest")
}

# Render tab-separated rows from stdin as an aligned table on stdout. The
# first row is the header (bold and underlined on a color terminal, followed
# by a rule otherwise). Cells must be plain text: width is counted in
# characters, so color codes inside cells would break the alignment.
#
# --color N:ESC colors column N (repeatable; only on a color terminal).
# --fit truncates the last column so rows fit the terminal width.
#
# Usage: printf 'NAME\tSIZE\na\t1\n' | __gash_table [--right 2,3] [--indent N] [--no-header]
#                                                   [--color 1:"$ESC"] [--fit]
__gash_table() {
    local right="" indent=0 header=1 fit=0
    local -A colcolor=()
    while (($# > 0)); do
        case "$1" in
            --color)
                [[ "${2-}" == [0-9]*:* ]] && colcolor[${2%%:*}]="${2#*:}"
                shift 2 || break
                ;;
            --fit)
                fit=1
                shift
                ;;
            --right)
                right=",${2-},"
                shift 2 || break
                ;;
            --indent)
                indent="${2-0}"
                shift 2 || break
                ;;
            --no-header)
                header=0
                shift
                ;;
            *) shift ;;
        esac
    done
    [[ "$indent" =~ ^[0-9]+$ ]] || indent=0

    local -a rows=() widths=() cells=()
    local line c ncols=0
    while IFS= read -r line || [[ -n "$line" ]]; do
        rows+=("$line")
        __gash_split_tabs cells "$line"
        for c in "${!cells[@]}"; do
            ((${#cells[c]} > ${widths[c]:-0})) && widths[c]=${#cells[c]}
        done
        ((${#cells[@]} > ncols)) && ncols=${#cells[@]}
    done
    ((${#rows[@]} > 0)) || return 0

    local color=0
    __gash_use_color && color=1
    # Room left for the last column when it must fit the terminal.
    local last_max=-1
    if ((fit && ncols > 0)); then
        local used=$indent
        for ((c = 0; c < ncols - 1; c++)); do used=$((used + ${widths[c]:-0} + 2)); done
        last_max=$(($(__gash_tty_width) - used))
        ((last_max < 10)) && last_max=10
    fi
    local pad r out cell plain esc
    printf -v pad '%*s' "$indent" ''
    for r in "${!rows[@]}"; do
        __gash_split_tabs cells "${rows[r]}"
        out="" plain=""
        for ((c = 0; c < ncols; c++)); do
            cell="${cells[c]-}"
            if ((c == ncols - 1)) && ((last_max > 0 && ${#cell} > last_max)); then
                cell="${cell:0:last_max-1}${__GASH_GLYPH_ELLIPSIS}"
            fi
            if ((c == ncols - 1)) && [[ "$right" != *",$((c + 1)),"* ]]; then
                :
            elif [[ "$right" == *",$((c + 1)),"* ]]; then
                printf -v cell '%*s' "${widths[c]}" "$cell"
            else
                printf -v cell '%-*s' "${widths[c]}" "$cell"
            fi
            plain+="$cell"
            esc="${colcolor[$((c + 1))]-}"
            if ((color)) && [[ -n "$esc" ]] && ! ((r == 0 && header)); then
                out+="${esc}${cell}${__GASH_COLOR_OFF}"
            else
                out+="$cell"
            fi
            ((c < ncols - 1)) && out+="  " && plain+="  "
        done
        if ((r == 0 && header)); then
            out="$plain"
            if ((color)); then
                printf '%s%s%s%s%s\n' "$pad" "$__GASH_BOLD" "$__GASH_UNDERLINE" "$out" "$__GASH_COLOR_OFF"
            else
                printf '%s%s\n' "$pad" "$out"
                local rule="" total=0 i
                for c in "${!widths[@]}"; do total=$((total + widths[c])); done
                if ((last_max > 0 && ${widths[ncols - 1]:-0} > last_max)); then
                    total=$((total - widths[ncols - 1] + last_max))
                fi
                total=$((total + 2 * (ncols - 1)))
                for ((i = 0; i < total; i++)); do rule+="$__GASH_GLYPH_HLINE"; done
                printf '%s%s\n' "$pad" "$rule"
            fi
        else
            printf '%s%s\n' "$pad" "$out"
        fi
    done
}
