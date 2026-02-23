#!/usr/bin/env bash

# Tests for the Gash help system (lib/core/help.sh + gash_help in gash.sh)

describe "Help System - Registry"

it "__gash_register_help stores help text" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    [[ -n "${__GASH_HELP_REGISTRY[files_largest]+x}" ]]
'

it "__gash_register_help stores short description" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    [[ -n "${__GASH_HELP_SHORT[files_largest]}" ]]
'

it "__gash_register_help stores aliases" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    [[ "${__GASH_HELP_ALIASES[files_largest]}" == *"flf"* ]]
'

it "__gash_register_help stores module" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    [[ "${__GASH_HELP_MODULE[files_largest]}" == "files" ]]
'

it "__gash_register_help stores see-also" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    [[ -n "${__GASH_HELP_SEE_ALSO[files_largest]}" ]]
'

it "__gash_register_help rejects empty func name" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    set +e
    __gash_register_help "" --short "test" <<< "body"
    rc=$?
    set -e
    [[ $rc -eq 1 ]]
'

it "help registry is populated with >40 entries after full load" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

    count="${#__GASH_HELP_REGISTRY[@]}"
    [[ $count -gt 40 ]]
'

describe "Help System - Display"

it "gash_help shows help for known function" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help files_largest 2>&1)"
    [[ "$out" == *"USAGE"* ]]
    [[ "$out" == *"EXAMPLES"* ]]
    [[ "$out" == *"files_largest"* ]]
'

it "gash_help resolves alias to function" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help flf 2>&1)"
    [[ "$out" == *"files_largest"* ]]
    [[ "$out" == *"alias"* ]]
'

it "gash_help unknown function returns error" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    set +e
    out="$(gash_help nonexistent_func_xyz 2>&1)"
    rc=$?
    set -e
    [[ $rc -ne 0 ]]
    [[ "$out" == *"No help found"* ]]
'

it "gash_help no args shows overview" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help 2>&1)"
    [[ "$out" == *"Gash"* ]]
    [[ "$out" == *"FILES"* ]]
    [[ "$out" == *"SYSTEM"* ]]
'

it "gash_help --short returns one line" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --short files_largest 2>&1)"
    line_count=$(echo "$out" | wc -l)
    [[ $line_count -eq 1 ]]
    [[ -n "$out" ]]
'

it "gash_help --bash delegates to builtin help" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --bash cd 2>&1)"
    [[ "$out" == *"cd"* ]]
'

it "gash_help -h shows self-help" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help -h 2>&1)"
    [[ "$out" == *"gash_help"* ]]
    [[ "$out" == *"--list"* ]]
    [[ "$out" == *"--search"* ]]
'

describe "Help System - Search"

it "gash_help --search finds by function name" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --search docker 2>&1)"
    [[ "$out" == *"docker_stop_all"* ]]
'

it "gash_help --search finds by alias" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --search flf 2>&1)"
    [[ "$out" == *"files_largest"* ]]
'

it "gash_help --search no results shows message" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    set +e
    out="$(gash_help --search xyznonexistent 2>&1)"
    rc=$?
    set -e
    [[ "$out" == *"No results"* ]]
'

describe "Help System - List"

it "gash_help --list shows grouped output" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --list 2>&1)"
    [[ "$out" == *"FILES"* ]]
    [[ "$out" == *"SYSTEM"* ]]
    [[ "$out" == *"GIT"* ]]
    [[ "$out" == *"files_largest"* ]]
'

it "gash_help --list includes aliases in output" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --list 2>&1)"
    [[ "$out" == *"flf"* ]]
    [[ "$out" == *"dsa"* ]]
'

describe "Help System - Integration"

it "ai_query help includes multi-piping examples" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help ai_query 2>&1)"
    [[ "$out" == *"MULTI-SOURCE PIPING"* ]]
    [[ "$out" == *"ai_query"* ]]
'

it "gash_help --short resolves alias" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
    export GASH_VERSION="test"

    out="$(gash_help --short flf 2>&1)"
    [[ -n "$out" ]]
    [[ "$out" == *"largest"* ]]
'
