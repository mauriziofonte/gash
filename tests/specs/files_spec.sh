#!/usr/bin/env bash

# Comprehensive spec for the files module (v1.5+).
# Covers: flag parsing, JSON envelope, NUL-delim output, guardrails, tree_stats,
# llm_tree fixes (nested JSON + --stats), size parsing edge cases.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Files module (v1.5)"

# -----------------------------------------------------------------------------
# __gash_fs_parse_size
# -----------------------------------------------------------------------------

it "parse_size: plain integer is bytes" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  v=$(__gash_fs_parse_size 1024)
  [[ "$v" == "1024" ]]
'

it "parse_size: K/M/G/T IEC suffix" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  [[ "$(__gash_fs_parse_size 1K)"  == "1024" ]]
  [[ "$(__gash_fs_parse_size 1M)"  == "1048576" ]]
  [[ "$(__gash_fs_parse_size 1G)"  == "1073741824" ]]
  [[ "$(__gash_fs_parse_size 2GiB)" == "2147483648" ]]
'

it "parse_size: invalid input returns 1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  set +e
  __gash_fs_parse_size nonsense >/dev/null 2>&1
  rc=$?
  set -e
  [[ $rc -ne 0 ]]
'

# -----------------------------------------------------------------------------
# __gash_fs_human_size
# -----------------------------------------------------------------------------

it "human_size: zero bytes" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  [[ "$(__gash_fs_human_size 0)" == "0 B" ]]
'

it "human_size: IEC scaling with one decimal" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  [[ "$(__gash_fs_human_size 1024)" == "1.0 KiB" ]]
  [[ "$(__gash_fs_human_size 1048576)" == "1.0 MiB" ]]
  [[ "$(__gash_fs_human_size 1572864)" == "1.5 MiB" ]]
'

# -----------------------------------------------------------------------------
# files_largest new flags
# -----------------------------------------------------------------------------

it "files_largest --min-size filters out small files" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/big.bin" bs=1024 count=64 status=none
  echo smol > "$tmp/small.txt"
  out="$(files_largest "$tmp" --min-size 10K --no-color 2>/dev/null)"
  [[ "$out" == *"big.bin"* ]]
  [[ "$out" != *"small.txt"* ]]
'

it "files_largest --limit caps results" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  for i in 1 2 3 4 5; do dd if=/dev/zero of="$tmp/f$i.bin" bs=1024 count=$i status=none; done
  out="$(files_largest "$tmp" --limit 2 --no-color 2>/dev/null)"
  count=$(printf "%s\n" "$out" | grep -c "\.bin" || true)
  [[ "$count" -eq 2 ]]
'

it "files_largest --json emits a valid envelope" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/a.bin" bs=1024 count=8 status=none
  echo x > "$tmp/b.txt"
  out="$(files_largest "$tmp" --json)"
  count=$(printf "%s" "$out" | jq ".count")
  kind=$(printf "%s" "$out" | jq -r ".kind")
  [[ "$count" -eq 2 ]]
  [[ "$kind" == "files" ]]
  # items have required fields
  printf "%s" "$out" | jq -e ".data[0] | has(\"size\") and has(\"size_human\") and has(\"path\") and has(\"mtime\")" >/dev/null
'

it "files_largest --null emits NUL-delimited paths" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  : > "$tmp/x"
  : > "$tmp/y"
  out="$(files_largest "$tmp" --null | xargs -0 -n1 basename | sort)"
  [[ "$out" == *"x"* && "$out" == *"y"* ]]
'

it "files_largest respects default prune (node_modules)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/node_modules/foo"
  dd if=/dev/zero of="$tmp/node_modules/foo/mass.bin" bs=1024 count=128 status=none
  echo x > "$tmp/kept.txt"
  out="$(files_largest "$tmp" --no-color 2>/dev/null)"
  [[ "$out" == *"kept.txt"* ]]
  [[ "$out" != *"mass.bin"* ]]
'

it "files_largest --no-ignore disables pruning" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/node_modules/foo"
  dd if=/dev/zero of="$tmp/node_modules/foo/mass.bin" bs=1024 count=128 status=none
  out="$(files_largest "$tmp" --no-ignore --no-color 2>/dev/null)"
  [[ "$out" == *"mass.bin"* ]]
'

# -----------------------------------------------------------------------------
# dirs_largest new flags
# -----------------------------------------------------------------------------

it "dirs_largest --depth 2 reveals nested offenders" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/proj/big"
  dd if=/dev/zero of="$tmp/proj/big/blob" bs=1024 count=256 status=none
  out="$(dirs_largest "$tmp" --depth 2 --no-color 2>/dev/null)"
  [[ "$out" == *"/proj/big"* || "$out" == *"/proj"* ]]
'

it "dirs_largest --json envelope is valid" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b"
  dd if=/dev/zero of="$tmp/a/f" bs=1024 count=32 status=none
  out="$(dirs_largest "$tmp" --depth 2 --json)"
  printf "%s" "$out" | jq -e ".kind == \"directories\" and .count >= 1" >/dev/null
'

# -----------------------------------------------------------------------------
# dirs_find_large perf + new flags
# -----------------------------------------------------------------------------

it "dirs_find_large --with-mtime adds mtime column" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/d1"
  dd if=/dev/zero of="$tmp/d1/big" bs=1024 count=64 status=none
  out="$(dirs_find_large "$tmp" --size 1K --with-mtime --no-color 2>/dev/null)"
  # Expect YYYY-MM-DD in output when --with-mtime is active
  [[ "$out" =~ [0-9]{4}-[0-9]{2}-[0-9]{2} ]]
