#!/usr/bin/env bash

# Gash Module: LLM Utilities: llm_exec
# Read-only command execution for agents.
# Sourced by lib/modules/llm.sh.
#
# llm_exec runs a command line in a child bash, never in the caller's shell,
# after three checks:
#
#   1. The dangerous-pattern blocklist (__llm_validate_command).
#   2. A quote-aware parse of the line: plain words joined by |, &&, || and
#      ";" only. No expansions ($, backticks), no subshells or braces, no
#      redirections except to /dev/null and between stdout/stderr, no
#      background jobs, no VAR=value prefixes.
#   3. Every command must be on a read-only allowlist, with per-command
#      rules for the ones that also have writing options (find -delete,
#      sort -o, git branch -D, ...). Unquoted globs are expanded up front
#      and refused when a match looks like an option or a secret file.
#      Recursive commands may not follow symlinks or start at / or above
#      a protected tree (__llm_exec_check_walk).
#
# The child starts from a short allowlisted environment (__llm_exec_env),
# and its grep, rg, diff and git leave secret files out of what they read
# (__llm_exec_prelude and the __llm_cx_* helpers).
#
# Settings, read from ~/.gash_env only (never from the environment, so an
# agent cannot loosen its own rules):
#   GASH_LLM_EXEC_ALLOW="cmd1 cmd2"  extra commands, any arguments
#   GASH_LLM_EXEC=unrestricted       skip checks 2 and 3
#   GASH_LLM_SANDBOX=1               run inside bwrap: read-only filesystem,
#                                    no network

# Commands allowed with any arguments, unless __llm_exec_check_args has a
# rule for them.
__LLM_EXEC_ALLOWED=" ls cat head tail wc sort uniq cut tr paste join comm column nl tac rev fold expand unexpand grep egrep fgrep zcat zgrep bzcat xzcat rg fd find tree stat file du df free uptime uname whoami id groups w who last hostname date pwd cd echo printf true false test which type basename dirname realpath readlink diff cmp md5sum sha1sum sha256sum sha512sum base64 strings od hexdump jq yq ps pgrep pidof lsof ss netstat ip nproc lscpu lsblk findmnt getent locale crontab git sed awk gawk systemctl journalctl docker php node python python3 ruby java go npm pip pip3 composer "

# -----------------------------------------------------------------------------
# Command checks
# -----------------------------------------------------------------------------

# Probe once whether a sandboxed text tool exists (GNU sed --sandbox,
# gawk --sandbox).
__llm_exec_has_sandboxed() {
    local var="__LLM_EXEC_HAS_${1^^}"
    if [[ -z "${!var-}" ]]; then
        case "$1" in
            sed) printf 'x\n' | command sed --sandbox -n p >/dev/null 2>&1 && printf -v "$var" 1 || printf -v "$var" 0 ;;
            gawk) type -P gawk >/dev/null 2>&1 && gawk --sandbox 'BEGIN{}' >/dev/null 2>&1 && printf -v "$var" 1 || printf -v "$var" 0 ;;
        esac
    fi
    [[ "${!var}" == 1 ]]
}

# Refuse a word: the option is not allowed for this command.
__llm_exec_refuse_option() {
    __llm_error "option_not_allowed" "$1 $2" "FATAL" "false" "llm_exec runs read-only commands; '$2' can write, delete, run programs or read past the secret-file and path checks"
    return 1
}

# Positional (non-option) words of a command, skipping the command name.
# Usage: __llm_exec_positionals OUT WORD...
__llm_exec_positionals() {
    local -n __lp_out="$1"
    shift 2
    __lp_out=()
    local w
    for w in "$@"; do
        [[ "$w" == -* ]] || __lp_out+=("$w")
    done
}

