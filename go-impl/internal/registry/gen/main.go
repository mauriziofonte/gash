// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command gen extracts the registry catalog from the Gash 2.x Bash sources
// and writes catalog_gen.go. It runs at development time only (go
// generate); the binary never reads Bash.
//
// Sources: lib/ (functions, help registrations, needs_help fallbacks,
// module short aliases, alias packs, card topics, settings), gash.sh (the
// headless settings subset), bash_completion (completion branches) and
// go-impl/PORT/MATRIX.md (class, decision, Go command, the variables the
// stubs pass). Every MATRIX function row must match a 2.x function and the
// reverse, or gen fails.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func main() {
	root := flag.String("root", "../../..", "repository root (holds lib/ and go-impl/)")
	out := flag.String("out", "catalog_gen.go", "output file")
	flag.Parse()
	c, err := extract(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	src, err := c.render()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

// fnDef is one 2.x function definition.
type fnDef struct {
	name     string
	file     string
	line     uint
	inFunc   string // enclosing function, if nested
	calls    map[string]bool
	vars     map[string]bool
	words    map[string]bool // literal identifier arguments of calls
	fallback *usage
}

type usage struct{ program, usage, desc string }

type helpEntry struct {
	name, module, short, aliases, seeAlso, body string
	file                                        string
	line                                        uint
}

type aliasDef struct {
	name, definition, function, pack, condition, source string
}

type completion struct {
	names   []string
	body    string
	comment string
}

// catalog is everything gen extracts.
type catalog struct {
	defs        map[string][]*fnDef
	helps       map[string]*helpEntry
	aliases     []aliasDef
	topics      [][4]string
	settings    []string
	headlessSet []string
	completions []completion
	packs       []string
	matrix      *matrix
}

// allSettings marks a function that reads every known setting.
const allSettings = "__GASH_SETTINGS_KNOWN*"

// dispatchers run their first argument as a command.
var dispatchers = map[string]bool{
	"__gash_no_history": true, "__gash_task": true, "command": true, "builtin": true, "exec": true,
}

func extract(root string) (*catalog, error) {
	c := &catalog{defs: map[string][]*fnDef{}, helps: map[string]*helpEntry{}}
	var files []string
	for _, pat := range []string{"lib/core/*.sh", "lib/modules/*.sh", "lib/modules/*/*.sh", "lib/aliases/*.sh", "lib/shell/*.sh"} {
		m, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil {
			return nil, err
		}
		for _, p := range m {
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
		}
	}
	sort.Strings(files)
	for _, rel := range files {
		s, err := parseSource(root, rel)
		if err != nil {
			return nil, err
		}
		c.scan(s, s.file.Stmts, scope{})
	}
	gashSh, err := parseSource(root, "gash.sh")
	if err != nil {
		return nil, err
	}
	c.scanHeadlessSettings(gashSh)
	comp, err := parseSource(root, "bash_completion")
	if err != nil {
		return nil, err
	}
	if err := c.scanCompletion(comp); err != nil {
		return nil, err
	}
	c.matrix, err = readMatrix(filepath.Join(root, "go-impl/PORT/MATRIX.md"))
	if err != nil {
		return nil, err
	}
	return c, nil
}

// scope is the context of a statement: the enclosing function and the
// conditions under which it runs.
type scope struct {
	fn    *fnDef
	conds []string
}

func (sc scope) with(cond string) scope {
	return scope{fn: sc.fn, conds: append(append([]string(nil), sc.conds...), cond)}
}

func (c *catalog) scan(s *source, stmts []*syntax.Stmt, sc scope) {
	for _, st := range stmts {
		c.stmt(s, st, sc)
	}
}

func (c *catalog) stmt(s *source, st *syntax.Stmt, sc scope) {
	switch x := st.Cmd.(type) {
	case *syntax.FuncDecl:
		d := &fnDef{
			name: x.Name.Value, file: s.rel, line: line(x),
			calls: map[string]bool{}, vars: map[string]bool{}, words: map[string]bool{},
		}
		if sc.fn != nil {
			d.inFunc = sc.fn.name
		}
		c.defs[d.name] = append(c.defs[d.name], d)
		c.body(d, x.Body)
		c.stmt(s, x.Body, scope{fn: d, conds: sc.conds})
	case *syntax.CallExpr:
		c.call(s, st, x, sc)
	case *syntax.DeclClause:
		for _, a := range x.Args {
			if a.Name != nil && a.Name.Value == "__GASH_SETTINGS_KNOWN" && sc.fn == nil {
				if v, ok := litWord(a.Value); ok {
					c.settings = strings.Fields(v)
				}
			}
		}
	case *syntax.IfClause:
		c.ifClause(s, x, sc)
	case *syntax.BinaryCmd:
		c.stmt(s, x.X, sc)
		left := printNode(x.X)
		switch x.Op {
		case syntax.AndStmt:
			c.stmt(s, x.Y, sc.with(left))
		case syntax.OrStmt:
			c.stmt(s, x.Y, sc.with("! "+left))
		default:
			c.stmt(s, x.Y, sc)
		}
	case *syntax.Block:
		c.scan(s, x.Stmts, sc)
	case *syntax.Subshell:
		c.scan(s, x.Stmts, sc)
	case *syntax.ForClause:
		c.scan(s, x.Do, sc.with("for "+printNode(x.Loop)))
	case *syntax.WhileClause:
		c.scan(s, x.Do, sc.with("while "+printStmts(x.Cond)))
	case *syntax.CaseClause:
		for _, item := range x.Items {
			var pats []string
			for _, p := range item.Patterns {
				pats = append(pats, printNode(p))
			}
			c.scan(s, item.Stmts, sc.with("case "+printNode(x.Word)+" in "+strings.Join(pats, " | ")))
		}
	}
	// Assignments at top level: the card topics.
	if ce, ok := st.Cmd.(*syntax.CallExpr); ok && len(ce.Args) == 0 && sc.fn == nil {
		for _, a := range ce.Assigns {
			if a.Name != nil && a.Name.Value == "__gash_alias_files" && a.Array != nil && s.rel == "lib/aliases/_loader.sh" {
				for _, el := range a.Array.Elems {
					if v, ok := litWord(el.Value); ok {
						c.packs = append(c.packs, v)
					}
				}
			}
			if a.Name != nil && a.Name.Value == "__GASH_CARD_TOPICS" && a.Array != nil {
				for _, el := range a.Array.Elems {
					v, ok := litWord(el.Value)
					if !ok {
						continue
					}
					parts := strings.SplitN(v, "|", 4)
					if len(parts) == 4 {
						c.topics = append(c.topics, [4]string{parts[0], parts[1], parts[2], parts[3]})
					}
				}
			}
		}
	}
}

func printStmts(stmts []*syntax.Stmt) string {
	var parts []string
	for _, st := range stmts {
		parts = append(parts, printNode(st))
	}
	return strings.Join(parts, "; ")
}

func (c *catalog) ifClause(s *source, x *syntax.IfClause, sc scope) {
	cond := printStmts(x.Cond)
	c.scan(s, x.Then, sc.with(cond))
	if x.Else == nil {
		return
	}
	neg := sc.with("! { " + cond + "; }")
	if len(x.Else.Cond) > 0 {
		c.ifClause(s, x.Else, neg)
	} else {
		c.scan(s, x.Else.Then, neg)
	}
}

// body collects the calls and variable reads of a function body.
func (c *catalog) body(d *fnDef, body *syntax.Stmt) {
	r := newRefs()
	r.collect(body)
	for v := range r.vars {
		d.vars[v] = true
	}
	syntax.Walk(body, func(node syntax.Node) bool {
		switch x := node.(type) {
		case *syntax.CallExpr:
			name := callName(x)
			if name != "" {
				d.calls[name] = true
			}
			if dispatchers[name] && len(x.Args) > 1 {
				if n, ok := litWord(x.Args[1]); ok {
					d.calls[n] = true
				}
			}
			for _, a := range x.Args[min(1, len(x.Args)):] {
				if w, ok := litWord(a); ok && isIdent(w) {
					d.words[w] = true
				}
			}
			if (name == "needs_help" || name == "__llm_needs_help") && len(x.Args) >= 4 && d.fallback == nil {
				p, ok1 := litWord(x.Args[1])
				u, ok2 := litWord(x.Args[2])
				h, ok3 := litWord(x.Args[3])
				if ok1 && ok2 && ok3 {
					d.fallback = &usage{p, u, h}
				}
			}
		case *syntax.ForClause:
			// for name in $__GASH_SETTINGS_KNOWN: reads every setting
			// from the shell (gash_doctor).
			if wi, ok := x.Loop.(*syntax.WordIter); ok {
				for _, w := range wi.Items {
					for _, p := range w.Parts {
						if pe, ok := p.(*syntax.ParamExp); ok && pe.Param != nil && pe.Param.Value == "__GASH_SETTINGS_KNOWN" {
							d.vars[allSettings] = true
						}
					}
				}
			}
		case *syntax.TestClause:
			syntax.Walk(x.X, func(n syntax.Node) bool {
				if u, ok := n.(*syntax.UnaryTest); ok && u.Op == syntax.TsVarSet {
					if w, ok := u.X.(*syntax.Word); ok {
						if v, ok := litWord(w); ok {
							d.vars[v] = true
						}
					}
				}
				return true
			})
		}
		return true
	})
}

func (c *catalog) call(s *source, st *syntax.Stmt, ce *syntax.CallExpr, sc scope) {
	switch callName(ce) {
	case "__gash_register_help":
		c.helpCall(s, st, ce)
	case "alias":
		c.aliasCall(s, ce, sc)
	}
}

func (c *catalog) helpCall(s *source, st *syntax.Stmt, ce *syntax.CallExpr) {
	if len(ce.Args) < 2 {
		return
	}
	name, ok := litWord(ce.Args[1])
	if !ok || name == "" || strings.HasPrefix(name, "$") {
		return
	}
	h := &helpEntry{name: name, file: s.rel, line: line(ce)}
	args := ce.Args[2:]
	for i := 0; i < len(args); i++ {
		opt, _ := litWord(args[i])
		val := ""
		if i+1 < len(args) {
			val, _ = litWord(args[i+1])
		}
		switch opt {
		case "--aliases":
			h.aliases = val
			i++
		case "--module":
			h.module = val
			i++
		case "--short":
			h.short = val
			i++
		case "--see-also":
			h.seeAlso = val
			i++
		}
	}
	for _, r := range st.Redirs {
		if r.Hdoc != nil && (r.Op == syntax.Hdoc || r.Op == syntax.DashHdoc) {
			// The heredoc is quoted: its text is literal. 2.x read it
			// whole, then stripped every trailing newline.
			h.body = strings.TrimRight(heredocText(r.Hdoc), "\n")
		}
	}
	c.helps[name] = h
}

// heredocText returns the raw text of a quoted heredoc body (no escape
// processing: the delimiter was quoted).
func heredocText(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		if lit, ok := p.(*syntax.Lit); ok {
			b.WriteString(lit.Value)
		}
	}
	return b.String()
}

