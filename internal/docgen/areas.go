// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package docgen renders the generated parts of the user documentation:
// docs/<area>/reference.md for each area (every function from the help
// registry, every gash command from its own --help) and the generated
// blocks of the hand-written pages (the prerequisites of an area, the
// area index). go test ./internal/docgen -update rewrites them; the same
// tests fail when a page is stale or a link between pages is broken.
package docgen

import (
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
)

// Area is one folder of docs/: a guide (README.md, written by hand) and
// its reference (reference.md, generated).
type Area struct {
	// Key is the folder name under docs/.
	Key string
	// Title is the page title.
	Title string
	// Summary is one sentence: what the area covers.
	Summary string
	// Commands are the gash commands documented with the area ("hte",
	// "win"): each with every subcommand, from their --help.
	Commands []string
}

// Areas are the folders of docs/, in index order.
var Areas = []Area{
	{Key: "shell", Title: "Shell", Summary: "The prompt, navigation, history search, aliases, colors and the help card that Gash adds to an interactive Bash."},
	{Key: "files", Title: "Files and archives", Summary: "Find what fills a disk, summarize a tree, extract any archive and back up a file."},
	{Key: "system", Title: "System", Summary: "Processes, ports, services, disks, sudo history and a system report."},
	{Key: "git", Title: "Git", Summary: "Sync, tags, patches, repository repair and the gl* log shortcuts."},
	{Key: "docker", Title: "Docker", Summary: "Start, stop and prune containers, and check and upgrade Compose stacks."},
	{Key: "remote", Title: "Remote hosts", Summary: "Run commands on, and copy and sync files with, the SSH hosts named in ~/.gash_env, and unlock SSH keys at login."},
	{Key: "databases", Title: "Databases", Summary: "The MySQL, MariaDB, PostgreSQL and SQLite connections of ~/.gash_env: list, test and query them read-only."},
	{Key: "runtimes", Title: "PHP, Node.js and Chrome", Summary: "Pick PHP and Node.js versions per project, run commands under a version, and keep headless Chrome current.",
		Commands: []string{"runtime", "run"}},
	{Key: "hte", Title: "Local web stack (HTE)", Summary: "gash hte: a userland Apache and PHP-FPM for local sites with HTTPS, one PHP-FPM pool per site.",
		Commands: []string{"hte"}},
	{Key: "windows", Title: "Windows and WSL", Summary: "Inspect and repair WSL, and read the Windows host from inside it.",
		Commands: []string{"wsl", "win"}},
	{Key: "logs", Title: "Logs", Summary: "gash logs: find the log files that fill a disk and reclaim the space safely.",
		Commands: []string{"logs"}},
	{Key: "ai", Title: "AI assistants", Summary: "Ask Claude or Gemini from the terminal, with the context of this system."},
	{Key: "agents", Title: "Tools for AI agents", Summary: "The read-only llm_* functions and the agent_* write tools that AI coding agents call: one JSON answer per call, with the safety rails of AGENTS.md.",
		Commands: []string{"agent-functions", "agents-hint"}},
	{Key: "gash", Title: "Gash itself", Summary: "Help, the doctor, prerequisites, settings, updates and removal.",
		Commands: []string{"version", "init", "update"}},
}

// areaOverrides place functions whose help topic is not their area.
var areaOverrides = map[string]string{
	"hgrep": "shell", "history_grep": "shell", "mkdir_cd": "shell", "path_add": "shell",
	"all_colors": "shell", "needs_help": "shell", "needs_confirm_prompt": "shell", "help": "shell",
	"gash_inspiring_quote": "shell",
	"wsl_compact":          "windows", "wsl_explorer": "windows", "wsl_restart": "windows",
	"wsl_shutdown": "windows", "wsl_taskmanager": "windows",
	"gash_db_list": "databases", "gash_db_test": "databases",
	"gash_ai_list": "ai",
	"nvm":          "runtimes",
}

// topicAreas place the other functions by their help topic.
var topicAreas = map[string]string{
	"files": "files", "system": "system", "nav": "shell", "git": "git", "docker": "docker",
	"remote": "remote", "dev": "runtimes", "llm": "agents", "agent": "agents", "ai": "ai", "gash": "gash",
}

// AreaOf is the area that documents a function.
func AreaOf(f registry.Function) string {
	if a, ok := areaOverrides[f.Name]; ok {
		return a
	}
	return topicAreas[f.Topic]
}

// FunctionsOf are the documented functions of an area (those with help),
// sorted by name.
func FunctionsOf(area string) []registry.Function {
	var out []registry.Function
	for _, f := range registry.Functions() {
		if f.HasHelp && AreaOf(f) == area {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// anchor is the GitHub heading anchor of a heading text: lower case,
// punctuation but hyphens and underscores dropped, spaces as hyphens.
func anchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127:
			b.WriteRune(r)
		}
	}
	return b.String()
}