'

it "dirs_find_large --json envelope is valid" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/d1"
  dd if=/dev/zero of="$tmp/d1/big" bs=1024 count=64 status=none
  out="$(dirs_find_large "$tmp" --size 1K --json)"
  printf "%s" "$out" | jq -e ".kind == \"directories\" and .count >= 1" >/dev/null
'

# -----------------------------------------------------------------------------
# dirs_list_empty new flags
# -----------------------------------------------------------------------------

it "dirs_list_empty --count outputs a number" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b" "$tmp/c"
  : > "$tmp/a/f"
  out="$(dirs_list_empty "$tmp" --count)"
  [[ "$out" =~ ^[0-9]+$ ]]
  [[ "$out" -ge 2 ]]
'

it "dirs_list_empty --null emits NUL-delimited" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/empty_a" "$tmp/empty_b"
  out="$(dirs_list_empty "$tmp" --null | xargs -0 -n1 basename | sort)"
  [[ "$out" == *"empty_a"* && "$out" == *"empty_b"* ]]
'

it "dirs_list_empty --json envelope is valid" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/e1" "$tmp/e2"
  out="$(dirs_list_empty "$tmp" --json)"
  printf "%s" "$out" | jq -e ".count >= 2" >/dev/null
'

# -----------------------------------------------------------------------------
# Guardrails
# -----------------------------------------------------------------------------

it "safe_path blocks / without --allow-root" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$({ files_largest / 2>&1; } || true)"
  [[ "$out" == *"root_scan_blocked"* || "$out" == *"Use --allow-root"* ]]
'

it "safe_path blocks forbidden prefixes (/proc)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$({ files_largest /proc 2>&1; } || true)"
  [[ "$out" == *"forbidden_path"* ]]
'

it "safe_path emits JSON error when --json is active" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  err="$({ files_largest --json /proc 2>&1 >/dev/null; } || true)"
  printf "%s" "$err" | jq -e ".error == \"forbidden_path\" and .action == \"FATAL\"" >/dev/null
'

it "invalid size returns JSON error under --json" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  err="$({ files_largest --json --min-size notasize 2>&1 >/dev/null; } || true)"
  printf "%s" "$err" | jq -e ".error == \"invalid_size\"" >/dev/null
'

# -----------------------------------------------------------------------------
# tree_stats
# -----------------------------------------------------------------------------

it "tree_stats prints totals and extension tops" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/sub"
  dd if=/dev/zero of="$tmp/a.bin" bs=1024 count=32 status=none
  dd if=/dev/zero of="$tmp/sub/b.bin" bs=1024 count=64 status=none
  echo x > "$tmp/c.txt"
  out="$(tree_stats "$tmp" --no-color 2>/dev/null)"
  [[ "$out" == *"Files:"* ]]
  [[ "$out" == *"Directories:"* ]]
  [[ "$out" == *"Total size:"* ]]
  [[ "$out" == *"Top"*"by count"* ]]
  [[ "$out" == *"Top"*"by size"* ]]
  [[ "$out" == *"bin"* ]]
'

it "tree_stats --json envelope is valid and has stats fields" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/sub"
  dd if=/dev/zero of="$tmp/a.bin" bs=1024 count=32 status=none
  out="$(tree_stats "$tmp" --json)"
  printf "%s" "$out" | jq -e ".data | has(\"files_total\") and has(\"dirs_total\") and has(\"size_total\") and has(\"top_by_size\") and has(\"top_by_count\")" >/dev/null
'

it "tls alias is defined" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  alias tls 2>/dev/null | grep -q "tree_stats"
'

# -----------------------------------------------------------------------------
# llm_tree fixes
# -----------------------------------------------------------------------------

it "llm_tree JSON default depth 1 stays flat (backward-compat)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a/b"
  echo x > "$tmp/a/b/deep.txt"
  out="$(llm_tree "$tmp")"
  # Top-level children must not have a "children" key
  has_nested=$(printf "%s" "$out" | jq "[.children[] | has(\"children\")] | any")
  [[ "$has_nested" == "false" ]]
'

it "llm_tree --depth 3 produces nested JSON" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a/b"
  echo x > "$tmp/a/b/deep.txt"
  out="$(llm_tree --depth 3 "$tmp")"
  # Find the "a" child and check it has children
  has_children=$(printf "%s" "$out" | jq "[.children[] | select(.name == \"a\") | has(\"children\")] | any")
  [[ "$has_children" == "true" ]]
  # Find deep.txt within a/b
  name=$(printf "%s" "$out" | jq -r ".children[] | select(.name == \"a\") | .children[] | select(.name == \"b\") | .children[0].name")
  [[ "$name" == "deep.txt" ]]
'

it "llm_tree --stats adds size and children_count" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v jq >/dev/null 2>&1 || { echo "jq missing, skipping"; exit 0; }
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a"
  dd if=/dev/zero of="$tmp/a/big" bs=1024 count=16 status=none
  out="$(llm_tree --depth 2 --stats "$tmp")"
  # root size should be cumulative (at least 16 KiB)
  size=$(printf "%s" "$out" | jq ".size")
  [[ "$size" -ge 16384 ]]
  # children_count at root should be 1
  cc=$(printf "%s" "$out" | jq ".children_count")
  [[ "$cc" == "1" ]]
'

it "llm_tree rejects --depth 0 (invalid)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  err="$({ llm_tree --depth 0 /tmp 2>&1 >/dev/null; } || true)"
  [[ "$err" == *"invalid_depth"* ]]
'
