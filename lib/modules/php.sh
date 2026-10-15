#!/usr/bin/env bash

# Gash Module: PHP Version Detection
# Detects which PHP version a project wants (project-local, not system-wide).
# Independent of lib/aliases/php.sh, which generates interactive-only phpXY/
# composerXY aliases for whatever system PHP binaries exist and is skipped
# entirely under GASH_HEADLESS (lib/aliases/_loader.sh).
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   php_detect_version         - Detect the PHP version a project wants

# -----------------------------------------------------------------------------
# Version Detection
# -----------------------------------------------------------------------------

# Priority: .target-php-version (walk-up parent dirs) -> composer.json
# .require.php -> .github/copilot-instructions.md -> fallback 8.4. Permissive
# extraction (accepts 8.2, 8.2.*, ^8.1, >=8.0, etc.), returns MAJOR.MINOR.
__php_detect_version_string() {
    local project_dir="${1:-.}"

    local search_dir
    search_dir=$(cd "$project_dir" 2>/dev/null && pwd || echo "$project_dir")
    # Walk up only with an absolute path, and stop once dirname stops
    # changing, so a stray relative arg can't loop forever.
    while [[ "$search_dir" == /* && "$search_dir" != "/" ]]; do
        if [[ -f "$search_dir/.target-php-version" ]]; then
            local version
            version=$(head -1 "$search_dir/.target-php-version" | grep -oE '[0-9]+\.[0-9]+' || echo "")
            if [[ -n "$version" ]]; then
                echo "$version"
                return 0
            fi
        fi
        local parent
        parent=$(dirname "$search_dir")
        [[ "$parent" == "$search_dir" ]] && break
        search_dir="$parent"
    done

    if [[ -f "$project_dir/composer.json" ]]; then
        local php_req
        php_req=$(jq -r '.require.php // empty' "$project_dir/composer.json" 2>/dev/null || echo "")
        if [[ -n "$php_req" ]]; then
            local version
            version=$(echo "$php_req" | grep -oE '[0-9]+\.[0-9]+' | head -1)
            if [[ -n "$version" ]]; then
                echo "$version"
                return 0
            fi
        fi
    fi

    if [[ -f "$project_dir/.github/copilot-instructions.md" ]]; then
        local version
        version=$(grep -oP '(?i)(?<=php[[:space:]:]{0,3})[0-9]+\.[0-9]+' \
            "$project_dir/.github/copilot-instructions.md" 2>/dev/null | head -1 || echo "")
        if [[ -n "$version" ]]; then
            echo "$version"
            return 0
        fi
    fi

    echo "8.4"
}

# Detect the PHP version a project wants, and resolve the matching binary /
# gash-exec function names. JSON output under --json or GASH_HEADLESS=1,
# human-readable text otherwise.
# Usage: php_detect_version [project_dir] [--json]
php_detect_version() {
    needs_help "php_detect_version" "php_detect_version [project_dir] [--json]" \
        "Detect which PHP version a project wants (.target-php-version, composer.json, repo instructions) and resolve the matching binary/gash-exec function names." \
        "${1-}" && return

    local project_dir="." llm_mode=0
    local arg
    for arg in "$@"; do
        case "$arg" in
            --json | --llm) llm_mode=1 ;; # --llm: 1.x spelling, kept as alias
            -*)
                __gash_error "Unknown option: $arg"
                return 1
                ;;
            *) project_dir="$arg" ;;
        esac
    done

    local version
    version="$(__php_detect_version_string "$project_dir")"

    local major="${version%%.*}"
    local php_raw_binary=""
    if [[ -x "/usr/bin/php${version}" ]]; then
        php_raw_binary="/usr/bin/php${version}"
    elif [[ -x "/usr/bin/php${major}" ]]; then
        php_raw_binary="/usr/bin/php${major}"
    else
        php_raw_binary="$(type -P php || echo php)"
    fi

    local vernodot="${version//./}"
    local php_command="php${vernodot}"

    local composer_raw_binary=""
    local composer_path
    composer_path="$(type -P composer 2>/dev/null || true)"
    if [[ -n "$composer_path" ]]; then
        composer_raw_binary="$composer_path"
    elif [[ -x "/usr/local/bin/composer" ]]; then
        composer_raw_binary="/usr/local/bin/composer"
    elif [[ -x "/usr/bin/composer" ]]; then
        composer_raw_binary="/usr/bin/composer"
    fi

    local composer_command="composer${vernodot}"
    [[ -z "$composer_raw_binary" ]] && composer_command="composer"

    if [[ $llm_mode -eq 1 ]] || __gash_headless_gate_active; then
        printf '{"version":"%s","php_raw_binary":"%s","php_command":"%s","composer_raw_binary":"%s","composer_command":"%s"}\n' \
            "$(__gash_json_escape "$version")" \
            "$(__gash_json_escape "$php_raw_binary")" \
            "$(__gash_json_escape "$php_command")" \
            "$(__gash_json_escape "$composer_raw_binary")" \
            "$(__gash_json_escape "$composer_command")"
    else
        __gash_info "PHP version for '$project_dir': $version"
        __gash_kv --indent 2 --width 20 --value-color accent \
            "php_raw_binary" "$php_raw_binary" \
            "php_command" "$php_command" \
            "composer_raw_binary" "${composer_raw_binary:-<none>}" \
            "composer_command" "$composer_command"
    fi
}

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "php_detect_version" \
        --module "php" \
        --short "Detect the PHP version a project wants" \
        <<'HELP'
USAGE
  php_detect_version [project_dir] [--json]

EXAMPLES
  # Human-readable, current directory
  php_detect_version

  # JSON, specific project
  php_detect_version /path/to/project --json

NOTES
  Priority: .target-php-version (walk-up) -> composer.json .require.php ->
  .github/copilot-instructions.md -> fallback 8.4. JSON output is also
  emitted automatically under GASH_HEADLESS=1, --json is not required there.
HELP

fi # end help registration guard
