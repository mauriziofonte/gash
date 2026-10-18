#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

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

it "tag checks are exact: v1.0 is not v1.0.1, nor a branch called v1.0" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  git init -q --bare "$tmp/origin.git"
  mkdir "$tmp/work"; cd "$tmp/work"
  git init -q; git config user.email t@e; git config user.name T
  echo x >f; git add f; git commit -qm init
  git remote add origin "$tmp/origin.git"
  git tag v1.0.1 && git push -q origin refs/tags/v1.0.1
  git branch v2.0

  # v1.0 does not exist even though v1.0.1 does: delete refuses, add works.
  ! git_delete_tag v1.0 >/dev/null 2>&1
  git_add_tag v1.0 >/dev/null 2>&1
  git ls-remote --exit-code --tags origin refs/tags/v1.0 >/dev/null

  # A branch with the tag name neither blocks the tag nor gets pushed/deleted.
  git_add_tag v2.0 >/dev/null 2>&1
  git_delete_tag v2.0 >/dev/null 2>&1
  git rev-parse -q --verify refs/heads/v2.0 >/dev/null
  ! git rev-parse -q --verify refs/tags/v2.0 >/dev/null
  git ls-remote --exit-code --tags origin refs/tags/v1.0.1 >/dev/null

  ! git_add_tag "bad..name" >/dev/null 2>&1
'
