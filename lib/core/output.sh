#!/usr/bin/env bash

# Gash Core: Output Functions
# Centralized output formatting for consistent messaging across Gash.
#
# All functions are errexit-safe and can be used with `set -e`.
#
# COLOR POLICY (v1.5+):
#   Gash follows a hybrid color gating model:
#
#   1. Load-time: if GASH_HEADLESS=1, NO_COLOR (any non-empty value) or
#      GASH_NO_COLOR (non-empty and not 0) is set,
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

# Terminal control (not colors: kept when colors are off, and only ever
# written to a terminal by the primitives below).
__GASH_ERASE_EOL=$'\e[K'
__GASH_CURSOR_HIDE=$'\e[?25l'
__GASH_CURSOR_SHOW=$'\e[?25h'
__GASH_CLEAR_SCREEN=$'\e[H\e[2J'

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
    __GASH_GLYPH_BAR_FULL='━'
    __GASH_GLYPH_BAR_EMPTY='─'
    __GASH_GLYPH_DOT='·'
    __GASH_GLYPH_ELLIPSIS='…'
    __GASH_GLYPH_POINTER='❯'
    __GASH_GLYPH_TREE_MID='├─'
    __GASH_GLYPH_TREE_END='└─'
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
    __GASH_GLYPH_TREE_MID='|-'
    __GASH_GLYPH_TREE_END='`-'
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
#   NO_COLOR       any non-empty value disables (https://no-color.org)
#   GASH_NO_COLOR  any non-empty value other than 0 disables (0 = colors on)
__gash_color_env_ok() {
    [[ "${GASH_HEADLESS-}" == "1" ]] && return 1
    [[ -n "${NO_COLOR-}" ]] && return 1
    [[ -n "${GASH_NO_COLOR-}" && "${GASH_NO_COLOR}" != "0" ]] && return 1
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

# A block of `local VAR=''` declarations that, eval'd inside a function,
# shadows every Gash color variable with an empty one for that call (and,
# through bash dynamic scoping, for every function it calls).
#
# Usage pattern (inside a function body):
#   __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS"
# The TTY test must run in the function itself: inside a command
# substitution stdout is the substitution's pipe, never the terminal.
__GASH_COLOR_OFF_LOCALS="local __GASH_COLOR_OFF=''
local __GASH_RED='' __GASH_GREEN='' __GASH_YELLOW=''
local __GASH_BLUE='' __GASH_CYAN='' __GASH_WHITE=''
local __GASH_BOLD_RED='' __GASH_BOLD_GREEN='' __GASH_BOLD_YELLOW=''
local __GASH_BOLD_BLUE='' __GASH_BOLD_CYAN='' __GASH_BOLD_WHITE=''
local __GASH_BOLD='' __GASH_DIM='' __GASH_UNDERLINE=''
local __GASH_COLOR_PRIMARY='' __GASH_COLOR_SECONDARY=''
local __GASH_COLOR_ACCENT='' __GASH_COLOR_MUTED=''
local __GASH_COLOR_SUCCESS='' __GASH_COLOR_WARNING=''
local __GASH_COLOR_ERROR='' __GASH_COLOR_INFO=''"

# Deprecated: run as `eval "$(__gash_color_scope ...)"` its TTY test sees
# the command substitution's pipe, so it always blanks the colors. Kept for
# external callers; use the __GASH_COLOR_OFF_LOCALS pattern above.
__gash_color_scope() {
    local no_color="${1-0}"
    __gash_want_colors "$no_color" && return 0
    printf '%s\n' "$__GASH_COLOR_OFF_LOCALS"
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

# Print an SSH-related message with optional color variant (stderr).
# Usage: __gash_ssh "message" [color]
# Colors: info (default), success, warning, error. A color terminal gets
# the variant's glyph before "SSH:"; plain output keeps "SSH: message".
__gash_ssh() {
    local message="${1-}" color="${2:-info}"
    if __gash_use_color; then
        local c="$__GASH_COLOR_INFO" g="$__GASH_GLYPH_INFO"
        case "$color" in
            success) c="$__GASH_COLOR_SUCCESS" g="$__GASH_GLYPH_OK" ;;
            warning) c="$__GASH_COLOR_WARNING" g="$__GASH_GLYPH_WARN" ;;
            error) c="$__GASH_COLOR_ERROR" g="$__GASH_GLYPH_ERR" ;;
        esac
        printf '%s%s%s %sSSH:%s %s\n' "$c" "$g" "$__GASH_COLOR_OFF" "$__GASH_COLOR_PRIMARY" "$__GASH_COLOR_OFF" "$message" >&2
    else
        printf 'SSH: %s\n' "$message" >&2
    fi
}

