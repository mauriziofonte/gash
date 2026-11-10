# Moving to Gash 4.0

Gash 4.0 finishes what 3.5 started: every command has one name,
`gash NAMESPACE COMMAND`, and the flat function names of 3.x
(`files_largest`, `git_sync`, `sysinfo`) are no longer defined in a new
shell. `GASH_LEGACY_NAMES=1` brings them back for the whole 4.x series.
This page says what changed, how to find what your files still call, and
lists every former name with the command that replaces it.

## What changed

- **The former flat names leave the default.** A Gash 4.0 shell, interactive
  or headless, defines the commands (`gash files largest`), their
  shortcuts (`flf`), the agent functions (`llm_*`, `agent_*`) and the
  script helpers (`needs_help`, `needs_confirm_prompt`, `path_add`). A
  former name typed or called from a script is `command not found`.
  `gash_migrate` goes too; `gash migrate` stays.
- **`gash hte` is `gash localweb`.** The local Apache and PHP-FPM sites have a namespace that says what they are: `gash localweb install`, `gash localweb site create shop.test`. `gash hte` still runs it through 4.x; typed at the prompt it prints a tip once per session. HTE stands for *homelab test environment*, the name of hte-cli, the standalone PHP tool the stack began as: in 4.0 the name stayed in the guide's folder, in your files (`~/.config/gash/hte`, `hte.toml`), in the markers of the hosts file and in the error codes (`hte_not_installed`). Gash 4.1 renames all of them and moves an existing install ([Files of Gash 4.0](localweb/README.md#files-of-gash-40)); `gash hte` is deprecated there and goes away in 5.0.
- **`gash doctor --legacy`** lists the lines of your files that call a
  former name, with the command to use instead.

## What stays

- **The commands, the shortcuts and Tab completion** work as in 3.5.
- **The agent functions.** `llm_*` and `agent_*` are permanent names, and
  so are `gash agent-functions` and `gash agents-hint`
  ([AGENTS.md](../AGENTS.md)).
- **`gash_username`**, which prompts call and a 3.x install runs while it
  updates itself to 4.0.
- **The `gl*` git log functions** (`gl`, `gla`, `glo` and the others),
  which are their own shortcuts.

## Find what your files call

```bash
gash doctor --legacy                      # ~/.bashrc, ~/.bash_local and the other startup files
gash doctor --legacy ~/bin/backup.sh      # any file
gash doctor --legacy --json ~/bin/*.sh    # for a script
```

Each line names the file and line, the former name and its command; the status is 1 when it finds one. A name your files define for a command of your own (`alias hgrep='gash shell history-smart'`, `hgrep() { ... }`) is not a call and is not reported, nor is a word of a `gash` command (`gash ai sysinfo`). An interactive shell whose startup files call a former name says so once a day on stderr, until the files are clean or `GASH_LEGACY_NAMES=1` is set.

## Keep the former names for now

Add one line to `~/.gash_env`, then open a new terminal:

```bash
GASH_LEGACY_NAMES=1
```

A script that sources Gash headless can set it for itself:

```bash
GASH_HEADLESS=1 GASH_LEGACY_NAMES=1 bash -c 'source ~/.gash/gash.sh; files_largest ~ --limit 5'
```

With the setting, every former name runs its command with the same output and exit status, as in 3.5, and the tip of each one prints once per session at the prompt (`GASH_LEGACY_HINTS=0` turns the tips off). The setting works through Gash 4.x ([configuration](configuration.md)).

To keep one name you type often, and no other, make it your own alias in `~/.bash_local`, which keeps working after 4.x: `alias hgrep='gash shell history-smart'`. Gash does not report it as a former name.

## Every former name

