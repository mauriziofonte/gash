// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// CardAliasesVar carries the caller's alias values from the gash stub:
// one NAME<TAB>VALUE line per card name the shell defines as an alias,
// with backslash and newline escaped as \\ and \n.
const CardAliasesVar = "GASH__CARD_ALIASES"

// CardLiveVar is "1" when the gash stub runs in a shell that is not
// headless: the card then counts and lists only the aliases of
// CardAliasesVar, the ones the shell defines (3.0: SELF-12 fixed; 2.x
// counted and listed every alias of the alias files, also those the shadow
// rule dropped and the packs whose tool is absent). Without it (a headless
// shell, a script calling the binary) the card shows every registered
// alias, as 2.x did.
const CardLiveVar = "GASH__CARD_LIVE"

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
	// gash -h and gash NS --help are the pages themselves.
	words, noColor, _ := pageWords(c.Args)
	color := c.Term.WantColors(noColor)
	s := ui.StyleOf(c.Term, !color)
	k := cardOf(s, c.Stdout, liveAliases(c.Env.Get(CardAliasesVar)), c.Env.Get(CardLiveVar) == "1")
	return m.pages(c, k, words)
}

// cardWriter renders the card of one call.
type cardWriter struct {
	s    ui.Style
	w    io.Writer
	live map[string]string
	only bool // list only the aliases of live (CardLiveVar)
}

func cardOf(s ui.Style, w io.Writer, live map[string]string, only bool) cardWriter {
	return cardWriter{s: s, w: w, live: live, only: only}
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
// identity ones ("ls -> ls") left out, and with k.only those the shell
// does not define.
func (k cardWriter) aliasLines(file string) string {
	var b strings.Builder
	for _, a := range registry.CardAliases(file) {
		if a.Each != "" && k.only {
			// A family entry (phpXY): the shell's own php83, php74, ...
			// (LEDGER DEV-21).
			for _, name := range k.family(a.Each) {
				b.WriteString(name + "\t" + k.live[name] + "\n")
			}
			continue
		}
		name, value := readTab(a.Name + "\t" + a.Value)
		if v, ok := k.live[name]; ok {
			value = v
		} else if k.only {
			continue
		}
		if value == name {
			continue
		}
		b.WriteString(name + "\t" + value + "\n")
	}
	return b.String()
}

// family returns the live aliases named prefix followed by digits only,
// sorted by bytes.
func (k cardWriter) family(prefix string) []string {
	var out []string
	for name := range k.live {
		rest, ok := strings.CutPrefix(name, prefix)
		if ok && rest != "" && strings.Trim(rest, "0123456789") == "" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// aliasNotes are the conditions of the card entries of files that
// aliasLines shows, one "NAMES: CONDITION" line per condition in file
// order (LEDGER DEV-21). A card that lists only the aliases the shell
// defines needs none: those are defined.
func (k cardWriter) aliasNotes(files []string) []string {
	if k.only {
		return nil
	}
	var order []string
	names := map[string][]string{}
	for _, f := range files {
		for _, a := range registry.CardAliases(f) {
			if a.Condition == "" || a.Value == a.Name {
				continue
			}
			if names[a.Condition] == nil {
				order = append(order, a.Condition)
			}
			names[a.Condition] = append(names[a.Condition], a.Name)
		}
	}
	out := make([]string, 0, len(order))
	for _, c := range order {
		out = append(out, strings.Join(names[c], ", ")+": "+c)
	}
	return out
}

// grid writes alias entries (NAME<TAB>VALUE) as many pairs per row as
// the terminal holds, names accent, expansions muted (cut at 26
// characters), then the conditions of the files' entries.
func (k cardWriter) grid(entries, files []string) {
	s, w := k.s, k.w
	namew, valw := 0, 0
	for _, e := range entries {
		name, value, _ := strings.Cut(e, "\t")
		namew = max(namew, ui.Len(name))
		valw = max(valw, ui.Len(value))
	}
	valw = min(valw, 26)
	cols := (s.Width - 2) / (namew + 2 + valw + 3)
	cols = max(min(cols, 3), 1)
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
	if notes := k.aliasNotes(files); len(notes) > 0 {
		k.nl()
		for _, note := range notes {
			s.Print(w, "muted", "  "+note)
		}
	}
}
