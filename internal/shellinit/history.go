// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

// historyBlock is lib/shell/history.sh: GASH_HISTORY=1 (the default) sets
// the history policy and defines __gash_history_hook, which the prompt hook
// calls. A HISTCONTROL the user set is kept, with ignoreboth added only when
// it has none of ignorespace, ignoredups, ignoreboth (3.0: SHELL-04 fixed;
// 2.x overwrote it). All pure Bash, no fork.
const historyBlock = `if [[ "${GASH_HISTORY:-1}" == "1" ]]; then
    case ":${HISTCONTROL-}:" in
        *:ignore*) ;;
        *) HISTCONTROL="${HISTCONTROL:+$HISTCONTROL:}ignoreboth" ;;
    esac
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
