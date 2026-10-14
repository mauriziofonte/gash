#!/usr/bin/env bash
#
#   ____           _
#  / ___| __ _ ___| |__
# | |  _ / _` / __| '_ \
# | |_| | (_| \__ \ | | |
#  \____|\__,_|___/_| |_|
#
# Gash — a Bash configuration framework for humans and LLM agents.
#
# Entry point, sourced from ~/.gashrc. Two modes:
#
#   interactive  prompt, history policy, aliases, banner, completion and
#                every function module (the usual shell experience)
#   headless     GASH_HEADLESS=1: function modules only, no output, no
#                prompt, no aliases, no user files — for scripts and agents
#
# https://github.com/mauriziofonte/gash · MIT License · (c) Maurizio Fonte

# -----------------------------------------------------------------------------
# Requirements
# -----------------------------------------------------------------------------

# Bash 4.4 or newer (associative arrays, namerefs, ${var,,}). Written in
# plain sh syntax so that older shells reach the message instead of a parse
# error.
if [ -z "${BASH_VERSINFO-}" ] || [ "${BASH_VERSINFO[0]}" -lt 4 ] ||
    { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -lt 4 ]; }; then
    echo "Gash requires Bash 4.4 or newer (this is ${BASH_VERSION:-not Bash})." >&2
    echo "On macOS: brew install bash, then use $(brew --prefix 2>/dev/null || echo /opt/homebrew)/bin/bash." >&2
    return 1 2>/dev/null || exit 1
fi

# Only interactive shells get Gash, unless headless mode is requested.
if [[ $- != *i* && "${GASH_HEADLESS-}" != "1" ]]; then
    return 0
fi

# -----------------------------------------------------------------------------
# Location and version
# -----------------------------------------------------------------------------

