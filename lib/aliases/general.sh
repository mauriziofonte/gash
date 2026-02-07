#!/usr/bin/env bash

# Gash Aliases: General
# Miscellaneous aliases, command replacements, and Gash-specific shortcuts.

# Override help command with Gash help
alias help=gash_help

# Add alias for "services_stop" with --force flag
alias quit='services_stop --force'

# WSL specific aliases
if grep -qi "microsoft" /proc/version 2>/dev/null && [[ -n "${WSLENV-}" ]]; then
    alias wslrestart="history -a && cmd.exe /C wsl --shutdown"
    alias wslshutdown="history -a && cmd.exe /C wsl --shutdown"
    alias explorer="explorer.exe ."
    alias taskmanager="cd /mnt/c && cmd.exe /C taskmgr &"
fi

# Replace less with most if installed
if command -v most >/dev/null 2>&1; then
    alias less='most'
    export PAGER='most'
fi

# Set default editor
if command -v nano >/dev/null 2>&1; then
    export EDITOR='nano'
elif command -v vim >/dev/null 2>&1; then
    export EDITOR='vim'
fi

# Helper function to replace commands with better alternatives
__gash_add_command_replace_alias() {
    if command -v "$2" >/dev/null 2>&1; then
        alias "$1"="$2"
    fi
}

# Replace commands with better alternatives if available
__gash_add_command_replace_alias 'tail' 'multitail'
__gash_add_command_replace_alias 'df' 'pydf'
__gash_add_command_replace_alias 'top' 'htop'

# Use mtr for traceroute and tracepath if installed
if command -v mtr >/dev/null 2>&1; then
    alias traceroute='mtr'
    alias tracepath='mtr'
fi

# Colorize output of diff if colordiff is installed
if command -v colordiff >/dev/null 2>&1; then
    alias diff='colordiff'
fi

# Cleanup helper function
unset -f __gash_add_command_replace_alias
