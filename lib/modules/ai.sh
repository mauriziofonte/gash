#!/usr/bin/env bash

# =============================================================================
# Gash Module: AI Chat Integration
# =============================================================================
#
# Interactive chat with Claude/Gemini APIs directly from terminal.
#
# Dependencies: core/output.sh, core/config.sh, core/utils.sh
# Runtime: curl, jq
#
# Configuration in ~/.gash_env:
#   AI:claude=YOUR_CLAUDE_API_KEY
#   AI:gemini=YOUR_GEMINI_API_KEY
#
# Public functions:
#   ai_ask [provider]          - Interactive chat (alias: ask)
#   ai_query [provider] query  - Non-interactive query
#   gash_ai_list               - List configured providers (in config.sh)
#
# =============================================================================

# =============================================================================
# CONSTANTS
# =============================================================================

# Guard against re-sourcing (readonly variables cannot be redefined)
if [[ -z "${__AI_CLAUDE_API:-}" ]]; then
    # API Endpoints
    readonly __AI_CLAUDE_API="https://api.anthropic.com/v1/messages"
    readonly __AI_GEMINI_API="https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

    # Models
    readonly __AI_CLAUDE_MODEL="claude-haiku-4-5-20251001"
    readonly __AI_GEMINI_MODEL="gemini-2.0-flash"

    # Timeouts (seconds)
    readonly __AI_CONNECT_TIMEOUT=5   # Connection timeout (quick fail if unreachable)
    readonly __AI_RESPONSE_TIMEOUT=60 # Response timeout (allow LLM time to think)

    # System prompt for structured JSON output
    readonly __AI_SYSTEM_PROMPT='You are a bash/linux terminal assistant. Respond ONLY with valid JSON.

RESPONSE TYPES (select in order of priority):
1. "troubleshoot": Content provided via pipe → analyze, identify issue, suggest fix
2. "command": User asks HOW TO DO something → provide bash command + explanation
3. "explanation": User asks WHAT something IS → provide conceptual explanation
4. "code": User asks to WRITE a script/config → provide code snippet
5. "fallback": Everything else (greetings, unclassifiable) → brief direct answer

RULES:
1. Match user language in text field
2. For "command": command = exact bash command, text = brief explanation
3. For "code": code = complete snippet, lang = language name
4. For "troubleshoot": issue = problem found, suggestion = how to fix, text = summary
5. For "fallback": only text field needed
6. Keep all fields concise (1-3 sentences max)
7. NEVER use markdown formatting in any field'

    # JSON Schema for structured responses (5 types)
    readonly __AI_RESPONSE_SCHEMA='{
  "type": "object",
  "properties": {
    "type": {
      "type": "string",
      "enum": ["command", "explanation", "code", "troubleshoot", "fallback"],
      "description": "Response type"
    },
    "command": {
      "type": "string",
      "description": "Bash command. Only when type=command."
    },
    "text": {
      "type": "string",
      "description": "Main response text. Required for all types."
    },
    "code": {
      "type": "string",
      "description": "Code snippet. Only when type=code."
    },
    "lang": {
      "type": "string",
      "description": "Code language (bash, python, etc). Only when type=code."
    },
    "issue": {
      "type": "string",
      "description": "Problem identified. Only when type=troubleshoot."
    },
    "suggestion": {
      "type": "string",
      "description": "How to fix/resolve. Only when type=troubleshoot."
    }
  },
  "required": ["type", "text"],
  "additionalProperties": false
}'
fi

# =============================================================================
# INTERNAL HELPERS
# =============================================================================

# Check if jq is available
__ai_require_jq() {
    if ! type -P jq >/dev/null 2>&1; then
        __gash_error "jq is required for AI module. Install with: sudo apt install jq"
        return 1
    fi
    return 0
}

# Check if curl is available
__ai_require_curl() {
    if ! type -P curl >/dev/null 2>&1; then
        __gash_error "curl is required for AI module. Install with: sudo apt install curl"
        return 1
    fi
    return 0
}

# Escape string for JSON
# Usage: __ai_json_escape "string"
__ai_json_escape() {
    local str="${1-}"
    # Escape backslashes first, then other special chars
    str="${str//\\/\\\\}"
    str="${str//\"/\\\"}"
    str="${str//$'\n'/\\n}"
    str="${str//$'\r'/\\r}"
    str="${str//$'\t'/\\t}"
    printf '%s' "$str"
}

