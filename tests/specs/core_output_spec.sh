#!/usr/bin/env bash

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

describe "Core Output"

it "defines color variables" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    [[ -n "$__GASH_COLOR_OFF" ]]
    [[ -n "$__GASH_RED" ]]
    [[ -n "$__GASH_GREEN" ]]
    [[ -n "$__GASH_CYAN" ]]
    [[ -n "$__GASH_BOLD_WHITE" ]]
'

it "__gash_info prints cyan Info prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_info "test message" 2>&1)"
    [[ "$out" == *"Info:"* ]]
    [[ "$out" == *"test message"* ]]
'

it "__gash_error prints red Error prefix to stderr" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_error "error message" 2>&1)"
    [[ "$out" == *"Error:"* ]]
    [[ "$out" == *"error message"* ]]
'

it "__gash_success prints green OK prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_success "done" 2>&1)"
    [[ "$out" == *"OK:"* ]]
    [[ "$out" == *"done"* ]]
'

it "__gash_warning prints yellow Warning prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_warning "careful" 2>&1)"
    [[ "$out" == *"Warning:"* ]]
    [[ "$out" == *"careful"* ]]
'

it "__gash_ssh prints SSH prefix with color variants" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out1="$(__gash_ssh "connected" 2>&1)"
    out2="$(__gash_ssh "key added" success 2>&1)"
    out3="$(__gash_ssh "warning msg" warning 2>&1)"
    out4="$(__gash_ssh "failed" error 2>&1)"

    [[ "$out1" == *"SSH:"* ]]
    [[ "$out2" == *"SSH:"* ]]
    [[ "$out3" == *"SSH:"* ]]
    [[ "$out4" == *"SSH:"* ]]
'

it "__gash_step prints step indicator" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_step 1 3 "Installing..." 2>&1)"
    [[ "$out" == *"[1/3]"* ]]
    [[ "$out" == *"Installing..."* ]]
'

it "__gash_debug prints only when GASH_DEBUG=1" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    # Without GASH_DEBUG, should print nothing
    out1="$(__gash_debug "debug msg" 2>&1)"
    [[ -z "$out1" ]]

    # With GASH_DEBUG=1, should print
    export GASH_DEBUG=1
    out2="$(__gash_debug "debug msg" 2>&1)"
    [[ "$out2" == *"Debug:"* ]]
    [[ "$out2" == *"debug msg"* ]]
'

it "__gash_print outputs colored text without prefix" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    out="$(__gash_print green "green text")"
    [[ "$out" == *"green text"* ]]
'

it "all output functions handle empty messages" bash -c '
    set -euo pipefail
    ROOT="${GASH_TEST_ROOT}"
    source "$ROOT/lib/core/output.sh"

    # Should not fail with empty args
    __gash_info "" >/dev/null 2>&1
    __gash_error "" 2>/dev/null
    __gash_success "" >/dev/null 2>&1
    __gash_warning "" >/dev/null 2>&1
    __gash_ssh "" >/dev/null 2>&1
    __gash_step "" "" "" >/dev/null 2>&1
'

describe "Core Output - primitives"

it "__gash_color_of maps semantic names to the palette" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    __gash_color_of v accent && [[ "$v" == "$__GASH_COLOR_ACCENT" ]]
    __gash_color_of v muted && [[ "$v" == "$__GASH_COLOR_MUTED" ]]
    __gash_color_of v bold && [[ "$v" == "$__GASH_BOLD" ]]
    __gash_color_of v "" && [[ -z "$v" ]]
    __gash_color_of v none && [[ -z "$v" ]]
    __gash_color_of v "$__GASH_COLOR_ERROR" && [[ "$v" == "$__GASH_COLOR_ERROR" ]]
'

it "__gash_table: an all-empty column is a spacer, also under set -u" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(printf "a\t1\t\tb\t2\nccc\t3\n" | __gash_table --no-header)"
    [[ "$out" == "$(printf "a    1    b  2\nccc  3")" ]]
