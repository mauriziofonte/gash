#!/usr/bin/env bash
#
# Output style, static rules (S1-S12). Every human-facing output in lib/
# goes through lib/core/output.sh: semantic palette, glyphs, primitives.
# Each rule prints "file:line: text" for every hit and passes only at zero
# hits. The exception list is closed: it lives in __style_exempt below and
# must not grow.
#
# The scan is quote-aware: shell comments are dropped, string literals
# (single, double, $'...' and heredoc bodies, awk programs included) are
# kept. Rules that only make sense on literals (S8, S9) read the literal
# text alone.

describe "Output style: static rules"

__style_scan_file="$(mktemp "${TMPDIR:-/tmp}/gash-style-scan.XXXXXX")"
trap 'command rm -f "$__style_scan_file"; trap - EXIT' EXIT
export __style_scan_file

# One record per logical line: FILE TAB LINE TAB FUNC TAB CODE TAB STR TAB RAW.
# CODE is the line without its comment; STR the text of its string literals.
# A line continued with a backslash is joined with the next ones.
__style_scan_awk='
function flush() {
    if (have) {
        gsub(/\t/, " ", code); gsub(/\t/, " ", str); gsub(/\t/, " ", raw)
        printf "%s\t%d\t%s\t%s\t%s\t%s\n", FILENAME, start, func_name, code, str, raw
    }
    have = 0; code = ""; str = ""; raw = ""
}
FNR == 1 { flush(); state = 0; hd = ""; func_name = "" }
{
    line = $0
    if (hd != "") {
        probe = line
        if (hd_tabs) sub(/^\t+/, "", probe)
        if (probe == hd) { hd = ""; next }
        if (!have) { start = FNR; have = 1 }
        code = line; str = line; raw = line
        flush()
        next
    }
    if (state == 0 && !have) {
        if (line ~ /^[A-Za-z_][A-Za-z0-9_]*\(\)[ ]*\{/) {
            func_name = line; sub(/\(.*/, "", func_name)
        } else if (line ~ /^}/) {
            func_name = ""
        }
    }
    if (!have) { start = FNR; have = 1 }
    raw = (raw == "" ? line : raw " " line)
    n = length(line); i = 1; prev = " "; pend = ""
    while (i <= n) {
        c = substr(line, i, 1)
        if (state == 0) {
            if (c == "\\") { code = code substr(line, i, 2); prev = "x"; i += 2; continue }
            if (c == "#" && (prev == " " || prev == ";" || prev == "(" || prev == "|" || prev == "&")) break
            if (c == "<" && substr(line, i, 2) == "<<" && substr(line, i + 2, 1) != "<") {
                rest = substr(line, i + 2)
                tabs = 0
                if (substr(rest, 1, 1) == "-") { tabs = 1; rest = substr(rest, 2) }
                sub(/^[ ]*/, "", rest)
                if (match(rest, /^["\047]?[A-Za-z_][A-Za-z0-9_]*/)) {
                    pend = substr(rest, 1, RLENGTH); gsub(/["\047]/, "", pend); pend_tabs = tabs
                }
            }
            if (c == "\047") { state = (prev == "$") ? 3 : 1 }
            else if (c == "\"") { state = 2 }
            code = code c
        } else if (state == 1) {
            if (c == "\047") state = 0; else str = str c
            code = code c
        } else {
            if (c == "\\") { code = code substr(line, i, 2); str = str substr(line, i, 2); prev = "x"; i += 2; continue }
            if ((state == 2 && c == "\"") || (state == 3 && c == "\047")) state = 0; else str = str c
            code = code c
        }
        prev = c
        i++
    }
    if (state != 0) { str = str " " }
    if (state == 0 && substr(code, length(code), 1) == "\\" && pend == "") {
        code = substr(code, 1, length(code) - 1) " "
        next
    }
    flush()
    if (pend != "") { hd = pend; hd_tabs = pend_tabs }
}
END { flush() }
'

(
  cd "$GASH_TEST_ROOT" || exit 1
  files=(gash.sh)
  while IFS= read -r f; do files+=("$f"); done < <(find lib -type f -name '*.sh' | LC_ALL=C sort)
  awk "$__style_scan_awk" "${files[@]}" >"$__style_scan_file"
)

# Rule patterns (ERE, read by awk through ENVIRON, so no escape processing).
export __style_re_s1='(^|[^A-Za-z0-9_-])echo[ ]+-[A-Za-z]*e'
export __style_re_s2='printf[ ]+(-v[ ]+[A-Za-z_][A-Za-z0-9_]*[ ]+)?["'"'"'][^"'"'"']*%b'
export __style_re_s3='\\(033|x1[bB]|u001[bB]|[eE])(\[|\])|\$'"'"'\\(e|E|033|x1[bB])|%c",[ ]*27'
export __style_re_s4='__GASH_(BOLD_)?(RED|GREEN|YELLOW|BLUE|CYAN|WHITE)([^A-Z_]|$)'
export __style_re_s5='__gash_print[ ]+["'"'"']?(bold_)?(red|green|yellow|blue|cyan|white)([^a-z_]|$)'
export __style_re_s6='\$\{__GASH_(COLOR_[A-Z]+|BOLD[A-Z_]*|DIM|UNDERLINE|RED|GREEN|YELLOW|BLUE|CYAN|WHITE|GLYPH_[A-Z_]+|SYMBOL_[A-Z]+):?-'
export __style_re_s7='%-?([0-9]+|\*)(\.[0-9]+)?s'
export __style_re_s8='(^|[^A-Za-z0-9_$])\[(!|\+|i|OK|x|X)\]'
export __style_re_s9a='printf[ ]+(-v[ ]+[A-Za-z_][A-Za-z0-9_]*[ ]+)?["'"'"']%\.0s'
export __style_re_s9b='-----|=====|─────'
export __style_re_s10='(^|[^A-Za-z0-9_])(echo|printf)([^A-Za-z0-9_].*)?(>&2|>[ ]*/dev/stderr)'
export __style_re_s11='(\[|test)[ ].*-t[ ]+[12]([^0-9]|$)'
export __style_re_s12='(^|[^A-Za-z0-9_])read[ ]([^;|&]*[ ])?-[A-Za-z]*p'

# The closed exception list (brief section 4.3, plus the Bash-version
# message of gash.sh for S10, approved 2026-10-15). Do not extend.
export __style_exempt_output="lib/core/output.sh"
export __style_exempt_core="lib/core/"
export __style_exempt_s10="lib/core/ gash.sh@__gash_require_bash_message"
export __style_exempt_s3="lib/core/output.sh lib/shell/prompt.sh lib/aliases/colors.sh lib/core/utils.sh@all_colors"

# Run a rule over the scan. FIELD is code or str, ERE the pattern, EXEMPT a
# space-separated list of path prefixes (a "path@function" entry exempts one
# function of that file). Prints "file:line: text" per hit, fails on any.
__style_rule() {
  local field="$1" ere="$2" exempt="${3-}" col=4 hits
  [[ "$field" == str ]] && col=5
  hits="$(__RE="$ere" __EXEMPT="$exempt" awk -F '\t' -v col="$col" '
        BEGIN { re = ENVIRON["__RE"]; n = split(ENVIRON["__EXEMPT"], ex, " ") }
        $col ~ re {
            for (k = 1; k <= n; k++) {
                e = ex[k]; fn = ""
                if (index(e, "@")) { fn = substr(e, index(e, "@") + 1); e = substr(e, 1, index(e, "@") - 1) }
                if (substr($1, 1, length(e)) == e && (fn == "" || fn == $3)) next
            }
            raw = $6; sub(/^[ ]+/, "", raw)
            print $1 ":" $2 ": " raw
        }' "$__style_scan_file")"
  if [[ -n "$hits" ]]; then
    printf '%s\n' "$hits"
    printf '%d hit(s)\n' "$(printf '%s\n' "$hits" | wc -l)"
    return 1
  fi
  return 0
}
export -f __style_rule

it "the scan sees every shell file of lib/ and gash.sh" bash -c '
    set -euo pipefail
    cd "$GASH_TEST_ROOT"
    want="$( (echo gash.sh; find lib -type f -name "*.sh") | LC_ALL=C sort -u)"
    got="$(cut -f1 "$__style_scan_file" | LC_ALL=C sort -u)"
    [[ "$want" == "$got" ]] || { diff <(echo "$want") <(echo "$got"); exit 1; }
    # Comments are dropped, literals kept.
    awk -F "\t" "\$1 == \"lib/core/output.sh\" && \$4 ~ /COLOR POLICY/ { bad = 1 } END { exit bad }" "$__style_scan_file"
    awk -F "\t" "\$1 == \"lib/core/output.sh\" && \$5 ~ /Info:/ { ok = 1 } END { exit !ok }" "$__style_scan_file"
'

it "S1: no echo -e, -ne or -en" bash -c '__style_rule code "$__style_re_s1"'
it "S2: no printf %b outside lib/core/output.sh" bash -c '__style_rule code "$__style_re_s2" "$__style_exempt_output"'
it "S3: no literal escape sequences" bash -c '__style_rule code "$__style_re_s3" "$__style_exempt_s3"'
it "S4: no hue palette names outside lib/core/output.sh" bash -c '__style_rule code "$__style_re_s4" "$__style_exempt_output"'
it "S5: no __gash_print with a hue name" bash -c '__style_rule code "$__style_re_s5"'
it "S6: no private fallback palettes" bash -c '__style_rule code "$__style_re_s6"'
it "S7: no width-padded %Ns formats outside lib/core" bash -c '__style_rule code "$__style_re_s7" "$__style_exempt_core"'
it "S8: no ASCII status markers in string literals" bash -c '__style_rule str "$__style_re_s8"'
it "S9: no hand-drawn rules" bash -c '
    rc=0
    __style_rule code "$__style_re_s9a" "$__style_exempt_output" || rc=1
    __style_rule str "$__style_re_s9b" "$__style_exempt_output" || rc=1
    exit "$rc"
'
it "S10: no echo or printf to stderr outside lib/core" bash -c '__style_rule code "$__style_re_s10" "$__style_exempt_s10"'
it "S11: no private TTY tests outside lib/core" bash -c '__style_rule code "$__style_re_s11" "$__style_exempt_core"'
it "S12: no read -p prompts outside lib/core" bash -c '__style_rule code "$__style_re_s12" "$__style_exempt_core"'
