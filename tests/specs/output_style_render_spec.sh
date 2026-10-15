#!/usr/bin/env bash
#
# Output style, rendering (R1-R7). Every public human-facing function runs
# under a real pseudo-terminal (script(1)), loaded as an interactive user
# has it, and the bytes it prints are checked against the style contract:
#
#   R1  every key of __GASH_HELP_REGISTRY not starting with llm_, plus
#       gash_username, has at least one fixture
#   R2  fixtures run on a PTY with tests/mocks/bin first on PATH,
#       COLUMNS=100, LANG=C.UTF-8 (no network, no sudo, no real Docker)
#   R3  every SGR sequence is a palette value, an attribute, the reset, or
#       one that output.sh lists in __gash_sgr_extra
#   R4  the only other CSI sequences are erase-line, cursor hide/show and
#       the clear-screen primitive's
#   R5  status lines on the terminal start with a glyph, never with a word
#       prefix or an ASCII marker
#   R6  with stdout piped, and with NO_COLOR=1: no ESC byte at all, and
#       status lines carry their word prefix
#   R7  headless loading prints nothing
#
# Fixture format: fx FUNCTION LABEL CODE [INPUT]
#   CODE runs in bash after Gash is loaded, in a fresh directory that is
#   also HOME. INPUT is typed on the terminal (e.g. "n" answers a
#   confirmation); end of input follows, so a further read gets EOF.

describe "Output style: rendering on a terminal"

__fx_ids=()
declare -A __fx_func=() __fx_code=() __fx_input=()
fx() {
  local id="$1:$2"
  __fx_ids+=("$id")
  __fx_func[$id]="$1"
  __fx_code[$id]="$3"
  __fx_input[$id]="${4-}"
}

# -----------------------------------------------------------------------------
# Fixtures
# -----------------------------------------------------------------------------

# @@FIXTURES@@

fx gash_username plain 'gash_username'

# -----------------------------------------------------------------------------
# Harness
# -----------------------------------------------------------------------------

__render_dir="$(mktemp -d "${TMPDIR:-/tmp}/gash-render.XXXXXX")"
trap 'command rm -rf "$__render_dir"; trap - EXIT' EXIT
export __render_dir

# util-linux script on Linux, BSD script on macOS.
if script -qec true /dev/null >/dev/null 2>&1; then
  __render_pty=util-linux
else
  __render_pty=bsd
fi
export __render_pty

# The child: a terminal without echo or newline translation, Gash loaded as
# in color_gating_spec.sh, then the fixture.
cat >"$__render_dir/child.sh" <<'EOF'
dir="$1" mode="$2"
stty -echo -onlcr 2>/dev/null
: >"$dir/ready"
unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
[[ "$mode" == nocolor ]] && export NO_COLOR=1
export HOME="$dir/home" COLUMNS=100 LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=xterm-256color
export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
mkdir -p "$HOME"
cd "$HOME" || exit 99
source "$GASH_TEST_ROOT/tests/gash-test.sh"
gash_source_all "$GASH_TEST_ROOT"
if [[ "$mode" == pipe ]]; then
    { source "$dir/code.sh"; } | cat
else
    source "$dir/code.sh"
fi
EOF

# Run fixture ID in MODE (tty, pipe, nocolor); the bytes go to out.MODE.
__render_run() {
  local id="$1" mode="$2" n="$3"
  local dir="$__render_dir/$n.$mode"
  mkdir -p "$dir"
  printf '%s\n' "${__fx_code[$id]}" >"$dir/code.sh"
  local cmd="bash --noprofile --norc $__render_dir/child.sh $dir $mode"
  {
    local i=0
    while [[ ! -e "$dir/ready" ]] && ((i < 400)); do
      sleep 0.05
      i=$((i + 1))
    done
    [[ -n "${__fx_input[$id]}" ]] && printf '%s\n' "${__fx_input[$id]}"
    # End of input for every later read.
    for i in 1 2 3 4 5 6 7 8; do printf '\004'; done
  } | if [[ "$__render_pty" == util-linux ]]; then
    timeout 60 script -qec "$cmd" /dev/null
  else
    timeout 60 script -q /dev/null bash -c "$cmd"
  fi >"$__render_dir/$n.$mode.out" 2>&1
  printf '%s\n' "$?" >"$__render_dir/$n.$mode.rc"
}

# All runs, in parallel.
__render_jobs="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
__render_n=0
for __render_id in "${__fx_ids[@]}"; do
  for __render_mode in tty pipe nocolor; do
    while (($(jobs -rp | wc -l) >= __render_jobs)); do
      wait -n 2>/dev/null || true
    done
    __render_run "$__render_id" "$__render_mode" "$__render_n" &
  done
  printf '%s\n' "$__render_id" >"$__render_dir/$__render_n.id"
  __render_n=$((__render_n + 1))
done
wait
unset __render_id __render_mode __render_n

# The SGR sequences R3 accepts, from output.sh itself.
(
  unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  printf '%s\n' "$__GASH_COLOR_PRIMARY" "$__GASH_COLOR_SECONDARY" "$__GASH_COLOR_ACCENT" \
    "$__GASH_COLOR_MUTED" "$__GASH_COLOR_SUCCESS" "$__GASH_COLOR_WARNING" \
    "$__GASH_COLOR_ERROR" "$__GASH_COLOR_INFO" "$__GASH_BOLD" "$__GASH_DIM" \
    "$__GASH_UNDERLINE" "$__GASH_COLOR_OFF"
  __gash_sgr_extra
) | LC_ALL=C sort -u >"$__render_dir/sgr.allowed"

