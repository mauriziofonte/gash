#!/usr/bin/env bash

# shellcheck source=lib/functions.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/lib/functions.sh"

describe "SSH auto-unlock"

it "parses ~/.gash_ssh_credentials tolerantly" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"
  trap "rm -rf $tmp" EXIT

  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"
  : > "$tmp/.ssh/key2"

  # includes: CRLF, whitespace, tilde, comment, bad line, missing key
  cat > "$tmp/creds" <<EOF
# comment\r
  ~/.ssh/key1:\tP@ss:with:colons\r
~/.ssh/key2:  leading-space-password\t\r
badlinewithoutcolon\r
~/.ssh/missing:pw\r
EOF

  export HOME="$tmp"
  parsed="$(__gash_read_ssh_credentials_file "$tmp/creds")"

  # should include 2 valid rows
  count="$(printf "%s" "$parsed" | grep -v "^__GASH_PARSE_ERROR__" | wc -l | tr -d " ")"
  [[ "$count" == "2" ]]
'

it "prints install hint when expect is missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"

  cat > "$tmp/.gash_ssh_credentials" <<EOF
~/.ssh/key1:pw
EOF

  # PATH has ssh-add + core utils, but no expect (even if installed on the system)
  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  ln -s "$ROOT/tests/mocks/bin/ssh-add" "$tmpbin/ssh-add"
  ln -s "$(command -v base64)" "$tmpbin/base64"
  ln -s "$(command -v mktemp)" "$tmpbin/mktemp"
  ln -s "$(command -v tr)" "$tmpbin/tr"
  ln -s "$(command -v rm)" "$tmpbin/rm"
  export PATH="$tmpbin"
  export MOCK_SSH_AGENT=1

  out="$({ HOME="$tmp" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"
  [[ "$out" == *"expect"* && "$out" == *"install"* ]]
'

it "handles ssh-agent not running (no SSH_AUTH_SOCK)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"

  cat > "$tmp/.gash_ssh_credentials" <<EOF
~/.ssh/key1:pw
EOF

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SSH_AGENT=0

  out="$({ HOME="$tmp" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"
  [[ "$out" == *"ssh-agent is not running"* ]]
'

it "formats expect output using gash style" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/functions.sh"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"
  : > "$tmp/.ssh/key2"

  cat > "$tmp/.gash_ssh_credentials" <<EOF
~/.ssh/key1:pw
~/.ssh/key2:pw2
EOF

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SSH_AGENT=1

  out="$({ HOME="$tmp" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"

  # spawn lines should be filtered; output should be prefixed and readable
  [[ "$out" != *"spawn ssh-add"* ]]
  [[ "$out" == *"SSH:"*"Identity added"* ]]
  [[ "$out" == *"no identities"* ]]
'
