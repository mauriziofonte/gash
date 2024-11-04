#!/usr/bin/env bash

# Only run mesg if the shell is interactive and mesg is available
if [[ "$-" == *i* ]] && command -v mesg >/dev/null 2>&1; then
    mesg n 2> /dev/null || true
fi

# run inxi information tool
if [ -x "`which inxi 2>&1`" ]; then
    inxi -IpRS -v0 -c5
fi

# Greet the fantastic person behind the screen
GASH_USER_NAME=$(gash_username)
echo
echo -e "\033[0;36m💻 G\033[0;33ma\033[38;5;214ms\033[0;32mh\033[0m -- \033[1;37mGash, Another SHell!\033[0m"
echo -e "👋 Hello, \033[1;32m$GASH_USER_NAME\033[0m!"
gash_inspiring_quote
echo -e "\033[1;97m✨\033[0m \033[38;5;214mHave a nice day!\033[0m \033[1;97m😊\033[0m"
echo

# Unset variables to avoid polluting the environment
unset GASH_USER_NAME