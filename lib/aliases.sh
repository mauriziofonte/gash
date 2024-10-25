#!/usr/bin/env bash

# Set LS_COLORS if not already set
if [ -z "$LS_COLORS" ]; then
    LS_COLORS+='*.7z=38;5;40:*.WARC=38;5;40:*.a=38;5;40:*.arj=38;5;40:*.br=38;5;40:'
    LS_COLORS+='*.bz2=38;5;40:*.cpio=38;5;40:*.gz=38;5;40:*.lrz=38;5;40:*.lz=38;5;40:'
    LS_COLORS+='*.lzma=38;5;40:*.lzo=38;5;40:*.rar=38;5;40:*.s7z=38;5;40:*.sz=38;5;40:'
    LS_COLORS+='*.tar=38;5;40:*.tbz=38;5;40:*.tgz=38;5;40:*.warc=38;5;40:*.xz=38;5;40:'
    LS_COLORS+='*.z=38;5;40:*.zip=38;5;40:*.zipx=38;5;40:*.zoo=38;5;40:*.zpaq=38;5;40:'
    LS_COLORS+='*.zst=38;5;40:*.zstd=38;5;40:*.zz=38;5;40:*@.service=38;5;45:'
    LS_COLORS+='*AUTHORS=38;5;220;1:*CHANGELOG=38;5;220;1:*LICENSE=38;5;220;1:'

    # Add the file type definitions from the first snippet
    LS_COLORS+='di=1;32:ln=1;30;47:so=30;45:pi=30;45:ex=1;31:bd=30;46:'
    LS_COLORS+='cd=30;46:su=30;41:sg=30;41:tw=30;41:ow=30;41:*.rpm=1;31:*.deb=1;31:'

    # Add the file type definitions from the second snippet
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
if [ -n "$LESS" ]; then
    export LESS_TERMCAP_mb=$'\E[01;31m'       # Begin blinking
    export LESS_TERMCAP_md=$'\E[01;38;5;74m'  # Begin bold
    export LESS_TERMCAP_me=$'\033[0m'           # End mode
    export LESS_TERMCAP_se=$'\033[0m'           # End standout-mode
    export LESS_TERMCAP_so=$'\E[38;5;246m'    # Begin standout-mode
    export LESS_TERMCAP_ue=$'\033[0m'           # End underline
    export LESS_TERMCAP_us=$'\E[04;38;5;146m' # Begin underline
fi

# Enable color support for ls and other utilities if dircolors is available
if command -v dircolors > /dev/null 2>&1; then
    if [ -r ~/.dircolors ]; then
        eval "$(dircolors -b ~/.dircolors)"
    elif [ -r /etc/DIR_COLORS ]; then
        eval "$(dircolors -b /etc/DIR_COLORS)"
    else
        eval "$(dircolors -b)"
    fi

    alias dir='dir --color=auto'
    alias vdir='vdir --color=auto'
    alias grep='grep --color=auto'
    alias fgrep='fgrep --color=auto'
    alias egrep='egrep --color=auto'

    # Aliases for ls
    alias ls='ls --color=auto'
    alias ll='ls -l --color=auto'
    alias la='ls -la --color=auto'
    alias l='ls -CF --color=auto'
    alias lash='ls -lash --color=auto'
    alias sl='ls --color=auto'  # Correct common typo
else
    alias ll='ls -l'
    alias la='ls -la'
    alias l='ls -CF'
    alias lash='ls -lash'
    alias sl='ls'  # Correct common typo
fi

# Override the default help command
alias help=gash_help

# Add an alias for "stop_services" with --force flag
alias quit='stop_services --force'

# Aliases for cd
alias ..='cd ..'
alias ...='cd ../..'
alias cd..='cd ..'
alias ...="cd ../../../"
alias ....="cd ../../../../"
alias .....="cd ../../../../"
alias .4="cd ../../../../"
alias .5="cd ../../../../.."

# Safer versions of common commands
alias cp='cp -iv'
alias mv='mv -iv'
alias rm='rm -Iv --one-file-system --preserve-root'
alias mkdir='mkdir -pv'

# Calculator
alias bc='bc -l'

# Ports
alias ports='netstat -tulanp'

# WSL specific alias
if grep -qi "microsoft" /proc/version && [ -n "$WSLENV" ]; then
    alias wslrestart="history -a && cmd.exe /C wsl --shutdown"
    alias wslshutdown="history -a && cmd.exe /C wsl --shutdown"
    alias explorer="explorer.exe ."
    alias taskmanager="cd /mnt/c && cmd.exe /C taskmgr &"
fi

