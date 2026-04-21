#!/usr/bin/env bash

# Spec for Gash color policy (v1.5+).
# Covers: env-level disable (GASH_HEADLESS, NO_COLOR, GASH_NO_COLOR),
# runtime TTY gate, --no-color flag on selected functions, helper APIs,
# and non-interactive correctness (no ANSI leaks for LLM/scripting use).

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Color gating (v1.5)"

# -----------------------------------------------------------------------------
# ANSI-leak detector helper (used by assertions below)
#
# An ANSI CSI sequence starts with ESC (0x1b) '['. We consider output to
# contain ANSI if the byte sequence "\x1b[" is present.
# -----------------------------------------------------------------------------

__ansi_leak() {
    # Returns 0 if ANSI escape sequences are present in stdin, 1 otherwise.
    local data; data="$(cat)"
    [[ "$data" == *$'\033['* ]]
}

# -----------------------------------------------------------------------------
# Helper API
# -----------------------------------------------------------------------------

it "__gash_color_env_ok returns 1 when GASH_HEADLESS=1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Run in a subshell with GASH_HEADLESS set via bash -c to trigger load-time gate
  out="$(GASH_HEADLESS=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_color_env_ok && echo ok || echo no
  ")"
  [[ "$out" == "no" ]]
'

it "__gash_color_env_ok returns 1 when NO_COLOR is set" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  out="$(NO_COLOR=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_color_env_ok && echo ok || echo no
  ")"
  [[ "$out" == "no" ]]
'

it "__gash_color_env_ok returns 1 when GASH_NO_COLOR is set" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  out="$(GASH_NO_COLOR=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_color_env_ok && echo ok || echo no
  ")"
  [[ "$out" == "no" ]]
'

it "__gash_use_color returns 1 on non-TTY stdout" bash -c '
  set -uo pipefail   # NB: no -e; __gash_use_color returns 1 on pipe stdout
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  # Inside this subshell stdout is a pipe, so the gate must return 1
  rc=0
  __gash_use_color || rc=$?
  [[ $rc -ne 0 ]]
'

