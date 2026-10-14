#!/usr/bin/env bash

# Gash Module: LLM Utilities: project
# llm_project, llm_deps.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# Project Analysis
# -----------------------------------------------------------------------------

# Detect project type and return info as JSON.
# Usage: llm_project [PATH]
llm_project() {
    if __llm_needs_help "llm_project" "llm_project [PATH]" "Detect project type and info (JSON)" "$@"; then
        return 0
    fi

    __gash_no_history __llm_project_impl "$@"
}

__llm_project_impl() {
    local target_path
    __llm_path_arg target_path "Use a project directory or no argument" "$@" || return 1

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1
    if [[ ! -e "$safe_path" ]]; then
        __llm_error "path_not_found" "$safe_path" "RETRY" "true" "Provide an existing project directory"
        return 1
    fi
    if [[ ! -d "$safe_path" ]]; then
        __llm_error "not_a_directory" "$safe_path" "RETRY" "true" "Provide a project directory"
        return 1
    fi

    local project_type="unknown"
    local framework=""
    local has_tests="false"
    local entry_point=""
    local package_manager=""

    # Detect by config files
    if [[ -f "$safe_path/composer.json" ]]; then
        project_type="php"
        package_manager="composer"

        if [[ -f "$safe_path/artisan" ]]; then
            framework="laravel"
            entry_point="public/index.php"
        elif [[ -d "$safe_path/symfony" ]] || grep -q '"symfony/' "$safe_path/composer.json" 2>/dev/null; then
            framework="symfony"
            entry_point="public/index.php"
        fi

        [[ -d "$safe_path/tests" ]] && has_tests="true"

    elif [[ -f "$safe_path/package.json" ]]; then
        project_type="javascript"
        package_manager="npm"
        [[ -f "$safe_path/yarn.lock" ]] && package_manager="yarn"
        [[ -f "$safe_path/pnpm-lock.yaml" ]] && package_manager="pnpm"

        if grep -q '"next"' "$safe_path/package.json" 2>/dev/null; then
            framework="nextjs"
        elif grep -q '"react"' "$safe_path/package.json" 2>/dev/null; then
            framework="react"
        elif grep -q '"vue"' "$safe_path/package.json" 2>/dev/null; then
            framework="vue"
        elif grep -q '"express"' "$safe_path/package.json" 2>/dev/null; then
            framework="express"
        fi

        [[ -d "$safe_path/tests" ]] || [[ -d "$safe_path/__tests__" ]] && has_tests="true"

    elif [[ -f "$safe_path/requirements.txt" ]] || [[ -f "$safe_path/pyproject.toml" ]]; then
        project_type="python"
        package_manager="pip"
        [[ -f "$safe_path/pyproject.toml" ]] && package_manager="poetry"

        if [[ -f "$safe_path/manage.py" ]]; then
            framework="django"
        elif grep -q 'flask' "$safe_path/requirements.txt" 2>/dev/null; then
            framework="flask"
        fi

        [[ -d "$safe_path/tests" ]] && has_tests="true"

    elif [[ -f "$safe_path/go.mod" ]]; then
        project_type="go"
        package_manager="go"
        if [[ -d "$safe_path/tests" || -n "$(find "$safe_path" -name "*_test.go" -print -quit 2>/dev/null)" ]]; then
            has_tests="true"
        fi

    elif [[ -f "$safe_path/Cargo.toml" ]]; then
        project_type="rust"
        package_manager="cargo"
        [[ -d "$safe_path/tests" ]] && has_tests="true"
    fi

    # Output JSON (path is user-controlled: escape it)
    cat <<EOF
{
  "type": "$project_type",
  "framework": "$framework",
  "package_manager": "$package_manager",
  "has_tests": $has_tests,
  "entry_point": "$entry_point",
  "path": "$(__gash_json_escape "$safe_path")"
}
EOF
}

# List project dependencies.
# Usage: llm_deps [PATH] [--dev]
llm_deps() {
    if __llm_needs_help "llm_deps" "llm_deps [PATH] [--dev]" "List project dependencies (JSON)" "$@"; then
        return 0
    fi

    __gash_no_history __llm_deps_impl "$@"
}

__llm_deps_impl() {
    local target_path="." include_dev=0
    while (($# > 0)); do
        case "$1" in
            --dev) include_dev=1 ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --dev or a path"
                return 1
                ;;
            *) target_path="$1" ;;
        esac
        shift
    done

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! __llm_has_jq; then
        __llm_error "jq_not_found" "jq is required for JSON output" "FATAL" "false" "Install jq: sudo apt install jq"
        return 1
    fi

    # Output: {manager, dependencies} or, with --dev, {manager, prod, dev}.
    local shape='{manager: $m, dependencies: $prod}'
    ((include_dev)) && shape='{manager: $m, prod: $prod, dev: $dev}'

    if [[ -f "$safe_path/composer.json" ]]; then
        jq -c --arg m composer "(.require // {} | keys) as \$prod | (.\"require-dev\" // {} | keys) as \$dev | $shape" "$safe_path/composer.json"
    elif [[ -f "$safe_path/package.json" ]]; then
        jq -c --arg m npm "(.dependencies // {} | keys) as \$prod | (.devDependencies // {} | keys) as \$dev | $shape" "$safe_path/package.json"
    elif [[ -f "$safe_path/requirements.txt" ]]; then
        # Package name: up to the first version, extra or marker character.
        jq -cRn --arg m pip "[inputs | sub(\"#.*\"; \"\") | gsub(\"^\\\\s+|\\\\s+$\"; \"\") | select(length > 0 and (startswith(\"-\") | not)) | sub(\"[ <>=!~;\\\\[@].*\"; \"\")] as \$prod | [] as \$dev | $shape" <"$safe_path/requirements.txt"
    elif [[ -f "$safe_path/go.mod" ]]; then
        "${__GASH_AWK:-awk}" '
            /^require[[:space:]]*\(/ { block = 1; next }
            block && /^\)/ { block = 0; next }
            block && NF >= 2 { print $1 }
            /^require[[:space:]]+[^(]/ { print $2 }' "$safe_path/go.mod" |
            jq -cRn --arg m go "[inputs | select(length > 0)] as \$prod | [] as \$dev | $shape"
    else
        __llm_error "no_package_file" "No composer.json, package.json, requirements.txt or go.mod found" "CONTINUE" "true" "Not a package-managed project"
        return 1
    fi
}
