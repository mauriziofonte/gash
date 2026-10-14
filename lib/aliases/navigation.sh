#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: Navigation
# Directory navigation and listing aliases.

# Detect ls color flag based on platform (or take it from the probe cache)
if [[ -z "${__GASH_PROBE_LS_FLAG+x}" ]]; then
    __GASH_PROBE_LS_FLAG='' __GASH_PROBE_DIRTY=1
    if ls --color=auto . >/dev/null 2>&1; then
        __GASH_PROBE_LS_FLAG='--color=auto'
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        __GASH_PROBE_LS_FLAG='-G'
    fi
fi
__gash_ls_color_flag="$__GASH_PROBE_LS_FLAG"

# Directory listing aliases
if [[ -n "$__gash_ls_color_flag" ]]; then
    alias ls="ls $__gash_ls_color_flag"
    alias ll="ls -l $__gash_ls_color_flag"
    alias la="ls -la $__gash_ls_color_flag"
    alias l="ls -CF $__gash_ls_color_flag"
    alias lash="ls -lash $__gash_ls_color_flag"
    alias sl="ls $__gash_ls_color_flag" # Correct common typo

    if command -v dir >/dev/null 2>&1; then
        alias dir="dir $__gash_ls_color_flag"
    fi
    if command -v vdir >/dev/null 2>&1; then
        alias vdir="vdir $__gash_ls_color_flag"
    fi
else
    alias ll='ls -l'
    alias la='ls -la'
    alias l='ls -CF'
    alias lash='ls -lash'
    alias sl='ls' # Correct common typo
fi

unset __gash_ls_color_flag

# Directory navigation shortcuts
alias ..='cd ..'
alias ...='cd ../..'
alias cd..='cd ..'
alias ....='cd ../../..'
alias .....='cd ../../../..'
alias .4='cd ../../../..'
alias .5='cd ../../../../..'

# Clear screen
alias cls='clear'

# Show PATH entries one per line
alias path='echo -e ${PATH//:/\\n}'
