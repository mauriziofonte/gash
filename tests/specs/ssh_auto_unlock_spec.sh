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

it "skips keys the agent already holds" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/.ssh"
  : > "$tmp/.ssh/key1"
  : > "$tmp/.ssh/key2"
  printf "SSH:~/.ssh/key1=pw\nSSH:~/.ssh/key2=pw2\n" > "$tmp/.gash_env"

  export PATH="$ROOT/tests/mocks/bin:$PATH" MOCK_SSH_AGENT=1 MOCK_SSH_LOADED="SHA256:fp-key1"
  export MOCK_EXPECT_STDIN="$tmp/sent"
  HOME="$tmp" GASH_ENV_FILE="$tmp/.gash_env" __GASH_ENV_LOADED="" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock >/dev/null 2>&1
  sent="$(base64 -d <"$tmp/sent" 2>/dev/null || base64 -D <"$tmp/sent")"
  [[ "$sent" == *key2* && "$sent" != *key1* ]]

  # Everything loaded: expect is not run at all.
  : > "$tmp/sent"
  MOCK_SSH_LOADED="SHA256:fp-key1 SHA256:fp-key2" HOME="$tmp" GASH_ENV_FILE="$tmp/.gash_env" __GASH_ENV_LOADED="" GASH_SSH_AUTOUNLOCK_RAN= gash_ssh_auto_unlock >/dev/null 2>&1
  [[ ! -s "$tmp/sent" ]]
'

it "reuses the agent on the fixed socket instead of starting another" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  command -v python3 >/dev/null 2>&1 || gash_skip "python3 needed to create a socket"
  # Short path: macOS limits a socket path to 104 bytes.
  tmp="$(mktemp -d /tmp/gs.XXXXXX)"; trap "rm -rf $tmp" EXIT
  export XDG_RUNTIME_DIR="$tmp/run"; mkdir -p "$XDG_RUNTIME_DIR"
  python3 -c "import socket,sys; s=socket.socket(socket.AF_UNIX); s.bind(sys.argv[1])" "$XDG_RUNTIME_DIR/gash-ssh-agent.sock"
  mkdir "$tmp/bin"
  printf "#!/bin/sh\necho started >> $tmp/log\n" > "$tmp/bin/ssh-agent"
  printf "#!/bin/sh\n[ -n \"\$ALIVE\" ] && exit 1\nexit 2\n" > "$tmp/bin/ssh-add"
  chmod +x "$tmp/bin"/*
  PATH="$tmp/bin:$PATH"
  unset SSH_AUTH_SOCK
  ALIVE=1 __gash_try_start_ssh_agent
  [[ "$SSH_AUTH_SOCK" == "$XDG_RUNTIME_DIR/gash-ssh-agent.sock" && ! -e "$tmp/log" ]]
  # A dead socket is removed and a new agent is started on the same path.
  unset SSH_AUTH_SOCK
  __gash_try_start_ssh_agent || true
  grep -q started "$tmp/log"
  [[ ! -S "$XDG_RUNTIME_DIR/gash-ssh-agent.sock" ]]
'
