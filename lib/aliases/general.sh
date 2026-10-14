#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: General
# Miscellaneous aliases and Gash shortcuts.

# `help` shows Gash help for Gash commands and falls back to the Bash
# builtin for everything else (`help cd`, `help -d read`, ...).
function help {
    if [[ $# -eq 0 ]]; then
        gash_help
    elif [[ "$1" == -* ]] || [[ "$(type -t "$1" 2>/dev/null)" == "builtin" || "$(type -t "$1" 2>/dev/null)" == "keyword" ]]; then
        builtin help "$@"
    else
        gash_help "$@"
    fi
}

# Stop common local services (web servers, databases) quickly.
alias quit='services_stop'

# -----------------------------------------------------------------------------
# Tool replacements (opt-in: GASH_REPLACE_TOOLS=1 in ~/.gash_env)
#
# These swap standard commands for alternatives with different flags and
# output, which surprises scripts and muscle memory, so Gash 2 leaves them
# off unless asked.
# -----------------------------------------------------------------------------

if [[ "${GASH_REPLACE_TOOLS:-0}" == "1" ]]; then
    __gash_add_command_replace_alias() {
        if type -P "$2" >/dev/null 2>&1; then
            alias "$1"="$2"
        fi
    }

    if type -P most >/dev/null 2>&1; then
        alias less='most'
        export PAGER='most'
    fi
    __gash_add_command_replace_alias 'tail' 'multitail'
    __gash_add_command_replace_alias 'df' 'pydf'
    __gash_add_command_replace_alias 'top' 'htop'
    __gash_add_command_replace_alias 'traceroute' 'mtr'
    __gash_add_command_replace_alias 'tracepath' 'mtr'
    __gash_add_command_replace_alias 'diff' 'colordiff'

    unset -f __gash_add_command_replace_alias
fi
