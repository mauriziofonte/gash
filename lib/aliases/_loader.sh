#!/usr/bin/env bash

# Gash Aliases Loader
# Loads all alias files from lib/aliases/ directory in the correct order.
#
# This file is sourced by gash.sh when NOT in GASH_HEADLESS mode.
# Load order matters: colors first (sets LS_COLORS), then navigation (uses colors)

__GASH_ALIASES_DIR="${GASH_DIR:-$HOME/.gash}/lib/aliases"

# Define load order explicitly for predictable behavior
__gash_alias_files=(
    "colors"
    "navigation"
    "safety"
    "git"
    "docker"
    "php"
    "general"
)

for __gash_alias_file in "${__gash_alias_files[@]}"; do
    if [[ -f "$__GASH_ALIASES_DIR/${__gash_alias_file}.sh" ]]; then
        source "$__GASH_ALIASES_DIR/${__gash_alias_file}.sh"
    fi
done

unset __gash_alias_file __gash_alias_files __GASH_ALIASES_DIR
