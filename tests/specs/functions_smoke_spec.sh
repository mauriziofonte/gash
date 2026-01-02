#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=lib/functions.sh
source "$ROOT_DIR/lib/functions.sh"

describe "Functions (smoke)"

it "needs_help prints usage on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  out="$(needs_help "prog" "prog ARG" "desc" "-h" 2>&1 || true)"
  [[ "$out" == *"Usage:"*"prog ARG"* ]]
'

it "__gash_trim_ws trims tabs/spaces" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  v="$(__gash_trim_ws "$(printf "\t  hello  \t")")"
  [[ "$v" == "hello" ]]
'

it "backup_file creates timestamped backup" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  echo "x" > "$tmp/a.txt"
  (cd "$tmp" && backup_file "$tmp/a.txt" >/dev/null)
  ls "$tmp" | grep -q "a.txt_backup_"
'

it "extract rejects missing file" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  out="$({ extract /no/such/file.zip; } 2>&1 || true)"
  [[ "$out" == *"does not exist"* ]]
'

it "largest_files errors on missing directory" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  out="$({ largest_files /no/such/dir; } 2>&1 || true)"
  [[ "$out" == *"Error:"*"does not exist"* ]]
'

it "largest_files lists files in directory" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/big" bs=1024 count=10 status=none
  echo "x" > "$tmp/small"

  out="$(largest_files "$tmp" 2>/dev/null)"
  [[ "$out" == *"big"* ]]
'

it "myip fails gracefully without curl/wget" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ myip; } 2>&1 || true)"
  [[ "$out" == *"requires either"*"wget"*"curl"* ]]
'

it "portkill reports when lsof missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ portkill 1234; } 2>&1 || true)"
  [[ "$out" == *"requires"* && "$out" == *lsof* && "$out" == *"not available"* ]]
'

it "docker_stop_all errors when docker missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_stop_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'
