#!/usr/bin/env bash

# Gash Aliases: Colors Configuration
# Sets LS_COLORS and related color settings for terminal output.

# Set LS_COLORS if not already set
if [[ -z "${LS_COLORS-}" ]]; then
    LS_COLORS+='*.7z=38;5;40:*.WARC=38;5;40:*.a=38;5;40:*.arj=38;5;40:*.br=38;5;40:'
    LS_COLORS+='*.bz2=38;5;40:*.cpio=38;5;40:*.gz=38;5;40:*.lrz=38;5;40:*.lz=38;5;40:'
    LS_COLORS+='*.lzma=38;5;40:*.lzo=38;5;40:*.rar=38;5;40:*.s7z=38;5;40:*.sz=38;5;40:'
    LS_COLORS+='*.tar=38;5;40:*.tbz=38;5;40:*.tgz=38;5;40:*.warc=38;5;40:*.xz=38;5;40:'
    LS_COLORS+='*.z=38;5;40:*.zip=38;5;40:*.zipx=38;5;40:*.zoo=38;5;40:*.zpaq=38;5;40:'
    LS_COLORS+='*.zst=38;5;40:*.zstd=38;5;40:*.zz=38;5;40:*@.service=38;5;45:'
    LS_COLORS+='*AUTHORS=38;5;220;1:*CHANGELOG=38;5;220;1:*LICENSE=38;5;220;1:'

    # File type definitions
    LS_COLORS+='di=1;32:ln=1;30;47:so=30;45:pi=30;45:ex=1;31:bd=30;46:'
    LS_COLORS+='cd=30;46:su=30;41:sg=30;41:tw=30;41:ow=30;41:*.rpm=1;31:*.deb=1;31:'

    # Extended file type definitions
    LS_COLORS+='bd=38;5;68:ca=38;5;17:cd=38;5;113;1:di=38;5;30:do=38;5;127:'
    LS_COLORS+='ex=38;5;208;1:pi=38;5;126:fi=0:ln=target:mh=38;5;222;1:no=0:'
    LS_COLORS+='or=48;5;196;38;5;232;1:ow=38;5;220;1:sg=48;5;3;38;5;0:'
    LS_COLORS+='su=38;5;220;1;3;100;1:so=38;5;197:st=38;5;86;48;5;234:'
    LS_COLORS+='tw=48;5;235;38;5;139;3:'

    export LS_COLORS
fi

# Set LSCOLORS for macOS if using BSD ls
if [[ "$OSTYPE" == "darwin"* ]]; then
    export LSCOLORS='CxahafafBxagagabababab'
fi

# Set LESS_TERMCAP variables for syntax highlighting in less if supported
if [[ -n "${LESS-}" ]]; then
    export LESS_TERMCAP_mb=$'\E[01;31m'       # Begin blinking
    export LESS_TERMCAP_md=$'\E[01;38;5;74m'  # Begin bold
    export LESS_TERMCAP_me=$'\033[0m'         # End mode
    export LESS_TERMCAP_se=$'\033[0m'         # End standout-mode
    export LESS_TERMCAP_so=$'\E[38;5;246m'    # Begin standout-mode
    export LESS_TERMCAP_ue=$'\033[0m'         # End underline
    export LESS_TERMCAP_us=$'\E[04;38;5;146m' # Begin underline
fi

# Enable color support for ls and other utilities if dircolors is available
if command -v dircolors >/dev/null 2>&1; then
    if [[ -r ~/.dircolors ]]; then
        eval "$(dircolors -b ~/.dircolors)"
    elif [[ -r /etc/DIR_COLORS ]]; then
        eval "$(dircolors -b /etc/DIR_COLORS)"
    else
        eval "$(dircolors -b)"
    fi

    # Enable color grep
    if grep --color=auto -q "" /dev/null >/dev/null 2>&1; then
        alias grep='grep --color=auto'
        alias fgrep='fgrep --color=auto'
        alias egrep='egrep --color=auto'
    fi
fi