# Per-command argument rules. Words are already unquoted.
# Usage: __llm_exec_check_args NAME WORD...
__llm_exec_check_args() {
    local name="$1" w
    shift
    local -a pos=()

    case "$name" in
        find)
            for w in "$@"; do
                case "$w" in
                    -exec | -execdir | -ok | -okdir | -delete | -fprint | -fprint0 | -fprintf | -fls)
                        __llm_exec_refuse_option find "$w"
                        return 1
                        ;;
                esac
            done
            ;;
        fd)
            for w in "$@"; do
                case "$w" in
                    -x | -X | --exec | --exec=* | --exec-batch | --exec-batch=*)
                        __llm_exec_refuse_option fd "$w"
                        return 1
                        ;;
                esac
            done
            ;;
        rg)
            for w in "$@"; do
                [[ "$w" == --pre || "$w" == --pre=* ]] && {
                    __llm_exec_refuse_option rg "$w"
                    return 1
                }
            done
            ;;
        sort | tree)
            for w in "$@"; do
                if [[ "$w" == --output* || "$w" == --compress-program* || "$w" =~ ^-[^-]*o ]]; then
                    __llm_exec_refuse_option "$name" "$w"
                    return 1
                fi
            done
            ;;
        sed | yq)
            for w in "$@"; do
                if [[ "$w" == --in-place* || "$w" == --inplace* || "$w" =~ ^-[^-]*i ]]; then
                    __llm_exec_refuse_option "$name" "$w"
                    return 1
                fi
                # yq (mikefarah): -s/--split-exp writes files, load*() reads
                # any file from inside the expression, unchecked.
                if [[ "$name" == yq ]] && [[ "$w" == --split-exp* || "$w" =~ ^-[^-]*s ]]; then
                    __llm_exec_refuse_option yq "$w"
                    return 1
                fi
                if [[ "$name" == yq && "$w" =~ load(_[A-Za-z0-9]+)?[[:space:]]*\( ]]; then
                    __llm_error "option_not_allowed" "yq $w" "FATAL" "false" "yq load functions read files the path checks do not see; pass the files as arguments"
                    return 1
                fi
            done
            if [[ "$name" == sed ]] && ! __llm_exec_has_sandboxed sed; then
                __llm_error "command_not_allowed" "sed" "STOP" "true" "llm_exec allows sed only where GNU sed --sandbox exists; use grep, head or llm_grep"
                return 1
            fi
            ;;
        awk | gawk)
            if ! __llm_exec_has_sandboxed gawk; then
                __llm_error "command_not_allowed" "$name" "STOP" "true" "llm_exec allows awk only where gawk --sandbox exists; install gawk or use grep, cut or jq"
                return 1
            fi
            ;;
        file)
            for w in "$@"; do
                if [[ "$w" == --compile || "$w" =~ ^-[^-]*C ]]; then
                    __llm_exec_refuse_option file "$w"
                    return 1
                fi
            done
            ;;
        uniq)
            # uniq INPUT OUTPUT writes OUTPUT.
            local prev=""
            for w in "$@"; do
                [[ "$w" != -* && "$prev" != -[fsw] ]] && pos+=("$w")
                prev="$w"
            done
            ((${#pos[@]} <= 1)) || {
                __llm_exec_refuse_option uniq "${pos[1]}"
                return 1
            }
            ;;
        hostname)
            for w in "$@"; do
                if [[ "$w" != -* || "$w" == -F || "$w" == --file* || "$w" == -b || "$w" == --boot ]]; then
                    __llm_exec_refuse_option hostname "$w"
                    return 1
                fi
            done
            ;;
        date)
            for w in "$@"; do
                if [[ "$w" == --set* || "$w" == -s* ]]; then
                    __llm_exec_refuse_option date "$w"
                    return 1
                fi
            done
            ;;
        crontab)
            [[ $# -eq 1 && "$1" == -l ]] || {
                __llm_exec_refuse_option crontab "$*"
                return 1
            }
            ;;
        ps)
            # BSD-style "e" (ps e, ps auxe) prints every process's
            # environment: the tokens of the agent's own parent included.
            local prev=""
            for w in "$@"; do
                if [[ "$w" =~ ^[A-Za-z]+$ && "$w" == *e* && ! "$prev" =~ ^(-[oOpqugtCGUsk]|--(pid|ppid|sid|user|User|group|Group|tty|format|sort|cols|columns|rows|width))$ ]]; then
                    __llm_error "option_not_allowed" "ps $w" "FATAL" "false" "ps e prints the environment of every process; list processes without it (ps aux, ps -ef)"
                    return 1
                fi
                prev="$w"
            done
            ;;
        getent)
            __llm_exec_positionals pos "$name" "$@"
            if [[ "${pos[0]-}" == shadow || "${pos[0]-}" == gshadow ]]; then
                __gash_json_error "forbidden_path" "getent ${pos[0]}" "FATAL" "The shadow databases (/etc/shadow, /etc/gshadow) are protected"
                return 1
            fi
            ;;
        php | node | python | python3 | ruby | java | composer)
            [[ $# -eq 1 && "$1" =~ ^(--version|-v|-V|version)$ ]] || {
                __llm_error "option_not_allowed" "$name $*" "FATAL" "false" "llm_exec runs $name only to print its version (--version)"
                return 1
            }
            ;;
        go | npm | pip | pip3)
            local -A ok=([go]=" version env " [npm]=" --version -v ls list " [pip]=" --version -V list show freeze " [pip3]=" --version -V list show freeze ")
            [[ $# -ge 1 && "${ok[$name]}" == *" $1 "* ]] || {
                __llm_error "option_not_allowed" "$name $*" "FATAL" "false" "llm_exec allows only: $name${ok[$name]}"
                return 1
            }
            ;;
        ip)
            __llm_exec_positionals pos "$name" "$@"
            if [[ ! "${pos[0]-}" =~ ^(a|addr|address|r|route|l|link|n|neigh|neighbour|rule|maddr)$ ]] ||
                [[ -n "${pos[1]-}" && ! "${pos[1]}" =~ ^(show|list|ls|get)$ ]]; then
                __llm_exec_refuse_option ip "${pos[*]}"
                return 1
            fi
            ;;
        systemctl)
            __llm_exec_positionals pos "$name" "$@"
            if [[ -n "${pos[0]-}" && ! "${pos[0]}" =~ ^(status|show|cat|is-active|is-enabled|is-failed|list-units|list-unit-files|list-timers|list-sockets|list-dependencies)$ ]]; then
                __llm_exec_refuse_option systemctl "${pos[0]}"
                return 1
            fi
            ;;
        journalctl)
            for w in "$@"; do
                case "$w" in
                    --vacuum* | --rotate | --flush | --sync | --relinquish-var | --smart-relinquish-var | --setup-keys | --update-catalog)
                        __llm_exec_refuse_option journalctl "$w"
                        return 1
                        ;;
                esac
            done
            ;;
        docker)
            local sub="${1-}" sub2="${2-}"
            if [[ "$sub" =~ ^(image|container|volume|network|system|context|compose)$ ]]; then
                [[ "$sub2" =~ ^(ls|list|ps|inspect|history|logs|top|port|diff|df|info|show|config|images|version)$ ]] || {
                    __llm_exec_refuse_option "docker $sub" "$sub2"
                    return 1
                }
            elif [[ ! "$sub" =~ ^(ps|images|logs|inspect|version|info|stats|top|port|diff|history)$ ]]; then
                __llm_exec_refuse_option docker "$sub"
                return 1
            fi
            ;;
        git)
            __llm_exec_check_git "$@" || return 1
            ;;
        diff)
            # A recursive diff follows symlinks unless --no-dereference.
            for w in "$@"; do
                [[ "$w" == -- ]] && break
                if [[ "$w" == --recursive || "$w" =~ ^-[^-]*r ]] && ! __llm_exec_diff_noderef; then
                    __llm_error "option_not_allowed" "diff $w" "FATAL" "false" "Recursive diff needs GNU diff (--no-dereference) here: it would follow symlinks out of the tree; compare single files"
                    return 1
                fi
            done
            ;;
    esac
    return 0
}

# Probe once whether diff has --no-dereference (GNU diff 3.3+).
__llm_exec_diff_noderef() {
    if [[ -z "${__LLM_EXEC_DIFF_NODEREF-}" ]]; then
        if command diff --no-dereference /dev/null /dev/null >/dev/null 2>&1; then
            __LLM_EXEC_DIFF_NODEREF=1
        else
            __LLM_EXEC_DIFF_NODEREF=0
        fi
    fi
    [[ "$__LLM_EXEC_DIFF_NODEREF" == 1 ]]
}

# Probe once whether grep is GNU grep, which reads options after operands
# (the secret exclusions can then go last, where they win over a caller's
# --include). Other greps get them first.
__llm_exec_gnu_grep() {
    if [[ -z "${__LLM_EXEC_GNU_GREP-}" ]]; then
        if { command grep --version | command grep -q GNU; } 2>/dev/null; then
            __LLM_EXEC_GNU_GREP=1
        else
            __LLM_EXEC_GNU_GREP=0
        fi
    fi
    [[ "$__LLM_EXEC_GNU_GREP" == 1 ]]
}