# Gather context information (~80 tokens)
# Returns JSON: {"cwd":"/path","shell":"bash","distro":"ubuntu","pkg":"apt","git":"branch","files":["a","b"],"exit":0}
__ai_gather_context() {
    local cwd shell_name distro pkg_mgr git_branch exit_code files_json

    cwd="$(pwd)"

    # Detect shell
    if [[ -n "${BASH_VERSION:-}" ]]; then
        shell_name="bash"
    elif [[ -n "${ZSH_VERSION:-}" ]]; then
        shell_name="zsh"
    else
        shell_name="sh"
    fi

    # Last exit code (captured by PROMPT_COMMAND hook)
    exit_code="${__AI_LAST_EXIT:-0}"

    # Detect distro and package manager
    if [[ -f /etc/os-release ]]; then
        distro=$(grep -oP '^ID=\K\w+' /etc/os-release 2>/dev/null || echo "linux")
    elif [[ "$(uname)" == "Darwin" ]]; then
        distro="macos"
    else
        distro="linux"
    fi

    case "$distro" in
        ubuntu|debian|pop|mint) pkg_mgr="apt" ;;
        fedora|rhel|centos|rocky|alma) pkg_mgr="dnf" ;;
        arch|manjaro|endeavouros) pkg_mgr="pacman" ;;
        opensuse*|suse) pkg_mgr="zypper" ;;
        alpine) pkg_mgr="apk" ;;
        macos) pkg_mgr="brew" ;;
        *) pkg_mgr="unknown" ;;
    esac

    # Git branch (null if not in repo)
    git_branch=""
    if type -P git >/dev/null 2>&1; then
        git_branch=$(git branch --show-current 2>/dev/null) || true
    fi

    # Top 8 files/dirs in cwd (names only)
    files_json=$(ls -1 2>/dev/null | head -8 | jq -R -s -c 'split("\n") | map(select(. != ""))')
    [[ -z "$files_json" ]] && files_json="[]"

    # Build JSON using jq for proper escaping
    jq -n -c \
        --arg cwd "$cwd" \
        --arg shell "$shell_name" \
        --arg distro "$distro" \
        --arg pkg "$pkg_mgr" \
        --arg git "$git_branch" \
        --argjson files "$files_json" \
        --argjson exit "$exit_code" \
        '{cwd:$cwd,shell:$shell,distro:$distro,pkg:$pkg,git:($git|if . == "" then null else . end),files:$files,exit:$exit}'
}

# Execute without recording in history
# Usage: __ai_no_history <function_name> [args...]
__ai_no_history() {
    local hist_was_enabled=0

    if [[ -o history ]]; then
        hist_was_enabled=1
        set +o history
    fi

    "$@"
    local rc=$?

    if [[ $hist_was_enabled -eq 1 ]]; then
        set -o history
    fi

    return $rc
}

# Format and display AI response based on type
# Usage: __ai_format_response "json_response"
__ai_format_response() {
    local json="${1-}"

    local resp_type text command code lang issue suggestion
    resp_type=$(echo "$json" | jq -r '.type // "fallback"' 2>/dev/null)
    text=$(echo "$json" | jq -r '.text // ""' 2>/dev/null)
    command=$(echo "$json" | jq -r '.command // ""' 2>/dev/null)
    code=$(echo "$json" | jq -r '.code // ""' 2>/dev/null)
    lang=$(echo "$json" | jq -r '.lang // "bash"' 2>/dev/null)
    issue=$(echo "$json" | jq -r '.issue // ""' 2>/dev/null)
    suggestion=$(echo "$json" | jq -r '.suggestion // ""' 2>/dev/null)

    case "$resp_type" in
        command)
            [[ -n "$command" ]] && echo -e "${__GASH_CYAN}Command:${__GASH_COLOR_OFF} ${__GASH_BOLD_WHITE}\`${command}\`${__GASH_COLOR_OFF}"
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}Explanation:${__GASH_COLOR_OFF} ${text}"
            ;;
        explanation)
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}Explanation:${__GASH_COLOR_OFF} ${text}"
            ;;
        troubleshoot)
            [[ -n "$issue" ]] && echo -e "${__GASH_RED}Issue:${__GASH_COLOR_OFF} ${issue}"
            [[ -n "$suggestion" ]] && echo -e "${__GASH_YELLOW}Suggestion:${__GASH_COLOR_OFF} ${suggestion}"
            # Show text only if no issue (as summary)
            { [[ -n "$text" && -z "$issue" ]] && echo "$text"; } || true
            ;;
        code)
            [[ -n "$text" ]] && echo -e "${__GASH_GREEN}${text}${__GASH_COLOR_OFF}"
            echo -e "${__GASH_CYAN}# ${lang}${__GASH_COLOR_OFF}"
            [[ -n "$code" ]] && echo "$code"
            ;;
        fallback|*)
            # Direct output, no labels
            [[ -n "$text" ]] && echo "$text"
            ;;
    esac
}

