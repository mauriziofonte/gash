#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: Colors Configuration
# Sets LS_COLORS and related color settings for terminal output.

# Set LSCOLORS for macOS if using BSD ls
if [[ "$OSTYPE" == "darwin"* ]]; then
    export LSCOLORS='CxahafafBxagagabababab'
fi

# Set LESS_TERMCAP variables for syntax highlighting in less if supported
if [[ -n "${LESS-}" ]]; then
    export LESS_TERMCAP_mb=$'\E[01;31m'       # Begin blinking
    export LESS_TERMCAP_md=$'\E[01;38;5;74m'  # Begin bold
    export LESS_TERMCAP_me=$'\033[0m'         # End mode
    export LESS_TERMCAP_se=$'\033[0m'         # End standout-mode
    export LESS_TERMCAP_so=$'\E[38;5;246m'    # Begin standout-mode
    export LESS_TERMCAP_ue=$'\033[0m'         # End underline
    export LESS_TERMCAP_us=$'\E[04;38;5;146m' # Begin underline
fi

# LS_COLORS from dircolors (honors ~/.dircolors), unless already set. The
# __GASH_PROBE_* variables hold answers from the probe cache (_loader.sh).
if [[ -z "${LS_COLORS-}" ]]; then
    if [[ -n "${__GASH_PROBE_LS_COLORS+x}" ]]; then
        export LS_COLORS="$__GASH_PROBE_LS_COLORS"
    elif type -P dircolors >/dev/null 2>&1; then
        if [[ -r ~/.dircolors ]]; then
            eval "$(dircolors -b ~/.dircolors)"
        elif [[ -r /etc/DIR_COLORS ]]; then
            eval "$(dircolors -b /etc/DIR_COLORS)"
        else
            eval "$(dircolors -b)"
        fi
        [[ -n "${LS_COLORS+x}" ]] && __GASH_PROBE_LS_COLORS="$LS_COLORS" __GASH_PROBE_DIRTY=1
    fi
fi

# Color grep (egrep/fgrep are deprecated in GNU grep 3.8+; use -E/-F)
if [[ -z "${__GASH_PROBE_GREP_COLOR-}" ]]; then
    __GASH_PROBE_GREP_COLOR=0 __GASH_PROBE_DIRTY=1
    command grep --color=auto -q "" /dev/null >/dev/null 2>&1 && __GASH_PROBE_GREP_COLOR=1
fi
if [[ "$__GASH_PROBE_GREP_COLOR" == "1" ]]; then
    alias grep='grep --color=auto'
fi
