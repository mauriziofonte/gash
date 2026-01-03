#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "LLM Utilities"

# =============================================================================
# Security Tests (CRITICAL)
# =============================================================================

it "__llm_validate_command blocks rm -rf /" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(__llm_validate_command "rm -rf /" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"dangerous_command_blocked"* ]]
'

it "__llm_validate_command blocks dd if=" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(__llm_validate_command "dd if=/dev/zero of=/dev/sda" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"dangerous_command_blocked"* ]]
'

it "__llm_validate_command blocks fork bomb" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(__llm_validate_command ":(){:|:&};:" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"dangerous_command_blocked"* ]]
'

it "__llm_validate_command allows safe commands" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    __llm_validate_command "ls -la /tmp"
    __llm_validate_command "cat /etc/hosts"
    __llm_validate_command "grep pattern file.txt"
'

it "__llm_validate_path blocks path traversal" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(__llm_validate_path "../../../etc/passwd" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"path_traversal_blocked"* ]]
'

it "__llm_validate_path blocks forbidden paths" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(__llm_validate_path "/root/.ssh" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"forbidden_path"* ]]
'

it "__llm_validate_path allows safe paths" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    result="$(__llm_validate_path "/tmp")"
    [[ "$result" == "/tmp" ]]

    result="$(__llm_validate_path ".")"
    [[ -n "$result" ]]
'

it "__llm_is_secret_file detects .env files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    __llm_is_secret_file ".env"
    __llm_is_secret_file ".env.local"
    __llm_is_secret_file "id_rsa"
'

it "__llm_is_secret_file allows regular files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    ! __llm_is_secret_file "config.json"
    ! __llm_is_secret_file "package.json"
    ! __llm_is_secret_file "README.md"
'

# =============================================================================
# llm_exec Tests
# =============================================================================

it "llm_exec blocks dangerous commands" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(llm_exec "rm -rf /" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"dangerous_command_blocked"* ]]
'

it "llm_exec executes safe commands" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    result="$(llm_exec "echo hello")"
    [[ "$result" == "hello" ]]
'

it "llm_exec shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_exec -h)"
    [[ "$out" == *"llm_exec"* ]]
    [[ "$out" == *"Execute command safely"* ]]
'

# =============================================================================
# llm_tree Tests
# =============================================================================

it "llm_tree outputs JSON by default" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_tree "$ROOT/lib" 2>/dev/null)"
    [[ "$out" == *"{"* ]]
    [[ "$out" == *"path"* ]]
    [[ "$out" == *"children"* ]]
'

it "llm_tree outputs text with --text" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_tree --text "$ROOT/lib" 2>/dev/null)"
    # Text mode should not have JSON braces at start
    [[ "$out" != "{"* ]]
'

it "llm_tree shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_tree -h)"
    [[ "$out" == *"llm_tree"* ]]
'

# =============================================================================
# llm_find Tests
# =============================================================================

it "llm_find finds files by pattern" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_find "*.sh" "$ROOT/lib" 2>/dev/null)"
    [[ "$out" == *".sh"* ]]
'

it "llm_find shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_find -h)"
    [[ "$out" == *"llm_find"* ]]
'

# =============================================================================
# llm_grep Tests
# =============================================================================

it "llm_grep finds patterns in files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_grep "function" "$ROOT/lib" --ext sh 2>/dev/null)"
    [[ -n "$out" ]]
'

it "llm_grep shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_grep -h)"
    [[ "$out" == *"llm_grep"* ]]
'

# =============================================================================
# llm_project Tests
# =============================================================================

it "llm_project detects project info" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    tmp="$(mktemp -d)"
    trap "rm -rf $tmp" EXIT

    # Create a fake PHP project
    echo "{\"require\":{\"php\":\">=8.0\"}}" > "$tmp/composer.json"

    out="$(llm_project "$tmp" 2>/dev/null)"
    [[ "$out" == *"php"* ]]
    [[ "$out" == *"composer"* ]]
'

it "llm_project shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_project -h)"
    [[ "$out" == *"llm_project"* ]]
'

