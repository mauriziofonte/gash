#!/usr/bin/env bash

# Gash Module: Git Operations
# Functions for git repository management, tags, and patching.
#
# Dependencies: core/output.sh, core/validation.sh, core/utils.sh
#
# Public functions (LONG name + SHORT alias):
#   git_list_tags (glt)       - List all local and remote tags
#   git_add_tag (gat)         - Create and push annotated tag
#   git_delete_tag (gdt)      - Delete tag locally and on remote
#   git_dump_revisions (gdr)  - Dump all revisions of a file
#   git_apply_patch (gap)     - Apply patch from feature branch
#   git_repair (gheal)        - Repair a repo with corrupted 0-byte loose objects

# -----------------------------------------------------------------------------
# File History
# -----------------------------------------------------------------------------

# Dump all revisions of a file in a git repo into multiple separate files.
# Usage: git_dump_revisions FILENAME
# Alias: gdr
git_dump_revisions() {
    needs_help "git_dump_revisions" "git_dump_revisions FILENAME" \
        "Dump all revisions of a file in a GIT repo into multiple separate files. Example: git_dump_revisions path/to/somefile.txt. Alias: gdr" \
        "${1-}" && return

    local file="${1-}"

    __gash_require_arg "$file" "filename" "git_dump_revisions <filename>" || return 1
    __gash_require_git_repo || return 1
    __gash_require_file "$file" "File '$file' does not exist in the repository." || return 1

    local index=1

    for commit in $(git log --pretty=format:%h "$file"); do
        local padindex
        padindex=$(printf %03d "$index")
        local out="$file.$padindex.$commit"
        local log="$out.logmsg"

        __gash_info "Saving version $index to file $out for commit $commit"

        # Save commit log message in a separate log file
        echo "*******************************************************" >"$log"
        git log -1 --pretty=format:"%s%nAuthored by %an at %ai%n%n%b%n" "$commit" >>"$log"
        echo "*******************************************************" >>"$log"

        # Save the actual file content for the commit
        git show "$commit:./$file" >"$out"

        index=$((index + 1))
    done
}

# -----------------------------------------------------------------------------
# Patching
# -----------------------------------------------------------------------------

# Create and apply a patch from a feature branch to the main branch.
# Usage: git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH
# Alias: gap
git_apply_patch() {
    needs_help "git_apply_patch" "git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" \
        "Create and apply a patch from a feature branch to the main branch. Example: git_apply_patch main old-feat 123456. Alias: gap" \
        "${1-}" && return

    __gash_require_git_repo || return 1

    local main_branch="${1-}"
    local feature_branch="${2-}"
    local commit_hash="${3-}"

    # Validate arguments
    if [ -z "$main_branch" ] || [ -z "$feature_branch" ] || [ -z "$commit_hash" ]; then
        __gash_error "Missing arguments."
        needs_help "git_apply_patch" "git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" \
            "Create and apply a patch from a feature branch to the main branch." "--help"
        return 1
    fi

    # Check if branches exist
    if ! git show-ref --verify --quiet "refs/heads/$main_branch"; then
        __gash_error "Main branch '$main_branch' does not exist."
        return 1
    fi

    if ! git show-ref --verify --quiet "refs/heads/$feature_branch"; then
        __gash_error "Feature branch '$feature_branch' does not exist."
        return 1
    fi

    # Check if commit hash is valid
    if ! git cat-file -e "$commit_hash" 2>/dev/null; then
        __gash_error "Commit hash '$commit_hash' is not valid."
        return 1
    fi

    # Refuse to start over a dirty working tree: a failed 3-way apply would
    # mix conflict markers with pre-existing local changes, with no way back.
    if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
        __gash_error "Working tree is not clean. Commit or stash your changes before applying a patch."
        return 1
    fi

    # Start patching process
    __gash_step 1 4 "Checking out to main branch '$main_branch'..."
    git checkout "$main_branch" && git pull origin "$main_branch" || {
        __gash_error "Failed to checkout or pull the main branch."
        return 1
    }

    __gash_step 2 4 "Checking out to feature branch '$feature_branch'..."
    git checkout "$feature_branch" || {
        __gash_error "Failed to checkout the feature branch."
        return 1
    }

    __gash_step 3 4 "Generating patch from '$feature_branch' since commit '$commit_hash'..."
    local patch_file="${feature_branch}.patch"
    git diff-index "$commit_hash" --binary >"$patch_file" || {
        __gash_error "Failed to create patch file."
        rm -f "$patch_file"
        return 1
    }

    __gash_step 4 4 "Applying patch to '$main_branch'..."
    git checkout "$main_branch" || {
        __gash_error "Failed to checkout the main branch."
        rm -f "$patch_file"
        return 1
    }
    if ! git apply --3way "$patch_file"; then
        # Roll back: the tree was clean at entry, so a hard reset restores
        # exactly the pre-apply state (no user changes can be lost here).
        __gash_error "Failed to apply the patch. Rolling back working tree."
        git reset --hard HEAD >/dev/null 2>&1 || true
        __gash_info "Patch file kept for inspection: $patch_file"
        return 1
    fi

    rm -f "$patch_file"
    __gash_success "Patch applied successfully from '$feature_branch' to '$main_branch'."
}

