// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// namespaces are the first words of the canonical commands, in overview
// order.
var namespaces = []Namespace{
	{Name: "files", Group: GroupEveryday, Area: "files", Summary: "Largest files and folders, tree stats, archives, backups"},
	{Name: "system", Group: GroupEveryday, Area: "system", Summary: "Processes, ports, services, disks, public IP"},
	{Name: "shell", Group: GroupEveryday, Area: "shell", Summary: "History search, mkcd, sudo-last, colors, navigation",
		Packs: []string{"navigation", "safety", "colors", "general"}, Topics: []string{"nav", "navigation"}},
	{Name: "git", Group: GroupEveryday, Area: "git", Summary: "Sync, tags, patches, repair, log views", Packs: []string{"git"}},
	{Name: "docker", Group: GroupEveryday, Area: "docker", Summary: "Containers and Compose stacks", Packs: []string{"docker"}},
	{Name: "remote", Group: GroupEveryday, Area: "remote", Summary: "Hosts of ~/.gash_env: exec, copy, sync, SSH keys"},
	{Name: "db", Group: GroupEveryday, Area: "databases", Summary: "Database connections of ~/.gash_env: list and test"},
	{Name: "runtime", Group: GroupEveryday, Area: "runtimes", Summary: "PHP and Node.js versions, headless Chrome",
		Packs: []string{"php"}, Delegate: "runtime", Topics: []string{"dev", "php", "node"}},
	{Name: "ai", Group: GroupEveryday, Area: "ai", Summary: "Ask Claude or Gemini from the terminal"},
	{Name: "hte", Group: GroupPlatform, Area: "hte", Summary: "Local Apache and PHP-FPM sites with HTTPS", Delegate: "hte"},
	{Name: "wsl", Group: GroupPlatform, Area: "windows", Summary: "The WSL distribution: info, doctor, paths, restart, compact", Delegate: "wsl"},
	{Name: "win", Group: GroupPlatform, Area: "windows", Summary: "The Windows host, read from WSL; Explorer, Task Manager", Delegate: "win"},
	{Name: "logs", Group: GroupPlatform, Area: "logs", Summary: "Find and reclaim disk space from log files", Delegate: "logs"},
	{Name: "llm", Group: GroupAgents, Area: "agents", Summary: "Read-only JSON tools for AI agents (also llm_*)"},
	{Name: "agent", Group: GroupAgents, Area: "agents", Summary: "Write tools for AI agents, --dry-run first (also agent_*)"},
}

// vDB are the value kinds of the database options.
var vDB = map[string]ArgKind{"-c": ArgDB, "-f": ArgFile, "--format": ArgFormat}

