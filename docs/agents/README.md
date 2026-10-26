# Tools for AI agents

This guide is for a person who wants AI coding agents (Claude Code, Codex,
Gemini CLI, Cursor, Aider and the like) to work through Gash rather than
through raw shell commands. Gash gives them two families of functions: the
`llm_*` functions only read, and the `agent_*` functions make the changes,
each with a dry run. Every call answers with one JSON value, and every
refusal is one JSON line the agent can act on. This page shows how to try
them, how an agent finds them, which settings bound what they may do, and
what Gash refuses. The agents themselves follow [AGENTS.md](../../AGENTS.md),
the contract they read.

Platforms: Linux, macOS and WSL. `llm_service` needs systemd (Linux, or WSL
with systemd on), and the `llm_exec` sandbox (`GASH_LLM_SANDBOX=1`) needs
`bwrap`, which exists on Linux only.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ssh](../requirements.md#tool-ssh) (feature), for `agent_put`, `agent_replace` (--host), `llm_assert` (--host), `llm_config` (--host), `llm_count` (--host), `llm_csv` (--host), `llm_db_databases` (a +ssh URL), `llm_db_explain` (a +ssh URL), `llm_db_query` (a +ssh URL), `llm_db_sample` (a +ssh URL), `llm_db_schema` (a +ssh URL), `llm_db_tables` (a +ssh URL), `llm_deps` (--host), `llm_docker_exec` (--host), `llm_docker_logs` (--host), `llm_docker_ps` (--host), `llm_dotenv` (--host), `llm_env` (--host), `llm_find` (--host), `llm_git_log` (--host), `llm_git_overview` (--host), `llm_git_status` (--host), `llm_grep` (--host), `llm_hash` (--host), `llm_json` (--host), `llm_lint` (--host), `llm_log` (--host), `llm_ports` (--host), `llm_procs` (--host), `llm_project` (--host), `llm_read` (--host), `llm_remote_run`, `llm_same` (a host side), `llm_service` (--host), `llm_stat` (--host), `llm_sysinfo` (--host), `llm_textcheck` (--host), `llm_textstats` (--host), `llm_tools` (--host), `llm_tree` (--host) and `llm_wait` (--host)
- [ps](../requirements.md#tool-ps) (feature), for `agent_kill` (on macOS, or without /proc), `llm_procs` (on macOS, or without /proc) and `llm_wait` (--gone, on macOS)
- [lsof](../requirements.md#tool-lsof) (feature), for `llm_procs` (--port)
- [ss](../requirements.md#tool-ss) (feature), for `llm_ports`
- [docker](../requirements.md#tool-docker) (feature), for `agent_docker_exec`, `llm_docker_exec`, `llm_docker_logs` and `llm_docker_ps`
- [php](../requirements.md#tool-php) (feature), for `llm_lint` (.php files) and `llm_runtime` (php)
- [composer](../requirements.md#tool-composer) (optional), for `llm_runtime` (composer)
- [node](../requirements.md#tool-node) (feature), for `llm_lint` (.js files) and `llm_runtime` (node)
- [python3](../requirements.md#tool-python3) (feature), for `llm_lint` (.py files)
- [gawk](../requirements.md#tool-gawk) (optional), for `llm_exec` (awk)
- [gnu-sed](../requirements.md#tool-gnu-sed) (optional), for `llm_exec` (sed)
- [diffutils](../requirements.md#tool-diffutils) (optional), for `llm_exec` (diff -r)
- [bwrap](../requirements.md#tool-bwrap) (optional), for `llm_exec` (GASH_LLM_SANDBOX=1)
- [systemctl](../requirements.md#tool-systemctl) (feature), for `llm_service`
- [journalctl](../requirements.md#tool-journalctl) (optional), for `llm_service` (--log)
<!-- END generated: prerequisites -->

## Reads and writes: the two families

| Family | What it does | Examples |
|---|---|---|
| `llm_*` | Reads, and nothing else: files, code search, git state, database schemas and read-only queries, processes, ports, services, HTTP and TLS probes, the same reads on a registered SSH host | `llm_tree`, `llm_grep`, `llm_git_status`, `llm_db_query`, `llm_exec` |
| `agent_*` | The changes: replace text in files, commit (and push), signal processes, run a command in a container, upload a file to a host, run a test suite | `agent_replace`, `agent_git_commit`, `agent_kill`, `agent_test` |

Each `llm_*` and `agent_*` function prints its answer as JSON on stdout
(`--format tsv` or `table` for rows) and its errors as one JSON line on
stderr. An error carries an `action`: `RETRY` means fix the arguments,
`CONTINUE` skip this step, `STOP` ask you, `FATAL` stop and report.

The split is the point: an agent that you allow only the `llm_*` functions
cannot change anything through Gash. A test suite runs the project's own
code, which can write, so the test runner `agent_test` is a write tool.

Every function is in the [reference](reference.md), with all its options.

## Try a few reads

The functions work in any shell where Gash is loaded. Here they read a
small repository with one commit, one modified file and an untracked one.
`llm_tree` shows the layout (secret files are listed by name, never read):

<!-- example
mkdir -p shop/src && cd shop
git init -q -b main
printf '# Shop\n' >README.md
printf '<?php\necho "hello";\n' >src/index.php
printf 'APP_KEY=secret\n' >.env
printf '.env\n' >.gitignore
git add -A && git -c user.name=Dev -c user.email=dev@example.com commit -q -m 'first commit'
printf '<?php\necho "hello world";\n' >src/index.php
printf 'TODO\n' >notes.txt
-->
```console
$ cd shop
$ llm_tree --depth 2
{"path":"~/shop","type":"directory","children":[{"name":".env","type":"file"},{"name":".gitignore","type":"file"},{"name":"README.md","type":"file"},{"name":"notes.txt","type":"file"},{"name":"src","type":"directory","children":[{"name":"index.php","type":"file"}]}]}
$ llm_read src/index.php
==> src/index.php:1-2 <==
1:<?php
2:echo "hello world";
$ llm_git_status
{
  "branch": "main",
  "ahead": 0,
  "behind": 0,
  "staged": [],
  "modified": ["src/index.php"],
  "untracked": ["notes.txt"],
  "conflicted": []
}
$ llm_read .env
{"error":"secret_file_blocked","details":".env","action":"FATAL","recoverable":false,"hint":"This file contains secrets and cannot be read"}
```

`llm_read` numbers the lines, so the agent can cite them. The `.env` file
holds secrets, so `llm_read` refuses it with `secret_file_blocked`: its
action is `FATAL`, and the agent must not look for another way to read
it.

`llm_exec` runs a command line, but only read-only commands from an
allowlist:

<!-- example
mkdir -p shop/src && printf 'x\n' >shop/src/index.php
-->
```console
$ cd shop
$ llm_exec "ls src"
index.php
$ llm_exec "rm -rf src"
{"error":"command_not_allowed","details":"rm","action":"STOP","recoverable":true,"hint":"llm_exec runs read-only commands (ls, cat, grep, find, git status, ...). Ask the user to add 'rm' to GASH_LLM_EXEC_ALLOW in ~/.gash_env if it is needed"}
```

`command_not_allowed` has the action `STOP`: the agent asks you. The
[settings below](#decide-what-agents-may-run) let you allow more.

## Preview a change with --dry-run

Every `agent_*` function takes `--dry-run`: it runs the same checks and
reports what would change, and writes nothing.

<!-- example
mkdir -p shop/src && printf '<?php\necho "hello world";\n' >shop/src/index.php
-->
```console
$ cd shop
$ agent_replace src/index.php --old "hello world" --new "hello shop" --dry-run
{"files":1,"replaced":{"src/index.php":[1]},"written":false,"dry_run":true,"diff":{"src/index.php":{"add":1,"del":1}}}
$ cat src/index.php
<?php
echo "hello world";
```

`"written":false` and the file is unchanged. Without `--dry-run`, the
replacement happens only when each `--old` text matches exactly once in
every file named (`--count N` and `--min N` change that); otherwise no file
changes at all. The other write tools follow the same rules: every target
is named (no glob is expanded, no implicit "all"), and every target is
checked before the first write.

## Let an agent find the tools

An agent needs two things: a way to run the functions, and to know when to
use them.

**Running them.** An agent's shell is not interactive, so it does not load
Gash on its own. There are two ways:

- Headless mode: source `gash.sh` with `GASH_HEADLESS=1` in each command.
  It loads only the functions, prints nothing while loading, uses no colors
  and defines no prompt or history settings.

  ```bash
  GASH_HEADLESS=1 bash -c 'source ~/.gash/gash.sh; llm_git_status'
  ```

- Direct calls: every `llm_*` and `agent_*` function also runs as
  `gash FUNCTION ARGS`, with the same output and exit status, without
  loading any Bash. Outside a Gash shell, `gash` is the launcher
  `~/.gash/bin/gash`: link it into a directory on the agent's `PATH`.

  ```bash
  ln -s ~/.gash/bin/gash ~/.local/bin/gash
  gash llm_git_status
  ```

**Knowing when.** Pick what suits the agent:

- Point it to the contract. Add a line such as "Use Gash for reads and
  edits: see ~/.gash/AGENTS.md" to the project's `CLAUDE.md`, `AGENTS.md`
  or `GEMINI.md`, or to the agent's system prompt.
  [AGENTS.md](../../AGENTS.md) also has ready-made snippets for these files.
- `gash agents-hint` prints a Markdown table that maps raw commands to the
  function that replaces them (`git status --short` to `llm_git_status`,
  `sed -i` to `agent_replace`), with the error actions in one paragraph.
  Append it to the agent's instructions:

  ```bash
  gash agents-hint >> CLAUDE.md
  gash agents-hint --format json   # the same table as JSON
  ```

- `gash agent-functions` lists every function of both families, with its
  family and description; `--json` adds the usage lines, the options and
  the raw command it replaces, for a wrapper that registers each function
  as a tool.

<!-- example -->
```console
$ gash agent-functions | grep git
agent_git_commit   write  Stage explicit paths, commit, optionally push, in one call
llm_git_diff       read   Git diff with statistics
llm_git_log        read   Recent commits (JSON)
llm_git_overview   read   Repository state in one small object (JSON)
llm_git_show       read   A commit's metadata and files, its patch, or a file at a revision
llm_git_status     read   Compact git status (JSON)
```

## Decide what agents may run

These settings bound what the functions do. The first three go in
`~/.gash_env` only: Gash reads them from that file and never from the
environment, so an agent cannot loosen them by exporting a variable.

| Setting | Effect |
|---|---|
| `GASH_LLM_EXEC_ALLOW="make npm"` | More commands for `llm_exec`, with any arguments. Secret files and protected paths stay refused |
| `GASH_LLM_EXEC=unrestricted` | `llm_exec` skips its parser and allowlist; only the blocklist of destructive patterns remains |
| `GASH_LLM_SANDBOX=1` | `llm_exec` runs inside `bwrap`: a read-only filesystem and no network |
| `DB:name=...` | The database connections `llm_db_*` may use. Every query runs in a read-only session; give Gash an account with read-only grants too |
| `HOST:alias=user@host` | The SSH hosts `--host`, `llm_remote_run`, `llm_same` and `agent_put` may reach. A host that is not registered is refused |
| `GASH_DB_TIMEOUT` | How long a PostgreSQL query may run, in seconds (default 30) |

The `llm_exec` settings also apply to `llm_wait --cmd-ok`,
`llm_docker_exec`, `gash run --safe` and the commands of `llm_remote_run`
(the sandbox aside: it is local). They are read from the file at each
call, so a change applies at once. The [configuration](../configuration.md) page lists every
setting, and [security.md](../security.md#llm_exec) what each one changes.

These settings bound Gash, not the agent. An agent that also has a general
shell tool can run any command, or edit `~/.gash_env`, without going
through Gash. To contain such an agent, run it in a container or a VM.

## What Gash refuses

Whatever the agent asks, these rails hold:

- **Secret files.** `.env` files, private keys, credential files and the
  directories that hold them (`~/.ssh`, `~/.aws`, ...) are never read or
  written: `secret_file_blocked`. `llm_dotenv` tells whether keys are set
  in a `.env` file, with the secret values masked.
- **Protected paths.** `/proc`, `/sys`, `/dev`, `/boot`, `/root`,
  `/etc/shadow` and the other system files are refused: `forbidden_path`.
- **System paths for writes.** An `agent_*` write to `/bin`, `/usr/lib`,
  `/etc/ssh` and the like is refused even with `--force`
  (`write_path_denied`). A write to `/etc`, `/var`, `/usr/local`, `/root`
  or a `.ssh` directory outside your home and project is `sensitive_path`:
  the agent must ask you, then pass `--force`.
- **Databases** are read-only: a write statement is refused before it
  reaches the server, and the session itself is read-only.
- **Git.** `agent_git_commit` commits only the paths it is given (`-A`
  only when asked), never force-pushes, and has no `--amend` or
  `--no-verify`. The `llm_git_*` reads run git with the repository's
  hooks, fsmonitor, external diff, textconv and pager turned off.
- **Processes.** `agent_kill` signals only your own processes, never
  PID 1, the agent itself or its parents.
- **Confirmations.** A function that asks a person before it acts (killing
  a process by name, pruning Docker, `gash hte site remove`) refuses to run
  for an agent without `-y` or `--yes`. The agent should pass it only after
  you agreed.

[security.md](../security.md) describes each check and what it does not
protect against.

## Keep a log of agent calls

With `GASH_TELEMETRY_FILE` exported, every `llm_*` and `agent_*` call
appends one JSON line to that file: the time, the function, the duration in
milliseconds, the bytes of output, the exit status and the number of
truncated lines. It never records an argument, a path or a value, and
never leaves the machine.

<!-- example
mkdir shop && printf '# Shop\n' >shop/README.md && printf 'KEY=v\n' >shop/.env
mask: "ts":[0-9]+
mask: "ms":[0-9.]+
-->
```console
$ export GASH_TELEMETRY_FILE=~/gash-calls.jsonl
$ llm_read shop/README.md >/dev/null
$ llm_read shop/.env 2>/dev/null
$ cat ~/gash-calls.jsonl
{"ts":1793004212,"fn":"llm_read","ms":0.308,"out_bytes":36,"rc":0,"truncated":0}
{"ts":1793004212,"fn":"llm_read","ms":0.058,"out_bytes":0,"rc":1,"truncated":0}
```

The second call read a secret file: Gash refused it, so the line has exit
status 1 and no output bytes.

## When something goes wrong

| Error | Meaning and what to do |
|---|---|
| `command_not_allowed` | `llm_exec` refused a command that is not on its read-only list. If you agree, add the command to `GASH_LLM_EXEC_ALLOW` in `~/.gash_env`. |
| `option_not_allowed` | An allowed command with an option that writes or follows symlinks (`find -delete`, `sed -i`). The dedicated function usually does the job: `agent_replace` for `sed -i`. |
| `secret_file_blocked`, `forbidden_path` | A secret file or a protected path. Final: the agent must not try another route. |
| `sensitive_path` | A write under `/etc`, `/var`, `/usr/local`, `/root` or a `.ssh` directory. Rerun with `--force` only if you want that change. |
| `confirmation_required` | The function asks a person first. Run it yourself, or let the agent pass `--yes` once you agreed. |
| `sandbox_unavailable` | `GASH_LLM_SANDBOX=1` is set but `bwrap` is missing or cannot run. Install bubblewrap, or remove the setting. |
| `prerequisite_missing` (or a tool's own code, such as `docker_not_found`) | A program the function needs is not installed. The `install` member holds the command: run it, or `gash_doctor --install TOOL` in a terminal. |
| `binary_not_found` | The gash binary of this version is missing and could not be downloaded. The hint names the fix, usually `bash ~/.gash/install.sh --repair`. |
| `gash: command not found` | The agent calls `gash FUNCTION` but the launcher is not on its `PATH`: link `~/.gash/bin/gash` as shown above, or use headless mode. |

The full list of error codes and their actions is in
[AGENTS.md](../../AGENTS.md).

## See also

- [reference.md](reference.md): every `llm_*` and `agent_*` function, and
  the `gash agent-functions` and `gash agents-hint` commands.
- [AGENTS.md](../../AGENTS.md): the contract agents follow.
- [security.md](../security.md): the guardrails in detail.
- [configuration.md](../configuration.md): `~/.gash_env` and every setting.
- [Databases](../databases/README.md) and [Remote hosts](../remote/README.md):
  the `DB:` and `HOST:` entries agents use.
