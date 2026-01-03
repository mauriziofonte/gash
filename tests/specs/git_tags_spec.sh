#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Git tag helpers"

it "fails gracefully when origin is missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  cd "$tmp"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test"
  echo x > f
  git add f
  git commit -q -m init

  out="$({ git_list_tags; } 2>&1 || true)"
  [[ "$out" == *"Remote"* && "$out" == *origin* && "$out" == *"not configured"* ]]
'

it "creates and deletes tags against a local origin" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT

  origin="$tmp/origin.git"
  work="$tmp/work"
  mkdir -p "$origin"
  git init -q --bare "$origin"

  mkdir -p "$work"
  cd "$work"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test"
  echo x > f
  git add f
  git commit -q -m init
  git remote add origin "$origin"

  git_add_tag v-test "test tag" >/dev/null 2>&1
  # tag should exist locally and remotely
  git rev-parse v-test >/dev/null 2>&1
  git ls-remote --tags origin 2>/dev/null | grep -q "refs/tags/v-test"

  git_delete_tag v-test >/dev/null 2>&1
  ! git rev-parse v-test >/dev/null 2>&1
  ! git ls-remote --tags origin | grep -q "refs/tags/v-test"
'
