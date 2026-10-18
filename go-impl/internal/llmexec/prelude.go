// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// childHelpers are the child-side functions __llm_exec_prelude injects with
// `declare -f __llm_cx_run __llm_cx_grep __llm_cx_git __gash_json_error
// __gash_json_escape __gash_json_escape_to`: the secret-excluding grep, rg and
// git wrappers and the JSON error helpers the git wrapper prints with.
//
// The text is that command's output under bash 5.2, byte for byte, so the
// child script has the same lines as the 2.x one and a bash syntax error in
// the command line reports the same line number. Bytes that a Go raw string
// cannot hold or that editors tend to strip are written as placeholders and
// restored by helperBytes: <SP> a trailing space, <TAB>, <CR>, <BS>, <FF>,
// <SOH> (0x01) and <US> (0x1f).
var childHelpers = helperBytes.Replace(childHelpersText)

var helperBytes = strings.NewReplacer(
	"<SP>", " ", "<TAB>", "\t", "<CR>", "\r", "<BS>", "\b", "<FF>", "\f",
	"<SOH>", "\x01", "<US>", "\x1f",
)

const childHelpersText = `__llm_cx_run ()<SP>
{<SP>
    local cmd="$1" n="$2" w seen=0;
    shift 2;
    local -a add=("${@:1:n}") args=();
    shift "$n";
    for w in "$@";
    do
        if ((! seen)) && [[ "$w" == -- ]]; then
            args+=("${add[@]}");
            seen=1;
        fi;
        args+=("$w");
    done;
    ((seen)) || args+=("${add[@]}");
    command "$cmd" "${args[@]}"
}
__llm_cx_grep ()<SP>
{<SP>
    if [[ "${__LLM_CX_GREP_LAST-}" == 1 ]]; then
        __llm_cx_run grep "${#__LLM_CX_GREP[@]}" "${__LLM_CX_GREP[@]}" "$@";
    else
        command grep "${__LLM_CX_GREP[@]}" "$@";
    fi
}
__llm_cx_git ()<SP>
{<SP>
    local -a pre=() head=() opts=() ex=();
    while (($# > 0)) && [[ "$1" == -* ]]; do
        pre+=("$1");
        if [[ "$1" == -C ]]; then
            pre+=("${2-}");
            shift;
        fi;
        shift;
    done;
    if (($# == 0)); then
        command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}";
        return;
    fi;
    local sub="$1" w patch=0;
    shift;
    head=("$sub");
    for w in "$@";
    do
        case "$w" in<SP>
            -p | -u | --patch* | -U* | --unified* | --word-diff* | --color-words* | -W | --function-context | -c | --cc | --dd | -m | --remerge-diff* | --diff-merges* | --binary | --full-diff | -t)
                patch=1
            ;;
        esac;
    done;
    case "$sub" in<SP>
        show | diff | diff-tree | diff-index | diff-files)
            opts=(--no-ext-diff --no-textconv);
            ex=("${__LLM_CX_GITEX[@]}")
        ;;
        log | whatchanged)
            opts=(--no-ext-diff --no-textconv);
            ((patch)) && ex=("${__LLM_CX_GITEX[@]}")
        ;;
        reflog)
            if [[ "${1-}" == show ]]; then
                head+=(show);
                shift;
            fi;
            if [[ "${1-}" != exists ]]; then
                opts=(--no-ext-diff --no-textconv);
                ((patch)) && ex=("${__LLM_CX_GITEX[@]}");
            fi
        ;;
        grep)
            opts=(--no-textconv);
            ex=("${__LLM_CX_GITEX[@]}")
        ;;
        blame)
            opts=(--no-textconv)
        ;;
    esac;
    for w in "$@";
    do
        [[ "$w" == -L* || "$w" == --follow ]] && ex=();
    done;
    case "$sub" in<SP>
        show | diff | cat-file)
            for w in "$@";
            do
                [[ "$w" == -- ]] && break;
                [[ "$w" == -* || "$w" == *:* ]] && continue;
                [[ "$sub" == cat-file && " $* " == *" -"[tse]" "* ]] && break;
                if [[ "$(command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" cat-file -t "$w^{}" 2> /dev/null)" == blob ]]; then
                    __gash_json_error "option_not_allowed" "git $sub $w" "FATAL" "llm_exec prints a file from git only as REV:PATH (e.g. HEAD:src/app.php), so that the path can be checked";
                    return 1;
                fi;
            done
        ;;
    esac;
    command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" "${head[@]}" "${opts[@]}" "$@" "${ex[@]}"
}
__gash_json_error ()<SP>
{<SP>
    local type="${1:-unknown_error}" details="${2-}" action="${3:-STOP}" hint="${4-}";
    case "$action" in<SP>
        RETRY | CONTINUE | STOP | FATAL)

        ;;
        *)
            action="STOP"
        ;;
    esac;
    local recoverable=true;
    [[ "$action" == "FATAL" ]] && recoverable=false;
    local json;
    json="{\"error\":\"$(__gash_json_escape "$type")\"";
    [[ -n "$details" ]] && json+=",\"details\":\"$(__gash_json_escape "$details")\"";
    json+=",\"action\":\"$action\",\"recoverable\":$recoverable";
    [[ -n "$hint" ]] && json+=",\"hint\":\"$(__gash_json_escape "$hint")\"";
    printf '%s}\n' "$json" 1>&2;
    return 1
}
__gash_json_escape ()<SP>
{<SP>
    local __gash_esc_out;
    __gash_json_escape_to __gash_esc_out "${1-}";
    printf '%s' "$__gash_esc_out"
}
__gash_json_escape_to ()<SP>
{<SP>
    local str="${2-}";
    str="${str//\\/\\\\}";
    str="${str//\"/\\\"}";
    str="${str//'
'/\\n}";
    str="${str//'<CR>'/\\r}";
    str="${str//'<TAB>'/\\t}";
    str="${str//'<BS>'/\\b}";
    str="${str//'<FF>'/\\f}";
    if [[ "$str" == *['<SOH>'-'<US>']* ]]; then
        local LC_ALL=C out="" ch code i;
        for ((i = 0; i < ${#str}; i++))
        do
            ch="${str:i:1}";
            if [[ "$ch" == ['<SOH>'-'<US>'] ]]; then
                printf -v code '%d' "'$ch";
                printf -v ch '\\u%04x' "$((code & 255))";
            fi;
            out+="$ch";
        done;
        str="$out";
    fi;
    printf -v "$1" '%s' "$str"
}
`

