#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Prompt"

it "loads gash.sh in an interactive shell and builds PS1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp"
  ln -s "$ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; __construct_ps1 0; printf \"%s\\n\" \"\${PROMPT_COMMAND-}\"; printf \"%s\\n\" \"\${PS1-}\"" 2>/dev/null)"

  [[ "$out" == *"__construct_ps1"* ]]
  [[ "$out" == *"\\u"* ]]
  [[ "$out" == *"\\w"* ]]
'

it "defines __gash_in_git_repo function" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
declare -f __gash_in_git_repo >/dev/null 2>&1 && echo "func:defined" || echo "func:missing"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"func:defined"* ]]
'

it "uses __GASH_PS1_* color variables instead of generic names" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Check that gash.sh uses prefixed color variables
    grep -q "__GASH_PS1_OFF" "$ROOT/gash.sh"
    grep -q "__GASH_PS1_RED" "$ROOT/gash.sh"
    grep -q "__GASH_PS1_BBLUE" "$ROOT/gash.sh"

    # Check that old unprefixed names are NOT used in PS1 context
    ! grep -E "^\s*PS1\+.*\\\$\{(Color_Off|Red|Green|Blue|BBlue)\}" "$ROOT/gash.sh"
'

it "__gash_in_git_repo returns success in a git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a git repo in the temp directory
    mkdir -p "$tmp/project"
    (cd "$tmp/project" && git init -q)

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/project"
__gash_in_git_repo && echo "in_repo:yes" || echo "in_repo:no"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"in_repo:yes"* ]]
'

it "__gash_in_git_repo returns failure outside git repo" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a non-git directory
    mkdir -p "$tmp/notgit"

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/notgit"
__gash_in_git_repo && echo "in_repo:yes" || echo "in_repo:no"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"in_repo:no"* ]]
'

it "__gash_in_git_repo respects GASH_GIT_EXCLUDE" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    ln -s "$ROOT" "$tmp/.gash"

    # Create a git repo
    mkdir -p "$tmp/excluded_repo"
    (cd "$tmp/excluded_repo" && git init -q)

    inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1 || true
cd "\$HOME/excluded_repo"

# Without exclusion - should return success
unset GASH_GIT_EXCLUDE
__gash_in_git_repo && echo "without_exclude:in_repo" || echo "without_exclude:not_in_repo"

# With exclusion - should return failure
export GASH_GIT_EXCLUDE="\$HOME/excluded_repo"
__gash_in_git_repo && echo "with_exclude:in_repo" || echo "with_exclude:not_in_repo"
EOF
)"

    out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

    [[ "$out" == *"without_exclude:in_repo"* ]]
    [[ "$out" == *"with_exclude:not_in_repo"* ]]
'

it "uses optimized git commands with GIT_OPTIONAL_LOCKS=0" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Check that gash.sh uses GIT_OPTIONAL_LOCKS=0 for performance
    grep -q "GIT_OPTIONAL_LOCKS=0" "$ROOT/gash.sh"
'

it "uses git symbolic-ref instead of git name-rev for branch detection" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Check that gash.sh uses symbolic-ref (faster)
    grep -q "git symbolic-ref --short HEAD" "$ROOT/gash.sh"

    # Check that it does NOT use git name-rev (slower)
    ! grep -q "git name-rev" "$ROOT/gash.sh"
'

it ".gash_env.template documents GASH_GIT_EXCLUDE" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"

    # Check that the template includes GASH_GIT_EXCLUDE documentation
    grep -q "GASH_GIT_EXCLUDE" "$ROOT/.gash_env.template"
    grep -q "PS1 Prompt Configuration" "$ROOT/.gash_env.template"
'
