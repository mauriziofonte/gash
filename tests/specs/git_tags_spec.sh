#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=lib/functions.sh
source "$ROOT_DIR/lib/functions.sh"

describe "Git tag helpers"

it "fails gracefully when origin is missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  cd "$tmp"
  git init -q
  git config user.email "test@example.com"
  git config user.name "Test"
  echo x > f
  git add f
  git commit -q -m init

  out="$({ gtags; } 2>&1 || true)"
  [[ "$out" == *"Remote"* && "$out" == *origin* && "$out" == *"not configured"* ]]
'

it "creates and deletes tags against a local origin" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

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

  gadd_tag v-test "test tag" >/dev/null
  # tag should exist locally and remotely
  git rev-parse v-test >/dev/null
  git ls-remote --tags origin | grep -q "refs/tags/v-test"

  gdel_tag v-test >/dev/null
  ! git rev-parse v-test >/dev/null 2>&1
  ! git ls-remote --tags origin | grep -q "refs/tags/v-test"
'
