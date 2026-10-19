// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// Module states of apache modules list.
const (
	ModuleBase      = "base"      // in the userland list: every generation loads it
	ModuleEnabled   = "enabled"   // an extra module of hte.toml (apache modules enable)
	ModuleAvailable = "available" // in the system module directory, not loaded
)

// ModuleInfo is one module of apache modules list.
type ModuleInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
	// Builtin: compiled into the Apache binary, so no LoadModule line.
	Builtin bool `json:"builtin"`
	// Path is the module file, "" when built in or missing.
	Path string `json:"path,omitempty"`
	// Missing: a base or enabled module without a file (apache_module_missing).
	Missing bool `json:"missing,omitempty"`
}

// ModulesResult is apache modules list.
type ModulesResult struct {
	Dir     string       `json:"dir"` // the system module directory
	Modules []ModuleInfo `json:"modules"`
}

// ModuleResult is apache modules enable and disable.
type ModuleResult struct {
	Module   string   `json:"module"`
	Changed  bool     `json:"changed"`
	Applied  *Applied `json:"applied,omitempty"`
	Warnings []string `json:"warnings"`
}

// ApacheModulesList is apache modules list: the userland list, the extra
// modules of hte.toml, then the other modules of the system module
// directory, each with its state.
func (e *Engine) ApacheModulesList(ctx context.Context) (*ModulesResult, error) {
	v, err := e.ready("apache modules list")
	if err != nil {
		return nil, err
	}
	set, err := e.installed(v)
	if err != nil {
		return nil, err
	}
	a, err := discover.FindApache(e.Sysroot, v.os)
	if err != nil {
		return nil, err
	}
	builtin, err := discover.BuiltinModules(a, e.probe(ctx))
	if err != nil {
		return nil, err
	}
	resolved := discover.ResolveModules(e.Sysroot, a, builtin, set.Modules)
	res := &ModulesResult{Dir: a.ModuleDir, Modules: []ModuleInfo{}}
	listed := map[string]bool{}
	add := func(name, st string) {
		if listed[name] {
			return
		}
		listed[name] = true
		info := ModuleInfo{Name: name, State: st, Builtin: slices.Contains(builtin, name)}
		if i := slices.IndexFunc(resolved.Load, func(m render.Module) bool { return m.Name == name }); i >= 0 {
			info.Path = resolved.Load[i].Path
		} else if st == ModuleAvailable {
			info.Path = path.Join(a.ModuleDir, "mod_"+name+".so")
		}
		info.Missing = slices.Contains(resolved.Missing, name)
		res.Modules = append(res.Modules, info)
	}
	for _, m := range render.BaseModules {
		add(m, ModuleBase)
	}
	for _, m := range set.Modules {
		add(m, ModuleEnabled)
	}
	for _, m := range e.systemModules(a) {
		add(m, ModuleAvailable)
	}
	return res, nil
}

// systemModules are the module names of the system module directory
// (mod_<name>.so regular files), sorted.
func (e *Engine) systemModules(a discover.Apache) []string {
	ents, err := e.Sysroot.ReadDir(a.ModuleDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, ent := range ents {
		name, ok := strings.CutPrefix(ent.Name(), "mod_")
		if name, ok2 := strings.CutSuffix(name, ".so"); ok && ok2 && state.ModuleName(name) &&
			e.Sysroot.IsRegular(path.Join(a.ModuleDir, ent.Name())) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// ApacheModuleEnable is apache modules enable NAME: a module of the system
// module directory joins the extra modules of hte.toml, and the stack
// moves to a generation that loads it (rolled back, and hte.toml put
// back, when it fails). A module the directory lacks is
// apache_module_missing; one of the userland list or already enabled
// changes nothing.
func (e *Engine) ApacheModuleEnable(ctx context.Context, name string) (*ModuleResult, error) {
	return e.moduleChange(ctx, "apache modules enable", name, func(a discover.Apache, set *state.Settings) (bool, []string, error) {
		file := path.Join(a.ModuleDir, "mod_"+name+".so")
		if !state.ModuleName(name) || !e.Sysroot.IsRegular(file) {
			return false, nil, hteerr.Newf(hteerr.ApacheModuleMissing, "gash hte apache modules list shows the modules of "+a.ModuleDir,
				"%s has no module %q (no %s)", a.ModuleDir, name, path.Base(file))
		}
		switch {
		case slices.Contains(render.BaseModules, name):
			return false, []string{name + " is in the userland module list: every generation loads it"}, nil
		case slices.Contains(set.Modules, name):
			return false, nil, nil
		}
		set.Modules = append(set.Modules, name)
		return true, nil, nil
	})
}

// ApacheModuleDisable is apache modules disable NAME: an extra module
// leaves hte.toml and the stack moves to a generation without it. A
// module of the userland list cannot be disabled (conflicting_options);
// one that is not enabled changes nothing.
func (e *Engine) ApacheModuleDisable(ctx context.Context, name string) (*ModuleResult, error) {
	return e.moduleChange(ctx, "apache modules disable", name, func(_ discover.Apache, set *state.Settings) (bool, []string, error) {
		if slices.Contains(render.BaseModules, name) {
			return false, nil, hteerr.Newf(hteerr.ConflictingOptions, "only the modules apache modules enable added can be disabled",
				"%s is in the userland module list, which every generation loads", name)
		}
		i := slices.Index(set.Modules, name)
		if i < 0 {
			return false, nil, nil
		}
		set.Modules = slices.Delete(set.Modules, i, i+1)
		return true, nil, nil
	})
}

func (e *Engine) moduleChange(ctx context.Context, command, name string, edit func(discover.Apache, *state.Settings) (bool, []string, error)) (*ModuleResult, error) {
	v, err := e.ready(command)
	if err != nil {
		return nil, err
	}
	res, err := e.locked(v, command, "", func() (any, error) {
		set, err := e.installed(v)
		if err != nil {
			return nil, err
		}
		a, err := discover.FindApache(e.Sysroot, v.os)
		if err != nil {
			return nil, err
		}
		next := set
		next.Modules = slices.Clone(set.Modules)
		changed, warnings, err := edit(a, &next)
		if err != nil {
			return nil, err
		}
		out := &ModuleResult{Module: name, Changed: changed, Warnings: append([]string{}, warnings...)}
		if !changed {
			return out, nil
		}
		if err := v.store.SaveSettings(next); err != nil {
			return nil, err
		}
		sites, err := e.sites(v)
		if err == nil {
			var applied Applied
			if applied, err = e.apply(ctx, v, next, sites); err == nil {
				out.Applied = &applied
				return out, nil
			}
		}
		if rerr := v.store.SaveSettings(set); rerr != nil {
			return nil, hteerr.Newf(hteerr.InvalidState, "", "%v; putting hte.toml back failed: %v", err, rerr)
		}
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return res.(*ModuleResult), nil
}
