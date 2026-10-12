#!/usr/bin/env bash

# Gash Module: LLM Utilities: llm_exec
# Validated command execution.
# Sourced by lib/modules/llm.sh.

# -----------------------------------------------------------------------------
# Core Helper
# -----------------------------------------------------------------------------

# Execute a command safely without recording in bash history.
# Validates the command against dangerous patterns before execution.
# Usage: llm_exec <command>
# Example: llm_exec "ls -la /tmp"
llm_exec() {
    if needs_help "llm_exec" "llm_exec <command>" "Execute command safely (no history, validated against dangerous patterns)" "${1-}"; then
        return 0
    fi

    local cmd="${1-}"

    if [[ -z "$cmd" ]]; then
        __llm_error "empty_command" "" "RETRY" "true" "Provide a command to execute"
        return 1
    fi

    # Validate command is safe
    if ! __llm_validate_command "$cmd"; then
        return 1
    fi

    # Execute without history
    __gash_no_history eval "$cmd"
}
