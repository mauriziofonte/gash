#!/usr/bin/env bash

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export GASH_TEST_ROOT="$ROOT_DIR"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

# Load all specs
for spec in "$ROOT_DIR"/tests/specs/*_spec.sh; do
  # shellcheck disable=SC1090
  source "$spec"
done

gash_test_summary
