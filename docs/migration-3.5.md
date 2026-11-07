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

Gash 4.0 takes the former flat names out of the default:
[Moving to Gash 4.0](migration-4.0.md) says what changed and has the
table of every former name.
