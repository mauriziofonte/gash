#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Completion"

it "bash_completion returns early if gash functions are missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  out="$(bash --noprofile --norc -c "source \"$ROOT/bash_completion\"; echo ok" 2>&1)"
  [[ "$out" == "ok" ]]
'

it "bash_completion suggests public gash functions" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("" "")
COMP_CWORD=1
__gash_complete

printf '%s\n' "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"

  # Includes public functions
  [[ "$out" == *"files_largest"* ]]
  [[ "$out" == *"gash_unload"* ]]

  # Excludes internal helpers
  [[ "$out" != *"__gash_tty_width"* ]]
  [[ "$out" != *"needs_help"* ]]
'
