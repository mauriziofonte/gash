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
