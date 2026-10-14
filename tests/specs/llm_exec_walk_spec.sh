#!/usr/bin/env bash
#
# llm_exec: what recursive commands and git may read (secret files, protected
# trees), the child's environment, and the read-only git and yq rules.

describe "LLM exec walks, git contents and environment"

it "grep -r, egrep, rg and diff -r leave secret files out" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  mkdir -p p/sub/.ssh p/.config/gcloud empty
  printf "PASSWORD=env\n" >p/.env
  printf "PASSWORD=local\n" >p/sub/.ENV.local
  printf "PASSWORD=key\n" >p/sub/.ssh/notes
  printf "PASSWORD=gcloud\n" >p/.config/gcloud/adc.json
  printf "PASSWORD=cred\n" >p/sub/credentials
  printf "PASSWORD=ok\n" >p/app.txt
  ln -s "$tmp/p/.env" p/innocent.txt

  for line in "grep -r PASSWORD p" "grep -rn PASSWORD p --include=\\*" "grep -r PASSWORD -- p" \
      "egrep -r PASS p" "fgrep -r PASS p" "diff -rN p empty"; do
    out="$(llm_exec "$line" 2>&1 || true)"
    [[ "$out" == *ok* ]] || { echo "nothing found: $line: $out"; exit 1; }
    for leak in env local key gcloud cred; do
      [[ "$out" != *"PASSWORD=$leak"* ]] || { echo "leak ($leak): $line: $out"; exit 1; }
    done
  done
  if command -v rg >/dev/null 2>&1; then
    for line in "rg PASSWORD p" "rg -uuu PASSWORD p" "rg -uuu PASSWORD p -g *.txt -g *.local" "rg PASSWORD -- p"; do
      out="$(llm_exec "$line" 2>&1 || true)"
      [[ "$out" == *PASSWORD=ok* && "$out" != *PASSWORD=env* && "$out" != *PASSWORD=local* && "$out" != *PASSWORD=key* ]] ||
        { echo "rg: $line: $out"; exit 1; }
    done
  fi
'

it "walks that follow symlinks are refused" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  mkdir p
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }
  for line in "grep -R x p" "grep -rnR x p" "grep --dereference-recursive x p" "rg -L x p" "rg --follow x p" \
      "find -L p" "find p -follow" "find p -files0-from list" "ls -RL p" "tree -l p" "du -L p" "fd -L x p"; do
    [[ "$(verdict "$line")" == option_not_allowed ]] || { echo "not refused: $line"; exit 1; }
  done
  # Not a walk: allowed.
  [[ "$(llm_exec "ls -L p && echo ok")" == ok ]]
'

it "walks cannot start at / or above a protected tree" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  mkdir -p p/d
  ln -s / p/rootlink
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }
  for line in "grep -r PASS /" "grep -r PASS /etc" "grep -d recurse PASS /" "rg PASS /" "rg -e PASS /etc" \
      "find / -name x" "ls -R /" "du -sh /" "tree /" "fd x /" "fd x --search-path /" "diff -r / p" \
      "ls -R p/rootlink" "cd / && grep -r PASS" "cd / && find . -name x" "cd / && rg PASS"; do
    [[ "$(verdict "$line")" == forbidden_path ]] || { echo "not refused: $line"; exit 1; }
  done
  err="$(llm_exec "find / -name x" 2>&1 >/dev/null || true)"
  jq -e ".action == \"FATAL\" and .details == \"find /\"" <<<"$err" >/dev/null
  # Listings may start at /etc (only content searches would read /etc/shadow);
  # plain listings and walks below / are fine.
  llm_exec "find /etc -maxdepth 0" >/dev/null
  llm_exec "ls / >/dev/null"
  [[ "$(llm_exec "cd / && find $tmp/p -name d")" == "$tmp/p/d" ]]
  [[ "$(llm_exec "grep -r -e / p || echo none")" == none ]]
'

