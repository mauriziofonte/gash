#!/usr/bin/env bash

# Gash Shell: history policy (interactive shells only).
#
# GASH_HISTORY=1 (default)  Gash manages history: unlimited size, timestamps,
#                           no duplicates or space-prefixed commands, append
#                           mode, and every command is written to the history
#                           file as soon as it runs.
# GASH_HISTORY=0            Gash leaves history settings alone.
# GASH_HISTORY_SYNC=1       Also re-read the history file before each prompt,
#                           so commands typed in other terminals show up with
#                           the arrow keys (costs a full re-read per prompt).

if [[ "${GASH_HISTORY:-1}" == "1" ]]; then
    HISTCONTROL=ignoreboth
    shopt -s histappend
    HISTSIZE=-1
    HISTFILESIZE=-1
    export HISTTIMEFORMAT='%F %T '
fi

# Called from the prompt hook, after the exit status has been saved.
__gash_history_hook() {
    [[ "${GASH_HISTORY:-1}" == "1" ]] || return 0
    history -a
    if [[ "${GASH_HISTORY_SYNC:-0}" == "1" ]]; then
        history -c
        history -r
    fi
}
