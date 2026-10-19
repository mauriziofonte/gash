// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

// promptBlock is lib/shell/prompt.sh, emitted verbatim except for the
// forced divergence SHELL-01: the git branch, VIRTUAL_ENV basename,
// debian_chroot and the screen session name are stored in
// __GASH_PS1_BRANCH, __GASH_PS1_VENV, __GASH_PS1_CHROOT and __GASH_PS1_STY,
// and PS1 holds only a reference (${__GASH_PS1_BRANCH}). Bash expands the
// reference once per prompt and never re-scans the value, so a command
// substitution in a branch name of a cloned repository renders literally
// instead of running (LEDGER SHELL-01, PRD MUST NOT 7). The rendered prompt
// is byte-identical to 2.x for ordinary names.
//
// Every value the hook reads (the palette, the glyph, __gash_color_env_ok)
// is computed in Bash with no fork, as 2.x did; the host color stays lazy.
const promptBlock = `# -----------------------------------------------------------------------------
# Palette (real escape bytes, wrapped in \[ \] when placed in PS1)
# -----------------------------------------------------------------------------

__GASH_PS1_OFF=$'\e[0m'
__GASH_PS1_RED=$'\e[0;31m'
__GASH_PS1_GREEN=$'\e[0;32m'
__GASH_PS1_BLUE=$'\e[0;34m'
__GASH_PS1_BRED=$'\e[1;31m'
__GASH_PS1_BGREEN=$'\e[1;32m'
__GASH_PS1_BYELLOW=$'\e[1;33m'
__GASH_PS1_BBLUE=$'\e[1;34m'
__GASH_PS1_BPURPLE=$'\e[1;35m'
__GASH_PS1_BWHITE=$'\e[1;37m'
__GASH_PS1_DIM=$'\e[2m'

# Glyphs: Unicode when the locale can render it, ASCII otherwise.
if [[ "${LC_ALL:-${LC_CTYPE:-${LANG-}}}" == *[Uu][Tt][Ff]-8* || "${LC_ALL:-${LC_CTYPE:-${LANG-}}}" == *[Uu][Tt][Ff]8* ]]; then
    __GASH_PS1_AHEAD='↑' __GASH_PS1_BEHIND='↓'
else
    __GASH_PS1_AHEAD='+' __GASH_PS1_BEHIND='-'
fi

# Stable per-machine color for @host (only shown over SSH), computed once
# into __GASH_PS1_HOST_COLOR without subprocesses.
__gash_ps1_host_color() {
    local seed="" id_file n=0 c i
    for id_file in /etc/machine-id /var/lib/dbus/machine-id; do
        if [[ -r "$id_file" ]]; then
            IFS= read -r seed <"$id_file" || true
            break
        fi
    done
    [[ -z "$seed" ]] && seed="${HOSTNAME-}"
    for ((i = 0; i < ${#seed} && i < 32; i++)); do
        printf -v c '%d' "'${seed:i:1}"
        n=$(((n * 31 + c) % 1000003))
    done
    # 31..36 = red..cyan; skip 30 (black) and 37 (white).
    printf -v __GASH_PS1_HOST_COLOR '\e[1;%dm' "$((31 + n % 6))"
}

# -----------------------------------------------------------------------------
# Git
# -----------------------------------------------------------------------------

# Succeeds when $PWD is inside a git work tree that should show in the
# prompt, and sets __GASH_GIT_ROOT. Skipped when the root is $HOME (dotfile
# repositories) or listed in GASH_GIT_EXCLUDE (colon-separated paths).
__gash_in_git_repo() {
    __GASH_GIT_ROOT=""
    local d="$PWD"
    while [[ -n "$d" ]]; do
        [[ -e "$d/.git" ]] && break
        d="${d%/*}"
    done
    if [[ -z "$d" ]]; then
        [[ -e "/.git" ]] || return 1
        d="/"
    fi

    [[ "$d" == "$HOME" ]] && return 1

    if [[ -n "${GASH_GIT_EXCLUDE-}" ]]; then
        local -a excluded=()
        local e
        IFS=':' read -r -a excluded <<<"$GASH_GIT_EXCLUDE"
        for e in "${excluded[@]}"; do
            [[ -n "$e" && "$d" == "${e%/}" ]] && return 1
        done
    fi

    __GASH_GIT_ROOT="$d"
    return 0
}

# Set __GASH_PS1_GIT_SEG to the git segment " (branch*↑1↓2)" with PS1
# escapes, or to nothing. A variable, not output: no subshell per prompt.
# Uses the palette and the \[ \] wrappers (ps1_o, ps1_c) of its caller,
# __gash_ps1_build. The branch name is stored in __GASH_PS1_BRANCH and the
# segment references it (SHELL-01): Bash never re-scans its value.
__gash_ps1_git() {
    __GASH_PS1_GIT_SEG=""
    type -P git >/dev/null 2>&1 || return 0
    __gash_in_git_repo || return 0

    local line branch="" oid="" ab="" ahead=0 behind=0 dirty=""
    while IFS= read -r line; do
        case "$line" in
            "# branch.head "*) branch="${line#\# branch.head }" ;;
            "# branch.oid "*) oid="${line#\# branch.oid }" ;;
            "# branch.ab "*) ab="${line#\# branch.ab }" ;;
            "#"*) ;;
            *)
                dirty="*"
                break
                ;;
        esac
    done < <(GIT_OPTIONAL_LOCKS=0 git -C "$__GASH_GIT_ROOT" status --porcelain=v2 --branch --untracked-files=no 2>/dev/null)

    if [[ -z "$branch" ]]; then
        return 0
    elif [[ "$branch" == "(detached)" ]]; then
        branch="${oid:0:7}"
        [[ -z "$branch" || "$branch" == "(initia" ]] && branch="detached"
    fi

    if [[ -n "$ab" ]]; then
        ahead="${ab%% *}"
        ahead="${ahead#+}"
        behind="${ab##* }"
        behind="${behind#-}"
    fi

    local o="${ps1_o-\\[}" c="${ps1_c-\\]}"
    __GASH_PS1_BRANCH="$branch"
    local seg="${o}${__GASH_PS1_BBLUE}${c}(\${__GASH_PS1_BRANCH}"
    [[ -n "$dirty" ]] && seg+="${o}${__GASH_PS1_BYELLOW}${c}*${o}${__GASH_PS1_BBLUE}${c}"
    if [[ "$ahead" != "0" || "$behind" != "0" ]]; then
        seg+="${o}${__GASH_PS1_DIM}${c}"
        [[ "$ahead" != "0" ]] && seg+="${__GASH_PS1_AHEAD}${ahead}"
        [[ "$behind" != "0" ]] && seg+="${__GASH_PS1_BEHIND}${behind}"
        seg+="${o}${__GASH_PS1_OFF}${__GASH_PS1_BBLUE}${c}"
    fi
    seg+=")${o}${__GASH_PS1_OFF}${c}"
    __GASH_PS1_GIT_SEG=" $seg"
}

# -----------------------------------------------------------------------------
# PS1 builder
# -----------------------------------------------------------------------------

# Usage: __gash_ps1_build EXIT_CODE [static]
# "static" leaves out the parts that fork (git status, the jobs count): the
# PS1 set at load time, which the hook replaces before the first prompt.
__gash_ps1_build() {
    local exit_code="${1:-0}" static=0
    [[ "${2-}" == "static" ]] && static=1

    # \[ \] wrappers and palette, all empty when NO_COLOR / GASH_NO_COLOR
    # turn colors off: checked here, at every prompt, so toggling either one
    # takes effect at the next prompt. The locals shadow the palette for
    # __gash_ps1_git too.
    local ps1_o='\[' ps1_c='\]'
    if ! __gash_color_env_ok; then
        ps1_o='' ps1_c=''
        local __GASH_PS1_OFF='' __GASH_PS1_RED='' __GASH_PS1_GREEN='' __GASH_PS1_BLUE=''
        local __GASH_PS1_BRED='' __GASH_PS1_BGREEN='' __GASH_PS1_BYELLOW='' __GASH_PS1_BBLUE=''
        local __GASH_PS1_BPURPLE='' __GASH_PS1_BWHITE='' __GASH_PS1_DIM=''
    fi
    local o="$ps1_o" c="$ps1_c"
    local R="${o}${__GASH_PS1_OFF}${c}"
    local ps1=""

    if [[ -n "${debian_chroot-}" ]]; then
        __GASH_PS1_CHROOT="$debian_chroot"
        ps1+="(\${__GASH_PS1_CHROOT})"
    fi

    if [[ -n "${VIRTUAL_ENV-}" ]]; then
        __GASH_PS1_VENV="${VIRTUAL_ENV##*/}"
        ps1+="${o}${__GASH_PS1_BWHITE}${c}(\${__GASH_PS1_VENV})${R} "
    fi

    # user: red for root, blue under sudo/su, green otherwise
    if [[ "${EUID-}" == "0" ]]; then
        ps1+="${o}${__GASH_PS1_RED}${c}"
    elif [[ "${USER-}" != "${LOGNAME-}" ]]; then
        ps1+="${o}${__GASH_PS1_BLUE}${c}"
    else
        ps1+="${o}${__GASH_PS1_GREEN}${c}"
    fi
    ps1+="\u${R}"

    if [[ -n "${SSH_CONNECTION-}" ]]; then
        local host_color=""
        if [[ -n "$o" ]]; then
            [[ -z "${__GASH_PS1_HOST_COLOR-}" ]] && __gash_ps1_host_color
            host_color="$__GASH_PS1_HOST_COLOR"
        fi
        ps1+="${o}${__GASH_PS1_BWHITE}${c}@${o}${host_color}${c}\h${R}"
    fi

    ps1+=":${o}${__GASH_PS1_BYELLOW}${c}\w${R}"

    if ((static == 0)); then
        __gash_ps1_git
        ps1+="$__GASH_PS1_GIT_SEG"
    fi

    # Running and stopped jobs (one subshell; ` + "`jobs`" + ` is a builtin). Not
    # finished ones: bash < 5.3 keeps a finished entry for any program run
    # by a RETURN trap, which would show as a phantom job.
    local jobs_out=""
    ((static)) || jobs_out="$(
        jobs -pr
        jobs -ps
    )"
    if [[ -n "$jobs_out" ]]; then
        local nl="${jobs_out//[!$'\n']/}"
        ps1+=" ${o}${__GASH_PS1_BGREEN}${c}[j$((${#nl} + 1))]${R}"
    fi

    # GNU screen sessions
    local screen_dir
    for screen_dir in "/run/screen/S-${USER-}" "/var/run/screen/S-${USER-}" "/var/run/screens/S-${USER-}" "/var/run/uscreens/S-${USER-}"; do
        [[ -d "$screen_dir" ]] || continue
        local -a sessions=("$screen_dir"/*)
        if [[ -e "${sessions[0]}" ]]; then
            __GASH_PS1_STY="${STY-}"
            __GASH_PS1_STY="${__GASH_PS1_STY%%.*}"
            ps1+=" ${o}${__GASH_PS1_BGREEN}${c}[s${#sessions[@]}${__GASH_PS1_STY:+:\${__GASH_PS1_STY}}]${R}"
        fi
        break
    done

    [[ "$exit_code" != "0" ]] && ps1+=" ${o}${__GASH_PS1_BRED}${c}[!${exit_code}]${R}"

    ps1+=" ${o}${__GASH_PS1_BPURPLE}${c}\\\$${R} "
    PS1="$ps1"
    PS2="${o}${__GASH_PS1_BPURPLE}${c}>${R} "
}

# Window title: "cwd [user]" locally, "user@host:cwd" over SSH.
__gash_set_terminal_title() {
    case "${TERM-}" in
        xterm* | rxvt* | screen* | tmux* | alacritty | foot* | kitty* | wezterm | vte* | gnome* | konsole*) ;;
        *) return 0 ;;
    esac
    local cwd="${PWD/#$HOME/\~}"
    if [[ -n "${SSH_CONNECTION-}" ]]; then
        printf '\e]2;%s\a' "${USER-}@${HOSTNAME-}:${cwd}"
    else
        printf '\e]2;%s\a' "${cwd} [${USER-}]"
    fi
}

# -----------------------------------------------------------------------------
# Prompt hook
# -----------------------------------------------------------------------------

# Runs before every prompt. Must be the first PROMPT_COMMAND entry so $? is
# still the status of the user's last command.
__gash_prompt_command() {
    local exit_code=$?
    __AI_LAST_EXIT=$exit_code
    __gash_history_hook
    if [[ "${GASH_PROMPT:-1}" == "1" ]]; then
        __gash_ps1_build "$exit_code"
        __gash_set_terminal_title
    fi
    return "$exit_code"
}

# Add the hook in front of any existing PROMPT_COMMAND (string or, on bash
# 5.1+, array form) instead of replacing it: direnv, VTE, bash-preexec and
# friends keep working.
#
# __gash_prompt_is_array: PROMPT_COMMAND is an indexed array with element 0
# set (without the subshell of ` + "`declare -p`" + `). An array without element 0
# takes the string path: assigning the string sets element 0, an entry
# that bash runs first, as the array path would.
__gash_prompt_is_array() {
    [[ -n "${PROMPT_COMMAND+x}" && "${PROMPT_COMMAND@a}" == *a* ]]
}

# shellcheck disable=SC2128,SC2178 # PROMPT_COMMAND is a string or an array
__gash_prompt_install() {
    if __gash_prompt_is_array; then
        local entry
        for entry in "${PROMPT_COMMAND[@]}"; do
            [[ "$entry" == "__gash_prompt_command" ]] && return 0
        done
        PROMPT_COMMAND=("__gash_prompt_command" "${PROMPT_COMMAND[@]}")
    else
        [[ "${PROMPT_COMMAND-}" == *"__gash_prompt_command"* ]] && return 0
        PROMPT_COMMAND="__gash_prompt_command${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
    fi
}

# Remove the hook (used by gash_unload).
# shellcheck disable=SC2178
__gash_prompt_uninstall() {
    [[ -n "${PROMPT_COMMAND+x}" ]] || return 0
    if __gash_prompt_is_array; then
        local -a kept=()
        local entry
        for entry in "${PROMPT_COMMAND[@]}"; do
            [[ "$entry" == "__gash_prompt_command" ]] || kept+=("$entry")
        done
        PROMPT_COMMAND=("${kept[@]}")
    else
        PROMPT_COMMAND="${PROMPT_COMMAND//__gash_prompt_command; /}"
        PROMPT_COMMAND="${PROMPT_COMMAND//__gash_prompt_command/}"
    fi
}

__gash_prompt_install

# A PS1 right away, for code that reads it at startup and for a later
# PROMPT_COMMAND assignment that drops the hook; the hook builds the full
# one (git status included) before the first prompt.
if [[ "${GASH_PROMPT:-1}" == "1" ]]; then
    __gash_ps1_build 0 static
fi`
