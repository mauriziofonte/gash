// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// HelpCommand is `gash help [NAME...|TOPIC|--search WORD|--list|--short
// NAME] [--no-color]` and the former gash_help: the page of any name a
// person may type (canonical, shortcut, former flat name, llm_*), a
// namespace page, a topic page, a search of every page.
func (m *Module) HelpCommand(c *invoke.Call) int {
	words, noColor, wantsHelp := pageWords(c.Args)
	color := c.Term.WantColors(noColor)
	s := ui.StyleOf(c.Term, !color)
	w := c.Stdout
	k := cardOf(s, w, liveAliases(c.Env.Get(CardAliasesVar)), c.Env.Get(CardLiveVar) == "1")
	p := s.Printer(c.Stderr)

	if wantsHelp && len(words) == 0 {
		if f, ok := registry.Lookup("gash_help"); ok && help.Display(w, color, f) {
			return 0
		}
	}
	if len(words) == 0 {
		k.overview(c)
		return 0
	}
	switch words[0] {
	case "--search":
		if len(words) < 2 || words[1] == "" {
			p.Usage("gash help --search WORD")
			return 1
		}
		return k.search(strings.Join(words[1:], " "))
	case "--list":
		k.list()
		return 0
	case "--short":
		if len(words) < 2 {
			p.Usage("gash help --short NAME")
			return 1
		}
		if cmd, ok := registry.CanonicalName(strings.Join(words[1:], " ")); ok {
			write(w, cmd.Short()+"\n")
			return 0
		}
		if f, ok := registry.Lookup(words[1]); ok && f.Help.Short != "" {
			write(w, f.Help.Short+"\n")
			return 0
		}
		p.Error("No help found for '" + strings.Join(words[1:], " ") + "'.")
		return 1
	case "--bash":
		p.Error("gash help --bash is Bash's builtin help: run it from a shell that sourced gash.sh.")
		return 1
	}
	if topic, ok := topicPages[words[0]]; ok && len(words) == 1 {
		topic(k)
		k.nl()
		return 0
	}
	name := strings.Join(words, " ")
	if cmd, ok := registry.CanonicalName(name); ok {
		if f, ok := cmd.Function(); ok && help.Display(w, color, f) {
			return 0
		}
		// A command without a help page of its own (a 3.x command, a
		// git log view): its line, and where its options are.
		k.nl()
		write(w, "  "+cmd.Name()+"\n\n  "+cmd.Short()+"\n\n  Options: "+cmd.Name()+" --help\n\n")
		return 0
	}
	if f, ok := registry.Lookup(name); ok && help.Display(w, color, f) {
		return 0 // a script helper (needs_help, path_add)
	}
	if ns, ok := registry.NamespaceOfTopic(words[0]); ok && (len(words) == 1 || registry.IsGroup(append([]string{ns.Name}, words[1:]...)...)) {
		return m.pages(c, k, words)
	}
	if words[0] == "all" || words[0] == "gash" || words[0] == "management" {
		return m.pages(c, k, words)
	}
	return unknownCommand(c, words, true)
}

// topicPages are the pages that are not commands.
var topicPages = map[string]func(k cardWriter){
	"shortcuts":      (cardWriter).shortcutsPage,
	"shell-defaults": (cardWriter).shellDefaultsPage,
	"scripting":      (cardWriter).scriptingPage,
	"migrate":        (cardWriter).migratePage,
	"topics":         (cardWriter).topicsPage,
}

func (k cardWriter) title(text, note string) {
	b, mu, r := "", "", ""
	if k.s.Color {
		b, mu, r = k.s.P.Bold, k.s.P.Muted, k.s.P.Off
	}
	k.nl()
	line := "  " + b + text + r
	if note != "" {
		line += " " + mu + k.s.G.Dot + " " + note + r
	}
	write(k.w, line+"\n")
}

func (k cardWriter) topicsPage() {
	k.title("gash help TOPIC", "pages that are not commands")
	k.nl()
	k.s.Table(k.w, []string{
		"shortcuts\tevery shortcut: of the commands and of the packs",
		"shell-defaults\tthe aliases that change a system command (rm -I, ls --color)",
		"scripting\tneeds_help, needs_confirm_prompt and path_add for your scripts",
		"migrate\tthe former flat names and their commands",
	}, "--no-header", "--indent", "2", "--color", "1:accent")
}

// shortcutsPage lists every command shortcut, then every pack shortcut.
func (k cardWriter) shortcutsPage() {
	k.title("Shortcuts", "aliases that expand to a command; GASH_SHORTCUTS picks the packs")
	rows := []string{"SHORTCUT\tCOMMAND"}
	for _, sc := range registry.Shortcuts() {
		cmd, _ := registry.CommandOfShortcut(sc)
		if k.only {
			if _, ok := k.live[sc]; !ok {
				continue
			}
		}
		rows = append(rows, sc+"\t"+cmd.Name())
	}
	k.nl()
	k.s.Table(k.w, rows, "--indent", "2", "--color", "1:accent")
	for _, ns := range registry.Namespaces() {
		var entries []string
		var files []string
		for _, p := range ns.Packs {
			if !shortcutPacks[p] {
				continue
			}
			files = append(files, p)
			for _, line := range strings.Split(strings.TrimSuffix(k.aliasLines(p), "\n"), "\n") {
				if name, value := readTab(line); name != "" {
					if _, isCmd := registry.CommandOfShortcut(name); !isCmd {
						if _, isFn := registry.Lookup(name); !isFn {
							entries = append(entries, name+"\t"+value)
						}
					}
				}
			}
		}
		if len(entries) == 0 {
			continue
		}
		k.nl()
		k.s.Heading(k.w, "--indent", "2", "--note", "gash "+ns.Name, strings.ToUpper(strings.Join(files, ", ")))
		k.grid(entries, files)
	}
}

