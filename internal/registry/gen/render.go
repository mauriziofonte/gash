// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
)

// stubFile maps a 2.x definition file to the file a caller sources.
func stubFile(file string) string {
	dir, base := path.Split(file)
	switch dir {
	case "lib/modules/llm/", "lib/modules/ai/":
		return strings.TrimSuffix(dir, "/") + ".sh"
	}
	return dir + base
}

// moduleOf derives a module name from a file when no help registers one.
func moduleOf(file string) string {
	dir, base := path.Split(file)
	base = strings.TrimSuffix(base, ".sh")
	switch dir {
	case "lib/modules/llm/":
		return "llm"
	case "lib/modules/ai/":
		return "ai"
	}
	return base
}

func (c *catalog) build() ([]registry.Function, error) {
	m := c.matrix
	passedEnv := m.passedEnv()
	passedSettings := m.passedSettings()
	allEnv := m.allEnv()
	settingOrder := map[string]int{}
	for i, s := range c.settings {
		settingOrder[s] = i
	}
	common := map[string]bool{}
	for _, v := range registry.CommonShellVars {
		common[v] = true
	}
	completions := map[string]completion{}
	for _, cm := range c.completions {
		for _, n := range cm.names {
			completions[n] = cm
		}
	}

	var names []string
	for name := range c.defs {
		if !strings.HasPrefix(name, "_") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var errs []string
	var out []registry.Function
	for _, name := range names {
		defs := c.defs[name]
		def := defs[0]
		for _, d := range defs {
			if strings.HasPrefix(d.file, "lib/core/") || strings.HasPrefix(d.file, "lib/modules/") {
				def = d
				break
			}
		}
		row, ok := m.functions[name]
		if !ok {
			errs = append(errs, "2.x function without a MATRIX row: "+name)
			continue
		}
		f := registry.Function{
			Name:     name,
			File:     def.file,
			StubFile: stubFile(def.file),
			Class:    registry.Class(row.cells[2]),
			Decision: row.cells[3],
			Headless: strings.HasPrefix(def.file, "lib/core/") || strings.HasPrefix(def.file, "lib/modules/"),
		}
		switch gocmd := row.cells[5]; {
		case strings.HasPrefix(gocmd, "shim"):
			f.Impl = registry.ImplShim
		case strings.HasPrefix(gocmd, "gash init bash"):
			f.Impl = registry.ImplInit
		default:
			f.Impl = registry.ImplBinary
		}
		if h, ok := c.helps[name]; ok {
			f.HasHelp = true
			f.Help = registry.Help{Body: h.body, Short: h.short, Aliases: h.aliases, Module: h.module, SeeAlso: h.seeAlso}
			f.Module = h.module
		}
		if f.Module == "" {
			f.Module = moduleOf(def.file)
		}
		for _, d := range defs {
			if d.fallback != nil {
				f.Fallback = &registry.Usage{Program: d.fallback.program, Usage: d.fallback.usage, Description: d.fallback.desc}
				break
			}
		}
		for _, a := range c.aliases {
			if a.pack == "" && a.function == name {
				f.Aliases = append(f.Aliases, a.name)
			}
		}
		f.LoadsEnv = c.reaches(name, "__gash_load_env")
		read := c.closure(name)
		if read[allSettings] {
			for _, s := range c.settings {
				read[s] = true
			}
		}
		for v := range read {
			switch {
			case passedSettings[v]:
				f.Settings = append(f.Settings, v)
			case passedEnv[v] && !common[v]:
				f.ShellVars = append(f.ShellVars, v)
			}
			if allEnv[v] && !strings.HasPrefix(v, "__GASH_") {
				f.Env = append(f.Env, v)
			}
		}
		sort.Slice(f.Settings, func(i, j int) bool { return settingOrder[f.Settings[i]] < settingOrder[f.Settings[j]] })
		sort.Strings(f.ShellVars)
		sort.Strings(f.Env)
		if cm, ok := completions[name]; ok {
			f.Completion = cm.body
			f.CompletionComment = cm.comment
		}
		out = append(out, f)
	}
	for name := range m.functions {
		if _, ok := c.defs[name]; !ok {
			errs = append(errs, "MATRIX function row without a 2.x function: "+name)
		}
	}
	for name := range c.helps {
		if _, ok := c.defs[name]; !ok {
			errs = append(errs, "help registered for an unknown function: "+name)
		}
	}
	topicOf := map[string]string{}
	packTopic := map[string]string{}
	for _, t := range c.topics {
		for _, mod := range strings.Fields(t[1]) {
			topicOf[mod] = t[0]
		}
		for _, p := range strings.Fields(t[2]) {
			packTopic[p] = t[0]
		}
	}
	for i := range out {
		f := &out[i]
		if t, ok := topicOf[f.Module]; ok {
			f.Topic = t
		} else if strings.HasPrefix(f.File, "lib/aliases/") {
			f.Topic = packTopic[moduleOf(f.File)]
		}
	}
	sort.Strings(errs)
	if len(errs) > 0 {
		return nil, fmt.Errorf("MATRIX cross-check failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return out, nil
}

// closure returns every variable and identifier argument read by name and
// by the functions it calls, transitively.
func (c *catalog) closure(name string) map[string]bool {
	read := map[string]bool{}
	seen := map[string]bool{}
	var visit func(n string)
	visit = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, d := range c.defs[n] {
			for v := range d.vars {
				read[v] = true
			}
			for w := range d.words {
				read[w] = true
			}
			for callee := range d.calls {
				if _, ok := c.defs[callee]; ok {
					visit(callee)
				}
			}
		}
	}
	visit(name)
	return read
}

// reaches reports whether name's body calls target, directly or through
// the functions it calls.
func (c *catalog) reaches(name, target string) bool {
	seen := map[string]bool{}
	var visit func(n string) bool
	visit = func(n string) bool {
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, d := range c.defs[n] {
			for callee := range d.calls {
				if callee == target {
					return true
				}
				if _, ok := c.defs[callee]; ok && visit(callee) {
					return true
				}
			}
		}
		return false
	}
	return visit(name)
}

func (c *catalog) render() ([]byte, error) {
	fns, err := c.build()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>\n\n")
	b.WriteString("// Code generated by internal/registry/gen from the Gash 2.x sources; DO NOT EDIT.\n\n")
	b.WriteString("package registry\n\n")

	b.WriteString("var baseFunctions = []Function{\n")
	for _, f := range fns {
		b.WriteString("\t{\n")
		field(&b, "Name", q(f.Name))
		field(&b, "Module", q(f.Module))
		if f.Topic != "" {
			field(&b, "Topic", q(f.Topic))
		}
		field(&b, "File", q(f.File))
		field(&b, "StubFile", q(f.StubFile))
		field(&b, "Class", fmt.Sprintf("Class(%s)", q(string(f.Class))))
		field(&b, "Decision", q(f.Decision))
		field(&b, "Impl", fmt.Sprintf("Impl(%s)", q(string(f.Impl))))
		field(&b, "Headless", strconv.FormatBool(f.Headless))
		if len(f.Aliases) > 0 {
			field(&b, "Aliases", strs(f.Aliases))
		}
		if f.HasHelp {
			field(&b, "HasHelp", "true")
			fmt.Fprintf(&b, "\t\tHelp: Help{\n\t\t\tShort: %s,\n\t\t\tModule: %s,\n", q(f.Help.Short), q(f.Help.Module))
			if f.Help.Aliases != "" {
				fmt.Fprintf(&b, "\t\t\tAliases: %s,\n", q(f.Help.Aliases))
			}
			if f.Help.SeeAlso != "" {
				fmt.Fprintf(&b, "\t\t\tSeeAlso: %s,\n", q(f.Help.SeeAlso))
			}
			fmt.Fprintf(&b, "\t\t\tBody: %s,\n\t\t},\n", multiline(f.Help.Body))
		}
		if f.Fallback != nil {
			fmt.Fprintf(&b, "\t\tFallback: &Usage{Program: %s, Usage: %s, Description: %s},\n",
				q(f.Fallback.Program), q(f.Fallback.Usage), q(f.Fallback.Description))
		}
		if len(f.Settings) > 0 {
			field(&b, "Settings", strs(f.Settings))
		}
		if f.LoadsEnv {
			field(&b, "LoadsEnv", "true")
		}
		if len(f.ShellVars) > 0 {
			field(&b, "ShellVars", strs(f.ShellVars))
		}
		if len(f.Env) > 0 {
			field(&b, "Env", strs(f.Env))
		}
		if f.Completion != "" {
			if f.CompletionComment != "" {
				field(&b, "CompletionComment", q(f.CompletionComment))
			}
			field(&b, "Completion", multiline(f.Completion))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")

	b.WriteString("var aliases = []Alias{\n")
	for _, a := range c.aliases {
		fmt.Fprintf(&b, "\t{Name: %s, Definition: %s", q(a.name), q(a.definition))
		if a.function != "" {
			fmt.Fprintf(&b, ", Function: %s", q(a.function))
		}
		if a.pack != "" {
			fmt.Fprintf(&b, ", Pack: %s", q(a.pack))
		}
		if a.condition != "" {
			fmt.Fprintf(&b, ", Condition: %s", q(a.condition))
		}
		fmt.Fprintf(&b, ", Source: %s},\n", q(a.source))
	}
	b.WriteString("}\n\n")

	b.WriteString("var packs = []Pack{\n")
	for _, p := range c.packOrder() {
		fmt.Fprintf(&b, "\t{Name: %s, File: %s},\n", q(p), q("lib/aliases/"+p+".sh"))
	}
	b.WriteString("}\n\n")

	headless := map[string]bool{}
	for _, s := range c.headlessSet {
		headless[s] = true
	}
	fileOnly := map[string]bool{}
	for _, r := range c.matrix.settings {
		if len(r.cells) > 4 && strings.Contains(r.cells[4], "file only") {
			for _, n := range names(r.cells[0]) {
				fileOnly[n] = true
			}
		}
	}
	passed := c.matrix.passedSettings()
	b.WriteString("var settings = []Setting{\n")
	for _, s := range c.settings {
		fmt.Fprintf(&b, "\t{Name: %s, Headless: %t, FileOnly: %t, Passed: %t},\n", q(s), headless[s], fileOnly[s], passed[s])
	}
	b.WriteString("}\n\n")

	b.WriteString("var topics = []Topic{\n")
	for _, t := range c.topics {
		fmt.Fprintf(&b, "\t{Name: %s, Modules: %s, AliasFiles: %s, Description: %s},\n",
			q(t[0]), strs(strings.Fields(t[1])), strs(strings.Fields(t[2])), q(t[3]))
	}
	b.WriteString("}\n\n")

	b.WriteString("var cardAliases = map[string][]CardAlias{\n")
	files := make([]string, 0, len(c.cardAliases))
	for f := range c.cardAliases {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		fmt.Fprintf(&b, "\t%s: {\n", q(f))
		for _, a := range c.cardAliases[f] {
			fmt.Fprintf(&b, "\t\t{Name: %s, Value: %s},\n", q(a.name), q(a.value))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")

	return format.Source(b.Bytes())
}

// packOrder returns the alias packs in the order of lib/aliases/_loader.sh.
func (c *catalog) packOrder() []string { return c.packs }

func field(b *bytes.Buffer, name, value string) {
	fmt.Fprintf(b, "\t\t%s: %s,\n", name, value)
}

func q(s string) string { return strconv.Quote(s) }

func strs(list []string) string {
	if len(list) == 0 {
		return "nil"
	}
	parts := make([]string, len(list))
	for i, s := range list {
		parts[i] = q(s)
	}
	return "[]string{" + strings.Join(parts, ", ") + "}"
}

// multiline renders a long string as a concatenation of one quoted line
// each, so the generated file stays readable and diffs stay small.
func multiline(s string) string {
	if !strings.Contains(s, "\n") {
		return q(s)
	}
	lines := strings.SplitAfter(s, "\n")
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			parts = append(parts, q(l))
		}
	}
	return "\"\" +\n\t\t\t" + strings.Join(parts, " +\n\t\t\t")
}