# Git aliases
if command -v git >/dev/null 2>&1; then
    alias gl="git log --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit"
    alias glog="git log --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit"
    alias gst='git status'
    alias gstatus='git status'
    alias ga='git add'
    alias gadd='git add'
    alias gc='git commit'
    alias gcommit='git commit'
    alias gp='git push'
    alias gpush='git push'
    alias gco='git checkout'
    alias gcheckout='git checkout'
    alias gb='git branch'
    alias gbranch='git branch'
    alias gd='git diff'
    alias gdiff='git diff'
    alias gt=gtags
    alias gta=gadd_tag
    alias gaddtag=gadd_tag
    alias gtd=gdel_tag
    alias gdeltag=gdel_tag
fi

# Function to replace commands with better alternatives if available
function __add_command_replace_alias() {
    if command -v "$2" > /dev/null 2>&1; then
        alias "$1"="$2"
    fi
}

# Function to add an alias if supported by the system
function __add_alias_if_supported() {
    local binary_name="$1"
    local alias_name="$2"
    local alias_command="$3"

    if command -v "$binary_name" &> /dev/null; then
        alias "$alias_name"="$alias_command"
    fi
}

# Replace less with most if installed
if command -v most > /dev/null 2>&1; then
    alias less='most'
    export PAGER='most'
fi

# Set default editor
if command -v nano > /dev/null 2>&1; then
    export EDITOR='nano'
elif command -v vim > /dev/null 2>&1; then
    export EDITOR='vim'
fi

# Replace commands with better alternatives if available
__add_command_replace_alias 'tail' 'multitail'
__add_command_replace_alias 'df' 'pydf'
__add_command_replace_alias 'top' 'htop'

# Use mtr for traceroute and tracepath if installed
if command -v mtr > /dev/null 2>&1; then
    alias traceroute='mtr'
    alias tracepath='mtr'
fi

# Colorize output of diff if colordiff is installed
if command -v colordiff > /dev/null 2>&1; then
    alias diff='colordiff'
fi

# Shortcuts for common commands
alias cls='clear'
alias path='echo -e ${PATH//:/\\n}'

# Safety nets for system commands
alias chown='chown --preserve-root'
alias chmod='chmod --preserve-root'
alias chgrp='chgrp --preserve-root'

# Safe reboot and shutdown
alias reboot='sudo /sbin/reboot'
alias shutdown='sudo /sbin/shutdown'

# PHP & Composer version-specific aliases
for version in 8.3 8.2 8.1 8.0 7.4 7.3 7.2 7.1 7.0 5.6; do
    version_alias=${version//./}
    __add_alias_if_supported "php$version" "php$version_alias" "/usr/bin/php$version -d allow_url_fopen=1 -d memory_limit=2048M"
    __add_alias_if_supported "php$version" "composer$version_alias" "/usr/bin/php$version -d allow_url_fopen=1 -d memory_limit=2048M /usr/local/bin/composer"
done

# Additional Composer-specific aliases
__add_alias_if_supported "composer" "composer" "/usr/bin/php -d allow_url_fopen=1 -d memory_limit=2048M /usr/local/bin/composer"
__add_alias_if_supported "composer" "composer-packages-update" "composer global update"

# Docker-related aliases
__add_alias_if_supported "docker" "dcls" "docker container ls -a"
__add_alias_if_supported "docker" "dclsr" "docker container ls"
__add_alias_if_supported "docker" "dils" "docker image ls"
__add_alias_if_supported "docker" "dirm" "docker image prune -a"
__add_alias_if_supported "docker" "dstop" "docker stop"
__add_alias_if_supported "docker" "dstopall" "docker_stop_all"
__add_alias_if_supported "docker" "dstart" "docker start"
__add_alias_if_supported "docker" "dstartall" "docker_start_all"
__add_alias_if_supported "docker" "dexec" "docker exec -it"
__add_alias_if_supported "docker" "drm" "docker rm"
__add_alias_if_supported "docker" "drmi" "docker rmi"
__add_alias_if_supported "docker" "dlogs" "docker logs -f"
__add_alias_if_supported "docker" "dinspect" "docker inspect"
__add_alias_if_supported "docker" "dnetls" "docker network ls"
__add_alias_if_supported "docker" "dpruneall" "docker_prune_all"

# Docker-compose-related aliases
__add_alias_if_supported "docker-compose" "dc" "docker-compose"
__add_alias_if_supported "docker-compose" "dcup" "docker-compose up -d"
__add_alias_if_supported "docker-compose" "dcdown" "docker-compose down"
__add_alias_if_supported "docker-compose" "dclogs" "docker-compose logs -f"
__add_alias_if_supported "docker-compose" "dcb" "docker-compose build"
__add_alias_if_supported "docker-compose" "dcrestart" "docker-compose restart"
__add_alias_if_supported "docker-compose" "dcps" "docker-compose ps"
__add_alias_if_supported "docker-compose" "dcpull" "docker-compose pull"

# Unset helper functions
unset -f __add_command_replace_alias
unset -f __add_alias_if_supported