it "git commands that print contents leave secret files out" bash -c '
  set -euo pipefail
  command -v git >/dev/null 2>&1 || exit 77
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  git init -q r
  cd r
  git config user.email t@t
  git config user.name t
  mkdir -p sub/.ssh
  printf "PASSWORD=env\n" >.env
  printf "PASSWORD=local\n" >.ENV.local
  printf "PASSWORD=key\n" >sub/.ssh/notes
  printf "PASSWORD=ok\n" >app.txt
  git add -A
  git commit -qm one
  printf "PASSWORD=env2\n" >.env
  printf "PASSWORD=ok2\n" >app.txt
  git commit -qam two
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }

  for line in "git grep PASSWORD" "git grep -h PASSWORD HEAD~1" "git grep PASSWORD -- ." "git log -p" \
      "git log --all -p --format=%s" "git show" "git show HEAD~1" "git diff HEAD~1 HEAD" "git whatchanged -p" \
      "git reflog -p" "git -C $tmp/r log -p"; do
    out="$(llm_exec "$line" 2>&1)"
    [[ "$out" == *PASSWORD=ok* ]] || { echo "nothing found: $line: $out"; exit 1; }
    [[ "$out" != *PASSWORD=env* && "$out" != *PASSWORD=local* && "$out" != *PASSWORD=key* ]] ||
      { echo "leak: $line: $out"; exit 1; }
  done
  # Listings keep every commit; a REV:PATH is checked like a file name.
  [[ "$(llm_exec "git log --oneline" | wc -l)" -eq 2 ]]
  [[ "$(llm_exec "git show HEAD:app.txt")" == PASSWORD=ok2 ]]
  [[ "$(verdict "git show HEAD:.ENV.local")" == secret_file_blocked ]]
  [[ "$(verdict "git log -L1,1:.ENV.local")" == secret_file_blocked ]]
  # A blob named by its id has no path to check.
  blob="$(git rev-parse HEAD:.env)"
  for line in "git show $blob" "git cat-file -p $blob" "git cat-file blob $blob" "git diff $blob $(git rev-parse HEAD~1:.env)"; do
    [[ "$(verdict "$line")" == option_not_allowed ]] || { echo "not refused: $line"; exit 1; }
  done
  [[ "$(llm_exec "git cat-file -t $blob")" == blob ]]
  # Ways around the pathspecs, refused.
  for line in "git --literal-pathspecs log -p" "git grep --no-index PASS" "git diff --no-index . $tmp" \
      "git cat-file --batch" "git stash show -p" "git remote show origin"; do
    [[ "$(verdict "$line")" == option_not_allowed ]] || { echo "not refused: $line"; exit 1; }
  done
'

it "git in llm_exec takes no optional locks: status does not write the index" bash -c '
  set -euo pipefail
  command -v git >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  git init -q r
  cd r
  printf "a\n" >f
  git add f
  # A stale stat cache: plain git status would rewrite .git/index.
  touch -d "2001-01-01" f
  before="$(cksum <.git/index)"
  llm_exec "git status --short" >/dev/null
  llm_git_status . >/dev/null
  [[ "$(cksum <.git/index)" == "$before" ]]
'

it "the llm_exec child gets a short allowlisted environment" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  export MY_API_TOKEN=sekrit GH_PAT=ghp_x LC_TIME=C.UTF-8 RIPGREP_CONFIG_PATH="$tmp/rgrc" POSIXLY_CORRECT=1
  env="$(llm_exec "jq -nc env")"
  jq -e ".MY_API_TOKEN == null and .GH_PAT == null and .RIPGREP_CONFIG_PATH == null and .POSIXLY_CORRECT == null" <<<"$env" >/dev/null
  jq -e ".HOME == \"$tmp\" and .PATH == \"$PATH\" and .LC_TIME == \"C.UTF-8\" and .GIT_OPTIONAL_LOCKS == \"0\"" <<<"$env" >/dev/null
  [[ "$(llm_exec "jq -n env.MY_API_TOKEN")" == null ]]
  # ps e would print other processes environments: refused.
  [[ "$(llm_exec "ps eww" 2>&1 >/dev/null | jq -r .error)" == option_not_allowed ]]
  [[ "$(llm_exec "ps auxe" 2>&1 >/dev/null | jq -r .error)" == option_not_allowed ]]
  llm_exec "ps -o pid,etime -p $$" >/dev/null
  # Unrestricted mode keeps the caller environment (only the blocklist applies).
  printf "GASH_LLM_EXEC=unrestricted\n" >"$tmp/.gash_env"
  __gash_reload_env
  [[ "$(llm_exec "jq -nr env.MY_API_TOKEN")" == sekrit ]]
'

it "yq load functions and split files, getent shadow are refused" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  verdict() { llm_exec "$1" 2>&1 >/dev/null | jq -r .error; }
  for line in "yq -n '"'"'load(\"x.yml\")'"'"'" "yq '"'"'.a = load_str(\"f\")'"'"' a.yml" "yq -s .a a.yml" "yq --split-exp .a a.yml"; do
    [[ "$(verdict "$line")" == option_not_allowed ]] || { echo "not refused: $line"; exit 1; }
  done
  [[ "$(verdict "getent shadow")" == forbidden_path ]]
  [[ "$(verdict "getent gshadow root")" == forbidden_path ]]
'
