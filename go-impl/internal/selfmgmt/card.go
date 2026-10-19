// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// CardAliasesVar carries the caller's alias values from the gash stub:
// one NAME<TAB>VALUE line per card name the shell defines as an alias,
// with backslash and newline escaped as \\ and \n.
const CardAliasesVar = "GASH__CARD_ALIASES"

// card is gash [TOPIC|all] [--no-color] (lib/modules/gash.sh): the
// reference card built from the help registry and the alias files. `gash
// migrate` runs gash_migrate, -h shows the registered help, an unknown
// topic is an error.
func (m *Module) card(c *invoke.Call) int {
	if c.Arg(0) == "migrate" {
		return c.Handlers.Run(&invoke.Call{
			Ctx: c.Ctx, Name: "gash_migrate", Args: c.Args[1:], Env: c.Env,
			Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr, Term: c.Term,
		})
	}
	noColor, topic := false, ""
	for _, a := range c.Args {
		switch a {
		case "--no-color":
			noColor = true
		case "-h", "--help":
			topic = "--help"
		default:
			topic = a
		}
	}
	color := c.Term.WantColors(noColor)
	w := c.Stdout
	if topic == "--help" {
		if f, ok := registry.Lookup("gash"); ok {
			help.Display(w, color, f)
		}
		return 0
	}
	switch topic {
	case "navigation":
		topic = "nav"
	case "management":
		topic = "gash"
	case "php", "node":
		topic = "dev"
	}
	topics := registry.Topics()
	var chosen registry.Topic
	if topic != "" && topic != "all" {
		var names []string
		known := false
		for _, t := range topics {
			names = append(names, t.Name)
			if t.Name == topic && !known {
				chosen, known = t, true
			}
		}
		if !known {
			ui.NewPrinter(c.Stderr, c.Term, noColor).
				Error("Unknown topic: " + topic + ". Topics: " + strings.Join(names, " ") + ", all")
			return 1
		}
	}

	// __gash_want_colors "$no_color" || eval "$__GASH_COLOR_OFF_LOCALS".
	s := ui.StyleOf(c.Term, !color)
	k := cardOf(s, w, liveAliases(c.Env.Get(CardAliasesVar)))
	p, b, mu, r := "", "", "", ""
	if s.Color {
		p, b, mu, r = s.P.Primary, s.P.Bold, s.P.Muted, s.P.Off
	}
	dot := s.G.Dot
	k.nl()
	s.Box(w, p,
		b+"Gash"+r+" "+c.Env.Get("GASH_VERSION")+" "+mu+dot+" Bash for humans and AI agents"+r,
		mu+strconv.Itoa(len(help.Registered()))+" functions "+dot+" gash <topic> "+dot+" gash_help <name>"+r)

	switch topic {
	case "":
	case "all":
		for _, t := range topics {
			k.topic(t)
		}
		k.nl()
		return 0
	default:
		k.topic(chosen)
		k.nl()
		return 0
	}

	// Overview: one row per topic, then where to go next.
	k.nl()
	var rows strings.Builder
	rows.WriteString("TOPIC\tFUNCTIONS\tALIASES\tWHAT\n")
	for _, t := range topics {
		na := 0
		for _, f := range t.AliasFiles {
			na += nonEmptyLines(k.aliasLines(f))
		}
		rows.WriteString(t.Name + "\t" + strconv.Itoa(len(cardFunctions(t.Modules))) + "\t" + strconv.Itoa(na) + "\t" + t.Description + "\n")
	}
	s.Table(w, ui.ReadRows(strings.NewReader(rows.String())), "--indent", "2", "--right", "2,3", "--fit", "--color", "1:accent")
	k.nl()
	s.Table(w, []string{
		"gash <topic>\tfunctions and aliases of a topic, e.g. gash git",
		"gash all\tthe whole card",
		"gash_help <name>\tfull help with examples, e.g. gash_help flf",
		"gash_help --search\tsearch every help page",
		"gash_doctor\tcheck the installation",
	}, "--no-header", "--indent", "2", "--color", "1:accent")
	k.nl()
	return 0
}

// cardWriter renders the card of one call.
type cardWriter struct {
	s    ui.Style
	w    io.Writer
	live map[string]string
}

func cardOf(s ui.Style, w io.Writer, live map[string]string) cardWriter {
	return cardWriter{s: s, w: w, live: live}
}

func (k cardWriter) nl() { _, _ = io.WriteString(k.w, "\n") }

// liveAliases decodes CardAliasesVar: the alias values of the caller's
// shell (BASH_ALIASES) by name; empty when the binary runs without the
// stub, and the card shows the values the alias files write.
func liveAliases(v string) map[string]string {
	live := map[string]string{}
	for _, line := range strings.Split(v, "\n") {
		name, value, ok := strings.Cut(line, "\t")
		if !ok || name == "" {
			continue
		}
		live[name] = unescapeLine(value)
	}
	return live
}

