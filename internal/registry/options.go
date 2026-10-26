// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"regexp"
	"strings"
	"sync"
)

// Option is one option of a function as its registered help lists it:
// the names that spell it (-C, --context) and the words of value it takes
// (Values; Optional when the help writes its one value in brackets).
type Option struct {
	Names    []string
	Values   int
	Optional bool
}

// Takes reports whether the option takes a value, required or optional.
func (o Option) Takes() bool { return o.Values > 0 || o.Optional }

var (
	optionsOnce sync.Once
	optionsOf   map[string][]Option

	optName  = regexp.MustCompile(`^(-[A-Za-z0-9]|--[a-z][a-z0-9-]*)$`)
	metaUp   = regexp.MustCompile(`^[A-Z0-9_.:,=|<>/*+-]*[A-Z][A-Z0-9_.:,=|<>/*+-]*$`)
	metaAlt  = regexp.MustCompile(`^[A-Za-z0-9_-]+(\|[A-Za-z0-9_-]+)+$`)
	metaList = regexp.MustCompile(`^[a-z0-9]+(,[a-z0-9]+)+$`)
)

// Options returns the options that the help of the function name lists,
// in help order: the lines of its OPTIONS section, and the condition
// lines of its USAGE (llm_wait). nil when it has no help.
func Options(name string) []Option {
	optionsOnce.Do(func() {
		optionsOf = map[string][]Option{}
		for i := range functions {
			if f := &functions[i]; f.HasHelp {
				optionsOf[f.Name] = parseOptions(f.Help.Body)
			}
		}
	})
	return optionsOf[name]
}

// OptionOf returns the option of the function name spelled flag.
func OptionOf(name, flag string) (Option, bool) {
	for _, o := range Options(name) {
		for _, n := range o.Names {
			if n == flag {
				return o, true
			}
		}
	}
	return Option{}, false
}

// parseOptions reads the option lines of a help body: an OPTIONS line is
// indented by two spaces, a USAGE condition line by four, and either
// starts with its spec ("-C, --context N"), which ends at a run of two
// spaces or at the first word that is no value name.
func parseOptions(body string) []Option {
	var out []Option
	section := ""
	for _, line := range strings.Split(body, "\n") {
		if line != "" && line[0] != ' ' && strings.ToUpper(line) == line {
			section = line
			continue
		}
		spec := strings.TrimLeft(line, " ")
		indent := len(line) - len(spec)
		if !(section == "OPTIONS" && indent == 2 || section == "USAGE" && indent == 4) || !strings.HasPrefix(spec, "-") {
			continue
		}
		if i := strings.Index(spec, "  "); i >= 0 {
			spec = spec[:i]
		}
		out = append(out, specOptions(spec)...)
	}
	return out
}

// specOptions reads one spec. A name followed by a comma and no value
// names the same option as the next name (-C, --context N); [--code N]
// is an option of its own, [REMOTE] an optional value.
func specOptions(spec string) []Option {
	var out []Option
	alias := []string(nil)
	cur := -1
	for _, w := range specWords(spec) {
		comma := strings.HasSuffix(w, ",")
		w = strings.TrimSuffix(w, ",")
		optional := strings.HasPrefix(w, "[") && strings.HasSuffix(w, "]") && !strings.HasPrefix(w, "[-")
		w = strings.Trim(w, "[]")
		switch {
		case optName.MatchString(w):
			if len(alias) == 1 && long(alias[0]) && long(w) {
				// --absolute, --relative: two options.
				out = append(out, Option{Names: alias})
				alias = nil
			}
			out = append(out, Option{Names: append(alias, w)})
			alias, cur = nil, len(out)-1
			if comma {
				// Spelled again by the next name, unless a value follows.
				alias, cur = out[cur].Names, -1
				out = out[:len(out)-1]
			}
		case cur >= 0 && isMetavar(w):
			if optional {
				out[cur].Optional = true
			} else {
				out[cur].Values++
			}
		default:
			if alias != nil {
				out = append(out, Option{Names: alias})
			}
			return out
		}
	}
	if alias != nil {
		out = append(out, Option{Names: alias})
	}
	return out
}

func long(name string) bool { return strings.HasPrefix(name, "--") }

// specWords splits a spec at spaces, a "quoted value" kept as one word.
func specWords(spec string) []string {
	var out []string
	for _, w := range strings.Fields(spec) {
		if n := len(out); n > 0 && strings.HasPrefix(out[n-1], `"`) && !strings.HasSuffix(out[n-1], `"`) {
			out[n-1] += " " + w
			continue
		}
		out = append(out, w)
	}
	return out
}

// isMetavar reports whether w names a value: N, PATH..., A:B, "N <e>",
// f|d, h1,h2.
func isMetavar(w string) bool {
	if strings.HasPrefix(w, `"`) {
		return true
	}
	w = strings.TrimSuffix(w, "...")
	return metaUp.MatchString(w) || metaAlt.MatchString(w) || metaList.MatchString(w)
}

// scanners are the filesystem scanners of the agent catalog (AGENTS.md
// 3.2), which take --flag=value as the llm_* and agent_* functions do.
var scanners = map[string]bool{
	"files_largest": true, "dirs_largest": true, "dirs_find_large": true, "dirs_list_empty": true, "tree_stats": true,
}

// EqualsForm reports whether the function name takes --flag=value for a
// value flag of its help (3.4.2): the llm_* and agent_* functions and the
// filesystem scanners.
func EqualsForm(name string) bool {
	return strings.HasPrefix(name, "llm_") || strings.HasPrefix(name, "agent_") || scanners[name]
}

// OptionsHint is the hint of an unknown_option of the function name: the
// options its help lists (3.4.2: hand-written lists fell behind the help).
func OptionsHint(name string) string {
	var names []string
	for _, o := range Options(name) {
		names = append(names, strings.Join(o.Names, "/"))
	}
	if len(names) == 0 {
		return name + " takes no options"
	}
	return "Options: " + strings.Join(names, ", ") + " (" + name + " --help)"
}
