#!/usr/bin/env bash
#
# Shell hygiene: what loading Gash may and may not change in the user's
# shell. Every test runs a fresh interactive bash with HOME pointing at a
# temporary directory where ~/.gash is a symlink to this checkout.

describe "Shell hygiene"

it "interactive load leaks no unprefixed globals" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    __t_before=\"\$(compgen -v)\"
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    comm -13 <(sort <<<\"\$__t_before\") <(compgen -v | sort)
  " 2>/dev/null)"

  # Variables Gash owns, plus the shell settings it is documented to set.
  allowed="^(__GASH_|__gash_|GASH_|__AI_|__LLM_|__REMOTE_|__SYSINFO_|__t_before$)"
  allowed+="|^(LSCOLORS|PIPESTATUS|PS1|PS2|PROMPT_COMMAND|EDITOR|LS_COLORS|LESSOPEN|LESSCLOSE)$"
  allowed+="|^(HISTCONTROL|HISTSIZE|HISTFILESIZE|HISTTIMEFORMAT)$"
  leaked="$(grep -Ev "$allowed" <<<"$out" || true)"
  if [[ -n "$leaked" ]]; then
    echo "leaked globals:"; echo "$leaked"
    exit 1
  fi
'

it "umask is left alone unless GASH_UMASK is set" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "umask 077; source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; umask" 2>/dev/null)"
  [[ "$out" == "0077" ]]

  out="$(HOME="$tmp" GASH_UMASK=027 bash --noprofile --norc -i -c "umask 077; source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; umask" 2>/dev/null)"
  [[ "$out" == "0027" ]]

  # Invalid values are ignored, not applied.
  out="$(HOME="$tmp" GASH_UMASK=999 bash --noprofile --norc -i -c "umask 077; source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; umask" 2>/dev/null)"
  [[ "$out" == "0077" ]]
'

it "existing PROMPT_COMMAND string is kept and the hook is added once" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    PROMPT_COMMAND=\"user_hook\"
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    printf \"%s\" \"\$PROMPT_COMMAND\"
  " 2>/dev/null)"
  [[ "$out" == "__gash_prompt_command; user_hook" ]]
'

it "existing PROMPT_COMMAND array is kept (bash 5.1+)" bash -c '
  set -euo pipefail
  if (( BASH_VERSINFO[0] < 5 || (BASH_VERSINFO[0] == 5 && BASH_VERSINFO[1] < 1) )); then
    exit 77
  fi
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    PROMPT_COMMAND=(hook_a hook_b)
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    printf \"%s|\" \"\${PROMPT_COMMAND[@]}\"
    gash_unload >/dev/null 2>&1
    printf \"/\"
    printf \"%s|\" \"\${PROMPT_COMMAND[@]}\"
  " 2>/dev/null)"
  [[ "$out" == "__gash_prompt_command|hook_a|hook_b|/hook_a|hook_b|" ]]
'

it "prompt hook preserves the exit status for later hooks" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    (exit 3); __gash_prompt_command; echo \"rc:\$? last:\$__AI_LAST_EXIT\"
  " 2>/dev/null)"
  [[ "$out" == *"rc:3 last:3"* ]]
'

it "loading leaves no RETURN/DEBUG trap and no shell options behind" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    __t_opts=\"\$-\"
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    echo \"return:[\$(trap -p RETURN)] debug:[\$(trap -p DEBUG)]\"
    [[ \"\$-\" == \"\$__t_opts\" ]] && echo opts-same || echo \"opts:\$__t_opts->\$-\"
  " 2>/dev/null)"
  [[ "$out" == *"return:[] debug:[]"* ]]
  [[ "$out" == *"opts-same"* ]]
'

it "a set -euo pipefail caller survives sourcing in both modes" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  out="$(HOME="$tmp" GASH_HEADLESS=1 bash -c "set -euo pipefail; source \"\$HOME/.gash/gash.sh\"; echo alive" 2>&1)"
  [[ "$out" == "alive" ]]

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "set -euo pipefail; source \"\$HOME/.gash/gash.sh\" >/dev/null; echo alive" 2>/dev/null)"
  [[ "$out" == *"alive"* ]]
'

it "headless mode prints nothing and leaves the prompt untouched" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  printf "%s\n" "GASH_BOGUS=1" "nonsense line" >"$tmp/.gash_env"
  chmod 600 "$tmp/.gash_env"

  out="$(HOME="$tmp" GASH_HEADLESS=1 bash --noprofile --norc -c "
    PS1=keep; PROMPT_COMMAND=keep
    source \"\$HOME/.gash/gash.sh\"
    printf \"%s|%s|%s\" \"\$PS1\" \"\$PROMPT_COMMAND\" \"\${#__GASH_LOAD_ERRORS[@]}\"
  " 2>&1)"
  [[ "$out" == "keep|keep|0" ]]
'

it "non-Bash shells get a clear message instead of a parse error" bash -c '
  set -euo pipefail
  command -v dash >/dev/null 2>&1 || exit 77
  rc=0
  out="$(dash -c ". \"$GASH_TEST_ROOT/gash.sh\"" 2>&1)" || rc=$?
  [[ $rc -ne 0 ]]
  [[ "$out" == *"requires Bash 4.4"* ]]
'

it "every public function answers --help without side effects" bash -c '
  set -uo pipefail
  stubs="$(mktemp -d)"; trap "/bin/rm -rf \"$stubs\"" EXIT
  log="$stubs/calls.log"
  for b in docker git sudo rm curl wget ssh scp rsync systemctl apt apt-get npm composer \
           lsof fuser mv cp chmod chown tar crontab kill pkill php node mysql psql sqlite3 ln mkdir touch; do
    printf "#!/bin/sh\necho \"%s \$*\" >>\"%s\"\n" "$b" "$log" >"$stubs/$b"
    /bin/chmod +x "$stubs/$b"
  done
  before="$(declare -F | cut -d" " -f3)"
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  fails=0 n=0
  for f in $(declare -F | cut -d" " -f3 | grep -v "^_"); do
    grep -qx "$f" <<<"$before" && continue
    case "$f" in needs_help | needs_confirm_prompt) continue ;; esac
    n=$((n + 1))
    : >"$log"
    out="$( (PATH="$stubs:$PATH"; "$f" --help) </dev/null 2>&1 )"; rc=$?
    if [[ $rc -ne 0 || -s "$log" || -z "$out" ]]; then
      echo "$f --help: rc=$rc calls=$(tr "\n" ";" <"$log")" >&2
      fails=$((fails + 1))
    fi
  done
  ((n > 60)) && ((fails == 0))
'

it "commands leave no RETURN trap and keep the caller EXIT trap" bash -c '
  set -uo pipefail
  tmp="$(mktemp -d)"; mkdir -p "$tmp/d/e"; echo x >"$tmp/d/f"
  trap "/bin/rm -rf \"$tmp\"" EXIT
  before="$(trap -p EXIT)"
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  for c in files_largest dirs_largest dirs_find_large dirs_list_empty tree_stats; do
    "$c" --json "$tmp/d" >/dev/null 2>&1
    [[ -z "$(trap -p RETURN)" ]] || { echo "$c left: $(trap -p RETURN)"; exit 1; }
  done
  [[ "$(trap -p EXIT)" == "$before" ]]
'

it "cleanup traps call command rm, so the user's rm alias never applies" bash -c '
  set -euo pipefail
  # Trap strings are parsed when they run, where interactive aliases such
  # as the safety alias rm="rm -Iv" expand.
  ! grep -rn "trap " "$GASH_TEST_ROOT/lib" | grep -E "[^[:alnum:]_ ] *rm "
'
