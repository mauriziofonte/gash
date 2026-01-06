#!/usr/bin/env bash

describe "GASH_HEADLESS Mode"

it "loads core functions in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"

# Core output functions should be available
type __gash_info >/dev/null 2>&1; echo "gash_info:\$?"
type __gash_error >/dev/null 2>&1; echo "gash_error:\$?"
type __gash_success >/dev/null 2>&1; echo "gash_success:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"gash_info:0"* ]]
    [[ "$out" == *"gash_error:0"* ]]
    [[ "$out" == *"gash_success:0"* ]]
'

it "loads LLM functions in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"

# LLM functions should be available
type llm_tree >/dev/null 2>&1; echo "llm_tree:\$?"
type llm_grep >/dev/null 2>&1; echo "llm_grep:\$?"
type llm_find >/dev/null 2>&1; echo "llm_find:\$?"
type llm_db_query >/dev/null 2>&1; echo "llm_db_query:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"llm_tree:0"* ]]
    [[ "$out" == *"llm_grep:0"* ]]
    [[ "$out" == *"llm_find:0"* ]]
    [[ "$out" == *"llm_db_query:0"* ]]
'

it "does not modify PS1/PS2 in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
PS1=ORIG_PS1
PS2=ORIG_PS2
PROMPT_COMMAND=ORIG_PC

export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"

echo "PS1:\${PS1-}"
echo "PS2:\${PS2-}"
echo "PROMPT_COMMAND:\${PROMPT_COMMAND-}"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"PS1:ORIG_PS1"* ]]
    [[ "$out" == *"PS2:ORIG_PS2"* ]]
    [[ "$out" == *"PROMPT_COMMAND:ORIG_PC"* ]]
'

it "produces no output in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"
EOF
)"

    # Capture all output (stdout and stderr)
    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>&1)"

    # Should produce no output at all
    [[ -z "$out" ]]
'

it "does not load aliases in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"

# Common Gash aliases should NOT be defined
alias ll 2>/dev/null; echo "ll:\$?"
alias gst 2>/dev/null; echo "gst:\$?"
alias sl 2>/dev/null; echo "sl:\$?"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    # Aliases should NOT exist (exit code 1)
    [[ "$out" == *"ll:1"* ]]
    [[ "$out" == *"gst:1"* ]]
    [[ "$out" == *"sl:1"* ]]
'

it "does not source user bash_local in headless mode" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a bash_local that would set a marker variable
    echo "export BASH_LOCAL_WAS_LOADED=1" > "$tmp/.bash_local"

    inner_cmd="$(cat <<EOF
set -u
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"

echo "BASH_LOCAL_WAS_LOADED:\${BASH_LOCAL_WAS_LOADED-not_set}"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    # bash_local should NOT have been loaded
    [[ "$out" == *"BASH_LOCAL_WAS_LOADED:not_set"* ]]
'

it "works in non-interactive shell" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Without GASH_HEADLESS, non-interactive shells would exit early
    # With GASH_HEADLESS, it should work
    inner_cmd="$(cat <<EOF
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"
type llm_tree >/dev/null 2>&1 && echo "SUCCESS"
EOF
)"

    # Run in explicitly non-interactive mode (no -i flag)
    out="$(HOME="$tmp" bash --noprofile --norc -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"SUCCESS"* ]]
'
