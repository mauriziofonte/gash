#!/usr/bin/env bash

# Gash Core: Validation Functions
# Centralized validation helpers to eliminate duplicated checks.
#
# All functions return 0 on success, 1 on failure.
# Error messages are printed via __gash_error (requires core/output.sh).

# -----------------------------------------------------------------------------
# Command Validation
# -----------------------------------------------------------------------------

# Verify a command is available in PATH.
# Usage: __gash_require_command "command" ["custom error message"]
# Returns: 0 if command exists, 1 otherwise
__gash_require_command() {
    local cmd="${1-}"
    local custom_msg="${2-}"

    if [[ -z "$cmd" ]]; then
        __gash_error "No command specified for validation."
        return 1
    fi

    if ! command -v "$cmd" >/dev/null 2>&1; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "Command '$cmd' is not installed or not in PATH."
        fi
        return 1
    fi

    return 0
}

# Verify a binary exists (not an alias or function).
# Usage: __gash_require_binary "binary" ["custom error message"]
# Returns: 0 if binary exists, 1 otherwise
__gash_require_binary() {
    local binary="${1-}"
    local custom_msg="${2-}"

    if [[ -z "$binary" ]]; then
        __gash_error "No binary specified for validation."
        return 1
    fi

    if ! type -P "$binary" >/dev/null 2>&1; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "Binary '$binary' is not installed or not in PATH."
        fi
        return 1
    fi

    return 0
}

# -----------------------------------------------------------------------------
# File System Validation
# -----------------------------------------------------------------------------

# Verify a file exists.
# Usage: __gash_require_file "path" ["custom error message"]
# Returns: 0 if file exists, 1 otherwise
__gash_require_file() {
    local filepath="${1-}"
    local custom_msg="${2-}"

    if [[ -z "$filepath" ]]; then
        __gash_error "No file path specified."
        return 1
    fi

    if [[ ! -f "$filepath" ]]; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "File '$filepath' does not exist."
        fi
        return 1
    fi

    return 0
}

# Verify a directory exists.
# Usage: __gash_require_dir "path" ["custom error message"]
# Returns: 0 if directory exists, 1 otherwise
__gash_require_dir() {
    local dirpath="${1-}"
    local custom_msg="${2-}"

    if [[ -z "$dirpath" ]]; then
        __gash_error "No directory path specified."
        return 1
    fi

    if [[ ! -d "$dirpath" ]]; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "Directory '$dirpath' does not exist."
        fi
        return 1
    fi

    return 0
}

# Verify a file is readable.
# Usage: __gash_require_readable "path" ["custom error message"]
# Returns: 0 if readable, 1 otherwise
__gash_require_readable() {
    local filepath="${1-}"
    local custom_msg="${2-}"

    if [[ -z "$filepath" ]]; then
        __gash_error "No file path specified."
        return 1
    fi

    if [[ ! -r "$filepath" ]]; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "File '$filepath' is not readable."
        fi
        return 1
    fi

    return 0
}

# -----------------------------------------------------------------------------
# Git Validation
# -----------------------------------------------------------------------------

# Verify we are inside a Git repository.
# Usage: __gash_require_git_repo ["custom error message"]
# Returns: 0 if in git repo, 1 otherwise
__gash_require_git_repo() {
    local custom_msg="${1-}"

    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "Not in a git repository."
        fi
        return 1
    fi

    return 0
}

# Verify the remote 'origin' is configured.
# Usage: __gash_require_git_remote ["remote_name"] ["custom error message"]
# Returns: 0 if remote exists, 1 otherwise
__gash_require_git_remote() {
    local remote="${1:-origin}"
    local custom_msg="${2-}"

    if ! git remote get-url "$remote" >/dev/null 2>&1; then
        if [[ -n "$custom_msg" ]]; then
            __gash_error "$custom_msg"
        else
            __gash_error "Remote '$remote' is not configured."
        fi
        return 1
    fi

    return 0
}

# Verify we are inside a Git repository with a configured remote.
# Combines __gash_require_git_repo and __gash_require_git_remote.
# Usage: __gash_require_git_repo_with_remote ["remote_name"]
# Returns: 0 if both checks pass, 1 otherwise
__gash_require_git_repo_with_remote() {
    local remote="${1:-origin}"

    __gash_require_git_repo || return 1
    __gash_require_git_remote "$remote" || return 1

    return 0
}

# -----------------------------------------------------------------------------
# Argument Validation
# -----------------------------------------------------------------------------

# Verify an argument is not empty.
# Usage: __gash_require_arg "value" "argument_name" ["usage_hint"]
# Returns: 0 if not empty, 1 otherwise
__gash_require_arg() {
    local value="${1-}"
    local arg_name="${2:-argument}"
    local usage_hint="${3-}"

    if [[ -z "$value" ]]; then
        if [[ -n "$usage_hint" ]]; then
            __gash_error "Missing $arg_name. Usage: $usage_hint"
        else
            __gash_error "Missing required $arg_name."
        fi
        return 1
    fi

    return 0
}

# Verify argument count is at least N.
# Usage: __gash_require_args_min count minimum "usage_hint"
# Returns: 0 if count >= minimum, 1 otherwise
__gash_require_args_min() {
    local count="${1:-0}"
    local minimum="${2:-1}"
    local usage_hint="${3-}"

    if [[ "$count" -lt "$minimum" ]]; then
        if [[ -n "$usage_hint" ]]; then
            __gash_error "Not enough arguments. Usage: $usage_hint"
        else
            __gash_error "Expected at least $minimum argument(s), got $count."
        fi
        return 1
    fi

    return 0
}

# -----------------------------------------------------------------------------
# Docker Validation
# -----------------------------------------------------------------------------

# Verify Docker is installed and available.
# Usage: __gash_require_docker
# Returns: 0 if docker is available, 1 otherwise
__gash_require_docker() {
    __gash_require_command "docker" "Docker is not installed."
}
