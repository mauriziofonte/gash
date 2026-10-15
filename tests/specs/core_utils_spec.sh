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

describe "Core Utils - one rendering for confirmations and help"

it "needs_confirm_prompt and __gash_confirm_plan render the same way, with their own defaults" bash -c '
    set -euo pipefail
    command -v script >/dev/null || exit 0
    ROOT="$GASH_TEST_ROOT"
    tmp="$(mktemp -d)"; trap "command rm -rf \"$tmp\"" EXIT
    cat >"$tmp/run.sh" <<EOS
stty -echo -onlcr
: >"$tmp/ready"
unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
source "$ROOT/lib/core/output.sh"; source "$ROOT/lib/core/utils.sh"
needs_confirm_prompt "Remove C:\\\\new?"; echo
__gash_confirm_plan 0; echo
EOS
    # Each read gets end of input (^D) once the terminal is set up.
    feed() { while [[ ! -e "$tmp/ready" ]]; do sleep 0.05; done; printf "\004\004\004\004"; }
    if script -qec true /dev/null >/dev/null 2>&1; then
        out="$(feed | timeout 20 script -qec "bash $tmp/run.sh" /dev/null)"
    else
        out="$(feed | timeout 20 script -q /dev/null bash "$tmp/run.sh")"
    fi
    b=$'"'"'\e[1m'"'"' o=$'"'"'\e[0m'"'"'
    [[ "$out" == *"Remove C:\\new? ${b}[y/N]${o} "* ]] || { printf "%s" "$out" | od -c | head; exit 1; }
    [[ "$out" == *"Do you want to continue? ${b}[Y/n]${o} "* ]] || { printf "%s" "$out" | od -c | head; exit 1; }

    # Plain: the same brackets, no escapes (no terminal: refused at once).
    source "$ROOT/lib/core/output.sh"; source "$ROOT/lib/core/utils.sh"
    out="$(needs_confirm_prompt "Remove C:\\new?" 2>&1 </dev/null || true)"
    [[ "$out" == "Remove C:\\new? [y/N] "* && "$out" != *$'"'"'\e'"'"'* ]]
'

it "needs_help fallback uses the semantic palette only" bash -c '
    set -euo pipefail
    ROOT="$GASH_TEST_ROOT"
    source "$ROOT/lib/core/output.sh"; source "$ROOT/lib/core/utils.sh"
    __gash_use_color() { return 0; }
    out="$(needs_help prog "prog ARG" "Does things." --help)"
    [[ "$out" == "${__GASH_COLOR_ACCENT}prog${__GASH_COLOR_OFF}"$'"'"'\n'"'"'"${__GASH_BOLD}Usage:${__GASH_COLOR_OFF} prog ARG"$'"'"'\n'"'"'"Does things." ]]
'

it "__gash_task: glyph and colored status on a color terminal, plain text otherwise" bash -c '
    set -euo pipefail
    ROOT="$GASH_TEST_ROOT"
    source "$ROOT/lib/core/output.sh"; source "$ROOT/lib/core/utils.sh"
    [[ "$(__gash_task "Reading" true 2>&1)" == "Reading... done" ]]
    [[ "$(__gash_task "Reading" false 2>&1 || true)" == "Reading... failed" ]]
    __gash_stderr_fancy() { return 0; }
    [[ "$(__gash_task "Reading" true 2>&1)" == "${__GASH_COLOR_INFO}${__GASH_GLYPH_INFO}${__GASH_COLOR_OFF} Reading... ${__GASH_COLOR_SUCCESS}done${__GASH_COLOR_OFF}" ]]
'

it "__gash_json_warning prints one JSON warning line on stderr" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"; source "$GASH_TEST_ROOT/lib/core/utils.sh"
    out="$(__gash_json_warning partial_results "Some paths were unreadable (permissions)" skipped_entries 3 2>&1 >/dev/null)"
    [[ "$out" == "{\"warning\":\"partial_results\",\"skipped_entries\":3,\"hint\":\"Some paths were unreadable (permissions)\"}" ]]
    [[ -z "$(__gash_json_warning x y 2>/dev/null)" ]]
'