// shellDefaultsPage lists the packs that change system commands.
func (k cardWriter) shellDefaultsPage() {
	k.title("Shell defaults", "aliases that change what a system command does")
	k.nl()
	for _, l := range []string{
		"An interactive shell gets these unless a setting turns them off:",
		"GASH_SAFETY_ALIASES=0 drops the safety pack (cp -i, mv -i, rm -I, chmod",
		"--preserve-root), GASH_REPLACE_TOOLS=0 keeps php, composer and less as",
		"they are. They are not shortcuts: the command keeps its own name.",
	} {
		write(k.w, "  "+l+"\n")
	}
	for _, p := range []string{"safety", "colors", "general", "navigation"} {
		var entries []string
		for _, line := range strings.Split(strings.TrimSuffix(k.aliasLines(p), "\n"), "\n") {
			if name, value := readTab(line); name != "" {
				entries = append(entries, name+"\t"+value)
			}
		}
		if len(entries) == 0 {
			continue
		}
		k.nl()
		k.s.Heading(k.w, "--indent", "2", strings.ToUpper(p))
		k.grid(entries, []string{p})
	}
}

// scriptingPage shows the helpers a user's script may call.
func (k cardWriter) scriptingPage() {
	k.title("Script helpers", "functions for your scripts and ~/.bash_local, not commands")
	for _, name := range []string{"needs_help", "needs_confirm_prompt", "path_add"} {
		if f, ok := registry.Lookup(name); ok && !help.Display(k.w, k.s.Color, f) {
			k.nl()
			write(k.w, "  "+name+"\n")
		}
	}
}

// migratePage is the table of the former flat names.
func (k cardWriter) migratePage() {
	k.title("From the flat names to the commands", "Gash 4 defines the former names only with GASH_LEGACY_NAMES=1")
	type row struct{ old, now, sc string }
	var rows []row
	for i := range registry.Tree() {
		cmd := &registry.Tree()[i]
		for _, f := range cmd.FormerNames() {
			rows = append(rows, row{f, cmd.Name(), cmd.Shortcut})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].old < rows[j].old })
	lines := []string{"FORMER NAME\tCOMMAND\tSHORTCUT"}
	for _, r := range rows {
		lines = append(lines, r.old+"\t"+r.now+"\t"+r.sc)
	}
	k.nl()
	k.s.Table(k.w, lines, "--indent", "2", "--color", "2:accent", "--color", "3:accent")
	k.nl()
	write(k.w, "  The llm_* and agent_* names are not former names: they stay, as the\n")
	write(k.w, "  stable names of the agent tools (gash llm read is llm_read).\n")
}

// list is gash help --list: every command by namespace.
func (k cardWriter) list() {
	for _, ns := range registry.Namespaces() {
		cmds := registry.CommandsUnder(ns.Name)
		if len(cmds) == 0 {
			continue
		}
		k.nl()
		k.s.Heading(k.w, "--indent", "2", "--note", ns.Summary, "gash "+ns.Name)
		var rows []string
		for _, cmd := range cmds {
			rows = append(rows, strings.Join(cmd.Path[1:], " ")+"\t"+cmd.Shortcut+"\t"+cmd.Short())
		}
		k.s.Table(k.w, rows, "--no-header", "--indent", "4", "--fit", "--color", "1:accent", "--color", "2:accent", "--color", "3:muted")
	}
	k.nl()
}

// search is gash help --search WORD: the commands whose names, shortcut,
// former names, description or help text hold word (case folded).
func (k cardWriter) search(word string) int {
	kw := strings.ToLower(word)
	var rows []string
	for i := range registry.Tree() {
		cmd := &registry.Tree()[i]
		if cmd.Hidden {
			continue
		}
		text := cmd.Name() + "\n" + cmd.Shortcut + "\n" + strings.Join(cmd.FormerNames(), " ") + "\n" + cmd.Short() + "\n" + cmd.Handler
		if f, ok := cmd.Function(); ok && f.HasHelp {
			text += "\n" + registry.CanonicalHelp(f).Body
		}
		if strings.Contains(strings.ToLower(text), kw) {
			rows = append(rows, cmd.Name()+"\t"+cmd.Shortcut+"\t"+cmd.Short())
		}
	}
	k.nl()
	if len(rows) == 0 {
		write(k.w, "  No results for '"+word+"'.\n\n")
		return 1
	}
	k.s.Heading(k.w, "--indent", "2", "Search results for '"+word+"':")
	k.nl()
	k.s.Table(k.w, rows, "--no-header", "--indent", "2", "--fit", "--color", "1:accent", "--color", "2:accent", "--color", "3:muted")
	k.nl()
	return 0
}
