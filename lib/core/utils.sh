#!/usr/bin/env bash

# Gash Core: Utility Functions
# General-purpose helpers used across Gash modules.
#
# All functions are errexit-safe and can be used with `set -e`.

# -----------------------------------------------------------------------------
# Help System
# -----------------------------------------------------------------------------

# Print help message for a function if requested.
# Usage: needs_help "program" "usage" "description" "$1" && return
# Returns: 0 if help was requested (and printed), 1 otherwise
needs_help() {
    local program="${1-}"
    local usage="${2-}"
    local help="${3-}"
    local user_input="${4-}"

    if [[ "$user_input" == "--help" || "$user_input" == "-h" ]]; then
        echo -e "\033[38;5;214m${program}\033[0m"
        echo -e "\033[1;97mUsage:\033[0m \033[1;96m${usage}\033[0m"
        echo -e "\033[1;97m${help}\033[0m"
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
needs_confirm_prompt() {
    local prompt="${1-}"
    command printf '%b' "$prompt ${__GASH_BOLD_WHITE:=\e[1;37m}(y/N):${__GASH_COLOR_OFF:=\033[0m} "
    read -r REPLY < /dev/tty
    if [[ "$REPLY" =~ ^[Yy]$ ]]; then
        return 0
    fi
    return 1
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
    if [[ "${COLUMNS-}" =~ ^[0-9]+$ ]] && (( COLUMNS > 0 )); then
        printf '%s' "$COLUMNS"
        return 0
    fi

    # Try tput
    if command -v tput >/dev/null 2>&1; then
        w="$(tput cols 2>/dev/null || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && (( w > 0 )); then
            printf '%s' "$w"
            return 0
        fi
    fi

    # Try stty
    if command -v stty >/dev/null 2>&1; then
        w="$(stty size 2>/dev/null | awk '{print $2}' || true)"
        if [[ "$w" =~ ^[0-9]+$ ]] && (( w > 0 )); then
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

# -----------------------------------------------------------------------------
# Path Utilities
# -----------------------------------------------------------------------------

# Expand tilde (~) in path to $HOME.
# Usage: expanded=$(__gash_expand_tilde_path "~/file")
__gash_expand_tilde_path() {
    local p="${1-}"
    if [[ "$p" == "~/"* ]]; then
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

    for i in {0..255}; do
        printf "\x1b[38;5;%smcolour%s\x1b[0m\n" "$i" "$i"
    done
}
