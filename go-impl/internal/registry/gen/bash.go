// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// source is one parsed 2.x file.
type source struct {
	rel  string // path relative to the repository root
	src  []byte
	file *syntax.File
}

func parseSource(root, rel string) (*source, error) {
	b, err := os.ReadFile(root + "/" + rel)
	if err != nil {
		return nil, err
	}
	p := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.KeepComments(true))
	f, err := p.Parse(bytes.NewReader(b), rel)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	return &source{rel: rel, src: b, file: f}, nil
}

// text returns the source bytes of a node.
func (s *source) text(n syntax.Node) string {
	return string(s.src[n.Pos().Offset():n.End().Offset()])
}

// line returns the 1-based line of a node.
func line(n syntax.Node) uint { return n.Pos().Line() }

// litWord evaluates a word made only of literal parts (unquoted literals,
// single quotes, $'...' and double quotes without expansions), as Bash
// would. ok is false when the word holds any expansion.
func litWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	var b strings.Builder
	for _, part := range w.Parts {
		if !litPart(&b, part) {
			return "", false
		}
	}
	return b.String(), true
}

func litPart(b *strings.Builder, part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		b.WriteString(unescapeUnquoted(p.Value))
	case *syntax.SglQuoted:
		if p.Dollar {
			s, ok := ansiC(p.Value)
			if !ok {
				return false
			}
			b.WriteString(s)
		} else {
			b.WriteString(p.Value)
		}
	case *syntax.DblQuoted:
		if p.Dollar {
			return false
		}
		for _, q := range p.Parts {
			lit, ok := q.(*syntax.Lit)
			if !ok {
				return false
			}
			b.WriteString(unescapeDouble(lit.Value))
		}
	default:
		return false
	}
	return true
}

// unescapeUnquoted removes the backslashes of an unquoted literal.
func unescapeUnquoted(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			if s[i] == '\n' {
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unescapeDouble removes the backslashes that are special inside double
// quotes: before $ ` " \ and newline.
func unescapeDouble(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '`', '"', '\\':
				i++
			case '\n':
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ansiC decodes the few $'...' escapes the 2.x literals use.
func ansiC(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'e', 'E':
			b.WriteByte(0x1b)
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		case '"':
			b.WriteByte('"')
		default:
			return "", false
		}
	}
	return b.String(), true
}

// callName returns the literal command name of a call, or "".
func callName(ce *syntax.CallExpr) string {
	if ce == nil || len(ce.Args) == 0 {
		return ""
	}
	s, _ := litWord(ce.Args[0])
	return s
}

// printNode prints a node with the canonical shfmt layout, on one line
// where possible.
func printNode(n syntax.Node) string {
	var buf bytes.Buffer
	pr := syntax.NewPrinter(syntax.Indent(4), syntax.SwitchCaseIndent(true))
	_ = pr.Print(&buf, n)
	return strings.TrimSpace(buf.String())
}

// refs collects, inside a node, every literal word (candidate function or
// variable names given as arguments), every parameter expansion name and
// every arithmetic variable.
type refs struct {
	words map[string]bool
	vars  map[string]bool
}

func newRefs() *refs { return &refs{words: map[string]bool{}, vars: map[string]bool{}} }

func (r *refs) collect(n syntax.Node) {
	syntax.Walk(n, func(node syntax.Node) bool {
		switch x := node.(type) {
		case *syntax.Word:
			if s, ok := litWord(x); ok {
				r.words[s] = true
				// key=value words, and lists such as " GASH_A GASH_B ".
				for _, f := range strings.FieldsFunc(s, func(c rune) bool {
					return !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9')
				}) {
					r.words[f] = true
				}
			}
			for _, p := range x.Parts {
				if dq, ok := p.(*syntax.DblQuoted); ok {
					for _, q := range dq.Parts {
						if lit, ok := q.(*syntax.Lit); ok {
							r.words[lit.Value] = true
						}
					}
				}
			}
		case *syntax.ParamExp:
			if x.Param != nil {
				r.vars[x.Param.Value] = true
			}
		case *syntax.ArithmCmd:
			r.arith(x.X)
		case *syntax.ArithmExp:
			r.arith(x.X)
		case *syntax.Assign:
			if x.Name != nil && x.Append {
				r.vars[x.Name.Value] = true
			}
		}
		return true
	})
}

func (r *refs) arith(e syntax.ArithmExpr) {
	if e == nil {
		return
	}
	syntax.Walk(e, func(node syntax.Node) bool {
		if w, ok := node.(*syntax.Word); ok {
			if s, ok := litWord(w); ok && isIdent(s) {
				r.vars[s] = true
			}
		}
		return true
	})
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
