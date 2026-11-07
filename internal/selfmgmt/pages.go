// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"github.com/mauriziofonte/gash/internal/buildinfo"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// The pages of 3.5: `gash` (the overview of the namespaces), `gash NS`
// (the commands of a namespace, its shortcuts and the agent tools of its
// domain), `gash NS GROUP` (gash git tag) and `gash all`.

// shortcutPacks are the alias packs that are typing shortcuts; the
// others change a system command (rm -I, ls --color) and are listed as
// shell defaults.
var shortcutPacks = map[string]bool{"git": true, "docker": true, "navigation": true, "php": true}

// pageWords splits the card arguments into the words and --no-color.
func pageWords(args []string) (words []string, noColor, wantsHelp bool) {
	for _, a := range args {
		switch a {
		case "--no-color":
			noColor = true
		case "-h", "--help":
			wantsHelp = true
		default:
			words = append(words, a)
		}
	}
	return words, noColor, wantsHelp
}

func isNamespace(word string) bool {
	_, ok := registry.NamespaceOf(word)
	return ok
}

// pages renders the page the words select, or reports an unknown command.
func (m *Module) pages(c *invoke.Call, k cardWriter, words []string) int {
	switch {
	case len(words) == 0:
		k.overview(c)
		return 0
	case words[0] == "all":
		k.overview(c)
		for _, ns := range registry.Namespaces() {
			k.namespacePage(c, ns, nil)
		}
		k.selfPage()
		k.nl()
		return 0
	case words[0] == "gash" || words[0] == "management" || len(words) == 1 && registry.IsGroup(words[0]) && !isNamespace(words[0]):
		// gash config: a group of Gash's own commands.
		k.selfPage()
		k.nl()
		return 0
	}
	if ns, ok := registry.NamespaceOfTopic(words[0]); ok {
		group := append([]string{ns.Name}, words[1:]...)
		if len(words) == 1 || registry.IsGroup(group...) {
			k.namespacePage(c, ns, group[1:])
			k.nl()
			return 0
		}
	}
	return unknownCommand(c, words, false)
}

// overview is `gash`: the namespaces by group, then where to go next.
func (k cardWriter) overview(c *invoke.Call) {
	s, w := k.s, k.w
	b, mu, r := "", "", ""
	if s.Color {
		b, mu, r = s.P.Bold, s.P.Muted, s.P.Off
	}
	k.nl()
	version := c.Env.Get("GASH_VERSION")
	if version == "" {
		version = buildinfo.Version
	}
	write(w, "  "+b+"Gash "+version+r+" "+mu+s.G.Dot+" Bash for humans and AI agents"+r+"\n")
	for _, g := range registry.Groups {
		var rows []string
		for _, ns := range registry.Namespaces() {
			if ns.Group != g {
				continue
			}
			rows = append(rows, ns.Name+"\t"+ns.Summary)
		}
		if g == registry.GroupAgents {
			if run, ok := registry.CommandAt("run"); ok {
				rows = append(rows, "run\t"+run.Short())
			}
		}
		if g == registry.GroupGash {
			var words []string
			for _, cmd := range registry.TopCommands() {
				if cmd.Path[0] != "run" && cmd.Path[0] != "all" {
					words = append(words, cmd.Path[0])
				}
			}
			words = append(words, "config")
			k.nl()
			s.Heading(w, "--indent", "2", string(g))
			write(w, "    "+strings.Join(words, "  ")+"\n")
			continue
		}
		k.nl()
		s.Heading(w, "--indent", "2", string(g))
		s.Table(w, rows, "--no-header", "--indent", "4", "--fit", "--color", "1:accent")
	}
	k.nl()
	s.Table(w, []string{
		"gash NAMESPACE\tits commands and shortcuts, e.g. gash git",
		"gash help NAME\ta command, a shortcut or a former name, e.g. gash help flf",
		"gash help --search WORD\tsearch every page",
		"gash help shortcuts\tevery shortcut, the short aliases of the commands",
		"gash all\tevery namespace page",
		"TAB\tcompletes namespaces, commands, options and their values",
	}, "--no-header", "--indent", "2", "--color", "1:accent")
	k.nl()
}

