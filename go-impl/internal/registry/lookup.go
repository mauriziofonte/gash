// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"sort"
	"sync"
)

var byName = sync.OnceValue(func() map[string]*Function {
	m := make(map[string]*Function, len(functions))
	for i := range functions {
		m[functions[i].Name] = &functions[i]
	}
	return m
})

// Functions returns every 2.x public function, sorted by name. The slice
// is shared: do not modify it.
func Functions() []Function { return functions }

// Lookup returns the function named name.
func Lookup(name string) (*Function, bool) {
	f, ok := byName()[name]
	return f, ok
}

// ByAlias returns the function a module short alias points to.
func ByAlias(alias string) (*Function, bool) {
	for i := range aliases {
		if aliases[i].Pack == "" && aliases[i].Name == alias {
			return Lookup(aliases[i].Function)
		}
	}
	return nil, false
}

// Aliases returns the module short aliases and the alias pack entries.
func Aliases() []Alias { return aliases }

// Packs returns the alias packs in 2.x load order.
func Packs() []Pack { return packs }

// Settings returns the GASH_* settings in 2.x order.
func Settings() []Setting { return settings }

// Topics returns the reference card topics in card order.
func Topics() []Topic { return topics }

// Modules returns the distinct modules, sorted.
func Modules() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range functions {
		if !seen[f.Module] {
			seen[f.Module] = true
			out = append(out, f.Module)
		}
	}
	sort.Strings(out)
	return out
}

// StubVars returns the variables the stub of f passes: CommonShellVars,
// then f's own shell variables, then its settings.
func (f *Function) StubVars() []string {
	out := append([]string(nil), CommonShellVars...)
	seen := map[string]bool{}
	for _, v := range out {
		seen[v] = true
	}
	for _, list := range [][]string{f.ShellVars, f.Settings} {
		for _, v := range list {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}
