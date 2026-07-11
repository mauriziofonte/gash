#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

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

  git_repair >/dev/null 2>&1

  # After repair, the blob is restored and fsck reports a clean repo.
  git cat-file -p "$blob_hash" >/dev/null 2>&1
  git fsck --full >/dev/null 2>&1
'