# Commands that walk a directory tree (grep -r, rg, find, fd, tree, du,
# ls -R, diff -r): no walk that follows symlinks (a link can lead into a
# protected tree or to a secret file under another name), and no walk that
# starts at / or above a protected tree. Content searches (grep, rg, diff)
# count every forbidden path, so /etc is refused for them; listings only
# the trees (/proc, /sys, /dev, /boot, /root, root's home).
# Reads the parser's __le_words / __le_globs.
# Usage: __llm_exec_check_walk CWD
__llm_exec_check_walk() {
    local cwd="$1" name="${__le_words[0]}"
    local mode=names walk=0 implicit=1 pattern=0 follow=0
    local vals="" shortvals="" rec_short="" follow_short=""
    case "$name" in
        grep | egrep | fgrep)
            mode=content pattern=1 rec_short=rR follow_short=R shortvals=efmABCdD
            vals=" -e -f -m -A -B -C -d -D --regexp --file --max-count --after-context --before-context --context --include --exclude --exclude-dir --exclude-from --label --binary-files --devices --directories "
            ;;
        rg)
            mode=content walk=1 pattern=1 follow_short=L shortvals=efgtTmABCjMrEd
            vals=" -e -f -g -t -T -m -A -B -C -j -M -r -E -d --regexp --file --glob --iglob --type --type-not --type-add --type-clear --max-count --after-context --before-context --context --threads --max-columns --replace --encoding --max-depth --max-filesize --sort --sortr --pre-glob --ignore-file --path-separator --colors --context-separator --field-match-separator --field-context-separator --engine --dfa-size-limit --regex-size-limit "
            ;;
        fd)
            walk=1 pattern=1 follow_short=L shortvals=etEdSjc
            vals=" -e -t -E -d -S -j -c --extension --type --exclude --max-depth --min-depth --exact-depth --size --changed-within --changed-before --owner --max-results --threads --format --ignore-file --path-separator --search-path --base-directory "
            ;;
        tree)
            walk=1 follow_short=l shortvals=LPIHT
            vals=" -L -P -I -H -T --filelimit --charset --timefmt --sort "
            ;;
        du)
            walk=1 follow_short=L shortvals=dBtX
            vals=" -d -B -t -X --max-depth --block-size --threshold --exclude --exclude-from --time-style "
            ;;
        ls)
            rec_short=R follow_short=L shortvals=IwT
            vals=" -I -w -T --ignore --hide --width --tabsize --block-size --format --sort --time --time-style --indicator-style --quoting-style "
            ;;
        diff)
            mode=content rec_short=r implicit=0 shortvals=xXIFLCUSWD
            vals=" -x -X -I -F -L -C -U -S -W -D --exclude --exclude-from --ignore-matching-lines --show-function-line --label --starting-file --width --ifdef --from-file --to-file "
            ;;
        find)
            __llm_exec_check_find "$cwd"
            return
            ;;
        *) return 0 ;;
    esac

    local i k w c expect="" dashdash=0 recursive="$walk"
    local -a ops=() extra_ops=()
    for ((i = 1; i < ${#__le_words[@]}; i++)); do
        w="${__le_words[i]}"
        if [[ -n "$expect" ]]; then
            case "$expect" in
                --search-path | --base-directory) extra_ops+=("$w") ;;
                -d | --directories) [[ "$name" != rg && "$name" != fd && "$name" != du && "$w" == recurse ]] && recursive=1 ;;
            esac
            expect=""
            continue
        fi
        if ((! dashdash)) && [[ "$w" == -* && "$w" != - ]]; then
            case "$w" in
                --) dashdash=1 ;;
                --recursive | --directories=recurse) [[ "$name" != rg && "$name" != fd ]] && recursive=1 ;;
                --dereference-recursive) recursive=1 follow=1 ;;
                --follow | --dereference) follow=1 ;;
                --files) [[ "$name" == rg ]] && pattern=0 mode=names ;;
                --regexp | --file)
                    pattern=0
                    expect="$w"
                    ;;
                --regexp=* | --file=*) pattern=0 ;;
                --search-path=* | --base-directory=*) extra_ops+=("${w#*=}") ;;
                --files0-from*)
                    __llm_error "option_not_allowed" "$name $w" "FATAL" "false" "Walks whose starting points come from a file are refused; name the directories"
                    return 1
                    ;;
                --*=*) ;;
                --*) [[ "$vals" == *" $w "* ]] && expect="$w" ;;
                *)
                    for ((k = 1; k < ${#w}; k++)); do
                        c="${w:k:1}"
                        [[ "$rec_short" == *"$c"* ]] && recursive=1
                        [[ "$follow_short" == *"$c"* ]] && follow=1
                        if [[ "$shortvals" == *"$c"* ]]; then
                            [[ "$c" == [ef] && "$name" != fd ]] && pattern=0
                            ((k == ${#w} - 1)) && expect="-$c"
                            break
                        fi
                    done
                    ;;
            esac
            continue
        fi
        ops+=("$i")
    done
    ((recursive)) || return 0

    if ((follow)); then
        __llm_error "option_not_allowed" "$name: symlink-following walk" "FATAL" "false" "Recursive walks that follow symlinks are refused (a link can lead into a protected tree or to a secret file); drop the option (grep: -r instead of -R)"
        return 1
    fi
    # The pattern operand is not a path.
    ((pattern && ${#ops[@]} > 0)) && ops=("${ops[@]:1}")
    __llm_exec_check_walk_roots "$cwd" "$mode" "$name" "$implicit" "${ops[@]}" -- "${extra_ops[@]}"
}

# find: starting points are the words before the first expression word;
# -L, -follow and -files0-from (starting points read from a file) are
# refused.
# Usage: __llm_exec_check_find CWD
__llm_exec_check_find() {
    local cwd="$1" i w in_ops=1 skip=0
    local -a ops=()
    for ((i = 1; i < ${#__le_words[@]}; i++)); do
        w="${__le_words[i]}"
        if ((skip)); then
            skip=0
            continue
        fi
        case "$w" in
            -L | -follow | -files0-from)
                __llm_error "option_not_allowed" "find $w" "FATAL" "false" "find walks that follow symlinks or read their starting points from a file are refused; drop $w"
                return 1
                ;;
            -H | -P | -O*) continue ;;
            -D)
                skip=1
                continue
                ;;
        esac
        if ((in_ops)) && [[ "$w" != -* && "$w" != "(" && "$w" != ")" && "$w" != "!" && "$w" != , ]]; then
            ops+=("$i")
        else
            in_ops=0
        fi
    done
    __llm_exec_check_walk_roots "$cwd" names find 1 "${ops[@]}" --
}

# Refuse a walk root at / or above a protected tree. OPERAND_INDEX words
# are positions in __le_words (globs checked by what they match), the words
# after "--" literal paths; with none of either and IMPLICIT 1, the walk
# starts at CWD.
# Usage: __llm_exec_check_walk_roots CWD MODE NAME IMPLICIT OPERAND_INDEX... -- PATH...
__llm_exec_check_walk_roots() {
    local cwd="$1" mode="$2" name="$3" implicit="$4" idx m p
    shift 4
    local -a roots=()
    while (($# > 0)) && [[ "$1" != -- ]]; do
        idx="$1"
        shift
        if [[ -n "${__le_globs[idx]}" ]]; then
            while IFS= read -r m; do
                [[ -n "$m" ]] && roots+=("$m")
            done < <(cd -- "$cwd" 2>/dev/null && compgen -G "${__le_globs[idx]}" 2>/dev/null)
        else
            roots+=("${__le_words[idx]}")
        fi
    done
    (($# > 0)) && shift
    roots+=("$@")
    ((${#roots[@]} == 0 && implicit)) && roots=(.)
    for p in ${roots[@]+"${roots[@]}"}; do
        if __llm_path_above_forbidden "$(__llm_exec_abs "$p" "$cwd")" "$mode"; then
            __gash_json_error "forbidden_path" "$name $p" "FATAL" "A recursive command cannot start at / or above a protected tree (/proc, /sys, /dev, /boot, /root; for content searches also /etc); name a narrower directory"
            return 1
        fi
    done
    return 0
}

# git: read-only subcommands, no config overrides, no output files.
__llm_exec_check_git() {
    while (($# > 0)) && [[ "$1" == -* ]]; do
        case "$1" in
            -C)
                shift
                ;;
            # Not --literal-pathspecs: the secret exclusions are pathspec magic.
            --no-pager | -P | --git-dir=* | --work-tree=* | --no-optional-locks) ;;
            *)
                __llm_exec_refuse_option git "$1"
                return 1
                ;;
        esac
        shift
    done

    local sub="${1-}"
    shift
    if [[ ! " status log diff show blame shortlog describe grep ls-files ls-tree rev-parse rev-list cat-file show-ref for-each-ref merge-base name-rev count-objects whatchanged version reflog branch tag remote config stash worktree diff-tree diff-index diff-files check-ignore check-attr " == *" $sub "* ]]; then
        __llm_exec_refuse_option git "${sub:-(no subcommand)}"
        return 1
    fi

    local w
    for w in "$@"; do
        case "$w" in
            # --no-index: grep and diff outside the repository, where the
            # secret exclusions (pathspecs) do not apply. --batch: cat-file
            # prints any object named on stdin.
            --output* | -O* | --open-files-in-pager* | --ext-diff | --textconv | --filters | --no-index | --batch | --batch=* | --batch-command*)
                __llm_exec_refuse_option "git $sub" "$w"
                return 1
                ;;
        esac
    done

    local -a pos=()
    __llm_exec_positionals pos "$sub" "$@"
    case "$sub" in
        branch | tag)
            for w in "$@"; do
                case "$w" in
                    -d | -D | -m | -M | -c | -C | -f | -a | -s | -u | -e | -F | --delete | --move | --copy | --force | --annotate | --sign | --local-user* | --edit | --file* | --message* | -m* | --set-upstream-to* | --unset-upstream | --edit-description | --track* | --create-reflog)
                        # -a means --all for branch (a listing flag).
                        [[ "$sub" == branch && "$w" == -a ]] && continue
                        __llm_exec_refuse_option "git $sub" "$w"
                        return 1
                        ;;
                esac
            done
            if ((${#pos[@]} > 0)) && [[ " $* " != *" -l "* && " $* " != *" --list "* ]]; then
                __llm_error "option_not_allowed" "git $sub ${pos[0]}" "FATAL" "false" "Naming a $sub creates it; to filter the list use: git $sub --list PATTERN"
                return 1
            fi
            ;;
        remote)
            [[ -z "${pos[0]-}" || "${pos[0]}" =~ ^(show|get-url)$ ]] || {
                __llm_exec_refuse_option "git remote" "${pos[0]}"
                return 1
            }
            # git remote show contacts the remote (no timeout, may prompt)
            # unless -n.
            if [[ "${pos[0]-}" == show && " $* " != *" -n "* ]]; then
                __llm_error "option_not_allowed" "git remote show" "FATAL" "false" "git remote show contacts the network; use git remote show -n NAME or git remote -v"
                return 1
            fi
            ;;
        config)
            local has_read=0
            [[ "${pos[0]-}" == get || "${pos[0]-}" == list ]] && has_read=1
            for w in "$@"; do
                case "$w" in
                    --add | --unset* | --replace-all | --rename-section | --remove-section | -e | --edit | set | unset | rename-section | remove-section | edit)
                        __llm_exec_refuse_option "git config" "$w"
                        return 1
                        ;;
                    --get* | --list | -l) has_read=1 ;;
                esac
            done
            ((has_read)) || {
                __llm_error "option_not_allowed" "git config $*" "FATAL" "false" "Only reads: git config --get NAME, git config --list"
                return 1
            }
            ;;
        stash)
            [[ "${pos[0]-}" =~ ^(list|show)$ ]] || {
                __llm_exec_refuse_option "git stash" "${pos[0]:-(push)}"
                return 1
            }
            # stash show takes no pathspecs, so it cannot leave secret files
            # out of a patch: summaries only.
            if [[ "${pos[0]}" == show ]]; then
                for w in "$@"; do
                    case "$w" in
                        show | stash@* | [0-9]* | -u | --include-untracked | --only-untracked | --stat* | --numstat | --shortstat | --name-only | --name-status | --summary | --compact-summary | --dirstat* | --no-color | --color | --color=* | -z | --abbrev* | --no-renames | -q | --quiet) ;;
                        *)
                            __llm_error "option_not_allowed" "git stash show $w" "FATAL" "false" "git stash show prints summaries only here (--stat, --name-only); for the patch use: git diff 'stash@{0}^' 'stash@{0}'"
                            return 1
                            ;;
                    esac
                done
            fi
            ;;
        worktree)
            [[ "${pos[0]-}" == list ]] || {
                __llm_exec_refuse_option "git worktree" "${pos[0]-}"
                return 1
            }
            ;;
        reflog)
            [[ "${pos[0]-}" =~ ^(expire|delete)$ ]] && {
                __llm_exec_refuse_option "git reflog" "${pos[0]}"
                return 1
            }
            ;;
    esac
    return 0
}

# Check one simple command. Reads the parser's __le_words / __le_globs.
# Usage: __llm_exec_check_simple EXTRA_ALLOWED
__llm_exec_check_simple() {
    local extra="$1"
    local name="${__le_words[0]}"

    if [[ -n "${__le_globs[0]}" || ! "$name" =~ ^[A-Za-z0-9._+-]+$ ]]; then
        if [[ "$name" == *=* ]]; then
            __llm_error "construct_not_allowed" "${name%%=*}=..." "RETRY" "true" "Variable assignments are not allowed; pass values as arguments"
        else
            __llm_error "command_not_allowed" "$name" "STOP" "true" "Run commands by plain name; paths, globs and keywords are not allowed"
        fi
        return 1
    fi

    if [[ "$extra" != *" $name "* ]]; then
        if [[ "$__LLM_EXEC_ALLOWED" != *" $name "* ]]; then
            __llm_error "command_not_allowed" "$name" "STOP" "true" "llm_exec runs read-only commands (ls, cat, grep, find, git status, ...). Ask the user to add '$name' to GASH_LLM_EXEC_ALLOW in ~/.gash_env if it is needed"
            return 1
        fi
        __llm_exec_check_args "${__le_words[@]}" || return 1
        __llm_exec_check_walk "${__le_cwd:-$PWD}" || return 1
    fi

    # Arguments and glob matches: no secret files, no paths under the
    # forbidden trees (/proc, /sys, /dev, /root, ...), no matches that a
    # command would take as an option (a file named "-delete"). Relative
    # names resolve against the directory a `cd` earlier in the line moved
    # to; globs expand there too.
    local i w m cwd="${__le_cwd:-$PWD}" skip_first=0 seen_pos=0 plain=0
    [[ "$cwd" != "$PWD" ]] && plain=1
    # The first operand of these is a pattern, script or filter, not a
    # file, unless -e/-f supply it.
    if [[ " grep egrep fgrep zgrep rg sed awk gawk jq yq " == *" $name "* ]]; then
        skip_first=1
        for w in "${__le_words[@]:1}"; do
            case "$w" in
                -e* | -f* | --regexp* | --file* | --expression* | --from-file*) skip_first=0 ;;
            esac
        done
    fi
    for ((i = 1; i < ${#__le_words[@]}; i++)); do
        w="${__le_words[i]}"
        if [[ -n "${__le_globs[i]}" ]]; then
            while IFS= read -r m; do
                [[ -z "$m" ]] && continue
                if [[ "$m" == -* ]]; then
                    __llm_error "glob_matches_option" "$m" "RETRY" "true" "A file name matched by the glob starts with '-'; prefix the pattern with ./"
                    return 1
                fi
                if __llm_is_secret_file "$m"; then
                    __llm_error "secret_file_blocked" "$m" "FATAL" "false" "Secret files cannot be read through llm_exec"
                    return 1
                fi
                __llm_exec_check_path "$m" "$cwd" 1 || return 1
            done < <(cd -- "$cwd" 2>/dev/null && compgen -G "${__le_globs[i]}" 2>/dev/null)
        fi
        # git also names files as REV:PATH (HEAD:config/.env.local) and
        # -L RANGE:PATH.
        if __llm_is_secret_file "$w" || { [[ "$w" == *=* ]] && __llm_is_secret_file "${w#*=}"; } ||
            { [[ "$name" == git && "$w" == *:* ]] && __llm_secret_name "${w##*:}"; }; then
            __llm_error "secret_file_blocked" "$w" "FATAL" "false" "Secret files cannot be read through llm_exec"
            return 1
        fi
        if [[ "$w" != -* ]] && ((skip_first && ! seen_pos)); then
            seen_pos=1
            continue
        fi
        [[ " echo printf basename dirname " == *" $name "* ]] && continue
        case "$w" in
            --*=*) __llm_exec_check_path "${w#*=}" "$cwd" "$plain" || return 1 ;;
            --*) ;;
            -?*) __llm_exec_check_path "${w:2}" "$cwd" 0 || return 1 ;;
            *=*)
                __llm_exec_check_path "$w" "$cwd" "$plain" || return 1
                __llm_exec_check_path "${w#*=}" "$cwd" "$plain" || return 1
                ;;
            *) __llm_exec_check_path "$w" "$cwd" "$plain" || return 1 ;;
        esac
    done

    # Later commands in the line run where this cd went (unknown targets,
    # such as `cd -`, keep the current guess).
    if [[ "$name" == cd ]]; then
        local target=""
        for w in "${__le_words[@]:1}"; do
            [[ "$w" == -? ]] || target="$w"
        done
        case "$target" in
            "") __le_cwd="$HOME" ;;
            -) ;;
            \~ | \~/*) __le_cwd="$HOME${target#\~}" ;;
            /*) __le_cwd="$target" ;;
            *) __le_cwd="${cwd%/}/$target" ;;
        esac
    fi
    return 0
}

# Refuse a path under __LLM_FORBIDDEN_PATHS (/dev/null excepted). Words
# that do not look like paths (no "/", "~", "." or "..") are checked only
# when PLAIN is 1.
# Usage: __llm_exec_check_path WORD CWD PLAIN
__llm_exec_check_path() {
    local p="$1" cwd="$2"
    [[ -z "$p" ]] && return 0
    if [[ "$3" != 1 && "$p" != */* && "$p" != "~"* && "$p" != . && "$p" != .. ]]; then
        return 0
    fi
    p="$(__llm_exec_abs "$p" "$cwd")"
    [[ "$p" == /dev/null ]] && return 0
    if __llm_path_forbidden "$p"; then
        __gash_json_error "forbidden_path" "$1" "FATAL" "This path is protected and cannot be accessed"
        return 1
    fi
    return 0
}

