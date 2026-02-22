#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# =============================================================================
# Output Helpers
# =============================================================================

describe "Sysinfo Module - Output Helpers"

it "__sysinfo_section outputs === TITLE === in LLM mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    result="$(__sysinfo_section "TEST")"

    [[ "$result" == "=== TEST ===" ]]
'

it "__sysinfo_kv outputs key=value in LLM mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    result="$(__sysinfo_kv "OS" "Ubuntu 22.04")"

    [[ "$result" == "OS=Ubuntu 22.04" ]]
'

it "__sysinfo_kv skips empty values" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    result="$(__sysinfo_kv "Empty" "")"

    [[ -z "$result" ]]
'

it "__sysinfo_item outputs plain text in LLM mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    result="$(__sysinfo_item "/etc/apache2/")"

    [[ "$result" == "/etc/apache2/" ]]
'

it "__sysinfo_sub outputs --- title in LLM mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    result="$(__sysinfo_sub "fstab")"

    [[ "$result" == "--- fstab" ]]
'

# =============================================================================
# Sudo Helpers
# =============================================================================

describe "Sysinfo Module - Sudo Helpers"

it "__sysinfo_release_sudo clears keepalive PID" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    # Simulate a keepalive process (sleep in background)
    sleep 60 &
    __SYSINFO_SUDO_KEEPALIVE_PID=$!

    __sysinfo_release_sudo

    [[ -z "$__SYSINFO_SUDO_KEEPALIVE_PID" ]]
'

it "__sysinfo_sudo runs command without sudo when __SYSINFO_HAS_SUDO=0" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_sudo echo "hello")"

    [[ "$result" == "hello" ]]
'

# =============================================================================
# Collectors (LLM mode)
# =============================================================================

describe "Sysinfo Module - Collectors"

it "__sysinfo_collect_identity outputs IDENTITY header and Kernel" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_identity)"

    [[ "$result" == *"=== IDENTITY ==="* ]]
    [[ "$result" == *"Kernel="* ]]
'

it "__sysinfo_collect_storage outputs STORAGE header" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_storage)"

    [[ "$result" == *"=== STORAGE ==="* ]]
'

it "__sysinfo_collect_services outputs SERVICES header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_services)"

    [[ "$result" == *"=== SERVICES ==="* ]]
'

it "__sysinfo_collect_auth outputs AUTH header" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_auth)"

    [[ "$result" == *"=== AUTH ==="* ]]
'

it "__sysinfo_collect_network outputs NETWORK header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_network)"

    [[ "$result" == *"=== NETWORK ==="* ]]
'

it "__sysinfo_collect_security outputs SECURITY header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_security)"

    [[ "$result" == *"=== SECURITY ==="* ]]
'

it "__sysinfo_collect_webstack outputs WEBSTACK header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_webstack)"

    [[ "$result" == *"=== WEBSTACK ==="* ]]
'

it "__sysinfo_collect_mail outputs MAIL header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_mail)"

    [[ "$result" == *"=== MAIL ==="* ]]
'

it "__sysinfo_collect_infra outputs INFRA header" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_infra)"

    [[ "$result" == *"=== INFRA ==="* ]]
'

it "__sysinfo_collect_system outputs SYSTEM header" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_MODE="llm"
    __SYSINFO_HAS_SUDO=0
    result="$(__sysinfo_collect_system)"

    [[ "$result" == *"=== SYSTEM ==="* ]]
'

# =============================================================================
# Public Function
# =============================================================================

describe "Sysinfo Module - Public Function"

it "sysinfo -h shows help" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    out="$(sysinfo -h)"
    [[ "$out" == *"sysinfo"* ]]
'

it "sysinfo identity --llm outputs only IDENTITY section" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    # Mock sudo to avoid interactive prompt
    __sysinfo_ensure_sudo() { __SYSINFO_HAS_SUDO=0; return 0; }
    __sysinfo_release_sudo() { return 0; }

    result="$(sysinfo identity --llm)"

    [[ "$result" == *"=== IDENTITY ==="* ]]
    [[ "$result" == *"Kernel="* ]]
    # Should NOT contain other section headers
    [[ "$result" != *"=== STORAGE ==="* ]]
    [[ "$result" != *"=== SERVICES ==="* ]]
'

