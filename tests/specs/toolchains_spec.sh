#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=tests/gash-test.sh
source "$ROOT_DIR/tests/gash-test.sh"

describe "path_add"

it "prepends existing dirs in the order given, once, and skips missing ones" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b"
  PATH="/usr/bin:/bin"
  path_add "$tmp/a" "$tmp/missing" "$tmp/b/"
  [[ "$PATH" == "$tmp/a:$tmp/b:/usr/bin:/bin" ]]
  path_add "$tmp/b" "$tmp/a"
  [[ "$PATH" == "$tmp/a:$tmp/b:/usr/bin:/bin" ]]
'

it "--append adds at the end and ~ is expanded" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  mkdir -p "$HOME/.cargo/bin"
  PATH="/usr/bin:/bin"
  path_add --append "~/.cargo/bin"
  [[ "$PATH" == "/usr/bin:/bin:$HOME/.cargo/bin" ]]
'

describe "GASH_PATH_ADD and GASH_NVM (lib/shell/toolchains.sh)"

it "GASH_PATH_ADD puts the listed toolchains first" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  mkdir -p "$HOME/.local/bin" "$HOME/.cargo/bin"
  PATH="/usr/bin:/bin"
  GASH_PATH_ADD="~/.local/bin ~/.cargo/bin ~/.config/composer/vendor/bin"
  source "$ROOT/lib/shell/toolchains.sh"
  [[ "$PATH" == "$HOME/.local/bin:$HOME/.cargo/bin:/usr/bin:/bin" ]]
'

it "the default version of nvm resolves through aliases" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  source "$ROOT/lib/shell/toolchains.sh"
  export NVM_DIR="$HOME/.nvm"
  mkdir -p "$NVM_DIR/versions/node"/{v20.11.0,v22.1.0,v22.20.0,v220.0.0} "$NVM_DIR/alias/lts"
  echo 22 > "$NVM_DIR/alias/default"
  [[ "$(__gash_nvm_default_dir)" == "$NVM_DIR/versions/node/v22.20.0" ]]
  echo "lts/*" > "$NVM_DIR/alias/default"
  echo "lts/iron" > "$NVM_DIR/alias/lts/*"
  echo "v20.11.0" > "$NVM_DIR/alias/lts/iron"
  [[ "$(__gash_nvm_default_dir)" == "$NVM_DIR/versions/node/v20.11.0" ]]
  echo "v22.1.0" > "$NVM_DIR/alias/default"
  [[ "$(__gash_nvm_default_dir)" == "$NVM_DIR/versions/node/v22.1.0" ]]
  echo "node" > "$NVM_DIR/alias/default"
  [[ "$(__gash_nvm_default_dir)" == "$NVM_DIR/versions/node/v220.0.0" ]]
  echo "system" > "$NVM_DIR/alias/default"
  [[ -z "$(__gash_nvm_default_dir)" ]]
'

it "GASH_NVM=lazy puts node on PATH and loads nvm.sh on the first nvm call" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  export NVM_DIR="$HOME/.nvm"
  mkdir -p "$NVM_DIR/versions/node/v22.20.0/bin"
  echo 22 > "$NVM_DIR/alias/default" 2>/dev/null || { mkdir -p "$NVM_DIR/alias"; echo 22 > "$NVM_DIR/alias/default"; }
  # A stand-in nvm.sh: records how it was sourced and defines nvm.
  cat > "$NVM_DIR/nvm.sh" <<EOF
echo "sourced \$*" >> "$HOME/nvm.log"
nvm() { echo "real nvm \$*"; }
EOF
  PATH="/usr/bin:/bin"
  GASH_NVM=lazy
  source "$ROOT/lib/shell/toolchains.sh"
  [[ "$PATH" == "$NVM_DIR/versions/node/v22.20.0/bin:/usr/bin:/bin" ]]
  [[ ! -e "$HOME/nvm.log" ]]
  [[ "$(nvm ls)" == "real nvm ls" ]]
  [[ "$(cat "$HOME/nvm.log")" == "sourced --no-use" ]]
'

it "GASH_NVM=on sources nvm.sh at once; off (default) does nothing" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  export NVM_DIR="$HOME/.nvm"
  mkdir -p "$NVM_DIR"
  echo "nvm() { :; }; echo loaded >> \"$HOME/nvm.log\"" > "$NVM_DIR/nvm.sh"
  source "$ROOT/lib/shell/toolchains.sh"
  ! declare -F nvm >/dev/null
  GASH_NVM=on
  source "$ROOT/lib/shell/toolchains.sh"
  declare -F nvm >/dev/null
  [[ "$(cat "$HOME/nvm.log")" == "loaded" ]]
'

it "GASH_NVM and GASH_PATH_ADD are known ~/.gash_env settings" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  printf "GASH_NVM=lazy\nGASH_PATH_ADD=\"~/.local/bin ~/bin\"\n" > "$HOME/.gash_env"
  chmod 600 "$HOME/.gash_env"
  out="$(__gash_load_env 2>&1)"
  [[ -z "$out" ]]
  __gash_load_env
  [[ "$(__gash_get_setting GASH_NVM)" == "lazy" ]]
  [[ "$(__gash_get_setting GASH_PATH_ADD)" == "~/.local/bin ~/bin" ]]
'

describe "Plan helpers (apt-style)"

it "__gash_human_size uses IEC units" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  [[ "$(__gash_human_size 0)" == "0 B" ]]
  [[ "$(__gash_human_size 1536)" == "1.5 KiB" ]]
  [[ "$(__gash_human_size 149159936)" == "142.2 MiB" ]]
  [[ "$(__gash_human_size abc)" == "0 B" ]]
'

it "__gash_confirm_plan: -y passes, no terminal refuses without waiting" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  __gash_confirm_plan 1
  out="$(GASH_HEADLESS=1 __gash_confirm_plan 0 2>&1)" && exit 1
  [[ "$out" == *"pass -y"* ]]
'

it "__gash_task reports done or failed and keeps the status" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  out="$(__gash_task "Working" echo data 2>"$HOME/err")"
  [[ "$out" == "data" && "$(cat "$HOME/err")" == "Working... done" ]]
  rc=0; __gash_task "Breaking" false 2>"$HOME/err" || rc=$?
  [[ $rc -eq 1 && "$(cat "$HOME/err")" == "Breaking... failed" ]]
'

it "__gash_download saves the file and removes it when curl fails" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"
  HOME="$(mktemp -d)"; trap "rm -rf $HOME" EXIT
  mkdir -p "$HOME/bin"
  ln -s "$ROOT/tests/mocks/bin/curl" "$HOME/bin/curl"
  # Answers of tests/mocks/bin/curl (and of the HTTPS fake in Go mode).
  printf "GET https://example.test/f 200 hello\n" > "$HOME/routes"
  printf "HEAD * 200\n  Content-Length: 5\n* * 404\n" > "$HOME/routes-fail"
  export MOCK_CURL_ROUTES="$HOME/routes"
  PATH="$HOME/bin:$PATH"
  err="$(__gash_download https://example.test/f "$HOME/f" 2>&1 >/dev/null)"
  [[ "$(cat "$HOME/f")" == "hello" && "$err" == *"[5 B]"* && "$err" == *"Fetched 5 B"* ]]
  rc=0; err="$(MOCK_CURL_ROUTES="$HOME/routes-fail" __gash_download https://example.test/g "$HOME/g" 2>&1)" || rc=$?
  [[ $rc -eq 22 && ! -e "$HOME/g" && "$err" == *"Download failed"* ]]
'
