#!/usr/bin/env bash

# Gash Shell: prompt (interactive shells only).
#
# Layout:  [chroot][(venv) ]user[@host]:cwd [(branch*↑1↓2)] [j2] [s1] [!127] $
#
# Cost per prompt: one `git status` inside a repository plus one subshell
# for `jobs`; nothing else forks (the repository root is found by walking
# up from $PWD in bash).
#
# GASH_PROMPT=0 keeps your own PS1; the hook still runs for history and for
# the exit status that the AI module reads.

# -----------------------------------------------------------------------------
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
# without subprocesses.
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
    printf '\e[1;%dm' "$((31 + n % 6))"
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

    local seg="\[${__GASH_PS1_BBLUE}\](${branch}"
    [[ -n "$dirty" ]] && seg+="\[${__GASH_PS1_BYELLOW}\]*\[${__GASH_PS1_BBLUE}\]"
    if [[ "$ahead" != "0" || "$behind" != "0" ]]; then
        seg+="\[${__GASH_PS1_DIM}\]"
        [[ "$ahead" != "0" ]] && seg+="${__GASH_PS1_AHEAD}${ahead}"
        [[ "$behind" != "0" ]] && seg+="${__GASH_PS1_BEHIND}${behind}"
        seg+="\[${__GASH_PS1_OFF}${__GASH_PS1_BBLUE}\]"
    fi
    seg+=")\[${__GASH_PS1_OFF}\]"
    __GASH_PS1_GIT_SEG=" $seg"
}

# -----------------------------------------------------------------------------
# PS1 builder
# -----------------------------------------------------------------------------

__gash_ps1_build() {
    local exit_code="${1:-0}"
    local R="\[${__GASH_PS1_OFF}\]"
    local ps1=""

    [[ -n "${debian_chroot-}" ]] && ps1+="(${debian_chroot})"

    if [[ -n "${VIRTUAL_ENV-}" ]]; then
        ps1+="\[${__GASH_PS1_BWHITE}\](${VIRTUAL_ENV##*/})${R} "
    fi

    # user: red for root, blue under sudo/su, green otherwise
    if [[ "${EUID-}" == "0" ]]; then
        ps1+="\[${__GASH_PS1_RED}\]"
    elif [[ "${USER-}" != "${LOGNAME-}" ]]; then
        ps1+="\[${__GASH_PS1_BLUE}\]"
    else
        ps1+="\[${__GASH_PS1_GREEN}\]"
    fi
    ps1+="\u${R}"

    if [[ -n "${SSH_CONNECTION-}" ]]; then
        [[ -z "${__GASH_PS1_HOST_COLOR-}" ]] && __GASH_PS1_HOST_COLOR="$(__gash_ps1_host_color)"
        ps1+="\[${__GASH_PS1_BWHITE}\]@\[${__GASH_PS1_HOST_COLOR}\]\h${R}"
    fi

    ps1+=":\[${__GASH_PS1_BYELLOW}\]\w${R}"

    __gash_ps1_git
    ps1+="$__GASH_PS1_GIT_SEG"

    # background jobs (one subshell; `jobs` is a builtin)
    local jobs_out
    jobs_out="$(jobs -p)"
    if [[ -n "$jobs_out" ]]; then
        local nl="${jobs_out//[!$'\n']/}"
        ps1+=" \[${__GASH_PS1_BGREEN}\][j$((${#nl} + 1))]${R}"
    fi

    # GNU screen sessions
    local screen_dir
    for screen_dir in "/run/screen/S-${USER-}" "/var/run/screen/S-${USER-}" "/var/run/screens/S-${USER-}" "/var/run/uscreens/S-${USER-}"; do
        [[ -d "$screen_dir" ]] || continue
        local -a sessions=("$screen_dir"/*)
        if [[ -e "${sessions[0]}" ]]; then
            local current="${STY-}"
            current="${current%%.*}"
            ps1+=" \[${__GASH_PS1_BGREEN}\][s${#sessions[@]}${current:+:$current}]${R}"
        fi
        break
    done

    [[ "$exit_code" != "0" ]] && ps1+=" \[${__GASH_PS1_BRED}\][!${exit_code}]${R}"

    ps1+=" \[${__GASH_PS1_BPURPLE}\]\\\$${R} "
    PS1="$ps1"
    PS2="\[${__GASH_PS1_BPURPLE}\]>${R} "
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
# shellcheck disable=SC2128,SC2178 # PROMPT_COMMAND is a string or an array
__gash_prompt_install() {
    local decl
    decl="$(declare -p PROMPT_COMMAND 2>/dev/null)" || decl=""
    if [[ "$decl" == "declare -a"* ]]; then
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
    local decl
    decl="$(declare -p PROMPT_COMMAND 2>/dev/null)" || return 0
    if [[ "$decl" == "declare -a"* ]]; then
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

if [[ "${GASH_PROMPT:-1}" == "1" ]]; then
    __gash_ps1_build 0
fi
