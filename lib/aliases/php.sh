#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: PHP & Composer
# PHP version-specific and Composer aliases.

# Helper function to add alias if binary exists
# Replaces __CMD_BINARY placeholder with actual binary path
__gash_add_php_alias() {
    local binary_name="$1"
    local alias_name="$2"
    local alias_command="$3"

    local BINARY
    BINARY=$(type -P "$binary_name" 2>/dev/null) || true

    if [[ -n "$BINARY" ]]; then
        alias "$alias_name"="${alias_command//__CMD_BINARY/$BINARY}"
    fi
}

if command -v php >/dev/null 2>&1; then
    # Detect the latest versioned PHP binary on PATH (php8.5, php8.4, ...)
    PHP_LV_BIN=""
    for version in 8.5 8.4 8.3 8.2 8.1 8.0 7.4 7.3 7.2 7.1 7.0 5.6; do
        PHP_LV_BIN=$(type -P "php$version" 2>/dev/null) && break
    done

    # PHP version-specific aliases
    for version in 8.5 8.4 8.3 8.2 8.1 8.0 7.4 7.3 7.2 7.1 7.0 5.6; do
        version_alias=${version//./}
        __gash_add_php_alias "php$version" "php$version_alias" "__CMD_BINARY -d allow_url_fopen=1 -d memory_limit=2048M"
    done

    # Default PHP version alias
    if [[ -n "$PHP_LV_BIN" ]]; then
        alias php="$PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M"
    fi

    # Composer aliases (using Composer 2)
    if type -P composer >/dev/null 2>&1; then
        COMPOSER_BINARY=$(type -P composer)

        for version in 8.5 8.4 8.3 8.2 8.1 8.0 7.4 7.3 7.2 7.1 7.0 5.6; do
            version_alias=${version//./}
            __gash_add_php_alias "php$version" "composer$version_alias" "__CMD_BINARY -d allow_url_fopen=1 -d memory_limit=2048M $COMPOSER_BINARY"
        done

        # Default composer on latest PHP
        if [[ -n "$PHP_LV_BIN" ]]; then
            alias composer="$PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M $COMPOSER_BINARY"
        fi

        # Composer global update
        alias composer-packages-update="$COMPOSER_BINARY global update"

        # Composer self-update
        alias composer-self-update="sudo $COMPOSER_BINARY self-update"
    fi

    # Composer 1 aliases (legacy support)
    if type -P composer1 >/dev/null 2>&1; then
        COMPOSER1_BINARY=$(type -P composer1)

        for version in 7.2 7.1 7.0 5.6; do
            version_alias=${version//./}
            __gash_add_php_alias "php$version" "1composer$version_alias" "__CMD_BINARY -d allow_url_fopen=1 -d memory_limit=2048M $COMPOSER1_BINARY"
        done

        # Composer 1 self-update
        alias composer1-self-update="sudo $COMPOSER1_BINARY self-update"
    fi

    # hte-cli aliases (if installed via Composer global)
    if [[ -f ~/.config/composer/vendor/bin/hte-cli && -n "$PHP_LV_BIN" ]]; then
        alias hte="sudo $PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M ~/.config/composer/vendor/bin/hte-cli create"
        alias hte-create="sudo $PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M ~/.config/composer/vendor/bin/hte-cli create"
        alias hte-remove="sudo $PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M ~/.config/composer/vendor/bin/hte-cli remove"
        alias hte-details="sudo $PHP_LV_BIN -d allow_url_fopen=1 -d memory_limit=2048M ~/.config/composer/vendor/bin/hte-cli details"
    fi

    # Cleanup temporary variables
    unset PHP_LV_BIN COMPOSER_BINARY version COMPOSER1_BINARY version_alias
fi

# Cleanup helper function
unset -f __gash_add_php_alias
