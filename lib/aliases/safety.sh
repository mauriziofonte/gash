#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: Safety
# Safer defaults for destructive file operations (GASH_SAFETY_ALIASES=0 to
# turn them off). GNU-only flags are added only when the tools support them.

if [[ "${GASH_SAFETY_ALIASES:-1}" == "1" ]]; then
    alias cp='cp -iv'
    alias mv='mv -iv'
    alias mkdir='mkdir -pv'

    if [[ "$(command rm --version 2>/dev/null)" == *GNU* ]]; then
        alias rm='rm -Iv --one-file-system --preserve-root'
        alias chown='chown --preserve-root'
        alias chmod='chmod --preserve-root'
        alias chgrp='chgrp --preserve-root'
    else
        alias rm='rm -iv'
    fi
fi

# Calculator with math library
alias bc='bc -l'

# Listening and connected sockets
if type -P ss >/dev/null 2>&1; then
    alias ports='ss -tulanp'
elif type -P netstat >/dev/null 2>&1; then
    alias ports='netstat -an'
fi

# Reboot and shutdown always go through sudo (no accidental remote reboot
# from an unprivileged shell that happens to have the binaries on PATH).
alias reboot='sudo /sbin/reboot'
alias shutdown='sudo /sbin/shutdown'
