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

it "unknown settings and malformed lines warn with a line number, never any content" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  printf "%s\n" "GASH_NOPE=topsecret" "AI:claude" "PASSWORD=hunter2" "GASH_PROMPT=1" \
    "sk-ant-api03-PASTEDTOKENWITHOUTSEPARATORS" "DB:prod=ftp://u:pw@h/x" "AI:openai=sk-zzz" \
    "HOST:box=nohost" "SSH:~/.ssh/missing_key=pass" >"$tmp/env"
  chmod 600 "$tmp/env"
  unset GASH_HEADLESS
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  GASH_ENV_FILE="$tmp/env"

  out="$(__gash_load_env 2>&1)"
  for n in 1 2 3 5 6 7 8 9; do
    [[ "$out" == *"Line $n of $tmp/env:"* ]] || { echo "no warning for line $n: $out"; exit 1; }
  done
  for leak in GASH_NOPE topsecret PASSWORD hunter2 sk-ant PASTED prod ftp pw@ openai sk-zzz box nohost missing_key pass; do
    [[ "$out" != *"$leak"* ]] || { echo "leaked $leak: $out"; exit 1; }
  done
  [[ "$out" != *"~/.gash_env"* ]]
  [[ "$(__gash_get_setting GASH_PROMPT 2>/dev/null)" == "1" ]]

  # Under $HOME the file is shown as ~/...
  HOME="$tmp" __gash_reload_env 2>"$tmp/err"
  grep -q "Line 1 of ~/env:" "$tmp/err"
'

it "an inline # comment after a value is not part of the value" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  GASH_ENV_FILE="$tmp/env"

  # Every commented setting of the template, uncommented, parses to exactly
  # the value written before its comment.
  n=0
  while IFS= read -r line; do
    [[ "$line" =~ ^#\ (GASH_[A-Z0-9_]+)=([^[:space:]]*)[[:space:]]+# ]] || continue
    name="${BASH_REMATCH[1]}" want="${BASH_REMATCH[2]}"
    printf "%s\n" "${line#\# }" >"$tmp/env"
    __gash_reload_env
    got="$(__gash_get_setting "$name")"
    [[ "$got" == "$want" ]] || { echo "$name: got [$got], want [$want]"; exit 1; }
    n=$((n + 1))
  done <"$GASH_TEST_ROOT/.gash_env.template"
  ((n >= 10))

  # Quoted values keep a # inside the quotes and drop a comment after them.
  printf "%s\n" "GASH_LLM_EXEC_ALLOW=\"make # npm\"   # comment" "GASH_GIT_EXCLUDE='"'"'/a b'"'"'" \
    "GASH_AI_MODEL_CLAUDE=x#y" "GASH_DB_TIMEOUT=5	# tab" >"$tmp/env"
  __gash_reload_env
  [[ "$(__gash_get_setting GASH_LLM_EXEC_ALLOW)" == "make # npm" ]]
  [[ "$(__gash_get_setting GASH_GIT_EXCLUDE)" == "/a b" ]]
  [[ "$(__gash_get_setting GASH_AI_MODEL_CLAUDE)" == "x#y" ]]
  [[ "$(__gash_get_setting GASH_DB_TIMEOUT)" == "5" ]]
'

it "the permission check warns only when group or other bits are set" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  unset GASH_HEADLESS
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  source "$GASH_TEST_ROOT/lib/core/config.sh"
  printf "GASH_PROMPT=1\n" >"$tmp/env"
  for mode in 600 400 700 500; do
    chmod "$mode" "$tmp/env"
    [[ -z "$(__gash_check_env_perms "$tmp/env" 2>&1)" ]] || { echo "warned on $mode"; exit 1; }
  done
  for mode in 640 604 660 644 710; do
    chmod "$mode" "$tmp/env"
    [[ "$(__gash_check_env_perms "$tmp/env" 2>&1)" == *"mode $mode"* ]] || { echo "silent on $mode"; exit 1; }
  done
  chmod 600 "$tmp/env"
'

it "headless mode applies the function settings of ~/.gash_env, quietly, environment first" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir "$tmp/bin"
  cat >"$tmp/bin/psql" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$PGOPTIONS" >"$HOME/pgoptions"
cat >/dev/null
printf "[]\n"
MOCK
  chmod +x "$tmp/bin/psql"
  # Mode 644 and an unknown setting would both warn in an interactive shell.
  printf "%s\n" "GASH_DB_TIMEOUT=5   # seconds" "GASH_AI_CONFIRM=0" "GASH_AI_MODEL_CLAUDE=claude-x" \
    "GASH_AI_MODEL_GEMINI=gemini-x" "GASH_PROMPT=0" "GASH_UMASK=077" "GASH_HISTORY=0" "GASH_NOPE=1" \
    "DB:default=pgsql://u:p@127.0.0.1:5432/app" >"$tmp/.gash_env"
  chmod 644 "$tmp/.gash_env"
  probe="source \"$GASH_TEST_ROOT/gash.sh\"; printf \"%s|\" \"\$GASH_DB_TIMEOUT\" \"\$GASH_AI_CONFIRM\" \"\$GASH_AI_MODEL_CLAUDE\" \"\$GASH_AI_MODEL_GEMINI\" \"\${GASH_PROMPT-unset}\" \"\${GASH_UMASK-unset}\" \"\${GASH_HISTORY-unset}\" \"\$(umask)\""

  out="$(cd "$tmp" && HOME="$tmp" GASH_HEADLESS=1 bash --noprofile --norc -c "umask 022; $probe" 2>&1)"
  [[ "$out" == "5|0|claude-x|gemini-x|unset|unset|unset|0022|" ]] || { echo "got: $out"; exit 1; }

  out="$(cd "$tmp" && HOME="$tmp" GASH_HEADLESS=1 GASH_DB_TIMEOUT=9 GASH_AI_CONFIRM=1 bash --noprofile --norc -c "umask 022; $probe" 2>&1)"
  [[ "$out" == "9|1|claude-x|gemini-x|"* ]] || { echo "got: $out"; exit 1; }

  # psql receives the statement timeout from the file.
  (cd "$tmp" && HOME="$tmp" PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 bash --noprofile --norc -c \
    "source \"$GASH_TEST_ROOT/gash.sh\"; llm_db_query \"SELECT 1\" >/dev/null")
  grep -q "statement_timeout=5s" "$tmp/pgoptions"
'

it "gash_env_init honours GASH_ENV_FILE" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  GASH_ENV_FILE="$tmp/conf.env" gash_env_init >/dev/null 2>&1
  [[ -f "$tmp/conf.env" && ! -e "$tmp/.gash_env" ]]
  [[ "$(stat -c %a "$tmp/conf.env" 2>/dev/null || stat -f %Lp "$tmp/conf.env")" == 600 ]]
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