# GASH_DIR is wherever this file lives, so custom install locations and
# symlinked checkouts work without configuration.
__gash_self="${BASH_SOURCE[0]}"
[[ "$__gash_self" == */* ]] || __gash_self="./$__gash_self"
GASH_DIR="${__gash_self%/*}"
[[ "$GASH_DIR" == /* ]] || GASH_DIR="$PWD/${GASH_DIR#./}"
GASH_DIR="${GASH_DIR%/.}"
unset __gash_self

# GASH_VERSION is public: read by modules, scripts and agents.
# shellcheck disable=SC2034
GASH_VERSION="unknown"
# shellcheck disable=SC2034
if [[ -r "$GASH_DIR/VERSION" ]]; then
    IFS= read -r GASH_VERSION <"$GASH_DIR/VERSION" || true
fi

# -----------------------------------------------------------------------------
# Loader
# -----------------------------------------------------------------------------

# Files that failed to load, reported by `gash_doctor`. Headless mode keeps
# stderr quiet (GASH_DEBUG=1 shows it) but never loses the failure itself.
__GASH_LOAD_ERRORS=()

__gash_source() {
    local file="$1" rc=0 aliases_on=0
    if [[ ! -r "$file" ]]; then
        __GASH_LOAD_ERRORS+=("${file#"$GASH_DIR"/}: missing")
        return 1
    fi
    # Aliases expand when a function body is parsed: on a re-source in an
    # interactive shell, `cp` would be baked in as the user's `cp -iv`.
    shopt -q expand_aliases && aliases_on=1
    shopt -u expand_aliases
    if [[ "${GASH_HEADLESS-}" == "1" && "${GASH_DEBUG-}" != "1" ]]; then
        source "$file" 2>/dev/null || rc=$?
    else
        source "$file" || rc=$?
    fi
    ((aliases_on)) && shopt -s expand_aliases
    if ((rc != 0)); then
        __GASH_LOAD_ERRORS+=("${file#"$GASH_DIR"/}: exit $rc")
    fi
    return 0
}

# Core first, in dependency order (not alphabetical): output has no
# dependencies, help must exist before anything registers help, config
# uses output and utils.
__gash_load_core() {
    local name
    for name in output help utils validation config; do
        __gash_source "$GASH_DIR/lib/core/$name.sh"
    done
}

# Function modules are independent of each other; load them all.
__gash_load_modules() {
    local file
    for file in "$GASH_DIR"/lib/modules/*.sh; do
        [[ -f "$file" ]] && __gash_source "$file"
    done
}

# -----------------------------------------------------------------------------
# Headless mode
# -----------------------------------------------------------------------------

if [[ "${GASH_HEADLESS-}" == "1" ]]; then
    __gash_load_core
    __gash_load_modules
    declare -F __gash_migrate_autostart >/dev/null && __gash_migrate_autostart
    return 0
fi

# -----------------------------------------------------------------------------
# Interactive mode
# -----------------------------------------------------------------------------

# Snapshot the state Gash is about to change, for `gash_unload`.
if [[ -z "${__GASH_SNAPSHOT_TAKEN-}" ]]; then
    __GASH_SNAPSHOT_TAKEN=1
    __GASH_SNAPSHOT=()
    for __gash_var in PS1 PS2 HISTCONTROL HISTTIMEFORMAT HISTSIZE HISTFILESIZE EDITOR; do
        if [[ -n "${!__gash_var+x}" ]]; then
            __GASH_SNAPSHOT+=("$__gash_var=${!__gash_var}")
        else
            __GASH_SNAPSHOT+=("$__gash_var")
        fi
    done
    shopt -q histappend && __GASH_SNAPSHOT+=("shopt:histappend=1") || __GASH_SNAPSHOT+=("shopt:histappend=0")
    shopt -q checkwinsize && __GASH_SNAPSHOT+=("shopt:checkwinsize=1") || __GASH_SNAPSHOT+=("shopt:checkwinsize=0")
    __GASH_SNAPSHOT+=("umask=$(umask)")
    __GASH_PRE_FUNCS=$'\n'"$(compgen -A function || true)"$'\n'
    __GASH_PRE_ALIASES=$'\n'"$(compgen -a || true)"$'\n'
    unset __gash_var
fi

__gash_load_core

# Settings from ~/.gash_env (GASH_PROMPT, GASH_HISTORY, ...). Variables
# already set in the environment take precedence.
declare -f __gash_apply_settings >/dev/null && __gash_apply_settings

__gash_source "$GASH_DIR/lib/shell/terminal.sh"
__gash_source "$GASH_DIR/lib/shell/history.sh"
__gash_source "$GASH_DIR/lib/shell/prompt.sh"

__gash_load_modules

# Aliases and user files are sourced at top level (not inside a function),
# so `declare` and `local`-free code in them behaves exactly as in .bashrc.
if [[ -f "$GASH_DIR/lib/aliases/_loader.sh" ]]; then
    __GASH_ALIASES_ON=0
    shopt -q expand_aliases && __GASH_ALIASES_ON=1
    shopt -u expand_aliases
    source "$GASH_DIR/lib/aliases/_loader.sh" || __GASH_LOAD_ERRORS+=("lib/aliases/_loader.sh: exit $?")
    ((__GASH_ALIASES_ON)) && shopt -s expand_aliases
    unset __GASH_ALIASES_ON
fi

__gash_source "$GASH_DIR/lib/shell/banner.sh"

# Record what exists after loading; `gash_unload` removes what is here but
# was not in the snapshot taken before (the diff is computed only then).
if [[ -z "${__GASH_POST_FUNCS-}" ]]; then
    __GASH_POST_FUNCS=$'\n'"$(compgen -A function || true)"$'\n'
    __GASH_POST_ALIASES=$'\n'"$(compgen -a || true)"$'\n'
fi

# User customizations, last so they can override anything above.
[[ -f "$HOME/.bash_aliases" ]] && source "$HOME/.bash_aliases"
[[ -f "$HOME/.bash_local" ]] && source "$HOME/.bash_local"

# A Gash 1.x configuration: guided migration (interactive terminal) or a
# one-line reminder (anywhere else).
declare -F __gash_migrate_autostart >/dev/null && __gash_migrate_autostart

# Keep the file's status 0: a `set -e` caller sourcing ~/.gashrc must not
# die because the last optional file was missing.
true
