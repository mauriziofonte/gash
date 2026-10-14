#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Functions (smoke)"

it "gash_help displays grouped overview" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  export GASH_VERSION="test"

  out="$(gash_help 2>&1)"
  [[ "$out" == *"Gash"* ]]
  [[ "$out" == *"FILES"* ]]
  [[ "$out" == *"GIT"* ]]
  [[ "$out" == *"files_largest"* ]]
  [[ "$out" == *"git_add_tag"* ]]
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

describe "WSL Functions"

it "wsl_restart shows help on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(wsl_restart -h 2>&1)"
  [[ "$out" == *"wsl_restart"* ]]
'

it "wsl_shutdown shows help on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(wsl_shutdown -h 2>&1)"
  [[ "$out" == *"wsl_shutdown"* ]]
'

it "wsl_explorer shows help on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(wsl_explorer -h 2>&1)"
  [[ "$out" == *"wsl_explorer"* ]]
'

it "wsl_taskmanager shows help on -h" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(wsl_taskmanager -h 2>&1)"
  [[ "$out" == *"wsl_taskmanager"* ]]
'

it "wsl_restart fails outside WSL with mocked detection" bash -c '
  set -uo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Override WSL detection to always fail
  __gash_require_wsl() {
      __gash_error "This command requires WSL (Windows Subsystem for Linux)."
      return 1
  }

  set +e
  out="$(wsl_restart 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"WSL"* ]]
'

it "wsl_explorer fails outside WSL with mocked detection" bash -c '
  set -uo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  __gash_require_wsl() {
      __gash_error "This command requires WSL (Windows Subsystem for Linux)."
      return 1
  }

  set +e
  out="$(wsl_explorer 2>&1)"
  rc=$?
  set -e

  [[ $rc -ne 0 ]]
  [[ "$out" == *"WSL"* ]]
'

it "wr alias points to wsl_restart" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  alias_def="$(alias wr 2>/dev/null)"
  [[ "$alias_def" == *"wsl_restart"* ]]
'

it "wex alias points to wsl_explorer" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  alias_def="$(alias wex 2>/dev/null)"
  [[ "$alias_def" == *"wsl_explorer"* ]]
'

describe "Functions (smoke) - port_kill"

it "port_kill targets listeners only, validates the port and asks first" bash -c '
  set -uo pipefail
  tmp="$(mktemp -d)"; trap "kill \$victim 2>/dev/null; /bin/rm -rf \"$tmp\"" EXIT
  sleep 30 & victim=$!
  # lsof mock: answers only the listener queries, logging every call.
  printf "#!/bin/sh\necho \"\$*\" >>\"%s/lsof.log\"\ncase \"\$*\" in *-sTCP:LISTEN*) echo %s ;; esac\n" "$tmp" "$victim" >"$tmp/lsof"
  chmod +x "$tmp/lsof"
  export PATH="$tmp:$PATH" GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"

  port_kill 70000 2>/dev/null && exit 1
  port_kill 80x 2>/dev/null && exit 1
  [[ ! -e "$tmp/lsof.log" ]] || exit 1

  # Headless without --force: lists, refuses, kills nothing.
  port_kill 8080 2>/dev/null && exit 1
  kill -0 "$victim" || exit 1
  grep -q -- "-iTCP:8080 -sTCP:LISTEN" "$tmp/lsof.log" || exit 1
  grep -q -- "-i:8080" "$tmp/lsof.log" && exit 1

  port_kill --force 8080 >/dev/null 2>&1 || exit 1
  wait "$victim" 2>/dev/null
  kill -0 "$victim" 2>/dev/null && exit 1
  exit 0
'
