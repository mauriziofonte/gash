#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Node upgrade (nvmc)"

# nvm is a shell function, mocked per test. The installed versions are
# directories under a temporary NVM_DIR, as nvm lays them out.
NVM_MOCK='
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node"/{v18.20.3,v20.9.0,v20.11.0,v22.1.0}
  log="$tmp/nvm.log"
  nvm() {
    echo "$*" >> "$log"
    case "$1" in
      ls-remote) [[ -n "${FAIL_REMOTE-}" ]] && return 1; printf "        v18.20.3\n        v18.20.8   (Latest LTS: Hydrogen)\n        v20.19.5   (Latest LTS: Iron)\n        v22.20.0   (Latest LTS: Jod)\n        v24.10.0\n" ;;
      current) echo "v20.9.0" ;;
      install) [[ "$2" == "${FAIL_INSTALL-none}"* ]] && return 1; return 0 ;;
      use|uninstall) return 0 ;;
    esac
  }
'

it "rejects unknown options" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(node_upgrade --bogus 2>&1)" && exit 1
  [[ "$out" == *"Unknown option: --bogus"* ]]
'

it "errors when nvm is not loaded" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(node_upgrade --dry-run 2>&1)" && exit 1
  [[ "$out" == *"nvm is not loaded"* ]]
'

it "--dry-run prints the plan per installed major and changes nothing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  out="$(node_upgrade --dry-run 2>&1)"
  [[ "$out" == *"Latest LTS: v22 (Jod)"* ]]
  [[ "$out" == *"v18.20.8  (replaces v18.20.3"* ]]
  [[ "$out" == *"v20.19.5  (replaces v20.11.0"* ]]
  [[ "$out" == *"v22.20.0  (replaces v22.1.0"* ]]
  # Majors that are not installed are never added.
  [[ "$out" != *"v24"* ]]
  [[ "$out" == *"REMOVED:"*"v18.20.3 v20.9.0 v20.11.0 v22.1.0"* ]]
  [[ "$out" == *"3 newly installed, 4 to remove, 0 up to date."* ]]
  ! grep -qE "^(install|uninstall)" "$log"
'

it "refuses without a terminal unless -y" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  out="$(node_upgrade 2>&1 </dev/null)" && exit 1
  [[ "$out" == *"pass -y"* ]]
  ! grep -qE "^(install|uninstall)" "$log"
'

it "-y installs with the old globals, switches, then removes old patches" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  node_upgrade -y >/dev/null 2>&1
  grep -qx "install v20.19.5 --reinstall-packages-from=v20.11.0" "$log"
  # The active v20.9.0 is removed only after moving to the new v20 patch.
  use_line="$(grep -n "^use v20.19.5" "$log" | cut -d: -f1)"
  rm_line="$(grep -n "^uninstall v20.9.0" "$log" | cut -d: -f1)"
  [[ -n "$use_line" && -n "$rm_line" && "$use_line" -lt "$rm_line" ]]
  grep -qx "uninstall v18.20.3" "$log"
'

it "keeps the patches of a major whose install failed" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  FAIL_INSTALL=v22
  node_upgrade -y >/dev/null 2>&1 || true
  ! grep -q "^uninstall v22.1.0" "$log"
  grep -q "^uninstall v18.20.3" "$log"
'

it "reports up to date when every major is on its newest patch" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  rm -rf "$NVM_DIR/versions/node"/*
  mkdir -p "$NVM_DIR/versions/node/v22.20.0"
  out="$(node_upgrade 2>&1 </dev/null)"
  [[ "$out" == *"0 newly installed, 0 to remove, 1 up to date."* ]]
'

it "fails without touching anything when the remote index is unreadable" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  '"$NVM_MOCK"'
  FAIL_REMOTE=1
  out="$(node_upgrade -y 2>&1)" && exit 1
  [[ "$out" == *"Nothing changed"* ]]
'

describe "Node LTS codenames"

it "resolves lts/<name> through nvm alias files, also new codenames" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/alias/lts"
  echo "v26.1.0" > "$NVM_DIR/alias/lts/futurename"
  echo "v24.10.0" > "$NVM_DIR/alias/lts/*"
  [[ "$(__node_parse_version_to_major lts/futurename)" == "26" ]]
  [[ "$(__node_parse_version_to_major "lts/*")" == "24" ]]
  [[ "$(__node_parse_version_to_major lts/krypton)" == "24" ]]
'
