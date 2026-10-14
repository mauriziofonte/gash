#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: PHP & Composer
#
# For every installed phpX.Y: phpXY and composerXY (e.g. php84, composer84)
# run that version with allow_url_fopen=1 and memory_limit=2048M.
#
# With GASH_REPLACE_TOOLS=1, `php` and `composer` themselves are also
# pointed at the newest installed phpX.Y with the same settings.

if type -P php >/dev/null 2>&1; then
    __gash_php_opts="-d allow_url_fopen=1 -d memory_limit=2048M"
    __gash_php_latest=""
    __gash_composer=""
    __gash_composer1=""
    type -P composer >/dev/null 2>&1 && __gash_composer="$(type -P composer)"
    type -P composer1 >/dev/null 2>&1 && __gash_composer1="$(type -P composer1)"

    # Newest first, so the first one found is the latest.
    for __gash_v in 8.5 8.4 8.3 8.2 8.1 8.0 7.4 7.3 7.2 7.1 7.0 5.6; do
        type -P "php$__gash_v" >/dev/null 2>&1 || continue
        __gash_bin="$(type -P "php$__gash_v")"
        [[ -z "$__gash_php_latest" ]] && __gash_php_latest="$__gash_bin"
        alias "php${__gash_v//./}"="$__gash_bin $__gash_php_opts"
        [[ -n "$__gash_composer" ]] &&
            alias "composer${__gash_v//./}"="$__gash_bin $__gash_php_opts $__gash_composer"
        case "$__gash_v" in
            7.2 | 7.1 | 7.0 | 5.6)
                [[ -n "$__gash_composer1" ]] &&
                    alias "1composer${__gash_v//./}"="$__gash_bin $__gash_php_opts $__gash_composer1"
                ;;
        esac
    done

    if [[ "${GASH_REPLACE_TOOLS:-0}" == "1" && -n "$__gash_php_latest" ]]; then
        alias php="$__gash_php_latest $__gash_php_opts"
        [[ -n "$__gash_composer" ]] && alias composer="$__gash_php_latest $__gash_php_opts $__gash_composer"
    fi

    if [[ -n "$__gash_composer" ]]; then
        alias composer-packages-update="$__gash_composer global update"
        alias composer-self-update="sudo $__gash_composer self-update"
    fi
    [[ -n "$__gash_composer1" ]] && alias composer1-self-update="sudo $__gash_composer1 self-update"

    # hte-cli (installed via Composer global)
    if [[ -f ~/.config/composer/vendor/bin/hte-cli && -n "$__gash_php_latest" ]]; then
        __gash_hte="sudo $__gash_php_latest $__gash_php_opts $HOME/.config/composer/vendor/bin/hte-cli"
        alias hte="$__gash_hte create"
        alias hte-create="$__gash_hte create"
        alias hte-remove="$__gash_hte remove"
        alias hte-details="$__gash_hte details"
    fi

    unset __gash_php_opts __gash_php_latest __gash_composer __gash_composer1 __gash_v __gash_bin __gash_hte
fi
