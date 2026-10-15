#!/usr/bin/env bash

# Gash Module: guided migration from Gash 1.x to 2.x
#
# Gash 2 changes defaults that 1.x users rely on (command replacements,
# umask, llm_exec, AI consent, the ~/.gashrc loader). Instead of a silent
# change, the first interactive shell after the upgrade walks the user
# through the changes that concern them, one screen each, previews the
# edits as a diff, backs up every file it touches and can roll back.
#
# State lives in ${XDG_STATE_HOME:-~/.local/state}/gash:
#   schema               configuration schema version ("2" once migrated)
#   notice               epoch of the last non-interactive notice
#   backups/<stamp>/     copies of the touched files + "manifest"
#
# Public functions:
#   gash_migrate [--rollback|--status] [--yes] [--plain]   (also: gash migrate)

__GASH_SCHEMA_VERSION=2

__gash_migrate_state_dir() {
    printf '%s/gash' "${XDG_STATE_HOME:-$HOME/.local/state}"
}

# Read a whole file into VAR without forking (empty when unreadable).
__gash_migrate_slurp() {
    local -n __gms_out="$1"
    __gms_out=""
    [[ -r "$2" ]] || return 1
    IFS= read -r -d '' __gms_out <"$2" || true
}

# Succeeds when this account still has a Gash 1.x configuration: the
# schema is not recorded yet and ~/.gashrc is the 1.x loader (it exports
# GASH_BASH, which 2.x never writes).
__gash_migrate_needed() {
    local schema="" gashrc=""
    __gash_migrate_slurp schema "$(__gash_migrate_state_dir)/schema" || true
    schema="${schema//[[:space:]]/}"
    # Glob, not =~: this runs at every shell start and must not leave
    # BASH_REMATCH behind.
    [[ -n "$schema" && "$schema" != *[!0-9]* ]] && ((10#$schema >= __GASH_SCHEMA_VERSION)) && return 1
    __gash_migrate_slurp gashrc "$HOME/.gashrc" || return 1
    [[ "$gashrc" == *GASH_BASH* ]]
}

__gash_migrate_mark_done() {
    local dir
    dir="$(__gash_migrate_state_dir)"
    mkdir -p "$dir" 2>/dev/null && printf '%s\n' "$__GASH_SCHEMA_VERSION" >"$dir/schema"
}

# Called at the end of gash.sh. Interactive terminal: run the guided
# migration. Anywhere else (headless, pipes, scripts): one line on stderr,
# at most once a day, and nothing else.
__gash_migrate_autostart() {
    __gash_migrate_needed || return 0
    if [[ "${GASH_HEADLESS-}" != "1" && $- == *i* ]] && __gash_tty_io; then
        gash_migrate --auto
        return 0
    fi
    local dir now last=""
    dir="$(__gash_migrate_state_dir)"
    printf -v now '%(%s)T' -1
    __gash_migrate_slurp last "$dir/notice" || true
    last="${last//[^0-9]/}"
    [[ -n "$last" ]] && ((now - last < 86400)) && return 0
    mkdir -p "$dir" 2>/dev/null && printf '%s\n' "$now" >"$dir/notice" 2>/dev/null
    __gash_note 'gash: your configuration is from Gash 1.x; open a terminal and run "gash migrate" to review the changes in 2.0'
    return 0
}

# -----------------------------------------------------------------------------
# Inventory
# -----------------------------------------------------------------------------

# Files a 1.x user may have customized, as they exist.
__gash_migrate_user_files() {
    local f
    for f in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.bash_aliases" "$HOME/.bash_local"; do
        [[ -f "$f" ]] && printf '%s\n' "$f"
    done
    return 0
}

# A setting configured in the environment or in ~/.gash_env.
__gash_migrate_has_setting() {
    [[ -n "${!1+x}" ]] && return 0
    local content=""
    __gash_migrate_slurp content "${GASH_ENV_FILE:-$HOME/.gash_env}" || return 1
    [[ $'\n'"$content" =~ $'\n'[[:space:]]*"$1"= ]]
}

# Code lines of every ~/.gashrc that a 1.x installer ever wrote.
__GASH_MIGRATE_V1_GASHRC=$'export GASH_BASH="$HOME/.gash/gash.sh"
export GASH_COMPLETION="$HOME/.gash/bash_completion"
export GASH_BASH=$HOME/.gash/gash.sh
export GASH_COMPLETION=$HOME/.gash/bash_completion
if [ -f "$GASH_BASH" ]; then
if [ -f $GASH_BASH ]; then
source "$GASH_BASH" # This loads Gash
source $GASH_BASH # This loads Gash
source $GASH_BASH
source $GASH_COMPLETION # This loads Gash completion
[ -f "$GASH_COMPLETION" ] && source "$GASH_COMPLETION" # This loads Gash completion
if [ "${GASH_HEADLESS-}" != "1" ] && [ -f "$GASH_COMPLETION" ]; then
source "$GASH_COMPLETION" # This loads Gash completion
fi'

# Lines of ~/.gashrc that the 1.x template did not have (the user's own).
__gash_migrate_gashrc_extras() {
    local line trimmed
    while IFS= read -r line || [[ -n "$line" ]]; do
        trimmed="${line#"${line%%[![:space:]]*}"}"
        trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"
        [[ -z "$trimmed" || "$trimmed" == \#* ]] && continue
        [[ $'\n'"$__GASH_MIGRATE_V1_GASHRC"$'\n' == *$'\n'"$trimmed"$'\n'* ]] && continue
        printf '%s\n' "$line"
    done <"$HOME/.gashrc"
    return 0
}

# Lines of ~/.gash_env that Gash 2 skips: "NUMBER<TAB>reason" (missing SSH
# keys are left alone, the key may come back).
__gash_migrate_env_problems() {
    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}" msg
    [[ -f "$env_file" ]] || return 0
    while IFS= read -r msg; do
        [[ "$msg" =~ Line\ ([0-9]+)\ of\ [^:]+:\ (.*)$ ]] || continue
        [[ "${BASH_REMATCH[2]}" == "SSH key file not found"* ]] && continue
        printf '%s\t%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
    done < <(
        __GASH_ENV_LOADED=""
        GASH_ENV_FILE="$env_file" GASH_NO_COLOR=1 __gash_load_env 2>&1 >/dev/null
    )
    return 0
}

# ~/.gash_env readable by group or others.
__gash_migrate_env_open() {
    local env_file="${GASH_ENV_FILE:-$HOME/.gash_env}" perms
    [[ -f "$env_file" ]] || return 1
    perms="$(stat -c '%a' "$env_file" 2>/dev/null || stat -f '%Lp' "$env_file" 2>/dev/null)" || return 1
    [[ "$perms" =~ [0-7]*[0-7]([0-7])([0-7])$ ]] || return 1
    ((BASH_REMATCH[1] != 0 || BASH_REMATCH[2] != 0))
}

# Command replacements 1.x made on this machine: "cmd -> replacement".
__gash_migrate_tool_swaps() {
    local pair
    for pair in less:most tail:multitail df:pydf top:htop traceroute:mtr tracepath:mtr diff:colordiff; do
        type -P "${pair#*:}" >/dev/null 2>&1 && printf '%s -> %s\n' "${pair%%:*}" "${pair#*:}"
    done
    local php
    for php in /usr/bin/php[0-9]*; do
        [[ -x "$php" ]] && {
            printf 'php -> %s (with -d memory_limit=2048M)\n' "${php##*/}"
            break
        }
    done
    return 0
}

# Lines in the user's files that 2.0 reads differently:
# "FILE<TAB>LINE<TAB>RULE<TAB>TEXT". Rules: dryrun (fixable), portkill,
# llmexec, gashvar.
__gash_migrate_scan_scripts() {
    local f n line rule trimmed
    while IFS= read -r f; do
        n=0
        while IFS= read -r line || [[ -n "$line" ]]; do
            n=$((n + 1))
            trimmed="${line#"${line%%[![:space:]]*}"}"
            [[ -z "$trimmed" || "$trimmed" == \#* ]] && continue
            rule=""
            if [[ "$line" =~ (^|[^[:alnum:]_])(docker_compose_upgrade|dcup2)([^[:alnum:]_]|$) && "$line" =~ [[:space:]]-n([[:space:]\"\';\)]|$) ]]; then
                rule=dryrun
            elif [[ "$line" =~ (^|[^[:alnum:]_])(port_kill|ptk)([^[:alnum:]_-]|$) ]]; then
                rule=portkill
            elif [[ "$line" =~ (^|[^[:alnum:]_])llm_exec([^[:alnum:]_]|$) ]]; then
                rule=llmexec
            elif [[ "$line" =~ GASH_(BASH|COMPLETION)([^[:alnum:]_]|$) ]]; then
                rule=gashvar
            fi
            [[ -n "$rule" ]] && printf '%s\t%s\t%s\t%s\n' "$f" "$n" "$rule" "$trimmed"
        done <"$f"
    done < <(__gash_migrate_user_files)
    return 0
}

# -----------------------------------------------------------------------------
# Edits (on work copies)
# -----------------------------------------------------------------------------

# Set NAME=VALUE in an env file copy: replace an active line, else append.
__gash_migrate_set_in() {
    local file="$1" name="$2" value="$3" line found=0 out="" header=0
    if [[ -f "$file" ]]; then
        while IFS= read -r line || [[ -n "$line" ]]; do
            if [[ "$line" =~ ^[[:space:]]*"$name"= ]]; then
                ((found)) && continue
                line="$name=$value"
                found=1
            fi
            [[ "$line" == "# Added by gash migrate"* ]] && header=1
            out+="$line"$'\n'
        done <"$file"
    fi
    if ((! found)); then
        ((header)) || out+=$'\n'"# Added by gash migrate (Gash 2.0 settings)"$'\n'
        out+="$name=$value"$'\n'
    fi
    printf '%s' "$out" >"$file"
}

# Comment out the given line numbers of a file copy, with a reason marker.
__gash_migrate_comment_lines() {
    local file="$1" nums=" $2 " line n=0 out=""
    while IFS= read -r line || [[ -n "$line" ]]; do
        n=$((n + 1))
        [[ "$nums" == *" $n "* ]] && line="# [skipped by Gash 2] $line"
        out+="$line"$'\n'
    done <"$file"
    printf '%s' "$out" >"$file"
}

# Rewrite "dcup2 ... -n" as "--dry-run" on the given lines of a file copy.
__gash_migrate_fix_dryrun() {
    local file="$1" nums=" $2 " line n=0 out=""
    while IFS= read -r line || [[ -n "$line" ]]; do
        n=$((n + 1))
        if [[ "$nums" == *" $n "* ]]; then
            while [[ "$line" =~ ^(.*[[:space:]])-n([[:space:]\"\';\)].*)?$ ]]; do
                line="${BASH_REMATCH[1]}--dry-run${BASH_REMATCH[2]}"
            done
        fi
        out+="$line"$'\n'
    done <"$file"
    printf '%s' "$out" >"$file"
}

# The 2.0 ~/.gashrc, plus the user's own lines from the 1.x file.
__gash_migrate_new_gashrc() {
    local out="$1" extras="$2" dir="${GASH_DIR:-$HOME/.gash}"
    if [[ "$dir" == "$HOME/.gash" ]]; then
        cat "$dir/.gashrc" >"$out"
    else
        local line
        while IFS= read -r line || [[ -n "$line" ]]; do
            [[ "$line" == __gash_home=* ]] && line="__gash_home=\"\${GASH_DIR:-$dir}\""
            printf '%s\n' "$line"
        done <"$dir/.gashrc" >"$out"
    fi
    if [[ -n "$extras" ]]; then
        printf '\n# Kept from your Gash 1.x ~/.gashrc\n%s\n' "$extras" >>"$out"
    fi
}

# -----------------------------------------------------------------------------
# Screens
# -----------------------------------------------------------------------------

__gash_migrate_tui() {
    [[ "${GASH_TUI-}" != plain && "${TERM-}" != dumb ]] && __gash_tty_io
}

# Draw one screen: a title line, then BODY wrapped in a box.
# Usage: __gash_migrate_screen STEP TOTAL TITLE BODY
__gash_migrate_screen() {
    local step="$1" total="$2" title="$3" body="$4" width
    width=$(($(__gash_tty_width) - 8))
    ((width > 72)) && width=72
    ((width < 30)) && width=30
    if __gash_migrate_tui; then __gash_clear_screen; fi
    local B="" P="" R=""
    if __gash_use_color; then
        B="$__GASH_BOLD" P="$__GASH_COLOR_PRIMARY" R="$__GASH_COLOR_OFF"
    fi
    local -a lines=()
    __gash_wrap lines "$width" "$body"
    echo
    __gash_heading --indent 2 --note "${step:+$__GASH_GLYPH_DOT $step of $total}" "Gash 2.0 migration"
    echo
    __gash_box --color "$P" "${B}${title}${R}" "" "${lines[@]}"
    echo
}

# Colored unified diff of OLD (or nothing) against NEW, labelled PATH.
__gash_migrate_diff() {
    # Headers in the structure color, added lines success, removed error.
    local old="$1" new="$2" label="$3" G="" Rd="" C="" R=""
    if __gash_use_color; then
        G="$__GASH_COLOR_SUCCESS" Rd="$__GASH_COLOR_ERROR" C="$__GASH_COLOR_PRIMARY" R="$__GASH_COLOR_OFF"
    fi
    [[ -f "$old" ]] || old=/dev/null
    # diff exits 1 when the files differ: never let that reach `set -e`.
    { diff -u --label "$label" --label "$label (new)" "$old" "$new" 2>/dev/null || true; } |
        while IFS= read -r line; do
            case "$line" in
                ---* | +++*) printf '  %s%s%s\n' "$C" "$line" "$R" ;;
                @@*) printf '  %s%s%s\n' "$C" "$line" "$R" ;;
                +*) printf '  %s%s%s\n' "$G" "$line" "$R" ;;
                -*) printf '  %s%s%s\n' "$Rd" "$line" "$R" ;;
                *) printf '  %s\n' "$line" ;;
            esac
        done
}

# -----------------------------------------------------------------------------
# Rollback and status
# -----------------------------------------------------------------------------

__gash_migrate_rollback() {
    local yes="$1" dir backup="" b
    dir="$(__gash_migrate_state_dir)/backups"
    for b in "$dir"/*/manifest; do
        [[ -f "$b" && "$b" != *.restored/manifest ]] && backup="${b%/manifest}"
    done
    if [[ -z "$backup" ]]; then
        __gash_error "Nothing to roll back: no migration backup in ${dir/#$HOME/\~}"
        return 1
    fi
    local state path name
    echo
    printf '  Restoring the files saved on %s:\n' "${backup##*/}"
    while IFS=$'\t' read -r state path name; do
        if [[ "$state" == present ]]; then
            __gash_list --indent 4 "${path/#$HOME/\~} (restored)"
        else
            __gash_list --indent 4 "${path/#$HOME/\~} (removed, it did not exist before)"
        fi
    done <"$backup/manifest"
    echo
    if [[ "$yes" != 1 ]]; then
        local pick=""
        __gash_choose pick 1 "Restore these files" "Cancel" || return 1
        [[ "$pick" == 1 ]] || return 1
    fi
    while IFS=$'\t' read -r state path name; do
        if [[ "$state" == present ]]; then
            cp -p -- "$backup/$name" "$path" || {
                __gash_error "Could not restore ${path/#$HOME/\~}"
                return 1
            }
        else
            rm -f -- "$path"
        fi
    done <"$backup/manifest"
    mv -- "$backup" "$backup.restored"
    rm -f -- "$(__gash_migrate_state_dir)/schema"
    __gash_success "Restored. Open a new terminal; run 'gash migrate' to review the changes again."
}

__gash_migrate_status() {
    local dir schema="" b n=0
    dir="$(__gash_migrate_state_dir)"
    __gash_migrate_slurp schema "$dir/schema" || true
    schema="${schema//[[:space:]]/}"
    local -a kv=("Configuration schema:" "${schema:-1 (not migrated)}")
    if __gash_migrate_needed; then
        kv+=("Migration:" 'pending, run "gash migrate"')
    else
        kv+=("Migration:" "nothing pending")
    fi
    for b in "$dir"/backups/*/manifest; do
        [[ -f "$b" ]] || continue
        n=$((n + 1))
        b="${b%/manifest}"
        kv+=("Backup:" "${b/#$HOME/\~}")
    done
    ((n)) || kv+=("Backups:" "none")
    echo
    __gash_kv --indent 2 --width 0 "${kv[@]}"
    echo
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------

# Guided migration of a Gash 1.x configuration.
# Usage: gash_migrate [--rollback|--status] [--yes] [--plain]
gash_migrate() {
    needs_help "gash_migrate" "gash_migrate [--rollback|--status] [--yes] [--plain]" \
        "Review the Gash 2.0 changes that concern your configuration and apply your choices, with a diff preview and backups. Also: gash migrate" \
        "${1-}" && return

    local action=run yes=0 auto=0 a
    for a in "$@"; do
        case "$a" in
            --rollback) action=rollback ;;
            --status) action=status ;;
            --yes | -y) yes=1 ;;
            --plain) local GASH_TUI=plain ;;
            --auto) auto=1 ;;
            *)
                __gash_error "gash_migrate: unknown option '$a'"
                return 1
                ;;
        esac
    done
    case "$action" in
        rollback)
            __gash_migrate_rollback "$yes"
            return
            ;;
        status)
            __gash_migrate_status
            return 0
            ;;
    esac

    local work
    work="$(mktemp -d "${TMPDIR:-/tmp}/gash-migrate.XXXXXX")" || {
        __gash_error "gash_migrate: cannot create a temporary directory"
        return 1
    }
    # shellcheck disable=SC2064 # expand $work now: the trap outlives the local
    trap "command rm -rf -- '$work'; trap - RETURN" RETURN

    local gashrc="$HOME/.gashrc" env_file="${GASH_ENV_FILE:-$HOME/.gash_env}"
    local -a items=()
    local gashrc_extras="" env_problems="" tool_swaps="" scripts="" cur_umask
    if [[ -f "$gashrc" ]] && grep -q GASH_BASH "$gashrc" 2>/dev/null; then
        items+=(gashrc)
        gashrc_extras="$(__gash_migrate_gashrc_extras)"
    fi
    env_problems="$(__gash_migrate_env_problems)"
    if [[ -n "$env_problems" ]] || __gash_migrate_env_open; then items+=(env); fi
    if ! __gash_migrate_has_setting GASH_REPLACE_TOOLS; then
        tool_swaps="$(__gash_migrate_tool_swaps)"
        [[ -n "$tool_swaps" ]] && items+=(tools)
    fi
    cur_umask="$(umask)"
    if [[ "$cur_umask" != 0022 && "$cur_umask" != 022 ]] && ! __gash_migrate_has_setting GASH_UMASK; then
        items+=(umask)
    fi
    scripts="$(__gash_migrate_scan_scripts)"
    [[ -n "$scripts" ]] && items+=(scripts)
    __gash_migrate_has_setting GASH_LLM_EXEC || items+=(llm)
    if ! __gash_migrate_has_setting GASH_AI_CONFIRM && [[ -f "$env_file" ]] && grep -q '^[[:space:]]*AI:' "$env_file" 2>/dev/null; then
        items+=(ai)
    fi

    local total=${#items[@]} pick=""

    # Welcome
    __gash_migrate_screen "" "" "Welcome to Gash 2.0" \
        "Gash 2.0 changes a few defaults of 1.x and reads some settings differently. $total of them concern your setup, one screen each. Nothing is written until you have seen the final diff, and every file is backed up first (undo with: gash migrate --rollback)."
    if ((yes)); then
        pick=1
    elif ((auto)); then
        __gash_choose pick 1 "Review the changes now" "Remind me next time" "Keep everything as it is and don't ask again" || pick=2
    else
        __gash_choose pick 1 "Review the changes now" "Cancel" || pick=2
    fi
    case "$pick" in
        2)
            echo "  OK, nothing changed. Run 'gash migrate' whenever you like."
            return 0
            ;;
        3)
            __gash_migrate_mark_done
            echo "  OK, nothing changed. 'gash migrate' still works later."
            return 0
            ;;
    esac

    # One screen per item; decisions[item] = chosen option.
    local -A dec=()
    local item step=0 body
    for item in "${items[@]}"; do
        step=$((step + 1))
        case "$item" in
            gashrc)
                body="Your ~/.gashrc is the 1.x loader: it hard-codes ~/.gash and exports GASH_BASH and GASH_COMPLETION. The 2.0 loader finds Gash wherever it is installed (GASH_DIR) and skips completion when headless."
                if [[ -n "$gashrc_extras" ]]; then
                    body+=$'\n\n'"Your own lines are kept at the end of the new file:"$'\n'
                    while IFS= read -r a; do body+=$'\n'"- $a"; done <<<"$gashrc_extras"
                fi
                __gash_migrate_screen "$step" "$total" "The ~/.gashrc loader" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Use the 2.0 loader (recommended)" "Keep my ~/.gashrc as it is" || return 1
                ;;
            env)
                body="Gash 2.0 reads ~/.gash_env with a strict grammar, so a malformed line can never leak into a warning or a command."
                if [[ -n "$env_problems" ]]; then
                    body+=$'\n\n'"These lines are skipped from now on:"$'\n'
                    local ln why
                    while IFS=$'\t' read -r ln why; do body+=$'\n'"- line $ln: $why"; done <<<"$env_problems"
                fi
                __gash_migrate_env_open && body+=$'\n\n'"The file is readable by other users, and it holds passwords and tokens: the recommended fix also makes it private (chmod 600)."
                __gash_migrate_screen "$step" "$total" "Settings file ~/.gash_env" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Comment out the skipped lines and make the file private (recommended)" "Leave the file as it is" || return 1
                ;;
            tools)
                body="Gash 1.x replaced standard commands with alternatives when they were installed. On this machine that meant:"$'\n'
                while IFS= read -r a; do body+=$'\n'"- $a"; done <<<"$tool_swaps"
                body+=$'\n\n'"They take different flags and print differently, which surprises scripts and muscle memory, so 2.0 keeps the standard commands unless you ask (GASH_REPLACE_TOOLS=1)."
                __gash_migrate_screen "$step" "$total" "Command replacements" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Use the standard commands (recommended)" "Keep the replacements (GASH_REPLACE_TOOLS=1)" || return 1
                ;;
            umask)
                body="Gash 1.x forced umask 022 in every shell, so new files were readable by everyone. 2.0 leaves the system default alone, which is $cur_umask here. Set GASH_UMASK to choose your own."
                __gash_migrate_screen "$step" "$total" "File permissions (umask)" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Keep the system umask $cur_umask (recommended)" "Keep 022 as in 1.x (GASH_UMASK=022)" || return 1
                ;;
            scripts)
                body="These lines in your shell files use commands that behave differently in 2.0:"$'\n'
                local f n rule text fixable=0
                while IFS=$'\t' read -r f n rule text; do
                    body+=$'\n'"- ${f/#$HOME/\~}:$n  $text"
                    case "$rule" in
                        dryrun)
                            body+=$'\n'"  -n now means --limit everywhere; this line becomes --dry-run."
                            fixable=1
                            ;;
                        portkill) body+=$'\n'"  port_kill now stops only listening processes and asks first; add --force where nobody can answer." ;;
                        llmexec) body+=$'\n'"  llm_exec now runs read-only commands from an allowlist, in a child shell." ;;
                        gashvar) body+=$'\n'"  GASH_BASH and GASH_COMPLETION are no longer exported; use \$GASH_DIR/gash.sh." ;;
                    esac
                done <<<"$scripts"
                __gash_migrate_screen "$step" "$total" "Your shell files" "$body"
                if ((fixable)); then
                    ((yes)) && pick=1 || __gash_choose pick 1 "Fix the -n lines for me (recommended)" "Leave my files alone" || return 1
                else
                    ((yes)) && pick=2 || __gash_choose pick 1 "Continue (I'll edit them myself)" || return 1
                    pick=2
                fi
                ;;
            llm)
                body="llm_exec used to run any command line (minus a blocklist) inside your own shell. In 2.0 it runs in a child shell and only allows read-only commands (ls, cat, grep, git status, ...). Add your own with GASH_LLM_EXEC_ALLOW, or switch back to any command with GASH_LLM_EXEC=unrestricted."
                __gash_migrate_screen "$step" "$total" "Agents: llm_exec" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Use the read-only allowlist (recommended)" "Allow any command as in 1.x (GASH_LLM_EXEC=unrestricted)" || return 1
                ;;
            ai)
                body="Gash 1.x sent data to the AI provider without asking. 2.0 redacts secrets and, by default, asks before sending data that Gash collected itself (ai_sysinfo). Your own questions go straight through."
                __gash_migrate_screen "$step" "$total" "AI: asking before sending" "$body"
                ((yes)) && pick=1 || __gash_choose pick 1 "Ask before sending collected data (recommended)" "Ask before every request (GASH_AI_CONFIRM=1)" "Never ask (GASH_AI_CONFIRM=0)" || return 1
                ;;
        esac
        dec[$item]="$pick"
    done

    # Build the new files in the work dir.
    local -a touched=()
    local env_new="$work/env"
    [[ -f "$env_file" ]] && cp -p -- "$env_file" "$env_new"
    local env_changed=0
    if [[ "${dec[gashrc]-}" == 1 ]]; then
        __gash_migrate_new_gashrc "$work/gashrc" "$gashrc_extras"
        if bash -n "$work/gashrc" 2>/dev/null; then
            touched+=("$gashrc|$work/gashrc")
        else
            __gash_warning "The new ~/.gashrc would not parse with your lines added; keeping the 1.x file."
        fi
    fi
    if [[ "${dec[env]-}" == 1 ]]; then
        local nums=""
        [[ -n "$env_problems" ]] && nums="$(cut -f1 <<<"$env_problems" | tr '\n' ' ')"
        [[ -n "$nums" ]] && __gash_migrate_comment_lines "$env_new" "$nums"
        env_changed=1
    fi
    [[ "${dec[tools]-}" == 2 ]] && __gash_migrate_set_in "$env_new" GASH_REPLACE_TOOLS 1 && env_changed=1
    [[ "${dec[umask]-}" == 2 ]] && __gash_migrate_set_in "$env_new" GASH_UMASK 022 && env_changed=1
    [[ "${dec[llm]-}" == 2 ]] && __gash_migrate_set_in "$env_new" GASH_LLM_EXEC unrestricted && env_changed=1
    [[ "${dec[ai]-}" == 2 ]] && __gash_migrate_set_in "$env_new" GASH_AI_CONFIRM 1 && env_changed=1
    [[ "${dec[ai]-}" == 3 ]] && __gash_migrate_set_in "$env_new" GASH_AI_CONFIRM 0 && env_changed=1
    if ((env_changed)); then
        if [[ -f "$env_file" ]] && cmp -s "$env_file" "$env_new" && ! __gash_migrate_env_open; then
            :
        elif [[ -f "$env_new" ]]; then
            touched+=("$env_file|$env_new")
        fi
    fi
    if [[ "${dec[scripts]-}" == 1 ]]; then
        local f n rule text i=0
        local -A fixnums=()
        while IFS=$'\t' read -r f n rule text; do
            [[ "$rule" == dryrun ]] && fixnums[$f]+="$n "
        done <<<"$scripts"
        for f in "${!fixnums[@]}"; do
            i=$((i + 1))
            cp -p -- "$f" "$work/script$i"
            __gash_migrate_fix_dryrun "$work/script$i" "${fixnums[$f]}"
            touched+=("$f|$work/script$i")
        done
    fi

    # Preview and confirm.
    if __gash_migrate_tui; then __gash_clear_screen; fi
    echo
    if ((${#touched[@]} == 0)); then
        __gash_heading --indent 2 "No file needs to change."
        echo
    else
        __gash_heading --indent 2 "These changes will be written:"
        echo
        local t path
        for t in "${touched[@]}"; do
            if cmp -s "${t%%|*}" "${t#*|}"; then
                path="${t%%|*}"
                printf '  %s: content unchanged, made private (chmod 600)\n' "${path/#$HOME/\~}"
            else
                path="${t%%|*}"
                __gash_migrate_diff "$path" "${t#*|}" "${path/#$HOME/\~}"
            fi
            echo
        done
        if ((! yes)); then
            __gash_choose pick 1 "Write these changes" "Cancel, write nothing" || pick=2
            if [[ "$pick" != 1 ]]; then
                echo "  Nothing written. Run 'gash migrate' to start over."
                return 0
            fi
        fi
    fi

    # Back up, then write.
    local state_dir stamp backup=""
    state_dir="$(__gash_migrate_state_dir)"
    if ((${#touched[@]} > 0)); then
        stamp="$(date +%Y%m%d-%H%M%S)"
        backup="$state_dir/backups/$stamp"
        mkdir -p "$backup" || {
            __gash_error "Cannot create the backup directory ${backup/#$HOME/\~}; nothing written"
            return 1
        }
        local path name k=0
        for t in "${touched[@]}"; do
            path="${t%%|*}"
            k=$((k + 1))
            name="$k-${path##*/}"
            if [[ -f "$path" ]]; then
                cp -p -- "$path" "$backup/$name" || {
                    __gash_error "Backup of ${path/#$HOME/\~} failed; nothing written"
                    return 1
                }
                printf 'present\t%s\t%s\n' "$path" "$name" >>"$backup/manifest"
            else
                printf 'absent\t%s\t%s\n' "$path" "$name" >>"$backup/manifest"
            fi
        done
        for t in "${touched[@]}"; do
            path="${t%%|*}"
            cat -- "${t#*|}" >"$path" || {
                __gash_error "Writing ${path/#$HOME/\~} failed; undo with: gash migrate --rollback"
                return 1
            }
            [[ "$path" == "$env_file" ]] && chmod 600 "$path"
        done
    fi
    __gash_migrate_mark_done

    # Summary.
    echo
    __gash_success "Gash is now configured for 2.0."
    if [[ -n "$backup" ]]; then
        for t in "${touched[@]}"; do
            path="${t%%|*}"
            __gash_list --indent 4 "${path/#$HOME/\~}"
        done
        printf '  Backups in %s; undo with: gash migrate --rollback\n' "${backup/#$HOME/\~}"
    fi
    echo
    __gash_heading --indent 2 "Also new in 2.0, nothing to configure:"
    __gash_list --indent 4 \
        "port_kill stops only listening processes and asks first (--force skips)." \
        "remote_* commands ask before touching ~/.ssh on the other side." \
        "-n means --limit everywhere; dry runs are --dry-run." \
        "llm_git_status reports conflicted files; llm_tree marks truncated output." \
        "Your EDITOR, HISTCONTROL and PROMPT_COMMAND are respected." \
        "Every command answers --help without side effects; 'gash' shows the card."
    printf '\n  %s\n\n' "Open a new terminal to load the new configuration."
    return 0
}

if declare -p __GASH_HELP_REGISTRY &>/dev/null; then

    __gash_register_help "gash_migrate" \
        --module "gash" \
        --short "Guided migration of a Gash 1.x configuration to 2.0" \
        --see-also "gash gash_doctor" \
        <<'HELP'
USAGE
  gash_migrate [--yes] [--plain]
  gash_migrate --rollback [--yes]
  gash_migrate --status
  (also: gash migrate ...)

  Walks through the Gash 2.0 changes that concern your setup, one screen
  each: the ~/.gashrc loader, ~/.gash_env lines 2.0 skips, command
  replacements (GASH_REPLACE_TOOLS), umask, lines of your shell files that
  use changed commands, llm_exec and AI consent. You see a diff of every
  file before anything is written; each file is backed up first.

  It starts by itself in the first interactive shell after the upgrade.
  In scripts and headless shells Gash only prints a one-line reminder on
  stderr, at most once a day.

OPTIONS
  --yes         Take the recommended choice everywhere, no questions
  --plain       Numbered prompts instead of arrow-key menus
  --rollback    Restore the files saved by the last migration
  --status      Schema version, pending migration and backups

FILES
  ~/.local/state/gash/schema     configuration schema version
  ~/.local/state/gash/backups/   one folder per migration, with a manifest
  (under $XDG_STATE_HOME/gash when that is set)

EXAMPLES
  # Review the changes again
  gash migrate

  # Undo the last migration
  gash migrate --rollback
HELP

fi
