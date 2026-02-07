#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "AI Module - Config Parsing"

it "parses AI:claude=TOKEN from config" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:claude=sk-ant-api03-test-token-123
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    token="$(__gash_get_ai_token "claude")"
    [[ "$token" == "sk-ant-api03-test-token-123" ]]
'

it "parses AI:gemini=TOKEN from config" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:gemini=AIzaSy-test-gemini-key-456
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    token="$(__gash_get_ai_token "gemini")"
    [[ "$token" == "AIzaSy-test-gemini-key-456" ]]
'

it "rejects unknown AI provider" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:openai=sk-test-openai-key
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    # Should warn but not fail
    __gash_load_env 2>/dev/null

    # Unknown provider should not be stored
    set +e
    token="$(__gash_get_ai_token "openai")"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

it "parses multiple AI providers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:claude=claude-token-123
AI:gemini=gemini-token-456
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    claude_token="$(__gash_get_ai_token "claude")"
    gemini_token="$(__gash_get_ai_token "gemini")"

    [[ "$claude_token" == "claude-token-123" ]]
    [[ "$gemini_token" == "gemini-token-456" ]]
'

describe "AI Module - Token Retrieval"

it "__gash_get_ai_token returns token for valid provider" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:claude=my-claude-token
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    token="$(__gash_get_ai_token "claude")"
    [[ "$token" == "my-claude-token" ]]
'

it "__gash_get_ai_token returns empty for unknown provider" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:claude=my-claude-token
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    set +e
    token="$(__gash_get_ai_token "unknown")"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ -z "$token" ]]
'

it "__gash_get_first_ai_provider returns first configured" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    # Create temp config file
    tmp="$(mktemp)"
    trap "rm -f $tmp" EXIT

    cat > "$tmp" <<EOF
AI:gemini=gemini-token
AI:claude=claude-token
EOF

    export GASH_ENV_FILE="$tmp"
    __GASH_ENV_LOADED=""

    __gash_load_env

    first="$(__gash_get_first_ai_provider)"
    [[ "$first" == "gemini" ]]
'

it "gash_ai_list shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"

    out="$(gash_ai_list -h)"
    [[ "$out" == *"gash_ai_list"* ]]
'

describe "AI Module - Helper Functions"

it "__gash_json_escape escapes quotes correctly" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__gash_json_escape "hello \"world\"")"
    [[ "$result" == "hello \\\"world\\\"" ]]
'

it "__gash_json_escape escapes newlines" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    input=$'"'"'line1
line2'"'"'
    result="$(__gash_json_escape "$input")"
    [[ "$result" == "line1\\nline2" ]]
'

it "__gash_json_escape escapes backslashes" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__gash_json_escape "path\\to\\file")"
    [[ "$result" == "path\\\\to\\\\file" ]]
'

it "__ai_gather_context returns valid JSON with 7 fields" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__ai_gather_context)"

    # Verify JSON is valid with jq
    echo "$result" | jq -e . > /dev/null 2>&1

    # Verify all 7 required fields
    [[ "$result" == *"cwd"* ]]
    [[ "$result" == *"shell"* ]]
    [[ "$result" == *"distro"* ]]
    [[ "$result" == *"pkg"* ]]
    [[ "$result" == *"files"* ]]
    [[ "$result" == *"exit"* ]]
'

it "__ai_gather_context includes current directory" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__ai_gather_context)"
    current_dir="$(pwd)"

    [[ "$result" == *"$current_dir"* ]]
'

describe "AI Module - Dependency Checks"

it "__ai_require_jq fails when jq missing" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Override PATH to hide jq
    export PATH="/nonexistent"

    set +e
    __ai_require_jq 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

it "__ai_require_curl fails when curl missing" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Override PATH to hide curl
    export PATH="/nonexistent"

    set +e
    __ai_require_curl 2>/dev/null
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
'

describe "AI Module - API Call Functions"

