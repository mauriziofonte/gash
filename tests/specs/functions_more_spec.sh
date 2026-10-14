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

  # Disk-backed filesystems only: ext4 and xfs, the WSL drive, not tmpfs.
  grep -qE "^  /  +/dev/sda1 +ext4 +50 GiB +10 GiB +38 GiB +[^ ]+ +21%$" <<<"$out"
  grep -qE "^  /data +/dev/sdb1 +xfs .* 92%$" <<<"$out"
  grep -qE "^  /mnt/c +C:.* 9p .* 81%$" <<<"$out"
  grep -qE "^  /media/my disk +/dev/sdc1 " <<<"$out"
  [[ "$out" != *tmpfs* && "$out" != *"/run/user"* ]]
'

it "disk_usage falls back to df -Pk without types (BSD/macOS)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  export PATH="$ROOT/tests/mocks/bin:$PATH" MOCK_DF_NO_TYPE=1
  out="$(disk_usage 2>/dev/null)"
  grep -qE "^  /  +/dev/disk3s1 +- .* 5%$" <<<"$out"
  [[ "$out" != *devfs* ]]
  ! disk_usage --bogus 2>/dev/null
'

it "disk_usage colors the bar by fill: green, yellow from 75%, red from 90%" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  export PATH="$ROOT/tests/mocks/bin:$PATH"
  __gash_use_color() { return 0; }
  out="$(disk_usage 2>/dev/null)"
  [[ "$(grep " 21%" <<<"$out")" == *"${__GASH_GREEN}"* ]]
  [[ "$(grep " 81%" <<<"$out")" == *"${__GASH_YELLOW}"* ]]
  [[ "$(grep " 92%" <<<"$out")" == *"${__GASH_BOLD_RED}"* ]]
'

it "dirs_find_large works without numfmt (pure-bash IEC fallback)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Shadow numfmt with a failing stub at the front of PATH.
  # The pure-bash fallback in __gash_fs_parse_size must kick in transparently.
  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  cat > "$tmpbin/numfmt" <<STUB
#!/usr/bin/env bash
exit 127
STUB
  chmod +x "$tmpbin/numfmt"
  export PATH="$tmpbin:$PATH"

  tmp="$(mktemp -d)"
  mkdir -p "$tmp/d1"
  dd if=/dev/zero of="$tmp/d1/big" bs=1024 count=64 status=none

  out="$(dirs_find_large --size 1K "$tmp" 2>/dev/null || true)"
  /bin/rm -rf "$tmp"
  [[ "$out" == *"d1"* ]]
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

it "process_kill without a terminal refuses unless --force, whatever the number of matches" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  # pgrep mock: one match for my-proc, two for many-proc.
  printf "#!/bin/sh\ncase \"\$*\" in *many-proc*) printf \"3333\\\\n4444\\\\n\" ;; *my-proc*) echo 2222 ;; *) exit 1 ;; esac\n" >"$tmp/pgrep"
  chmod +x "$tmp/pgrep"
  export PATH="$tmp:$PATH" MOCK_KILL_LOG="$tmp/kill.log"
  kill() { printf "%s\n" "$*" >>"$MOCK_KILL_LOG"; return 1; }

  export GASH_HEADLESS=1
  for name in my-proc many-proc; do
    out="$(process_kill "$name" 2>&1)" && { echo "killed without --force: $name"; exit 1; }
    [[ "$out" == *"pass --force"* ]]
  done
  [[ ! -e "$MOCK_KILL_LOG" ]]

  process_kill my-proc --force >/dev/null 2>&1
  process_kill --force many-proc >/dev/null 2>&1
  process_kill -f my-proc >/dev/null 2>&1
  grep -qx 2222 "$MOCK_KILL_LOG"
  grep -qx 3333 "$MOCK_KILL_LOG"
  grep -qx 4444 "$MOCK_KILL_LOG"
  ! process_kill --bogus my-proc 2>/dev/null

  # Not headless but no controlling terminal: same refusal.
  if setsid -w true </dev/null >/dev/null 2>&1; then
    : >"$MOCK_KILL_LOG"
    unset GASH_HEADLESS
    out="$(setsid -w bash -c "source \"$ROOT/tests/gash-test.sh\"; gash_source_all \"$ROOT\"; kill() { echo \"\$*\" >>\"$MOCK_KILL_LOG\"; }; process_kill my-proc" </dev/null 2>&1)" && exit 1
    [[ "$out" == *"pass --force"* && ! -s "$MOCK_KILL_LOG" ]]
  fi
'

it "gash_inspiring_quote skips source comments and shows them with --source" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/quotes"
  printf "%s\n" "# Header comment" "" "# A Book, 1999" "\"Only quote.\" — Someone" > "$tmp/quotes/list.txt"
  export GASH_DIR="$tmp"
  for _ in 1 2 3 4 5; do
    out="$(gash_inspiring_quote)"
    [[ "$out" == "Quote: \"Only quote.\" — Someone" ]]
  done
  out="$(gash_inspiring_quote --source)"
  [[ "$out" == *"Source: A Book, 1999"* ]]
  ! gash_inspiring_quote --bogus 2>/dev/null
'

describe "Quotes file (quotes/list.txt)"

it "every quote has a source line, the right shape, and no duplicate" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  file="$ROOT/quotes/list.txt"
  prev="" n=0 bad=0
  declare -A seen=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      "" | "#"*) ;;
      *)
        n=$((n + 1))
        if [[ "$prev" != "# "?* || "$prev" == "# Gash"* ]]; then
          echo "no source line above: $line"; bad=1
        fi
        if [[ ! "$line" =~ ^\".+\"\ —\ [^\ ].*[^\ ]$ ]]; then
          echo "not \"Text.\" — Author: $line"; bad=1
        fi
        key="${line%%\" — *}"; key="${key,,}"
        if [[ -n "${seen[$key]-}" ]]; then
          echo "duplicate: $line"; bad=1
        fi
        seen[$key]=1
        ;;
    esac
    prev="$line"
  done <"$file"
  (( n >= 300 )) || { echo "only $n quotes"; bad=1; }
  (( bad == 0 ))
'
