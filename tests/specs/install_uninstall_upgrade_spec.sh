#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "Install / Uninstall / Upgrade"

it "install.sh installs into HOME and updates profile" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  profile="$tmp_home/.bashrc"
  : > "$profile"

  # Use local repo as git source (no network).
  HOME="$tmp_home" \
  PROFILE="$profile" \
  GASH_INSTALL_GIT_REPO="$ROOT" \
  bash "$ROOT/install.sh" --assume-yes --quiet >/dev/null 2>&1

  [[ -d "$tmp_home/.gash" ]]
  [[ -f "$tmp_home/.gashrc" ]]

  # Profile should source ~/.gashrc
  grep -q "source \"\$HOME/.gashrc\"" "$profile"
'

it "gash_uninstall removes gash and cleans profile" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  mkdir -p "$tmp_home/.gash"
  : > "$tmp_home/.gashrc"

  # Create a fake profile containing the install block.
  profile="$tmp_home/.bashrc"
  cat > "$profile" <<EOF
# something before
# Load Gash Bash
# This loads the Gash customized Bash.
if [ -f "\$HOME/.gashrc" ]; then
  source "\$HOME/.gashrc"
fi
# something after
EOF

  # Simulate user confirmation.
  needs_confirm_prompt() { return 0; }

  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1 || true

  [[ ! -e "$tmp_home/.gashrc" ]]
  [[ ! -d "$tmp_home/.gash" ]]

  # Install block should be removed; other lines should remain.
  ! grep -q "Load Gash Bash" "$profile"
  grep -q "something before" "$profile"
  grep -q "something after" "$profile"
'

it "gash_upgrade updates from older tag to latest tag" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT

  origin="$tmp/origin.git"
  work="$tmp/work"

  git init -q "$work"
  cd "$work"
  git config user.email test@example.com
  git config user.name Test

  echo v1 > f
  git add f
  git commit -q -m c1
  git tag -a v1 -m v1

  # Ensure tag timestamps differ so "latest tag" selection is deterministic.
  /bin/sleep 1

  echo v2 > f
  git add f
  git commit -q -m c2
  git tag -a v2 -m v2

  git init -q --bare "$origin"
  git remote add origin "$origin"
  git push -q origin --all
  git push -q origin --tags

  # Clone into HOME/.gash and checkout older tag.
  tmp_home="$tmp/home"
  mkdir -p "$tmp_home"
  git clone -q "$origin" "$tmp_home/.gash" 2>/dev/null
  cd "$tmp_home/.gash"
  git -c advice.detachedHead=false checkout -q v1 2>/dev/null

  # Now upgrade should move to v2.
  HOME="$tmp_home" GASH_DIR="$tmp_home/.gash" gash_upgrade >/dev/null 2>&1

  cd "$tmp_home/.gash"
  current_tag="$(git describe --tags --abbrev=0 2>/dev/null)"
  [[ "$current_tag" == "v2" ]]
'

it "gash_uninstall preserves other if/fi blocks in profile" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  mkdir -p "$tmp_home/.gash"
  : > "$tmp_home/.gashrc"

  # Profile with Gash block + another if/fi block
  profile="$tmp_home/.bashrc"
  cat > "$profile" <<EOF
# Something before
# Load Gash Bash
# This loads the Gash customized Bash.
if [ -f "\$HOME/.gashrc" ]; then
  source "\$HOME/.gashrc"
fi
# My custom block
if [ -f "\$HOME/.my_config" ]; then
  echo "Loading custom"
fi
# Something after
EOF

  needs_confirm_prompt() { return 0; }
  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1 || true

  # Gash block removed
  ! grep -q "Load Gash Bash" "$profile"

  # Other blocks preserved
  grep -q "My custom block" "$profile"
  grep -q "Loading custom" "$profile"
  grep -q "Something after" "$profile"
'

it "gash_uninstall removes new format with delimiters" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  mkdir -p "$tmp_home/.gash"
  : > "$tmp_home/.gashrc"

  # Profile with new delimited format
  profile="$tmp_home/.bashrc"
  cat > "$profile" <<EOF
# Something before
# >>> GASH START >>>
# Load Gash Bash - Do not edit this block
if [ -f "\$HOME/.gashrc" ]; then
  source "\$HOME/.gashrc"
