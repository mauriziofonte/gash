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
        echo "*******************************************************" > "$log"
        git log -1 --pretty=format:"%s%nAuthored by %an at %ai%n%n%b%n" "$commit" >> "$log"
        echo "*******************************************************" >> "$log"

        # Save the actual file content for the commit
        git show "$commit:./$file" > "$out"

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
    git diff-index "$commit_hash" --binary > "$patch_file" || {
        __gash_error "Failed to create patch file."
        return 1
    }

    __gash_step 4 4 "Applying patch to '$main_branch'..."
    git checkout "$main_branch" && git apply --3way "$patch_file" || {
        __gash_error "Failed to apply the patch."
        return 1
    }

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

    # Check if tag already exists
    if git rev-parse "$tag_name" >/dev/null 2>&1 || git ls-remote --tags origin | grep -qF "refs/tags/$tag_name"; then
        __gash_error "Tag '$tag_name' already exists."
        return 1
    fi

    # Create and push the tag
    git tag -a "$tag_name" -m "$tag_message"
    __gash_success "Tag '$tag_name' created."

    git push origin "$tag_name"
    __gash_success "Tag '$tag_name' pushed to remote."

    return 0
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

    # Check if tag exists
    if ! git rev-parse "$tag_name" >/dev/null 2>&1 && ! git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        __gash_error "Tag '$tag_name' does not exist."
        return 1
    fi

    # Delete local tag if exists
    if git rev-parse "$tag_name" >/dev/null 2>&1; then
        git tag -d "$tag_name" && __gash_success "Local tag '$tag_name' deleted."
    fi

    # Delete remote tag if exists
    if git ls-remote --tags origin | grep -qF "refs/tags/$tag_name"; then
        git push origin --delete "$tag_name" && __gash_success "Remote tag '$tag_name' deleted."
    fi

    return 0
}

# -----------------------------------------------------------------------------
# Short Aliases
# -----------------------------------------------------------------------------
alias glt='git_list_tags'
alias gat='git_add_tag'
alias gdt='git_delete_tag'
alias gdr='git_dump_revisions'
alias gpatch='git_apply_patch'
