#!/usr/bin/env bash
#
# Interactive startup: what Gash runs (and does not run) when a shell
# starts, the probe cache, aliases that would hide a program, load timing.
# Every test runs a fresh interactive bash with HOME pointing at a temporary
# directory where ~/.gash is a symlink to this checkout.

describe "Startup"

it "GASH_TIMING=1 records each file; nothing is recorded by default" bash -c '
  set -euo pipefail
  [[ -n "${EPOCHREALTIME-}" ]] || { echo "SKIP: needs Bash 5.0+"; exit 77; }
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"

  probe="source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; source \"\$HOME/.gash/bash_completion\"; printf \"%s\n\" \"\${__GASH_LOAD_TIMES[@]}\""
  out="$(HOME="$tmp" GASH_TIMING=1 bash --noprofile --norc -i -c "$probe" 2>/dev/null)"
  grep -Eq "^[0-9]+ lib/core/output.sh$" <<<"$out"
  grep -Eq "^[0-9]+ lib/shell/prompt.sh$" <<<"$out"
  grep -Eq "^[0-9]+ lib/aliases/git.sh$" <<<"$out"
  grep -Eq "^[0-9]+ gash.sh \(total\)$" <<<"$out"
  grep -Eq "^[0-9]+ bash_completion$" <<<"$out"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$probe" 2>/dev/null)"
  [[ -z "$out" ]]
'

it "gash_doctor --timing lists the files and the whole-shell time" bash -c '
  set -euo pipefail
  [[ -n "${EPOCHREALTIME-}" ]] || { echo "SKIP: needs Bash 5.0+"; exit 77; }
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  cp "$GASH_TEST_ROOT/.gashrc" "$tmp/.gashrc"
  echo "source ~/.gashrc" >"$tmp/.bashrc"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; gash_doctor --timing" 2>&1 </dev/null)"
  grep -Eq "^ +[0-9]+\.[0-9]  lib/modules/gash.sh$" <<<"$out"
  grep -Eq "^ +[0-9]+\.[0-9]  gash.sh \(total\)$" <<<"$out"
  grep -Eq "New shell with your ~/.bashrc: [0-9]+\.[0-9] ms \(bash alone: [0-9]+\.[0-9] ms" <<<"$out"
  # The child shell printed its banner somewhere else.
  [[ "$out" != *"Quote:"* ]]
'

it "a new shell runs no probe program once the cache is warm" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  mkdir "$tmp/bin"
  # Wrappers that log each call, then run the real program.
  for p in ls rm grep dircolors lesspipe tput fold mesg; do
    real="$(type -P "$p" || true)"
    [[ -n "$real" ]] || continue
    printf "#!/bin/sh\necho %s >>\"%s/calls\"\nexec %s \"\$@\"\n" "$p" "$tmp" "$real" >"$tmp/bin/$p"
    chmod +x "$tmp/bin/$p"
  done
  load="source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; alias ls; alias rm 2>/dev/null; printf \"LS_COLORS=%s\n\" \"\${LS_COLORS-}\""

  first="$(HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "$load" 2>/dev/null </dev/null)"
  [[ -s "$tmp/.cache/gash/probes" ]]
  : >"$tmp/calls"
  second="$(HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "$load" 2>/dev/null </dev/null)"
  [[ "$first" == "$second" ]] || { echo "first: $first"; echo "second: $second"; exit 1; }
  if [[ -s "$tmp/calls" ]]; then
    echo "programs run with a warm cache:"; cat "$tmp/calls"; exit 1
  fi
'

