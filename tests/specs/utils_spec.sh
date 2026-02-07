#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Core Utils - __gash_json_escape"

it "__gash_json_escape escapes backslashes" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    result=$(__gash_json_escape "path\\to\\file")
    [[ "$result" == "path\\\\to\\\\file" ]]
'

it "__gash_json_escape escapes double quotes" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    result=$(__gash_json_escape "say \"hello\"")
    [[ "$result" == "say \\\"hello\\\"" ]]
'

it "__gash_json_escape escapes newlines and tabs" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    input=$'"'"'line1\nline2\ttab'"'"'
    result=$(__gash_json_escape "$input")
    [[ "$result" == *"\\n"* ]]
    [[ "$result" == *"\\t"* ]]
'

it "__gash_json_escape handles empty string" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    result=$(__gash_json_escape "")
    [[ -z "$result" ]]
'

describe "Core Utils - __gash_no_history"

it "__gash_no_history preserves return code on success" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    __gash_no_history true
    rc=$?
    [[ $rc -eq 0 ]]
'

it "__gash_no_history preserves return code on failure" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    set +e
    __gash_no_history false
    rc=$?
    set -e
    [[ $rc -ne 0 ]]
'

it "__gash_no_history passes arguments to wrapped command" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"

    my_func() { [[ "$1" == "hello" ]] && [[ "$2" == "world" ]]; }

    __gash_no_history my_func "hello" "world"
'
