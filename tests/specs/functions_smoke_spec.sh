#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Functions (smoke)"

it "gash_help displays custom commands section" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Override builtin help to avoid actual bash help output
  help() { :; }

  out="$(gash_help 2>/dev/null)"
  [[ "$out" == *"Custom Commands"* ]]
  [[ "$out" == *"files_largest"* ]]
  [[ "$out" == *"flf"* ]]
  [[ "$out" == *"git_add_tag"* ]]
  [[ "$out" == *"gat"* ]]
'

it "needs_help prints usage on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(needs_help "prog" "prog ARG" "desc" "-h" 2>&1 || true)"
  [[ "$out" == *"Usage:"*"prog ARG"* ]]
'

it "__gash_trim_ws trims tabs/spaces" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  v="$(__gash_trim_ws "$(printf "\t  hello  \t")")"
  [[ "$v" == "hello" ]]
'

it "file_backup creates timestamped backup" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  echo "x" > "$tmp/a.txt"
  (cd "$tmp" && file_backup "$tmp/a.txt" >/dev/null)
  ls "$tmp" | grep -q "a.txt_backup_"
'

it "archive_extract rejects missing file" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$({ archive_extract /no/such/file.zip; } 2>&1 || true)"
  [[ "$out" == *"does not exist"* ]]
'

it "files_largest errors on missing directory" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$({ files_largest /no/such/dir; } 2>&1 || true)"
  [[ "$out" == *"Error:"*"does not exist"* ]]
'

it "files_largest lists files in directory" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/big" bs=1024 count=10 status=none
  echo "x" > "$tmp/small"

  out="$(files_largest "$tmp" 2>/dev/null)"
  [[ "$out" == *"big"* ]]
'

it "ip_public fails gracefully without curl/wget" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ ip_public; } 2>&1 || true)"
  [[ "$out" == *"requires either"*"wget"*"curl"* ]]
'

it "port_kill reports when lsof missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ port_kill 1234; } 2>&1 || true)"
  [[ "$out" == *"requires"* && "$out" == *lsof* && "$out" == *"not available"* ]]
'

it "docker_stop_all errors when docker missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_stop_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'