# Absolute form of a word as the child would see it: ~ and ~user expanded,
# relative paths joined to CWD (not resolved).
# Usage: __llm_exec_abs WORD CWD
__llm_exec_abs() {
    local p="$1" cwd="$2" user home
    case "$p" in
        \~ | \~/*) p="$HOME${p#\~}" ;;
        \~*)
            user="${p%%/*}"
            user="${user#"~"}"
            if [[ -n "$user" && "$user" != *[!A-Za-z0-9._-]* ]]; then
                eval "home=~$user"
                [[ "$home" == /* ]] && p="$home${p#"~$user"}"
            fi
            ;;
    esac
    [[ "$p" == /* ]] || p="${cwd%/}/$p"
    printf '%s\n' "$p"
}

# Parser helpers for __llm_exec_check_line. They work on its locals (word,
# pat, uq, inword, globby, __le_words, __le_globs) through bash's dynamic
# scoping.

# Close the current word, if any, into __le_words / __le_globs.
__llm_exec_flush_word() {
    if ((inword)); then
        if [[ "$uq" == *"{"* && ("$uq" == *","* || "$uq" == *".."*) ]]; then
            __llm_error "construct_not_allowed" "$word" "RETRY" "true" "Brace expansion is not allowed; list the words explicitly or quote them"
            return 1
        fi
        # shellcheck disable=SC2088 # a literal "~/" prefix is meant
        [[ "$pat" == "~/"* ]] && pat="$HOME/${pat#"~/"}"
        __le_words+=("$word")
        if ((globby)); then
            __le_globs+=("$pat")
        else
            __le_globs+=("")
        fi
    fi
    word="" pat="" uq="" inword=0 globby=0
}

# Close the current simple command and check it.
# Usage: __llm_exec_end_simple EXTRA_ALLOWED
__llm_exec_end_simple() {
    __llm_exec_flush_word || return 1
    if ((${#__le_words[@]} > 0)); then
        __llm_exec_check_simple "$1" || return 1
    fi
    __le_words=() __le_globs=()
}

# Parse a command line and check every simple command in it.
# Usage: __llm_exec_check_line CMD EXTRA_ALLOWED
__llm_exec_check_line() {
    local cmd="$1" extra="$2" __le_cwd="$PWD"
    local -a __le_words=() __le_globs=()
    local word="" pat="" uq="" inword=0 globby=0
    local i n=${#cmd} c next j
    local redir_re='^(2>&1|1?>&2|[12]?>[[:space:]]*/dev/null|&>[[:space:]]*/dev/null)([[:space:];|&]|$)'

    for ((i = 0; i < n; i++)); do
        c="${cmd:i:1}"
        next="${cmd:i+1:1}"

        # Redirections allowed at a word boundary: to /dev/null, 2>&1, >&2.
        if ((! inword)) && [[ "$c" == [12\>\&] && "${cmd:i}" =~ $redir_re ]]; then
            ((i += ${#BASH_REMATCH[1]} - 1))
            continue
        fi

        case "$c" in
            " " | $'\t')
                __llm_exec_flush_word || return 1
                ;;
            $'\n' | ";")
                __llm_exec_end_simple "$extra" || return 1
                ;;
            "#")
                if ((inword)); then
                    word+="$c" pat+="$c" uq+="$c"
                else
                    while ((i + 1 < n)) && [[ "${cmd:i+1:1}" != $'\n' ]]; do ((i++)); done
                fi
                ;;
            "'")
                inword=1
                j=$((i + 1))
                while ((j < n)) && [[ "${cmd:j:1}" != "'" ]]; do
                    c="${cmd:j:1}"
                    word+="$c"
                    [[ "$c" == [\*\?\[\]\\] ]] && pat+="\\"
                    pat+="$c"
                    ((j++))
                done
                if ((j >= n)); then
                    __llm_error "unterminated_quote" "'" "RETRY" "true" "Close every quote"
                    return 1
                fi
                i=$j
                ;;
            '"')
                inword=1
                for ((j = i + 1; j < n; j++)); do
                    c="${cmd:j:1}"
                    if [[ "$c" == '"' ]]; then
                        break
                    elif [[ "$c" == '$' || "$c" == '`' ]]; then
                        __llm_error "construct_not_allowed" "$c inside double quotes" "RETRY" "true" "Expansions are not allowed; use single quotes for literal text"
                        return 1
                    elif [[ "$c" == "\\" && "${cmd:j+1:1}" == [\$\`\"\\$'\n'] ]]; then
                        ((j++))
                        c="${cmd:j:1}"
                        [[ "$c" == $'\n' ]] && continue
                    fi
                    word+="$c"
                    [[ "$c" == [\*\?\[\]\\] ]] && pat+="\\"
                    pat+="$c"
                done
                if ((j >= n)); then
                    __llm_error "unterminated_quote" '"' "RETRY" "true" "Close every quote"
                    return 1
                fi
                i=$j
                ;;
            "\\")
                ((i++))
                c="${cmd:i:1}"
                [[ "$c" == $'\n' || -z "$c" ]] && continue
                inword=1
                word+="$c"
                [[ "$c" == [\*\?\[\]\\] ]] && pat+="\\"
                pat+="$c"
                ;;
            '$' | '`')
                __llm_error "construct_not_allowed" "$c" "RETRY" "true" "Variable expansion and command substitution are not allowed; write values literally"
                return 1
                ;;
            "|")
                [[ "$next" == "&" ]] && {
                    __llm_error "construct_not_allowed" "|&" "RETRY" "true" "Use 2>&1 |"
                    return 1
                }
                [[ "$next" == "|" ]] && ((i++))
                __llm_exec_end_simple "$extra" || return 1
                ;;
            "&")
                if [[ "$next" == "&" ]]; then
                    ((i++))
                    __llm_exec_end_simple "$extra" || return 1
                else
                    __llm_error "construct_not_allowed" "&" "RETRY" "true" "Background jobs are not allowed"
                    return 1
                fi
                ;;
            "<" | ">")
                __llm_error "construct_not_allowed" "$c" "RETRY" "true" "Redirections are not allowed (except to /dev/null, 2>&1 and >&2); llm_exec cannot write files"
                return 1
                ;;
            "(" | ")")
                __llm_error "construct_not_allowed" "$c" "RETRY" "true" "Subshells and grouping are not allowed; join plain commands with |, &&, || or ;"
                return 1
                ;;
            "*" | "?" | "[")
                inword=1 globby=1
                word+="$c" pat+="$c" uq+="$c"
                ;;
            *)
                inword=1
                word+="$c" pat+="$c" uq+="$c"
                ;;
        esac
    done
    __llm_exec_end_simple "$extra" || return 1
    return 0
}

