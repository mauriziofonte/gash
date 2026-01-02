#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=lib/functions.sh
source "$ROOT_DIR/lib/functions.sh"

describe "No exit in sourced functions"

it "gash_upgrade returns instead of exiting on cd failure" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  # force an invalid GASH_DIR
  out="$({ GASH_DIR=/no/such/dir gash_upgrade; echo "status=$?"; } 2>&1 || true)"
  [[ "$out" == *"status="* ]]
  [[ "$out" == *"Failed to change directory"* || "$out" == *"Gash is not installed"* ]]
'
