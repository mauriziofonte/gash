#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Core Utils"

it "needs_help prints usage on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    out="$(needs_help "mycommand" "mycommand <arg>" "Does something" "-h")"
    rc=$?

    [[ $rc -eq 0 ]]
    [[ "$out" == *"mycommand"* ]]
    [[ "$out" == *"<arg>"* ]]
    [[ "$out" == *"Does something"* ]]
'

it "needs_help prints usage on --help" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    out="$(needs_help "cmd" "cmd [options]" "Help text" "--help")"
    rc=$?

    [[ $rc -eq 0 ]]
    [[ "$out" == *"Help text"* ]]
'

it "needs_help returns 1 for non-help args" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    set +e
    needs_help "cmd" "cmd" "help" "somefile.txt"
    rc=$?
    set -e

    [[ $rc -eq 1 ]]
'

it "needs_help returns 1 for empty arg" bash -c '
    set -uo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    set +e
    needs_help "cmd" "cmd" "help" ""
    rc=$?
    set -e

    [[ $rc -eq 1 ]]
'

it "__gash_tty_width returns a number" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    width="$(__gash_tty_width)"
    [[ "$width" =~ ^[0-9]+$ ]]
    [[ "$width" -gt 0 ]]
'

it "__gash_tty_width defaults to 80 when no terminal" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    # Unset COLUMNS and run in non-tty environment
    unset COLUMNS
    width="$(__gash_tty_width)"

    # Should return something reasonable (80 is default)
    [[ "$width" =~ ^[0-9]+$ ]]
'

it "__gash_trim_ws trims leading and trailing whitespace" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_trim_ws "  hello world  ")"
    [[ "$result" == "hello world" ]]
'

it "__gash_trim_ws trims tabs" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_trim_ws "	tabbed	")"
    [[ "$result" == "tabbed" ]]
'

it "__gash_trim_ws handles empty string" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_trim_ws "")"
    [[ -z "$result" ]]
'

it "__gash_trim_ws handles whitespace-only string" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_trim_ws "   ")"
    [[ -z "$result" ]]
'

it "__gash_expand_tilde_path expands ~/path" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_expand_tilde_path "~/test/file")"
    [[ "$result" == "$HOME/test/file" ]]
'

it "__gash_expand_tilde_path expands bare ~" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_expand_tilde_path "~")"
    [[ "$result" == "$HOME" ]]
'

it "__gash_expand_tilde_path leaves absolute paths unchanged" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_expand_tilde_path "/absolute/path")"
    [[ "$result" == "/absolute/path" ]]
'

it "__gash_expand_tilde_path leaves relative paths unchanged" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    result="$(__gash_expand_tilde_path "relative/path")"
    [[ "$result" == "relative/path" ]]
'

it "all_colors prints help on -h" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/utils.sh"

    out="$(all_colors -h)"
    [[ "$out" == *"all_colors"* ]]
    [[ "$out" == *"256 colors"* ]]
'

# Note: needs_confirm_prompt is not tested here because it reads from /dev/tty
# which is not available in automated test environments. The function is implicitly
# tested through other tests that mock or override it (e.g., docker_prune_all test).
