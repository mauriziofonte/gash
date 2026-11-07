// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package complete answers `gash __complete WORDS... PARTIAL` for the
// command tree of 3.5: the namespaces, the commands of a group, the
// options of a command (with their descriptions) and the values of an
// argument or an option, read live where they come from (the hosts and
// databases of ~/.gash_env, git tags and refs, Docker containers, systemd
// units, processes, listening ports). The 3.x commands of the tree (gash
// update, gash wsl info, gash localweb ...) are relayed to their cobra tree.
//
// The reply is cobra's format, which the generated bash completion reads:
// one "VALUE<TAB>DESCRIPTION" line per candidate, then ":DIRECTIVE".
// A line "!offset N" before the directive says that the words from the
// Nth word after gash on are a command line of their own (gash run --
// CMD, gash agent test -- CMD): bash completes them as that command.
package complete

import (
	"context"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The directives of cobra's protocol (cobra.ShellCompDirective).
const (
	DirError      = 1
	DirNoSpace    = 2
	DirNoFileComp = 4
	DirFilterDirs = 16
	DirKeepOrder  = 32
)

// Candidate is one completion and its description.
type Candidate struct{ Value, Desc string }

// Reply is the answer to one completion request.
type Reply struct {
	Cands     []Candidate
	Directive int
	// Offset is the index, among the words after gash, of the first word
	// of a command line that bash completes as its own command; -1 none.
	Offset int
	// Relay is set when the words belong to a 3.x command: the caller
	// asks its cobra tree with __complete Relay... instead.
	Relay []string
}

// Env is what the value providers read.
type Env struct {
	Ctx context.Context
	// Get reads the caller's environment (HOME, GASH_ENV_FILE).
	Get func(string) string
	// Delegate returns the completions the 3.x command name gives for
	// args (its last word the partial one): the delegate namespaces' own
	// words (gash wsl info, gash localweb site).
	Delegate func(name string, args []string) []Candidate
	// Run, Environ and Dir run the programs that list values (git tags,
	// Docker containers); nil Run lists none.
	Run     runner.Runner
	Environ []string
	Dir     string
}

// Words completes partial after words (the words after gash).
func Words(words []string, partial string, env Env) Reply {
	if env.Ctx == nil {
		env.Ctx = context.Background()
	}
	// The longest run of tree words at the start: a group or a command.
	var path []string
	for _, w := range words {
		next := append(slices.Clone(path), w)
		if _, ok := registry.CommandAt(next...); !ok && !registry.IsGroup(next...) {
			break
		}
		path = next
	}
	rest := words[len(path):]
	cmd, isCmd := registry.CommandAt(path...)

	if !isCmd {
		if len(path) == 0 && len(rest) > 0 {
			// gash localweb ...: a namespace that is its 3.x command alone.
			if ns, ok := registry.NamespaceOf(rest[0]); ok && ns.Delegate != "" {
				return Reply{Relay: append(append([]string{ns.Delegate}, rest[1:]...), partial), Offset: -1}
			}
		}
		if len(rest) > 0 {
			// Words the tree does not list: a delegate's own (gash localweb
			// site list), else nothing.
			if ns, ok := delegateOf(path); ok {
				return Reply{Relay: append(append([]string{ns.Delegate}, rest...), partial), Offset: -1}
			}
			return Reply{Directive: DirNoFileComp, Offset: -1}
		}
		r := Reply{Cands: children(path, partial), Directive: DirNoFileComp, Offset: -1}
		if ns, ok := delegateOf(path); ok && env.Delegate != nil {
			for _, c := range env.Delegate(ns.Delegate, []string{partial}) {
				// gash runtime which stands for gash runtime php.
				if c.Desc != "" || !registry.Forwards(ns) {
					r.Cands = merge(r.Cands, []Candidate{c})
				}
			}
		}
		if len(path) == 0 && strings.HasPrefix(partial, "-") {
			r.Cands = filter([]Candidate{{"--help", "the overview"}, {"--no-color", "no colors"}}, partial)
		}
		return r
	}
	r := command(cmd, len(path), rest, partial, env)
	if len(rest) == 0 && registry.IsGroup(path...) && !strings.HasPrefix(partial, "-") {
		// gash git log <TAB>: its subcommands, then its own arguments.
		r.Cands = merge(children(path, partial), r.Cands)
		r.Directive |= DirNoFileComp
	}
	return r
}

// delegateOf returns the namespace of path when it is one namespace with
// a 3.x tree of its own.
func delegateOf(path []string) (registry.Namespace, bool) {
	if len(path) != 1 {
		return registry.Namespace{}, false
	}
	ns, ok := registry.NamespaceOf(path[0])
	return ns, ok && ns.Delegate != ""
}

// children are the next words after path, with the description of the
// command or of the group they name.
func children(path []string, partial string) []Candidate {
	var out []Candidate
	for _, w := range registry.Children(path...) {
		if !strings.HasPrefix(w, partial) {
			continue
		}
		next := append(slices.Clone(path), w)
		desc := ""
		if c, ok := registry.CommandAt(next...); ok {
			desc = c.Short()
		} else if ns, ok := registry.NamespaceOf(w); ok && len(path) == 0 {
			desc = ns.Summary
		} else if n := len(registry.CommandsUnder(next...)); n == 1 {
			desc = "1 command"
		} else {
			desc = strconv.Itoa(n) + " commands"
		}
		out = append(out, Candidate{w, desc})
	}
	return out
}

// command completes the words after a command (at index at of the words).
func command(cmd *registry.Command, at int, rest []string, partial string, env Env) Reply {
	switch cmd.Command3x {
	case "":
	case "help":
		return helpWords(rest, partial)
	case "all":
		return Reply{Directive: DirNoFileComp, Offset: -1}
	default:
		if cmd.Command3x == "run" {
			if i := slices.Index(rest, "--"); i >= 0 {
				return Reply{Offset: at + i + 1}
			}
		}
		args := append(append(append([]string{cmd.Command3x}, cmd.Prefix...), rest...), partial)
		return Reply{Relay: args, Offset: -1}
	}
	f, ok := cmd.Function()
	if !ok {
		return Reply{Offset: -1}
	}
	opts := registry.CompletionOptions(f.Name)
	if i := slices.Index(rest, "--"); i >= 0 {
		if slices.Contains(cmd.Args, registry.ArgCommand) || strings.Contains(f.Help.Body, "-- CMD") {
			return Reply{Offset: at + i + 1}
		}
		return Reply{Offset: -1} // files
	}
	// --name=value
	if name, value, ok := strings.Cut(partial, "="); ok && strings.HasPrefix(name, "--") {
		if o, ok := optionNamed(opts, name); ok && o.Takes() {
			r := values(optionKind(cmd, o, name), o, value, env, cmd)
			for i := range r.Cands {
				r.Cands[i].Value = name + "=" + r.Cands[i].Value
			}
			return r
		}
		return Reply{Directive: DirNoFileComp, Offset: -1}
	}
	// The value of the option before.
	if n := len(rest); n > 0 {
		if o, ok := optionNamed(opts, rest[n-1]); ok && o.Values > 0 {
			return values(optionKind(cmd, o, rest[n-1]), o, partial, env, cmd)
		}
	}
	if strings.HasPrefix(partial, "-") {
		used := map[string]bool{}
		for _, w := range rest {
			used[w] = true
		}
		// "-" offers each option once, by its long name; "-x" and "--x"
		// the names that start so.
		var out []Candidate
		for _, o := range opts {
			if slices.ContainsFunc(o.Names, func(n string) bool { return used[n] }) {
				continue
			}
			if partial == "-" {
				name := o.Names[0]
				if i := slices.IndexFunc(o.Names, func(n string) bool { return strings.HasPrefix(n, "--") }); i >= 0 {
					name = o.Names[i]
				}
				out = append(out, Candidate{name, o.Desc})
				continue
			}
			for _, n := range o.Names {
				if strings.HasPrefix(n, partial) {
					out = append(out, Candidate{n, o.Desc})
				}
			}
		}
		if !used["--help"] && strings.HasPrefix("--help", partial) && !slices.ContainsFunc(out, func(c Candidate) bool { return c.Value == "--help" }) {
			out = append(out, Candidate{"--help", "this command's help"})
		}
		return Reply{Cands: out, Directive: DirNoFileComp | DirKeepOrder, Offset: -1}
	}
	// A positional argument: its index among the words that are no
	// option or option value.
	pos := 0
	for i := 0; i < len(rest); i++ {
		w := rest[i]
		if strings.HasPrefix(w, "-") && w != "-" {
			if o, ok := optionNamed(opts, w); ok && o.Values > 0 && !strings.Contains(w, "=") {
				i += o.Values
			}
			continue
		}
		pos++
	}
	if len(cmd.Args) == 0 {
		return Reply{Offset: -1} // files, as before 3.5
	}
	kind := cmd.Args[min(pos, len(cmd.Args)-1)]
	if kind == registry.ArgCommand {
		return Reply{Offset: at + len(rest)}
	}
	return values(kind, registry.Option{}, partial, env, cmd)
}

// helpWords completes gash help: a topic, a namespace and the commands
// under it, or a flag.
func helpWords(rest []string, partial string) Reply {
	if len(rest) > 0 && (rest[0] == "--search" || rest[0] == "--bash") {
		return Reply{Directive: DirNoFileComp, Offset: -1}
	}
	if len(rest) > 0 && rest[0] == "--short" {
		rest = rest[1:]
	}
	if strings.HasPrefix(partial, "-") {
		return Reply{Cands: filter([]Candidate{
			{"--list", "every command, by namespace"}, {"--search", "search every page"},
			{"--short", "the one-line description"}, {"--bash", "Bash's builtin help"}, {"--no-color", "no colors"},
		}, partial), Directive: DirNoFileComp | DirKeepOrder, Offset: -1}
	}
	r := Reply{Cands: children(rest, partial), Directive: DirNoFileComp, Offset: -1}
	if len(rest) == 0 {
		for _, t := range []Candidate{{"shortcuts", "every shortcut"}, {"shell-defaults", "the aliases that change a system command"},
			{"scripting", "helpers for your scripts"}, {"migrate", "the former flat names"}, {"topics", "the pages that are no command"}} {
			if strings.HasPrefix(t.Value, partial) {
				r.Cands = append(r.Cands, t)
			}
		}
	}
	return r
}

// optionNamed returns the option spelled name.
func optionNamed(opts []registry.Option, name string) (registry.Option, bool) {
	for _, o := range opts {
		if slices.Contains(o.Names, name) {
			return o, true
		}
	}
	return registry.Option{}, false
}

// optionKind is the kind of the value of option o spelled name: the
// command's own, else from the option's name or its value's name.
func optionKind(cmd *registry.Command, o registry.Option, name string) registry.ArgKind {
	for _, n := range o.Names {
		if k, ok := cmd.Values[n]; ok {
			return k
		}
	}
	switch name {
	case "--host":
		return registry.ArgHost
	case "--format":
		return registry.ArgFormat
	case "--runtime":
		return registry.ArgRuntime
	case "--connection":
		return registry.ArgDB
	}
	switch strings.Trim(o.Meta, `"`) {
	case "PATH", "FILE", "FILENAME", "SCRIPT":
		return registry.ArgFile
	case "DIR", "DIRECTORY", "ROOT":
		return registry.ArgDir
	case "HOST", "ALIAS":
		return registry.ArgHost
	case "CONTAINER":
		return registry.ArgContainer
	case "UNIT", "SERVICE":
		return registry.ArgUnit
	}
	if strings.Contains(o.Meta, "|") && !strings.ContainsAny(o.Meta, "<>") {
		return registry.ArgChoice
	}
	return registry.ArgAny
}

// values completes a value of the kind.
func values(kind registry.ArgKind, o registry.Option, partial string, env Env, cmd *registry.Command) Reply {
	r := Reply{Directive: DirNoFileComp, Offset: -1}
	switch kind {
	case registry.ArgFile:
		r.Directive = 0
	case registry.ArgDir:
		r.Directive = DirFilterDirs
	case registry.ArgChoice:
		list := cmd.Choices
		if o.Meta != "" {
			list = strings.Split(o.Meta, "|")
		}
		for _, c := range list {
			if strings.ToUpper(c) != c { // NAME is a value name, not a word
				r.Cands = append(r.Cands, Candidate{c, ""})
			}
		}
		r.Directive |= DirKeepOrder
	case registry.ArgFormat:
		r.Cands = []Candidate{{"json", "one JSON document"}, {"jsonl", "one JSON object per line"},
			{"tsv", "tab-separated, a header line"}, {"table", "aligned columns"}}
		r.Directive |= DirKeepOrder
	case registry.ArgAI:
		r.Cands = []Candidate{{"claude", "Anthropic Claude"}, {"gemini", "Google Gemini"}}
	case registry.ArgRuntime:
		for _, n := range []string{"php", "node", "composer", "npm", "npx"} {
			r.Cands = append(r.Cands, Candidate{n, ""})
		}
		if o.Meta != "" { // --runtime NAME:VERSION
			r.Cands = []Candidate{{"node:", "a Node.js version"}, {"php:", "a PHP version"},
				{"node:project", "the version the project pins"}, {"php:project", "the version the project pins"}}
			r.Directive |= DirNoSpace
		}
	default:
		if p := providers[kind]; p != nil {
			r.Cands = p(env)
		}
	}
	r.Cands = filter(r.Cands, partial)
	return r
}

// filter keeps the candidates that start with partial.
func filter(cands []Candidate, partial string) []Candidate {
	out := cands[:0:0]
	for _, c := range cands {
		if strings.HasPrefix(c.Value, partial) {
			out = append(out, c)
		}
	}
	return out
}

// merge appends the candidates of b that a does not hold.
func merge(a, b []Candidate) []Candidate {
	for _, c := range b {
		if !slices.ContainsFunc(a, func(x Candidate) bool { return x.Value == c.Value }) {
			a = append(a, c)
		}
	}
	return a
}

// Write writes the reply in cobra's format.
func Write(w io.Writer, r Reply, descriptions bool) {
	var b strings.Builder
	for _, c := range r.Cands {
		b.WriteString(c.Value)
		if descriptions && c.Desc != "" {
			b.WriteString("\t" + oneLine(c.Desc))
		}
		b.WriteByte('\n')
	}
	if r.Offset >= 0 {
		b.WriteString("!offset " + strconv.Itoa(r.Offset) + "\n")
	}
	b.WriteString(":" + strconv.Itoa(r.Directive) + "\n")
	_, _ = io.WriteString(w, b.String())
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s, _, _ = strings.Cut(s, "\n")
	return s
}