// namespacePage is `gash NS` (group: the words after NS, gash git tag).
func (k cardWriter) namespacePage(c *invoke.Call, ns registry.Namespace, group []string) {
	s, w := k.s, k.w
	words := append([]string{ns.Name}, group...)
	b, mu, r := "", "", ""
	if s.Color {
		b, mu, r = s.P.Bold, s.P.Muted, s.P.Off
	}
	k.nl()
	write(w, "  "+b+"gash "+strings.Join(words, " ")+r+" "+mu+s.G.Dot+" "+ns.Summary+r+"\n")

	rows := []string{"COMMAND\tUSAGE\tSHORTCUT\tDESCRIPTION"}
	cmds := registry.CommandsUnder(words...)
	// The usage column takes at most half of what the name and shortcut
	// columns leave (34 at most): the description keeps the other half.
	namew, scw := len("COMMAND"), len("SHORTCUT")
	for _, cmd := range cmds {
		namew = max(namew, ui.Len(strings.Join(cmd.Path[1:], " ")))
		scw = max(scw, ui.Len(cmd.Shortcut))
	}
	usagew := min(34, max(16, (s.Width-2-namew-scw-6)/2))
	for _, cmd := range cmds {
		usage := commandUsage(cmd)
		if ui.Len(usage) > usagew {
			usage = ui.Sub(usage, 0, usagew-1) + s.G.Ellipsis
		}
		rows = append(rows, strings.Join(cmd.Path[1:], " ")+"\t"+usage+"\t"+cmd.Shortcut+"\t"+cmd.Short())
	}
	if len(group) == 0 && ns.Delegate != "" {
		// A tree command that forwards the delegate's own words (gash
		// runtime which php 8.4 is gash runtime php 8.4) stands for the
		// words the delegate lists without a description.
		forwards := registry.Forwards(ns)
		for _, d := range delegateCommands(c, ns.Delegate) {
			if forwards && d[1] == "" {
				continue
			}
			if _, taken := registry.CommandAt(ns.Name, d[0]); !taken && !registry.IsGroup(ns.Name, d[0]) {
				rows = append(rows, d[0]+"\t\t\t"+d[1])
			}
		}
	}
	k.nl()
	s.Table(w, rows, "--indent", "2", "--fit", "--color", "1:accent", "--color", "2:muted", "--color", "3:accent")

	if len(group) == 0 {
		k.packs(ns)
		if tools := agentTools(ns.Name); len(tools) > 0 {
			k.nl()
			s.Heading(w, "--indent", "2", "FOR AGENTS")
			for _, l := range ui.Wrap(max(s.Width-4, 40), strings.Join(tools, " "+s.G.Dot+" ")) {
				write(w, "  "+mu+l+r+"\n")
			}
		}
	}
	k.nl()
	guide := ""
	if ns.Area != "" {
		guide = " " + s.G.Dot + " Guide: docs/" + ns.Area + "/README.md"
	}
	write(w, "  "+mu+"Details: gash "+strings.Join(words, " ")+" COMMAND --help"+guide+r+"\n")
}

// packs writes the shortcut grid of a namespace's alias packs, then its
// shell defaults.
func (k cardWriter) packs(ns registry.Namespace) {
	var shortcuts, defaults []string
	var shortcutFiles, defaultFiles []string
	for _, p := range ns.Packs {
		text := strings.TrimSuffix(k.aliasLines(p), "\n")
		if text == "" {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			name, value := readTab(line)
			if name == "" {
				continue
			}
			if _, isCmd := registry.CommandOfShortcut(name); isCmd {
				continue // a shortcut of a command: in the table above
			}
			if _, isFn := registry.Lookup(name); isFn {
				continue
			}
			if shortcutPacks[p] {
				shortcuts = append(shortcuts, name+"\t"+value)
			} else {
				defaults = append(defaults, name+"\t"+value)
			}
		}
		if shortcutPacks[p] {
			shortcutFiles = append(shortcutFiles, p)
		} else {
			defaultFiles = append(defaultFiles, p)
		}
	}
	if len(shortcuts) > 0 {
		k.nl()
		k.s.Heading(k.w, "--indent", "2", "--note", "packs "+strings.Join(shortcutFiles, ", ")+" "+k.s.G.Dot+" GASH_SHORTCUTS", "SHORTCUTS")
		k.grid(shortcuts, shortcutFiles)
	}
	if len(defaults) > 0 {
		k.nl()
		k.s.Heading(k.w, "--indent", "2", "--note", "GASH_SAFETY_ALIASES, GASH_REPLACE_TOOLS "+k.s.G.Dot+" gash help shell-defaults", "SHELL DEFAULTS")
		k.grid(defaults, defaultFiles)
	}
}

