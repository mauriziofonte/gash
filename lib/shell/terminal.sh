#!/usr/bin/env bash

# Gash Shell: terminal settings (interactive shells only).

# Keep LINES/COLUMNS in sync after each command.
shopt -s checkwinsize

# Make `less` friendlier for non-text input files (only if the user or the
# distribution has not configured a preprocessor already). These are the
# two lines `eval "$(lesspipe)"` sets, without running it.
__gash_lesspipe_env() {
    local REPLY
    [[ -z "${LESSOPEN-}" ]] && __gash_path_of lesspipe || return 0
    [[ "$REPLY" == /* ]] || REPLY="$PWD/${REPLY#./}"
    export LESSOPEN="| $REPLY %s"
    export LESSCLOSE="$REPLY %s %s"
}
__gash_lesspipe_env
unset -f __gash_lesspipe_env

# Identify the chroot you work in (used by the prompt).
if [[ -z "${debian_chroot-}" && -r /etc/debian_chroot ]]; then
    debian_chroot=$(</etc/debian_chroot)
fi

# Default editor, only when the user has not chosen one.
if [[ -z "${EDITOR-}" ]]; then
    if type -P nano >/dev/null 2>&1; then
        export EDITOR='nano'
    elif type -P vim >/dev/null 2>&1; then
        export EDITOR='vim'
    fi
fi

# Default file-creation mask: untouched unless GASH_UMASK is set (Gash 1.x
# forced 022, which silently loosened hardened umasks like 077).
if [[ -n "${GASH_UMASK-}" ]]; then
    if [[ "$GASH_UMASK" =~ ^[0-7]{3,4}$ ]]; then
        # The mask before Gash, for `gash_unload` (recorded once).
        if [[ -n "${__GASH_SNAPSHOT_TAKEN-}" && " ${__GASH_SNAPSHOT[*]-} " != *" umask="* ]]; then
            __GASH_SNAPSHOT+=("umask=$(umask)")
        fi
        umask "$GASH_UMASK"
    else
        __gash_warning "GASH_UMASK must be an octal mask such as 022 or 077 (ignored)"
    fi
fi
