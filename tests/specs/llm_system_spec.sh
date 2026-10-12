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

it "llm_ports returns an empty array without ss or netstat" bash -c '
  set -euo pipefail
  command -v jq >/dev/null 2>&1 || exit 77
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  # A PATH with the text tools but neither ss nor netstat.
  for t in sort awk gawk mawk; do
    p="$(type -P "$t")" && ln -s "$p" "$tmp/$t"
  done
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(PATH="$tmp" llm_ports)"
  [[ "$out" == "[]" ]]
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
