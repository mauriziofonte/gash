#!/usr/bin/env bash
#
# Terminal layout of the filesystem scanners: rank, share bars, relative
# paths and a footer on a TTY; the plain "SIZE  PATH" lines stay for pipes.

describe "Filesystem scanners - terminal layout"

it "files_largest on a terminal ranks, draws share bars and sums up" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=120
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/a/b"
  head -c 4096 /dev/zero >"$tmp/a/b/big.bin"
  head -c 1024 /dev/zero >"$tmp/small.txt"
  source "$GASH_TEST_ROOT/gash.sh"
  __gash_fs_rich_ok() { return 0; }
  out="$(files_largest "$tmp")"
  [[ "$out" == *"Largest files in $tmp"* ]]
  grep -qE "^  1  4\.0 KiB  #{16}  [0-9-]+ [0-9:]+  a/b/big\.bin$" <<<"$out"
  grep -qE "^  2  1\.0 KiB  #{4}\.{12}  .*  small\.txt$" <<<"$out"
  grep -qE "^  2 files - 5\.0 KiB total - [0-9.]+m?s$" <<<"$out"
  # Nothing over the threshold: one quiet line.
  [[ "$(files_largest --min-size 1G "$tmp")" == *"No files matched."* ]]
  # Colors on: the name is highlighted, the directory part is not.
  __gash_use_color() { return 0; }
  out="$(files_largest "$tmp")"
  [[ "$out" == *"a/b/${__GASH_COLOR_OFF}${__GASH_BOLD}big.bin"* ]]
  [[ "$(files_largest --no-color "$tmp")" != *$'"'"'\033'"'"'* ]]
'

it "dirs_largest and dirs_find_large share the layout with their own titles" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=120
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/big" "$tmp/small"
  head -c 65536 /dev/urandom >"$tmp/big/f"
  head -c 1024 /dev/urandom >"$tmp/small/f"
  source "$GASH_TEST_ROOT/gash.sh"
  __gash_fs_rich_ok() { return 0; }
  out="$(dirs_largest "$tmp")"
  [[ "$out" == *"Largest directories in $tmp"* ]]
  grep -qE "^  1 +[0-9.]+ KiB  #{16}  big$" <<<"$out"
  grep -qE "^  2 directories - largest [0-9.]+ KiB - " <<<"$out"
  out="$(dirs_find_large --size 32K "$tmp")"
  [[ "$out" == *"Directories over 32K in $tmp"* ]]
  grep -qE "  big$" <<<"$out"
'

it "piped output keeps the plain SIZE  PATH lines" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  head -c 2048 /dev/zero >"$tmp/x"
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(files_largest "$tmp")"
  grep -qE "^2\.0 KiB +$tmp/x  [0-9-]+ [0-9:]+$" <<<"$out"
  [[ "$out" != *"Largest files"* ]]
  [[ "$(tree_stats "$tmp")" == "Tree statistics for $tmp"* ]]
'

it "tree_stats on a terminal shows grouped totals and share bars" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=120
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  head -c 3000000 /dev/zero >"$tmp/a.bin"
  printf x >"$tmp/b.txt"; printf y >"$tmp/c.txt"
  source "$GASH_TEST_ROOT/gash.sh"
  __gash_fs_rich_ok() { return 0; }
  out="$(tree_stats "$tmp")"
  grep -qE "^  Files +3 +Total size +2\.8 MiB \(3,000,002 bytes\)$" <<<"$out"
  grep -qE "^  bin +2\.8 MiB  #{20}   99\.9%$" <<<"$out"
  grep -qE "^  txt +2  #{13}\.{7}   66\.6%$" <<<"$out"
  [[ "$out" == *"Scanned in"* ]]
'

it "__gash_group_digits and __gash_bar" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1 LC_ALL=C
  source "$GASH_TEST_ROOT/gash.sh"
  n=""; __gash_group_digits n 1234567; [[ "$n" == "1,234,567" ]]
  __gash_group_digits n 999; [[ "$n" == "999" ]]
  __gash_group_digits n -1000; [[ "$n" == "-1,000" ]]
  bar=""; __gash_bar bar 1 100 10; [[ "$bar" == "#"$'"'"'\t'"'"'"........." ]]
  __gash_bar bar 0 100 4; [[ "$bar" == $'"'"'\t'"'"'"...." ]]
  __gash_bar bar 50 0 4; [[ "$bar" == $'"'"'\t'"'"'"...." ]]
  __gash_bar bar 7 4 4; [[ "$bar" == "####"$'"'"'\t'"'"' ]]
'

# FS-50: a tab inside an extension shifted file-name text into the numeric
# fields of the top-extension rows, and the terminal layout evaluated those
# fields as Bash arithmetic (an array subscript runs its command
# substitutions). The fields are now coerced to digits before any arithmetic,
# and --json no longer emits a non-numeric field.
it "tree_stats never evaluates file-name text as arithmetic (FS-50)" bash -c '
  set -o pipefail
  export GASH_HEADLESS=1 LC_ALL=C COLUMNS=120
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir "$tmp/tree" "$tmp/cwd"
  printf x >"$tmp/tree/ok.txt"
  # Extension = <tab> + an array subscript whose command substitution would
  # create ./marker if the field reached arithmetic. \044 is a literal $, so
  # nothing is substituted while the file name is built.
  fn="$(printf "bad.z\ta[\044(touch marker)]")"
  : >"$tmp/tree/$fn"
  source "$GASH_TEST_ROOT/gash.sh"
  __gash_fs_rich_ok() { return 0; }
  cd "$tmp/cwd"
  tree_stats "$tmp/tree" >out.txt 2>&1 || true
  [[ ! -e marker ]] || { echo "file-name text was evaluated as code"; exit 1; }
  grep -qE "^  txt " out.txt || { echo "ordinary extension row missing"; exit 1; }
  if command -v jq >/dev/null 2>&1; then
    tree_stats --json "$tmp/tree" >json.txt 2>&1 || true
    jq -e . <json.txt >/dev/null 2>&1 || { echo "tree_stats --json emitted invalid JSON"; exit 1; }
  fi
'
