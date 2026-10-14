#!/usr/bin/env bash

# Gash Shell: startup banner and session extras (interactive shells only).
#
# GASH_BANNER=0     no banner and no quote at startup
# GASH_SHOW_INXI=1  print a short `inxi` system summary at startup

# Refuse `write`/`talk` messages from other users on this terminal (`mesg`
# acts on the terminal on standard input: without one there is nothing to do).
if [[ -t 0 ]] && type -P mesg >/dev/null 2>&1; then
    mesg n 2>/dev/null || true
fi

if [[ "${GASH_SHOW_INXI-0}" == "1" ]] && type -P inxi >/dev/null 2>&1; then
    inxi -IpRS -v0 -c5
fi

if [[ "${GASH_BANNER:-1}" == "1" ]]; then
    printf '%b\n' "${__GASH_CYAN-}Gash ${GASH_VERSION}${__GASH_COLOR_OFF-} ${__GASH_BOLD_WHITE-}—${__GASH_COLOR_OFF-} ${__GASH_BOLD_GREEN-}${USER:-$(gash_username)}${__GASH_COLOR_OFF-}"
    gash_inspiring_quote || true
fi

# Auto-unlock SSH keys configured in ~/.gash_env. With none configured
# (already known from loading ~/.gash_env), only its run-once marker is set.
if declare -F gash_ssh_auto_unlock >/dev/null 2>&1; then
    if [[ -z "${GASH_SSH_AUTOUNLOCK_RAN-}" && -n "${__GASH_ENV_LOADED-}" && -z "${__GASH_ENV_SSH_KEYS-}" ]]; then
        export GASH_SSH_AUTOUNLOCK_RAN=1
    else
        gash_ssh_auto_unlock
    fi
fi