// unescapeLine undoes the stubs' escaping of one value: \\ is a
// backslash and \n a newline.
func unescapeLine(value string) string {
	if !strings.Contains(value, "\\") {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			if value[i] == 'n' {
				b.WriteByte('\n')
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

// readTab is IFS=$'\t' read -r name value on one line: tabs around the
// fields dropped, the value keeps the tabs inside it.
func readTab(line string) (name, value string) {
	line = strings.TrimLeft(line, "\t")
	name, value, _ = strings.Cut(line, "\t")
	return name, strings.Trim(value, "\t")
}

// aliasLines is the output of __gash_card_aliases FILE: the entries of
// the file with the value the caller's shell gives a defined alias, the
// identity ones ("ls -> ls") left out.
func (k cardWriter) aliasLines(file string) string {
	var b strings.Builder
	for _, a := range registry.CardAliases(file) {
		name, value := readTab(a.Name + "\t" + a.Value)
		if v, ok := k.live[name]; ok {
			value = v
		}
		if value == name {
			continue
		}
		b.WriteString(name + "\t" + value + "\n")
	}
	return b.String()
}

// nonEmptyLines is ... | grep -c .
func nonEmptyLines(text string) int {
	n := 0
	for _, l := range strings.Split(text, "\n") {
		if l != "" {
			n++
		}
	}
	return n
}

// cardFunctions is __gash_card_functions MODULES: the functions whose
// help module is one of modules, sorted by bytes (LEDGER SELF-11).
func cardFunctions(modules []string) []*registry.Function {
	var out []*registry.Function
	for _, f := range help.Registered() {
		if f.Help.Module != "" && slices.Contains(modules, f.Help.Module) {
			out = append(out, f)
		}
	}
	return out
}

// cardUsage is __gash_card_usage FUNCTION: the first non-empty line after
// the USAGE line of the help text, without its indentation and without
// the function name in front.
func cardUsage(f *registry.Function) string {
	found := false
	for _, line := range strings.Split(f.Help.Body, "\n") {
		if found {
			line = strings.TrimLeft(line, " \t\n\v\f\r")
			if line == "" {
				continue
			}
			if rest, ok := strings.CutPrefix(line, f.Name); ok {
				line = strings.TrimPrefix(rest, " ")
			}
			return line
		}
		if strings.HasPrefix(line, "USAGE") {
			found = true
		}
	}
	return ""
}

// topic is __gash_card_topic: the heading, the functions table, then the
// alias grid.
func (k cardWriter) topic(t registry.Topic) {
	s, w := k.s, k.w
	k.nl()
	s.Heading(w, "--indent", "2", "--note", t.Description, strings.ToUpper(t.Name))
	k.nl()

	fns := cardFunctions(t.Modules)
	if len(fns) > 0 {
		var rows strings.Builder
		rows.WriteString("FUNCTION\tALIAS\tUSAGE\tDESCRIPTION\n")
		for _, f := range fns {
			usage := cardUsage(f)
			if ui.Len(usage) > 30 {
				usage = ui.Sub(usage, 0, 29) + s.G.Ellipsis
			}
			rows.WriteString(f.Name + "\t" + strings.ReplaceAll(f.Help.Aliases, " ", ", ") + "\t" + usage + "\t" + f.Help.Short + "\n")
		}
		s.Table(w, ui.ReadRows(strings.NewReader(rows.String())),
			"--indent", "2", "--fit", "--color", "1:accent", "--color", "2:accent", "--color", "3:muted")
	}

	var entries []string
	for _, f := range t.AliasFiles {
		text := strings.TrimSuffix(k.aliasLines(f), "\n")
		if text == "" {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			if name, value := readTab(line); name != "" {
				entries = append(entries, name+"\t"+value)
			}
		}
	}
	if len(entries) == 0 {
		return
	}

	// Grid: as many "name  expansion" pairs per row as the terminal holds,
	// names accent, expansions muted (cut at 26 characters).
	namew, valw := 0, 0
	for _, e := range entries {
		name, value, _ := strings.Cut(e, "\t")
		namew = max(namew, ui.Len(name))
		valw = max(valw, ui.Len(value))
	}
	valw = min(valw, 26)
	cols := (s.Width - 2) / (namew + 2 + valw + 3)
	cols = max(min(cols, 3), 1)

	if len(fns) > 0 {
		k.nl()
	}
	s.Heading(w, "--indent", "2", "Aliases")
	// Pairs are columns 1-2, 4-5, 7-8; 3 and 6 stay empty, as spacers.
	opts := []string{"--no-header", "--indent", "2"}
	for c := 0; c < cols; c++ {
		opts = append(opts, "--color", strconv.Itoa(3*c+1)+":accent", "--color", strconv.Itoa(3*c+2)+":muted",
			"--max", strconv.Itoa(3*c+2)+":"+strconv.Itoa(valw))
	}
	var rows strings.Builder
	row := ""
	for i, e := range entries {
		if row != "" {
			row += "\t\t"
		}
		row += e
		if (i+1)%cols == 0 || i+1 == len(entries) {
			rows.WriteString(row + "\n")
			row = ""
		}
	}
	s.Table(w, ui.ReadRows(strings.NewReader(rows.String())), opts...)
}
