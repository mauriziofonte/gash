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

it "normalizes paths before classifying: no slash or dot tricks, ~/.ssh asks" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  for p in //etc/shadow /etc//shadow /./etc/shadow /etc/./ssh/ /etc/shadow/ ///usr//bin/sh //; do
    [[ "$(__remote_classify_path "$p")" == deny:* ]] || { echo "not denied: $p"; exit 1; }
  done
  for p in "~/.ssh/authorized_keys" .ssh/id_ed25519 "~root/.ssh/x" /home/bob/.ssh/ ./.ssh; do
    [[ "$(__remote_classify_path "$p")" == "ask:.ssh" ]] || { echo "not asked: $p"; exit 1; }
    ! __remote_is_safe_for_delete "$p" || { echo "delete-safe: $p"; exit 1; }
  done
  [[ "$(__remote_classify_path "~/notes.txt")" == allow ]]
  [[ "$(__remote_classify_path ..config)" == allow ]]
  [[ "$(__remote_classify_path "a/../../etc")" == "deny:path_traversal" ]]
  # rsync --delete into the remote home itself is not "safe".
  ! __remote_is_safe_for_delete "~"
  ! __remote_is_safe_for_delete "~/"
  __remote_is_safe_for_delete "~/project"
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

it "a .ssh component anywhere asks; /boot/.ssh stays denied" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  for p in /srv/git/.ssh/authorized_keys /Users/x/.ssh /Users/x/.SSH/authorized_keys \
    /opt/app/.ssh /tmp/x/.ssh /var/tmp/.ssh/id_rsa /root/.ssh "~/a/.ssh/b" "~bob/.ssh" a/.ssh; do
    [[ "$(__remote_classify_path "$p")" == "ask:.ssh" ]] || { echo "not asked: $p"; exit 1; }
    ! __remote_is_safe_for_delete "$p" || { echo "delete-safe: $p"; exit 1; }
  done
  [[ "$(__remote_classify_path /boot/.ssh)" == "deny:/boot" ]]
  [[ "$(__remote_classify_path /home/u/.sshx)" == allow ]]
  [[ "$(__remote_classify_path /srv/git/ssh)" == allow ]]

  # Headless: refused without --force, whatever home it is in.
  tmp="$(mktemp)"; trap "rm -f $tmp" EXIT
  echo "HOST:git=git@git.example.com" > "$tmp"
  export GASH_ENV_FILE="$tmp" GASH_HEADLESS=1
  out="$(remote_push git ./f /srv/git/.ssh/authorized_keys 2>&1 || true)"
  [[ "$out" == *"(matched: .ssh)"*"refused in headless mode without --force"* ]]
  __remote_check_safety /srv/git/.ssh/authorized_keys push 1
'

it "rsync --delete never treats the remote home itself as safe, however it is written" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  for p in "~" "~/" "~bob" "~bob/" /home/u /home/u/ /home/u/. //home//u// /Users/u /Users/u/ /root /root/; do
    ! __remote_is_safe_for_delete "$p" || { echo "delete-safe: $p"; exit 1; }
  done
  for p in "~/project" project /home/u/project /home/u/project/ /tmp /tmp/x /var/tmp/x; do
    __remote_is_safe_for_delete "$p" || { echo "not delete-safe: $p"; exit 1; }
  done

  export GASH_HEADLESS=1
  out="$(__remote_check_delete_safety /home/u 0 2>&1 || true)"
  [[ "$out" == *"Refused in headless mode without --force"* ]]
  __remote_check_delete_safety /home/u 1
'

it "another user's home is spelled out before the safety tables" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(__remote_normalize_path "~root/x")" == /root/x ]]
  [[ "$(__remote_normalize_path "~bob/.bashrc")" == /home/bob/.bashrc ]]
  [[ "$(__remote_normalize_path "~/a//b/")" == a/b ]]
  [[ "$(__remote_classify_path "~root/x")" == ask:* ]]
'
