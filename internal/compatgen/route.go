// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
)

// The shell side of the 3.5 command tree: the gash function routes a
// canonical command (gash files largest) to the shell function of its
// handler (__gash_fn_files_largest since 4.0, files_largest in 3.x), so
// that the command does in the shell what the handler's stub does (a stub
// override's cd, history and ssh-agent steps included); the words it does
// not route go to the binary.

// routeNode is one word of the tree below gash.
type routeNode struct {
	word    string
	handler string // the handler a command of these words runs
	kids    []*routeNode
}

func (n *routeNode) child(word string) *routeNode {
	for _, k := range n.kids {
		if k.word == word {
			return k
		}
	}
	k := &routeNode{word: word}
	n.kids = append(n.kids, k)
	return k
}

// routeTree is the tree of the commands that run a catalog function.
func routeTree() *routeNode {
	root := &routeNode{}
	for _, c := range registry.Tree() {
		handler := c.Handler
		if c.Command3x == "help" {
			// gash help --bash TOPIC is Bash's builtin help: gash_help's
			// shell function runs it.
			handler = "gash_help"
		}
		if handler == "" || c.Hidden {
			continue
		}
		n := root
		for _, w := range c.Path {
			n = n.child(w)
		}
		n.handler = handler
	}
	return root
}

// GashRoute is the case statement at the top of the gash function.
func GashRoute() string {
	var b strings.Builder
	writeRoute(&b, routeTree(), 1, "    ")
	return strings.TrimRight(b.String(), "\n")
}

func writeRoute(b *strings.Builder, n *routeNode, depth int, pad string) {
	if len(n.kids) == 0 {
		return
	}
	pos := []string{"", "${1-}", "${2-}", "${3-}"}[depth]
	b.WriteString(pad + "case \"" + pos + "\" in\n")
	for _, k := range n.kids {
		b.WriteString(pad + "    " + k.word + ")\n")
		writeRoute(b, k, depth+1, pad+"        ")
		if k.handler != "" {
			b.WriteString(pad + "        shift " + itoa(depth) + "\n")
			if registry.Legacy(k.handler) {
				// 4.0: the function of a legacy name is private.
				b.WriteString(pad + "        " + PrivatePrefix + k.handler + " \"$@\"\n")
			} else {
				b.WriteString(pad + "        __gash_route " + k.handler + " \"$@\"\n")
			}
			b.WriteString(pad + "        return\n")
		}
		b.WriteString(pad + "        ;;\n")
	}
	b.WriteString(pad + "esac\n")
}

func itoa(n int) string { return string(rune('0' + n)) }

// FormerTable is the body of __GASH_FORMER: each former flat name with the
// command and shortcut that replace it, for the one tip per name and
// session an interactive shell prints.
func FormerTable() string {
	type row struct{ name, value string }
	var rows []row
	for _, c := range registry.Tree() {
		for _, f := range c.FormerNames() {
			v := c.Name()
			if c.Shortcut != "" && c.Shortcut != f {
				v += " (shortcut " + c.Shortcut + ")"
			}
			rows = append(rows, row{f, v})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	var b strings.Builder
	for _, r := range rows {
		b.WriteString("    [" + r.name + "]='" + r.value + "'\n")
	}
	// 4.0: the namespace hte is localweb (gash hte runs it through 4.x).
	b.WriteString("    ['gash hte']='gash localweb'\n")
	return strings.TrimRight(b.String(), "\n")
}

// shortcutDefinition is the alias definition of a module short alias in
// 3.5: the canonical command its function runs.
func shortcutDefinition(a registry.Alias) string {
	if c, ok := registry.CommandOf(a.Function); ok && c.Shortcut == a.Name {
		return a.Name + "='" + c.Name() + "'"
	}
	return a.Definition
}