it "__ai_call_claude builds request and returns structured JSON with type" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__ai_call_claude "test-token" "how to list files?")"

    # Verify it returns valid JSON with type field
    [[ "$result" == *"type"* ]]
    [[ "$result" == *"text"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

it "__ai_call_gemini builds request and returns structured JSON with type" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__ai_call_gemini "test-token" "how to list files?")"

    # Verify it returns valid JSON with type field
    [[ "$result" == *"type"* ]]
    [[ "$result" == *"text"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

it "__ai_call_claude handles API error" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl with error simulation
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    export MOCK_CURL_ERROR=1

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    set +e
    result="$(__ai_call_claude "test-token" "query" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$result" == *"error"* ]] || [[ "$result" == *"Error"* ]]
'

describe "AI Module - Public Functions"

it "ai_ask shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    out="$(ai_ask -h)"
    [[ "$out" == *"ai_ask"* ]]
'

it "ai_query shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    out="$(ai_query -h)"
    [[ "$out" == *"ai_query"* ]]
'

it "ai_query fails without provider configured" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Set empty providers
    __GASH_ENV_AI_PROVIDERS=""
    __GASH_ENV_LOADED="1"

    set +e
    out="$(ai_query "test query" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"No AI provider"* ]] || [[ "$out" == *"provider"* ]]
'

it "ai_query uses first available provider (type=command)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Set up provider
    __GASH_ENV_AI_PROVIDERS="claude	test-token"
    __GASH_ENV_LOADED="1"

    # Use "how to" query to trigger command type
    result="$(ai_query "how to list files?")"

    # Verify formatted output with Command: and Explanation:
    [[ "$result" == *"Command:"* ]]
    [[ "$result" == *"Explanation:"* ]]
'

it "ai_query uses specified provider (type=explanation)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Set up both providers
    __GASH_ENV_AI_PROVIDERS="claude	claude-token
gemini	gemini-token"
    __GASH_ENV_LOADED="1"

    # Use "what is" query to trigger explanation type
    result="$(ai_query gemini "what is bash?")"

    # Verify formatted output with Explanation: only
    [[ "$result" == *"Explanation:"* ]]
'

it "ai_query returns fallback for generic queries" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Use mock curl
    export PATH="${ROOT}/tests/mocks/bin:$PATH"

    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Set up provider
    __GASH_ENV_AI_PROVIDERS="claude	test-token"
    __GASH_ENV_LOADED="1"

    # Generic query triggers fallback (no labels)
    result="$(ai_query "hello")"

    # Fallback type = direct text, no labels
    [[ "$result" != *"Command:"* ]]
    [[ "$result" != *"Explanation:"* ]]
    # Should have some text response
    [[ -n "$result" ]]
'

describe "AI Module - Response Formatting"

it "__ai_format_response type=command shows Command and Explanation" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"type":"command","command":"ls -la","text":"Lists all files including hidden ones."}'"'"'
    result="$(__ai_format_response "$json_response")"

    [[ "$result" == *"Command:"* ]]
    [[ "$result" == *"ls -la"* ]]
    [[ "$result" == *"Explanation:"* ]]
    [[ "$result" == *"Lists all files"* ]]
'

it "__ai_format_response type=explanation shows only Explanation" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"type":"explanation","text":"Kubernetes is a container orchestration platform."}'"'"'
    result="$(__ai_format_response "$json_response")"

    # Should NOT contain Command: line
    [[ "$result" != *"Command:"* ]]
    # Should contain Explanation
    [[ "$result" == *"Explanation:"* ]]
    [[ "$result" == *"Kubernetes"* ]]
'

it "__ai_format_response type=fallback shows direct text (no labels)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"type":"fallback","text":"Hello! How can I help?"}'"'"'
    result="$(__ai_format_response "$json_response")"

    # Should NOT contain any labels
    [[ "$result" != *"Command:"* ]]
    [[ "$result" != *"Explanation:"* ]]
    [[ "$result" != *"Issue:"* ]]
    # Should contain direct text
    [[ "$result" == *"Hello"* ]]
'

it "__ai_format_response type=troubleshoot shows Issue and Suggestion" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"type":"troubleshoot","issue":"PHP Fatal error","suggestion":"Check variable initialization","text":"Analysis complete"}'"'"'
    result="$(__ai_format_response "$json_response")"

    [[ "$result" == *"Issue:"* ]]
    [[ "$result" == *"PHP Fatal error"* ]]
    [[ "$result" == *"Suggestion:"* ]]
    [[ "$result" == *"Check variable"* ]]
'

it "__ai_format_response type=code shows description and code block" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"type":"code","text":"Creates a backup script.","code":"#!/bin/bash\ntar -czf backup.tar.gz ~/","lang":"bash"}'"'"'
    result="$(__ai_format_response "$json_response")"

    [[ "$result" == *"Creates a backup"* ]]
    [[ "$result" == *"# bash"* ]]
    [[ "$result" == *"tar -czf"* ]]
'

it "__ai_format_response handles missing type (defaults to fallback)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json_response='"'"'{"text":"Just some text."}'"'"'
    result="$(__ai_format_response "$json_response")"

    # Should output text directly (fallback behavior)
    [[ "$result" == *"Just some text"* ]]
'

describe "AI Module - Alias"

it "ask alias is defined" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Check alias exists
    alias_def="$(alias ask 2>/dev/null)"
    [[ "$alias_def" == *"ai_ask"* ]]
'

# =============================================================================
# __ai_handle_curl_error Tests
# =============================================================================

describe "AI Module - Curl Error Handler"

it "__ai_handle_curl_error returns 0 on success (HTTP 200, no curl error)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    __ai_handle_curl_error "Test" "" "200" "1" "{}" ""
    rc=$?
    [[ $rc -eq 0 ]]
'

it "__ai_handle_curl_error returns 1 on curl exit code 28 (timeout)" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    set +e
    out=$(__ai_handle_curl_error "Test" "28" "" "3" "" "" 2>&1)
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"timed out"* ]]
'

it "__ai_handle_curl_error returns 1 on HTTP 401" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    set +e
    out=$(__ai_handle_curl_error "Test" "" "401" "1" "{\"error\":\"invalid_api_key\"}" "" 2>&1)
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"401"* ]]
'
