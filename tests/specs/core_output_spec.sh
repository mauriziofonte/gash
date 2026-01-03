#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Core Output"

it "defines color variables" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    [[ -n "$__GASH_COLOR_OFF" ]]
    [[ -n "$__GASH_RED" ]]
    [[ -n "$__GASH_GREEN" ]]
    [[ -n "$__GASH_CYAN" ]]
    [[ -n "$__GASH_BOLD_WHITE" ]]
'

it "__gash_info prints cyan Info prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_info "test message")"
    [[ "$out" == *"Info:"* ]]
    [[ "$out" == *"test message"* ]]
'

it "__gash_error prints red Error prefix to stderr" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_error "error message" 2>&1)"
    [[ "$out" == *"Error:"* ]]
    [[ "$out" == *"error message"* ]]
'

it "__gash_success prints green OK prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_success "done")"
    [[ "$out" == *"OK:"* ]]
    [[ "$out" == *"done"* ]]
'

it "__gash_warning prints yellow Warning prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_warning "careful")"
    [[ "$out" == *"Warning:"* ]]
    [[ "$out" == *"careful"* ]]
'

it "__gash_ssh prints SSH prefix with color variants" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out1="$(__gash_ssh "connected")"
    out2="$(__gash_ssh "key added" success)"
    out3="$(__gash_ssh "warning msg" warning)"
    out4="$(__gash_ssh "failed" error)"

    [[ "$out1" == *"SSH:"* ]]
    [[ "$out2" == *"SSH:"* ]]
    [[ "$out3" == *"SSH:"* ]]
    [[ "$out4" == *"SSH:"* ]]
'

it "__gash_step prints step indicator" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_step 1 3 "Installing...")"
    [[ "$out" == *"[1/3]"* ]]
    [[ "$out" == *"Installing..."* ]]
'

it "__gash_debug prints only when GASH_DEBUG=1" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    # Without GASH_DEBUG, should print nothing
    out1="$(__gash_debug "debug msg" 2>&1)"
    [[ -z "$out1" ]]

    # With GASH_DEBUG=1, should print
    export GASH_DEBUG=1
    out2="$(__gash_debug "debug msg" 2>&1)"
    [[ "$out2" == *"Debug:"* ]]
    [[ "$out2" == *"debug msg"* ]]
'

it "__gash_print outputs colored text without prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_print green "green text")"
    [[ "$out" == *"green text"* ]]
'

it "all output functions handle empty messages" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    # Should not fail with empty args
    __gash_info "" >/dev/null
    __gash_error "" 2>/dev/null
    __gash_success "" >/dev/null
    __gash_warning "" >/dev/null
    __gash_ssh "" >/dev/null
    __gash_step "" "" "" >/dev/null
'
