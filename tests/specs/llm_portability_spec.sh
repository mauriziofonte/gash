#!/usr/bin/env bash
#
# llm_* on systems that differ from the developer's: comma-decimal locales,
# realpath without -m, env without -0, the two yq flavours.

describe "LLM portability"

it "timings work when EPOCHREALTIME uses a decimal comma (it_IT, de_DE)" bash -c '
  set -euo pipefail
  export GASH_HEADLESS=1
  source "$GASH_TEST_ROOT/gash.sh"
  [[ "$(__llm_elapsed_us "1792007131,734850" "1792007131,834851")" == 100001 ]]
  [[ "$(__llm_elapsed_us "1792007131.999999" "1792007132.000001")" == 2 ]]
  # The special variable loses its magic once unset: simulate the locale.
  ( unset EPOCHREALTIME; EPOCHREALTIME="1792007131,734850"; [[ "$(__gash_fs_now_ms)" == 1792007131734 ]] )
  # A real comma locale, when the system has one.
  loc="$(locale -a 2>/dev/null | grep -Eix "(it_IT|de_DE|fr_FR|es_ES|pt_BR)\.utf-?8" | head -n 1 || true)"
  if [[ -n "$loc" ]]; then
    tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
    out="$(LC_ALL="$loc" bash -c "export GASH_HEADLESS=1; source \"$GASH_TEST_ROOT/gash.sh\"; [[ \$EPOCHREALTIME == *,* ]] || exit 0; files_largest --json \"$tmp\"")"
    [[ -z "$out" ]] || jq -e ".scan_time | test(\"^[0-9.]+(ms|s)$\")" <<<"$out" >/dev/null
  fi
'

it "a slow-looking clock never makes llm_db_query run twice" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  cat >"$tmp/bin/mysql" <<"MOCK"
#!/usr/bin/env bash
echo x >>"$MOCK_DIR/calls"
cat >/dev/null
printf "id\n1\n"
MOCK
  chmod +x "$tmp/bin/mysql"
  cp "$tmp/bin/mysql" "$tmp/bin/mariadb"
  printf "DB:default=mysql://u:p@127.0.0.1:3306/app\n" >"$tmp/.gash_env"
  export PATH="$tmp/bin:$PATH" GASH_HEADLESS=1 HOME="$tmp" MOCK_DIR="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  unset EPOCHREALTIME
  EPOCHREALTIME="1792007131,734850"
  out="$(llm_db_query "SELECT 1")"
  jq -e ".query_time == \"0µs\" and (has(\"slow_query_explain\") | not)" <<<"$out" >/dev/null
  [[ "$(wc -l <"$tmp/calls")" -eq 1 ]]
'

