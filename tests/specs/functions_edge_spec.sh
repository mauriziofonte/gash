#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Functions (edge)"

it "files_largest handles paths with spaces" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/file with space.txt" bs=1024 count=1 status=none
  dd if=/dev/zero of="$tmp/normal.txt" bs=1024 count=2 status=none

  out="$(files_largest "$tmp" 2>/dev/null)"
  [[ "$out" == *"file with space.txt"* ]]
'

it "dirs_largest handles spaces and lists only directories" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/dir one" "$tmp/dir_two"
  dd if=/dev/zero of="$tmp/dir one/big" bs=1024 count=3 status=none
  dd if=/dev/zero of="$tmp/dir_two/small" bs=1024 count=1 status=none
  : > "$tmp/top file.txt"

  out="$(dirs_largest "$tmp" 2>/dev/null)"
  [[ "$out" == *"dir one"* ]]
  [[ "$out" == *"dir_two"* ]]
  [[ "$out" != *"top file.txt"* ]]
'

it "dirs_largest lists subdirectories" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b"
  dd if=/dev/zero of="$tmp/a/file" bs=1024 count=10 status=none

  out="$(dirs_largest "$tmp" 2>/dev/null)"
  [[ "$out" == *"$tmp/a"* || "$out" == *"/a"* ]]
'

it "history_grep filters history output" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Override history builtin with a function
  history() {
    cat <<EOF
  1  echo hello
  2  ls -la
  3  echo world
EOF
  }

  out="$(history_grep echo 2>/dev/null)"
  [[ "$out" == *"hello"* ]]
  [[ "$out" == *"world"* ]]
'

it "history_grep fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(history_grep 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "process_kill calls kill for matching processes" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  export MOCK_KILL_LOG="$tmp/kill.log"

  # Override bash builtin kill with a function we can observe.
  kill() {
    printf "%s\n" "$*" >> "$MOCK_KILL_LOG"
    return 0
  }

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  process_kill my-proc >/dev/null

  grep -q -- "-9 2222" "$MOCK_KILL_LOG"
'

it "process_find fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(process_find 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "process_kill fails without calling kill when missing arg" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  export MOCK_KILL_LOG="$tmp/kill.log"

  kill() {
    printf "%s\n" "$*" >> "$MOCK_KILL_LOG"
    return 0
  }

  set +e
  out="$(process_kill 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
  [[ ! -f "$MOCK_KILL_LOG" || ! -s "$MOCK_KILL_LOG" ]]
'

it "sudo_last runs sudo for explicit command" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(sudo_last echo ok)"
  [[ "$out" == "ok" ]]
'

it "mkdir_cd creates dir and cds" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  mkdir_cd "$tmp/newdir"
  [[ "$PWD" == "$tmp/newdir" ]]
'

it "mkdir_cd fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(mkdir_cd 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "archive_extract fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(archive_extract 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "file_backup fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(file_backup 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "port_kill fails gracefully when missing arg under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(port_kill 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "docker_prune_all returns 0 when user declines" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

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
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

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
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  set +e
  out="$(git_dump_revisions 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Missing"* ]]
'

it "git_apply_patch fails gracefully when missing args under nounset" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  cd "$tmp"

  git init -q
  git config user.email test@example.com
  git config user.name Test

  set +e
  out="$(git_apply_patch 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"Usage:"* ]]
'

it "git_apply_patch errors outside git repo" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  cd "$tmp"

  out="$({ git_apply_patch main feat 123; } 2>&1 || true)"
  [[ "$out" == *"Not in a git repository"* ]]
'

it "gash_uninstall returns error when not installed" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  out="$({ HOME="$tmp" gash_uninstall; } 2>&1 || true)"
  [[ "$out" == *"Gash is not installed"* ]]
'
