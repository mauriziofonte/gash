// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

// historyBlock is lib/shell/history.sh, emitted verbatim: GASH_HISTORY=1
// (the default) sets the history policy unconditionally (the overwrite of a
// user's HISTCONTROL is the reproduced LEDGER SHELL-04 defect), and defines
// __gash_history_hook, which the prompt hook calls. All pure Bash, no fork.
const historyBlock = `if [[ "${GASH_HISTORY:-1}" == "1" ]]; then
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
}`
