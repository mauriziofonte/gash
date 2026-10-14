#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "WSL memory compaction (wslc)"

it "rejects unknown options before anything else" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(wsl_compact --now 2>&1)" && exit 1
  [[ "$out" == *"Unknown option: --now"* ]]
'

it "refuses outside WSL without calling sudo" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  grep -qi microsoft /proc/version 2>/dev/null && gash_skip "running under WSL"
  sudo() { echo "sudo called" >&2; return 1; }
  out="$(wsl_compact 2>&1)" && exit 1
  [[ "$out" == *"requires WSL"* && "$out" != *"sudo called"* ]]
'

it "the memory snapshot has free memory, slab and per-zone blocks" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  [[ -r /proc/buddyinfo ]] || gash_skip "no /proc/buddyinfo"
  snap="$(__gash_wsl_mem_snapshot)"
  [[ "$snap" =~ ^[0-9]+\ MiB\|[0-9]+\ MiB\|[A-Za-z0-9]+=[0-9]+ ]]
'