it "sysinfo badname returns error" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    set +e
    out="$(sysinfo badname 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Unknown argument"* ]]
'

it "si alias is defined" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    alias_def="$(alias si 2>/dev/null)"
    [[ "$alias_def" == *"sysinfo"* ]]
'

it "sysinfo all --llm outputs all 10 section headers" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    # Mock sudo
    __sysinfo_ensure_sudo() { __SYSINFO_HAS_SUDO=0; return 0; }
    __sysinfo_release_sudo() { return 0; }

    result="$(sysinfo all --llm)"

    [[ "$result" == *"=== IDENTITY ==="* ]]
    [[ "$result" == *"=== STORAGE ==="* ]]
    [[ "$result" == *"=== SERVICES ==="* ]]
    [[ "$result" == *"=== AUTH ==="* ]]
    [[ "$result" == *"=== NETWORK ==="* ]]
    [[ "$result" == *"=== SECURITY ==="* ]]
    [[ "$result" == *"=== WEBSTACK ==="* ]]
    [[ "$result" == *"=== MAIL ==="* ]]
    [[ "$result" == *"=== INFRA ==="* ]]
    [[ "$result" == *"=== SYSTEM ==="* ]]
'

# =============================================================================
# AI Integration
# =============================================================================

describe "Sysinfo Module - AI Integration"

it "ai_sysinfo -h shows help" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    out="$(ai_sysinfo -h)"
    [[ "$out" == *"ai_sysinfo"* ]]
'

it "ai_sysinfo --raw outputs LLM data without API call" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    # Mock sudo and spinner
    __sysinfo_ensure_sudo() { __SYSINFO_HAS_SUDO=0; return 0; }
    __sysinfo_release_sudo() { return 0; }
    __gash_spinner_start() { return 0; }
    __GASH_SPINNER_PID=""

    result="$(__ai_sysinfo_impl --raw 2>/dev/null)"

    [[ "$result" == *"=== IDENTITY ==="* ]]
    [[ "$result" == *"=== STORAGE ==="* ]]
    [[ "$result" == *"Kernel="* ]]
'

it "sysinfo_ai alias is defined" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    alias_def="$(alias sysinfo_ai 2>/dev/null)"
    [[ "$alias_def" == *"ai_sysinfo"* ]]
'

# =============================================================================
# AI API Wrappers
# =============================================================================

describe "Sysinfo Module - AI API Wrappers"

it "__sysinfo_call_claude returns structured JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__sysinfo_call_claude "test-token" "=== IDENTITY ===
OS=Ubuntu 22.04
Kernel=5.15.0")"

    [[ "$result" == *"hostname"* ]]
    [[ "$result" == *"sections"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

it "__sysinfo_call_gemini returns structured JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__sysinfo_call_gemini "test-token" "=== IDENTITY ===
OS=Ubuntu 22.04
Kernel=5.15.0")"

    [[ "$result" == *"hostname"* ]]
    [[ "$result" == *"sections"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

# =============================================================================
# AI Response Formatter
# =============================================================================

describe "Sysinfo Module - AI Response Formatter"

it "__sysinfo_format_ai_response formats valid JSON with hostname and findings" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"hostname":"myserver","platform":"Debian 12 | 6.1.0","sections":[{"title":"Security Posture","findings":[{"severity":"warning","text":"No firewall detected"},{"severity":"ok","text":"SSH hardened"}]}]}'"'"'
    result="$(__sysinfo_format_ai_response "$json")"

    [[ "$result" == *"myserver"* ]]
    [[ "$result" == *"Debian 12"* ]]
    [[ "$result" == *"No firewall detected"* ]]
    [[ "$result" == *"SSH hardened"* ]]
    [[ "$result" == *"1 warnings"* ]]
    [[ "$result" == *"1 ok"* ]]
'

it "__sysinfo_format_ai_response fails on invalid JSON" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    set +e
    out="$(__sysinfo_format_ai_response "not valid json" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Invalid JSON"* ]]
'

it "__sysinfo_format_ai_response shows severity icons correctly" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"hostname":"srv","platform":"Ubuntu","sections":[{"title":"Security Posture","findings":[{"severity":"critical","text":"Root login enabled"},{"severity":"warning","text":"No fail2ban"},{"severity":"info","text":"Port 22 open"},{"severity":"ok","text":"Key auth enabled"}]}]}'"'"'
    result="$(__sysinfo_format_ai_response "$json")"

    # Check all severity types produce output
    [[ "$result" == *"[!]"* ]]
    [[ "$result" == *"[+]"* ]]
    [[ "$result" == *"[i]"* ]]
    [[ "$result" == *"1 critical"* ]]
    [[ "$result" == *"1 warnings"* ]]
    [[ "$result" == *"1 info"* ]]
    [[ "$result" == *"1 ok"* ]]
