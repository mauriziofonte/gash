// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"strconv"
	"strings"
)

// Env builds a child environment. Nothing of the parent passes unless an
// allowlist names it; the entries keep the order they were first added in.
// The zero value is an empty environment ready to use.
type Env struct {
	names  []string
	values map[string]string
}

// NewEnv returns an empty environment.
func NewEnv() *Env { return &Env{} }

// Allow copies from parent ("NAME=value" entries, as os.Environ returns
// them) the variables that names lists, when parent has them. A name
// ending in "*" is a prefix ("LC_*"): its matches are added in parent
// order. Of duplicate parent entries the first wins, as os.Getenv reads
// them. A variable already in e keeps its value.
func (e *Env) Allow(parent []string, names ...string) *Env {
	first := make(map[string]string, len(parent))
	var order []string
	for _, kv := range parent {
		i := strings.IndexByte(kv, '=')
		if i <= 0 || strings.IndexByte(kv[:i], 0) >= 0 {
			continue
		}
		if _, dup := first[kv[:i]]; !dup {
			first[kv[:i]] = kv[i+1:]
			order = append(order, kv[:i])
		}
	}
	for _, n := range names {
		if prefix, ok := strings.CutSuffix(n, "*"); ok {
			for _, name := range order {
				if strings.HasPrefix(name, prefix) {
					e.add(name, first[name])
				}
			}
			continue
		}
		if v, ok := first[n]; ok {
			e.add(n, v)
		}
	}
	return e
}

// AllowAll copies every variable of parent: for the children to which 2.x
// passed the caller's whole environment.
func (e *Env) AllowAll(parent []string) *Env { return e.Allow(parent, "*") }

func (e *Env) add(name, value string) {
	if _, ok := e.values[name]; ok {
		return
	}
	e.Set(name, value)
}

// Set sets a variable, replacing its value in place when it is present.
// It panics on a name that is empty or holds "=" or NUL.
func (e *Env) Set(name, value string) *Env {
	if name == "" || strings.ContainsAny(name, "=\x00") {
		panic("runner: invalid environment variable name " + strconv.Quote(name))
	}
	if e.values == nil {
		e.values = map[string]string{}
	}
	if _, ok := e.values[name]; !ok {
		e.names = append(e.names, name)
	}
	e.values[name] = value
	return e
}

// Unset removes a variable.
func (e *Env) Unset(name string) *Env {
	if _, ok := e.values[name]; !ok {
		return e
	}
	delete(e.values, name)
	for i, n := range e.names {
		if n == name {
			e.names = append(e.names[:i:i], e.names[i+1:]...)
			break
		}
	}
	return e
}

// Lookup returns a variable's value and whether it is set.
func (e *Env) Lookup(name string) (string, bool) {
	v, ok := e.values[name]
	return v, ok
}

// Environ returns the entries, "NAME=value", in order; never nil.
func (e *Env) Environ() []string {
	out := make([]string, 0, len(e.names))
	for _, n := range e.names {
		out = append(out, n+"="+e.values[n])
	}
	return out
}
