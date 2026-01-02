#!/usr/bin/env bash

# Only run mesg if the shell is interactive and mesg is available
if [[ "$-" == *i* ]] && command -v mesg >/dev/null 2>&1; then
    mesg n 2> /dev/null || true
fi

# Optional system info (off by default to keep startup compact)
if [[ "${GASH_SHOW_INXI-0}" == "1" ]] && command -v inxi >/dev/null 2>&1; then
    inxi -IpRS -v0 -c5
fi

# Startup banner (minimal)
GASH_USER_NAME=$(gash_username)
echo -e "\033[0;36mGash\033[0m \033[1;37m—\033[0m \033[1;32m$GASH_USER_NAME\033[0m"
gash_inspiring_quote || true

# Auto-unlock SSH keys if configured (uses ~/.gash_ssh_credentials)
if declare -f gash_ssh_auto_unlock >/dev/null 2>&1; then
    gash_ssh_auto_unlock
fi

# Unset variables to avoid polluting the environment
unset GASH_USER_NAME