it "the probe cache is ignored when it does not match or holds bad values" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  load="source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; alias ls"

  want="$(HOME="$tmp" bash --noprofile --norc -i -c "$load" 2>/dev/null </dev/null)"
  cache="$tmp/.cache/gash/probes"
  # A value outside the allowed set is never used.
  sed -i.bak "s|^ls_flag=.*|ls_flag=--evil; touch $tmp/pwned|" "$cache"
  out="$(HOME="$tmp" bash --noprofile --norc -i -c "$load" 2>/dev/null </dev/null)"
  [[ "$out" == "$want" && ! -e "$tmp/pwned" ]]
  # Another TERM is another key: probed again, cache rewritten.
  out="$(HOME="$tmp" TERM=dumb-other bash --noprofile --norc -i -c "$load" 2>/dev/null </dev/null)"
  [[ "$out" == "$want" ]]
  grep -q "|dumb-other|" "$cache"
  # XDG_CACHE_HOME moves the cache.
  HOME="$tmp" XDG_CACHE_HOME="$tmp/xdg" bash --noprofile --norc -i -c "$load" >/dev/null 2>&1 </dev/null
  [[ -s "$tmp/xdg/gash/probes" ]]
'

it "an alias named like a program on PATH is not defined; wrappers and earlier aliases stay" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  mkdir "$tmp/bin"
  for p in gp gst gc; do printf "#!/bin/sh\n" >"$tmp/bin/$p"; chmod +x "$tmp/bin/$p"; done
  show="for a in gp gst gc gpr ls; do printf \"%s=%s\n\" \"\$a\" \"\${BASH_ALIASES[\$a]-none}\"; done"

  out="$(HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "set -euo pipefail; alias gc=\"echo mine\"; source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; $show" 2>/dev/null </dev/null)"
  grep -qx "gp=none" <<<"$out"
  grep -qx "gst=none" <<<"$out"
  grep -qx "gpr=git pull --rebase" <<<"$out"
  # Defined before Gash: left to Gash as before (not dropped).
  grep -qx "gc=git commit" <<<"$out"
  grep -q "^ls=ls" <<<"$out"

  # Same answer from the cache (also under set -euo pipefail); a program
  # removed from PATH brings its alias back.
  out2="$(HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "set -euo pipefail; alias gc=\"echo mine\"; source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; $show" 2>/dev/null </dev/null)"
  [[ "$out" == "$out2" ]]
  sleep 1
  rm "$tmp/bin/gp"
  out="$(HOME="$tmp" PATH="$tmp/bin:$PATH" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; $show" 2>/dev/null </dev/null)"
  grep -qx "gp=git pull" <<<"$out"
'

it "LESSOPEN and LESSCLOSE match what eval \"\$(lesspipe)\" sets" bash -c '
  set -euo pipefail
  command -v lesspipe >/dev/null 2>&1 || { echo "SKIP: lesspipe not available"; exit 77; }
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  want="$(unset LESSOPEN LESSCLOSE; SHELL=/bin/bash; eval "$(lesspipe)"; printf "%s|%s" "$LESSOPEN" "$LESSCLOSE")"
  out="$(env -u LESSOPEN -u LESSCLOSE HOME="$tmp" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; printf \"%s|%s\" \"\$LESSOPEN\" \"\$LESSCLOSE\"" 2>/dev/null </dev/null)"
  [[ "$out" == "$want" ]] || { echo "want: $want"; echo "got:  $out"; exit 1; }
  # A LESSOPEN already set is kept.
  out="$(LESSOPEN="|mine %s" HOME="$tmp" bash --noprofile --norc -i -c "source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1; printf \"%s\" \"\$LESSOPEN\"" 2>/dev/null </dev/null)"
  [[ "$out" == "|mine %s" ]]
'

it "__gash_path_of finds what type -P finds, without a subshell" bash -c '
  set -euo pipefail
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  source "$GASH_TEST_ROOT/lib/core/utils.sh"
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/a" "$tmp/b" "$tmp/cwd"
  printf "#!/bin/sh\n" >"$tmp/b/tool"; chmod +x "$tmp/b/tool"
  mkdir "$tmp/a/tool"
  cp "$tmp/b/tool" "$tmp/cwd/here"
  cd "$tmp/cwd"
  for p in "$tmp/a:$tmp/b" "$tmp/a:" ":$tmp/a" "$tmp/a::$tmp/b" "$tmp/a"; do
    for n in tool here missing; do
      want="$(PATH="$p" type -P "$n" || true)"
      got=""; PATH="$p" __gash_path_of "$n" && got="$REPLY"
      [[ "$want" == "$got" ]] || { echo "PATH=$p $n: want [$want] got [$got]"; exit 1; }
    done
  done
