#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=lib/functions.sh
source "$ROOT_DIR/lib/functions.sh"

describe "Functions (more)"

it "disk_usage_fs formats filtered filesystem types" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(disk_usage_fs 2>/dev/null)"

  # Should include only ext*/xfs/btrfs/... lines, so ext4 and xfs from mock df
  [[ "$out" == *"ext4"*"/"* ]]
  [[ "$out" == *"xfs"*"/data"* ]]
'

it "find_large_dirs errors when numfmt missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ find_large_dirs --size 1M /tmp; } 2>&1 || true)"
  [[ "$out" == *"requires"*numfmt* ]]
'

it "find_large_dirs lists a directory over threshold" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/d1"
  dd if=/dev/zero of="$tmp/d1/big" bs=1024 count=32 status=none

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  # Use a tiny threshold to avoid unit mismatches between du -sk (KiB) and numfmt output.
  out="$(find_large_dirs --size 1 "$tmp" 2>/dev/null || true)"
  [[ "$out" == *"$tmp/d1"* ]]
'

it "find_large_dirs sorts by size and supports K/M suffix" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/big" "$tmp/small"
  dd if=/dev/zero of="$tmp/big/f" bs=1024 count=128 status=none
  dd if=/dev/zero of="$tmp/small/f" bs=1024 count=2 status=none

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(find_large_dirs --size 1K "$tmp" 2>/dev/null || true)"

  # The base directory itself may appear; assert that "big" is listed before "small".
  big_line="$(printf "%s\n" "$out" | grep -n -- "$tmp/big" | head -n1 | cut -d: -f1)"
  small_line="$(printf "%s\n" "$out" | grep -n -- "$tmp/small" | head -n1 | cut -d: -f1)"
  [[ -n "$big_line" ]]
  [[ -n "$small_line" ]]
  [[ "$big_line" -lt "$small_line" ]]
'

it "list_empty_dirs lists empty dirs" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/empty" "$tmp/nonempty"
  : > "$tmp/nonempty/f"

  out="$(list_empty_dirs "$tmp")"
  [[ "$out" == *"$tmp/empty"* ]]
'

it "psgrep prints error when no match" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  out="$({ psgrep __definitely_not_a_process__; } 2>&1 || true)"
  [[ "$out" == *"No process found"* ]]
'

it "stop_services --force runs without prompting" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SYSTEMCTL_ACTIVE=
  # No service is active in mock => should be silent and succeed.
  stop_services --force
'

it "docker_start_all errors when docker missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_start_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'

it "docker_prune_all errors when docker missing (no prompt)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_prune_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'

it "gash_inspiring_quote prints a quote when file present" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/quotes"
  printf "%s\n" "Hello quote" "Another quote" > "$tmp/quotes/list.txt"

  export GASH_DIR="$tmp"
  out="$(gash_inspiring_quote 2>/dev/null)"
  [[ "$out" == *"Quote:"* ]]
'

it "gash_username returns a non-empty string" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  u="$(gash_username)"
  [[ -n "$u" ]]
'
