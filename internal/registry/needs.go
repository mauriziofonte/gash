// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"sort"
	"sync"
)

// Need is an external tool a function runs: a name of the prerequisite
// catalog (internal/prereq), and the flag or case that needs it.
type Need struct {
	Tool string
	// When is the flag, file type or case that needs the tool ("--port",
	// ".zip", "on macOS"); empty when every call does.
	When string
}

// hostFunctions are the functions that take --host: each one needs ssh
// for it.
var hostFunctions = []string{
	"llm_tree", "llm_find", "llm_grep", "llm_config", "llm_stat", "llm_read", "llm_log", "llm_hash", "llm_count",
	"llm_assert", "llm_textstats", "llm_textcheck", "llm_json", "llm_csv", "llm_dotenv", "llm_git_status",
	"llm_git_log", "llm_git_overview", "llm_procs", "llm_ports", "llm_service", "llm_docker_ps", "llm_docker_logs",
	"llm_docker_exec", "llm_wait", "llm_sysinfo", "llm_lint", "llm_tools", "llm_project", "llm_deps", "llm_env",
	"agent_replace",
}

// needs is the census of 2026-10-26: every function and 3.x command that
// runs an external program a system may lack (the programs every POSIX
// system has are left out). Keys are registry function names or "gash
// WORD" commands.
var needs = map[string][]Need{
	// Git.
	"git_list_tags":      {{"git", ""}},
	"git_add_tag":        {{"git", ""}},
	"git_delete_tag":     {{"git", ""}},
	"git_dump_revisions": {{"git", ""}},
	"git_apply_patch":    {{"git", ""}},
	"git_repair":         {{"git", ""}},
	"git_sync":           {{"git", ""}},
	"gl":                 {{"git", ""}},
	"gla":                {{"git", ""}},
	"glf":                {{"git", ""}},
	"glg":                {{"git", ""}},
	"glo":                {{"git", ""}},
	"gls":                {{"git", ""}},
	"llm_git_status":     {{"git", ""}},
	"llm_git_diff":       {{"git", ""}},
	"llm_git_log":        {{"git", ""}},
	"llm_git_overview":   {{"git", ""}},
	"llm_git_show":       {{"git", ""}},
	"agent_git_commit":   {{"git", ""}},
	"gash_upgrade":       {{"git", ""}, {"cosign", "GASH_REQUIRE_SIGNED=1"}, {"gpg", "GASH_REQUIRE_SIGNED=1"}},
	"gash update":        {{"git", ""}, {"bash", ""}, {"cosign", "GASH_REQUIRE_SIGNED=1"}},

	// Remote hosts and SSH.
	"remote_exec":          {{"ssh", ""}},
	"remote_push":          {{"scp", ""}},
	"remote_pull":          {{"scp", ""}},
	"remote_sync_push":     {{"rsync", ""}, {"ssh", ""}},
	"remote_sync_pull":     {{"rsync", ""}, {"ssh", ""}},
	"llm_remote_run":       {{"ssh", ""}},
	"llm_same":             {{"ssh", "a host side"}},
	"agent_put":            {{"ssh", ""}},
	"llm_db_query":         {{"ssh", "a +ssh URL"}},
	"llm_db_tables":        {{"ssh", "a +ssh URL"}},
	"llm_db_schema":        {{"ssh", "a +ssh URL"}},
	"llm_db_sample":        {{"ssh", "a +ssh URL"}},
	"llm_db_explain":       {{"ssh", "a +ssh URL"}},
	"llm_db_databases":     {{"ssh", "a +ssh URL"}},
	"gash_db_test":         {{"ssh", "a +ssh URL"}},
	"gash_ssh_auto_unlock": {{"ssh-add", ""}, {"ssh-agent", ""}},

	// Processes and ports.
	"llm_procs":    {{"ps", "on macOS, or without /proc"}, {"lsof", "--port"}, {"pgrep", "--name"}},
	"agent_kill":   {{"ps", "on macOS, or without /proc"}},
	"process_find": {{"ps", ""}},
	"process_kill": {{"pgrep", ""}},
	"port_kill":    {{"lsof", ""}},
	"llm_ports":    {{"ss", ""}},

	// Docker.
	"docker_stop_all":        {{"docker", ""}},
	"docker_start_all":       {{"docker", ""}},
	"docker_prune_all":       {{"docker", ""}},
	"docker_compose_check":   {{"docker", ""}, {"docker-compose", ""}},
	"docker_compose_upgrade": {{"docker", ""}, {"docker-compose", ""}},
	"llm_docker_ps":          {{"docker", ""}},
	"llm_docker_logs":        {{"docker", ""}},
	"llm_docker_exec":        {{"docker", ""}},
	"llm_docker_check":       {{"docker", ""}, {"docker-compose", ""}},
	"agent_docker_exec":      {{"docker", ""}},

	// Archives.
	"archive_extract": {{"tar", "tar archives"}, {"gzip", ".gz"}, {"bzip2", ".bz2"}, {"xz", ".xz"}, {"zstd", ".zst"},
		{"unzip", ".zip"}, {"7z", ".7z"}, {"unrar", ".rar"}, {"uncompress", ".Z"}},
	"chrome_headless_update": {{"unzip", ""}},

	// Runtimes and linters.
	"llm_lint":           {{"php", ".php files"}, {"node", ".js files"}, {"python3", ".py files"}, {"bash", ".sh files"}},
	"llm_runtime":        {{"php", "php"}, {"node", "node"}, {"composer", "composer"}},
	"gash runtime":       {{"php", "php"}, {"node", "node"}, {"composer", "composer"}},
	"gash run":           {{"php", "--runtime php"}, {"node", "--runtime node"}, {"composer", "--runtime composer"}, {"bash", "--safe"}},
	"php_detect_version": {{"php", ""}},
	"node_upgrade":       {{"node", ""}},
	"nvm_prune_versions": {{"node", ""}},

	// The agent command runner.
	"llm_exec": {{"bash", ""}, {"gawk", "awk"}, {"gnu-sed", "sed"}, {"diffutils", "diff -r"}, {"bwrap", "GASH_LLM_SANDBOX=1"}},
	"llm_wait": {{"bash", "--cmd-ok"}, {"ps", "--gone, on macOS"}},

	// System.
	"llm_service":   {{"systemctl", ""}, {"journalctl", "--log"}},
	"services_stop": {{"systemctl", ""}, {"sudo", ""}},
	"sudo_last":     {{"sudo", ""}},
	"sysinfo":       {{"sudo", "the privileged sections"}},
	"ai_sysinfo":    {{"sudo", "the privileged sections"}},
	"wsl_compact":   {{"sudo", ""}},
	"hgrep":         {{"fzf", "--pick"}},
	"history_grep":  {{"fzf", "--pick"}},
	"gash logs":     {{"sudo", "clean --sudo"}},
	"gash init":     {{"bc", "the calc alias"}},

	// Local web stack.
	"gash hte": {{"apache", ""}, {"php-fpm", "PHP sites"}, {"openssl", ""}, {"sudo", ""}},

	// Windows, under WSL.
	"gash win":        {{"powershell.exe", ""}},
	"wsl_restart":     {{"cmd.exe", ""}},
	"wsl_shutdown":    {{"cmd.exe", ""}},
	"wsl_taskmanager": {{"cmd.exe", ""}},
}

// needsIndex is needs plus ssh for every --host function, built on first
// use so the binary does no work at start.
var needsIndex = sync.OnceValue(func() map[string][]Need {
	m := make(map[string][]Need, len(needs)+len(hostFunctions))
	for k, v := range needs {
		m[k] = v
	}
	for _, fn := range hostFunctions {
		m[fn] = append(append([]Need(nil), m[fn]...), Need{"ssh", "--host"})
	}
	return m
})

// NeedsOf returns the tools a function or "gash WORD" command runs.
func NeedsOf(name string) []Need { return needsIndex()[name] }

// NeedNames returns every function and command that has needs, sorted.
func NeedNames() []string {
	m := needsIndex()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
