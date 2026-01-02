#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=lib/functions.sh
source "$ROOT_DIR/lib/functions.sh"

describe "Functions (edge)"

it "largest_files handles paths with spaces" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/file with space.txt" bs=1024 count=1 status=none
  dd if=/dev/zero of="$tmp/normal.txt" bs=1024 count=2 status=none

  out="$(largest_files "$tmp" 2>/dev/null)"
  [[ "$out" == *"file with space.txt"* ]]
'

it "largest_dirs handles spaces and lists only directories" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/dir one" "$tmp/dir_two"
  dd if=/dev/zero of="$tmp/dir one/big" bs=1024 count=3 status=none
  dd if=/dev/zero of="$tmp/dir_two/small" bs=1024 count=1 status=none
  : > "$tmp/top file.txt"

  out="$(largest_dirs "$tmp" 2>/dev/null)"
  [[ "$out" == *"dir one"* ]]
  [[ "$out" == *"dir_two"* ]]
  [[ "$out" != *"top file.txt"* ]]
'

it "largest_dirs lists subdirectories" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b"
  dd if=/dev/zero of="$tmp/a/file" bs=1024 count=10 status=none

  out="$(largest_dirs "$tmp" 2>/dev/null)"
  [[ "$out" == *"$tmp/a"* || "$out" == *"/a"* ]]
'

it "hgrep filters history output" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  # Override history builtin with a function
  history() {
    cat <<EOF
  1  echo hello
  2  ls -la
  3  echo world
EOF
  }

  out="$(hgrep echo 2>/dev/null)"
  [[ "$out" == *"hello"* ]]
  [[ "$out" == *"world"* ]]
'

it "hgrep fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(hgrep 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "pskill calls kill for matching processes" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  export MOCK_KILL_LOG="$tmp/kill.log"

  # Override bash builtin kill with a function we can observe.
  kill() {
    printf "%s\n" "$*" >> "$MOCK_KILL_LOG"
    return 0
  }

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  pskill my-proc >/dev/null

  grep -q -- "-9 2222" "$MOCK_KILL_LOG"
'

it "psgrep fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(psgrep 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "pskill fails without calling kill when missing arg" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  export MOCK_KILL_LOG="$tmp/kill.log"

  kill() {
    printf "%s\n" "$*" >> "$MOCK_KILL_LOG"
    return 0
  }

  set +e
  out="$(pskill 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
  [[ ! -f "$MOCK_KILL_LOG" || ! -s "$MOCK_KILL_LOG" ]]
'

it "please runs sudo for explicit command" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(please echo ok)"
  [[ "$out" == "ok" ]]
'

it "mkcd creates dir and cds" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkcd "$tmp/newdir"
  [[ "$PWD" == "$tmp/newdir" ]]
'

it "mkcd fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(mkcd 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "extract fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(extract 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "backup_file fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(backup_file 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "portkill fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(portkill 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "docker_prune_all returns 0 when user declines" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  export PATH="$ROOT/tests/mocks/bin:/usr/bin:/bin"

  # Override the prompt to simulate a "No" response.
  needs_confirm_prompt() { return 1; }

  # docker_prune_all uses a prompt helper that returns non-zero on "No"; avoid errexit.
  set +e
  docker_prune_all
  rc=$?
  set -e
  [[ $rc -eq 0 ]]
'

it "git_dump_revisions writes per-commit files" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  cd "$tmp"

  git init -q
  git config user.email test@example.com
  git config user.name Test

  mkdir -p d
  echo v1 > d/f.txt
  git add d/f.txt
  git commit -q -m c1

  echo v2 > d/f.txt
  git add d/f.txt
  git commit -q -m c2

  git_dump_revisions d/f.txt >/dev/null

  # Expect at least two dumped versions
  count="$(/usr/bin/find "$tmp" -type f -name "f.txt.*" | /usr/bin/wc -l | /usr/bin/tr -d " ")"
  [[ "$count" -ge 2 ]]
'

it "git_dump_revisions fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  set +e
  out="$(git_dump_revisions 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Please specify"* ]]
'

it "git_apply_feature_patch fails gracefully when missing args under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  cd "$tmp"

  git init -q
  git config user.email test@example.com
  git config user.name Test

  set +e
  out="$(git_apply_feature_patch 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Usage:"* ]]
'

it "git_apply_feature_patch errors outside git repo" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  cd "$tmp"

  out="$({ git_apply_feature_patch main feat 123; } 2>&1 || true)"
  [[ "$out" == *"Not in a Git repository"* ]]
'

it "gash_uninstall returns error when not installed" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  out="$({ HOME="$tmp" gash_uninstall; } 2>&1 || true)"
  [[ "$out" == *"Gash is not installed"* ]]
'
