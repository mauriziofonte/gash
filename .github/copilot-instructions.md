# Gash - Project Overview

## Architecture

Gash is a **sourced** Bash framework for interactive shells.

```
~/.gashrc → ~/.gash/gash.sh → lib/core/*.sh → lib/modules/*.sh → lib/aliases.sh → lib/prompt.sh
```

### Directory Structure

```
lib/
├── core/           # Internal libraries (prefixed __gash_*)
│   ├── config.sh   # ~/.gash_env parser (__gash_load_env, __gash_get_db_url, __gash_parse_db_url)
│   ├── output.sh   # __gash_info, __gash_error, __gash_success, __gash_warning, __gash_step
│   ├── validation.sh # __gash_require_*, argument/file/git checks
│   └── utils.sh    # needs_help, needs_confirm_prompt, __gash_tty_width, __gash_trim_ws
├── modules/        # Feature modules (LONG name + short alias)
│   ├── docker.sh   # docker_stop_all (dsa), docker_start_all (daa), docker_prune_all (dpa)
│   ├── files.sh    # files_largest (flf), dirs_largest (dld), dirs_find_large (dfl),
│   │               # dirs_list_empty (dle), archive_extract (axe), file_backup (fbk)
│   ├── gash.sh     # gash_help, gash_upgrade, gash_uninstall, gash_unload, gash_inspiring_quote
│   ├── git.sh      # git_list_tags (glt), git_add_tag (gat), git_delete_tag (gdt),
│   │               # git_dump_revisions (gdr), git_apply_patch (gap)
│   ├── llm.sh      # LLM utilities (llm_exec, llm_tree, llm_find, llm_grep, llm_db_*, etc.)
│   │               # NO short aliases - designed for AI agents, excludes from bash history
│   ├── ssh.sh      # gash_ssh_auto_unlock
│   └── system.sh   # disk_usage (du2), history_grep (hg), ip_public (myip),
│                   # process_find (pf), process_kill (pk), port_kill (ptk),
│                   # services_stop (svs), sudo_last (plz), mkdir_cd (mkcd)
├── aliases.sh      # Aliases and environment setup
└── prompt.sh       # Greeting banner
```

## Coding Standards

### Strict Mode Compatibility

All code must work with `set -euo pipefail`:

```bash
# ALWAYS use ${1-} for optional params
local arg="${1-}"

# ALWAYS use || true for commands that may fail
BINARY=$(type -P "$cmd" 2>/dev/null) || true

# NEVER exit from sourced code - use return
__gash_error "Failed"; return 1
```

### Function Naming

| Pattern         | Scope           | Example                              |
| --------------- | --------------- | ------------------------------------ |
| `long_name`     | Public API      | `git_add_tag`, `files_largest`       |
| `short`         | Public alias    | `gat`, `flf`                         |
| `__gash_*`      | Internal helper | `__gash_require_arg`, `__gash_info`  |
| `__function_*`  | Module-private  | `__gash_generate_expect_script`      |

### Output Functions (lib/core/output.sh)

```bash
__gash_info "message"      # Cyan "Info:" prefix
__gash_error "message"     # Red "Error:" to stderr, returns 1
__gash_success "message"   # Green "OK:" prefix
__gash_warning "message"   # Yellow "Warning:" prefix
__gash_step 1 5 "message"  # "[1/5]" step indicator
```

### Validation Functions (lib/core/validation.sh)

```bash
__gash_require_arg "$value" "name" "usage hint" || return 1
__gash_require_file "$path" || return 1
__gash_require_dir "$path" || return 1
__gash_require_command "cmd" "error msg" || return 1
__gash_require_git_repo || return 1
__gash_require_git_repo_with_remote || return 1
__gash_require_docker || return 1
```

### Standard Function Template