# =============================================================================
# API CALL FUNCTIONS
# =============================================================================

# Call Claude API with structured output
# Usage: __ai_call_claude <token> <query>
# Returns: JSON response with command and explanation
__ai_call_claude() {
    local token="${1-}"
    local query="${2-}"

    local context
    context="$(__ai_gather_context)"

    # Build request body with structured output using jq for proper escaping
    local body
    body=$(jq -n \
        --arg model "$__AI_CLAUDE_MODEL" \
        --arg system "$__AI_SYSTEM_PROMPT" \
        --arg context "Context: $context" \
        --arg query "$query" \
        --argjson schema "$__AI_RESPONSE_SCHEMA" \
        '{
            model: $model,
            max_tokens: 1024,
            system: $system,
            messages: [{role: "user", content: ($context + "\n\n" + $query)}],
            output_config: {
                format: {
                    type: "json_schema",
                    schema: $schema
                }
            }
        }')

    # Make API call
    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)

    # Track start time for timeout differentiation
    local start_time elapsed_time
    start_time=$(date +%s)

    http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
        --connect-timeout "$__AI_CONNECT_TIMEOUT" \
        --max-time "$__AI_RESPONSE_TIMEOUT" \
        -H "Content-Type: application/json" \
        -H "x-api-key: $token" \
        -H "anthropic-version: 2023-06-01" \
        -d "$body" \
        "$__AI_CLAUDE_API" 2>"$tmp_err") || curl_exit=$?

    elapsed_time=$(( $(date +%s) - start_time ))
    response=$(<"$tmp_file")
    local curl_error=$(<"$tmp_err")
    rm -f "$tmp_file" "$tmp_err"

    # Handle curl failures first
    if [[ -n "${curl_exit:-}" ]]; then
        case "$curl_exit" in
            5)  __gash_error "Claude API: Couldn't resolve proxy" ;;
            6)  __gash_error "Claude API: Could not resolve host (check DNS/internet)" ;;
            7)  __gash_error "Claude API: Failed to connect (server unreachable)" ;;
            18) __gash_error "Claude API: Partial response received (transfer interrupted)" ;;
            22) __gash_error "Claude API: HTTP error returned" ;;
            28)
                # Distinguish connection timeout vs response timeout based on elapsed time
                if [[ $elapsed_time -le $(( __AI_CONNECT_TIMEOUT + 2 )) ]]; then
                    __gash_error "Claude API: Connection timed out after ${elapsed_time}s (server unreachable)"
                else
                    __gash_error "Claude API: Response timed out after ${elapsed_time}s (max: ${__AI_RESPONSE_TIMEOUT}s)"
                fi
                ;;
            35) __gash_error "Claude API: SSL/TLS handshake failed" ;;
            47) __gash_error "Claude API: Too many redirects" ;;
            52) __gash_error "Claude API: Server returned empty response" ;;
            55) __gash_error "Claude API: Failed sending data to server" ;;
            56) __gash_error "Claude API: Failed receiving data from server" ;;
            60) __gash_error "Claude API: SSL certificate problem (verify failed)" ;;
            *)  __gash_error "Claude API: curl failed (exit $curl_exit)${curl_error:+: $curl_error}" ;;
        esac
        return 1
    fi

    # Check HTTP status
    if [[ -z "$http_code" || "$http_code" == "000" ]]; then
        __gash_error "Claude API: No response received (network error)"
        return 1
    fi

    if [[ "$http_code" != "200" ]]; then
        local error_msg
        error_msg=$(echo "$response" | jq -r '.error.message // .error // "Unknown error"' 2>/dev/null)
        __gash_error "Claude API error ($http_code): $error_msg"
        return 1
    fi

    # Extract response text (contains structured JSON)
    local text
    text=$(echo "$response" | jq -r '.content[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Claude response"
        return 1
    fi

    printf '%s' "$text"
}

