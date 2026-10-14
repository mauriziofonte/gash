#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Git sync (gsync)"

# A work tree with a bare origin, one commit pushed on main.
REPO='
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  git init -q --bare "$tmp/origin.git"
  git init -q -b main "$tmp/work" 2>/dev/null || { git init -q "$tmp/work"; git -C "$tmp/work" checkout -q -b main; }
  cd "$tmp/work"
  git config user.email t@example.com; git config user.name T
  echo a > a; git add a; git commit -q -m init
  git remote add origin "$tmp/origin.git"
  git push -q -u origin main 2>/dev/null
'

it "commits new, modified and deleted files and pushes" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  echo b > b; echo a2 > a; mkdir sub; echo c > sub/c
  (cd sub && git_sync >/dev/null 2>&1)
  [[ -z "$(git status --porcelain)" ]]
  [[ "$(git log -1 --format=%s)" == "chore: sync" ]]
  [[ "$(git rev-parse HEAD)" == "$(git --git-dir="$tmp/origin.git" rev-parse main)" ]]
  git rm -q b
  git_sync -m "remove b" >/dev/null 2>&1
  [[ "$(git --git-dir="$tmp/origin.git" log -1 --format=%s main)" == "remove b" ]]
'

it "--dry-run shows the plan and changes nothing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  echo new > new
  out="$(git_sync --dry-run 2>&1)"
  [[ "$out" == *"Would commit on main"* && "$out" == *"new"* && "$out" == *"Would push main to origin/main"* ]]
  [[ "$(git status --porcelain)" == "?? new" ]]
'

it "--no-push commits only" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  echo x > x
  git_sync --no-push >/dev/null 2>&1
  [[ "$(git rev-list --count origin/main..HEAD)" == "1" ]]
'

it "pushes a new branch to origin and sets its upstream" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  git checkout -q -b feature
  echo f > f
  git_sync >/dev/null 2>&1
  [[ "$(git rev-parse --abbrev-ref "@{upstream}")" == "origin/feature" ]]
'

it "pushes pending commits even with nothing to commit" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  echo y > y; git add y; git commit -q -m local
  out="$(git_sync 2>&1)"
  [[ "$out" == *"Nothing to commit"* && "$out" == *"Pushed main"* ]]
  [[ "$(git rev-list --count origin/main..HEAD)" == "0" ]]
'

it "refuses on a detached HEAD and during a merge" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  '"$REPO"'
  git checkout -q --detach
  out="$(git_sync 2>&1)" && exit 1
  [[ "$out" == *"Detached HEAD"* ]]
  git checkout -q main
  git rev-parse HEAD > .git/MERGE_HEAD
  out="$(git_sync 2>&1)" && exit 1
  [[ "$out" == *"merge is in progress"* ]]
'

it "rejects unknown options and -m without a message" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(git_sync --bogus 2>&1)" && exit 1
  [[ "$out" == *"Unknown option: --bogus"* ]]
  out="$(git_sync -m 2>&1)" && exit 1
  [[ "$out" == *"-m needs a message"* ]]
'
