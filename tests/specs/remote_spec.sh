#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"
gash_source_all "$ROOT_DIR"

describe "Remote host operations (rex/rpush/rpull/rspush/rspull)"

it "classifies DENY paths correctly" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  [[ "$(__remote_classify_path /etc/shadow)" == deny:* ]]
  [[ "$(__remote_classify_path /etc/passwd)" == deny:* ]]
  [[ "$(__remote_classify_path /etc/ssh/sshd_config)" == deny:* ]]
  [[ "$(__remote_classify_path /boot/grub)" == deny:* ]]
  [[ "$(__remote_classify_path /usr/bin/bash)" == deny:* ]]
  [[ "$(__remote_classify_path /)" == deny:* ]]
  [[ "$(__remote_classify_path "/tmp/../etc/shadow")" == "deny:path_traversal" ]]
'

it "demotes /etc/systemd, /etc/crontab, /etc/init.d to ASK (not DENY)" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  [[ "$(__remote_classify_path /etc/systemd/system/myapp.service)" == ask:* ]]
  [[ "$(__remote_classify_path /etc/crontab)" == ask:* ]]
  [[ "$(__remote_classify_path /etc/init.d/apache2)" == ask:* ]]
  [[ "$(__remote_classify_path /root/.bashrc)" == ask:* ]]
'

it "classifies ALLOW paths correctly" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  [[ "$(__remote_classify_path /tmp/foo)" == "allow" ]]
  [[ "$(__remote_classify_path /var/tmp/foo)" == "allow" ]]
  [[ "$(__remote_classify_path /home/maurizio/project)" == "allow" ]]
  [[ "$(__remote_classify_path relative/path)" == "allow" ]]
  [[ "$(__remote_classify_path /opt/myapp)" == "allow" ]]
'

it "resolve_target accepts a raw user@host interactively" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  target=""
  __remote_resolve_target "someone@example.com" target
  [[ "$target" == "someone@example.com" ]]
'

it "resolve_target refuses a raw user@host under GASH_HEADLESS=1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export GASH_HEADLESS=1
  target=""
  out="$(__remote_resolve_target "someone@example.com" target 2>&1 || true)"
  [[ "$out" == *"Unknown host alias"* ]]
  [[ -z "$target" ]]
'

it "resolve_target accepts a registered alias under GASH_HEADLESS=1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp)"; trap "rm -f $tmp" EXIT
  echo "HOST:hnas=root@hnas.example.com" > "$tmp"
  export GASH_ENV_FILE="$tmp"
  export GASH_HEADLESS=1

  target=""
  __remote_resolve_target "hnas" target
  [[ "$target" == "root@hnas.example.com" ]]
'

it "remote_push refuses an unregistered alias under GASH_HEADLESS=1" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  export GASH_ENV_FILE="/nonexistent-$RANDOM-$RANDOM"
  export GASH_HEADLESS=1

  out="$(remote_push nope ./f /tmp/f 2>&1 || true)"
  [[ "$out" == *"Unknown host alias"* ]]
'

it "remote_push refuses a DENY path even with --force" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  out="$(remote_push "someone@example.com" ./f /etc/shadow --force 2>&1 || true)"
  [[ "$out" == *"DENY list"* ]]
  [[ "$out" == *"never allowed"* ]]
'

it "remote_sync_push refuses an ASK path under GASH_HEADLESS=1 without --force" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  tmp="$(mktemp)"; trap "rm -f $tmp" EXIT
  echo "HOST:hnas=root@hnas.example.com" > "$tmp"
  export GASH_ENV_FILE="$tmp"
  export GASH_HEADLESS=1

  out="$(remote_sync_push hnas ./dir /etc/systemd/system/myapp 2>&1 || true)"
  [[ "$out" == *"refused in headless mode without --force"* ]]
'
