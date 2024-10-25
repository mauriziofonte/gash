#!/usr/bin/env bash

#
#     _____           _                              _   _                  _____ _          _ _ _ 
#   / ____|         | |           /\               | | | |                / ____| |        | | | |
#  | |  __  __ _ ___| |__        /  \   _ __   ___ | |_| |__   ___ _ __  | (___ | |__   ___| | | |
#  | | |_ |/ _` / __| '_ \      / /\ \ | '_ \ / _ \| __| '_ \ / _ \ '__|  \___ \| '_ \ / _ \ | | |
#  | |__| | (_| \__ \ | | |_   / ____ \| | | | (_) | |_| | | |  __/ |     ____) | | | |  __/ | |_|
#  \_____|\__,_|___/_| |_( ) /_/    \_\_| |_|\___/ \__|_| |_|\___|_|    |_____/|_| |_|\___|_|_(_)
#                        |/                                                                      
#
# GASH (Gash, Another SHell) – your terminal's new best friend! It's a lean, mean, Bash-enhancing machine 
# that sprinkles your shell with colorful prompts, smart aliases, and handy functions, 
# all without the bloated fluff. Think of it as the "Oh My Zsh" for Bash, 
# but with fewer hipster vibes and way less get-stuff-done power. 
# Minimalistic, extendable, and designed to make your terminal feel like home. 
# Get ready to love your shell again!
# 
# This script is the main entry point for Gash. It is intended to be sourced from your ~/.gashrc file.
#
# Author: Maurizio Fonte (https://www.mauriziofonte.it)
# Version: 1.0.3
# Release Date: 2024-10-24
# Last Update: 2024-10-26
# License: Apache License
#
# If you find any issue, please report it on GitHub: https://github.com/mauriziofonte/gash/issues
#

# Exit early if not running interactively.
[[ $- != *i* ]] && return

# Exit early if we don't have a ~/.gash/ directory (why are we here?)
if [ ! -d "$HOME/.gash" ]; then
    return
fi

# define some constants
BASH_NAME="Gash"
GASH_VERSION="1.0.0"
GASH_DIR="$HOME/.gash"

###########################################################
#                    History Configuration                #
###########################################################

# Prevent duplicate entries and commands starting with space from being recorded in history.
HISTCONTROL=ignoreboth:erasedups

# Append new history lines to the history file instead of overwriting it.
shopt -s histappend

# Unlimited history size (both in-memory and on-disk).
HISTSIZE=
HISTFILESIZE=

# Record timestamp for each command in history.
export HISTTIMEFORMAT='%F %T '

###########################################################
#                    Terminal Settings                    #         
###########################################################

# Check the window size after each command and update LINES and COLUMNS if necessary.
shopt -s checkwinsize

# Make 'less' more friendly for non-text input files.
if command -v lesspipe >/dev/null 2>&1; then
    eval "$(lesspipe)"
fi

###########################################################
#                 Chroot Identification                   #
###########################################################

# Set variable identifying the chroot you work in (used in the prompt below).
if [ -z "${debian_chroot}" ] && [ -r /etc/debian_chroot ]; then
    debian_chroot=$(< /etc/debian_chroot)
fi

###########################################################
#                    Color Definitions                    #
###########################################################

# Reset
Color_Off='\033[0m'       # Text Reset

# Regular Colors
Red='\e[0;31m'          # Red
Green='\e[0;32m'        # Green
Yellow='\e[0;33m'       # Yellow
Blue='\e[0;34m'         # Blue
Purple='\e[0;35m'       # Purple
Cyan='\e[0;36m'         # Cyan
White='\e[0;37m'        # White

# Bold
BRed='\e[1;31m'         # Bold Red
BGreen='\e[1;32m'       # Bold Green
BYellow='\e[1;33m'      # Bold Yellow
BBlue='\e[1;34m'        # Bold Blue
BPurple='\e[1;35m'      # Bold Purple
BWhite='\e[1;37m'       # Bold White

###########################################################
#                  Prompt Customization                   #
###########################################################

# Enable color support if the terminal supports it.
if command -v tput >/dev/null 2>&1 && tput setaf 1 >/dev/null 2>&1; then
    color_prompt=yes
else
    color_prompt=
fi

# Fully Qualified Domain Name.
if command -v hostname >/dev/null 2>&1; then
    FQDN=$(hostname -f)
else
    FQDN="machine"
fi

# Function to set the terminal title.
function __set_terminal_title() {
    local title=""
    local current_dir="${PWD/#$HOME/~}"

    if [ -n "${SSH_CONNECTION}" ]; then
        title="$(whoami)@${FQDN}:${current_dir}"
    else
        title="${current_dir} [$(whoami)]"
    fi

    printf "\033]2;%s\007" "$title"
}

# Function to generate a unique machine ID for color assignment.
function __get_machine_id() {
    local id_file=""
    for f in /etc/machine-id /var/lib/dbus/machine-id; do
        if [ -f "$f" ]; then
            id_file="$f"
            break
        fi
    done

    if [ -n "$id_file" ]; then
        echo $((0x$(head -c 15 "$id_file" | tr -d '[:space:]')))
    elif command -v hostid >/dev/null 2>&1; then
        echo $((0x$(hostid)))
    else
        echo $(( $(hostname | cksum | awk '{print $1}') % 256 ))
    fi
}