# -----------------------------------------------------------------------------
# Tag Management
# -----------------------------------------------------------------------------

# List all tags (local and remote).
# Usage: git_list_tags
# Alias: glt
git_list_tags() {
    needs_help "git_list_tags" "git_list_tags" "Lists all local and remote tags. Alias: glt" "${1-}" && return

    __gash_require_git_repo_with_remote || return 1

    __gash_print "bold_blue" "Local tags:"
    git tag -l

    __gash_print "bold_blue" "Remote tags:"
    git ls-remote --tags origin | awk '{print $2}' | sed 's|refs/tags/||'

    return 0
}

# Create an annotated tag and push it to the remote.
# Usage: git_add_tag TAG_NAME ["TAG_MESSAGE"]
# Alias: gat
git_add_tag() {
    needs_help "git_add_tag" "git_add_tag <tag_name> \"<tag_message>\"" \
        "Creates an annotated tag and pushes it to remote. Alias: gat" \
        "${1-}" && return

    __gash_require_git_repo_with_remote || return 1
    __gash_require_arg "${1-}" "tag name" "git_add_tag <tag_name> [message]" || return 1

    local tag_name="${1-}"
    local tag_message="${2:-"Release $tag_name"}"
    __gash_git_check_tag_name "$tag_name" || return 1

    # Exact ref matches only: "v1.0" must not collide with "v1.0.1" or a
    # branch called v1.0.
    if __gash_git_local_tag "$tag_name" || __gash_git_remote_tag "$tag_name"; then
        __gash_error "Tag '$tag_name' already exists."
        return 1
    fi

    git tag -a "$tag_name" -m "$tag_message" || {
        __gash_error "Could not create tag '$tag_name'."
        return 1
    }
    __gash_success "Tag '$tag_name' created."

    git push origin "refs/tags/$tag_name" || {
        __gash_error "Tag '$tag_name' was created locally but could not be pushed."
        return 1
    }
    __gash_success "Tag '$tag_name' pushed to remote."
}

# Delete a tag locally and on the remote.
# Usage: git_delete_tag TAG_NAME
# Alias: gdt
git_delete_tag() {
    needs_help "git_delete_tag" "git_delete_tag <tag_name>" \
        "Deletes a tag both locally and on remote. Alias: gdt" \
        "${1-}" && return

    __gash_require_git_repo_with_remote || return 1
    __gash_require_arg "${1-}" "tag name" "git_delete_tag <tag_name>" || return 1

    local tag_name="${1-}"
    __gash_git_check_tag_name "$tag_name" || return 1

    local local_tag=0 remote_tag=0
    __gash_git_local_tag "$tag_name" && local_tag=1
    __gash_git_remote_tag "$tag_name" && remote_tag=1
    if ((local_tag == 0 && remote_tag == 0)); then
        __gash_error "Tag '$tag_name' does not exist."
        return 1
    fi

    local rc=0
    if ((local_tag)); then
        if git tag -d "$tag_name" >/dev/null; then
            __gash_success "Local tag '$tag_name' deleted."
        else
            rc=1
        fi
    fi
    if ((remote_tag)); then
        if git push origin --delete "refs/tags/$tag_name"; then
            __gash_success "Remote tag '$tag_name' deleted."
        else
            __gash_error "Could not delete remote tag '$tag_name'."
            rc=1
        fi
    fi
    return "$rc"
}