'

it "the hook builds the prompt; the PS1 set at load runs no git status" bash -c '
  set -euo pipefail
  command -v git >/dev/null 2>&1 || { echo "SKIP: git not available"; exit 77; }
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  git init -q "$tmp/repo" 2>&1
  git -C "$tmp/repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m init 2>&1
  out="$(cd "$tmp/repo" && HOME="$tmp" TERM=dumb bash --noprofile --norc -i -c "
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    printf \"load:%s\n\" \"\$PS1\"
    __gash_prompt_command
    printf \"hook:%s\n\" \"\$PS1\"
  " 2>/dev/null </dev/null)"
  load="$(grep "^load:" <<<"$out" || true)"
  hook="$(grep "^hook:" <<<"$out" || true)"
  [[ "$load" != *"(master"* && "$load" != *"(main"* ]]
  [[ "$hook" == *"(master"* || "$hook" == *"(main"* ]]
'

it "the hook goes first in a PROMPT_COMMAND array or string, once" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    PROMPT_COMMAND=(\"echo a\" \"echo b\")
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    declare -p PROMPT_COMMAND
    __gash_prompt_uninstall
    declare -p PROMPT_COMMAND
  " 2>/dev/null </dev/null)"
  grep -q "PROMPT_COMMAND=(\[0\]=\"__gash_prompt_command\" \[1\]=\"echo a\" \[2\]=\"echo b\")" <<<"$out"
  grep -q "PROMPT_COMMAND=(\[0\]=\"echo a\" \[1\]=\"echo b\")" <<<"$out"

  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    PROMPT_COMMAND=\"echo a\"
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    printf \"%s\n\" \"\$PROMPT_COMMAND\"
    __gash_prompt_uninstall
    printf \"%s\n\" \"\$PROMPT_COMMAND\"
  " 2>/dev/null </dev/null)"
  [[ "$out" == "__gash_prompt_command; echo a"$'\''\n'\''"echo a" ]]
'

it "completion is registered for public Gash functions and help aliases only" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  out="$(HOME="$tmp" bash --noprofile --norc -i -c "
    mine() { :; }
    _mine() { :; }
    source \"\$HOME/.gash/gash.sh\" >/dev/null 2>&1
    source \"\$HOME/.gash/bash_completion\"
    complete -p
  " 2>/dev/null </dev/null)"
  grep -q " files_largest$" <<<"$out"
  grep -q " llm_tree$" <<<"$out"
  grep -q " gdoctor$" <<<"$out"
  ! grep -q " mine$\| _mine$\| needs_help$\| __gash_complete$" <<<"$out"
'

it "an interactive shell with Gash starts in under 400 ms (median of 5)" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  ln -s "$GASH_TEST_ROOT" "$tmp/.gash"
  cp "$GASH_TEST_ROOT/.gashrc" "$tmp/.gashrc"
  echo "source ~/.gashrc" >"$tmp/.bashrc"
  cd "$tmp"
  HOME="$tmp" bash -i -c exit </dev/null >/dev/null 2>&1   # warm the probe cache
  times=()
  for i in 1 2 3 4 5; do
    t="$( { TIMEFORMAT=%3R; time HOME="$tmp" bash -i -c exit </dev/null >/dev/null 2>&1; } 2>&1 )"
    t="${t//[!0-9]/}"
    times+=("$((10#$t))")
  done
  median="$(printf "%s\n" "${times[@]}" | sort -n | sed -n 3p)"
  echo "startup ms: ${times[*]} (median $median)"
  ((median < 400))
'
