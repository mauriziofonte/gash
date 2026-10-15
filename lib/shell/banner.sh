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

# "Gash VERSION — USER": the product in the primary color, the user as the
# accent, colors only where output.sh allows them.
__gash_banner_line() {
    local P="" M="" A="" R=""
    if __gash_use_color; then
        P="$__GASH_COLOR_PRIMARY" M="$__GASH_COLOR_MUTED" A="$__GASH_COLOR_ACCENT" R="$__GASH_COLOR_OFF"
    fi
    printf '%sGash %s%s %s—%s %s%s%s\n' "$P" "$GASH_VERSION" "$R" "$M" "$R" "$A" "${USER:-$(gash_username)}" "$R"
}

if [[ "${GASH_BANNER:-1}" == "1" ]]; then
    __gash_banner_line
    gash_inspiring_quote || true
fi
unset -f __gash_banner_line

# Auto-unlock SSH keys configured in ~/.gash_env. With none configured
# (already known from loading ~/.gash_env), only its run-once marker is set.
if declare -F gash_ssh_auto_unlock >/dev/null 2>&1; then
    if [[ -z "${GASH_SSH_AUTOUNLOCK_RAN-}" && -n "${__GASH_ENV_LOADED-}" && -z "${__GASH_ENV_SSH_KEYS-}" ]]; then
        export GASH_SSH_AUTOUNLOCK_RAN=1
    else
        gash_ssh_auto_unlock
    fi
fi