func (c *catalog) aliasCall(s *source, ce *syntax.CallExpr, sc scope) {
	pack := ""
	switch {
	case strings.HasPrefix(s.rel, "lib/aliases/"):
		pack = strings.TrimSuffix(strings.TrimPrefix(s.rel, "lib/aliases/"), ".sh")
	case strings.HasPrefix(s.rel, "lib/modules/"):
	default:
		return
	}
	for _, a := range ce.Args[1:] {
		def := s.text(a)
		name := def
		if i := strings.Index(def, "="); i >= 0 {
			name = def[:i]
		}
		if n, ok := litWordPrefix(a); ok {
			name = n
		}
		ad := aliasDef{
			name: name, definition: def, pack: pack,
			condition: strings.Join(sc.conds, " && "),
			source:    fmt.Sprintf("%s:%d", s.rel, line(a)),
		}
		if pack == "" {
			if v, ok := litWord(a); ok {
				if i := strings.Index(v, "="); i >= 0 {
					ad.name, ad.function = v[:i], v[i+1:]
				}
			}
		}
		c.aliases = append(c.aliases, ad)
	}
}

// litWordPrefix returns the literal alias name of a name=value word when
// the part before = is literal.
func litWordPrefix(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, p := range w.Parts {
		var part strings.Builder
		if !litPart(&part, p) {
			break
		}
		b.WriteString(part.String())
		if i := strings.Index(b.String(), "="); i >= 0 {
			return b.String()[:i], true
		}
	}
	return "", false
}

