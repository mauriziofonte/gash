# Git

The git functions do in one command the jobs that take several: save and
push all your work, tag a release on the remote, carry a branch's changes to
another branch as a patch, keep every past version of a file, and repair a
repository whose object files a crash left empty. Short `gl*` commands draw
the log, and two-to-four-letter aliases cover the everyday git commands.

Platforms: Linux, macOS and WSL, wherever `git` is installed. The aliases and
the `gl*` commands are defined only when `git` is on `PATH`.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Nothing beyond what Gash itself needs ([requirements](../requirements.md)).
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

All of them run your own `git`, with your configuration, hooks, signing
settings and credential helpers. The ones that change a repository or its
remote show what they will do and ask first; see
[Confirmation and dry runs](#confirmation-and-dry-runs).

## Save and push everything

[`git_sync`](reference.md#git_sync) (`gsync`) stages every change of the work
tree (new, modified and deleted files), commits them, and pushes the current
branch to its upstream. `--dry-run` shows what it would commit and where it
would push:

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q --bare origin.git
git clone -q origin.git app 2>/dev/null
cd app
printf '# App\n' >README.md
git add README.md
git commit -q -m 'First commit'
git push -q origin main
printf '# App\n\nRun make.\n' >README.md
printf 'notes\n' >notes.txt
mask: Committed [0-9a-f]{7,}
-->
```console
$ cd app
$ gsync --dry-run
Would commit on main with message "chore: sync":
 M README.md
?? notes.txt
Would push main to origin/main.
$ gsync -m "docs: how to run" --yes
OK: Committed b0773ac on main: docs: how to run
OK: Pushed main to origin/main.
$ gsync --yes
Info: Nothing to commit on main.
Info: main is already on origin/main.
```

The message defaults to `chore: sync`; `-m` sets your own. It commits only
when there is something to commit, and pushes only what the upstream does not
have yet. A branch without an upstream is pushed to `origin` under the same
name, and becomes its upstream. `--no-push` stops after the commit.

`git add --all` follows `.gitignore`: in a repository without one, check
`git status` first, or a build directory or a secret file goes into the
commit. `git_sync` refuses to run on a detached HEAD and during a merge,
rebase, cherry-pick or revert.

## Tag a release

[`git_add_tag`](reference.md#git_add_tag) (`gat`) creates an annotated tag and
pushes it to `origin` in one step; the message defaults to `Release TAG`.
[`git_list_tags`](reference.md#git_list_tags) (`glt`) lists the tags of the
repository and those of `origin`, and
[`git_delete_tag`](reference.md#git_delete_tag) (`gdt`) deletes a tag in
both places:

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q --bare origin.git
git clone -q origin.git app 2>/dev/null
cd app
printf '# App\n' >README.md
git add README.md
git commit -q -m 'First commit'
git push -q origin main
-->
```console
$ cd app
$ gat v1.0.0 --dry-run
Would create tag 'v1.0.0' and push refs/tags/v1.0.0 to origin (~/origin.git).
$ gat v1.0.0 "First stable release" --yes
OK: Tag 'v1.0.0' created.
To ~/origin.git
 * [new tag]         v1.0.0 -> v1.0.0
OK: Tag 'v1.0.0' pushed to remote.
$ glt
Local tags:
v1.0.0
Remote tags:
v1.0.0
$ gdt v1.0.0 --yes
OK: Local tag 'v1.0.0' deleted.
To ~/origin.git
 - [deleted]         v1.0.0
OK: Remote tag 'v1.0.0' deleted.
```

Both check `origin` before they change anything: a tag that already exists,
locally or on `origin`, is refused, and when `origin` cannot be reached
nothing is created or deleted. `git_delete_tag` deletes the tag where it
exists, which may be only one of the two places. A tag name must be valid for
git and cannot start with `-`.

## Read the log

Six log commands draw `git log` in compact forms, and pass any other option
to it (`-10`, `--since=1.week`, `--author=ada`):

| Command | Shows |
|---|---|
| `gl` | the current branch as a graph: hash, refs, subject, age, author |
| `gla` | the same for all branches |
| `glo` | one line per commit: hash, refs, subject |
| `glg` | the graph along first parents only, so merges read as one line |
| `gls` | each commit with the files it changed |
| `glf FILE [OPTIONS]` | the history of one file, with its changes, across renames |

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q --bare origin.git
git clone -q origin.git app 2>/dev/null
cd app
export GIT_AUTHOR_DATE=2026-01-15T10:00:00Z GIT_COMMITTER_DATE=2026-01-15T10:00:00Z
printf 'port: 80\n' >config.yml
git add config.yml
git commit -q -m 'Add config'
git push -q origin main
git switch -q -c feature
printf 'port: 8080\n' >config.yml
git commit -q -am 'Use port 8080'
git switch -q main
-->
```console
$ cd app
$ glo --all
* 0c68eb0 (feature) Use port 8080
* dbfe35f (HEAD -> main, origin/main) Add config
```

`gl --help` prints the same summary with examples. On a terminal the output
goes through git's pager, in color unless `NO_COLOR` or `GASH_NO_COLOR` is
set. `glf FILE` takes `git log` options after the file
(`glf config.yml -2`, the last two changes).

## Learn the short aliases

The git pack defines short aliases for the everyday commands: `gst` is `git
status`, `gaa` is `git add --all`, `gcm` is `git commit -m`, `gco` is `git
checkout`, `gpf` is `git push --force-with-lease`, and so on. To see what an
alias runs, ask Bash:

<!-- example -->
```console
$ alias gcm gpf
alias gcm='git commit -m'
alias gpf='git push --force-with-lease'
```

`gash git` lists the git functions and every git alias your shell defines,
and the [git pack](reference.md#the-git-pack) table has them all. An alias is
not defined when a program of the same name is installed: `gs` (`git status
-sb`) gives way to Ghostscript, `gc` to Graphviz, `gp` to PARI/GP. Define it
in `~/.bash_aliases` if you want it anyway.

## Keep every version of a file

[`git_dump_revisions`](reference.md#git_dump_revisions) (`gdr`) writes each
committed version of a file next to it, numbered from the newest, with a
second file that holds the commit's message, author and date:

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q app
cd app
export GIT_AUTHOR_DATE=2026-01-15T10:00:00Z GIT_COMMITTER_DATE=2026-01-15T10:00:00Z
printf 'port: 80\n' >config.yml
git add config.yml
git commit -q -m 'Add config'
export GIT_AUTHOR_DATE=2026-02-01T09:30:00Z GIT_COMMITTER_DATE=2026-02-01T09:30:00Z
printf 'port: 8080\n' >config.yml
git commit -q -am 'Use port 8080'
-->
```console
$ cd app
$ gdr config.yml
Info: Saving version 1 to file config.yml.001.4d30200 for commit 4d30200
Info: Saving version 2 to file config.yml.002.dbfe35f for commit dbfe35f
$ cat config.yml.002.*[0-9a-f]
port: 80
$ cat config.yml.002.*.logmsg
*******************************************************
Add config
Authored by Ada Lovelace at 2026-01-15 10:00:00 +0000


*******************************************************
```

The files are new and untracked: delete them, or add them to `.gitignore`,
when you are done. The file must be tracked; it may be given by a relative or
an absolute path inside the work tree.

## Carry a branch's changes to another branch

[`git_apply_patch`](reference.md#git_apply_patch) (`gpatch`) takes every
change between a commit and the tip of a feature branch and applies it, as
one patch, to the main branch, without committing it. It is a way to bring
work over without its history. The three arguments are the target branch, the
feature branch and the commit to start from, usually the one the feature
branch started at:

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q --bare origin.git
git clone -q origin.git app 2>/dev/null
cd app
export GIT_AUTHOR_DATE=2026-01-15T10:00:00Z GIT_COMMITTER_DATE=2026-01-15T10:00:00Z
printf 'port: 80\n' >config.yml
git add config.yml
git commit -q -m 'Add config'
git push -q origin main
git switch -q -c feature
printf 'port: 80\nworkers: 4\n' >config.yml
git commit -q -am 'Add workers'
printf 'hello\n' >NEWS
git add NEWS
git commit -q -m 'Add NEWS'
git switch -q main
-->
```console
$ cd app
$ glo --all
* 380114d (feature) Add NEWS
* 7d68f8b Add workers
* dbfe35f (HEAD -> main, origin/main) Add config
$ gpatch main feature dbfe35f --dry-run
Would check out main, pull origin main and apply feature's changes since dbfe35f.
$ gpatch main feature dbfe35f --yes
[1/4] Checking out to main branch 'main'...
Already on 'main'
Your branch is up to date with 'origin/main'.
From ~/origin
 * branch            main       -> FETCH_HEAD
Already up to date.
[2/4] Checking out to feature branch 'feature'...
Switched to branch 'feature'
[3/4] Generating patch from 'feature' since commit 'dbfe35f'...
[4/4] Applying patch to 'main'...
Switched to branch 'main'
Your branch is up to date with 'origin/main'.
Falling back to direct application...
Applied patch to 'config.yml' cleanly.
OK: Patch applied successfully from 'feature' to 'main'.
$ git status --short
A  NEWS
M  config.yml
```

It checks out the target branch and pulls it from `origin`, writes the patch
from the feature branch, then applies it to the target with
`git apply --3way`. The changes end up staged on the target branch, for you
to review and commit. The work tree must be clean before it starts. If a
step fails, the branch you started on is checked out again; a patch that does
not apply is kept, and the message gives its path.

## Repair a repository after a crash

A crash or an out-of-memory kill during a git command can leave object files
of zero bytes in `.git/objects`. git then reports `object file ... is empty`
or a corrupt object, and a plain `git fetch` does not help: the references
already match, so git believes it has the objects.
[`git_repair`](reference.md#git_repair) (`gheal`) clones `origin` into a
temporary bare repository, deletes the empty object files, copies the packs
of the clone in, then runs `git gc` and `git fsck --full` to check the
result. `--dry-run` counts the empty files and shows the plan:

<!-- example
git config --global user.name 'Ada Lovelace'
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git init -q --bare origin.git
git clone -q origin.git app 2>/dev/null
cd app
printf '# App\n' >README.md
git add README.md
git commit -q -m 'First commit'
git push -q origin main
mkdir -p .git/objects/4d
: >.git/objects/4d/5f8a0c3e2b1d4c6a7e9f0b1c2d3e4f5a6b7c8d
-->
```console
$ cd app
$ gheal --dry-run
Object store: ~/app/.git/objects
Empty (corrupted) object files: 1
Plan: make a fresh bare clone of ~/origin.git, delete the empty files,
      copy the packs of the clone, run git gc and git fsck --full.
```

Nothing is deleted unless the clone succeeds, and only empty loose object
files count: pack files and the other files of `.git` are never touched.
Commits that never reached `origin` cannot come back from it; push often
from machines that crash. It works in linked worktrees and submodules too.

## Confirmation and dry runs

The functions that change a repository or `origin` show their plan and ask:

| Function | Asks before | Default answer | `--dry-run` |
|---|---|---|---|
| `git_sync` | committing and pushing | no (`[y/N]`) | yes |
| `git_add_tag` | creating and pushing the tag | no (`[y/N]`) | yes |
| `git_delete_tag` | deleting the tag | no (`[y/N]`) | yes |
| `git_apply_patch` | checking out, pulling and applying | no (`[y/N]`) | yes |
| `git_repair` | cloning and deleting the empty files | yes (`[Y/n]`) | yes |

`-y` or `--yes` answers for you. Without a terminal to ask on (a script, an
agent, `GASH_HEADLESS=1`) these functions refuse unless you pass it. The
others only read (`git_list_tags`, the `gl*` commands) or only add new files
(`git_dump_revisions`).

## When something goes wrong

| Message | Meaning and what to do |
|---|---|
| `Error: Not in a git repository.` | Run it inside a work tree |
| `Error: Remote 'origin' is not configured.` | The tag functions and `git_repair` work with `origin`: `git remote add origin URL` |
| `Error: Cannot reach origin (URL): nothing was created or deleted.` | The network, the URL or your credentials failed; `git ls-remote origin` shows git's own error |
| `Error: Refusing to commit and push in headless mode without --yes.` | A function of the table above ran without a terminal (the action in the message varies). Check the plan with `--dry-run`, then add `--yes` |
| `Error: No terminal to ask for confirmation: pass -y to go ahead.` | The same, for `git_repair` |
| `Info: Cancelled.` | You answered no; nothing changed. Status 1 |
| `Error: Detached HEAD: check out a branch first.` | `git_sync` pushes a branch: `git switch NAME` |
| `Error: A rebase is in progress: finish or abort it first (git rebase --continue or --abort).` | Finish the operation git is in the middle of (merge, rebase, cherry-pick or revert) |
| `Error: Branch 'NAME' has no upstream and there is no 'origin' remote: use --no-push or add a remote.` | Add a remote, or commit with `git_sync --no-push` |
| `Error: Tag 'NAME' already exists.` | Choose another name, or delete it first with `git_delete_tag` |
| `Error: Invalid tag name: 'NAME'.` | `git check-ref-format` refuses it (spaces, `..`, a leading `-` and the like) |
| `Error: Working tree is not clean. Commit or stash your changes before applying a patch.` | `git_apply_patch` needs a clean work tree, untracked files included |
| `Error: Commit hash 'HASH' is not valid.` | The commit does not exist in this repository: check `glo --all` |
| `Error: Failed to apply the patch. Rolling back working tree.` | The changes conflict with the target branch. The target is reset to its last commit, the path of the kept patch follows; apply it by hand with `git apply --3way PATH` |
| `Error: File 'FILE' does not exist in the repository.` or `Error: git failed on 'FILE'.` | `git_dump_revisions` needs a tracked file of this work tree |
| `Error: Failed to clone 'URL': nothing was deleted.` | `git_repair` could not clone `origin`; fix the access to `origin` first |
| `Error: Repo still reports issues after repair. Manual inspection needed.` | The damage is not only empty files; `git fsck --full` lists what is wrong |

## See also

- [reference.md](reference.md): every function and alias of this area, with all options.
- [Shell](../shell/README.md): the git part of the prompt and `GASH_GIT_EXCLUDE`.
- [Tools for AI agents](../agents/README.md): `llm_git_status`, `llm_git_diff`, `llm_git_log` and `agent_git_commit`.