# Tag helpers: exact ref checks, never substring or pattern matches.
__gash_git_check_tag_name() {
    if ! git check-ref-format "refs/tags/${1-}" 2>/dev/null; then
        __gash_error "Invalid tag name: '${1-}'."
        return 1
    fi
}
__gash_git_local_tag() {
    git rev-parse -q --verify "refs/tags/${1-}" >/dev/null 2>&1
}
__gash_git_remote_tag() {
    git ls-remote --exit-code --tags origin "refs/tags/${1-}" >/dev/null 2>&1
}

# -----------------------------------------------------------------------------
# Repository Maintenance
# -----------------------------------------------------------------------------

# Repair a repo with corrupted (0-byte) loose objects by re-fetching them from
# the 'origin' remote through a temporary bare clone. Works in linked
# worktrees and submodules (the object store is asked of git, not assumed
# at .git/objects).
# Usage: git_repair [-y] [--dry-run]
# Alias: gheal
git_repair() {
    needs_help "git_repair" "git_repair [-y] [--dry-run]" \
        "Repair a repo with corrupted 0-byte loose objects by re-fetching them from origin. Alias: gheal" \
        "${1-}" && return

    local assume_yes=0 dry_run=0 arg
    for arg in "$@"; do
        case "$arg" in
            -y | --yes) assume_yes=1 ;;
            --dry-run) dry_run=1 ;;
            *)
                __gash_error "Unknown option: $arg (usage: git_repair [-y] [--dry-run])"
                return 1
                ;;
        esac
    done

    __gash_require_git_repo_with_remote || return 1

    local objects origin_url
    objects="$(git rev-parse --path-format=absolute --git-path objects 2>/dev/null)" ||
        objects="$(cd "$(git rev-parse --git-path objects)" && pwd -P)" || {
        __gash_error "Cannot locate the object store of this repository."
        return 1
    }
    origin_url="$(git remote get-url origin)"

    local -a empty=()
    mapfile -t empty < <(find "$objects" -type f -empty 2>/dev/null)

    printf 'Object store: %s
' "$objects"
    printf 'Empty (corrupted) object files: %d