# Print a progress note without a word prefix (stderr): "<glyph> TEXT" on a
# color terminal, "TEXT" otherwise (apt-style "Get:" and "Fetched" lines).
# Usage: __gash_note "Fetched 3 MiB in 2s"
__gash_note() {
    if __gash_use_color; then
        printf '%s%s%s %s\n' "$__GASH_COLOR_INFO" "$__GASH_GLYPH_INFO" "$__GASH_COLOR_OFF" "${1-}" >&2
    else
        printf '%s\n' "${1-}" >&2
    fi
}

# Print a hint after an error (stderr): "Hint: TEXT", muted on a color
# terminal.
# Usage: __gash_hint "Run gash_env_init to create one"
__gash_hint() {
    if __gash_use_color; then
        printf '%sHint: %s%s\n' "$__GASH_COLOR_MUTED" "${1-}" "$__GASH_COLOR_OFF" >&2
    else
        printf 'Hint: %s\n' "${1-}" >&2
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
    __gash_emit "$__GASH_COLOR_MUTED" "$__GASH_GLYPH_INFO Debug:" "Debug:" "${1-}"
}

# Print a usage reminder after a usage error (stderr).
# Color terminal: "<glyph> Usage: TEXT". Otherwise: "Usage: TEXT".
# Usage: __gash_usage "name ARG [OPTIONS]"
__gash_usage() {
    __gash_emit "$__GASH_COLOR_INFO" "$__GASH_GLYPH_INFO Usage:" "Usage:" "${1-}"
}

# The escape sequence of a semantic style NAME into VAR: primary,
# secondary, accent, muted, success, warning, error, info, bold, dim,
# underline; "" or none for no style. Anything else is used as given (an
# escape sequence from the palette variables).
# Usage: __gash_color_of VAR NAME
__gash_color_of() {
    local -n __gco_out="$1"
    case "${2-}" in
        "" | none) __gco_out="" ;;
        primary) __gco_out="$__GASH_COLOR_PRIMARY" ;;
        secondary) __gco_out="$__GASH_COLOR_SECONDARY" ;;
        accent) __gco_out="$__GASH_COLOR_ACCENT" ;;
        muted) __gco_out="$__GASH_COLOR_MUTED" ;;
        success) __gco_out="$__GASH_COLOR_SUCCESS" ;;
        warning) __gco_out="$__GASH_COLOR_WARNING" ;;
        error) __gco_out="$__GASH_COLOR_ERROR" ;;
        info) __gco_out="$__GASH_COLOR_INFO" ;;
        bold) __gco_out="$__GASH_BOLD" ;;
        dim) __gco_out="$__GASH_DIM" ;;
        underline) __gco_out="$__GASH_UNDERLINE" ;;
        *) __gco_out="$2" ;;
    esac
}

# Print text in a style, without prefix (stdout).
# Usage: __gash_print STYLE "message"
# Styles: the semantic names of __gash_color_of. The older hue names (red,
# green, yellow, blue, cyan, white, bold_*) still work for user scripts.
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
        primary | secondary | accent | muted | success | warning | error | info | bold | dim | underline)
            __gash_color_of color_code "$color"
            ;;
    esac
    printf '%s%s%s\n' "$color_code" "$message" "$__GASH_COLOR_OFF"
}

# -----------------------------------------------------------------------------
# Glyph-only status lines (kept for older callers)
# -----------------------------------------------------------------------------

# The same as __gash_success, __gash_warning and __gash_error: a glyph on a
# color terminal, the word prefix in plain output.
__gash_success_v2() { __gash_success "${1-}"; }
__gash_warning_v2() { __gash_warning "${1-}"; }
__gash_error_v2() { __gash_error "${1-}"; }

# -----------------------------------------------------------------------------
# Spinner for Long Operations
# -----------------------------------------------------------------------------
#
# Drawn on stderr, only when stderr is a terminal and not in headless mode,
# so captured stdout never contains spinner frames. Escape sequences (color,
# cursor, erase) only when __gash_use_color allows them too: with NO_COLOR or
# a piped stdout the line is redrawn with carriage returns and spaces alone.
# Job control is left as the caller had it.

