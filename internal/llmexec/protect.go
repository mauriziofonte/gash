// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "github.com/mauriziofonte/gash/internal/pathguard"

// protectGit is the prelude part of the user's PROTECT: trees (restricted
// mode, git not named in GASH_LLM_EXEC_ALLOW); "" when there are none, so
// the script of a call without them is the one it always was.
//
// git runs inside the repository, where no path word names the files it
// prints, so its wrapper learns the repository root first, then:
//   - the trees inside it are excluded from what git prints of the
//     content (show, diff, grep, log -p: __LLM_CX_GITEX, which
//     __llm_cx_git applies), all of it when the repository lies inside a
//     tree;
//   - the PROTECT:all trees are excluded from the name listings too
//     (status, ls-files, log, whatchanged);
//   - a REV:PATH word (show, cat-file, grep) in a tree is refused:
//     forbidden_path for a PROTECT:all one, secret_file_blocked for a
//     PROTECT:content one.
func protectGit(g *pathguard.Guard) string {
	if len(g.Hidden) == 0 && len(g.Sealed) == 0 {
		return ""
	}
	return "__LLM_CX_PALL=(" + quoteArray(g.Hidden) + ")\n" +
		"__LLM_CX_PCON=(" + quoteArray(g.Sealed) + ")\n" +
		protectGitText
}

const protectGitText = `__llm_cx_pin() {
    local t; shift;
    for t in "$@"; do
        [[ -n "$t" ]] || continue;
        [[ "$__llm_cx_pp" == "$t" || "$__llm_cx_pp" == "$t"/* ]] && return 0;
    done;
    return 1
}
git() {
    local -a pre=() args=("$@") gx=("${__LLM_CX_GITEX[@]}") nx=();
    local top pfx sub w t p __llm_cx_pp;
    while ((${#args[@]})) && [[ "${args[0]}" == -* ]]; do
        pre+=("${args[0]}");
        if [[ "${args[0]}" == -C ]]; then pre+=("${args[1]-}"); args=("${args[@]:1}"); fi;
        args=("${args[@]:1}");
    done;
    sub="${args[0]-}";
    top=$(command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" rev-parse --show-toplevel 2>/dev/null);
    if [[ "$top" == /* ]]; then
        __llm_cx_pp="$top";
        if __llm_cx_pin x "${__LLM_CX_PALL[@]}" "${__LLM_CX_PCON[@]}"; then
            gx+=(':(exclude,top)*');
        fi;
        for t in "${__LLM_CX_PALL[@]}" "${__LLM_CX_PCON[@]}"; do
            [[ -n "$t" && "$t" == "$top"/* ]] && gx+=(":(exclude,top,literal)${t#"$top"/}");
        done;
        for t in "${__LLM_CX_PALL[@]}"; do
            [[ -n "$t" && "$t" == "$top"/* ]] && nx+=(":(exclude,top,literal)${t#"$top"/}");
        done;
        case "$sub" in
            show | cat-file | grep)
                pfx=$(command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" rev-parse --show-prefix 2>/dev/null);
                for w in "${args[@]:1}"; do
                    [[ "$w" == -- ]] && break;
                    [[ "$w" == -* || "$w" != *:* ]] && continue;
                    p="${w#*:}";
                    [[ "$p" == ./* ]] && p="$pfx${p#./}";
                    __llm_cx_pp="$top/$p";
                    __llm_cx_pp="${__llm_cx_pp%/}";
                    if __llm_cx_pin x "${__LLM_CX_PALL[@]}"; then
                        __gash_json_error "forbidden_path" "git $sub $w" "FATAL" "This path is protected and cannot be accessed";
                        return 1;
                    fi;
                    if __llm_cx_pin x "${__LLM_CX_PCON[@]}"; then
                        __gash_json_error "secret_file_blocked" "git $sub $w" "FATAL" "Secret files cannot be read through llm_exec";
                        return 1;
                    fi;
                done
            ;;
        esac;
    fi;
    if ((${#nx[@]})); then
        case "$sub" in
            status | ls-files | log | whatchanged)
                for w in "${args[@]:1}"; do
                    [[ "$w" == -- ]] && { args+=("${nx[@]}"); nx=(); break; };
                done;
                ((${#nx[@]})) && args+=(-- "${nx[@]}")
            ;;
        esac;
    fi;
    local -a __LLM_CX_GITEX=("${gx[@]}");
    __llm_cx_git "${pre[@]}" "${args[@]}"
}
`
