#!/usr/bin/env bash
#
# The shared AI HTTP client: what leaves the machine (redaction, argv,
# consent) and what comes back (sanitizing, retries, refusals).

describe "AI client - what is sent"

it "secrets are redacted before sending" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"

    out="$(__ai_redact <<'"'"'EOF'"'"'
DB_PASSWORD=hunter2
export API_KEY="abc123def"
url: postgres://app:s3cret@db:5432/app
aws AKIAABCDEFGHIJKLMNOP
gh ghp_abcdefghijklmnopqrstuvwxyz0123456789
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAA
-----END OPENSSH PRIVATE KEY-----
keep this line
EOF
)"
    for secret in hunter2 abc123def s3cret AKIAABCDEFGHIJKLMNOP ghp_abcdefghij b3BlbnNzaC1rZXktdjEAAAAA; do
        [[ "$out" != *"$secret"* ]] || { echo "leaked: $secret"; exit 1; }
    done
    [[ "$out" == *"keep this line"* ]]
    [[ "$out" == *"postgres://app:"* ]]
'

it "redaction keeps ordinary words after password/token/secret" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"

    out="$(printf "%s\n" "token expired at login" "my secret is safe" "token Zk29dLq8Pz7Wm4Rt1xYv" | __ai_redact)"
    [[ "$out" == *"token expired at login"* ]]
    [[ "$out" == *"my secret is safe"* ]]
    [[ "$out" != *"Zk29dLq8Pz7Wm4Rt1xYv"* ]]
'

it "the request body and API key never appear in curl argv" bash -c '
    set -euo pipefail
    command -v jq >/dev/null 2>&1 || exit 77
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"
    tmp=$(mktemp -d); trap "rm -rf $tmp" EXIT
    export MOCK_CURL_LOG="$tmp/log"

    __ai_call_claude "sk-ant-secret-token" "what is unique-marker-42?" >/dev/null
    argv="$(grep "^curl " "$MOCK_CURL_LOG")"
    [[ "$argv" != *"unique-marker-42"* ]]
    [[ "$argv" != *"sk-ant-secret-token"* ]]
    grep -q "^body: .*unique-marker-42" "$MOCK_CURL_LOG"
'

it "consent: auto asks only for collected data, refuses without a terminal" bash -c '
    set -uo pipefail
    export GASH_HEADLESS=1
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"

    unset GASH_AI_CONFIRM
    __ai_confirm_send claude query "your question" 10 || exit 1
    err="$(__ai_confirm_send claude data "system data" 4096 2>&1)" && exit 1
    [[ "$err" == *"GASH_AI_CONFIRM=0"* ]] || exit 1
    [[ "$err" == *"4 KB"* ]] || exit 1

    GASH_AI_CONFIRM=1 __ai_confirm_send claude query "your question" 10 2>/dev/null && exit 1
    GASH_AI_CONFIRM=0 __ai_confirm_send claude data "system data" 10 || exit 1
    exit 0
'

it "ai_query sends nothing when consent is refused" bash -c '
    set -uo pipefail
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH" GASH_HEADLESS=1 GASH_AI_CONFIRM=1
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"
    tmp=$(mktemp -d); trap "rm -rf $tmp" EXIT
    export MOCK_CURL_LOG="$tmp/log"
    __GASH_ENV_AI_PROVIDERS="claude	test-token"
    __GASH_ENV_LOADED="1"

    ai_query "how to list files?" </dev/null >/dev/null 2>&1 && exit 1
    [[ ! -s "$MOCK_CURL_LOG" ]]
'

describe "AI client - what comes back"

it "terminal escapes in a reply are stripped" bash -c '
    set -euo pipefail
    command -v jq >/dev/null 2>&1 || exit 77
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"

    inner="$(jq -cn --arg t $'"'"'ok\e]0;pwned\a\e[31mred'"'"' "{type: \"explanation\", text: \$t}")"
    export MOCK_CURL_AI_RESPONSE="$(jq -cn --arg i "$inner" "{content: [{type: \"text\", text: \$i}], stop_reason: \"end_turn\"}")"

    out="$(__ai_call_claude "test-token" "anything")"
    [[ "$out" != *$'"'"'\e'"'"'* && "$out" != *$'"'"'\a'"'"'* ]]
    [[ "$(jq -r .text <<<"$out")" == "ok]0;pwned[31mred" ]]
'

it "a 529 overload is retried after Retry-After" bash -c '
    set -euo pipefail
    command -v jq >/dev/null 2>&1 || exit 77
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"
    tmp=$(mktemp -d); trap "rm -rf $tmp" EXIT
    export MOCK_CURL_AI_BUSY_ONCE=529 MOCK_CURL_STATE="$tmp/state" MOCK_CURL_LOG="$tmp/log"

    out="$(__ai_call_claude "test-token" "what is bash?" 2>"$tmp/err")"
    jq -e .type <<<"$out" >/dev/null
    grep -q "retrying in 1s (1/3)" "$tmp/err"
    [[ "$(grep -c "^curl " "$MOCK_CURL_LOG")" == 2 ]]
'

it "a refusal stop_reason is reported, not parsed" bash -c '
    set -uo pipefail
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
    export MOCK_CURL_AI_RESPONSE="{\"content\":[],\"stop_reason\":\"refusal\"}"
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"

    err="$(__ai_call_claude "test-token" "anything" 2>&1 >/dev/null)" && exit 1
    [[ "$err" == *"declined"* ]]
'

it "GASH_AI_MODEL_CLAUDE overrides the model sent" bash -c '
    set -euo pipefail
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH" GASH_AI_MODEL_CLAUDE=claude-haiku-5-5
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"
    tmp=$(mktemp -d); trap "rm -rf $tmp" EXIT
    export MOCK_CURL_LOG="$tmp/log"

    __ai_call_claude "test-token" "what is bash?" >/dev/null
    grep -q "\"model\":\"claude-haiku-5-5\"" "$MOCK_CURL_LOG"
'

describe "AI client - request shape"

it "Claude 5 models get effort low, older ones no effort field" bash -c '
    set -euo pipefail
    command -v jq >/dev/null 2>&1 || exit 77
    export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
    export GASH_HEADLESS=1
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    source "$GASH_TEST_ROOT/lib/core/utils.sh"
    source "$GASH_TEST_ROOT/lib/core/config.sh"
    source "$GASH_TEST_ROOT/lib/modules/ai.sh"
    tmp=$(mktemp -d); trap "rm -rf $tmp" EXIT
    export MOCK_CURL_LOG="$tmp/log"

    __ai_call_claude "tok" "hello" >/dev/null
    body="$(sed -n "s/^body: //p" "$MOCK_CURL_LOG" | tail -1)"
    jq -e ".model == \"claude-haiku-5-5\" and .output_config.effort == \"low\" and .output_config.format.type == \"json_schema\"" <<<"$body" >/dev/null

    GASH_AI_MODEL_CLAUDE=claude-haiku-4-5 __ai_call_claude "tok" "hello" >/dev/null
    body="$(sed -n "s/^body: //p" "$MOCK_CURL_LOG" | tail -1)"
    jq -e ".model == \"claude-haiku-4-5\" and (.output_config | has(\"effort\") | not)" <<<"$body" >/dev/null
'
