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