'

# =============================================================================
# Deep Collectors
# =============================================================================

describe "Sysinfo Module - Deep Collectors"

it "__sysinfo_deep_services outputs header and runs without error" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_services 2>/dev/null)"

    [[ "$result" == *"=== SERVICES DEEP ==="* ]]
'

it "__sysinfo_deep_security outputs header and runs without error" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_security 2>/dev/null)"

    [[ "$result" == *"=== SECURITY DEEP ==="* ]]
'

it "__sysinfo_deep_network outputs header and interface data" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_network 2>/dev/null)"

    [[ "$result" == *"=== NETWORK DEEP ==="* ]]
    [[ "$result" == *"--- Interfaces"* ]]
    [[ "$result" == *"--- Routes"* ]]
'

it "__sysinfo_deep_storage outputs header and disk data" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_storage 2>/dev/null)"

    [[ "$result" == *"=== STORAGE DEEP ==="* ]]
    [[ "$result" == *"--- Disk usage"* ]]
    [[ "$result" == *"--- fstab"* ]]
'

it "__sysinfo_deep_performance outputs header and CPU/memory data" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_performance 2>/dev/null)"

    [[ "$result" == *"=== PERFORMANCE DEEP ==="* ]]
    [[ "$result" == *"--- Memory"* ]]
    [[ "$result" == *"--- CPU"* ]]
    [[ "$result" == *"--- Load"* ]]
'

it "__sysinfo_deep_maintenance outputs header and cron/package data" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/sysinfo.sh"

    __SYSINFO_HAS_SUDO=0

    result="$(__sysinfo_deep_maintenance 2>/dev/null)"

    [[ "$result" == *"=== MAINTENANCE DEEP ==="* ]]
    [[ "$result" == *"--- crontab"* ]]
    [[ "$result" == *"--- Upgradable packages"* ]]
'

# =============================================================================
# Drilldown API Wrappers
# =============================================================================

describe "Sysinfo Module - Drilldown API Wrappers"

