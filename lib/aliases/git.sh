#!/usr/bin/env bash
# shellcheck disable=SC2139 # aliases intentionally expand values at definition time

# Gash Aliases: Git
# Git workflow aliases for common operations.

if command -v git >/dev/null 2>&1; then

    # =========================================================================
    # Git Log - Professional visualization with multiple variants
    # =========================================================================

    # Pretty formats in the Gash palette (256-color numbers of
    # lib/core/output.sh): hash accent (214), refs primary (39), date and
    # author muted (245), bold for emphasis. Git drops them when its output
    # is not a terminal; __gash_git_log drops them for NO_COLOR too.
    __GASH_GIT_LOG_FORMAT='%C(214)%h%C(reset)%C(39)%d%C(reset) %s %C(245)(%cr) <%an>%C(reset)'
    __GASH_GIT_LOG_FORMAT_FULL='%C(214)%h%C(reset) %C(bold)%an%C(reset) %C(245)%ar%C(reset)%C(39)%d%C(reset)%n  %s%n'
    __GASH_GIT_LOG_FORMAT_ONELINE='%C(214)%h%C(reset)%C(39)%d%C(reset) %s'

    # git log with the Gash palette: graph lanes alternate primary and muted
    # unless the user configured log.graphColors; no colors at all when
    # NO_COLOR or GASH_NO_COLOR is set.
    __gash_git_log() {
        local -a pre=() post=()
        if ! __gash_color_env_ok; then
            post=(--no-color)
        elif ! git config --get log.graphColors >/dev/null 2>&1; then
            pre=(-c "log.graphColors=39,245")
        fi
        git "${pre[@]}" log "${post[@]}" "$@"
    }

    # Main git log function with help and variants. Defined with the
    # `function` keyword: a user alias with the same name would otherwise be
    # expanded inside `name() {` and break the whole file.
    # Compact log with graph (gl --help: all variants)
    function gl {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            cat <<'EOF'
Git Log Aliases - Professional log visualization

USAGE:
  gl [options]      Compact log with graph (current branch)
  gla [options]     All branches with graph
  glo [options]     Ultra-compact oneline format
  glg [options]     Graph focused (first-parent only)
  gls [options]     With file statistics
  glf [file]        Log for specific file with patches

EXAMPLES:
  gl -10            Show last 10 commits
  gl --since=1.week Show commits from last week
  gla               Show all branches
  gls -5            Last 5 commits with changed files
  glf src/main.js   History of specific file

OPTIONS (passed to git log):
  -n, -<number>     Limit number of commits
  --since=<date>    Show commits after date
  --until=<date>    Show commits before date
  --author=<name>   Filter by author
  --grep=<pattern>  Filter by commit message
  --no-merges       Exclude merge commits
  -p, --patch       Show patches (diffs)

TIPS:
  • Use 'q' to exit the pager
  • Combine with grep: gl | grep "fix"
  • Date formats: "2024-01-01", "1 week ago", "yesterday"
EOF
            return 0
        fi
        __gash_git_log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit "$@"
    }

    # All branches
    function gla {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        __gash_git_log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit --all "$@"
    }

    # Ultra-compact oneline
    function glo {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        __gash_git_log --graph --pretty=tformat:"$__GASH_GIT_LOG_FORMAT_ONELINE" "$@"
    }

    # Graph focused (first-parent for cleaner merge view)
    function glg {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        __gash_git_log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit --first-parent "$@"
    }

    # With file statistics
    function gls {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        __gash_git_log --pretty=format:"$__GASH_GIT_LOG_FORMAT_FULL" --stat "$@"
    }

    # File-specific log with patches
    function glf {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        if [[ -z "${1-}" ]]; then
            __gash_usage "glf <file> [options]"
            __gash_note "Shows commit history for a specific file with patches"
            return 1
        fi
        __gash_git_log --follow -p -- "$@"
    }

    # Legacy alias (backward compatibility)
    alias glog='gl'

    # =========================================================================
    # Status
    # =========================================================================
    alias gst='git status'
    # Short status with branch info; skipped when Ghostscript owns `gs`.
    if ! type -P gs >/dev/null 2>&1; then
        alias gs='git status -sb'
    fi

    # =========================================================================
    # Add
    # =========================================================================
    alias ga='git add'
    alias gaa='git add --all'
    alias gap='git add --patch' # Interactive staging

    # =========================================================================
    # Commit
    # =========================================================================
    alias gc='git commit'
    alias gcm='git commit -m'
    alias gca='git commit --amend'
    alias gcan='git commit --amend --no-edit'

    # =========================================================================
    # Push / Pull
    # =========================================================================
    alias gp='git pull'
    alias gpr='git pull --rebase'
    alias gpsh='git push'
    alias gpf='git push --force-with-lease' # Safer force push

    # =========================================================================
    # Checkout / Switch
    # =========================================================================
    alias gco='git checkout'
    alias gcb='git checkout -b' # Create and switch to new branch
    alias gsw='git switch'      # Modern branch switching
    alias gswc='git switch -c'  # Create and switch (modern)

    # =========================================================================
    # Branch
    # =========================================================================
    alias gb='git branch'
    alias gba='git branch -a' # All branches (including remote)
    alias gbd='git branch -d' # Delete branch (safe)
    alias gbD='git branch -D' # Force delete branch

    # =========================================================================
    # Diff
    # =========================================================================
    alias gd='git diff'
    alias gds='git diff --staged'    # Diff staged changes
    alias gdw='git diff --word-diff' # Word-level diff

    # =========================================================================
    # Stash
    # =========================================================================
    alias gsh='git stash'
    alias gshp='git stash pop'
    alias gshl='git stash list'
    alias gsha='git stash apply'

    # =========================================================================
    # Remote
    # =========================================================================
    alias gf='git fetch'
    alias gfa='git fetch --all --prune'
    alias gr='git remote -v'

    # =========================================================================
    # Reset
    # =========================================================================
    alias grh='git reset HEAD'
    alias grh1='git reset HEAD~1' # Undo last commit (keep changes)
    alias grhh='git reset --hard HEAD'

    # =========================================================================
    # Rebase
    # =========================================================================
    alias grb='git rebase'
    alias grbc='git rebase --continue'
    alias grba='git rebase --abort'

fi