// tree is every canonical command, in page order within each namespace.
var tree = []Command{
	// files
	{Path: []string{"files", "largest"}, Handler: "files_largest", Shortcut: "flf", Args: []ArgKind{ArgDir}},
	{Path: []string{"files", "largest-dirs"}, Handler: "dirs_largest", Shortcut: "dld", Args: []ArgKind{ArgDir}},
	{Path: []string{"files", "big-dirs"}, Handler: "dirs_find_large", Shortcut: "dfl", Args: []ArgKind{ArgDir}},
	{Path: []string{"files", "empty-dirs"}, Handler: "dirs_list_empty", Shortcut: "dle", Args: []ArgKind{ArgDir}},
	{Path: []string{"files", "stats"}, Handler: "tree_stats", Shortcut: "tls", Args: []ArgKind{ArgDir}},
	{Path: []string{"files", "extract"}, Handler: "archive_extract", Shortcut: "axe", Args: []ArgKind{ArgFile, ArgDir}},
	{Path: []string{"files", "backup"}, Handler: "file_backup", Shortcut: "fbk", Args: []ArgKind{ArgFile}},

	// system
	{Path: []string{"system", "info"}, Handler: "sysinfo", Shortcut: "si"},
	{Path: []string{"system", "disks"}, Handler: "disk_usage", Shortcut: "du2"},
	{Path: []string{"system", "ip"}, Handler: "ip_public", Shortcut: "myip"},
	{Path: []string{"system", "ps"}, Handler: "process_find", Shortcut: "pf", Args: []ArgKind{ArgProc}},
	{Path: []string{"system", "kill"}, Handler: "process_kill", Shortcut: "pk", Args: []ArgKind{ArgProc}},
	{Path: []string{"system", "port-kill"}, Handler: "port_kill", Shortcut: "ptk", Args: []ArgKind{ArgPort}},
	{Path: []string{"system", "stop-services"}, Handler: "services_stop", Shortcut: "svs"},

	// shell
	{Path: []string{"shell", "history"}, Handler: "history_grep", Shortcut: "hg", Args: []ArgKind{ArgAny}, Shell: true},
	{Path: []string{"shell", "history-smart"}, Handler: "hgrep", Args: []ArgKind{ArgAny}, Shell: true},
	{Path: []string{"shell", "mkcd"}, Handler: "mkdir_cd", Shortcut: "mkcd", Args: []ArgKind{ArgDir}, Shell: true},
	{Path: []string{"shell", "sudo-last"}, Handler: "sudo_last", Shortcut: "plz", Shell: true},
	{Path: []string{"shell", "colors"}, Handler: "all_colors"},
	{Path: []string{"shell", "quote"}, Handler: "gash_inspiring_quote"},
	{Path: []string{"shell", "username"}, Handler: "gash_username"},

	// git
	{Path: []string{"git", "sync"}, Handler: "git_sync", Shortcut: "gsync"},
	{Path: []string{"git", "tag", "add"}, Handler: "git_add_tag", Shortcut: "gat", Args: []ArgKind{ArgAny}},
	{Path: []string{"git", "tag", "delete"}, Handler: "git_delete_tag", Shortcut: "gdt", Args: []ArgKind{ArgGitTag}},
	{Path: []string{"git", "tag", "list"}, Handler: "git_list_tags", Shortcut: "glt"},
	{Path: []string{"git", "patch"}, Handler: "git_apply_patch", Shortcut: "gpatch", Args: []ArgKind{ArgGitRef, ArgGitRef, ArgAny}},
	{Path: []string{"git", "repair"}, Handler: "git_repair", Shortcut: "gheal"},
	{Path: []string{"git", "revisions"}, Handler: "git_dump_revisions", Shortcut: "gdr", Args: []ArgKind{ArgFile}},
	{Path: []string{"git", "log"}, Handler: "gl", Shortcut: "gl", Summary: "Compact log with graph of the current branch (git log options pass through)"},
	{Path: []string{"git", "log", "all"}, Handler: "gla", Shortcut: "gla", Summary: "Log of all branches with graph"},
	{Path: []string{"git", "log", "oneline"}, Handler: "glo", Shortcut: "glo", Summary: "Ultra-compact one-line log"},
	{Path: []string{"git", "log", "graph"}, Handler: "glg", Shortcut: "glg", Summary: "Graph of the first-parent history (cleaner merges)"},
	{Path: []string{"git", "log", "stat"}, Handler: "gls", Shortcut: "gls", Summary: "Log with the files each commit changed"},
	{Path: []string{"git", "log", "file"}, Handler: "glf", Shortcut: "glf", Summary: "History of one file, with patches", Args: []ArgKind{ArgFile}},

	// docker
	{Path: []string{"docker", "start-all"}, Handler: "docker_start_all", Shortcut: "daa"},
	{Path: []string{"docker", "stop-all"}, Handler: "docker_stop_all", Shortcut: "dsa"},
	{Path: []string{"docker", "prune-all"}, Handler: "docker_prune_all", Shortcut: "dpa"},
	{Path: []string{"docker", "compose", "scan"}, Handler: "docker_compose_scan", Shortcut: "dcscan", Args: []ArgKind{ArgDir}},
	{Path: []string{"docker", "compose", "check"}, Handler: "docker_compose_check", Shortcut: "dcc", Args: []ArgKind{ArgDir}},
	{Path: []string{"docker", "compose", "upgrade"}, Handler: "docker_compose_upgrade", Shortcut: "dcup2", Args: []ArgKind{ArgDir}},

	// remote
	{Path: []string{"remote", "hosts"}, Handler: "remote_hosts", Shortcut: "rh"},
	{Path: []string{"remote", "exec"}, Handler: "remote_exec", Shortcut: "rex", Args: []ArgKind{ArgHost, ArgAny}, Values: map[string]ArgKind{"--script": ArgFile}},
	{Path: []string{"remote", "push"}, Handler: "remote_push", Shortcut: "rpush", Args: []ArgKind{ArgHost, ArgFile, ArgAny}},
	{Path: []string{"remote", "pull"}, Handler: "remote_pull", Shortcut: "rpull", Args: []ArgKind{ArgHost, ArgAny, ArgFile}},
	{Path: []string{"remote", "sync", "push"}, Handler: "remote_sync_push", Shortcut: "rspush", Args: []ArgKind{ArgHost, ArgDir, ArgAny}},
	{Path: []string{"remote", "sync", "pull"}, Handler: "remote_sync_pull", Shortcut: "rspull", Args: []ArgKind{ArgHost, ArgAny, ArgDir}},
	{Path: []string{"remote", "unlock-keys"}, Handler: "gash_ssh_auto_unlock", Shell: true},

	// db
	{Path: []string{"db", "list"}, Handler: "gash_db_list"},
	{Path: []string{"db", "test"}, Handler: "gash_db_test", Args: []ArgKind{ArgDB}},

	// ai
	{Path: []string{"ai", "ask"}, Handler: "ai_ask", Shortcut: "ask", Args: []ArgKind{ArgAI, ArgAny}},
	{Path: []string{"ai", "query"}, Handler: "ai_query", Args: []ArgKind{ArgAI, ArgAny}},
	{Path: []string{"ai", "sysinfo"}, Handler: "ai_sysinfo", Shortcut: "sysinfo_ai", Args: []ArgKind{ArgAI}},
	{Path: []string{"ai", "providers"}, Handler: "gash_ai_list"},

	// runtime (the rest of gash runtime is its 3.x resolver)
	{Path: []string{"runtime", "which"}, Command3x: "runtime", Usage: "php|node|composer|npm|npx VERSION [--path]", Summary: "Resolve a php or node version to its binary, without nvm.sh",
		Args: []ArgKind{ArgRuntime, ArgAny}},
	{Path: []string{"runtime", "detect", "php"}, Handler: "php_detect_version", Args: []ArgKind{ArgDir}},
	{Path: []string{"runtime", "detect", "node"}, Handler: "node_detect_version", Args: []ArgKind{ArgDir}},
	{Path: []string{"runtime", "upgrade", "node"}, Handler: "node_upgrade", Shortcut: "nvmc", Shell: true},
	{Path: []string{"runtime", "upgrade", "chrome"}, Handler: "chrome_headless_update", Shortcut: "chri"},
	{Path: []string{"runtime", "prune", "node"}, Handler: "nvm_prune_versions", Shortcut: "nvmp", Shell: true},

	// wsl and win (the rest of each is its 3.x tree)
	{Path: []string{"wsl", "restart"}, Handler: "wsl_restart", Shortcut: "wr", Shell: true},
	{Path: []string{"wsl", "shutdown"}, Handler: "wsl_shutdown", Shortcut: "wsd", Shell: true},
	{Path: []string{"wsl", "compact"}, Handler: "wsl_compact", Shortcut: "wslc"},
	{Path: []string{"win", "explorer"}, Handler: "wsl_explorer", Shortcut: "wex", Args: []ArgKind{ArgDir}},
	{Path: []string{"win", "taskmgr"}, Handler: "wsl_taskmanager", Shortcut: "wtm"},

	// llm and agent: filled from the catalog by llmCommands (one per
	// llm_*/agent_* function), then the lists.
	{Path: []string{"llm", "list"}, Command3x: "agent-functions", Summary: "List the llm_* and agent_* tools (--json for a wrapper)"},
	{Path: []string{"llm", "hint"}, Command3x: "agents-hint", Summary: "The routing table for an agent's instructions (Markdown)"},
	{Path: []string{"agent", "list"}, Command3x: "agent-functions", Summary: "List the llm_* and agent_* tools (--json for a wrapper)"},

	// Gash itself
	{Path: []string{"help"}, Command3x: "help", Summary: "Help for any command, shortcut or former name; --search WORD", Args: []ArgKind{ArgAny}},
	{Path: []string{"doctor"}, Handler: "gash_doctor", Shortcut: "gdoctor", Shell: true},
	{Path: []string{"update"}, Command3x: "update", Summary: "Update Gash to the latest verified release (--rollback)"},
	{Path: []string{"version"}, Command3x: "version", Summary: "Print the version (--json)"},
	{Path: []string{"config", "init"}, Handler: "gash_env_init"},
	{Path: []string{"uninstall"}, Handler: "gash_uninstall", Shell: true},
	{Path: []string{"unload"}, Handler: "gash_unload", Shell: true},
	{Path: []string{"init"}, Command3x: "init", Summary: "Print the interactive Bash layer for gash.sh (gash init bash)"},
	{Path: []string{"run"}, Command3x: "run", Summary: "Run a command with its output shaped and its exit status kept",
		Args: []ArgKind{ArgCommand}, Values: map[string]ArgKind{"--runtime": ArgRuntime}},
	{Path: []string{"all"}, Command3x: "all", Summary: "Every namespace page, one after the other"},
}