# =============================================================================
# llm_config Tests
# =============================================================================

it "llm_config blocks .env files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    tmp="$(mktemp -d)"
    trap "rm -rf $tmp" EXIT
    echo "SECRET=value" > "$tmp/.env"

    set +e
    out="$(llm_config "$tmp/.env" 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"secret_file_blocked"* ]]
'

it "llm_config reads JSON files" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    tmp="$(mktemp -d)"
    trap "rm -rf $tmp" EXIT
    echo "{\"key\":\"value\"}" > "$tmp/config.json"

    out="$(llm_config "$tmp/config.json" 2>/dev/null)"
    [[ "$out" == *"key"* ]]
    [[ "$out" == *"value"* ]]
'

# =============================================================================
# llm_git_status Tests
# =============================================================================

it "llm_git_status outputs JSON in git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    tmp="$(mktemp -d)"
    trap "rm -rf $tmp" EXIT
    cd "$tmp"
    git init -q
    git config user.email "test@example.com"
    git config user.name "Test"
    echo x > f
    git add f
    git commit -q -m init

    out="$(llm_git_status "$tmp" 2>/dev/null)"
    [[ "$out" == *"branch"* ]]
    [[ "$out" == *"staged"* ]]
    [[ "$out" == *"modified"* ]]
'

it "llm_git_status shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_git_status -h)"
    [[ "$out" == *"llm_git_status"* ]]
'

# =============================================================================
# llm_git_log Tests
# =============================================================================

it "llm_git_log outputs JSON array" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    tmp="$(mktemp -d)"
    trap "rm -rf $tmp" EXIT
    cd "$tmp"
    git init -q
    git config user.email "test@example.com"
    git config user.name "Test"
    echo x > f
    git add f
    git commit -q -m "initial commit"

    out="$(llm_git_log "$tmp" 2>/dev/null)"
    [[ "$out" == "["* ]]
    [[ "$out" == *"hash"* ]]
    [[ "$out" == *"subject"* ]]
'

# =============================================================================
# llm_ports Tests
# =============================================================================

it "llm_ports outputs JSON array" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_ports 2>/dev/null)"
    [[ "$out" == "["* ]]
    [[ "$out" == *"]"* ]]
'

it "llm_ports shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_ports -h)"
    [[ "$out" == *"llm_ports"* ]]
'

# =============================================================================
# llm_env Tests
# =============================================================================

it "llm_env outputs JSON object" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_env 2>/dev/null)"
    [[ "$out" == "{"* ]]
    [[ "$out" == *"}"* ]]
'

it "llm_env filters out secrets" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    export MY_SECRET_PASSWORD="should_not_appear"
    export MY_API_KEY="should_not_appear"
    export NORMAL_VAR="should_appear"

    out="$(llm_env 2>/dev/null)"

    # Should not contain secret variables
    [[ "$out" != *"SECRET_PASSWORD"* ]]
    [[ "$out" != *"API_KEY"* ]]
'

it "llm_env shows help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/modules/llm.sh"

    out="$(llm_env -h)"
    [[ "$out" == *"llm_env"* ]]
'

# =============================================================================
# Database Tests (only structure, skip if no DB available)
# =============================================================================

it "llm_db_query blocks write operations" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(llm_db_query "INSERT INTO users VALUES (1)" -d test 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"write_operation_blocked"* ]]
'

it "llm_db_query blocks DELETE" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(llm_db_query "DELETE FROM users" -d test 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"write_operation_blocked"* ]]
'

it "llm_db_query blocks DROP" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(llm_db_query "DROP TABLE users" -d test 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"write_operation_blocked"* ]]
'

it "llm_db_schema validates table name" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"
    source "$ROOT/lib/core/utils.sh"
    source "$ROOT/lib/core/config.sh"
    source "$ROOT/lib/modules/llm.sh"

    set +e
    out="$(llm_db_schema "users; DROP TABLE users" -d test 2>&1)"
    rc=$?
    set -e

    [[ $rc -ne 0 ]]
    [[ "$out" == *"invalid_table_name"* ]]
'
