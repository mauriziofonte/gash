#!/usr/bin/env bash
#
# GASH_* settings in ~/.gash_env: parsing, precedence and their effect on
# the interactive shell.

describe "Settings (~/.gash_env)"

it "settings are parsed with quotes stripped and read back" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  cat >"$tmp/env" <<ENV
# comment
GASH_PROMPT=0
GASH_GIT_EXCLUDE="/srv/a b:/srv/c"
GASH_AI_MODEL_CLAUDE='"'"'claude-x'"'"'
ENV
  chmod 600 "$tmp/env"
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  GASH_ENV_FILE="$tmp/env"

  [[ "$(__gash_get_setting GASH_PROMPT)" == "0" ]]
  [[ "$(__gash_get_setting GASH_GIT_EXCLUDE)" == "/srv/a b:/srv/c" ]]
  [[ "$(__gash_get_setting GASH_AI_MODEL_CLAUDE)" == "claude-x" ]]
  ! __gash_get_setting GASH_BANNER >/dev/null
'

it "a variable already set in the environment wins over the file" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  printf "%s\n" "GASH_BANNER=0" "GASH_HISTORY=0" >"$tmp/env"
  chmod 600 "$tmp/env"
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  GASH_ENV_FILE="$tmp/env"

  GASH_BANNER=1
  unset GASH_HISTORY
  __gash_apply_settings
  [[ "$GASH_BANNER" == "1" ]]
  [[ "$GASH_HISTORY" == "0" ]]
'

it "unknown settings and malformed lines warn with a line number, never a value" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  printf "%s\n" "GASH_NOPE=topsecret" "AI:claude" "PASSWORD=hunter2" "GASH_PROMPT=1" >"$tmp/env"
  chmod 600 "$tmp/env"
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  GASH_ENV_FILE="$tmp/env"

  out="$(__gash_load_env 2>&1)"
  [[ "$out" == *"Line 1"*"GASH_NOPE"* ]]
  [[ "$out" == *"Line 2"* ]]
  [[ "$out" == *"Line 3"*"PASSWORD"* ]]
  [[ "$out" != *"topsecret"* && "$out" != *"hunter2"* ]]
  [[ "$(__gash_get_setting GASH_PROMPT 2>/dev/null)" == "1" ]]
'

it "GASH_PROMPT=0 keeps the user prompt but still installs the hook" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  printf "%s\n" "GASH_PROMPT=0" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    PS1=\"mine> \"
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    __gash_prompt_command
    printf \"%s|%s\" \"\$PS1\" \"\$PROMPT_COMMAND\"
  " 2>/dev/null)"
  [[ "$out" == "mine> |__gash_prompt_command" ]]
'

it "GASH_HISTORY=0 leaves the history settings alone" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  run() {
    HOME="$tmp" bash --noprofile --norc -i -c "
      HISTSIZE=77
      source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
      printf \"%s\" \"\$HISTSIZE\"
    " 2>/dev/null
  }
  [[ "$(run)" == "-1" ]]
  printf "%s\n" "GASH_HISTORY=0" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"
  [[ "$(run)" == "77" ]]
'

it "tool replacements are off by default and on with GASH_REPLACE_TOOLS=1" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  mkdir -p "$tmp/bin"
  for t in htop pydf colordiff; do printf "#!/bin/sh\n" >"$tmp/bin/$t"; chmod +x "$tmp/bin/$t"; done

  run() {
    HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "
      source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
      for a in top df diff; do alias \$a >/dev/null 2>&1 && printf \"%s \" \$a; done
      true
    " 2>/dev/null
  }
  [[ -z "$(run)" ]]
  printf "%s\n" "GASH_REPLACE_TOOLS=1" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"
  [[ "$(run)" == "top df diff " ]]
'

it "safety aliases can be turned off with GASH_SAFETY_ALIASES=0" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  run() {
    HOME="$tmp" bash --noprofile --norc -i -c "
      source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
      alias rm >/dev/null 2>&1 && echo has-rm || echo no-rm
    " 2>/dev/null
  }
  [[ "$(run)" == "has-rm" ]]
  printf "%s\n" "GASH_SAFETY_ALIASES=0" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"
  [[ "$(run)" == "no-rm" ]]
'

it "every setting documented in .gash_env.template is accepted by the parser" bash -c '
  set -euo pipefail
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  n=0
  while IFS= read -r name; do
    n=$((n + 1))
    [[ "$__GASH_SETTINGS_KNOWN" == *" $name "* ]] || { echo "not accepted: $name"; exit 1; }
  done < <(grep -oE "^# (GASH_[A-Z0-9_]+)=" "$GASH_TEST_ROOT/.gash_env.template" | sed "s/^# //; s/=\$//")
  ((n >= 9))
'

it "php and composer are only redirected to phpX.Y with GASH_REPLACE_TOOLS=1" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  mkdir -p "$tmp/bin"
  for t in php php8.3 composer; do printf "#!/bin/sh\n" >"$tmp/bin/$t"; chmod +x "$tmp/bin/$t"; done

  run() {
    HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "
      source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
      for a in php composer php83 composer83; do alias \$a >/dev/null 2>&1 && printf \"%s \" \$a; done
      true
    " 2>/dev/null
  }
  [[ "$(run)" == "php83 composer83 " ]]
  printf "%s\n" "GASH_REPLACE_TOOLS=1" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"
  [[ "$(run)" == "php composer php83 composer83 " ]]
'