# Shell code run before the command in the child: drop any exported
# function that shadows an allowed command, then pin the safe variants.
# Usage: __llm_exec_prelude EXTRA_ALLOWED [unrestricted]
__llm_exec_prelude() {
    local extra="$1" names
    # shellcheck disable=SC2086 # word-split on purpose: one name per word
    names="$(printf '%s ' $__LLM_EXEC_ALLOWED $extra)"
    printf 'unset -f %s 2>/dev/null\n' "$names"
    # CDPATH would make `cd proc` land somewhere the path checks did not see.
    printf 'unset CDPATH\n'
    [[ "${2-}" == unrestricted ]] && return 0
    [[ "$extra" == *" sed "* ]] || printf 'sed() { command sed --sandbox "$@"; }\n'
    [[ "$extra" == *" awk "* ]] || printf 'awk() { command gawk --sandbox "$@"; }\n'
    [[ "$extra" == *" gawk "* ]] || printf 'gawk() { command gawk --sandbox "$@"; }\n'

    # Secret files stay out of recursive searches and diffs: exclusion
    # options added to every grep, rg and diff call.
    local -a ex=() rgex=()
    local dir
    __llm_secret_excludes grep ex
    for dir in "${__LLM_SECRET_DIRS[@]}"; do
        # grep --exclude-dir matches one name: .config/gcloud -> gcloud.
        [[ "$dir" == */* ]] && ex+=("--exclude-dir=$(__llm_icase_glob "${dir##*/}")")
    done
    __llm_secret_excludes rg rgex
    declare -f __llm_cx_run __llm_cx_grep __llm_cx_git \
        __gash_json_error __gash_json_escape __gash_json_escape_to
    printf '__LLM_CX_GREP=(%s)\n' "$(printf '%q ' "${ex[@]}")"
    printf '__LLM_CX_RG=(%s)\n' "$(printf '%q ' "${rgex[@]}")"
    __llm_exec_gnu_grep && printf '__LLM_CX_GREP_LAST=1\n'
    # diff -X FILE: globs matched on base names, files and directories
    # alike, read from a pipe (GNU diff repeats its options in each
    # "diff ..." header line: one short option keeps them readable).
    local -a dx=()
    local w
    for w in "${ex[@]}"; do
        dx+=("${w#*=}")
    done
    printf '__LLM_CX_DIFFX=(%s)\n' "$(printf '%q ' "${dx[@]}")"
    __llm_exec_diff_noderef && printf '__LLM_CX_DIFF=(--no-dereference)\n'
    [[ "$extra" == *" grep "* ]] || printf 'grep() { __llm_cx_grep "$@"; }\n'
    [[ "$extra" == *" egrep "* ]] || printf 'egrep() { __llm_cx_grep -E "$@"; }\n'
    [[ "$extra" == *" fgrep "* ]] || printf 'fgrep() { __llm_cx_grep -F "$@"; }\n'
    [[ "$extra" == *" rg "* ]] || printf 'rg() { __llm_cx_run rg ${#__LLM_CX_RG[@]} "${__LLM_CX_RG[@]}" "$@"; }\n'
    [[ "$extra" == *" diff "* ]] || printf 'diff() { command diff ${__LLM_CX_DIFF[@]+"${__LLM_CX_DIFF[@]}"} -X <(printf "%%s\\n" "${__LLM_CX_DIFFX[@]}") "$@"; }\n'

    if [[ "$extra" != *" git "* ]]; then
        printf '__LLM_CX_GITCFG=(%s)\n' "$(printf '%q ' "${__LLM_GIT_SAFE_CONFIG[@]}")"
        local -a gx=()
        local pattern
        for pattern in "${__LLM_SECRET_PATTERNS[@]}"; do
            gx+=(":(exclude,icase,glob)**/$pattern")
        done
        for dir in "${__LLM_SECRET_DIRS[@]}"; do
            gx+=(":(exclude,icase,glob)**/$dir/**")
        done
        printf '__LLM_CX_GITEX=(%s)\n' "$(printf '%q ' "${gx[@]}")"
        printf 'git() { __llm_cx_git "$@"; }\n'
    fi
    return 0
}

