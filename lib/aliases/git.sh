#!/usr/bin/env bash

# Gash Aliases: Git
# Git workflow aliases for common operations.

if command -v git >/dev/null 2>&1; then

    # =========================================================================
    # Git Log - Professional visualization with multiple variants
    # =========================================================================

    # Pretty format definitions (reusable)
    __GASH_GIT_LOG_FORMAT='%C(auto)%h%d %s %C(dim)(%cr) %C(cyan)<%an>%C(reset)'
    __GASH_GIT_LOG_FORMAT_FULL='%C(auto)%h %C(bold blue)%an%C(reset) %C(dim)%ar%C(reset)%C(auto)%d%C(reset)%n  %s%n'

    # Main git log function with help and variants
    gl() {
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
        git log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit "$@"
    }

    # All branches
    gla() {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        git log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit --all "$@"
    }

    # Ultra-compact oneline
    glo() {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        git log --oneline --decorate --graph "$@"
    }

    # Graph focused (first-parent for cleaner merge view)
    glg() {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        git log --graph --pretty=format:"$__GASH_GIT_LOG_FORMAT" --abbrev-commit --first-parent "$@"
    }

    # With file statistics
    gls() {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        git log --pretty=format:"$__GASH_GIT_LOG_FORMAT_FULL" --stat "$@"
    }

    # File-specific log with patches
    glf() {
        if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
            gl --help
            return 0
        fi
        if [[ -z "${1-}" ]]; then
            echo "Usage: glf <file> [options]" >&2
            echo "Shows commit history for a specific file with patches" >&2
            return 1
        fi
        git log --follow -p -- "$@"
    }

    # Legacy alias (backward compatibility)
    alias glog='gl'

    # =========================================================================
    # Status
    # =========================================================================
    alias gst='git status'
    alias gs='git status -sb'  # Short status with branch info

    # =========================================================================
    # Add
    # =========================================================================
    alias ga='git add'
    alias gaa='git add --all'
    alias gap='git add --patch'  # Interactive staging

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
    alias gp='git push'
    alias gpf='git push --force-with-lease'  # Safer force push
    alias gpl='git pull'
    alias gplr='git pull --rebase'

    # =========================================================================
    # Checkout / Switch
    # =========================================================================
    alias gco='git checkout'
    alias gcb='git checkout -b'  # Create and switch to new branch
    alias gsw='git switch'       # Modern branch switching
    alias gswc='git switch -c'   # Create and switch (modern)

    # =========================================================================
    # Branch
    # =========================================================================
    alias gb='git branch'
    alias gba='git branch -a'    # All branches (including remote)
    alias gbd='git branch -d'    # Delete branch (safe)
    alias gbD='git branch -D'    # Force delete branch

    # =========================================================================
    # Diff
    # =========================================================================
    alias gd='git diff'
    alias gds='git diff --staged'  # Diff staged changes
    alias gdw='git diff --word-diff'  # Word-level diff

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
    alias grh1='git reset HEAD~1'  # Undo last commit (keep changes)
    alias grhh='git reset --hard HEAD'

    # =========================================================================
    # Rebase
    # =========================================================================
    alias grb='git rebase'
    alias grbc='git rebase --continue'
    alias grba='git rebase --abort'

    # =========================================================================
    # Tags (link to gash functions if available)
    # =========================================================================
    if declare -f gtags >/dev/null 2>&1; then
        alias gt='gtags'
    fi
    if declare -f gadd_tag >/dev/null 2>&1; then
        alias gta='gadd_tag'
    fi
    if declare -f gdel_tag >/dev/null 2>&1; then
        alias gtd='gdel_tag'
    fi

fi
