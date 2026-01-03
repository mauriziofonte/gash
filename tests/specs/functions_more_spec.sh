#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Functions (more)"

it "disk_usage formats filtered filesystem types" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(disk_usage 2>/dev/null)"

  # Should include only ext*/xfs/btrfs/... lines, so ext4 and xfs from mock df
  [[ "$out" == *"ext4"*"/"* ]]
  [[ "$out" == *"xfs"*"/data"* ]]
'

it "dirs_find_large errors when numfmt missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ dirs_find_large --size 1M /tmp; } 2>&1 || true)"
  [[ "$out" == *"requires"*numfmt* ]]
'

it "dirs_find_large lists a directory over threshold" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/d1"
  dd if=/dev/zero of="$tmp/d1/big" bs=1024 count=32 status=none

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  # Use a tiny threshold to avoid unit mismatches between du -sk (KiB) and numfmt output.
  out="$(dirs_find_large --size 1 "$tmp" 2>/dev/null || true)"
  [[ "$out" == *"$tmp/d1"* ]]
'

it "dirs_find_large sorts by size and supports K/M suffix" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/big" "$tmp/small"
  dd if=/dev/zero of="$tmp/big/f" bs=1024 count=128 status=none
  dd if=/dev/zero of="$tmp/small/f" bs=1024 count=2 status=none

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(dirs_find_large --size 1K "$tmp" 2>/dev/null || true)"

  # The base directory itself may appear; assert that "big" is listed before "small".
  big_line="$(printf "%s\n" "$out" | grep -n -- "$tmp/big" | head -n1 | cut -d: -f1)"
  small_line="$(printf "%s\n" "$out" | grep -n -- "$tmp/small" | head -n1 | cut -d: -f1)"
  [[ -n "$big_line" ]]
  [[ -n "$small_line" ]]
  [[ "$big_line" -lt "$small_line" ]]
'

it "dirs_list_empty lists empty dirs" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/empty" "$tmp/nonempty"
  : > "$tmp/nonempty/f"

  out="$(dirs_list_empty "$tmp")"
  [[ "$out" == *"$tmp/empty"* ]]
'

it "process_find prints error when no match" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Build a pattern at runtime so it does not appear in ps command line
  a="ZZX"; b="XYY"; c="99Q"; d="Q"
  pattern="${a}${b}${c}${d}"
  out="$({ process_find "$pattern"; } 2>&1 || true)"
  [[ "$out" == *"Error:"*"No process found"* ]]
'

it "services_stop --force runs without prompting" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SYSTEMCTL_ACTIVE=
  # No service is active in mock => should be silent and succeed.
  services_stop --force
'

it "docker_start_all errors when docker missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_start_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'

it "docker_prune_all errors when docker missing (no prompt)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  export PATH="$tmpbin"

  out="$({ docker_prune_all; } 2>&1 || true)"
  [[ "$out" == *"Docker is not installed"* ]]
'

it "gash_inspiring_quote prints a quote when file present" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

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
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  u="$(gash_username)"
  [[ -n "$u" ]]
'