func (c *catalog) scanHeadlessSettings(s *source) {
	syntax.Walk(s.file, func(node syntax.Node) bool {
		ce, ok := node.(*syntax.CallExpr)
		if ok && callName(ce) == "__gash_apply_settings" && len(ce.Args) > 1 && c.headlessSet == nil {
			for _, a := range ce.Args[1:] {
				if v, ok := litWord(a); ok {
					c.headlessSet = append(c.headlessSet, v)
				}
			}
		}
		return true
	})
}

func (c *catalog) scanCompletion(s *source) error {
	var found bool
	syntax.Walk(s.file, func(node syntax.Node) bool {
		fd, ok := node.(*syntax.FuncDecl)
		if !ok || fd.Name.Value != "__gash_complete" {
			return true
		}
		syntax.Walk(fd.Body, func(n syntax.Node) bool {
			cc, ok := n.(*syntax.CaseClause)
			if !ok || found {
				return true
			}
			found = true
			for _, item := range cc.Items {
				var cm completion
				for _, p := range item.Patterns {
					v, _ := litWord(p)
					cm.names = append(cm.names, v)
				}
				for _, com := range item.Comments {
					if com.Pos().Line() < item.Pos().Line() {
						cm.comment = strings.TrimSpace(com.Text)
					}
				}
				if len(item.Stmts) > 0 {
					first, last := item.Stmts[0], item.Stmts[len(item.Stmts)-1]
					start := int(first.Pos().Offset())
					// From the start of the first line, so dedent sees it.
					for start > 0 && s.src[start-1] != '\n' {
						start--
					}
					cm.body = dedent(string(s.src[start:last.End().Offset()]))
				}
				c.completions = append(c.completions, cm)
			}
			return false
		})
		return false
	})
	if !found {
		return fmt.Errorf("bash_completion: no case statement in __gash_complete")
	}
	return nil
}

// dedent removes the common leading spaces of non-empty lines.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " "))
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	for i, l := range lines {
		if len(l) >= minIndent && minIndent > 0 {
			lines[i] = l[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}
