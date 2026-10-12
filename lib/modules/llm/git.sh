#!/usr/bin/env bash

# Gash Module: LLM Utilities: git
# llm_git_status, llm_git_diff, llm_git_log.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# Git Functions
# -----------------------------------------------------------------------------

# Compact git status as JSON.
# Usage: llm_git_status [PATH]
llm_git_status() {
    if needs_help "llm_git_status" "llm_git_status [PATH]" "Compact git status (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_status_impl "$@"
}

__llm_git_status_impl() {
    local target_path="${1-.}"

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    if ! git -C "$safe_path" rev-parse --git-dir >/dev/null 2>&1; then
        __llm_error "not_a_git_repo" "$safe_path is not a git repository" "CONTINUE" "false" "Not inside a git repository. Skip git operations"
        return 1
    fi

    local branch
    branch="$(git -C "$safe_path" branch --show-current 2>/dev/null)"

    local ahead=0 behind=0
    local tracking
    tracking="$(git -C "$safe_path" rev-parse --abbrev-ref '@{upstream}' 2>/dev/null)"
    if [[ -n "$tracking" ]]; then
        ahead="$(git -C "$safe_path" rev-list --count '@{upstream}..HEAD' 2>/dev/null || echo 0)"
        behind="$(git -C "$safe_path" rev-list --count 'HEAD..@{upstream}' 2>/dev/null || echo 0)"
    fi

    local staged=() modified=() untracked=()

    # -z: NUL-delimited records, no core.quotePath mangling, rename entries
    # carry the new name as a separate NUL field (consumed and skipped).
    local line status file
    while IFS= read -r -d '' line; do
        status="${line:0:2}"
        file="${line:3}"

        case "$status" in
            "R "* | "C "*)
                # Rename/copy: this field is the NEW name; the next NUL
                # field is the old name - consume and discard it.
                staged+=("$file")
                IFS= read -r -d '' _ || true
                ;;
            "A "* | "M "* | "D "*) staged+=("$file") ;;
            " M" | " D") modified+=("$file") ;;
            "??") untracked+=("$file") ;;
            "AM" | "MM")
                staged+=("$file")
                modified+=("$file")
                ;;
        esac
    done < <(git -C "$safe_path" status --porcelain -z 2>/dev/null)

    # Build JSON: every value goes through jq-backed escaping; empty arrays
    # render as [] (never [""]).
    echo "{"
    printf '  "branch": %s,\n' "$(__gash_json_str "$branch")"
    echo "  \"ahead\": $ahead,"
    echo "  \"behind\": $behind,"
    printf '  "staged": %s,\n' "$(__gash_json_arr ${staged[@]+"${staged[@]}"})"
    printf '  "modified": %s,\n' "$(__gash_json_arr ${modified[@]+"${modified[@]}"})"
    printf '  "untracked": %s\n' "$(__gash_json_arr ${untracked[@]+"${untracked[@]}"})"
    echo "}"
}

# Git diff with stats.
# Usage: llm_git_diff [--staged] [PATH]
llm_git_diff() {
    if needs_help "llm_git_diff" "llm_git_diff [--staged] [PATH]" "Git diff with stats" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_diff_impl "$@"
}

__llm_git_diff_impl() {
    local staged_only=0
    local target_path="."

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --staged)
                staged_only=1
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

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    local diff_args=(--stat --no-color)
    [[ $staged_only -eq 1 ]] && diff_args+=(--staged)

    git -C "$safe_path" diff "${diff_args[@]}" 2>/dev/null
}

# Recent git log as JSON.
# Usage: llm_git_log [--limit N] [PATH]
llm_git_log() {
    if needs_help "llm_git_log" "llm_git_log [--limit N] [PATH]" "Recent git log (JSON)" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_git_log_impl "$@"
}

__llm_git_log_impl() {
    local limit=10
    local target_path="."

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --limit)
                limit="${2-10}"
                shift 2 || {
                    __llm_error "missing_value" "$1" "RETRY" "true" "$1 needs a value"
                    return 1
                }
                ;;
            *)
                target_path="$1"
                shift
                ;;
        esac
    done

    __llm_validate_uint "limit" "$limit" || return 1

    local safe_path
    safe_path="$(__llm_validate_path "$target_path")" || return 1

    if ! type -P git >/dev/null 2>&1; then
        __llm_error "git_not_found" "Git is not installed" "CONTINUE" "false" "Git binary not found. Skip git operations"
        return 1
    fi

    if ! __llm_has_jq; then
        __llm_error "jq_not_found" "jq is required for JSON output" "FATAL" "false" "Install jq: sudo apt install jq"
        return 1
    fi

    # Unit/record separators (US/RS) as field delimiters, JSON built by jq:
    # a quote in a commit subject must never invalidate the array.
    git -C "$safe_path" log -n "$limit" --format='%h%x1f%s%x1f%an%x1f%ci%x1e' 2>/dev/null |
        jq -cRs 'split("\u001e") | map(select(length > 0) | ltrimstr("\n") | split("\u001f") | select(length == 4) | {hash: .[0], subject: .[1], author: .[2], date: .[3]})' || true
    return 0
}