# Check one fixture's captures (number N). Prints every violation.
__render_check() {
  local n="$1" id fails=0 f seq line
  id="$(cat "$__render_dir/$n.id")"
  local tty="$__render_dir/$n.tty.out"
  local esc=$'\e'

  for f in tty pipe nocolor; do
    if [[ "$(cat "$__render_dir/$n.$f.rc")" == 124 ]]; then
      printf '%s: %s run timed out (waiting for input?)\n' "$id" "$f"
      fails=1
    fi
  done

  # R3: SGR allowlist.
  while IFS= read -r seq; do
    [[ -z "$seq" ]] && continue
    if ! grep -qxF -- "$seq" "$__render_dir/sgr.allowed"; then
      printf '%s: R3 SGR not in the palette: ' "$id"
      printf '%s' "$seq" | od -c | head -n 1
      fails=1
    fi
  done < <(LC_ALL=C grep -aoE "${esc}\\[[0-9;]*m" "$tty" | LC_ALL=C sort -u)

  # R4: other escape sequences.
  while IFS= read -r seq; do
    [[ -z "$seq" ]] && continue
    case "$seq" in
      "${esc}[K" | "${esc}[?25l" | "${esc}[?25h" | "${esc}[H" | "${esc}[2J") ;;
      *)
        printf '%s: R4 escape sequence not allowed: ' "$id"
        printf '%s' "$seq" | od -c | head -n 1
        fails=1
        ;;
    esac
  done < <(LC_ALL=C sed "s/${esc}\\[[0-9;]*m//g" "$tty" | LC_ALL=C grep -aoE "${esc}(\\[[0-9;?]*[@-~]|.)" | LC_ALL=C sort -u)

  # R5: status lines start with a glyph.
  while IFS= read -r line; do
    printf '%s: R5 status line without a glyph: %s\n' "$id" "$line"
    fails=1
  done < <(LC_ALL=C sed "s/${esc}\\[[0-9;?]*[@-~]//g" "$tty" | tr '\r' '\n' |
    grep -aE '^[[:space:]]*((Info|OK|Warning|Error|Debug|SSH):|\[(!|\+|i|OK|x|X)\])' || true)

  # R6: plain modes.
  for f in pipe nocolor; do
    if LC_ALL=C grep -aq "$esc" "$__render_dir/$n.$f.out"; then
      printf '%s: R6 escape bytes with %s: ' "$id" "$f"
      LC_ALL=C grep -ao "${esc}.\{0,8\}" "$__render_dir/$n.$f.out" | head -n 1 | od -c | head -n 1
      fails=1
    fi
    while IFS= read -r line; do
      printf '%s: R6 status line without its word prefix (%s): %s\n' "$id" "$f" "$line"
      fails=1
    done < <(tr '\r' '\n' <"$__render_dir/$n.$f.out" | grep -aE '^[[:space:]]*(✓|✗|! )' || true)
  done

  if ((fails)); then
    printf '\n--- %s on the terminal ---\n' "$id"
    head -c 3000 "$tty" | cat -v
    return 1
  fi
  return 0
}
export -f __render_check

# -----------------------------------------------------------------------------
# Checks
# -----------------------------------------------------------------------------

__render_list="$(
  for __render_f in "$__render_dir"/*.id; do
    printf '%s %s\n' "$(basename "$__render_f" .id)" "$(cat "$__render_f")"
  done | sort -n
)"
printf '%s\n' "${__fx_func[@]}" | LC_ALL=C sort -u >"$__render_dir/covered"

it "R1: every public human-facing function has a fixture" bash -c '
    set -euo pipefail
    want="$(bash -c "
        unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
        source \"\$GASH_TEST_ROOT/tests/gash-test.sh\"
        gash_source_all \"\$GASH_TEST_ROOT\"
        for k in \"\${!__GASH_HELP_REGISTRY[@]}\"; do
            [[ \"\$k\" == llm_* ]] || echo \"\$k\"
        done
        echo gash_username
    " | LC_ALL=C sort -u)"
    missing="$(LC_ALL=C comm -23 <(printf "%s\n" "$want") "$__render_dir/covered")"
    if [[ -n "$missing" ]]; then
        echo "functions without a fixture:"
        printf "  %s\n" $missing
        exit 1
    fi
    echo "$(wc -l <<<"$want") functions covered"
'

while read -r __render_k __render_id; do
  [[ -n "${__render_k-}" ]] || continue
  it "$__render_id: R2-R6" bash -c "__render_check $__render_k"
done <<<"$__render_list"
unset __render_k __render_id __render_list

it "R7: headless loading prints nothing, on a terminal or not" bash -c '
    set -euo pipefail
    out="$(GASH_HEADLESS=1 bash --noprofile --norc -c "source \"$GASH_TEST_ROOT/gash.sh\"" 2>&1)"
    [[ -z "$out" ]] || { printf "%s\n" "$out" | od -c | head; exit 1; }
    cmd="GASH_HEADLESS=1 bash --noprofile --norc -c \"source $GASH_TEST_ROOT/gash.sh\""
    if [[ "$__render_pty" == util-linux ]]; then
        out="$(script -qec "$cmd" /dev/null </dev/null 2>&1)"
    else
        out="$(script -q /dev/null bash -c "$cmd" </dev/null 2>&1)"
    fi
    [[ -z "$out" ]] || { printf "%s\n" "$out" | od -c | head; exit 1; }
'