<!-- BEGIN generated: names (go test ./internal/docgen -update) -->
| Former name | Command | Shortcut |
|---|---|---|
| `ai_ask` | `gash ai ask` | `ask` |
| `ai_query` | `gash ai query` |  |
| `ai_sysinfo` | `gash ai sysinfo` | `sysinfo_ai` |
| `all_colors` | `gash shell colors` |  |
| `archive_extract` | `gash files extract` | `axe` |
| `chrome_headless_update` | `gash runtime upgrade chrome` | `chri` |
| `dirs_find_large` | `gash files big-dirs` | `dfl` |
| `dirs_largest` | `gash files largest-dirs` | `dld` |
| `dirs_list_empty` | `gash files empty-dirs` | `dle` |
| `disk_usage` | `gash system disks` | `du2` |
| `docker_compose_check` | `gash docker compose check` | `dcc` |
| `docker_compose_scan` | `gash docker compose scan` | `dcscan` |
| `docker_compose_upgrade` | `gash docker compose upgrade` | `dcup2` |
| `docker_prune_all` | `gash docker prune-all` | `dpa` |
| `docker_start_all` | `gash docker start-all` | `daa` |
| `docker_stop_all` | `gash docker stop-all` | `dsa` |
| `file_backup` | `gash files backup` | `fbk` |
| `files_largest` | `gash files largest` | `flf` |
| `gash_ai_list` | `gash ai providers` |  |
| `gash_db_list` | `gash db list` |  |
| `gash_db_test` | `gash db test` |  |
| `gash_doctor` | `gash doctor` | `gdoctor` |
| `gash_env_init` | `gash config init` |  |
| `gash_help` | `gash help` |  |
| `gash_inspiring_quote` | `gash shell quote` |  |
| `gash_ssh_auto_unlock` | `gash remote unlock-keys` |  |
| `gash_uninstall` | `gash uninstall` |  |
| `gash_unload` | `gash unload` |  |
| `gash_upgrade` | `gash update` |  |
| `gash_username` | `gash shell username` |  |
| `git_add_tag` | `gash git tag add` | `gat` |
| `git_apply_patch` | `gash git patch` | `gpatch` |
| `git_delete_tag` | `gash git tag delete` | `gdt` |
| `git_dump_revisions` | `gash git revisions` | `gdr` |
| `git_list_tags` | `gash git tag list` | `glt` |
| `git_repair` | `gash git repair` | `gheal` |
| `git_sync` | `gash git sync` | `gsync` |
| `hgrep` | `gash shell history-smart` |  |
| `history_grep` | `gash shell history` | `hg` |
| `ip_public` | `gash system ip` | `myip` |
| `mkdir_cd` | `gash shell mkcd` | `mkcd` |
| `node_detect_version` | `gash runtime detect node` |  |
| `node_upgrade` | `gash runtime upgrade node` | `nvmc` |
| `nvm_prune_versions` | `gash runtime prune node` | `nvmp` |
| `php_detect_version` | `gash runtime detect php` |  |
| `port_kill` | `gash system port-kill` | `ptk` |
| `process_find` | `gash system ps` | `pf` |
| `process_kill` | `gash system kill` | `pk` |
| `remote_exec` | `gash remote exec` | `rex` |
| `remote_hosts` | `gash remote hosts` | `rh` |
| `remote_pull` | `gash remote pull` | `rpull` |
| `remote_push` | `gash remote push` | `rpush` |
| `remote_sync_pull` | `gash remote sync pull` | `rspull` |
| `remote_sync_push` | `gash remote sync push` | `rspush` |
| `services_stop` | `gash system stop-services` | `svs` |
| `sudo_last` | `gash shell sudo-last` | `plz` |
| `sysinfo` | `gash system info` | `si` |
| `tree_stats` | `gash files stats` | `tls` |
| `wsl_compact` | `gash wsl compact` | `wslc` |
| `wsl_explorer` | `gash win explorer` | `wex` |
| `wsl_restart` | `gash wsl restart` | `wr` |
| `wsl_shutdown` | `gash wsl shutdown` | `wsd` |
| `wsl_taskmanager` | `gash win taskmgr` | `wtm` |
<!-- END generated: names -->

`gash_migrate`, the pointer to the Gash 1.x route, is `gash migrate`.
