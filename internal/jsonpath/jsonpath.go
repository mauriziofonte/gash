// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package jsonpath is the path syntax of llm_json --path, llm_config
// --path and the --json-path flags (llm_http, llm_wait): dotted keys and
// bracket steps, evaluated on a value decoded by encoding/json.
//
//	a.b[0].c            key a, key b, index 0, key c
//	paths["/api/x"].post a quoted key holds any character
//	[2], [-1]           an index of the root array; -1 is the last element
//	"" or "."           the whole value
//
// A leading "$" or "." is accepted and ignored.
package jsonpath

import (
	"errors"
	"strconv"
	"strings"
)

// Step is one step of a path: a key of an object or an index of an array.
type Step struct {
	Key     string
	Index   int
	IsIndex bool
}

// ErrSyntax is a path that does not parse.
var ErrSyntax = errors.New("invalid path")

// Parse parses p.
func Parse(p string) ([]Step, error) {
	p = strings.TrimPrefix(p, "$")
	var steps []Step
	i := 0
	if strings.HasPrefix(p, ".") {
		i = 1
	}
	for i < len(p) {
		switch p[i] {
		case '[':
			j := i + 1
			if j < len(p) && (p[j] == '"' || p[j] == '\'') {
				key, n, err := quoted(p[j:])
				if err != nil {
					return nil, err
				}
				j += n
				if j >= len(p) || p[j] != ']' {
					return nil, ErrSyntax
				}
				steps = append(steps, Step{Key: key})
				i = j + 1
			} else {
				k := strings.IndexByte(p[j:], ']')
				if k < 0 {
					return nil, ErrSyntax
				}
				n, err := strconv.Atoi(strings.TrimSpace(p[j : j+k]))
				if err != nil {
					return nil, ErrSyntax
				}
				steps = append(steps, Step{Index: n, IsIndex: true})
				i = j + k + 1
			}
		case '.':
			i++
			if i >= len(p) || p[i] == '.' || p[i] == '[' {
				return nil, ErrSyntax
			}
			continue
		default:
			j := i
			for j < len(p) && p[j] != '.' && p[j] != '[' {
				if p[j] == ']' || p[j] == '"' {
					return nil, ErrSyntax
				}
				j++
			}
			steps = append(steps, Step{Key: p[i:j]})
			i = j
		}
		if i < len(p) && p[i] != '.' && p[i] != '[' {
			return nil, ErrSyntax
		}
	}
	return steps, nil
}

// quoted reads a quoted key at the start of s ("..." or '...', with \
// escaping the quote or a backslash) and returns it and the bytes read.
func quoted(s string) (string, int, error) {
	q := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case s[i] == q:
			return b.String(), i + 1, nil
		default:
			b.WriteByte(s[i])
		}
	}
	return "", 0, ErrSyntax
}

// Get walks v along steps. It returns the value and true, or, when a step
// does not exist, nil, false and the longest prefix of steps that does
// (for the error hint).
func Get(v any, steps []Step) (any, []Step, bool) {
	cur := v
	for n, s := range steps {
		switch x := cur.(type) {
		case map[string]any:
			if s.IsIndex {
				return nil, steps[:n], false
			}
			next, ok := x[s.Key]
			if !ok {
				return nil, steps[:n], false
			}
			cur = next
		case []any:
			if !s.IsIndex {
				return nil, steps[:n], false
			}
			i := s.Index
			if i < 0 {
				i += len(x)
			}
			if i < 0 || i >= len(x) {
				return nil, steps[:n], false
			}
			cur = x[i]
		default:
			return nil, steps[:n], false
		}
	}
	return cur, steps, true
}

// Format writes steps back in the canonical syntax: dotted keys where the
// key is a plain word, quoted brackets otherwise; "." for the root.
func Format(steps []Step) string {
	if len(steps) == 0 {
		return "."
	}
	var b strings.Builder
	for i, s := range steps {
		switch {
		case s.IsIndex:
			b.WriteString("[" + strconv.Itoa(s.Index) + "]")
		case plain(s.Key):
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s.Key)
		default:
			b.WriteString("[" + strconv.Quote(s.Key) + "]")
		}
	}
	return b.String()
}

func plain(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}
