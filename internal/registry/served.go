// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// served are the functions the per-shell server (gash __serve) runs:
// read-only, never asking a question, never reading the standard input
// except servedStdin.
var served = []string{
	"llm_tree", "llm_find", "llm_grep", "llm_project", "llm_config", "llm_deps",
	"llm_env", "llm_exec", "llm_procs", "llm_ports", "llm_git_status", "llm_git_log",
	"llm_git_diff", "llm_docker_check", "llm_db_tables", "llm_db_schema", "llm_db_sample",
	"llm_db_query", "llm_db_explain",
	"tree_stats", "files_largest", "dirs_largest", "dirs_find_large", "dirs_list_empty",
	"disk_usage", "process_find", "ip_public", "all_colors",
	"gash", "gash_help", "gash_username", "gash_inspiring_quote", "gash_ai_list", "gash_db_list",
	"node_detect_version", "php_detect_version",
}

// servedStdin are the served functions that may read the standard input.
var servedStdin = map[string]bool{"llm_exec": true}

// Served returns the names of the functions the per-shell server runs.
func Served() []string { return served }

// ServedReadsStdin reports a served function that may read its standard
// input (the server declines it when that is a file it cannot share).
func ServedReadsStdin(name string) bool { return servedStdin[name] }