```bash
function_name() {
    needs_help "function_name" "function_name ARG" \
        "Description of what this does." \
        "${1-}" && return

    local arg="${1-}"
    __gash_require_arg "$arg" "argument" "function_name <arg>" || return 1

    # Implementation...
    __gash_info "Doing something..."
}
```

## Testing

### Run Tests

```bash
bash tests/run.sh
```

### Test Structure

Tests use `gash_source_all "$ROOT"` to load all modules:

```bash
it "test name" bash -c '
  set -euo pipefail
  ROOT="${GASH_TEST_ROOT}"
  source "$ROOT/tests/gash-test.sh"; gash_source_all "$ROOT"

  # Test code...
'
```

### Mocking

-   Mock binaries: `tests/mocks/bin/`
-   Set PATH: `export PATH="$ROOT/tests/mocks/bin:$PATH"`
-   Bash builtins: override with functions inside test

## Key Behaviors

### Shell State Management

`gash.sh` snapshots shell state before modifications. `gash_unload` restores:

-   PS1, PS2, PROMPT_COMMAND
-   HISTCONTROL, HISTSIZE, HISTFILESIZE
-   shopt settings (histappend, checkwinsize)
-   Functions and aliases added by Gash

### Configuration System (`~/.gash_env`)

Unified config file for SSH keys and database credentials. Parsed by `lib/core/config.sh`.

#### File Format

```bash
# SSH keys: SSH:keypath=passphrase
SSH:~/.ssh/id_ed25519=my passphrase

# Database: DB:name=driver://user:password@host:port/database
DB:default=mysql://root:pass@localhost:3306/myapp
DB:postgres=pgsql://pguser:secret@dbhost:5432/analytics
```

#### Config Functions (lib/core/config.sh)

```bash
__gash_load_env()          # Load and cache ~/.gash_env
__gash_get_ssh_keys()      # Get SSH key entries (TAB-separated keypath\tpassphrase)
__gash_get_db_url()        # Get DB URL by connection name
__gash_parse_db_url()      # Parse URL into driver/user/pass/host/port/db
__gash_url_decode()        # Decode %XX sequences
__gash_url_encode()        # Encode special characters

# Public helpers
gash_db_list()             # List available DB connections
gash_db_test()             # Test a DB connection
gash_env_init()            # Create ~/.gash_env from template
```

#### Environment Variables

```bash
GASH_ENV_FILE              # Override config file path (default: ~/.gash_env)
```

### SSH Auto-Unlock

Invoked from `lib/prompt.sh` if SSH keys configured in `~/.gash_env`:

-   Config: `~/.gash_env` with `SSH:keypath=passphrase` entries
-   Auto-start agent: automatic when SSH keys are configured

### Completion

`bash_completion` uses `__GASH_ADDED_FUNCS` to suggest public Gash functions.
Internal functions (`__*`, `_*`) are filtered.

## Common Patterns

```bash
# Command availability check
if type -P cmd >/dev/null 2>&1; then ...

# errexit-safe prompt
if ! needs_confirm_prompt "Continue?"; then return 0; fi

# Temp directory with cleanup
tmp="$(mktemp -d)"; trap "/bin/rm -rf $tmp" EXIT

# Local variables always
local var="value"
```

## Defensive Programming

### errexit-safe External Commands

```bash
# Network calls that may fail
ip=$(curl -s https://api.example.com 2>/dev/null) || true
if [[ -z "$ip" ]]; then __gash_error "Failed"; return 1; fi

# Commands with fallbacks
user_name=$(whoami 2>/dev/null) || true
if [[ -z "$user_name" ]]; then user_name="UNKNOWN"; fi
```

### Case Statements with Fallback

```bash
local rc=0
case "$file" in
    *.tar.gz) tar xzf "$file" || rc=1 ;;
    *)        __gash_error "Unsupported"; rc=1 ;;
esac
return $rc
```

### Validation Before Operations

Always validate inputs before performing operations:

