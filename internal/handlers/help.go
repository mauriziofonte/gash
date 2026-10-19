// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"io"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

func registerHelp(t *invoke.Table) {
	t.Handle("gash_help", gashHelp)
}

// gashHelp is gash_help [--no-color] [FUNCTION|ALIAS|--list|--search
// KEYWORD|--short FUNCTION|-h]. --bash TOPIC stays in the generated shell
// function, which runs Bash's builtin help.
func gashHelp(c *invoke.Call) int {
	noColor := false
	var args []string
	for _, a := range c.Args {
		if a == "--no-color" {
			noColor = true
			continue
		}
		args = append(args, a)
	}
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	s := ui.StyleOf(c.Term, noColor)
	p := s.Printer(c.Stderr)
	w := c.Stdout
	// A, M, R of 2.x: the palette in scope, when __gash_use_color.
	a, m, r := "", "", ""
	if s.Color {
		a, m, r = s.P.Accent, s.P.Muted, s.P.Off
	}
	// The help pages take the gate with the palette: a --no-color call
	// shadowed the colors to empty strings.
	pageColor := s.Color && !noColor
	nl := func() { write(w, "\n") }

	switch first := arg(0); first {
	case "--bash":
		p.Error("gash_help --bash is Bash's builtin help: run it from a shell that sourced gash.sh.")
		return 1
	case "--list":
		nl()
		s.Heading(w, "--indent", "2", "--note", "(use 'gash_help FUNCTION' for details)", "Gash Functions")
		help.List(s, w)
		nl()
		return 0
	case "--search":
		keyword := arg(1)
		if keyword == "" {
			p.Usage("gash_help --search KEYWORD")
			return 1
		}
		nl()
		s.Heading(w, "--indent", "2", "Search results for '"+keyword+"':")
		nl()
		rc := 0
		if !help.Search(s, w, keyword) {
			rc = 1
		}
		nl()
		return rc
	case "--short":
		name := arg(1)
		if name == "" {
			p.Usage("gash_help --short FUNCTION")
			return 1
		}
		if f, ok := registry.Lookup(name); ok && f.HasHelp && f.Help.Short != "" {
			write(w, f.Help.Short+"\n")
			return 0
		}
		if f, ok := help.FindByAlias(name); ok && f.Help.Short != "" {
			write(w, f.Help.Short+"\n")
			return 0
		}
		p.Error("No help found for '" + name + "'.")
		return 1
	case "-h", "--help":
		nl()
		write(w, "  "+a+"gash_help"+r+m+"  (module: gash)"+r+"\n")
		nl()
		s.Heading(w, "--indent", "2", "Gash help system with examples, search, and discovery.")
		nl()
		s.Heading(w, "--indent", "2", "USAGE")
		s.Table(w, []string{
			"gash_help\tOverview of all functions",
			"gash_help FUNCTION\tDetailed help with examples",
			"gash_help ALIAS\tLookup by short alias name",
			"gash_help --list\tList all functions by module",
			"gash_help --search KEYWORD\tSearch across all help text",
			"gash_help --short FUNCTION\tOne-line description only",
			"gash_help --bash TOPIC\tBash builtin help",
		}, "--no-header", "--indent", "4", "--color", "1:accent", "--color", "2:muted")
		nl()
		return 0
	case "":
		version := c.Env.Get("GASH_VERSION")
		if version == "" {
			version = "?"
		}
		nl()
		s.Heading(w, "--indent", "2", "--note", "v"+version+" - Gash, Another SHell!", "Gash")
		nl()
		s.Table(w, []string{
			"gash_help FUNCTION\tDetailed help with examples",
			"gash_help ALIAS\tLookup by short alias (e.g. flf, dsa, ask)",
			"gash_help --list\tList all functions by module",
			"gash_help --search KEYWORD\tSearch across all help text",
			"gash\tReference card with aliases",
			"FUNCTION --help\tQuick usage reminder",
		}, "--no-header", "--indent", "2", "--color", "1:accent", "--color", "2:muted")
		help.List(s, w)
		nl()
		write(w, "  "+m+"Tip: Every function supports -h/--help for a quick usage reminder."+r+"\n")
		write(w, "  "+m+"     Use gash_help FUNCTION for detailed help with real-world examples."+r+"\n")
		nl()
		return 0
	default:
		if f, ok := registry.Lookup(first); ok && help.Display(w, pageColor, f) {
			return 0
		}
		if f, ok := help.FindByAlias(first); ok {
			write(w, "\n  "+m+"'"+first+"' is an alias for '"+f.Name+"'"+r+"\n")
			help.Display(w, pageColor, f)
			return 0
		}
		p.Error("No help found for '" + first + "'.")
		p.Hint("Try: gash_help --list  or  gash_help --search " + first)
		return 1
	}
}

func write(w io.Writer, s string) { _, _ = io.WriteString(w, s) }
