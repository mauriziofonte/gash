// Gash 4.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

// The generated files are sourced, and their cold definitions evaluated,
// with aliases off (__gash_source, __gash_cold), so an alias of the user's
// shell is not baked into a function. Bash 5.2 still parses the text of a
// command or process substitution again each time it runs, with the alias
// expansion of that moment: in an interactive shell with
// GASH_REPLACE_TOOLS=1, tail is multitail when $(... | tail -1) runs, and
// node_upgrade read multitail's usage as the newest Node.js patch. So the
// generator quotes the command name of every command inside $(...) and
// <(...): \tail is never expanded as an alias and still runs the function,
// builtin or program of that name.

// aliasable matches a literal command name that can be an alias: an alias
// name holds no /, $, `, =, quote or shell metacharacter.
var aliasable = regexp.MustCompile(`^[A-Za-z0-9_.:+@%,-]+$`)

// unalias returns src (the file path) with a backslash before the command
// name of each simple command run by a command or process substitution.
// A name that is not a plain literal (quoted, escaped, from a variable) is
// left as it is, and so are the commands of backquoted substitutions,
// where a backslash has its own rules.
func unalias(path string, src []byte) ([]byte, error) {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(bytes.NewReader(src), path)
	if err != nil {
		return nil, fmt.Errorf("compatgen: %s: %w", path, err)
	}
	var offsets []int
	var stack []syntax.Node
	inside, backq := 0, 0 // the substitutions on the stack, the backquoted ones
	count := func(n syntax.Node, d int) {
		switch n := n.(type) {
		case *syntax.CmdSubst:
			if n.Backquotes {
				backq += d
			} else {
				inside += d
			}
		case *syntax.ProcSubst:
			inside += d
		}
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		if n == nil {
			count(stack[len(stack)-1], -1)
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		count(n, 1)
		if c, ok := n.(*syntax.CallExpr); ok && inside > 0 && backq == 0 && len(c.Args) > 0 {
			if name, ok := plainName(c.Args[0]); ok && aliasable.MatchString(name) {
				offsets = append(offsets, int(c.Args[0].Pos().Offset()))
			}
		}
		return true
	})
	if len(offsets) == 0 {
		return src, nil
	}
	sort.Ints(offsets)
	out := make([]byte, 0, len(src)+len(offsets))
	last := 0
	for _, o := range offsets {
		out = append(out, src[last:o]...)
		out = append(out, '\\')
		last = o
	}
	return append(out, src[last:]...), nil
}

// plainName is the value of a word made of one unquoted literal.
func plainName(w *syntax.Word) (string, bool) {
	if len(w.Parts) != 1 {
		return "", false
	}
	l, ok := w.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return l.Value, true
}