```bash
function_name() {
    __gash_require_arg "$1" "filename" "function_name <file>" || return 1
    __gash_require_file "$1" || return 1
    # Now safe to use $1
}
```

## Don'ts

-   Never `exit` from sourced code
-   Never read `$1` directly - use `${1-}`
-   Never leave temp variables at file scope
-   Never use `command -v` for binary detection (returns aliases) - use `type -P`
-   Never hardcode ANSI colors - use `__gash_*` output functions
-   Never assume commands succeed - always handle failures with `|| true` or `|| return 1`

## LLM Module (lib/modules/llm.sh)

The LLM module provides utilities optimized for AI coding assistants. It has special requirements.

### LLM Module Design Principles

| Principle | Reason |
| --------- | ------ |
| **JSON output** | Machine-parseable, minimal token overhead |
| **No short aliases** | LLMs don't need quick typing - only `llm_*` names |
| **No bash history** | Use `__llm_no_history` wrapper to exclude from history |
| **Read-only default** | DB queries, file operations are non-destructive |
| **Defensive** | Validate all inputs, block dangerous commands |

### LLM Function Template

```bash
llm_function_name() {
    __llm_no_history  # Exclude from bash history

    needs_help "llm_function_name" "llm_function_name [OPTIONS] ARG" \
        "Description of what this does. Output: JSON" \
        "${1-}" && return

    local arg="${1-}"
    __gash_require_arg "$arg" "argument" "llm_function_name <arg>" || return 1

    # Validate/sanitize input
    arg=$(__llm_validate_path "$arg") || return 1

    # Implementation - output JSON or compact text
    echo '{"result":"value"}'
}
# NO short alias for LLM functions
```

### Security Helpers (Internal)

```bash
__llm_no_history              # Disable history for current command
__llm_validate_path "$path"   # Sanitize path, block traversal/dangerous paths
__llm_validate_command "$cmd" # Check against dangerous pattern blacklist
__llm_is_secret_file "$file"  # Check if file contains secrets
```

### Dangerous Pattern Blacklist

The `__LLM_DANGEROUS_PATTERNS` array blocks commands like:

-   Filesystem destruction: `rm -rf /`, `rm -rf ~`, `rm -rf *`
-   Disk operations: `dd if=`, `mkfs`, `> /dev/sd*`
-   System destruction: fork bombs, `shutdown`, `reboot`
-   Privilege escalation: `chmod -R 777 /`, `chown -R`
-   Remote code execution: `curl|bash`, `wget|sh`
-   Obfuscation attempts: backticks, `$()`, extra spaces, chained `cd /; rm`

### Secret File Detection

Files matching these patterns are blocked:

-   `.env`, `.env.*` (environment files)
-   `*.pem`, `*.key`, `*_rsa`, `*_dsa`, `*_ed25519` (keys)
-   `credentials*`, `secrets*`, `*password*`, `*token*`
-   `~/.ssh/*`, `~/.aws/*`, `~/.gash_env`

### Database Security

-   Credentials from `~/.gash_env` (never hardcoded)
-   Connection selected via `-c CONNECTION` parameter (default: `default`)
-   Only `SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN` allowed
-   `INSERT`, `UPDATE`, `DELETE`, `DROP`, `TRUNCATE`, `ALTER` blocked
-   SQL injection patterns detected and rejected

### Database Functions Usage

```bash
# Use default connection
llm_db_query "SELECT * FROM users"
llm_db_tables
llm_db_schema users

# Use specific connection
llm_db_query "SELECT * FROM orders" -c legacy
llm_db_tables -c postgres
llm_db_schema products -c remote
```

### Adding New LLM Functions

1. Use `llm_` prefix (no short alias)
2. Call `__llm_no_history` at function start
3. Validate all user inputs with `__llm_validate_*` helpers
4. Output JSON or newline-separated compact text (no ANSI colors)
5. Add tests in `tests/specs/llm_spec.sh` including security tests
