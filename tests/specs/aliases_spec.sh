#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Aliases"

it "defines core navigation and safety aliases" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  export PATH="$ROOT/tests/mocks/bin:$PATH"

  # Keep LS_COLORS empty so aliases.sh initializes it.
  export LS_COLORS=""

  # Ensure the helper does not leak globals.
  unset BINARY 2>/dev/null || true

  # shellcheck source=lib/aliases.sh
  source "$ROOT/lib/aliases.sh"

  a1="$(alias ..)"
  aDots2="$(alias ...)"
  aDots3="$(alias ....)"
  aDots4="$(alias .....)"
  a2="$(alias ll)"
  a3="$(alias rm)"
  a4="$(alias help)"

  [[ "$a1" == *"cd .."* ]]
  [[ "$aDots2" == *"cd ../.."* ]]
  [[ "$aDots3" == *"cd ../../.."* ]]
  [[ "$aDots4" == *"cd ../../../.."* ]]
  [[ "$a2" == *"ls"* ]]
  [[ "$a3" == *"--preserve-root"* ]]
  [[ "$a4" == *"gash_help"* ]]

  # Must remain unset (no pollution).
  [[ -z "${BINARY+x}" ]]
'