# Function to construct the PS1 prompt.
function __construct_ps1() {
    local exit_code="$1"  # Pass the saved exit code from PROMPT_COMMAND

    if [ ! -n "${HOST_COLOR}" ]; then
        local H=$(__get_machine_id)
        HOST_COLOR=$(tput setaf $((H%5 + 2)))
    fi

    PS1=''

    PS1+="${debian_chroot:+($debian_chroot)}"

    # virtualenv
    if [ -n "${VIRTUAL_ENV}" ]; then
        local VENV=`basename $VIRTUAL_ENV`
        PS1+="\[${BWhite}\](${VENV}) \[${Color_Off}\]"
    fi

    # user
    if [ ${USER} == root ]; then
        PS1+="\[${Red}\]" # root
    elif [ ${USER} != ${LOGNAME} ]; then
        PS1+="\[${Blue}\]" # normal user
    else
        PS1+="\[${Green}\]" # normal user
    fi
    PS1+="\u\[${Color_Off}\]"

    if [ -n "${SSH_CONNECTION}" ]; then
        PS1+="\[${BWhite}\]@"
        PS1+="\[${UWhite}${HOST_COLOR}\]\h\[${Color_Off}\]"
    fi

    # current directory
    PS1+=":\[${BYellow}\]\w"

    # background jobs
    local NO_JOBS=`jobs -p | wc -w`
    if [ ${NO_JOBS} != 0 ]; then
        PS1+=" \[${BGreen}\][j${NO_JOBS}]\[${Color_Off}\]"
    fi

    # screen sessions
    local SCREEN_PATHS="/var/run/screens/S-`whoami` /var/run/screen/S-`whoami` /var/run/uscreens/S-`whoami`"

    for screen_path in ${SCREEN_PATHS}; do
        if [ -d ${screen_path} ]; then
            SCREEN_JOBS=`ls ${screen_path} | wc -w`
            if [ ${SCREEN_JOBS} != 0 ]; then
                local current_screen="$(echo ${STY} | cut -d '.' -f 1)"
                if [ -n "${current_screen}" ]; then
                    current_screen=":${current_screen}"
                fi
                PS1+=" \[${BGreen}\][s${SCREEN_JOBS}${current_screen}]\[${Color_Off}\]"
            fi
            break
        fi
    done

    # git branch
    if [ -x "`which git 2>&1`" ]; then
        local branch="$(git name-rev --name-only HEAD 2>/dev/null)"

        if [ -n "${branch}" ]; then
            local git_status="$(git status --porcelain -b 2>/dev/null)"
            local letters="$( echo "${git_status}" | grep --regexp=' \w ' | sed -e 's/^\s\?\(\w\)\s.*$/\1/' )"
            local untracked="$( echo "${git_status}" | grep -F '?? ' | sed -e 's/^\?\(\?\)\s.*$/\1/' )"
            local status_line="$( echo -e "${letters}\n${untracked}" | sort | uniq | tr -d '[:space:]' )"
            PS1+=" \[${BBlue}\](${branch}"
            if [ -n "${status_line}" ]; then
                PS1+=" ${status_line}"
            fi
            PS1+=")\[${Color_Off}\]"
        fi
    fi

    # exit code
    if [ ${exit_code} != 0 ]; then
        PS1+=" \[${BRed}\][!${exit_code}]\[${Color_Off}\]"
    fi

    PS1+=" \[${BPurple}\]\\$\[${Color_Off}\] " # prompt

    __set_terminal_title
}

# Set the prompt command to construct PS1 before each prompt and update history immediately.
if [ "$color_prompt" = yes ]; then
    PROMPT_COMMAND='EXIT_CODE=$?; history -a; history -c; history -r; __construct_ps1 $EXIT_CODE'
    PS2="\[${BPurple}\]>\[${Color_Off}\] "  # Continuation prompt.
else
    PS1='${debian_chroot:+($debian_chroot)}\u@\h:\w\$ '
fi

unset color_prompt

###########################################################
#                   Alias and Includes                    #
###########################################################

# Set default file creation permissions to be readable by everyone.
umask 022

# Load Functions
if [ -f "$GASH_DIR/lib/functions.sh" ]; then
    source "$GASH_DIR/lib/functions.sh"
else
    echo -e "${Red}Warning: Gash 'functions.sh' not found.${Color_Off}"
fi

# Load Aliases
if [ -f "$GASH_DIR/lib/aliases.sh" ]; then
    source "$GASH_DIR/lib/aliases.sh"
else
    echo -e "${Red}Warning: Gash 'aliases.sh' not found.${Color_Off}"
fi

# Load Telemetry Disabler
if [ -f "$GASH_DIR/lib/telemetryoff.sh" ]; then
    source "$GASH_DIR/lib/telemetryoff.sh"
else
    echo -e "${Red}Warning: Gash 'telemetryoff.sh' not found.${Color_Off}"
fi

# Load the Prompt
if [ -f "$GASH_DIR/lib/prompt.sh" ]; then
    source "$GASH_DIR/lib/prompt.sh"
else
    echo -e "${Red}Warning: Gash 'prompt.sh' not found.${Color_Off}"
fi

# Load custom aliases from ~/.bash_aliases if it exists.
if [ -f ~/.bash_aliases ]; then
    source ~/.bash_aliases
fi

# Load additional configurations from ~/.bash_local if it exists.
if [ -f ~/.bash_local ]; then
    source ~/.bash_local
fi

###########################################################
#                   End of gash.sh                        #
###########################################################