#!/usr/bin/env bash
#
# llm_exec: read-only allowlist, child shell, settings from ~/.gash_env only,
# and the secret-file rules shared with llm_grep / llm_find / llm_config.

describe "LLM exec and secret files"

it "llm_exec runs plain read-only command lines in a child shell" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  printf "b\na\n" >list.txt

  [[ "$(llm_exec "echo hello")" == hello ]]
  [[ "$(llm_exec "sort list.txt | head -n 1")" == a ]]
  [[ "$(llm_exec "ls nothing-here 2>/dev/null || echo missing")" == missing ]]
  [[ "$(llm_exec "echo '"'"'a b'"'"' \"c d\" e\\;f")" == "a b c d e;f" ]]
  [[ "$(llm_exec $'"'"'echo one\necho two # trailing > comment'"'"')" == $'"'"'one\ntwo'"'"' ]]
  [[ "$(llm_exec "echo err >&2" 2>&1)" == err ]]

  # The caller shell is untouched: no cd, no variables, exit status passes through.
  llm_exec "cd /"
  [[ "$PWD" == "$tmp" ]]
  set +e; llm_exec "grep -q zzz list.txt"; rc=$?; set -e
  [[ $rc -eq 1 ]]
'

it "llm_exec refuses expansions, redirections, subshells and unknown commands" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }

  [[ "$(verdict "echo \$HOME")" == construct_not_allowed ]]
  [[ "$(verdict "echo \"\$(id)\"")" == construct_not_allowed ]]
  [[ "$(verdict "echo \`id\`")" == construct_not_allowed ]]
  [[ "$(verdict "echo x > out")" == construct_not_allowed ]]
  [[ "$(verdict "cat < /etc/hostname")" == construct_not_allowed ]]
  [[ "$(verdict "sleep 5 &")" == construct_not_allowed ]]
  [[ "$(verdict "(ls)")" == construct_not_allowed ]]
  [[ "$(verdict "{rm,-rf,x}")" == construct_not_allowed ]]
  [[ "$(verdict "FOO=1 ls")" == construct_not_allowed ]]
  [[ "$(verdict "rm -f x")" == command_not_allowed ]]
  [[ "$(verdict "/bin/rm x")" == command_not_allowed ]]
  [[ "$(verdict "ls | xargs rm")" == command_not_allowed ]]
  [[ "$(verdict "echo '"'"'open")" == unterminated_quote ]]
  [[ ! -e out ]]
'

it "llm_exec refuses writing options of allowed commands" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  touch keep.txt
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }

  for line in "find . -delete" "find . '"'"'-exec'"'"' rm {} ;" "find . -dele\\te" \
      "sort -o out keep.txt" "sort -ro out keep.txt" "sed -i s/a/b/ keep.txt" \
      "uniq keep.txt out" "date -s now" "git branch -D main" "git branch newname" \
      "git -c core.pager=sh log" "git config user.name x" "git push" "git stash" \
      "systemctl stop nginx" "docker rm x" "php -r 1" "crontab -r"; do
    [[ "$(verdict "$line")" == option_not_allowed ]] || { echo "not refused: $line"; exit 1; }
  done
  [[ -e keep.txt && ! -e out ]]
'

it "llm_exec checks what unquoted globs expand to" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  touch -- -delete a.log
  [[ "$(llm_exec "find * -name x" 2>&1 >/dev/null | jq -r .error)" == glob_matches_option ]]
  [[ -e -delete && -e a.log ]]
  llm_exec "ls ./*" >/dev/null
  [[ "$(llm_exec "echo '"'"'*'"'"'")" == "*" ]]
  printf "S=1\n" >prod.ENV
  [[ "$(llm_exec "cat *.ENV" 2>&1 >/dev/null | jq -r .error)" == secret_file_blocked ]]
'

it "llm_exec runs sed and awk sandboxed" bash -c '
  set -euo pipefail
  printf "x\n" | sed --sandbox -n p >/dev/null 2>&1 || gash_skip "sed has no --sandbox"
  gash_require gawk
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  printf "a b\n" >in.txt
  [[ "$(llm_exec "sed -n 1p in.txt")" == "a b" ]]
  [[ "$(llm_exec "awk '"'"'{print \$2}'"'"' in.txt")" == b ]]
  ! llm_exec "sed -n '"'"'1w out'"'"' in.txt" 2>/dev/null
  ! llm_exec "awk '"'"'BEGIN{system(\"touch pwned\")}'"'"'" 2>/dev/null
  [[ ! -e out && ! -e pwned ]]
'

it "llm_exec ignores exported functions that shadow allowed commands" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  echo() { builtin echo pwned; }
  export -f echo
  [[ "$(llm_exec "echo safe")" == safe ]]
'

it "llm_exec settings come from ~/.gash_env, never from the environment" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  export GASH_LLM_EXEC=unrestricted GASH_LLM_EXEC_ALLOW=seq
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(llm_exec "seq 2" 2>&1 >/dev/null | jq -r .error)" == command_not_allowed ]]

  printf "GASH_LLM_EXEC_ALLOW=\"seq\"\n" >"$tmp/.gash_env"
  __gash_reload_env
  [[ "$(llm_exec "seq 2 | wc -l")" == 2 ]]
  [[ "$(llm_exec "seq 2 | xargs echo" 2>&1 >/dev/null | jq -r .error)" == command_not_allowed ]]

  printf "GASH_LLM_EXEC=unrestricted\n" >"$tmp/.gash_env"
  __gash_reload_env
  [[ "$(llm_exec "seq 2 | xargs echo")" == "1 2" ]]
  [[ "$(llm_exec "rm -rf /" 2>&1 >/dev/null | jq -r .error)" == dangerous_command_blocked ]]
'

it "llm_exec wraps the child in bwrap when GASH_LLM_SANDBOX=1" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin" "$tmp/empty"
  printf "GASH_LLM_SANDBOX=1\n" >"$tmp/.gash_env"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"

  # No bwrap: refused, nothing runs.
  [[ "$(PATH="$tmp/empty" llm_exec "echo hi" 2>&1 >/dev/null | jq -r .error)" == sandbox_unavailable ]]

  cat >"$tmp/bin/bwrap" <<"MOCK"
#!/usr/bin/env bash
printf "%s\n" "$@" >>"$HOME/bwrap.args"
MOCK
  chmod +x "$tmp/bin/bwrap"
  unset __LLM_EXEC_HAS_BWRAP
  PATH="$tmp/bin:$PATH" llm_exec "echo hi"
  grep -qx -- "--ro-bind" "$tmp/bwrap.args"
  grep -qx -- "--unshare-all" "$tmp/bwrap.args"
  grep -qx -- "--die-with-parent" "$tmp/bwrap.args"
'

it "__llm_is_secret_file covers keys, credentials, secret dirs and symlinks" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  for f in .env .ENV .env.local prod.env id_rsa id_ed25519 deploy_rsa server.PEM \
      .pgpass .netrc .git-credentials .npmrc kubeconfig terraform.tfstate \
      "$tmp/.ssh/config" "$tmp/.aws/config" "$tmp/.config/gcloud/creds.db" secrets.yml; do
    __llm_is_secret_file "$f" || { echo "missed: $f"; exit 1; }
  done
  for f in .env.example id_rsa.pub README.md config.json CredentialsController.php env.php; do
    ! __llm_is_secret_file "$f" || { echo "false positive: $f"; exit 1; }
  done
  printf "S=1\n" >"$tmp/.env"
  ln -s "$tmp/.env" "$tmp/settings.json"
  __llm_is_secret_file "$tmp/settings.json"
'

it "llm_grep and llm_find never read secret files" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  mkdir -p "$tmp/p/.ssh"
  printf "TOKEN=needle\n" >"$tmp/p/.ENV"
  printf "needle\n" >"$tmp/p/.ssh/notes.php"
  printf "needle\n" >"$tmp/p/app.php"
  for engine in default grep; do
    if [[ $engine == grep ]]; then __llm_has_rg() { return 1; }; fi
    out="$(llm_grep needle "$tmp/p")"
    [[ "$out" == *app.php* && "$out" != *ENV* && "$out" != *.ssh* ]]
    out="$(llm_find "*" "$tmp/p" --contains needle)"
    [[ "$out" == *app.php* && "$out" != *ENV* && "$out" != *.ssh* ]]
  done
  [[ "$(llm_grep needle "$tmp/p/.ENV" 2>&1 >/dev/null | jq -r .error)" == secret_file_blocked ]]
  ln -s "$tmp/p/.ENV" "$tmp/p/conf.json"
  [[ "$(llm_config "$tmp/p/conf.json" 2>&1 >/dev/null | jq -r .error)" == secret_file_blocked ]]
'

it "__llm_validate_path blocks whole system trees, not look-alike names" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  for p in /proc /proc/self/environ /sys/kernel /dev/sda /root /root/.bashrc /etc/shadow /etc/sudoers.d/x; do
    ! __llm_validate_path "$p" >/dev/null 2>&1 || { echo "allowed: $p"; exit 1; }
  done
  [[ "$(__llm_validate_path /rootfs/x 2>/dev/null)" == /rootfs/x ]]
  [[ "$(__llm_validate_path /devices 2>/dev/null)" == /devices ]]
'

it "llm_exec refuses forbidden system paths in arguments, globs and after cd" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  ln -s /proc/self "$tmp/me"
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }

  rel="$(realpath --relative-to="$tmp" /proc 2>/dev/null)" || rel="../../../../../../../../proc"
  for line in "head -c 50 /proc/self/environ" "cat /proc/self/cmdline" "ls /sys" "cat /root/.bashrc" \
      "head /etc/shadow" "cat $rel/self/environ" "cat me/environ" \
      "cd / && cat proc/self/environ" "cd / && ls proc" "cd /etc && cat shadow" \
      "grep -f/proc/self/environ x" "ls --directory=/sys" "git -C /root status" \
      "cat ~root/.profile" "wc -c /dev/sda"; do
    [[ "$(verdict "$line")" == forbidden_path ]] || { echo "not refused: $line"; exit 1; }
  done
  # A glob is checked by what it matches (macOS has no /proc to match).
  if [[ -d /proc/self ]]; then
    [[ "$(verdict "cat /pro?/self/environ")" == forbidden_path ]]
  fi
  # The envelope is the one the other llm_* functions use.
  err="$(llm_exec "cat /proc/self/environ" 2>&1 >/dev/null || true)"
  jq -e ".action == \"FATAL\" and .recoverable == false and .details == \"/proc/self/environ\"" <<<"$err" >/dev/null

  # /dev/null stays usable, as an argument and in redirections; patterns
  # and plain text that merely mention a path are not paths.
  [[ "$(llm_exec "cat /dev/null && echo ok")" == ok ]]
  [[ "$(llm_exec "ls nothing 2>/dev/null || echo none")" == none ]]
  [[ "$(llm_exec "echo /proc/x")" == /proc/x ]]
  printf "see /proc/cpuinfo\n" >notes.txt
  [[ "$(llm_exec "grep /proc/ notes.txt")" == "see /proc/cpuinfo" ]]
  [[ "$(llm_exec "cat ./notes.txt")" == "see /proc/cpuinfo" ]]
'

it "llm_exec reads ~/.gash_env once per call and prints each warning once" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  printf "%s\n" "garbage line" "GASH_LLM_EXEC_ALLOW=seq" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"
  export HOME="$tmp"
  cd "$tmp"
  # Headless: parsed quietly at startup, nothing on stderr.
  out="$(GASH_HEADLESS=1 bash --noprofile --norc -c "source \"$GASH_TEST_ROOT/gash.sh\"; llm_exec \"seq 2\"" 2>"$tmp/err")"
  [[ "$out" == $'"'"'1\n2'"'"' && ! -s "$tmp/err" ]]
  # Modules alone (no startup parse): the first call parses once, warns once.
  out="$(bash --noprofile --norc -c "source \"$GASH_TEST_ROOT/tests/gash-test.sh\"; gash_source_all \"$GASH_TEST_ROOT\"; llm_exec \"seq 2\"; llm_exec \"seq 1\"" 2>"$tmp/err")"
  [[ "$out" == $'"'"'1\n2\n1'"'"' ]]
  [[ "$(grep -c "Line 1" "$tmp/err")" == 1 ]] || { cat "$tmp/err"; exit 1; }
'

it "the grep fallback (no rg on PATH) skips nested secret directories" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin" "$tmp/p/.config/gcloud" "$tmp/p/src" "$tmp/p/node_modules" "$tmp/p/sp ace" "$tmp/p/a/.kube"
  for t in bash grep find xargs head realpath sed tr cat sort awk mkdir rm mktemp dirname basename; do
    p="$(type -P "$t")" && ln -s "$p" "$tmp/bin/$t"
  done
  printf "needle\n" >"$tmp/p/.config/gcloud/creds.py"
  printf "needle\n" >"$tmp/p/a/.kube/config.py"
  printf "needle\n" >"$tmp/p/node_modules/x.js"
  printf "needle\n" >"$tmp/p/.env"
  printf "needle\n" >"$tmp/p/src/app.py"
  printf "needle\n" >"$tmp/p/sp ace/b.js"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  export PATH="$tmp/bin"
  ! type -P rg >/dev/null
  out="$(llm_grep needle "$tmp/p")"
  [[ "$out" == *"src/app.py:1:needle"* && "$out" == *"sp ace/b.js:1:needle"* ]]
  [[ "$out" != *gcloud* && "$out" != *.kube* && "$out" != *node_modules* && "$out" != *.env* ]]
  [[ "$(llm_grep needle "$tmp/p" --ext py)" == "$tmp/p/src/app.py:1:needle" ]]
  out="$(llm_find "*" "$tmp/p" --contains needle)"
  [[ "$out" == *src/app.py* && "$out" != *gcloud* && "$out" != *.kube* ]]
  # Nothing found is still a success, and stdin is never read.
  [[ -z "$(llm_grep zzz "$tmp/p" </dev/null)" ]]
'
