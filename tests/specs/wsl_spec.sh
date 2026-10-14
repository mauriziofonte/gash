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

describe "WSL helpers"

it "wsl_restart asks first and refuses without a terminal; explorer gets a Windows path" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  __gash_require_wsl() { return 0; }
  printf "#!/bin/sh\necho \"cmd \$*\" >> $tmp/log\n" > "$tmp/cmd.exe"
  printf "#!/bin/sh\necho \"explorer \$*\" >> $tmp/log; exit 1\n" > "$tmp/explorer.exe"
  printf "#!/bin/sh\necho \"WIN:\$2\"\n" > "$tmp/wslpath"
  chmod +x "$tmp"/*
  PATH="$tmp:$PATH"
  rc=0; GASH_HEADLESS=1 wsl_restart >/dev/null 2>&1 || rc=$?
  [[ $rc -eq 1 && ! -e "$tmp/log" ]]
  ! wsl_shutdown --now 2>/dev/null
  wsl_shutdown -y >/dev/null 2>&1
  grep -q "^cmd /C wsl --shutdown" "$tmp/log"
  wsl_explorer "$tmp"
  grep -qx "explorer WIN:$tmp" "$tmp/log"
  ! wsl_explorer "$tmp/missing" 2>/dev/null
'