'

it "__gash_table_into: lines into an array, styled by the caller's color decision" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    __gash_table_into lines --no-header --color 1:accent < <(printf "a\t1\nbb\t2\n")
    [[ "${#lines[@]}" -eq 2 && "${lines[0]}" == "a   1" && "${lines[1]}" == "bb  2" ]]
    # Colors on in this shell only, off in any subshell (as a capture is).
    __gash_use_color() { [[ "$BASHPID" == "$$" ]]; }
    [[ "$(printf "a\t1\n" | __gash_table --no-header --color 1:accent)" == "a  1" ]]
    __gash_table_into lines --no-header --color 1:accent < <(printf "a\t1\n")
    [[ "${lines[0]}" == "${__GASH_COLOR_ACCENT}a${__GASH_COLOR_OFF}  1" ]]
'

it "__gash_table: --gap and --min reproduce a fixed-width layout" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/utils.sh"; source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(printf "a\tone\nlonger\ttwo\n" | __gash_table --no-header --indent 2 --gap 1 --min 1:15)"
    want="$(printf "  %-15s %s\n" a one longer two)"
    [[ "$out" == "$want" ]]
'

it "__gash_table: --cell-color styles a column per row and hides the style column" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/utils.sh"; source "$GASH_TEST_ROOT/lib/core/output.sh"
    rows="$(printf "N\tS\tST\nok\t1\tsuccess\nbad\t2\terror\n")"
    plain="$(__gash_table --cell-color 2:3 <<<"$rows")"
    [[ "$plain" != *success* && "$plain" != *error* && "$plain" != *ST* ]]
    __gash_use_color() { return 0; }
    out="$(__gash_table --cell-color 2:3 <<<"$rows")"
    [[ "$out" == *"${__GASH_COLOR_SUCCESS}1${__GASH_COLOR_OFF}"* ]]
    [[ "$out" == *"${__GASH_COLOR_ERROR}2${__GASH_COLOR_OFF}"* ]]
    [[ "$out" != *success* ]]
'

it "__gash_table: --bar draws VALUE/MAX, full part in the cell style, empty part muted" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/utils.sh"; source "$GASH_TEST_ROOT/lib/core/output.sh"
    rows="$(printf "NAME\tUSE\tPCT\nroot\t50/100\t50%%\n")"
    out="$(__gash_table --bar 2:10 <<<"$rows")"
    [[ "$(sed -n 3p <<<"$out")" == "root  ${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}  50%" ]]
    __gash_use_color() { return 0; }
    out="$(__gash_table --bar 2:4 --color 2:warning <<<"$rows")"
    [[ "$out" == *"${__GASH_COLOR_WARNING}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_COLOR_OFF}${__GASH_COLOR_MUTED}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}${__GASH_COLOR_OFF}"* ]]
'

it "__gash_table --bar: a cell that is not VALUE/MAX prints as text (a header row given as data)" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(printf "NAME\tUSE\na\t5/10\n" | __gash_table --no-header --bar 2:4)"
    [[ "$(head -n 1 <<<"$out")" == "NAME  USE" ]]
    [[ "$(tail -n 1 <<<"$out")" == "a     ${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_FULL}${__GASH_GLYPH_BAR_EMPTY}${__GASH_GLYPH_BAR_EMPTY}" ]]
'

it "__gash_table: --max cuts the end, --max-left keeps it" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/utils.sh"; source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(printf "abcdefghij\t/dev/mapper/vg-root\tx\n" | __gash_table --no-header --max 1:5 --max-left 2:8)"
    [[ "$out" == "abcd${__GASH_GLYPH_ELLIPSIS}  ${__GASH_GLYPH_ELLIPSIS}vg-root  x" ]]
'

