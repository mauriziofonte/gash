#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "PHP version detection (php_detect_version)"

it "reads .target-php-version (highest priority)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  echo "8.1" > "$tmp/.target-php-version"
  echo "{\"require\":{\"php\":\"^8.3\"}}" > "$tmp/composer.json"

  out="$(php_detect_version "$tmp" --llm)"
  echo "$out" | jq -e ".version == \"8.1\"" >/dev/null
'

it ".target-php-version walks up parent directories" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  echo "7.4" > "$tmp/.target-php-version"
  mkdir -p "$tmp/sub/deeper"

  out="$(php_detect_version "$tmp/sub/deeper" --llm)"
  echo "$out" | jq -e ".version == \"7.4\"" >/dev/null
'

it "falls back to composer.json require.php" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  echo "{\"require\":{\"php\":\"^8.2\"}}" > "$tmp/composer.json"

  out="$(php_detect_version "$tmp" --llm)"
  echo "$out" | jq -e ".version == \"8.2\"" >/dev/null
'

it "falls back to 8.4 when nothing explicit is found" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT

  out="$(php_detect_version "$tmp" --llm)"
  echo "$out" | jq -e ".version == \"8.4\"" >/dev/null
'

it "matches the exact JSON field names of the legacy detect-php-version.sh contract" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  out="$(php_detect_version "$tmp" --llm)"
  for field in version php_raw_binary php_command composer_raw_binary composer_command; do
    echo "$out" | jq -e "has(\"$field\")" >/dev/null
  done
'

it "emits JSON automatically under GASH_HEADLESS=1 even without --llm" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1
  out="$(php_detect_version "$tmp")"
  echo "$out" | jq -e ".version == \"8.4\"" >/dev/null
'

it "prints human-readable text when interactive (no --llm, no headless)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  out="$(php_detect_version "$tmp" 2>&1)"
  [[ "$out" == *"PHP version for"* ]]
  ! echo "$out" | jq -e "." >/dev/null 2>&1
'
