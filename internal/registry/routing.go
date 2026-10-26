// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"strings"
)

// baseReplaces is the Replaces field of the agent functions that predate
// 3.4 (their entries live in catalog_gen.go). It is kept apart so that the
// routing rows of old functions never touch their catalog entries;
// mergeCatalog fills it in. A line "SHAPE => FLAGS" routes SHAPE to the
// function called with FLAGS.
var baseReplaces = map[string]string{
	"llm_config": "cat config/app.yaml (a config file that is not secret)\n" +
		"python3 -c \"import tomllib, yaml ...\"; yq .a.b f => --path a.b",
	"llm_db_explain": "mysql -e \"EXPLAIN SELECT ...\"",
	"llm_db_query": "mysql -N -B -e \"SELECT ...\"; sqlite3 f \"SELECT ...\" => --format tsv\n" +
		"llm_db_query ... | jq -r '.rows[0][0]' => --scalar",
	"llm_db_sample":    "mysql -e \"SELECT * FROM t LIMIT 5\"",
	"llm_db_schema":    "mysql -e \"SHOW CREATE TABLE t\"; sqlite3 f .schema\nmysqldump --no-data => --all",
	"llm_db_tables":    "mysql -e \"SHOW TABLES\"; sqlite3 f .tables\nSELECT count(*) FROM t, per table => --counts",
	"llm_deps":         "cat composer.json package.json | jq .require",
	"llm_docker_check": "docker compose pull --dry-run; docker images",
	"llm_env":          "env | grep -i PATH; printenv NODE_ENV",
	"llm_exec":         "bash -c \"...\" for an allowlisted command",
	"llm_find": "find D -name '*.php' -not -path '*/vendor/*'\n" +
		"find D -type f | wc -l => --count\n" +
		"find D -newer F | sort | head => --newer F --sort mtime",
	"llm_git_diff": "git diff --stat | tail -1 => --summary\ngit diff --name-status; git diff --numstat => --files",
	"llm_git_log":  "git log --oneline -20 -- path => --path P",
	"llm_git_status": "git status --short; git branch -vv\n" +
		"for r; do git -C $r status -sb; done => --repos\n" +
		"git fetch && git status -sb => --fetch",
	"llm_grep": "grep -rn --include='*.php' X . | grep -v vendor => --ext php\n" +
		"grep -o RE f | sort | uniq -c => --distinct",
	"llm_ports":   "ss -tuln\nss -tlnp | grep :8080; lsof -i :8080 => --port 8080",
	"llm_procs":   "ps aux | grep X | grep -v grep => --name X\nkill -0 PID => --pid PID",
	"llm_project": "ls; cat composer.json package.json (what kind of project is this)",
	"llm_tree":    "tree -L 2 -I node_modules\ndu -sh */ | sort -rh | head => --top 10",
}

// Route is one row of the routing table: the raw command shape an agent
// would type, and the Gash call that answers it.
type Route struct {
	Fn    string // the function
	Use   string // the call: the function and the flags of this shape
	Shape string // the raw command shape
	Write bool   // an agent_* write tool
}

// Routes returns the routing rows of every llm_* and agent_* function
// with a Replaces field: the read functions first, then the write tools,
// each group by name, one row per Replaces line.
func Routes() []Route {
	var reads, writes []Route
	for _, f := range functions {
		if !agentFamily(f.Name) || f.Replaces == "" {
			continue
		}
		for _, line := range strings.Split(f.Replaces, "\n") {
			shape, flags, _ := strings.Cut(line, " => ")
			r := Route{Fn: f.Name, Use: f.Name, Shape: strings.TrimSpace(shape), Write: strings.HasPrefix(f.Name, "agent_")}
			if flags = strings.TrimSpace(flags); flags != "" {
				r.Use += " " + flags
			}
			if r.Write {
				writes = append(writes, r)
			} else {
				reads = append(reads, r)
			}
		}
	}
	return append(reads, writes...)
}

// RoutesOf returns the routing rows of the function named name, in the
// order of its Replaces lines.
func RoutesOf(name string) []Route {
	var out []Route
	for _, r := range Routes() {
		if r.Fn == name {
			out = append(out, r)
		}
	}
	return out
}

// agentFamily reports whether name is an agent function: llm_* reads and
// agent_* writes.
func agentFamily(name string) bool {
	return strings.HasPrefix(name, "llm_") || strings.HasPrefix(name, "agent_")
}

// RoutingTable is the routing table as Markdown, the one AGENTS.md
// section 7 holds and `gash agents-hint` prints.
func RoutingTable() string {
	var b strings.Builder
	b.WriteString("| Instead of | Use |\n| --- | --- |\n")
	for _, r := range Routes() {
		b.WriteString("| " + mdCode(r.Shape) + " | " + mdCode(r.Use) + " |\n")
	}
	return b.String()
}

// AgentsHint is what `gash agents-hint` prints: a short preamble for an
// agent's instructions, then the routing table.
func AgentsHint() string {
	return "## Gash tools\n\n" +
		"Prefer these to the raw commands on the left: one call, compact JSON on stdout " +
		"(`--format tsv` for rows), output capped by `--max-lines` and `--width`. " +
		"Run them as `gash FN ARGS` or, after `GASH_HEADLESS=1 source ~/.gash/gash.sh`, as `FN ARGS`; " +
		"`FN --help` lists the flags. Errors are one JSON line on stderr: " +
		"`action` RETRY = fix the arguments, CONTINUE = skip, STOP = ask the user, FATAL = stop. " +
		"`llm_*` only read. `agent_*` write: name every target, run `--dry-run` first.\n\n" +
		RoutingTable()
}

// mdCode writes s as a Markdown code span inside a table cell: a pipe is
// escaped, and a backtick in s widens the fence.
func mdCode(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	if fence != "`" {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}
