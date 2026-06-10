#!/usr/bin/env bash

# Gash Aliases — legacy monolithic entry point (backward compatibility).
#
# All alias definitions live in lib/aliases/*.sh and are loaded through
# lib/aliases/_loader.sh. This file used to carry a stale duplicate of
# those definitions (with broken references such as `quit` pointing to a
# non-existent stop_services, and gt/gta/gtd mapping to functions that
# were never defined): it now simply delegates to the loader, so the two
# entry points can never diverge again.

__gash_aliases_compat_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -f "$__gash_aliases_compat_dir/aliases/_loader.sh" ]]; then
    if [[ -z "${GASH_DIR-}" ]]; then
        # The loader resolves files via ${GASH_DIR}/lib/aliases: when sourced
        # standalone (tests, manual use) derive it from our own location.
        GASH_DIR="$(dirname "$__gash_aliases_compat_dir")"
        source "$__gash_aliases_compat_dir/aliases/_loader.sh"
        unset GASH_DIR
    else
        source "$__gash_aliases_compat_dir/aliases/_loader.sh"
    fi
fi

unset __gash_aliases_compat_dir
