#!/usr/bin/env bash

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export GASH_TEST_ROOT="$ROOT_DIR"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

# Load specs: single file if argument provided, all specs otherwise
if [[ -n "${1-}" ]]; then
  local_spec="${1-}"
  # Resolve relative paths from tests/ dir
  if [[ ! -f "$local_spec" ]] && [[ -f "$ROOT_DIR/tests/$local_spec" ]]; then
    local_spec="$ROOT_DIR/tests/$local_spec"
  fi
  # shellcheck disable=SC1090
  source "$local_spec"
else
  for spec in "$ROOT_DIR"/tests/specs/*_spec.sh; do
    # shellcheck disable=SC1090
    source "$spec"
  done
fi

gash_test_summary
