# agent

Write tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`agent_docker_exec`](#agent-docker-exec) |  | Run any command in a container, no shell quoting (JSON) |
| [`agent_git_commit`](#agent-git-commit) |  | Stage explicit paths, commit, optionally push, in one call |
| [`agent_kill`](#agent-kill) |  | Signal processes by PID or command line, never the caller (JSON) |
| [`agent_replace`](#agent-replace) |  | Replace text in explicit files, all or nothing |

## agent_docker_exec

Run any command in a container, no shell quoting (JSON)

#### Usage

```text
agent_docker_exec CTR [--user U] [--dry-run] [--width N] [--max-lines N] [--head N] [--tail N] -- CMD...
```

#### Options

```text
--user U, -u U         Run as user U (name or uid[:gid])
--dry-run              Print the docker argv; run nothing
--width N              Cut each output line at N columns
--max-lines N, --head N, --tail N
                       Keep the first/last N lines of each stream
-- CMD...              The command, one word per argument
```

#### Examples

```bash
# Instead of: docker exec -u www-data app sh -c 'php artisan migrate --force'
agent_docker_exec app --user www-data -- php artisan migrate --force
agent_docker_exec app --dry-run -- composer install --no-dev
```

#### Notes

```text
Replaces docker exec CTR sh -c '...' for commands that change things:
CMD's words reach docker exec as an array, no shell runs, nothing to
quote. Read-only work belongs to llm_docker_exec. Rails: the llm_exec
dangerous-command blocklist (dangerous_command_blocked, FATAL) and no
secret file named in CMD (secret_file_blocked). Output as
llm_docker_exec: {"rc","out","err"}, exit status 0 when the command
ran (read rc); --dry-run: {"argv":[...],"dry_run":true,"written":false}.
```

See also: `llm_docker_exec`

## agent_git_commit

Stage explicit paths, commit, optionally push, in one call

#### Usage

```text
agent_git_commit -m SUBJECT [-m BODY]... (--add PATH... | -A) [--author "N <e>"]
                 [--push [REMOTE]] [--dry-run] [--force] [--timeout S] [--format F]
                 [--max-lines N] [--head N] [--tail N] [--width N] [PATH]
```

#### Options

```text
-m, --message TEXT     The subject; each further -m is a body paragraph
--add PATH...          The paths to stage (files or directories, relative to
                       PATH), every word up to the next option
-A, --all              Stage every change of the work tree (never implied)
--author "N <e>"       The commit's author
--push [REMOTE]        Push the branch afterwards: to its upstream, or to
                       REMOTE under the branch's name; never forced
--dry-run              Report the files, the branch and the push target;
                       stage, commit and push nothing
--force                Allow a repository on a sensitive path (/etc, /var,
                       /usr/local, /root) outside your HOME and working
                       directory: only after the user agreed. It never forces
                       a push
--timeout S            Bound the commit (hooks) and the push (default 120s)
--format F             json (default) or jsonl: the report is one object
--max-lines N          Paths listed by --dry-run (default 100; also --head N,
                       --tail N); --width N cuts each path
```

#### Examples

```bash
# Commit two files and push to the upstream
agent_git_commit -m 'fix: parser' -m 'Handles empty input.' --add src/p.go src/p_test.go --push
# {"hash":"a1b2c3d","branch":"main","files":2,"pushed":true,"remote":"origin","ahead":0}

# Preview a commit of every change in another repository
agent_git_commit -m 'chore: update' -A --dry-run ~/src/site
```

#### Notes

```text
Replaces git add A B && git commit -m S -m B && git push && git log -1.
Checked before anything is written: a detached HEAD (detached_head) and
unresolved conflicts (merge_conflict) are STOP; nothing to commit
(nothing_to_commit), a missing path (file_not_found), a push without an
upstream (no_upstream) or to an unknown remote (unknown_remote), and a
branch behind its upstream (push_rejected) are RETRY; a secret file in the
commit (staged already or under an --add path) is secret_file_blocked
(FATAL). The commit holds what is staged plus the --add paths, as git add
&& git commit does. git runs with your configuration and hooks and never
prompts. A failed commit (a hook, the identity) is commit_failed (RETRY)
with the paths left staged; a push the remote refuses is push_rejected
(RETRY) with the remote's message, any other push failure push_failed
(STOP); both carry "hash" and "branch" of the commit, which exists.
ahead is null without an upstream to compare with.
```

See also: `llm_git_overview`, `llm_git_status`

## agent_kill

Signal processes by PID or command line, never the caller (JSON)

#### Usage

```text
agent_kill NAME|PID... [--signal S] [--dry-run] [--format F]
```

#### Options

```text
NAME                   A regular expression on the full command line, as
                       llm_procs --name matches it
PID                    One process
--signal S, -s S       TERM (default), KILL, HUP, INT, USR1, ... or a number
--dry-run              Report the PIDs and command lines; signal nothing
--format F             json (default), jsonl, tsv or table: one row per
                       process
```

#### Examples

```bash
# Instead of: pkill -f 'artisan serve' (exit 1 when nothing runs)
agent_kill 'artisan serve' --dry-run
agent_kill 'artisan serve'
agent_kill 4242 4243 --signal KILL
```

#### Notes

```text
Replaces pkill -f x and kill PID. Only your own user's processes; never
PID 1, this call or one of its ancestors (the shell or agent that runs
it): an explicit PID breaking that is kill_refused (FATAL), a NAME never
matches them. A NAME matching every command line (.*) is
pattern_too_broad. Nothing matched is success: {"killed":0,"pids":[]}.
Output {"killed":N,"pids":[...]}; --dry-run adds "cmds", "signal",
"dry_run":true,"written":false. It does not wait for the processes to
exit: llm_wait --gone PID does.
```

See also: `llm_procs`

## agent_replace

Replace text in explicit files, all or nothing

#### Usage

```text
agent_replace FILE... (--old A --new B)... [--count N | --min N] [--regex] [--dry-run] [--force] [--format F] [--width N]
```

#### Options

```text
--old A --new B        One replacement; repeat the pair for more, applied in
                       order (each to the result of the one before)
--count N              Every pair must match exactly N times in every file
                       (default: exactly 1)
--min N                Every pair must match at least N times (replaces all)
--regex                --old is a Go (RE2) regular expression; $1 in --new is
                       a group
--dry-run              Report the counts and the lines each file would gain
                       and lose; write nothing
--force                Allow a sensitive path (/etc, /var, /usr/local, /root,
                       .ssh) outside your HOME and working directory: only
                       after the user agreed
--format F             json (default), jsonl, tsv or table: one row per file
                       and pair
--width N              Cut output lines at N columns
```

#### Examples

```bash
# Rename a class in two files; fails, writing nothing, unless each has it once
agent_replace app/A.php app/B.php --old 'OldName' --new 'NewName'

# Every occurrence, at least one per file, preview first
agent_replace src/x.js src/y.js --old 'var ' --new 'let ' --min 1 --dry-run

# Two pairs, a regular expression with a group
agent_replace conf.ini --old 'debug=1' --new 'debug=0' --old 'v([0-9]+)' --new 'v$1-lts' --regex
```

#### Notes

```text
Replaces python3 open().read().replace() + write scripts and sed -i, with
the count assertion built in. All or nothing: every pair's count is checked
in every file in memory first; one mismatch is replace_count_mismatch
(RETRY) and no file changes. Writes go to a temporary file renamed over the
target (mode kept); a file that changed meanwhile is file_changed and
nothing is written. Targets are explicit: no glob is expanded, each file is
named once. Secret files, protected paths and system paths are refused
(FATAL); a sensitive path is sensitive_path (STOP) without --force; binary
files and files over 64 MiB are refused. Output:
{"files":N,"replaced":{"FILE":[n1,n2]},"written":true}.
```