it "__gash_table: --fit N shrinks a middle column to the terminal width" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/utils.sh"; source "$GASH_TEST_ROOT/lib/core/output.sh"
    long="$(printf "%050d" 0)"
    out="$(COLUMNS=40 __gash_table --no-header --fit 2 <<<"$(printf "a\t%s\tEND\n" "$long")")"
    [[ "${#out}" -le 40 ]]
    [[ "$out" == *"${__GASH_GLYPH_ELLIPSIS}  END" ]]
'

it "__gash_kv: padded bold keys, optional value style, separator and width" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(__gash_kv --indent 2 --width 20 php_command php8.3 composer_command composer)"
    [[ "$out" == "$(printf "  %-20s %s\n" php_command php8.3 composer_command composer)" ]]
    out="$(__gash_kv "Public IP:" 1.2.3.4)"
    [[ "$out" == "Public IP: 1.2.3.4" ]]
    out="$(__gash_kv --indent 4 --width 0 --sep = GASH_PROMPT 1)"
    [[ "$out" == "    GASH_PROMPT=1" ]]
    __gash_use_color() { return 0; }
    out="$(__gash_kv --value-color accent "IP:" 1.2.3.4)"
    [[ "$out" == "${__GASH_BOLD}IP:${__GASH_COLOR_OFF} ${__GASH_COLOR_ACCENT}1.2.3.4${__GASH_COLOR_OFF}" ]]
'

it "__gash_heading: bold title on a color terminal, plain text otherwise" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_heading --indent 2 "USAGE")" == "  USAGE" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_heading "USAGE")" == "${__GASH_BOLD}USAGE${__GASH_COLOR_OFF}" ]]
'

it "__gash_tty_io: true only when stdin and stdout are terminals" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    ! __gash_tty_io </dev/null
    if script -qec true /dev/null >/dev/null 2>&1; then
        out="$(script -qec "bash -c \"source $GASH_TEST_ROOT/lib/core/output.sh; __gash_tty_io && echo yes\"" /dev/null </dev/null)"
        [[ "$out" == yes* ]]
    fi
'

