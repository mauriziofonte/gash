// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package compatgen generates the Bash that 3.x ships (compat.md 1) from
// the registry and the templates in go-impl/compat: gash.sh,
// lib/compat/bootstrap.sh, the 4 core shims, the 3 module shims,
// bash_completion and bin/gash. A staged tree (Stage) also gets a shim for
// every other module, so the 2.x specs that source lib/modules/NAME.sh by
// path run against the binary.
package compatgen

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"text/template"

	"github.com/mauriziofonte/gash/compat"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/registry"
)

// File is one generated file.
type File struct {
	Path string
	Mode uint32
	Data []byte
}

// CommittedModules are the module shims of PRD 5.2 (the 3 module paths
// external harnesses source).
var CommittedModules = []string{"php", "node", "remote"}

// coreFiles maps each core shim to its template.
var coreFiles = []string{"output", "validation", "utils", "config"}

// Generate returns the files committed at the repository root (after the
// cutover), in path order. With stage, it adds a module shim for every
// module.
func Generate(stage bool) ([]File, error) {
	return generate(compat.FS, stage)
}

func generate(fsys fs.FS, stage bool) ([]File, error) {
	d := buildData()
	t, err := parse(fsys)
	if err != nil {
		return nil, err
	}
	var files []File
	add := func(path, tmpl string, mode uint32, data any) error {
		var b bytes.Buffer
		if err := t.ExecuteTemplate(&b, tmpl, data); err != nil {
			return fmt.Errorf("compatgen: %s: %w", path, err)
		}
		files = append(files, File{Path: path, Mode: mode, Data: b.Bytes()})
		return nil
	}
	steps := []struct {
		path, tmpl string
		mode       uint32
		data       any
	}{
		{"gash.sh", "gash.sh.tmpl", 0o644, d},
		{"lib/compat/bootstrap.sh", "bootstrap.sh.tmpl", 0o644, d},
		{"bash_completion", "bash_completion.tmpl", 0o644, d},
		{"bin/gash", "gash.tmpl", 0o755, d},
		{"lib/compat/legacy.sh", "legacy.sh.tmpl", 0o644, d},
	}
	for _, name := range coreFiles {
		steps = append(steps, struct {
			path, tmpl string
			mode       uint32
			data       any
		}{"lib/core/" + name + ".sh", name + ".sh.tmpl", 0o644, d})
	}
	for _, m := range d.Modules {
		if !stage && !committedModule(m.Name) {
			continue
		}
		steps = append(steps, struct {
			path, tmpl string
			mode       uint32
			data       any
		}{"lib/modules/" + m.Name + ".sh", "module.sh.tmpl", 0o644, m})
	}
	for _, s := range steps {
		if err := add(s.path, s.tmpl, s.mode, s.data); err != nil {
			return nil, err
		}
	}
	cold := map[string]coldDef{}
	for i := range files {
		if !lazyFile(files[i].Path) {
			continue
		}
		b, err := lazify(files[i].Path, files[i].Data, cold)
		if err != nil {
			return nil, err
		}
		files[i].Data = b
	}
	files = append(files, coldFiles(cold)...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func committedModule(name string) bool {
	for _, m := range CommittedModules {
		if m == name {
			return true
		}
	}
	return false
}

// LoadEnvLine starts the stub of a function whose 2.x body loads the
// settings file (registry LoadsEnv): the shim's cached __gash_load_env
// prints the file's warnings once per shell, before the first such call,
// and the binary reads the file silently (LEDGER CORE-18). An override of
// such a function starts with it too ({{% loadenv %}}).
const LoadEnvLine = `declare -F __gash_load_env >/dev/null 2>&1 && __gash_load_env`

// PrivatePrefix starts the name under which gash.sh defines the function
// of a legacy flat name (__gash_fn_files_largest): 4.0 defines the flat
// name itself only with GASH_LEGACY_NAMES=1 (lib/compat/legacy.sh).
const PrivatePrefix = "__gash_fn_"

// overridePrefix names the templates (in compat/stubs.tmpl) that replace
// the one-line stub of a function: {{% define "stub/NAME" %}}.
const overridePrefix = "stub/"

func parse(fsys fs.FS) (*template.Template, error) {
	var t *template.Template
	// stub is the function a delegated function gets: its override when
	// one is defined (a shell-mutating function keeps its parent-shell
	// part in Bash), else the one-line call into the binary.
	stub := func(name string) (string, error) {
		f, ok := registry.Lookup(name)
		if !ok || !f.HasStub() {
			return "", fmt.Errorf("stub: %q has no stub in the registry", name)
		}
		if o := t.Lookup(overridePrefix + name); o != nil {
			var b strings.Builder
			if err := o.Execute(&b, f); err != nil {
				return "", err
			}
			return strings.Trim(b.String(), "\n"), nil
		}
		if f.LoadsEnv {
			return name + "() {\n    " + LoadEnvLine + "\n    __gash_call " + name + " \"$@\"\n}", nil
		}
		return name + `() { __gash_call ` + name + ` "$@"; }`, nil
	}
	// shellStub is the stub gash.sh and the core shims define: a legacy
	// name (registry.Legacy) is defined as __gash_fn_NAME, which the gash
	// function routes to and lib/compat/legacy.sh wraps under NAME.
	shellStub := func(name string) (string, error) {
		s, err := stub(name)
		if err != nil || !registry.Legacy(name) {
			return s, err
		}
		def := name + "() {"
		i := strings.Index("\n"+s, "\n"+def)
		if i < 0 {
			return "", fmt.Errorf("stub: %q has no line starting with %q", name, def)
		}
		return s[:i] + PrivatePrefix + s[i:], nil
	}
	t, err := template.New("compat").Delims("{{%", "%}}").Funcs(template.FuncMap{
		"stub":       shellStub,
		"publicStub": stub,
		// coreStub is the stub of a core shim: sourced alone (a 2.x
		// harness, no gash.sh), the shim keeps the 2.x name of a legacy
		// function too.
		"coreStub": func(name string) (string, error) {
			s, err := shellStub(name)
			if err != nil || !registry.Legacy(name) {
				return s, err
			}
			return s + "\nif [[ -z \"${__GASH_BOOT_LOADED-}\" ]]; then\n    " + name + "() { " + PrivatePrefix + name + " \"$@\"; }\nfi", nil
		},
		"private":     func(name string) string { return PrivatePrefix + name },
		"loadenv":     func() string { return LoadEnvLine },
		"gashRoute":   GashRoute,
		"formerTable": FormerTable,
		"join":        strings.Join,
		"indent":      indent,
		// cardAliasNames is the names of the card entries as shell words,
		// for the gash stub.
		"cardAliasNames": func() string { return shellWords(registry.CardAliasNames()) },
		"cardAliasFamilies": func() string {
			var pats []string
			for _, p := range registry.CardAliasFamilies() {
				pats = append(pats, p+"[0-9]*")
			}
			return strings.Join(pats, " | ")
		},
	}).ParseFS(fsys, "*.tmpl")
	if err != nil {
		return nil, err
	}
	for _, o := range t.Templates() {
		name, ok := strings.CutPrefix(o.Name(), overridePrefix)
		if !ok {
			continue
		}
		if f, found := registry.Lookup(name); !found || !f.HasStub() {
			return nil, fmt.Errorf("compatgen: override %q: %q has no stub in the registry", o.Name(), name)
		}
	}
	return t, nil
}

// shellWords joins words as Bash words: a word with a byte outside
// [A-Za-z0-9_.,:/=+@%-], or starting with ~, is single-quoted.
func shellWords(words []string) string {
	out := make([]string, len(words))
	for i, w := range words {
		safe := w != "" && w[0] != '~'
		for j := 0; j < len(w) && safe; j++ {
			c := w[j]
			safe = 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("_.,:/=+@%-~", c) >= 0
		}
		if safe {
			out[i] = w
			continue
		}
		out[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

// indent prefixes every non-empty line of s with n spaces.
func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

// Data is what the templates render.
type Data struct {
	Common        []string
	CallVars      []NamedList
	CallSettings  []NamedList
	Modules       []Module
	ModuleAliases []registry.Alias
	// CompleteWords are the names that complete like a command of the
	// tree, with its words: former flat names, llm_*/agent_* names and
	// shortcuts.
	CompleteWords []NamedWords
	// Legacy are the flat names lib/compat/legacy.sh defines (4.0:
	// GASH_LEGACY_NAMES=1), sorted.
	Legacy []string
	// Served are the functions the per-shell server runs.
	Served []string
	// PrereqBash is the package manager detection and the install
	// commands of the prerequisites the fetch needs (internal/prereq).
	PrereqBash string
}

// NamedList is one entry of an associative array of the bootstrap.
type NamedList struct {
	Name string
	Vars []string
}

// Module is the stubs and short aliases of one 2.x module file.
type Module struct {
	Name    string
	Stubs   []string
	Aliases []registry.Alias
}

// NamedWords is a name and the canonical words it stands for.
type NamedWords struct {
	Name, Words string
}

func buildData() Data {
	d := Data{Common: registry.CommonShellVars, Served: registry.Served(), PrereqBash: prereq.BashFunctions("__gash_", 4, ""),
		Legacy: registry.LegacyNames()}
	common := map[string]bool{}
	for _, v := range registry.CommonShellVars {
		common[v] = true
	}
	var passed []string
	for _, s := range registry.Settings() {
		if s.Passed {
			passed = append(passed, s.Name)
		}
	}
	modules := map[string]*Module{}
	for i := range registry.Functions() {
		f := &registry.Functions()[i]
		if !f.HasStub() {
			continue
		}
		settings := map[string]bool{}
		for _, s := range f.Settings {
			settings[s] = true
		}
		var own []string
		for _, v := range f.ShellVars {
			if !common[v] && !settings[v] {
				own = append(own, v)
			}
		}
		if len(own) > 0 {
			d.CallVars = append(d.CallVars, NamedList{f.Name, own})
		}
		if len(f.Settings) > 0 {
			d.CallSettings = append(d.CallSettings, NamedList{f.Name, f.Settings})
		}
		if m, ok := strings.CutPrefix(f.StubFile, "lib/modules/"); ok {
			m = strings.TrimSuffix(m, ".sh")
			if modules[m] == nil {
				modules[m] = &Module{Name: m}
			}
			modules[m].Stubs = append(modules[m].Stubs, f.Name)
		}
	}
	// gash init bash reads the interactive settings, environment first.
	d.CallSettings = append(d.CallSettings, NamedList{"__init", passed})

	for _, a := range registry.Aliases() {
		if a.Pack != "" || a.Function == "" {
			continue
		}
		f, ok := registry.Lookup(a.Function)
		if !ok {
			continue
		}
		a.Definition = shortcutDefinition(a)
		d.ModuleAliases = append(d.ModuleAliases, a)
		m := strings.TrimSuffix(strings.TrimPrefix(f.StubFile, "lib/modules/"), ".sh")
		if modules[m] != nil {
			modules[m].Aliases = append(modules[m].Aliases, a)
		}
	}
	for _, name := range registry.Modules() {
		if m := modules[name]; m != nil {
			d.Modules = append(d.Modules, *m)
		}
	}
	// Modules whose name is not a registry module name (the llm parts map
	// to lib/modules/llm.sh, already covered); keep any left, sorted.
	var extra []string
	for name := range modules {
		found := false
		for _, m := range d.Modules {
			found = found || m.Name == name
		}
		if !found {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		d.Modules = append(d.Modules, *modules[name])
	}

	d.CompleteWords = completeWords()
	return d
}

// completeWords are the names of __GASH_COMPLETE_WORDS, sorted.
func completeWords() []NamedWords {
	seen := map[string]bool{}
	var out []NamedWords
	add := func(name, words string) {
		if name != "" && name != "gash" && !seen[name] {
			seen[name] = true
			out = append(out, NamedWords{name, words})
		}
	}
	for i := range registry.Tree() {
		c := &registry.Tree()[i]
		if c.Hidden {
			continue
		}
		for _, f := range c.FormerNames() {
			add(f, c.Words())
		}
		if c.Permanent() {
			add(c.Handler, c.Words())
		}
		add(c.Shortcut, c.Words())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
