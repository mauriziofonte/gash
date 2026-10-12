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
    if needs_help "llm_project" "llm_project [PATH]" "Detect project type and info (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_project_impl "$@"
}

__llm_project_impl() {
    local target_path="${1-.}"

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

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
    if needs_help "llm_deps" "llm_deps [PATH] [--dev]" "List project dependencies (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_deps_impl "$@"
}

__llm_deps_impl() {
    local target_path="."
    local include_dev=0

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dev)
                include_dev=1
                shift
                ;;
            *)
                target_path="$1"
                shift
                ;;
        esac
    done

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if [[ -f "$safe_path/composer.json" ]]; then
        if __llm_has_jq; then
            if [[ $include_dev -eq 1 ]]; then
                jq '{manager:"composer",prod:.require|keys,dev:."require-dev"|keys}' "$safe_path/composer.json"
            else
                jq '{manager:"composer",dependencies:.require|keys}' "$safe_path/composer.json"
            fi
        else
            echo '{"manager":"composer","note":"install jq for detailed output"}'
        fi

    elif [[ -f "$safe_path/package.json" ]]; then
        if __llm_has_jq; then
            if [[ $include_dev -eq 1 ]]; then
                jq '{manager:"npm",prod:.dependencies|keys,dev:.devDependencies|keys}' "$safe_path/package.json"
            else
                jq '{manager:"npm",dependencies:.dependencies|keys}' "$safe_path/package.json"
            fi
        else
            echo '{"manager":"npm","note":"install jq for detailed output"}'
        fi

    elif [[ -f "$safe_path/requirements.txt" ]]; then
        echo '{"manager":"pip","dependencies":['
        grep -v '^#' "$safe_path/requirements.txt" | grep -v '^$' |
            sed 's/[<>=].*//' |
            awk '{printf "\"%s\",", $1}' | sed 's/,$//' || true
        echo ']}'
    else
        __llm_error "no_package_file" "No composer.json, package.json, or requirements.txt found" "CONTINUE" "false" "Not a package-managed project"
        return 1
    fi
}
