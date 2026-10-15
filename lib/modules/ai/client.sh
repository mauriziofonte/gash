#!/usr/bin/env bash

# Gash Module: AI: shared HTTP client (redaction, consent, retries, sanitizing).
# Sourced by lib/modules/ai.sh.

# =============================================================================
# INTERNAL HELPERS
# =============================================================================

# Debug output (enabled with GASH_AI_DEBUG=1)
# Usage: __ai_debug "label" "content"
__ai_debug() {
    [[ "${GASH_AI_DEBUG:-0}" != "1" ]] && return 0
    local label="${1-}"
    local content="${2-}"
    printf '\n%s\n' "${__GASH_CYAN}[AI DEBUG] ${label}:${__GASH_COLOR_OFF}" >&2
    echo "$content" >&2
}

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

# Handle curl exit codes and HTTP errors for AI API calls.
# Returns: 0 if no error, 1 if fatal error, 2 if retryable (rate limit,
# overload or a transient server error); the delay to wait before retrying
# is left in __AI_RETRY_DELAY (seconds).
# HEADERS_FILE (optional) holds the response headers, for Retry-After.
# Usage: __ai_handle_curl_error <provider> <curl_exit> <http_code> <elapsed_time> <response> <curl_error> [headers_file]
__ai_handle_curl_error() {
    local provider="$1" curl_exit="${2:-}" http_code="$3"
    local elapsed_time="$4" response="$5" curl_error="${6:-}" headers="${7:-}"

    if [[ -n "$curl_exit" ]]; then
        case "$curl_exit" in
            5) __gash_error "$provider API: Couldn't resolve proxy" ;;
            6) __gash_error "$provider API: Could not resolve host (check DNS/internet)" ;;
            7) __gash_error "$provider API: Failed to connect (server unreachable)" ;;
            18) __gash_error "$provider API: Partial response received (transfer interrupted)" ;;
            22) __gash_error "$provider API: HTTP error returned" ;;
            28)
                if [[ $elapsed_time -le $((__AI_CONNECT_TIMEOUT + 2)) ]]; then
                    __gash_error "$provider API: Connection timed out after ${elapsed_time}s (server unreachable)"
                else
                    __gash_error "$provider API: Response timed out after ${elapsed_time}s"
                fi
                ;;
            35) __gash_error "$provider API: SSL/TLS handshake failed" ;;
            47) __gash_error "$provider API: Too many redirects" ;;
            52) __gash_error "$provider API: Server returned empty response" ;;
            55) __gash_error "$provider API: Failed sending data to server" ;;
            56) __gash_error "$provider API: Failed receiving data from server" ;;
            60) __gash_error "$provider API: SSL certificate problem (verify failed)" ;;
            *) __gash_error "$provider API: curl failed (exit $curl_exit)${curl_error:+: $curl_error}" ;;
        esac
        return 1
    fi

    if [[ -z "$http_code" || "$http_code" == "000" ]]; then
        __gash_error "$provider API: No response received (network error)"
        return 1
    fi

    case "$http_code" in
        200) return 0 ;;
        429 | 500 | 502 | 503 | 504 | 529)
            # Retry-After header (seconds), else Gemini RetryInfo, else 5s.
            local retry_delay=""
            if [[ -n "$headers" && -f "$headers" ]]; then
                retry_delay="$(tr -d '\r' <"$headers" | "${__GASH_AWK:-awk}" -F': *' 'tolower($1) == "retry-after" { print $2; exit }')"
            fi
            if [[ -z "$retry_delay" ]]; then
                retry_delay=$(jq -r '.error.details[]? | select(."@type" // "" | contains("RetryInfo")) | .retryDelay // empty' <<<"$response" 2>/dev/null | head -1)
            fi
            if [[ "$retry_delay" =~ ^([0-9.]+)ms$ ]]; then
                # LC_NUMERIC=C: a comma-decimal locale would print "0,6"
                retry_delay=$(LC_NUMERIC=C "${__GASH_AWK:-awk}" "BEGIN{printf \"%.1f\", ${BASH_REMATCH[1]}/1000}")
            else
                retry_delay="${retry_delay%s}"
            fi
            [[ "${retry_delay:-}" =~ ^[0-9]+\.?[0-9]*$ ]] || retry_delay=5
            [[ ${retry_delay%%.*} -lt 1 ]] && retry_delay=1
            [[ ${retry_delay%%.*} -gt 60 ]] && retry_delay=60
            __AI_RETRY_DELAY="$retry_delay"
            __AI_RETRY_PROVIDER="$provider"
            return 2
            ;;
    esac

    local error_msg
    error_msg=$(jq -r '.error.message // .error // "Unknown error"' <<<"$response" 2>/dev/null)
    __gash_error "$provider API error ($http_code): $(__ai_sanitize_text "${error_msg:-Unknown error}")"
    return 1
}

