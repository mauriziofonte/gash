#!/usr/bin/env bash

# Gash Shell: toolchains on PATH and nvm (interactive shells only).
#
# GASH_PATH_ADD="DIR..."   Directories put at the front of PATH, in the
#                          order given, when they exist; ~ is expanded.
#                          e.g. "~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin"
# GASH_NVM=off (default)   Gash does not touch nvm (load it yourself).
# GASH_NVM=lazy            node, npm, npx and global packages of nvm's
#                          default version are on PATH at once; nvm.sh itself
#                          loads the first time `nvm` runs. Saves the
#                          0.2-0.5 s that sourcing nvm.sh adds to every shell.
# GASH_NVM=on              Source nvm.sh and its completion now, as nvm's
#                          installer would.

if [[ -n "${GASH_PATH_ADD-}" ]]; then
    # shellcheck disable=SC2086 # word splitting is the format: "DIR DIR..."
    path_add ${GASH_PATH_ADD}
fi

# Directory of the version nvm's "default" alias points to, following
# aliases like "22", "lts/*", "lts/jod", "node" the way nvm resolves them
# against what is installed. Prints nothing when it cannot tell.
__gash_nvm_default_dir() {
    local nvm_dir="${NVM_DIR:-$HOME/.nvm}" name hops=0 d best=""
    [[ -r "$nvm_dir/alias/default" ]] || return 0
    IFS= read -r name <"$nvm_dir/alias/default" || [[ -n "$name" ]] || return 0
    # Aliases may point at other aliases (default -> lts/* -> lts/jod -> v22.x).
    while ((hops++ < 5)) && [[ -r "$nvm_dir/alias/$name" && "$name" != */../* ]]; do
        IFS= read -r name <"$nvm_dir/alias/$name" || break
    done
    case "$name" in
        node | stable) name="" ;;
        v[0-9]*) name="${name#v}" ;;
        [0-9]*) ;;
        *) return 0 ;;
    esac
    # Exact version, or the newest installed one matching a partial version.
    if [[ -n "$name" && -d "$nvm_dir/versions/node/v$name" && "$name" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        printf '%s\n' "$nvm_dir/versions/node/v$name"
        return 0
    fi
    for d in "$nvm_dir"/versions/node/v"${name}"*; do
        [[ -d "$d" ]] || continue
        [[ -z "$name" || "${d##*/v}" == "$name" || "${d##*/v}" == "$name".* ]] || continue
        if [[ -z "$best" ]] || [[ "$(printf '%s\n%s\n' "${best##*/v}" "${d##*/v}" | sort -V | tail -1)" == "${d##*/v}" ]]; then
            best="$d"
        fi
    done
    [[ -n "$best" ]] && printf '%s\n' "$best"
    return 0
}

# Source nvm.sh (and its completion) into this shell.
__gash_nvm_load() {
    local nvm_dir="${NVM_DIR:-$HOME/.nvm}"
    unset -f nvm
    [[ -s "$nvm_dir/nvm.sh" ]] || {
        __gash_error "nvm is not installed in $nvm_dir."
        return 1
    }
    # --no-use: the version on PATH stays the one already chosen.
    # shellcheck source=/dev/null
    source "$nvm_dir/nvm.sh" "$@"
    # shellcheck source=/dev/null
    [[ -r "$nvm_dir/bash_completion" ]] && source "$nvm_dir/bash_completion"
    return 0
}

case "${GASH_NVM:-off}" in
    on | 1)
        export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
        if [[ -s "$NVM_DIR/nvm.sh" ]] && ! declare -F nvm >/dev/null; then
            __gash_nvm_load
        fi
        ;;
    lazy)
        export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
        if [[ -s "$NVM_DIR/nvm.sh" ]] && ! declare -F nvm >/dev/null; then
            __gash_nvm_bin="$(__gash_nvm_default_dir)"
            [[ -n "$__gash_nvm_bin" ]] && path_add "$__gash_nvm_bin/bin"
            unset __gash_nvm_bin
            # The first `nvm` call replaces this stub with the real nvm.
            nvm() {
                __gash_nvm_load --no-use || return 1
                nvm "$@"
            }
        fi
        ;;
esac
