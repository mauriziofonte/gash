#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "Git repair (gheal)"

it "errors outside a git repository" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  cd "$tmp"

  out="$({ git_repair; } 2>&1 || true)"
  [[ "$out" == *"Not in a git repository."* ]]
'

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

  out="$({ git_repair; } 2>&1 || true)"
  [[ "$out" == *"Remote"* && "$out" == *origin* && "$out" == *"not configured"* ]]
'

it "repairs a repo with corrupted loose objects" bash -c '
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
  echo "hello world" > f
  git add f
  git commit -q -m init
  git remote add origin "$origin"
  git push -q origin HEAD:refs/heads/main

  blob_hash="$(git rev-parse HEAD:f)"
  obj_path=".git/objects/${blob_hash:0:2}/${blob_hash:2}"
  [[ -f "$obj_path" ]]

  # Corrupt the loose object (simulate crash/OOM truncation to 0 bytes).
  # Git creates loose objects read-only, so the write bit must be restored.
  chmod u+w "$obj_path"
  : > "$obj_path"
  [[ ! -s "$obj_path" ]]

  # Before repair, the blob content is unreadable (cat-file -e only checks
  # existence and is fooled by a 0-byte file; -p actually inflates it).
  ! git cat-file -p "$blob_hash" >/dev/null 2>&1

  git_repair -y >/dev/null 2>&1

  # After repair, the blob is restored and fsck reports a clean repo.
  git cat-file -p "$blob_hash" >/dev/null 2>&1
  git fsck --full >/dev/null 2>&1
'

it "repairs inside a linked worktree, and --dry-run changes nothing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  git init -q --bare "$tmp/origin.git"
  git init -q "$tmp/work"
  cd "$tmp/work"
  git config user.email t@example.com; git config user.name T
  echo "hello" > f; git add f; git commit -q -m init
  git remote add origin "$tmp/origin.git"
  git push -q origin HEAD:refs/heads/main
  git worktree add -q "$tmp/wt" -b side
  cd "$tmp/wt"

  blob="$(git rev-parse HEAD:f)"
  obj="$tmp/work/.git/objects/${blob:0:2}/${blob:2}"
  chmod u+w "$obj"; : > "$obj"

  out="$(git_repair --dry-run 2>&1)"
  [[ "$out" == *"Empty (corrupted) object files: 1"* && ! -s "$obj" && -e "$obj" ]]
  rc=0; git_repair </dev/null >/dev/null 2>&1 || rc=$?
  [[ $rc -eq 1 && -e "$obj" ]]

  git_repair -y >/dev/null 2>&1
  [[ "$(git cat-file -p "$blob")" == "hello" ]]
  git fsck --full >/dev/null 2>&1
'

it "says there is nothing to repair on a healthy repository" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  git init -q "$tmp/work"; cd "$tmp/work"
  git config user.email t@example.com; git config user.name T
  echo x > f; git add f; git commit -q -m init
  git remote add origin "$tmp/nowhere.git"
  out="$(git_repair 2>&1)"
  [[ "$out" == *"nothing to repair"* ]]
'