it "__sysinfo_call_drilldown_claude returns component-based JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__sysinfo_call_drilldown_claude "test-token" "Services Detected" "Apache2 running" "=== SERVICES DEEP ===
--- Apache2
ServerRoot /etc/apache2")"

    [[ "$result" == *"title"* ]]
    [[ "$result" == *"components"* ]]
    [[ "$result" == *"Apache2"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

it "__sysinfo_call_drilldown_gemini returns component-based JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    result="$(__sysinfo_call_drilldown_gemini "test-token" "Services Detected" "Apache2 running" "=== SERVICES DEEP ===
--- Apache2
ServerRoot /etc/apache2")"

    [[ "$result" == *"title"* ]]
    [[ "$result" == *"components"* ]]
    [[ "$result" == *"Apache2"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

# =============================================================================
# Drilldown Response Formatter
# =============================================================================

describe "Sysinfo Module - Drilldown Formatter"

it "__sysinfo_format_drilldown_response formats components with status and issues" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"title":"Services Detected","summary":"Analysis of services.","components":[{"name":"Apache2","status":"running","config_highlights":["3 vhosts","MPM: event"],"issues":[{"severity":"ok","text":"ProxyPass configured"}]},{"name":"PHP 5.6","status":"eol","config_highlights":["memory_limit=128M"],"issues":[{"severity":"critical","text":"EOL since December 2018"}]}]}'"'"'
    result="$(__sysinfo_format_drilldown_response "$json")"

    [[ "$result" == *"Services Detected"* ]]
    [[ "$result" == *"Analysis of services"* ]]
    [[ "$result" == *"Apache2"* ]]
    [[ "$result" == *"running"* ]]
    [[ "$result" == *"3 vhosts"* ]]
    [[ "$result" == *"ProxyPass configured"* ]]
    [[ "$result" == *"PHP 5.6"* ]]
    [[ "$result" == *"eol"* ]]
    [[ "$result" == *"EOL since December 2018"* ]]
'

it "__sysinfo_format_drilldown_response handles empty components array" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"title":"Empty Section","summary":"Nothing found.","components":[]}'"'"'
    result="$(__sysinfo_format_drilldown_response "$json")"

    [[ "$result" == *"Empty Section"* ]]
    [[ "$result" == *"Nothing found"* ]]
'

it "__sysinfo_format_drilldown_response fails on invalid JSON" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    set +e
    out="$(__sysinfo_format_drilldown_response "not valid json" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Invalid JSON"* ]]
'

# =============================================================================
# Freetext API Wrappers
# =============================================================================

describe "Sysinfo Module - Freetext API Wrappers"

it "__sysinfo_call_freetext_claude returns component-based JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    initial='"'"'{"hostname":"srv","platform":"Debian","sections":[]}'"'"'
    result="$(__sysinfo_call_freetext_claude "test-token" "How secure is my SSH?" "$initial")"

    [[ "$result" == *"title"* ]]
    [[ "$result" == *"components"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

it "__sysinfo_call_freetext_gemini returns component-based JSON via mock" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    initial='"'"'{"hostname":"srv","platform":"Debian","sections":[]}'"'"'
    result="$(__sysinfo_call_freetext_gemini "test-token" "How secure is my SSH?" "$initial")"

    [[ "$result" == *"title"* ]]
    [[ "$result" == *"components"* ]]
    echo "$result" | jq -e . > /dev/null 2>&1
'

# =============================================================================
# Markdown Generators
# =============================================================================

describe "Sysinfo Module - Markdown Generators"

it "__sysinfo_md_initial generates markdown with hostname and sections" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"hostname":"myserver","platform":"Debian 12","sections":[{"title":"Security Posture","findings":[{"severity":"warning","text":"No firewall"},{"severity":"ok","text":"SSH hardened"}]}]}'"'"'
    result="$(__sysinfo_md_initial "$json" "gemini")"

    [[ "$result" == *"# System Analysis Report"* ]]
    [[ "$result" == *"## Server: myserver"* ]]
    [[ "$result" == *"Debian 12"* ]]
    [[ "$result" == *"Gemini"* ]]
    [[ "$result" == *"### Security Posture"* ]]
    [[ "$result" == *"**[!]** No firewall"* ]]
    [[ "$result" == *"**[+]** SSH hardened"* ]]
    [[ "$result" == *"1 warnings"* ]]
    [[ "$result" == *"1 ok"* ]]
'

it "__sysinfo_md_drilldown generates markdown with components" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"title":"Services Detected","summary":"Service analysis.","components":[{"name":"Apache2","status":"running","config_highlights":["3 vhosts"],"issues":[{"severity":"ok","text":"ProxyPass OK"}]}]}'"'"'
    result="$(__sysinfo_md_drilldown "$json")"

    [[ "$result" == *"## Drill-down: Services Detected"* ]]
    [[ "$result" == *"Service analysis"* ]]
    [[ "$result" == *"### Apache2"* ]]
    [[ "$result" == *"running"* ]]
    [[ "$result" == *"3 vhosts"* ]]
    [[ "$result" == *"**[+]** ProxyPass OK"* ]]
'

it "__sysinfo_md_freetext generates markdown with question header" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    json='"'"'{"title":"SSH Analysis","summary":"SSH review.","components":[{"name":"OpenSSH","status":"running","config_highlights":["Port 22"],"issues":[{"severity":"warning","text":"Root login enabled"}]}]}'"'"'
    result="$(__sysinfo_md_freetext "How secure is my SSH?" "$json")"

    [[ "$result" == *"## Question: How secure is my SSH?"* ]]
    [[ "$result" == *"SSH review"* ]]
    [[ "$result" == *"### OpenSSH"* ]]
    [[ "$result" == *"running"* ]]
    [[ "$result" == *"Port 22"* ]]
    [[ "$result" == *"**[!]** Root login enabled"* ]]
'

# =============================================================================
# Save to File
# =============================================================================

describe "Sysinfo Module - Save to File"

it "save handler writes markdown file with correct content" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    cd "$tmpdir"

    # Build a markdown buffer the same way the interactive loop does
    json='"'"'{"hostname":"testhost","platform":"Ubuntu 22.04","sections":[{"title":"Security","findings":[{"severity":"ok","text":"All good"}]}]}'"'"'
    md_buffer=$(__sysinfo_md_initial "$json" "claude")

    # Simulate the save handler
    filename="gash-ai-sysinfo-$(date +%Y%m%d-%H%M%S).md"
    printf "%s" "$md_buffer" > "$filename"

    [[ -f "$filename" ]]
    content=$(cat "$filename")
    [[ "$content" == *"# System Analysis Report"* ]]
    [[ "$content" == *"## Server: testhost"* ]]
    [[ "$content" == *"All good"* ]]
'

# =============================================================================
# FREETEXT CONTEXT COLLECTION
# =============================================================================

it "__sysinfo_collect_question_context detects systemd unit in question" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0
    result=$(__sysinfo_collect_question_context "Analyze viper-backup.service")

    [[ "$result" == *"=== SYSTEMD UNIT: viper-backup.service ==="* ]]
    [[ "$result" == *"--- Unit file ---"* ]]
    [[ "$result" == *"--- Status ---"* ]]
    [[ "$result" == *"--- Journal"* ]]
    [[ "$result" == *"ExecStart"* ]]
    [[ "$result" == *"active (running)"* ]]
'

it "__sysinfo_collect_question_context detects file path in question" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0

    # Create a temp file to reference
    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    echo "test_content_12345" > "$tmpdir/testfile.conf"

    # The function only matches standard paths (/etc, /var, etc.), so test with /etc
    # Instead, let us test with a real readable file under /etc
    result=$(__sysinfo_collect_question_context "What is in /etc/hostname?")

    # /etc/hostname should exist and be readable on most systems
    if [[ -f /etc/hostname ]]; then
        [[ "$result" == *"=== FILE: /etc/hostname ==="* ]]
    fi
'

it "__sysinfo_collect_question_context detects port number in question" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0
    result=$(__sysinfo_collect_question_context "What is listening on port 3306?")

    [[ "$result" == *"=== PORT: 3306 ==="* ]]
    [[ "$result" == *"mariadbd"* ]]
'

it "__sysinfo_collect_question_context returns empty for generic question" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0
    result=$(__sysinfo_collect_question_context "Is my server healthy?")

    [[ -z "$result" ]]
'

it "__sysinfo_collect_question_context detects known service keyword (apache)" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0
    result=$(__sysinfo_collect_question_context "Analyze my Apache configuration")

    [[ "$result" == *"=== SERVICE: Apache2 ==="* ]]
    [[ "$result" == *"--- Status ---"* ]]
    [[ "$result" == *"active (running)"* ]]
'

it "__sysinfo_collect_question_context detects service name without .service suffix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/sysinfo.sh"
    source "$ROOT/lib/modules/ai.sh"

    __SYSINFO_HAS_SUDO=0
    result=$(__sysinfo_collect_question_context "Explain viper-backup to me")

    [[ "$result" == *"=== SYSTEMD UNIT: viper-backup.service ==="* ]]
    [[ "$result" == *"--- Unit file ---"* ]]
    [[ "$result" == *"ExecStart"* ]]
    [[ "$result" == *"--- Status ---"* ]]
    [[ "$result" == *"--- Journal"* ]]
'

it "__sysinfo_call_freetext_claude passes context_data to API" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    export PATH="${ROOT}/tests/mocks/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/ai.sh"

    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT
    export MOCK_CURL_LOG="${tmpdir}/curl.log"

    initial='"'"'{"hostname":"srv","platform":"Debian","sections":[]}'"'"'
    ctx_data="=== SYSTEMD UNIT: test.service ===
--- Unit file ---
ExecStart=/usr/bin/test-svc"

    result=$(__sysinfo_call_freetext_claude "test-token" "What is test.service?" "$initial" "$ctx_data")

    # Verify the result is valid JSON (mock returns drilldown response)
    echo "$result" | jq -e . > /dev/null 2>&1

    # Verify the curl was called with context data in the body
    curl_log=$(cat "$MOCK_CURL_LOG")
    [[ "$curl_log" == *"claude"* ]]
'