# Call Gemini API with structured output
# Usage: __ai_call_gemini <token> <query>
# Returns: JSON response with command and explanation
__ai_call_gemini() {
    local token="${1-}"
    local query="${2-}"

    local context
    context="$(__ai_gather_context)"

    # Build request body with structured output using jq for proper escaping
    local body
    body=$(jq -n \
        --arg system "$__AI_SYSTEM_PROMPT" \
        --arg context "Context: $context" \
        --arg query "$query" \
        --argjson schema "$__AI_RESPONSE_SCHEMA" \
        '{
            contents: [{
                parts: [{text: ($system + "\n\n" + $context + "\n\n" + $query)}]
            }],
            generationConfig: {
                maxOutputTokens: 1024,
                responseMimeType: "application/json",
                responseJsonSchema: $schema
            }
        }')

    # Make API call (token in query param)
    local response http_code curl_exit
    local tmp_file tmp_err
    tmp_file=$(mktemp)
    tmp_err=$(mktemp)

    # Track start time for timeout differentiation
    local start_time elapsed_time
    start_time=$(date +%s)

    http_code=$(curl -s -w "%{http_code}" -o "$tmp_file" \
        --connect-timeout "$__AI_CONNECT_TIMEOUT" \
        --max-time "$__AI_RESPONSE_TIMEOUT" \
        -H "Content-Type: application/json" \
        -d "$body" \
        "${__AI_GEMINI_API}?key=${token}" 2>"$tmp_err") || curl_exit=$?

    elapsed_time=$(( $(date +%s) - start_time ))
    response=$(<"$tmp_file")
    local curl_error=$(<"$tmp_err")
    rm -f "$tmp_file" "$tmp_err"

    # Handle curl failures first
    if [[ -n "${curl_exit:-}" ]]; then
        case "$curl_exit" in
            5)  __gash_error "Gemini API: Couldn't resolve proxy" ;;
            6)  __gash_error "Gemini API: Could not resolve host (check DNS/internet)" ;;
            7)  __gash_error "Gemini API: Failed to connect (server unreachable)" ;;
            18) __gash_error "Gemini API: Partial response received (transfer interrupted)" ;;
            22) __gash_error "Gemini API: HTTP error returned" ;;
            28)
                # Distinguish connection timeout vs response timeout based on elapsed time
                if [[ $elapsed_time -le $(( __AI_CONNECT_TIMEOUT + 2 )) ]]; then
                    __gash_error "Gemini API: Connection timed out after ${elapsed_time}s (server unreachable)"
                else
                    __gash_error "Gemini API: Response timed out after ${elapsed_time}s (max: ${__AI_RESPONSE_TIMEOUT}s)"
                fi
                ;;
            35) __gash_error "Gemini API: SSL/TLS handshake failed" ;;
            47) __gash_error "Gemini API: Too many redirects" ;;
            52) __gash_error "Gemini API: Server returned empty response" ;;
            55) __gash_error "Gemini API: Failed sending data to server" ;;
            56) __gash_error "Gemini API: Failed receiving data from server" ;;
            60) __gash_error "Gemini API: SSL certificate problem (verify failed)" ;;
            *)  __gash_error "Gemini API: curl failed (exit $curl_exit)${curl_error:+: $curl_error}" ;;
        esac
        return 1
    fi

    # Check HTTP status
    if [[ -z "$http_code" || "$http_code" == "000" ]]; then
        __gash_error "Gemini API: No response received (network error)"
        return 1
    fi

    if [[ "$http_code" != "200" ]]; then
        local error_msg
        error_msg=$(echo "$response" | jq -r '.error.message // .error // "Unknown error"' 2>/dev/null)
        __gash_error "Gemini API error ($http_code): $error_msg"
        return 1
    fi

    # Extract response text (contains structured JSON)
    local text
    text=$(echo "$response" | jq -r '.candidates[0].content.parts[0].text // empty' 2>/dev/null)

    if [[ -z "$text" ]]; then
        __gash_error "Failed to parse Gemini response"
        return 1
    fi

    printf '%s' "$text"
}

# =============================================================================
# PUBLIC FUNCTIONS
# =============================================================================

# Interactive AI chat
# Usage: ai_ask [provider]
# Example: ai_ask          # uses first available provider
# Example: ai_ask claude   # uses Claude
# Example: ai_ask gemini   # uses Gemini
ai_ask() {
    needs_help "ai_ask" "ai_ask [provider]" \
        "Interactive AI chat. Provider: claude or gemini (default: first available)" \
        "${1-}" && return

    __ai_no_history __ai_ask_impl "$@"
}

