#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Core Validation"

it "__gash_require_command succeeds for existing command" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    __gash_require_command "bash"
'

it "__gash_require_command fails for missing command" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_command "nonexistent_command_xyz" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"not installed"* ]] || [[ "$out" == *"not in PATH"* ]]
'

it "__gash_require_command uses custom error message" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_command "nonexistent" "Custom error here" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"Custom error here"* ]]
'

it "__gash_require_file succeeds for existing file" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp)"
    trap "/bin/rm -f $tmp" EXIT

    __gash_require_file "$tmp"
'

it "__gash_require_file fails for missing file" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_file "/nonexistent/path/file.txt" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"does not exist"* ]]
'

it "__gash_require_dir succeeds for existing directory" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT

    __gash_require_dir "$tmp"
'

it "__gash_require_dir fails for missing directory" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_dir "/nonexistent/path" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"does not exist"* ]]
'

it "__gash_require_git_repo fails outside git repo" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT
    cd "$tmp"

    set +e
    out="$(__gash_require_git_repo 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"git repository"* ]]
'

it "__gash_require_git_repo succeeds inside git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT
    cd "$tmp"
    git init -q

    __gash_require_git_repo
'

it "__gash_require_git_remote fails when remote missing" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT
    cd "$tmp"
    git init -q

    set +e
    out="$(__gash_require_git_remote 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"origin"* ]] && [[ "$out" == *"not configured"* ]]
'

it "__gash_require_arg fails for empty value" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_arg "" "filename" "command <filename>" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"filename"* ]]
    [[ "$out" == *"command <filename>"* ]]
'

it "__gash_require_arg succeeds for non-empty value" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    __gash_require_arg "somevalue" "filename"
'

it "__gash_require_docker checks docker command" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT

    # Create fake docker
    mkdir -p "$tmp/bin"
    echo "#!/bin/bash" > "$tmp/bin/docker"
    chmod +x "$tmp/bin/docker"

    export PATH="$tmp/bin:$PATH"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    __gash_require_docker
'

it "__gash_require_binary succeeds for existing binary" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    __gash_require_binary "bash"
'

it "__gash_require_binary fails for missing binary" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_binary "nonexistent_binary_xyz" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"not installed"* ]] || [[ "$out" == *"not in PATH"* ]]
'

it "__gash_require_binary ignores aliases and functions" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    # Create a function that would match "command -v" but not "type -P"
    fake_binary() { echo "I am a function"; }

    set +e
    out="$(__gash_require_binary "fake_binary" 2>&1)"
    rc=$?
    set -e

    # Should fail because fake_binary is a function, not a binary
    [[ $rc -ne 0 ]]
'

it "__gash_require_readable succeeds for readable file" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp)"
    trap "/bin/rm -f $tmp" EXIT
    chmod 644 "$tmp"

    __gash_require_readable "$tmp"
'

it "__gash_require_readable fails for missing file" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    set +e
    out="$(__gash_require_readable "/nonexistent/file.txt" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"not readable"* ]]
'

it "__gash_require_git_repo_with_remote fails outside git repo" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT
    cd "$tmp"

    set +e
    out="$(__gash_require_git_repo_with_remote 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"git repository"* ]]
'

it "__gash_require_git_repo_with_remote fails when remote missing" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT
    cd "$tmp"
    git init -q

    set +e
    out="$(__gash_require_git_repo_with_remote 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"origin"* ]] && [[ "$out" == *"not configured"* ]]
'

it "__gash_require_git_repo_with_remote succeeds with repo and remote" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/validation.sh"

    tmp="$(mktemp -d)"
    trap "/bin/rm -rf $tmp" EXIT

    # Create a bare origin
    git init -q --bare "$tmp/origin.git"

    # Create a working repo with remote
    mkdir "$tmp/work"
    cd "$tmp/work"
    git init -q
    git remote add origin "$tmp/origin.git"

    __gash_require_git_repo_with_remote
'