it "path guards resolve symlinks without realpath -m (BSD, older macOS)" bash -c '
  set -euo pipefail
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin" "$tmp/a/b"
  # A realpath that knows no -m, like BSD.
  printf "#!/bin/sh\n[ \"\$1\" = -m ] && exit 1\nexit 1\n" >"$tmp/bin/realpath"
  chmod +x "$tmp/bin/realpath"
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  cd "$tmp"
  ln -s a/b l
  ln -s ../.. a/b/up
  ln -s loop2 loop1
  ln -s loop1 loop2
  ln -s /root rootlink
  printf "S=1\n" >.env
  ln -s .env settings.json
  expect=()
  cases=(l l/../x a/b/up/x /etc//x/./y/../z . a/./b/ / "a/new line" missing/x)
  for c in "${cases[@]}"; do expect+=("$(realpath -m -- "$c")|$(realpath -m -s -- "$c")"); done
  export PATH="$tmp/bin:$PATH"
  [[ "$(type -P realpath)" == "$tmp/bin/realpath" ]]
  for i in "${!cases[@]}"; do
    got="$(__llm_realpath "${cases[i]}")|$(__llm_realpath -s "${cases[i]}")"
    [[ "$got" == "${expect[i]}" ]] || { echo "${cases[i]}: $got != ${expect[i]}"; exit 1; }
  done
  ! __llm_realpath loop1 >/dev/null
  ! __llm_validate_path rootlink/x >/dev/null 2>&1
  __llm_is_secret_file settings.json
'

it "llm_env works where env has no -0 (BSD)" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  printf "#!/bin/sh\necho \"env: illegal option -- 0\" >&2\nexit 1\n" >"$tmp/bin/env"
  chmod +x "$tmp/bin/env"
  export GASH_HEADLESS=1 HOME="$tmp" MY_MULTI=$'"'"'a\nb=c'"'"'
  source "$GASH_TEST_ROOT/gash.sh"
  out="$(PATH="$tmp/bin:$PATH" llm_env)"
  jq -e ".HOME == \"$tmp\" and .MY_MULTI == \"a\nb=c\" and (has(\"b\") | not)" <<<"$out" >/dev/null
'

it "llm_config converts YAML with either yq flavour and falls back to the raw text" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  mkdir -p "$tmp/bin"
  printf "a: 1\n" >"$tmp/c.yml"
  export GASH_HEADLESS=1 HOME="$tmp" PATH="$tmp/bin:$PATH"
  source "$GASH_TEST_ROOT/gash.sh"
  mock() {
    printf "#!/usr/bin/env bash\n[[ \$1 == --version ]] && { echo \"%s\"; exit 0; }\nprintf \"%%s\\\\n\" \"\$*\" >\"%s/yq.args\"\n%s\n" "$1" "$tmp" "$2" >"$tmp/bin/yq"
    chmod +x "$tmp/bin/yq"
    unset __LLM_YQ_FLAVOUR
  }
  # kislyuk (python-yq, the Debian/Ubuntu package): yq . FILE
  mock "yq 3.2.3" "echo \"{\\\"a\\\": 1}\""
  jq -e ".a == 1" <<<"$(llm_config "$tmp/c.yml")" >/dev/null
  [[ "$(cat "$tmp/yq.args")" == ". $tmp/c.yml" ]]
  # mikefarah (Go): yq eval -o=json . FILE
  mock "yq (https://github.com/mikefarah/yq/) version v4.40.5" "echo \"{\\\"a\\\": 1}\""
  jq -e ".a == 1" <<<"$(llm_config "$tmp/c.yml")" >/dev/null
  [[ "$(cat "$tmp/yq.args")" == "eval -o=json . $tmp/c.yml" ]]
  # A yq that fails, or one Gash does not know: the raw text, exit 0.
  mock "yq 3.2.3" "echo \"jq: Unknown option -o=json\" >&2; exit 2"
  [[ "$(llm_config "$tmp/c.yml" 2>&1)" == "a: 1" ]]
  mock "yq version 3.4.1" "echo nope"
  [[ "$(llm_config "$tmp/c.yml" 2>&1)" == "a: 1" ]]
'

it "llm_config refuses files over --max-bytes (1 MiB by default) with a JSON error" bash -c '
  set -euo pipefail
  gash_require jq
  tmp="$(mktemp -d)"; trap "rm -rf $tmp" EXIT
  export GASH_HEADLESS=1 HOME="$tmp"
  source "$GASH_TEST_ROOT/gash.sh"
  head -c 1048577 /dev/zero | tr "\0" "a" >"$tmp/big.txt"
  printf "{}\n" >"$tmp/small.json"
  err="$(llm_config "$tmp/big.txt" 2>&1 >/dev/null || true)"
  jq -e ".error == \"file_too_large\" and .action == \"RETRY\"" <<<"$err" >/dev/null
  [[ "$(llm_config "$tmp/big.txt" --max-bytes 2000000 | wc -c)" -eq 1048577 ]]
  [[ "$(llm_config --max-bytes 2 "$tmp/small.json" 2>&1 >/dev/null | jq -r .error)" == file_too_large ]]
  [[ "$(llm_config "$tmp/small.json" --max-bytes x 2>&1 >/dev/null | jq -r .error)" == invalid_size ]]
  [[ "$(llm_config "$tmp/small.json")" == "{}" ]]
'
