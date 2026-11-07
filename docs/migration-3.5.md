# Moving to the command names of 3.5

Gash 3.5 gives every command one name, `gash NAMESPACE COMMAND`
(`gash files largest`, `gash git tag add`, `gash system info`), and keeps
the short aliases as shortcuts that expand to it. Nothing you use stops
working in 3.5: this page lists what changed, what keeps working, and what
4.0 will change.

## What changed

- **One name per command.** `gash` alone shows the namespaces and their
  commands, `gash files` shows one namespace, `gash help files largest`
  the full page of a command. Gash's own commands sit at the top level:
  `gash doctor`, `gash update`, `gash help`, `gash uninstall`,
  `gash unload`, `gash version`.
- **Shortcuts.** In an interactive shell, `flf` expands to
  `gash files largest`, `gsync` to `gash git sync`. `gash help shortcuts`
  lists them all. `GASH_SHORTCUTS` in `~/.gash_env` picks the packs you
  want: `commands`, `git`, `docker`, `navigation`, `php`, `all` (the
  default) or `none` ([configuration](configuration.md)).
- **Tab completion.** The binary answers it: Tab completes each word of a
  command, the options of every command with their descriptions, and the
  values they take (the hosts and databases of `~/.gash_env`, git tags and
  branches, containers, services, processes, listening ports). It never
  shows a password.
- **Help.** `gash help NAME` takes a command, a shortcut or a former name;
  `gash help --search WORD` searches every page; a mistyped name gets a
  suggestion (`Did you mean: gash git sync?`).

## What keeps working

- **The flat names of 3.4.** `files_largest`, `git_sync` and the others
  run as before, with the same output and exit status. Typed at the
  prompt, each prints one line on stderr, once per session:
  `tip: files_largest is now gash files largest (shortcut flf)`. A former
  name called from a script, from a function of yours or in headless mode
  prints nothing. `GASH_LEGACY_HINTS=0` turns the tip off.
- **The agent functions.** `llm_*` and `agent_*` are permanent names, also
  reachable as `gash llm NAME` and `gash agent NAME` (`llm_db_query` is
  `gash llm db-query`). They print no tip and will not change in 4.0
  ([AGENTS.md](../AGENTS.md)).
- **The script helpers.** `needs_help`, `needs_confirm_prompt` and
  `path_add` stay functions for your scripts and `~/.bash_local`.

## Removed in 3.5

These aliases duplicated another name and are gone:

| Alias | Use instead |
|---|---|
| `dstopall` | `dsa` (`gash docker stop-all`) |
| `dstartall` | `daa` (`gash docker start-all`) |
| `dpruneall` | `dpa` (`gash docker prune-all`) |
| `glog` | `gl` (`gash git log`) |
| `quit` | `svs` (`gash system stop-services`) |

`dcc`, `dcup2` and `dcscan` were defined twice, by the docker pack and by
the module; they stay once, as the shortcuts of `gash docker compose
check`, `gash docker compose upgrade` and `gash docker compose scan`.

## In 4.0

The former flat names leave the default: a fresh 4.0 shell defines the
commands, the shortcuts, the agent functions and the script helpers only.
A setting will bring the flat names back for those who need them. Until
then, a script that calls a former name can move to the command at its
own pace: the table below has every one.

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
