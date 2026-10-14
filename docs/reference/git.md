# git

Tags, patches, repository repair, git shortcuts.

_Generated from the help registry by `tools/gen-docs.sh`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`git_add_tag`](#git-add-tag) | `gat` | Create an annotated tag and push to remote |
| [`git_apply_patch`](#git-apply-patch) | `gpatch` | Apply a patch from a feature branch to main |
| [`git_delete_tag`](#git-delete-tag) | `gdt` | Delete a tag locally and on remote |
| [`git_dump_revisions`](#git-dump-revisions) | `gdr` | Dump all revisions of a file into separate files |
| [`git_list_tags`](#git-list-tags) | `glt` | List all local and remote tags |
| [`git_repair`](#git-repair) | `gheal` | Repair a repo with corrupted 0-byte loose objects |

## git_add_tag

Create an annotated tag and push to remote

Alias: `gat`

#### Usage

```text
git_add_tag TAG_NAME ["TAG_MESSAGE"]
```

#### Examples

```bash
# Create a tag with auto-generated message
gat v1.0.0

# Create a tag with a custom message
gat v2.0.0 "Major release with breaking changes"

# Release workflow: commit, tag, done
gcm "prepare release" && gat v1.2.3 "Bugfix release"
```

#### Notes

```text
Creates an annotated tag and pushes it to the remote
in one step. Requires a clean git repo with a remote.
```

See also: `git_list_tags`, `git_delete_tag`

## git_apply_patch

Apply a patch from a feature branch to main

Alias: `gpatch`

#### Usage

```text
git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH
```

#### Examples

```bash
# Cherry-pick a commit from feature to main via patch
gpatch main feature-auth abc1234

# Apply a specific fix from an old branch
gpatch develop hotfix-db e5f6a7b
```

#### Notes

```text
Steps: checks out main, creates a patch from the specified commit
on the feature branch, applies it, and returns you to main.
The commit hash must exist on the feature branch.
```

See also: `git_dump_revisions`

## git_delete_tag

Delete a tag locally and on remote

Alias: `gdt`

#### Usage

```text
git_delete_tag TAG_NAME
```

#### Examples

```bash
# Remove a mis-tagged release
gdt v1.0.0-beta

# Fix a tag: delete and recreate
gdt v1.0.0 && gat v1.0.0 "Fixed release"
```

#### Notes

```text
Deletes the tag both locally and on the remote origin.
```

See also: `git_list_tags`, `git_add_tag`

## git_dump_revisions

Dump all revisions of a file into separate files

Alias: `gdr`

#### Usage

```text
git_dump_revisions FILENAME
```

#### Examples

```bash
# Trace how a config file evolved over time
gdr path/to/config.yml

# Dump revisions of a deployment script
gdr scripts/deploy.sh
```

#### Notes

```text
Creates files named: FILENAME.001.HASH, FILENAME.001.HASH.logmsg
Each revision is dumped with its commit log message alongside.
```

See also: `git_apply_patch`

## git_list_tags

List all local and remote tags

Alias: `glt`

#### Usage

```text
git_list_tags
```

#### Examples

```bash
# List all tags in the current repo
glt

# Check existing tags before creating a new release
glt && gat v2.0.0 "Major release"
```

See also: `git_add_tag`, `git_delete_tag`

## git_repair

Repair a repo with corrupted 0-byte loose objects

Alias: `gheal`

#### Usage

```text
git_repair
```

#### Examples

```bash
# Fix a repo after a crash/OOM left empty loose object files
gheal
```

#### Notes

```text
Deletes 0-byte loose objects, re-fetches them from 'origin' via a
temporary bare clone, then runs 'git gc --prune=now' and 'git fsck
--full' to verify. Requires a git repo with a configured remote.
```

See also: `git_list_tags`

## Aliases

| Alias | Runs |
|---|---|
| `gl` | `Compact log with graph (gl --help: all variants)` |
| `gla` | `All branches` |
| `glo` | `Ultra-compact oneline` |
| `glg` | `Graph focused (first-parent for cleaner merge view)` |
| `gls` | `With file statistics` |
| `glf` | `File-specific log with patches` |
| `glog` | `gl` |
| `gst` | `git status` |
| `gs` | `git status -sb` |
| `ga` | `git add` |
| `gaa` | `git add --all` |
| `gap` | `git add --patch` |
| `gc` | `git commit` |
| `gcm` | `git commit -m` |
| `gca` | `git commit --amend` |
| `gcan` | `git commit --amend --no-edit` |
| `gp` | `git pull` |
| `gpr` | `git pull --rebase` |
| `gpsh` | `git push` |
| `gpf` | `git push --force-with-lease` |
| `gco` | `git checkout` |
| `gcb` | `git checkout -b` |
| `gsw` | `git switch` |
| `gswc` | `git switch -c` |
| `gb` | `git branch` |
| `gba` | `git branch -a` |
| `gbd` | `git branch -d` |
| `gbD` | `git branch -D` |
| `gd` | `git diff` |
| `gds` | `git diff --staged` |
| `gdw` | `git diff --word-diff` |
| `gsh` | `git stash` |
| `gshp` | `git stash pop` |
| `gshl` | `git stash list` |
| `gsha` | `git stash apply` |
| `gf` | `git fetch` |
| `gfa` | `git fetch --all --prune` |
| `gr` | `git remote -v` |
| `grh` | `git reset HEAD` |
| `grh1` | `git reset HEAD~1` |
| `grhh` | `git reset --hard HEAD` |
| `grb` | `git rebase` |
| `grbc` | `git rebase --continue` |
| `grba` | `git rebase --abort` |