// selfPage lists the commands of Gash itself.
func (k cardWriter) selfPage() {
	s, w := k.s, k.w
	b, mu, r := "", "", ""
	if s.Color {
		b, mu, r = s.P.Bold, s.P.Muted, s.P.Off
	}
	k.nl()
	write(w, "  "+b+"gash"+r+" "+mu+s.G.Dot+" Gash itself: help, doctor, settings, updates"+r+"\n")
	rows := []string{"COMMAND\tSHORTCUT\tDESCRIPTION"}
	for i := range registry.Tree() {
		cmd := &registry.Tree()[i]
		if cmd.Hidden || len(cmd.Path) > 1 && cmd.Path[0] != "config" {
			continue
		}
		rows = append(rows, cmd.Words()+"\t"+cmd.Shortcut+"\t"+cmd.Short())
	}
	k.nl()
	s.Table(w, rows, "--indent", "2", "--fit", "--color", "1:accent", "--color", "2:accent")
}

// agentTools are the llm and agent commands of a namespace's domain
// (gash git: gash llm git-status ...).
func agentTools(ns string) []string {
	if ns == "llm" || ns == "agent" {
		return nil
	}
	var out []string
	for _, cmd := range registry.Tree() {
		if !cmd.Permanent() {
			continue
		}
		rest := cmd.Handler[strings.IndexByte(cmd.Handler, '_')+1:]
		if strings.HasPrefix(rest, ns+"_") {
			out = append(out, cmd.Name())
		}
	}
	return out
}

// commandUsage is the first USAGE line of a command's help, without the
// command's name.
func commandUsage(cmd *registry.Command) string {
	if cmd.Usage != "" {
		return cmd.Usage
	}
	f, ok := cmd.Function()
	if !ok || !f.HasHelp {
		return ""
	}
	h := registry.CanonicalHelp(f)
	_, rest, ok := strings.Cut(h.Body, "USAGE\n")
	if !ok {
		return ""
	}
	line, _, _ := strings.Cut(rest, "\n")
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(strings.TrimPrefix(line, cmd.Name()), " ")
	return line
}

// delegateCommands asks a 3.x command for its first words (its cobra
// completion): name and description.
func delegateCommands(c *invoke.Call, word string) [][2]string {
	h, ok := c.Handlers.LookupCommand(word)
	if !ok {
		return nil
	}
	var out bytes.Buffer
	h(&invoke.Call{
		Ctx: c.Ctx, Name: word, Args: []string{cobra.ShellCompRequestCmd, ""}, Env: c.Env,
		Stdin: c.Stdin, Stdout: &out, Stderr: io.Discard, Term: c.Term, Handlers: c.Handlers,
	})
	var list [][2]string
	for _, line := range strings.Split(out.String(), "\n") {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		name, desc, _ := strings.Cut(line, "\t")
		if strings.HasPrefix(name, "-") || name == "help" || name == "completion" {
			continue
		}
		list = append(list, [2]string{name, desc})
	}
	return list
}

// unknownCommand reports words that name no page and no command, with
// the closest names: a line for a person at a terminal, the JSON envelope
// otherwise.
func unknownCommand(c *invoke.Call, words []string, forHelp bool) int {
	typed := "gash " + strings.Join(words, " ")
	hint := "Run: gash (namespaces) or gash help --search WORD"
	if near := registry.Suggest(strings.Join(words, " ")); len(near) > 0 {
		hint = "Did you mean " + strings.Join(near, " or ") + "?"
	}
	if ns, ok := registry.NamespaceOfTopic(words[0]); ok {
		hint += " All " + ns.Name + " commands: gash " + ns.Name
	}
	if forHelp {
		// gash help NAME: a page that does not exist, as gash_help said it.
		p := ui.NewPrinter(c.Stderr, c.Term, false)
		p.Error("No help found for '" + strings.Join(words, " ") + "'.")
		p.Hint(hint)
		return 1
	}
	if c.Term.StderrTTY() {
		p := ui.NewPrinter(c.Stderr, c.Term, false)
		p.Error("unknown command: " + typed)
		p.Hint(hint)
		return 1
	}
	_ = contract.New("unknown_command", "Unknown command: "+typed, hint).Write(c.Stderr)
	return 1
}

func write(w io.Writer, s string) { _, _ = io.WriteString(w, s) }