it "__gash_heading --note: secondary text after the title, muted in color" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_heading --indent 2 --note "(module: git)" "GIT")" == "  GIT  (module: git)" ]]
    [[ "$(__gash_heading --note "" "GIT")" == "GIT" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_heading --note "x" "T")" == "${__GASH_BOLD}T${__GASH_COLOR_OFF}  ${__GASH_COLOR_MUTED}x${__GASH_COLOR_OFF}" ]]
'

it "__gash_usage, __gash_note and __gash_debug keep their plain prefixes and get a glyph in color" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_usage "hgrep PATTERN" 2>&1 >/dev/null)" == "Usage: hgrep PATTERN" ]]
    [[ -z "$(__gash_usage "x" 2>/dev/null)" ]]
    [[ "$(__gash_note "Get: http://x" 2>&1)" == "Get: http://x" ]]
    [[ "$(GASH_DEBUG=1 __gash_debug "d" 2>&1)" == "Debug: d" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_usage "x" 2>&1)" == "${__GASH_COLOR_INFO}${__GASH_GLYPH_INFO} Usage:${__GASH_COLOR_OFF} x" ]]
    [[ "$(__gash_note "n" 2>&1)" == "${__GASH_COLOR_INFO}${__GASH_GLYPH_INFO}${__GASH_COLOR_OFF} n" ]]
    [[ "$(__gash_ssh "k" success 2>&1)" == "${__GASH_COLOR_SUCCESS}${__GASH_GLYPH_OK}${__GASH_COLOR_OFF} ${__GASH_COLOR_PRIMARY}SSH:${__GASH_COLOR_OFF} k" ]]
'

it "__gash_ask_render: QUESTION [ANSWERS] on stderr, answers bold in color, question verbatim" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_ask_render "Delete C:\\new\\c?" "y/N" 2>&1)" == "Delete C:\\new\\c? [y/N] " ]]
    [[ -z "$(__gash_ask_render "q" "y/N" 2>/dev/null)" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_ask_render "q" "Y/n" 2>&1)" == "q ${__GASH_BOLD}[Y/n]${__GASH_COLOR_OFF} " ]]
'

it "__gash_read_line reads a line and prompts only on a terminal" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$( { __gash_read_line ans "Question: " <<<"  hello  "; printf "[%s]" "$ans"; } 2>&1)"
    [[ "$out" == "[hello]" ]]
'

it "__gash_clear_screen prints nothing when stdout is not a terminal, with NO_COLOR or headless" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ -z "$(__gash_clear_screen)" ]]
    __gash_use_color() { return 1; }
    [[ -z "$(__gash_clear_screen)" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_clear_screen)" == "$__GASH_CLEAR_SCREEN" ]]
    [[ -z "$(GASH_HEADLESS=1 __gash_clear_screen)" ]]
    [[ "$__GASH_CLEAR_SCREEN" == $'"'"'\e[H\e[2J'"'"' ]]
'

it "__gash_color_chart: 256 names, colored only on a color terminal; __gash_sgr_extra lists its swatches" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    out="$(__gash_color_chart)"
    [[ "$(wc -l <<<"$out")" -eq 256 && "$out" != *$'"'"'\e'"'"'* ]]
    [[ "$(sed -n 1p <<<"$out")" == colour0 ]]
    [[ "$(__gash_sgr_extra | wc -l)" -eq 256 ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_color_chart | sed -n 2p)" == $'"'"'\e[38;5;1m'"'"'colour1"$__GASH_COLOR_OFF" ]]
'

it "spinner and progress print no escape sequence when stdout is not a terminal" bash -c '
    set -euo pipefail
    command -v script >/dev/null || exit 0
    cmd="source $GASH_TEST_ROOT/lib/core/output.sh; __gash_progress 1 2 half | cat; __gash_progress 2 2 done | cat"
    if script -qec true /dev/null >/dev/null 2>&1; then
        out="$(script -qec "bash -c \"$cmd\"" /dev/null </dev/null)"
    else
        out="$(script -q /dev/null bash -c "$cmd" </dev/null)"
    fi
    [[ "$out" == *"half"* && "$out" == *"100%"* ]]
    [[ "$out" != *$'"'"'\e'"'"'* ]]
'

it "__gash_list: bulleted items, bullet colored on a color terminal" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_list "/etc/php/" "a b")" == "  ${__GASH_GLYPH_BULLET} /etc/php/"$'"'"'\n'"'"'"  ${__GASH_GLYPH_BULLET} a b" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_list --indent 4 x)" == "    ${__GASH_COLOR_PRIMARY}${__GASH_GLYPH_BULLET}${__GASH_COLOR_OFF} x" ]]
'

it "__gash_mark: glyph in the status color on a terminal, word prefix otherwise" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_mark --indent 2 warning "disk 91%")" == "  Warning: disk 91%" ]]
    [[ "$(__gash_mark ok "fine")" == "OK: fine" ]]
    [[ "$(__gash_mark error "down")" == "Error: down" ]]
    [[ "$(__gash_mark info "note")" == "Info: note" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_mark error "down")" == "${__GASH_COLOR_ERROR}${__GASH_GLYPH_ERR}${__GASH_COLOR_OFF} down" ]]
'

it "__gash_hint: muted Hint line on stderr" bash -c '
    set -euo pipefail
    source "$GASH_TEST_ROOT/lib/core/output.sh"
    [[ "$(__gash_hint "do x" 2>&1 >/dev/null)" == "Hint: do x" ]]
    __gash_use_color() { return 0; }
    [[ "$(__gash_hint "do x" 2>&1)" == "${__GASH_COLOR_MUTED}Hint: do x${__GASH_COLOR_OFF}" ]]
'
