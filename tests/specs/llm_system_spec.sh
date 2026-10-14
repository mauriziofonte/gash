#!/usr/bin/env bash
#
# llm_ports / llm_project behavior (Layer 2 correctness fixes).

describe "LLM system and project"

it "llm_ports parses ss output with a Netid column and honors --listen" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/ss" <<"SS"
#!/usr/bin/env bash
echo "Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port"
echo "tcp   LISTEN 0      4096   [::]:22            [::]:*"
echo "udp   UNCONN 0      0      0.0.0.0:68         0.0.0.0:*"
[[ "$1" == *l* ]] || echo "tcp   ESTAB  0      0      10.0.0.2:44312     1.2.3.4:443"
SS
  chmod +x "$tmp/bin/ss"
  export PATH="$tmp/bin:$PATH" GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"

  out="$(llm_ports --listen)"
  jq -e "length == 2" <<<"$out" >/dev/null
  jq -e ".[0] == {port:22, proto:\"tcp\", state:\"LISTEN\", address:\"[::]\"}" <<<"$out" >/dev/null
  jq -e ".[1] == {port:68, proto:\"udp\", state:\"LISTEN\", address:\"0.0.0.0\"}" <<<"$out" >/dev/null

  out="$(llm_ports)"
  jq -e "length == 3 and (map(.state) | index(\"ESTAB\") != null)" <<<"$out" >/dev/null
'

it "llm_ports reports a missing ss and netstat as a CONTINUE error" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  # A PATH with the text tools but neither ss nor netstat.
  for t in sort awk gawk mawk; do
    p="$(type -P "$t")" && ln -s "$p" "$tmp/$t"
  done
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  rc=0
  out="$(PATH="$tmp" llm_ports 2>"$tmp/err")" || rc=$?
  [[ $rc -eq 1 && -z "$out" ]]
  jq -e ".error == \"ports_tool_not_found\" and .action == \"CONTINUE\" and .recoverable == true
    and (.hint | test(\"iproute2\") and test(\"net-tools\"))" "$tmp/err" >/dev/null
'

it "llm_project reports has_tests=false for a Go module without tests" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  printf "module x\n" >"$tmp/go.mod"
  printf "package main\n" >"$tmp/main.go"
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(llm_project "$tmp" | jq -r .has_tests)" == "false" ]]
  printf "package main\n" >"$tmp/main_test.go"
  [[ "$(llm_project "$tmp" | jq -r .has_tests)" == "true" ]]
'

it "llm_git_status reports renames, odd names, conflicts and ahead/behind" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp" GIT_CONFIG_NOSYSTEM=1
  git config --global user.email t@example.com
  git config --global user.name T
  git config --global init.defaultBranch main
  source "$GASH_TEST_ROOT/gash.sh"
  git init -q "$tmp/up"; cd "$tmp/up"
  printf "a\n" >a.txt; printf "b\n" >b.txt; git add .; git commit -qm one
  git clone -q "$tmp/up" "$tmp/r"; cd "$tmp/r"
  printf "c\n" >c.txt; git add c.txt; git commit -qm two
  git mv a.txt "renamed a.txt"
  printf "x\n" >>b.txt
  printf "n\n" >"$(printf "new\nline.txt")"
  out="$(llm_git_status)"
  jq -e ".branch == \"main\" and .ahead == 1 and .behind == 0" <<<"$out" >/dev/null
  jq -e ".staged == [\"renamed a.txt\"] and .modified == [\"b.txt\"]" <<<"$out" >/dev/null
  jq -e ".untracked == [\"new\nline.txt\"] and .conflicted == []" <<<"$out" >/dev/null

  # A merge conflict lands in "conflicted", not in staged/modified.
  git reset -q --hard; git clean -qfd
  git checkout -qb side HEAD~1; printf "side\n" >b.txt; git commit -qam side
  git checkout -q main; printf "main\n" >b.txt; git commit -qam main
  git merge -q side >/dev/null 2>&1 || true
  out="$(llm_git_status)"
  jq -e ".conflicted == [\"b.txt\"] and (.staged | index(\"b.txt\")) == null" <<<"$out" >/dev/null
'

it "llm_env honors --filter, redacts URL passwords and hides secret names" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  export GASH_HEADLESS=1 DB_URL="mysql://app:s3cret@db:3306/x" GITHUB_TOKEN=t MY_VAR=1
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(llm_env --filter "^(DB_URL|GITHUB_TOKEN|MY_VAR)$")"
  jq -e "keys == [\"DB_URL\", \"MY_VAR\"]" <<<"$out" >/dev/null
  jq -e ".DB_URL == \"mysql://app:***@db:3306/x\"" <<<"$out" >/dev/null
  [[ "$(llm_env --filter "(" 2>&1 >/dev/null | jq -r .error)" == invalid_filter ]]
'

it "llm_deps reads requirements.txt and go.mod into the same JSON shape" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir "$tmp/py" "$tmp/go"
  printf "# c\nrequests>=2\nflask[async]==3 ; python_version>\"3\"\n-r more.txt\n  numpy\n" >"$tmp/py/requirements.txt"
  printf "module x\n\nrequire (\n\tgithub.com/a/b v1.0.0\n)\nrequire example.com/z v1.2.3\n" >"$tmp/go/go.mod"
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(llm_deps "$tmp/py" | jq -c .dependencies)" == "[\"requests\",\"flask\",\"numpy\"]" ]]
  [[ "$(llm_deps "$tmp/go" --dev | jq -c "[.prod, .dev]")" == "[[\"github.com/a/b\",\"example.com/z\"],[]]" ]]
'

it "llm_git_* never run programs named by the repository config" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  command -v git >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp" GIT_CONFIG_NOSYSTEM=1
  git config --global user.email t@example.com
  git config --global user.name T
  git init -q "$tmp/r"; cd "$tmp/r"
  printf "a\n" >a.txt; printf "*.txt diff=evil\n" >.gitattributes
  git add .; git commit -qm one
  printf "b\n" >>a.txt; git add a.txt; printf "c\n" >>a.txt
  # An untrusted checkout: every hook that git would run writes a marker.
  printf "#!/bin/sh\necho \"\$0 \$*\" >>\"%s/ran\"\nexit 1\n" "$tmp" >"$tmp/evil.sh"
  chmod +x "$tmp/evil.sh"
  mkdir hooks; cp "$tmp/evil.sh" hooks/post-index-change
  git config core.fsmonitor "$tmp/evil.sh"
  git config core.hooksPath "$tmp/r/hooks"
  git config diff.external "$tmp/evil.sh"
  git config diff.evil.command "$tmp/evil.sh"
  git config diff.evil.textconv "$tmp/evil.sh"
  git config core.pager "$tmp/evil.sh"
  git config log.showSignature true
  git config gpg.program "$tmp/evil.sh"
  # Sanity: plain git does run them here (otherwise the test proves nothing).
  git status >/dev/null 2>&1 || true
  [[ -e "$tmp/ran" ]] || exit 77
  rm -f "$tmp/ran"

  source "$GASH_TEST_ROOT/gash.sh"
  llm_git_status | jq -e ".staged == [\"a.txt\"] and .modified == [\"a.txt\"]" >/dev/null
  [[ "$(llm_git_diff)" == *"a.txt"* ]]
  [[ "$(llm_git_diff --staged)" == *"a.txt"* ]]
  llm_git_log | jq -e "length == 1 and .[0].subject == \"one\"" >/dev/null
  [[ ! -e "$tmp/ran" ]] || { cat "$tmp/ran"; exit 1; }
'
