#!/usr/bin/env bash
#
# Guided v1 -> v2 migration: detection, screens driven through the plain
# prompt, backups, rollback, and the one-line notice outside a terminal.

describe "Migration from Gash 1.x"

# A home that looks like a Gash 1.x user's, with Gash itself at ~/.gash.
__migrate_fixture='
  set -euo pipefail
  command -v diff >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  H="$tmp/home"; mkdir -p "$H" "$tmp/bin"
  ln -s "$GASH_TEST_ROOT" "$H/.gash"
  cat >"$H/.gashrc" <<"V1"
# If you come from bash you might have to change your $PATH.
# export PATH=$HOME/bin:$HOME/.local/bin:/usr/local/bin:$PATH

# Path to your Gash installation.
export GASH_BASH="$HOME/.gash/gash.sh"
export GASH_COMPLETION="$HOME/.gash/bash_completion"

if [ -f "$GASH_BASH" ]; then
    source "$GASH_BASH" # This loads Gash
    if [ "${GASH_HEADLESS-}" != "1" ] && [ -f "$GASH_COMPLETION" ]; then
        source "$GASH_COMPLETION" # This loads Gash completion
    fi
fi
export MY_OWN=1
V1
  printf "AI:claude=sk-test\nFOO BAR\n" >"$H/.gash_env"
  chmod 644 "$H/.gash_env"
  printf "alias up=\"dcup2 ~/stack -n\"\nptk 8080\n" >"$H/.bash_local"
  printf "#!/bin/sh\n" >"$tmp/bin/htop"; chmod +x "$tmp/bin/htop"
  export HOME="$H" PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 XDG_STATE_HOME="$H/.local/state"
  unset GASH_REPLACE_TOOLS GASH_UMASK GASH_LLM_EXEC GASH_AI_CONFIRM
  cp -p "$H/.gashrc" "$tmp/gashrc.orig"; cp -p "$H/.gash_env" "$tmp/env.orig"; cp -p "$H/.bash_local" "$tmp/local.orig"
  source "$GASH_TEST_ROOT/gash.sh" 2>/dev/null
  umask 0002
'

it "detects a 1.x ~/.gashrc and nothing else" bash -c "$__migrate_fixture"'
  __gash_migrate_needed
  printf "2\n" >"$XDG_STATE_HOME/gash/schema" 2>/dev/null || { mkdir -p "$XDG_STATE_HOME/gash"; printf "2\n" >"$XDG_STATE_HOME/gash/schema"; }
  ! __gash_migrate_needed
  rm "$XDG_STATE_HOME/gash/schema"
  cp "$GASH_TEST_ROOT/.gashrc" "$H/.gashrc"
  ! __gash_migrate_needed
'

it "outside a terminal Gash prints one reminder a day on stderr" bash -c "$__migrate_fixture"'
  err="$(bash -c "source \"$GASH_TEST_ROOT/gash.sh\"" 2>&1 >/dev/null)"
  [[ -z "$err" ]]   # the fixture already sourced Gash once today
  rm -f "$XDG_STATE_HOME/gash/notice"
  err="$(bash -c "source \"$GASH_TEST_ROOT/gash.sh\"" 2>&1 >/dev/null)"
  [[ "$err" == *"gash migrate"* && $(wc -l <<<"$err") -eq 1 ]]
  # Never a migration screen without a terminal.
  [[ ! -e "$XDG_STATE_HOME/gash/schema" ]]
'