func init() {
	tree = withAgentCommands(tree)
}

// agentValues are the value kinds of the options of the llm_* and agent_*
// functions that are not --host or --format.
var agentValues = map[string]map[string]ArgKind{
	"llm_db_query": vDB, "llm_db_tables": vDB, "llm_db_schema": vDB, "llm_db_sample": vDB,
	"llm_db_explain": vDB, "llm_db_databases": vDB,
	"llm_remote_run": {"--host": ArgHost, "--format": ArgFormat},
	"llm_same":       {"--host": ArgHost},
	"agent_put":      {"--host": ArgHost},
	"agent_test":     {"--runtime": ArgRuntime},
	"llm_exec":       {},
}

// agentArgs are the positional kinds of the llm_* and agent_* functions.
var agentArgs = map[string][]ArgKind{
	"llm_read": {ArgFile}, "llm_tree": {ArgDir}, "llm_find": {ArgDir}, "llm_stat": {ArgFile},
	"llm_hash": {ArgFile}, "llm_textstats": {ArgFile}, "llm_textcheck": {ArgFile}, "llm_json": {ArgFile},
	"llm_csv": {ArgFile}, "llm_dotenv": {ArgFile}, "llm_config": {ArgFile}, "llm_lint": {ArgFile},
	"llm_log": {ArgFile}, "llm_count": {ArgAny, ArgFile}, "llm_grep": {ArgAny, ArgDir},
	"llm_project": {ArgDir}, "llm_deps": {ArgDir}, "llm_docker_logs": {ArgContainer},
	"llm_docker_exec": {ArgContainer, ArgCommand}, "agent_docker_exec": {ArgContainer, ArgCommand},
	"llm_service": {ArgUnit}, "llm_git_show": {ArgGitRef}, "llm_runtime": {ArgRuntime, ArgAny},
	"llm_exec": {ArgCommand}, "agent_replace": {ArgAny, ArgAny, ArgFile}, "agent_git_commit": {ArgFile},
	"llm_docker_check": {ArgDir}, "llm_remote_run": {ArgAny}, "agent_test": {ArgCommand},
}

