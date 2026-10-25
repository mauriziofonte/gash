# gash

Help, doctor, configuration, upgrade.

`gash_doctor`, `gash_env_init`, `gash_uninstall`, `gash_upgrade` and
`gash_username` refuse an unknown option (`Error: Unknown option: X`) or an
unexpected argument (`Error: Unexpected argument: X`) with status 2, before
they change anything.

_Written from the help registry (`gash_help NAME`); `go test ./internal/registry` checks it against the help text._

| Function | Alias | Description |
|---|---|---|
| [`gash`](#gash) |  | Gash reference card with all functions and aliases |
| [`gash_ai_list`](#gash-ai-list) |  | List configured AI providers |
| [`gash_db_list`](#gash-db-list) |  | List configured database connections |
| [`gash_db_test`](#gash-db-test) |  | Test a database connection |
| [`gash_doctor`](#gash-doctor) | `gdoctor` | Run health checks on the Gash installation |
| [`gash_env_init`](#gash-env-init) |  | Create ~/.gash_env configuration template |
| [`gash_help`](#gash-help) |  | Help system with examples, search, and discovery |
| [`gash_inspiring_quote`](#gash-inspiring-quote) |  | Display a random quote, optionally with its source |
| [`gash_migrate`](#gash-migrate) |  | Shows how to migrate a Gash 1.x configuration |
| [`gash_uninstall`](#gash-uninstall) |  | Uninstall Gash and clean up configurations |
| [`gash_unload`](#gash-unload) |  | Restore shell state to before Gash was loaded |
| [`gash_upgrade`](#gash-upgrade) |  | Upgrade Gash to the latest version |
| [`gash_username`](#gash-username) |  | Print the current user name |

## gash

Gash reference card with all functions and aliases

#### Usage

```text
gash [TOPIC|all] [--no-color]
gash migrate [...]   Same as gash_migrate (Gash 1.x configuration)

gash                 Overview: topics with function and alias counts
gash TOPIC           Functions (alias, usage, description) and aliases of a topic
gash all             The whole card, every topic
```

#### Topics

```text
files, system, nav, git, docker, remote, dev, llm, ai, gash
(navigation, management, php and node still work as old names)

The card is built from the help registry and the alias files. In an
interactive shell it counts and lists only the aliases the shell
defines (one skipped because a program of that name exists, or a pack
whose tool is missing, is left out); headless it lists them all.
```

#### Examples

```bash
# Quick reminder of all git aliases
gash git

# See what Docker functions are available
gash docker

# See every function and alias in Gash, plain text for a file
gash all --no-color > gash-card.txt

# Check AI-related functions
gash ai
```

See also: `gash_help`, `gash_doctor`

## gash_ai_list

List configured AI providers

#### Usage

```text
gash_ai_list
```

#### Examples

```bash
# Show configured AI providers
gash_ai_list
```

#### Notes

```text
Configure providers in ~/.gash_env with:
  AI:claude=sk-ant-api03-xxxxx
  AI:gemini=AIzaSyxxxxx
The first configured provider is used by default.
```

See also: `ai_ask`, `ai_query`, `gash_env_init`

## gash_db_list

List configured database connections

#### Usage

```text
gash_db_list
```

#### Examples

```bash
# Show all database connections from ~/.gash_env
gash_db_list
```

#### Notes

```text
Passwords are masked in output.
Configure connections in ~/.gash_env with:
  DB:name=driver://user:pass@host:port/database
Supported drivers: mysql, mariadb, pgsql
```

See also: `gash_db_test`, `gash_env_init`

## gash_db_test

Test a database connection

#### Usage

```text
gash_db_test [CONNECTION_NAME]
```

#### Examples

```bash
# Test the default connection
gash_db_test

# Test a named connection
gash_db_test postgres
gash_db_test legacy
```

See also: `gash_db_list`, `gash_env_init`

## gash_doctor

Run health checks on the Gash installation

Alias: `gdoctor`

#### Usage

```text
gash_doctor [--timing] [--no-color]
```

#### Options

```text
--timing    What each file and step costs when a shell starts
--no-color  Plain output
```

#### Examples

```bash
# Run a full health check
gash_doctor

# Find what makes new shells slow
gash_doctor --timing
```

#### Notes

```text
Checks: core files, modules, the binary and gash-hosts-helper
(against the hashes release/SHA256SUMS pins), the generated
shims (against this gash version), config file permissions,
external tools (git, curl, jq, sqlite3, docker).
Reports missing or misconfigured components and returns 1
when a check fails (2 for an unknown option).

--timing loads Gash in a new interactive shell and lists the
milliseconds each file and step takes (5 ms or more stands out),
then times whole new shells: with your ~/.bashrc, and bash alone,
3 times each (a line says first that your ~/.bashrc runs 3 times).
Needs Bash 5.0 or newer. Starting a shell with GASH_TIMING=1 keeps
the same figures in its __GASH_LOAD_TIMES array.
```

See also: `gash_upgrade`, `gash`

## gash_env_init

Create ~/.gash_env configuration template

#### Usage

```text
gash_env_init [--force]
```

#### Examples

```bash
# Create config from template (will not overwrite existing)
gash_env_init

# Overwrite existing config with fresh template
gash_env_init --force
```

#### Notes

```text
Creates ~/.gash_env with template sections for:
  SSH:~/.ssh/id_ed25519=your_passphrase
  DB:default=mysql://user:pass@host:port/db
  AI:claude=sk-ant-api03-xxxxx
Always set permissions: chmod 600 ~/.gash_env
```

See also: `gash_db_list`, `gash_ai_list`, `gash_ssh_auto_unlock`

## gash_help

Help system with examples, search, and discovery

#### Usage

```text
gash_help                    Overview of all functions
gash_help FUNCTION           Detailed help with real-world examples
gash_help ALIAS              Lookup by short alias name
gash_help --list             List all functions grouped by module
gash_help --search KEYWORD   Search across all help text
gash_help --short FUNCTION   One-line description (for scripting)
gash_help --bash TOPIC       Bash builtin help (e.g. cd, export)
```

#### Examples

```bash
# Get detailed help for a function
gash_help ai_query

# Don't remember the long name? Use the alias
gash_help flf

# Find all Docker-related functions
gash_help --search docker

# Find functions that work with pipes
gash_help --search pipe

# Get just the description for scripting
gash_help --short files_largest

# Access bash builtin help
gash_help --bash cd
```

See also: `gash`

## gash_inspiring_quote

Display a random quote, optionally with its source

#### Usage

```text
gash_inspiring_quote [--source]
```

#### Examples

```bash
# The quote shown under the banner
gash_inspiring_quote

# Where it was said or written
gash_inspiring_quote --source
```

#### Notes

```text
Quotes live in quotes/list.txt: one `"Text." — Author` line each,
under a `# ` line with the source (book, essay, talk, mail; a link
where one exists). New quotes need a source that can be checked;
see CONTRIBUTING.md.
```

## gash_migrate

Shows how to migrate a Gash 1.x configuration

#### Usage

```text
gash_migrate
(also: gash migrate)

Gash 3 does not migrate a Gash 1.x configuration. gash_migrate prints
one line on stderr with the route and returns 1: check out v2.1.0 in
your Gash directory, open a new terminal, run gash_migrate there (the
2.x guided review, with a diff preview and backups), then gash_upgrade.
It writes nothing.

While ~/.gashrc is still the 1.x loader, an interactive shell prints a
one-line reminder on stderr, at most once a day. Headless shells print
nothing.
```

#### Files

```text
~/.local/state/gash/schema     configuration schema version (2 or more:
                               no reminder)
~/.local/state/gash/notice     time of the last reminder
(under $XDG_STATE_HOME/gash when that is set)
```

#### Examples

```bash
# Show the route
gash migrate
```

See also: `gash`, `gash_doctor`, `gash_upgrade`

## gash_uninstall

Uninstall Gash and clean up configurations

#### Usage

```text
gash_uninstall
```

#### Examples

```bash
# Remove Gash completely (interactive confirmation)
gash_uninstall
```

#### Notes

```text
Removes the Gash sourcing block from your shell profile,
deletes the installation directory, and cleans up.
Kept as FILE.gash-uninstall.bak: each profile as it was
before the edit, and a ~/.gashrc that holds your own lines.
Restart your terminal after uninstalling.
```

See also: `gash_upgrade`, `gash_unload`

## gash_unload

Restore shell state to before Gash was loaded

#### Usage

```text
gash_unload
```

#### Examples

```bash
# Temporarily disable Gash in the current terminal
gash_unload

# Re-enable Gash without restarting
source ~/.gashrc
```

#### Notes

```text
Removes all functions, aliases, and settings introduced by Gash.
Restores PS1, PROMPT_COMMAND, history settings, and shopt options
to the state captured before Gash loaded. Session-only: does not
modify any files.
```

See also: `gash_uninstall`

## gash_upgrade

Upgrade Gash to the latest version

#### Usage

```text
gash_upgrade
```

#### Examples

```bash
# Upgrade to latest version
gash_upgrade
# Then restart your terminal to apply changes
```

#### Notes

```text
Fetches the release tags and checks out the newest release (never
an older one). It runs the steps of gash update. A checkout newer
than the latest release (a pre-release, or a commit after it) is
left as it is.
Before the checkout, the release binary is downloaded into
~/.local/share/gash/versions/<version>/ and verified (the hash in
the tag's release/SHA256SUMS, else the release's checksums.txt;
GASH_REQUIRE_SIGNED=1 also requires a signed tag or a Sigstore
release signature that cosign verifies, and a pinned binary).
After it, the new version is loaded in a test shell; if that
fails, the previous version is checked out again.
Open a new terminal afterwards to load the new version.
To go back: gash update --rollback (the previous version and its
binary are kept), or git -C ~/.gash checkout vX.Y.Z
```

See also: `gash_doctor`, `gash_uninstall`

## gash_username

Print the current user name

#### Usage

```text
gash_username
```

#### Examples

```bash
# The name the banner and the prompt show
gash_username
```

#### Notes

```text
Tries whoami, then id -un, then the /etc/passwd entry of the
current user id; prints UNKNOWN_USERNAME when all of them fail.
```

