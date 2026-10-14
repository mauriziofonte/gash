#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

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

# -----------------------------------------------------------------------------
# Probe cache
#
# colors.sh, navigation.sh and safety.sh ask the system which options its
# tools take (`ls --color`, `grep --color`, GNU `rm`) and run `dircolors`, a
# program each, and this loader looks up every alias name on PATH (below).
# The answers, in the __GASH_PROBE_* variables, are kept in
# ${XDG_CACHE_HOME:-~/.cache}/gash/probes and reused while Gash's version,
# OSTYPE, TERM, COLORTERM, PATH and the dircolors database stay the same, no
# PATH directory and no dircolors database changed after the cache was
# written, and the cache is less than a day old. A probe the cache does not
# answer runs, and sets __GASH_PROBE_DIRTY so the cache is written again.
# Alias files loaded on their own (no GASH_VERSION) probe every time.
# -----------------------------------------------------------------------------

__GASH_PROBE_FIELDS="rm_gnu grep_color ls_flag ls_colors shadow_before shadow_after shadowed"

# Usage: __gash_probe_cache_key  (sets __GASH_PROBE_KEY and __GASH_PROBE_DEPS)
__gash_probe_cache_key() {
    # read -a, not `local -` and set -f (Bash 4.4/5.0 crash at exit).
    local dir
    local -a dirs=()
    __GASH_PROBE_KEY="2|${GASH_VERSION-}|${OSTYPE-}|${TERM-}|${COLORTERM-}|${PATH-}"
    __GASH_PROBE_DEPS=()
    IFS=: read -r -a dirs <<<"${PATH-}"
    for dir in ${dirs[@]+"${dirs[@]}"}; do
        [[ "$dir" == /* && -d "$dir" ]] && __GASH_PROBE_DEPS+=("$dir")
    done
    for dir in "${HOME-}/.dircolors" /etc/DIR_COLORS; do
        if [[ -r "$dir" ]]; then
            __GASH_PROBE_KEY+="|$dir"
            __GASH_PROBE_DEPS+=("$dir")
            break
        fi
    done
    return 0
}

# Read the cache into __GASH_PROBE_* when it is valid for this shell.
__gash_probe_cache_read() {
    local file="$1" line now created="" key="" dep
    [[ -r "$file" ]] || return 0
    for dep in ${__GASH_PROBE_DEPS[@]+"${__GASH_PROBE_DEPS[@]}"}; do
        [[ "$file" -nt "$dep" ]] || return 0
    done
    local -a values=()
    while IFS= read -r line; do
        case "$line" in
            key=*) key="${line#key=}" ;;
            created=*) created="${line#created=}" ;;
            rm_gnu=[01] | grep_color=[01] | ls_flag= | ls_flag=--color=auto | ls_flag=-G) values+=("$line") ;;
            ls_colors=* | shadow_before=* | shadow_after=*) values+=("$line") ;;
            shadowed=*)
                # Alias names only: letters, digits and . _ - (no regex:
                # BASH_REMATCH would be left behind)
                [[ "${line#shadowed=}" != *[!A-Za-z0-9._\ -]* ]] && values+=("$line")
                ;;
        esac
    done <"$file"
    [[ "$key" == "$__GASH_PROBE_KEY" && -n "$created" && "$created" != *[!0-9]* ]] || return 0
    printf -v now '%(%s)T' -1
    ((now >= created && now - created < 86400)) || return 0
    for line in ${values[@]+"${values[@]}"}; do
        case "$line" in
            rm_gnu=*) __GASH_PROBE_RM_GNU="${line#*=}" ;;
            grep_color=*) __GASH_PROBE_GREP_COLOR="${line#*=}" ;;
            ls_flag=*) __GASH_PROBE_LS_FLAG="${line#*=}" ;;
            ls_colors=*) __GASH_PROBE_LS_COLORS="${line#*=}" ;;
            shadow_before=*) __GASH_PROBE_SHADOW_BEFORE="${line#*=}" ;;
            shadow_after=*) __GASH_PROBE_SHADOW_AFTER="${line#*=}" ;;
            shadowed=*) __GASH_PROBE_SHADOWED="${line#*=}" ;;
        esac
    done
    return 0
}

# Write the cache (when a probe ran): aside, then renamed over the old one.
__gash_probe_cache_write() {
    local file="$1" now name var
    [[ -d "${file%/*}" ]] || command mkdir -p "${file%/*}" 2>/dev/null || return 0
    printf -v now '%(%s)T' -1
    if {
        printf 'key=%s\ncreated=%s\n' "$__GASH_PROBE_KEY" "$now"
        for name in $__GASH_PROBE_FIELDS; do
            var="__GASH_PROBE_${name^^}"
            [[ -n "${!var+x}" ]] && printf '%s=%s\n' "$name" "${!var}"
        done
    } >"$file.$$" 2>/dev/null; then
        command mv -f "$file.$$" "$file" 2>/dev/null || command rm -f "$file.$$" 2>/dev/null
    fi
    return 0
}

# Gash aliases whose name is also a program on PATH, into
# __GASH_PROBE_SHADOWED. Not counted: aliases that existed before (BEFORE,
# the alias names then) and the ones that wrap the program of the same name
# on purpose (ls, rm, grep, the GASH_REPLACE_TOOLS ones...).
# Usage: __gash_probe_shadowed BEFORE
__gash_probe_shadowed() {
    local name
    local -a names=()
    local -A before=()
    IFS=$' \t\n' read -r -a names <<<"$1"
    for name in ${names[@]+"${names[@]}"}; do
        before["$name"]=1
    done
    __GASH_PROBE_SHADOWED=""
    for name in "${!BASH_ALIASES[@]}"; do
        [[ -n "${before["$name"]+x}" ]] && continue
        case "$name" in
            ls | dir | vdir | sl | cp | mv | rm | mkdir | chown | chmod | chgrp | grep | bc | reboot | shutdown) continue ;;
            less | tail | df | top | traceroute | tracepath | diff | php | composer) continue ;;
            php[0-9]* | composer[0-9]* | 1composer[0-9]*) continue ;;
        esac
        type -P "$name" >/dev/null 2>&1 && __GASH_PROBE_SHADOWED+="${__GASH_PROBE_SHADOWED:+ }$name"
    done
    return 0
}

__gash_probe_cache_file=""
if [[ -n "${GASH_VERSION-}" ]]; then
    if [[ "${XDG_CACHE_HOME-}" == /* ]]; then
        __gash_probe_cache_file="$XDG_CACHE_HOME/gash/probes"
    elif [[ -n "${HOME-}" ]]; then
        __gash_probe_cache_file="$HOME/.cache/gash/probes"
    fi
fi
if [[ -n "$__gash_probe_cache_file" ]]; then
    __gash_probe_cache_key
    __gash_probe_cache_read "$__gash_probe_cache_file"
fi

__gash_alias_before="${!BASH_ALIASES[*]}"

for __gash_alias_file in "${__gash_alias_files[@]}"; do
    if [[ -f "$__GASH_ALIASES_DIR/${__gash_alias_file}.sh" ]]; then
        ((${__GASH_TIMING:-0})) && __gash_alias_t0="$EPOCHREALTIME"
        source "$__GASH_ALIASES_DIR/${__gash_alias_file}.sh"
        ((${__GASH_TIMING:-0})) && __gash_time_record "lib/aliases/${__gash_alias_file}.sh" "$__gash_alias_t0"
    fi
done

# A Gash alias never hides a program: one whose name is also a program on
# PATH (gp: PARI/GP, gc: Graphviz, dc: the calculator) is dropped. The
# answer depends on the aliases before and after loading (and on PATH).
if [[ -z "${__GASH_PROBE_SHADOWED+x}" || "${__GASH_PROBE_SHADOW_BEFORE-}" != "$__gash_alias_before" ||
    "${__GASH_PROBE_SHADOW_AFTER-}" != "${!BASH_ALIASES[*]}" ]]; then
    __gash_probe_shadowed "$__gash_alias_before"
    __GASH_PROBE_SHADOW_BEFORE="$__gash_alias_before"
    __GASH_PROBE_SHADOW_AFTER="${!BASH_ALIASES[*]}"
    __GASH_PROBE_DIRTY=1
fi
if [[ -n "${__GASH_PROBE_SHADOWED-}" ]]; then
    read -r -a __gash_alias_files <<<"$__GASH_PROBE_SHADOWED"
    for __gash_alias_file in "${__gash_alias_files[@]}"; do
        unalias "$__gash_alias_file" 2>/dev/null || true
    done
fi

if [[ -n "$__gash_probe_cache_file" && -n "${__GASH_PROBE_DIRTY-}" ]]; then
    __gash_probe_cache_write "$__gash_probe_cache_file"
fi

unset -f __gash_probe_cache_key __gash_probe_cache_read __gash_probe_cache_write __gash_probe_shadowed
unset __gash_alias_file __gash_alias_files __GASH_ALIASES_DIR __gash_alias_t0 __gash_probe_cache_file __gash_alias_before
unset __GASH_PROBE_FIELDS __GASH_PROBE_KEY __GASH_PROBE_DEPS __GASH_PROBE_DIRTY
unset __GASH_PROBE_RM_GNU __GASH_PROBE_GREP_COLOR __GASH_PROBE_LS_FLAG __GASH_PROBE_LS_COLORS
unset __GASH_PROBE_SHADOW_BEFORE __GASH_PROBE_SHADOW_AFTER __GASH_PROBE_SHADOWED
