#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "NVM prune (nvmp)"

# NVM is always a shell function (sourced from $NVM_DIR/nvm.sh), never a PATH
# binary — that's why the module checks `declare -f nvm`, not `command -v`.
# So it's mocked here as an inline bash function per test, not a PATH script
# like the docker/curl mocks (there's nothing for `command -v nvm` to find).

it "errors when nvm is not loaded" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$({ nvm_prune_versions; } 2>&1 || true)"
  [[ "$out" == *"nvm is not loaded"* ]]
'

it "dry-run (default) lists duplicates without uninstalling" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  log="$tmp/uninstall.log"

  nvm() {
    case "$1" in
      ls) printf "       v18.18.0\n->     v18.20.4\n       v20.9.0\n       v20.11.0\n       v20.11.1\n" ;;
      current) echo "v18.20.4" ;;
      uninstall) echo "uninstall:$2" >> "$log" ;;
    esac
  }

  out="$(nvm_prune_versions 2>&1)"
  [[ "$out" == *"Dry run: would uninstall"* ]]
  [[ "$out" == *"v18.18.0"* ]]
  [[ "$out" == *"v20.9.0"* ]]
  [[ "$out" == *"v20.11.0"* ]]
  [[ ! -f "$log" ]]
'

it "--force uninstalls only older duplicate patches, never current or latest" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  log="$tmp/uninstall.log"

  nvm() {
    case "$1" in
      ls) printf "       v18.18.0\n->     v18.20.4\n       v20.9.0\n       v20.11.0\n       v20.11.1\n" ;;
      current) echo "v18.20.4" ;;
      uninstall) echo "uninstall:$2" >> "$log" ;;
    esac
  }

  nvm_prune_versions --force >/dev/null 2>&1

  [[ -f "$log" ]]
  grep -q "uninstall:v18.18.0" "$log"
  grep -q "uninstall:v20.9.0" "$log"
  grep -q "uninstall:v20.11.0" "$log"
  ! grep -q "uninstall:v18.20.4" "$log"
  ! grep -q "uninstall:v20.11.1" "$log"
'

describe "Node version detection (node_detect_version)"

# node_detect_version reads $NVM_DIR/versions/node directly (filesystem),
# not via the `nvm` shell function — so tests fake a NVM_DIR tree instead of
# mocking `nvm` like the nvmp tests above.

it "errors when NVM_DIR does not exist" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export NVM_DIR="/nonexistent-$RANDOM-$RANDOM"
  out="$({ node_detect_version; } 2>&1 || true)"
  [[ "$out" == *"NVM directory not found"* ]]
'

it "reads .nvmrc (highest priority)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node/v18.18.0" "$NVM_DIR/versions/node/v20.9.0" "$NVM_DIR/versions/node/v20.11.1"

  proj="$tmp/proj"; mkdir -p "$proj"
  echo "18" > "$proj/.nvmrc"
  echo "{\"engines\":{\"node\":\"20\"}}" > "$proj/package.json"

  out="$(node_detect_version "$proj" --llm)"
  echo "$out" | jq -e ".version == \"18\" and .full_version == \"18.18.0\"" >/dev/null
'

it "falls back to package.json engines.node" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node/v20.9.0"

  proj="$tmp/proj"; mkdir -p "$proj"
  echo "{\"engines\":{\"node\":\">=20\"}}" > "$proj/package.json"

  out="$(node_detect_version "$proj" --llm)"
  echo "$out" | jq -e ".version == \"20\"" >/dev/null
'

it "falls back to latest installed when nothing specified" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node/v18.18.0" "$NVM_DIR/versions/node/v22.14.0"

  proj="$tmp/proj"; mkdir -p "$proj"
  out="$(node_detect_version "$proj" --llm)"
  echo "$out" | jq -e ".version == \"22\" and .full_version == \"22.14.0\"" >/dev/null
'

it "warns and substitutes the closest version when the requested major is not installed" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node/v22.14.0"

  proj="$tmp/proj"; mkdir -p "$proj"
  echo "18" > "$proj/.nvmrc"

  err="$(node_detect_version "$proj" --llm 2>&1 1>/dev/null)"
  out="$(node_detect_version "$proj" --llm 2>/dev/null)"
  [[ "$err" == *"not installed"* ]]
  echo "$out" | jq -e ".version == \"22\"" >/dev/null
'

it "matches the exact JSON field names of the legacy detect-node-version.sh contract" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export NVM_DIR="$tmp/nvm"
  mkdir -p "$NVM_DIR/versions/node/v20.9.0"

  out="$(node_detect_version "$tmp" --llm)"
  for field in version full_version node_command npm_command npx_command; do
    echo "$out" | jq -e "has(\"$field\")" >/dev/null
  done
'
