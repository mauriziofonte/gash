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
		add(f.Name, registry.NeedsOf(f.Name))
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

// Index is the generated block of docs/README.md: one row per area.
func Index() string {
	var b strings.Builder
	b.WriteString("| Area | What it covers | Reference |\n|---|---|---|\n")
	for _, a := range Areas {
		b.WriteString("| [" + a.Title + "](" + a.Key + "/README.md) | " + cell(a.Summary) + " | [reference](" + a.Key + "/reference.md) |\n")
	}
	return b.String()
}

func join(list []string) string {
	if len(list) <= 1 {
		return strings.Join(list, "")
	}
	return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
}
