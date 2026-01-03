#!/usr/bin/env bash

# Gash Core: Output Functions
# Centralized output formatting for consistent messaging across Gash.
#
# All functions are errexit-safe and can be used with `set -e`.

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
__GASH_BOLD_WHITE='\033[1;37m'

# -----------------------------------------------------------------------------
# Core Output Functions
# -----------------------------------------------------------------------------

# Print an informational message.
# Usage: __gash_info "message"
__gash_info() {
    local message="${1-}"
    echo -e "${__GASH_CYAN}Info:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}"
}

# Print an error message to stderr.
# Usage: __gash_error "message"
__gash_error() {
    local message="${1-}"
    echo -e "${__GASH_BOLD_RED}Error:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}" >&2
}

# Print a success message.
# Usage: __gash_success "message"
__gash_success() {
    local message="${1-}"
    echo -e "${__GASH_BOLD_GREEN}OK:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}"
}

# Print a warning message.
# Usage: __gash_warning "message"
__gash_warning() {
    local message="${1-}"
    echo -e "${__GASH_BOLD_YELLOW}Warning:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}"
}

# Print an SSH-related message with optional color variant.
# Usage: __gash_ssh "message" [color]
# Colors: info (default), success, warning, error
__gash_ssh() {
    local message="${1-}"
    local color="${2:-info}"
    local msg_color

    case "$color" in
        success) msg_color="${__GASH_BOLD_GREEN}" ;;
        warning) msg_color="${__GASH_BOLD_YELLOW}" ;;
        error)   msg_color="${__GASH_BOLD_RED}" ;;
        *)       msg_color="${__GASH_BOLD_WHITE}" ;;
    esac

    echo -e "${__GASH_CYAN}SSH:${__GASH_COLOR_OFF} ${msg_color}${message}${__GASH_COLOR_OFF}"
}

# Print a step indicator for multi-step operations.
# Usage: __gash_step current total "message"
__gash_step() {
    local current="${1-}"
    local total="${2-}"
    local message="${3-}"
    echo -e "${__GASH_CYAN}[${current}/${total}]${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}${message}${__GASH_COLOR_OFF}"
}

# Print a debug message (only if GASH_DEBUG=1).
# Usage: __gash_debug "message"
__gash_debug() {
    if [[ "${GASH_DEBUG:-0}" == "1" ]]; then
        local message="${1-}"
        echo -e "${__GASH_BLUE}Debug:${__GASH_COLOR_OFF} ${message}" >&2
    fi
}

# Print raw colored text without prefix.
# Usage: __gash_print color "message"
# Colors: red, green, yellow, blue, cyan, white
__gash_print() {
    local color="${1-}"
    local message="${2-}"
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
        bold_white)  color_code="${__GASH_BOLD_WHITE}" ;;
        *)      color_code="${__GASH_COLOR_OFF}" ;;
    esac

    echo -e "${color_code}${message}${__GASH_COLOR_OFF}"
}