// quoteArray renders a bash array body as 2.x prints it with
// "$(printf '%q ' "${arr[@]}")": each element in printf %q form followed by
// one space (the command substitution keeps it). An empty slice yields an
// empty quoted word and a space, because printf runs its format once with no
// arguments; the 2.x tables are never empty, so this only keeps the port
// exact.
func quoteArray(elems []string) string {
	if len(elems) == 0 {
		return "'' "
	}
	var b strings.Builder
	for _, e := range elems {
		b.WriteString(printfQ(e))
		b.WriteByte(' ')
	}
	return b.String()
}

// printfQ is bash's printf %q for printable ASCII (sh_backslash_quote with
// flags 3): a backslash before each shell-special byte, before a leading #,
// and before a ~ at the start or after : or =. The prelude tables hold only
// printable ASCII; any other byte falls back to single quoting, which bash
// reads back to the same value.
func printfQ(s string) string {
	if s == "" {
		return "''"
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 && s[i] != ' ' || s[i] > 0x7e {
			return singleQuote(s)
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case strings.IndexByte(" !\"$&'()*,;<>?[\\]^`{|}", c) >= 0:
			b.WriteByte('\\')
		case c == '#' && i == 0:
			b.WriteByte('\\')
		case c == '~' && (i == 0 || s[i-1] == ':' || s[i-1] == '='):
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// unsetNames is the `unset -f` argument of the prelude: the allowlist plus the
// extra commands, each followed by a space (as `printf '%s '` emits; the
// 2.x format adds one more before 2>/dev/null).
func unsetNames(extra []string) string {
	var b strings.Builder
	for _, c := range allowedCommands {
		b.WriteString(c)
		b.WriteByte(' ')
	}
	for _, c := range extra {
		b.WriteString(c)
		b.WriteByte(' ')
	}
	return b.String()
}

// preludeUnrestricted is __llm_exec_prelude in unrestricted mode: drop shadowing
// functions and CDPATH, nothing else.
func preludeUnrestricted(extra []string) string {
	var b strings.Builder
	b.WriteString("unset -f " + unsetNames(extra) + " 2>/dev/null\n")
	b.WriteString("unset CDPATH\n")
	return b.String()
}

// prelude is __llm_exec_prelude in restricted mode: the shadowing unset, the
// sandboxed text-tool wrappers, the child helper functions, the secret
// exclusion arrays and the grep/rg/diff/git wrappers that keep secret files
// out of what the child reads.
func prelude(extra []string, pr Probe) string {
	inExtra := map[string]bool{}
	for _, c := range extra {
		inExtra[c] = true
	}
	var b strings.Builder
	b.WriteString("unset -f " + unsetNames(extra) + " 2>/dev/null\n")
	b.WriteString("unset CDPATH\n")
	if !inExtra["sed"] {
		b.WriteString("sed() { command sed --sandbox \"$@\"; }\n")
	}
	if !inExtra["awk"] {
		b.WriteString("awk() { command gawk --sandbox \"$@\"; }\n")
	}
	if !inExtra["gawk"] {
		b.WriteString("gawk() { command gawk --sandbox \"$@\"; }\n")
	}

	// Secret exclusions.
	grepEx := pathguard.SecretExcludes("grep")
	for _, dir := range pathguard.SecretDirs {
		if strings.Contains(dir, "/") {
			base := dir[strings.LastIndexByte(dir, '/')+1:]
			grepEx = append(grepEx, "--exclude-dir="+pathguard.IcaseGlob(base))
		}
	}
	rgEx := pathguard.SecretExcludes("rg")

	b.WriteString(childHelpers)
	b.WriteString("__LLM_CX_GREP=(" + quoteArray(grepEx) + ")\n")
	b.WriteString("__LLM_CX_RG=(" + quoteArray(rgEx) + ")\n")
	if pr.IsGNUGrep() {
		b.WriteString("__LLM_CX_GREP_LAST=1\n")
	}
	dx := make([]string, 0, len(grepEx))
	for _, w := range grepEx {
		if i := strings.IndexByte(w, '='); i >= 0 {
			dx = append(dx, w[i+1:])
		} else {
			dx = append(dx, w)
		}
	}
	b.WriteString("__LLM_CX_DIFFX=(" + quoteArray(dx) + ")\n")
	if pr.HasDiffNoDeref() {
		b.WriteString("__LLM_CX_DIFF=(--no-dereference)\n")
	}
	if !inExtra["grep"] {
		b.WriteString("grep() { __llm_cx_grep \"$@\"; }\n")
	}
	if !inExtra["egrep"] {
		b.WriteString("egrep() { __llm_cx_grep -E \"$@\"; }\n")
	}
	if !inExtra["fgrep"] {
		b.WriteString("fgrep() { __llm_cx_grep -F \"$@\"; }\n")
	}
	if !inExtra["rg"] {
		b.WriteString("rg() { __llm_cx_run rg ${#__LLM_CX_RG[@]} \"${__LLM_CX_RG[@]}\" \"$@\"; }\n")
	}
	if !inExtra["diff"] {
		b.WriteString("diff() { command diff ${__LLM_CX_DIFF[@]+\"${__LLM_CX_DIFF[@]}\"} -X <(printf \"%s\\n\" \"${__LLM_CX_DIFFX[@]}\") \"$@\"; }\n")
	}
	if !inExtra["git"] {
		b.WriteString("__LLM_CX_GITCFG=(" + quoteArray(pathguard.GitSafeConfig) + ")\n")
		var gx []string
		for _, pat := range pathguard.SecretPatterns {
			gx = append(gx, ":(exclude,icase,glob)**/"+pat)
		}
		for _, dir := range pathguard.SecretDirs {
			gx = append(gx, ":(exclude,icase,glob)**/"+dir+"/**")
		}
		b.WriteString("__LLM_CX_GITEX=(" + quoteArray(gx) + ")\n")
		b.WriteString("git() { __llm_cx_git \"$@\"; }\n")
	}
	return b.String()
}