' "${#empty[@]}"
    if ((${#empty[@]} == 0)); then
        if git fsck --connectivity-only >/dev/null 2>&1; then
            __gash_success "No empty objects and the history is connected: nothing to repair."
            return 0
        fi
        printf 'git fsck reports problems that are not empty files; trying the repair anyway.
'
    fi
    printf 'Plan: delete the empty files, copy the packs of a fresh bare clone of %s,
' "$origin_url"
    printf '      run git gc and git fsck --full.
'
    ((dry_run)) && return 0
    __gash_confirm_plan "$assume_yes" || return 1

    local tmp_dir
    tmp_dir=$(mktemp -d) || {
        __gash_error "Failed to create temporary directory."
        return 1
    }
    trap 'command rm -rf "$tmp_dir" >/dev/null 2>&1; trap - RETURN' RETURN

    __gash_step 1 5 "Removing ${#empty[@]} corrupted (0-byte) loose object(s)..."
    ((${#empty[@]})) && { rm -f -- "${empty[@]}" || return 1; }

    __gash_step 2 5 "Cloning '$origin_url' into a temporary bare repo..."
    if ! git clone --quiet --bare "$origin_url" "$tmp_dir/recovery.git"; then
        __gash_error "Failed to clone '$origin_url'."
        return 1
    fi

    # A fresh clone keeps its objects in packs; pack names are content
    # hashes, so a pack already present is the same pack.
    __gash_step 3 5 "Copying the recovered packs into the repo..."
    local f copied=0
    mkdir -p "$objects/pack" || return 1
    for f in "$tmp_dir/recovery.git/objects/pack"/pack-*; do
        [[ -f "$f" ]] || continue
        [[ -e "$objects/pack/${f##*/}" ]] && continue
        cp "$f" "$objects/pack/" || {
            __gash_error "Copying ${f##*/} into $objects/pack failed."
            return 1
        }
        copied=$((copied + 1))
    done
    # A local clone may keep loose objects (hard links): copy those too.
    for f in "$tmp_dir/recovery.git/objects"/??/*; do
        [[ -f "$f" ]] || continue
        f="${f#"$tmp_dir/recovery.git/objects/"}"
        [[ -e "$objects/$f" ]] && continue
        mkdir -p "$objects/${f%/*}" && cp "$tmp_dir/recovery.git/objects/$f" "$objects/$f" || {
            __gash_error "Copying object $f failed."
            return 1
        }
    done
    __gash_info "Copied $copied pack file(s)."

    __gash_step 4 5 "Running garbage collection..."
    git gc --quiet || __gash_warning "git gc reported a problem; checking integrity anyway."

    __gash_step 5 5 "Verifying repo integrity..."
    if git fsck --full; then
        __gash_success "Repo repaired and verified clean."
    else
        __gash_error "Repo still reports issues after repair. Manual inspection needed."
        return 1
    fi
}

# -----------------------------------------------------------------------------
# Sync
# -----------------------------------------------------------------------------

# Stage every change (new, modified, deleted files), commit it and push the
# current branch: the "save everything and send it" step of a personal repo.
# Refuses on a detached HEAD and in the middle of a merge, rebase,
# cherry-pick or revert. The first push of a branch sets its upstream on
# origin.
# Usage: git_sync [-m MESSAGE] [--no-push] [--dry-run]
# Alias: gsync
git_sync() {
    needs_help "git_sync" "git_sync [-m MESSAGE] [--no-push] [--dry-run]" \
        "Stage everything, commit it and push the current branch. Alias: gsync" \
        "${1-}" && return

    local msg="chore: sync" push=1 dry_run=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -m | --message)
                if [[ $# -lt 2 || -z "$2" ]]; then
                    __gash_error "$1 needs a message."
                    return 1
                fi
                msg="$2"
                shift
                ;;
            --no-push) push=0 ;;
            --dry-run) dry_run=1 ;;
            *)
                __gash_error "Unknown option: $1 (usage: git_sync [-m MESSAGE] [--no-push] [--dry-run])"
                return 1
                ;;
        esac
        shift
    done

    __gash_require_git_repo || return 1

    local branch
    branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null)" || {
        __gash_error "Detached HEAD: check out a branch first."
        return 1
    }

    local state what
    for state in MERGE_HEAD:merge CHERRY_PICK_HEAD:cherry-pick REVERT_HEAD:revert rebase-merge:rebase rebase-apply:rebase; do
        what="${state#*:}"
        if [[ -e "$(git rev-parse --git-path "${state%%:*}")" ]]; then
            __gash_error "A $what is in progress: finish or abort it first (git $what --continue or --abort)."
            return 1
        fi
    done

    local upstream="" remote=""
    upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null)" || upstream=""
    if ((push)) && [[ -z "$upstream" ]]; then
        if ! git remote get-url origin >/dev/null 2>&1; then
            __gash_error "Branch '$branch' has no upstream and there is no 'origin' remote: use --no-push or add a remote."
            return 1
        fi
        remote=origin
    fi

    local changes
    changes="$(git status --porcelain --untracked-files=all)"

    if ((dry_run)); then
        if [[ -n "$changes" ]]; then
            printf 'Would commit on %s with message "%s":\n' "$branch" "$msg"
            git status --short --untracked-files=all
        else
            printf 'Nothing to commit on %s.\n' "$branch"
        fi
        if ((push)); then
            if [[ -n "$upstream" ]]; then
                printf 'Would push %s to %s.\n' "$branch" "$upstream"
            else
                printf 'Would push %s to origin/%s and set it as upstream.\n' "$branch" "$branch"
            fi
        fi
        return 0
    fi

    if [[ -n "$changes" ]]; then
        git add --all || return 1
        git commit --quiet -m "$msg" || return 1
        __gash_success "Committed $(git rev-parse --short HEAD) on $branch: $msg"
    else
        __gash_info "Nothing to commit on $branch."
    fi

    ((push)) || return 0
    if [[ -n "$upstream" ]]; then
        if [[ "$(git rev-list --count '@{upstream}..HEAD' 2>/dev/null)" == "0" ]]; then
            __gash_info "$branch is already on $upstream."
            return 0
        fi
        git push --quiet || return 1
        __gash_success "Pushed $branch to $upstream."
    else
        git push --quiet -u "$remote" "$branch" || return 1
        __gash_success "Pushed $branch to $remote/$branch (upstream set)."
    fi
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias glt='git_list_tags'
alias gat='git_add_tag'
alias gdt='git_delete_tag'
alias gdr='git_dump_revisions'
alias gpatch='git_apply_patch'
alias gheal='git_repair'
alias gsync='git_sync'

# =============================================================================
# Help Registration
# =============================================================================

if declare -p __GASH_HELP_REGISTRY &>/dev/null 2>&1; then

    __gash_register_help "git_list_tags" \
        --aliases "glt" \
        --module "git" \
        --short "List all local and remote tags" \
        --see-also "git_add_tag git_delete_tag" \
        <<'HELP'
USAGE
  git_list_tags

EXAMPLES
  # List all tags in the current repo
  glt

  # Check existing tags before creating a new release
  glt && gat v2.0.0 "Major release"
HELP

    __gash_register_help "git_add_tag" \
        --aliases "gat" \
        --module "git" \
        --short "Create an annotated tag and push to remote" \
        --see-also "git_list_tags git_delete_tag" \
        <<'HELP'
USAGE
  git_add_tag TAG_NAME ["TAG_MESSAGE"]

EXAMPLES
  # Create a tag with auto-generated message
  gat v1.0.0

  # Create a tag with a custom message
  gat v2.0.0 "Major release with breaking changes"

  # Release workflow: commit, tag, done
  gcm "prepare release" && gat v1.2.3 "Bugfix release"

NOTES
  Creates an annotated tag and pushes it to the remote
  in one step. Requires a clean git repo with a remote.
HELP

    __gash_register_help "git_delete_tag" \
        --aliases "gdt" \
        --module "git" \
        --short "Delete a tag locally and on remote" \
        --see-also "git_list_tags git_add_tag" \
        <<'HELP'
USAGE
  git_delete_tag TAG_NAME

EXAMPLES
  # Remove a mis-tagged release
  gdt v1.0.0-beta

  # Fix a tag: delete and recreate
  gdt v1.0.0 && gat v1.0.0 "Fixed release"

NOTES
  Deletes the tag both locally and on the remote origin.
HELP

    __gash_register_help "git_dump_revisions" \
        --aliases "gdr" \
        --module "git" \
        --short "Dump all revisions of a file into separate files" \
        --see-also "git_apply_patch" \
        <<'HELP'
USAGE
  git_dump_revisions FILENAME

EXAMPLES
  # Trace how a config file evolved over time
  gdr path/to/config.yml

  # Dump revisions of a deployment script
  gdr scripts/deploy.sh

NOTES
  Creates files named: FILENAME.001.HASH, FILENAME.001.HASH.logmsg
  Each revision is dumped with its commit log message alongside.
HELP

    __gash_register_help "git_apply_patch" \
        --aliases "gpatch" \
        --module "git" \
        --short "Apply a patch from a feature branch to main" \
        --see-also "git_dump_revisions" \
        <<'HELP'
USAGE
  git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH

EXAMPLES
  # Cherry-pick a commit from feature to main via patch
  gpatch main feature-auth abc1234

  # Apply a specific fix from an old branch
  gpatch develop hotfix-db e5f6a7b

NOTES
  Steps: checks out main, creates a patch from the specified commit
  on the feature branch, applies it, and returns you to main.
  The commit hash must exist on the feature branch.
HELP

    __gash_register_help "git_repair" \
        --aliases "gheal" \
        --module "git" \
        --short "Repair a repo with corrupted 0-byte loose objects" \
        --see-also "git_list_tags" \
        <<'HELP'
USAGE
  git_repair [-y] [--dry-run]

EXAMPLES
  # Fix a repo after a crash/OOM left empty loose object files
  gheal

  # Only count the empty objects and show the plan
  gheal --dry-run

NOTES
  Deletes 0-byte loose objects, copies the packs of a temporary bare
  clone of 'origin' into the object store, then runs 'git gc' and
  'git fsck --full' to verify. A plain 'git fetch' cannot do this: the
  refs already match, so git believes it has the objects. Works in
  linked worktrees and submodules. Shows the plan and asks first; -y
  skips the question. Requires a git repo with an 'origin' remote.
HELP

    __gash_register_help "git_sync" \
        --aliases "gsync" \
        --module "git" \
        --short "Stage everything, commit and push the current branch" \
        --see-also "git_repair" \
        <<'HELP'
USAGE
  git_sync [-m MESSAGE] [--no-push] [--dry-run]

EXAMPLES
  # Save everything with the default message "chore: sync" and push
  gsync

  # With a real message
  gsync -m "docs: notes from the meeting"

  # What would be committed and where it would go
  gsync --dry-run

NOTES
  Stages new, modified and deleted files of the whole work tree
  (git add --all), commits only when there is something to commit, then
  pushes the current branch to its upstream. A branch without upstream is
  pushed to origin and the upstream is set. Refuses on a detached HEAD
  and during a merge, rebase, cherry-pick or revert. .gitignore applies:
  check `git status` first in repositories without one.
HELP

fi # end help registration guard
