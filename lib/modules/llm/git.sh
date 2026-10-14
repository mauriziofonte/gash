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

    # One porcelain v2 call: branch, ahead/behind and every entry, NUL
    # separated so any file name survives. X (index) != "." means staged,
    # Y (worktree) != "." means modified; unmerged entries are conflicted.
    local branch="" ahead=0 behind=0
    local staged=() modified=() untracked=() conflicted=()
    local rec xy rest f
    while IFS= read -r -d '' rec; do
        case "$rec" in
            "# branch.head "*)
                branch="${rec#"# branch.head "}"
                [[ "$branch" == "(detached)" ]] && branch=""
                ;;
            "# branch.ab "*)
                rest="${rec#"# branch.ab +"}"
                ahead="${rest%% *}"
                behind="${rest##*-}"
                ;;
            "1 "* | "2 "*)
                xy="${rec:2:2}"
                # Path follows 8 fields (type 1) or 9 fields (type 2).
                rest="$rec"
                local skip=8
                [[ "$rec" == "2 "* ]] && skip=9
                for ((f = 0; f < skip; f++)); do rest="${rest#* }"; done
                [[ "${xy:0:1}" != "." ]] && staged+=("$rest")
                [[ "${xy:1:1}" != "." ]] && modified+=("$rest")
                # Renames/copies carry the original path as the next record.
                [[ "$rec" == "2 "* ]] && { IFS= read -r -d '' _ || true; }
                ;;
            "u "*)
                rest="$rec"
                for ((f = 0; f < 10; f++)); do rest="${rest#* }"; done
                conflicted+=("$rest")
                ;;
            "? "*) untracked+=("${rec#"? "}") ;;
        esac
    done < <(git -C "$safe_path" status --porcelain=v2 --branch -z 2>/dev/null)
    [[ "$ahead" =~ ^[0-9]+$ ]] || ahead=0
    [[ "$behind" =~ ^[0-9]+$ ]] || behind=0

    # Build JSON: every value goes through jq-backed escaping; empty arrays
    # render as [] (never [""]).
    echo "{"
    printf '  "branch": %s,\n' "$(__gash_json_str "$branch")"
    echo "  \"ahead\": $ahead,"
    echo "  \"behind\": $behind,"
    printf '  "staged": %s,\n' "$(__gash_json_arr ${staged[@]+"${staged[@]}"})"
    printf '  "modified": %s,\n' "$(__gash_json_arr ${modified[@]+"${modified[@]}"})"
    printf '  "untracked": %s,\n' "$(__gash_json_arr ${untracked[@]+"${untracked[@]}"})"
    printf '  "conflicted": %s\n' "$(__gash_json_arr ${conflicted[@]+"${conflicted[@]}"})"
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
            --staged | --cached)
                staged_only=1
                shift
                ;;
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --staged or a path"
                return 1
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
            -*)
                __llm_error "unknown_option" "$1" "RETRY" "true" "Use --limit N or a path"
                return 1
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