# -----------------------------------------------------------------------------
# Child-side helpers
# -----------------------------------------------------------------------------
# Copied into the llm_exec child with declare -f; the prelude defines the
# arrays they read (__LLM_CX_*).

# Run CMD with EXTRA inserted before the first "--" of ARGS, or after them:
# in grep and rg a later include/exclude rule wins over an earlier one.
# Usage: __llm_cx_run CMD N EXTRA(N words)... ARGS...
__llm_cx_run() {
    local cmd="$1" n="$2" w seen=0
    shift 2
    local -a add=("${@:1:n}") args=()
    shift "$n"
    for w in "$@"; do
        if ((! seen)) && [[ "$w" == -- ]]; then
            args+=("${add[@]}")
            seen=1
        fi
        args+=("$w")
    done
    ((seen)) || args+=("${add[@]}")
    command "$cmd" "${args[@]}"
}

# grep, egrep and fgrep with the secret exclusions: last for GNU grep, first
# for a grep that stops reading options at the first operand.
# shellcheck disable=SC2154 # __LLM_CX_* are set by the child prelude
__llm_cx_grep() {
    if [[ "${__LLM_CX_GREP_LAST-}" == 1 ]]; then
        __llm_cx_run grep "${#__LLM_CX_GREP[@]}" "${__LLM_CX_GREP[@]}" "$@"
    else
        command grep "${__LLM_CX_GREP[@]}" "$@"
    fi
}

