# agent

Write tools for AI agents.

_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._

| Function | Alias | Description |
|---|---|---|
| [`agent_docker_exec`](#agent-docker-exec) |  | Run any command in a container, no shell quoting (JSON) |
| [`agent_git_commit`](#agent-git-commit) |  | Stage explicit paths, commit, optionally push, in one call |
| [`agent_kill`](#agent-kill) |  | Signal processes by PID or command line, never the caller (JSON) |
| [`agent_put`](#agent-put) |  | Upload a file to a registered host: atomic, optional backup and hash check |
| [`agent_replace`](#agent-replace) |  | Replace text in explicit files, all or nothing |
| [`agent_test`](#agent-test) |  | Run a test suite and summarize it: counts and each failure's file:line |

## agent_docker_exec

Run any command in a container, no shell quoting (JSON)

Replaces:

- `docker exec ctr sh -c '...' (writes)`

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
quote. Read-only work belongs to llm_docker_exec. Rails: the agent
write blocklist (rm -rf /, chown -R or chmod -R of /, mkfs, dd to a
disk, shutdown, curl | sh, keys: dangerous_command_blocked, FATAL; a path
below a root is fine) and no secret file named in CMD
(secret_file_blocked). Output as
llm_docker_exec: {"rc","out","err"}, exit status 0 when the command
ran (read rc); --dry-run: {"argv":[...],"dry_run":true,"written":false}.
```

See also: `llm_docker_exec`

## agent_git_commit

Stage explicit paths, commit, optionally push, in one call

Replaces:

- `git add A B && git commit -m S && git push && git log -1` (`agent_git_commit --add A B --push`)

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

Replaces:

- `pkill -f x; kill PID`

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

## agent_put

Upload a file to a registered host: atomic, optional backup and hash check

Replaces:

- `scp f h:/tmp/x && ssh h "install -m 644 /tmp/x /dest && sha256sum /dest"`

#### Usage

```text
agent_put ALIAS:PATH --from FILE|- [--mode M] [--backup] [--verify] [--force] [--dry-run] [--timeout S]
```

#### Options

```text
--from FILE|-          The content: a local file, or - for stdin (64 MiB max)
--mode M               Octal mode of the written file (default: the old
                       file's, or the host's umask for a new one)
--backup               Copy the old file to PATH.bak-<UTC time> first
--verify               Check the sha256 on the host before the rename
--force                Allow an ASK path (/etc, /var, /usr/local, /root, any
                       .ssh): only after the user agreed
--dry-run              Check the target on the host and report; write nothing
--timeout S            Bound the call (default 300)
```

#### Examples

```bash
# Deploy one file, keep the old one, check what arrived
agent_put web:/var/www/app/config.php --from config.php --backup --verify --force

# Generated content from stdin, into the remote home
printf '%s\n' "$TOKEN_LINE" | agent_put web:app/.version --from -
```

#### Notes

```text
Replaces scp f h:/tmp/x; ssh h "install -m 644 /tmp/x /dest"; sha256sum on
both sides. One ssh round trip: the content goes on stdin to a temporary
file next to PATH (the old file's mode kept), its size is checked (and its
sha256 with --verify), then it is renamed over PATH; a failed check
replaces nothing (write_failed, put_verify_failed: STOP). PATH's directory
must exist; a relative PATH is below the remote home. ALIAS is a
registered HOST: alias. The transfer path policy applies before connecting
and again on the host on the resolved path: DENY paths are
write_path_denied (FATAL, even with --force), ASK paths sensitive_path
(STOP) without --force; secret files are never written or uploaded
(secret_file_blocked). Output: {"host","path","bytes","sha256",
"replaced","backup","verified","written":true}; --dry-run gives
"exists" and "dry_run":true,"written":false.
```

## agent_replace

Replace text in explicit files, all or nothing

Replaces:

- `python3 - <<EOF ... s.replace(...) ... EOF; sed -i`

#### Usage

```text
agent_replace FILE... (--old A --new B)... [--count N | --min N] [--regex] [--dry-run] [--force] [--format F] [--width N] [--host ALIAS]
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
--host ALIAS           Edit the files on a registered HOST: alias (the gash
                       binary runs there; one ssh round trip; AGENTS.md 3.12)
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

## agent_test

Run a test suite and summarize it: counts and each failure's file:line

Replaces:

- `vendor/bin/pest \| grep -E 'Tests:\|FAIL'; npm run test \| tail -20`

#### Usage

```text
agent_test [--runner auto|NAME] [--fail-only] [--dry-run] [--max-lines N] [--width N] [--timeout S] [--absolute] -- CMD...
```

#### Options

```text
--runner auto|NAME     The output format: pest, phpunit, phpstan, psalm,
                       vitest, jest, mocha, pytest, go, cargo, playwright
                       (default auto: from CMD and the output)
--fail-only            Print nothing when CMD exits 0 with no failure
--dry-run              Print the command, its format and the directory;
                       run nothing
--max-lines N          Failures listed (default 50; --head N, --tail N);
                       unparsed output: the last N lines (default 20)
--width N              Cut messages at N columns (default 200)
--timeout S            End CMD after S (default: no limit)
--relative, --absolute Paths below the working directory relative (default)
                       or as the runner printed them
```

#### Examples

```bash
# Pest with a filter
agent_test -- vendor/bin/pest --filter UserTest

# A package script; silent when green
agent_test --fail-only -- npm test

# Go with the JSON reporter
agent_test -- go test -json ./...
```

#### Notes

```text
A write tool: the suite runs the project's own code, which can change
files, caches or a test database, so it is an agent_* function and not
an llm_* read. Replaces pest | grep -E 'Tests:|FAIL' and npm test |
tail -20. CMD is a test suite: a runner (pest, phpunit, paratest,
phpstan, psalm, vitest, jest, mocha, playwright, pytest; a path such as
vendor/bin/pest, also through php, node, npx, pnpm exec or python -m),
php artisan test, go test, cargo test, the test script of npm, pnpm,
yarn, bun or composer, make test; any other command is
not_a_test_command (gash run runs it). CMD runs as given, stdout and
stderr merged, stdin /dev/null; the exit status is CMD's. Output:
{"runner","passed","failed","skipped","duration_s","failures":[{"name",
"file","line","message"}],"parsed":true,"exit_code"}; analyzer findings
(phpstan, psalm) are failures. Output no parser reads:
{"runner":null,"duration_s","parsed":false,"exit_code","tail":[...]}.
--dry-run: {"argv":[...],"runner","cwd","dry_run":true,"written":false}
(runner null when only the output would tell). Options come before --;
a program not on PATH is command_not_found. The suite runs without
llm_exec's rails.
```

See also: `llm_lint`
