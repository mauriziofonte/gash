#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "SSH auto-unlock"

it "parses ~/.gash_env SSH entries tolerantly" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"
  trap "rm -rf $tmp" EXIT

  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"
  : > "$tmp/.ssh/key2"

  # includes: CRLF, whitespace, comments, invalid lines
  cat > "$tmp/.gash_env" <<EOF
# comment
  SSH:~/.ssh/key1=P@ss:with:colons
SSH:~/.ssh/key2=  leading-space-password
badlinewithoutprefix
SSH:~/.ssh/missing=pw
EOF

  export HOME="$tmp"
  export GASH_ENV_FILE="$tmp/.gash_env"
  __GASH_ENV_LOADED=""

  __gash_load_env 2>/dev/null

  parsed="$(__gash_get_ssh_keys)"

  # should include 2 valid rows (missing key file is skipped with warning)
  count="$(printf "%s" "$parsed" | grep -c . | tr -d " ")"
  [[ "$count" == "2" ]]
'

it "prints install hint when expect is missing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"

  cat > "$tmp/.gash_env" <<EOF
SSH:~/.ssh/key1=pw
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

  out="$({ HOME="$tmp" GASH_ENV_FILE="$tmp/.gash_env" __GASH_ENV_LOADED="" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"
  [[ "$out" == *"expect"* && "$out" == *"install"* ]]
'

it "shows error when ssh-agent cannot be started" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"

  cat > "$tmp/.gash_env" <<EOF
SSH:~/.ssh/key1=pw
EOF

  # Create a mock ssh-agent that fails
  tmpbin="$(mktemp -d)"; trap "/bin/rm -rf $tmpbin" EXIT
  cat > "$tmpbin/ssh-agent" <<MOCK
#!/bin/bash
exit 1
MOCK
  chmod +x "$tmpbin/ssh-agent"

  export PATH="$tmpbin:$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SSH_AGENT=0
  unset SSH_AUTH_SOCK SSH_AGENT_PID || true

  out="$({ HOME="$tmp" GASH_ENV_FILE="$tmp/.gash_env" __GASH_ENV_LOADED="" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"
  [[ "$out" == *"ssh-agent is not running"* ]]
'

it "auto-starts ssh-agent when SSH keys are configured" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"

  cat > "$tmp/.gash_env" <<EOF
SSH:~/.ssh/key1=pw
EOF

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export HOME="$tmp"
  export GASH_ENV_FILE="$tmp/.gash_env"
  __GASH_ENV_LOADED=""

  unset SSH_AUTH_SOCK SSH_AGENT_PID GASH_SSH_AUTOUNLOCK_RAN MOCK_SSH_AGENT || true

  outf="$(mktemp)"; trap "rm -f $outf" EXIT
  gash_ssh_auto_unlock >"$outf" 2>&1
  out="$(cat "$outf")"
  [[ -n "${SSH_AUTH_SOCK-}" ]]
  [[ "$out" != *"ssh-agent is not running"* ]]
  [[ "$out" == *"SSH:"*"Identity added"* ]]
  [[ -n "${GASH_SSH_AUTOUNLOCK_RAN-}" ]]
'

it "formats expect output using gash style" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"
  : > "$tmp/.ssh/key2"

  cat > "$tmp/.gash_env" <<EOF
SSH:~/.ssh/key1=pw
SSH:~/.ssh/key2=pw2
EOF

  export PATH="$ROOT/tests/mocks/bin:$PATH"
  export MOCK_SSH_AGENT=1

  out="$({ HOME="$tmp" GASH_ENV_FILE="$tmp/.gash_env" __GASH_ENV_LOADED="" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock; } 2>&1)"

  # spawn lines should be filtered; output should be prefixed and readable
  [[ "$out" != *"spawn ssh-add"* ]]
  [[ "$out" == *"SSH:"*"Identity added"* ]]
  [[ "$out" == *"no identities"* ]]
'