__ai_ask_impl() {
    local provider="${1-}"

    # Check dependencies
    __ai_require_curl || return 1
    __ai_require_jq || return 1

    # Determine provider
    if [[ -z "$provider" ]]; then
        provider=$(__gash_get_first_ai_provider) || {
            __gash_error "No AI provider configured in ~/.gash_env"
            __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
            return 1
        }
    fi

    # Validate provider
    if [[ ! "$provider" =~ ^(claude|gemini)$ ]]; then
        __gash_error "Unknown provider '$provider'. Use 'claude' or 'gemini'"
        return 1
    fi

    # Get token
    local token
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        __gash_info "Add: AI:$provider=YOUR_API_KEY"
        return 1
    }

    # Show prompt and read query
    local query
    printf "%s: " "$provider"
    read -r query

    if [[ -z "$query" ]]; then
        __gash_warning "Empty query, aborting"
        return 1
    fi

    # Show spinner with provider name
    local provider_display
    provider_display="$(echo "$provider" | sed 's/.*/\u&/')"  # Capitalize first letter
    __gash_spinner_start "${provider_display} is thinking..."

    # Call API
    local response
    case "$provider" in
        claude)
            response=$(__ai_call_claude "$token" "$query")
            ;;
        gemini)
            response=$(__ai_call_gemini "$token" "$query")
            ;;
    esac

    local rc=$?

    # Stop spinner (no status message)
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    if [[ $rc -ne 0 ]]; then
        return 1
    fi

    # Display formatted response
    __ai_format_response "$response"
}

# Non-interactive AI query
# Usage: ai_query [provider] "query"
# Example: ai_query "how to list files by size?"
# Example: ai_query claude "what is 2+2?"
ai_query() {
    needs_help "ai_query" "ai_query [provider] \"query\"" \
        "Non-interactive AI query. Provider is optional (uses first available)" \
        "${1-}" && return

    __ai_no_history __ai_query_impl "$@"
}

__ai_query_impl() {
    local provider=""
    local query=""
    local piped_content=""

    # FIRST: Check for pipe input (stdin not a terminal)
    if [[ ! -t 0 ]]; then
        # stdin is not a terminal = pipe input
        # Truncate to max 4KB (~1000 tokens) to avoid token waste
        piped_content=$(head -c 4096)
        local piped_size=${#piped_content}
        if [[ $piped_size -ge 4096 ]]; then
            piped_content="${piped_content}
[... truncated to 4KB]"
        fi
    fi

    # Parse arguments: ai_query [provider] "query"
    if [[ $# -eq 1 ]]; then
        # Only query provided
        query="$1"
    elif [[ $# -ge 2 ]]; then
        # Check if first arg is a provider
        if [[ "$1" =~ ^(claude|gemini)$ ]]; then
            provider="$1"
            shift
            query="$*"
        else
            # First arg is part of query
            query="$*"
        fi
    fi

    if [[ -z "$query" ]]; then
        __gash_error "No query provided"
        return 1
    fi

    # If pipe content exists, prepend it to query for troubleshoot mode
    if [[ -n "$piped_content" ]]; then
        query="[PIPE INPUT - TROUBLESHOOT MODE]
---
${piped_content}
---
User question: ${query}"
    fi

    # Check dependencies
    __ai_require_curl || return 1
    __ai_require_jq || return 1

    # Determine provider
    if [[ -z "$provider" ]]; then
        provider=$(__gash_get_first_ai_provider) || {
            __gash_error "No AI provider configured in ~/.gash_env"
            __gash_info "Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY"
            return 1
        }
    fi

    # Get token
    local token
    token=$(__gash_get_ai_token "$provider") || {
        __gash_error "No token configured for '$provider' in ~/.gash_env"
        return 1
    }

    # Show spinner with provider name
    local provider_display
    provider_display="$(echo "$provider" | sed 's/.*/\u&/')"  # Capitalize first letter
    __gash_spinner_start "${provider_display} is thinking..."

    # Call API
    local response
    case "$provider" in
        claude)
            response=$(__ai_call_claude "$token" "$query")
            ;;
        gemini)
            response=$(__ai_call_gemini "$token" "$query")
            ;;
    esac

    local rc=$?

    # Stop spinner (no status message)
    if [[ -n "${__GASH_SPINNER_PID-}" ]]; then
        kill "$__GASH_SPINNER_PID" 2>/dev/null || true
        wait "$__GASH_SPINNER_PID" 2>/dev/null || true
        __GASH_SPINNER_PID=""
        printf "\r\033[K"
        trap - EXIT
    fi

    if [[ $rc -ne 0 ]]; then
        return 1
    fi

    # Display formatted response
    __ai_format_response "$response"
}

# =============================================================================
# ALIAS
# =============================================================================

alias ask='ai_ask'