# True when stderr may carry escape sequences: a terminal, and colors on.
# Usage: __gash_stderr_fancy && printf '%s' "$__GASH_ERASE_EOL" >&2
__gash_stderr_fancy() {
    [[ -t 2 ]] && __gash_use_color
}

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

    local on="" off="" hide="" frames="$__GASH_SPINNER_FRAMES"
    if __gash_stderr_fancy; then
        on="$__GASH_COLOR_PRIMARY" off="$__GASH_COLOR_OFF" hide="$__GASH_CURSOR_HIDE"
    fi

    # Without monitor mode bash prints no "[1] PID" line for the background
    # job; restore whatever the caller had right after.
    local had_monitor=0
    [[ $- == *m* ]] && had_monitor=1
    set +m
    (
        n=${#frames} i=0
        printf '%s' "$hide" >&2
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
        if __gash_stderr_fancy; then
            printf '\r%s%s' "$__GASH_ERASE_EOL" "$__GASH_CURSOR_SHOW" >&2
        else
            printf '\r%*s\r' $((${#__GASH_SPINNER_MSG} + 2)) '' >&2
        fi
        case "$status" in
            success) __gash_success "${__GASH_SPINNER_MSG}" ;;
            error) __gash_error "${__GASH_SPINNER_MSG}" ;;
            warning) __gash_warning "${__GASH_SPINNER_MSG}" ;;
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

    local on="" off="" el=""
    if __gash_stderr_fancy; then
        on="$__GASH_COLOR_PRIMARY" off="$__GASH_COLOR_OFF" el="$__GASH_ERASE_EOL"
    fi
    printf '\r%s%s%s %3d%% %s%s' "$on" "$bar" "$off" "$percent" "$message" "$el" >&2
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

# A bold line without rules, for a title inside a block (stdout).
# Usage: __gash_heading [--indent N] "Title"
__gash_heading() {
    local indent=0
    if [[ "${1-}" == "--indent" ]]; then
        indent="${2-0}"
        shift 2
    fi
    [[ "$indent" =~ ^[0-9]+$ ]] || indent=0
    if __gash_use_color; then
        printf '%*s%s%s%s\n' "$indent" '' "$__GASH_BOLD" "${1-}" "$__GASH_COLOR_OFF"
    else
        printf '%*s%s\n' "$indent" '' "${1-}"
    fi
}

# Key/value lines (stdout): "KEY VALUE", keys padded to the longest one (or
# to --width N; 0 = no padding) and joined to the value by --sep (default
# one space). On a color terminal keys are bold and values take
# --value-color STYLE (a __gash_color_of name) when given.
# Usage: __gash_kv [--indent N] [--width N] [--sep STR] [--value-color STYLE] KEY VALUE [KEY VALUE...]
__gash_kv() {
    local indent=0 width=-1 sep=" " vstyle=""
    while (($# > 0)); do
        case "$1" in
            --indent)
                indent="${2-0}"
                shift 2 || break
                ;;
            --width)
                width="${2-0}"
                shift 2 || break
                ;;
            --sep)
                sep="${2-}"
                shift 2 || break
                ;;
            --value-color)
                vstyle="${2-}"
                shift 2 || break
                ;;
            *) break ;;
        esac
    done
    [[ "$indent" =~ ^[0-9]+$ ]] || indent=0
    [[ "$width" =~ ^-?[0-9]+$ ]] || width=-1
    local i key
    if ((width < 0)); then
        width=0
        for ((i = 1; i <= $#; i += 2)); do
            key="${!i}"
            ((${#key} > width)) && width=${#key}
        done
    fi
    local kon="" von="" off=""
    if __gash_use_color; then
        kon="$__GASH_BOLD" off="$__GASH_COLOR_OFF"
        __gash_color_of von "$vstyle"
    fi
    local val j
    for ((i = 1; i <= $#; i += 2)); do
        j=$((i + 1))
        key="${!i}" val="${!j-}"
        printf '%*s%s%-*s%s%s%s%s%s\n' "$indent" '' "$kon" "$width" "$key" "$off" "$sep" "$von" "$val" "${von:+$off}"
    done
}

# A bulleted list (stdout): "  • ITEM" per item, the bullet in the primary
# color (or STYLE) on a color terminal.
# Usage: __gash_list [--indent N] [--color STYLE] ITEM...
__gash_list() {
    local indent=2 style=primary
    while (($# > 0)); do
        case "$1" in
            --indent)
                indent="${2-2}"
                shift 2 || break
                ;;
            --color)
                style="${2-}"
                shift 2 || break
                ;;
            *) break ;;
        esac
    done
    [[ "$indent" =~ ^[0-9]+$ ]] || indent=2
    local on="" off="" item
    if __gash_use_color; then
        __gash_color_of on "$style"
        off="$__GASH_COLOR_OFF"
    fi
    for item in "$@"; do
        printf '%*s%s%s%s %s\n' "$indent" '' "$on" "$__GASH_GLYPH_BULLET" "${on:+$off}" "$item"
    done
}

# A status line on stdout, for findings in a report rather than diagnostics:
# "<glyph> TEXT" in the status color on a color terminal, "Word: TEXT"
# otherwise (OK:, Info:, Warning:, Error:, as the emitters print them).
# STATUS: ok, info, warning or error.
# Usage: __gash_mark [--indent N] STATUS "TEXT"
__gash_mark() {
    local indent=0
    if [[ "${1-}" == "--indent" ]]; then
        indent="${2-0}"
        shift 2
    fi
    [[ "$indent" =~ ^[0-9]+$ ]] || indent=0
    local status="${1-info}" text="${2-}" c g word
    case "$status" in
        ok | success) c="$__GASH_COLOR_SUCCESS" g="$__GASH_GLYPH_OK" word="OK:" ;;
        warning) c="$__GASH_COLOR_WARNING" g="$__GASH_GLYPH_WARN" word="Warning:" ;;
        error) c="$__GASH_COLOR_ERROR" g="$__GASH_GLYPH_ERR" word="Error:" ;;
        *) c="$__GASH_COLOR_INFO" g="$__GASH_GLYPH_INFO" word="Info:" ;;
    esac
    if __gash_use_color; then
        printf '%*s%s%s%s %s\n' "$indent" '' "$c" "$g" "$__GASH_COLOR_OFF" "$text"
    else
        printf '%*s%s %s\n' "$indent" '' "$word" "$text"
    fi
}

# Clear the terminal (stdout is a terminal only; nothing otherwise).
# Usage: __gash_clear_screen
__gash_clear_screen() {
    [[ -t 1 && "${GASH_HEADLESS-}" != "1" ]] || return 0
    printf '%s' "$__GASH_CLEAR_SCREEN"
}

# Print a question with its answers in brackets (stderr), the way every
# confirmation in Gash looks: "QUESTION [y/N] ". The answers are bold on a
# color terminal; the question is printed as given (never escape-expanded).
# Usage: __gash_ask_render "Delete 3 files?" "y/N"
__gash_ask_render() {
    if __gash_use_color; then
        printf '%s %s[%s]%s ' "${1-}" "$__GASH_BOLD" "${2-}" "$__GASH_COLOR_OFF" >&2
    else
        printf '%s [%s] ' "${1-}" "${2-}" >&2
    fi
}

# Read one line of input into VAR after printing PROMPT (stderr), like
# `read -r -p`: the prompt only appears when input is a terminal.
# Usage: __gash_read_line VAR "Question: "
__gash_read_line() {
    [[ -t 0 ]] && printf '%s' "${2-}" >&2
    read -r "$1"
}

# The 256-color chart of all_colors (stdout): one "colourN" line per
# color, each in its own color on a color terminal, plain otherwise.
# Usage: __gash_color_chart
__gash_color_chart() {
    local i
    if __gash_use_color; then
        for ((i = 0; i < 256; i++)); do
            printf '\e[38;5;%dmcolour%d%s\n' "$i" "$i" "$__GASH_COLOR_OFF"
        done
    else
        for ((i = 0; i < 256; i++)); do
            printf 'colour%d\n' "$i"
        done
    fi
}

# Every SGR sequence the primitives above print besides the palette and
# the attributes (the swatches of __gash_color_chart), one per line: what
# tests/specs/output_style_render_spec.sh accepts on a terminal.
__gash_sgr_extra() {
    local i
    for ((i = 0; i < 256; i++)); do
        printf '\e[38;5;%dm\n' "$i"
    done
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
# characters, styles come from the options. Column numbers are 1-based and
# count the input columns, hidden ones included.
#
#   --right N,N       right-align these columns
#   --indent N        N spaces before every row
#   --gap N           N spaces between columns (default 2)
#   --no-header       the first row is data
#   --color N:STYLE   style of column N (a __gash_color_of name, or an
#                     escape sequence); repeatable, never on the header
#   --cell-color N:M  style of column N taken, row by row, from column M
#                     (a style name, or empty); column M is not shown
#   --bar N:W         column N holds VALUE/MAX: drawn as a W-cell bar, the
#                     full part in the column's style (primary by default),
#                     the empty part muted
#   --min N:W         column N at least W wide
#   --max N:W         cut column N to W characters, ellipsis at the end
#   --max-left N:W    cut column N to W characters, keeping the end
#   --fit [N]         cut column N (default: the last) so that rows fit
#                     the terminal width
#
# Usage: printf 'NAME\tSIZE\na\t1\n' | __gash_table [options]
__gash_table() {
    local right="" indent=0 header=1 fit=0 fitcol=0 gap=2
    local -A colcolor=() cellcolor=() barw=() minw=() maxw=() maxleft=() hidden=()
    while (($# > 0)); do
        case "$1" in
            --color)
                [[ "${2-}" == [0-9]*:* ]] && colcolor[${2%%:*}]="${2#*:}"
                shift 2 || break
                ;;
            --cell-color)
                if [[ "${2-}" =~ ^([0-9]+):([0-9]+)$ ]]; then
                    cellcolor[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}"
                    hidden[${BASH_REMATCH[2]}]=1
                fi
                shift 2 || break
                ;;
            --bar)
                [[ "${2-}" =~ ^([0-9]+):([0-9]+)$ ]] && barw[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}"
                shift 2 || break
                ;;
            --min | --max | --max-left)
                if [[ "${2-}" =~ ^([0-9]+):([0-9]+)$ ]]; then
                    case "$1" in
                        --min) minw[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}" ;;
                        --max) maxw[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}" ;;
                        --max-left) maxleft[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}" ;;
                    esac
                fi
                shift 2 || break
                ;;
            --fit)
                fit=1
                if [[ "${2-}" =~ ^[0-9]+$ ]]; then
                    fitcol="$2"
                    shift
                fi
                shift
                ;;
            --gap)
                [[ "${2-}" =~ ^[0-9]+$ ]] && gap="$2"
                shift 2 || break
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

    local -a rows=() widths=() cells=() vis=()
    local line c n ncols=0 w
    while IFS= read -r line || [[ -n "$line" ]]; do
        rows+=("$line")
        __gash_split_tabs cells "$line"
        ((${#cells[@]} > ncols)) && ncols=${#cells[@]}
    done
    ((${#rows[@]} > 0)) || return 0

    # Visible columns, 0-based, in order.
    for ((c = 0; c < ncols; c++)); do
        [[ -n "${hidden[$((c + 1))]-}" ]] || vis+=("$c")
    done
    ((${#vis[@]} > 0)) || return 0
    local last=${vis[${#vis[@]} - 1]}
    ((fitcol > 0)) || fitcol=$((last + 1))

    # Cut cells, then measure.
    local r cell lim joined
    for r in "${!rows[@]}"; do
        __gash_split_tabs cells "${rows[r]}"
        for c in "${vis[@]}"; do
            n=$((c + 1))
            cell="${cells[c]-}"
            if [[ -n "${barw[$n]-}" ]] && ! ((r == 0 && header)); then
                w=${barw[$n]}
            else
                lim="${maxw[$n]-}"
                if [[ -n "$lim" ]] && ((lim > 0 && ${#cell} > lim)); then
                    cell="${cell:0:lim-1}${__GASH_GLYPH_ELLIPSIS}"
                fi
                lim="${maxleft[$n]-}"
                if [[ -n "$lim" ]] && ((lim > 0 && ${#cell} > lim)); then
                    cell="${__GASH_GLYPH_ELLIPSIS}${cell: -$((lim - 1))}"
                fi
                cells[c]="$cell"
                w=${#cell}
            fi
            ((w > ${widths[c]:-0})) && widths[c]=$w
        done
        joined="${cells[0]-}"
        for ((c = 1; c < ${#cells[@]}; c++)); do joined+=$'\t'"${cells[c]}"; done
        rows[r]="$joined"
    done
    for c in "${vis[@]}"; do
        w="${minw[$((c + 1))]-}"
        [[ -n "$w" ]] && ((w > ${widths[c]:-0})) && widths[c]=$w
    done

    # Room left for the fitted column when rows must fit the terminal.
    local fit_max=-1 fc=$((fitcol - 1))
    if ((fit)) && [[ -z "${hidden[$fitcol]-}" ]] && ((fc < ncols)); then
        local used=$indent
        for c in "${vis[@]}"; do
            ((c == fc)) && continue
            used=$((used + ${widths[c]:-0} + gap))
        done
        fit_max=$(($(__gash_tty_width) - used))
        ((fit_max < 10)) && fit_max=10
        ((${widths[fc]:-0} > fit_max)) && widths[fc]=$fit_max
    fi

    local color=0
    __gash_use_color && color=1
    local pad sep r out plain esc style full empty bar v mx
    printf -v pad '%*s' "$indent" ''
    printf -v sep '%*s' "$gap" ''
    local muted="" off=""
    ((color)) && muted="$__GASH_COLOR_MUTED" off="$__GASH_COLOR_OFF"
    for r in "${!rows[@]}"; do
        __gash_split_tabs cells "${rows[r]}"
        out="" plain=""
        for c in "${vis[@]}"; do
            n=$((c + 1))
            cell="${cells[c]-}"
            # Style of this cell: its row's style column, else the column's.
            style="${colcolor[$n]-}"
            [[ -n "${cellcolor[$n]-}" ]] && style="${cells[${cellcolor[$n]} - 1]-}"
            esc=""
            ((color)) && __gash_color_of esc "$style"
            if [[ -n "${barw[$n]-}" ]] && ! ((r == 0 && header)); then
                v="${cell%%/*}" mx="${cell#*/}"
                [[ "$v" =~ ^[0-9]+$ ]] || v=0
                [[ "$mx" =~ ^[0-9]+$ ]] || mx=0
                __gash_bar bar "$v" "$mx" "${barw[$n]}"
                full="${bar%%$'\t'*}" empty="${bar#*$'\t'}"
                ((color)) && [[ -z "$esc" ]] && esc="$__GASH_COLOR_PRIMARY"
                printf -v cell '%s%s' "$full" "$empty"
                plain+="$cell"
                if ((color)); then
                    out+="${esc}${full}${off}${muted}${empty}${off}"
                else
                    out+="$cell"
                fi
                printf -v cell '%*s' $((widths[c] - barw[$n])) ''
                plain+="$cell" out+="$cell"
                ((c == last)) || plain+="$sep" out+="$sep"
                continue
            fi
            if ((c == fc && fit_max > 0 && ${#cell} > widths[c])); then
                if [[ -n "${maxleft[$n]-}" ]]; then
                    cell="${__GASH_GLYPH_ELLIPSIS}${cell: -$((widths[c] - 1))}"
                else
                    cell="${cell:0:widths[c]-1}${__GASH_GLYPH_ELLIPSIS}"
                fi
            fi
            if [[ "$right" == *",$n,"* ]]; then
                printf -v cell '%*s' "${widths[c]}" "$cell"
            elif ((c != last)); then
                printf -v cell '%-*s' "${widths[c]}" "$cell"
            fi
            plain+="$cell"
            if ((color)) && [[ -n "$esc" ]] && ! ((r == 0 && header)); then
                out+="${esc}${cell}${__GASH_COLOR_OFF}"
            else
                out+="$cell"
            fi
            ((c == last)) || plain+="$sep" out+="$sep"
        done
        if ((r == 0 && header)); then
            if ((color)); then
                printf '%s%s%s%s%s\n' "$pad" "$__GASH_BOLD" "$__GASH_UNDERLINE" "$plain" "$__GASH_COLOR_OFF"
            else
                printf '%s%s\n' "$pad" "$plain"
                local rule="" total=0 i
                for c in "${vis[@]}"; do total=$((total + widths[c])); done
                total=$((total + gap * (${#vis[@]} - 1)))
                for ((i = 0; i < total; i++)); do rule+="$__GASH_GLYPH_HLINE"; done
                printf '%s%s\n' "$pad" "$rule"
            fi
        else
            printf '%s%s\n' "$pad" "$out"
        fi
    done
}
