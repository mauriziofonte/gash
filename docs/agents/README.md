# Tools for AI agents

The read-only llm_* functions and the agent_* write tools that AI coding agents call: one JSON answer per call, with the safety rails of AGENTS.md.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ssh](../requirements.md#tool-ssh) (feature), for `agent_put`, `agent_replace` (--host), `llm_assert` (--host), `llm_config` (--host), `llm_count` (--host), `llm_csv` (--host), `llm_db_databases` (a +ssh URL), `llm_db_explain` (a +ssh URL), `llm_db_query` (a +ssh URL), `llm_db_sample` (a +ssh URL), `llm_db_schema` (a +ssh URL), `llm_db_tables` (a +ssh URL), `llm_deps` (--host), `llm_docker_exec` (--host), `llm_docker_logs` (--host), `llm_docker_ps` (--host), `llm_dotenv` (--host), `llm_env` (--host), `llm_find` (--host), `llm_git_log` (--host), `llm_git_overview` (--host), `llm_git_status` (--host), `llm_grep` (--host), `llm_hash` (--host), `llm_json` (--host), `llm_lint` (--host), `llm_log` (--host), `llm_ports` (--host), `llm_procs` (--host), `llm_project` (--host), `llm_read` (--host), `llm_remote_run`, `llm_same` (a host side), `llm_service` (--host), `llm_stat` (--host), `llm_sysinfo` (--host), `llm_textcheck` (--host), `llm_textstats` (--host), `llm_tools` (--host), `llm_tree` (--host) and `llm_wait` (--host)
- [ps](../requirements.md#tool-ps) (feature), for `agent_kill` (on macOS, or without /proc), `llm_procs` (on macOS, or without /proc) and `llm_wait` (--gone, on macOS)
- [pgrep](../requirements.md#tool-pgrep) (feature), for `llm_procs` (--name)
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

Every function, alias and command of this area, with all its options: [reference.md](reference.md).
