# Shell

When Gash loads in an interactive Bash, the shell gets a prompt that shows
the git branch and the status of the last command, unlimited history with
timestamps and two ways to search it, short aliases to move around,
interactive forms of the commands that overwrite or delete, and a help card
that lists every function. This page explains each part, what it changes in
your shell, and the setting that turns it off.

Platforms: Linux, macOS and WSL, in an interactive Bash. A headless shell
(`GASH_HEADLESS=1`) gets none of it: no prompt, history policy or aliases.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [fzf](../requirements.md#tool-fzf) (optional), for `hgrep` (--pick) and `history_grep` (--pick)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

The settings named on this page go in `~/.gash_env` as `NAME=value` lines and
take effect in the next shell; [configuration](../configuration.md) describes
each one, and how a variable set before Gash loads wins over the file.

## Read the prompt

The prompt is built again before every command, from left to right:

| Part | Shown when | Looks like |
|---|---|---|
| chroot name | `debian_chroot` is set (from `/etc/debian_chroot`) | `(name)` |
| virtualenv | a Python virtualenv is active | `(venv)` |
| user | always: green, red for root, blue when `$USER` and `$LOGNAME` differ | `ada` |
| host | only over SSH, in a color fixed per machine | `@web1` |
| directory | always, with `~` for your home | `:~/src/app` |
| git | inside a git work tree | `(main*↑1↓2)` |
| jobs | running or stopped jobs in this shell | `[j2]` |
| screen | you have GNU screen sessions; the number of the current one when inside it | `[s2:4711]` |
| exit status | the last command failed | `[!127]` |
| sign | always: `$`, `#` for root | `$` |

The git part names the branch, or the short commit hash on a detached HEAD.
`*` means tracked files have changes (new untracked files do not count),
`↑N` commits to push and `↓N` commits to pull (`+N` and `-N` when the locale
is not UTF-8). On terminals that have a title (xterm, GNOME, Konsole, kitty,
tmux, screen and similar) Gash also sets it to `~/src/app [ada]`, or to
`ada@web1:~/src/app` over SSH.

Names the prompt shows (branch, virtualenv, chroot, screen session) are
printed as they are: a branch named `$(rm -rf ~)` in a cloned repository is
text, never a command.

To change what the prompt does:

- `GASH_PROMPT=0` keeps your own `PS1`. History still works as described
  below. `GASH_PROMPT=0 bash` starts one shell with your own prompt.
- `GASH_GIT_EXCLUDE=/srv/huge-repo:/home/ada/monorepo` leaves out the git part
  in those repositories: the prompt runs `git status` at every command, which
  is slow in a large work tree. The value is a colon-separated list of
  repository roots, matched exactly. A repository whose root is your home
  directory (a dotfiles repository) never shows the git part.
- `NO_COLOR=1` or `GASH_NO_COLOR=1` turn the colors off from the next prompt.

Gash adds its hook in front of `PROMPT_COMMAND` and keeps what was there, so
direnv, VTE and similar tools keep working.

## Move around

The navigation aliases climb directories: `..` runs `cd ..`, `...` two levels,
`....` three, `.....` and `.4` four, `.5` five (`cd..` is there for the typo).
`mkcd` ([`mkdir_cd`](reference.md#mkdir_cd)) creates a directory, with its
parents, and enters it:

<!-- example
mkdir -p projects
-->
```console
$ mkcd projects/client/frontend
$ pwd
~/projects/client/frontend
$ ...
$ pwd
~/projects
```

`mkdir_cd` takes exactly one directory, taken as a name even when it starts
with `-`, and prints nothing. The listing aliases add the color option of your
`ls`: `ll` (long), `la` (long, with hidden files), `l` (columns), `lash` (long,
hidden, human sizes). `path` prints `PATH` one directory per line, and `cls`
clears the screen. `gash nav` lists the navigation aliases exactly as your
shell defines them.

## Put directories on PATH once

[`path_add`](reference.md#path_add) puts directories in front of `PATH`, in
the order given, or at the end with `--append`. It skips a directory that does
not exist and leaves one that is already on `PATH` where it is, so running it
again never makes `PATH` longer:

<!-- example
mkdir -p .local/bin tools/bin
-->
```console
$ PATH=/usr/bin:/bin
$ path_add ~/.local/bin ~/.cargo/bin
$ path_add --append ~/tools/bin
$ path_add ~/.local/bin
$ path
~/.local/bin
/usr/bin
/bin
~/tools/bin
```

`~/.cargo/bin` does not exist here, so it was skipped. To do this in every new
shell, list the directories in `GASH_PATH_ADD` instead of calling `path_add`
from your own files.

## Search your history

With the default `GASH_HISTORY=1`, Gash keeps every command: the history has
no size limit, each command is written to `~/.bash_history` with its time as
soon as it finishes, and commands that start with a space or repeat the one
before are not saved (unless your own `HISTCONTROL` says otherwise).
`GASH_HISTORY_SYNC=1` also reads the file again before each prompt, so a
terminal sees the commands typed in the others. `GASH_HISTORY=0` leaves your
history settings alone.

Two functions search it:

| | [`hgrep`](reference.md#hgrep) | [`hg`](reference.md#history_grep) (`history_grep`) |
|---|---|---|
| Reads | the history file, which every shell writes to | the history of this shell: what it loaded when it started and what you typed since |
| Pattern | fixed text, or a regular expression with `-E` | a basic regular expression (grep) |
| Shows | each command once, with the time of its last run | each command once, with a row number |
| Extras | count, JSON, newest first, limit, fuzzy pick | none |

Both ignore case and leave out their own calls. `hgrep` lists the matches
oldest first:

<!-- example
printf '%s\n' '#1772442900' 'git pull --rebase' '#1772443200' 'make test' '#1772445900' 'git push origin main' '#1772445960' 'ssh deploy@web1' '#1772526600' 'git pull --rebase' '#1772559900' 'git push origin main' '#1772622000' 'git push --force-with-lease origin fix-login' >.bash_history
-->
```console
$ hgrep "git push"
[2026-03-03 17:45:00] git push origin main
[2026-03-04 11:00:00] git push --force-with-lease origin fix-login
$ hgrep -E "^git (push|pull)" -r -n 2
[2026-03-04 11:00:00] git push --force-with-lease origin fix-login
[2026-03-03 17:45:00] git push origin main
$ hgrep git -c
3
$ hgrep ssh --json
[{"timestamp":"2026-03-02 10:06:00","epoch":1772445960,"count":1,"command":"ssh deploy@web1"}]
```

`-n N` keeps the N most recent matches, `-r` puts the newest first, `-c`
counts the distinct commands, and `--json` gives each one with the number of
times it ran. On a terminal, matches are highlighted and `-g` groups commands
that share a prefix such as `cd app &&`. `--pick` opens the matches in `fzf`
and prints the one you choose. A pattern that starts with `-` goes after
`--`: `hgrep -- --force`.

`hgrep` only sees commands saved with a timestamp. Commands saved before Gash
managed your history, or with `GASH_HISTORY=0`, have none: `hg` still finds
them while they are in the history of the current shell.

## Find a function: the help card

`gash` alone prints the card: one row per topic, and where to go next. The
alias counts follow what is installed: an alias whose tool is missing, or
that a program of the same name hides, is not counted.

<!-- example
mask: [0-9]+ +[0-9]+ {2}
-->
```console
$ gash

╭───────────────────────────────────────────────────╮
│  Gash 3.4.0 · Bash for humans and AI agents       │
│  121 functions · gash <topic> · gash_help <name>  │
╰───────────────────────────────────────────────────╯

  TOPIC   FUNCTIONS  ALIASES  WHAT
  ────────────────────────────────────────────────────────────────────────────────────
  files           7        0  Largest files and folders, tree stats, archives, backups
  system         18       12  Processes, ports, services, disks, history, WSL
  nav             0       17  Directory navigation and listings
  git             7       38  Sync, tags, patches, repository repair, git shortcuts
  docker          7       19  Containers and Compose stacks
  remote          7        0  Hosts from ~/.gash_env: exec, push, pull, sync
  dev             5        4  PHP and Node.js versions, headless Chrome
  llm            48        0  Read-only JSON tools for AI agents
  agent           6        0  Write tools for AI agents
  ai              3        0  Ask Claude or Gemini from the terminal
  gash           13        0  Help, doctor, configuration, upgrade

  gash <topic>        functions and aliases of a topic, e.g. gash git
  gash all            the whole card
  gash_help <name>    full help with examples, e.g. gash_help flf
  gash_help --search  search every help page
  gash_doctor         check the installation
```

`gash git` shows the functions of one topic with their short alias and usage,
then the aliases your shell defines for it; `gash all` shows every topic.
[`gash_help`](../gash/reference.md#gash_help) gives the full help of a
function or of its alias (`gash_help flf`), with examples;
`gash_help --search WORD` searches every help page, and
`gash_help --short NAME` prints the one-line description:

<!-- example -->
```console
$ gash_help --short dfl
Find directories exceeding a size threshold (single-walk aggregation)
```

Every function also answers `-h` or `--help`. Gash replaces the `help`
builtin with a small function: `help NAME` shows the Gash help of a function
or alias, and Bash's own help for builtins and keywords (`help cd`).

### Give your own functions the same help

Two helpers of Gash work in your own functions, in `~/.bash_local` for
example. `needs_help PROGRAM USAGE TEXT "${1-}"` prints a short help and
returns 0 when its last argument, your function's first one, is `-h` or
`--help`, and returns 1 otherwise:

<!-- example
printf 'one\ntwo\n' >list.txt
-->
```console
$ count_lines() { needs_help count_lines "count_lines FILE" "Print the number of lines of FILE." "${1-}" && return; wc -l <"$1"; }
$ count_lines --help
count_lines
Usage: count_lines FILE
Print the number of lines of FILE.
$ count_lines list.txt
2
```

`needs_confirm_prompt "Delete the cache?"` asks the question, reads a `[y/N]`
answer from the terminal and returns 0 only for `y` or `Y`. In headless mode
it returns 1 without asking, and so it does when there is no terminal to read
the answer from.

## Colors

Gash output is colored on a terminal and plain when piped. `NO_COLOR` (any
value) or `GASH_NO_COLOR` (any value but `0`) turn the colors off everywhere,
the prompt included; many functions also take `--no-color` for one call.
[`all_colors`](reference.md#all_colors) prints the 256 terminal colors, each
line `colourN` drawn in color N, to see what your terminal renders and pick a
number for `\e[38;5;Nm`.

In the shell, `grep` gets `--color=auto` when your grep supports it, and the
`ls` aliases get the color option of your `ls`. When `LS_COLORS` is not set,
Gash sets it from `dircolors`, which reads `~/.dircolors` when you have one.

## The safety aliases

By default, the commands that overwrite or delete ask first and say what they
do:

<!-- example -->
```console
$ alias cp mv rm
alias cp='cp -iv'
alias mv='mv -iv'
alias rm='rm -Iv --one-file-system --preserve-root'
```

`cp` and `mv` ask before they overwrite a file and print each file they
handle. `rm -I` asks once before it removes more than three files or removes
recursively, stays on one filesystem, and refuses to remove `/`. `mkdir` also
creates missing parents, and `chown`, `chmod` and `chgrp` refuse to work on
`/` recursively. The `rm`, `chown`, `chmod` and `chgrp` forms need the GNU
tools; elsewhere (macOS) `rm` becomes `rm -iv`, which asks for every file. The
[safety pack](reference.md#the-safety-pack) lists them all.

To run the plain command once, put a backslash in front of it: `\rm -r build`.
To turn the pack off, set `GASH_SAFETY_ALIASES=0`; in the next shell:

<!-- example
printf 'GASH_SAFETY_ALIASES=0\n' >.gash_env
-->
```console
$ alias cp
bash: alias: cp: not found
$ alias bc
alias bc='bc -l'
```

The same pack also defines `bc` (with the math library), `ports` (the open
sockets, through `ss` or `netstat`), `reboot` and `shutdown` (both through
`sudo`): those stay defined with `GASH_SAFETY_ALIASES=0`.

## Richer replacements of standard tools

`GASH_REPLACE_TOOLS=1` aliases some standard commands to their richer
replacements, each only when the replacement is installed: for example `top`
runs `htop` and `diff` runs `colordiff`, and `less` runs `most`, which also
becomes `PAGER`. In the PHP pack, `php` and `composer` then run the newest
installed `phpX.Y` ([runtimes](../runtimes/README.md)). The default, `0`,
leaves the standard commands alone. [Configuration](../configuration.md) lists
every replacement.

## What else the shell layer sets

When a shell starts, Gash also:

- prints a banner line with its version and your user name, then a quote
  ([`gash_inspiring_quote`](reference.md#gash_inspiring_quote); `--source`
  says where it comes from). `GASH_BANNER=0` hides both.
- sets `EDITOR` to `nano`, or `vim`, when you have not chosen one.
- applies `GASH_UMASK` (an octal mask such as `027`) when you set it.
- sets `LESSOPEN` so `less` can show archives and other files `lesspipe`
  knows, when `lesspipe` is installed and `LESSOPEN` is not set.
- refuses `write` and `talk` messages on the terminal (`mesg n`).

A Gash alias never hides an installed program of the same name, apart from
the ones that wrap that program on purpose (`ls`, `cp`, `rm`, `grep` and the
like), and never replaces an alias you defined before Gash loaded. Your
`~/.bash_aliases` and `~/.bash_local` load after Gash and can redefine any
alias or function.

## When something goes wrong

| What you see | What it means and what to do |
|---|---|
| An alias of this page does not exist (`type ll` finds nothing) | A program of the same name is installed, or you defined the name before Gash loaded. `type NAME` shows what runs; define the alias again in `~/.bash_aliases` |
| The prompt is yours, not Gash's, or the git part stopped updating | `GASH_PROMPT` is not `1`, or a tool that loads after Gash assigned `PROMPT_COMMAND` instead of adding to it. Load that tool before Gash, or have it append to `PROMPT_COMMAND` |
| The prompt is slow inside one repository | The git part runs `git status` at each prompt. Add the repository root to `GASH_GIT_EXCLUDE` |
| `Error: Cannot read history file: PATH` | `hgrep` reads `HISTFILE` (default `~/.bash_history`), which does not exist yet or is not readable. It exists once a command has run with `GASH_HISTORY=1` |
| `hgrep` finds nothing that `hg` finds | Those commands have no timestamp in the history file (see [Search your history](#search-your-history)) |
| `Error: Invalid regex: PATTERN` | The `-E` pattern is not a valid regular expression. Without `-E` the pattern is plain text |
| `Error: --pick requires fzf; install it or drop --pick` | Install `fzf` ([requirements](../requirements.md#tool-fzf)). `--pick` also needs a terminal: `Error: --pick requires an interactive terminal` |
| `Error: Unexpected argument: NAME` from `mkcd` | `mkdir_cd` takes one directory; quote a name that contains spaces |
| `Error: Unknown topic: NAME. Topics: ...` | `gash` takes one of the topics it lists, or `all` |
| `Error: No help found for 'NAME'.` | No function or alias has that name; the hint suggests `gash_help --search NAME` |
| `Warning: GASH_UMASK must be an octal mask such as 022 or 077 (ignored)` | Set three or four octal digits, or remove the setting |

## See also

- [reference.md](reference.md): every function and alias of this area, with all options.
- [Configuration](../configuration.md): `~/.gash_env` and every `GASH_*` setting.
- [Gash itself](../gash/README.md): `gash_help`, `gash_doctor`, updates and removal.
- [Git](../git/README.md): the git aliases and the `gl` log functions.