# git with the safe configuration, no textconv or external diff, and the
# secret files left out (exclude pathspecs) of everything that prints file
# contents: grep, diff, show, and log/whatchanged/reflog with a patch. A
# blob named by object id (not REV:PATH, whose path the parser checked)
# is refused.
# shellcheck disable=SC2154 # __LLM_CX_* are set by the child prelude
__llm_cx_git() {
    local -a pre=() head=() opts=() ex=()
    while (($# > 0)) && [[ "$1" == -* ]]; do
        pre+=("$1")
        if [[ "$1" == -C ]]; then
            pre+=("${2-}")
            shift
        fi
        shift
    done
    if (($# == 0)); then
        command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}"
        return
    fi
    local sub="$1" w patch=0
    shift
    head=("$sub")
    for w in "$@"; do
        case "$w" in
            -p | -u | --patch* | -U* | --unified* | --word-diff* | --color-words* | -W | --function-context | -c | --cc | --dd | -m | --remerge-diff* | --diff-merges* | --binary | --full-diff | -t) patch=1 ;;
        esac
    done
    case "$sub" in
        show | diff | diff-tree | diff-index | diff-files)
            opts=(--no-ext-diff --no-textconv)
            ex=("${__LLM_CX_GITEX[@]}")
            ;;
        log | whatchanged)
            opts=(--no-ext-diff --no-textconv)
            ((patch)) && ex=("${__LLM_CX_GITEX[@]}")
            ;;
        reflog)
            if [[ "${1-}" == show ]]; then
                head+=(show)
                shift
            fi
            if [[ "${1-}" != exists ]]; then
                opts=(--no-ext-diff --no-textconv)
                ((patch)) && ex=("${__LLM_CX_GITEX[@]}")
            fi
            ;;
        grep)
            opts=(--no-textconv)
            ex=("${__LLM_CX_GITEX[@]}")
            ;;
        blame) opts=(--no-textconv) ;;
    esac
    # -L RANGE:FILE and --follow take exactly one (checked) path.
    for w in "$@"; do
        [[ "$w" == -L* || "$w" == --follow ]] && ex=()
    done
    case "$sub" in
        show | diff | cat-file)
            for w in "$@"; do
                [[ "$w" == -- ]] && break
                [[ "$w" == -* || "$w" == *:* ]] && continue
                [[ "$sub" == cat-file && " $* " == *" -"[tse]" "* ]] && break
                if [[ "$(command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" cat-file -t "$w^{}" 2>/dev/null)" == blob ]]; then
                    __gash_json_error "option_not_allowed" "git $sub $w" "FATAL" \
                        "llm_exec prints a file from git only as REV:PATH (e.g. HEAD:src/app.php), so that the path can be checked"
                    return 1
                fi
            done
            ;;
    esac
    command git "${__LLM_CX_GITCFG[@]}" "${pre[@]}" "${head[@]}" "${opts[@]}" "$@" "${ex[@]}"
}

