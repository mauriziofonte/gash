// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package contract holds the machine error contract of Gash: the error
// envelope, the table that binds every code to its action, and the JSON
// string encoders that reproduce what 2.x emitted.
package contract

import (
	"sort"
	"sync"
)

// Action tells an agent what to do with an error (AGENTS.md section 5).
type Action string

// The four actions. Any other value given to the 2.x emitter became STOP.
const (
	Retry    Action = "RETRY"
	Continue Action = "CONTINUE"
	Stop     Action = "STOP"
	Fatal    Action = "FATAL"
)

// NormalizeAction returns the action named by s, or STOP for anything that
// is not one of the four actions (the rule of 2.x __gash_json_error).
func NormalizeAction(s string) Action {
	switch Action(s) {
	case Retry, Continue, Stop, Fatal:
		return Action(s)
	}
	return Stop
}

// Recoverable is false exactly for FATAL.
func (a Action) Recoverable() bool { return a != Fatal }

// Origin says where a code comes from.
type Origin string

// Origins of the codes in the table.
const (
	Origin2x Origin = "2.x"
	Origin3x Origin = "3.x"
)

// Code is one row of the code table.
type Code struct {
	Name   string
	Action Action
	Area   string
	Origin Origin
}

// codeTable binds every error code to its fixed action. The 2.x rows are
// AGENTS.md section 5 plus every code the 2.x sources emit (__llm_error,
// __gash_json_error, __gash_fs_error and __llm_validate_uint, whose
// invalid_<name> codes are listed one by one). The 3.x rows are the new
// codes of go-impl/PORT/MATRIX.md ("New in 3.x") and design/hte.md 9.
var codeTable = []Code{
	// Arguments.
	{"unknown_option", Retry, "arguments", Origin2x},
	{"missing_value", Retry, "arguments", Origin2x},
	{"unexpected_argument", Retry, "arguments", Origin2x},
	{"invalid_limit", Retry, "arguments", Origin2x},
	{"invalid_depth", Retry, "arguments", Origin2x},
	{"invalid_size", Retry, "arguments", Origin2x},
	{"invalid_top", Retry, "arguments", Origin2x},
	// Since 2.1.1 (LLM-01).
	{"invalid_context", Retry, "arguments", Origin2x},
	{"invalid_rows", Retry, "arguments", Origin2x},
	{"invalid_port", Retry, "arguments", Origin2x},
	{"invalid_filter", Retry, "arguments", Origin2x},
	{"invalid_type", Retry, "arguments", Origin2x},
	{"invalid_database_name", Retry, "arguments", Origin2x},
	{"missing_pattern", Retry, "arguments", Origin2x},
	{"missing_query", Retry, "arguments", Origin2x},
	{"missing_table", Retry, "arguments", Origin2x},
	{"invalid_table_name", Retry, "arguments", Origin2x},
	{"missing_file", Retry, "arguments", Origin2x},
	{"empty_command", Retry, "arguments", Origin2x},
	// Paths.
	{"path_traversal_blocked", Fatal, "paths", Origin2x},
	{"forbidden_path", Fatal, "paths", Origin2x},
	{"root_scan_blocked", Fatal, "paths", Origin2x},
	{"secret_file_blocked", Fatal, "paths", Origin2x},
	{"path_not_found", Retry, "paths", Origin2x},
	{"not_a_directory", Retry, "paths", Origin2x},
	{"path_unreadable", Retry, "paths", Origin2x},
	{"file_not_found", Retry, "paths", Origin2x},
	{"file_too_large", Retry, "paths", Origin2x},
	// llm_exec.
	{"dangerous_command_blocked", Fatal, "llm_exec", Origin2x},
	{"option_not_allowed", Fatal, "llm_exec", Origin2x},
	{"command_not_allowed", Stop, "llm_exec", Origin2x},
	{"sandbox_unavailable", Stop, "llm_exec", Origin2x},
	{"construct_not_allowed", Retry, "llm_exec", Origin2x},
	{"unterminated_quote", Retry, "llm_exec", Origin2x},
	{"glob_matches_option", Retry, "llm_exec", Origin2x},
	// SQL guard.
	{"write_operation_blocked", Fatal, "sql guard", Origin2x},
	{"multiple_statements_blocked", Fatal, "sql guard", Origin2x},
	{"client_command_blocked", Fatal, "sql guard", Origin2x},
	{"dot_command_blocked", Fatal, "sql guard", Origin2x},
	{"executable_comment_blocked", Retry, "sql guard", Origin2x},
	{"backslash_in_literal", Retry, "sql guard", Origin2x},
	{"unterminated_literal", Retry, "sql guard", Origin2x},
	// Database.
	{"no_db_config", Stop, "database", Origin2x},
	{"no_database", Stop, "database", Origin2x},
	{"unknown_db_type", Stop, "database", Origin2x},
	{"sqlite_error", Retry, "database", Origin2x},
	{"mysql_error", Retry, "database", Origin2x},
	{"pgsql_error", Retry, "database", Origin2x},
	{"sqlite_file_not_found", Retry, "database", Origin2x},
	{"missing_sqlite_file", Retry, "database", Origin2x},
	{"sqlite_not_found", Fatal, "database", Origin2x},
	{"mysql_not_found", Fatal, "database", Origin2x},
	{"psql_not_found", Fatal, "database", Origin2x},
	{"invalid_sqlite_file", Fatal, "database", Origin2x},
	// Context.
	{"not_a_git_repo", Continue, "context", Origin2x},
	{"git_not_found", Continue, "context", Origin2x},
	{"no_package_file", Continue, "context", Origin2x},
	{"ports_tool_not_found", Continue, "context", Origin2x},
	{"jq_not_found", Fatal, "context", Origin2x},
	// llm_docker_check.
	{"no_services", Continue, "llm_docker_check", Origin2x},
	{"docker_not_found", Continue, "llm_docker_check", Origin2x},
	{"compose_not_installed", Continue, "llm_docker_check", Origin2x},
	{"compose_not_found", Retry, "llm_docker_check", Origin2x},
	// The type __gash_json_error used when it was given none.
	{"unknown_error", Stop, "fallback", Origin2x},

	// New in 3.x.
	{"binary_not_found", Stop, "compat", Origin3x},
	{"internal_error", Stop, "binary", Origin3x},
	{"invalid_regex", Retry, "arguments", Origin3x},
	{"invalid_manifest", Retry, "context", Origin3x},
	{"invalid_path", Retry, "paths", Origin3x},
	{"hte_not_installed", Stop, "hte", Origin3x},
	{"site_not_found", Retry, "hte", Origin3x},
	{"site_exists", Retry, "hte", Origin3x},
	{"invalid_domain", Retry, "hte", Origin3x},
	{"invalid_docroot", Retry, "hte", Origin3x},
	{"docroot_not_found", Retry, "hte", Origin3x},
	{"php_version_not_installed", Retry, "hte", Origin3x},
	{"invalid_profile", Retry, "hte", Origin3x},
	{"confirmation_required", Retry, "hte", Origin3x},
	{"lock_timeout", Retry, "hte", Origin3x},
	{"port_in_use", Stop, "hte", Origin3x},
	{"privileged_step_required", Stop, "hte", Origin3x},
	{"config_test_failed", Stop, "hte", Origin3x},
	{"reload_failed", Stop, "hte", Origin3x},
	{"apache_not_found", Fatal, "hte", Origin3x},
	{"platform_unsupported", Fatal, "hte", Origin3x},
	{"receipt_mismatch", Fatal, "hte", Origin3x},
	{"apache_module_missing", Stop, "hte", Origin3x},
	{"root_not_allowed", Fatal, "hte", Origin3x},
	{"conflicting_options", Retry, "hte", Origin3x},
	{"invalid_state", Stop, "hte", Origin3x},
	{"openssl_not_found", Fatal, "hte", Origin3x},
	{"php_not_enabled", Retry, "hte", Origin3x},
	{"invalid_ports", Retry, "hte", Origin3x},
	{"invalid_autostart", Retry, "hte", Origin3x},
	{"invalid_db_url", Stop, "database", Origin3x},
	{"invalid_service_name", Continue, "llm_docker_check", Origin3x},
	{"invalid_env_file", Retry, "llm_docker_check", Origin3x},
	// New in 3.4: log reclaim (gash logs). confirmation_required is shared
	// with hte (above).
	{"invalid_duration", Retry, "logs", Origin3x},
	// New in 3.4: Windows interop (gash win, gash wsl) — the winbridge
	// contract. interop_timeout is RETRY (the Windows side may be slow);
	// the rest are STOP (a human, an elevated terminal, or a policy change
	// is needed) except the argument guard, which is RETRY.
	{"interop_unavailable", Stop, "win", Origin3x},
	{"interop_timeout", Retry, "win", Origin3x},
	{"windows_access_denied", Stop, "win", Origin3x},
	{"elevation_required", Stop, "win", Origin3x},
	{"windows_not_found", Stop, "win", Origin3x},
	{"invalid_windows_argument", Retry, "win", Origin3x},
	// New in 3.4: agent toolkit, shared output layer (internal/llmfmt):
	// --format, the output budget, -j and --timeout.
	{"invalid_format", Retry, "llm", Origin3x},
	{"invalid_width", Retry, "llm", Origin3x},
	{"invalid_max_lines", Retry, "llm", Origin3x},
	{"invalid_head", Retry, "llm", Origin3x},
	{"invalid_tail", Retry, "llm", Origin3x},
	{"invalid_jobs", Retry, "llm", Origin3x},
	{"invalid_timeout", Retry, "llm", Origin3x},
	// New in 3.4: agent toolkit, edits (agent_replace). Nothing is
	// written on any of these.
	{"invalid_replacement", Retry, "agent", Origin3x},
	{"invalid_count", Retry, "agent", Origin3x},
	{"replace_count_mismatch", Retry, "agent", Origin3x},
	{"binary_file", Retry, "agent", Origin3x},
	{"file_changed", Retry, "agent", Origin3x},
	{"write_failed", Stop, "agent", Origin3x},
	// The write path policy of every agent_* target (the transfer DENY
	// and ASK tiers of internal/remote, applied locally too).
	{"write_path_denied", Fatal, "agent", Origin3x},
	{"sensitive_path", Stop, "agent", Origin3x},

	// New in 3.4: agent toolkit, files and search (llm_find, llm_grep,
	// llm_tree extensions).

	// New in 3.4: agent toolkit, text reads (llm_stat, llm_count,
	// llm_assert, llm_read, llm_log, llm_hash, llm_textstats,
	// llm_textcheck).

	// New in 3.4: agent toolkit, data (llm_db_* formats, llm_json,
	// llm_csv, llm_config formats, llm_dotenv, llm_date).

	// New in 3.4: agent toolkit, git (llm_git_* extensions,
	// llm_git_overview, llm_git_show, agent_git_commit).

	// New in 3.4: agent toolkit, network (llm_http, llm_dns, llm_tls,
	// llm_tcp, llm_ports --port).

	// New in 3.4: agent toolkit, processes, waiting and services (llm_wait,
	// llm_procs, llm_service, llm_docker_*, llm_sysinfo, agent_kill,
	// agent_docker_exec).
	{"invalid_pid", Retry, "llm", Origin3x},
	{"ps_not_found", Continue, "llm", Origin3x},
	{"missing_condition", Retry, "llm", Origin3x},
	{"invalid_interval", Retry, "llm", Origin3x},
	{"invalid_code", Retry, "llm", Origin3x},
	{"invalid_url", Retry, "llm", Origin3x},
	{"invalid_json_path", Retry, "llm", Origin3x},
	{"missing_unit", Retry, "llm", Origin3x},
	{"systemd_unavailable", Continue, "llm", Origin3x},
	{"missing_container", Retry, "llm", Origin3x},
	{"missing_command", Retry, "llm", Origin3x},
	{"invalid_user", Retry, "llm", Origin3x},
	{"invalid_since", Retry, "llm", Origin3x},
	{"container_not_found", Retry, "llm", Origin3x},
	{"container_not_running", Continue, "llm", Origin3x},
	{"docker_unavailable", Continue, "llm", Origin3x},
	{"docker_error", Retry, "llm", Origin3x},
	{"invalid_log", Retry, "llm", Origin3x},
	{"missing_target", Retry, "agent", Origin3x},
	{"invalid_signal", Retry, "agent", Origin3x},
	{"pattern_too_broad", Retry, "agent", Origin3x},
	{"kill_refused", Fatal, "agent", Origin3x},
	{"kill_failed", Stop, "agent", Origin3x},

	// New in 3.4: agent toolkit, runtimes, lint and tests (gash runtime,
	// gash run, llm_tools, llm_runtime, llm_lint, llm_test).

	// New in 3.4: agent toolkit, remote and edits (--host, llm_remote_run,
	// llm_same, agent_replace, agent_put).
}

// codeIndex is built on first use, so the binary does no work at start.
var codeIndex = sync.OnceValue(func() map[string]Action {
	m := make(map[string]Action, len(codeTable))
	for _, c := range codeTable {
		m[c.Name] = c.Action
	}
	return m
})

// ActionOf returns the fixed action of code.
func ActionOf(code string) (Action, bool) {
	a, ok := codeIndex()[code]
	return a, ok
}

// Codes returns the code table sorted by name.
func Codes() []Code {
	out := append([]Code(nil), codeTable...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