fi
# <<< GASH END <<<
# Something after
EOF

  needs_confirm_prompt() { return 0; }
  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1 || true

  # Gash block removed
  ! grep -q "GASH START" "$profile"
  ! grep -q "GASH END" "$profile"
  ! grep -q "gashrc" "$profile"

  # Other content preserved
  grep -q "Something before" "$profile"
  grep -q "Something after" "$profile"
'

it "gash_uninstall removes custom GASH_DIR location" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  custom_dir="$tmp/custom_gash"
  mkdir -p "$custom_dir"
  : > "$tmp/.gashrc"
  : > "$tmp/.bashrc"

  needs_confirm_prompt() { return 0; }
  HOME="$tmp" GASH_DIR="$custom_dir" gash_uninstall >/dev/null 2>&1 || true

  [[ ! -d "$custom_dir" ]]
'

it "gash_uninstall cleans all profile files" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  mkdir -p "$tmp_home/.gash"
  : > "$tmp_home/.gashrc"

  gash_block="# Load Gash Bash
# comment
if [ -f \"\$HOME/.gashrc\" ]; then
  source \"\$HOME/.gashrc\"
fi"

  # Create all profile files with Gash block
  for f in .bashrc .bash_profile .profile; do
    echo "$gash_block" > "$tmp_home/$f"
  done

  needs_confirm_prompt() { return 0; }
  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1 || true

  # Verify removal from all
  for f in .bashrc .bash_profile .profile; do
    ! grep -q "Load Gash Bash" "$tmp_home/$f"
  done
'

it "gash_uninstall handles missing profile files gracefully" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  mkdir -p "$tmp_home/.gash"
  : > "$tmp_home/.gashrc"
  # NO profile files created

  needs_confirm_prompt() { return 0; }
  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1

  # Should succeed
  [[ ! -d "$tmp_home/.gash" ]]
'

it "install then uninstall leaves system clean" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  profile="$tmp_home/.bashrc"
  echo "# Original content" > "$profile"

  # Install
  HOME="$tmp_home" \
  PROFILE="$profile" \
  GASH_INSTALL_GIT_REPO="$ROOT" \
  bash "$ROOT/install.sh" --assume-yes --quiet >/dev/null 2>&1

  [[ -d "$tmp_home/.gash" ]]

  # Uninstall
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  needs_confirm_prompt() { return 0; }
  HOME="$tmp_home" gash_uninstall >/dev/null 2>&1 || true

  # Verify clean state
  [[ ! -d "$tmp_home/.gash" ]]
  [[ ! -f "$tmp_home/.gashrc" ]]
  ! grep -q "gashrc" "$profile"
  grep -q "Original content" "$profile"
'

it "gash_upgrade reports already up-to-date" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT

  # Setup git repo with one tag
  work="$tmp/work"
  origin="$tmp/origin.git"

  git init -q "$work"
  cd "$work"
  git config user.email test@example.com
  git config user.name Test
  echo v1 > f
  git add f
  git commit -q -m c1
  git tag -a v1 -m v1

  git init -q --bare "$origin"
  git remote add origin "$origin"
  git push -q origin --all
  git push -q origin --tags

  # Clone and checkout v1
  tmp_home="$tmp/home"
  mkdir -p "$tmp_home"
  git clone -q "$origin" "$tmp_home/.gash" 2>/dev/null
  cd "$tmp_home/.gash"
  git -c advice.detachedHead=false checkout -q v1 2>/dev/null

  # Upgrade should report already up-to-date
  out="$(HOME="$tmp_home" GASH_DIR="$tmp_home/.gash" gash_upgrade 2>&1)"
  [[ "$out" == *"already up-to-date"* ]]
'

it "install.sh uses new delimited format" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"

  tmp_home="$(mktemp -d)"; trap "/bin/rm -rf $tmp_home" EXIT
  profile="$tmp_home/.bashrc"
  : > "$profile"

  HOME="$tmp_home" \
  PROFILE="$profile" \
  GASH_INSTALL_GIT_REPO="$ROOT" \
  bash "$ROOT/install.sh" --assume-yes --quiet >/dev/null 2>&1

  # Check for new delimiters
  grep -q ">>> GASH START >>>" "$profile"
  grep -q "<<< GASH END <<<" "$profile"
'
