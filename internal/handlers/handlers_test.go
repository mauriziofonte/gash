// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

func run(t *testing.T, name string, env map[string]string, tty bool, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, tty, false)}
	rc := Table().Run(c)
	return out.String(), errb.String(), rc
}

// Building the table checks every registration against the registry
// (Handle panics on an unknown or repeated name).
func TestTableBuilds(t *testing.T) {
	tbl := Table()
	require.Equal(t, []string{"ai_ask", "ai_query", "ai_sysinfo", "all_colors", "archive_extract", "chrome_headless_update", "dirs_find_large", "dirs_largest", "dirs_list_empty", "disk_usage", "docker_compose_check", "docker_compose_scan", "docker_compose_upgrade", "docker_prune_all", "docker_start_all", "docker_stop_all", "file_backup", "files_largest", "gash", "gash_ai_list", "gash_db_list", "gash_db_test", "gash_doctor", "gash_env_init", "gash_help", "gash_inspiring_quote", "gash_migrate", "gash_ssh_auto_unlock", "gash_uninstall", "gash_upgrade", "gash_username", "git_add_tag", "git_apply_patch", "git_delete_tag", "git_dump_revisions", "git_list_tags", "git_repair", "git_sync", "gl", "gla", "glf", "glg", "glo", "gls", "hgrep", "history_grep", "ip_public", "llm_config", "llm_db_explain", "llm_db_query", "llm_db_sample", "llm_db_schema", "llm_db_tables", "llm_deps", "llm_docker_check", "llm_env", "llm_exec", "llm_find", "llm_git_diff", "llm_git_log", "llm_git_status", "llm_grep", "llm_ports", "llm_procs", "llm_project", "llm_tree", "mkdir_cd", "node_detect_version", "node_upgrade", "nvm_prune_versions", "php_detect_version", "port_kill", "process_find", "process_kill", "remote_exec", "remote_hosts", "remote_pull", "remote_push", "remote_sync_pull", "remote_sync_push", "services_stop", "sudo_last", "sysinfo", "tree_stats", "wsl_compact", "wsl_explorer", "wsl_restart", "wsl_shutdown", "wsl_taskmanager"}, tbl.Names())
	require.Equal(t, []string{"env load", "help", "json arr", "json str", "output box", "output choose",
		"output download", "output table", "output wrap"}, tbl.CompatOps())
}

func TestAllColors(t *testing.T) {
	out, errOut, rc := run(t, "all_colors", nil, false)
	require.Equal(t, 0, rc)
	require.Empty(t, errOut)
	require.True(t, strings.HasPrefix(out, "colour0\ncolour1\n"))

	out, _, _ = run(t, "all_colors", nil, true)
	require.True(t, strings.HasPrefix(out, "\x1b[38;5;0mcolour0\x1b[0m\n"))

	// The environment gate: NO_COLOR on a terminal.
	out, _, _ = run(t, "all_colors", map[string]string{"NO_COLOR": "1"}, true)
	require.True(t, strings.HasPrefix(out, "colour0\n"))

	out, _, rc = run(t, "all_colors", map[string]string{"GASH__HELP": "0"}, false, "-h")
	require.Equal(t, 0, rc)
	require.Equal(t, "all_colors\nUsage: all_colors\nDisplays all available terminal colors (256 colors).\n", out)
}

func TestCompatHelp(t *testing.T) {
	tbl := Table()
	h, _, rest, ok := tbl.LookupCompat([]string{"help", "all_colors"})
	require.True(t, ok)
	var out bytes.Buffer
	e := invoke.EnvMap(nil)
	rc := h(&invoke.Call{Args: rest, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 0, rc)
	require.Contains(t, out.String(), "  all_colors  (module: utils)\n")

	out.Reset()
	rc = h(&invoke.Call{Args: []string{"mycommand"}, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 1, rc)
	require.Empty(t, out.String())
}

func TestCardUnknownTopic(t *testing.T) {
	_, errOut, rc := run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, false, "nope")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Unknown topic: nope. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	_, errOut, rc = run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, true, "--no-color", "x")
	require.Equal(t, 1, rc)
	require.Equal(t, "✗ Error: Unknown topic: x. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	_, errOut, rc = run(t, "gash", nil, true, "nope")
	require.Equal(t, 1, rc)
	require.Equal(t, "\x1b[38;5;196mx Error:\x1b[0m Unknown topic: nope. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	// Synonyms are topics.
	out, errOut, rc := run(t, "gash", nil, false, "navigation")
	require.Equal(t, 0, rc)
	require.Empty(t, errOut)
	require.Contains(t, out, "\n  NAV  Directory navigation and listings\n")

	out, _, rc = run(t, "gash", nil, false, "git", "-h")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "\n  gash  (module: gash)\n")

	_, errOut, rc = run(t, "gash", nil, false, "migrate", "--x")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error: gash_migrate: Gash 3 does not migrate a Gash 1.x configuration;")
}

func TestUsername(t *testing.T) {
	out, _, rc := run(t, "gash_username", map[string]string{"PATH": "/usr/bin:/bin"}, false)
	require.Equal(t, 0, rc)
	require.NotEmpty(t, strings.TrimSpace(out))
}