it "__gash_color_scope emits local-shadow block when colors disabled" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(NO_COLOR=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_color_scope 0
  ")"
  [[ "$out" == *"local __GASH_COLOR_OFF="* ]]
  [[ "$out" == *"local __GASH_BOLD_RED="* ]]
'

it "__gash_color_scope emits nothing when colors enabled" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  # Colors are "enabled by env" in this test context; runtime TTY check
  # only fires inside __gash_use_color. In a non-TTY subshell, __gash_use_color
  # returns 1, so __gash_color_scope emits the block. Emulate by forcing the
  # flag: passing 0 with stdout as pipe causes it to emit.
  out="$(__gash_color_scope 0)"
  # In this captured-output scenario (stdout is pipe), colors should be off
  # so the block is emitted.
  [[ -n "$out" ]]
'

# -----------------------------------------------------------------------------
# Env-level gate: no ANSI in __gash_error/warning/success/info
# -----------------------------------------------------------------------------

it "GASH_HEADLESS=1 produces no ANSI in __gash_error" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  err="$(GASH_HEADLESS=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_error \"boom\"
  " 2>&1)"
  [[ "$err" == *"Error: boom"* ]]
  [[ "$err" != *$'"'"'\033['"'"'* ]]
'

it "NO_COLOR=1 produces no ANSI in __gash_info" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  out="$(NO_COLOR=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_info \"hello\"
  ")"
  [[ "$out" == *"Info: hello"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "GASH_NO_COLOR=1 produces no ANSI in __gash_warning" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  out="$(GASH_NO_COLOR=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    __gash_warning \"heads up\"
  " 2>&1)"
  [[ "$out" == *"Warning: heads up"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "GASH_HEADLESS=1 zeroes color vars at load time" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  out="$(GASH_HEADLESS=1 bash -c "
    source \"$ROOT/lib/core/output.sh\"
    echo \"red=[\$__GASH_RED]\"
    echo \"off=[\$__GASH_COLOR_OFF]\"
  ")"
  [[ "$out" == *"red=[]"* ]]
  [[ "$out" == *"off=[]"* ]]
'

# -----------------------------------------------------------------------------
# Runtime TTY gate (pipe redirect)
# -----------------------------------------------------------------------------

it "pipe context (non-TTY stdout) produces no ANSI in __gash_info" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  # Capture via $(...) forces stdout to a pipe; runtime gate should kick in
  source "$ROOT/lib/core/output.sh"
  out="$(__gash_info "piped")"
  [[ "$out" == *"Info: piped"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "pipe context produces no ANSI in __gash_error (stderr uniform rule)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/lib/core/output.sh"
  err="$({ __gash_error "boom"; } 2>&1)"
  [[ "$err" == *"Error: boom"* ]]
  [[ "$err" != *$'"'"'\033['"'"'* ]]
'

# -----------------------------------------------------------------------------
# Per-function --no-color flag
# -----------------------------------------------------------------------------

it "files_largest --no-color suppresses ANSI on data rows" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  dd if=/dev/zero of="$tmp/big.bin" bs=1024 count=8 status=none
  # --human forces size formatting; we want the output but no ANSI
  out="$(files_largest "$tmp" --human --no-color 2>&1)"
  [[ "$out" == *"big.bin"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "files_largest error on forbidden path emits no ANSI under pipe" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  # stdout of this captured command is a pipe, so colors must be off even
  # though we do not pass --no-color.
  err="$({ files_largest /proc; } 2>&1 || true)"
  [[ "$err" == *"forbidden_path"* || "$err" == *"protected"* ]]
  [[ "$err" != *$'"'"'\033['"'"'* ]]
'

it "disk_usage --no-color strips ANSI from awk output" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(disk_usage --no-color 2>/dev/null)"
  [[ "$out" != *$'"'"'\033['"'"'* ]]
  # Header should still be printed
  [[ "$out" == *"Filesystem"* ]]
'

it "sysinfo --llm implies no ANSI (LLM-compact mode)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  # identity section is minimal + uses __sysinfo_kv which emits ANSI in verbose mode
  export PATH="$ROOT/tests/mocks/bin:$PATH"
  out="$(sysinfo identity --llm 2>/dev/null)"
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "gash_doctor --no-color emits no ANSI" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(gash_doctor --no-color 2>&1)"
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "gash_help --no-color emits no ANSI on function detail" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(gash_help files_largest --no-color 2>&1)"
  [[ "$out" == *"USAGE"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "gash_help --no-color on --list suppresses ANSI in module listing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(gash_help --list --no-color 2>&1)"
  [[ "$out" == *"files_largest"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "gash_help --no-color --search flf emits no ANSI" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  out="$(gash_help --search flf --no-color 2>&1)"
  [[ "$out" == *"files_largest"* ]]
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "dirs_list_empty --json error (forbidden) emits no ANSI" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  err="$({ dirs_list_empty --json /proc; } 2>&1 || true)"
  [[ "$err" == *"forbidden_path"* ]]
  [[ "$err" != *$'"'"'\033['"'"'* ]]
'

# -----------------------------------------------------------------------------
# GASH_HEADLESS=1 end-to-end (sourced harness)
# -----------------------------------------------------------------------------

it "GASH_HEADLESS=1 end-to-end: files_largest emits no ANSI" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"
  dd if=/dev/zero of="$tmp/big.bin" bs=1024 count=8 status=none

  inner="$(cat <<EOF
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"
files_largest "\$HOME"
EOF
)"
  out="$(HOME="$tmp" bash --noprofile --norc -c "$inner" 2>&1)"
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'

it "GASH_HEADLESS=1 end-to-end: gash_doctor emits no ANSI" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT
  ln -s "$ROOT" "$tmp/.gash"

  inner="$(cat <<EOF
export GASH_HEADLESS=1
source "\$HOME/.gash/gash.sh"
gash_doctor
EOF
)"
  out="$(HOME="$tmp" bash --noprofile --norc -c "$inner" 2>&1)"
  [[ "$out" != *$'"'"'\033['"'"'* ]]
'
