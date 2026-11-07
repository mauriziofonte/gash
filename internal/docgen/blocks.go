// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"errors"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/registry"
)

// SetBlock replaces the generated block NAME of a hand-written page: the
// lines between "<!-- BEGIN generated: NAME -->" and
// "<!-- END generated: NAME -->".
func SetBlock(src, name, body string) (string, error) {
	begin := "<!-- BEGIN generated: " + name + " (go test ./internal/docgen -update) -->\n"
	end := "<!-- END generated: " + name + " -->"
	i := strings.Index(src, begin)
	j := strings.Index(src, end)
	if i < 0 || j < i {
		return "", errors.New("no generated block " + name)
	}
	return src[:i+len(begin)] + body + src[j:], nil
}

// Prerequisites is the generated block of an area guide: the tools its
// functions and commands need beyond what Gash itself needs, each linked
// to its install commands in docs/requirements.md.
func Prerequisites(a Area) string {
	users := map[string][]string{}
	var order []string
	add := func(who string, needs []registry.Need) {
		for _, n := range needs {
			t, ok := prereq.Lookup(n.Tool)
			if !ok || t.Level == prereq.Required {
				continue
			}
			label := "`" + who + "`"
			if n.When != "" {
				label += " (" + n.When + ")"
			}
			if !slices.Contains(order, n.Tool) {
				order = append(order, n.Tool)
			}
			if !slices.Contains(users[n.Tool], label) {
				users[n.Tool] = append(users[n.Tool], label)
			}
		}
	}
	for _, f := range FunctionsOf(a.Key) {
		who := f.Name
		if c, ok := registry.CommandOf(f.Name); ok && len(c.FormerNames()) > 0 {
			who = c.Name()
		}
		add(who, registry.NeedsOf(f.Name))
	}
	for _, c := range a.Commands {
		add("gash "+c, registry.NeedsOf("gash "+c))
	}
	if len(order) == 0 {
		return "Nothing beyond what Gash itself needs ([requirements](../requirements.md)).\n"
	}
	// Catalog order: the order of docs/requirements.md.
	var tools []*prereq.Tool
	for _, t := range prereq.Catalog() {
		if slices.Contains(order, t.Name) {
			tools = append(tools, t)
		}
	}
	var b strings.Builder
	b.WriteString("Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):\n\n")
	for _, t := range tools {
		b.WriteString("- [" + t.Name + "](../requirements.md#tool-" + t.Name + ") (" + t.Level.String() + "), for " + join(users[t.Name]) + "\n")
	}
	return b.String()
}

// Index is the table of the areas, one row per area, with links below
// dir ("" in docs/README.md, "docs/" in README.md).
func Index(dir string) string {
	var b strings.Builder
	b.WriteString("| Area | What it covers | Reference |\n|---|---|---|\n")
	for _, a := range Areas {
		b.WriteString("| [" + a.Title + "](" + dir + a.Key + "/README.md) | " + cell(a.Summary) + " | [reference](" + dir + a.Key + "/reference.md) |\n")
	}
	return b.String()
}

func join(list []string) string {
	if len(list) <= 1 {
		return strings.Join(list, "")
	}
	return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
}

// Migration is the generated table of docs/migration-3.5.md: every former
// flat name with its canonical command and shortcut, by former name.
func Migration() string {
	type row struct{ former, canon, shortcut string }
	var rows []row
	for i := range registry.Tree() {
		c := &registry.Tree()[i]
		if c.Hidden {
			continue
		}
		for _, n := range c.FormerNames() {
			rows = append(rows, row{n, c.Name(), c.Shortcut})
		}
	}
	slices.SortFunc(rows, func(a, b row) int { return strings.Compare(a.former, b.former) })
	var b strings.Builder
	b.WriteString("| Former name | Command | Shortcut |\n|---|---|---|\n")
	for _, r := range rows {
		sc := ""
		if r.shortcut != "" {
			sc = "`" + r.shortcut + "`"
		}
		b.WriteString("| `" + r.former + "` | `" + r.canon + "` | " + sc + " |\n")
	}
	return b.String()
}
