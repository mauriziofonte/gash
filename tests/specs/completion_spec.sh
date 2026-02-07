#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Completion"

it "bash_completion returns early if gash functions are missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  out="$(bash --noprofile --norc -c "source \"$ROOT/bash_completion\"; echo ok" 2>&1)"
  [[ "$out" == "ok" ]]
'

it "completion returns empty for no-arg functions" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("disk_usage" "")
COMP_CWORD=1
__gash_complete

[[ \${#COMPREPLY[@]} -eq 0 ]]
echo "PASS"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"PASS"* ]]
'

it "completion suggests git tags for git_add_tag" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  # Create a temp git repo with a tag
  gitdir="$tmp/repo"
  mkdir -p "$gitdir"
  git -C "$gitdir" init -q
  git -C "$gitdir" commit --allow-empty -m "init" -q
  git -C "$gitdir" tag v1.0.0
  git -C "$gitdir" tag v2.0.0

  inner_cmd="$(cat <<EOF
set -u
cd "$gitdir"
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("git_add_tag" "v1")
COMP_CWORD=1
__gash_complete

printf "%s\n" "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"v1.0.0"* ]]
  [[ "$out" != *"v2.0.0"* ]]
'

it "completion suggests directories for files_largest" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  # Create test directories
  workdir="$tmp/work"
  mkdir -p "$workdir/alpha_dir" "$workdir/beta_dir"
  touch "$workdir/some_file.txt"

  inner_cmd="$(cat <<EOF
set -u
cd "$workdir"
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("files_largest" "al")
COMP_CWORD=1
__gash_complete

printf "%s\n" "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"alpha_dir"* ]]
  # Should not contain files
  [[ "$out" != *"some_file"* ]]
  # Should not contain beta_dir (does not match "al" prefix)
  [[ "$out" != *"beta_dir"* ]]
'

it "completion suggests sections for gash command" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("gash" "gi")
COMP_CWORD=1
__gash_complete

printf "%s\n" "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"git"* ]]
'

it "completion suggests files for archive_extract" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  # Create test files
  workdir="$tmp/work"
  mkdir -p "$workdir"
  touch "$workdir/archive.tar.gz" "$workdir/backup.zip"

  inner_cmd="$(cat <<EOF
set -u
cd "$workdir"
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("archive_extract" "ar")
COMP_CWORD=1
__gash_complete

printf "%s\n" "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"archive.tar.gz"* ]]
  [[ "$out" != *"backup.zip"* ]]
'

it "completion suggests AI providers for ai_ask" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

COMP_WORDS=("ai_ask" "cl")
COMP_CWORD=1
__gash_complete

printf "%s\n" "\${COMPREPLY[@]}"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"claude"* ]]
  [[ "$out" != *"gemini"* ]]
'

it "completion is registered for functions not aliases" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner_cmd="$(cat <<EOF
set -u
source "\$HOME/.gash/gash.sh" >/dev/null 2>&1
source "\$HOME/.gash/bash_completion" >/dev/null 2>&1

# Function completion should be registered
complete -p docker_stop_all >/dev/null 2>&1 || { echo "FAIL_FUNC"; exit 1; }

# Alias completion should NOT be registered (bash expands aliases first)
if complete -p dsa >/dev/null 2>&1; then
    echo "FAIL_ALIAS"
    exit 1
fi

echo "PASS"
EOF
)"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$inner_cmd" 2>/dev/null)"
  [[ "$out" == *"PASS"* ]]
'
