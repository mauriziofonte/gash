// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"slices"
	"sort"
	"strings"
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

// commands are the public commands of the binary: `gash WORD` runs them
// instead of a card topic.
var commands = []string{"init", "update", "version", "hte", "logs", "wsl", "win", "runtime", "run", "agent-functions"}

// Commands returns the public commands of the binary (internal/cli selects
// them, the generated completion offers them after the card topics).
func Commands() []string { return slices.Clone(commands) }

// CardAliases returns the reference card entries of the alias file FILE
// (lib/aliases/FILE.sh) in file order; nil for a file no topic lists.
func CardAliases(file string) []CardAlias { return cardAliases[file] }

// CardAliasNames returns the names of every card entry, sorted, each
// once: the names whose live alias value the gash stub passes.
func CardAliasNames() []string {
	var out []string
	for _, list := range cardAliases {
		for _, a := range list {
			out = append(out, a.Name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// CardAliasFamilies returns the Each prefixes of the card entries that
// stand for a family of aliases (php for php83), sorted, each once: the
// gash stub passes the live aliases named PREFIX followed by digits.
func CardAliasFamilies() []string {
	var out []string
	for _, list := range cardAliases {
		for _, a := range list {
			if a.Each != "" {
				out = append(out, a.Each)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

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

// Delegated reports whether the generated stub of f calls the binary
// (`gash NAME`), which makes NAME a command of the binary: every function
// except the gash card itself (the binary's root), the Bash helpers of
// the core shims (needs_help, needs_confirm_prompt, path_add) and the
// interactive-only shell functions that gash init bash emits. gash_unload
// counts here (it gets a stub slot, filled by the stub/gash_unload
// override), although its override runs entirely in Bash and never calls
// the binary for the function itself, only needs_help for --help.
func (f *Function) Delegated() bool {
	switch {
	case f.Name == "gash":
		return false
	case f.Impl == ImplShim:
		return !strings.HasPrefix(f.StubFile, "lib/core/")
	case f.Impl == ImplInit:
		return f.Headless
	}
	return true
}

// HasStub reports whether the generated Bash defines f as a one-line stub
// that calls the binary: the delegated functions, and the gash card,
// whose stub hands every argument to the binary's root (`gash version`
// from the shell reaches the 3.x command).
func (f *Function) HasStub() bool { return f.Delegated() || f.Name == "gash" }

// ModuleOrder is the order of the help modules in gash_help and gash_help
// --list: the fixed list of 2.x, then the modules 2.x appended in Bash's
// hash order, as Bash 5.2.21 printed them (LEDGER CORE-07). Every module
// with registered help is in it.
var ModuleOrder = []string{
	"files", "system", "git", "docker", "docker-compose", "ai", "sysinfo", "gash", "ssh", "llm", "utils",
	"node", "remote", "chrome", "php",
	// New in 3.4: the agent_* write family, its own help module.
	"agent",
}