# Model for a provider: GASH_AI_MODEL_CLAUDE / GASH_AI_MODEL_GEMINI, else
# the built-in default.
# Usage: __ai_model claude|gemini
__ai_model() {
    case "$1" in
        claude) printf '%s' "${GASH_AI_MODEL_CLAUDE:-$__AI_CLAUDE_MODEL}" ;;
        gemini) printf '%s' "${GASH_AI_MODEL_GEMINI:-$__AI_GEMINI_MODEL}" ;;
    esac
}

# Drop control characters a response could use to drive the terminal
# (escape sequences, carriage returns), keeping newlines and tabs.
# Usage: __ai_sanitize_text TEXT
__ai_sanitize_text() {
    local s="${1-}"
    s="${s//[$'\001'-$'\010'$'\013'-$'\037'$'\177']/}"
    printf '%s' "$s"
}

# Same for every string inside a JSON document (prints compact JSON).
__ai_sanitize_json() {
    jq -c 'def clean: if type == "string" then gsub("[\u0001-\u0008\u000b-\u001f\u007f-\u009f]"; "")
                       elif type == "object" then with_entries(.value |= clean)
                       elif type == "array" then map(clean)
                       else . end;
           clean'
}

# Redact secrets from text before it leaves the machine: private key blocks,
# passwords in URLs, values of password/secret/token/key settings, and
# well-known token formats. Reads stdin, writes stdout.
__ai_redact() {
    "${__GASH_AWK:-awk}" '
        /-----BEGIN [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-----/ { print "[REDACTED PRIVATE KEY]"; skip = 1; next }
        skip { if ($0 ~ /-----END [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-----/) skip = 0; next }
        { print }' |
        sed -E \
            -e 's#([A-Za-z][A-Za-z0-9+.-]*://[^/:@[:space:]]*):[^/@[:space:]]+@#\1:[REDACTED]@#g' \
            -e "s/(([Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd]|[Ww][Dd])?|[Ss][Ee][Cc][Rr][Ee][Tt]|[Tt][Oo][Kk][Ee][Nn]|[Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Aa][Cc][Cc][Ee][Ss][Ss][_-]?[Kk][Ee][Yy]|[Pp][Rr][Ii][Vv][Aa][Tt][Ee][_-]?[Kk][Ee][Yy]|[Cc][Rr][Ee][Dd][Ee][Nn][Tt][Ii][Aa][Ll][Ss]?)[A-Za-z0-9_.-]*[\"']?[[:space:]]*[=:][[:space:]]*)(\"[^\"]*\"|'[^']*'|[^[:space:],;]+)/\\1[REDACTED]/g" \
            -e "s/(([Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd]|[Ww][Dd])?|[Ss][Ee][Cc][Rr][Ee][Tt]|[Tt][Oo][Kk][Ee][Nn]|[Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Aa][Cc][Cc][Ee][Ss][Ss][_-]?[Kk][Ee][Yy]|[Pp][Rr][Ii][Vv][Aa][Tt][Ee][_-]?[Kk][Ee][Yy]|[Cc][Rr][Ee][Dd][Ee][Nn][Tt][Ii][Aa][Ll][Ss]?)[[:space:]]+)[A-Za-z0-9_+\/=.-]{16,}/\\1[REDACTED]/g" \
            -e 's/AKIA[0-9A-Z]{16}/[REDACTED]/g' \
            -e 's/(gh[pousr]_|github_pat_)[A-Za-z0-9_]{20,}/[REDACTED]/g' \
            -e 's/(sk-ant-|sk-)[A-Za-z0-9_-]{20,}/[REDACTED]/g' \
            -e 's/xox[abposr]-[A-Za-z0-9-]{10,}/[REDACTED]/g' \
            -e 's/AIza[0-9A-Za-z_-]{35}/[REDACTED]/g'
}

# Ask before sending data to a provider.
# GASH_AI_CONFIRM: auto (default) asks before sending data Gash collected
# itself (ai_sysinfo), never for what the user typed or piped; 1 always
# asks; 0 never asks.
# Without a terminal to ask on, "auto" and "1" refuse to send.
# Usage: __ai_confirm_send PROVIDER KIND WHAT BYTES   (KIND: query|data)
__ai_confirm_send() {
    local provider="$1" kind="$2" what="$3" bytes="${4:-0}"
    local mode="${GASH_AI_CONFIRM:-auto}"
    [[ "$mode" == 0 ]] && return 0
    [[ "$mode" == auto && "$kind" == query ]] && return 0

    local size="${bytes} B"
    ((bytes >= 1024)) && size="$(((bytes + 512) / 1024)) KB"
    if ! { : </dev/tty; } 2>/dev/null || [[ "${GASH_HEADLESS:-0}" == 1 ]]; then
        __gash_error "Not sending $what ($size) to $provider: no terminal to confirm on. Set GASH_AI_CONFIRM=0 in ~/.gash_env to allow it"
        return 1
    fi
    local answer=""
    printf '%s' "Send $what ($size, secrets redacted) to $provider ($(__ai_model "$provider"))? [y/N] " >/dev/tty
    IFS= read -r answer </dev/tty || answer=""
    [[ "$answer" == [yY] || "$answer" == [yY][eE][sS] ]]
}

# Send one structured-output request and print the model's JSON reply.
# The user text is redacted before sending; the request body travels in a
# private temp file (never argv) and the API key through a curl config
# read from a pipe. Rate limits, overloads and transient 5xx are retried
# up to three times, honoring Retry-After.
# Usage: __ai_request PROVIDER TOKEN SYSTEM USER SCHEMA MAX_TOKENS TIMEOUT
__ai_request() {
    local __ai_tmp_body __ai_tmp_out __ai_tmp_err __ai_tmp_hdr rc=0
    __ai_tmp_body=$(umask 077 && mktemp) || return 1
    __ai_tmp_out=$(mktemp) && __ai_tmp_err=$(mktemp) && __ai_tmp_hdr=$(mktemp) || {
        rm -f "$__ai_tmp_body" "${__ai_tmp_out-}" "${__ai_tmp_err-}"
        return 1
    }
    __ai_request_send "$@" || rc=$?
    rm -f "$__ai_tmp_body" "$__ai_tmp_out" "$__ai_tmp_err" "$__ai_tmp_hdr"
    return "$rc"
}

# Body of __ai_request; uses its temp files (__ai_tmp_*).
__ai_request_send() {
    local provider="$1" token="$2" system="$3" user="$4" schema="$5"
    local max_tokens="${6:-1024}" timeout="${7:-$__AI_RESPONSE_TIMEOUT}"
    local label="Claude" model url key_header
    model="$(__ai_model "$provider")"
    case "$provider" in
        claude)
            url="$__AI_CLAUDE_API"
            key_header="x-api-key"
            ;;
        gemini)
            label="Gemini"
            url="${__AI_GEMINI_BASE}/${model}:generateContent"
            key_header="x-goog-api-key"
            ;;
        *)
            __gash_error "Unknown AI provider: $provider"
            return 1
            ;;
    esac

    user="$(printf '%s\n' "$user" | __ai_redact)"

    local body
    if [[ "$provider" == claude ]]; then
        # Claude 5 models think by default; terminal answers need little of
        # it, so effort is low there (older models reject the field).
        local effort=""
        [[ "$model" =~ ^claude-[a-z]+-5([-.]|$) ]] && effort="low"
        body=$(jq -cn --arg model "$model" --arg system "$system" --arg user "$user" \
            --argjson max_tokens "$max_tokens" --argjson schema "$schema" --arg effort "$effort" \
            '{model: $model, max_tokens: $max_tokens, system: $system,
              messages: [{role: "user", content: $user}],
              output_config: ({format: {type: "json_schema", schema: $schema}}
                              + (if $effort == "" then {} else {effort: $effort} end))}') || return 1
    else
        body=$(jq -cn --arg system "$system" --arg user "$user" \
            --argjson max_tokens "$max_tokens" --argjson schema "$schema" \
            '{systemInstruction: {parts: [{text: $system}]},
              contents: [{role: "user", parts: [{text: $user}]}],
              generationConfig: {maxOutputTokens: $max_tokens,
                                 responseMimeType: "application/json",
                                 responseJsonSchema: $schema}}') || return 1
    fi
    __ai_debug "$label Request Body" "$(jq -C . <<<"$body" 2>/dev/null || printf '%s' "$body")"

    local tmp_body="$__ai_tmp_body" tmp_out="$__ai_tmp_out" tmp_err="$__ai_tmp_err" tmp_hdr="$__ai_tmp_hdr"
    printf '%s' "$body" >"$tmp_body"

    local -a extra=()
    [[ "$provider" == claude ]] && extra=(-H "anthropic-version: 2023-06-01")

    local http_code curl_exit curl_error response start elapsed rc attempt=0 max_retries=3
    while true; do
        curl_exit=""
        start=$(date +%s)
        http_code=$(curl -sS -w "%{http_code}" -o "$tmp_out" -D "$tmp_hdr" \
            --connect-timeout "$__AI_CONNECT_TIMEOUT" --max-time "$timeout" \
            -H "Content-Type: application/json" "${extra[@]}" \
            --config <(printf 'header = "%s: %s"\n' "$key_header" "$token") \
            --data-binary "@$tmp_body" \
            "$url" 2>"$tmp_err") || curl_exit=$?
        elapsed=$(($(date +%s) - start))
        response=$(<"$tmp_out")
        curl_error=$(<"$tmp_err")

        __ai_debug "$label HTTP Code" "$http_code (elapsed: ${elapsed}s)"
        __ai_debug "$label Raw Response" "$(jq -C . <<<"$response" 2>/dev/null || printf '%s' "$response")"
        [[ -n "$curl_error" ]] && __ai_debug "$label Curl Stderr" "$curl_error"

        rc=0
        __ai_handle_curl_error "$label" "$curl_exit" "$http_code" "$elapsed" "$response" "$curl_error" "$tmp_hdr" || rc=$?
        if ((rc == 0)); then
            break
        elif ((rc == 2 && attempt < max_retries)); then
            attempt=$((attempt + 1))
            __gash_warning "$label API busy (HTTP $http_code), retrying in ${__AI_RETRY_DELAY}s (${attempt}/${max_retries})..."
            sleep "$__AI_RETRY_DELAY"
        elif ((rc == 2)); then
            __gash_error "$label API still busy (HTTP $http_code) after ${max_retries} retries"
            return 1
        else
            return 1
        fi
    done

    # Text of the reply (thinking blocks and thought parts skipped), plus
    # the reason generation stopped.
    local text stop
    if [[ "$provider" == claude ]]; then
        text=$(jq -r '[.content[]? | select(.type == "text") | .text] | join("")' <<<"$response" 2>/dev/null)
        stop=$(jq -r '.stop_reason // empty' <<<"$response" 2>/dev/null)
        case "$stop" in
            refusal)
                __gash_error "Claude declined this request"
                return 1
                ;;
            max_tokens)
                __gash_error "Claude's answer was cut off (max_tokens $max_tokens reached)"
                return 1
                ;;
        esac
    else
        text=$(jq -r '[.candidates[0].content.parts[]? | select(.thought != true) | .text // empty] | join("")' <<<"$response" 2>/dev/null)
        stop=$(jq -r '.candidates[0].finishReason // empty' <<<"$response" 2>/dev/null)
        case "$stop" in
            SAFETY | PROHIBITED_CONTENT | BLOCKLIST | SPII)
                __gash_error "Gemini declined this request ($stop)"
                return 1
                ;;
            MAX_TOKENS)
                __gash_error "Gemini's answer was cut off (maxOutputTokens $max_tokens reached)"
                return 1
                ;;
        esac
    fi

    __ai_debug "$label Extracted JSON" "$(jq -C . <<<"$text" 2>/dev/null || printf '%s' "$text")"
    if [[ -z "$text" ]] || ! jq -e . >/dev/null 2>&1 <<<"$text"; then
        __gash_error "Failed to parse $label response"
        return 1
    fi
    __ai_sanitize_json <<<"$text"
}