it "--yes applies the recommended choices, backs up, and --rollback restores" bash -c "$__migrate_fixture"'
  out="$(gash migrate --yes 2>&1)"
  [[ "$out" == *"configured for 2.0"* ]]
  # New loader, the user line kept, still valid Bash.
  grep -q "__gash_home=" "$H/.gashrc"
  ! grep -q "GASH_BASH" "$H/.gashrc"
  [[ "$(tail -n 1 "$H/.gashrc")" == "export MY_OWN=1" ]]
  bash -n "$H/.gashrc"
  # Skipped line commented out, file made private, nothing else added.
  grep -qx "# \[skipped by Gash 2\] FOO BAR" "$H/.gash_env"
  ! grep -q "^GASH_" "$H/.gash_env"
  [[ "$(stat -c %a "$H/.gash_env" 2>/dev/null || stat -f %Lp "$H/.gash_env")" == 600 ]]
  # The -n line is fixed.
  grep -qx "alias up=\"dcup2 ~/stack --dry-run\"" "$H/.bash_local"
  ! __gash_migrate_needed
  ls "$XDG_STATE_HOME"/gash/backups/*/manifest >/dev/null

  gash migrate --rollback --yes >/dev/null
  cmp "$H/.gashrc" "$tmp/gashrc.orig"
  cmp "$H/.gash_env" "$tmp/env.orig"
  cmp "$H/.bash_local" "$tmp/local.orig"
  __gash_migrate_needed
  ! gash migrate --rollback --yes 2>/dev/null
'

it "each screen choice lands in ~/.gash_env as the matching setting" bash -c "$__migrate_fixture"'
  # welcome, gashrc, env, tools=keep, umask=022, scripts, llm=unrestricted, ai=always, write
  printf "1\n2\n2\n2\n2\n2\n2\n2\n1\n" | gash migrate --plain >"$tmp/out" 2>&1
  grep -qx "GASH_REPLACE_TOOLS=1" "$H/.gash_env"
  grep -qx "GASH_UMASK=022" "$H/.gash_env"
  grep -qx "GASH_LLM_EXEC=unrestricted" "$H/.gash_env"
  grep -qx "GASH_AI_CONFIRM=1" "$H/.gash_env"
  [[ "$(grep -c "Added by gash migrate" "$H/.gash_env")" -eq 1 ]]
  # Kept as asked: 1.x loader, skipped line, script line.
  cmp "$H/.gashrc" "$tmp/gashrc.orig"
  grep -qx "FOO BAR" "$H/.gash_env"
  cmp "$H/.bash_local" "$tmp/local.orig"
  grep -q "Gash 2.0 migration . 7 of 7" "$tmp/out"
  # The settings are read back by Gash 2.
  [[ "$(__gash_reload_env; __gash_get_setting GASH_UMASK)" == 022 ]]
'

it "nothing is written on cancel, on later, or with a settings file to create" bash -c "$__migrate_fixture"'
  # Cancel at the diff preview.
  printf "1\n1\n1\n1\n1\n1\n1\n1\n2\n" | gash migrate --plain >/dev/null 2>&1
  cmp "$H/.gashrc" "$tmp/gashrc.orig"
  __gash_migrate_needed
  [[ ! -d "$XDG_STATE_HOME/gash/backups" ]]
  # "Remind me next time" from the automatic start.
  printf "2\n" | gash_migrate --auto --plain >/dev/null
  __gash_migrate_needed
  # "Don'"'"'t ask again" records the schema and changes no file.
  printf "3\n" | gash_migrate --auto --plain >/dev/null
  ! __gash_migrate_needed
  cmp "$H/.gashrc" "$tmp/gashrc.orig"

  # No ~/.gash_env yet: a chosen setting creates it, private.
  rm -rf "$XDG_STATE_HOME" "$H/.gash_env" "$H/.bash_local"
  printf "1\n1\n2\n2\n1\n1\n" | gash migrate --plain >/dev/null 2>&1
  grep -qx "GASH_REPLACE_TOOLS=1" "$H/.gash_env"
  grep -qx "GASH_UMASK=022" "$H/.gash_env"
  [[ "$(stat -c %a "$H/.gash_env" 2>/dev/null || stat -f %Lp "$H/.gash_env")" == 600 ]]
  grep -q "absent" "$XDG_STATE_HOME"/gash/backups/*/manifest
  gash migrate --rollback --yes >/dev/null
  [[ ! -e "$H/.gash_env" ]]
'

it "gash migrate --status and option errors" bash -c "$__migrate_fixture"'
  [[ "$(gash migrate --status)" == *"pending"* ]]
  ! gash migrate --bogus 2>/dev/null
  [[ "$(gash_migrate --help)" == *"--rollback"* ]]
'

it "__gash_choose reads a number, takes the default, and gives up on bad input" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  r=""
  __gash_choose r 2 a b c <<<"" >/dev/null; [[ "$r" == 2 ]]
  __gash_choose r 1 a b c <<<"3" >/dev/null; [[ "$r" == 3 ]]
  __gash_choose r 1 a b c < <(printf "9\nx\n1\n") >/dev/null; [[ "$r" == 1 ]]
  ! __gash_choose r 1 a b </dev/null >/dev/null
  ! __gash_choose r 1 a b < <(printf "9\n9\n9\n") >/dev/null
  ! __gash_choose r 1 a b <<<"q" >/dev/null
'

it "__gash_choose follows arrow keys, j/k and digits on a terminal" bash -c '
  set -euo pipefail
  script --version 2>&1 | grep -q util-linux || exit 77
  run() { printf "%b" "$1" | script -qec "bash -c '"'"'GASH_HEADLESS=1 source \"$GASH_TEST_ROOT/gash.sh\"; __gash_choose r 1 a b c d && echo RESULT=\$r || echo RESULT=none'"'"'" /dev/null | tr -d "\r" | sed -n "s/^RESULT=//p"; }
  [[ "$(run "\033[B\033[B\033[Aj\r")" == 3 ]]
  [[ "$(run "\033[A\r")" == 4 ]]
  [[ "$(run "4k\r")" == 3 ]]
  [[ "$(run "q")" == none ]]
'
