// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

const agentFunctionsUsage = `Usage: gash agent-functions [--json]

List the agent functions: every llm_* (read-only, family "read") and
agent_* (family "write") function. Each runs as gash <fn> args, with the
output of the sourced function.

  --json   one JSON array: [{"name","family","short","usage","flags","replaces"}]
           (usage: the USAGE lines of its help; flags: the options they name;
           replaces: the raw shell shape it replaces, or null)
`

// agentFunction is one row of gash agent-functions --json.
type agentFunction struct {
	Name     string   `json:"name"`
	Family   string   `json:"family"`
	Short    string   `json:"short"`
	Usage    []string `json:"usage"`
	Flags    []string `json:"flags"`
	Replaces *string  `json:"replaces"`
}

var usageFlag = lazyre.MustCompile(`(?:^|[\s\[|(])(--?[A-Za-z][A-Za-z0-9-]*)`)

// agentFunctions is gash agent-functions [--json] (plan S7): the llm_*
// and agent_* functions of the registry that the binary runs.
func (m *Module) agentFunctions(c *invoke.Call) int {
	if _, partial, ok := completing(c); ok {
		return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp, "--json", "--help")
	}
	asJSON := false
	for _, a := range c.Args {
		switch {
		case isHelp(a):
			_, _ = io.WriteString(c.Stdout, agentFunctionsUsage)
			return 0
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "gash agent-functions takes --json")
		default:
			return fail(c, "unexpected_argument", a, "gash agent-functions takes no argument")
		}
	}
	rows := listAgentFunctions(c.Handlers)
	if asJSON {
		_ = llmfmt.JSONLine(c.Stdout, rows)
		return 0
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r.Name))
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.Name + strings.Repeat(" ", width-len(r.Name)+2) + r.Family + strings.Repeat(" ", 7-len(r.Family)) + r.Short + "\n")
	}
	_, _ = io.WriteString(c.Stdout, b.String())
	return 0
}

// listAgentFunctions builds the rows, sorted by name; with a table, only
// the functions it has a handler for.
func listAgentFunctions(t *invoke.Table) []agentFunction {
	rows := []agentFunction{}
	for i := range registry.Functions() {
		f := &registry.Functions()[i]
		family := ""
		switch {
		case strings.HasPrefix(f.Name, "llm_"):
			family = "read"
		case strings.HasPrefix(f.Name, "agent_"):
			family = "write"
		default:
			continue
		}
		if t != nil {
			if _, ok := t.Lookup(f.Name); !ok {
				continue
			}
		}
		usage := usageLines(f.Help.Body)
		row := agentFunction{Name: f.Name, Family: family, Short: f.Help.Short, Usage: usage, Flags: []string{}}
		seen := map[string]bool{}
		for _, u := range usage {
			for _, mm := range usageFlag.FindAllStringSubmatch(u, -1) {
				if !seen[mm[1]] {
					seen[mm[1]] = true
					row.Flags = append(row.Flags, mm[1])
				}
			}
		}
		if f.Replaces != "" {
			r := f.Replaces
			row.Replaces = &r
		}
		rows = append(rows, row)
	}
	return rows
}

// usageLines are the lines of a help body's USAGE block, trimmed.
func usageLines(body string) []string {
	out := []string{}
	_, rest, ok := strings.Cut(body, "USAGE\n")
	if !ok {
		return out
	}
	for _, l := range strings.Split(rest, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			break
		}
		out = append(out, l)
	}
	return out
}