// withAgentCommands inserts gash llm NAME and gash agent NAME for every
// llm_* and agent_* function of the catalog before the lists: the name is
// the function's without the prefix, "_" spelled "-" (llm_db_query ->
// gash llm db-query).
func withAgentCommands(t []Command) []Command {
	var llm, agent []Command
	for _, f := range functions {
		ns, rest := "", ""
		switch {
		case len(f.Name) > 4 && f.Name[:4] == "llm_":
			ns, rest = "llm", f.Name[4:]
		case len(f.Name) > 6 && f.Name[:6] == "agent_":
			ns, rest = "agent", f.Name[6:]
		default:
			continue
		}
		vals := map[string]ArgKind{"--host": ArgHost, "--format": ArgFormat}
		for k, v := range agentValues[f.Name] {
			vals[k] = v
		}
		c := Command{Path: []string{ns, dashed(rest)}, Handler: f.Name, Args: agentArgs[f.Name], Values: vals}
		if ns == "llm" {
			llm = append(llm, c)
		} else {
			agent = append(agent, c)
		}
	}
	var out []Command
	for _, c := range t {
		if len(c.Path) == 2 && c.Path[1] == "list" && c.Path[0] == "llm" {
			out = append(out, llm...)
		}
		if len(c.Path) == 2 && c.Path[1] == "list" && c.Path[0] == "agent" {
			out = append(out, agent...)
		}
		out = append(out, c)
	}
	return out
}

func dashed(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == '_' {
			b[i] = '-'
		}
	}
	return string(b)
}
