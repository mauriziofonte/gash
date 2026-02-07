#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Gash Module"

it "gash_doctor shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/modules/gash.sh"
    export GASH_DIR="$ROOT"

    out="$(gash_doctor -h)"
    [[ "$out" == *"gash_doctor"* ]]
    [[ "$out" == *"health checks"* ]]
'

it "gash_doctor runs without errors when GASH_DIR is set" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/modules/gash.sh"
    export GASH_DIR="$ROOT"
    export GASH_VERSION="test"

    out="$(gash_doctor 2>&1)"
    rc=$?
    [[ $rc -eq 0 ]]
    [[ "$out" == *"Core Files"* ]]
    [[ "$out" == *"Modules"* ]]
    [[ "$out" == *"External Tools"* ]]
'

it "gash_doctor detects missing modules gracefully" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/modules/gash.sh"
    export GASH_DIR="/tmp/nonexistent_gash_dir_$$"
    export GASH_VERSION="test"

    out="$(gash_doctor 2>&1)"
    # Should still complete (rc=0) even if files are missing
    [[ "$out" == *"MISSING"* ]] || [[ "$out" == *"not found"* ]]
'

it "gdoctor alias resolves to gash_doctor" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    shopt -s expand_aliases
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/validation.sh"
    source "$ROOT/lib/modules/gash.sh"

    alias_def="$(alias gdoctor 2>/dev/null)"
    [[ "$alias_def" == *"gash_doctor"* ]]
'