# Environment of the llm_exec child: a short allowlist, never the caller's
# tokens (jq env, gawk ENVIRON, ps e would print them). Git runs without
# optional locks (status does not write the index) and never prompts.
# Usage: __llm_exec_env OUT
__llm_exec_env() {
    local -n __ee_out="$1"
    local name names
    __ee_out=()
    # No process substitution: an exported POSIXLY_CORRECT turns it off
    # before bash 5.1.
    names="$(compgen -e)"
    while IFS= read -r name; do
        case "$name" in
            PATH | HOME | USER | LOGNAME | LANG | LANGUAGE | LC_* | TERM | TZ | TMPDIR | NO_COLOR | XDG_RUNTIME_DIR | DOCKER_HOST | DOCKER_CONTEXT)
                __ee_out+=("$name=${!name-}")
                ;;
        esac
    done <<<"$names"
    __ee_out+=("PWD=$PWD" GIT_OPTIONAL_LOCKS=0 GIT_TERMINAL_PROMPT=0)
}

# Probe once whether bwrap can start a read-only sandbox here.
__llm_exec_has_bwrap() {
    if [[ -z "${__LLM_EXEC_HAS_BWRAP-}" ]]; then
        if type -P bwrap >/dev/null 2>&1 && bwrap --ro-bind / / --dev /dev --proc /proc --unshare-all --die-with-parent true >/dev/null 2>&1; then
            __LLM_EXEC_HAS_BWRAP=1
        else
            __LLM_EXEC_HAS_BWRAP=0
        fi
    fi
    [[ "$__LLM_EXEC_HAS_BWRAP" == 1 ]]
}

# Setting from ~/.gash_env only (empty when the config module is absent).
__llm_exec_setting() {
    declare -F __gash_get_setting >/dev/null 2>&1 || return 0
    __gash_get_setting "$1" || true
}

# -----------------------------------------------------------------------------
# Public function
# -----------------------------------------------------------------------------

# Execute a read-only command line without recording it in bash history.
# Usage: llm_exec <command>
# Example: llm_exec "ls -la /tmp"
llm_exec() {
    if needs_help "llm_exec" "llm_exec <command>" "Execute command safely: read-only allowlist, child shell, no history" "${1-}"; then
        return 0
    fi

    __gash_no_history __llm_exec_impl "$@"
}

__llm_exec_impl() {
    local cmd="${1-}"
    if (($# > 1)); then
        __llm_error "unexpected_argument" "${2}" "RETRY" "true" "Pass the whole command line as one quoted argument"
        return 1
    fi
    if [[ -z "${cmd//[[:space:]]/}" ]]; then
        __llm_error "empty_command" "" "RETRY" "true" "Provide a command to execute"
        return 1
    fi

    __llm_validate_command "$cmd" || return 1

    local extra=" " mode sandbox w
    local allow
    # Parse ~/.gash_env once, here: the reads below run in subshells.
    if declare -F __gash_load_env >/dev/null 2>&1; then
        __gash_load_env
    fi
    allow="$(__llm_exec_setting GASH_LLM_EXEC_ALLOW)"
    for w in ${allow//[,:]/ }; do
        [[ "$w" =~ ^[A-Za-z0-9._+-]+$ ]] && extra+="$w "
    done
    mode="$(__llm_exec_setting GASH_LLM_EXEC)"
    sandbox="$(__llm_exec_setting GASH_LLM_SANDBOX)"

    if [[ "$mode" != "unrestricted" ]]; then
        __llm_exec_check_line "$cmd" "$extra" || return 1
    fi

    if [[ "$sandbox" == 1 ]] && ! __llm_exec_has_bwrap; then
        __llm_error "sandbox_unavailable" "bwrap" "STOP" "true" "GASH_LLM_SANDBOX=1 needs bubblewrap (bwrap) with user namespaces; ask the user to install it or turn the setting off"
        return 1
    fi
    local -a runner=("${BASH:-bash}" --noprofile --norc -c "$(__llm_exec_prelude "$extra" "$mode")"$'\n'"$cmd")
    if [[ "$sandbox" == 1 ]]; then
        runner=(bwrap --ro-bind / / --dev /dev --proc /proc --unshare-all --die-with-parent --chdir "$PWD" "${runner[@]}")
    fi

    # The child starts from a short allowlisted environment (unrestricted
    # mode keeps the caller's).
    if [[ "$mode" == "unrestricted" ]]; then
        BASH_ENV="" ENV="" "${runner[@]}"
        return
    fi
    local -a child_env=()
    __llm_exec_env child_env
    command env -i "${child_env[@]}" "${runner[@]}"
}
